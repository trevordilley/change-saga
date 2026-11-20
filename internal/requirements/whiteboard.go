package requirements

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

const WhiteboardDir = "___whiteboards"
const MaxWhiteboards = 10_000
const WhiteboardSchemaURL = "https://changesaga.dev/schema/v5/whiteboard-revision.schema.json"

// Compatibility projections retain legacy Go type names for readers. Current
// documentation is stored in one editable current.json; identity stays stable,
// and Git supplies earlier values. Legacy graphs are read only for migration.
type WhiteboardRevision struct {
	Schema    string           `json:"$schema"`
	Version   int              `json:"version"`
	ID        string           `json:"id"`
	Parents   []string         `json:"parents"`
	Name      string           `json:"name"`
	Document  diagram.Document `json:"document"`
	CreatedAt time.Time        `json:"created_at"`
}

type ResolvedWhiteboard struct {
	Revision    WhiteboardRevision   `json:"revision"`
	Composition *diagram.Composition `json:"-"`
	SVG         []byte               `json:"-"`
}

// WhiteboardContentDigest names authored content, including exact nested pins,
// independently of the legacy storage identity used by a compatibility reader.
func WhiteboardContentDigest(value WhiteboardRevision) (string, error) {
	return currentrecord.Digest(value)
}

// ListWhiteboardIDs lists named documents without loading their revision histories.
// The optional root and every entry must be real directories with stable IDs.
// Store staging directories are unpublished and do not consume document slots.
func ListWhiteboardIDs(root string) ([]string, error) {
	dir := filepath.Join(root, WhiteboardDir)
	present, err := realDirectory(dir)
	if err != nil || !present {
		return nil, err
	}
	// Allow bounded staging debris as well as the document limit. CommitDir
	// stages beside the final board, including while unlocked readers list it.
	entries, err := boundedReadDir(dir, 2*MaxWhiteboards+1)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == WhiteboardMetadataName && entry.Type().IsRegular() {
			continue
		}
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".change-saga-write-") {
			continue
		}
		if !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("whiteboard entry %q must be a real directory with a valid board ID", entry.Name())
		}
		if strings.HasPrefix(entry.Name(), ".change-saga-stage-") {
			continue
		}
		if !livingid.ValidID(entry.Name()) {
			return nil, fmt.Errorf("whiteboard entry %q must have a valid board ID", entry.Name())
		}
		ids = append(ids, entry.Name())
		if len(ids) > MaxWhiteboards {
			return nil, fmt.Errorf("whiteboard document count exceeds maximum of %d", MaxWhiteboards)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

func whiteboardHistory(root, sagaID, boardID string) (map[string]WhiteboardRevision, []string, error) {
	if !livingid.ValidID(sagaID) || !livingid.ValidID(boardID) {
		return nil, nil, fmt.Errorf("whiteboard requires valid saga and board IDs")
	}
	for _, dir := range []string{filepath.Join(root, WhiteboardDir), filepath.Join(root, WhiteboardDir, boardID)} {
		present, err := realDirectory(dir)
		if err != nil || !present {
			return nil, nil, err
		}
	}
	dir := filepath.Join(root, WhiteboardDir, boardID)
	current, err := currentrecord.Read(dir)
	if err != nil {
		return nil, nil, err
	}
	if current != nil {
		var value WhiteboardRevision
		value.Schema = WhiteboardSchemaURL
		value.Version = 1
		value.Parents = []string{}
		if err := currentrecord.Project(current.Content, &value); err != nil {
			return nil, nil, err
		}
		digest, err := currentrecord.Digest(current.Content)
		if err != nil {
			return nil, nil, err
		}
		value.ID = "sha256-" + strings.TrimPrefix(digest, "sha256:")
		if value.Name == "" {
			return nil, nil, fmt.Errorf("whiteboard name is required")
		}
		if err := value.Document.Validate(); err != nil {
			return nil, nil, err
		}
		if err := validateWhiteboardItems(sagaID, value.Document); err != nil {
			return nil, nil, err
		}
		return map[string]WhiteboardRevision{value.ID: value}, []string{value.ID}, nil
	}
	present, err := realDirectory(filepath.Join(dir, "revisions"))
	if err != nil {
		return nil, nil, err
	}
	if !present {
		return nil, nil, fmt.Errorf("whiteboard directory %s is missing", filepath.Join(dir, "revisions"))
	}
	entries, err := boundedReadDir(dir, 1)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if entry.Name() != "revisions" {
			return nil, nil, fmt.Errorf("unknown whiteboard entry %s", entry.Name())
		}
	}
	dir = filepath.Join(dir, "revisions")
	entries, err = whiteboardRevisionFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("whiteboard has no revisions")
	}
	revisions := map[string]WhiteboardRevision{}
	parents := map[string][]string{}
	for _, entry := range entries {
		var v WhiteboardRevision
		if err := readInventoryJSON(filepath.Join(dir, entry.Name()), &v); err != nil {
			return nil, nil, fmt.Errorf("whiteboard %s: %w", entry.Name(), err)
		}
		if err := validateWhiteboardRevision(v, strings.TrimSuffix(entry.Name(), ".json")); err != nil {
			return nil, nil, err
		}
		if err := validateWhiteboardItems(sagaID, v.Document); err != nil {
			return nil, nil, err
		}
		revisions[v.ID], parents[v.ID] = v, v.Parents
	}
	for id, v := range revisions {
		for _, parent := range v.Parents {
			if _, ok := revisions[parent]; !ok {
				return nil, nil, fmt.Errorf("whiteboard revision %s has missing parent %s", id, parent)
			}
		}
	}
	heads, roots, cycle := graphHeads(parents, mapKeys(revisions))
	if cycle || len(roots) != 1 {
		return nil, nil, fmt.Errorf("whiteboard revisions require an acyclic graph with one root")
	}
	return revisions, heads, nil
}

