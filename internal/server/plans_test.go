package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

func addServerPlan(t *testing.T, root, id string) {
	t.Helper()
	if err := planstore.Create(root, saga.PlanManifest{ID: id, Title: "Plan " + id}, saga.DeckManifest{ID: id, Title: "Plan " + id, Objective: "Explain the work."}); err != nil {
		t.Fatal(err)
	}
}

func addServerTask(t *testing.T, root, plan, id, story string, unblocks ...string) {
	t.Helper()
	if _, err := planstore.AddTask(root, plan, saga.TaskManifest{ID: id, Title: "Task " + id, Story: story}, unblocks, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPlansHaveTheirOwnSide(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans").Body.String()
	for _, want := range []string{
		`data-plan-index`,
		`data-directory-row="first"`,
		`class="side-tab current" href="/plans" data-side="plans" aria-current="page"`,
		`data-side="documentation"`,
		`data-side="review"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Plans page has no %q", want)
		}
	}
}

// A Plan's essay and slides are in the viewer a review's deck uses. Nothing in a
// Plan can be deprecated, so the viewer's request for that must not fail.
func TestAPlanPageHoldsItsEssayInTheDeckViewer(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	overview := &saga.DeckOverview{Body: "Why this work is shaped this way."}
	if err := planstore.Create(fixture.root, saga.PlanManifest{ID: "first", Title: "Plan first"}, saga.DeckManifest{ID: "first", Title: "Plan first", Objective: "Explain the work.", Overview: overview}); err != nil {
		t.Fatal(err)
	}
	if _, validation, err := saga.Load(fixture.root); err != nil || !validation.Valid {
		t.Fatalf("fixture: %v %#v", err, validation.Issues)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first").Body.String()
	if !strings.Contains(page, `data-deck-overview=`) || !strings.Contains(page, "Why this work is shaped this way.") {
		t.Errorf("the Plan's essay is not in the viewer:\n%s", page)
	}
	target := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":plan:first:deck:first"
	if panel := getPage(t, handler, "/api/deprecations?compact=1&target="+url.QueryEscape(target)); panel.Code != http.StatusOK {
		t.Errorf("deprecations of a Plan's deck = %d, want 200", panel.Code)
	}
}

func TestAPlanHasDeckAndTasksViews(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	for _, path := range []string{"/plans/first", "/plans/first/tasks"} {
		page := getPage(t, handler, path).Body.String()
		for _, want := range []string{`href="/plans/first"`, `href="/plans/first/tasks"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s has no header link %s", path, want)
			}
		}
		if !strings.Contains(page, `href="`+path+`" aria-current="page"`) {
			t.Errorf("%s does not mark its own header link current", path)
		}
	}
}

func TestPlanTasksEndAtStoriesAndSayWhatEachTakes(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":story:missing"
	addServerTask(t, fixture.root, "first", "feature-work", story)
	addServerTask(t, fixture.root, "first", "refactor", "", "feature-work")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first/tasks").Body.String()
	for _, want := range []string{
		`data-whiteboard-view`,
		`data-plan-node="task:refactor"`,
		`data-plan-node="story:` + story + `"`,
		`data-plan-edge-from="task:refactor" data-plan-edge-to="task:feature-work"`,
		`data-plan-edge-from="task:feature-work" data-plan-edge-to="story:` + story + `"`,
		`0 of 2 tasks done`,
		`id="plan-node-task-refactor"`,
		`It takes these 2 tasks, 0 done.`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Tasks view has no %q", want)
		}
	}
}

func TestATaskPageDrawsOnlyItsChain(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":story:missing"
	addServerTask(t, fixture.root, "first", "feature-work", story)
	addServerTask(t, fixture.root, "first", "refactor", "", "feature-work")
	addServerTask(t, fixture.root, "first", "elsewhere", story)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first/tasks/refactor").Body.String()
	if !strings.Contains(page, `data-plan-node="story:`+story+`"`) {
		t.Error("the chain does not reach its story")
	}
	if strings.Contains(page, `data-plan-node="task:elsewhere"`) {
		t.Error("a Task outside the chain was drawn")
	}
}

