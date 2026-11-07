package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/plancheckpoint"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

var planOperations = []string{"create", "list", "guide", "set", "add-phase", "remove-phase", "add-prototype", "link-review", "checkpoint", "expect", "pass", "widen", "deliver", "add-name", "set-name", "remove-name", "names"}

// Plan dispatches the Plan commands.
func Plan(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("plan", planOperations, out)
	}
	operation := "plan " + args[0]
	var err error
	switch args[0] {
	case "create":
		err = planCreate(ctx, args[1:], out)
	case "list":
		err = planList(ctx, args[1:], out)
	case "guide":
		err = planGuide(ctx, args[1:], out)
	case "remove-phase":
		err = planRemovePhase(ctx, args[1:], out)
	case "add-phase":
		err = planAddPhase(ctx, args[1:], out)
	case "add-prototype":
		err = planAddPrototype(ctx, args[1:], out)
	case "link-review":
		err = planLinkReview(ctx, args[1:], out)
	case "deliver":
		err = planDeliver(ctx, args[1:], out)
	case "set":
		err = planSet(ctx, args[1:], out)
	case "pass":
		err = planPass(ctx, args[1:], out)
	case "checkpoint":
		err = planCheckpoint(ctx, args[1:], out)
	case "widen":
		err = planWiden(ctx, args[1:], out)
	case "expect":
		err = planExpect(ctx, args[1:], out)
	case "add-name":
		err = planAddName(ctx, args[1:], out)
	case "set-name":
		err = planSetName(ctx, args[1:], out)
	case "remove-name":
		err = planRemoveName(ctx, args[1:], out)
	case "names":
		err = planNames(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["plan"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

// planIDOf accepts a Plan's id or URN.
func planIDOf(value string) string {
	if id := saga.PlanIDOf(value); id != "" {
		return id
	}
	return value
}

func planCreate(_ context.Context, args []string, out io.Writer) error {
	name := "plan create"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable plan id")
	title := flags.String("title", "", "plan title")
	objective := flags.String("objective", "", "what the plan's deck explains; defaults to the work and how it can run in parallel")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *title); err != nil {
		return err
	}
	if *objective == "" {
		*objective = "Explain the work to be done and how it can be executed in parallel."
	}
	root := flags.Arg(0)
	manifest := saga.PlanManifest{ID: *id, Title: strings.TrimSpace(*title)}
	deck := saga.DeckManifest{ID: *id, Title: manifest.Title, Objective: strings.TrimSpace(*objective)}
	if err := planstore.Create(root, manifest, deck); err != nil {
		return err
	}
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.PlanTarget(sagaManifest.ID, *id)
	path := saga.PlansDir + "/" + *id + saga.PlanSuffix
	created := []string{urn, saga.PlanDeckTarget(sagaManifest.ID, *id, *id)}
	if err := writeLivingMutation(out, name, urn, path, created, nil, false, *jsonOutput); err != nil {
		return err
	}
	if !*jsonOutput {
		fmt.Fprintf(out, "Next: change-saga task add --plan %s --id ID --title TEXT --story URN %s, then change-saga apply-slide --plan %s --from SLIDE.json %s\n", *id, root, *id, root)
	}
	return nil
}

// planSummary is one Plan as plan list reports it.
type planSummary struct {
	ID        string   `json:"id"`
	Target    string   `json:"target"`
	Title     string   `json:"title"`
	Delivered bool     `json:"delivered"`
	Tasks     int      `json:"tasks"`
	TasksDone int      `json:"tasks_done"`
	Reviews   []string `json:"reviews"`
	Stories   []string `json:"stories"`
	Features  []string `json:"features"`
}

// storyFeatures maps each story URN to the feature that holds it, which is how a
// Plan's features are worked out from its Tasks.
func storyFeatures(root, sagaID string) (map[string]string, error) {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return nil, err
	}
	features := make(map[string]string, len(document.Stories))
	for _, story := range document.Stories {
		features["urn:change-saga:"+sagaID+":story:"+story.Identity.ID] = story.Feature
	}
	return features, nil
}

func summarizePlan(plan *saga.Plan, features map[string]string) planSummary {
	summary := planSummary{ID: plan.ID, Target: plan.Target, Title: plan.Title, Delivered: plan.Delivered(), Tasks: len(plan.Tasks),
		Reviews: append([]string{}, plan.Reviews...), Stories: plan.PlanStories(), Features: []string{}}
	for _, task := range plan.Tasks {
		if task.State == saga.TaskDone {
			summary.TasksDone++
		}
	}
	seen := map[string]bool{}
	for _, story := range summary.Stories {
		if feature := features[story]; feature != "" && !seen[feature] {
			seen[feature] = true
			summary.Features = append(summary.Features, feature)
		}
	}
	sort.Strings(summary.Features)
	return summary
}

func planList(_ context.Context, args []string, out io.Writer) error {
	name := "plan list"
	flags := commandFlags(name, commandUsage[name], out)
	feature := flags.String("feature", "", "only Plans whose Tasks lead to a story in this feature")
	story := flags.String("story", "", "only Plans with a Task naming this story")
	all := flags.Bool("all", false, "also print delivered Plans, which the text hides by default; --json always lists every Plan")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	document, validation, err := saga.Load(flags.Arg(0))
	if err != nil {
		return err
	}
	if !validation.Valid {
		return fmt.Errorf("the Saga is invalid; run change-saga validate")
	}
	features, err := storyFeatures(document.Root, document.Manifest.ID)
	if err != nil {
		return err
	}
	summaries := []planSummary{}
	for _, plan := range document.Plans {
		summary := summarizePlan(plan, features)
		if *story != "" && !slices.Contains(summary.Stories, *story) {
			continue
		}
		if *feature != "" && !slices.Contains(summary.Features, *feature) {
			continue
		}
		summaries = append(summaries, summary)
	}
	if *jsonOutput {
		return writeJSON(out, struct {
			Plans []planSummary `json:"plans"`
		}{summaries})
	}
	hidden := 0
	shown := 0
	for _, summary := range summaries {
		if summary.Delivered && !*all {
			hidden++
			continue
		}
		shown++
		state := "open"
		if summary.Delivered {
			state = "delivered"
		}
		fmt.Fprintf(out, "%s: %s (%s)\n  %d of %d tasks done; touches %d %s in %s", summary.ID, summary.Title, state, summary.TasksDone, summary.Tasks,
			len(summary.Stories), plural(len(summary.Stories), "story", "stories"), plansFeatureList(summary.Features))
		if len(summary.Reviews) > 0 {
			fmt.Fprintf(out, "; reviews: %s", strings.Join(summary.Reviews, ", "))
		}
		fmt.Fprintln(out)
	}
	if shown == 0 && hidden == 0 {
		fmt.Fprintln(out, "No Plans. Write one with change-saga plan create.")
	} else if shown == 0 {
		fmt.Fprintln(out, "No open Plans.")
	}
	if hidden > 0 {
		fmt.Fprintf(out, "%d delivered %s hidden; --all shows them.\n", hidden, plural(hidden, "plan", "plans"))
	}
	return nil
}

func plansFeatureList(features []string) string {
	if len(features) == 0 {
		return "no features yet"
	}
	return strings.Join(features, ", ")
}

// planGuideWorkflow is the guided way to a Plan someone can be confident in. An
// agent follows it with the person, researching and prototyping until each Task
// is well understood. The command reports; the judgment stays with them.
const planGuideWorkflow = `Guide to a Plan you can be confident in

Work through this with the person who asked. A Plan is well understood when each Task
is reasonably granular: a coherent piece of work that one human developer, coding it
without AI help, would finish in two days or less.

1. Research. Find the stories this work serves with change-saga query requirements,
   and read the code they touch. Present what you found, including what does not exist
   yet and what surprised you, before proposing anything.
2. Ask. Settle what you cannot answer from the code. Do not invent intent. If the work
   has no story to serve, write the story first: every Task must lead to one.
3. Draft. Create the Plan with plan create, its phases with plan add-phase (a phase's
   Tasks run in parallel, so a Task only depends on earlier phases), and its
   Tasks top-down with task add: the Task naming the story, then the prerequisites it
   needs with --unblocks. Dependencies decide what can run in parallel.
4. Prototype. Where an experience or an approach is unclear, build a prototype and
   attach it with plan add-prototype. Research and prototypes are how confidence is
   earned, so go back to step 1 whenever a Task is still vague.
5. Size. Judge each Task as one human developer coding it without AI help, not by how
   long an agent would take, and record it with task set --sizing. Split a Task that is
   too big or not understood, but not so finely that it stops being a meaningful unit of
   work. Never record within-2-days to get past this step.
6. Explain. Write the technical design as the deck with apply-slide --plan and its
   essay with deck overview --plan, organized by phase. Give each diagram node that
   represents work an Item that selects the node and whose record names its Task: that
   Item is what lets a reader select the node and open the Task.
7. Checkpoint. A large Plan is reviewed in stretches. After a few phases, add a
   checkpoint with plan add-phase --checkpoint [--after PHASE]: it holds no Tasks and
   covers every Task since the previous one. State what must be true by then with plan
   expect: invariants of the work, not stories. Size each stretch so its review takes five
   or six slides, about one per story, not ten to twenty. Ask who passes them, and record
   it with plan set --checkpoint-passer person|agent.
8. Check. Run plan guide --plan ID until it has nothing left to name.

While the work happens, one agent owns the Plan: the operator. It picks the Tasks
task list --ready offers, delegates each to an implementing agent with the Task's URN,
and keeps the record. Once a Task's work is merged into the operator's branch, the
operator runs task done; it also passes checkpoints, merges them, and updates the Saga.
Implementors focus on delivery: code and tests on their own branch. They read their
Task with query task, report with task start when they begin, task note on a finding
that changes the work, and task note --blocked when they cannot go on, and leave the
Saga's records to the operator. Do not narrate: every note is a tool call. task start
and task note are live activity, shared by every worktree of this repository and never
committed. task done is durable, and it is the one that counts.

At a checkpoint, the operator stops and proves the stretch works: run the tests, then
plan checkpoint --plan ID --checkpoint PHASE says what it covers and what to do next.
Review the checkpoint's branch as a change into the Plan's feature branch. Once it
passes, by its invoker or by the operator when they allowed it and nothing is
surprising, the operator merges it there and delegates the next stretch from it.
`

func planGuide(ctx context.Context, args []string, out io.Writer) error {
	name := "plan guide"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "report what this Plan still needs, instead of only the workflow")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags); err != nil {
		return err
	}
	fmt.Fprint(out, planGuideWorkflow)
	if *plan == "" {
		return nil
	}
	document, found, err := planOf(flags.Arg(0), *plan)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\nPlan %s:\n", found.ID)
	notReady := 0
	if len(found.Tasks) == 0 {
		notReady++
		fmt.Fprintln(out, "  It has no tasks yet.")
	}
	for _, task := range found.Tasks {
		switch {
		case task.Sizing == nil:
			notReady++
			fmt.Fprintf(out, "  Not sized yet: %s (%s)\n", task.ID, task.Title)
		case !task.Sizing.WithinTwoDays:
			notReady++
			fmt.Fprintf(out, "  Judged larger than two days, so split it: %s (%s)\n", task.ID, task.Title)
		}
	}
	// Once a Plan has names, work on none is unexpected and a name no Task
	// reaches may be unneeded. A Plan without names is not asked to have them.
	if view := saga.PlanNames(found.NameManifests(), found.Manifests()); view != nil {
		notReady += len(view.Unnamed) + len(view.Unreached)
		writeNameGaps(out, found, view)
	}
	var titles map[string]plancheckpoint.Story
	for _, phase := range found.Phases {
		if !phase.IsCheckpoint() || phase.CheckpointPassed() {
			continue
		}
		if titles == nil {
			titles = plancheckpoint.Stories(document.Root, document.Manifest.ID)
		}
		report, err := reportCheckpoint(ctx, document, found, phase.ID, titles)
		if err != nil {
			return err
		}
		if len(phase.Checkpoint.Expectations) == 0 {
			notReady++
			fmt.Fprintf(out, "  Checkpoint %s states no expectations: say what must be true by then with plan expect.\n", phase.ID)
		}
		if report.Estimate > checkpointSlideBudget {
			notReady++
			fmt.Fprintf(out, "  Checkpoint %s addresses %d stories in %d tasks: about %d slides to explain, more than %d. Split it with another checkpoint.\n", phase.ID, len(report.Stories), report.Tasks, report.Estimate, checkpointSlideBudget)
		}
	}
	if found.Deck == nil || len(found.Deck.Slides) == 0 {
		notReady++
		fmt.Fprintln(out, "  Its deck has no slides yet.")
	} else if found.Deck.Overview == nil {
		notReady++
		fmt.Fprintln(out, "  Its deck has no essay yet.")
	}
	if notReady == 0 {
		fmt.Fprintln(out, "  Every task is judged two days or less, and the deck has slides and an essay. Whether it is right is still for people to judge.")
	}
	return nil
}

