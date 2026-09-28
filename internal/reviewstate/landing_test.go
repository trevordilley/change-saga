package reviewstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// landingRepo is a repository whose main branch has one commit and whose
// feature branch commits a review record beside a code change, the way a
// pull request carries its review.
func landingRepo(t *testing.T) (string, *saga.Review) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")
	write(t, dir, "app.txt", "one\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "checkout", "-q", "-b", "feature")
	write(t, dir, "app.txt", "one\ntwo\n")
	reviewDir := filepath.Join(dir, "change.saga", "___reviews", "pr-1.review")
	write(t, reviewDir, "review.json", `{"id":"pr-1"}`)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feature with its review")
	return dir, &saga.Review{ReviewManifest: saga.ReviewManifest{ID: "pr-1", Base: "main", Head: "feature"}, Directory: reviewDir}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func detect(t *testing.T, dir string, review *saga.Review) State {
	t.Helper()
	return NewLandings(dir).Detect(context.Background(), review)
}

func TestDetectOpenReview(t *testing.T) {
	dir, review := landingRepo(t)
	if got := detect(t, dir, review); got != (State{State: StateOpen, Source: StateDetected}) {
		t.Fatalf("open review: %+v", got)
	}
}

func TestDetectFastForwardMerge(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--ff-only", "feature")
	if got := detect(t, dir, review); got != (State{State: StateMerged, Source: StateDetected, LandedIn: "main"}) {
		t.Fatalf("fast-forward: %+v", got)
	}
}

func TestDetectMergeCommit(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "other.txt", "other\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "other work")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	if got := detect(t, dir, review); !got.Merged() || got.Source != StateDetected {
		t.Fatalf("merge commit: %+v", got)
	}
}

// A squash merge leaves the branch's head outside main, which asking
// whether the head is an ancestor would miss.
func TestDetectSquashMergeWithDeletedBranch(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--squash", "feature")
	git(t, dir, "commit", "-q", "-m", "squashed feature")
	if got := detect(t, dir, review); !got.Merged() {
		t.Fatalf("squash with branch: %+v", got)
	}
	git(t, dir, "branch", "-q", "-D", "feature")
	if got := detect(t, dir, review); !got.Merged() || got.LandedIn != "main" {
		t.Fatalf("squash with deleted branch: %+v", got)
	}
}

func TestDetectRecordedMergeWins(t *testing.T) {
	dir, review := landingRepo(t)
	review.Merged = &saga.ReviewMerge{Landed: "abc"}
	if got := detect(t, dir, review); got != (State{State: StateMerged, Source: StateRecorded}) {
		t.Fatalf("recorded: %+v", got)
	}
}

// A new branch has no commits of its own yet, so its head is main's tip;
// its review is not in main and is not mistaken for a merged one.
func TestDetectBranchWithoutCommitsIsOpen(t *testing.T) {
	dir, _ := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "checkout", "-q", "-b", "fresh")
	reviewDir := filepath.Join(dir, "change.saga", "___reviews", "pr-2.review")
	write(t, reviewDir, "review.json", `{"id":"pr-2"}`)
	review := &saga.Review{ReviewManifest: saga.ReviewManifest{ID: "pr-2", Base: "main"}, Directory: reviewDir}
	if got := detect(t, dir, review); got.Merged() {
		t.Fatalf("fresh branch: %+v", got)
	}
}

// A base pinned to a commit never moves; the review is looked for in the
// default branch instead.
func TestDetectPinnedBaseReadsDefaultBranch(t *testing.T) {
	dir, review := landingRepo(t)
	review.Base = git(t, dir, "rev-parse", "main")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--ff-only", "feature")
	if got := detect(t, dir, review); !got.Merged() || got.LandedIn != "main" {
		t.Fatalf("pinned base: %+v", got)
	}
}

// A Saga outside the code checkout keeps its records elsewhere, so Git
// cannot tell; it is reported open, never guessed.
func TestDetectCompanionSagaIsUnknown(t *testing.T) {
	dir, _ := landingRepo(t)
	elsewhere := filepath.Join(t.TempDir(), "change.saga", "___reviews", "pr-1.review")
	review := &saga.Review{ReviewManifest: saga.ReviewManifest{ID: "pr-1", Base: "main"}, Directory: elsewhere}
	if got := detect(t, dir, review); got != (State{State: StateOpen, Source: StateUnknown}) {
		t.Fatalf("companion: %+v", got)
	}
}

// A review committed to main ahead of its change, as one created on main or
// a stacked review whose parent merged first, stays open while its branch
// still has changes main lacks.
func TestDetectRecordAheadOfItsChangeIsOpen(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "checkout", "-q", "feature", "--", "change.saga")
	git(t, dir, "commit", "-q", "-m", "the review, ahead of its change")
	if got := detect(t, dir, review); got != (State{State: StateOpen, Source: StateDetected}) {
		t.Fatalf("record ahead of change: %+v", got)
	}
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	if got := detect(t, dir, review); !got.Merged() {
		t.Fatalf("after its change landed: %+v", got)
	}
}
