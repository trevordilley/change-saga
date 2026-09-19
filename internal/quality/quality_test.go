package quality

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/diffuri"
)

var fixtureTime = time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)

func TestLoadProjectsQualityRecordsAndGraphHeads(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	writeJSON(t, filepath.Join(epicQuality(root), "policies", "cutoff.json"), validPolicy("cutoff"))

	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if document.Adoption != Adopted || document.SagaID != "checkout" || len(document.TestCases) != 1 || len(document.Policies) != 1 {
		t.Fatalf("document = %#v", document)
	}
	testCase := document.TestCases[0]
	if testCase.CurrentRevision == nil || testCase.CurrentRevision.ID != "r1" || testCase.RevisionConflict() {
		t.Fatalf("revision projection = heads %v current %#v", testCase.RevisionHeads, testCase.CurrentRevision)
	}
	if testCase.CurrentLifecycle == nil || testCase.CurrentLifecycle.State != StateActive || testCase.LifecycleConflict() {
		t.Fatalf("lifecycle projection = heads %v current %#v", testCase.LifecycleHeads, testCase.CurrentLifecycle)
	}
	if testCase.CurrentRun == nil || testCase.CurrentRun.Result != RunPassed || testCase.RunConflict() {
		t.Fatalf("run projection = heads %v current %#v", testCase.RunHeads, testCase.CurrentRun)
	}
	if !testCase.CurrentRun.Current || testCase.HeadRun == nil || testCase.HeadRun.ID != "ci-1" {
		t.Fatalf("current run currency = current %#v head %#v", testCase.CurrentRun, testCase.HeadRun)
	}
	if len(testCase.EvidenceHeads) != 1 || !strings.HasSuffix(testCase.EvidenceHeads[0], ":evidence:test-code") {
		t.Fatalf("evidence heads = %v", testCase.EvidenceHeads)
	}
	if len(document.PolicySets) != 1 || document.PolicySets[0].Current == nil || document.PolicySets[0].Current.ID != "cutoff" {
		t.Fatalf("policy projection = %#v", document.PolicySets)
	}
	if len(testCase.CurrentRevision.Steps) != 2 || testCase.CurrentRevision.Steps[0].ID != "submit" || testCase.CurrentRevision.Steps[1].ID != "inspect" {
		t.Fatalf("ordered steps changed: %#v", testCase.CurrentRevision.Steps)
	}
}

