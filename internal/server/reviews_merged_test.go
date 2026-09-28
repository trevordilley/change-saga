package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

// newMergedReviewFixture merges pr-7's branch into main without repin
// --onto, then opens pr-8 on a new branch: one review Git shows landed and
// one still open.
func newMergedReviewFixture(t *testing.T) serverReviewFixture {
	t.Helper()
	fixture := newServerReviewFixture(t)
	serverGit(t, fixture.repo, "checkout", "main")
	serverGit(t, fixture.repo, "merge", "--no-ff", "-m", "Merge pull request #7", "feature/pg")
	serverGit(t, fixture.repo, "checkout", "-b", "feature/next")
	writeServerFile(t, filepath.Join(fixture.repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres-v2\" }\n")
	if err := reviewstore.Create(fixture.root, saga.ReviewManifest{ID: "pr-8", Title: "Name the queue table", Base: "main", Head: "feature/next", PullRequest: &saga.PullRequest{Number: 8}}, saga.DeckManifest{ID: "pr-8", Title: "PR 8", Objective: "Explain the rename."}); err != nil {
		t.Fatal(err)
	}
	serverGit(t, fixture.repo, "add", ".")
	serverGit(t, fixture.repo, "commit", "-m", "Name the queue table")
	return fixture
}

// A review whose change merged shows Merged and is set aside: hidden until
// a reader searches for it or asks for the merged ones, after the open
// reviews, which keep their place.
func TestReviewIndexSetsMergedReviewsAside(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	index := getPage(t, handler, "/reviews").Body.String()
	for _, want := range []string{
		`<tr data-directory-row="pr-8"`,
		`<tr hidden data-directory-row="pr-7" data-directory-archived-row`,
		`<article class="review-summary" hidden data-review-summary="pr-7"`,
		`data-review-state-badge="merged">Merged</span>`,
		`<caption data-directory-caption>1 review · 1 merged hidden</caption>`,
		`name="archived" value="show" data-directory-archived> Show 1 merged</label>`,
		`data-directory-archived-noun="merged"`,
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("review index is missing %q:\n%s", want, index)
		}
	}
	// The sidebar sets the merged review aside the same way, and counts it
	// in one row that opens the Reviews page showing it.
	for _, want := range []string{`data-nav-row="nav-review-target-pr-8-`, `data-nav-row="nav-reviews-merged"`, `href="/reviews?archived=show"`, `>1 merged</a>`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the sidebar is missing %q:\n%s", want, index)
		}
	}
	if strings.Contains(index, `data-nav-row="nav-review-target-pr-7-`) {
		t.Fatal("the sidebar still lists the merged review")
	}
	if strings.Index(index, `data-directory-row="pr-8"`) > strings.Index(index, `data-directory-row="pr-7"`) {
		t.Fatal("a merged review came before an open one")
	}
	if !strings.Contains(index, `data-directory-none hidden`) {
		t.Fatal("an unfiltered index said nothing matches")
	}

	for _, path := range []string{"/reviews?q=postgres", "/reviews?q=pr-7", "/reviews?archived=show"} {
		body := getPage(t, handler, path).Body.String()
		if !strings.Contains(body, `<tr data-directory-row="pr-7" data-directory-archived-row`) || strings.Contains(body, `<article class="review-summary" hidden data-review-summary="pr-7"`) {
			t.Fatalf("%s did not show the merged review:\n%s", path, body)
		}
	}
	if body := getPage(t, handler, "/reviews?q=table").Body.String(); !strings.Contains(body, `<tr hidden data-directory-row="pr-7"`) || !strings.Contains(body, `<tr data-directory-row="pr-8"`) {
		t.Fatalf("a search that matches only the open review showed the merged one:\n%s", body)
	}
}

// The review this reader was opened to compare stays in view even once its
// change has landed.
func TestReviewIndexKeepsTheComparedMergedReviewInView(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{Against: "main~1", Head: "feature/pg"})
	index := getPage(t, handler, "/reviews").Body.String()
	if !strings.Contains(index, `<tr class="current" data-directory-row="pr-7" data-directory-archived-row`) || !strings.Contains(index, `data-review-state-badge="merged"`) || strings.Contains(index, "merged hidden") || !strings.Contains(index, `data-nav-row="nav-review-target-pr-7-`) || strings.Contains(index, `nav-reviews-merged`) {
		t.Fatalf("the compared review was set aside:\n%s", index)
	}
}