func planAddPhase(_ context.Context, args []string, out io.Writer) error {
	name := "plan add-phase"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "stable phase id")
	title := flags.String("title", "", "phase title")
	after := flags.String("after", "", "put the phase right after this one instead of at the end")
	checkpoint := flags.Bool("checkpoint", false, "make it a checkpoint: it holds no Tasks, covers every Task since the previous checkpoint, and holds the work after it until it passes")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id, *title); err != nil {
		return err
	}
	phase := saga.PlanPhase{ID: *id, Title: strings.TrimSpace(*title)}
	if *checkpoint {
		phase.Checkpoint = &saga.PlanCheckpoint{}
	}
	if err := planstore.AddPhase(flags.Arg(0), planIDOf(*plan), phase, *after); err != nil {
		return err
	}
	return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), *jsonOutput)
}

func planRemovePhase(_ context.Context, args []string, out io.Writer) error {
	name := "plan remove-phase"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "phase id")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id); err != nil {
		return err
	}
	if err := planstore.RemovePhase(flags.Arg(0), planIDOf(*plan), *id); err != nil {
		return err
	}
	return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), *jsonOutput)
}

func planLinkReview(_ context.Context, args []string, out io.Writer) error {
	name := "plan link-review"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	review := flags.String("review", "", "id of a review that implements the plan")
	checkpoint := flags.String("checkpoint", "", "the checkpoint this review covers")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *review); err != nil {
		return err
	}
	if err := planstore.LinkCheckpointReview(flags.Arg(0), planIDOf(*plan), *review, *checkpoint); err != nil {
		return err
	}
	return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), *jsonOutput)
}

