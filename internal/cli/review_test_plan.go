package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingapp"
)

func reviewTestPlan(ctx context.Context, args []string, out io.Writer) error {
	var usage bytes.Buffer
	flags := commandFlags("review test-plan", commandUsage["review test-plan"], &usage)
	id := flags.String("review", "", "review id")
	repo := flags.String("repo", "", "code checkout when separate")
	jsonOutput := flags.Bool("json", false, "emit the complete test plan as JSON")
	fail := func(err error) error {
		if jsonFlagRequested(args) && !errors.Is(err, flag.ErrHelp) {
			if writeErr := writeJSON(out, map[string]any{"schema": "change-saga.test-plan/v1", "ok": false, "error": map[string]string{"code": "test_plan_failed", "message": err.Error()}}); writeErr != nil {
				return writeErr
			}
			return &StatusError{Code: 1}
		}
		return err
	}
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) || !jsonFlagRequested(args) {
			if _, writeErr := io.Copy(out, &usage); writeErr != nil {
				return writeErr
			}
		}
		return fail(err)
	}
	if flags.NArg() != 1 || strings.TrimSpace(*id) == "" {
		return fail(fmt.Errorf("usage: %s", commandUsage["review test-plan"]))
	}
	plan, err := livingapp.PlanReviewTests(ctx, flags.Arg(0), *repo, *id)
	if err != nil {
		return fail(err)
	}
	if *jsonOutput {
		if err := writeJSON(out, plan); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "Review %s: %d automated test cases for %d affected stories\n", *id, len(plan.Tests), len(plan.Stories))
		fmt.Fprintf(out, "Range: %s..%s\n", plan.Range.BaseOID, plan.Range.HeadOID)
		for _, test := range plan.Tests {
			fmt.Fprintf(out, "\n%s — %s\n", test.Title, test.TestCase)
			fmt.Fprintf(out, "  Why: %s\n", strings.Join(test.Reasons, "; "))
			for _, story := range test.Stories {
				fmt.Fprintf(out, "  Story: %s\n", story)
			}
			for _, code := range test.Code {
				fmt.Fprintf(out, "  Code: %s\n", code.String())
			}
			for _, command := range test.Commands {
				fmt.Fprintf(out, "  Recorded command: %s\n", command.Command)
			}
		}
		if len(plan.Gaps) > 0 {
			fmt.Fprintln(out, "\nCoverage gaps:")
		}
		for _, gap := range plan.Gaps {
			fmt.Fprintf(out, "  %s: %s — %s\n", gap.Code, gap.Target, gap.Message)
		}
		fmt.Fprintln(out, "\nSelection follows recorded dependencies; it is not proof of coverage. No tests were executed.")
	}
	if !plan.Complete {
		return &StatusError{Code: 3}
	}
	return nil
}
