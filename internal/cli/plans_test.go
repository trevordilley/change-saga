package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

const planStoryURN = "urn:change-saga:app:story:first-story"

// newPlanFixture is a Saga with one story and a Plan named first.
func newPlanFixture(t *testing.T) reviewFixture {
	t.Helper()
	fixture, _ := newEmptyReviewFixture(t)
	run(t, Story, "add", "--feature", testFeature, "--id", "first-story", "--revision", "r1", "--event", "proposed",
		"--title", "First story", "--statement", "As a user I can do the first thing", "--persona", personaURNFor("app"), fixture.root)
	run(t, Plan, "create", "--id", "first", "--title", "First plan", fixture.root)
	return fixture
}

// refused runs a command that must fail, and returns its error text.
func refused(t *testing.T, command func(context.Context, []string, *bytes.Buffer) error, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	err := command(context.Background(), args, &output)
	if err == nil {
		t.Fatalf("%v was accepted:\n%s", args, output.String())
	}
	return err.Error()
}

func taskCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Task(ctx, args, out)
}

func planCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return Plan(ctx, args, out)
}

func loadedPlan(t *testing.T, root, id string) *saga.Plan {
	t.Helper()
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: err=%v issues=%#v", err, validation.Issues)
	}
	plan := document.FindPlan(id)
	if plan == nil {
		t.Fatalf("plan %s does not exist", id)
	}
	return plan
}

func TestPlanCreateAddsAStandaloneRecordWithADeck(t *testing.T) {
	t.Parallel()
	plan := loadedPlan(t, newPlanFixture(t).root, "first")
	if plan.Target != "urn:change-saga:app:plan:first" || plan.Title != "First plan" {
		t.Fatalf("plan = %#v", plan.PlanManifest)
	}
	if plan.Deck == nil || plan.Deck.Role != saga.DeckRolePlan || plan.Deck.Target != "urn:change-saga:app:plan:first:deck:first" {
		t.Fatalf("deck = %#v", plan.Deck)
	}
}

func TestPlanCreateRefusesAnExistingPlan(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	message := refused(t, func(ctx context.Context, args []string, out *bytes.Buffer) error { return Plan(ctx, args, out) }, "create", "--id", "first", "--title", "Again", fixture.root)
	if !strings.Contains(message, "already exists") {
		t.Fatalf("message = %q", message)
	}
}

func TestTaskAddRequiresAPathToAStory(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	if message := refused(t, taskCommand, "add", "--plan", "first", "--id", "lonely", "--title", "Lonely", root); !strings.Contains(message, "leads to no user story") {
		t.Fatalf("a task with no story was not refused: %q", message)
	}
	if message := refused(t, taskCommand, "add", "--plan", "first", "--id", "gone", "--title", "Gone", "--story", "urn:change-saga:app:story:nope", root); !strings.Contains(message, "does not exist") {
		t.Fatalf("a missing story was not refused: %q", message)
	}
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	run(t, Task, "add", "--plan", "first", "--id", "refactor", "--title", "Refactor first", "--unblocks", "feature-work", root)
	plan := loadedPlan(t, root, "first")
	if plan.FindTask("refactor").Story != "" || len(plan.FindTask("feature-work").DependsOn) != 1 {
		t.Fatalf("tasks = %#v", plan.Manifests())
	}
}

func TestTaskChangesThatStrandATaskAreRefused(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	run(t, Task, "add", "--plan", "first", "--id", "refactor", "--title", "Refactor first", "--unblocks", "feature-work", root)

	for name, args := range map[string][]string{
		"removing the dependency":   {"depend", "--plan", "first", "--task", "feature-work", "--on", "refactor", "--remove", root},
		"clearing the only story":   {"set", "--plan", "first", "--task", "feature-work", "--story", "", root},
		"removing the Task with it": {"remove", "--plan", "first", "--task", "feature-work", root},
	} {
		if message := refused(t, taskCommand, args...); !strings.Contains(message, "task refactor: it leads to no user story") {
			t.Errorf("%s: message = %q", name, message)
		}
	}
	if message := refused(t, taskCommand, "depend", "--plan", "first", "--task", "refactor", "--on", "feature-work", root); !strings.Contains(message, "dependency cycle") {
		t.Errorf("a cycle was not refused: %q", message)
	}
	if plan := loadedPlan(t, root, "first"); len(plan.Tasks) != 2 || plan.FindTask("feature-work").Story != planStoryURN {
		t.Fatalf("a refused change wrote something: %#v", plan.Manifests())
	}
}

