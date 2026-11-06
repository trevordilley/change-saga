package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// activityRepo is a Saga committed to a Git repository, with one Plan of two
// Tasks, the second unblocking the first, and a second worktree on a branch.
func activityRepo(t *testing.T) (root, otherRoot string) {
	t.Helper()
	fixture := newPlanFixture(t)
	manifest, err := saga.ReadManifest(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	run(t, Task, "add", "--plan", "first", "--id", "read", "--title", "Read", "--story", "urn:change-saga:"+manifest.ID+":story:first-story", fixture.root)
	run(t, Task, "add", "--plan", "first", "--id", "source", "--title", "Source", "--unblocks", "read", fixture.root)
	git(t, fixture.repo, "add", "-A")
	git(t, fixture.repo, "commit", "-qm", "the plan")
	worktree := filepath.Join(t.TempDir(), "other")
	git(t, fixture.repo, "worktree", "add", "-q", "-b", "feat/other", worktree)
	return fixture.root, filepath.Join(worktree, filepath.Base(fixture.root))
}

func taskOutput(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := Task(context.Background(), args, &out); err != nil {
		t.Fatalf("task %v: %v", args, err)
	}
	return out.String()
}

func gitStatus(t *testing.T, root string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", root, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return string(output)
}

func TestStartAndNoteAreSeenFromAnotherWorktreeAndWriteNothing(t *testing.T) {
	root, other := activityRepo(t)
	if out := taskOutput(t, "start", "--plan", "first", "--task", "source", "--agent", "ada", other); !strings.Contains(out, "Started source as ada") {
		t.Fatalf("start said %q", out)
	}
	taskOutput(t, "note", "--plan", "first", "--task", "source", "--agent", "ada", "tide tables moved", other)
	if status := gitStatus(t, other); status != "" {
		t.Fatalf("activity must not touch the checkout: %q", status)
	}

	// Another agent is told, and it is recorded anyway.
	out := taskOutput(t, "start", "--plan", "first", "--task", "source", "--agent", "ben", root)
	if !strings.Contains(out, "Also on it: ada") {
		t.Fatalf("start must warn who else is on it: %q", out)
	}
	task := queryData(t, "task", "--saga", root, "--plan", "first", "--task", "source")["task"].(map[string]any)
	activity := task["activity"].(map[string]any)
	if activity["state"] != "working" || !strings.Contains(strings.Join(anyStrings(activity["agents"]), ","), "ada") {
		t.Fatalf("activity = %+v", activity)
	}
	if _, found := task["activity_history"]; found {
		t.Fatal("history must be an explicit request")
	}
	if task["state"] != saga.TaskPlanned {
		t.Fatalf("the durable state must not change: %v", task["state"])
	}
	task = queryData(t, "task", "--saga", root, "--plan", "first", "--task", "source", "--history")["task"].(map[string]any)
	if history := task["activity_history"].([]any); len(history) != 3 {
		t.Fatalf("history = %+v", history)
	}
}

func TestBlockedAndADoneOnABranchReadBackDistinctly(t *testing.T) {
	root, other := activityRepo(t)
	taskOutput(t, "note", "--plan", "first", "--task", "source", "--blocked", "--agent", "ada", "no tide data", other)
	rows := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	row := findRow(t, rows, "source")
	if row["activity"].(map[string]any)["state"] != "blocked" || row["activity"].(map[string]any)["message"] != "no tide data" {
		t.Fatalf("row = %+v", row)
	}
	// Done in the branch's checkout is durable there and only activity here.
	taskOutput(t, "done", "--plan", "first", "--task", "source", "--summary", "found it", other)
	rows = queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	row = findRow(t, rows, "source")
	activity := row["activity"].(map[string]any)
	if activity["state"] != "done_on_branch" || activity["branch"] != "feat/other" || row["state"] != saga.TaskPlanned {
		t.Fatalf("row = %+v", row)
	}
	rows = queryData(t, "plan", "--saga", other, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	row = findRow(t, rows, "source")
	if row["state"] != saga.TaskDone || row["activity"] != nil {
		t.Fatalf("durably done reads as done: %+v", row)
	}
}

func TestReadyListSaysWhoStartedATask(t *testing.T) {
	root, other := activityRepo(t)
	taskOutput(t, "start", "--plan", "first", "--task", "source", "--agent", "ada", other)
	out := taskOutput(t, "list", "--plan", "first", "--ready", root)
	if !strings.Contains(out, "source: Source (started by ada") {
		t.Fatalf("task list --ready said %q", out)
	}
}

func TestStateChangesReportActivity(t *testing.T) {
	root, other := activityRepo(t)
	taskOutput(t, "set", "--plan", "first", "--task", "source", "--state", "in_progress", "--agent", "ada", other)
	rows := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	if activity := findRow(t, rows, "source")["activity"].(map[string]any); activity["state"] != "working" {
		t.Fatalf("activity = %+v", activity)
	}
	taskOutput(t, "set", "--plan", "first", "--task", "source", "--state", "planned", "--agent", "ada", other)
	rows = queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	if activity := findRow(t, rows, "source")["activity"]; activity != nil {
		t.Fatalf("a released Task has no live state: %+v", activity)
	}
}

func TestActivityRefusesWhatIsNotThere(t *testing.T) {
	root, _ := activityRepo(t)
	var out bytes.Buffer
	if err := Task(context.Background(), []string{"start", "--plan", "first", "--task", "nope", root}, &out); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v", err)
	}
	if err := Task(context.Background(), []string{"note", "--plan", "nope", "--task", "read", "words", root}, &out); err == nil {
		t.Fatal("a note on a missing Plan must be refused")
	}
	if err := Task(context.Background(), []string{"start", "--plan", "../../elsewhere", "--task", "read", root}, &out); err == nil {
		t.Fatal("a Plan id must not reach outside the log")
	}
	if err := Task(context.Background(), []string{"note", "--plan", "first", "--task", "read", root}, &out); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("a note without words must show usage: %v", err)
	}
}

