package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// newReviewOnlyFixture is a repository whose Saga holds nothing but init's
// records, with a feature branch changing two files. With reviewed, the
// branch has a review whose one Item covers every changed line.
func newReviewOnlyFixture(t *testing.T, reviewed bool) reviewFixture {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Dev")
	git(t, repo, "config", "user.email", "dev@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"sqs\" }\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	root := filepath.Join(repo, "app.saga")
	run(t, Init, "--repo", repo, root)
	git(t, repo, "checkout", "-b", "feature/pg")
	writeFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres\" }\n")
	writeFile(t, filepath.Join(repo, "store.go"), "package store\n\nfunc Table() string { return \"jobs\" }\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Move the queue to Postgres")
	if reviewed {
		createCoveredReview(t, repo, root, "pr-7")
	}
	return reviewFixture{repo: repo, root: root}
}

// createCoveredReview creates review id of the checkout's HEAD, with one
// slide whose Item covers every changed line.
func createCoveredReview(t *testing.T, repo, root, id string) {
	t.Helper()
	run(t, Review, "create", "--id", id, "--base", "main", root)
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, reviewSlideSVG)
	run(t, AddSlide, "--review", id, "--intent", "explain", "--layout", "diagram", "--source", visual, root, "queue")
	run(t, AddItem, "--review", id, "--slide", "queue", "--kind", "node", "--element-id", "node", "--description", "The queue moves to a table", root)
	for _, path := range []string{"queue.go", "store.go"} {
		run(t, Cover, "--target", saga.ReviewItemTarget("app", id, "queue", "node"), "--path", path, "--changed-lines", "--repo", repo, root)
	}
}

func statusText(t *testing.T, root string, args ...string) string {
	t.Helper()
	return run(t, Status, append(args, root)...)
}

func statusJSON(t *testing.T, root string, args ...string) statusDocument {
	t.Helper()
	var document statusDocument
	if err := json.Unmarshal([]byte(run(t, Status, append(append(args, "--json"), root)...)), &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestStatusOfAReviewOnlySagaLeadsWithTheReview(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	for _, want := range []string{
		"Review pr-7: its deck explains 6 of 6 changed lines and file events of the pull request — every changed line is explained.",
		"Next actions: none. The review deck explains every changed line",
		"Growing the Saga is optional",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status lacks %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Changed lines nothing references", "Coverage of", "elevator pitch", "[personas]", "[changed_source]"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("review-only status shows %q:\n%s", unwanted, text)
		}
	}
	document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if document.Growth != (growthState{Reviews: 1, Offer: growthOfferQuiet}) {
		t.Fatalf("growth = %+v", document.Growth)
	}
	if review := document.Coverage.Areas.Review; !review.Complete || review.Total != 6 || len(document.ChangeReviews) != 1 {
		t.Fatalf("review area = %+v, reviews of change %v", review, document.ChangeReviews)
	}
	for _, action := range document.NextActions {
		t.Fatalf("a covered review-only change asks for %s", action.ID)
	}

	full := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main", "--growth")
	for _, want := range []string{"Coverage of the change", "review          6/6 changed lines explained by the review deck", "Growth ("} {
		if !strings.Contains(full, want) {
			t.Fatalf("status --growth lacks %q:\n%s", want, full)
		}
	}
}

func TestStatusAsksForAReviewOfAnUnreviewedChange(t *testing.T) {
	fixture := newReviewOnlyFixture(t, false)
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if !strings.Contains(text, "Review: none yet. No review explains the 6 changed lines of this change.") || !strings.Contains(text, "change-saga review create") {
		t.Fatalf("status does not ask for a review:\n%s", text)
	}
	document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if len(document.NextActions) != 1 || document.NextActions[0].ID != "review:create" {
		t.Fatalf("next actions = %+v", document.NextActions)
	}
}

func TestStatusOffersGrowthPastACoupleOfReviews(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	for _, id := range []string{"pr-8", "pr-9"} {
		createCoveredReview(t, fixture.repo, fixture.root, id)
	}
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if !strings.Contains(text, "Grow the Saga (optional, never required): it holds 3 reviews") || !strings.Contains(text, "setup-initial-saga") {
		t.Fatalf("status past a couple of reviews does not offer growth:\n%s", text)
	}
	if strings.Contains(text, "why:") || strings.Count(text, "\n  1. [") != 1 {
		t.Fatalf("the growth offer is not short:\n%s", text)
	}
	if document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main"); document.Growth.Offer != growthOfferProminent {
		t.Fatalf("growth = %+v", document.Growth)
	}
}

func TestStatusOfALivingSagaKeepsTheFullReport(t *testing.T) {
	fixture := newReviewFixture(t)
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if !strings.Contains(text, "Review pr-7: its deck explains") || !strings.Contains(text, "Coverage of the change") {
		t.Fatalf("living status lacks the review headline or the areas:\n%s", text)
	}
	if document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main"); !document.Growth.LivingDocumentation || document.Growth.Offer != growthOfferFull {
		t.Fatalf("growth = %+v", document.Growth)
	}
}

func TestCheckCoversReview(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	out := run(t, Check, "--covers", "review", "--repo", fixture.repo, "--against", "main", fixture.root)
	if !strings.Contains(out, "fully covered") {
		t.Fatalf("check --covers review:\n%s", out)
	}
	writeFile(t, filepath.Join(fixture.repo, "extra.go"), "package queue\n\nvar Extra = 1\n")
	git(t, fixture.repo, "add", "extra.go")
	git(t, fixture.repo, "commit", "-m", "extra")
	var output bytes.Buffer
	err := Check(context.Background(), []string{"--covers", "review", "--repo", fixture.repo, "--against", "main", fixture.root}, &output)
	var exit *StatusError
	if !errors.As(err, &exit) || exit.Code != checkExitUncovered || !strings.Contains(output.String(), "extra.go") {
		t.Fatalf("check of an unexplained line = %v\n%s", err, output.String())
	}

	unreviewed := newReviewOnlyFixture(t, false)
	output.Reset()
	err = Check(context.Background(), []string{"--covers", "review", "--repo", unreviewed.repo, "--against", "main", unreviewed.root}, &output)
	if !errors.As(err, &exit) || exit.Code != checkExitUncovered || !strings.Contains(output.String(), unreviewedReason) {
		t.Fatalf("check of an unreviewed change = %v\n%s", err, output.String())
	}
}

func TestHoldsLivingDocumentation(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	if holdsLivingDocumentation(fixture.root) {
		t.Fatal("a Saga of init and one review holds living documentation")
	}
	if err := os.MkdirAll(filepath.Join(fixture.root, "___personas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !holdsLivingDocumentation(fixture.root) {
		t.Fatal("a Saga with personas holds no living documentation")
	}
}
