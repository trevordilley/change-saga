package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

// livePlanFixture is a Plan with two Tasks, the second unblocking the first.
func livePlanFixture(t *testing.T) (*app, http.Handler, serverReviewFixture) {
	t.Helper()
	fixture := newServerReviewFixture(t)
	addServerPlan(t, fixture.root, "first")
	story := "urn:change-saga:" + serverSagaID(t, fixture.root) + ":story:missing"
	addServerTask(t, fixture.root, "first", "work", story)
	addServerTask(t, fixture.root, "first", "prepare", "", "work")
	application, handler := reviewApp(t, fixture, gitdiff.Range{})
	return application, handler, fixture
}

func reportLive(t *testing.T, fixture serverReviewFixture, task, agent, kind, message string, at time.Time) {
	t.Helper()
	log, reason := planactivity.Open(context.Background(), fixture.root, "", serverSagaID(t, fixture.root))
	if log == nil {
		t.Fatalf("no log: %s", reason)
	}
	if err := log.Append("first", planactivity.Event{At: at, Agent: agent, Task: task, Kind: kind, Message: message, Branch: agent + "-branch"}); err != nil {
		t.Fatal(err)
	}
}

func liveAsk(t *testing.T, handler http.Handler, since string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plans/first/activity?since="+url.QueryEscape(since), nil))
	return recorder
}

func liveDelta(t *testing.T, handler http.Handler, since string) planLiveResponse {
	t.Helper()
	recorder := liveAsk(t, handler, since)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var state planLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
}

// liveState is everything, as a page with no cursor is told.
func liveState(t *testing.T, handler http.Handler) planLiveResponse {
	t.Helper()
	return liveDelta(t, handler, "")
}

func TestLiveStatusReadsEachState(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	event := func(age time.Duration, agent, kind string) planactivity.Event {
		return planactivity.Event{At: now.Add(-age), Agent: agent, Task: "t", Kind: kind, Message: "said", Branch: "feat/x"}
	}
	for _, test := range []struct {
		name, state string
		canStart    bool
		held        string
		events      []planactivity.Event
		class, word string
		who         string
	}{
		{"nobody reported", saga.TaskPlanned, false, "", nil, "waiting", "Waiting", ""},
		{"could start", saga.TaskPlanned, true, "", nil, "ready", "Can start now", ""},
		{"held by a checkpoint", saga.TaskPlanned, false, "Proven", nil, "waiting", "Held by checkpoint Proven", ""},
		{"held but started anyway", saga.TaskPlanned, false, "Proven", []planactivity.Event{event(time.Minute, "ada", planactivity.KindStart)}, "working", "In progress", "ada"},
		{"durable in progress", saga.TaskInProgress, false, "", nil, "working", "In progress", ""},
		{"started", saga.TaskPlanned, true, "", []planactivity.Event{event(time.Minute, "ada", planactivity.KindStart)}, "working", "In progress", "ada"},
		{"blocked", saga.TaskPlanned, true, "", []planactivity.Event{event(time.Minute, "ada", planactivity.KindBlocked)}, "blocked", "Blocked", "ada"},
		{"quiet is not failed", saga.TaskPlanned, true, "", []planactivity.Event{event(2*time.Hour, "ada", planactivity.KindStart)}, "working quiet", "In progress, quiet", "ada"},
		{"done on a branch", saga.TaskPlanned, true, "", []planactivity.Event{event(time.Minute, "ada", planactivity.KindDone)}, "done-branch", "Done on feat/x", ""},
		{"durably done wins", saga.TaskDone, false, "", []planactivity.Event{event(time.Minute, "ada", planactivity.KindBlocked)}, "done", "Done", ""},
		{"many agents", saga.TaskPlanned, false, "", []planactivity.Event{event(3*time.Minute, "a", planactivity.KindStart), event(2*time.Minute, "b", planactivity.KindStart), event(time.Minute, "c", planactivity.KindStart)}, "working", "In progress", "c, b, +1"},
	} {
		status := liveStatus("t", test.state, test.canStart, test.held, planactivity.Summarize(test.events)["t"], now)
		if status.Class != test.class || status.Word != test.word || status.Who != test.who {
			t.Errorf("%s: %+v", test.name, status)
		}
		if strings.Contains(strings.ToLower(status.Word+status.Class), "fail") {
			t.Errorf("%s: never read as failed: %+v", test.name, status)
		}
	}
}

