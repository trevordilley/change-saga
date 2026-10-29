package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/prototypes"
)

// prototypeAdd adds a prototype that is an HTML file in the project.
// Nothing is copied into the Saga: the revision pins the entrypoint and the
// directory served beside it at a commit, as code evidence pins lines.
func prototypeAdd(ctx context.Context, args []string, out io.Writer) error {
	name := "prototype add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable prototype id")
	revision := flags.String("revision", "", "stable initial revision id")
	title := flags.String("title", "", "prototype title")
	entry := flags.String("path", "", "the prototype's .html entrypoint")
	rootDir := flags.String("root", "", "directory served beside the entrypoint")
	commit := flags.String("commit", "", "commit the files are pinned at; defaults to HEAD")
	repo := flags.String("repo", "", "repository checkout; defaults to the one containing the Saga")
	state := flags.String("state", string(prototypes.StateDraft), "draft, ready, or retired")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if *revision == "" {
		*revision = "r1"
	}
	if err := requireLivingArgs(flags, *id, *title, *entry); err != nil {
		return err
	}
	root := flags.Arg(0)
	checkout := firstNonEmpty(*repo, root)
	entrypoint, packageRoot, err := repoPrototypePaths(ctx, checkout, *entry, *rootDir, *rootDir != "")
	if err != nil {
		return err
	}
	source, err := prototypes.ResolveRepoSource(ctx, checkout, entrypoint, packageRoot, *commit)
	if err != nil {
		return err
	}
	sagaID, err := prototypeSagaID(root)
	if err != nil {
		return err
	}
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	result, err := prototypes.AddRepo(root, sagaID, prototypes.AddRepoInput{
		Feature: target.ID, ID: *id, RevisionID: *revision, Title: *title, State: prototypes.State(*state),
		Source: source, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

// prototypeUpdate records a new version of a prototype added with
// prototype add after its files changed, as a revision whose parent is the
// current one.
func prototypeUpdate(ctx context.Context, args []string, out io.Writer) error {
	name := "prototype update"
	flags := commandFlags(name, commandUsage[name], out)
	prototype := flags.String("prototype", "", "canonical prototype URN")
	revision := flags.String("revision", "", "stable new revision id")
	commit := flags.String("commit", "", "commit to re-pin at; defaults to HEAD")
	title := flags.String("title", "", "title; defaults to the current revision's")
	state := flags.String("state", "", "draft, ready, or retired; defaults to the current revision's")
	repo := flags.String("repo", "", "repository checkout; defaults to the one containing the Saga")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *prototype); err != nil {
		return err
	}
	root := flags.Arg(0)
	document, err := prototypes.Load(root, "")
	if err != nil {
		return err
	}
	id := (*prototype)[strings.LastIndex(*prototype, ":")+1:]
	if want, _ := prototypes.PrototypeURN(document.SagaID, id); want != *prototype {
		return fmt.Errorf("--prototype must be a canonical prototype URN of this Saga")
	}
	found := document.Find(id)
	if found == nil {
		return fmt.Errorf("prototype %q does not exist", id)
	}
	current := found.CurrentRevision
	if current == nil {
		return fmt.Errorf("prototype %q has more than one current revision; reconcile it with prototype revise", id)
	}
	if current.Source.Kind != prototypes.SourceRepo {
		return fmt.Errorf("prototype %q is not a repository prototype; use prototype revise", id)
	}
	checkout := firstNonEmpty(*repo, root)
	source, err := prototypes.ResolveRepoSource(ctx, checkout, current.Source.Entrypoint, current.Source.Root, *commit)
	if err != nil {
		return err
	}
	if source.Object == current.Source.Object {
		return fmt.Errorf("prototype %q already matches %s at %s; nothing changed to record", id, source.RepoPackage(), source.Commit[:12])
	}
	if *revision == "" {
		*revision = nextRevisionID(*found)
	}
	parent, _ := prototypes.RevisionURN(document.SagaID, id, current.ID)
	input := prototypes.ReviseInput{
		Prototype: *prototype, ID: *revision, Parents: []string{parent},
		Title: firstNonEmpty(*title, current.Title), State: prototypes.State(firstNonEmpty(*state, string(current.State))),
		Source: source, RequestID: *requestID,
	}
	result, err := prototypes.Revise(root, document.SagaID, input)
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

// nextRevisionID is the first rN after the prototype's revision count that
// no revision uses yet.
func nextRevisionID(prototype prototypes.Prototype) string {
	for n := len(prototype.Revisions) + 1; ; n++ {
		if id := fmt.Sprintf("r%d", n); prototype.Revision(id) == nil {
			return id
		}
	}
}

// repoPrototypePaths turns the authored entrypoint and root into
// repository-relative slash paths. A path that exists from the current
// directory is taken from there; otherwise it is already repository-relative.
// Without an explicit root, the entrypoint's directory is served beside it,
// or the file alone when it sits at the repository root.
func repoPrototypePaths(ctx context.Context, checkout, entry, root string, explicitRoot bool) (string, string, error) {
	top, err := gitexec.Output(ctx, "-C", checkout, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("%s is not inside a Git repository; pass --repo", checkout)
	}
	repoRoot := strings.TrimSpace(string(top))
	relative := func(value string) (string, error) {
		if _, statErr := os.Stat(value); statErr == nil || filepath.IsAbs(value) {
			absolute, err := filepath.Abs(value)
			if err != nil {
				return "", err
			}
			if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
				absolute = resolved
			}
			if resolved, err := filepath.EvalSymlinks(repoRoot); err == nil {
				repoRoot = resolved
			}
			rel, err := filepath.Rel(repoRoot, absolute)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("%s is outside the repository %s", value, repoRoot)
			}
			return filepath.ToSlash(rel), nil
		}
		return path.Clean(filepath.ToSlash(value)), nil
	}
	entrypoint, err := relative(entry)
	if err != nil {
		return "", "", err
	}
	if path.Ext(entrypoint) != ".html" {
		return "", "", fmt.Errorf("--path must name the prototype's .html entrypoint")
	}
	if !explicitRoot {
		if dir := path.Dir(entrypoint); dir != "." {
			return entrypoint, dir, nil
		}
		return entrypoint, "", nil
	}
	packageRoot, err := relative(root)
	if err != nil {
		return "", "", err
	}
	if packageRoot == "." {
		return "", "", fmt.Errorf("--root cannot be the repository root; omit it to serve the entrypoint alone")
	}
	return entrypoint, packageRoot, nil
}
