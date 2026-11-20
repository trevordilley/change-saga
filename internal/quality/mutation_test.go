package quality

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/qualityid"
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
	result, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), CreatedAt: fixtureTime, RequestID: "add-deadline"})
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

func currentSelector(t *testing.T, path string) coderef.Reference {
	t.Helper()
	return coderef.Reference{Commit: strings.Repeat("a", 40), Path: path, Start: 1, End: 9, Digest: coderef.DigestBytes([]byte(path))}
}

func TestAddTestCaseCreatesLoadablePackageAndReplays(t *testing.T) {
	root := newQualitySaga(t, false)
	result := addDeadline(t, root)
	wantPaths := []string{
		"___features/core.feature/___quality/test-cases/deadline.test",
		"___features/core.feature/___quality/test-cases/deadline.test/current.json",
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
	replayed, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: deadlineDefinition(), RequestID: "add-deadline"})
	if err != nil || !replayed.Replayed {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	changed := deadlineDefinition()
	changed.Title = "Different"
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: changed, RequestID: "add-deadline"}); err == nil || !strings.Contains(err.Error(), "already exists") {
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
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: definition}); err == nil {
		t.Fatal("duplicate step id was accepted")
	}
	if _, err := SetPolicy(root, SetPolicyInput{Feature: "core", Criterion: "not-a-urn", StoryRevision: "x", RequiredKinds: []CoverageKind{CoveragePositive}, AllowedAutomation: []Automation{AutomationManual}, Rationale: "r"}); err == nil {
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

func TestCurrentEditsRefuseAmbiguousLegacyDefinitionsBeforeCleanup(t *testing.T) {
	root := newQualitySaga(t, true)
	writeValidTestCase(t, root, "deadline")
	dir := testPackage(root, "deadline")
	left := revisionFrom("left", []string{revisionURN("r1")}, "First branch")
	right := revisionFrom("right", []string{revisionURN("r1")}, "Independent branch")
	writeJSON(t, filepath.Join(dir, "revisions", "left.json"), left)
	writeJSON(t, filepath.Join(dir, "revisions", "right.json"), right)
	before := snapshotPaths(t, root)
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: deadlineDefinition()}); err == nil || !strings.Contains(err.Error(), "competing") {
		t.Fatalf("ambiguous legacy upgrade: %v", err)
	}
	if after := snapshotPaths(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("ambiguous definitions were changed or deleted")
	}
	// An explicit author combines the independent intent at one atomic current
	// publication. This is legacy resolution, not a new Saga revision graph.
	merged := revisionFrom("combined", nil, "Combined branch intent")
	merged.Preconditions = []string{"First branch intent", "Independent branch intent"}
	if err := currentrecord.Write(dir, merged, eventFrom("active", nil, StateActive)); err != nil {
		t.Fatal(err)
	}
	if err := currentrecord.Normalize(root); err != nil {
		t.Fatal(err)
	}
	definition := Definition{Title: merged.Title, CoverageKinds: merged.CoverageKinds, Automation: merged.Automation, Preconditions: merged.Preconditions, Steps: merged.Steps, ExpectedResult: merged.ExpectedResult}
	definition.ExpectedResult = "The combined outcome"
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: definition}); err != nil {
		t.Fatal(err)
	}
	loaded := loadDeadline(t, root)
	if loaded.CurrentRevision.ExpectedResult != definition.ExpectedResult || !reflect.DeepEqual(loaded.CurrentRevision.Preconditions, merged.Preconditions) {
		t.Fatal("explicitly combined authored intent was lost")
	}
	if _, err := os.Stat(filepath.Join(dir, "revisions")); !os.IsNotExist(err) {
		t.Fatal("obsolete definitions were retained after explicit current publication")
	}
	if len(loaded.Evidence) == 0 || len(loaded.Runs) == 0 {
		t.Fatal("actual evidence or execution facts were deleted")
	}
}

func TestCurrentStepEditCannotInheritHistoricalEvidence(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "inspect-code", TestCase: deadlineURN, Role: EvidenceTestImplementation, Steps: []string{"inspect"}, Code: []coderef.Reference{currentSelector(t, "inspect_test.go")}}); err != nil {
		t.Fatal(err)
	}
	originalPin := loadDeadline(t, root).Evidence[0].TestRevision
	withoutInspect := deadlineDefinition()
	withoutInspect.Steps = withoutInspect.Steps[:1]
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: withoutInspect}); err != nil {
		t.Fatal(err)
	}
	reintroduced := deadlineDefinition()
	reintroduced.Steps[1].Action = "Inspect a different observable outcome"
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: reintroduced}); err != nil {
		t.Fatal(err)
	}
	loaded := loadDeadline(t, root)
	if len(loaded.Evidence) != 1 || loaded.Evidence[0].TestRevision != originalPin || loaded.Evidence[0].Current {
		t.Fatalf("historical evidence silently attached to changed step: %+v", loaded.Evidence)
	}
	if loaded.CurrentRevision.Steps[1].ID != "inspect" {
		t.Fatal("authored step identity changed")
	}
}

