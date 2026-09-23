package livingapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestFeatureContextCompactExpansionNeighborsAndExactLinks(t *testing.T) {
	session := featureContextFixture("snapshot-one")
	first, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	page := first.Data.(FeatureContextPage)
	if first.Page.Total != 2 || first.Page.Returned != 1 || first.Page.NextCursor == nil || page.Projection != "compact" || !page.Completeness.Bounded || page.Completeness.RelationHops != 1 {
		t.Fatalf("compact page = %+v data=%+v", first.Page, page)
	}
	if page.Feature.Description != "" {
		t.Fatalf("compact feature leaked description prose: %+v", page.Feature)
	}
	if len(page.Stories) != 1 || page.Stories[0].Requirement != "urn:change-saga:test:story:alpha" {
		t.Fatalf("compact stories leaked prose or lost identity: %+v", page.Stories)
	}
	if len(page.Terms) != 1 || page.Terms[0].DefinitionMaturity != requirements.DefinitionMaturityAccepted || page.Terms[0].ImplementationEvidence != requirements.ImplementationEvidencePresent {
		t.Fatalf("compact term semantics = %+v", page.Terms)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "Alpha statement") || strings.Contains(string(encoded), "project vocabulary definition") || strings.Contains(string(encoded), strings.Repeat("a", 40)) {
		t.Fatalf("compact projection leaked expanded prose or code: %s", encoded)
	}
	if len(page.Neighbors) != 1 || page.Neighbors[0].Feature != "other" || page.Neighbors[0].Requirement != "urn:change-saga:test:story:neighbor" || len(page.Neighbors[0].Relations) != 2 || page.Neighbors[0].Relations[0].From == "" || page.Neighbors[0].Relations[0].Type == "" {
		t.Fatalf("cross-feature neighbors = %+v", page.Neighbors)
	}
	if !contextHasGap(page.Gaps, "design_missing") || !contextHasConflict(page.Conflicts, "intent") {
		t.Fatalf("known gaps/conflicts = gaps:%+v conflicts:%+v", page.Gaps, page.Conflicts)
	}
	if len(page.Visuals) != 1 || len(page.Visuals[0].Items) != 1 || page.Visuals[0].Items[0].CodeReferenceCount != 1 || len(page.Visuals[0].Items[0].Code) != 0 {
		t.Fatalf("compact visual index = %+v", page.Visuals)
	}

	second, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "urn:change-saga:test:feature:main"}, Limit: 1, Cursor: *first.Page.NextCursor})
	if err != nil || second.Page.Returned != 1 || second.Page.HasMore || second.Data.(FeatureContextPage).Stories[0].ID != "beta" {
		t.Fatalf("second page = %+v err=%v", second, err)
	}

	expanded, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	detail := expanded.Data.(FeatureContextPage)
	if detail.Projection != "expanded" || detail.Expanded == nil || detail.Expanded.Requirement.CurrentRevision.Statement != "Alpha statement" {
		t.Fatalf("expanded requirement = %+v", detail)
	}
	if detail.Feature.Description != "Main feature" {
		t.Fatalf("expanded feature description = %+v", detail.Feature)
	}
	if len(detail.Terms) != 1 || detail.Terms[0].Definition != "project vocabulary definition" || detail.Terms[0].DefinitionMaturity != requirements.DefinitionMaturityAccepted || detail.Terms[0].ImplementationEvidence != requirements.ImplementationEvidencePresent {
		t.Fatalf("expanded terms = %+v", detail.Terms)
	}
	if len(detail.Visuals) != 1 || len(detail.Visuals[0].Items) != 1 || len(detail.Visuals[0].Items[0].Code) != 1 {
		t.Fatalf("expanded visual evidence = %+v", detail.Visuals)
	}
	item := detail.Visuals[0].Items[0]
	if item.CriterionLinks[0] != "urn:change-saga:test:story:alpha:criterion:accept" || item.Code[0].Location().String() != strings.Repeat("a", 40)+":app.go#L3-L4" {
		t.Fatalf("exact item links = %+v", item)
	}
}

