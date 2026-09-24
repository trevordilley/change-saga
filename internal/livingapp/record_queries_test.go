package livingapp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestPersonaReadAndEightStoryReferencesAreExactBoundedAndTruthful(t *testing.T) {
	root := livingFixture(t)
	persona := fixturePersona()
	for index := 1; index <= 8; index++ {
		id := fmt.Sprintf("story-%02d", index)
		addFixtureStory(t, root, id, []string{persona})
	}
	addFixtureTerm(t, root, "shopper-word", []string{}, []string{persona})

	session := openFixture(t, root)
	read, err := session.Query(context.Background(), Query{Operation: "personas", Filters: Filters{Persona: persona}})
	if err != nil {
		t.Fatal(err)
	}
	personas := read.Data.(PersonaPage).Personas
	if len(personas) != 1 || personas[0].CurrentRevision == nil || personas[0].CurrentRevision.Name != "Shopper" || personas[0].CurrentRevision.Description != "Buys things" || personas[0].State != "active" {
		t.Fatalf("persona read = %#v", personas)
	}
	if personas[0].RevisionConflict || personas[0].LifecycleConflict || len(personas[0].RevisionHeads) != 1 || len(personas[0].LifecycleHeads) != 1 {
		t.Fatalf("persona heads = %#v", personas[0])
	}

	var cursor string
	var collected []RecordReference
	for {
		page, err := session.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: "shopper"}, Cursor: cursor, Limit: 3})
		if err != nil {
			t.Fatal(err)
		}
		data := page.Data.(ReferencePage)
		if data.Subject != persona || data.Counts.Total != 9 || data.Counts.Incoming != 9 || data.Counts.Outgoing != 0 || !data.Completeness.Complete {
			t.Fatalf("reference metadata = %#v", data)
		}
		if page.Page.Returned != len(data.References) || page.Page.Total != data.Counts.Total {
			t.Fatalf("page=%#v data=%#v", page.Page, data)
		}
		collected = append(collected, data.References...)
		if !page.Page.HasMore {
			break
		}
		cursor = *page.Page.NextCursor
	}
	stories := 0
	for _, reference := range collected {
		if reference.Kind == "story_persona" {
			stories++
			if reference.Direction != "incoming" || reference.Provenance.Class != "typed_field" || reference.Owner == "" || reference.Selector == "" {
				t.Fatalf("story reference lacks exact provenance: %#v", reference)
			}
		}
	}
	if stories != 8 || len(collected) != 9 {
		t.Fatalf("story references=%d all=%#v", stories, collected)
	}
	excluded := map[string]bool{}
	result, _ := session.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}})
	completeness := result.Data.(ReferencePage).Completeness
	for _, exclusion := range completeness.ExcludedClasses {
		excluded[exclusion.Class] = true
	}
	if !excluded["free_form_prose"] || !excluded["embedded_svg_text"] {
		t.Fatalf("reference exclusions = %#v", excluded)
	}
	if !contains(completeness.CoveredClasses, "canonical living relations, including complete-slide transaction relations") {
		t.Fatalf("reference coverage = %#v", completeness.CoveredClasses)
	}
}

func TestPersonaReferencesIncludeOnboardingItemOwnership(t *testing.T) {
	opened := openFixture(t, livingFixture(t)).(*session)
	persona := fixturePersona()
	item := &saga.Item{Target: "urn:change-saga:test:item:intro", ItemManifest: saga.ItemManifest{Record: persona}}
	opened.saga.Onboarding = []*saga.Deck{{Slides: []*saga.Slide{{Items: []*saga.Item{item}}}}}

	result, err := opened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}})
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range result.Data.(ReferencePage).References {
		if reference.Kind == "onboarding_item" && reference.Source == item.Target && reference.Target == persona && reference.Owner == item.Target && reference.Selector == "record" {
			return
		}
	}
	t.Fatalf("onboarding Item provenance is missing: %#v", result.Data)
}

