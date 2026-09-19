package livingapp

import (
	"reflect"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// A criterion's traceability includes design that addresses its whole story,
// listed again as broad, and each verifying test case whose current run
// passed, with the run on its path.
func TestTraceabilityReachesStoryDesignAndPassingTests(t *testing.T) {
	design := "urn:change-saga:checkout:chapter:refund-design"
	failing := requirements.Relation{ID: "failing-verifies", Type: requirements.RelationVerifies, From: testURN("failing"), To: criterionURN("cutoff"), State: requirements.RelationActive}
	s := &session{
		saga: &saga.Saga{},
		plan: workplan.Plan{},
		requirements: requirements.Document{SagaID: fixtureSaga, Stories: []requirements.Story{refundStory("cutoff")}, Relations: []requirements.Relation{
			{ID: "design-addresses", Type: requirements.RelationAddresses, From: design, To: storyURN, State: requirements.RelationActive},
			verifies("cutoff-verifies", "cutoff-test", "r1", "cutoff", storyR2),
			failing,
		}},
		quality: quality.Document{TestCases: []quality.TestCase{
			{Identity: quality.TestCaseIdentity{ID: "cutoff-test"}, CurrentRun: &quality.Run{ID: "ci-1", Result: quality.RunPassed}},
			{Identity: quality.TestCaseIdentity{ID: "failing"}, CurrentRun: &quality.Run{ID: "ci-1", Result: quality.RunFailed}},
		}},
	}
	rows, _ := s.traceRows(Filters{Kind: "cutoff"})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	row := rows[0]
	if !reflect.DeepEqual(row.Design, []string{design}) || !reflect.DeepEqual(row.BroadDesign, []string{design}) {
		t.Fatalf("story-level design = %v, broad = %v", row.Design, row.BroadDesign)
	}
	if !reflect.DeepEqual(row.Evidence, []string{testURN("cutoff-test")}) {
		t.Fatalf("evidence = %v, want only the passing test", row.Evidence)
	}
	wantPaths := [][]string{{criterionURN("cutoff"), storyURN, design}, {criterionURN("cutoff"), testURN("cutoff-test"), testURN("cutoff-test") + ":run:ci-1"}}
	for _, want := range wantPaths {
		found := false
		for _, path := range row.Paths {
			found = found || reflect.DeepEqual(path, want)
		}
		if !found {
			t.Fatalf("paths %v lack %v", row.Paths, want)
		}
	}
}

// With a locator, a code location at any commit finds the evidence whose
// lines remapped there; without one only the pinned commit matches.
func TestTraceabilityRefMatchesRemappedEvidence(t *testing.T) {
	pinned := coderef.Reference{Commit: fixtureBase, Path: "refund.go", Start: 3, End: 4}
	value := reviewEvidence{Reference: pinned, Location: pinned.Location()}
	moved := coderef.Location{Commit: fixtureHead, Path: "refund.go", Start: 5, End: 6}
	s := &session{}
	ref := coderef.Location{Commit: fixtureHead, Path: "refund.go", Start: 6, End: 6}.String()
	if s.reviewEvidenceMatches(value, Filters{Ref: ref}) {
		t.Fatal("without a locator a location at another commit matched")
	}
	remapped := func(coderef.Reference) (coderef.Location, bool) { return moved, true }
	if !s.reviewEvidenceMatches(value, Filters{Ref: ref, Locate: remapped}) {
		t.Fatal("a location on the remapped lines did not match")
	}
	stale := func(coderef.Reference) (coderef.Location, bool) { return pinned.Location(), false }
	if s.reviewEvidenceMatches(value, Filters{Ref: ref, Locate: stale}) {
		t.Fatal("evidence whose lines changed matched")
	}
}
