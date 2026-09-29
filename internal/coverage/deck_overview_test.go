package coverage

import (
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestOverviewReferencesNeverTransferCodeCoverage(t *testing.T) {
	item := &saga.Item{ItemManifest: saga.ItemManifest{ID: "node", Label: "Node", Documentation: &saga.DocumentationLink{Target: "urn:change-saga:test:component:engine", Revision: "urn:change-saga:test:component:engine:revision:r1"}}, Target: saga.ItemTarget("test", "flow", "node")}
	slide := &saga.Slide{SlideManifest: saga.SlideManifest{ID: "flow", Front: []string{"Claims a change"}}, Target: saga.SlideTarget("test", "flow"), Items: []*saga.Item{item}}
	deck := &saga.Deck{DeckManifest: saga.DeckManifest{ID: "main", Overview: &saga.DeckOverview{Body: "[Flow](annotation:a) and [Node](annotation:b)", Annotations: []saga.OverviewAnnotation{{ID: "a", Label: "Flow", Slide: "flow"}, {ID: "b", Label: "Node", Slide: "flow", Item: "node"}}}}, Target: saga.DeckTarget("test", "main"), Slides: []*saga.Slide{slide}}
	if !deck.OverviewReport().Complete {
		t.Fatal("fixture overview is not complete")
	}
	changes := gitdiff.ChangeSet{BaseOID: testBase, HeadOID: testHead, Atoms: []gitdiff.Atom{lineAtom("app.go", "new", 1)}}
	document := &saga.Saga{Decks: []*saga.Deck{deck}, Section: &saga.Section{Target: saga.SagaTarget("test"), Children: []*saga.Section{saga.ProjectDeck(deck)}}}
	report := Evaluate(context.Background(), document, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if report.Summary.Covered != 0 || report.Summary.Uncovered != 1 {
		t.Fatalf("overview/documentation transferred code coverage: %+v", report.Summary)
	}
	item.Code = []saga.CodeFile{{References: []coderef.Reference{lineReference(testHead, "app.go", 1, 1)}}}
	document.Section.Children[0] = saga.ProjectDeck(deck)
	report = Evaluate(context.Background(), document, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if report.Summary.Covered != 1 || report.Summary.Overlapping != 0 {
		t.Fatalf("overview duplicated Item evidence: %+v", report.Summary)
	}
	for _, assignments := range report.Ownership {
		for _, assignment := range assignments {
			if assignment.Target != item.Target {
				t.Fatalf("overview gained ownership: %+v", assignment)
			}
		}
	}
}