func TestTermReferencesCoverApplicabilityBothDirections(t *testing.T) {
	root := livingFixture(t)
	story := addFixtureStory(t, root, "checkout", []string{fixturePersona()})
	term := addFixtureTerm(t, root, "basket", []string{story}, []string{fixturePersona()})
	other := addFixtureTerm(t, root, "cart", nil, []string{term})

	result, err := openFixture(t, root).Query(context.Background(), Query{Operation: "term-references", Filters: Filters{Term: "basket"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	data := result.Data.(ReferencePage)
	if data.Subject != term || data.Counts.Total != 3 || data.Counts.Outgoing != 2 || data.Counts.Incoming != 1 || !data.Completeness.Complete {
		t.Fatalf("term references = %#v", data)
	}
	want := map[string]bool{"term_story": false, "term_record": false}
	for _, reference := range data.References {
		want[reference.Kind] = true
		if reference.Owner == "" || reference.Selector == "" || reference.Provenance.Resource == "" {
			t.Fatalf("reference lacks provenance: %#v", reference)
		}
		if reference.Source == other && reference.Direction != "incoming" {
			t.Fatalf("reverse term link = %#v", reference)
		}
	}
	if !want["term_story"] || !want["term_record"] {
		t.Fatalf("term reference kinds = %#v", data.References)
	}
}

func TestConflictedTermDoesNotFabricateCurrentContentOrCompleteReferences(t *testing.T) {
	root := livingFixture(t)
	term := addFixtureTerm(t, root, "basket", nil, []string{fixturePersona()})
	writeConflictingTermRevision(t, root, "basket", "branch-a", "Basket A")
	writeConflictingTermRevision(t, root, "basket", "branch-b", "Basket B")
	opened := openFixture(t, root).(*session)

	result, err := opened.Query(context.Background(), Query{Operation: "term-references", Filters: Filters{Term: term}})
	if err != nil {
		t.Fatal(err)
	}
	completeness := result.Data.(ReferencePage).Completeness
	if completeness.Complete || len(completeness.UnresolvedOwners) != 1 || completeness.UnresolvedOwners[0].Owner != term || len(completeness.UnresolvedOwners[0].Heads) != 2 {
		t.Fatalf("conflicted term completeness = %#v", completeness)
	}
	statuses := termStatuses(opened.requirements.SagaID, opened.requirements.Terms, nil)
	for _, status := range statuses {
		if status.Term == term {
			if !status.RevisionConflict || status.CurrentRevision != "" || status.Name != "" || status.Definition != "" {
				t.Fatalf("conflicted term fabricated current content: %#v", status)
			}
			return
		}
	}
	t.Fatalf("term %s was not projected", term)
}

func TestRecordSelectorsConflictsAndCursorsUseStructuredSemantics(t *testing.T) {
	root := livingFixture(t)
	persona := fixturePersona()
	addFixtureStory(t, root, "one", []string{persona})
	addFixtureStory(t, root, "two", []string{persona})
	session := openFixture(t, root)

	for _, test := range []struct {
		operation string
		filters   Filters
		code      ErrorCode
	}{
		{operation: "personas", filters: Filters{Persona: "urn:change-saga:foreign:persona:shopper"}, code: CodeInvalidArgument},
		{operation: "personas", filters: Filters{Persona: "urn:not-a-persona"}, code: CodeInvalidArgument},
		{operation: "personas", filters: Filters{Persona: "missing"}, code: CodeNotFound},
		{operation: "term-references", filters: Filters{Term: "missing"}, code: CodeNotFound},
	} {
		if _, err := session.Query(context.Background(), Query{Operation: test.operation, Filters: test.filters}); !errorHasCode(err, test.code) {
			t.Errorf("%s %#v error = %#v", test.operation, test.filters, err)
		}
	}

	first, err := session.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}, Limit: 1})
	if err != nil || first.Page.NextCursor == nil {
		t.Fatalf("first page = %#v err=%v", first, err)
	}
	if _, err := session.Query(context.Background(), Query{Operation: "term-references", Filters: Filters{Term: "missing"}, Cursor: *first.Page.NextCursor, Limit: 1}); !errorHasCode(err, CodeNotFound) {
		t.Fatalf("exact selection must fail before cursor use: %#v", err)
	}
	if _, err := session.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}, Cursor: *first.Page.NextCursor + "A", Limit: 1}); !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("tampered cursor error = %#v", err)
	}

	writeConflictingPersonaRevision(t, root, "shopper", "branch-a", "Shopper A")
	writeConflictingPersonaRevision(t, root, "shopper", "branch-b", "Shopper B")
	reopened := openFixture(t, root)
	if _, err := reopened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}, Cursor: *first.Page.NextCursor, Limit: 1}); !errorHasCode(err, CodeStaleSnapshot) {
		t.Fatalf("stale cursor error = %#v", err)
	}
	read, err := reopened.Query(context.Background(), Query{Operation: "personas", Filters: Filters{Persona: persona}})
	if err != nil {
		t.Fatal(err)
	}
	row := read.Data.(PersonaPage).Personas[0]
	if !row.RevisionConflict || row.CurrentRevision != nil || len(row.RevisionHeads) != 2 {
		t.Fatalf("conflicted persona fabricated a current revision: %#v", row)
	}
	references, err := reopened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: persona}})
	if err != nil {
		t.Fatal(err)
	}
	completeness := references.Data.(ReferencePage).Completeness
	if completeness.Complete || len(completeness.UnresolvedOwners) != 1 || completeness.UnresolvedOwners[0].Owner != persona {
		t.Fatalf("conflicted persona references = %#v", completeness)
	}
}

