package reviewapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestOverviewExpandsOnlySelectedDeck(t *testing.T) {
	makeDeck := func(target string) *saga.Deck {
		return &saga.Deck{Target: target, DeckManifest: saga.DeckManifest{
			ID: "architecture", Title: target,
			Overview: &saga.DeckOverview{Body: "Report for " + target},
		}}
	}
	implementation := makeDeck("urn:change-saga:example:deck:architecture")
	onboarding := makeDeck("urn:change-saga:example:deck:onboarding")
	review := makeDeck("urn:change-saga:example:review:pr-1:deck:architecture")
	decks := []*saga.Deck{implementation, onboarding, review}
	doc := &saga.Saga{
		Manifest: saga.Manifest{ID: "example"},
		Section: &saga.Section{Target: "urn:change-saga:example:saga", Kind: "saga",
			Children: []*saga.Section{saga.ProjectDeck(implementation), saga.ProjectDeck(onboarding)}},
		Decks: []*saga.Deck{implementation}, Onboarding: []*saga.Deck{onboarding},
		Reviews: []*saga.Review{{Target: "urn:change-saga:example:review:pr-1", Deck: review}},
	}
	s := &session{document: doc, targets: map[string]*targetEntry{}, summaryOnly: true}
	s.indexSection(doc.Section, "")
	s.indexReviews()
	ctx := context.Background()
	directory, err := s.Overview(ctx, OverviewQuery{})
	if err != nil || len(directory.Decks) != len(decks) {
		t.Fatalf("directory: %+v, %v", directory, err)
	}
	before, err := json.Marshal(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, deck := range decks {
		deck.Overview.Body = strings.Repeat(deck.Overview.Body, 10000)
	}
	after, err := s.Overview(ctx, OverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(after)
	if err != nil || string(encoded) != string(before) {
		t.Fatal("growing deck prose must not change the directory payload", err)
	}
	for _, deck := range decks {
		t.Run(deck.Target, func(t *testing.T) {
			result, err := s.Overview(ctx, OverviewQuery{Deck: deck.Target})
			if err != nil || len(result.Decks) != len(decks) {
				t.Fatalf("expanded directory: %+v, %v", result, err)
			}
			for _, entry := range result.Decks {
				if entry.Target == deck.Target {
					if entry.Overview == nil || entry.Overview.Overview.Body != deck.Overview.Body || entry.Overview.Generated {
						t.Fatalf("selected report was not preserved: %+v", entry)
					}
				} else if entry.Overview != nil {
					t.Fatalf("unrelated report expanded: %s", entry.Target)
				}
			}
		})
	}
	// A legacy deck's generated report is still available on demand.
	implementation.Overview = nil
	legacy, err := s.Overview(ctx, OverviewQuery{Deck: implementation.Target})
	if err != nil || legacy.Decks[0].Overview == nil || !legacy.Decks[0].Overview.Generated {
		t.Fatalf("legacy report: %+v, %v", legacy, err)
	}
	for _, target := range []string{"urn:change-saga:example:deck:missing", doc.Section.Target, "urn:change-saga:foreign:deck:architecture"} {
		if _, err := s.Overview(ctx, OverviewQuery{Deck: target}); ErrorCodeOf(err) != CodeNotFound {
			t.Fatalf("%s: expected not_found, got %v", target, err)
		}
	}
	if _, err := s.Overview(ctx, OverviewQuery{Deck: "architecture"}); ErrorCodeOf(err) != CodeInvalidArgument {
		t.Fatalf("bare ID: expected invalid_argument, got %v", err)
	}
}
