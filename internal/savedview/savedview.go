// Package savedview loads an immutable saved Saga view: the technical
// inventory exactly as committed at one commit of the Saga's canonical source
// repository. It exists so a new historical documentation pin can be admitted
// only against an explicit, verifiable view (technicalpolicy.AdmitPin), never
// by accepting an arbitrary old revision. It performs no writes to the Saga.
package savedview

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// Reason is a stable refusal code for a view that cannot be loaded.
type Reason string

const (
	CommitUnavailable Reason = "view_commit_unavailable"
	SagaMissing       Reason = "view_saga_missing"
	SourceMismatch    Reason = "view_source_mismatch"
	InventoryInvalid  Reason = "view_inventory_invalid"
)

// Error reports why a view could not be established. There is no fallback.
type Error struct {
	Reason  Reason
	Message string
}

func (e *Error) Error() string { return string(e.Reason) + ": " + e.Message }

// MaxViewFiles and MaxViewBytes bound the inventory extracted from one view.
const (
	MaxViewFiles = 200_000
	MaxViewBytes = 256 << 20
)

// View is one loaded saved view. SourceCommit equals Commit: the Saga is
// committed inside its canonical source repository, so the saved Saga and the
// source it documents are the same immutable commit.
type View struct {
	Commit       string
	Artifact     string // git:<commit>:<saga path relative to the repository>
	SourceCommit string
	Manifest     saga.Manifest
	Inventory    requirements.Inventory
}

// Load resolves rev once to a full commit in checkout, requires the Saga at
// sagaRoot to be committed there with the same identity and source
// repository, and loads its inventory at that commit with the current strict
// reader. checkout must already be verified as the canonical source checkout.
func Load(ctx context.Context, checkout, sagaRoot string, current saga.Manifest, rev string) (View, error) {
	fail := func(reason Reason, format string, args ...any) (View, error) {
		return View{}, &Error{Reason: reason, Message: fmt.Sprintf(format, args...)}
	}
	top, err := git(ctx, checkout, "rev-parse", "--show-toplevel")
	if err != nil {
		return fail(CommitUnavailable, "%s is not a Git checkout", checkout)
	}
	absSaga, err := filepath.Abs(sagaRoot)
	if err != nil {
		return View{}, err
	}
	top = strings.TrimSpace(top)
	if resolved, err := filepath.EvalSymlinks(absSaga); err == nil {
		absSaga = resolved
	}
	if resolved, err := filepath.EvalSymlinks(top); err == nil {
		top = resolved
	}
	rel, err := filepath.Rel(top, absSaga)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fail(SagaMissing, "the Saga is not inside its canonical source checkout, so no commit of it can be a saved view; saved views are unavailable for Sagas kept in a companion repository")
	}
	rel = filepath.ToSlash(rel)
	oid, err := git(ctx, checkout, "rev-parse", "--verify", "--end-of-options", rev+"^{commit}")
	oid = strings.TrimSpace(oid)
	if err != nil || !coderef.ValidCommit(oid) {
		return fail(CommitUnavailable, "view %q is not an available commit", rev)
	}
	if _, err := git(ctx, checkout, "cat-file", "-e", oid+"^{tree}"); err != nil {
		return fail(CommitUnavailable, "view %s tree is unavailable (shallow or partial clone?)", oid)
	}
	prefix := rel
	if prefix == "." {
		prefix = ""
	}
	manifestPath := path.Join(prefix, saga.ManifestName)
	data, err := git(ctx, checkout, "show", oid+":"+manifestPath)
	if err != nil {
		return fail(SagaMissing, "no %s at %s:%s", saga.ManifestName, oid, manifestPath)
	}
	var manifest saga.Manifest
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || manifest.Version != saga.SagaVersion {
		return fail(SagaMissing, "the Saga manifest at %s is not a readable v%d manifest", oid, saga.SagaVersion)
	}
	if manifest.ID != current.ID || manifest.Source.Repository != current.Source.Repository {
		return fail(SourceMismatch, "the Saga at %s has identity %q/%q, not %q/%q", oid, manifest.ID, manifest.Source.Repository, current.ID, current.Source.Repository)
	}
	temp, err := os.MkdirTemp("", "change-saga-view-")
	if err != nil {
		return View{}, err
	}
	defer os.RemoveAll(temp)
	if err := extractTree(ctx, checkout, oid, path.Join(prefix, requirements.InventoryDir), filepath.Join(temp, requirements.InventoryDir)); err != nil {
		return fail(InventoryInvalid, "%v", err)
	}
	inventory, err := requirements.LoadInventory(temp, manifest.ID)
	if err != nil {
		return fail(InventoryInvalid, "inventory at %s: %v", oid, err)
	}
	return View{Commit: oid, Artifact: "git:" + oid + ":" + rel, SourceCommit: oid, Manifest: manifest, Inventory: inventory}, nil
}

