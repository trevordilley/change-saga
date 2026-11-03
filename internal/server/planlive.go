package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A Plan left open shows what agents are doing now. What they report lives in a
// log shared by the repository's worktrees, read by planactivity; this file turns
// it into what a reader sees. One status function decides how a Task reads, for
// the first render and for every update after, so the two never disagree.

const (
	// planFeedRows and planHistoryRows bound what a page carries. The log keeps
	// more; query task --history reads it.
	planFeedRows    = 30
	planHistoryRows = 30
	// planNodeDetailRunes is what fits on a board node beside its check.
	planNodeDetailRunes = 30
	// planActivityRecheck is how soon to look for a log again when there was none:
	// a Saga can be committed while the server runs, but rarely.
	planActivityRecheck = time.Minute
)

// planActivitySource remembers where the Saga's log is, so a reader polling for
// hours never asks Git.
type planActivitySource struct {
	mutex   sync.Mutex
	log     planactivity.Log
	reason  string
	checked time.Time
}

func (a *app) planActivityLog(ctx context.Context) (planactivity.Log, string) {
	source := &a.planActivity
	source.mutex.Lock()
	defer source.mutex.Unlock()
	if source.log != nil || (!source.checked.IsZero() && time.Since(source.checked) < planActivityRecheck) {
		return source.log, source.reason
	}
	source.checked = time.Now()
	manifest, err := saga.ReadManifest(a.root)
	if err != nil {
		return nil, ""
	}
	source.log, source.reason = planactivity.Open(ctx, a.root, a.sourceDir, manifest.ID)
	return source.log, source.reason
}

// planActivityEvents reads a Plan's events, and why there can be none. A log that
// cannot be read is no activity: it is never worth failing a page.
func (a *app) planActivityEvents(ctx context.Context, planID string) ([]planactivity.Event, string) {
	log, reason := a.planActivityLog(ctx)
	if log == nil {
		return nil, reason
	}
	events, err := log.Read(planID)
	if err != nil {
		return nil, ""
	}
	return events, ""
}

// taskReadiness is what a Task's status reads beside its own record: whether it
// could start, and what agents have said about it.
type taskReadiness struct {
	can  map[string]bool
	live map[string]*planactivity.TaskActivity
	now  time.Time
}

// liveStatus says how a Task reads. The durable record wins: a Task done in this
// checkout is done, whatever the log says. Otherwise what agents last said decides,
// and a Task nobody reported on reads as its record does.
func liveStatus(id, state string, canStart bool, live *planactivity.TaskActivity, now time.Time) taskStatus {
	status := taskStatus{TaskID: id}
	switch {
	case state == saga.TaskDone:
		status.Class, status.Word = "done", "Done"
	case live.State() == planactivity.StateFinished:
		status.Class, status.Word = "done-branch", "Done on another branch"
		if live.Finished.Branch != "" {
			status.Word = "Done on " + live.Finished.Branch
		}
	case live.State() == planactivity.StateBlocked:
		status.Class, status.Word = "blocked", "Blocked"
	case live.State() == planactivity.StateWorking:
		status.Class, status.Word = "working", "In progress"
	case state == saga.TaskInProgress:
		status.Class, status.Word = "working", "In progress"
	case canStart:
		status.Class, status.Word = "ready", "Can start now"
	default:
		status.Class, status.Word = "waiting", "Waiting"
	}
	if status.Class == "working" || status.Class == "blocked" {
		agents := live.Agents()
		if len(agents) > 2 {
			agents = append(agents[:2:2], "+"+strconv.Itoa(len(live.Agents())-2))
		}
		status.Who = strings.Join(agents, ", ")
		if live != nil && len(live.Holders) > 0 {
			status.Message = live.Holders[0].Message
		}
		if live.Quiet(now) {
			status.Class += " quiet"
			status.Word += ", quiet"
		}
	}
	return status
}

// planLiveView is what a Plan's pages need to stay current, and the feed.
type planLiveView struct {
	// URL is where to poll. A delivered Plan no longer changes, so it has none.
	URL string
	// Reason says why there can be no live activity.
	Reason string
	Feed   []planActivityRow
}

// planActivityRow is one thing an agent said, in a feed or a Task's history.
type planActivityRow struct {
	Agent, Class, Verb, Branch, Message, At, Stamp, Ago string
	// TaskTitle and Drawer name the Task, in a feed only.
	TaskTitle, Drawer string
}