func planAddPrototype(_ context.Context, args []string, out io.Writer) error {
	name := "plan add-prototype"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	prototype := flags.String("prototype", "", "prototype URN")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *prototype); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	prefix := "urn:change-saga:" + sagaManifest.ID + ":prototype:"
	if id, found := strings.CutPrefix(*prototype, prefix); !found || !saga.ValidID(id) {
		return fmt.Errorf("--prototype must be a prototype URN of this Saga: %s<id>", prefix)
	}
	if err := planstore.AddPrototype(root, planIDOf(*plan), *prototype); err != nil {
		return err
	}
	return writePlanChange(out, name, root, planIDOf(*plan), *jsonOutput)
}

func planDeliver(ctx context.Context, args []string, out io.Writer) error {
	name := "plan deliver"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan); err != nil {
		return err
	}
	root := flags.Arg(0)
	landings := reviewstate.NewLandings(firstNonEmpty(*repo, root))
	if err := planstore.Deliver(root, planIDOf(*plan), func(review *saga.Review) bool {
		return landings.Detect(ctx, review).Merged()
	}); err != nil {
		return err
	}
	return writePlanChange(out, name, root, planIDOf(*plan), *jsonOutput)
}

func planSet(_ context.Context, args []string, out io.Writer) error {
	name := "plan set"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	passer := flags.String("checkpoint-passer", "", "who passes the Plan's checkpoints: person, or agent, which passes them unless what it found is surprising or urgent")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *passer); err != nil {
		return err
	}
	if err := planstore.SetCheckpointPasser(flags.Arg(0), planIDOf(*plan), *passer); err != nil {
		return err
	}
	return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), *jsonOutput)
}