func TestFeatureContextTermSemanticsPreserveLegacyUnknownAndConflicts(t *testing.T) {
	t.Run("legacy omitted values", func(t *testing.T) {
		session := featureContextFixture("snapshot")
		session.requirements.Terms[0].CurrentRevision.DefinitionMaturity = nil
		session.requirements.Terms[0].CurrentRevision.ImplementationEvidence = nil
		result, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}})
		if err != nil {
			t.Fatal(err)
		}
		term := result.Data.(FeatureContextPage).Terms[0]
		if term.DefinitionMaturity != requirements.DefinitionMaturityUnknown || term.ImplementationEvidence != requirements.ImplementationEvidenceUnknown {
			t.Fatalf("legacy semantics = %+v", term)
		}
	})

	t.Run("conflicting heads", func(t *testing.T) {
		session := featureContextFixture("snapshot")
		term := &session.requirements.Terms[0]
		other := *term.CurrentRevision
		other.ID = "r2"
		other.Name = "Competing widget"
		proposed := requirements.DefinitionMaturityProposed
		absent := requirements.ImplementationEvidenceAbsent
		other.DefinitionMaturity = &proposed
		other.ImplementationEvidence = &absent
		term.Revisions = []requirements.TermRevision{*term.CurrentRevision, other}
		term.RevisionHeads = []string{"urn:change-saga:test:term:widget:revision:r1", "urn:change-saga:test:term:widget:revision:r2"}
		term.CurrentRevision = nil
		result, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}})
		if err != nil {
			t.Fatal(err)
		}
		terms := result.Data.(FeatureContextPage).Terms
		if len(terms) != 1 || !terms[0].RevisionConflict || terms[0].Name != "" || terms[0].Definition != "" || terms[0].DefinitionMaturity != requirements.DefinitionMaturityUnknown || terms[0].ImplementationEvidence != requirements.ImplementationEvidenceUnknown {
			t.Fatalf("conflicted term semantics = %+v", terms)
		}
	})
}

func TestFeatureContextRejectsAmbiguousTitlesInvalidFeaturesAndCrossSnapshotCursors(t *testing.T) {
	session := featureContextFixture("snapshot-one")
	for _, testCase := range []struct {
		name    string
		feature string
		code    ErrorCode
	}{
		{name: "missing", code: CodeInvalidArgument},
		{name: "same title is not guessed", feature: "Shared", code: CodeNotFound},
		{name: "wrong saga urn", feature: "urn:change-saga:other:feature:main", code: CodeInvalidArgument},
		{name: "unknown exact id", feature: "missing", code: CodeNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: testCase.feature}})
			if !errorHasCode(err, testCase.code) {
				t.Fatalf("error = %#v, want %s", err, testCase.code)
			}
		})
	}
	_, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "neighbor"}})
	if !errorHasCode(err, CodeNotFound) {
		t.Fatalf("cross-feature expansion error = %#v", err)
	}

	first, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}, Limit: 1})
	if err != nil || first.Page.NextCursor == nil {
		t.Fatalf("first page = %+v err=%v", first, err)
	}
	_, err = session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}, Cursor: *first.Page.NextCursor})
	if !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("expanded cursor error = %#v", err)
	}
	_, err = session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "other"}, Limit: 1, Cursor: *first.Page.NextCursor})
	if !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("cross-feature cursor error = %#v", err)
	}
	changed := featureContextFixture("snapshot-two")
	_, err = changed.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}, Limit: 1, Cursor: *first.Page.NextCursor})
	if !errorHasCode(err, CodeStaleSnapshot) {
		t.Fatalf("stale cursor error = %#v", err)
	}
}

func TestRequirementProjectionExposesAndFiltersFeatureOwnership(t *testing.T) {
	session := featureContextFixture("snapshot")
	result, err := session.Query(context.Background(), Query{Operation: "requirements", Filters: Filters{Feature: "urn:change-saga:test:feature:other"}})
	if err != nil {
		t.Fatal(err)
	}
	rows := result.Data.(RequirementPage).Requirements
	if len(rows) != 1 || rows[0].ID != "neighbor" || rows[0].Feature != "other" {
		t.Fatalf("feature-filtered requirements = %+v", rows)
	}
}

