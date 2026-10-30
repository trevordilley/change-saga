package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/quality"
)

func TestRoadmapUsesFeatureNavigationAndThemeGroups(t *testing.T) {
	sources := appNavFixture(t)
	planned := sources.document.Features[0]
	planned.Plan = applayout.FeaturePlan{Status: "planned", Priority: 1, Milestone: "Launch"}
	sources.pageFeature = planned.ID
	nodes := makeAppNavTree(sources)
	roadmap := findNav(t, nodes, "Roadmap")
	built := findNav(t, nodes, "Features")
	if len(roadmap.Children) != 1 || roadmap.Children[0].Href != featureHref(planned.ID) || !roadmap.Children[0].Expanded {
		t.Fatalf("roadmap does not reuse feature navigation: %+v", roadmap)
	}
	for _, node := range built.Children {
		if node.Href == featureHref(planned.ID) {
			t.Fatal("planned feature leaked into built list")
		}
	}
	if len(sources.document.Features) != 2 {
		t.Fatal("projection changed canonical model")
	}
	sources.reviewSide = true
	for _, node := range makeAppNavTree(sources) {
		if node.Title == "Roadmap" && !node.Hidden {
			t.Fatal("roadmap leaked into Reviews")
		}
	}
}

func TestRoadmapPlanInvalidatesOutline(t *testing.T) {
	root := t.TempDir()
	feature, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: "next", Title: "Next"})
	if err != nil {
		t.Fatal(err)
	}
	before, _, _, err := sagaFingerprints(root, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(feature.Dir, applayout.FeaturePlanName)
	if err := os.WriteFile(path, []byte(`{"status":"planned"}`), 0600); err != nil {
		t.Fatal(err)
	}
	added, _, _, err := sagaFingerprints(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"status":"built"}`), 0600); err != nil {
		t.Fatal(err)
	}
	edited, _, _, err := sagaFingerprints(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if before == added || added == edited {
		t.Fatal("planning updates do not invalidate cached feature placement")
	}
}

// A Roadmap feature that augments an existing feature says so on the Roadmap,
// and the existing feature's page lists it under Planned extensions.
func TestRoadmapAndFeaturePageLinkAugmentingFeatures(t *testing.T) {
	sources := appNavFixture(t)
	doc := sources.document
	billing, catalog := doc.Features[0], doc.Features[1]
	catalog.Plan = applayout.FeaturePlan{Status: "planned", Augments: billing.Target}
	graph := newAppGraph(doc, sources.requirements, quality.Document{})
	roadmap := featureCollectionDirectory(doc, graph, "", "", "all", true)
	if len(roadmap.Rows) != 1 || roadmap.Rows[0].Cells[1].Text != "→ extends Billing" || roadmap.Rows[0].Cells[1].Href != featureHref("billing") {
		t.Fatalf("roadmap must show what the planned feature extends: %+v", roadmap.Rows)
	}
	page, err := graph.featurePage("billing")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Extensions) != 1 || page.Extensions[0].Href != featureHref("catalog") {
		t.Fatalf("billing must list its planned extensions: %+v", page.Extensions)
	}
	planned, err := graph.featurePage("catalog")
	if err != nil || planned.Extends == nil || planned.Extends.Href != featureHref("billing") {
		t.Fatalf("the planned feature must link what it extends: %+v %v", planned, err)
	}
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := tmpl.ExecuteTemplate(&html, "feature-page", page); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "Planned extensions") || !strings.Contains(html.String(), `href="`+featureHref("catalog")+`"`) {
		t.Fatalf("feature page lacks its planned extensions:\n%s", html.String())
	}
	html.Reset()
	if err := tmpl.ExecuteTemplate(&html, "feature-page", planned); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "→ extends <a href=\""+featureHref("billing")+"\">Billing</a>") {
		t.Fatalf("planned feature page lacks its extends link:\n%s", html.String())
	}
}
