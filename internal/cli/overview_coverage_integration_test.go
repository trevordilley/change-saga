package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// A complete report is the outer link in the explanation chain. It cannot
// manufacture the exact Item evidence needed by the independent code check.
func TestOverviewCoverageRequiresIndependentItemEvidence(t *testing.T) {
	t.Parallel()
	fixture, _ := newEmptyReviewFixture(t)
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	if _, err := ApplySlideTransaction(context.Background(), fixture.root, t.TempDir(), fixture.repo,
		reviewSlideRequest("flow-create", "create", "absent", reviewDiagram()), false); err != nil {
		t.Fatal(err)
	}
	before := reviewReport(t, fixture).Coverage.Summary
	if before.Total == 0 || before.Uncovered == 0 {
		t.Fatalf("fixture has no uncovered code: %+v", before)
	}
	overview := saga.DeckOverview{
		Body:        "# Architecture\n\n[Queue ownership](annotation:queue) explains the request path.\n\n![Request flow](slide:flow)",
		Annotations: []saga.OverviewAnnotation{{ID: "queue", Label: "Queue", Slide: "flow", Item: "queue"}},
	}
	data, err := json.Marshal(overview)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "overview.json")
	writeFile(t, source, string(data))
	run(t, Deck, "overview", "--review", "pr-7", "--file", source, fixture.root)
	run(t, Deck, "overview", "--review", "pr-7", "--check", fixture.root)
	after := reviewReport(t, fixture).Coverage.Summary
	if after.Total != before.Total || after.Uncovered != before.Uncovered || after.Covered != before.Covered {
		t.Fatalf("overview changed exact code coverage: before=%+v after=%+v", before, after)
	}
	for _, entry := range []struct{ item, path string }{{"queue", "queue.go"}, {"table", "store.go"}} {
		run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "flow", entry.item), "--path", entry.path, "--changed-lines", "--repo", fixture.repo, fixture.root)
	}
	covered := reviewReport(t, fixture).Coverage.Summary
	if covered.Total != before.Total || covered.Uncovered != 0 || covered.Covered != covered.Total {
		t.Fatalf("Item evidence did not complete the chain: %+v", covered)
	}
	run(t, Deck, "overview", "--review", "pr-7", "--check", fixture.root)
}
