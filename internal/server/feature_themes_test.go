package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestFeatureThemeGroupsPreserveIdentityAndIsolateNavigation(t *testing.T) {
	sources := appNavFixture(t)
	doc := sources.document
	doc.Features = append(doc.Features, appNavFeature("settings", "Settings"))
	for _, id := range []string{"reliable", "trusted"} {
		doc.FeatureThemes = append(doc.FeatureThemes, &saga.FeatureTheme{FeatureTheme: applayout.FeatureTheme{ID: id, Title: id, Overview: "# Why\n\n**Authored** rationale. <script>alert(1)</script>", Features: []string{doc.Features[0].Target}}, Target: applayout.FeatureThemeURN(doc.Manifest.ID, id)})
	}
	sources.featureGrouping = "theme"
	nav := makeAppNavTree(sources)
	ids := map[string]bool{}
	pointers := map[*navNodeView]bool{}
	var copies []*navNodeView
	var visit func([]*navNodeView)
	visit = func(nodes []*navNodeView) {
		for _, node := range nodes {
			if node.NodeID != "" && ids[node.NodeID] {
				t.Fatalf("duplicate nav identity %s", node.NodeID)
			}
			ids[node.NodeID] = true
			if pointers[node] {
				t.Fatal("shared mutable navigation pointer")
			}
			pointers[node] = true
			if node.Href == featureHref("billing") {
				copies = append(copies, node)
			}
			visit(node.Children)
		}
	}
	visit(nav)
	if len(copies) != 2 {
		t.Fatalf("shared feature copies = %d", len(copies))
	}
	if !copies[0].Expanded || !copies[1].Expanded {
		t.Fatal("active feature copies must expand")
	}
	copies[0].Children[0].Title = "changed"
	if copies[1].Children[0].Title == "changed" {
		t.Fatal("group rows share children")
	}
	groups := featureGroups(doc, sources.requirements, "theme")
	if len(groups) != 3 || len(groups[2].Features) != 2 {
		t.Fatalf("missing ungrouped: %+v", groups)
	}
	graph := &appGraph{document: doc, requirements: sources.requirements}
	view := groupedFeaturesDirectory(doc, graph, "", "", "theme")
	if len(view.Rows) != 4 {
		t.Fatalf("directory lost memberships: %+v", view.Rows)
	}
	if len(groupedFeaturesDirectory(doc, graph, "", "", "all").Rows) != 3 {
		t.Fatal("all view duplicates features")
	}
	page, err := featureThemePage(doc, "reliable")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page.Body), "<strong>Authored</strong>") || strings.Contains(string(page.Body), "<script>") {
		t.Fatalf("unsafe/unrendered Markdown: %s", page.Body)
	}
	if len(page.Features) != 1 || page.Features[0].Href != featureHref("billing") {
		t.Fatal("theme page feature deep link changed")
	}
}

func TestFeaturePersonaGroupingUsesCurrentProposedStories(t *testing.T) {
	sources := appNavFixture(t)
	buyer := appNavPersonaURN(t, "buyer")
	seller := appNavPersonaURN(t, "seller")
	sources.document.Features = append(sources.document.Features, appNavFeature("settings", "Settings"))
	sources.requirements.Stories = []requirements.Story{
		appNavStory("billing", "one", 1, requirements.StateProposed, buyer, seller),
		appNavStory("billing", "two", 2, requirements.StateProposed, buyer),
		appNavStory("catalog", "three", 3, requirements.StateProposed, seller),
	}
	groups := featureGroups(sources.document, sources.requirements, "persona")
	counts := map[string]int{}
	for _, group := range groups {
		counts[group.ID] = len(group.Features)
	}
	if counts["persona-buyer"] != 1 || counts["persona-seller"] != 2 || counts["ungrouped"] != 1 {
		t.Fatalf("persona membership: %v", counts)
	}
}

func TestFeatureGroupingDefaultsAndPreference(t *testing.T) {
	doc := &saga.Saga{}
	request := httptest.NewRequest(http.MethodGet, "/features", nil)
	if featureGrouping(request, doc) != "all" {
		t.Fatal("legacy default changed")
	}
	doc.FeatureThemes = []*saga.FeatureTheme{{}}
	if featureGrouping(request, doc) != "theme" {
		t.Fatal("themes do not default to theme grouping")
	}
	request.AddCookie(&http.Cookie{Name: "saga-feature-group", Value: "persona"})
	if featureGrouping(request, doc) != "persona" {
		t.Fatal("preference not preserved on deep link")
	}
	request.URL.RawQuery = "group=all"
	if featureGrouping(request, doc) != "all" {
		t.Fatal("explicit grouping does not override cookie")
	}
	request.URL.RawQuery = "group=invalid"
	if featureGrouping(request, doc) != "persona" {
		t.Fatal("invalid group changes preference")
	}
}

func TestThemeItemReferencesOpenCanonicalProductPages(t *testing.T) {
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	for kind, href := range map[string]string{"feature": "/features/core", "persona": "/personas/core", "story": "/requirements/core"} {
		view := &landmarkView{Landmark: saga.Landmark{Target: "urn:change-saga:shop:slide:why:item:point", ItemMeta: &saga.ItemManifest{Record: "urn:change-saga:shop:" + kind + ":core"}}}
		link := view.RecordLink()
		if link == nil || link.Href != href {
			t.Fatalf("%s link: %+v", kind, link)
		}
		var html bytes.Buffer
		if err := tmpl.ExecuteTemplate(&html, "item-record-link", view); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html.String(), `href="`+href+`"`) || !strings.Contains(html.String(), "Open referenced") {
			t.Fatal(html.String())
		}
		view.ItemMeta.Record = "urn:change-saga:other:" + kind + ":core"
		if view.RecordLink() != nil {
			t.Fatal("cross-Saga Item generated a local link")
		}
	}
}
