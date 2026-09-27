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
		"Review pr-7: its deck explains 6 of 6 changed lines and file events of this change — every changed line is explained.",
		"Next actions: none. The review deck explains every changed line",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status lacks %q:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"Changed lines nothing references", "Coverage of", "elevator pitch", "[personas]", "[changed_source]", "Grow", "setup-initial-saga"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("review-only status shows %q:\n%s", unwanted, text)
		}
	}
	document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if document.Documentation != (documentationState{Report: reportReviewFirst}) {
		t.Fatalf("documentation = %+v", document.Documentation)
	}
	if review := document.Coverage.Areas.Review; !review.Complete || review.Total != 6 || len(document.ChangeReviews) != 1 {
		t.Fatalf("review area = %+v, reviews of change %v", review, document.ChangeReviews)
	}
	for _, action := range document.NextActions {
		t.Fatalf("a covered review-only change asks for %s", action.ID)
	}

	full := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main", "--full")
	for _, want := range []string{"Coverage of the change", "review          6/6 changed lines explained by the review deck", "Growth ("} {
		if !strings.Contains(full, want) {
			t.Fatalf("status --full lacks %q:\n%s", want, full)
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

// A team may never grow its Saga beyond reviews, so however many reviews it
// holds, status never pitches documentation.
func TestStatusNeverPitchesGrowth(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	for _, id := range []string{"pr-8", "pr-9"} {
		createCoveredReview(t, fixture.repo, fixture.root, id)
	}
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	for _, unwanted := range []string{"Grow", "setup-initial-saga", "persona", "[overview]", "optional"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("status of a Saga with three reviews pitches %q:\n%s", unwanted, text)
		}
	}
	document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	for _, action := range document.NextActions {
		t.Fatalf("a covered review-only change asks for %s", action.ID)
	}
}

func TestStatusOfALivingSagaKeepsTheFullReport(t *testing.T) {
	fixture := newReviewFixture(t)
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	text := statusText(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if !strings.Contains(text, "Review pr-7: its deck explains") || !strings.Contains(text, "Coverage of the change") {
		t.Fatalf("living status lacks the review headline or the areas:\n%s", text)
	}
	if document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main"); !document.Documentation.LivingDocumentation || document.Documentation.Report != reportFull {
		t.Fatalf("documentation = %+v", document.Documentation)
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

func TestReviewTargetsAreDiscoverable(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	children := func(parent string) []string {
		t.Helper()
		var envelope struct {
			OK   bool `json:"ok"`
			Data struct {
				Children []struct {
					Kind   string `json:"kind"`
					Target string `json:"target"`
				} `json:"children"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(run(t, Query, "children", "--saga", fixture.root, "--repo", fixture.repo, "--parent", parent)), &envelope); err != nil || !envelope.OK {
			t.Fatalf("query children %s: %v %+v", parent, err, envelope)
		}
		result := []string{}
		for _, child := range envelope.Data.Children {
			result = append(result, child.Kind+" "+child.Target)
		}
		return result
	}
	review := saga.ReviewTarget("app", "pr-7")
	if got := strings.Join(children(saga.SagaTarget("app")), "\n"); !strings.Contains(got, "review "+review) {
		t.Fatalf("the Saga's children omit its review:\n%s", got)
	}
	deck := saga.ReviewDeckTarget("app", "pr-7", "pr-7")
	if got := children(review); len(got) != 1 || got[0] != "review-deck "+deck {
		t.Fatalf("review children = %v", got)
	}
	if got := children(deck); len(got) != 1 || got[0] != "slide "+saga.ReviewSlideTarget("app", "pr-7", "queue") {
		t.Fatalf("review deck children = %v", got)
	}
	if got := children(saga.ReviewSlideTarget("app", "pr-7", "queue")); len(got) != 1 || got[0] != "item "+saga.ReviewItemTarget("app", "pr-7", "queue", "node") {
		t.Fatalf("review slide children = %v", got)
	}

	var output bytes.Buffer
	err := Cover(context.Background(), []string{"--target", saga.ReviewItemTarget("app", "pr-7", "queue", "missing"), "--path", "queue.go", "--changed-lines", "--repo", fixture.repo, fixture.root}, &output)
	if err == nil || !strings.Contains(err.Error(), saga.ReviewItemTarget("app", "pr-7", "queue", "node")) || !strings.Contains(err.Error(), "--parent "+review) {
		t.Fatalf("an unknown review target's error does not list the review's targets: %v", err)
	}
}

func TestReviewListNamesTheRecordOfAStaleReference(t *testing.T) {
	fixture := newReviewOnlyFixture(t, true)
	writeFile(t, filepath.Join(fixture.repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres-v2\" }\n")
	git(t, fixture.repo, "commit", "-am", "Rename the queue table")
	out := run(t, Review, "list", "--uncovered", "--repo", fixture.repo, fixture.root)
	index := strings.Index(out, "      record ___reviews/pr-7.review/")
	if !strings.Contains(out, "stale ") || index < 0 {
		t.Fatalf("review list does not name the stale reference's record:\n%s", out)
	}
	record := strings.Fields(out[index:])[1]
	run(t, ReplaceCoverage, "--record", record, "--target", saga.ReviewItemTarget("app", "pr-7", "queue", "node"), "--path", "queue.go", "--changed-lines", "--repo", fixture.repo, fixture.root)
	if out := run(t, Review, "list", "--uncovered", "--repo", fixture.repo, fixture.root); !strings.Contains(out, "No uncovered changes") {
		t.Fatalf("replacing the named record did not repair the review:\n%s", out)
	}
}
