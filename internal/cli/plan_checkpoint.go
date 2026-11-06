package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// checkpointSlideBudget is how many slides a checkpoint's review can sanely hold.
// Ten to twenty means the checkpoint should have been two.
const checkpointSlideBudget = 6

// checkpointStory is a story a checkpoint's Tasks lead to.
type checkpointStory struct {
	Target   string `json:"target"`
	Title    string `json:"title"`
	Criteria int    `json:"criteria"`
}

// checkpointReport is what plan checkpoint says about one checkpoint.
type checkpointReport struct {
	Plan       string                                `json:"plan"`
	Checkpoint string                                `json:"checkpoint"`
	Title      string                                `json:"title"`
	State      string                                `json:"state"`
	Passed     *saga.CheckpointPass                  `json:"passed,omitempty"`
	Passer     string                                `json:"passer"`
	Phases     []string                              `json:"phases"`
	Tasks      int                                   `json:"tasks"`
	TasksDone  int                                   `json:"tasks_done"`
	Unfinished []string                              `json:"unfinished,omitempty"`
	Stories    []checkpointStory                     `json:"stories"`
	Estimate   int                                   `json:"estimated_slides"`
	Review     string                                `json:"review,omitempty"`
	Slides     int                                   `json:"slides,omitempty"`
	Surprises  []string                              `json:"surprises"`
	Next       string                                `json:"next"`
	scope      []saga.TaskManifest                   `json:"-"`
	phase      saga.PlanPhase                        `json:"-"`
	activity   map[string]*planactivity.TaskActivity `json:"-"`
}

// checkpointStories are the stories the scope's Tasks lead to: each Task's own,
// or for a Task without one, those of the Tasks it unblocks.
func checkpointStories(plan *saga.Plan, scope []saga.TaskManifest, titles map[string]checkpointStory) []checkpointStory {
	unblocks := saga.TaskUnblocks(plan.Manifests())
	byID := map[string]saga.TaskManifest{}
	for _, task := range plan.Manifests() {
		byID[task.ID] = task
	}
	found := []string{}
	for _, task := range scope {
		seen := map[string]bool{task.ID: true}
		queue := []saga.TaskManifest{task}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if current.Story != "" {
				if !slices.Contains(found, current.Story) {
					found = append(found, current.Story)
				}
				continue
			}
			for _, id := range unblocks[current.ID] {
				if next, known := byID[id]; known && !seen[id] {
					seen[id] = true
					queue = append(queue, next)
				}
			}
		}
	}
	stories := make([]checkpointStory, 0, len(found))
	for _, urn := range found {
		story, known := titles[urn]
		if !known {
			story = checkpointStory{Target: urn, Title: urn}
		}
		stories = append(stories, story)
	}
	return stories
}

func storyTitles(root, sagaID string) map[string]checkpointStory {
	titles := map[string]checkpointStory{}
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return titles
	}
	for _, story := range document.Stories {
		urn := "urn:change-saga:" + sagaID + ":story:" + story.Identity.ID
		value := checkpointStory{Target: urn, Title: story.Identity.ID}
		if story.CurrentRevision != nil {
			if story.CurrentRevision.Title != "" {
				value.Title = story.CurrentRevision.Title
			}
			value.Criteria = len(story.CurrentRevision.AcceptanceCriteria)
		}
		titles[urn] = value
	}
	return titles
}

