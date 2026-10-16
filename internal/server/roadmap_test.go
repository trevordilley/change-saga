package server

import (
	"github.com/twentyideas/changesaga/internal/applayout"
	"os"
	"path/filepath"
	"testing"
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