func TestRunAndEvidenceCurrencyRequiresCurrentPinsAndSource(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	path := filepath.Join(testPackage(root, "deadline"), "evidence", "test-code.json")
	evidence := validEvidence("test-code", EvidenceTestImplementation)
	selector, err := diffuri.Build(diffuri.Reference{
		Repository: "https://example.com/repo.git", Base: "older-base", Head: "older-head", Kind: "line",
		Path: "internal/refund_test.go", Side: "new", Start: 41, End: 88,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence.Diffs = []string{selector}
	writeJSON(t, path, evidence)

	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	testCase := document.TestCases[0]
	if testCase.HeadRun == nil || testCase.CurrentRun != nil {
		t.Fatalf("stale run projection = head %#v current %#v", testCase.HeadRun, testCase.CurrentRun)
	}
	if testCase.Evidence[0].Current || !strings.Contains(strings.Join(testCase.Evidence[0].StaleReasons, ","), "source comparison changed") {
		t.Fatalf("evidence currency = %#v", testCase.Evidence[0])
	}
	if testCase.Runs[0].Current || !strings.Contains(strings.Join(testCase.Runs[0].StaleReasons, ","), "evidence is not current") {
		t.Fatalf("run currency = %#v", testCase.Runs[0])
	}
}

func TestAbsentAndEmptyQualityRootsHaveDistinctAdoptionStates(t *testing.T) {
	root := newQualitySaga(t, false)
	document, err := Load(root)
	if err != nil || document.Adoption != NotAdopted {
		t.Fatalf("absent root = %s, %v", document.Adoption, err)
	}
	if err := os.Mkdir(epicQuality(root), 0o755); err != nil {
		t.Fatal(err)
	}
	document, err = Load(root)
	if err != nil || document.Adoption != AdoptedEmpty {
		t.Fatalf("empty root = %s, %v", document.Adoption, err)
	}
}

func TestConcurrentDefinitionLifecycleAndRunHeadsRemainConflicts(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	packageDir := testPackage(root, "deadline")

	for _, revision := range []Revision{
		revisionFrom("r2-a", []string{revisionURN("r1")}, "First branch"),
		revisionFrom("r2-b", []string{revisionURN("r1")}, "Second branch"),
	} {
		writeJSON(t, filepath.Join(packageDir, "revisions", revision.ID+".json"), revision)
	}
	for _, event := range []LifecycleEvent{
		eventFrom("deprecated", []string{eventURN("active")}, StateDeprecated),
		eventFrom("retired", []string{eventURN("active")}, StateRetired),
	} {
		writeJSON(t, filepath.Join(packageDir, "events", event.ID+".json"), event)
	}
	for _, run := range []Run{
		runFrom("ci-2-a", []string{runURN("ci-1")}, RunPassed),
		runFrom("ci-2-b", []string{runURN("ci-1")}, RunFailed),
	} {
		writeJSON(t, filepath.Join(packageDir, "runs", run.ID+".json"), run)
	}

	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	testCase := document.TestCases[0]
	if !testCase.RevisionConflict() || testCase.CurrentRevision != nil || len(testCase.RevisionHeads) != 2 {
		t.Fatalf("revision conflict = heads %v current %#v", testCase.RevisionHeads, testCase.CurrentRevision)
	}
	if !testCase.LifecycleConflict() || testCase.CurrentLifecycle != nil || len(testCase.LifecycleHeads) != 2 {
		t.Fatalf("lifecycle conflict = heads %v current %#v", testCase.LifecycleHeads, testCase.CurrentLifecycle)
	}
	if !testCase.RunConflict() || testCase.CurrentRun != nil || len(testCase.RunHeads) != 2 {
		t.Fatalf("run conflict = heads %v current %#v", testCase.RunHeads, testCase.CurrentRun)
	}
}

func TestPolicyAndEvidenceSupersessionUsesGraphHeads(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	packageDir := testPackage(root, "deadline")

	oldPolicy := validPolicy("cutoff-v1")
	newPolicy := validPolicy("cutoff-v2")
	newPolicy.Supersedes = []string{"urn:change-saga:checkout:quality-policy:cutoff-v1"}
	writeJSON(t, filepath.Join(epicQuality(root), "policies", "cutoff-v1.json"), oldPolicy)
	writeJSON(t, filepath.Join(epicQuality(root), "policies", "cutoff-v2.json"), newPolicy)

	oldEvidence := validEvidence("artifact-v1", EvidenceExecutionArtifact)
	oldEvidence.Diffs = []string{}
	oldEvidence.Verifications = []string{"urn:change-saga:checkout:verification:ci-log"}
	newEvidence := validEvidence("artifact-v2", EvidenceExecutionArtifact)
	newEvidence.Diffs = []string{}
	newEvidence.Verifications = []string{"urn:change-saga:checkout:verification:ci-log-2"}
	newEvidence.Supersedes = []string{"urn:change-saga:checkout:test-case:deadline:evidence:artifact-v1"}
	writeJSON(t, filepath.Join(packageDir, "evidence", "artifact-v1.json"), oldEvidence)
	writeJSON(t, filepath.Join(packageDir, "evidence", "artifact-v2.json"), newEvidence)

	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	set := document.PolicySets[0]
	if set.Conflict() || set.Current == nil || set.Current.ID != "cutoff-v2" || len(set.Heads) != 1 {
		t.Fatalf("policy heads = %#v", set)
	}
	heads := strings.Join(document.TestCases[0].EvidenceHeads, ",")
	if strings.Contains(heads, "artifact-v1") || !strings.Contains(heads, "artifact-v2") || !strings.Contains(heads, "test-code") {
		t.Fatalf("evidence heads = %s", heads)
	}

	conflict := validPolicy("cutoff-concurrent")
	writeJSON(t, filepath.Join(epicQuality(root), "policies", "cutoff-concurrent.json"), conflict)
	document, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.PolicySets) != 1 || !document.PolicySets[0].Conflict() || document.PolicySets[0].Current != nil {
		t.Fatalf("policy conflict = %#v", document.PolicySets)
	}
}

func TestValidationRejectsRemovedStepReuseAndInvalidGraphs(t *testing.T) {
	t.Run("removed step reuse", func(t *testing.T) {
		root := newQualitySaga(t, true)
		writeValidTestCase(t, root, "deadline")
		packageDir := testPackage(root, "deadline")
		r2 := revisionFrom("r2", []string{revisionURN("r1")}, "Without submit")
		r2.Steps = []Step{{ID: "inspect", Action: "Inspect reason", ExpectedResult: "Reason is present"}}
		r3 := revisionFrom("r3", []string{revisionURN("r2")}, "Reuse")
		writeJSON(t, filepath.Join(packageDir, "revisions", "r2.json"), r2)
		writeJSON(t, filepath.Join(packageDir, "revisions", "r3.json"), r3)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "reuses removed step") {
			t.Fatalf("reuse error = %v", err)
		}
	})

	t.Run("missing run parent", func(t *testing.T) {
		root := newQualitySaga(t, true)
		writeValidTestCase(t, root, "deadline")
		run := runFrom("ci-2", []string{runURN("missing")}, RunPassed)
		writeJSON(t, filepath.Join(testPackage(root, "deadline"), "runs", "ci-2.json"), run)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "missing parent") {
			t.Fatalf("missing-parent error = %v", err)
		}
	})

	t.Run("policy cycle", func(t *testing.T) {
		root := newQualitySaga(t, true)
		first := validPolicy("first")
		second := validPolicy("second")
		first.Supersedes = []string{"urn:change-saga:checkout:quality-policy:second"}
		second.Supersedes = []string{"urn:change-saga:checkout:quality-policy:first"}
		writeJSON(t, filepath.Join(epicQuality(root), "policies", "first.json"), first)
		writeJSON(t, filepath.Join(epicQuality(root), "policies", "second.json"), second)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "must be acyclic") {
			t.Fatalf("cycle error = %v", err)
		}
	})
}

