package changeview

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func moveGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = repo
	command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.test")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func moveWrite(t *testing.T, repo, name, body string) {
	t.Helper()
	target := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func moveCommit(t *testing.T, repo, subject string) string {
	t.Helper()
	moveGit(t, repo, "add", "-A")
	moveGit(t, repo, "commit", "-q", "-m", subject)
	return moveGit(t, repo, "rev-parse", "HEAD")
}

// A repository whose Saga was renamed from app.saga to change.saga, after an
// unrelated Saga once held the name change.saga and was removed.
func movedSagaRepository(t *testing.T) (repo string, commits map[string]string) {
	t.Helper()
	repo = t.TempDir()
	moveGit(t, repo, "init", "-q", "-b", "main")
	commits = map[string]string{}
	moveWrite(t, repo, "change.saga/saga.json", `{"id":"old-per-change-saga"}`+"\n")
	commits["unrelated"] = moveCommit(t, repo, "An unrelated Saga")
	moveGit(t, repo, "rm", "-q", "-r", "change.saga")
	commits["removed"] = moveCommit(t, repo, "Remove the unrelated Saga")
	moveWrite(t, repo, "app.saga/saga.json", `{"id":"shop","title":"Shop"}`+"\n")
	moveWrite(t, repo, "app.saga/story.json", "first\n")
	moveWrite(t, repo, "app.saga/edited.json", "before the move\n")
	commits["added"] = moveCommit(t, repo, "Add the stories")
	moveWrite(t, repo, "app.saga/story.json", "second\n")
	commits["revised"] = moveCommit(t, repo, "Revise the story")
	moveGit(t, repo, "mv", "app.saga", "change.saga")
	moveWrite(t, repo, "change.saga/edited.json", "rewritten while it moved\n")
	commits["moved"] = moveCommit(t, repo, "Move the Saga")
	moveWrite(t, repo, "change.saga/story.json", "third\n")
	commits["after"] = moveCommit(t, repo, "Revise the story again")
	return repo, commits
}

func subjects(commits []commitInfo) []string {
	names := []string{}
	for _, commit := range commits {
		names = append(names, commit.Subject)
	}
	return names
}

// A record's history continues through the move that carried its file
// unchanged, and the move itself is not one of its changes.
func TestRecordHistoryFollowsTheSagaThroughAMove(t *testing.T) {
	t.Parallel()
	repo, _ := movedSagaRepository(t)
	location := Location{Repo: repo, Path: "change.saga"}
	commits, err := recordCommits(context.Background(), location, []string{"story.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(commits), " | "); got != "Add the stories | Revise the story | Revise the story again" {
		t.Fatalf("history across the move = %s", got)
	}
	// A file the move also rewrote has a different content history; it
	// starts at the move rather than being joined to its old path.
	commits, err = recordCommits(context.Background(), location, []string{"edited.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(commits), " | "); got != "Move the Saga" {
		t.Fatalf("history of a file edited while it moved = %s", got)
	}
}

// Reading the Saga at a commit before its move reads it where it was then,
// and never an unrelated Saga that once had today's name.
func TestSagaIsReadWhereItWasAtEachCommit(t *testing.T) {
	t.Parallel()
	repo, commits := movedSagaRepository(t)
	location := Location{Repo: repo, Path: "change.saga"}
	for name, want := range map[string]string{"unrelated": "app.saga", "revised": "app.saga", "moved": "change.saga", "after": "change.saga"} {
		if got := sagaPathAt(context.Background(), location, commits[name]); got != want {
			t.Errorf("Saga path at %s = %s, want %s", name, got, want)
		}
	}
	root, err := extract(context.Background(), location, commits["revised"], t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "story.json")); err != nil || string(data) != "second\n" {
		t.Fatalf("the Saga before its move = %q, %v", data, err)
	}
	if _, err := extract(context.Background(), location, commits["unrelated"], t.TempDir()); !errors.Is(err, errAbsent) {
		t.Fatalf("a commit before the Saga existed read another Saga: %v", err)
	}
}
