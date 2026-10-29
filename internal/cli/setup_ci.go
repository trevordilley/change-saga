package cli

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed setup_ci_prompt.md
var setupCIPrompt string

// SetupCI prints guidance for a coding agent or maintainer; it never changes
// workflows, ownership files, repository settings, or the Saga.
func SetupCI(args []string, out io.Writer) error {
	flags := commandFlags("setup-ci", commandUsage["setup-ci"], out)
	repo := flags.String("repo", ".", "repository whose CI and review policy will be configured")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: %s", commandUsage["setup-ci"])
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
	_, err = fmt.Fprintf(out, "Repository: %s\n\n%s", root, setupCIPrompt)
	return err
}
