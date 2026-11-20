package saga

import (
	"strings"
	"testing"
	"time"
)

func checkpointPhases() []PlanPhase {
	return []PlanPhase{
		{ID: "one", Title: "One"}, {ID: "two", Title: "Two"},
		{ID: "cp-1", Title: "First checkpoint", Checkpoint: &PlanCheckpoint{}},
		{ID: "three", Title: "Three"},
		{ID: "cp-2", Title: "Second checkpoint", Checkpoint: &PlanCheckpoint{}},
		{ID: "four", Title: "Four"},
	}
}

func checkpointTasks() []TaskManifest {
	return []TaskManifest{{ID: "a", Phase: "one"}, {ID: "b", Phase: "two"}, {ID: "c", Phase: "three"}, {ID: "d", Phase: "four"}, {ID: "loose"}}
}

func taskIDs(tasks []TaskManifest) string {
	ids := []string{}
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return strings.Join(ids, ",")
}

func TestACheckpointCoversTheTasksSinceThePreviousOne(t *testing.T) {
	t.Parallel()
	phases, tasks := checkpointPhases(), checkpointTasks()
	for checkpoint, want := range map[string]string{"cp-1": "a,b", "cp-2": "c", "four": "", "absent": ""} {
		if got := taskIDs(CheckpointScope(phases, tasks, checkpoint)); got != want {
			t.Errorf("scope of %s = %q, want %q", checkpoint, got, want)
		}
	}
}

func TestTheFirstUnpassedCheckpointHoldsEverythingAfterIt(t *testing.T) {
	t.Parallel()
	phases, tasks := checkpointPhases(), checkpointTasks()
	holds := CheckpointHolds(phases, tasks)
	if len(holds) != 2 || holds["c"] != "cp-1" || holds["d"] != "cp-1" {
		t.Fatalf("holds = %v", holds)
	}
	phases[2].Checkpoint.Passed = &CheckpointPass{At: time.Now(), By: "trevor", Reason: "works"}
	if holds := CheckpointHolds(phases, tasks); len(holds) != 1 || holds["d"] != "cp-2" {
		t.Fatalf("after the first passes, holds = %v", holds)
	}
	if holds := CheckpointHolds([]PlanPhase{{ID: "one", Title: "One"}}, tasks); len(holds) != 0 {
		t.Fatalf("a Plan with no checkpoints holds nothing: %v", holds)
	}
}

func TestACheckpointHoldsNoTasks(t *testing.T) {
	t.Parallel()
	problems := PlanTaskProblems(checkpointPhases(), []TaskManifest{{ID: "x", Phase: "cp-1", Story: "urn:change-saga:s:story:x"}})
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "checkpoint") {
		t.Fatalf("problems = %+v", problems)
	}
}

func TestACheckpointsRecordIsValidated(t *testing.T) {
	t.Parallel()
	manifest := PlanManifest{Schema: PlanSchemaURL, Version: 5, ID: "p", Title: "P", CreatedAt: time.Now(), Phases: checkpointPhases()}
	if err := ValidatePlanManifest(manifest); err != nil {
		t.Fatal(err)
	}
	manifest.CheckpointPasser = "anyone"
	if err := ValidatePlanManifest(manifest); err == nil {
		t.Fatal("an unknown passer must be refused")
	}
	manifest.CheckpointPasser = CheckpointPasserAgent
	manifest.Phases[2].Checkpoint.Passed = &CheckpointPass{At: time.Now(), By: "agent-1"}
	if err := ValidatePlanManifest(manifest); err == nil || !strings.Contains(err.Error(), "why") {
		t.Fatalf("a pass with no reason must be refused: %v", err)
	}
}