func TestAPlansPagesRenderActivityFirstTime(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	now := time.Now().UTC()
	reportLive(t, fixture, "prepare", "ada", planactivity.KindStart, "", now.Add(-time.Minute))
	reportLive(t, fixture, "work", "ben", planactivity.KindBlocked, "no <tide> data", now)

	board := getPage(t, handler, "/plans/first/tasks").Body.String()
	for _, want := range []string{
		`data-plan-live-url="/api/plans/first/activity"`,
		`class="plan-board-node task working" data-plan-node="task:prepare"`,
		`In progress · ada`,
		`class="plan-board-node task blocked" data-plan-node="task:work"`,
		`data-plan-task-history="work"`,
		`On it: ben. no &lt;tide&gt; data`,
		`data-plan-feed`,
		`<strong>ben</strong> blocked on <button type="button" class="plan-activity-task" data-plan-drawer="plan-node-task-work"`,
	} {
		if !strings.Contains(board, want) {
			t.Errorf("the board has no %q", want)
		}
	}
	if strings.Contains(board, "no <tide>") {
		t.Error("an agent's message must be escaped")
	}
	deck := getPage(t, handler, "/plans/first").Body.String()
	if !strings.Contains(deck, `data-plan-live-url="/api/plans/first/activity"`) || !strings.Contains(deck, `data-plan-feed`) {
		t.Error("the deck page must carry the feed and poll")
	}
	task := getPage(t, handler, "/plans/first/tasks/work").Body.String()
	for _, want := range []string{`data-plan-live-url`, `data-plan-task-status="work"`, `data-plan-task-history="work"`, "no &lt;tide&gt; data"} {
		if !strings.Contains(task, want) {
			t.Errorf("the Task page has no %q", want)
		}
	}
}

func TestTheActivityEndpointReadsNoSagaAndSays304WhenNothingChanged(t *testing.T) {
	t.Parallel()
	application, handler, fixture := livePlanFixture(t)
	reportLive(t, fixture, "prepare", "ada", planactivity.KindStart, "", time.Now().UTC())
	application.fresh.mutex.Lock()
	checks, asked := application.fresh.checks, application.fresh.asked
	application.fresh.mutex.Unlock()

	state := liveState(t, handler)
	if !state.Available || state.All != 2 || state.Tasks["prepare"].Class != "working" || state.Tasks["prepare"].Who != "ada" || state.Feed == nil || !strings.Contains(*state.Feed, "ada") {
		t.Fatalf("state = %+v", state)
	}
	if state.Tasks["work"].Class != "waiting" || state.Tasks["work"].History != "" {
		t.Fatalf("a Task nobody reported on reads as its record: %+v", state.Tasks["work"])
	}

	if second := liveAsk(t, handler, state.Cursor); second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("status %d, %d bytes", second.Code, second.Body.Len())
	}
	application.fresh.mutex.Lock()
	defer application.fresh.mutex.Unlock()
	if application.fresh.checks != checks || !application.fresh.asked.Equal(asked) {
		t.Fatal("polling must neither check the Saga nor keep its watcher awake")
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/plans/nope/activity", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("a missing Plan: %d", missing.Code)
	}
}

