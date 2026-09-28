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

var (
	open    = State{State: StateOpen, Source: StateDetected}
	unknown = State{State: StateOpen, Source: StateUnknown}
)

// landingRepo is a repository whose main branch has one commit and whose
// feature branch commits a review record beside a code change, the way a
// pull request carries its review: pull request #1, following feature.
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
	review := addReview(t, dir, "pr-1", 1, "feature")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feature with its review")
	return dir, review
}

// addReview writes a review record into the working tree.
func addReview(t *testing.T, dir, id string, number int, head string) *saga.Review {
	t.Helper()
	reviewDir := filepath.Join(dir, "change.saga", "___reviews", id+".review")
	write(t, reviewDir, "review.json", `{"id":"`+id+`"}`)
	return &saga.Review{ReviewManifest: saga.ReviewManifest{ID: id, Base: "main", Head: head, PullRequest: &saga.PullRequest{Number: number}}, Directory: reviewDir}
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

func commitAll(t *testing.T, dir, message string) {
	t.Helper()
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", message)
}

func detect(t *testing.T, dir string, review *saga.Review) State {
	t.Helper()
	return NewLandings(dir).Detect(context.Background(), review)
}

func expectState(t *testing.T, name string, got, want State) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %+v, want %+v", name, got, want)
	}
}

func merged(ref string) State { return State{State: StateMerged, Source: StateDetected, LandedIn: ref} }

func TestDetectOpenReview(t *testing.T) {
	dir, review := landingRepo(t)
	expectState(t, "open review", detect(t, dir, review), open)
}

func TestDetectRecordedMergeWins(t *testing.T) {
	dir, review := landingRepo(t)
	review.Merged = &saga.ReviewMerge{Landed: "abc"}
	expectState(t, "recorded", detect(t, dir, review), State{State: StateMerged, Source: StateRecorded})
}

// A merge commit merges the branch: its head is in main but not a commit of
// main's own line. That holds after the branch is deleted too, when the
// merge's subject names the pull request or the branch.
func TestDetectMergeCommit(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "other.txt", "other\n")
	commitAll(t, dir, "other work")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from acme/feature", "feature")
	expectState(t, "merge commit", detect(t, dir, review), merged("main"))
	git(t, dir, "branch", "-q", "-D", "feature")
	expectState(t, "merge commit, branch deleted", detect(t, dir, review), merged("main"))
}

// A merge commit that names nothing still merged the branch while the
// branch is there to show it: its head joined main through the merge.
func TestDetectMergeCommitThatNamesNothing(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "other.txt", "other\n")
	commitAll(t, dir, "other work")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge some work", "feature")
	expectState(t, "merge commit naming nothing", detect(t, dir, review), merged("main"))
}

func TestDetectMergeCommitOfADeletedBranchNeedsItsName(t *testing.T) {
	for subject, want := range map[string]State{
		"Merge branch 'feature'": merged("main"),
		"Merge some work":        unknown,
	} {
		dir, review := landingRepo(t)
		git(t, dir, "checkout", "-q", "main")
		git(t, dir, "merge", "-q", "--no-ff", "-m", subject, "feature")
		git(t, dir, "branch", "-q", "-D", "feature")
		expectState(t, subject, detect(t, dir, review), want)
	}
}

// A fast-forward leaves main's line running through the branch's commits,
// exactly as a record committed to main before a branch with no commits of
// its own; which one it was cannot be told, so it is never called merged.
func TestDetectFastForwardIsUnknown(t *testing.T) {
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--ff-only", "feature")
	expectState(t, "fast-forward", detect(t, dir, review), unknown)
}

// A squash leaves the branch's head outside main, which asking whether the
// head is an ancestor would miss; merging it changes nothing. Once the
// branch is gone, GitHub's "(#N)" subject is what is left.
func TestDetectSquashMerge(t *testing.T) {
	for subject, deleted := range map[string]State{
		"Move the queue (#1)": merged("main"),
		"Move the queue":      unknown,
	} {
		dir, review := landingRepo(t)
		git(t, dir, "checkout", "-q", "main")
		git(t, dir, "merge", "-q", "--squash", "feature")
		git(t, dir, "commit", "-q", "-m", subject)
		expectState(t, subject+", branch present", detect(t, dir, review), merged("main"))
		git(t, dir, "branch", "-q", "-D", "feature")
		expectState(t, subject+", branch deleted", detect(t, dir, review), deleted)
	}
}

