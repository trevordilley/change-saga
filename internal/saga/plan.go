package saga

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
)

// A Plan describes a body of work and how it can be executed in parallel: the
// technical design for a change, as a slide deck organized by phase, and the
// Tasks that carry it out. It belongs to no feature. Its Tasks lead to user
// stories, and its stories and features are worked out from them. Git is the
// history of a Plan: when it changes it is the same record, committed again.
//
//	___plans/<id>.plan/
//	  plan.json               title, phases, prototypes, reviews, delivered
//	  deck/                   the Plan's deck: one flat deck bundle, role plan
//	  tasks/<id>.task/
//	    task.json             title, phase, story, depends_on, code, sizing, state
//	    essay.md              optional
const (
	PlansDir         = "___plans"
	PlanSuffix       = ".plan"
	PlanManifestName = "plan.json"
	PlanDeckDir      = "deck"
	PlanTasksDir     = "tasks"
	TaskSuffix       = ".task"
	TaskManifestName = "task.json"
	TaskEssayName    = "essay.md"

	PlanVersion   = SagaVersion
	PlanSchemaURL = "https://changesaga.dev/schema/v5/plan.schema.json"
	TaskSchemaURL = "https://changesaga.dev/schema/v5/task.schema.json"
)

// The states of a Task. Done is what an agent reports as it finishes; it never
// proves the work is correct.
const (
	TaskPlanned    = "planned"
	TaskInProgress = "in_progress"
	TaskDone       = "done"
)

// PlanPhase is one named stage of a Plan. Its place in Phases is its order; a
// phase is not a dependency, and Tasks in different phases may run together.
type PlanPhase struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Checkpoint makes the phase a checkpoint: it holds no Tasks and covers every
	// Task since the previous checkpoint. That stretch is reviewed together, and
	// the work after it waits until it passes.
	Checkpoint *PlanCheckpoint `json:"checkpoint,omitempty"`
}

// PlanCheckpoint is what a checkpoint phase records.
type PlanCheckpoint struct {
	// Review is the id of the review of the checkpoint's work.
	Review string `json:"review,omitempty"`
	// Passed records that the checkpoint passed, so the work after it may start.
	Passed *CheckpointPass `json:"passed,omitempty"`
}

// CheckpointPass says who passed a checkpoint, when, and why.
type CheckpointPass struct {
	At time.Time `json:"at"`
	By string    `json:"by"`
	// Agent says an agent passed it, under the rule of whoever invoked the Plan.
	Agent  bool   `json:"agent,omitempty"`
	Reason string `json:"reason"`
}

// Who may pass a Plan's checkpoints. A person is the default.
const (
	CheckpointPasserPerson = "person"
	CheckpointPasserAgent  = "agent"
)

// PlanManifest is plan.json.
type PlanManifest struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	// Phases is ordered. A Plan is encouraged, not required, to have them.
	Phases []PlanPhase `json:"phases,omitempty"`
	// Prototypes are the URNs of the prototypes showing what the Plan builds.
	Prototypes []string `json:"prototypes,omitempty"`
	// Reviews are the ids of the pull request reviews that implement the Plan.
	Reviews []string `json:"reviews,omitempty"`
	// CheckpointPasser is who passes its checkpoints: a person, or an agent unless
	// what it found is surprising or urgent. Empty means a person.
	CheckpointPasser string `json:"checkpoint_passer,omitempty"`
	// DeliveredAt records that an author marked the Plan delivered. A delivered
	// Plan is hidden by default, as a merged review is, and refuses edits.
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// TaskSizing is the judgment that one human developer, coding without AI help,
// would finish a Task in two days or less, which implies it is well understood.
type TaskSizing struct {
	WithinTwoDays bool   `json:"within_two_days"`
	Reason        string `json:"reason,omitempty"`
}

