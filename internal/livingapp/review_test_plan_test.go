package livingapp

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

func testPlanFixture() (*session, *saga.Review, gitdiff.ChangeSet) {
	item := &saga.Item{Target: "urn:change-saga:checkout:review:pr-1:slide:change:item:code",
		ItemManifest: saga.ItemManifest{ID: "code", Record: storyURN},
		Code:         []saga.CodeFile{{References: []coderef.Reference{{Commit: fixtureBase, Path: "refund.go", Start: 3, End: 3}}}}}
	review := &saga.Review{Target: "urn:change-saga:checkout:review:pr-1", Deck: &saga.Deck{Slides: []*saga.Slide{{Items: []*saga.Item{item}}}}}
	s := &session{saga: &saga.Saga{Section: &saga.Section{Target: "urn:change-saga:checkout:saga"}},
		requirements: requirements.Document{SagaID: fixtureSaga, Stories: []requirements.Story{refundStory("cutoff", "eligible")}}}
	for _, id := range []string{"unit", "e2e", "unrelated"} {
		test := quality.TestCase{Identity: quality.TestCaseIdentity{ID: id},
			CurrentRevision:  &quality.Revision{ID: "r1", Title: id, Automation: quality.AutomationAutomated},
			CurrentLifecycle: &quality.LifecycleEvent{State: quality.StateActive},
			EvidenceHeads:    []string{testURN(id) + ":evidence:code"},
			Evidence: []quality.Evidence{{ID: "code", Role: quality.EvidenceTestImplementation, TestRevision: testRevisionURN(id, "r1"),
				Code: []coderef.Reference{{Commit: fixtureHead, Path: id + "_test.go", Start: 5, End: 8}}}},
			Runs: []quality.Run{{ID: "ci", TestRevision: testRevisionURN(id, "r1"), Command: "go test -run " + id, ExecutedAt: statusFixtureTime}}}
		s.quality.TestCases = append(s.quality.TestCases, test)
	}
	s.requirements.Relations = []requirements.Relation{
		verifies("unit-cutoff", "unit", "r1", "cutoff", storyR2),
		verifies("e2e-eligible", "e2e", "r1", "eligible", storyR2),
		verifies("unit-eligible", "unit", "r1", "eligible", storyR2),
	}
	changes := gitdiff.ChangeSet{BaseOID: fixtureBase, HeadOID: fixtureHead, Atoms: []gitdiff.Atom{{Key: "deleted", Kind: "line", Path: "refund.go", Side: "old", Line: 3}}}
	return s, review, changes
}

func runTestPlan(s *session, review *saga.Review, changes gitdiff.ChangeSet) ReviewTestPlan {
	return s.reviewTestPlan(context.Background(), review, reviewstate.Range{BaseOID: changes.BaseOID, HeadOID: changes.HeadOID}, changes, coderesolve.Pinned{}, nil)
}

func TestReviewTestPlanSelectsWholeStoryDeduplicatesAndPreservesCommands(t *testing.T) {
	s, review, changes := testPlanFixture()
	plan := runTestPlan(s, review, changes)
	if !plan.Complete || len(plan.Stories) != 1 || len(plan.Tests) != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.Tests[0].TestCase != testURN("e2e") || plan.Tests[1].TestCase != testURN("unit") {
		t.Fatalf("tests: %+v", plan.Tests)
	}
	if len(plan.Tests[1].Criteria) != 2 || len(plan.Tests[1].Code) != 1 || plan.Tests[1].Commands[0].Command != "go test -run unit" {
		t.Fatalf("dedup/code/command: %+v", plan.Tests[1])
	}
	if !reflect.DeepEqual(plan.Stories[0].Via, [][]string{{review.Deck.Slides[0].Items[0].Target, storyURN}}) {
		t.Fatal(plan.Stories)
	}
	first, _ := json.Marshal(plan)
	for i := 0; i < 10; i++ {
		next, _ := json.Marshal(runTestPlan(s, review, changes))
		if string(first) != string(next) {
			t.Fatal("nondeterministic plan")
		}
	}
	// Linking only one criterion still selects tests of sibling criteria in its story.
	review.Deck.Slides[0].Items[0].Record = criterionURN("cutoff")
	if got := runTestPlan(s, review, changes); len(got.Tests) != 2 {
		t.Fatalf("criterion did not reach whole story: %+v", got)
	}
}

