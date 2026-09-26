package server

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A review's coverage is read once for its range and its Items' code, and
// reused while neither changes: the reviews index no longer diffs every
// review's range on every visit. What it reuses is what a fresh read gives.
func TestReviewCoverageIsReadOncePerRangeAndItems(t *testing.T) {
	fixture := newServerReviewFixture(t)
	application, handler := reviewApp(t, fixture, gitdiff.Range{})
	getPage(t, handler, "/reviews")
	first := application.reviewCoverages.reads
	if first == 0 {
		t.Fatal("the reviews index read no coverage")
	}
	getPage(t, handler, "/reviews")
	getPage(t, handler, "/reviews/pr-7")
	if application.reviewCoverages.reads != first {
		t.Fatalf("an unchanged review's coverage was read again: %d reads, want %d", application.reviewCoverages.reads, first)
	}

	ctx := context.Background()
	document, _, err := saga.Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("pr-7")
	rng, err := reviewstate.ResolveRange(ctx, fixture.root, review)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	repository := document.Manifest.Source.Repository
	cached, err := application.reviewCoverage(ctx, review, rng, repository, resolver)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := reviewstate.ReadCoverage(ctx, review, rng, fixture.root, repository, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cached, fresh) {
		t.Fatal("the kept coverage differs from a fresh read")
	}

	// The key is what coverage reads: another range, or an Item's code, is
	// another entry.
	key, _ := reviewCoverageKey(review, rng, repository, "")
	moved := rng
	moved.HeadOID = moved.BaseOID
	if other, _ := reviewCoverageKey(review, moved, repository, ""); other == key {
		t.Fatal("a moved range kept the same coverage key")
	}
	for _, slide := range review.Deck.Slides {
		for index := range slide.Items {
			if len(slide.Items[index].Code) > 0 {
				slide.Items[index].Code = nil
				if edited, _ := reviewCoverageKey(review, rng, repository, ""); edited == key {
					t.Fatal("an Item's changed code kept the same coverage key")
				}
				return
			}
		}
	}
	t.Fatal("the fixture review has no Item with code")
}

// Git reads the checkout's attribute files when it diffs two commits, so a
// review's kept coverage is not reused once they change: a file Git now
// calls binary has no lines to cover.
func TestReviewCoverageFollowsCheckoutAttributes(t *testing.T) {
	fixture := newServerReviewFixture(t)
	application, _ := reviewApp(t, fixture, gitdiff.Range{})
	ctx := context.Background()
	document, _, err := saga.Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("pr-7")
	rng, err := reviewstate.ResolveRange(ctx, fixture.root, review)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	repository := document.Manifest.Source.Repository
	before, err := application.reviewCoverage(ctx, review, rng, repository, resolver)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, filepath.Join(fixture.repo, ".gitattributes"), "* binary\n")
	after, err := application.reviewCoverage(ctx, review, rng, repository, resolver)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := reviewstate.ReadCoverage(ctx, review, rng, fixture.root, repository, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(before, fresh) {
		t.Fatal("marking every file binary did not change a fresh read; the test proves nothing")
	}
	if !reflect.DeepEqual(after, fresh) {
		t.Fatalf("coverage kept from before the attributes changed was reused: got %+v, fresh read %+v", after, fresh)
	}
}

// A running server reads the reviews' coverage in the background, so the
// first visit to the reviews index finds it already read.
func TestAWatchedSagaReadsReviewCoverageBeforeAnyoneAsks(t *testing.T) {
	fixture := newServerReviewFixture(t)
	application, handler := reviewApp(t, fixture, gitdiff.Range{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { application.watchSaga(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	reads := func() int {
		application.reviewCoverages.mutex.Lock()
		defer application.reviewCoverages.mutex.Unlock()
		return application.reviewCoverages.reads
	}
	deadline := time.Now().Add(30 * time.Second)
	for reads() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the watched Saga never read the reviews' coverage")
		}
		time.Sleep(10 * time.Millisecond)
	}
	warmed := reads()
	getPage(t, handler, "/reviews")
	if reads() != warmed {
		t.Fatalf("the first visit read coverage the watcher had read: %d reads, want %d", reads(), warmed)
	}
}

// Coverage that rests on a pinned commit the repository lacks may change after
// a fetch, so it is shown but not kept; nor is coverage read for a request
// that was abandoned.
func TestReviewCoverageKeepsNoProvisionalAnswer(t *testing.T) {
	fixture := newServerReviewFixture(t)
	application, _ := reviewApp(t, fixture, gitdiff.Range{})
	ctx := context.Background()
	document, _, err := saga.Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("pr-7")
	rng, err := reviewstate.ResolveRange(ctx, fixture.root, review)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	repository := document.Manifest.Source.Repository
	pinned := false
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for file := range item.Code {
				for reference := range item.Code[file].References {
					item.Code[file].References[reference].Commit = strings.Repeat("0", 40)
					pinned = true
				}
			}
		}
	}
	if !pinned {
		t.Fatal("the fixture review has no code reference")
	}
	for range 2 {
		if _, err := application.reviewCoverage(ctx, review, rng, repository, resolver); err != nil {
			t.Fatal(err)
		}
	}
	if application.reviewCoverages.reads != 2 || len(application.reviewCoverages.entries) != 0 {
		t.Fatalf("provisional coverage was kept: %d reads, %d entries", application.reviewCoverages.reads, len(application.reviewCoverages.entries))
	}
	abandoned, cancel := context.WithCancel(ctx)
	cancel()
	fresh := document.FindReview("pr-7")
	application.reviewCoverage(abandoned, fresh, rng, repository, resolver)
	if len(application.reviewCoverages.entries) != 0 {
		t.Fatal("coverage read for an abandoned request was kept")
	}
}
