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
)

//go:embed setup_initial_saga_prompt.md
var setupInitialSagaPrompt string

// SetupInitialSaga prints a one-time, repository-aware agent workflow. It does
// not create or modify a Saga; the coding agent follows the emitted workflow.
func SetupInitialSaga(args []string, out io.Writer) error {
	flags := commandFlags("setup-initial-saga", commandUsage["setup-initial-saga"], out)
	repo := flags.String("repo", ".", "repository to inspect for an existing app Saga")
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
	if len(sagas) > 0 && !*overhaul {
		fmt.Fprintln(out, "Initial Saga setup normally runs once, and this repository already contains:")
		for _, path := range sagas {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		fmt.Fprintln(out, "\nDo not run initial setup for ordinary documentation work. Update the existing Saga with the regular Change Saga commands instead.")
		fmt.Fprintln(out, "Only for an intentional, user-approved documentation overhaul, rerun:")
		fmt.Fprintf(out, "  change-saga setup-initial-saga --overhaul --repo %q\n", root)
		return nil
	}

	fmt.Fprintln(out, "Follow this one-time agent workflow to establish the repository's app Saga.")
	fmt.Fprintf(out, "Repository: %s\n", root)
	if len(sagas) == 0 {
		fmt.Fprintln(out, "Repository state: no .saga directory was found. Create one app Saga only after the interview establishes its initial product model.")
	} else {
		fmt.Fprintln(out, "Repository state: the user explicitly requested a documentation overhaul. Update the existing canonical app Saga; do not create a duplicate.")
		for _, path := range sagas {
			fmt.Fprintf(out, "  - %s\n", path)
		}
		if len(sagas) > 1 {
			fmt.Fprintln(out, "Ask which Saga is canonical before making changes.")
		}
	}
	fmt.Fprintln(out)
	_, err = io.WriteString(out, setupInitialSagaPrompt)
	return err
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
		fmt.Fprintln(out, "\nNo app Saga was detected in the current repository.")
		fmt.Fprintln(out, "  Run \"change-saga setup-initial-saga\" once for guided setup.")
		return
	}
	fmt.Fprintln(out, "\nExisting app Saga detected:")
	for _, path := range sagas {
		fmt.Fprintf(out, "  - %s\n", path)
	}
	fmt.Fprintln(out, "Use ordinary Change Saga commands to keep it current. Initial setup is only for a user-approved documentation overhaul.")
}
