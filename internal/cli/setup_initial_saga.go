package cli

import (
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
)

//go:embed setup_initial_saga_prompt.md
var setupInitialSagaPrompt string

// SetupInitialSaga prints a one-time, repository-aware agent workflow. It does
// not create or modify a Saga; the coding agent follows the emitted workflow.
func SetupInitialSaga(args []string, out io.Writer) error {
	flags := commandFlags("setup-initial-saga", commandUsage["setup-initial-saga"], out)
	repo := flags.String("repo", ".", "repository to inspect for an existing Saga")
	overhaul := flags.Bool("overhaul", false, "emit the workflow despite an existing Saga; use only for an intentional documentation overhaul")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s", commandUsage["setup-initial-saga"])
	}

	root, err := filepath.Abs(*repo)
	if err != nil {
		return fmt.Errorf("resolve repository: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect repository: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("repository is not a directory: %s", root)
	}

	sagas, err := findSagaDirectories(root)
	if err != nil {
		return fmt.Errorf("inspect repository for Saga directories: %w", err)
	}
	// A Saga that holds only reviews is what this workflow grows: taking it
	// after a few reviews is the intended path, not an overhaul.
	growing := len(sagas) == 1 && reviewOnlySaga(filepath.Join(root, sagas[0]))
	if len(sagas) > 0 && !*overhaul && !growing {
		fmt.Fprintln(out, "Initial Saga setup normally runs once, and this repository already contains:")
		for _, path := range sagas {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		fmt.Fprintln(out, "\nDo not run initial setup for ordinary documentation work. Update the existing Saga with the regular Change Saga commands instead.")
		fmt.Fprintln(out, "Only for an intentional, user-approved documentation overhaul, rerun:")
		fmt.Fprintf(out, "  change-saga setup-initial-saga --overhaul --repo %q\n", root)
		return nil
	}

	fmt.Fprintln(out, "Follow this one-time agent workflow to establish the repository's Saga.")
	fmt.Fprintf(out, "Repository: %s\n", root)
	switch {
	case len(sagas) == 0:
		fmt.Fprintln(out, "Repository state: no .saga directory was found. Most teams start with a review of a pull request instead (change-saga init, then change-saga review create on the branch) and take this guided setup after a few reviews; continue only if the user asked for the full setup now. Create the repository's Saga (change-saga init creates change.saga) only after the interview establishes its initial product model.")
	case growing && !*overhaul:
		reviews := countReviews(filepath.Join(root, sagas[0]))
		fmt.Fprintf(out, "Repository state: %s holds %d %s and no living documentation yet. Grow it in place: do not run change-saga init or create another Saga.\n", sagas[0], reviews, plural(reviews, "review", "reviews"))
	default:
		fmt.Fprintln(out, "Repository state: the user explicitly requested a documentation overhaul. Update the existing canonical Saga; do not create a duplicate.")
		for _, path := range sagas {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		if len(sagas) > 1 {
			fmt.Fprintln(out, "Ask which Saga is canonical before making changes. The recommended idiom is one Saga per repository, but that is the user's choice.")
		}
	}
	fmt.Fprintln(out)
	_, err = io.WriteString(out, setupInitialSagaPrompt)
	return err
}

// reviewOnlySaga reports whether dir is a Saga that holds nothing beyond its
// reviews: the Saga setup grows rather than overhauls.
func reviewOnlySaga(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, saga.ManifestName)); err != nil {
		return false
	}
	return !holdsLivingDocumentation(dir)
}

// countReviews counts the reviews a Saga directory holds.
func countReviews(dir string) int {
	entries, _ := os.ReadDir(filepath.Join(dir, saga.ReviewsDir))
	count := 0
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), saga.ReviewSuffix) {
			count++
		}
	}
	return count
}

func findSagaDirectories(root string) ([]string, error) {
	ignored := map[string]bool{
		".git": true, ".hg": true, ".svn": true,
		"node_modules": true, "vendor": true, "dist": true, "build": true,
		".next": true, ".cache": true,
	}
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || !entry.IsDir() {
			return nil
		}
		if ignored[entry.Name()] {
			return filepath.SkipDir
		}
		if strings.HasSuffix(entry.Name(), ".saga") {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
			return filepath.SkipDir
		}
		return nil
	})
	sort.Strings(found)
	return found, err
}

func printInitialSagaHelp(out io.Writer) {
	root, err := filepath.Abs(".")
	if err != nil {
		return
	}
	sagas, err := findSagaDirectories(root)
	if err != nil {
		return
	}
	if len(sagas) == 0 {
		fmt.Fprintln(out, "\nNo Saga was detected in the current repository.")
		fmt.Fprintln(out, "  Start with a review of a branch or pull request: \"change-saga init\" creates change.saga at the repository's root, then \"change-saga review create change.saga\".")
		fmt.Fprintln(out, "  After a few reviews, \"change-saga setup-initial-saga\" guides growing it into living documentation of the app.")
		return
	}
	if len(sagas) == 1 && reviewOnlySaga(filepath.Join(root, sagas[0])) {
		fmt.Fprintf(out, "\nExisting Saga detected, holding only reviews: %s\n", sagas[0])
		fmt.Fprintln(out, "Keep creating a review for each pull request. When the team wants the app itself documented, \"change-saga setup-initial-saga\" guides growing this Saga.")
		return
	}
	fmt.Fprintln(out, "\nExisting Saga detected:")
	for _, path := range sagas {
		fmt.Fprintf(out, "  - %s\n", path)
	}
	fmt.Fprintln(out, "Use ordinary Change Saga commands to keep it current. Initial setup is only for a user-approved documentation overhaul.")
}