// planActivityVerbs say what each kind of event means, in a Task's own history
// and in a feed that goes on to name the Task.
var planActivityVerbs = map[string][2]string{
	planactivity.KindStart:    {"started", "started"},
	planactivity.KindNote:     {"noted", "noted on"},
	planactivity.KindBlocked:  {"blocked", "blocked on"},
	planactivity.KindDone:     {"finished", "finished"},
	planactivity.KindReleased: {"let go", "let go of"},
}

func planActivityRowOf(event planactivity.Event, now time.Time, titles map[string]string, inFeed bool) planActivityRow {
	row := planActivityRow{Agent: event.Agent, Class: event.Kind, Message: event.Message, At: event.At.Format(time.RFC3339), Stamp: event.At.Local().Format("2006-01-02 15:04:05"), Ago: planactivity.Ago(now, event.At)}
	verbs, known := planActivityVerbs[event.Kind]
	if !known {
		verbs = [2]string{event.Kind, event.Kind}
	}
	row.Verb = verbs[0]
	if event.Kind == planactivity.KindDone {
		row.Branch = event.Branch
	}
	if inFeed {
		row.Verb, row.TaskTitle = verbs[1], event.Task
		if title, found := titles[event.Task]; found {
			row.TaskTitle, row.Drawer = title, planTaskDrawer(event.Task)
		}
	}
	return row
}

// planActivityRows are the newest limit events, newest first.
func planActivityRows(events []planactivity.Event, now time.Time, titles map[string]string, inFeed bool, limit int) []planActivityRow {
	rows := make([]planActivityRow, 0, min(len(events), limit))
	for index := len(events) - 1; index >= 0 && len(rows) < limit; index-- {
		rows = append(rows, planActivityRowOf(events[index], now, titles, inFeed))
	}
	return rows
}

func planActivityURL(planID string) string { return "/api" + planHref(planID) + "/activity" }

// planLiveFor reads a Plan's activity for the first render of one of its pages.
func (a *app) planLiveFor(ctx context.Context, plan *saga.Plan) (taskReadiness, *planLiveView) {
	ready := planReady(plan)
	events, reason := a.planActivityEvents(ctx, plan.ID)
	ready.live = planactivity.Summarize(events)
	view := &planLiveView{Reason: reason}
	if !plan.Delivered() && reason == "" {
		view.URL = planActivityURL(plan.ID)
	}
	titles := make(map[string]string, len(plan.Tasks))
	for _, task := range plan.Tasks {
		titles[task.ID] = task.Title
	}
	view.Feed = planActivityRows(events, ready.now, titles, true, planFeedRows)
	return ready, view
}

// planLiveTask is one Task as the page patches it.
type planLiveTask struct {
	Class   string `json:"class"`
	Word    string `json:"word"`
	Who     string `json:"who,omitempty"`
	OnIt    string `json:"on_it,omitempty"`
	Detail  string `json:"detail"`
	Label   string `json:"label"`
	Action  string `json:"action"`
	History string `json:"history,omitempty"`
}

// planLiveResponse is everything a page left open needs to catch up. It carries
// what to show, already decided and rendered, so the script only places it.
type planLiveResponse struct {
	Available bool                    `json:"available"`
	Reason    string                  `json:"reason,omitempty"`
	Done      int                     `json:"done"`
	All       int                     `json:"all"`
	Tasks     map[string]planLiveTask `json:"tasks"`
	Feed      string                  `json:"feed"`
}

