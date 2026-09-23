package server

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestSlideStorySummaryUsesOnlyItsExactItems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("fixture: %v", err)
	}
	slide := document.Decks[0].Slides[0]
	fragment := findFragmentByTarget(document, slide.Target)
	item := slide.Items[0].Target
	story := "urn:change-saga:visual:story:checkout"
	revision := story + ":revision:r1"
	records := requirements.Document{SagaID: "visual", Stories: []requirements.Story{{
		Identity: requirements.StoryIdentity{ID: "checkout"}, RevisionHeads: []string{revision},
		CurrentRevision: &requirements.Revision{ID: "r1", Story: story, Title: "Safe checkout", AcceptanceCriteria: []requirements.Criterion{{ID: "safe", Statement: "Cart is preserved"}}},
	}}}
	add := func(id, from, to string, state requirements.RelationState) {
		records.Relations = append(records.Relations, requirements.Relation{ID: id, Type: requirements.RelationExplains, From: from, To: to, ToRevision: revision, State: state, Scope: requirements.ScopeSelf})
	}
	add("direct", item, story, requirements.RelationActive)
	add("criterion", item, story+":criterion:safe", requirements.RelationActive)
	add("legacy", slide.Target, story, requirements.RelationActive)
	add("legacy-deck", document.Decks[0].Target, story, requirements.RelationActive)
	add("unrelated", saga.ItemTarget("visual", "another-slide", "node"), story, requirements.RelationActive)
	add("old", item, story, requirements.RelationSuperseded)
	view := makeFragmentView(fragment, viewScope{})
	if err := decorateFragmentStories(document, records, view); err != nil {
		t.Fatal(err)
	}
	if view.Stories.Count != 1 || len(view.Stories.Links) != 2 || len(view.Stories.Legacy) != 2 {
		t.Fatalf("summary duplicates a story or inherits broad links: %#v", view.Stories)
	}
	if view.LandmarkViews[0].Stories.Count != 1 || len(view.LandmarkViews[0].Stories.Links) != 2 || len(view.LandmarkViews[0].Stories.Legacy) != 0 {
		t.Fatal("Item inherited a broad or unrelated link")
	}
	var html bytes.Buffer
	if err := serverTemplate(t).ExecuteTemplate(&html, "fragment", view); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"data-open-stories", "Safe checkout", "Cart is preserved", "Needs element assignment", "data-status=\"current\"", "Linked stories: Safe checkout"} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("missing %q", want)
		}
	}
	// Missing and stale endpoints are visible, never presented as current.
	records.Relations[0].FromContentDigest = "sha256:" + strings.Repeat("f", 64)
	records.Relations[1].To = "urn:change-saga:visual:story:missing"
	view = makeFragmentView(fragment, viewScope{})
	if err := decorateFragmentStories(document, records, view); err != nil {
		t.Fatal(err)
	}
	if view.Stories.Links[0].Status != requirements.CurrencyStale || view.Stories.Links[1].Status != requirements.CurrencyInvalid {
		t.Fatalf("currency hidden: %#v", view.Stories.Links)
	}
	// A broad-only slide offers migration guidance but no Item story control.
	records.Relations = records.Relations[2:4]
	view = makeFragmentView(fragment, viewScope{})
	if err := decorateFragmentStories(document, records, view); err != nil {
		t.Fatal(err)
	}
	if view.Stories.Count != 0 || view.LandmarkViews[0].Stories.Count != 0 || len(view.Stories.Legacy) != 2 {
		t.Fatal("legacy links leaked into element summary")
	}
}
