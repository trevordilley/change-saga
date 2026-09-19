package reviewapp

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// Diff ownership reads a comparison's changed lines. Observing has none, so
// the query says a comparison is needed instead of reporting the line
// missing, and an unchanged line in a comparison says where to look instead.
func TestDiffOwnersExplainsWhenThereIsNoChangedLine(t *testing.T) {
	fixture := newServiceFixture(t)
	ctx := context.Background()
	observed, err := Open(ctx, OpenOptions{SagaRoot: fixture.root, SourceDir: fixture.repo, Range: gitdiff.Range{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = observed.DiffOwners(ctx, DiffOwnerQuery{Ref: fixture.atomRef})
	if ErrorCodeOf(err) != CodeInvalidArgument || !strings.Contains(err.Error(), "pass --against REV") {
		t.Fatalf("observe-mode diff-owners error = %v", err)
	}
	commit, _, _ := strings.Cut(fixture.atomRef, ":")
	_, err = fixture.session.DiffOwners(ctx, DiffOwnerQuery{Ref: commit + ":README.md#L1"})
	if ErrorCodeOf(err) != CodeNotFound || !strings.Contains(err.Error(), "query traceability --ref") {
		t.Fatalf("unchanged-line diff-owners error = %v", err)
	}
}
