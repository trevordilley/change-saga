package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/quality"
)

const qualityTestURN = "urn:change-saga:checkout:test-case:deadline"

// newQualityFixture hand-authors a v5 manifest. The core Saga loader does not
// accept v5 yet, but the quality writers only require what quality.Load does.
func newQualityFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "checkout.saga")
	data, err := json.Marshal(map[string]any{
		"$schema": quality.ManifestSchemaURL, "version": quality.Version, "id": "checkout", "title": "Checkout",
		"source": map[string]string{"repository": "https://example.com/repo.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "saga.json"), string(data))
	var output bytes.Buffer
	if err := Epic(context.Background(), []string{"add", "--id", testEpic, "--title", "Core", root}, &output); err != nil {
		t.Fatalf("epic add: %v\n%s", err, output.String())
	}
	return root
}

func runQuality(t *testing.T, stdin string, args ...string) livingMutationOutput {
	t.Helper()
	var output bytes.Buffer
	if err := qualityCommand(context.Background(), append(args, "--json"), &output, strings.NewReader(stdin)); err != nil {
		t.Fatalf("quality %v: %v\n%s", args, err, output.String())
	}
	return decodeLivingOutput(t, &output)
}

// qualityCode commits a twelve-line file and returns the checkout and the
// location of its lines 3-12, the code a quality evidence record pins.
func qualityCode(t *testing.T, path string) (repo, location string) {
	t.Helper()
	var body strings.Builder
	for line := 1; line <= 12; line++ {
		fmt.Fprintf(&body, "// line %d\n", line)
	}
	repo, commit := sourceRepo(t, map[string]string{path: body.String()})
	return repo, commit + ":" + path + "#L3-L12"
}

func TestQualityHelpListsTheAuthoringGrammar(t *testing.T) {
	var first, second bytes.Buffer
	if err := Quality(context.Background(), []string{"-h"}, &first); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help error = %v", err)
	}
	if err := Quality(context.Background(), []string{"--help"}, &second); !errors.Is(err, flag.ErrHelp) || first.String() != second.String() {
		t.Fatalf("help is not deterministic: %v", err)
	}
	for _, operation := range qualityOperations {
		if !strings.Contains(first.String(), operation) {
			t.Fatalf("help omitted %q:\n%s", operation, first.String())
		}
		var help bytes.Buffer
		if err := Quality(context.Background(), append(strings.Fields(operation), "-h"), &help); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%s help error = %v", operation, err)
		}
		if usage := commandUsage["quality "+operation]; usage == "" || !strings.Contains(help.String(), usage) {
			t.Fatalf("%s help omitted its usage line:\n%s", operation, help.String())
		}
	}
	var family bytes.Buffer
	if err := Quality(context.Background(), []string{"test-case", "-h"}, &family); !errors.Is(err, flag.ErrHelp) || !strings.Contains(family.String(), "add\n  revise\n  set-state") {
		t.Fatalf("family help = %v\n%s", err, family.String())
	}
	var overview bytes.Buffer
	PrintHelp(&overview)
	if !strings.Contains(overview.String(), commandUsage["quality"]) {
		t.Fatal("top-level help omitted quality")
	}
}

