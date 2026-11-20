package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelationRepinEditsCurrentConfirmationAndGitPreservesPriorPins(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := newLivingSaga(t)
	if err := addCheckoutStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	story := "urn:change-saga:atomic:story:checkout"
	testCase := "urn:change-saga:atomic:test-case:fast"
	relation := "urn:change-saga:atomic:relation:fast-verifies"
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast checkout", "--kind", "positive", "--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	if err := Relation(ctx, []string{"add", root, "--feature", testFeature, "--id", "fast-verifies", "--type", "verifies", "--from", testCase, "--to", story + ":criterion:fast", "--rationale", "Exercises the fast path."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	oldPin := currentTestPin(t, root, story)
	path := filepath.Join(testFeatureRel, "___requirements/relations/fast-verifies.json")
	oldBytes, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	commit := commitCurrentTestDocumentation(t, root, "Original confirmation")
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--rationale", "Still true."}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nothing to re-pin") {
		t.Fatalf("unchanged confirmation: %v", err)
	}
	if err := Story(ctx, []string{"revise", root, "--story", story, "--persona", testPersonaURN, "--title", "Checkout", "--statement", "As a buyer I can check out quickly", "--priority", "must", "--criterion", "fast=Checkout finishes in two seconds"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	newPin := currentTestPin(t, root, story)
	var output bytes.Buffer
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--rationale", "Two seconds is the promptness the case asserts.", "--json"}, &output); err != nil {
		t.Fatalf("reassessment: %v %s", err, output.String())
	}
	var result relationAddOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Resource != relation || len(result.DefaultedPins) != 1 || !strings.Contains(result.DefaultedPins[0], oldPin) || !strings.Contains(result.DefaultedPins[0], newPin) {
		t.Fatalf("current confirmation result: %s", output.String())
	}
	now, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(now, []byte(newPin)) || bytes.Contains(now, []byte(oldPin)) {
		t.Fatalf("confirmation did not update in place: %s", now)
	}
	if got := committedCurrentTestFile(t, root, commit, filepath.ToSlash(path)); got != string(oldBytes) {
		t.Fatal("Git lost the original confirmation")
	}
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements/relation-repins")); !os.IsNotExist(err) {
		t.Fatalf("secondary confirmation history retained: %v", err)
	}
	output.Reset()
	if err := Relation(ctx, []string{"status", root, "--relation", relation, "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var status relationStatusOutput
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Relations) != 1 || !status.Relations[0].Current() {
		t.Fatalf("reassessment not current: %s", output.String())
	}
	for _, extra := range [][]string{nil, {"--to-definition-digest", newPin}} {
		args := append([]string{"repin", root, "--relation", relation, "--rationale", "Already confirmed."}, extra...)
		if err := Relation(ctx, args, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nothing to re-pin") {
			t.Fatalf("duplicate reassessment %v: %v", extra, err)
		}
	}
	if err := Relation(ctx, []string{"supersede", root, "--relation", relation}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--to-definition-digest", newPin, "--rationale", "Too late."}, &bytes.Buffer{}); err == nil {
		t.Fatal("superseded relation was re-confirmed")
	}
	assertValid(t, root)
}
