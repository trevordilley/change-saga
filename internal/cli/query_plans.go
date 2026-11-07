package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Plans and Tasks are read through query like every other Saga record, so an
// agent gets their full essays as text instead of opening the rendered reader.

// planQuerySummary is one Plan as query plans lists it.
type planQuerySummary struct {
	ID          string     `json:"id"`
	Target      string     `json:"target"`
	Title       string     `json:"title"`
	Objective   string     `json:"objective,omitempty"`
	Path        string     `json:"path"`
	Phases      int        `json:"phase_count"`
	Tasks       int        `json:"task_count"`
	TasksDone   int        `json:"tasks_done"`
	Slides      int        `json:"slide_count"`
	Prototypes  []string   `json:"prototypes"`
	Reviews     []string   `json:"reviews"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

// planQueryRecord is one Plan with its essay, phases, and compact Tasks.
type planQueryRecord struct {
	planQuerySummary
	// ActivityNote says why there is no live activity, when there cannot be any.
	ActivityNote string              `json:"activity_note,omitempty"`
	Essay        *saga.DeckOverview  `json:"essay,omitempty"`
	PhaseRows    []planQueryPhase    `json:"phases"`
	TaskRows     []planQueryTaskRow  `json:"tasks"`
	SlideRows    []planQuerySlideRow `json:"slides"`
	// Names is the Plan's names with their progress and gaps, when it has any.
	Names *planNamesResult `json:"names,omitempty"`
}

type planQueryPhase struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Tasks []string `json:"tasks"`
}

// planQueryTaskRow leaves out the essay: query task returns it.
type planQueryTaskRow struct {
	taskRow
	HasEssay bool `json:"has_essay"`
}

type planQuerySlideRow struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Title  string `json:"title"`
}

// taskQueryRecord is one Task with its essay and where it appears.
type taskQueryRecord struct {
	taskRow
	Path   string              `json:"path"`
	Plan   string              `json:"plan"`
	Essay  string              `json:"essay,omitempty"`
	Slides []planQuerySlideRow `json:"slides"`
	// ActivityNote says why there is no live activity, when there cannot be any.
	ActivityNote string `json:"activity_note,omitempty"`
	// ActivityHistory is the Task's newest messages, only with --history.
	ActivityHistory []planactivity.Event `json:"activity_history,omitempty"`
}

// activityHistoryLimit bounds what query task --history returns, however long a
// log has grown.
const activityHistoryLimit = 200

func queryPlanSummary(plan *saga.Plan) planQuerySummary {
	summary := planQuerySummary{
		ID: plan.ID, Target: plan.Target, Title: plan.Title, Path: plan.Path,
		Phases: len(plan.Phases), Tasks: len(plan.Tasks), DeliveredAt: plan.DeliveredAt,
		Prototypes: append([]string{}, plan.Prototypes...), Reviews: append([]string{}, plan.Reviews...),
	}
	for _, task := range plan.Tasks {
		if task.State == saga.TaskDone {
			summary.TasksDone++
		}
	}
	if plan.Deck != nil {
		summary.Objective = plan.Deck.Objective
		summary.Slides = len(plan.Deck.Slides)
	}
	return summary
}

func taskRows(plan *saga.Plan) map[string]taskRow {
	manifests := plan.Manifests()
	unblocks := saga.TaskUnblocks(manifests)
	canStart := map[string]bool{}
	ready, held := saga.TasksReadyNow(plan.Phases, manifests)
	for _, task := range ready {
		canStart[task.ID] = true
	}
	rows := make(map[string]taskRow, len(plan.Tasks))
	for _, task := range plan.Tasks {
		rows[task.ID] = taskRow{TaskManifest: task.TaskManifest, Target: task.Target, Unblocks: append([]string{}, unblocks[task.ID]...), CanStart: canStart[task.ID], HeldBy: held[task.ID]}
	}
	return rows
}

// loadPlansForQuery parses one plans, plan, or task request and reads the
// current Plans, with a snapshot that changes when any of them does.
func loadPlansForQuery(operation string, args []string, out io.Writer, register func(*flag.FlagSet)) (*saga.Saga, string, *flag.FlagSet, bool, error) {
	flags := flag.NewFlagSet("query "+operation, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	register(flags)
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil, "", nil, true, writeQuerySuccess(out, "", queryHelpFor(operation), nil)
		}
		return nil, "", nil, true, writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *root == "" || flags.NArg() != 0 {
		return nil, "", nil, true, writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage[operation]})
	}
	document, validation, err := saga.LoadOutline(*root)
	if err != nil {
		return nil, "", nil, true, writeQueryFailure(out, normalizeQueryError(err))
	}
	if !validation.Valid {
		return nil, "", nil, true, writeQueryFailure(out, &queryError{Code: "invalid_saga", Message: "Saga validation failed", Details: validation})
	}
	encoded, _ := json.Marshal(document.Plans)
	return document, fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)), flags, false, nil
}

// planIDFor accepts a Plan's ID or URN, of this Saga only.
func planIDFor(document *saga.Saga, value string) (string, bool) {
	if !strings.HasPrefix(value, "urn:") {
		return value, true
	}
	return strings.CutPrefix(value, saga.PlanTarget(document.Manifest.ID, ""))
}

func queryPlans(args []string, out io.Writer) error {
	var cursor string
	var limit int
	var all bool
	document, snapshot, _, done, err := loadPlansForQuery("plans", args, out, func(flags *flag.FlagSet) {
		flags.StringVar(&cursor, "cursor", "", "snapshot-bound continuation")
		flags.IntVar(&limit, "limit", 50, "page size, 1 to 100")
		flags.BoolVar(&all, "all", false, "include delivered Plans, hidden by default")
	})
	if done {
		return err
	}
	if limit < 1 || limit > 100 || len(cursor) > 4096 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["plans"]})
	}
	plans := []*saga.Plan{}
	for _, plan := range document.Plans {
		if all || !plan.Delivered() {
			plans = append(plans, plan)
		}
	}
	key := fmt.Sprintf("plans:%s:%t", document.Manifest.ID, all)
	offset, cursorErr := decodeTermsCursor(cursor, key, snapshot, len(plans))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(offset+limit, len(plans))
	records := []planQuerySummary{}
	for _, plan := range plans[offset:end] {
		records = append(records, queryPlanSummary(plan))
	}
	page := queryPageEnvelope{Total: len(plans), Returned: len(records), HasMore: end < len(plans)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	return writeQuerySuccess(out, snapshot, map[string]any{"plans": records}, &page)
}

func queryPlan(args []string, out io.Writer) error {
	var selector, repo string
	document, snapshot, _, done, err := loadPlansForQuery("plan", args, out, func(flags *flag.FlagSet) {
		flags.StringVar(&selector, "plan", "", "plan ID or URN")
		flags.StringVar(&repo, "repo", "", "code checkout when the Saga lives outside Git")
	})
	if done {
		return err
	}
	id, ok := planIDFor(document, selector)
	if selector == "" || !ok {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["plan"]})
	}
	plan := document.FindPlan(id)
	if plan == nil {
		return writeQueryFailure(out, &queryError{Code: "not_found", Message: "plan does not exist"})
	}
	record := planQueryRecord{planQuerySummary: queryPlanSummary(plan), PhaseRows: []planQueryPhase{}, TaskRows: []planQueryTaskRow{}, SlideRows: []planQuerySlideRow{}}
	rows := taskRows(plan)
	for _, phase := range plan.Phases {
		row := planQueryPhase{ID: phase.ID, Title: phase.Title, Tasks: []string{}}
		for _, task := range plan.Tasks {
			if task.Phase == phase.ID {
				row.Tasks = append(row.Tasks, task.ID)
			}
		}
		record.PhaseRows = append(record.PhaseRows, row)
	}
	live, _, note := readTaskActivity(context.Background(), document.Root, repo, plan.ID)
	record.ActivityNote = note
	now := time.Now()
	for _, task := range plan.Tasks {
		row := rows[task.ID]
		row.Activity = activityView(live[task.ID], task.State, now)
		record.TaskRows = append(record.TaskRows, planQueryTaskRow{taskRow: row, HasEssay: task.Essay != ""})
	}
	if plan.Deck != nil {
		record.Essay = plan.Deck.Overview
		for _, slide := range plan.Deck.Slides {
			record.SlideRows = append(record.SlideRows, planQuerySlideRow{ID: slide.ID, Target: slide.Target, Title: slide.Title})
		}
	}
	if len(plan.Names) > 0 {
		names := namesResult(plan)
		record.Names = &names
	}
	return writeQuerySuccess(out, snapshot, map[string]any{"plan": record}, nil)
}

func queryTask(args []string, out io.Writer) error {
	var selector, planSelector, repo string
	var history bool
	document, snapshot, _, done, err := loadPlansForQuery("task", args, out, func(flags *flag.FlagSet) {
		flags.StringVar(&selector, "task", "", "task ID or URN")
		flags.StringVar(&planSelector, "plan", "", "plan ID or URN, when a task ID is used in several Plans")
		flags.BoolVar(&history, "history", false, "also return the Task's messages, up to the newest 200")
		flags.StringVar(&repo, "repo", "", "code checkout when the Saga lives outside Git")
	})
	if done {
		return err
	}
	if selector == "" {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["task"]})
	}
	taskID := selector
	if strings.HasPrefix(selector, "urn:") {
		before, after, found := strings.Cut(selector, ":task:")
		if !found {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--task must be a task ID or URN of this Saga"})
		}
		taskID = after
		if planSelector != "" && planSelector != before {
			if id, ok := planIDFor(document, before); !ok || (id != planSelector) {
				return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--task names a task of another Plan"})
			}
		}
		planSelector = before
	}
	plans := document.Plans
	if planSelector != "" {
		id, ok := planIDFor(document, planSelector)
		if !ok {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--plan must be a plan ID or URN of this Saga"})
		}
		plan := document.FindPlan(id)
		if plan == nil {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "plan does not exist"})
		}
		plans = []*saga.Plan{plan}
	}
	var owner *saga.Plan
	var task *saga.Task
	holders := []string{}
	for _, plan := range plans {
		if found := plan.FindTask(taskID); found != nil {
			owner, task = plan, found
			holders = append(holders, plan.ID)
		}
	}
	if task == nil {
		return writeQueryFailure(out, &queryError{Code: "not_found", Message: "task does not exist"})
	}
	if len(holders) > 1 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "task ID is used in several Plans; pass --plan", Details: map[string]any{"plans": holders}})
	}
	record := taskQueryRecord{taskRow: taskRows(owner)[task.ID], Path: task.Path, Plan: owner.Target, Essay: task.Essay, Slides: []planQuerySlideRow{}}
	if owner.Deck != nil {
		for _, slide := range owner.Deck.Slides {
			for _, item := range slide.Items {
				if item.Record == task.Target {
					record.Slides = append(record.Slides, planQuerySlideRow{ID: slide.ID, Target: slide.Target, Title: slide.Title})
					break
				}
			}
		}
	}
	live, _, note := readTaskActivity(context.Background(), document.Root, repo, owner.ID)
	record.ActivityNote = note
	record.Activity = activityView(live[task.ID], task.State, time.Now())
	if history && live[task.ID] != nil {
		events := live[task.ID].Events
		record.ActivityHistory = events[max(0, len(events)-activityHistoryLimit):]
	}
	return writeQuerySuccess(out, snapshot, map[string]any{"task": record}, nil)
}
