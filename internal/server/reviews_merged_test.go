package server

import (
	"net/http"
	"net/http/httptest"
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
	// A set-aside review's coverage is read on its own page, not here.
	if merged := index[strings.Index(index, `data-review-summary="pr-7"`):]; strings.Contains(merged[:strings.Index(merged, "</article>")], "data-review-coverage-summary") {
		t.Fatal("the index read a merged review's coverage")
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
	if details := getPage(t, handler, "/reviews/pr-7/summary").Body.String(); !strings.Contains(details, `0 of 0 changed lines explained by the deck`) {
		t.Fatalf("the compared review's details omitted its coverage:\n%s", details)
	}
	if !strings.Contains(index, `<tr class="current" data-directory-row="pr-7" data-directory-archived-row`) || !strings.Contains(index, `data-review-state-badge="merged"`) || strings.Contains(index, "merged hidden") || !strings.Contains(index, `data-nav-row="nav-review-target-pr-7-`) || strings.Contains(index, `nav-reviews-merged`) {
		t.Fatalf("the compared review was set aside:\n%s", index)
	}
}

// A merged review's own page keeps its row in the sidebar, like the compared
// review, so the reader can see where they are.
func TestSidebarKeepsTheViewedMergedReview(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	page := getPage(t, handler, "/reviews/pr-7").Body.String()
	if !strings.Contains(page, `data-nav-row="nav-review-target-pr-7-`) {
		t.Fatalf("the viewed merged review lost its sidebar row:\n%s", page)
	}
	if index := getPage(t, handler, "/reviews").Body.String(); strings.Contains(index, `data-nav-row="nav-review-target-pr-7-`) {
		t.Fatal("the merged review kept its row after the reader left it")
	}
}

// The sidebar's merged reviews are kept while the Saga and the refs are
// unchanged, and follow the refs when a merge moves them.
func TestSidebarFollowsAMergeOfAnOpenReview(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	for range 2 {
		if index := getPage(t, handler, "/reviews").Body.String(); !strings.Contains(index, `data-nav-row="nav-review-target-pr-8-`) || !strings.Contains(index, `>1 merged</a>`) {
			t.Fatalf("before pr-8 merged:\n%s", index)
		}
	}
	serverGit(t, fixture.repo, "checkout", "main")
	serverGit(t, fixture.repo, "merge", "--no-ff", "-m", "Merge pull request #8 from acme/feature/next", "feature/next")
	if index := getPage(t, handler, "/reviews").Body.String(); strings.Contains(index, `data-nav-row="nav-review-target-pr-8-`) || !strings.Contains(index, `>2 merged</a>`) {
		t.Fatalf("after pr-8 merged:\n%s", index)
	}
}

// A review created a moment ago has no slides yet. Its page says so and
// names the command that adds the first, instead of failing.
func TestReviewWithNoSlidesRendersItsEmptyDeck(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	page := getPage(t, handler, "/reviews/pr-8").Body.String()
	for _, want := range []string{`data-review-empty="pr-8"`, `Name the queue table`, `apply-slide --review pr-8`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the empty review page is missing %q:\n%s", want, page)
		}
	}
}

// The index opens knowing each review's state, and reads each review's
// range, decisions, and coverage after, once its row is in view: its row's
// cells and its card wait with a spinner, and one request fills them all.
func TestReviewIndexReadsDetailsAfterThePage(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	index := getPage(t, handler, "/reviews").Body.String()
	for _, want := range []string{
		`<span class="directory-load" hx-get="/reviews/pr-8/summary" hx-trigger="intersect once" hx-target="this" hx-swap="none" data-directory-load aria-hidden="true"></span>`,
		`<tr hidden data-directory-row="pr-7" data-directory-archived-row data-directory-text=`,
		`hx-get="/reviews/pr-7/summary"`,
		`<td id="` + reviewCellIDs("pr-8").rng + `"><span class="directory-pending" role="status" aria-label="Loading">`,
		`id="` + reviewDetailsID("pr-8") + `"><p class="review-loading" role="status">`,
		`href="/reviews?details=all"`,
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("the index is missing %q:\n%s", want, index)
		}
	}
	details := getPage(t, handler, "/reviews/pr-8/summary").Body.String()
	ids := reviewCellIDs("pr-8")
	for _, target := range []string{reviewDetailsID("pr-8"), ids.rng, ids.decisions, ids.outOfDate} {
		if !strings.Contains(details, `hx-swap-oob="innerHTML:#`+target+`"`) {
			t.Fatalf("the details do not fill %s:\n%s", target, details)
		}
	}
	if !strings.Contains(details, "data-review-coverage-summary") {
		t.Fatalf("an open review's details omitted its coverage:\n%s", details)
	}
	// A merged review's coverage is read on its own page.
	if merged := getPage(t, handler, "/reviews/pr-7/summary").Body.String(); strings.Contains(merged, "data-review-coverage-summary") {
		t.Fatalf("a merged review's details read its coverage:\n%s", merged)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/reviews/nothing/summary", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("details of an unknown review = %d", missing.Code)
	}
}

// Every page carries the spinner a slow navigation shows, hidden until the
// script shows it; it sits outside #page, so it outlives the swap.
func TestShellCarriesThePageLoadingIndicator(t *testing.T) {
	t.Parallel()
	fixture := newMergedReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	page := getPage(t, handler, "/reviews").Body.String()
	indicator := `<div class="page-loading" id="page-loading" role="status" hidden>`
	if !strings.Contains(page, indicator) || strings.Index(page, indicator) < strings.Index(page, `<div id="page"`) {
		t.Fatalf("the shell has no page-loading indicator outside #page:\n%s", page)
	}
}