func TestTheEndpointSeesARecordChangeAndNewActivity(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	if state := liveState(t, handler); state.Done != 0 || state.Tasks["prepare"].Class != "ready" || state.Ready == nil || !strings.Contains(*state.Ready, "Task prepare") {
		t.Fatalf("state = %+v", state)
	}
	if page := getPage(t, handler, "/plans/first/tasks").Body.String(); !strings.Contains(page, "1 task can start now") {
		t.Error("the board must name what can start")
	}
	// A Task someone is on is not free to pick up, on the first render or after.
	reportLive(t, fixture, "prepare", "ben", planactivity.KindStart, "", time.Now().UTC())
	if state := liveState(t, handler); state.Ready == nil || *state.Ready != "" {
		t.Fatalf("a held Task still reads as free: %v", state.Ready)
	}
	if page := getPage(t, handler, "/plans/first/tasks").Body.String(); strings.Contains(page, "can start now") {
		t.Error("a held Task must not read as able to start")
	}
	reportLive(t, fixture, "prepare", "ada", planactivity.KindDone, "finished", time.Now().UTC())
	if state := liveState(t, handler); state.Done != 0 || state.Tasks["prepare"].Class != "done-branch" || state.Tasks["prepare"].Word != "Done on ada-branch" {
		t.Fatalf("done elsewhere is not durable: %+v", state.Tasks["prepare"])
	}
	// The branch merges: the record says done, which now unblocks what waited on it.
	if err := planstore.UpdateTask(fixture.root, "first", "prepare", func(task *saga.TaskManifest) error {
		task.State = saga.TaskDone
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	state := liveState(t, handler)
	if state.Done != 1 || state.Tasks["prepare"].Class != "done" || state.Tasks["work"].Class != "ready" {
		t.Fatalf("state = %+v", state)
	}
}

func TestASagaOutsideGitSaysItHasNoLiveActivity(t *testing.T) {
	t.Parallel()
	application, handler, fixture := livePlanFixture(t)
	outside := filepath.Join(t.TempDir(), filepath.Base(fixture.root))
	if err := os.CopyFS(outside, os.DirFS(fixture.root)); err != nil {
		t.Fatal(err)
	}
	application.root, application.sourceDir = outside, outside
	state := liveState(t, handler)
	if state.Available || state.Reason != planactivity.NotInGit {
		t.Fatalf("state = %+v", state)
	}
	if page := getPage(t, handler, "/plans/first/tasks").Body.String(); !strings.Contains(page, "data-plan-live-reason") || strings.Contains(page, "data-plan-live-url") {
		t.Error("the page must say so, and not poll")
	}
}

func TestADeliveredPlanStopsPolling(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	for _, task := range []string{"prepare", "work"} {
		if err := planstore.UpdateTask(fixture.root, "first", task, func(value *saga.TaskManifest) error {
			value.State = saga.TaskDone
			return nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	deliverServerPlan(t, fixture.root, "first")
	if page := getPage(t, handler, "/plans/first/tasks").Body.String(); strings.Contains(page, "data-plan-live-url") {
		t.Error("a delivered Plan no longer changes, so nothing polls it")
	}
}

// However large the Plan, a poll carries only what changed since the page last asked.
func TestAPollCarriesOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	first := liveState(t, handler)
	if len(first.Tasks) != 2 || first.Feed == nil || first.Ready == nil {
		t.Fatalf("a page with no cursor gets everything: %+v", first)
	}
	reportLive(t, fixture, "work", "ada", planactivity.KindNote, "a finding", time.Now().UTC())
	delta := liveDelta(t, handler, first.Cursor)
	if len(delta.Tasks) != 1 || !strings.Contains(delta.Tasks["work"].History, "a finding") || delta.Feed == nil {
		t.Fatalf("delta = %+v", delta)
	}
	if delta.Ready != nil {
		t.Fatal("nothing about what can start changed")
	}
	if delta.Done != 0 || delta.All != 2 {
		t.Fatalf("the count always comes: %+v", delta)
	}
	if again := liveAsk(t, handler, delta.Cursor); again.Code != http.StatusNotModified {
		t.Fatalf("status %d", again.Code)
	}
	// An older cursor, as a page restored from history has, catches up on all of it.
	reportLive(t, fixture, "prepare", "ben", planactivity.KindStart, "", time.Now().UTC())
	if behind := liveDelta(t, handler, first.Cursor); len(behind.Tasks) != 2 {
		t.Fatalf("behind = %+v", behind.Tasks)
	}
	// A cursor from another run of the server means nothing.
	if stranger := liveDelta(t, handler, "elsewhere.1"); len(stranger.Tasks) != 2 || stranger.Feed == nil {
		t.Fatalf("stranger = %+v", stranger)
	}
}

// Nothing on disk changes when a Task goes quiet, so the clock has to say so.
func TestATaskGoingQuietReachesAPageWithNothingElseChanging(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	reportLive(t, fixture, "work", "ada", planactivity.KindStart, "", time.Now().UTC().Add(-planactivity.QuietAfter+time.Second))
	first := liveState(t, handler)
	if first.Tasks["work"].Class != "working" {
		t.Fatalf("task = %+v", first.Tasks["work"])
	}
	time.Sleep(1100 * time.Millisecond)
	if delta := liveDelta(t, handler, first.Cursor); delta.Tasks["work"].Class != "working quiet" {
		t.Fatalf("delta = %+v", delta.Tasks)
	}
}

func TestAPagesCursorMeansItsFirstPollIsSmall(t *testing.T) {
	t.Parallel()
	_, handler, _ := livePlanFixture(t)
	page := getPage(t, handler, "/plans/first/tasks").Body.String()
	_, rest, found := strings.Cut(page, `data-plan-live-cursor="`)
	if !found {
		t.Fatal("the page carries no cursor")
	}
	cursor, _, _ := strings.Cut(rest, `"`)
	if asked := liveAsk(t, handler, cursor); asked.Code != http.StatusNotModified {
		t.Fatalf("a page that is current must hear 304, got %d", asked.Code)
	}
}

func TestTheBoardAndAPollSayWhatACheckpointHolds(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	for _, phase := range []saga.PlanPhase{{ID: "early", Title: "Early"}, {ID: "proven", Title: "Proven", Checkpoint: &saga.PlanCheckpoint{}}, {ID: "late", Title: "Late"}} {
		if err := planstore.AddPhase(fixture.root, "first", phase, ""); err != nil {
			t.Fatal(err)
		}
	}
	for task, phase := range map[string]string{"prepare": "early", "work": "late"} {
		if err := planstore.UpdateTask(fixture.root, "first", task, func(value *saga.TaskManifest) error {
			value.Phase = phase
			if task == "prepare" {
				value.State = saga.TaskDone
			}
			return nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if state := liveState(t, handler); state.Tasks["work"].Word != "Held by checkpoint Proven" || state.Ready == nil || *state.Ready != "" {
		t.Fatalf("work = %+v, ready = %v", state.Tasks["work"], state.Ready)
	}
	if page := getPage(t, handler, "/plans/first/tasks").Body.String(); !strings.Contains(page, "Held by checkpoint Proven") || strings.Contains(page, "can start now") {
		t.Error("the board must say the checkpoint holds the Task, and offer nothing to start")
	}
}

// A Plan with checkpoints opens folded to its current stretch: a passed stretch
// is one node, the current one is drawn in full, and ?fold=0 shows every Task.
func TestTheBoardFoldsTheStretchesOtherThanTheCurrentOne(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	for _, phase := range []saga.PlanPhase{{ID: "early", Title: "Early"}, {ID: "proven", Title: "Proven", Checkpoint: &saga.PlanCheckpoint{}}, {ID: "late", Title: "Late"}, {ID: "shipped", Title: "Shipped", Checkpoint: &saga.PlanCheckpoint{}}} {
		if err := planstore.AddPhase(fixture.root, "first", phase, ""); err != nil {
			t.Fatal(err)
		}
	}
	for task, phase := range map[string]string{"prepare": "early", "work": "late"} {
		if err := planstore.UpdateTask(fixture.root, "first", task, func(value *saga.TaskManifest) error {
			value.Phase = phase
			if task == "prepare" {
				value.State = saga.TaskDone
			}
			return nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := planstore.PassCheckpoint(fixture.root, "first", "proven", saga.CheckpointPass{At: time.Now(), By: "trevor", Reason: "works"}); err != nil {
		t.Fatal(err)
	}
	folded := getPage(t, handler, "/plans/first/tasks").Body.String()
	for _, want := range []string{`data-plan-node="stretch:proven"`, `1 tasks, 1 done`, `data-plan-node="task:work"`, `data-plan-node="checkpoint:shipped"`, `data-plan-edge-from="stretch:proven" data-plan-edge-to="task:work"`, `href="?fold=0"`} {
		if !strings.Contains(folded, want) {
			t.Errorf("the folded board has no %q", want)
		}
	}
	if strings.Contains(folded, `data-plan-node="task:prepare"`) {
		t.Error("a passed stretch must fold into one node")
	}
	every := getPage(t, handler, "/plans/first/tasks?fold=0").Body.String()
	for _, want := range []string{`data-plan-node="task:prepare"`, `class="plan-board-node checkpoint passed" data-plan-node="checkpoint:proven"`, `href="?fold=1"`} {
		if !strings.Contains(every, want) {
			t.Errorf("the whole board has no %q", want)
		}
	}
}

// A review that covers a checkpoint shows the checkpoint: the Plan before, in, and
// after it, its stories and their proof, its size, and its pass.
func TestACheckpointsReviewShowsTheCheckpoint(t *testing.T) {
	t.Parallel()
	_, handler, fixture := livePlanFixture(t)
	for _, phase := range []saga.PlanPhase{{ID: "early", Title: "Early"}, {ID: "proven", Title: "Proven", Checkpoint: &saga.PlanCheckpoint{}}, {ID: "late", Title: "Late"}} {
		if err := planstore.AddPhase(fixture.root, "first", phase, ""); err != nil {
			t.Fatal(err)
		}
	}
	for task, phase := range map[string]string{"prepare": "early", "work": "late"} {
		if err := planstore.UpdateTask(fixture.root, "first", task, func(value *saga.TaskManifest) error {
			value.Phase = phase
			return nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := planstore.LinkCheckpointReview(fixture.root, "first", "pr-7", "proven"); err != nil {
		t.Fatal(err)
	}
	page := getPage(t, handler, "/reviews/pr-7").Body.String()
	for _, want := range []string{
		`data-review-checkpoint`, `Checkpoint <strong>Proven</strong>`, `in progress`,
		`<template id="review-checkpoint">`, `class="checkpoint-cell now"`, `class="checkpoint-cell later"`,
		`Finish its Tasks: prepare.`, `About 1 slide estimated`, `data-checkpoint-story`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the checkpoint's review has no %q", want)
		}
	}
	if err := planstore.UpdateTask(fixture.root, "first", "prepare", func(value *saga.TaskManifest) error {
		value.State = saga.TaskDone
		return nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := planstore.PassCheckpoint(fixture.root, "first", "proven", saga.CheckpointPass{At: time.Now(), By: "trevor", Reason: "the tests pass"}); err != nil {
		t.Fatal(err)
	}
	if page := getPage(t, handler, "/reviews/pr-7").Body.String(); !strings.Contains(page, "Passed by trevor on") || !strings.Contains(page, "the tests pass") {
		t.Error("a passed checkpoint's review must say who passed it and why")
	}
}