func planPass(ctx context.Context, args []string, out io.Writer) error {
	name := "plan pass"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	checkpoint := flags.String("checkpoint", "", "the checkpoint phase to pass")
	reason := flags.String("reason", "", "why it passes: what proves the work works")
	by := flags.String("by", "", "who passes it; required unless --agent, which defaults to the reporting agent's name")
	agent := flags.Bool("agent", false, "an agent passes it, under the rule of whoever invoked the Plan")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var metValues stringList
	flags.Var(&metValues, "met", "ID=HOW: how one of the checkpoint's expectations was shown to hold; repeat for each")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *checkpoint, *reason); err != nil {
		return err
	}
	met := []saga.MetExpectation{}
	for _, value := range metValues {
		id, how, found := strings.Cut(value, "=")
		if !found || strings.TrimSpace(id) == "" || strings.TrimSpace(how) == "" {
			return fmt.Errorf("--met %s must be ID=HOW", value)
		}
		met = append(met, saga.MetExpectation{Expectation: strings.TrimSpace(id), How: strings.TrimSpace(how)})
	}
	root := flags.Arg(0)
	who := strings.TrimSpace(*by)
	if who == "" && *agent {
		who = planactivity.Identity("", os.Getenv, planactivity.Branch(ctx, root))
	}
	if who == "" {
		return fmt.Errorf("--by names who passes the checkpoint; an agent passes with --agent")
	}
	planID := planIDOf(*plan)
	pass := saga.CheckpointPass{At: time.Now().UTC(), By: who, Agent: *agent, Reason: strings.TrimSpace(*reason), Met: met}
	// Where the checkpoint's review began, so a later review can widen back to it.
	if document, found, err := planOf(root, planID); err == nil {
		for _, phase := range found.Phases {
			if phase.ID == *checkpoint && phase.IsCheckpoint() {
				if review := document.FindReview(phase.Checkpoint.Review); review != nil {
					if reviewRange, err := reviewstate.ResolveRange(ctx, firstNonEmpty(*repo, root), review); err == nil {
						pass.Base = reviewRange.BaseOID
					}
				}
			}
		}
	}
	if err := planstore.PassCheckpoint(root, planID, *checkpoint, pass); err != nil {
		return err
	}
	if *jsonOutput {
		return writePlanChange(out, name, root, planID, true)
	}
	fmt.Fprintf(out, "Passed checkpoint %s as %s.\n", *checkpoint, who)
	into := "the branch the Plan's work comes from"
	if document, found, err := planOf(root, planID); err == nil {
		for _, phase := range found.Phases {
			if phase.ID == *checkpoint && phase.Checkpoint.Review != "" {
				if review := document.FindReview(phase.Checkpoint.Review); review != nil && review.Base != "" {
					into = review.Base
				}
			}
		}
	}
	fmt.Fprintf(out, "Next: merge the checkpoint's branch into %s, then start the next stretch from there in a new workspace.\n", into)
	return nil
}