func TestValidationCoversRevisionLifecycleEvidenceRunAndPolicyRecords(t *testing.T) {
	tests := []struct {
		name string
		edit func(string)
		want string
	}{
		{
			name: "duplicate step",
			edit: func(root string) {
				revision := revisionFrom("r1", nil, "Duplicate")
				revision.Steps = append(revision.Steps, revision.Steps[0])
				writeJSON(t, filepath.Join(testPackage(root, "deadline"), "revisions", "r1.json"), revision)
			},
			want: "step id \"submit\" is duplicated",
		},
		{
			name: "wrong lifecycle root",
			edit: func(root string) {
				event := eventFrom("proposed", nil, StateActive)
				writeJSON(t, filepath.Join(testPackage(root, "deadline"), "events", "proposed.json"), event)
			},
			want: "initial lifecycle state must be proposed",
		},
		{
			name: "noncanonical diff",
			edit: func(root string) {
				evidence := validEvidence("test-code", EvidenceTestImplementation)
				evidence.Diffs = []string{"saga-diff://v1/line?repository=https://example.com/repo.git&base=base&head=head&path=test.go&side=new&start=2&end=1"}
				writeJSON(t, filepath.Join(testPackage(root, "deadline"), "evidence", "test-code.json"), evidence)
			},
			want: "canonical exact line or event selector",
		},
		{
			name: "execution artifact with diff",
			edit: func(root string) {
				evidence := validEvidence("test-code", EvidenceExecutionArtifact)
				evidence.Verifications = []string{"urn:change-saga:checkout:verification:ci-log"}
				writeJSON(t, filepath.Join(testPackage(root, "deadline"), "evidence", "test-code.json"), evidence)
			},
			want: "cannot contain diff selectors",
		},
		{
			name: "run missing evidence",
			edit: func(root string) {
				run := runFrom("ci-1", nil, RunPassed)
				run.Evidence = []string{"urn:change-saga:checkout:test-case:deadline:evidence:missing"}
				writeJSON(t, filepath.Join(testPackage(root, "deadline"), "runs", "ci-1.json"), run)
			},
			want: "references missing evidence",
		},
		{
			name: "policy revision mismatch",
			edit: func(root string) {
				policy := validPolicy("cutoff")
				policy.StoryRevision = "urn:change-saga:checkout:story:other:revision:r1"
				writeJSON(t, filepath.Join(epicQuality(root), "policies", "cutoff.json"), policy)
			},
			want: "must pin the criterion's story",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := newQualitySaga(t, true)
			writeValidTestCase(t, root, "deadline")
			test.edit(root)
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v; want substring %q", err, test.want)
			}
		})
	}
}