// A base pinned to a commit never moves; the review is looked for in the
// default branch instead.
func TestDetectPinnedBaseReadsDefaultBranch(t *testing.T) {
	dir, review := landingRepo(t)
	review.Base = git(t, dir, "rev-parse", "main")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from acme/feature", "feature")
	expectState(t, "pinned base", detect(t, dir, review), merged("main"))
}

// A Saga outside the code checkout keeps its records elsewhere, so Git
// cannot tell; it is reported open, never guessed.
func TestDetectCompanionSagaIsUnknown(t *testing.T) {
	dir, _ := landingRepo(t)
	elsewhere := filepath.Join(t.TempDir(), "change.saga", "___reviews", "pr-1.review")
	review := &saga.Review{ReviewManifest: saga.ReviewManifest{ID: "pr-1", Base: "main", Head: "feature"}, Directory: elsewhere}
	expectState(t, "companion", detect(t, dir, review), unknown)
}

// recordAhead commits pr-2's review to main before its change exists, the
// way a review created on main, or a stacked review whose parent merged
// first, reaches main.
func recordAhead(t *testing.T) (string, *saga.Review) {
	t.Helper()
	dir, _ := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	review := addReview(t, dir, "pr-2", 2, "next")
	commitAll(t, dir, "Review for the next change")
	return dir, review
}

// A record ahead of its change is never merged: open while its branch adds
// something, and unknown while nothing tells it from a fast-forward.
func TestDetectRecordAheadOfItsChange(t *testing.T) {
	t.Run("branch with commits of its own", func(t *testing.T) {
		dir, review := recordAhead(t)
		git(t, dir, "checkout", "-q", "-b", "next")
		write(t, dir, "next.txt", "next\n")
		commitAll(t, dir, "next change")
		expectState(t, "branch with commits", detect(t, dir, review), open)
		git(t, dir, "checkout", "-q", "main")
		git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge some work", "next")
		expectState(t, "after its change merged", detect(t, dir, review), merged("main"))
	})
	t.Run("branch with no commits yet", func(t *testing.T) {
		dir, review := recordAhead(t)
		git(t, dir, "branch", "next")
		expectState(t, "branch with no commits", detect(t, dir, review), unknown)
	})
	t.Run("branch that does not resolve", func(t *testing.T) {
		dir, review := recordAhead(t)
		expectState(t, "unresolved branch", detect(t, dir, review), unknown)
	})
	t.Run("review that follows HEAD", func(t *testing.T) {
		dir, review := recordAhead(t)
		review.Head = ""
		expectState(t, "follows HEAD", detect(t, dir, review), unknown)
	})
}

// One branch can carry several pull requests one after another. The first
// one's review stays merged as the branch moves on, and the next one's is
// open.
func TestDetectReusedBranch(t *testing.T) {
	dir, first := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from acme/feature", "feature")
	git(t, dir, "checkout", "-q", "feature")
	write(t, dir, "app.txt", "one\ntwo\nthree\n")
	second := addReview(t, dir, "pr-3", 3, "feature")
	commitAll(t, dir, "the next pull request on the same branch")
	expectState(t, "first review", detect(t, dir, first), merged("main"))
	expectState(t, "second review", detect(t, dir, second), open)
}

// A stacked review's record can reach main in its parent's merge while its
// own branch, forked from the parent, still has work main lacks. The merge
// names the parent, not this review, so it stays open.
func TestDetectStackedReviewWhoseParentMergedFirst(t *testing.T) {
	dir, _ := landingRepo(t)
	child := addReview(t, dir, "pr-4", 4, "child")
	commitAll(t, dir, "Review for the stacked change")
	git(t, dir, "checkout", "-q", "-b", "child")
	write(t, dir, "child.txt", "child\n")
	commitAll(t, dir, "stacked change")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from acme/feature", "feature")
	expectState(t, "stacked child", detect(t, dir, child), open)
}
