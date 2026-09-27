package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// These are regressions of the review matching and measuring the review area
// reports. Each builds the history a team really has: stacked pull requests,
// a review whose change landed, a CI checkout, a pull request of another
// branch.

// reviewRepo is a repository with an origin and a Saga of reviews alone,
// committed on main.
func reviewRepo(t *testing.T) (repo, root string) {
	t.Helper()
	t.Setenv("CHANGE_SAGA_NO_GH", "1")
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Dev")
	git(t, repo, "config", "user.email", "dev@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "main.go"), "package app\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	root = filepath.Join(repo, "app.saga")
	run(t, Init, "--repo", repo, root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "saga")
	return repo, root
}

// commitFile writes path with lines on the current branch and commits it.
func commitFile(t *testing.T, repo, path, content string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, path), content)
	git(t, repo, "add", path)
	git(t, repo, "commit", "-m", "add "+path)
}

// explain creates review id (with extra create flags) with one slide and
// Item, covers every changed line of paths in the review's own range, and
// commits the Saga.
func explain(t *testing.T, repo, root, id string, paths []string, create ...string) {
	t.Helper()
	run(t, Review, append(append([]string{"create", "--id", id}, create...), root)...)
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, reviewSlideSVG)
	run(t, AddSlide, "--review", id, "--intent", "explain", "--layout", "diagram", "--source", visual, root, "change")
	run(t, AddItem, "--review", id, "--slide", "change", "--kind", "node", "--element-id", "node", "--description", "The change", root)
	for _, path := range paths {
		run(t, Cover, "--target", saga.ReviewItemTarget("app", id, "change", "node"), "--path", path, "--changed-lines", "--repo", repo, root)
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "review "+id)
}

func checkReview(t *testing.T, repo, root, against string) (int, string) {
	t.Helper()
	var output bytes.Buffer
	err := Check(context.Background(), []string{"--covers", "review", "--repo", repo, "--against", against, root}, &output)
	var exit *StatusError
	switch {
	case err == nil:
		return 0, output.String()
	case errors.As(err, &exit):
		return exit.Code, output.String()
	}
	t.Fatalf("check: %v\n%s", err, output.String())
	return 0, ""
}