func planExpect(_ context.Context, args []string, out io.Writer) error {
	name := "plan expect"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	checkpoint := flags.String("checkpoint", "", "the checkpoint phase")
	id := flags.String("id", "", "stable expectation id")
	statement := flags.String("statement", "", "what must be true by the checkpoint: an invariant or characteristic of the work, not a user story")
	remove := flags.Bool("remove", false, "remove the expectation instead")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *checkpoint, *id); err != nil {
		return err
	}
	if !*remove && strings.TrimSpace(*statement) == "" {
		return fmt.Errorf("--statement says what must be true; --remove removes the expectation")
	}
	if !saga.ValidID(*id) {
		return fmt.Errorf("--id must be a stable identifier")
	}
	value := *statement
	if *remove {
		value = ""
	}
	if err := planstore.SetExpectation(flags.Arg(0), planIDOf(*plan), *checkpoint, *id, value); err != nil {
		return err
	}
	return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), *jsonOutput)
}

func planWiden(_ context.Context, args []string, out io.Writer) error {
	name := "plan widen"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	checkpoint := flags.String("checkpoint", "", "the checkpoint whose review to widen")
	to := flags.String("to", "", "an earlier checkpoint to show every change since; the checkpoint itself shows only its own stretch again")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *checkpoint, *to); err != nil {
		return err
	}
	if err := planstore.WidenCheckpointReview(flags.Arg(0), planIDOf(*plan), *checkpoint, *to); err != nil {
		return err
	}
	if *jsonOutput {
		return writePlanChange(out, name, flags.Arg(0), planIDOf(*plan), true)
	}
	if *to == *checkpoint {
		fmt.Fprintf(out, "Checkpoint %s's review shows its own stretch again.\n", *checkpoint)
	} else {
		fmt.Fprintf(out, "Checkpoint %s's review now shows every change since checkpoint %s's review began.\n", *checkpoint, *to)
	}
	return nil
}

func writePlanChange(out io.Writer, operation, root, planID string, jsonOutput bool) error {
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.PlanTarget(sagaManifest.ID, planID)
	if jsonOutput {
		return writeLivingMutation(out, operation, urn, saga.PlansDir+"/"+planID+saga.PlanSuffix, nil, nil, false, true)
	}
	fmt.Fprintf(out, "Updated %s\n", urn)
	return nil
}

// planOf reads the Plan a read-only command names.
func planOf(root, value string) (*saga.Saga, *saga.Plan, error) {
	document, validation, err := saga.Load(root)
	if err != nil {
		return nil, nil, err
	}
	if !validation.Valid {
		return nil, nil, fmt.Errorf("the Saga is invalid; run change-saga validate")
	}
	plan := document.FindPlan(planIDOf(value))
	if plan == nil {
		return nil, nil, fmt.Errorf("plan %q does not exist", value)
	}
	return document, plan, nil
}

