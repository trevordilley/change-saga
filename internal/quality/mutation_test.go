package quality

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diffuri"
)

const deadlineURN = "urn:change-saga:checkout:test-case:deadline"

func deadlineDefinition() Definition {
	return Definition{
		Title: "Reject at deadline", CoverageKinds: []CoverageKind{CoverageEdge, CoverageNegative},
		Automation: AutomationAutomated, Preconditions: []string{"A purchase is exactly 30 days old."},
		Steps: []Step{
			{ID: "submit", Action: "Submit a refund request.", ExpectedResult: "The request is rejected."},
			{ID: "inspect", Action: "Inspect the reason.", ExpectedResult: "The cutoff reason is returned."},
		},
		ExpectedResult: "No refund is created.",
	}
}

func addDeadline(t *testing.T, root string) MutationResult {
	t.Helper()
	result, err := AddTestCase(root, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), CreatedAt: fixtureTime, RequestID: "add-deadline"})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func loadDeadline(t *testing.T, root string) TestCase {
	t.Helper()
	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range document.TestCases {
		if testCase.Identity.ID == "deadline" {
			return testCase
		}
	}
	t.Fatal("deadline test case missing")
	return TestCase{}
}

func currentSelector(t *testing.T, path string) string {
	t.Helper()
	selector, err := diffuri.Build(diffuri.Reference{
		Repository: "https://example.com/repo.git", Base: "base", Head: "head", Kind: "line",
		Path: path, Side: "new", Start: 1, End: 9,
	})
	if err != nil {
		t.Fatal(err)
	}
	return selector
}

func TestAddTestCaseCreatesLoadablePackageAndReplays(t *testing.T) {
	root := newQualitySaga(t, false)
	result := addDeadline(t, root)
	wantPaths := []string{
		"___quality/test-cases/deadline.test",
		"___quality/test-cases/deadline.test/revisions/r1.json",
		"___quality/test-cases/deadline.test/events/proposed.json",
	}
	if result.URN != deadlineURN || !reflect.DeepEqual(result.Paths, wantPaths) || result.Replayed {
		t.Fatalf("result = %#v", result)
	}
	testCase := loadDeadline(t, root)
	if testCase.CurrentRevision == nil || testCase.CurrentLifecycle == nil || testCase.CurrentLifecycle.State != StateProposed {
		t.Fatalf("projection = %#v", testCase)
	}
	if got := testCase.CurrentRevision.CoverageKinds; !reflect.DeepEqual(got, []CoverageKind{CoverageNegative, CoverageEdge}) {
		t.Fatalf("kinds are not canonical: %v", got)
	}
	before := snapshotPaths(t, root)
	replayed, err := AddTestCase(root, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), RequestID: "add-deadline"})
	if err != nil || !replayed.Replayed {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	changed := deadlineDefinition()
	changed.Title = "Different"
	if _, err := AddTestCase(root, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: changed, RequestID: "add-deadline"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("conflicting replay error = %v", err)
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("replay changed files:\n%v\n%v", before, after)
	}
}

func TestInvalidFirstWriteLeavesQualityUnadopted(t *testing.T) {
	root := newQualitySaga(t, false)
	before := snapshotPaths(t, root)
	definition := deadlineDefinition()
	definition.Steps = append(definition.Steps, Step{ID: "submit", Action: "Again.", ExpectedResult: "Again."})
	if _, err := AddTestCase(root, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: definition}); err == nil {
		t.Fatal("duplicate step id was accepted")
	}
	if _, err := SetPolicy(root, SetPolicyInput{Criterion: "not-a-urn", StoryRevision: "x", RequiredKinds: []CoverageKind{CoveragePositive}, AllowedAutomation: []Automation{AutomationManual}, Rationale: "r"}); err == nil {
		t.Fatal("invalid policy was accepted")
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("failed mutations changed files:\n%v\n%v", before, after)
	}
	document, err := Load(root)
	if err != nil || document.Adoption != NotAdopted {
		t.Fatalf("adoption = %v, %v", document.Adoption, err)
	}
}