// planActivityAPI answers a page left open. It reads the activity log and the
// Plan's small Task records, never the Saga, and says 304 while nothing a reader
// would see has changed, so a page can poll it for hours at almost no cost. It
// does not count as a reader using the server, so the Saga watcher still goes idle.
func (a *app) planActivityAPI(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !saga.ValidID(id) {
		http.NotFound(w, r)
		return
	}
	_, tasks, err := saga.ReadPlanRecords(a.root, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	events, reason := a.planActivityEvents(r.Context(), id)
	live := planactivity.Summarize(events)
	now := time.Now()
	canStart := map[string]bool{}
	for _, task := range saga.TasksReady(tasks) {
		canStart[task.ID] = true
	}
	titles := make(map[string]string, len(tasks))
	for _, task := range tasks {
		titles[task.ID] = task.Title
	}
	response := planLiveResponse{Available: reason == "", Reason: reason, All: len(tasks), Tasks: make(map[string]planLiveTask, len(tasks))}
	for _, task := range tasks {
		if task.State == saga.TaskDone {
			response.Done++
		}
		status := liveStatus(task.ID, task.State, canStart[task.ID], live[task.ID], now)
		row := planLiveTask{Class: status.Class, Word: status.Word, Who: status.Who, OnIt: status.OnIt(), Detail: status.Detail(), Label: task.Title + ": " + status.Word, Action: "Open task: " + task.Title + ". " + status.Word}
		if activity := live[task.ID]; activity != nil {
			row.History = renderPlanActivity(planActivityRows(activity.Events, now, nil, false, planHistoryRows))
		}
		response.Tasks[task.ID] = row
	}
	response.Feed = renderPlanActivity(planActivityRows(events, now, titles, true, planFeedRows))
	body, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "The activity could not be read.", http.StatusInternalServerError)
		return
	}
	digest := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(digest[:12]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func renderPlanActivity(rows []planActivityRow) string {
	var out bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&out, "plan-activity-rows", rows); err != nil {
		return ""
	}
	return out.String()
}

const planLiveTemplates = `{{define "plan-activity-rows"}}{{range .}}<li class="plan-activity-row {{.Class}}" data-plan-activity-kind="{{.Class}}"><span class="plan-activity-line"><strong>{{.Agent}}</strong> {{.Verb}}{{if .TaskTitle}} {{if .Drawer}}<button type="button" class="plan-activity-task" data-plan-drawer="{{.Drawer}}" data-plan-drawer-title="Task" data-plan-drawer-beside="true">{{.TaskTitle}}</button>{{else}}<span class="plan-activity-task">{{.TaskTitle}}</span>{{end}}{{end}}{{if .Branch}} on {{.Branch}}{{end}} <time datetime="{{.At}}" title="{{.Stamp}}">{{.Ago}}</time></span>{{if .Message}}<span class="plan-activity-message">{{.Message}}</span>{{end}}</li>{{end}}{{end}}
{{define "plan-live-feed"}}<details class="plan-live" data-plan-live-feed open><summary>Live updates</summary>{{if .Reason}}<p class="plan-note" data-plan-live-reason>{{.Reason}}</p>{{else}}<p class="plan-note" data-plan-live-status role="status" hidden></p><ol class="plan-activity" data-plan-feed data-plan-empty="No updates yet. Agents report with task start and task note.">{{template "plan-activity-rows" .Feed}}</ol>{{end}}</details>{{end}}
{{define "plan-task-history"}}<section class="plan-task-updates" data-plan-task-updates><h3>Updates</h3><ol class="plan-activity" data-plan-task-history="{{.ID}}" data-plan-empty="No updates yet.">{{template "plan-activity-rows" .History}}</ol></section>{{end}}
{{define "plan-task-on-it"}}<p class="plan-note plan-task-on-it" data-plan-task-on-it="{{.TaskID}}"{{if not .OnIt}} hidden{{end}}>{{.OnIt}}</p>{{end}}
`

