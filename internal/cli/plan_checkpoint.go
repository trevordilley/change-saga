package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/twentyideas/changesaga/internal/plancheckpoint"
	"github.com/twentyideas/changesaga/internal/saga"
)

// checkpointSlideBudget is how many slides a checkpoint's review can sanely hold.
const checkpointSlideBudget = plancheckpoint.SlideBudget

// reportCheckpoint reports one checkpoint, with the Plan's live activity.
func reportCheckpoint(ctx context.Context, document *saga.Saga, plan *saga.Plan, checkpointID string, stories map[string]plancheckpoint.Story) (*plancheckpoint.Report, error) {
	return plancheckpoint.Build(document, plan, checkpointID, stories, liveTaskActivity(ctx, document.Root, "", plan.ID))
}

func planCheckpoint(ctx context.Context, args []string, out io.Writer) error {
	name := "plan checkpoint"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	checkpoint := flags.String("checkpoint", "", "the checkpoint phase")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *checkpoint); err != nil {
		return err
	}
	document, found, err := planOf(flags.Arg(0), *plan)
	if err != nil {
		return err
	}
	report, err := reportCheckpoint(ctx, document, found, *checkpoint, plancheckpoint.Stories(document.Root, document.Manifest.ID))
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, report)
	}
	fmt.Fprintf(out, "Checkpoint %s: %s (%s)\n", report.Checkpoint, report.Title, report.State)
	if report.Passed != nil {
		who := report.Passed.By
		if report.Passed.Agent {
			who += ", an agent"
		}
		fmt.Fprintf(out, "  Passed by %s on %s: %s\n", who, report.Passed.At.Format(time.DateOnly), report.Passed.Reason)
	}
	fmt.Fprintf(out, "  Covers %d of its %d tasks done, in %s.\n", report.TasksDone, report.Tasks, plansFeatureList(report.Phases))
	fmt.Fprintf(out, "  Addresses %d %s:\n", len(report.Stories), plural(len(report.Stories), "story", "stories"))
	for _, story := range report.Stories {
		proven := 0
		for _, criterion := range story.Criteria {
			if criterion.Proven() {
				proven++
			}
		}
		fmt.Fprintf(out, "    %s (%d of %d %s with test evidence)\n", story.Title, proven, len(story.Criteria), plural(len(story.Criteria), "criterion", "criteria"))
	}
	size := fmt.Sprintf("  About %d %s to explain, one per story", report.Estimate, plural(report.Estimate, "slide", "slides"))
	if report.Slides > 0 {
		size += fmt.Sprintf("; its review %s took %d", report.Review, report.Slides)
	}
	fmt.Fprintln(out, size+".")
	if report.WidenedTo != "" {
		fmt.Fprintf(out, "  Its review is widened to every change since checkpoint %s.\n", report.WidenedTo)
	}
	if report.Estimate > checkpointSlideBudget {
		fmt.Fprintf(out, "  That is more than %d: consider splitting it with another checkpoint.\n", checkpointSlideBudget)
	}
	if len(report.Changes) > 0 {
		fmt.Fprintf(out, "  Since checkpoint %s passed, the Plan changed:\n", report.ChangedSince)
		for _, change := range report.Changes {
			switch change.Kind {
			case "added":
				fmt.Fprintf(out, "    added %s (%s) in %s\n", change.Task, change.Title, change.To)
			case "dropped":
				fmt.Fprintf(out, "    dropped %s from %s\n", change.Task, change.From)
			default:
				fmt.Fprintf(out, "    moved %s (%s) from %s to %s\n", change.Task, change.Title, change.From, change.To)
			}
		}
	}
	if report.Passed == nil {
		if report.Passer == saga.CheckpointPasserAgent {
			fmt.Fprintln(out, "  An agent may pass it unless something here is surprising or urgent.")
		} else {
			fmt.Fprintln(out, "  Whoever invoked the Plan passes it.")
		}
		for _, surprise := range report.Surprises {
			fmt.Fprintf(out, "  Surprising: %s.\n", surprise)
		}
	}
	fmt.Fprintf(out, "Next: %s\n", report.Next)
	return nil
}
