package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func liveState(t *testing.T, handler http.Handler) planLiveResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/plans/first/activity", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	var state planLiveResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	return state
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
		events      []planactivity.Event
		class, word string
		who         string
	}{
		{"nobody reported", saga.TaskPlanned, false, nil, "waiting", "Waiting", ""},
		{"could start", saga.TaskPlanned, true, nil, "ready", "Can start now", ""},
		{"durable in progress", saga.TaskInProgress, false, nil, "working", "In progress", ""},
		{"started", saga.TaskPlanned, true, []planactivity.Event{event(time.Minute, "ada", planactivity.KindStart)}, "working", "In progress", "ada"},
		{"blocked", saga.TaskPlanned, true, []planactivity.Event{event(time.Minute, "ada", planactivity.KindBlocked)}, "blocked", "Blocked", "ada"},
		{"quiet is not failed", saga.TaskPlanned, true, []planactivity.Event{event(2*time.Hour, "ada", planactivity.KindStart)}, "working quiet", "In progress, quiet", "ada"},
		{"done on a branch", saga.TaskPlanned, true, []planactivity.Event{event(time.Minute, "ada", planactivity.KindDone)}, "done-branch", "Done on feat/x", ""},
		{"durably done wins", saga.TaskDone, false, []planactivity.Event{event(time.Minute, "ada", planactivity.KindBlocked)}, "done", "Done", ""},
		{"many agents", saga.TaskPlanned, false, []planactivity.Event{event(3*time.Minute, "a", planactivity.KindStart), event(2*time.Minute, "b", planactivity.KindStart), event(time.Minute, "c", planactivity.KindStart)}, "working", "In progress", "c, b, +1"},
	} {
		status := liveStatus("t", test.state, test.canStart, planactivity.Summarize(test.events)["t"], now)
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
	if !state.Available || state.All != 2 || state.Tasks["prepare"].Class != "working" || state.Tasks["prepare"].Who != "ada" || !strings.Contains(state.Feed, "ada") {
		t.Fatalf("state = %+v", state)
	}
	if state.Tasks["work"].Class != "waiting" || state.Tasks["work"].History != "" {
		t.Fatalf("a Task nobody reported on reads as its record: %+v", state.Tasks["work"])
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/plans/first/activity", nil))
	etag := first.Header().Get("ETag")
	again := httptest.NewRequest(http.MethodGet, "/api/plans/first/activity", nil)
	again.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, again)
	if etag == "" || second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("etag %q, status %d, %d bytes", etag, second.Code, second.Body.Len())
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
	if state := liveState(t, handler); state.Done != 0 || state.Tasks["prepare"].Class != "ready" {
		t.Fatalf("state = %+v", state)
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
