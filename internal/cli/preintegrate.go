package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/semanticcheck"
)

// Preintegrate reports semantic convergence risks across explicit committed
// refs. It never invokes a Git writer and never changes the working tree.
func Preintegrate(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("preintegrate", commandUsage["preintegrate"], out)
	repo := flags.String("repo", ".", "Git repository containing the refs and Saga")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var refs stringList
	flags.Var(&refs, "ref", "explicit Git ref to compare; repeat at least twice")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || len(refs) < 2 {
		return fmt.Errorf("usage: %s", commandUsage["preintegrate"])
	}
	repository, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	sagaArgument := flags.Arg(0)
	if !filepath.IsAbs(sagaArgument) {
		sagaArgument = filepath.Join(repository, sagaArgument)
	}
	sagaPath, err := filepath.Abs(sagaArgument)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(repository, sagaPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("<saga> must be inside --repo")
	}
	report, err := semanticcheck.Check(ctx, semanticcheck.Options{Repository: repository, SagaPath: filepath.ToSlash(relative), Refs: refs})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "Read-only pre-integration report for %d refs\n", len(report.Refs))
	for _, ref := range report.Refs {
		fmt.Fprintf(out, "  %s %s %s\n", ref.Ref, ref.Commit, ref.SagaPath)
	}
	fmt.Fprintf(out, "Stable ID collisions: %d\n", len(report.Collisions))
	for _, finding := range report.Collisions {
		fmt.Fprintf(out, "  %s %s: %s\n", finding.Kind, finding.ID, finding.Explanation)
		for _, ref := range report.Refs {
			if path := finding.Paths[ref.Ref]; path != "" {
				fmt.Fprintf(out, "    %s %s:%s\n", ref.Ref, ref.Commit, path)
			}
		}
	}
	fmt.Fprintf(out, "Competing heads: %d\n", len(report.CompetingHeads))
	for _, finding := range report.CompetingHeads {
		fmt.Fprintf(out, "  story %s: %s\n", finding.Story, finding.Explanation)
		for _, provenance := range finding.Provenance {
			fmt.Fprintf(out, "    %s %s revisions=%v lifecycle=%v\n", provenance.Ref, provenance.Commit, finding.RevisionHeads[provenance.Ref], finding.LifecycleHeads[provenance.Ref])
		}
	}
	fmt.Fprintf(out, "Overlapping-intent candidates: %d\n", len(report.IntentCandidates))
	for _, finding := range report.IntentCandidates {
		fmt.Fprintf(out, "  %s <> %s (%.2f): %s\n", finding.Left, finding.Right, finding.Score, finding.Explanation)
		fmt.Fprintf(out, "    %s %s <> %s %s; shared=%v\n", finding.Provenance.Ref, finding.Provenance.Commit, finding.Other.Ref, finding.Other.Commit, finding.SharedTerms)
	}
	fmt.Fprintf(out, "Heuristic: %s\nNo refs, branches, working-tree files, or Git merges were changed.\n", report.Heuristic)
	return nil
}