func TestReviseRequiresEveryHeadAndReconcilesConcurrentEdits(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	r1 := deadlineURN + ":revision:r1"
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{}, Definition: deadlineDefinition()}); err == nil {
		t.Fatal("revision without parents was accepted")
	}
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{r1}, Definition: deadlineDefinition()}); err != nil {
		t.Fatal(err)
	}
	// A concurrent branch also revised r1; Git merges both files.
	concurrent := revisionFrom("r2b", []string{r1}, "Concurrent edit")
	writeJSON(t, filepath.Join(testPackage(root, "deadline"), "revisions", "r2b.json"), concurrent)
	testCase := loadDeadline(t, root)
	if !testCase.RevisionConflict() || testCase.CurrentRevision != nil {
		t.Fatalf("concurrent heads were not a conflict: %v", testCase.RevisionHeads)
	}
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r3", Parents: []string{deadlineURN + ":revision:r2"}, Definition: deadlineDefinition()}); err == nil || !strings.Contains(err.Error(), "every current head") {
		t.Fatalf("stale single parent error = %v", err)
	}
	result, err := ReviseTestCase(root, ReviseTestCaseInput{
		TestCase: deadlineURN, RevisionID: "r3", Parents: testCase.RevisionHeads, Definition: deadlineDefinition(), RequestID: "reconcile",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.CurrentHeads, []string{deadlineURN + ":revision:r3"}) {
		t.Fatalf("heads = %v", result.CurrentHeads)
	}
	if testCase = loadDeadline(t, root); testCase.RevisionConflict() || testCase.CurrentRevision.ID != "r3" {
		t.Fatalf("reconciliation projection = %v", testCase.RevisionHeads)
	}
	replay, err := ReviseTestCase(root, ReviseTestCaseInput{
		TestCase: deadlineURN, RevisionID: "r3", Parents: []string{deadlineURN + ":revision:r2", deadlineURN + ":revision:r2b"}, Definition: deadlineDefinition(), RequestID: "reconcile",
	})
	if err != nil || !replay.Replayed {
		t.Fatalf("revision replay = %#v, %v", replay, err)
	}
}

func TestReviseRejectsRemovedStepReuse(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	withoutInspect := deadlineDefinition()
	withoutInspect.Steps = withoutInspect.Steps[:1]
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{deadlineURN + ":revision:r1"}, Definition: withoutInspect}); err != nil {
		t.Fatal(err)
	}
	reused := deadlineDefinition()
	reused.Steps[1].Action = "A different action."
	before := snapshotPaths(t, root)
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r3", Parents: []string{deadlineURN + ":revision:r2"}, Definition: reused}); err == nil || !strings.Contains(err.Error(), "reuses removed step") {
		t.Fatalf("step reuse error = %v", err)
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected revision changed files")
	}
}

func TestLifecycleTransitionsAndActivationRules(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	proposed := deadlineURN + ":event:proposed"
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "active", Parents: []string{proposed}, State: StateActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "back", Parents: []string{deadlineURN + ":event:active"}, State: StateProposed}); err == nil || !strings.Contains(err.Error(), "cannot transition") {
		t.Fatalf("active -> proposed error = %v", err)
	}
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "retired", Parents: []string{deadlineURN + ":event:active"}, State: StateRetired, Reason: "Behavior removed."}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "again", Parents: []string{deadlineURN + ":event:retired"}, State: StateActive}); err == nil {
		t.Fatal("retired is not terminal")
	}

	stepless := newQualitySaga(t, false)
	definition := deadlineDefinition()
	definition.Steps = []Step{}
	if _, err := AddTestCase(stepless, AddTestCaseInput{ID: "deadline", RevisionID: "r1", Definition: definition}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTestCaseState(stepless, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "active", Parents: []string{proposed}, State: StateActive}); err == nil || !strings.Contains(err.Error(), "step") {
		t.Fatalf("stepless activation error = %v", err)
	}
}

func TestEvidenceRequiresCurrentPinsAndSupportsManualArtifacts(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	selector := currentSelector(t, "internal/refund_test.go")
	result, err := AddEvidence(root, AddEvidenceInput{TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{selector}})
	if err != nil {
		t.Fatal(err)
	}
	if result.URN != deadlineURN+":evidence:test-implementation" {
		t.Fatalf("generated evidence urn = %s", result.URN)
	}
	stale, _ := diffuri.Build(diffuri.Reference{Repository: "https://example.com/repo.git", Base: "base", Head: "older", Kind: "line", Path: "a.go", Side: "new", Start: 1, End: 2})
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "stale", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{stale}}); err == nil || !strings.Contains(err.Error(), "current source comparison") {
		t.Fatalf("stale source error = %v", err)
	}
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "artifact", TestCase: deadlineURN, Role: EvidenceExecutionArtifact, Diffs: []string{selector}}); err == nil {
		t.Fatal("execution artifact accepted a diff")
	}
	if _, err := AddEvidence(root, AddEvidenceInput{
		ID: "manual", TestCase: deadlineURN, Role: EvidenceExecutionArtifact,
		Citations: []string{"urn:change-saga:checkout:citation:qa-session"},
	}); err != nil {
		t.Fatal(err)
	}
	replacement, err := AddEvidence(root, AddEvidenceInput{
		ID: "test-code-2", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "internal/refund2_test.go")},
		Supersedes: []string{result.URN},
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(replacement.CurrentHeads, result.URN) || !contains(replacement.CurrentHeads, replacement.URN) {
		t.Fatalf("supersession heads = %v", replacement.CurrentHeads)
	}
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "again", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{selector}, Supersedes: []string{result.URN}}); err == nil {
		t.Fatal("superseded evidence was superseded twice")
	}
}