func TestReviewTestPlanFindsLivingDocumentationWithoutReviewStoryLink(t *testing.T) {
	s, review, changes := testPlanFixture()
	review.Deck.Slides[0].Items[0].Record = ""
	owner := "urn:change-saga:checkout:fragment:refund"
	s.saga.Section.Fragments = []*saga.Fragment{{Target: owner, Code: review.Deck.Slides[0].Items[0].Code}}
	s.requirements.Relations = append(s.requirements.Relations, requirements.Relation{ID: "explains", Type: requirements.RelationAddresses, State: requirements.RelationActive, From: owner, To: criterionURN("cutoff")})
	if got := runTestPlan(s, review, changes); !got.Complete || len(got.Tests) != 2 {
		t.Fatalf("living path: %+v", got)
	}
	// An unrelated review is never consulted.
	s.saga.Section.Fragments = nil
	s.saga.Reviews = []*saga.Review{review}
	if got := runTestPlan(s, &saga.Review{}, changes); len(got.Tests) != 0 || got.Complete {
		t.Fatalf("unrelated review leaked: %+v", got)
	}
}

func TestReviewTestPlanReportsGapsWithoutHidingCandidates(t *testing.T) {
	tests := []struct {
		name, code string
		mutate     func(*session, *saga.Review, *gitdiff.ChangeSet)
	}{
		{"unmapped", "unmapped_change", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) { r.Deck.Slides[0].Items[0].Record = "" }},
		{"missing code", "missing_test_code", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) { s.quality.TestCases[0].Evidence = nil }},
		{"stale code", "stale_test_code", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) {
			s.quality.TestCases[0].Evidence[0].Code[0].Commit = fixtureBase
		}},
		{"old evidence revision", "stale_test_evidence", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) {
			s.quality.TestCases[0].Evidence[0].TestRevision = testRevisionURN("unit", "r0")
		}},
		{"manual", "manual_test", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) {
			s.quality.TestCases[0].CurrentRevision.Automation = quality.AutomationManual
		}},
		{"hybrid", "hybrid_test", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) {
			s.quality.TestCases[0].CurrentRevision.Automation = quality.AutomationHybrid
		}},
		{"no command", "missing_command", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) { s.quality.TestCases[0].Runs = nil }},
		{"conflicted test", "conflicted_test", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) { s.quality.TestCases[0].CurrentRevision = nil }},
		{"no tests", "no_automated_tests", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) { s.quality.TestCases = nil }},
		{"stale link", "stale_test_link", func(s *session, r *saga.Review, c *gitdiff.ChangeSet) {
			s.currency = []requirements.RelationCurrency{{Relation: relationURN("unit-cutoff"), Status: requirements.CurrencyStale}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, r, c := testPlanFixture()
			tt.mutate(s, r, &c)
			p := runTestPlan(s, r, c)
			found := false
			for _, g := range p.Gaps {
				found = found || g.Code == tt.code
			}
			if p.Complete || !found {
				t.Fatalf("want %s: %+v", tt.code, p)
			}
		})
	}
}

func TestReviewTestPlanIncludesChangedTestAndExcludesRetiredAndSupersededLinks(t *testing.T) {
	s, r, c := testPlanFixture()
	s.quality.TestCases[0].CurrentLifecycle.State = quality.StateRetired
	s.requirements.Relations[1].State = requirements.RelationSuperseded
	c.Atoms = append(c.Atoms, gitdiff.Atom{Key: "test", Kind: "line", Path: "unrelated_test.go", Side: "new", Line: 6})
	p := runTestPlan(s, r, c)
	if len(p.Tests) != 1 || p.Tests[0].TestCase != testURN("unrelated") {
		t.Fatalf("tests: %+v", p.Tests)
	}
	if !strings.Contains(strings.Join(p.Tests[0].Reasons, " "), "intersects") {
		t.Fatal(p.Tests)
	}
	found := false
	for _, g := range p.Gaps {
		found = found || g.Code == "unlinked_test"
	}
	if !found {
		t.Fatal(p.Gaps)
	}
}

