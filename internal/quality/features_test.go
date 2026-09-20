package quality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
)

func addFeature(t *testing.T, root, id string) {
	t.Helper()
	if _, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: id, Title: strings.ToUpper(id), CreatedAt: fixtureTime}); err != nil {
		t.Fatal(err)
	}
}

func TestSameTestCaseIDInTwoFeaturesFailsToLoad(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	addFeature(t, root, "billing")
	second := filepath.Join(applayout.FeatureDir(root, "billing"), RootDir, "test-cases", "deadline.test")
	if err := os.CopyFS(second, os.DirFS(testPackage(root, "deadline"))); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unique across the app") {
		t.Fatalf("Load() error = %v; want an app-unique ID error", err)
	}
}

func TestSamePolicyIDInTwoFeaturesFailsToLoad(t *testing.T) {
	root := newQualitySaga(t, true)
	addFeature(t, root, "billing")
	writeJSON(t, filepath.Join(featureQuality(root), "policies", "cutoff.json"), validPolicy("cutoff"))
	writeJSON(t, filepath.Join(applayout.FeatureDir(root, "billing"), RootDir, "policies", "cutoff.json"), validPolicy("cutoff"))
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unique across the app") {
		t.Fatalf("Load() error = %v; want an app-unique ID error", err)
	}
}

func TestQualityLoadsEveryFeatureIntoOneDocument(t *testing.T) {
	root := newQualitySaga(t, false)
	addFeature(t, root, "billing")
	addDeadline(t, root)
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "billing", ID: "another", RevisionID: "r1", Definition: deadlineDefinition()}); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if document.Adoption != Adopted || len(document.TestCases) != 2 {
		t.Fatalf("document = %#v", document)
	}
	if first, second := document.TestCases[0], document.TestCases[1]; first.Identity.ID != "another" || first.Feature != "billing" || second.Identity.ID != "deadline" || second.Feature != "core" {
		t.Fatalf("combined test cases = %s@%s, %s@%s", first.Identity.ID, first.Feature, second.Identity.ID, second.Feature)
	}
}

func TestAddTestCaseWritesIntoTheNamedFeature(t *testing.T) {
	root := newQualitySaga(t, false)
	addFeature(t, root, "billing")
	result, err := AddTestCase(root, AddTestCaseInput{Feature: "billing", ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), CreatedAt: fixtureTime, RequestID: "add-deadline"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "___features/billing.feature/___quality/test-cases/deadline.test" {
		t.Fatalf("path = %q", result.Path)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(result.Path), "test-case.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(featureQuality(root)); !os.IsNotExist(err) {
		t.Fatalf("core feature quality root was touched: %v", err)
	}
	if testCase := loadDeadline(t, root); testCase.Feature != "billing" {
		t.Fatalf("feature = %q", testCase.Feature)
	}

	// Existing-record operations write into the feature holding the record.
	revised, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{result.CurrentHeads[0]}, Definition: deadlineDefinition()})
	if err != nil {
		t.Fatal(err)
	}
	if revised.Path != "___features/billing.feature/___quality/test-cases/deadline.test/revisions/r2.json" {
		t.Fatalf("revision path = %q", revised.Path)
	}

	// A replay naming a different feature is a conflict, not a replay.
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), RequestID: "add-deadline"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("cross-feature replay error = %v", err)
	}
}

func TestCreatesRequireAKnownFeature(t *testing.T) {
	root := newQualitySaga(t, false)
	if _, err := AddTestCase(root, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition()}); err == nil || !strings.Contains(err.Error(), "a feature is required") {
		t.Fatalf("missing feature error = %v", err)
	}
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "nope", ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition()}); err == nil || !strings.Contains(err.Error(), `feature "nope" does not exist`) {
		t.Fatalf("unknown feature error = %v", err)
	}
	policy := SetPolicyInput{
		Criterion:     "urn:change-saga:checkout:story:refund:criterion:cutoff",
		StoryRevision: "urn:change-saga:checkout:story:refund:revision:r1",
		RequiredKinds: []CoverageKind{CoveragePositive}, AllowedAutomation: []Automation{AutomationManual}, Rationale: "r",
	}
	if _, err := SetPolicy(root, policy); err == nil || !strings.Contains(err.Error(), "a feature is required") {
		t.Fatalf("missing policy feature error = %v", err)
	}
}

func TestLegacyRootQualityIsRejected(t *testing.T) {
	root := newQualitySaga(t, false)
	if err := os.Mkdir(filepath.Join(root, RootDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "belongs in a feature") {
		t.Fatalf("Load() error = %v", err)
	}
}
