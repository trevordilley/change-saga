package quality

import (
	"github.com/twentyideas/changesaga/internal/coderef"
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentDefinitionKeepsEvidenceAndMakesItStaleAfterEdit(t *testing.T) {
	root := newQualitySaga(t, false)
	definition := deadlineDefinition()
	if _, err := AddTestCase(root, AddTestCaseInput{Feature: "core", ID: "deadline", Definition: definition}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddEvidence(root, AddEvidenceInput{ID: "assertion", TestCase: deadlineURN, Role: EvidenceTestImplementation, Steps: []string{"inspect"}, Code: []coderef.Reference{currentSelector(t, "refund_test.go")}}); err != nil {
		t.Fatal(err)
	}
	before := loadDeadline(t, root)
	if len(before.Evidence) != 1 || !before.Evidence[0].Current {
		t.Fatal("fresh evidence is not current")
	}
	pin := before.Evidence[0].TestRevision
	definition.Steps[1].ExpectedResult = "The newly agreed result"
	if _, err := ReviseTestCase(root, ReviseTestCaseInput{TestCase: deadlineURN, Definition: definition}); err != nil {
		t.Fatal(err)
	}
	after := loadDeadline(t, root)
	if len(after.Evidence) != 1 || after.Evidence[0].Current || after.Evidence[0].TestRevision != pin {
		t.Fatalf("historical evidence was lost or silently made current: %+v", after.Evidence)
	}
	if after.CurrentRevision.Steps[1].ID != definition.Steps[1].ID {
		t.Fatal("stable step identity changed")
	}
	dir := filepath.Join(root, filepath.FromSlash(testCasePath("core", "deadline")))
	if _, err := os.Stat(filepath.Join(dir, "revisions")); !os.IsNotExist(err) {
		t.Fatalf("internal test history retained: %v", err)
	}
}
