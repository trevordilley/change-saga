package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// overviewPart is one of the overview's two written parts. The project name
// is saga.json's title and terms are living records, so neither is written
// here.
type overviewPart struct {
	command string
	pkg     string
	title   string
	order   int
}

var overviewParts = map[string]overviewPart{
	"set-pitch":       {command: "overview set-pitch", pkg: applayout.OverviewPitch, title: "Elevator pitch", order: 10},
	"set-description": {command: "overview set-description", pkg: applayout.OverviewDescription, title: "Description", order: 20},
}

// Overview dispatches the overview command family.
func Overview(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	return overviewCommand(ctx, args, out, os.Stdin)
}

func overviewCommand(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("overview", []string{"set-pitch", "set-description"}, out)
	}
	part, ok := overviewParts[args[0]]
	if !ok {
		return fmt.Errorf("usage: %s", commandUsage["overview"])
	}
	err := setOverviewPart(ctx, part, args[1:], out, stdin)
	if err != nil && jsonFlagRequested(args) {
		return reportJSONMutationFailure(out, err)
	}
	return err
}

// setOverviewPart writes the elevator pitch or description as Markdown,
// creating its fragment the first time.
func setOverviewPart(_ context.Context, part overviewPart, args []string, out io.Writer, stdin io.Reader) error {
	flags := commandFlags(part.command, commandUsage[part.command], out)
	text := flags.String("text", "", "the Markdown content")
	source := flags.String("source", "", "a Markdown file, or - for standard input")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	quiet := flags.Bool("quiet", false, "suppress successful output")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || (*text == "") == (*source == "") {
		return fmt.Errorf("usage: %s", commandUsage[part.command])
	}
	if *jsonOutput && *quiet {
		return fmt.Errorf("--json and --quiet cannot be combined")
	}
	data := []byte(*text)
	if *source != "" {
		var err error
		if *source == "-" {
			data, err = io.ReadAll(stdin)
		} else {
			data, err = os.ReadFile(*source)
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", strings.ToLower(part.title), err)
		}
	}
	if strings.TrimSpace(string(data)) == "" {
		return fmt.Errorf("the %s is empty; an absent part is already shown as a gap", strings.ToLower(part.title))
	}
	if !strings.HasSuffix(string(data), "\n") {
		data = append(data, '\n')
	}
	result := fragmentContentOutput{OK: true, Bytes: len(data)}
	err := authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		fragment := document.OverviewPart(part.pkg)
		if fragment == nil {
			manifest := saga.FragmentManifest{
				Version: saga.CurrentVersion, ID: document.Manifest.ID + "-" + strings.TrimSuffix(part.pkg, ".fragment"),
				Title: part.title, MediaType: "text/markdown", Entrypoint: "content.md", Order: part.order,
			}
			dir := filepath.Join(document.Root, applayout.OverviewDir, part.pkg)
			if err := store.CommitDir(document.Root, dir, func(stage string) error {
				if err := os.Chmod(stage, 0o755); err != nil {
					return err
				}
				return populateFragment(stage, manifest, "", data)
			}); err != nil {
				return err
			}
			result.Target = saga.FragmentTarget(document.Manifest.ID, manifest.ID)
			result.Path = applayout.OverviewDir + "/" + part.pkg + "/" + manifest.Entrypoint
			result.MediaType = manifest.MediaType
			return nil
		}
		entrypoint := filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))
		if info, err := os.Lstat(entrypoint); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fragment entrypoint must not be a symlink")
		}
		if err := store.WriteFile(entrypoint, data, 0o644, false); err != nil {
			return err
		}
		result.Target = fragment.Target
		result.Path = filepath.ToSlash(filepath.Join(fragment.Path, fragment.Entrypoint))
		result.MediaType = fragment.MediaType
		return nil
	})
	if err != nil {
		return err
	}
	switch {
	case *quiet:
		return nil
	case *jsonOutput:
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "Updated the %s: %s (%d bytes)\n", strings.ToLower(part.title), result.Path, result.Bytes)
	return nil
}