// TaskManifest is task.json.
type TaskManifest struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Phase   string `json:"phase,omitempty"`
	// Story is the URN of the user story the Task is part of. A Task without one
	// must unblock a Task that leads to a story.
	Story string `json:"story,omitempty"`
	// DependsOn names Tasks of the same Plan. What a Task unblocks is derived.
	DependsOn []string            `json:"depends_on,omitempty"`
	Code      []coderef.Reference `json:"code,omitempty"`
	Sizing    *TaskSizing         `json:"sizing,omitempty"`
	State     string              `json:"state"`
	Summary   string              `json:"summary,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
}

// Task is one loaded Task of a Plan.
type Task struct {
	TaskManifest
	// Path is the app-relative task directory.
	Path      string `json:"path"`
	Directory string `json:"-"`
	Target    string `json:"target"`
	// Essay is the task's optional essay.md.
	Essay string `json:"essay,omitempty"`
}

// Plan is one loaded Plan.
type Plan struct {
	PlanManifest
	// Path is the app-relative plan directory.
	Path      string  `json:"path"`
	Directory string  `json:"-"`
	Target    string  `json:"target"`
	Deck      *Deck   `json:"deck,omitempty"`
	Tasks     []*Task `json:"tasks,omitempty"`
}

// PlanTarget is the URN of a Plan; its deck, slides, Items, and Tasks are named
// beneath it so their IDs never collide with another Plan's.
// ReadPlanRecords reads only a Plan's record and its Tasks' records, for a caller
// asking too often to load the Saga. It does not validate them.
func ReadPlanRecords(root, planID string) (PlanManifest, []TaskManifest, error) {
	var plan PlanManifest
	dir := filepath.Join(root, PlansDir, planID+PlanSuffix)
	data, err := os.ReadFile(filepath.Join(dir, PlanManifestName))
	if err != nil {
		return plan, nil, err
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		return plan, nil, err
	}
	entries, err := os.ReadDir(filepath.Join(dir, PlanTasksDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return plan, nil, nil
		}
		return plan, nil, err
	}
	tasks := make([]TaskManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), TaskSuffix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, PlanTasksDir, entry.Name(), TaskManifestName))
		if err != nil {
			continue
		}
		var task TaskManifest
		if json.Unmarshal(data, &task) == nil {
			tasks = append(tasks, task)
		}
	}
	return plan, tasks, nil
}

// PlanRecordsStamp changes whenever ReadPlanRecords' answer might, reading only
// the size and time of those records.
func PlanRecordsStamp(root, planID string) (string, error) {
	dir := filepath.Join(root, PlansDir, planID+PlanSuffix)
	info, err := os.Stat(filepath.Join(dir, PlanManifestName))
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	fmt.Fprintf(digest, "%d\x00%d\x00", info.Size(), info.ModTime().UnixNano())
	entries, err := os.ReadDir(filepath.Join(dir, PlanTasksDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	for _, entry := range entries {
		if info, err := os.Stat(filepath.Join(dir, PlanTasksDir, entry.Name(), TaskManifestName)); err == nil {
			fmt.Fprintf(digest, "%s\x00%d\x00%d\x00", entry.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func PlanTarget(sagaID, planID string) string {
	return "urn:change-saga:" + sagaID + ":plan:" + planID
}

func PlanDeckTarget(sagaID, planID, deckID string) string {
	return PlanTarget(sagaID, planID) + ":deck:" + deckID
}

func PlanSlideTarget(sagaID, planID, slideID string) string {
	return PlanTarget(sagaID, planID) + ":slide:" + slideID
}

func PlanItemTarget(sagaID, planID, slideID, itemID string) string {
	return PlanSlideTarget(sagaID, planID, slideID) + ":item:" + itemID
}

func TaskTarget(sagaID, planID, taskID string) string {
	return PlanTarget(sagaID, planID) + ":task:" + taskID
}

// PlanIDOf returns the id of the Plan target names or names something inside,
// or "" when target is not plan-scoped. It reads the URN by segment, so a Saga
// or deck whose id is "plan" is never mistaken for one.
func PlanIDOf(target string) string {
	parts := strings.SplitN(target, ":", 5)
	if len(parts) != 5 || parts[0] != "urn" || parts[1] != "change-saga" || parts[3] != "plan" {
		return ""
	}
	id, _, _ := strings.Cut(parts[4], ":")
	return id
}

func planDeckTargets(sagaID, planID string) deckTargets {
	return deckTargets{
		deck:  func(id string) string { return PlanDeckTarget(sagaID, planID, id) },
		slide: func(id string) string { return PlanSlideTarget(sagaID, planID, id) },
		item:  func(slide, id string) string { return PlanItemTarget(sagaID, planID, slide, id) },
	}
}

// PlanDir returns the absolute directory of Plan id beneath root.
func PlanDir(root, id string) string {
	return filepath.Join(root, PlansDir, id+PlanSuffix)
}

// FindPlan returns the loaded Plan with id.
func (document *Saga) FindPlan(id string) *Plan {
	for _, plan := range document.Plans {
		if plan.ID == id {
			return plan
		}
	}
	return nil
}

// Slide returns the Plan slide with id or target.
func (plan *Plan) Slide(id string) *Slide {
	if plan.Deck == nil {
		return nil
	}
	for _, slide := range plan.Deck.Slides {
		if slide.ID == id || slide.Target == id {
			return slide
		}
	}
	return nil
}

// FindTask returns the Plan's Task with id or target.
func (plan *Plan) FindTask(id string) *Task {
	for _, task := range plan.Tasks {
		if task.ID == id || task.Target == id {
			return task
		}
	}
	return nil
}

// Manifests returns the Plan's Task manifests, for checking a candidate graph.
func (plan *Plan) Manifests() []TaskManifest {
	manifests := make([]TaskManifest, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		manifests = append(manifests, task.TaskManifest)
	}
	return manifests
}

// Delivered reports whether an author marked the Plan delivered.
func (plan *Plan) Delivered() bool { return plan.DeliveredAt != nil }

// ValidTaskState reports whether state is a Task state.
func ValidTaskState(state string) bool {
	return state == TaskPlanned || state == TaskInProgress || state == TaskDone
}

// ValidatePlanManifest checks plan.json.
func ValidatePlanManifest(manifest PlanManifest) error {
	switch {
	case manifest.Schema != PlanSchemaURL:
		return fmt.Errorf("$schema must be %s", PlanSchemaURL)
	case manifest.Version != PlanVersion:
		return fmt.Errorf("version must be %d", PlanVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	case manifest.DeliveredAt != nil && manifest.DeliveredAt.IsZero():
		return fmt.Errorf("delivered_at must be a time when present")
	}
	phases := map[string]bool{}
	for _, phase := range manifest.Phases {
		if !ValidID(phase.ID) || strings.TrimSpace(phase.Title) == "" {
			return fmt.Errorf("a phase needs a stable id and a title")
		}
		if phases[phase.ID] {
			return fmt.Errorf("phase %q appears more than once", phase.ID)
		}
		phases[phase.ID] = true
		if checkpoint := phase.Checkpoint; checkpoint != nil {
			if checkpoint.Review != "" && !ValidID(checkpoint.Review) {
				return fmt.Errorf("checkpoint %q names a review that is not a stable id", phase.ID)
			}
			if pass := checkpoint.Passed; pass != nil && (pass.At.IsZero() || strings.TrimSpace(pass.By) == "" || strings.TrimSpace(pass.Reason) == "") {
				return fmt.Errorf("checkpoint %q was passed without saying when, by whom, and why", phase.ID)
			}
		}
	}
	switch manifest.CheckpointPasser {
	case "", CheckpointPasserPerson, CheckpointPasserAgent:
	default:
		return fmt.Errorf("checkpoint_passer must be %s or %s", CheckpointPasserPerson, CheckpointPasserAgent)
	}
	return nil
}

// IsCheckpoint says the phase is a checkpoint.
func (phase PlanPhase) IsCheckpoint() bool { return phase.Checkpoint != nil }

// CheckpointPassed says the phase is a checkpoint that has passed.
func (phase PlanPhase) CheckpointPassed() bool {
	return phase.Checkpoint != nil && phase.Checkpoint.Passed != nil
}

// CheckpointScope is the Tasks a checkpoint covers: those in the phases after the
// previous checkpoint and before it. A Task with no phase is in no checkpoint.
func CheckpointScope(phases []PlanPhase, tasks []TaskManifest, checkpointID string) []TaskManifest {
	covered := map[string]bool{}
	pending := []string{}
	found := false
	for _, phase := range phases {
		if phase.IsCheckpoint() {
			if phase.ID == checkpointID {
				for _, id := range pending {
					covered[id] = true
				}
				found = true
				break
			}
			pending = pending[:0]
			continue
		}
		pending = append(pending, phase.ID)
	}
	scope := []TaskManifest{}
	if !found {
		return scope
	}
	for _, task := range tasks {
		if covered[task.Phase] {
			scope = append(scope, task)
		}
	}
	return scope
}

// CheckpointHolds names, for each Task waiting behind a checkpoint that has not
// passed, the first such checkpoint. Work after it should not start until it does.
func CheckpointHolds(phases []PlanPhase, tasks []TaskManifest) map[string]string {
	holder := map[string]string{}
	blocking := ""
	for _, phase := range phases {
		if phase.IsCheckpoint() {
			if blocking == "" && !phase.CheckpointPassed() {
				blocking = phase.ID
			}
			continue
		}
		if blocking != "" {
			holder[phase.ID] = blocking
		}
	}
	holds := map[string]string{}
	for _, task := range tasks {
		if checkpoint := holder[task.Phase]; checkpoint != "" {
			holds[task.ID] = checkpoint
		}
	}
	return holds
}

// ValidateTaskManifest checks one task.json on its own: everything except how
// it relates to the rest of its Plan, which PlanTaskProblems checks.
func ValidateTaskManifest(sagaID string, manifest TaskManifest) error {
	switch {
	case manifest.Schema != TaskSchemaURL:
		return fmt.Errorf("$schema must be %s", TaskSchemaURL)
	case manifest.Version != PlanVersion:
		return fmt.Errorf("version must be %d", PlanVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case !ValidTaskState(manifest.State):
		return fmt.Errorf("state must be %s, %s, or %s", TaskPlanned, TaskInProgress, TaskDone)
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	case manifest.Story != "" && !validRecordOfKinds(sagaID, manifest.Story, []string{"story"}):
		return fmt.Errorf("story must be a canonical story URN of this Saga")
	}
	for _, reference := range manifest.Code {
		if reference.WholeFile() {
			return fmt.Errorf("code references must name exact lines")
		}
		if err := coderef.Validate(reference); err != nil {
			return fmt.Errorf("code: %w", err)
		}
	}
	return nil
}

// TaskProblem is one Task a Plan's graph refuses, and why.
type TaskProblem struct {
	Task    string `json:"task"`
	Message string `json:"message"`
}

// PlanTaskProblems checks a Plan's Tasks as a graph: every dependency names a
// Task of the Plan, phases exist, there are no cycles, and every Task leads to a
// user story. It runs in time linear in the Tasks and their dependencies, so a
// command can afford it on every write. Problems are returned in Task order.
func PlanTaskProblems(phases []PlanPhase, tasks []TaskManifest) []TaskProblem {
	problems := []TaskProblem{}
	known := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if known[task.ID] {
			problems = append(problems, TaskProblem{task.ID, "the task id appears more than once"})
		}
		known[task.ID] = true
	}
	phaseIDs := make(map[string]bool, len(phases))
	phaseOrder := make(map[string]int, len(phases))
	checkpoints := map[string]bool{}
	for index, phase := range phases {
		phaseIDs[phase.ID] = true
		phaseOrder[phase.ID] = index
		checkpoints[phase.ID] = phase.IsCheckpoint()
	}
	taskPhase := make(map[string]string, len(tasks))
	for _, task := range tasks {
		taskPhase[task.ID] = task.Phase
	}
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		if task.Phase != "" && !phaseIDs[task.Phase] {
			problems = append(problems, TaskProblem{task.ID, fmt.Sprintf("phase %q is not a phase of this Plan", task.Phase)})
		}
		if checkpoints[task.Phase] {
			problems = append(problems, TaskProblem{task.ID, fmt.Sprintf("phase %q is a checkpoint, which holds no tasks: put the task in a phase before or after it", task.Phase)})
		}
		seen := map[string]bool{}
		for _, dependency := range task.DependsOn {
			switch {
			case dependency == task.ID:
				problems = append(problems, TaskProblem{task.ID, "a task cannot depend on itself"})
			case !known[dependency]:
				problems = append(problems, TaskProblem{task.ID, fmt.Sprintf("it depends on %q, which is not a task of this Plan", dependency)})
			case seen[dependency]:
				problems = append(problems, TaskProblem{task.ID, fmt.Sprintf("it depends on %q more than once", dependency)})
			default:
				dependencies[task.ID] = append(dependencies[task.ID], dependency)
				// A phase's Tasks can all run in parallel, and the next phase
				// follows, so a Task can only need Tasks of an earlier phase.
				if needed := taskPhase[dependency]; phaseIDs[task.Phase] && phaseIDs[needed] && phaseOrder[needed] >= phaseOrder[task.Phase] {
					problems = append(problems, TaskProblem{task.ID, fmt.Sprintf("it depends on %q, which is in phase %q: a task can only depend on tasks of an earlier phase, because a phase's tasks run in parallel", dependency, needed)})
				}
			}
			seen[dependency] = true
		}
	}
	problems = append(problems, taskCycleProblems(tasks, dependencies)...)

	// Every Task with a story leads to one. A Task it depends on unblocks it, so
	// that Task leads there too. Walking back from the stories reaches every Task
	// that leads to one, visiting each Task and dependency once.
	reached := make(map[string]bool, len(tasks))
	queue := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.Story != "" && !reached[task.ID] {
			reached[task.ID] = true
			queue = append(queue, task.ID)
		}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependency := range dependencies[current] {
			if !reached[dependency] {
				reached[dependency] = true
				queue = append(queue, dependency)
			}
		}
	}
	for _, task := range tasks {
		if !reached[task.ID] {
			problems = append(problems, TaskProblem{task.ID, "it leads to no user story: it needs --story, or to unblock a task that leads to one"})
		}
	}
	return problems
}

// taskCycleProblems reports each dependency cycle once, at the Task that closes it.
func taskCycleProblems(tasks []TaskManifest, dependencies map[string][]string) []TaskProblem {
	problems := []TaskProblem{}
	const (
		visiting = 1
		finished = 2
	)
	marks := make(map[string]int, len(tasks))
	stack := []string{}
	var visit func(string)
	visit = func(id string) {
		marks[id] = visiting
		stack = append(stack, id)
		for _, next := range dependencies[id] {
			switch marks[next] {
			case visiting:
				start := 0
				for index, value := range stack {
					if value == next {
						start = index
					}
				}
				cycle := append(append([]string{}, stack[start:]...), next)
				problems = append(problems, TaskProblem{id, "dependency cycle: " + strings.Join(cycle, " -> ")})
			case 0:
				visit(next)
			}
		}
		stack = stack[:len(stack)-1]
		marks[id] = finished
	}
	for _, task := range tasks {
		if marks[task.ID] == 0 {
			visit(task.ID)
		}
	}
	return problems
}

// TaskUnblocks returns what each Task unblocks: the derived inverse of
// depends_on, in Task order.
func TaskUnblocks(tasks []TaskManifest) map[string][]string {
	unblocks := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		for _, dependency := range task.DependsOn {
			unblocks[dependency] = append(unblocks[dependency], task.ID)
		}
	}
	return unblocks
}

// TasksReady returns the Tasks that can start now: not done, not in progress,
// and with every dependency done.
func TasksReady(tasks []TaskManifest) []TaskManifest {
	done := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		done[task.ID] = task.State == TaskDone
	}
	ready := []TaskManifest{}
	for _, task := range tasks {
		if task.State != TaskPlanned {
			continue
		}
		blocked := false
		for _, dependency := range task.DependsOn {
			if !done[dependency] {
				blocked = true
				break
			}
		}
		if !blocked {
			ready = append(ready, task)
		}
	}
	return ready
}

// PlanStories returns the story URNs a Plan's Tasks name, sorted. They are not
// authored on the Plan; a Task without a story reaches one of these through the
// Tasks it unblocks.
func (plan *Plan) PlanStories() []string {
	seen := map[string]bool{}
	stories := []string{}
	for _, task := range plan.Tasks {
		if task.Story != "" && !seen[task.Story] {
			seen[task.Story] = true
			stories = append(stories, task.Story)
		}
	}
	sort.Strings(stories)
	return stories
}

// loadPlans loads every Plan beneath ___plans.
func loadPlans(root string, manifest Manifest, options loadOptions, validation *Validation) ([]*Plan, error) {
	dir := filepath.Join(root, PlansDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var plans []*Plan
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		id := strings.TrimSuffix(entry.Name(), PlanSuffix)
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), PlanSuffix) || !ValidID(id) {
			addIssue(validation, "error", relativePath(root, path), "plans must be real <id>.plan directories")
			continue
		}
		plan, err := loadPlan(root, path, id, manifest, options, validation)
		if err != nil {
			return nil, err
		}
		if plan != nil {
			plans = append(plans, plan)
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].ID < plans[j].ID })
	return plans, nil
}

func loadPlan(root, dir, id string, manifest Manifest, options loadOptions, validation *Validation) (*Plan, error) {
	rel := relativePath(root, dir)
	manifestPath := filepath.Join(dir, PlanManifestName)
	var value PlanManifest
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		return nil, nil
	}
	if err := ValidatePlanManifest(value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
	}
	if value.ID != id {
		addIssue(validation, "error", relativePath(root, manifestPath), fmt.Sprintf("plan id %q must match its directory %q", value.ID, id+PlanSuffix))
	}
	plan := &Plan{PlanManifest: value, Path: rel, Directory: dir, Target: PlanTarget(manifest.ID, id)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case PlanManifestName:
			continue
		case PlanDeckDir, PlanTasksDir:
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
				addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "plan record directories must be real directories")
			}
			continue
		}
		if strings.HasPrefix(entry.Name(), ".change-saga-") {
			continue
		}
		addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown entry in a plan; a plan holds plan.json, deck, and tasks")
	}

	deckDir := filepath.Join(dir, PlanDeckDir)
	if realDirectoryExists(deckDir) {
		decks, err := loadDeckRecords(root, deckDir, planDeckTargets(manifest.ID, id), options, validation)
		if err != nil {
			return nil, err
		}
		if len(decks) != 1 {
			addIssue(validation, "error", relativePath(root, deckDir), "a plan deck bundle must contain exactly one deck record")
		} else {
			plan.Deck = decks[0]
			validateDeckRole(plan.Deck, DeckRolePlan, manifest.ID, validation)
		}
	} else {
		addIssue(validation, "error", rel, "a plan is always a slide deck; its deck directory is missing")
	}

	tasksDir := filepath.Join(dir, PlanTasksDir)
	if realDirectoryExists(tasksDir) {
		tasks, err := os.ReadDir(tasksDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range tasks {
			if strings.HasPrefix(entry.Name(), ".change-saga-") {
				continue
			}
			path := filepath.Join(tasksDir, entry.Name())
			taskID := strings.TrimSuffix(entry.Name(), TaskSuffix)
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), TaskSuffix) || !ValidID(taskID) {
				addIssue(validation, "error", relativePath(root, path), "tasks must be real <id>.task directories")
				continue
			}
			if task := loadTask(root, path, taskID, plan, manifest, validation); task != nil {
				plan.Tasks = append(plan.Tasks, task)
			}
		}
		sort.Slice(plan.Tasks, func(i, j int) bool { return plan.Tasks[i].ID < plan.Tasks[j].ID })
	}
	for _, problem := range PlanTaskProblems(plan.Phases, plan.Manifests()) {
		addIssue(validation, "error", relativePath(root, filepath.Join(tasksDir, problem.Task+TaskSuffix)), "task "+problem.Task+": "+problem.Message)
	}
	validatePlanItemRecords(plan, validation)
	return plan, nil
}

func loadTask(root, dir, id string, plan *Plan, manifest Manifest, validation *Validation) *Task {
	manifestPath := filepath.Join(dir, TaskManifestName)
	var value TaskManifest
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		return nil
	}
	if err := ValidateTaskManifest(manifest.ID, value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
	}
	if value.ID != id {
		addIssue(validation, "error", relativePath(root, manifestPath), fmt.Sprintf("task id %q must match its directory %q", value.ID, id+TaskSuffix))
	}
	task := &Task{TaskManifest: value, Path: relativePath(root, dir), Directory: dir, Target: TaskTarget(manifest.ID, plan.ID, id)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		addIssue(validation, "error", task.Path, err.Error())
		return task
	}
	for _, entry := range entries {
		switch {
		case entry.Name() == TaskManifestName, strings.HasPrefix(entry.Name(), ".change-saga-"):
		case entry.Name() == TaskEssayName && entry.Type().IsRegular():
			essay, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				addIssue(validation, "error", task.Path, err.Error())
				continue
			}
			task.Essay = string(essay)
		default:
			addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown entry in a task; a task holds task.json and essay.md")
		}
	}
	return task
}

// validatePlanItemRecords checks that a slide Item naming a Task names one of
// this Plan's. The shape of every record reference was checked with the deck.
func validatePlanItemRecords(plan *Plan, validation *Validation) {
	if plan.Deck == nil {
		return
	}
	for _, slide := range plan.Deck.Slides {
		for _, item := range slide.Items {
			if PlanIDOf(item.Record) != "" && plan.FindTask(item.Record) == nil {
				addIssue(validation, "error", item.Path, fmt.Sprintf("item names task %s, which is not a task of this plan", item.Record))
			}
		}
	}
}

// validPlanRecordReference reports whether a Plan Item's record is a story, or a
// Task beneath the Plan that owns deck.
func validPlanRecordReference(sagaID string, deck *Deck, value string) bool {
	if validRecordOfKinds(sagaID, value, []string{"story"}) {
		return true
	}
	planID := PlanIDOf(deck.Target)
	prefix := PlanTarget(sagaID, planID) + ":task:"
	id, found := strings.CutPrefix(value, prefix)
	return found && planID != "" && ValidID(id)
}
