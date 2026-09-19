package cli

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

type syncOutput struct {
	OK       bool   `json:"ok"`
	Commit   string `json:"commit"`
	Previous string `json:"previous,omitempty"`
	Cursor   string `json:"cursor"`
}

// Sync moves a companion Saga's sync cursor to the code commit it now
// documents. Every Saga commit that updates the documentation should move
// it, so a comparison can find the Saga that documented its merge-base. A
// Saga in its code repository has no cursor: it documents the commit it is
// read at.
func Sync(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("sync", commandUsage["sync"], out)
	repoDir := flags.String("repo", "", "code repository checkout the Saga documents")
	commit := flags.String("commit", "HEAD", "the code commit the Saga now documents")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	allowMismatch := flags.Bool("allow-repository-mismatch", false, "use a checkout whose origin differs from the declared repository")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *repoDir == "" {
		return fmt.Errorf("usage: %s", commandUsage["sync"])
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if location, err := changeview.Locate(ctx, root); err == nil && sameCheckout(ctx, location.Repo, *repoDir) {
		return fmt.Errorf("the Saga lives in the code repository, so it documents the commit it is read at and has no sync cursor")
	}
	if !*allowMismatch {
		top, err := gitTopLevel(ctx, *repoDir)
		if err != nil {
			return err
		}
		if err := gitdiff.VerifyRepository(ctx, top, manifest.Source.Repository); err != nil {
			return err
		}
	}
	resolved, err := resolveCommit(ctx, *repoDir, *commit)
	if err != nil {
		return err
	}
	result := syncOutput{OK: true, Commit: resolved, Cursor: saga.CursorName}
	if previous, ok, err := saga.ReadCursor(root); err == nil && ok {
		result.Previous = previous.Commit
	}
	if err := writeCursor(root, resolved); err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	if result.Previous != "" && result.Previous != resolved {
		fmt.Fprintf(out, "Moved the sync cursor from %s to %s\n", shortOID(result.Previous), resolved)
	} else {
		fmt.Fprintf(out, "The Saga documents %s\n", resolved)
	}
	return nil
}

func writeCursor(root, commit string) error {
	return authorMutation(root, func(locked *saga.Saga) error {
		return store.WriteJSON(filepath.Join(locked.Root, saga.CursorName), saga.Cursor{Schema: saga.CursorSchemaURL, Version: saga.CursorVersion, Commit: commit}, false)
	})
}

// companionCheckout reports whether the Saga at root lives in a different
// repository from the code checkout.
func companionCheckout(ctx context.Context, root, checkout string) bool {
	location, err := changeview.Locate(ctx, root)
	return err == nil && !sameCheckout(ctx, location.Repo, checkout)
}

func sameCheckout(ctx context.Context, sagaRepo, checkout string) bool {
	top, err := gitTopLevel(ctx, checkout)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(top); err == nil {
		top = resolved
	}
	return filepath.Clean(top) == filepath.Clean(sagaRepo)
}

func gitTopLevel(ctx context.Context, dir string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not a Git checkout", dir)
	}
	return strings.TrimSpace(string(output)), nil
}