const planLiveStyles = `
.plan-state.blocked{color:var(--review-changes-ink);background:var(--review-changes-bg)}
.plan-state.done-branch{color:var(--review-approved-ink);background:transparent;outline:1px dashed var(--review-approved-ink);outline-offset:-1px}
.plan-state.quiet{color:var(--muted);background:var(--review-none-bg)}
.plan-board-node.working rect{stroke-width:2.5}
.plan-board-node.blocked rect{fill:var(--review-changes-bg);stroke:var(--review-changes-ink);stroke-width:3}
.plan-board-node.blocked .plan-board-state{fill:var(--review-changes-ink)}
.plan-board-node.done-branch rect{fill:var(--bg);stroke:var(--review-approved-ink);stroke-dasharray:6 4}
.plan-board-node.done-branch .plan-board-state{fill:var(--review-approved-ink)}
.plan-board-node.quiet rect{stroke-dasharray:2 5}
.plan-board-node.quiet .plan-board-state{fill:var(--muted)}
.plan-board-glyph{display:none;font-size:15px;font-weight:700;text-anchor:end}
.plan-board-node.done .plan-board-check,.plan-board-node.done-branch .plan-board-check{display:block;fill:var(--review-approved-ink)}
.plan-board-node.blocked .plan-board-flag{display:block;fill:var(--review-changes-ink)}
.landmark-hotspot[data-plan-task-state~="working"]{border-color:var(--review-stale-ink);border-style:solid}
.landmark-hotspot[data-plan-task-state~="blocked"]{border-color:var(--review-changes-ink);border-style:solid;border-width:3px}
.landmark-hotspot[data-plan-task-state~="done-branch"]{border-color:var(--review-approved-ink);border-style:dashed}
.landmark-hotspot[data-plan-task-state~="quiet"]{border-style:dotted}
.landmark-hotspot[data-plan-task-state~="done-branch"]::before{content:'✓';position:absolute;left:-7px;top:-7px;width:16px;height:16px;box-sizing:border-box;border:1px dashed var(--review-approved-ink);border-radius:50%;background:var(--bg);color:var(--review-approved-ink);font:700 10px/14px var(--ui);text-align:center;pointer-events:none}
.landmark-hotspot[data-plan-task-state~="blocked"]::before{content:'!';position:absolute;left:-7px;top:-7px;width:16px;height:16px;box-sizing:border-box;border:1px solid var(--review-changes-ink);border-radius:50%;background:var(--review-changes-bg);color:var(--review-changes-ink);font:700 10px/14px var(--ui);text-align:center;pointer-events:none}
.landmark-hotspot[data-plan-task-holder]:not([data-plan-task-holder=""])::after{content:attr(data-plan-task-holder);position:absolute;left:4px;bottom:-9px;max-width:calc(100% - 8px);overflow:hidden;padding:0 6px;border-radius:8px;background:var(--bg);color:var(--ink);box-shadow:0 0 0 1px var(--line);font:600 11px/16px var(--ui);text-overflow:ellipsis;white-space:nowrap;pointer-events:none}
.plan-task-on-it{margin:5px 0 0}
.plan-task-updates h3{margin:14px 0 6px}
.plan-activity{list-style:none;margin:0;padding:0;font-size:13px}
.plan-activity:empty::before{content:attr(data-plan-empty);color:var(--muted)}
.plan-activity-row{padding:5px 0;border-top:1px solid var(--line-soft)}
.plan-activity-row:first-child{border-top:0}
.plan-activity-row.blocked{box-shadow:inset 3px 0 0 var(--review-changes-ink);padding-left:8px}
.plan-activity-line{display:block}
.plan-activity-line time{color:var(--muted);font-size:12px;white-space:nowrap}
.plan-activity-message{display:block;color:var(--ink);white-space:pre-wrap;overflow-wrap:anywhere}
.plan-activity-task{padding:0;border:0;background:none;color:var(--accent);font:inherit;text-align:left;cursor:pointer}
span.plan-activity-task{color:var(--ink);cursor:default}
button.plan-activity-task:hover{text-decoration:underline}
.plan-live summary{cursor:pointer;color:var(--muted);font:600 12px var(--ui)}
.plan-live .plan-activity{margin-top:4px}
.review-deck-rail .plan-live{margin:8px 0;padding:0 2px}
.review-deck-rail .plan-live .plan-activity{max-height:220px;overflow:auto}
.plan-tasks-view{position:relative}
.plan-tasks-view>.plan-live{position:absolute;left:12px;bottom:12px;z-index:3;width:min(340px,calc(100% - 24px));padding:8px 11px;border:1px solid var(--line);border-radius:9px;background:var(--bg)}
.plan-tasks-view>.plan-live .plan-activity{max-height:30vh;overflow:auto}
.plan-surface>.plan-live{margin:0 0 14px}
`