func TestAPlanPageReadsAsAPlanAndShowsItsTasks(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":story:missing"
	addServerTask(t, fixture.root, "first", "feature-work", story)
	addServerTask(t, fixture.root, "first", "refactor", "", "feature-work")
	if err := planstore.UpdateTask(fixture.root, "first", "refactor", func(task *saga.TaskManifest) error {
		task.State = saga.TaskDone
		task.Sizing = &saga.TaskSizing{WithinTwoDays: true}
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	deck := getPage(t, handler, "/plans/first").Body.String()
	for _, want := range []string{"Plan, not a review", "1 of 2 tasks done"} {
		if !strings.Contains(deck, want) {
			t.Errorf("the deck view has no %q", want)
		}
	}
	tasks := getPage(t, handler, "/plans/first/tasks").Body.String()
	for _, want := range []string{`data-plan-unsized`, "Not sized yet", `data-plan-ready`, "1 task can start now", `data-plan-chain`, "Leads to", "through "} {
		if !strings.Contains(tasks, want) {
			t.Errorf("the tasks view has no %q", want)
		}
	}
	task := getPage(t, handler, "/plans/first/tasks/refactor").Body.String()
	for _, want := range []string{
		`data-task-chain`, "No story of its own", `data-task-unblocks`, "Done",
		`class="plan-board plan-selecting"`,
		`plan-selected plan-path" data-plan-node="task:refactor"`,
		`plan-onward" data-plan-node="task:feature-work"`,
	} {
		if !strings.Contains(task, want) {
			t.Errorf("the task page has no %q", want)
		}
	}
}

func TestADeliveredPlanIsSetAsideLikeAMergedReview(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "open-plan")
	addServerPlan(t, fixture.root, "shipped")
	deliverServerPlan(t, fixture.root, "shipped")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	index := getPage(t, handler, "/plans").Body.String()
	for _, want := range []string{
		`<tr data-directory-row="open-plan"`,
		`<tr hidden data-directory-row="shipped" data-directory-archived-row`,
		`Show 1 delivered`,
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the Plans page has no %q", want)
		}
	}
	if shown := getPage(t, handler, "/plans?archived=show").Body.String(); strings.Contains(shown, `<tr hidden data-directory-row="shipped"`) {
		t.Error("the filter did not reveal the delivered Plan")
	}
	if page := getPage(t, handler, "/plans/shipped").Body.String(); !strings.Contains(page, "data-plan-delivered") {
		t.Error("a delivered Plan does not say so on its page")
	}
}

