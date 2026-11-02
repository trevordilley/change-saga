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

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

var planOperations = []string{"create", "list", "guide", "add-phase", "remove-phase", "add-prototype", "link-review", "deliver"}

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

Work through this with the person who asked. A Plan is well understood when you are
confident each Task would have taken two days or less before AI.

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
5. Size. Record each Task's judgment with task set --sizing. Split a Task that is too
   big or not understood. Never record within-2-days to get past this step.
6. Explain. Write the technical design as the deck with apply-slide --plan and its
   essay with deck overview --plan, organized by phase, with slides naming their Tasks.
7. Check. Run plan guide --plan ID until it has nothing left to name.
`

func planGuide(_ context.Context, args []string, out io.Writer) error {
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
	_, found, err := planOf(flags.Arg(0), *plan)
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
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *id, *title); err != nil {
		return err
	}
	if err := planstore.AddPhase(flags.Arg(0), planIDOf(*plan), saga.PlanPhase{ID: *id, Title: strings.TrimSpace(*title)}); err != nil {
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
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *plan, *review); err != nil {
		return err
	}
	if err := planstore.LinkReview(flags.Arg(0), planIDOf(*plan), *review); err != nil {
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
		return livingFamilyHelp("task", []string{"add", "depend", "set", "done", "remove", "list"}, out)
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
	sizing := flags.String("sizing", "", "within-2-days or over-2-days: whether this would have taken two days or less before AI")
	sizingReason := flags.String("sizing-reason", "", "why")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var unblocks, dependsOn, code stringList
	flags.Var(&unblocks, "unblocks", "task this one unblocks; repeatable")
	flags.Var(&dependsOn, "depends-on", "task this one depends on; repeatable")
	flags.Var(&code, "code", "PATH:START-END[=NOTE] to pin at HEAD; repeatable")
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
	value := saga.TaskManifest{ID: *id, Title: strings.TrimSpace(*title), Phase: *phase, Story: *story, DependsOn: dependsOn}
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
	sizing := flags.String("sizing", "", "within-2-days or over-2-days: whether this would have taken two days or less before AI")
	sizingReason := flags.String("sizing-reason", "", "why")
	repo := flags.String("repo", "", "code checkout when the Saga lives in a companion repository")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var code stringList
	flags.Var(&code, "code", "PATH:START-END[=NOTE] to pin at HEAD; repeatable; replaces the task's code references")
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
		return nil
	}, essay)
	if err != nil {
		return err
	}
	return writeTaskChange(out, name, root, planID, taskID, *jsonOutput)
}

func taskDone(_ context.Context, args []string, out io.Writer) error {
	name := "task done"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	task := flags.String("task", "", "task id or URN")
	summary := flags.String("summary", "", "what was done")
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
}

func taskList(_ context.Context, args []string, out io.Writer) error {
	name := "task list"
	flags := commandFlags(name, commandUsage[name], out)
	plan := flags.String("plan", "", "plan id or URN")
	ready := flags.Bool("ready", false, "only Tasks that can start now: not started, with every dependency done")
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
	for _, task := range saga.TasksReady(manifests) {
		canStart[task.ID] = true
	}
	rows := []taskRow{}
	for _, task := range found.Tasks {
		if *ready && !canStart[task.ID] {
			continue
		}
		rows = append(rows, taskRow{TaskManifest: task.TaskManifest, Target: task.Target, Unblocks: append([]string{}, unblocks[task.ID]...), CanStart: canStart[task.ID]})
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
		fmt.Fprintf(out, "%s: %s (%s)\n", row.ID, row.Title, state)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "No tasks.")
	}
	return nil
}
