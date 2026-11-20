package changeview

import (
	"context"
	"errors"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
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
	// Compare bytes as written, whatever line endings the platform's Git
	// would otherwise check out.
	moveGit(t, repo, "config", "core.autocrlf", "false")
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
	// A file the move also edited, as a squash-merged rename that revised
	// records does, keeps its past and counts the move as one of its changes.
	commits, err = recordCommits(context.Background(), location, []string{"edited.json"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(commits), " | "); got != "Add the stories | Move the Saga" {
		t.Fatalf("history of a file edited while it moved = %s", got)
	}
}

// Reading the Saga at a commit before its move reads it where it was then,
// and never an unrelated Saga that once had today's name.
func TestSagaIsReadWhereItWasAtEachCommit(t *testing.T) {
	t.Parallel()
	repo, commits := movedSagaRepository(t)
	location := Location{Repo: repo, Path: "change.saga"}
	for name, want := range map[string]string{"unrelated": "", "removed": "", "added": "app.saga", "revised": "app.saga", "moved": "change.saga", "after": "change.saga"} {
		got, exists := sagaPathAt(context.Background(), location, commits[name])
		if got != want || exists != (want != "") {
			t.Errorf("Saga path at %s = %q (exists %v), want %q", name, got, exists, want)
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

// Git calls one near-identical manifest a rename of another, so a Saga
// deleted in the commit that added a new one looks like a move. It is a
// different Saga, and nothing of it is read as this one's past.
func TestAReplacedSagaIsNotFollowedAsAMove(t *testing.T) {
	t.Parallel()
	for name, oldID := range map[string]string{"different id": "pr-12", "same id": "shop"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			repo := t.TempDir()
			moveGit(t, repo, "init", "-q", "-b", "main")
			moveGit(t, repo, "config", "core.autocrlf", "false")
			moveWrite(t, repo, "pr-12.saga/saga.json", `{"id":"`+oldID+`","title":"Shop"}`+"\n")
			moveWrite(t, repo, "pr-12.saga/README.md", "Open this Saga with change-saga open.\n")
			moveWrite(t, repo, "pr-12.saga/story.json", "the old per-change story\n")
			moveWrite(t, repo, "pr-12.saga/deck.json", "the old per-change deck\n")
			old := moveCommit(t, repo, "A per-change Saga")
			moveGit(t, repo, "rm", "-q", "-r", "pr-12.saga")
			moveWrite(t, repo, "app.saga/saga.json", `{"id":"shop","title":"Shop"}`+"\n")
			moveWrite(t, repo, "app.saga/README.md", "Open this Saga with change-saga open.\n")
			moveWrite(t, repo, "app.saga/story.json", "the app's story\n")
			born := moveCommit(t, repo, "Replace it with one app Saga")
			moveGit(t, repo, "mv", "app.saga", "change.saga")
			moveCommit(t, repo, "Rename the Saga")

			location := Location{Repo: repo, Path: "change.saga"}
			if at, exists := sagaPathAt(context.Background(), location, old); exists {
				t.Fatalf("the replaced Saga was read as this one's past at %s", at)
			}
			if at, exists := sagaPathAt(context.Background(), location, born); !exists || at != "app.saga" {
				t.Fatalf("Saga path at its birth = %q (exists %v)", at, exists)
			}
			if _, err := extract(context.Background(), location, old, t.TempDir()); !errors.Is(err, errAbsent) {
				t.Fatalf("the replaced Saga was extracted: %v", err)
			}
			for _, file := range []string{"README.md", "story.json"} {
				commits, err := recordCommits(context.Background(), location, []string{file})
				if err != nil {
					t.Fatal(err)
				}
				if got := strings.Join(subjects(commits), " | "); got != "Replace it with one app Saga" {
					t.Errorf("%s history = %s", file, got)
				}
			}
		})
	}
}

// A Saga that reuses the name of one deleted long ago, with no move at all,
// starts its history at its own birth.
func TestAReusedNameDoesNotInheritAnUnrelatedSagasPast(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	moveGit(t, repo, "init", "-q", "-b", "main")
	moveGit(t, repo, "config", "core.autocrlf", "false")
	moveWrite(t, repo, "change.saga/saga.json", `{"id":"old"}`+"\n")
	moveWrite(t, repo, "change.saga/README.md", "boilerplate\n")
	old := moveCommit(t, repo, "An unrelated Saga")
	moveGit(t, repo, "rm", "-q", "-r", "change.saga")
	moveCommit(t, repo, "Remove it")
	moveWrite(t, repo, "change.saga/saga.json", `{"id":"shop"}`+"\n")
	moveWrite(t, repo, "change.saga/README.md", "boilerplate\n")
	moveCommit(t, repo, "Start the repository's Saga")
	moveWrite(t, repo, "change.saga/README.md", "boilerplate, revised\n")
	moveCommit(t, repo, "Revise its README")

	location := Location{Repo: repo, Path: "change.saga"}
	commits, err := recordCommits(context.Background(), location, []string{"README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(commits), " | "); got != "Start the repository's Saga | Revise its README" {
		t.Fatalf("history = %s", got)
	}
	if _, err := extract(context.Background(), location, old, t.TempDir()); !errors.Is(err, errAbsent) {
		t.Fatalf("the unrelated Saga with the same name was extracted: %v", err)
	}
}

func TestRecordHistoryIncludesDeletedLegacyMembersAcrossSagaMoves(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	moveGit(t, repo, "init", "-q", "-b", "main")
	moveWrite(t, repo, "app.saga/saga.json", `{"id":"shop"}`)
	moveWrite(t, repo, "app.saga/checkout.story/story.json", `{"id":"checkout"}`)
	moveWrite(t, repo, "app.saga/checkout.story/revisions/first.json", "first")
	moveCommit(t, repo, "Introduce the story")
	moveWrite(t, repo, "app.saga/checkout.story/revisions/second.json", "second")
	moveCommit(t, repo, "Edit legacy content")
	moveGit(t, repo, "rm", "-q", "-r", "app.saga/checkout.story/revisions")
	moveWrite(t, repo, "app.saga/checkout.story/current.json", "second")
	moveCommit(t, repo, "Normalize current content")
	moveGit(t, repo, "mv", "app.saga", "change.saga")
	moveCommit(t, repo, "Move the Saga unchanged")
	moveWrite(t, repo, "change.saga/checkout.story/current.json", "third")
	moveCommit(t, repo, "Edit current content")
	files := recordHistoryFiles([]string{"checkout.story/story.json", "checkout.story/current.json"})
	commits, err := recordCommits(context.Background(), Location{Repo: repo, Path: "change.saga"}, files)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(subjects(commits), " | "); got != "Introduce the story | Edit legacy content | Normalize current content | Edit current content" {
		t.Fatalf("Git record history after normalization = %s", got)
	}
}

func TestCurrentTechnicalAndWhiteboardHistoryUsesGit(t *testing.T) {
	repo := t.TempDir()
	moveGit(t, repo, "init", "-q", "-b", "main")
	root := filepath.Join(repo, "app.saga")
	moveWrite(t, repo, "app.saga/saga.json", `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"test","title":"Test","source":{"repository":"https://example.test/app.git"}}`)
	definition := requirements.TechnicalDefinition{Name: "Store", Explanation: "First", Code: []requirements.Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("a", 40), Path: "source.go", Start: 1, End: 1, Digest: "sha256:" + strings.Repeat("a", 64), Note: "Exact declaration"}}}}
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "", nil, definition, true); err != nil {
		t.Fatal(err)
	}
	board := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "owner", Kind: "node", Shape: "rect", Label: "First", X: 10, Y: 10, Width: 200, Height: 100, Style: "normal"}}}
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "map", "", "", "Map", board); err != nil {
		t.Fatal(err)
	}
	first := moveCommit(t, repo, "Introduce current documentation")
	definition.Explanation, board.Elements[0].Label = "Edited", "Edited"
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "", nil, definition, false); err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "map", "", "", "Map", board); err != nil {
		t.Fatal(err)
	}
	moveCommit(t, repo, "Edit current documentation")
	for _, urn := range []string{"urn:change-saga:test:component:store", "urn:change-saga:test:whiteboard:map"} {
		history, err := NodeHistory(context.Background(), root, urn)
		if err != nil || len(history.Events) != 2 || history.Introduced == nil || history.Introduced.Head != first {
			t.Fatalf("Git history for %s: %+v %v", urn, history, err)
		}
	}
}