func TestPersonaConflictDetailsAreIndependentlyBoundedAndTraversable(t *testing.T) {
	opened := openFixture(t, livingFixture(t)).(*session)
	opened.requirements.Stories = nil
	opened.requirements.Terms = nil
	opened.requirements.Relations = nil
	opened.currency = nil
	opened.saga.Onboarding = nil
	opened.saga.Reviews = nil
	for index := 0; index < 1000; index++ {
		id := fmt.Sprintf("conflict-%04d", index)
		urn := "urn:change-saga:test:story:" + id
		opened.requirements.Stories = append(opened.requirements.Stories, requirements.Story{
			Identity:      requirements.StoryIdentity{ID: id},
			RevisionHeads: []string{urn + ":revision:a", urn + ":revision:b"},
			Revisions: []requirements.Revision{
				{ID: "a", Story: urn, Personas: []string{fixturePersona()}},
				{ID: "b", Story: urn},
			},
		})
	}

	query := Query{Operation: "persona-references", Filters: Filters{Persona: fixturePersona()}, Limit: 1}
	first, err := opened.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	data := first.Data.(ReferencePage)
	encoded, _ := json.Marshal(data)
	t.Logf("persona limit=1 references=%d unresolved=%d unresolved_total=%d response_data_bytes=%d", len(data.References), len(data.Completeness.UnresolvedOwners), data.Completeness.UnresolvedPage.Total, len(encoded))
	if first.Page.Total != 0 || first.Page.Returned != 0 || len(data.References) != 0 || data.Completeness.Complete ||
		data.Completeness.UnresolvedPage.Total != 1000 || data.Completeness.UnresolvedPage.Returned != 1 || !data.Completeness.UnresolvedPage.HasMore || len(data.Completeness.UnresolvedOwners) != 1 || len(encoded) > 5000 {
		t.Fatalf("bounded conflict page = %#v bytes=%d", first, len(encoded))
	}
	conflictCursor := *data.Completeness.UnresolvedPage.NextCursor
	all := collectUnresolved(t, opened, Query{Operation: "persona-references", Filters: Filters{Persona: fixturePersona()}, Limit: 1, ConflictLimit: 137})
	if len(all) != 1000 {
		t.Fatalf("traversed unresolved owners = %d", len(all))
	}
	seen := map[string]bool{}
	for _, unresolved := range all {
		if seen[unresolved.Owner] || len(unresolved.Heads) != 2 {
			t.Fatalf("unresolved owner lost identity or heads: %#v", unresolved)
		}
		seen[unresolved.Owner] = true
	}
	if _, err := opened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: fixturePersona()}, ConflictCursor: conflictCursor + "A", Limit: 1}); !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("tampered conflict cursor error = %#v", err)
	}
	if _, err := opened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: fixturePersona()}, Cursor: conflictCursor, Limit: 1}); !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("conflict cursor used as reference cursor = %#v", err)
	}
	opened.snapshot = "sha256:changed"
	if _, err := opened.Query(context.Background(), Query{Operation: "persona-references", Filters: Filters{Persona: fixturePersona()}, ConflictCursor: conflictCursor, Limit: 1}); !errorHasCode(err, CodeStaleSnapshot) {
		t.Fatalf("stale conflict cursor error = %#v", err)
	}
}