func TestLifecycleTransitionsAndActivationRules(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	proposed := deadlineURN + ":event:proposed"
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "active", Parents: []string{proposed}, State: StateActive}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTestCaseState(root, SetTestCaseStateInput{TestCase: deadlineURN, EventID: "back", Parents: []string{deadlineURN + ":event:active"}, State: StateProposed}); err == nil || !strings.Contains(err.Error(), "cannot change test state") {
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
	if _, err := AddTestCase(stepless, AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: definition}); err != nil {
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
	result, err := AddEvidence(root, AddEvidenceInput{TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{selector}})
	if err != nil {
		t.Fatal(err)
	}
	if result.URN != deadlineURN+":evidence:test-implementation" {
		t.Fatalf("generated evidence urn = %s", result.URN)
	}
	noted := selector
	noted.Note = "quality evidence explains itself"
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "noted", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{noted}}); err == nil || !strings.Contains(err.Error(), "cannot carry a note") {
		t.Fatalf("noted reference error = %v", err)
	}
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "artifact", TestCase: deadlineURN, Role: EvidenceExecutionArtifact, Code: []coderef.Reference{selector}}); err == nil {
		t.Fatal("execution artifact accepted code")
	}
	if _, err := AddEvidence(root, AddEvidenceInput{
		ID: "manual", TestCase: deadlineURN, Role: EvidenceExecutionArtifact,
		Citations: []string{"urn:change-saga:checkout:citation:qa-session"},
	}); err != nil {
		t.Fatal(err)
	}
	replacement, err := AddEvidence(root, AddEvidenceInput{
		ID: "test-code-2", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "internal/refund2_test.go")},
		Supersedes: []string{result.URN},
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(replacement.CurrentHeads, result.URN) || !contains(replacement.CurrentHeads, replacement.URN) {
		t.Fatalf("supersession heads = %v", replacement.CurrentHeads)
	}
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "again", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{selector}, Supersedes: []string{result.URN}}); err == nil {
		t.Fatal("superseded evidence was superseded twice")
	}
}

