// Package plancheckpoint works out what a Plan's checkpoint covers: its Tasks, the
// stories and criteria they address and the evidence behind each, how big it is to
// explain, and what would make passing it surprising. It reports; whoever passes the
// checkpoint judges.
package plancheckpoint

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/planactivity"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// SlideBudget is how many slides a checkpoint's review can sanely hold. Ten to
// twenty means the checkpoint should have been two.
const SlideBudget = 6

// Story is a story a checkpoint's Tasks lead to.
type Story struct {
	Target   string      `json:"target"`
	Title    string      `json:"title"`
	Criteria []Criterion `json:"criteria"`
}

// Criterion is one acceptance criterion and the tests that check it.
type Criterion struct {
	Target    string `json:"target"`
	Statement string `json:"statement"`
	Tests     []Test `json:"tests"`
}

// Test is a test case that checks a criterion, and how much evidence it has.
type Test struct {
	Target   string `json:"target"`
	Title    string `json:"title"`
	Evidence int    `json:"evidence"`
}

// Proven says some test with evidence checks the criterion.
func (criterion Criterion) Proven() bool {
	for _, test := range criterion.Tests {
		if test.Evidence > 0 {
			return true
		}
	}
	return false
}

// Report is what a checkpoint covers and where it stands.
type Report struct {
	Plan       string               `json:"plan"`
	Checkpoint string               `json:"checkpoint"`
	Title      string               `json:"title"`
	State      string               `json:"state"`
	Passed     *saga.CheckpointPass `json:"passed,omitempty"`
	Passer     string               `json:"passer"`
	Phases     []string             `json:"phases"`
	Tasks      int                  `json:"tasks"`
	TasksDone  int                  `json:"tasks_done"`
	Unfinished []string             `json:"unfinished,omitempty"`
	Stories    []Story              `json:"stories"`
	Estimate   int                  `json:"estimated_slides"`
	Review     string               `json:"review,omitempty"`
	Slides     int                  `json:"slides,omitempty"`
	Surprises  []string             `json:"surprises"`
	Next       string               `json:"next"`
	// Scope is the Tasks the checkpoint covers.
	Scope []saga.TaskManifest `json:"-"`
}

// Stories reads every story with its current criteria and the tests that check
// each, keyed by story URN. A part that cannot be read is simply left out.
func Stories(root, sagaID string) map[string]Story {
	stories := map[string]Story{}
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return stories
	}
	tests := map[string][]Test{}
	if records, err := quality.Load(root); err == nil {
		for _, testCase := range records.TestCases {
			if len(testCase.Revisions) == 0 {
				continue
			}
			revision := testCase.Revisions[len(testCase.Revisions)-1]
			target := "urn:change-saga:" + sagaID + ":test-case:" + testCase.Identity.ID
			for _, step := range revision.Steps {
				evidence := 0
				for _, record := range testCase.Evidence {
					if record.Role == quality.EvidenceTestImplementation && (len(record.Steps) == 0 || slices.Contains(record.Steps, step.ID)) {
						evidence++
					}
				}
				for _, criterion := range step.Criteria {
					tests[criterion] = append(tests[criterion], Test{Target: target, Title: revision.Title, Evidence: evidence})
				}
			}
		}
	}
	for _, record := range document.Stories {
		urn := "urn:change-saga:" + sagaID + ":story:" + record.Identity.ID
		story := Story{Target: urn, Title: record.Identity.ID, Criteria: []Criterion{}}
		if revision := record.CurrentRevision; revision != nil {
			if revision.Title != "" {
				story.Title = revision.Title
			}
			for _, criterion := range revision.AcceptanceCriteria {
				target := urn + ":criterion:" + criterion.ID
				story.Criteria = append(story.Criteria, Criterion{Target: target, Statement: criterion.Statement, Tests: append([]Test{}, tests[target]...)})
			}
		}
		stories[urn] = story
	}
	return stories
}

// storiesOf are the stories the scope's Tasks lead to: each Task's own, or for a
// Task without one, those of the Tasks it unblocks.
func storiesOf(plan *saga.Plan, scope []saga.TaskManifest, index map[string]Story) []Story {
	manifests := plan.Manifests()
	unblocks := saga.TaskUnblocks(manifests)
	byID := map[string]saga.TaskManifest{}
	for _, task := range manifests {
		byID[task.ID] = task
	}
	found := []string{}
	for _, task := range scope {
		seen := map[string]bool{task.ID: true}
		queue := []saga.TaskManifest{task}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if current.Story != "" {
				if !slices.Contains(found, current.Story) {
					found = append(found, current.Story)
				}
				continue
			}
			for _, id := range unblocks[current.ID] {
				if next, known := byID[id]; known && !seen[id] {
					seen[id] = true
					queue = append(queue, next)
				}
			}
		}
	}
	stories := make([]Story, 0, len(found))
	for _, urn := range found {
		story, known := index[urn]
		if !known {
			story = Story{Target: urn, Title: urn, Criteria: []Criterion{}}
		}
		stories = append(stories, story)
	}
	return stories
}

