package planactivity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	return dir
}

func tempLog(t *testing.T) *fileLog {
	t.Helper()
	return &fileLog{root: t.TempDir(), cache: map[string]cachedFile{}}
}

func TestWorktreesOfOneRepositoryShareActivity(t *testing.T) {
	t.Parallel()
	repo := newRepo(t)
	worktree := filepath.Join(t.TempDir(), "other")
	git(t, repo, "worktree", "add", "-q", "-b", "other", worktree)
	writer, reason := Open(context.Background(), worktree, "", "saga")
	if writer == nil {
		t.Fatalf("no log: %s", reason)
	}
	if err := writer.Append("plan", Event{Agent: "a", Task: "t", Kind: KindStart, Branch: Branch(context.Background(), worktree)}); err != nil {
		t.Fatal(err)
	}
	reader, _ := Open(context.Background(), repo, "", "saga")
	events, err := reader.Read("plan")
	if err != nil || len(events) != 1 || events[0].Branch != "other" {
		t.Fatalf("events = %+v, %v", events, err)
	}
	if status := git(t, repo, "status", "--porcelain"); status != "" {
		t.Fatalf("the log must not show up in Git: %q", status)
	}
	// Another Saga in the repository has its own log.
	sibling, _ := Open(context.Background(), repo, "", "other-saga")
	if events, _ := sibling.Read("plan"); len(events) != 0 {
		t.Fatalf("activity leaked across Sagas: %+v", events)
	}
}

func TestASagaOutsideGitHasNoLogUnlessItsCodeCheckoutHasOne(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	if log, reason := Open(context.Background(), outside, "", "saga"); log != nil || reason != NotInGit {
		t.Fatalf("log = %v, reason = %q", log, reason)
	}
	repo := newRepo(t)
	log, reason := Open(context.Background(), outside, repo, "saga")
	if log == nil {
		t.Fatalf("no log with a code checkout: %s", reason)
	}
	if err := log.Append("plan", Event{Agent: "a", Task: "t", Kind: KindStart}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "change-saga", "activity", "saga", "plan", "a.jsonl")); err != nil {
		t.Fatal(err)
	}
}

// TestHelperAppendProcess is run by TestConcurrentProcessesLoseNoEvent as a
// separate process.
func TestHelperAppendProcess(t *testing.T) {
	root := os.Getenv("PLANACTIVITY_APPEND_ROOT")
	if root == "" {
		t.Skip("run as a helper process")
	}
	log := &fileLog{root: root, cache: map[string]cachedFile{}}
	for index := range 200 {
		event := Event{Agent: os.Getenv("PLANACTIVITY_AGENT"), Task: "t", Kind: KindNote, Message: os.Getenv("PLANACTIVITY_ID") + ":" + strconv.Itoa(index) + strings.Repeat("x", 100)}
		if err := log.Append("plan", event); err != nil {
			t.Fatal(err)
		}
	}
}

// The shared log grows past one rotation, so appends race the rotation too; a
// second rotation would discard the oldest file on purpose.
func TestConcurrentProcessesLoseNoEvent(t *testing.T) {
	t.Parallel()
	for _, shared := range []bool{false, true} {
		root := t.TempDir()
		processes := []*exec.Cmd{}
		outputs := []*bytes.Buffer{}
		for id := range 4 {
			agent := "agent-" + strconv.Itoa(id)
			if shared {
				agent = "one-agent"
			}
			process := exec.Command(os.Args[0], "-test.run=^TestHelperAppendProcess$")
			process.Env = append(os.Environ(), "PLANACTIVITY_APPEND_ROOT="+root, "PLANACTIVITY_AGENT="+agent, "PLANACTIVITY_ID="+strconv.Itoa(id))
			output := &bytes.Buffer{}
			process.Stdout, process.Stderr = output, output
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			processes, outputs = append(processes, process), append(outputs, output)
		}
		for index, process := range processes {
			if err := process.Wait(); err != nil {
				t.Fatalf("%v\n%s", err, outputs[index])
			}
		}
		events, err := (&fileLog{root: root, cache: map[string]cachedFile{}}).Read("plan")
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, event := range events {
			seen[event.Message] = true
		}
		if len(seen) != 800 {
			t.Fatalf("shared=%t: read %d distinct events of 800", shared, len(seen))
		}
	}
}