func TestStrictReadOnlyLoaderRejectsUnknownFieldsAndSymlinks(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		root := newQualitySaga(t, true)
		writeValidTestCase(t, root, "deadline")
		path := filepath.Join(testPackage(root, "deadline"), "test-case.json")
		if err := os.WriteFile(path, []byte(`{"$schema":"https://changesaga.dev/schema/v5/test-case.schema.json","version":5,"id":"deadline","created_at":"2026-09-17T20:00:00Z","surprise":true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		before := snapshotPaths(t, root)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("unknown-field error = %v", err)
		}
		after := snapshotPaths(t, root)
		if strings.Join(before, "\n") != strings.Join(after, "\n") {
			t.Fatalf("read-only load changed paths: before %v after %v", before, after)
		}
	})

	t.Run("missing required array", func(t *testing.T) {
		root := newQualitySaga(t, true)
		writeValidTestCase(t, root, "deadline")
		path := filepath.Join(testPackage(root, "deadline"), "revisions", "r1.json")
		var record map[string]any
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		delete(record, "preconditions")
		writeJSON(t, path, record)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "required field \"preconditions\" is missing") {
			t.Fatalf("required-field error = %v", err)
		}
	})

	t.Run("blank optional command", func(t *testing.T) {
		root := newQualitySaga(t, true)
		writeValidTestCase(t, root, "deadline")
		path := filepath.Join(testPackage(root, "deadline"), "runs", "ci-1.json")
		var record map[string]any
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatal(err)
		}
		record["command"] = ""
		writeJSON(t, path, record)
		if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "command must be a nonblank") {
			t.Fatalf("command error = %v", err)
		}
	})

	if runtime.GOOS != "windows" {
		t.Run("symlinked record", func(t *testing.T) {
			root := newQualitySaga(t, true)
			writeValidTestCase(t, root, "deadline")
			policyPath := filepath.Join(epicQuality(root), "policies", "linked.json")
			if err := os.Symlink(filepath.Join(testPackage(root, "deadline"), "test-case.json"), policyPath); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "real JSON file") {
				t.Fatalf("symlink error = %v", err)
			}
		})
	}
}

func newQualitySaga(t *testing.T, qualityRoot bool) string {
	t.Helper()
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "saga.json"), map[string]any{
		"$schema": ManifestSchemaURL,
		"version": Version,
		"id":      "checkout",
		"title":   "Checkout",
		"source":  SourceIdentity{Repository: "https://example.com/repo.git", Base: "base", Head: "head"},
	})
	if _, err := applayout.WriteEpic(root, applayout.EpicManifest{ID: "core", Title: "Core", CreatedAt: fixtureTime}); err != nil {
		t.Fatal(err)
	}
	if qualityRoot {
		for _, dir := range []string{"policies", "test-cases"} {
			if err := os.MkdirAll(filepath.Join(epicQuality(root), dir), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func writeValidTestCase(t *testing.T, root, id string) {
	t.Helper()
	packageDir := testPackage(root, id)
	for _, dir := range []string{"revisions", "events", "evidence", "runs"} {
		if err := os.MkdirAll(filepath.Join(packageDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON(t, filepath.Join(packageDir, "test-case.json"), TestCaseIdentity{
		Schema: TestCaseSchemaURL, Version: Version, ID: id, CreatedAt: fixtureTime,
	})
	writeJSON(t, filepath.Join(packageDir, "revisions", "r1.json"), revisionFrom("r1", nil, "Reject at deadline"))
	writeJSON(t, filepath.Join(packageDir, "events", "proposed.json"), eventFrom("proposed", nil, StateProposed))
	writeJSON(t, filepath.Join(packageDir, "events", "active.json"), eventFrom("active", []string{eventURN("proposed")}, StateActive))
	writeJSON(t, filepath.Join(packageDir, "evidence", "test-code.json"), validEvidence("test-code", EvidenceTestImplementation))
	writeJSON(t, filepath.Join(packageDir, "runs", "ci-1.json"), runFrom("ci-1", nil, RunPassed))
}

func revisionFrom(id string, parents []string, title string) Revision {
	return Revision{
		Schema: RevisionSchemaURL, Version: Version, ID: id,
		TestCase: "urn:change-saga:checkout:test-case:deadline", Parents: nonnil(parents), Title: title,
		CoverageKinds: []CoverageKind{CoverageNegative, CoverageEdge}, Automation: AutomationAutomated,
		Preconditions: []string{"An eligible purchase is exactly 30 days old."},
		Steps: []Step{
			{ID: "submit", Action: "Submit a refund request.", ExpectedResult: "The request is rejected."},
			{ID: "inspect", Action: "Inspect the response reason.", ExpectedResult: "The cutoff reason is returned."},
		},
		ExpectedResult: "No refund is created.", CreatedAt: fixtureTime.Add(time.Duration(len(id)) * time.Minute),
	}
}

func eventFrom(id string, parents []string, state LifecycleState) LifecycleEvent {
	return LifecycleEvent{
		Schema: LifecycleEventSchemaURL, Version: Version, ID: id,
		TestCase: "urn:change-saga:checkout:test-case:deadline", Parents: nonnil(parents), State: state,
		CreatedAt: fixtureTime.Add(time.Duration(len(id)) * time.Minute),
	}
}

func validEvidence(id string, role EvidenceRole) Evidence {
	selector, err := diffuri.Build(diffuri.Reference{
		Repository: "https://example.com/repo.git", Base: "base", Head: "head", Kind: "line",
		Path: "internal/refund_test.go", Side: "new", Start: 41, End: 88,
	})
	if err != nil {
		panic(err)
	}
	return Evidence{
		Schema: EvidenceSchemaURL, Version: Version, ID: id,
		TestCase: "urn:change-saga:checkout:test-case:deadline", TestRevision: revisionURN("r1"),
		Role: role, Diffs: []string{selector}, Verifications: []string{}, Citations: []string{}, Supersedes: []string{}, CreatedAt: fixtureTime,
	}
}

func runFrom(id string, parents []string, result RunResult) Run {
	return Run{
		Schema: RunSchemaURL, Version: Version, ID: id,
		TestCase: "urn:change-saga:checkout:test-case:deadline", TestRevision: revisionURN("r1"), Parents: nonnil(parents),
		Source: SourceIdentity{Repository: "https://example.com/repo.git", Base: "base", Head: "head"},
		Result: result, Summary: "CI completed.", Command: "go test ./...", Evidence: []string{"urn:change-saga:checkout:test-case:deadline:evidence:test-code"},
		ExecutedAt: fixtureTime.Add(time.Hour),
	}
}

func validPolicy(id string) Policy {
	return Policy{
		Schema: PolicySchemaURL, Version: Version, ID: id,
		Criterion:         "urn:change-saga:checkout:story:refund:criterion:cutoff",
		StoryRevision:     "urn:change-saga:checkout:story:refund:revision:r1",
		RequiredKinds:     []CoverageKind{CoveragePositive, CoverageNegative, CoverageEdge},
		AllowedAutomation: []Automation{AutomationAutomated, AutomationManual}, Supersedes: []string{},
		Rationale: "The boundary has distinct risks.", CreatedAt: fixtureTime,
	}
}

// epicQuality is the quality root of the fixture's "core" epic.
func epicQuality(root string) string {
	return filepath.Join(applayout.EpicDir(root, "core"), RootDir)
}

func testPackage(root, id string) string {
	return filepath.Join(epicQuality(root), "test-cases", id+".test")
}

func revisionURN(id string) string {
	return "urn:change-saga:checkout:test-case:deadline:revision:" + id
}

func eventURN(id string) string {
	return "urn:change-saga:checkout:test-case:deadline:event:" + id
}

func runURN(id string) string {
	return "urn:change-saga:checkout:test-case:deadline:run:" + id
}

func nonnil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotPaths(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
