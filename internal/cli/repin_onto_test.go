package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
)

// landRepinOntoFixture publishes the transaction fixture's managed slide and a
// plain evidence file, both pinned at the fixture's base commit, then lands a
// later commit that leaves the referenced code unchanged.
func landRepinOntoFixture(t *testing.T) (root, repo, sagaID, landed string) {
	t.Helper()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "flow-create", "create", "absent", "worker")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	coverJSON(t, "--repo", repo, "--commit", commit, "--path", "service.go", "--lines", "3", "--name", "plain", root)
	writeFile(t, filepath.Join(repo, "notes.txt"), "landed\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "land")
	return root, repo, sagaID, strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

// currentFlowEvidenceCommit is the commit the managed slide's evidence is
// pinned at, read through the request apply-slide --print-current prints.
func currentFlowEvidenceCommit(t *testing.T, root string) string {
	t.Helper()
	var printed bytes.Buffer
	if err := ApplySlide(context.Background(), []string{"--print-current", "flow", root}, &printed); err != nil {
		t.Fatal(err)
	}
	var current SlideTransactionRequest
	if err := json.Unmarshal(printed.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	return current.Items[0].Evidence[0].References[0].Commit
}

// repin --onto moves the evidence of a slide apply-slide manages through one
// complete-slide update, as accept-proposed does, instead of reading the
// slide's evidence as if it were a file of its own.
func TestRepinOntoUpdatesAManagedSlideThroughItsTransaction(t *testing.T) {
	t.Parallel()
	root, repo, _, landed := landRepinOntoFixture(t)

	dry := runRepin(t, "--repo", repo, "--onto", landed, "--dry-run", root)
	if len(dry.Repinned) != 2 || currentFlowEvidenceCommit(t, root) == landed {
		t.Fatalf("dry run = %#v", dry)
	}
	result := runRepin(t, "--repo", repo, "--onto", landed, root)
	if len(result.Repinned) != 2 {
		t.Fatalf("repin --onto = %#v", result)
	}
	if got := currentFlowEvidenceCommit(t, root); got != landed {
		t.Fatalf("managed slide evidence pinned at %s, want %s", got, landed)
	}
	if plain := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "plain.json")); len(plain) != 1 || plain[0].Commit != landed {
		t.Fatalf("plain evidence = %#v", plain)
	}
}

// A repin --onto that cannot complete writes nothing: every edit, plain and
// managed, is validated before the first is written. Here the managed slide's
// criterion link went stale when its story was revised, so its complete-slide
// update is refused, and the plain evidence file must keep its old pin.
func TestRepinOntoThatFailsLeavesTheSagaUntouched(t *testing.T) {
	t.Parallel()
	root, repo, sagaID, landed := landRepinOntoFixture(t)
	story := "urn:change-saga:" + sagaID + ":story:run"
	mustRun(t, Story, "revise", "--feature", testFeature, "--story", story, "--revision", "r2", "--parent", story+":revision:r1",
		"--title", "Run safely", "--statement", "As a user I run the service again", "--criterion", "returns=Run returns without error", root)

	plainPath := filepath.Join(root, saga.CodeDirName, "plain.json")
	before, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	pinned := currentFlowEvidenceCommit(t, root)
	// A dry run reports the refused slide, and how to repair it, too.
	var output bytes.Buffer
	for _, dry := range []bool{true, false} {
		args := []string{"--repo", repo, "--onto", landed, root}
		if dry {
			args = append([]string{"--dry-run"}, args...)
		}
		output.Reset()
		err = Repin(context.Background(), args, &output)
		if err == nil || !strings.Contains(err.Error(), "update slide") || !strings.Contains(err.Error(), "apply-slide --print-current") {
			t.Fatalf("repin --onto (dry run %v) with a slide it cannot update = %v\n%s", dry, err, output.String())
		}
	}
	after, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("a failed repin --onto rewrote the plain evidence:\n%s", after)
	}
	if got := currentFlowEvidenceCommit(t, root); got != pinned {
		t.Fatalf("a failed repin --onto moved the slide's evidence to %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, saga.MergesDir)); !os.IsNotExist(err) {
		t.Fatalf("a failed repin --onto recorded a merge: %v", err)
	}
}