func TestQualityCommandsAuthorTheFrozenRecordsEndToEnd(t *testing.T) {
	root := newQualityFixture(t)
	definition := `{"id":"deadline","title":"Reject at deadline","coverage_kinds":["negative","edge"],"automation":"automated",
		"preconditions":["A purchase is exactly 30 days old."],
		"steps":[{"id":"submit","action":"Submit a refund request.","expected_result":"The request is rejected."}],
		"expected_result":"No refund is created.","request_id":"add-deadline"}`
	added := runQuality(t, definition, "test-case", "add", root, "--epic", testEpic, "--from", "-")
	wantCreated := []string{qualityTestURN, qualityTestURN + ":revision:r1", qualityTestURN + ":event:proposed"}
	if !added.OK || added.Resource != qualityTestURN || !reflect.DeepEqual(added.Created, wantCreated) {
		t.Fatalf("add = %#v", added)
	}
	if replay := runQuality(t, definition, "test", "add", root, "--epic", testEpic, "--from", "-"); !replay.Replayed {
		t.Fatalf("identical request did not replay: %#v", replay)
	}

	// A single-parent revision inherits every omitted field from its parent.
	revised := runQuality(t, "", "test-case", "revise", root, "--test", qualityTestURN, "--parent", qualityTestURN+":revision:r1",
		"--revision", "r2", "--step", `{"id":"submit","action":"Submit a refund request.","expected_result":"The request is rejected."}`,
		"--step", `{"id":"inspect","action":"Inspect the reason.","expected_result":"The cutoff reason is returned."}`)
	if !reflect.DeepEqual(revised.CurrentHeads, []string{qualityTestURN + ":revision:r2"}) {
		t.Fatalf("revise = %#v", revised)
	}
	runQuality(t, "", "test-case", "set-state", root, "--test-case", qualityTestURN, "--parent", qualityTestURN+":event:proposed", "--state", "active", "--reason", "Ready to run.")

	codeRepo, codeLocation := qualityCode(t, "internal/refund_test.go")
	evidence := runQuality(t, "", "evidence", "add", root, "--test", qualityTestURN, "--role", "test_implementation", "--repo", codeRepo, "--code", codeLocation)
	manual := runQuality(t, `[{"id":"qa","test_case":"`+qualityTestURN+`","role":"execution_artifact","citations":["urn:change-saga:checkout:citation:qa-notes"]}]`,
		"evidence", "add", root, "--batch", "-")
	failed := runQuality(t, "", "run", "record", root, "--commit", "0123456789abcdef0123456789abcdef01234567", "--test", qualityTestURN, "--id", "ci-1", "--result", "failed",
		"--summary", "Wrong reason.", "--evidence", evidence.Resource, "--command", "go test ./internal/refund")
	passed := runQuality(t, "", "run", "record", root, "--commit", "0123456789abcdef0123456789abcdef01234567", "--test", qualityTestURN, "--id", "ci-2", "--parent", failed.Resource,
		"--result", "passed", "--summary", "Manual and CI pass.", "--evidence", evidence.Resource, "--evidence", manual.Resource)
	runQuality(t, "", "policy", "set", root, "--epic", testEpic, "--criterion", "urn:change-saga:checkout:story:refund:criterion:cutoff",
		"--story-revision", "urn:change-saga:checkout:story:refund:revision:r2", "--require", "positive", "--require", "edge",
		"--allow", "automated", "--rationale", "The exact cutoff is a distinct risk.")

	document, err := quality.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	testCase := document.TestCases[0]
	if testCase.CurrentRevision.ID != "r2" || len(testCase.CurrentRevision.Steps) != 2 || testCase.CurrentRevision.Title != "Reject at deadline" {
		t.Fatalf("revision = %#v", testCase.CurrentRevision)
	}
	if testCase.CurrentLifecycle.State != quality.StateActive || testCase.CurrentLifecycle.Reason != "Ready to run." {
		t.Fatalf("lifecycle = %#v", testCase.CurrentLifecycle)
	}
	if testCase.CurrentRun == nil || testCase.CurrentRun.ID != "ci-2" || len(testCase.Runs) != 2 {
		t.Fatalf("runs = %#v", testCase.Runs)
	}
	if passed.Resource != qualityTestURN+":run:ci-2" || len(document.PolicySets) != 1 || document.PolicySets[0].Current == nil {
		t.Fatalf("passed = %#v policies = %#v", passed, document.PolicySets)
	}
	var pinned []string
	for _, value := range testCase.Evidence {
		for _, reference := range value.Code {
			pinned = append(pinned, reference.Location().String())
			if !strings.HasPrefix(reference.Digest, "sha256:") {
				t.Fatalf("quality code reference has no digest: %#v", reference)
			}
		}
	}
	if !reflect.DeepEqual(pinned, []string{codeLocation}) {
		t.Fatalf("quality evidence code = %v, want %s", pinned, codeLocation)
	}
	for _, record := range []string{"runs/ci-1.json", "evidence/test-implementation.json", "evidence/qa.json"} {
		if _, err := os.Stat(filepath.Join(testEpicDir(root), "___quality", "test-cases", "deadline.test", filepath.FromSlash(record))); err != nil {
			t.Fatalf("missing %s: %v", record, err)
		}
	}
}

func TestQualityMutationFailureReportsJSONAndWritesNothing(t *testing.T) {
	root := newQualityFixture(t)
	var output bytes.Buffer
	err := Quality(context.Background(), []string{
		"test-case", "add", root, "--epic", testEpic, "--id", "deadline", "--title", "No kinds", "--automation", "manual",
		"--expected-result", "Nothing.", "--json",
	}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("failure status = %v\n%s", err, output.String())
	}
	var failure livingMutationOutput
	if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil || failure.OK || failure.Operation != "quality test-case add" || failure.Error == nil {
		t.Fatalf("failure output = %s (%v)", output.String(), decodeErr)
	}
	if _, statErr := os.Lstat(filepath.Join(testEpicDir(root), quality.RootDir)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed add adopted quality: %v", statErr)
	}
	output.Reset()
	if err := qualityCommand(context.Background(), []string{"test-case", "add", root, "--epic", testEpic, "--from", "-"}, &output, strings.NewReader(`{"id":"x","score":1}`)); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("strict structured input error = %v", err)
	}
}

func TestValidateReportsQualityRecords(t *testing.T) {
	root := newLivingSaga(t)
	var output bytes.Buffer
	if err := Validate(context.Background(), []string{"--json", root}, &output); err != nil {
		t.Fatalf("validate without quality: %v\n%s", err, output.String())
	}
	// A malformed quality root is reported, not ignored.
	if err := os.MkdirAll(filepath.Join(testEpicDir(root), quality.RootDir, "experiments"), 0o755); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	err := Validate(context.Background(), []string{"--json", root}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("validate error = %v\n%s", err, output.String())
	}
	var result validationOutput
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
		t.Fatal(decodeErr)
	}
	found := false
	for _, issue := range result.Issues {
		found = found || (issue.Path == quality.RootDir && strings.Contains(issue.Message, "experiments"))
	}
	if result.Valid || !found {
		t.Fatalf("validation did not report quality: %#v", result.Issues)
	}
}