func TestTaskListReadyIsWhatCanStartNow(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	run(t, Task, "add", "--plan", "first", "--id", "one", "--title", "One", "--unblocks", "feature-work", root)
	run(t, Task, "add", "--plan", "first", "--id", "two", "--title", "Two", "--unblocks", "feature-work", root)

	ready := func() []string {
		var output bytes.Buffer
		if err := Task(context.Background(), []string{"list", "--plan", "first", "--ready", "--json", root}, &output); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Tasks []struct {
				ID string `json:"id"`
			} `json:"tasks"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, task := range result.Tasks {
			ids = append(ids, task.ID)
		}
		return ids
	}
	if got := strings.Join(ready(), ","); got != "one,two" {
		t.Fatalf("ready = %s, want one and two in parallel", got)
	}
	run(t, Task, "done", "--plan", "first", "--task", "one", "--summary", "Finished", root)
	run(t, Task, "done", "--task", "urn:change-saga:app:plan:first:task:two", root)
	if got := strings.Join(ready(), ","); got != "feature-work" {
		t.Fatalf("ready = %s, want feature-work once both prerequisites are done", got)
	}
	if plan := loadedPlan(t, root, "first"); plan.FindTask("one").State != saga.TaskDone || plan.FindTask("one").Summary != "Finished" {
		t.Fatalf("task one = %#v", plan.FindTask("one"))
	}
}

func TestTaskCodePinsExactLinesAtHead(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN,
		"--code", "queue.go:3-3=Where jobs go today", "--repo", fixture.repo, fixture.root)
	code := loadedPlan(t, fixture.root, "first").FindTask("feature-work").Code
	if len(code) != 1 || code[0].Path != "queue.go" || code[0].Start != 3 || code[0].Note != "Where jobs go today" || code[0].Digest == "" {
		t.Fatalf("code = %#v", code)
	}
}

func planSlideRequest(record string) SlideTransactionRequest {
	d := diagram.New()
	d.Elements = []diagram.Element{
		{ID: "title", Kind: "text", Label: "The work", X: 60, Y: 28, Width: 600, Height: 48, Style: "title", Decorative: true},
		{ID: "work", Kind: "node", Shape: "service", Label: "Work", X: 80, Y: 120, Width: 260, Height: 100, Style: "primary"},
	}
	return SlideTransactionRequest{
		Version: 1, Operation: "create", RequestID: "the-work", Plan: "first", ExpectedSnapshot: "absent",
		Slide:   SlideTransactionSlide{ID: "the-work", Title: "The work", Section: "First phase", Intent: "explain", Layout: "diagram", Takeaway: "The work.", ReadingOrder: []string{"work"}},
		Diagram: &d,
		Items: []SlideTransactionItemRequest{
			{ID: "work", Rank: 10, Kind: "node", Label: "Work", Description: "The work.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "work"}, Record: record},
		},
	}
}

func TestRemovingAPhaseIsRefusedWhileATaskIsInIt(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	run(t, Plan, "add-phase", "--plan", "first", "--id", "one", "--title", "One", root)
	run(t, Task, "add", "--plan", "first", "--id", "work", "--title", "Work", "--phase", "one", "--story", planStoryURN, root)
	if message := refused(t, planCommand, "remove-phase", "--plan", "first", "--id", "one", root); !strings.Contains(message, "still holds task work") {
		t.Fatalf("message = %q", message)
	}
	run(t, Task, "set", "--plan", "first", "--task", "work", "--phase", "", root)
	run(t, Plan, "remove-phase", "--plan", "first", "--id", "one", root)
	if phases := loadedPlan(t, root, "first").Phases; len(phases) != 0 {
		t.Fatalf("phases = %+v", phases)
	}
}

func TestPlanSlidesOweNoCodeAndNameTasks(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	ctx := context.Background()
	taskURN := "urn:change-saga:app:plan:first:task:feature-work"

	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, planSlideRequest("urn:change-saga:app:plan:first:task:nope"), false); err == nil {
		t.Fatal("an Item naming a missing Task was accepted")
	}
	result, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, planSlideRequest(taskURN), false)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	plan := loadedPlan(t, root, "first")
	slide := plan.Slide("the-work")
	if slide == nil || slide.Target != "urn:change-saga:app:plan:first:slide:the-work" || slide.Items[0].Record != taskURN {
		t.Fatalf("slide = %#v", slide)
	}

	// It can be read and edited through its URN, as a review slide can.
	run(t, Diagram, "describe", "--slide", slide.Target, root)
	if output := run(t, Diagram, "check", root); !strings.Contains(output, slide.Target) {
		t.Fatalf("diagram check did not report the Plan's slide:\n%s", output)
	}
	var printed SlideTransactionRequest
	if err := json.Unmarshal([]byte(run(t, ApplySlide, "--plan", "first", "--print-current", slide.Target, root)), &printed); err != nil {
		t.Fatalf("decode print-current: %v", err)
	}
	if printed.Plan != "first" || printed.Deck != "" || printed.ExpectedSnapshot != result.Snapshot {
		t.Fatalf("print-current does not name the Plan: %+v", printed)
	}
	update := planSlideRequest(taskURN)
	update.Operation, update.RequestID, update.ExpectedSnapshot = "update", "the-work-2", result.Snapshot
	update.Slide.Title = "The work, revised"
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, update, false); err != nil {
		t.Fatalf("update: %v", err)
	}

	// A slide naming a Task keeps that Task from being removed.
	if message := refused(t, taskCommand, "remove", "--plan", "first", "--task", "feature-work", root); !strings.Contains(message, "still names task") {
		t.Fatalf("message = %q", message)
	}

	withEvidence := planSlideRequest("")
	withEvidence.Operation, withEvidence.RequestID, withEvidence.ExpectedSnapshot = "create", "with-evidence", "absent"
	withEvidence.Slide.ID = "with-evidence"
	withEvidence.Items[0].Evidence = []saga.CodeFile{{Version: 2}}
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, withEvidence, false); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("a Plan Item with evidence was accepted: %v", err)
	}
}

func TestPlanDeckOverviewIsAuthoredLikeAnyOther(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	if _, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, planSlideRequest(""), false); err != nil {
		t.Fatal(err)
	}
	overview := t.TempDir() + "/overview.json"
	writeFile(t, overview, `{"body":"How we build it: [the work](annotation:work).","annotations":[{"id":"work","label":"The work","slide":"the-work"}]}`)
	run(t, Deck, "overview", "--plan", "first", "--file", overview, root)
	run(t, Deck, "overview", "--plan", "first", "--check", root)
	if plan := loadedPlan(t, root, "first"); plan.Deck.Overview == nil {
		t.Fatal("the overview was not written")
	}
}

func TestPlanDeliverIsRefusedUntilTheWorkIsDone(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	planCommand := func(ctx context.Context, args []string, out *bytes.Buffer) error { return Plan(ctx, args, out) }

	message := refused(t, planCommand, "deliver", "--plan", "first", root)
	if !strings.Contains(message, "task feature-work is planned") || !strings.Contains(message, "no Review is linked") {
		t.Fatalf("message = %q", message)
	}
	run(t, Task, "done", "--plan", "first", "--task", "feature-work", root)
	run(t, Plan, "link-review", "--plan", "first", "--review", "pr-7", root)
	if message := refused(t, planCommand, "deliver", "--plan", "first", root); !strings.Contains(message, "review pr-7 has not merged") {
		t.Fatalf("message = %q", message)
	}
	if err := planstore.Deliver(root, "first", func(*saga.Review) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if !loadedPlan(t, root, "first").Delivered() {
		t.Fatal("the plan was not marked delivered")
	}

	// A delivered Plan is hidden unless asked for, still reads, and refuses edits.
	if listed := run(t, Plan, "list", root); strings.Contains(listed, "First plan") || !strings.Contains(listed, "1 delivered plan hidden; --all shows them.") {
		t.Fatalf("list = %q", listed)
	}
	if listed := run(t, Plan, "list", "--all", root); !strings.Contains(listed, "First plan (delivered)") {
		t.Fatalf("list --all = %q", listed)
	}
	if message := refused(t, taskCommand, "set", "--plan", "first", "--task", "feature-work", "--title", "Changed", root); !strings.Contains(message, "was delivered") {
		t.Fatalf("message = %q", message)
	}
	request := planSlideRequest("")
	if _, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, request, false); err == nil || !strings.Contains(err.Error(), "delivered") {
		t.Fatalf("a delivered Plan accepted a slide: %v", err)
	}
}

func TestPlanGuideNamesWhatIsNotReady(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	if workflow := run(t, Plan, "guide", root); !strings.Contains(workflow, "Research.") || !strings.Contains(workflow, "whose record names its Task") || strings.Contains(workflow, "Plan first:") {
		t.Fatalf("workflow = %q", workflow)
	}
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	report := run(t, Plan, "guide", "--plan", "first", root)
	if !strings.Contains(report, "Not sized yet: feature-work") || !strings.Contains(report, "no slides yet") || strings.Contains(report, "Every task is judged") {
		t.Fatalf("report = %q", report)
	}
	run(t, Task, "set", "--plan", "first", "--task", "feature-work", "--sizing", "over-2-days", root)
	if report := run(t, Plan, "guide", "--plan", "first", root); !strings.Contains(report, "split it: feature-work") {
		t.Fatalf("report = %q", report)
	}
	run(t, Task, "set", "--plan", "first", "--task", "feature-work", "--sizing", "within-2-days", root)
	if _, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, planSlideRequest(""), false); err != nil {
		t.Fatal(err)
	}
	overview := t.TempDir() + "/overview.json"
	writeFile(t, overview, `{"body":"How we build it: [the work](annotation:work).","annotations":[{"id":"work","label":"The work","slide":"the-work"}]}`)
	run(t, Deck, "overview", "--plan", "first", "--file", overview, root)
	if report := run(t, Plan, "guide", "--plan", "first", root); !strings.Contains(report, "Every task is judged two days or less") {
		t.Fatalf("report = %q", report)
	}
}

func TestPlanListWorksOutStoriesAndFeaturesFromTasks(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	listed := run(t, Plan, "list", "--feature", testFeature, root)
	if !strings.Contains(listed, "touches 1 story in "+testFeature) {
		t.Fatalf("list = %q", listed)
	}
	if listed := run(t, Plan, "list", "--story", "urn:change-saga:app:story:other", root); !strings.Contains(listed, "No Plans.") {
		t.Fatalf("list --story = %q", listed)
	}
}

func TestRemovingAStoryReportsTheTasksThatNamedIt(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	preview := run(t, Story, "remove", "--story", planStoryURN, "--dry-run", root)
	if !strings.Contains(preview, "urn:change-saga:app:plan:first:task:feature-work") {
		t.Fatalf("the removal preview does not name the task:\n%s", preview)
	}
	run(t, Story, "remove", "--story", planStoryURN, root)
	var output bytes.Buffer
	err := Check(context.Background(), []string{"--covers", "health", root}, &output)
	if err == nil || !strings.Contains(output.String(), "feature-work") {
		t.Fatalf("health did not report the task: err=%v\n%s", err, output.String())
	}
}

func TestAPlanCrossesFeaturesAndFollowsAMovedStory(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	mustLiving(t, "feature add", featureCommand, "add", root, "--id", "search", "--title", "Search", "--status", "planned", "--augments", testFeature)
	run(t, Story, "add", "--feature", "search", "--id", "second-story", "--revision", "r1", "--event", "proposed",
		"--title", "Second story", "--statement", "As a user I can do the second thing", "--persona", personaURNFor("app"), root)
	run(t, Task, "add", "--plan", "first", "--id", "one", "--title", "One", "--story", planStoryURN, root)
	run(t, Task, "add", "--plan", "first", "--id", "two", "--title", "Two", "--story", "urn:change-saga:app:story:second-story", root)
	if listed := run(t, Plan, "list", root); !strings.Contains(listed, "touches 2 stories in "+testFeature+", search") && !strings.Contains(listed, "touches 2 stories in search, "+testFeature) {
		t.Fatalf("list = %q", listed)
	}

	run(t, Feature, "deliver", "--feature", "search", root)
	if got := loadedPlan(t, root, "first").FindTask("two").Story; got != "urn:change-saga:app:story:second-story" {
		t.Fatalf("the Task lost its story: %q", got)
	}
	if listed := run(t, Plan, "list", "--feature", testFeature, "--json", root); !strings.Contains(listed, `"first"`) {
		t.Fatalf("the receiving feature does not list the Plan: %s", listed)
	}
}

func TestPlanHoldsManyPrototypesAndReviews(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	for _, prototype := range []string{"one", "two"} {
		if err := planstore.AddPrototype(root, "first", "urn:change-saga:app:prototype:"+prototype); err != nil {
			t.Fatal(err)
		}
	}
	run(t, Plan, "link-review", "--plan", "first", "--review", "pr-7", root)
	if err := reviewstore.Create(root, saga.ReviewManifest{ID: "pr-8", Title: "More", Base: "main", Head: "feature/more", PullRequest: &saga.PullRequest{Number: 8, URL: "https://github.com/acme/app/pull/8"}}, saga.DeckManifest{ID: "pr-8", Title: "PR 8", Objective: "Explain the rest."}); err != nil {
		t.Fatal(err)
	}
	run(t, Plan, "link-review", "--plan", "first", "--review", "pr-8", root)
	plan := loadedPlan(t, root, "first")
	if len(plan.Prototypes) != 2 || len(plan.Reviews) != 2 {
		t.Fatalf("plan = %#v", plan.PlanManifest)
	}
}

func TestADeliveredPlanRefusesEveryEdit(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	run(t, Task, "add", "--plan", "first", "--id", "feature-work", "--title", "Feature work", "--story", planStoryURN, root)
	run(t, Task, "done", "--plan", "first", "--task", "feature-work", root)
	run(t, Plan, "link-review", "--plan", "first", "--review", "pr-7", root)
	if err := planstore.Deliver(root, "first", func(*saga.Review) bool { return true }); err != nil {
		t.Fatal(err)
	}
	overview := t.TempDir() + "/overview.json"
	writeFile(t, overview, `{"body":"Rewritten after delivery."}`)
	for name, attempt := range map[string]func() error{
		"add a phase": func() error {
			return Plan(context.Background(), []string{"add-phase", "--plan", "first", "--id", "late", "--title", "Late", root}, &bytes.Buffer{})
		},
		"link a review": func() error {
			return Plan(context.Background(), []string{"link-review", "--plan", "first", "--review", "pr-8", root}, &bytes.Buffer{})
		},
		"add a task": func() error {
			return Task(context.Background(), []string{"add", "--plan", "first", "--id", "late", "--title", "Late", "--story", planStoryURN, root}, &bytes.Buffer{})
		},
		"remove a task": func() error {
			return Task(context.Background(), []string{"remove", "--plan", "first", "--task", "feature-work", root}, &bytes.Buffer{})
		},
		"write the essay": func() error {
			return Deck(context.Background(), []string{"overview", "--plan", "first", "--file", overview, root}, &bytes.Buffer{})
		},
	} {
		if err := attempt(); err == nil || !strings.Contains(err.Error(), "delivered") {
			t.Errorf("%s on a delivered Plan: %v", name, err)
		}
	}
}

func TestPlanAndTaskAreTheOnlyWayToPlanWork(t *testing.T) {
	t.Parallel()
	root := newPlanFixture(t).root
	for _, old := range []string{"add-wave", "revise-wave", "add-item", "revise-item", "add-dependency", "add-contract", "assign", "progress", "record-merge"} {
		if err := Plan(context.Background(), []string{old, root}, &bytes.Buffer{}); err == nil {
			t.Errorf("plan %s still runs", old)
		}
	}
}

// Plans and Tasks read through query like other records, with their essays, so
// an agent does not need the rendered reader.
func TestPlansAndTasksReadThroughQuery(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	story := addStory(t, root, testFeature, "pay").Resource
	run(t, Plan, "create", "--id", "first", "--title", "First plan", root)
	run(t, Plan, "add-phase", "--plan", "first", "--id", "build", "--title", "Build", root)
	essay := filepath.Join(t.TempDir(), "essay.md")
	writeFile(t, essay, "Why this Task exists.\n")
	run(t, Task, "add", "--plan", "first", "--id", "gate", "--title", "Gate", "--story", story, "--phase", "build", "--essay-source", essay, root)
	run(t, Task, "add", "--plan", "first", "--id", "base", "--title", "Base", "--unblocks", "gate", root)
	query := func(args ...string) map[string]any {
		t.Helper()
		var output bytes.Buffer
		if err := Query(context.Background(), args, &output); err != nil {
			t.Fatalf("query %v: %v\n%s", args, err, output.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		decodeOneJSONValue(t, output.Bytes(), &envelope)
		return envelope.Data
	}
	if plans := query("plans", "--saga", root)["plans"].([]any); len(plans) != 1 || plans[0].(map[string]any)["task_count"] != float64(2) {
		t.Fatalf("plans = %#v", plans)
	}
	plan := query("plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)
	if phases := plan["phases"].([]any); len(phases) != 1 || phases[0].(map[string]any)["tasks"].([]any)[0] != "gate" {
		t.Fatalf("plan phases = %#v", plan["phases"])
	}
	if tasks := plan["tasks"].([]any); len(tasks) != 2 {
		t.Fatalf("plan tasks = %#v", tasks)
	}
	for _, args := range [][]string{{"--task", "gate"}, {"--task", "gate", "--plan", "first"}, {"--task", saga.PlanTarget("atomic", "first") + ":task:gate"}} {
		task := query(append([]string{"task", "--saga", root}, args...)...)["task"].(map[string]any)
		if task["essay"] != "Why this Task exists.\n" || task["story"] != story || task["depends_on"].([]any)[0] != "base" {
			t.Fatalf("task %v = %#v", args, task)
		}
	}
	var missing bytes.Buffer
	if err := Query(context.Background(), []string{"task", "--saga", root, "--task", "nope"}, &missing); err == nil && !strings.Contains(missing.String(), "not_found") {
		t.Fatalf("a missing task = %s", missing.String())
	}
}