func whiteboardRevisionFiles(dir string) ([]fs.DirEntry, error) {
	// WriteFile publishes through a regular same-directory temporary file and
	// an atomic link. Bound temporary debris separately from saved revisions.
	entries, err := boundedReadDir(dir, 2*MaxRevisionsPerStory)
	if err != nil {
		return nil, err
	}
	files := entries[:0]
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".change-saga-write-") {
			continue
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("whiteboard revision %s must be a regular JSON file", entry.Name())
		}
		files = append(files, entry)
		if len(files) > MaxRevisionsPerStory {
			return nil, fmt.Errorf("whiteboard revision count exceeds maximum of %d", MaxRevisionsPerStory)
		}
	}
	return files, nil
}

func validateWhiteboardRevision(v WhiteboardRevision, id string) error {
	if v.Schema != WhiteboardSchemaURL || v.Version != 1 || !livingid.ValidID(v.ID) || v.ID != id || v.Parents == nil || len(v.Parents) > 1 || v.CreatedAt.IsZero() {
		return fmt.Errorf("invalid whiteboard revision identity, schema, parents, version or time")
	}
	if strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Name) != v.Name || len(v.Name) > 200 {
		return fmt.Errorf("whiteboard name must contain 1 to 200 bytes")
	}
	for _, p := range v.Parents {
		if !livingid.ValidID(p) || p == v.ID {
			return fmt.Errorf("invalid whiteboard parent %q", p)
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > MaxRecordBytes {
		return fmt.Errorf("whiteboard revision exceeds one MiB")
	}
	return v.Document.Validate()
}

// ListWhiteboardRevisionIDs validates a document's stored revision history once
// and lists its IDs from the unique current head to the root. An absent board
// returns nil,nil. Like LoadWhiteboard, it does not resolve or render source pins.
func ListWhiteboardRevisionIDs(root, sagaID, boardID string) ([]string, error) {
	revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
	if err != nil || len(revisions) == 0 {
		return nil, err
	}
	if len(heads) != 1 {
		return nil, fmt.Errorf("whiteboard has conflicting revision heads: %v", heads)
	}
	ids := make([]string, 0, len(revisions))
	for id := heads[0]; ; {
		ids = append(ids, id)
		parents := revisions[id].Parents
		if len(parents) == 0 {
			return ids, nil
		}
		id = parents[0]
	}
}

// LoadWhiteboard reads selected current content. An optional content digest
// verifies that value or retrieves the exact bytes from real Git history. Old
// internal revision selectors are rejected, and missing evidence never falls
// back to current content. Ordinary reads do not persist normalization.
func LoadWhiteboard(root, sagaID, boardID, revision string) (*WhiteboardRevision, error) {
	if currentrecord.ValidDigest(revision) {
		current, err := LoadWhiteboard(root, sagaID, boardID, "")
		if err != nil {
			return nil, err
		}
		if current != nil {
			digest, err := currentrecord.Digest(*current)
			if err != nil {
				return nil, err
			}
			if digest == revision {
				return current, nil
			}
		}
		saved, err := currentrecord.FindGitContent(root, filepath.Join(WhiteboardDir, boardID), revision)
		if err != nil {
			return nil, err
		}
		value := WhiteboardRevision{Schema: WhiteboardSchemaURL, Version: 1, ID: "sha256-" + strings.TrimPrefix(revision, "sha256:"), Parents: []string{}}
		if err := currentrecord.Project(saved.Content, &value); err != nil {
			return nil, err
		}
		if strings.TrimSpace(value.Name) == "" {
			return nil, fmt.Errorf("historical whiteboard name is missing")
		}
		if err := value.Document.Validate(); err != nil {
			return nil, err
		}
		if err := validateWhiteboardItems(sagaID, value.Document); err != nil {
			return nil, err
		}
		return &value, nil
	}
	if revision != "" && !strings.HasPrefix(revision, "sha256-") {
		return nil, fmt.Errorf("Saga revision selectors have been removed; inspect the record at a Git commit instead")
	}

	revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
	if err != nil {
		return nil, err
	}
	if revision == "" {
		if len(revisions) == 0 {
			return nil, nil
		}
		if len(heads) != 1 {
			return nil, fmt.Errorf("whiteboard has conflicting revision heads: %v", heads)
		}
		revision = heads[0]
	}
	v, ok := revisions[revision]
	if !ok {
		return nil, fmt.Errorf("whiteboard revision %q does not exist", revision)
	}
	if !strings.HasPrefix(v.ID, "sha256-") {
		if err := currentrecord.NormalizeValue(root, &v); err != nil {
			return nil, err
		}
	}
	return &v, nil
}

// PublishWhiteboard edits the current document in place. The revision and
// parent parameters are accepted only for source compatibility with older Go
// callers; authors no longer supply or reconcile content revision identities.
func PublishWhiteboard(ctx context.Context, root, sagaID, boardID, revision, parent, name string, document diagram.Document) (*WhiteboardRevision, error) {
	if !livingid.ValidID(sagaID) || !livingid.ValidID(boardID) {
		return nil, fmt.Errorf("whiteboard requires stable Saga and board identities")
	}
	var result *WhiteboardRevision
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A full authored replacement can resolve legacy ambiguity. Never choose
		// a head implicitly, and keep every earlier authored value in Git before
		// removing legacy files. Ordinary reads and automatic normalization still
		// refuse an ambiguous selection.
		history, heads, err := whiteboardHistory(root, sagaID, boardID)
		if err != nil {
			return err
		}
		if len(heads) > 1 {
			if err := preserveLegacyWhiteboardInGit(ctx, root, boardID, history); err != nil {
				return err
			}
		}
		pins, err := currentrecord.AuditPins(root)
		if err != nil {
			return err
		}
		data, err := json.Marshal(document)
		if err != nil {
			return err
		}
		data, err = pins.Normalize(data)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &document); err != nil {
			return err
		}
		if err := validateWhiteboardItems(sagaID, document); err != nil {
			return err
		}
		value := WhiteboardRevision{Schema: WhiteboardSchemaURL, Version: 1, ID: "current", Parents: []string{}, Name: name, Document: document, CreatedAt: mutationTime(time.Time{})}
		if err := validateWhiteboardRevision(value, "current"); err != nil {
			return err
		}
		if err := validateWhiteboardItemPins(root, sagaID, boardID, document); err != nil {
			return err
		}
		if _, err := resolveWhiteboard(root, sagaID, value); err != nil {
			return err
		}
		dir := filepath.Join(root, WhiteboardDir, boardID)
		if len(history) == 0 {
			ids, err := ListWhiteboardIDs(root)
			if err != nil {
				return err
			}
			if len(ids) >= MaxWhiteboards {
				return fmt.Errorf("whiteboard document limit reached")
			}
			if _, err := store.EnsureDirWithin(root, filepath.Join(root, WhiteboardDir)); err != nil {
				return err
			}
			if err := store.CommitDir(root, dir, func(stage string) error { return currentrecord.Write(stage, value, nil) }); err != nil {
				return err
			}
		} else {
			if err := pins.PersistReferences(); err != nil {
				return err
			}
			if err := pins.PersistLegacyReferencesBeforeRemoval(filepath.Join(dir, "revisions"), "urn:change-saga:"+sagaID+":whiteboard:"+boardID+":revision:"); err != nil {
				return err
			}
			if err := currentrecord.Write(dir, value, nil); err != nil {
				return err
			}
			if err := currentrecord.Cleanup(dir, "revisions"); err != nil {
				return err
			}
		}
		result, err = LoadWhiteboard(root, sagaID, boardID, "")
		return err
	})
	return result, err
}