// planLiveJavaScript keeps a Plan's page current while it stays open. It polls only
// while the page is visible, and only places what the server decided: a class on a
// node, a word, a list of rows. It never redraws the board or reopens a drawer, so
// the camera, the selection, and an open slide-out all stay as the reader left them.
const planLiveJavaScript = `
  const planLive = {timer: null, url: null, etag: null, failures: 0, running: false};
  const planLiveClasses = ['done', 'done-branch', 'ready', 'working', 'waiting', 'blocked', 'quiet'];
  // Everything a Plan page shows or will show: the page, an open drawer, and the
  // templates its slide-outs and hotspots are cloned from.
  function planLiveRoots() {
    const roots = [document];
    const walk = root => qa('template', root).forEach(template => { roots.push(template.content); walk(template.content); });
    const page = q('#page');
    if (page) walk(page);
    return roots;
  }
  function setPlanLiveRows(list, html) {
    // Nothing is touched when nothing changed, so a reader's selection survives.
    if (list.dataset.planLiveRows === html) return;
    list.dataset.planLiveRows = html;
    list.innerHTML = html;
  }
  function applyPlanLive(state) {
    for (const root of planLiveRoots()) {
      for (const pill of qa('[data-plan-task-status]', root)) {
        const task = state.tasks[pill.dataset.planTaskStatus];
        if (task) { pill.className = 'plan-state ' + task.class; pill.textContent = task.word; }
      }
      for (const line of qa('[data-plan-task-on-it]', root)) {
        const task = state.tasks[line.dataset.planTaskOnIt];
        if (task) { line.textContent = task.on_it || ''; line.hidden = !task.on_it; }
      }
      for (const node of qa('[data-plan-node^="task:"]', root)) {
        const task = state.tasks[node.dataset.planNode.slice(5)];
        if (!task) continue;
        node.classList.remove(...planLiveClasses);
        node.classList.add(...task.class.split(' '));
        node.setAttribute('aria-label', task.label);
        const detail = q('.plan-board-state', node);
        if (detail) detail.textContent = task.detail;
      }
      for (const place of qa('[data-plan-task-id]', root)) {
        const task = state.tasks[place.dataset.planTaskId];
        if (task) { place.dataset.planTaskState = task.class; place.dataset.planTaskHolder = task.who || ''; }
      }
      for (const button of qa('[data-plan-task-action]', root)) {
        const task = state.tasks[button.dataset.planTaskAction];
        if (task) { button.setAttribute('aria-label', task.action); button.title = task.action; }
      }
      for (const list of qa('[data-plan-task-history]', root)) {
        const task = state.tasks[list.dataset.planTaskHistory];
        if (task) setPlanLiveRows(list, task.history || '');
      }
      for (const list of qa('[data-plan-feed]', root)) setPlanLiveRows(list, state.feed);
      for (const count of qa('[data-plan-done]', root)) count.textContent = String(state.done);
    }
  }
  function setPlanLiveStatus(message) {
    for (const status of qa('[data-plan-live-status]')) { status.textContent = message; status.hidden = !message; }
  }
  function currentPlanLiveURL() { return q('#page [data-plan-live-url]')?.dataset.planLiveUrl || null; }
  function schedulePlanLive(delay) {
    clearTimeout(planLive.timer);
    planLive.timer = setTimeout(pollPlanLive, delay);
  }
  async function pollPlanLive() {
    planLive.timer = null;
    const url = currentPlanLiveURL();
    // A page with nothing to watch, or one nobody can see, asks for nothing. Arriving
    // on a Plan or coming back to the tab starts this again.
    if (!url || document.hidden || planLive.running) return;
    if (url !== planLive.url) { planLive.url = url; planLive.etag = null; planLive.failures = 0; }
    planLive.running = true;
    let stop = false;
    try {
      const response = await fetch(url, {cache: 'no-store', headers: planLive.etag ? {'If-None-Match': planLive.etag} : {}});
      if (response.status === 404) stop = true;
      if (response.status !== 304 && !response.ok && !stop) throw new Error(String(response.status));
      planLive.failures = 0;
      setPlanLiveStatus('');
      if (response.status === 200 && currentPlanLiveURL() === url) {
        planLive.etag = response.headers.get('ETag');
        const state = await response.json();
        applyPlanLive(state);
        if (!state.available) { stop = true; setPlanLiveStatus(state.reason || ''); }
      }
    } catch {
      planLive.failures++;
      setPlanLiveStatus('Live updates paused. Trying again.');
    } finally {
      planLive.running = false;
    }
    if (!stop) schedulePlanLive(Math.min(30000, 2000 * 2 ** planLive.failures));
  }
  document.addEventListener('visibilitychange', () => { if (!document.hidden) schedulePlanLive(0); });
`