func TestFeatureContextPreservesVisualRelationScope(t *testing.T) {
	session := featureContextFixture("snapshot")
	criterion := "urn:change-saga:test:story:alpha:criterion:accept"
	session.requirements.Relations = []requirements.Relation{{
		ID: "slide-explains-alpha", Type: requirements.RelationExplains,
		From: "urn:change-saga:test:slide:flow", To: criterion,
		State: requirements.RelationActive, Scope: requirements.ScopeSelf,
	}}
	self, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	visuals := self.Data.(FeatureContextPage).Visuals
	if len(visuals) != 1 || len(visuals[0].Items) != 0 || len(visuals[0].CriterionLinks) != 1 {
		t.Fatalf("self-scoped slide relation was widened to Items: %+v", visuals)
	}
	session.requirements.Relations[0].Scope = requirements.ScopeDescendants
	descendants, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	visuals = descendants.Data.(FeatureContextPage).Visuals
	if len(visuals) != 1 || len(visuals[0].Items) != 1 || visuals[0].Items[0].CodeReferenceCount != 1 {
		t.Fatalf("descendant-scoped slide relation did not reach its Item: %+v", visuals)
	}
}

func TestFeatureContextStaleRelationsAreGapsNotCurrentLinks(t *testing.T) {
	session := featureContextFixture("snapshot")
	for index := range session.requirements.Relations {
		session.requirements.Relations[index].Stale = true
		session.requirements.Relations[index].StaleReasons = []string{"pinned revision changed"}
	}
	result, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	page := result.Data.(FeatureContextPage)
	if len(page.Neighbors) != 0 || len(page.Visuals) != 0 || contextHasConflict(page.Conflicts, "intent") || !contextHasGap(page.Gaps, "stale_relation") {
		t.Fatalf("stale relation projection = %+v", page)
	}
}

