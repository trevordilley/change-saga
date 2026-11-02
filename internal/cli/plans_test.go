package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/planstore"
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