// Task dispatches the Task commands.
func Task(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("task", []string{"add", "depend", "set", "start", "note", "done", "remove", "list"}, out)
	}
	operation := "task " + args[0]
	var err error
	switch args[0] {
	case "add":
		err = taskAdd(ctx, args[1:], out)
	case "depend":
		err = taskDepend(ctx, args[1:], out)
	case "set":
		err = taskSet(ctx, args[1:], out)
	case "start":
		err = taskStart(ctx, args[1:], out)
	case "note":
		err = taskNote(ctx, args[1:], out)
	case "done":
		err = taskDone(ctx, args[1:], out)
	case "remove":
		err = taskRemove(ctx, args[1:], out)
	case "list":
		err = taskList(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["task"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

// taskSelection accepts a Task's URN, which names its Plan, or its id with --plan.
func taskSelection(plan, task string) (string, string, error) {
	if owner := saga.PlanIDOf(task); owner != "" {
		_, id, found := strings.Cut(task, ":task:")
		if !found || !saga.ValidID(id) {
			return "", "", fmt.Errorf("--task must be a task id or a task URN")
		}
		return owner, id, nil
	}
	if plan == "" {
		return "", "", fmt.Errorf("--plan is required unless --task is a task URN")
	}
	return planIDOf(plan), task, nil
}

// taskCode pins each PATH:START-END at HEAD, so a Task's code references say
// exactly which lines mattered when they were written.
func taskCode(ctx context.Context, repo string, values []string) ([]coderef.Reference, error) {
	if len(values) == 0 {
		return nil, nil
	}
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		return nil, err
	}
	defer resolver.Close()
	head, err := resolveCommit(ctx, repo, "HEAD")
	if err != nil {
		return nil, err
	}
	references := make([]coderef.Reference, 0, len(values))
	for _, value := range values {
		spelled, note, _ := strings.Cut(value, "=")
		path, lines, found := cutLast(spelled, ":")
		first, last, ranged := strings.Cut(lines, "-")
		if !ranged {
			last = first
		}
		start, startErr := strconv.Atoi(first)
		end, endErr := strconv.Atoi(last)
		if !found || startErr != nil || endErr != nil {
			return nil, fmt.Errorf("--code %s must name exact lines: PATH:START-END", value)
		}
		reference, err := resolver.Author(ctx, coderef.Location{Commit: head, Path: path, Start: start, End: end}, note)
		if err != nil {
			return nil, fmt.Errorf("--code %s: %w", value, err)
		}
		references = append(references, reference)
	}
	return references, nil
}

// cutLast splits value around its last separator.
func cutLast(value, separator string) (string, string, bool) {
	index := strings.LastIndex(value, separator)
	if index < 0 {
		return value, "", false
	}
	return value[:index], value[index+len(separator):], true
}

func taskSizing(value, reason string) (*saga.TaskSizing, error) {
	switch value {
	case "":
		if reason != "" {
			return nil, fmt.Errorf("--sizing-reason needs --sizing")
		}
		return nil, nil
	case "within-2-days":
		return &saga.TaskSizing{WithinTwoDays: true, Reason: reason}, nil
	case "over-2-days":
		return &saga.TaskSizing{WithinTwoDays: false, Reason: reason}, nil
	}
	return nil, fmt.Errorf("--sizing must be within-2-days or over-2-days")
}

func readEssay(source string) (string, error) {
	if source == "-" {
		data, err := io.ReadAll(os.Stdin)
		return string(data), err
	}
	data, err := os.ReadFile(source)
	return string(data), err
}

func requireStory(root, story string) error {
	if story == "" {
		return nil
	}
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(story, "urn:change-saga:"+sagaManifest.ID+":story:") {
		return fmt.Errorf("--story must be a story URN of this Saga: urn:change-saga:%s:story:<id>", sagaManifest.ID)
	}
	return requireAppRecord(root, sagaManifest.ID, story)
}

func taskAdd(ctx context.Context, args []string, out io.Writer) error {
	name := "task add"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	id := flags.String("id", "", "stable task id")
	title := flags.String("title", "", "task title")
	phase := flags.String("phase", "", "phase id")
	story := flags.String("story", "", "URN of the user story this task is part of")
	essaySource := flags.String("essay-source", "", "Markdown file, or - for standard input")
	sizing := flags.String("sizing", "", "within-2-days or over-2-days: whether one human developer, coding without AI help, would finish this in two days or less")
	sizingReason := flags.String("sizing-reason", "", "why")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var unblocks, dependsOn, code stringList
	flags.Var(&unblocks, "unblocks", "task this one unblocks; repeatable")
	flags.Var(&dependsOn, "depends-on", "task this one depends on; repeatable")
	flags.Var(&code, "code", "PATH:START-END[=NOTE] to pin at HEAD; repeatable")
	var names stringList
	flags.Var(&names, "name", "a name of the Plan this task works on; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id, *title); err != nil {
		return err
	}
	root := flags.Arg(0)
	if err := requireStory(root, *story); err != nil {
		return err
	}
	value := saga.TaskManifest{ID: *id, Title: strings.TrimSpace(*title), Phase: *phase, Story: *story, DependsOn: dependsOn, Names: taskNames(names)}
	var err error
	if value.Sizing, err = taskSizing(*sizing, *sizingReason); err != nil {
		return err
	}
	if value.Code, err = taskCode(ctx, firstNonEmpty(*repo, root), code); err != nil {
		return err
	}
	essay := ""
	if *essaySource != "" {
		if essay, err = readEssay(*essaySource); err != nil {
			return err
		}
	}
	urn, err := planstore.AddTask(root, planIDOf(*plan), value, unblocks, essay)
	if err != nil {
		return err
	}
	path := saga.PlansDir + "/" + planIDOf(*plan) + saga.PlanSuffix + "/" + saga.PlanTasksDir + "/" + *id + saga.TaskSuffix
	return writeLivingMutation(out, name, urn, path, []string{urn}, nil, false, *jsonOutput)
}

func taskDepend(_ context.Context, args []string, out io.Writer) error {
	name := "task depend"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	task := flags.String("task", "", "task id or URN")
	on := flags.String("on", "", "the task it depends on")
	remove := flags.Bool("remove", false, "remove the dependency instead")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *task, *on); err != nil {
		return err
	}
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	err = planstore.UpdateTask(flags.Arg(0), planID, taskID, func(value *saga.TaskManifest) error {
		kept := make([]string, 0, len(value.DependsOn)+1)
		found := false
		for _, existing := range value.DependsOn {
			if existing == *on {
				found = true
				if *remove {
					continue
				}
			}
			kept = append(kept, existing)
		}
		if *remove && !found {
			return fmt.Errorf("task %s does not depend on %s", taskID, *on)
		}
		if !*remove && !found {
			kept = append(kept, *on)
		}
		value.DependsOn = kept
		return nil
	}, nil)
	if err != nil {
		return err
	}
	return writeTaskChange(out, name, flags.Arg(0), planID, taskID, *jsonOutput)
}

func taskSet(ctx context.Context, args []string, out io.Writer) error {
	name := "task set"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	task := flags.String("task", "", "task id or URN")
	title := flags.String("title", "", "task title")
	phase := flags.String("phase", "", "phase id; empty clears it")
	story := flags.String("story", "", "URN of the user story; empty clears it")
	state := flags.String("state", "", "planned, in_progress, or done")
	essaySource := flags.String("essay-source", "", "Markdown file, or - for standard input; an empty file removes the essay")
	sizing := flags.String("sizing", "", "within-2-days or over-2-days: whether one human developer, coding without AI help, would finish this in two days or less")
	sizingReason := flags.String("sizing-reason", "", "why")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	agent := flags.String("agent", "", "who is reporting a --state change; defaults to the DevSwarm workspace, else the Git branch")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var code stringList
	flags.Var(&code, "code", "PATH:START-END[=NOTE] to pin at HEAD; repeatable; replaces the task's code references")
	var names stringList
	flags.Var(&names, "name", "a name of the Plan this task works on; repeatable; replaces the task's names, and one empty value clears them")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *task); err != nil {
		return err
	}
	given := map[string]bool{}
	flags.Visit(func(value *flag.Flag) { given[value.Name] = true })
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	root := flags.Arg(0)
	if given["story"] {
		if err := requireStory(root, *story); err != nil {
			return err
		}
	}
	size, err := taskSizing(*sizing, *sizingReason)
	if err != nil {
		return err
	}
	references, err := taskCode(ctx, firstNonEmpty(*repo, root), code)
	if err != nil {
		return err
	}
	var essay *string
	if *essaySource != "" {
		text, err := readEssay(*essaySource)
		if err != nil {
			return err
		}
		essay = &text
	}
	err = planstore.UpdateTask(root, planID, taskID, func(value *saga.TaskManifest) error {
		if given["title"] {
			value.Title = strings.TrimSpace(*title)
		}
		if given["phase"] {
			value.Phase = *phase
		}
		if given["story"] {
			value.Story = *story
		}
		if given["state"] {
			value.State = *state
		}
		if size != nil {
			value.Sizing = size
		}
		if len(code) > 0 {
			value.Code = references
		}
		if given["name"] {
			value.Names = taskNames(names)
		}
		return nil
	}, essay)
	if err != nil {
		return err
	}
	if given["state"] {
		recordTaskStateActivity(ctx, root, *repo, *agent, planID, taskID, *state, "")
	}
	return writeTaskChange(out, name, root, planID, taskID, *jsonOutput)
}

