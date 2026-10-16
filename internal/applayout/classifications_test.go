package applayout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClassificationCatalogRejectsMalformedRecords(t *testing.T) {
	for _, scenario := range []string{"duplicate name", "unknown field", "mismatched id", "symlink file", "symlink directory"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, LabelsDir)
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			record := Classification{Schema: ClassificationSchema("label"), Version: 5, ID: "devops", Title: "DevOps"}
			data, _ := json.Marshal(record)
			path := filepath.Join(dir, "devops.json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "duplicate name":
				record.ID, record.Title = "other", "DEVOPS"
				data, _ = json.Marshal(record)
				if err := os.WriteFile(filepath.Join(dir, "other.json"), data, 0644); err != nil {
					t.Fatal(err)
				}
			case "unknown field":
				data = append(data[:len(data)-1], []byte(`,"features":[]}`)...)
				if err := os.WriteFile(path, data, 0644); err != nil {
					t.Fatal(err)
				}
			case "mismatched id":
				if err := os.Rename(path, filepath.Join(dir, "other.json")); err != nil {
					t.Fatal(err)
				}
			case "symlink file":
				if err := os.Rename(path, filepath.Join(root, "external.json")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "external.json"), path); err != nil {
					t.Skip(err)
				}
			case "symlink directory":
				if err := os.Rename(dir, filepath.Join(root, "external")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "external"), dir); err != nil {
					t.Skip(err)
				}
			}
			if _, err := ReadClassifications(root); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestLegacyMilestoneNamesRemainLiteral(t *testing.T) {
	catalog := Classifications{Milestones: []Classification{{ID: "v2", Title: "Launch"}}}
	if err := catalog.ValidatePlan("demo", FeaturePlan{Milestone: "v2"}); err != nil {
		t.Fatal(err)
	}
	if got := catalog.Title("demo", "milestone", "v2"); got != "v2" {
		t.Fatalf("legacy text became a reference: %s", got)
	}
	if got := catalog.Title("demo", "milestone", ClassificationURN("demo", "milestone", "v2")); got != "Launch" {
		t.Fatal(got)
	}
	if err := catalog.ValidatePlan("demo", FeaturePlan{Milestone: ClassificationURN("other", "milestone", "v2")}); err == nil {
		t.Fatal("cross-Saga reference accepted")
	}
}
