package reviewstate

import (
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// A merge found once is kept: after main moves on and the branch that showed
// it is deleted, which on its own reads unknown, the ledger still says
// merged, in this run and the next.
func TestLedgerCarriesAMergeForward(t *testing.T) {
	t.Setenv(CacheDirEnv, t.TempDir())
	ctx := context.Background()
	dir, review := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge some work", "feature")

	ledger := OpenLedger(ctx, dir)
	if ledger == nil {
		t.Fatal("no ledger")
	}
	expectState(t, "found merged", NewLandings(dir).UseLedger(ledger).Detect(ctx, review), merged("main"))
	if err := ledger.Save(); err != nil {
		t.Fatal(err)
	}

	git(t, dir, "branch", "-q", "-D", "feature")
	write(t, dir, "later.txt", "later\n")
	commitAll(t, dir, "main moves on")
	expectState(t, "without the ledger", detect(t, dir, review), unknown)
	expectState(t, "from the saved ledger", NewLandings(dir).UseLedger(OpenLedger(ctx, dir)).Detect(ctx, review), merged("main"))
}

// Open reviews are never kept: a review the ledger has not seen merged is
// asked again.
func TestLedgerKeepsOnlyMerges(t *testing.T) {
	t.Setenv(CacheDirEnv, t.TempDir())
	ctx := context.Background()
	dir, review := landingRepo(t)
	ledger := OpenLedger(ctx, dir)
	expectState(t, "open", NewLandings(dir).UseLedger(ledger).Detect(ctx, review), open)
	if _, ok := ledger.lookup("change.saga/___reviews/pr-1.review/review.json"); ok || ledger.dirty {
		t.Fatal("the ledger kept an open review")
	}
}

// Prepare reads every record's landing in one walk and answers as Detect
// would alone.
func TestPrepareAgreesWithDetect(t *testing.T) {
	dir, first := landingRepo(t)
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge pull request #1 from acme/feature", "feature")
	git(t, dir, "checkout", "-q", "-b", "next")
	write(t, dir, "next.txt", "next\n")
	second := addReview(t, dir, "pr-2", 2, "next")
	commitAll(t, dir, "next change")
	git(t, dir, "checkout", "-q", "main")
	third := addReview(t, dir, "pr-3", 3, "later")
	commitAll(t, dir, "Review ahead of its change")
	reviews := []*saga.Review{first, second, third}
	landings := NewLandings(dir)
	landings.Prepare(context.Background(), reviews)
	for index, want := range []State{merged("main"), open, unknown} {
		expectState(t, reviews[index].ID, landings.Detect(context.Background(), reviews[index]), want)
	}
}
