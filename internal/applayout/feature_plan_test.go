package applayout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeaturePlanLegacyAndUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	f, err := WriteFeature(root, FeatureManifest{ID: "core", Title: "Core"})
	if err != nil {
		t.Fatal(err)
	}
	features, err := Features(root)
	if err != nil || len(features) != 1 || features[0].Plan.State() != "built" {
		t.Fatalf("legacy feature: %+v %v", features, err)
	}
	target := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(target, []byte(`{"$schema":"https://changesaga.dev/schema/v5/feature-plan.schema.json","version":5,"status":"planned"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(f.Dir, FeaturePlanName)); err != nil {
		t.Fatal(err)
	}
	if _, err := Features(root); err == nil {
		t.Fatal("followed planning symlink")
	}
}