func TestPlanComparisonReadsWhatTheReviewsNamed(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	addServerTask(t, fixture.root, "first", "feature-work", "urn:change-saga:"+serverSagaID(t, fixture.root)+":story:missing")
	if err := planstore.LinkReview(fixture.root, "first", "pr-7"); err != nil {
		t.Fatal(err)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first/tasks").Body.String()
	for _, want := range []string{`data-plan-comparison-row="feature-work"`, "Not delivered", `data-plan-unplanned`} {
		if !strings.Contains(page, want) {
			t.Errorf("the comparison has no %q", want)
		}
	}
	if review := getPage(t, handler, "/reviews/pr-7").Body.String(); !strings.Contains(review, "data-review-plan") {
		t.Error("the review does not link back to its Plan")
	}
}

func serverSagaID(t *testing.T, root string) string {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	return manifest.ID
}

func deliverServerPlan(t *testing.T, root, id string) {
	t.Helper()
	addServerTask(t, root, id, "only", "urn:change-saga:"+serverSagaID(t, root)+":story:missing")
	if err := planstore.UpdateTask(root, id, "only", func(task *saga.TaskManifest) error { task.State = saga.TaskDone; return nil }, nil); err != nil {
		t.Fatal(err)
	}
	if err := planstore.LinkReview(root, id, "pr-7"); err != nil {
		t.Fatal(err)
	}
	if err := planstore.Deliver(root, id, func(*saga.Review) bool { return true }); err != nil {
		t.Fatal(err)
	}
}

// pinTaskCode gives a Task a reference to the fixture's real code, and a second
// whose commit does not exist.
func pinTaskCode(t *testing.T, fixture serverReviewFixture, plan, task string) {
	t.Helper()
	resolver, err := coderesolve.New(context.Background(), fixture.repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(context.Background(), coderef.Location{Commit: fixture.head, Path: "queue.go", Start: 3, End: 3}, "Where the queue is chosen")
	if err != nil {
		t.Fatal(err)
	}
	lost := reference
	lost.Commit, lost.Note = strings.Repeat("a", 40), "A commit that was never fetched"
	if err := planstore.UpdateTask(fixture.root, plan, task, func(manifest *saga.TaskManifest) error {
		manifest.Code = []coderef.Reference{reference, lost}
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestATaskReferenceOpensItsPinnedLinesOrSaysWhyItCannot(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	addServerTask(t, fixture.root, "first", "work", "urn:change-saga:app:story:missing")
	pinTaskCode(t, fixture, "first", "work")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	href := "/api/task-code?plan=first&amp;reference=0&amp;task=work"
	for _, path := range []string{"/plans/first/tasks/work", "/plans/first/tasks"} {
		page := getPage(t, handler, path).Body.String()
		if !strings.Contains(page, `data-target-code-href="`+href+`"`) || !strings.Contains(page, "Where the queue is chosen") {
			t.Errorf("%s does not offer the reference as a control", path)
		}
	}

	code := getPage(t, handler, "/api/task-code?plan=first&task=work&reference=0").Body.String()
	for _, want := range []string{`data-target-code-response`, `data-open-diffs="diffs-`, `data-attached-title="Task work"`, "Where the queue is chosen", `postgres`, `class="referenced"`, fixture.head[:12]} {
		if !strings.Contains(code, want) {
			t.Errorf("the code drawer has no %q:\n%s", want, code)
		}
	}

	lost := getPage(t, handler, "/api/task-code?plan=first&task=work&reference=1").Body.String()
	for _, want := range []string{"A commit that was never fetched", "could not be read from this checkout", `data-evidence-integrity="unavailable"`} {
		if !strings.Contains(lost, want) {
			t.Errorf("an unreadable reference does not say %q:\n%s", want, lost)
		}
	}
	if strings.Contains(lost, `class="referenced"`) {
		t.Errorf("an unreadable reference still shows lines:\n%s", lost)
	}

	for _, path := range []string{
		"/api/task-code?plan=gone&task=work&reference=0",
		"/api/task-code?plan=first&task=gone&reference=0",
		"/api/task-code?plan=first&task=work&reference=2",
		"/api/task-code?plan=first&task=work&reference=x",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, recorder.Code)
		}
	}
}

func TestAPlanSlidesNodesOpenTheirTaskOrStory(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:app:story:reached"
	unreached := "urn:change-saga:app:story:unreached"
	addServerTask(t, fixture.root, "first", "work", story)
	if err := planstore.UpdateTask(fixture.root, "first", "work", func(task *saga.TaskManifest) error {
		task.State = saga.TaskDone
		return nil
	}, &[]string{"The **essay** of the work."}[0]); err != nil {
		t.Fatal(err)
	}
	pinTaskCode(t, fixture, "first", "work")

	deckDir := filepath.Join(saga.PlanDir(fixture.root, "first"), saga.PlanDeckDir)
	deck, slide := saga.PlanDeckTarget("app", "first", "first"), saga.PlanSlideTarget("app", "first", "nodes")
	slideName, _ := saga.FlatSlideFilename(deck, slide, 0)
	asset, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeServerJSON(t, filepath.Join(deckDir, slideName), saga.SlideManifest{Version: saga.DeckRecordVersion, ID: "nodes", DeckID: "first", Title: "Nodes", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Entrypoint: asset, Takeaway: "Each node is work.", ReadingOrder: []string{"task-node", "story-node", "region", "surprise"}})
	writeServerFile(t, filepath.Join(deckDir, asset), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect id="task-node" width="5" height="5"/><rect id="story-node" x="5" width="5" height="5"/></svg>`)
	addItem := func(id, kind, body, record string, selector saga.LandmarkSelector) {
		name, _ := saga.FlatItemFilename(slide, saga.PlanItemTarget("app", "first", "nodes", id), 0)
		writeServerJSON(t, filepath.Join(deckDir, name), saga.ItemManifest{Version: saga.DeckRecordVersion, ID: id, SlideID: "nodes", Kind: kind, Label: id, Description: id + ".", Body: body, Selector: selector, Record: record})
	}
	task := saga.TaskTarget("app", "first", "work")
	addItem("task-node", "node", "", task, saga.LandmarkSelector{Type: "element", ElementID: "task-node"})
	addItem("story-node", "node", "", unreached, saga.LandmarkSelector{Type: "element", ElementID: "story-node"})
	addItem("region", "node", "", story, saga.LandmarkSelector{Type: "region", X: .1, Y: .1, Width: .2, Height: .2})
	addItem("surprise", "callout", "It went another way.", task, saga.LandmarkSelector{Type: "element", ElementID: "task-node"})
	if _, validation, err := saga.Load(fixture.root); err != nil || !validation.Valid {
		t.Fatalf("fixture: %v %#v", err, validation.Issues)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first").Body.String()
	for _, want := range []string{
		// The Task's node, and its done state on the node.
		`data-plan-item="task-node" data-landmark-target`, `data-element-id="task-node" data-plan-task-state="done"`,
		`data-plan-drawer="plan-node-task-work" data-plan-drawer-title="Task"`, `Open task: Task work. Done`,
		// The Task's slide-out on this page, with what a reader asks of it.
		`id="plan-node-task-work"`, `data-plan-task-essay`, `<strong>essay</strong>`, `data-target-code-href="/api/task-code?plan=first&amp;reference=0&amp;task=work"`, "Open this task",
		// A story opens the board's way, reached by a Task or not.
		`data-plan-item="story-node"`, `id="` + planStoryDrawer(story) + `"`, `id="` + planStoryDrawer(unreached) + `"`, "No Task of this Plan leads to it yet.", "It takes these 1 task, 1 done.",
		// A region draws its hotspot on the server.
		`class="landmark-hotspot" data-landmark-visual="` + domID(saga.PlanItemTarget("app", "first", "nodes", "region")) + `"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Plan page has no %q", want)
		}
	}
	if strings.Contains(page, `data-plan-item="surprise"`) {
		t.Error("a callout keeps its place in the list instead of taking a node's hotspot")
	}
}
