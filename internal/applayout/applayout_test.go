package applayout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeaturesListValidatesAndResolves(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if features, err := Features(root); err != nil || len(features) != 0 {
		t.Fatalf("app without features = %v, %v", features, err)
	}
	for _, id := range []string{"checkout", "catalog"} {
		if _, err := WriteFeature(root, FeatureManifest{ID: id, Title: strings.ToUpper(id), CreatedAt: time.Unix(0, 0).UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := WriteFeature(root, FeatureManifest{ID: "checkout", Title: "Again"}); err == nil {
		t.Fatal("a second feature with one id was written")
	}
	features, err := Features(root)
	if err != nil || len(features) != 2 || features[0].ID != "catalog" || features[1].Rel != "___features/checkout.feature" {
		t.Fatalf("features = %+v, %v", features, err)
	}
	if feature, err := Require(root, "app", "urn:change-saga:app:feature:catalog"); err != nil || feature.ID != "catalog" {
		t.Fatalf("require by URN = %+v, %v", feature, err)
	}
	if _, err := Require(root, "app", ""); err == nil || !strings.Contains(err.Error(), "known features: catalog, checkout") {
		t.Fatalf("require without a feature = %v", err)
	}
	if got := FeatureOfPath("___features/checkout.feature/___requirements/stories/pay.story"); got != "checkout" {
		t.Fatalf("FeatureOfPath = %q", got)
	}
	if got := FeatureOfPath("___overview/pitch.fragment"); got != "" {
		t.Fatalf("app-level path belongs to feature %q", got)
	}
	if err := os.Mkdir(filepath.Join(root, FeaturesDir, "stray"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Features(root); err == nil {
		t.Fatal("a directory that is not <id>.feature was accepted")
	}
}

func TestFeatureRootsAtTheAppRootAreRefused(t *testing.T) {
	root := t.TempDir()
	if err := RejectFeatureRootsAtAppRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, RequirementsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RejectFeatureRootsAtAppRoot(root); err == nil {
		t.Fatal("a root-level ___requirements was accepted")
	}
}

func TestUniqueIDsNameBothFeatures(t *testing.T) {
	ids := NewUniqueIDs("story")
	if err := ids.Claim("pay", "checkout"); err != nil {
		t.Fatal(err)
	}
	if err := ids.Claim("pay", "catalog"); err == nil || !strings.Contains(err.Error(), `"checkout" and "catalog"`) {
		t.Fatalf("duplicate across features = %v", err)
	}
}

// Features are presented in the order they were created, not alphabetically;
// features created at the same instant keep ID order.
func TestInCreationOrderPresentsFeaturesAsTheyWereIntroduced(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 9, 19, 20, 47, second, 0, time.UTC) }
	features := []Feature{
		{FeatureManifest: FeatureManifest{ID: "agent-loop", CreatedAt: at(9)}},
		{FeatureManifest: FeatureManifest{ID: "format", CreatedAt: at(1)}},
		{FeatureManifest: FeatureManifest{ID: "comparison", CreatedAt: at(5)}},
		{FeatureManifest: FeatureManifest{ID: "code-evidence", CreatedAt: at(5)}},
	}
	got := []string{}
	for _, feature := range InCreationOrder(features) {
		got = append(got, feature.ID)
	}
	if strings.Join(got, ",") != "format,code-evidence,comparison,agent-loop" {
		t.Fatalf("order = %v", got)
	}
	if features[0].ID != "agent-loop" {
		t.Fatal("InCreationOrder must not reorder its argument")
	}
}