func TestFeatureContextRepresentativeResponseMeasurement(t *testing.T) {
	session := featureContextFixture("snapshot")
	compact, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main"}})
	if err != nil {
		t.Fatal(err)
	}
	expanded, err := session.Query(context.Background(), Query{Operation: "context", Filters: Filters{Feature: "main", Expand: "alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	requirementsResult, err := session.Query(context.Background(), Query{Operation: "requirements", Filters: Filters{Feature: "main"}})
	if err != nil {
		t.Fatal(err)
	}
	relationsResult, err := session.Query(context.Background(), Query{Operation: "relations"})
	if err != nil {
		t.Fatal(err)
	}
	traceResult, err := session.Query(context.Background(), Query{Operation: "traceability", Filters: Filters{Feature: "main"}})
	if err != nil {
		t.Fatal(err)
	}
	compactJSON, _ := json.Marshal(compact.Data)
	expandedJSON, _ := json.Marshal(expanded.Data)
	legacyJSON, _ := json.Marshal([]any{requirementsResult.Data, relationsResult.Data, traceResult.Data})
	t.Logf("representative fixture: context compact=1 call/%d bytes; compact+one expansion=2 calls/%d bytes; requirements+relations+traceability=3 calls/%d bytes", len(compactJSON), len(compactJSON)+len(expandedJSON), len(legacyJSON))
	if strings.Contains(string(compactJSON), "Alpha statement") || !strings.Contains(string(legacyJSON), "Alpha statement") {
		t.Fatalf("measurement did not compare compact identities against existing full requirement prose")
	}
}

func featureContextFixture(snapshot string) *session {
	mainFeature := applayout.Feature{FeatureManifest: applayout.FeatureManifest{ID: "main", Title: "Shared", Description: "Main feature"}}
	otherFeature := applayout.Feature{FeatureManifest: applayout.FeatureManifest{ID: "other", Title: "Shared"}}
	alpha := contextStory("main", "alpha", "Alpha", "Alpha statement")
	beta := contextStory("main", "beta", "Beta", "Beta statement")
	neighbor := contextStory("other", "neighbor", "Neighbor", "Neighbor statement")
	alphaURN, _ := livingid.Story("test", "alpha")
	neighborURN, _ := livingid.Story("test", "neighbor")
	criterionURN, _ := livingid.Criterion("test", "alpha", "accept")
	ref := coderef.Reference{Commit: strings.Repeat("a", 40), Path: "app.go", Start: 3, End: 4, Digest: "sha256:" + strings.Repeat("b", 64)}
	deck := &saga.Deck{DeckManifest: saga.DeckManifest{ID: "flow", Title: "Flow"}, Target: "urn:change-saga:test:deck:flow"}
	slide := &saga.Slide{SlideManifest: saga.SlideManifest{ID: "flow", DeckID: "flow", Title: "Flow", Intent: "explain", Takeaway: "Alpha works"}, Target: "urn:change-saga:test:slide:flow"}
	item := &saga.Item{ItemManifest: saga.ItemManifest{ID: "control", SlideID: "flow", Label: "Control", Kind: "node"}, Target: "urn:change-saga:test:slide:flow:item:control", Code: []saga.CodeFile{{References: []coderef.Reference{ref}}}}
	slide.Items = []*saga.Item{item}
	deck.Slides = []*saga.Slide{slide}
	accepted := requirements.DefinitionMaturityAccepted
	present := requirements.ImplementationEvidencePresent
	termRevision := &requirements.TermRevision{ID: "r1", Name: "Widget", Definition: "project vocabulary definition", DefinitionMaturity: &accepted, ImplementationEvidence: &present, Stories: []string{alphaURN}, Code: []coderef.Reference{ref}}
	termLifecycle := &requirements.TermEvent{ID: "active", State: requirements.TermActive}
	term := requirements.Term{Identity: requirements.RecordIdentity{ID: "widget"}, Revisions: []requirements.TermRevision{*termRevision}, CurrentRevision: termRevision, CurrentLifecycle: termLifecycle, RevisionHeads: []string{"urn:change-saga:test:term:widget:revision:r1"}, LifecycleHeads: []string{"urn:change-saga:test:term:widget:event:active"}}
	return &session{
		snapshot: snapshot,
		requirements: requirements.Document{SagaID: "test", Features: []applayout.Feature{mainFeature, otherFeature}, Stories: []requirements.Story{alpha, beta, neighbor}, Terms: []requirements.Term{term}, Relations: []requirements.Relation{
			{ID: "alpha-refines-neighbor", Type: requirements.RelationRefines, From: alphaURN, To: neighborURN, State: requirements.RelationActive},
			{ID: "alpha-conflicts-neighbor", Type: requirements.RelationConflictsWith, From: alphaURN, To: neighborURN, State: requirements.RelationActive},
			{ID: "item-explains-alpha", Type: requirements.RelationExplains, From: item.Target, To: criterionURN, State: requirements.RelationActive},
		}},
		saga:    &saga.Saga{Manifest: saga.Manifest{ID: "test"}, Decks: []*saga.Deck{deck}},
		adopted: true,
	}
}

func contextStory(feature, id, title, statement string) requirements.Story {
	storyURN, _ := livingid.Story("test", id)
	revisionURN, _ := livingid.Revision("test", id, "r1")
	eventURN, _ := requirements.StoryEventURN("test", id, "accepted")
	revision := requirements.Revision{ID: "r1", Story: storyURN, Title: title, Statement: statement, AcceptanceCriteria: []requirements.Criterion{{ID: "accept", Statement: title + " accepted"}}}
	lifecycle := requirements.LifecycleEvent{ID: "accepted", Story: storyURN, State: requirements.StateAccepted}
	return requirements.Story{Feature: feature, Identity: requirements.StoryIdentity{ID: id}, Revisions: []requirements.Revision{revision}, Events: []requirements.LifecycleEvent{lifecycle}, RevisionHeads: []string{revisionURN}, LifecycleHeads: []string{eventURN}, CurrentRevision: &revision, CurrentLifecycle: &lifecycle}
}

func contextHasGap(values []FeatureContextGap, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func contextHasConflict(values []FeatureContextConflict, kind string) bool {
	for _, value := range values {
		if value.Kind == kind {
			return true
		}
	}
	return false
}