func changeReviews(t *testing.T, repo, root, against string) ([]string, []string) {
	t.Helper()
	var document struct {
		ChangeReviews []string `json:"change_reviews"`
		NextActions   []struct {
			ID string `json:"id"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal([]byte(run(t, Status, "--json", "--repo", repo, "--against", against, root)), &document); err != nil {
		t.Fatal(err)
	}
	actions := []string{}
	for _, action := range document.NextActions {
		actions = append(actions, action.ID)
	}
	return document.ChangeReviews, actions
}

// A stacked pull request's review covers its own range from its base
// branch; asked about the whole change against main, the lines the base
// branch changed are not explained by it.
func TestAStackedReviewDoesNotCoverTheWholeChange(t *testing.T) {
	repo, root := reviewRepo(t)
	git(t, repo, "checkout", "-b", "feature/base")
	commitFile(t, repo, "base.go", "package app\n\nvar Base = 1\n")
	git(t, repo, "checkout", "-b", "feature/top")
	commitFile(t, repo, "top.go", "package app\n\nvar Top = 2\n")
	explain(t, repo, root, "pr-2", []string{"top.go"}, "--base", "feature/base")

	if code, out := checkReview(t, repo, root, "feature/base"); code != 0 {
		t.Fatalf("the review does not cover its own range:\n%s", out)
	}
	code, out := checkReview(t, repo, root, "main")
	if code != checkExitUncovered || !strings.Contains(out, "base.go") {
		t.Fatalf("check --against main of a stacked review = %d, want 3 naming base.go:\n%s", code, out)
	}
	if text := run(t, Status, "--repo", repo, "--against", "main", root); strings.Contains(text, "every changed line is explained") {
		t.Fatalf("status claims a stacked review explains the whole change:\n%s", text)
	}

}

// A stacked review whose base branch was deleted after merging is still this
// change's review: it is measured, not reported missing.
func TestAReviewWhoseBaseIsDeletedIsStillFound(t *testing.T) {
	repo, root := reviewRepo(t)
	git(t, repo, "checkout", "-b", "feature/base")
	commitFile(t, repo, "base.go", "package app\n\nvar Base = 1\n")
	git(t, repo, "checkout", "-b", "feature/top")
	commitFile(t, repo, "top.go", "package app\n\nvar Top = 2\n")
	explain(t, repo, root, "pr-2", []string{"top.go"}, "--base", "feature/base")
	git(t, repo, "branch", "-D", "feature/base")
	reviews, actions := changeReviews(t, repo, root, "main")
	if len(reviews) != 1 || reviews[0] != "pr-2" {
		t.Fatalf("with its base deleted, the review of the change = %v", reviews)
	}
	for _, action := range actions {
		if action == "review:create" {
			t.Fatal("status asks for a duplicate review of a change that has one")
		}
	}
}

// Once a review's change lands, the next branch's change is not the old
// review's: it asks for a review of its own, and its own review is enough.
func TestALandedReviewDoesNotClaimTheNextChange(t *testing.T) {
	for _, follows := range []string{"its branch", "HEAD"} {
		t.Run(follows, func(t *testing.T) {
			repo, root := reviewRepo(t)
			git(t, repo, "checkout", "-b", "feature/a")
			commitFile(t, repo, "a.go", "package app\n\nvar A = 1\n")
			create := []string{"--base", "main"}
			if follows == "HEAD" {
				create = append(create, "--head", "HEAD")
			}
			explain(t, repo, root, "pr-a", []string{"a.go"}, create...)
			git(t, repo, "checkout", "main")
			git(t, repo, "merge", "--no-ff", "-m", "Merge feature/a", "feature/a")
			// The merged branch is deleted, so a review that names it no longer
			// resolves; its landed evidence still says it is not this change.
			git(t, repo, "branch", "-D", "feature/a")
			git(t, repo, "checkout", "-b", "feature/b")
			commitFile(t, repo, "b.go", "package app\n\nvar B = 2\n")

			reviews, actions := changeReviews(t, repo, root, "main")
			if len(reviews) != 0 || !strings.Contains(strings.Join(actions, " "), "review:create") {
				t.Fatalf("the next change's reviews = %v, actions %v; want none and review:create", reviews, actions)
			}
			explain(t, repo, root, "pr-b", []string{"b.go"}, "--base", "main")
			if code, out := checkReview(t, repo, root, "main"); code != 0 {
				t.Fatalf("the new review of the next change is not enough:\n%s", out)
			}
		})
	}
}

// A CI checkout has origin's branches and no local main: the review of the
// checked-out change is found and measured there as it is locally.
func TestAReviewIsFoundInACICheckout(t *testing.T) {
	repo, root := reviewRepo(t)
	git(t, repo, "checkout", "-b", "feature/pg")
	commitFile(t, repo, "pg.go", "package app\n\nvar PG = 1\n")
	explain(t, repo, root, "pr-7", []string{"pg.go"}, "--base", "main")
	if code, out := checkReview(t, repo, root, "main"); code != 0 {
		t.Fatalf("the review is not complete locally:\n%s", out)
	}
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	ci := filepath.Join(t.TempDir(), "ci")
	git(t, filepath.Dir(ci), "clone", "--quiet", "--no-checkout", repo, ci)
	git(t, ci, "remote", "set-url", "origin", "https://example.test/acme/app.git")
	git(t, ci, "checkout", "--quiet", "--detach", head)
	for _, branch := range []string{"main", "feature/pg"} {
		// Neither the base nor the head exists as a local branch in CI.
		_, _ = gitErr(ci, "branch", "-D", branch)
	}
	ciRoot := filepath.Join(ci, "app.saga")
	if code, out := checkReview(t, ci, ciRoot, "origin/main"); code != 0 {
		t.Fatalf("check --against origin/main in a CI checkout = %d, want 0:\n%s", code, out)
	}

	// A fork's branch never exists in the base repository's CI clone: the
	// review is still found, through its evidence in this change.
	_, _ = gitErr(ci, "update-ref", "-d", "refs/remotes/origin/feature/pg")
	if code, out := checkReview(t, ci, ciRoot, "origin/main"); code != 0 {
		t.Fatalf("check --against origin/main of a fork's pull request = %d, want 0:\n%s", code, out)
	}

	// actions/checkout's default for a pull request is GitHub's merge commit
	// of the head into the base: the review of its second parent is found.
	git(t, ci, "checkout", "--quiet", "--detach", "origin/main")
	git(t, ci, "-c", "user.name=CI", "-c", "user.email=ci@example.test", "merge", "--quiet", "--no-ff", "-m", "Merge pull request", head)
	if code, out := checkReview(t, ci, ciRoot, "origin/main"); code != 0 {
		t.Fatalf("check --against origin/main on a pull request's merge commit = %d, want 0:\n%s", code, out)
	}
}

// review create --head names the branch whose pull request it is.
func TestReviewCreateAsksGHAboutTheHeadBranch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in gh is a shell script")
	}
	repo, root := reviewRepo(t)
	git(t, repo, "checkout", "-b", "feature/top")
	git(t, repo, "remote", "set-url", "origin", "https://github.com/acme/app.git")
	bin := t.TempDir()
	gh := "#!/bin/sh\ncase \"$*\" in\n*feature/new*) echo '{\"number\":6,\"url\":\"https://github.com/acme/app/pull/6\",\"title\":\"Add new\",\"baseRefName\":\"main\"}' ;;\n*) echo '{\"number\":5,\"url\":\"https://github.com/acme/app/pull/5\",\"title\":\"Add top\",\"baseRefName\":\"main\"}' ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(gh), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CHANGE_SAGA_NO_GH", "")
	out := run(t, Review, "create", "--head", "feature/new", root)
	if !strings.Contains(out, "review:pr-6") {
		t.Fatalf("review create --head feature/new took another branch's pull request:\n%s", out)
	}
}

func gitErr(dir string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	return string(output), err
}

// Every pull request gets a review, so a change no review explains asks for
// one even in a Saga that documents the application; observing asks nothing.
func TestALivingSagaAsksForAReviewOfAnUnreviewedChange(t *testing.T) {
	repo, root := reviewRepo(t)
	run(t, Overview, "set-pitch", "--text", "An app.", root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "pitch")
	git(t, repo, "checkout", "-b", "feature/c")
	commitFile(t, repo, "c.go", "package app\n\nvar C = 3\n")
	if _, actions := changeReviews(t, repo, root, "main"); !strings.Contains(strings.Join(actions, " "), "review:create") {
		t.Fatalf("a living Saga's unreviewed change does not ask for a review: %v", actions)
	}
	var observed struct {
		NextActions []struct {
			ID string `json:"id"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal([]byte(run(t, Status, "--json", "--repo", repo, root)), &observed); err != nil {
		t.Fatal(err)
	}
	for _, action := range observed.NextActions {
		if action.ID == "review:create" {
			t.Fatal("observing asks for a review")
		}
	}
	explain(t, repo, root, "pr-c", []string{"c.go"}, "--base", "main")
	if _, actions := changeReviews(t, repo, root, "main"); strings.Contains(strings.Join(actions, " "), "review:create") {
		t.Fatalf("a reviewed change still asks for a review: %v", actions)
	}
}

// A review that follows HEAD whose branch was rewritten (squashed here, as
// amend and rebase also do) is pinned, not duplicated: status suggests review
// follow instead of review create.
func TestARewrittenReviewIsPinnedNotDuplicated(t *testing.T) {
	repo, root := reviewRepo(t)
	git(t, repo, "checkout", "-b", "feature/d")
	commitFile(t, repo, "d.go", "package app\n\nvar D = 4\n")
	explain(t, repo, root, "pr-d", []string{"d.go"}, "--base", "main", "--head", "HEAD")
	git(t, repo, "reset", "--soft", "main")
	git(t, repo, "commit", "-m", "Squashed")
	reviews, actions := changeReviews(t, repo, root, "main")
	joined := strings.Join(actions, " ")
	if len(reviews) != 0 || !strings.Contains(joined, "review:follow:pr-d") || strings.Contains(joined, "review:create") {
		t.Fatalf("an amended review: reviews %v, actions %v; want review:follow:pr-d and no review:create", reviews, actions)
	}
	if text := run(t, Status, "--repo", repo, "--against", "main", root); !strings.Contains(text, "review follow --review pr-d") {
		t.Fatalf("status does not suggest pinning the amended review:\n%s", text)
	}
}

// A review whose Items explain only deleted lines pins the comparison's base;
// that evidence is this change's.
func TestAReviewOfDeletedLinesIsThisChanges(t *testing.T) {
	repo, root := reviewRepo(t)
	commitFile(t, repo, "old.go", "package app\n\nvar Old = 1\n")
	git(t, repo, "checkout", "-b", "feature/e")
	git(t, repo, "rm", "-q", "old.go")
	git(t, repo, "commit", "-m", "remove old.go")
	explain(t, repo, root, "pr-e", []string{"old.go"}, "--base", "main", "--head", "HEAD")
	if code, out := checkReview(t, repo, root, "main"); code != 0 {
		t.Fatalf("a review of deleted lines = %d, want 0:\n%s", code, out)
	}
}