func TestReviewTestPlanFileDependenciesAndEvents(t *testing.T) {
	for _, event := range []string{"add", "delete", "rename"} {
		t.Run(event, func(t *testing.T) {
			s, r, c := testPlanFixture()
			atom := gitdiff.Atom{Key: "event", Kind: "event", Event: event, Path: "refund.go"}
			if event == "add" {
				r.Deck.Slides[0].Items[0].Code[0].References[0].Commit = fixtureHead
			} else if event == "rename" {
				atom.Path, atom.NewPath, atom.OldPath = "renamed.go", "renamed.go", "refund.go"
			}
			c.Atoms = []gitdiff.Atom{atom}
			if got := runTestPlan(s, r, c); !got.Complete || len(got.Tests) != 2 {
				t.Fatalf("file event must affect contained evidence: %+v", got)
			}
			// Mapping the event must never hide a changed line outside the range.
			c.Atoms = append(c.Atoms, gitdiff.Atom{Key: "outside", Kind: "line", Path: "refund.go", Side: "old", Line: 99})
			if got := runTestPlan(s, r, c); got.Complete {
				t.Fatalf("unrelated line was silently mapped: %+v", got)
			}
		})
	}
	s, r, c := testPlanFixture()
	ref := &r.Deck.Slides[0].Items[0].Code[0].References[0]
	ref.Start, ref.End = 0, 0
	c.Atoms[0].Line = 99
	if got := runTestPlan(s, r, c); !got.Complete || len(got.Tests) != 2 {
		t.Fatalf("whole-file dependency must include all changed lines: %+v", got)
	}
}

func TestReviewTestPlanUsesLatestRevisionCommandAndDeduplicatesSuites(t *testing.T) {
	s, r, c := testPlanFixture()
	for i := range s.quality.TestCases {
		test := &s.quality.TestCases[i]
		test.Runs = append(test.Runs, quality.Run{ID: "new", TestRevision: testRevisionURN(test.Identity.ID, "r1"), Command: "go test ./...", ExecutedAt: statusFixtureTime.Add(1)},
			quality.Run{ID: "other-revision", TestRevision: testRevisionURN(test.Identity.ID, "r0"), Command: "wrong command", ExecutedAt: statusFixtureTime.Add(2)})
	}
	p := runTestPlan(s, r, c)
	if !p.Complete || !reflect.DeepEqual(p.Commands, []string{"go test ./..."}) {
		t.Fatalf("suite command selection: %+v", p)
	}
	for _, test := range p.Tests {
		if len(test.Commands) != 1 || test.Commands[0].Run != test.TestCase+":run:new" {
			t.Fatalf("command provenance: %+v", test)
		}
	}
}

func TestReviewTestPlanFollowsInheritedInventoryEvidenceAndDeckStoryLinks(t *testing.T) {
	s, r, c := testPlanFixture()
	item := &saga.Item{Target: "urn:change-saga:checkout:deck:implementation:slide:flow:item:refund"}
	deck := &saga.Deck{Target: "urn:change-saga:checkout:deck:implementation", Slides: []*saga.Slide{{Target: "urn:change-saga:checkout:deck:implementation:slide:flow", Items: []*saga.Item{item}}}}
	s.saga.Decks = []*saga.Deck{deck}
	s.requirements.Relations = append(s.requirements.Relations, requirements.Relation{ID: "explains-deck", Type: requirements.RelationExplains, State: requirements.RelationActive, From: deck.Target, To: storyURN})
	inherited := []coverage.InheritedReference{{Inheritance: coverage.Inheritance{Item: item.Target}, Reference: r.Deck.Slides[0].Items[0].Code[0].References[0]}}
	r.Deck = nil
	p := s.reviewTestPlan(context.Background(), r, reviewstate.Range{BaseOID: c.BaseOID, HeadOID: c.HeadOID}, c, coderesolve.Pinned{}, inherited)
	if !p.Complete || len(p.Tests) != 2 || len(p.Stories) != 1 || p.Stories[0].Via[0][0] != deck.Target {
		t.Fatalf("inventory selection did not reach the deck's story: %+v", p)
	}
}
