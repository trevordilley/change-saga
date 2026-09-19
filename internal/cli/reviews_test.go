package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

const reviewSlideSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><rect id="node" x="10" y="10" width="100" height="100"/><text id="note" x="200" y="50">why</text></svg>`

// reviewFixture is a code repository holding an app Saga, with a feature
// branch that changes two files and a review of that branch whose two slides
// each reference one of them.
type reviewFixture struct {
	repo, root string
}

func run(t *testing.T, command func(context.Context, []string, io.Writer) error, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := command(context.Background(), args, &output); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, output.String())
	}
	return output.String()
}

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Dev")
	git(t, repo, "config", "user.email", "dev@example.test")
	writeFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"sqs\" }\n")
	writeFile(t, filepath.Join(repo, "store.go"), "package store\n\nfunc Table() string { return \"none\" }\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	root := filepath.Join(repo, "app.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatalf("init: %v\n%s", err, output.String())
	}
	addTestApp(t, root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "saga")
	git(t, repo, "checkout", "-b", "feature/pg")
	writeFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres\" }\n")
	writeFile(t, filepath.Join(repo, "store.go"), "package store\n\nfunc Table() string { return \"jobs\" }\n")
	git(t, repo, "commit", "-am", "Move the queue from SQS to a Postgres table")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	run(t, Review, "create", "--id", "pr-7", "--base", "main", "--head", "feature/pg", "--pr", "7", "--url", "https://github.com/acme/app/pull/7", "--title", "Move the queue to Postgres", root)
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, reviewSlideSVG)
	for _, slide := range []string{"queue", "table"} {
		run(t, AddSlide, "--review", "pr-7", "--intent", "explain", "--layout", "diagram", "--source", visual, root, slide)
		run(t, AddItem, "--review", "pr-7", "--slide", slide, "--kind", "node", "--element-id", "node", "--description", "The changed code", root)
	}
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "queue", "node"), "--ref", head+":queue.go#L3", "--repo", repo, root)
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "table", "node"), "--ref", head+":store.go#L3", "--repo", repo, root)
	assertValid(t, root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Review deck")
	return reviewFixture{repo: repo, root: root}
}

func reviewReport(t *testing.T, fixture reviewFixture) reviewstate.Report {
	t.Helper()
	var output bytes.Buffer
	if err := Review(context.Background(), []string{"list", "--review", "pr-7", "--json", fixture.root}, &output); err != nil {
		t.Fatalf("review list: %v\n%s", err, output.String())
	}
	var result reviewListOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || len(result.Reviews) != 1 {
		t.Fatalf("review list JSON: %v\n%s", err, output.String())
	}
	return result.Reviews[0]
}

func slideReport(t *testing.T, report reviewstate.Report, id string) reviewstate.SlideReport {
	t.Helper()
	for _, slide := range report.Slides {
		if slide.ID == id {
			return slide
		}
	}
	t.Fatalf("report has no slide %s: %#v", id, report.Slides)
	return reviewstate.SlideReport{}
}

func TestReviewDecisionsGoOutOfDateSlideBySlide(t *testing.T) {
	fixture := newReviewFixture(t)
	run(t, Review, "approve", "--review", "pr-7", "--slide", "queue", "--reviewer-kind", "human", "--body", "Clear transition", fixture.root)
	run(t, Review, "request-changes", "--review", "pr-7", "--slide", "table", "--reviewer-kind", "ai", "--reviewer-name", "Claude 1", "--agent", "claude-code", "--model", "claude-opus-5", "--body", "Show the index", fixture.root)
	git(t, fixture.repo, "add", ".")
	git(t, fixture.repo, "commit", "-m", "Review decisions")

	report := reviewReport(t, fixture)
	if report.Range == nil || report.Range.Following != "feature/pg" || report.Range.Frozen {
		t.Fatalf("review range = %#v", report.Range)
	}
	queue, table := slideReport(t, report, "queue"), slideReport(t, report, "table")
	if len(queue.Decisions) != 1 || queue.Decisions[0].State != saga.ApprovalApproved || queue.Decisions[0].Currency != reviewstate.Current {
		t.Fatalf("queue decisions = %#v", queue.Decisions)
	}
	if len(table.Decisions) != 1 || table.Decisions[0].State != saga.ApprovalChangesRequested || table.Decisions[0].Reviewer.Model != "claude-opus-5" {
		t.Fatalf("table decisions = %#v", table.Decisions)
	}

	// A push that changes the code slide one references puts only that
	// slide's decision out of date.
	writeFile(t, filepath.Join(fixture.repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres-v2\" }\n")
	git(t, fixture.repo, "commit", "-am", "Rename the queue table")
	report = reviewReport(t, fixture)
	queue, table = slideReport(t, report, "queue"), slideReport(t, report, "table")
	if queue.Decisions[0].Currency != reviewstate.OutOfDate || !strings.Contains(strings.Join(queue.Decisions[0].Reasons, ";"), "queue.go lines 3-3") {
		t.Fatalf("changed code did not put slide one out of date: %#v", queue.Decisions[0])
	}
	if table.Decisions[0].Currency != reviewstate.Current || table.Decisions[0].State != saga.ApprovalChangesRequested {
		t.Fatalf("slide two changed state: %#v", table.Decisions[0])
	}

	// Editing a slide after its decision puts that decision out of date too.
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, strings.Replace(reviewSlideSVG, "why", "why it moved", 1))
	run(t, SetSlideContent, "--review", "pr-7", "--target", "table", "--source", visual, fixture.root)
	table = slideReport(t, reviewReport(t, fixture), "table")
	if table.Decisions[0].Currency != reviewstate.OutOfDate || !strings.Contains(strings.Join(table.Decisions[0].Reasons, ";"), "the slide changed") {
		t.Fatalf("an edited slide kept a current decision: %#v", table.Decisions[0])
	}

	// A reviewer's later decision supersedes only their own.
	run(t, Review, "withdraw", "--review", "pr-7", "--slide", "table", "--reviewer-kind", "ai", "--reviewer-name", "Claude 1", "--agent", "claude-code", "--model", "claude-opus-5", fixture.root)
	if table = slideReport(t, reviewReport(t, fixture), "table"); len(table.Decisions) != 0 {
		t.Fatalf("a withdrawn decision is still reported: %#v", table.Decisions)
	}

	var text bytes.Buffer
	if err := Review(context.Background(), []string{"list", fixture.root}, &text); err != nil {
		t.Fatal(err)
	}
	for _, verdict := range []string{"ready", "approved review", "review is approved", "complete"} {
		if strings.Contains(strings.ToLower(text.String()), verdict) {
			t.Fatalf("review list states a verdict %q:\n%s", verdict, text.String())
		}
	}
	if !strings.Contains(text.String(), "out of date") {
		t.Fatalf("review list omitted currency:\n%s", text.String())
	}
}

func TestReviewCommentsThreadOnSlidesAndItemsOnly(t *testing.T) {
	fixture := newReviewFixture(t)
	output := run(t, Review, "comment", "--review", "pr-7", "--target", "table/node", "--body", "Is there an index on status?", "--reviewer-kind", "human", fixture.root)
	id := ""
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(line, "Comment: "); ok {
			id = value
		}
	}
	if id == "" {
		t.Fatalf("comment id was not printed:\n%s", output)
	}
	if got := slideReport(t, reviewReport(t, fixture), "table"); got.Comments != 1 || got.OpenThreads != 1 {
		t.Fatalf("slide discussion = %#v", got)
	}
	run(t, Review, "comment", "--review", "pr-7", "--reply-to", id, "--body", "Added in the migration.", "--resolve", "--reviewer-kind", "human", fixture.root)
	if got := slideReport(t, reviewReport(t, fixture), "table"); got.Comments != 2 || got.OpenThreads != 0 {
		t.Fatalf("resolved discussion = %#v", got)
	}
	var refused bytes.Buffer
	for _, target := range []string{saga.FragmentTarget("app", "app-overview"), "missing"} {
		if err := Review(context.Background(), []string{"comment", "--review", "pr-7", "--target", target, "--body", "no", "--reviewer-kind", "human", fixture.root}, &refused); err == nil {
			t.Fatalf("a comment on %q was accepted", target)
		}
	}
	if err := Review(context.Background(), []string{"approve", "--review", "pr-7", "--slide", "queue", fixture.root}, &refused); err == nil || !strings.Contains(err.Error(), "--reviewer-kind") {
		t.Fatalf("a decision without a reviewer seat = %v", err)
	}
	assertValid(t, fixture.root)
}

func TestReviewItemsReferenceRecordsAndNeverCountAsCoverage(t *testing.T) {
	fixture := newReviewFixture(t)
	story := saga.ReviewItemTarget("app", "pr-7", "queue", "note")
	var output bytes.Buffer
	if err := AddItem(context.Background(), []string{"--review", "pr-7", "--slide", "queue", "--kind", "statement", "--element-id", "note", "--description", "The epic this revises", "--record", "urn:change-saga:app:epic:" + testEpic, fixture.root}, &output); err != nil {
		t.Fatalf("review item record: %v\n%s", err, output.String())
	}
	if err := AddItem(context.Background(), []string{"--review", "pr-7", "--slide", "table", "--kind", "statement", "--element-id", "note", "--description", "x", "--record", "urn:change-saga:app:story:missing", fixture.root}, &output); err == nil {
		t.Fatal("a review item referencing a missing story was accepted")
	}
	document, validation, err := saga.Load(fixture.root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	review := document.FindReview("pr-7")
	if review == nil || review.Deck.Role != saga.DeckRoleReview || len(review.Deck.Slides) != 2 {
		t.Fatalf("review = %#v", review)
	}
	if item := review.Slide("queue").Items[1]; item.Target != story || item.Record != "urn:change-saga:app:epic:"+testEpic {
		t.Fatalf("record item = %#v", item)
	}
	// Review decks are not documentation: they are absent from the deck list
	// and the target tree coverage reads.
	walkTargets(document.Root, document.Section, func(target, _ string, _ bool) {
		if strings.Contains(target, ":review:") {
			t.Fatalf("review target %s joined the documentation tree", target)
		}
	})
	if len(document.Decks) != 0 {
		t.Fatalf("a review deck joined the documentation decks: %#v", document.Decks)
	}
}

func TestOnePullRequestHasOneReview(t *testing.T) {
	fixture := newReviewFixture(t)
	var output bytes.Buffer
	if err := Review(context.Background(), []string{"create", "--id", "again", "--base", "main", "--pr", "7", fixture.root}, &output); err == nil || !strings.Contains(err.Error(), "one review") {
		t.Fatalf("a second review of pull request 7 = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.root, saga.ReviewsDir, "again.review")); !os.IsNotExist(err) {
		t.Fatalf("a refused review left files behind: %v", err)
	}
}

func TestRepinFreezesTheLandedReviewAndHistoryLinksIt(t *testing.T) {
	fixture := newReviewFixture(t)
	persona := personaURNFor("app")
	run(t, AddItem, "--review", "pr-7", "--slide", "table", "--kind", "statement", "--element-id", "note", "--description", "Who the jobs table serves", "--record", persona, fixture.root)
	run(t, Review, "approve", "--review", "pr-7", "--slide", "queue", "--reviewer-kind", "human", fixture.root)
	git(t, fixture.repo, "add", ".")
	git(t, fixture.repo, "commit", "-m", "Review decisions")
	head := strings.TrimSpace(git(t, fixture.repo, "rev-parse", "HEAD"))
	fork := strings.TrimSpace(git(t, fixture.repo, "merge-base", "main", "HEAD"))
	git(t, fixture.repo, "checkout", "main")
	git(t, fixture.repo, "merge", "--squash", "feature/pg")
	git(t, fixture.repo, "commit", "-m", "Move the queue to Postgres (#7)")
	landed := strings.TrimSpace(git(t, fixture.repo, "rev-parse", "HEAD"))
	output := run(t, Repin, "--onto", "HEAD", fixture.root)
	if !strings.Contains(output, "Froze review pr-7") {
		t.Fatalf("repin did not freeze the review:\n%s", output)
	}
	git(t, fixture.repo, "add", ".")
	git(t, fixture.repo, "commit", "-m", "Re-pin after landing")
	git(t, fixture.repo, "branch", "-D", "feature/pg")

	report := reviewReport(t, fixture)
	if report.Merged == nil || report.Merged.Base != fork || report.Merged.Head != head || report.Merged.Landed != landed || report.Range == nil || !report.Range.Frozen || report.Range.HeadOID != head {
		t.Fatalf("frozen review = merged %#v range %#v", report.Merged, report.Range)
	}
	if queue := slideReport(t, report, "queue"); len(queue.Decisions) != 1 || queue.Decisions[0].Currency != reviewstate.Current {
		t.Fatalf("the frozen review lost its decisions: %#v", queue.Decisions)
	}
	var refused bytes.Buffer
	if err := Review(context.Background(), []string{"approve", "--review", "pr-7", "--slide", "table", "--reviewer-kind", "human", fixture.root}, &refused); err == nil {
		t.Fatal("a merged review accepted a new decision")
	}

	var history bytes.Buffer
	if err := Query(context.Background(), []string{"history", "--saga", fixture.root, "--node", persona}, &history); err != nil {
		t.Fatalf("history: %v\n%s", err, history.String())
	}
	var envelope struct {
		Data struct {
			Reviews []struct {
				ID      string   `json:"id"`
				Because []string `json:"because"`
			} `json:"reviews"`
		} `json:"data"`
	}
	if err := json.Unmarshal(history.Bytes(), &envelope); err != nil || len(envelope.Data.Reviews) != 1 || envelope.Data.Reviews[0].ID != "pr-7" {
		t.Fatalf("history did not link the review: %v\n%s", err, history.String())
	}
}
