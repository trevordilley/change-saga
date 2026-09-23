package livingapp

import (
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

func TestFeatureAuditFindsEveryBroadLinkAndExactAuthoringGaps(t *testing.T) {
	s := auditFixture(t, true)
	story := "urn:change-saga:test:story:checkout"
	c1 := story + ":criterion:fast"
	c2 := story + ":criterion:safe"
	deck := saga.DeckTarget("test", "implementation")
	slide := saga.SlideTarget("test", "flow")
	item := saga.ItemTarget("test", "flow", "handler")

	// These are the three legacy story-level links the old handoff review
	// missed, followed by newer broad links to exact criteria.
	links := []requirements.Relation{
		auditRelation("legacy-deck-addresses", requirements.RelationAddresses, deck, story),
		auditRelation("legacy-slide-addresses", requirements.RelationAddresses, slide, story),
		auditRelation("legacy-slide-explains", requirements.RelationExplains, slide, story),
		auditRelation("criterion-deck", requirements.RelationAddresses, deck, c1),
		auditRelation("criterion-slide", requirements.RelationExplains, slide, c2),
		auditRelation("exact", requirements.RelationExplains, item, c1),
	}
	s.requirements.Relations = links
	s.currency = auditCurrency(links)

	report, err := s.audit(Filters{Feature: "urn:change-saga:test:feature:checkout"})
	if err != nil {
		t.Fatal(err)
	}
	broad := []AuditFinding{}
	for _, finding := range report.Findings {
		if finding.Code == "broad_visual_intent" {
			broad = append(broad, finding)
		}
	}
	if len(broad) != 5 {
		t.Fatalf("broad findings = %#v", broad)
	}
	for _, id := range []string{"legacy-deck-addresses", "legacy-slide-addresses", "legacy-slide-explains", "criterion-deck", "criterion-slide"} {
		if !auditHasFinding(report, "urn:change-saga:test:relation:"+id, "broad_visual_intent") {
			t.Errorf("missing broad finding for %s", id)
		}
	}
	if auditHasFinding(report, c1, "criterion_explanation_missing") {
		t.Fatal("the exact Item relation did not satisfy its criterion")
	}
	if !auditHasFinding(report, c2, "criterion_explanation_missing") {
		t.Fatal("broad Slide relation incorrectly satisfied exact criterion explanation")
	}
	if report.Status != "findings" || report.ExitCode != 8 || report.Ready || !report.Complete {
		t.Fatalf("status = %#v", report)
	}
}

func TestFeatureAuditDistinguishesIntentionalRiskFromMissingAuthoring(t *testing.T) {
	t.Run("current cited implementation exception", func(t *testing.T) {
		s := auditFixture(t, false)
		story := "urn:change-saga:test:story:checkout"
		criterion := story + ":criterion:fast"
		s.requirements.Stories[0].Revisions[0].AcceptanceCriteria = s.requirements.Stories[0].Revisions[0].AcceptanceCriteria[:1]
		s.requirements.Stories[0].CurrentRevision.AcceptanceCriteria = s.requirements.Stories[0].CurrentRevision.AcceptanceCriteria[:1]
		item := saga.ItemTarget("test", "flow", "handler")
		relation := auditRelation("exact", requirements.RelationExplains, item, criterion)
		s.requirements.Relations = []requirements.Relation{relation}
		s.currency = auditCurrency(s.requirements.Relations)
		s.requirements.Citations = []requirements.Citation{{ID: "decision", Feature: "checkout"}}
		s.exceptions = []coverage.Exception{
			{URN: "urn:change-saga:test:coverage-exception:old-decision", Axis: coverage.AxisImplementation,
				Criterion: criterion, StoryRevision: story + ":revision:r1", Rationale: "Earlier recorded decision.",
				Citations: []string{"urn:change-saga:test:citation:decision"}, CreatedAt: time.Unix(1, 0).UTC()},
			{URN: "urn:change-saga:test:coverage-exception:no-implementation", Axis: coverage.AxisImplementation,
				Criterion: criterion, StoryRevision: story + ":revision:r1", Rationale: "The external provider owns this behavior.",
				Citations: []string{"urn:change-saga:test:citation:decision"}, Supersedes: []string{"urn:change-saga:test:coverage-exception:old-decision"}, CreatedAt: time.Unix(2, 0).UTC()},
		}

		report, err := s.audit(Filters{Feature: "checkout"})
		if err != nil {
			t.Fatal(err)
		}
		if auditHasFinding(report, item, "item_evidence_missing") || auditHasFinding(report, criterion, "criterion_explanation_missing") {
			t.Fatalf("explicit exception was treated as missing authoring: %#v", report.Findings)
		}
		if len(report.Risks) != 1 || report.Risks[0].Item != item || report.Risks[0].Criterion != criterion {
			t.Fatalf("intentional risks = %#v", report.Risks)
		}
		if !report.Ready || !report.Complete || report.Status != "pass" || report.ExitCode != 0 {
			t.Fatalf("a current explicit exception did not produce an honest pass: %#v", report)
		}
		if len(report.Exceptions) != 2 || report.Exceptions[0].State != "current" && report.Exceptions[1].State != "current" {
			t.Fatalf("exceptions = %#v", report.Exceptions)
		}
		if auditHasFinding(report, "urn:change-saga:test:coverage-exception:old-decision", "exception_not_current") {
			t.Fatal("superseded exception history was treated as a live failure")
		}
	})

	t.Run("no exception", func(t *testing.T) {
		s := auditFixture(t, false)
		report, err := s.audit(Filters{Feature: "checkout"})
		if err != nil {
			t.Fatal(err)
		}
		item := saga.ItemTarget("test", "flow", "handler")
		criterion := "urn:change-saga:test:story:checkout:criterion:fast"
		for _, code := range []string{"item_intent_missing", "item_evidence_missing"} {
			if !auditHasFinding(report, item, code) {
				t.Errorf("missing %s", code)
			}
		}
		if !auditHasFinding(report, criterion, "criterion_explanation_missing") {
			t.Fatal("missing criterion authoring gap")
		}
		if len(report.Risks) != 0 {
			t.Fatalf("unrecorded gap was called intentional: %#v", report.Risks)
		}
	})
}

func TestFeatureAuditReportsStaleCrossFeatureLinksAndConflicts(t *testing.T) {
	s := auditFixture(t, true)
	story := "urn:change-saga:test:story:checkout"
	item := saga.ItemTarget("test", "flow", "handler")
	other := "urn:change-saga:test:story:catalog"
	relation := auditRelation("cross", requirements.RelationExplains, item, other)
	s.requirements.Relations = []requirements.Relation{relation}
	s.requirements.Stories = append(s.requirements.Stories, requirements.Story{Feature: "catalog", Identity: requirements.StoryIdentity{ID: "catalog"}})
	s.currency = []requirements.RelationCurrency{{
		Relation: "urn:change-saga:test:relation:cross", Type: relation.Type, From: relation.From, To: relation.To,
		Status: requirements.CurrencyStale, Reasons: []requirements.CurrencyReason{{Code: requirements.ReasonRevisionChanged, Message: "to revision changed"}},
	}}
	s.requirements.Stories[0].RevisionHeads = []string{story + ":revision:r1", story + ":revision:r2"}
	s.requirements.Stories[0].CurrentRevision = nil

	report, err := s.audit(Filters{Feature: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if !auditHasFinding(report, "urn:change-saga:test:relation:cross", "stale_or_dangling_pin") ||
		!auditHasFinding(report, "urn:change-saga:test:relation:cross", "cross_feature_context") {
		t.Fatalf("relation findings = %#v", report.Findings)
	}
	if report.Complete || report.Status != "incomplete" || len(report.Conflicts) == 0 {
		t.Fatalf("conflicted report = %#v", report)
	}
}

func TestFeatureAuditAcceptsCurrentCrossFeatureItemIntentAndExplanation(t *testing.T) {
	s := auditFixture(t, true)
	checkoutStory := "urn:change-saga:test:story:checkout"
	catalogStory := "urn:change-saga:test:story:catalog"
	foreignItem := saga.ItemTarget("test", "catalog-flow", "shared-handler")
	localItem := saga.ItemTarget("test", "flow", "handler")
	s.requirements.Stories = append(s.requirements.Stories, auditStory("catalog", "catalog", requirements.StateAccepted))
	foreignDeck := &saga.Deck{Target: saga.DeckTarget("test", "catalog-implementation"), DeckManifest: saga.DeckManifest{ID: "catalog-implementation", Role: saga.DeckRoleChange}}
	foreignDeck.Slides = []*saga.Slide{{Target: saga.SlideTarget("test", "catalog-flow"), Items: []*saga.Item{{Target: foreignItem}}}}
	s.saga.Features = append(s.saga.Features, &saga.Feature{ID: "catalog", Target: "urn:change-saga:test:feature:catalog", Decks: []*saga.Deck{foreignDeck}})
	relations := []requirements.Relation{
		auditRelation("local-shared-intent", requirements.RelationExplains, localItem, catalogStory),
		auditRelation("foreign-explains-fast", requirements.RelationExplains, foreignItem, checkoutStory+":criterion:fast"),
		auditRelation("foreign-explains-safe", requirements.RelationExplains, foreignItem, checkoutStory+":criterion:safe"),
	}
	s.requirements.Relations = relations
	s.currency = auditCurrency(relations)

	report, err := s.audit(Filters{Feature: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if auditHasFinding(report, localItem, "item_intent_missing") || auditHasFinding(report, checkoutStory+":criterion:fast", "criterion_explanation_missing") || auditHasFinding(report, checkoutStory+":criterion:safe", "criterion_explanation_missing") {
		t.Fatalf("cross-feature exact links did not satisfy the audit: %#v", report.Findings)
	}
	for _, relation := range relations {
		if !auditHasFinding(report, "urn:change-saga:test:relation:"+relation.ID, "cross_feature_context") {
			t.Errorf("missing cross-feature context for %s", relation.ID)
		}
	}
	if !report.Ready || !report.Complete || report.Status != "pass" || report.ExitCode != 0 || report.Summary.Warnings != 0 || report.Summary.Errors != 0 {
		t.Fatalf("cross-feature context blocked readiness: %#v", report)
	}
}

func TestFeatureAuditRejectsUnresolvedAndRetiredCrossFeatureIntent(t *testing.T) {
	tests := []struct {
		name       string
		story      requirements.Story
		currency   requirements.Currency
		reason     requirements.CurrencyReason
		relationID string
		wantCode   string
		incomplete bool
	}{
		{name: "unresolved endpoint", story: requirements.Story{}, currency: requirements.CurrencyInvalid, reason: requirements.CurrencyReason{Code: requirements.ReasonEndpointMissing, Message: "to endpoint is missing"}, relationID: "missing", wantCode: "stale_or_dangling_pin"},
		{name: "retired story", story: auditStory("catalog", "catalog", requirements.StateRetired), currency: requirements.CurrencyCurrent, relationID: "retired", wantCode: "inactive_requirement_endpoint"},
		{name: "conflicting endpoint", story: auditStory("catalog", "catalog", requirements.StateAccepted), currency: requirements.CurrencyConflicted, reason: requirements.CurrencyReason{Code: requirements.ReasonMultipleRevisionHeads, Message: "to endpoint has multiple revision heads", Current: []string{"urn:change-saga:test:story:catalog:revision:r1", "urn:change-saga:test:story:catalog:revision:r2"}}, relationID: "conflicted", wantCode: "conflicted_relation", incomplete: true},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			s := auditFixture(t, true)
			item := saga.ItemTarget("test", "flow", "handler")
			to := "urn:change-saga:test:story:catalog"
			if testCase.story.Identity.ID != "" {
				s.requirements.Stories = append(s.requirements.Stories, testCase.story)
			}
			relation := auditRelation(testCase.relationID, requirements.RelationExplains, item, to)
			s.requirements.Relations = []requirements.Relation{relation}
			s.currency = []requirements.RelationCurrency{{Relation: "urn:change-saga:test:relation:" + testCase.relationID, Type: relation.Type, From: relation.From, To: relation.To, Status: testCase.currency, Reasons: nonemptyCurrencyReason(testCase.reason)}}
			report, err := s.audit(Filters{Feature: "checkout"})
			if err != nil {
				t.Fatal(err)
			}
			if !auditHasFinding(report, item, "item_intent_missing") || !auditHasFinding(report, "urn:change-saga:test:relation:"+testCase.relationID, testCase.wantCode) {
				t.Fatalf("invalid cross-feature intent findings = %#v", report.Findings)
			}
			if report.Ready || report.ExitCode != auditFindingsExitCode {
				t.Fatalf("invalid cross-feature intent passed: %#v", report)
			}
			if testCase.incomplete && (report.Complete || len(report.Conflicts) == 0) {
				t.Fatalf("conflicting relation was not preserved as incomplete: %#v", report)
			}
		})
	}
}

func TestFeatureAuditRejectsMissingAndUnknownFeature(t *testing.T) {
	s := auditFixture(t, true)
	for _, feature := range []string{"", "missing", "urn:change-saga:other:feature:checkout"} {
		if _, err := s.audit(Filters{Feature: feature}); err == nil {
			t.Errorf("feature %q was accepted", feature)
		}
	}
	s.exceptions = nil
	if _, err := s.audit(Filters{Feature: "checkout"}); err == nil {
		t.Fatal("audit ran on a session that did not load audit data into its snapshot")
	}
}

func auditFixture(t *testing.T, evidence bool) *session {
	t.Helper()
	story := "urn:change-saga:test:story:checkout"
	revision := story + ":revision:r1"
	lifecycle := story + ":event:accepted"
	item := &saga.Item{Target: saga.ItemTarget("test", "flow", "handler")}
	if evidence {
		item.Code = []saga.CodeFile{{References: []coderef.Reference{{Commit: strings.Repeat("a", 40)}}}}
	}
	deck := &saga.Deck{Target: saga.DeckTarget("test", "implementation"), DeckManifest: saga.DeckManifest{ID: "implementation", Role: saga.DeckRoleChange}}
	deck.Slides = []*saga.Slide{{Target: saga.SlideTarget("test", "flow"), Items: []*saga.Item{item}}}
	feature := &saga.Feature{ID: "checkout", Target: "urn:change-saga:test:feature:checkout", Decks: []*saga.Deck{deck}}
	return &session{
		requirements: requirements.Document{SagaID: "test", Stories: []requirements.Story{{
			Feature: "checkout", Identity: requirements.StoryIdentity{ID: "checkout"},
			Revisions:     []requirements.Revision{{ID: "r1", Story: story, AcceptanceCriteria: []requirements.Criterion{{ID: "fast", Statement: "It is fast."}, {ID: "safe", Statement: "It is safe."}}}},
			Events:        []requirements.LifecycleEvent{{ID: "accepted", Story: story, State: requirements.StateAccepted}},
			RevisionHeads: []string{revision}, LifecycleHeads: []string{lifecycle},
			CurrentRevision:  &requirements.Revision{ID: "r1", Story: story, AcceptanceCriteria: []requirements.Criterion{{ID: "fast", Statement: "It is fast."}, {ID: "safe", Statement: "It is safe."}}},
			CurrentLifecycle: &requirements.LifecycleEvent{ID: "accepted", Story: story, State: requirements.StateAccepted},
		}}},
		saga:       &saga.Saga{Manifest: saga.Manifest{ID: "test"}, Features: []*saga.Feature{feature}, Decks: []*saga.Deck{deck}},
		quality:    quality.Document{SagaID: "test", TestCases: []quality.TestCase{}},
		exceptions: []coverage.Exception{},
		plan: workplan.Plan{SagaID: "test", WorkItems: map[string]*workplan.WorkItem{},
			Waves: map[string]*workplan.Wave{}, Dependencies: map[string]*workplan.Dependency{}, Contracts: map[string]*workplan.Contract{}},
	}
}

func auditRelation(id string, kind requirements.RelationType, from, to string) requirements.Relation {
	return requirements.Relation{ID: id, Type: kind, From: from, To: to, State: requirements.RelationActive, Feature: "checkout"}
}

func auditStory(feature, id string, state requirements.LifecycleState) requirements.Story {
	story := "urn:change-saga:test:story:" + id
	revision := story + ":revision:r1"
	lifecycle := story + ":event:" + string(state)
	definition := requirements.Revision{ID: "r1", Story: story, AcceptanceCriteria: []requirements.Criterion{{ID: "works", Statement: "It works."}}}
	event := requirements.LifecycleEvent{ID: string(state), Story: story, State: state}
	return requirements.Story{Feature: feature, Identity: requirements.StoryIdentity{ID: id}, Revisions: []requirements.Revision{definition}, Events: []requirements.LifecycleEvent{event}, RevisionHeads: []string{revision}, LifecycleHeads: []string{lifecycle}, CurrentRevision: &definition, CurrentLifecycle: &event}
}

func nonemptyCurrencyReason(reason requirements.CurrencyReason) []requirements.CurrencyReason {
	if reason.Code == "" {
		return []requirements.CurrencyReason{}
	}
	return []requirements.CurrencyReason{reason}
}

func auditCurrency(relations []requirements.Relation) []requirements.RelationCurrency {
	result := make([]requirements.RelationCurrency, 0, len(relations))
	for _, relation := range relations {
		result = append(result, requirements.RelationCurrency{Relation: "urn:change-saga:test:relation:" + relation.ID, Type: relation.Type, From: relation.From, To: relation.To, Status: requirements.CurrencyCurrent})
	}
	return result
}

func auditHasFinding(report AuditReport, id, code string) bool {
	for _, finding := range report.Findings {
		if finding.ID == id && finding.Code == code {
			return true
		}
	}
	return false
}