// reportCheckpoint works out a checkpoint's scope, size, and what should stop an
// agent from passing it. It reports; whoever passes judges.
func reportCheckpoint(ctx context.Context, document *saga.Saga, plan *saga.Plan, checkpointID string, titles map[string]checkpointStory) (*checkpointReport, error) {
	index := slices.IndexFunc(plan.Phases, func(phase saga.PlanPhase) bool { return phase.ID == checkpointID })
	if index < 0 || !plan.Phases[index].IsCheckpoint() {
		return nil, fmt.Errorf("%q is not a checkpoint of plan %q", checkpointID, plan.ID)
	}
	phase := plan.Phases[index]
	report := &checkpointReport{Plan: plan.Target, Checkpoint: phase.ID, Title: phase.Title, Passed: phase.Checkpoint.Passed, Passer: firstNonEmpty(plan.CheckpointPasser, saga.CheckpointPasserPerson), Review: phase.Checkpoint.Review, Phases: []string{}, Surprises: []string{}, phase: phase}
	for earlier := index - 1; earlier >= 0 && !plan.Phases[earlier].IsCheckpoint(); earlier-- {
		report.Phases = append([]string{plan.Phases[earlier].ID}, report.Phases...)
	}
	report.scope = saga.CheckpointScope(plan.Phases, plan.Manifests(), checkpointID)
	report.Tasks = len(report.scope)
	for _, task := range report.scope {
		if task.State == saga.TaskDone {
			report.TasksDone++
		} else {
			report.Unfinished = append(report.Unfinished, task.ID)
		}
	}
	report.Stories = checkpointStories(plan, report.scope, titles)
	report.Estimate = max(1, len(report.Stories))

	// What happened during the stretch: since the previous checkpoint passed.
	began := plan.CreatedAt
	for earlier := index - 1; earlier >= 0; earlier-- {
		if plan.Phases[earlier].CheckpointPassed() {
			began = plan.Phases[earlier].Checkpoint.Passed.At
			break
		}
	}
	added := []string{}
	for _, task := range report.scope {
		if task.CreatedAt.After(began) && !began.Equal(plan.CreatedAt) {
			added = append(added, task.ID)
		}
	}
	if len(added) > 0 {
		report.Surprises = append(report.Surprises, fmt.Sprintf("Tasks added since the stretch began: %s", strings.Join(added, ", ")))
	}
	report.activity = liveTaskActivity(ctx, document.Root, "", plan.ID)
	blocked := []string{}
	for _, task := range report.scope {
		if activity := report.activity[task.ID]; activity != nil {
			for _, event := range activity.Events {
				if event.Kind == planactivity.KindBlocked && event.At.After(began) {
					blocked = append(blocked, task.ID)
					break
				}
			}
		}
	}
	if len(blocked) > 0 {
		report.Surprises = append(report.Surprises, fmt.Sprintf("an agent reported blocked on: %s", strings.Join(blocked, ", ")))
	}
	if review := document.FindReview(report.Review); review != nil && review.Deck != nil {
		report.Slides = len(review.Deck.Slides)
		inScope := map[string]bool{}
		for _, task := range report.scope {
			inScope[saga.TaskTarget(document.Manifest.ID, plan.ID, task.ID)] = true
		}
		departures := []string{}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				if item.Kind == "callout" && inScope[item.Record] {
					departures = append(departures, item.Label)
				}
			}
		}
		if len(departures) > 0 {
			report.Surprises = append(report.Surprises, fmt.Sprintf("the review says the work departed from the Plan: %s", strings.Join(departures, "; ")))
		}
		if report.Slides > max(report.Estimate, checkpointSlideBudget) {
			report.Surprises = append(report.Surprises, fmt.Sprintf("the review took %d slides, more than the %d estimated", report.Slides, report.Estimate))
		}
	}
	switch {
	case report.Passed != nil:
		report.State = "passed"
		report.Next = "Nothing: it passed. Work after it may start."
	case len(report.Unfinished) > 0:
		report.State = "in progress"
		report.Next = "Finish its Tasks: " + strings.Join(report.Unfinished, ", ") + "."
	case report.Review == "":
		report.State = "reached"
		report.Next = fmt.Sprintf("Prove the work works: run the tests, then create its review of the checkpoint's branch into the Plan's feature branch with change-saga review create --base FEATURE --head BRANCH, and link it with change-saga plan link-review --plan %s --review ID --checkpoint %s.", plan.ID, phase.ID)
	case report.Passer == saga.CheckpointPasserAgent && len(report.Surprises) == 0:
		report.State = "reached"
		report.Next = fmt.Sprintf("Nothing here is surprising, so the agent may pass it: change-saga plan pass --plan %s --checkpoint %s --agent --reason TEXT, then merge.", plan.ID, phase.ID)
	case report.Passer == saga.CheckpointPasserAgent:
		report.State = "reached"
		report.Next = "Stop for a person: something here is surprising. They pass it with change-saga plan pass --by NAME --reason TEXT."
	default:
		report.State = "reached"
		report.Next = fmt.Sprintf("Ask whoever invoked the Plan to pass it: change-saga plan pass --plan %s --checkpoint %s --by NAME --reason TEXT.", plan.ID, phase.ID)
	}
	return report, nil
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
	report, err := reportCheckpoint(ctx, document, found, *checkpoint, storyTitles(document.Root, document.Manifest.ID))
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
		fmt.Fprintf(out, "    %s (%d %s)\n", story.Title, story.Criteria, plural(story.Criteria, "criterion", "criteria"))
	}
	size := fmt.Sprintf("  About %d %s to explain, one per story", report.Estimate, plural(report.Estimate, "slide", "slides"))
	if report.Slides > 0 {
		size += fmt.Sprintf("; its review %s took %d", report.Review, report.Slides)
	}
	fmt.Fprintln(out, size+".")
	if report.Estimate > checkpointSlideBudget {
		fmt.Fprintf(out, "  That is more than %d: consider splitting it with another checkpoint.\n", checkpointSlideBudget)
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