// This is a migration guard, not another history store. A caller resolving an
// old fork must first preserve its inputs in Git, then supply the complete
// resolved document through the ordinary publish command.
func preserveLegacyWhiteboardInGit(ctx context.Context, root, boardID string, history map[string]WhiteboardRevision) error {
	failure := fmt.Errorf("commit the legacy whiteboard files to Git before publishing a resolved current document; no revision or parent selection is required")
	head, err := gitexec.Output(ctx, "-C", root, "rev-parse", "HEAD")
	if err != nil {
		return failure
	}
	prefix, err := gitexec.Output(ctx, "-C", root, "rev-parse", "--show-prefix")
	if err != nil {
		return failure
	}
	for id := range history {
		rel := filepath.ToSlash(filepath.Join(WhiteboardDir, boardID, "revisions", id+".json"))
		authored, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return err
		}
		saved, err := gitexec.Output(ctx, "-C", root, "show", strings.TrimSpace(string(head))+":"+strings.TrimSuffix(string(prefix), "\n")+rel)
		if err != nil || !bytes.Equal(saved, authored) {
			return failure
		}
	}
	return nil
}

func LoadResolvedWhiteboard(root, sagaID, boardID, revision string) (*ResolvedWhiteboard, error) {
	v, err := LoadWhiteboard(root, sagaID, boardID, revision)
	if err != nil || v == nil {
		return nil, err
	}
	return resolveWhiteboard(root, sagaID, *v)
}

