package saga

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func overviewDeck() *Deck {
	deck := &Deck{DeckManifest: DeckManifest{ID: "main"}, Target: DeckTarget("demo", "main")}
	for _, id := range []string{"one", "two"} {
		slide := &Slide{SlideManifest: SlideManifest{ID: id, Title: id, MediaType: "image/svg+xml"}, Target: SlideTarget("demo", id)}
		slide.Items = []*Item{{ItemManifest: ItemManifest{ID: "node", Label: "Node"}, Target: ItemTarget("demo", id, "node")}}
		deck.Slides = append(deck.Slides, slide)
	}
	return deck
}

func TestDeckOverviewEverySlideAndLegacy(t *testing.T) {
	deck := overviewDeck()
	legacy := deck.OverviewReport()
	if !legacy.Generated || !legacy.Validation.Valid || legacy.Complete || len(legacy.UncoveredSlides) != 2 || len(legacy.Validation.Issues) != 3 {
		t.Fatalf("legacy: %+v", legacy)
	}
	if deck.Overview != nil || !strings.Contains(legacy.Overview.Body, "Generated slide directory") {
		t.Fatal("fallback was persisted or not labeled")
	}
	again, _ := deck.EffectiveOverview()
	if !reflect.DeepEqual(legacy.Overview, again) {
		t.Fatal("fallback was nondeterministic")
	}
	deck.Overview = &DeckOverview{Body: "[First](annotation:first)\n\n| Claim | Link |\n| --- | --- |\n| Second | [Node](annotation:second) |\n\n![Visual](slide:one)", Annotations: []OverviewAnnotation{
		{ID: "first", Label: "First", Slide: "one"},
		{ID: "second", Label: "Second", Slide: deck.Slides[1].Target, Item: deck.Slides[1].Items[0].Target},
	}}
	report := deck.OverviewReport()
	if !report.Complete || !report.Validation.Valid || len(report.CoveredSlides) != 2 || len(report.Validation.Issues) != 0 {
		t.Fatalf("authored: %+v", report)
	}
	if report.References[1].Item != deck.Slides[1].Items[0].Target {
		t.Fatal("Item was not resolved")
	}
	deck.Overview.Body = "[First](annotation:first)\n\n`[code](annotation:second)`\n\n```md\n[code](annotation:second)\n```\n"
	report = deck.OverviewReport()
	if report.Complete || !report.Validation.Valid || len(report.CoveredSlides) != 1 || len(report.UncoveredSlides) != 1 {
		t.Fatalf("code counted: %+v", report)
	}
}

func TestDeckOverviewInvalidReferences(t *testing.T) {
	for _, test := range []struct {
		name, body  string
		annotations []OverviewAnnotation
	}{
		{"missing slide", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: "missing"}}},
		{"cross deck", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: SlideTarget("demo", "other")}}},
		{"cross saga", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: SlideTarget("other", "one")}}},
		{"prefix", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: SlideTarget("demo", "one") + ":item:node"}}},
		{"wrong Item owner", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: "one", Item: ItemTarget("demo", "two", "node")}}},
		{"missing Item", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: "one", Item: "missing"}}},
		{"unknown citation", "[A](annotation:unknown)", nil},
		{"duplicate", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: "A", Slide: "one"}, {ID: "a", Label: "B", Slide: "two"}}},
		{"invalid id", "[A](annotation:a/b)", []OverviewAnnotation{{ID: "a/b", Label: "A", Slide: "one"}}},
		{"blank label", "[A](annotation:a)", []OverviewAnnotation{{ID: "a", Label: " ", Slide: "one"}}},
		{"blank body", "  ", nil},
		{"bad visual", "![A](slide:missing)", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			deck := overviewDeck()
			deck.Overview = &DeckOverview{Body: test.body, Annotations: test.annotations}
			report := deck.OverviewReport()
			if report.Validation.Valid || report.Complete || len(report.CoveredSlides) > 0 {
				t.Fatalf("invalid report: %+v", report)
			}
		})
	}
	deck := overviewDeck()
	deck.Slides[0].MediaType = "text/html"
	deck.Overview = &DeckOverview{Body: "![A](slide:one)"}
	if ValidateDeckOverview(deck).Valid {
		t.Fatal("HTML slide was accepted as an image")
	}
}

func TestFrontFallbackAndSnapshot(t *testing.T) {
	slide := overviewDeck().Slides[0]
	if got := slide.FrontBullets(); !reflect.DeepEqual(got, []string{"Node"}) {
		t.Fatal(got)
	}
	slide.Takeaway = "Summary"
	if got := slide.FrontBullets(); !reflect.DeepEqual(got, []string{"Summary"}) {
		t.Fatal(got)
	}
	before, _ := SlideRevisionSnapshot(SlideTransactionRevision{Slide: slide.SlideManifest})
	slide.Front = []string{"Authored", "Second"}
	got := slide.FrontBullets()
	got[0] = "mutated"
	if slide.Front[0] != "Authored" {
		t.Fatal("front result aliases manifest")
	}
	after, _ := SlideRevisionSnapshot(SlideTransactionRevision{Slide: slide.SlideManifest})
	if before == after {
		t.Fatal("front did not change authoring snapshot")
	}
	slide.Front = nil
	serialized, _ := json.Marshal(slide.SlideManifest)
	if strings.Contains(string(serialized), `"front"`) {
		t.Fatal("absent front changes legacy serialization")
	}
	validation := Validation{Valid: true}
	validateFront([]string{" "}, "slide", &validation)
	if len(validation.Issues) != 1 || validation.Issues[0].Severity != "error" {
		t.Fatal("blank front accepted")
	}
}
