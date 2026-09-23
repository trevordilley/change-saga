package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/visualqa"
)

// VisualQA renders selected slides at the standard viewports and reports only
// mechanical browser findings. It never writes to the Saga.
func VisualQA(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("visual-qa", commandUsage["visual-qa"], out)
	feature := flags.String("feature", "", "feature id or URN; use onboarding for the app onboarding deck")
	deck := flags.String("deck", "", "deck id or URN")
	slide := flags.String("slide", "", "slide id or URN")
	output := flags.String("output", "", "managed output directory; defaults to ./change-saga-visual-qa/<saga-id>")
	repo := flags.String("repo", "", "source repository checkout when separate")
	playwright := flags.String("playwright-dir", "", "directory containing the installed Playwright node_modules")
	jsonOutput := flags.Bool("json", false, "emit the machine-readable report")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return fmt.Errorf("visual-qa requires exactly one Saga path")
	}
	report, err := visualqa.Run(ctx, visualqa.Options{
		SagaRoot: flags.Arg(0), SourceDir: *repo, OutputDir: *output,
		Feature: *feature, Deck: *deck, Slide: *slide, PlaywrightDir: *playwright,
	})
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return err
		}
	} else {
		destination := *output
		if destination == "" {
			destination = filepath.Join("change-saga-visual-qa", report.Saga)
		}
		fmt.Fprintf(out, "Rendered %d slide(s) to %s\nMechanical findings: %d\nSemantic arrow correctness: not evaluated\n", len(report.Slides), destination, len(report.Findings))
	}
	if !report.Passed {
		return &StatusError{Code: 3}
	}
	return nil
}
