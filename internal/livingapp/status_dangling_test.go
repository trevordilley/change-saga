package livingapp

import (
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A slide Item, test case, or onboarding Item that still points at a removed
// story or criterion is reported with how to adjust it; links to a story that
// exists, or to an archived one, are not.
func TestDanglingReferencesNameEveryRecordStillPointingAtARemovedStory(t *testing.T) {
	kept := requirements.Story{Feature: "core", Identity: requirements.StoryIdentity{ID: "browse"},
		CurrentRevision: &requirements.Revision{AcceptanceCriteria: []requirements.Criterion{{ID: "fast"}}}}
	gone := "urn:change-saga:test:story:checkout"
	in := StatusInputs{
		SagaID:  "test",
		Stories: []requirements.Story{kept},
		Decks: []*saga.Deck{{Slides: []*saga.Slide{{Target: "urn:change-saga:test:slide:flow", Items: []*saga.Item{
			{Target: "urn:change-saga:test:slide:flow:item:pay", CriterionLinks: []saga.CriterionLink{{Criterion: gone + ":criterion:fast"}}},
			{Target: "urn:change-saga:test:slide:flow:item:list", CriterionLinks: []saga.CriterionLink{{Criterion: "urn:change-saga:test:story:browse:criterion:fast"}}},
		}}}}},
		RecordDecks: []*saga.Deck{{DeckManifest: saga.DeckManifest{Role: saga.DeckRoleOnboarding}, Slides: []*saga.Slide{{Items: []*saga.Item{
			{Target: "urn:change-saga:test:slide:hello:item:who", ItemManifest: saga.ItemManifest{Record: gone}},
		}}}}},
		Quality: quality.Document{TestCases: []quality.TestCase{{Identity: quality.TestCaseIdentity{ID: "pay-test"},
			CurrentRevision: &quality.Revision{Steps: []quality.Step{{ID: "pay", Criteria: []string{gone + ":criterion:fast"}}}}}}},
	}
	found := map[string]DanglingReference{}
	for _, value := range danglingReferences(in) {
		found[value.Record] = value
	}
	for _, want := range []string{"urn:change-saga:test:slide:flow:item:pay", "urn:change-saga:test:slide:hello:item:who", "urn:change-saga:test:test-case:pay-test"} {
		if value, ok := found[want]; !ok || value.Fix == "" || len(value.Missing) != 1 {
			t.Fatalf("missing finding for %s: %#v", want, found)
		}
	}
	if _, ok := found["urn:change-saga:test:slide:flow:item:list"]; ok {
		t.Fatal("an Item linking a story that exists is not dangling")
	}
	if len(DanglingReferences(in, "urn:change-saga:test:story:browse")) != 4 {
		t.Fatal("previewing a removal must also report links to the story being removed")
	}
}