func TestEvidenceBatchIsAllOrNothing(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	before := snapshotPaths(t, root)
	_, err := AddEvidenceBatch(root, []AddEvidenceInput{
		{ID: "one", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "a_test.go")}},
		{ID: "two", TestCase: deadlineURN, Role: EvidenceImplementationUnderTest},
	})
	if err == nil || !strings.Contains(err.Error(), "evidence 2") {
		t.Fatalf("batch error = %v", err)
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("failed preflight wrote files:\n%v\n%v", before, after)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("write-failure rollback requires enforced directory permissions")
	}
	if _, err := AddTestCase(root, AddTestCaseInput{ID: "other", RevisionID: "r1", Definition: deadlineDefinition()}); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(testPackage(root, "other"), "evidence")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	before = snapshotPaths(t, root)
	_, err = AddEvidenceBatch(root, []AddEvidenceInput{
		{ID: "one", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "a_test.go")}},
		{ID: "one", TestCase: "urn:change-saga:checkout:test-case:other", Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "a_test.go")}},
	})
	if err == nil {
		t.Fatal("write into a read-only directory succeeded")
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("failed batch write left files:\n%v\n%v", before, after)
	}
}

func TestRunsAreImmutableEventsAndFailuresStayVisible(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	evidence, err := AddEvidence(root, AddEvidenceInput{ID: "test-code", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "a_test.go")}})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Result: RunFailed, Summary: "Rejected with the wrong reason.", Command: "go test ./...", Evidence: []string{evidence.URN}})
	if err != nil {
		t.Fatal(err)
	}
	testCase := loadDeadline(t, root)
	if testCase.HeadRun == nil || testCase.HeadRun.Result != RunFailed || !testCase.HeadRun.Current {
		t.Fatalf("failed head run = %#v", testCase.HeadRun)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-2", TestCase: deadlineURN, Result: RunPassed, Summary: "Passed.", Evidence: []string{evidence.URN}}); err == nil || !strings.Contains(err.Error(), "every current run head") {
		t.Fatalf("run that ignored the failed head error = %v", err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Parents: []string{failed.URN}, Result: RunPassed, Summary: "Overwrite.", Evidence: []string{evidence.URN}}); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("run overwrite error = %v", err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-2", TestCase: deadlineURN, Parents: []string{failed.URN}, Result: RunPassed, Summary: "Passed.", Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	// A concurrent manual run on another branch also followed ci-1; Git merges both.
	concurrent := runFrom("manual-1", nil, RunFailed)
	concurrent.Evidence = []string{evidence.URN}
	concurrent.Parents = []string{failed.URN}
	writeJSON(t, filepath.Join(testPackage(root, "deadline"), "runs", "manual-1.json"), concurrent)
	if testCase = loadDeadline(t, root); !testCase.RunConflict() || testCase.CurrentRun != nil {
		t.Fatalf("concurrent runs were not a conflict: %v", testCase.RunHeads)
	}
	conflictHeads := testCase.RunHeads
	passed, err := RecordRun(root, RecordRunInput{TestCase: deadlineURN, Parents: conflictHeads, Result: RunPassed, Summary: "Reconciled pass.", Evidence: []string{evidence.URN}, RequestID: "ci-3"})
	if err != nil {
		t.Fatal(err)
	}
	testCase = loadDeadline(t, root)
	if testCase.CurrentRun == nil || testCase.CurrentRun.Result != RunPassed || len(testCase.Runs) != 4 {
		t.Fatalf("reconciled run projection = %#v (%d runs)", testCase.CurrentRun, len(testCase.Runs))
	}
	if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(failed.Path))); err != nil || !strings.Contains(string(data), `"failed"`) {
		t.Fatalf("failed run was not preserved: %v", err)
	}
	replay, err := RecordRun(root, RecordRunInput{TestCase: deadlineURN, Parents: conflictHeads, Result: RunPassed, Summary: "Reconciled pass.", Evidence: []string{evidence.URN}, RequestID: "ci-3"})
	if err != nil || !replay.Replayed || replay.URN != passed.URN {
		t.Fatalf("generated-id replay = %#v, %v; want replay of %s", replay, err, passed.URN)
	}
	if runs := loadDeadline(t, root).Runs; len(runs) != 4 {
		t.Fatalf("replay wrote a run: %d runs", len(runs))
	}
}

