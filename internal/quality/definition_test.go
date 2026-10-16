package quality

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
)

func TestPlannedCaseEnrichesSameIdentityWithPinnedStepEvidence(t *testing.T) {
	root := newQualitySaga(t, false)
	definition := deadlineDefinition()
	definition.Steps[1].Criteria = []string{"urn:change-saga:checkout:story:refund:criterion:cutoff"}
	input := AddTestCaseInput{Feature: "core", ID: "deadline", RevisionID: "r1", Definition: definition, RequestID: "planned"}
	if _, err := AddTestCase(root, input); err != nil {
		t.Fatal(err)
	}
	if replay, err := AddTestCase(root, input); err != nil || !replay.Replayed {
		t.Fatalf("mapped replay: %+v %v", replay, err)
	}
	test := loadDeadline(t, root)
	if DefinitionStage(test) != "planned" || len(test.Evidence) != 0 || len(test.Runs) != 0 {
		t.Fatalf("planned: %+v", test)
	}
	path := filepath.Join(root, "___features/core.feature/___quality/test-cases/deadline.test/revisions/r1.json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	evidence := AddEvidenceInput{ID: "assertion", TestCase: deadlineURN, Role: EvidenceTestImplementation, Steps: []string{"inspect"}, Code: []coderef.Reference{currentSelector(t, "refund_test.go")}, RequestID: "implement"}
	if _, err := AddEvidence(root, evidence); err != nil {
		t.Fatal(err)
	}
	if replay, err := AddEvidence(root, evidence); err != nil || !replay.Replayed {
		t.Fatalf("evidence replay: %+v %v", replay, err)
	}
	test = loadDeadline(t, root)
	if DefinitionStage(test) != "implementation_linked" || test.CurrentRevision.ID != "r1" || test.Evidence[0].TestRevision != deadlineURN+":revision:r1" {
		t.Fatalf("enriched: %+v", test)
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("enrichment rewrote the definition")
	}
	definition.Steps[1].ExpectedResult = "The revised reason is returned."
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{deadlineURN + ":revision:r1"}, Definition: definition}); err != nil {
		t.Fatal(err)
	}
	test = loadDeadline(t, root)
	if DefinitionStage(test) != "stale_evidence" || test.Evidence[0].Current {
		t.Fatalf("revision must stale evidence: %+v", test)
	}
	evidence.ID = "assertion-r2"
	evidence.RequestID = "implement-r2"
	evidence.Supersedes = []string{deadlineURN + ":evidence:assertion"}
	if _, err := AddEvidence(root, evidence); err != nil {
		t.Fatal(err)
	}
	test = loadDeadline(t, root)
	if DefinitionStage(test) != "implementation_linked" || len(test.Revisions) != 2 || len(test.Evidence) != 2 {
		t.Fatalf("history lost: %+v", test)
	}
	after, _ = os.ReadFile(path)
	if !reflect.DeepEqual(original, after) {
		t.Fatal("revision rewrote history")
	}
}

func TestStepMappingInvalidWritesAreAtomic(t *testing.T) {
	root := newQualitySaga(t, false)
	addDeadline(t, root)
	for _, steps := range [][]string{{"missing"}, {"inspect", "inspect"}, {"../inspect"}} {
		before := snapshotPaths(t, root)
		if _, err := AddEvidence(root, AddEvidenceInput{TestCase: deadlineURN, Role: EvidenceTestImplementation, Steps: steps, Code: []coderef.Reference{currentSelector(t, "refund_test.go")}}); err == nil {
			t.Fatalf("accepted %v", steps)
		}
		if !reflect.DeepEqual(before, snapshotPaths(t, root)) {
			t.Fatal("invalid mapping wrote files")
		}
	}
	for _, criteria := range [][]string{{"urn:change-saga:other:story:refund:criterion:cutoff"}, {"not-a-criterion"}, {"urn:change-saga:checkout:story:refund:criterion:cutoff", "urn:change-saga:checkout:story:refund:criterion:cutoff"}} {
		before := snapshotPaths(t, root)
		definition := deadlineDefinition()
		definition.Steps[0].Criteria = criteria
		if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, RevisionID: "r2", Parents: []string{deadlineURN + ":revision:r1"}, Definition: definition}); err == nil {
			t.Fatalf("accepted %v", criteria)
		}
		if !reflect.DeepEqual(before, snapshotPaths(t, root)) {
			t.Fatal("invalid criteria wrote files")
		}
	}
}