func TestOutsideGitActivitySaysSoAndNothingFails(t *testing.T) {
	committed, _ := activityRepo(t)
	root := filepath.Join(t.TempDir(), "app.saga")
	if err := os.CopyFS(root, os.DirFS(committed)); err != nil {
		t.Fatal(err)
	}
	out := taskOutput(t, "start", "--plan", "first", "--task", "read", root)
	if !strings.Contains(out, "not in a Git repository") || !strings.Contains(out, "Nothing was recorded") {
		t.Fatalf("start said %q", out)
	}
	out = taskOutput(t, "note", "--plan", "first", "--task", "read", "--json", "words", root)
	var result taskActivityResult
	if err := json.Unmarshal([]byte(out), &result); err != nil || !result.OK || result.Recorded || result.Reason == "" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	taskOutput(t, "done", "--plan", "first", "--task", "read", root)
	plan := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)
	if !strings.Contains(plan["activity_note"].(string), "not in a Git repository") {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestAnUnwritableLogFailsNothing(t *testing.T) {
	root, _ := activityRepo(t)
	common := filepath.Join(filepath.Dir(root), ".git", "change-saga")
	if err := os.WriteFile(common, []byte("in the way"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := taskOutput(t, "start", "--plan", "first", "--task", "read", root)
	if !strings.Contains(out, "Could not write the activity log") {
		t.Fatalf("start said %q", out)
	}
	taskOutput(t, "done", "--plan", "first", "--task", "read", root)
	if rows := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any); findRow(t, rows, "read")["state"] != saga.TaskDone {
		t.Fatal("task done must still write the durable state")
	}
}

func TestQueryActivityStaysCompact(t *testing.T) {
	root, other := activityRepo(t)
	taskOutput(t, "note", "--plan", "first", "--task", "source", "--agent", "ada", strings.Repeat("long ", 400), other)
	rows := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	message := findRow(t, rows, "source")["activity"].(map[string]any)["message"].(string)
	if len([]rune(message)) > activityMessageRunes {
		t.Fatalf("message of %d runes", len([]rune(message)))
	}
	if row := findRow(t, rows, "read"); row["activity"] != nil {
		t.Fatalf("a Task nobody reported on carries nothing: %+v", row)
	}
}

func findRow(t *testing.T, rows []any, id string) map[string]any {
	t.Helper()
	for _, row := range rows {
		if value := row.(map[string]any); value["id"] == id {
			return value
		}
	}
	t.Fatalf("no row %s in %+v", id, rows)
	return nil
}

func anyStrings(value any) []string {
	values := []string{}
	for _, item := range value.([]any) {
		values = append(values, item.(string))
	}
	return values
}

// Activity is a view: what a Saga reports is the same whatever the log says.
func TestActivityChangesNoReport(t *testing.T) {
	root, other := activityRepo(t)
	report := func() string {
		var out bytes.Buffer
		if err := Status(context.Background(), []string{"--json", "--repo", filepath.Dir(root), "--allow-repository-mismatch", root}, &out); err != nil {
			t.Fatalf("status: %v\n%s", err, out.String())
		}
		if err := Validate(context.Background(), []string{root}, &out); err != nil {
			t.Fatalf("validate: %v\n%s", err, out.String())
		}
		// Check reports gaps as an error, which is part of what must not change.
		var checked bytes.Buffer
		checkErr := Check(context.Background(), []string{"--covers", "health", "--repo", filepath.Dir(root), "--allow-repository-mismatch", root}, &checked)
		return fmt.Sprintf("%s%s%v", out.String(), checked.String(), checkErr)
	}
	before := report()
	if !strings.Contains(before, `"stale_references"`) {
		t.Fatalf("status must report something to compare: %.300s", before)
	}
	taskOutput(t, "start", "--plan", "first", "--task", "source", "--agent", "ada", other)
	taskOutput(t, "note", "--plan", "first", "--task", "read", "--blocked", "--agent", "ada", "stuck", other)
	taskOutput(t, "done", "--plan", "first", "--task", "source", "--agent", "ben", other)
	if after := report(); after != before {
		beforeLines, afterLines := strings.Split(before, "\n"), strings.Split(after, "\n")
		for index := range min(len(beforeLines), len(afterLines)) {
			if beforeLines[index] != afterLines[index] {
				t.Fatalf("activity changed a report at line %d:\nbefore %s\nafter  %s", index, beforeLines[index], afterLines[index])
			}
		}
		t.Fatalf("activity changed a report's length: %d lines, then %d", len(beforeLines), len(afterLines))
	}
	// Done on a branch is not done here, so delivery still waits for it.
	message := refused(t, planCommand, "deliver", "--plan", "first", root)
	if !strings.Contains(message, "source") && !strings.Contains(message, "done") {
		t.Fatalf("plan deliver said %q", message)
	}
}

// Work after an unpassed checkpoint is not offered, and starting it anyway warns.
func TestACheckpointHoldsTheWorkAfterIt(t *testing.T) {
	root, _ := activityRepo(t)
	run(t, Plan, "add-phase", "--plan", "first", "--id", "early", "--title", "Early", root)
	run(t, Plan, "add-phase", "--plan", "first", "--id", "proven", "--title", "Proven", "--checkpoint", root)
	run(t, Plan, "add-phase", "--plan", "first", "--id", "late", "--title", "Late", root)
	run(t, Task, "set", "--plan", "first", "--task", "source", "--phase", "early", root)
	run(t, Task, "set", "--plan", "first", "--task", "read", "--phase", "late", root)
	run(t, Task, "done", "--plan", "first", "--task", "source", root)
	out := taskOutput(t, "list", "--plan", "first", "--ready", root)
	if strings.Contains(out, "read:") || !strings.Contains(out, "1 more task is held by checkpoint proven (Proven)") {
		t.Fatalf("task list --ready said %q", out)
	}
	rows := queryData(t, "plan", "--saga", root, "--plan", "first")["plan"].(map[string]any)["tasks"].([]any)
	if row := findRow(t, rows, "read"); row["held_by"] != "Proven" || row["can_start"] != false {
		t.Fatalf("row = %+v", row)
	}
	if out := taskOutput(t, "start", "--plan", "first", "--task", "read", "--agent", "ada", root); !strings.Contains(out, "Warning: checkpoint Proven holds this Task") || !strings.Contains(out, "Started read") {
		t.Fatalf("task start said %q", out)
	}
}