// A write that fails under the lock leaves no landing behind: slide updates
// are written first, so the injected failure of the first one writes no
// plain evidence and no cursor. (Not parallel: the fault hook is global.)
func TestRepinOntoFailureWhileWritingLeavesNoLanding(t *testing.T) {
	root, repo, _, landed := landRepinOntoFixture(t)
	plainPath := filepath.Join(root, saga.CodeDirName, "plain.json")
	cursorPath := filepath.Join(root, saga.CursorName)
	plainBefore, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	cursorBefore, _ := os.ReadFile(cursorPath)
	pinned := currentFlowEvidenceCommit(t, root)
	slideTransactionFault = func(step string) error {
		if step == "before-record-commit" {
			return errors.New("injected failure")
		}
		return nil
	}
	t.Cleanup(func() { slideTransactionFault = nil })
	var output bytes.Buffer
	if err := Repin(context.Background(), []string{"--repo", repo, "--onto", landed, root}, &output); err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("repin --onto with a failing slide write = %v\n%s", err, output.String())
	}
	slideTransactionFault = nil
	if after, _ := os.ReadFile(plainPath); !bytes.Equal(plainBefore, after) {
		t.Fatalf("a failed write rewrote the plain evidence:\n%s", after)
	}
	if after, _ := os.ReadFile(cursorPath); !bytes.Equal(cursorBefore, after) {
		t.Fatalf("a failed write moved the cursor:\n%s", after)
	}
	if got := currentFlowEvidenceCommit(t, root); got != pinned {
		t.Fatalf("a failed write moved the slide's evidence to %s", got)
	}
}

// Freezing an older landing records only that landing's branch commits,
// even when living evidence is already pinned at later commits, and the
// frozen review says when the change merged, not when it was frozen.
func TestRepinOntoRecordsOnlyTheLandedBranchCommits(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "app.go"), appFeature)
	commitAll(t, repo, "Base")
	root := filepath.Join(repo, "change.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--title", "App", root}, &output); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "Start the Saga")
	run(t, Review, "create", "--id", "feat", "--base", "main", "--head", "feat", "--repo", repo, root)
	commitAll(t, repo, "Create the review")

	git(t, repo, "checkout", "-q", "-b", "feat")
	writeFile(t, filepath.Join(repo, "one.go"), "package app\n\nfunc One() int { return 1 }\n")
	commitAll(t, repo, "Feature one")
	writeFile(t, filepath.Join(repo, "two.go"), "package app\n\nfunc Two() int { return 2 }\n")
	commitAll(t, repo, "Feature two")
	git(t, repo, "checkout", "-q", "main")
	merge := exec.Command("git", "-C", repo, "merge", "-q", "--no-ff", "feat", "-m", "Merge feat")
	merge.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2020-01-02T03:04:05Z")
	if out, err := merge.CombinedOutput(); err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}
	landed := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	// Later work on main, with living evidence pinned at it.
	writeFile(t, filepath.Join(repo, "later.go"), "package app\n\nfunc Later() int { return 3 }\n")
	later := commitAll(t, repo, "Later work")
	coverJSON(t, "--commit", later, "--path", "later.go", "--lines", "3", "--name", "later", root)
	commitAll(t, repo, "Record later evidence")

	result := runRepin(t, "--onto", landed, "--branch", "feat", "--review", "feat", root)
	var subjects []string
	for _, commit := range result.Commits {
		subjects = append(subjects, commit.Subject)
	}
	if strings.Join(subjects, "|") != "Feature one|Feature two" {
		t.Fatalf("the merge record lists %q, want only the landed branch's commits", subjects)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("feat")
	if review == nil || review.Merged == nil || !review.Merged.MergedAt.Equal(time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)) {
		t.Fatalf("frozen review = %#v", review)
	}
}