func TestTermConflictDetailsAreBoundedWhenNoReferencesResolve(t *testing.T) {
	root := livingFixture(t)
	subject := addFixtureTerm(t, root, "subject", nil, nil)
	opened := openFixture(t, root).(*session)
	base := *opened.requirements.FindTerm("subject")
	opened.requirements.Terms = []requirements.Term{base}
	opened.requirements.Stories = nil
	opened.requirements.Relations = nil
	opened.currency = nil
	for index := 0; index < 1000; index++ {
		id := fmt.Sprintf("term-conflict-%04d", index)
		urn, _ := requirements.TermURN("test", id)
		opened.requirements.Terms = append(opened.requirements.Terms, requirements.Term{
			Identity:      requirements.RecordIdentity{ID: id},
			RevisionHeads: []string{urn + ":revision:a", urn + ":revision:b"},
			Revisions: []requirements.TermRevision{
				{ID: "a", Term: urn, Records: []string{subject}},
				{ID: "b", Term: urn},
			},
		})
	}

	query := Query{Operation: "term-references", Filters: Filters{Term: subject}, Limit: 1}
	first, err := opened.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	data := first.Data.(ReferencePage)
	encoded, _ := json.Marshal(data)
	t.Logf("term limit=1 references=%d unresolved=%d unresolved_total=%d response_data_bytes=%d", len(data.References), len(data.Completeness.UnresolvedOwners), data.Completeness.UnresolvedPage.Total, len(encoded))
	if first.Page.Total != 0 || len(data.References) != 0 || data.Completeness.UnresolvedPage.Total != 1000 || len(data.Completeness.UnresolvedOwners) != 1 || len(encoded) > 5000 {
		t.Fatalf("bounded term conflict page = %#v bytes=%d", first, len(encoded))
	}
	all := collectUnresolved(t, opened, Query{Operation: "term-references", Filters: Filters{Term: subject}, Limit: 1, ConflictLimit: 211})
	if len(all) != 1000 {
		t.Fatalf("traversed term unresolved owners = %d", len(all))
	}
}

func collectUnresolved(t *testing.T, session Session, query Query) []ReferenceUnresolved {
	t.Helper()
	var result []ReferenceUnresolved
	for {
		page, err := session.Query(context.Background(), query)
		if err != nil {
			t.Fatal(err)
		}
		completeness := page.Data.(ReferencePage).Completeness
		if completeness.UnresolvedPage.Returned != len(completeness.UnresolvedOwners) || completeness.UnresolvedPage.Total != 1000 {
			t.Fatalf("unresolved page metadata = %#v", completeness)
		}
		result = append(result, completeness.UnresolvedOwners...)
		if !completeness.UnresolvedPage.HasMore {
			return result
		}
		query.ConflictCursor = *completeness.UnresolvedPage.NextCursor
	}
}

func addFixtureStory(t *testing.T, root, id string, personas []string) string {
	t.Helper()
	result, err := requirements.AddStory(root, "test", requirements.AddStoryInput{
		Feature: fixtureFeature, ID: id, RevisionID: "r1", EventID: "proposed", Title: id, Statement: "As a shopper I can " + id,
		Personas: personas, Citations: []string{}, AcceptanceCriteria: []requirements.Criterion{}, CreatedAt: fixtureTime, RequestID: "add-" + id,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.URN
}

func addFixtureTerm(t *testing.T, root, id string, stories, records []string) string {
	t.Helper()
	result, err := requirements.AddTerm(root, "test", requirements.AddTermInput{
		ID: id, RevisionID: "r1", EventID: "active", CreatedAt: fixtureTime, RequestID: "add-" + id,
		TermDefinition: requirements.TermDefinition{Name: id, Definition: "Definition of " + id, Stories: stories, Records: records},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.URN
}

func writeConflictingPersonaRevision(t *testing.T, root, id, revisionID, name string) {
	t.Helper()
	persona, _ := requirements.PersonaURN("test", id)
	parent, _ := requirements.PersonaRevisionURN("test", id, "r1")
	revision := requirements.PersonaRevision{
		Schema: requirements.PersonaRevisionSchemaURL, Version: requirements.AppRecordVersion, ID: revisionID,
		Persona: persona, Parents: []string{parent}, Name: name, Description: "Conflicting description", CreatedAt: fixtureTime,
	}
	data, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, applayout.PersonasDir, id+".persona", "revisions", revisionID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeConflictingTermRevision(t *testing.T, root, id, revisionID, name string) {
	t.Helper()
	term, _ := requirements.TermURN("test", id)
	parent, _ := requirements.TermRevisionURN("test", id, "r1")
	revision := requirements.TermRevision{
		Schema: requirements.TermRevisionSchemaURL, Version: requirements.AppRecordVersion, ID: revisionID,
		Term: term, Parents: []string{parent}, Name: name, Definition: "Conflicting definition", CreatedAt: fixtureTime,
	}
	data, err := json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, applayout.TermsDir, id+".term", "revisions", revisionID+".json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