// Facts projects the view's facts for one requested target, for AdmitPin.
func (v View) Facts(target string) technicalpolicy.SavedView {
	facts := technicalpolicy.SavedView{Artifact: v.Artifact, SourceCommit: v.SourceCommit, Resolved: true, Definition: technicalpolicy.DefinitionMissing}
	record := v.Inventory.Find(target)
	if record == nil {
		return facts
	}
	facts.Definition = technicalpolicy.DefinitionConflicted
	if record.CurrentRevision != nil {
		facts.Definition = technicalpolicy.DefinitionUnique
		facts.Binding = technicalpolicy.Pin{Target: target, Revision: target + ":revision:" + record.CurrentRevision.ID}
	}
	facts.Lifecycle = technicalpolicy.LifecycleConflicted
	if record.CurrentLifecycle != nil {
		facts.Lifecycle = technicalpolicy.Lifecycle(record.CurrentLifecycle.State)
	}
	return facts
}

// GlobalHealth projects current global facts for one target.
func GlobalHealth(d *requirements.Inventory, target string) technicalpolicy.GlobalHealth {
	health := technicalpolicy.GlobalHealth{Definition: technicalpolicy.DefinitionMissing}
	record := d.Find(target)
	if record == nil {
		return health
	}
	health.Definition = technicalpolicy.DefinitionConflicted
	if record.CurrentRevision != nil {
		health.Definition = technicalpolicy.DefinitionUnique
		health.Current = technicalpolicy.Pin{Target: target, Revision: target + ":revision:" + record.CurrentRevision.ID}
	}
	health.Lifecycle = technicalpolicy.LifecycleConflicted
	if record.CurrentLifecycle != nil {
		health.Lifecycle = technicalpolicy.Lifecycle(record.CurrentLifecycle.State)
	}
	return health
}

// extractTree copies the regular files beneath treePath at commit into dest.
// A missing tree is an empty inventory, not an error.
func extractTree(ctx context.Context, checkout, commit, treePath, dest string) error {
	listing, err := git(ctx, checkout, "ls-tree", "-r", "-z", "-l", "--full-tree", commit, "--", treePath)
	if err != nil {
		return err
	}
	type blob struct{ oid, path string }
	blobs := []blob{}
	total := 0
	for _, entry := range strings.Split(listing, "\x00") {
		if entry == "" {
			continue
		}
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 {
			return fmt.Errorf("unexpected tree entry %q", entry)
		}
		if fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			return fmt.Errorf("%s is not a regular file in the saved view", name)
		}
		size, err := strconv.Atoi(fields[3])
		if err != nil {
			return err
		}
		total += size
		if len(blobs) >= MaxViewFiles || total > MaxViewBytes {
			return fmt.Errorf("saved inventory exceeds %d files or %d bytes", MaxViewFiles, MaxViewBytes)
		}
		rel := strings.TrimPrefix(name, treePath+"/")
		if rel == name || strings.Contains(rel, "..") {
			return fmt.Errorf("unexpected tree path %q", name)
		}
		blobs = append(blobs, blob{fields[2], rel})
	}
	if len(blobs) == 0 {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", checkout, "cat-file", "--batch")
	var input bytes.Buffer
	for _, b := range blobs {
		input.WriteString(b.oid + "\n")
	}
	cmd.Stdin = &input
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Git blocks writing blobs nobody reads once the pipe fills (4 KiB on
	// Windows), so a failed extraction stops it before waiting.
	abort := func(err error) error {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	reader := bufio.NewReader(stdout)
	for _, b := range blobs {
		header, err := reader.ReadString('\n')
		if err != nil {
			return abort(err)
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			return abort(fmt.Errorf("blob %s is unavailable", b.oid))
		}
		size, _ := strconv.Atoi(fields[2])
		data := make([]byte, size+1)
		if _, err := io.ReadFull(reader, data); err != nil {
			return abort(err)
		}
		target := filepath.Join(dest, filepath.FromSlash(b.path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return abort(err)
		}
		if err := os.WriteFile(target, data[:size], 0o644); err != nil {
			return abort(err)
		}
	}
	return cmd.Wait()
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitexec.Output(ctx, append([]string{"-C", dir}, args...)...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", errors.New(strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}
