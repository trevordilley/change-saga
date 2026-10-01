package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

func overviewFixture() *saga.Deck {
	slide := &saga.Slide{SlideManifest: saga.SlideManifest{ID: "flow", Title: "Request flow", MediaType: "image/svg+xml", Entrypoint: "assets/flow.svg", Takeaway: "One request reaches the handler.", Front: []string{"Requests enter once.", "The handler owns the response."}}, Target: "urn:change-saga:demo:slide:flow"}
	slide.Items = []*saga.Item{{ItemManifest: saga.ItemManifest{ID: "handler", Label: "Request handler", Description: "Validates input before dispatch."}, Target: slide.Target + ":item:handler", Code: []saga.CodeFile{{References: []coderef.Reference{{Path: "src/handler.go"}}}}}}
	return &saga.Deck{DeckManifest: saga.DeckManifest{ID: "technical", Title: "Technical", Objective: "Explain the request.", Overview: &saga.DeckOverview{Body: "# Request report\n\n[Handler](annotation:handler).\n\n| Step | Result |\n| --- | --- |\n| Validate | Dispatch |\n\n![Flow](slide:flow)\n\n`[example](annotation:literal)`\n\n<script>unsafe()</script>", Annotations: []saga.OverviewAnnotation{{ID: "handler", Label: "Handler evidence", Slide: slide.Target, Item: "handler"}}}}, Target: "urn:change-saga:demo:deck:technical", Slides: []*saga.Slide{slide}}
}

func TestDeckOverviewRendersStructuredReportAndResolvedEvidence(t *testing.T) {
	deck := overviewFixture()
	view := makeDeckOverview(deck)
	if view.Generated || len(view.References) != 1 {
		t.Fatalf("authored report lost: %#v", view)
	}
	ref := view.References[0]
	if ref.Title != "Request flow · Request handler" || ref.ItemTarget != deck.Slides[0].Items[0].Target || !strings.Contains(strings.Join(ref.Details, " "), "src/handler.go") {
		t.Fatalf("wrong resolution: %#v", ref)
	}
	var out bytes.Buffer
	if err := serverTemplate(t).ExecuteTemplate(&out, "deck-overview", view); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<table>", "<td>Dispatch</td>", `/f/flow/assets/flow.svg`, `href="#` + ref.DOMID + `"`, `aria-label="Overview references"`, "Request handler", "src/handler.go", "annotation:literal", "&lt;script&gt;"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "<script>") {
		t.Fatal("authored script escaped sanitizer")
	}
	review := makeDeckOverview(deck, "pr-1")
	if !strings.Contains(string(review.Body), `/reviews/pr-1/visual/flow`) {
		t.Fatalf("review visual route missing: %s", review.Body)
	}
}

func TestDeckOverviewRejectsCrossDeckAndMissingReferences(t *testing.T) {
	deck := overviewFixture()
	deck.Overview.Body = "[Broken](annotation:missing) [Foreign](annotation:foreign) ![Bad](slide:other)"
	deck.Overview.Annotations = []saga.OverviewAnnotation{{ID: "foreign", Label: "Foreign", Slide: "urn:change-saga:elsewhere:slide:flow", Item: "handler"}}
	view := makeDeckOverview(deck)
	if len(view.References) != 2 {
		t.Fatalf("missing broken references: %#v", view.References)
	}
	for _, ref := range view.References {
		if ref.Problem == "" || ref.ItemTarget != "" {
			t.Fatalf("invalid ref resolved: %#v", ref)
		}
	}
	if strings.Contains(string(view.Body), "<img") || !strings.Contains(string(view.Body), "Unavailable overview visual") {
		t.Fatalf("invalid visual rendered: %s", view.Body)
	}
}

func TestDeckOverviewLegacyDirectoryAndFrontFallback(t *testing.T) {
	deck := overviewFixture()
	deck.Overview = nil
	deck.Slides[0].Front = nil
	view := makeDeckOverview(deck)
	if !view.Generated || len(view.Slides) != 1 || view.Slides[0].Bullets[0] != deck.Slides[0].Takeaway {
		t.Fatalf("fallback lost: %#v", view)
	}
	deck.Slides[0].Takeaway = ""
	view = makeDeckOverview(deck)
	if view.Slides[0].Bullets[0] != "Request handler" {
		t.Fatalf("item fallback lost: %#v", view.Slides)
	}
	var out bytes.Buffer
	if err := serverTemplate(t).ExecuteTemplate(&out, "deck-overview", view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Generated slide directory") || !strings.Contains(out.String(), `data-overview-slide`) {
		t.Fatal(out.String())
	}
}

func TestDeckOverviewEndpointReadsAuthoredManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	asset := writeEmbeddedSlideFixture(t, root)
	target := saga.DeckTarget("visual", "flow")
	name, _ := saga.FlatDeckFilename(target, 0)
	manifest := saga.DeckManifest{Version: 4, ID: "flow", Title: "Complex flow", Role: "change", Objective: "Explain requests.", Overview: &saga.DeckOverview{Body: "# Authored report\n\n[The surprise](annotation:premise)\n\n| Input | Outcome |\n| --- | --- |\n| Request | Response |\n\n![Flow](slide:change)", Annotations: []saga.OverviewAnnotation{{ID: "premise", Label: "Surprise evidence", Slide: "change", Item: "premise"}}}}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, filepath.Join(serverFeatureDir(root), saga.EmbeddedSlidesDir, "flow"+saga.EmbeddedDeckSuffix, name), string(data))
	handler := newMux(&app{root: root, template: serverTemplate(t)})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/decks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /decks: %d %s", response.Code, response.Body.String())
	}
	for _, want := range []string{"Authored report", "<table>", "<td>Response</td>", "/f/change/" + asset, "Surprise evidence", "Complex flow · Surprise", "data-slide-front", "The implementation path is explicit."} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("missing %q from endpoint: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "data-overview-generated") {
		t.Fatal("authored overview was replaced by fallback")
	}
}