func taskDone(ctx context.Context, args []string, out io.Writer) error {
	name := "task done"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	task := flags.String("task", "", "task id or URN")
	summary := flags.String("summary", "", "what was done")
	activity := addActivityFlags(flags)
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *task); err != nil {
		return err
	}
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	err = planstore.UpdateTask(flags.Arg(0), planID, taskID, func(value *saga.TaskManifest) error {
		value.State = saga.TaskDone
		if *summary != "" {
			value.Summary = *summary
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	recordTaskStateActivity(ctx, flags.Arg(0), *activity.repo, *activity.agent, planID, taskID, saga.TaskDone, *summary)
	return writeTaskChange(out, name, flags.Arg(0), planID, taskID, *jsonOutput)
}

func taskRemove(_ context.Context, args []string, out io.Writer) error {
	name := "task remove"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	task := flags.String("task", "", "task id or URN")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *task); err != nil {
		return err
	}
	planID, taskID, err := taskSelection(*plan, *task)
	if err != nil {
		return err
	}
	if err := planstore.RemoveTask(flags.Arg(0), planID, taskID); err != nil {
		return err
	}
	return writeTaskChange(out, name, flags.Arg(0), planID, taskID, *jsonOutput)
}

func writeTaskChange(out io.Writer, operation, root, planID, taskID string, jsonOutput bool) error {
	sagaManifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	urn := saga.TaskTarget(sagaManifest.ID, planID, taskID)
	if jsonOutput {
		return writeLivingMutation(out, operation, urn, "", nil, nil, false, true)
	}
	fmt.Fprintf(out, "Updated %s\n", urn)
	return nil
}

// taskRow is one Task as task list reports it.
type taskRow struct {
	saga.TaskManifest
	Target   string   `json:"target"`
	Unblocks []string `json:"unblocks"`
	CanStart bool     `json:"can_start"`
	// HeldBy is the title of the checkpoint the Task waits for, when one holds it.
	HeldBy string `json:"held_by,omitempty"`
	// Activity is the latest live activity, when there is any.
	Activity *taskActivityView `json:"activity,omitempty"`
}

func taskList(ctx context.Context, args []string, out io.Writer) error {
	name := "task list"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	ready := flags.Bool("ready", false, "only Tasks that can start now: not started, with every dependency done; one someone has started says so")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan); err != nil {
		return err
	}
	_, found, err := planOf(flags.Arg(0), *plan)
	if err != nil {
		return err
	}
	manifests := found.Manifests()
	unblocks := saga.TaskUnblocks(manifests)
	canStart := map[string]bool{}
	readyNow, held := saga.TasksReadyNow(found.Phases, manifests)
	for _, task := range readyNow {
		canStart[task.ID] = true
	}
	heldReady := map[string]int{}
	for _, task := range saga.TasksReady(manifests) {
		if checkpoint := held[task.ID]; checkpoint != "" {
			heldReady[checkpoint]++
		}
	}
	live := liveTaskActivity(ctx, flags.Arg(0), *repo, found.ID)
	now := time.Now()
	rows := []taskRow{}
	for _, task := range found.Tasks {
		if *ready && !canStart[task.ID] {
			continue
		}
		rows = append(rows, taskRow{TaskManifest: task.TaskManifest, Target: task.Target, Unblocks: append([]string{}, unblocks[task.ID]...), CanStart: canStart[task.ID], HeldBy: held[task.ID], Activity: activityView(live[task.ID], task.State, now)})
	}
	if *jsonOutput {
		return writeJSON(out, struct {
			Plan  string    `json:"plan"`
			Tasks []taskRow `json:"tasks"`
		}{found.Target, rows})
	}
	for _, row := range rows {
		state := row.State
		if row.CanStart {
			state = "can start"
		}
		if row.HeldBy != "" && row.State == saga.TaskPlanned {
			state = "held by checkpoint " + row.HeldBy
		}
		if row.Activity != nil {
			state = activityWords(row.Activity, now)
		}
		fmt.Fprintf(out, "%s: %s (%s)\n", row.ID, row.Title, state)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "No tasks.")
	}
	if *ready {
		for _, phase := range found.Phases {
			if count := heldReady[phase.Title]; count > 0 && phase.IsCheckpoint() {
				fmt.Fprintf(out, "%d more %s held by checkpoint %s (%s), which has not passed.\n", count, plural(count, "task is", "tasks are"), phase.ID, phase.Title)
			}
		}
	}
	return nil
}

// taskNames reads --name values, dropping empty ones, so one empty value clears.
func taskNames(values []string) []string {
	names := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			names = append(names, value)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}
