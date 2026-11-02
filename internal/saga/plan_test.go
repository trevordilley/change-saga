package saga

import (
	"fmt"
	"strings"
	"testing"
)

func task(id, story string, dependsOn ...string) TaskManifest {
	return TaskManifest{ID: id, Story: story, DependsOn: dependsOn, State: TaskPlanned}
}

const storyURN = "urn:change-saga:demo:story:a"

func problemFor(problems []TaskProblem, id string) string {
	for _, problem := range problems {
		if problem.Task == id {
			return problem.Message
		}
	}
	return ""
}

func TestPlanTaskProblemsAcceptsATaskThatNamesAStory(t *testing.T) {
	if problems := PlanTaskProblems(nil, []TaskManifest{task("a", storyURN)}); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestPlanTaskProblemsAcceptsATaskThatUnblocksItsWayToAStory(t *testing.T) {
	// c names the story; b unblocks c; a unblocks b.
	tasks := []TaskManifest{task("a", ""), task("b", "", "a"), task("c", storyURN, "b")}
	if problems := PlanTaskProblems(nil, tasks); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestPlanTaskProblemsRefusesATaskThatLeadsToNoStory(t *testing.T) {
	cases := map[string]struct {
		tasks    []TaskManifest
		stranded []string
	}{
		"alone":                         {[]TaskManifest{task("a", "")}, []string{"a"}},
		"depends on a storied task":     {[]TaskManifest{task("a", storyURN), task("b", "", "a")}, []string{"b"}},
		"a chain that never gets there": {[]TaskManifest{task("a", ""), task("b", "", "a")}, []string{"a", "b"}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			problems := PlanTaskProblems(nil, test.tasks)
			for _, id := range test.stranded {
				if !strings.Contains(problemFor(problems, id), "leads to no user story") {
					t.Errorf("task %s was not refused: %v", id, problems)
				}
			}
			if len(problems) != len(test.stranded) {
				t.Errorf("problems = %v, want only %v", problems, test.stranded)
			}
		})
	}
}

func TestPlanTaskProblemsRefusesACycleAndUnknownDependencies(t *testing.T) {
	cycle := PlanTaskProblems(nil, []TaskManifest{task("a", storyURN, "b"), task("b", "", "a")})
	if len(cycle) != 1 || !strings.Contains(cycle[0].Message, "dependency cycle: a -> b -> a") {
		t.Errorf("cycle problems = %v", cycle)
	}
	if message := problemFor(PlanTaskProblems(nil, []TaskManifest{task("a", storyURN, "a")}), "a"); !strings.Contains(message, "cannot depend on itself") {
		t.Errorf("self dependency = %q", message)
	}
	if message := problemFor(PlanTaskProblems(nil, []TaskManifest{task("a", storyURN, "gone")}), "a"); !strings.Contains(message, `"gone"`) {
		t.Errorf("unknown dependency = %q", message)
	}
}

func TestPlanTaskProblemsChecksPhases(t *testing.T) {
	withPhase := task("a", storyURN)
	withPhase.Phase = "first"
	if problems := PlanTaskProblems([]PlanPhase{{ID: "first", Title: "First"}}, []TaskManifest{withPhase}); len(problems) != 0 {
		t.Errorf("problems = %v", problems)
	}
	if message := problemFor(PlanTaskProblems(nil, []TaskManifest{withPhase}), "a"); !strings.Contains(message, `phase "first"`) {
		t.Errorf("unknown phase = %q", message)
	}
}

// A long chain and a wide fan both finish quickly: the walk is linear.
func TestPlanTaskProblemsHandlesLargeGraphs(t *testing.T) {
	tasks := []TaskManifest{task("t0", "")}
	for index := 1; index < 20000; index++ {
		tasks = append(tasks, task(fmt.Sprintf("t%d", index), "", fmt.Sprintf("t%d", index-1)))
	}
	tasks[len(tasks)-1].Story = storyURN
	if problems := PlanTaskProblems(nil, tasks); len(problems) != 0 {
		t.Fatalf("problems = %d, first %v", len(problems), problems[0])
	}
}

func TestTasksReadyAndUnblocksComeFromDependencies(t *testing.T) {
	done := task("a", "")
	done.State = TaskDone
	working := task("d", "", "a")
	working.State = TaskInProgress
	tasks := []TaskManifest{done, task("b", "", "a"), task("c", storyURN, "b", "d"), working}
	ready := TasksReady(tasks)
	if len(ready) != 1 || ready[0].ID != "b" {
		t.Errorf("ready = %v, want only b: a is done, c waits on b and d, d is underway", ready)
	}
	if got := TaskUnblocks(tasks)["a"]; len(got) != 2 || got[0] != "b" || got[1] != "d" {
		t.Errorf("a unblocks %v, want b and d", got)
	}
}

func TestPlanIDOfReadsTheURNBySegment(t *testing.T) {
	cases := map[string]string{
		"urn:change-saga:demo:plan:first":                "first",
		"urn:change-saga:demo:plan:first:task:a":         "first",
		"urn:change-saga:demo:plan:first:slide:s:item:i": "first",
		"urn:change-saga:plan:story:first":               "",
		"urn:change-saga:demo:review:first":              "",
		"not a urn":                                      "",
	}
	for target, want := range cases {
		if got := PlanIDOf(target); got != want {
			t.Errorf("PlanIDOf(%q) = %q, want %q", target, got, want)
		}
	}
}
