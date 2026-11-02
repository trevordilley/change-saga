package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

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

func TestAPlanHasDeckTasksAndCoverageViews(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	for _, path := range []string{"/plans/first", "/plans/first/tasks", "/plans/first/coverage"} {
		page := getPage(t, handler, path).Body.String()
		for _, want := range []string{`href="/plans/first"`, `href="/plans/first/tasks"`, `href="/plans/first/coverage"`} {
			if !strings.Contains(page, want) {
				t.Errorf("%s has no header link %s", path, want)
			}
		}
		if !strings.Contains(page, `href="`+path+`" aria-current="page"`) {
			t.Errorf("%s does not mark its own header link current", path)
		}
	}
}

func TestPlanCoverageEndsAtStoriesAndSaysWhatEachTakes(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":story:missing"
	addServerTask(t, fixture.root, "first", "feature-work", story)
	addServerTask(t, fixture.root, "first", "refactor", "", "feature-work")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/plans/first/coverage").Body.String()
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
			t.Errorf("the Coverage view has no %q", want)
		}
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
	for _, want := range []string{`data-task-chain`, "No story of its own", `data-task-unblocks`, "Done"} {
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
