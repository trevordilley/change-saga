package server

import (
	"context"
	"reflect"
	"testing"

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
	key, _ := reviewCoverageKey(review, rng, repository)
	moved := rng
	moved.HeadOID = moved.BaseOID
	if other, _ := reviewCoverageKey(review, moved, repository); other == key {
		t.Fatal("a moved range kept the same coverage key")
	}
	for _, slide := range review.Deck.Slides {
		for index := range slide.Items {
			if len(slide.Items[index].Code) > 0 {
				slide.Items[index].Code = nil
				if edited, _ := reviewCoverageKey(review, rng, repository); edited == key {
					t.Fatal("an Item's changed code kept the same coverage key")
				}
				return
			}
		}
	}
	t.Fatal("the fixture review has no Item with code")
}