func resolveWhiteboard(root, sagaID string, v WhiteboardRevision) (*ResolvedWhiteboard, error) {
	inventory, err := LoadInventory(root, sagaID)
	if err != nil {
		return nil, err
	}
	composition, err := resolveInventoryComposition(root, &inventory, v.Document, nil)
	if err != nil {
		return nil, err
	}
	svg, err := composition.Render(diagram.Options{Title: v.Name})
	if err != nil {
		return nil, err
	}
	return &ResolvedWhiteboard{Revision: v, Composition: composition, SVG: svg}, nil
}

// ValidateWhiteboards validates every named document's complete history,
// including source pins hidden by newer revisions. The subtree is optional.
func ValidateWhiteboards(root, sagaID string) error {
	if _, err := LoadWhiteboardMetadata(root, sagaID); err != nil {
		return err
	}
	if !livingid.ValidID(sagaID) {
		return fmt.Errorf("whiteboard requires a valid saga ID")
	}
	ids, err := ListWhiteboardIDs(root)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ValidateWhiteboard(root, sagaID, id); err != nil {
			return fmt.Errorf("whiteboard %s: %w", id, err)
		}
	}
	return nil
}

// ValidateWhiteboard validates selected current content and its exact source
// pins. Legacy parent structure is checked before selection; historical source
// versions belong to Git. A missing optional document is valid.
func ValidateWhiteboard(root, sagaID, boardID string) error {
	value, err := LoadWhiteboard(root, sagaID, boardID, "")
	if err != nil || value == nil {
		return err
	}
	_, err = resolveWhiteboard(root, sagaID, *value)
	return err
}