func TestEvidenceBatchIsAllOrNothing(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	before := snapshotPaths(t, root)
	_, err := AddEvidenceBatch(root, []AddEvidenceInput{
		{ID: "one", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "a_test.go")}},
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
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "other", RevisionID: "r1", Definition: deadlineDefinition()}); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(testPackage(root, "other"), "evidence")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	before = snapshotPaths(t, root)
	_, err = AddEvidenceBatch(root, []AddEvidenceInput{
		{ID: "one", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "a_test.go")}},
		{ID: "one", TestCase: "urn:change-saga:checkout:test-case:other", Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "a_test.go")}},
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
	evidence, err := AddEvidence(root, AddEvidenceInput{ID: "test-code", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "a_test.go")}})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Result: RunFailed, Summary: "Rejected with the wrong reason.", Command: "go test ./...", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}})
	if err != nil {
		t.Fatal(err)
	}
	testCase := loadDeadline(t, root)
	if testCase.HeadRun == nil || testCase.HeadRun.Result != RunFailed || !testCase.HeadRun.Current {
		t.Fatalf("failed head run = %#v", testCase.HeadRun)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-2", TestCase: deadlineURN, Result: RunPassed, Summary: "Passed.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}}); err == nil || !strings.Contains(err.Error(), "every current run head") {
		t.Fatalf("run that ignored the failed head error = %v", err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Parents: []string{failed.URN}, Result: RunPassed, Summary: "Overwrite.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}}); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("run overwrite error = %v", err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-2", TestCase: deadlineURN, Parents: []string{failed.URN}, Result: RunPassed, Summary: "Passed.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	// A concurrent manual run on another branch also followed ci-1; Git merges both.
	concurrent := runFrom("manual-1", nil, RunFailed)
	concurrent.Evidence = []string{evidence.URN}
	concurrent.TestRevision, _ = qualityid.Revision("checkout", "deadline", loadDeadline(t, root).CurrentRevision.ID)
	concurrent.Parents = []string{failed.URN}
	writeJSON(t, filepath.Join(testPackage(root, "deadline"), "runs", "manual-1.json"), concurrent)
	if testCase = loadDeadline(t, root); !testCase.RunConflict() || testCase.CurrentRun != nil {
		t.Fatalf("concurrent runs were not a conflict: %v", testCase.RunHeads)
	}
	conflictHeads := testCase.RunHeads
	passed, err := RecordRun(root, RecordRunInput{TestCase: deadlineURN, Parents: conflictHeads, Result: RunPassed, Summary: "Reconciled pass.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}, RequestID: "ci-3"})
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
	replay, err := RecordRun(root, RecordRunInput{TestCase: deadlineURN, Parents: conflictHeads, Result: RunPassed, Summary: "Reconciled pass.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}, RequestID: "ci-3"})
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
	evidence, err := AddEvidence(root, AddEvidenceInput{ID: "test-code", TestCase: deadlineURN, Role: EvidenceTestImplementation, Code: []coderef.Reference{currentSelector(t, "a_test.go")}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "ci-1", TestCase: deadlineURN, Result: RunPassed, Summary: "Passed.", Source: &SourceIdentity{Commit: testRunCommit}, Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	if loadDeadline(t, root).CurrentRun == nil {
		t.Fatal("fresh pass is not current")
	}
	changed := deadlineDefinition()
	changed.ExpectedResult = "A newly agreed observable outcome"
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: changed}); err != nil {
		t.Fatal(err)
	}
	testCase := loadDeadline(t, root)
	if testCase.CurrentRun != nil || testCase.HeadRun == nil || !contains(testCase.HeadRun.StaleReasons, "test revision changed") {
		t.Fatalf("pass after revision = current %#v head %#v", testCase.CurrentRun, testCase.HeadRun)
	}
	if _, err := RecordRun(root, RecordRunInput{ID: "old-source", TestCase: deadlineURN, Parents: testCase.RunHeads, Result: RunPassed, Summary: "Ran elsewhere.",
		Source: &SourceIdentity{Repository: "https://example.com/other.git", Commit: "89abcdef0123456789abcdef0123456789abcdef"}, Evidence: []string{evidence.URN}}); err != nil {
		t.Fatal(err)
	}
	if testCase = loadDeadline(t, root); testCase.CurrentRun != nil || !contains(testCase.HeadRun.StaleReasons, "source repository changed") {
		t.Fatalf("other-source run projection = %#v", testCase.HeadRun)
	}
}

func TestPolicySetKeepsOneHeadPerCriterionRevision(t *testing.T) {
	root := newQualitySaga(t, false)
	criterion := "urn:change-saga:checkout:story:refund:criterion:cutoff"
	storyRevision := "sha256:" + strings.Repeat("c", 64)
	first, err := SetPolicy(root, SetPolicyInput{Feature: "core",
		Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoverageEdge, CoveragePositive},
		AllowedAutomation: []Automation{AutomationManual, AutomationAutomated}, Rationale: "Boundary risk.", RequestID: "policy-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.URN != "urn:change-saga:checkout:quality-policy:policy" || first.Path != "___features/core.feature/___quality/policies/policy.json" {
		t.Fatalf("default policy = %#v", first)
	}
	replay, err := SetPolicy(root, SetPolicyInput{Feature: "core",
		Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive, CoverageEdge},
		AllowedAutomation: []Automation{AutomationAutomated, AutomationManual}, Rationale: "Boundary risk.", RequestID: "policy-1",
	})
	if err != nil || !replay.Replayed || replay.URN != first.URN {
		t.Fatalf("policy replay = %#v, %v", replay, err)
	}
	if _, err := SetPolicy(root, SetPolicyInput{Feature: "core", Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive}, AllowedAutomation: []Automation{AutomationManual}, Rationale: "Narrower."}); err == nil || !strings.Contains(err.Error(), "supersede every current head") {
		t.Fatalf("competing policy error = %v", err)
	}
	second, err := SetPolicy(root, SetPolicyInput{Feature: "core", Criterion: criterion, StoryRevision: storyRevision, RequiredKinds: []CoverageKind{CoveragePositive, CoverageNegative, CoverageEdge}, AllowedAutomation: []Automation{AutomationAutomated}, Supersedes: []string{first.URN}, Rationale: "All kinds."})
	if err != nil {
		t.Fatal(err)
	}
	if second.URN != "urn:change-saga:checkout:quality-policy:policy-2" {
		t.Fatalf("second default id = %s", second.URN)
	}
	document, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(document.PolicySets) != 1 || document.PolicySets[0].Current == nil || document.PolicySets[0].Current.ID != "policy-2" {
		t.Fatalf("policy sets = %#v", document.PolicySets)
	}
	if kinds := document.PolicySets[0].Current.RequiredKinds; !reflect.DeepEqual(kinds, []CoverageKind{CoveragePositive, CoverageNegative, CoverageEdge}) {
		t.Fatalf("kinds = %v", kinds)
	}
}

// testRunCommit is the code commit the fixture runs executed against.
const testRunCommit = "0123456789abcdef0123456789abcdef01234567"