func TestRevisingATestMakesItsPassingRunStale(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	evidence, err := AddEvidence(root, AddEvidenceInput{ID: "test-code", TestCase: deadlineURN, Role: EvidenceTestImplementation, Diffs: []string{currentSelector(t, "a_test.go")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Result: RunPassed, Summary: "Passed.", Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	if loadDeadline(t, root).CurrentRun == nil {
		t.Fatal("fresh pass is not current")
	}
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{deadlineURN + ":revision:r1"}, Definition: deadlineDefinition()}); err != nil {
		t.Fatal(err)
	}
	testCase := loadDeadline(t, root)
	if testCase.CurrentRun != nil || testCase.HeadRun == nil || !contains(testCase.HeadRun.StaleReasons, "test revision changed") {
		t.Fatalf("pass after revision = current %#v head %#v", testCase.CurrentRun, testCase.HeadRun)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "old-source", TestCase: deadlineURN, Parents: testCase.RunHeads, Result: RunPassed, Summary: "Ran elsewhere.",
		Source: &SourceIdentity{Repository: "https://example.com/repo.git", Base: "base", Head: "other"}, Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	if testCase = loadDeadline(t, root); testCase.CurrentRun != nil || !contains(testCase.HeadRun.StaleReasons, "source comparison changed") {
		t.Fatalf("other-source run projection = %#v", testCase.HeadRun)
	}
}

func TestPolicySetKeepsOneHeadPerCriterionRevision(t *testing.T) {
	root := newQualitySaga(t, false)
	criterion := "urn:change-saga:checkout:story:refund:criterion:cutoff"
	storyRevision := "urn:change-saga:checkout:story:refund:revision:r1"
	first, err := SetPolicy(root, SetPolicyInput{
		Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoverageEdge, CoveragePositive},
		AllowedAutomation: []Automation{AutomationManual, AutomationAutomated}, Rationale: "Boundary risk.", RequestID: "policy-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.URN != "urn:change-saga:checkout:quality-policy:refund.cutoff.r1" || first.Path != "___quality/policies/refund.cutoff.r1.json" {
		t.Fatalf("default policy = %#v", first)
	}
	replay, err := SetPolicy(root, SetPolicyInput{
		Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive, CoverageEdge},
		AllowedAutomation: []Automation{AutomationAutomated, AutomationManual}, Rationale: "Boundary risk.", RequestID: "policy-1",
	})
	if err != nil || !replay.Replayed || replay.URN != first.URN {
		t.Fatalf("policy replay = %#v, %v", replay, err)
	}
	if _, err := SetPolicy(root, SetPolicyInput{Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive}, AllowedAutomation: []Automation{AutomationManual}, Rationale: "Narrower."}); err == nil || !strings.Contains(err.Error(), "supersede every current head") {
		t.Fatalf("competing policy error = %v", err)
	}
	second, err := SetPolicy(root, SetPolicyInput{Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive, CoverageNegative, CoverageEdge}, AllowedAutomation: []Automation{AutomationAutomated}, Supersedes: []string{first.URN}, Rationale: "All kinds."})
	if err != nil {
		t.Fatal(err)
	}
	if second.URN != "urn:change-saga:checkout:quality-policy:refund.cutoff.r1-2" {
		t.Fatalf("second default id = %s", second.URN)
	}
	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.PolicySets) != 1 || document.PolicySets[0].Current == nil || document.PolicySets[0].Current.ID != "refund.cutoff.r1-2" {
		t.Fatalf("policy sets = %#v", document.PolicySets)
	}
	if kinds := document.PolicySets[0].Current.RequiredKinds; !reflect.DeepEqual(kinds, []CoverageKind{CoveragePositive, CoverageNegative, CoverageEdge}) {
		t.Fatalf("kinds = %v", kinds)
	}
}