// Build reports one checkpoint. stories comes from Stories, and activity is the
// Plan's live activity, which may be empty.
func Build(document *saga.Saga, plan *saga.Plan, checkpointID string, stories map[string]Story, activity map[string]*planactivity.TaskActivity) (*Report, error) {
	index := slices.IndexFunc(plan.Phases, func(phase saga.PlanPhase) bool { return phase.ID == checkpointID })
	if index < 0 || !plan.Phases[index].IsCheckpoint() {
		return nil, fmt.Errorf("%q is not a checkpoint of plan %q", checkpointID, plan.ID)
	}
	phase := plan.Phases[index]
	passer := plan.CheckpointPasser
	if passer == "" {
		passer = saga.CheckpointPasserPerson
	}
	report := &Report{Plan: plan.Target, Checkpoint: phase.ID, Title: phase.Title, Passed: phase.Checkpoint.Passed, Passer: passer, Review: phase.Checkpoint.Review, Phases: []string{}, Surprises: []string{}}
	for earlier := index - 1; earlier >= 0 && !plan.Phases[earlier].IsCheckpoint(); earlier-- {
		report.Phases = append([]string{plan.Phases[earlier].ID}, report.Phases...)
	}
	report.Scope = saga.CheckpointScope(plan.Phases, plan.Manifests(), checkpointID)
	report.Tasks = len(report.Scope)
	for _, task := range report.Scope {
		if task.State == saga.TaskDone {
			report.TasksDone++
		} else {
			report.Unfinished = append(report.Unfinished, task.ID)
		}
	}
	report.Stories = storiesOf(plan, report.Scope, stories)
	report.Estimate = max(1, len(report.Stories))

	// The stretch began when the previous checkpoint passed.
	var began time.Time
	for earlier := index - 1; earlier >= 0; earlier-- {
		if plan.Phases[earlier].CheckpointPassed() {
			began = plan.Phases[earlier].Checkpoint.Passed.At
			break
		}
	}
	added, blocked, unproven := []string{}, []string{}, []string{}
	for _, task := range report.Scope {
		if !began.IsZero() && task.CreatedAt.After(began) {
			added = append(added, task.ID)
		}
		if taskActivity := activity[task.ID]; taskActivity != nil {
			for _, event := range taskActivity.Events {
				if event.Kind == planactivity.KindBlocked && event.At.After(began) {
					blocked = append(blocked, task.ID)
					break
				}
			}
		}
	}
	for _, story := range report.Stories {
		for _, criterion := range story.Criteria {
			if !criterion.Proven() {
				unproven = append(unproven, criterion.Target[strings.LastIndex(criterion.Target, ":")+1:])
			}
		}
	}
	if len(added) > 0 {
		report.Surprises = append(report.Surprises, "Tasks added since the stretch began: "+strings.Join(added, ", "))
	}
	if len(blocked) > 0 {
		report.Surprises = append(report.Surprises, "an agent reported blocked on: "+strings.Join(blocked, ", "))
	}
	if len(unproven) > 0 {
		report.Surprises = append(report.Surprises, "criteria it addresses with no test evidence: "+strings.Join(unproven, ", "))
	}
	if review := document.FindReview(report.Review); review != nil && review.Deck != nil {
		report.Slides = len(review.Deck.Slides)
		inScope := map[string]bool{}
		for _, task := range report.Scope {
			inScope[saga.TaskTarget(document.Manifest.ID, plan.ID, task.ID)] = true
		}
		departures := []string{}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				if item.Kind == "callout" && inScope[item.Record] {
					departures = append(departures, item.Label)
				}
			}
		}
		if len(departures) > 0 {
			report.Surprises = append(report.Surprises, "the review says the work departed from the Plan: "+strings.Join(departures, "; "))
		}
		if report.Slides > max(report.Estimate, SlideBudget) {
			report.Surprises = append(report.Surprises, fmt.Sprintf("the review took %d slides, more than the %d estimated", report.Slides, report.Estimate))
		}
	}
	switch {
	case report.Passed != nil:
		report.State = "passed"
		report.Next = "Nothing: it passed. Work after it may start."
	case len(report.Unfinished) > 0:
		report.State = "in progress"
		report.Next = "Finish its Tasks: " + strings.Join(report.Unfinished, ", ") + "."
	case report.Review == "":
		report.State = "reached"
		report.Next = fmt.Sprintf("Prove the work works: run the tests, then create its review of the checkpoint's branch into the Plan's feature branch with change-saga review create --base FEATURE --head BRANCH, and link it with change-saga plan link-review --plan %s --review ID --checkpoint %s.", plan.ID, phase.ID)
	case report.Passer == saga.CheckpointPasserAgent && len(report.Surprises) == 0:
		report.State = "reached"
		report.Next = fmt.Sprintf("Nothing here is surprising, so the agent may pass it: change-saga plan pass --plan %s --checkpoint %s --agent --reason TEXT, then merge.", plan.ID, phase.ID)
	case report.Passer == saga.CheckpointPasserAgent:
		report.State = "reached"
		report.Next = "Stop for a person: something here is surprising. They pass it with change-saga plan pass --by NAME --reason TEXT."
	default:
		report.State = "reached"
		report.Next = fmt.Sprintf("Ask whoever invoked the Plan to pass it: change-saga plan pass --plan %s --checkpoint %s --by NAME --reason TEXT.", plan.ID, phase.ID)
	}
	return report, nil
}