func TestATornLineLosesOnlyItself(t *testing.T) {
	t.Parallel()
	log := tempLog(t)
	if err := log.Append("plan", Event{Agent: "a", Task: "t", Kind: KindStart}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(log.root, "plan", "a.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"at":"2026-11-02T00:00:00Z","agent":"a","ta`); err != nil {
		t.Fatal(err)
	}
	file.Close()
	if events, err := log.Read("plan"); err != nil || len(events) != 1 {
		t.Fatalf("a half-written last line must wait: %+v, %v", events, err)
	}
	if err := log.Append("plan", Event{Agent: "a", Task: "t", Kind: KindNote, Message: "after"}); err != nil {
		t.Fatal(err)
	}
	events, err := log.Read("plan")
	if err != nil || len(events) != 2 || events[1].Message != "after" {
		t.Fatalf("the event after a torn line must survive: %+v, %v", events, err)
	}
}

func TestMissingAndUnwritableDirectories(t *testing.T) {
	t.Parallel()
	log := tempLog(t)
	if events, err := log.Read("no-plan"); err != nil || len(events) != 0 {
		t.Fatalf("missing directory: %+v, %v", events, err)
	}
	blocked := tempLog(t)
	if err := os.WriteFile(filepath.Join(blocked.root, "plan"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := blocked.Append("plan", Event{Agent: "a", Task: "t", Kind: KindStart}); err == nil {
		t.Fatal("append into a file must report an error for the caller to ignore")
	}
}

func TestTheLogStaysBounded(t *testing.T) {
	t.Parallel()
	log := tempLog(t)
	message := strings.Repeat("m", MaxMessageRunes+50)
	total := 0
	for total < 4*rotateAt {
		if err := log.Append("plan", Event{Agent: "a", Task: "t", Kind: KindNote, Message: message}); err != nil {
			t.Fatal(err)
		}
		total += len(message)
	}
	entries, err := os.ReadDir(filepath.Join(log.root, "plan"))
	if err != nil {
		t.Fatal(err)
	}
	var size int64
	for _, entry := range entries {
		info, _ := entry.Info()
		size += info.Size()
	}
	if size > 2*rotateAt+4096 || len(entries) != 2 {
		t.Fatalf("%d files holding %d bytes", len(entries), size)
	}
	events, _ := log.Read("plan")
	if len(events) == 0 || len([]rune(events[0].Message)) != MaxMessageRunes {
		t.Fatalf("a message must be cut to the bound: %d events", len(events))
	}
}

func TestAFirstReportRemovesLogsNobodyHasWrittenToForAMonth(t *testing.T) {
	t.Parallel()
	log := tempLog(t)
	if err := log.Append("plan", Event{Agent: "old", Task: "t", Kind: KindStart}); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(log.root, "plan", "old.jsonl")
	longAgo := time.Now().Add(-retainFor - time.Hour)
	if err := os.Chtimes(old, longAgo, longAgo); err != nil {
		t.Fatal(err)
	}
	if err := log.Append("plan", Event{Agent: "new", Task: "t", Kind: KindStart}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old log still there: %v", err)
	}
}

func TestAgentFilesStayDistinctAndSafe(t *testing.T) {
	t.Parallel()
	slash, dash := agentFile("feat/x"), agentFile("feat-x")
	if slash == dash || strings.Contains(slash, "/") {
		t.Fatalf("%q and %q", slash, dash)
	}
	if name := agentFile("../../../etc/passwd"); strings.Contains(name, "/") || strings.HasPrefix(name, ".") {
		t.Fatalf("unsafe file name %q", name)
	}
	if name := agentFile(strings.Repeat("a", 300)); len(name) > 100 {
		t.Fatalf("long file name %q", name)
	}
}

func TestIdentity(t *testing.T) {
	t.Parallel()
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	devswarm := env(map[string]string{"DEVSWARM_BUILDER_NAME": "live-08fb259f", "DEVSWARM_REPO_ID": "08fb259f-ceb1-4a3d"})
	for _, test := range []struct {
		override string
		getenv   func(string) string
		branch   string
		want     string
	}{
		{"me", devswarm, "branch", "me"},
		{"", devswarm, "branch", "live"},
		{"", env(map[string]string{"DEVSWARM_BUILDER_NAME": "live-08fb259f"}), "branch", "live-08fb259f"},
		{"", env(nil), "branch", "branch"},
		{"", env(nil), "", "unknown"},
	} {
		if got := Identity(test.override, test.getenv, test.branch); got != test.want {
			t.Errorf("Identity(%q, %q) = %q, want %q", test.override, test.branch, got, test.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	event := func(minute int, agent, kind string) Event {
		return Event{At: at.Add(time.Duration(minute) * time.Minute), Agent: agent, Task: "t", Kind: kind, Message: fmt.Sprint(minute), Branch: agent + "-branch"}
	}
	for _, test := range []struct {
		name    string
		events  []Event
		state   State
		holders []string
	}{
		{"started", []Event{event(0, "a", KindStart)}, StateWorking, []string{"a"}},
		{"a note holds without a start", []Event{event(0, "a", KindNote)}, StateWorking, []string{"a"}},
		{"blocked", []Event{event(0, "a", KindStart), event(1, "a", KindBlocked)}, StateBlocked, []string{"a"}},
		{"a note clears blocked", []Event{event(0, "a", KindBlocked), event(1, "a", KindNote)}, StateWorking, []string{"a"}},
		{"two agents, latest first", []Event{event(0, "a", KindStart), event(1, "b", KindStart)}, StateWorking, []string{"b", "a"}},
		{"done", []Event{event(0, "a", KindStart), event(1, "a", KindDone)}, StateFinished, []string{}},
		{"done ends an older holder", []Event{event(0, "a", KindStart), event(1, "b", KindDone)}, StateFinished, []string{}},
		{"started again after done", []Event{event(0, "a", KindDone), event(1, "b", KindStart)}, StateWorking, []string{"b"}},
		{"released", []Event{event(0, "a", KindStart), event(1, "a", KindReleased)}, StateNone, []string{}},
		{"released after done", []Event{event(0, "a", KindDone), event(1, "a", KindReleased)}, StateNone, []string{}},
	} {
		activity := Summarize(test.events)["t"]
		if activity.State() != test.state {
			t.Errorf("%s: state = %q, want %q", test.name, activity.State(), test.state)
		}
		if strings.Join(activity.Agents(), ",") != strings.Join(test.holders, ",") {
			t.Errorf("%s: holders = %v, want %v", test.name, activity.Agents(), test.holders)
		}
	}
	activity := Summarize([]Event{event(0, "a", KindStart)})["t"]
	if activity.Quiet(at.Add(QuietAfter)) || !activity.Quiet(at.Add(QuietAfter+time.Second)) {
		t.Fatal("a held Task reads quiet only after QuietAfter")
	}
	var nothing *TaskActivity
	if nothing.State() != StateNone || nothing.Quiet(at) || nothing.Agents() != nil {
		t.Fatal("no activity must read as nothing")
	}
}

func TestEventsRoundTripAsCompactLines(t *testing.T) {
	t.Parallel()
	line, _ := json.Marshal(Event{At: time.Unix(0, 0).UTC(), Agent: "a", Task: "t", Kind: KindStart})
	if strings.Contains(string(line), "message") || strings.Contains(string(line), "branch") {
		t.Fatalf("empty fields must be left out: %s", line)
	}
}

func TestTheStampMovesWithTheLog(t *testing.T) {
	t.Parallel()
	log := tempLog(t)
	if log.Stamp("plan") != "" {
		t.Fatal("an absent log has no stamp")
	}
	if err := log.Append("plan", Event{Agent: "a", Task: "t", Kind: KindStart}); err != nil {
		t.Fatal(err)
	}
	first := log.Stamp("plan")
	if first == "" || log.Stamp("plan") != first {
		t.Fatalf("stamp %q must hold while nothing changes", first)
	}
	if err := log.Append("plan", Event{Agent: "b", Task: "t", Kind: KindNote}); err != nil {
		t.Fatal(err)
	}
	if log.Stamp("plan") == first {
		t.Fatal("a new event must move the stamp")
	}
}
