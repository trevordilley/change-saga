package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func newVerifyLoadingFixture(t *testing.T) (string, requirements.AddRelationInput) {
	t.Helper()
	root := newLivingSaga(t)
	if err := addCheckoutStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast checkout", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	prefix := "urn:change-saga:" + manifest.ID
	return root, requirements.AddRelationInput{Feature: testFeature, ID: "fast-verifies", Type: requirements.RelationVerifies,
		From: prefix + ":test-case:fast", To: prefix + ":story:checkout:criterion:fast", Rationale: "Exercises checkout."}
}

func verifyLoadingArgs(root string, input requirements.AddRelationInput) []string {
	return []string{"add", root, "--feature", input.Feature, "--id", input.ID, "--type", string(input.Type),
		"--from", input.From, "--to", input.To, "--rationale", input.Rationale,
		"--from-revision", input.FromRevision, "--to-revision", input.ToRevision, "--request-id", "verify-load", "--json"}
}

// Do not parallelize tests that assert the process-wide full-load counter.
func TestVerifyLoadingOneFullLoadAndImmutableReplay(t *testing.T) {
	root, input := newVerifyLoadingFixture(t)
	before := saga.FullLoadCount()
	var output bytes.Buffer
	if err := Relation(context.Background(), verifyLoadingArgs(root, input), &output); err != nil {
		t.Fatal(err)
	}
	if loads := saga.FullLoadCount() - before; loads != 1 {
		t.Fatalf("criterion verifies add used %d full loads, want 1", loads)
	}
	var added relationAddOutput
	if err := json.Unmarshal(output.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	story := strings.Split(input.To, ":criterion:")[0]
	wantDefaults := []string{"from_revision to current head " + input.From + ":revision:r1", "to_revision to current head " + story + ":revision:r1"}
	if !reflect.DeepEqual(added.DefaultedPins, wantDefaults) {
		t.Fatalf("defaulted pins = %#v", added.DefaultedPins)
	}
	path := filepath.Join(testFeatureDir(root), "___requirements", "relations", input.ID+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Story(context.Background(), []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1",
		"--persona", testPersonaURN, "--title", "Checkout", "--statement", "As a buyer I check out", "--priority", "must",
		"--criterion", "fast=A changed checkout criterion", "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	input.FromRevision, input.ToRevision = input.From+":revision:r1", story+":revision:r1"
	output.Reset()
	if err := Relation(context.Background(), verifyLoadingArgs(root, input), &output); err != nil {
		t.Fatal(err)
	}
	replayed := decodeLivingOutput(t, &output)
	if !replayed.Replayed || bytes.Contains(output.Bytes(), []byte("defaulted_pins")) {
		t.Fatalf("explicit historical replay = %s", output.String())
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatalf("replay changed immutable relation: %v", err)
	}
	currencies, err := relationCurrencies(root)
	if err != nil || len(currencies) != 1 || currencies[0].Current() {
		t.Fatalf("historical pin must remain stale: %#v, %v", currencies, err)
	}
}

func TestVerifyLoadingMatchesOriginalPinPreparation(t *testing.T) {
	root, base := newVerifyLoadingFixture(t)
	// Coverage parsing is deliberately retained once; repeat valid fixture
	// evidence to make the cost of the removed second load observable.
	coverage := fmt.Sprintf(`{"version":%d,"references":[{"commit":%q,"path":"fixture.go","start":1,"end":1,"digest":%q,"note":"Synthetic load fixture."}]}`, saga.CurrentVersion, strings.Repeat("a", 40), "sha256:"+strings.Repeat("b", 64))
	for i := 0; i < 128; i++ {
		writeFile(t, filepath.Join(overviewFragment(root), "___code", fmt.Sprintf("fixture-%03d.json", i)), coverage)
	}
	assertValid(t, root)
	story := strings.Split(base.To, ":criterion:")[0]
	cases := []struct {
		name      string
		change    func(*requirements.AddRelationInput)
		wantError string
		loads     uint64
	}{
		{"defaults", func(*requirements.AddRelationInput) {}, "", 1},
		{"explicit", func(i *requirements.AddRelationInput) {
			i.FromRevision = i.From + ":revision:r1"
			i.ToRevision = story + ":revision:r1"
		}, "", 1},
		{"missing-test-case", func(i *requirements.AddRelationInput) { i.From = strings.Replace(i.From, ":fast", ":missing", 1) }, "does not exist", 1},
		{"unknown-test-revision", func(i *requirements.AddRelationInput) { i.FromRevision = i.From + ":revision:missing" }, "does not exist", 1},
		{"missing-story", func(i *requirements.AddRelationInput) { i.To = strings.Replace(i.To, ":checkout:", ":missing:", 1) }, "does not exist", 1},
		{"story-target-fallback", func(i *requirements.AddRelationInput) { i.To = story }, "", 2},
	}
	var originalTime, reusedTime time.Duration
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.change(&input)
			oldInput, newInput := input, input
			manifest, _ := saga.ReadManifest(root)
			relationURN := "urn:change-saga:" + manifest.ID + ":relation:" + input.ID
			start := time.Now()
			if err := guardEmbeddedCriterionRelationMutation(root, relationURN); err != nil {
				t.Fatal(err)
			}
			oldDefaults, oldErr := defaultV5RelationPins(root, &oldInput)
			originalTime += time.Since(start)
			before := saga.FullLoadCount()
			start = time.Now()
			report, err := loadAndGuardEmbeddedCriterionRelationMutation(root, relationURN)
			if err != nil {
				t.Fatal(err)
			}
			newDefaults, newErr := defaultV5RelationPinsWithGuardedSaga(root, &newInput, report)
			reusedTime += time.Since(start)
			if loads := saga.FullLoadCount() - before; loads != tc.loads {
				t.Fatalf("loads = %d, want %d", loads, tc.loads)
			}
			if fmt.Sprint(oldErr) != fmt.Sprint(newErr) || !reflect.DeepEqual(oldInput, newInput) || !reflect.DeepEqual(oldDefaults, newDefaults) {
				t.Fatalf("pin preparation changed: old=%#v/%v/%v new=%#v/%v/%v", oldInput, oldDefaults, oldErr, newInput, newDefaults, newErr)
			}
			if tc.wantError != "" && (newErr == nil || !strings.Contains(newErr.Error(), tc.wantError)) {
				t.Fatalf("error = %v, want %q", newErr, tc.wantError)
			}
		})
	}
	t.Logf("six pin preparations with 128 coverage files: original=%s reused=%s (includes one fallback; informational timings only)", originalTime, reusedTime)
}

func TestVerifyLoadingRetainsFullCoverageValidation(t *testing.T) {
	root, input := newVerifyLoadingFixture(t)
	writeFile(t, filepath.Join(overviewFragment(root), "___code", "broken.json"), fmt.Sprintf(`{"version":%d,"references":"invalid"}`, saga.CurrentVersion))
	_, validation, loadErr := saga.Load(root)
	if loadErr == nil && validation.Valid {
		t.Fatal("fixture must fail full validation")
	}
	if _, narrativeValidation, err := saga.LoadNarrative(root); err != nil || !narrativeValidation.Valid {
		t.Fatalf("fixture must isolate validation omitted by narrative-only loading: %v, %#v", err, narrativeValidation.Issues)
	}
	var output bytes.Buffer
	err := relationAdd(context.Background(), verifyLoadingArgs(root, input)[1:], &output)
	if err == nil {
		t.Fatal("invalid coverage was ignored")
	}
	if loadErr == nil && err.Error() != "the Saga is invalid; run change-saga validate for details" {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements", "relations", input.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("invalid Saga published relation: %v", err)
	}
}

func TestVerifyLoadingRetainsEmbeddedIDCollisionGuard(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Run", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Run","expected_result":"Done"}`, "--expected-result", "Done")
	input := requirements.AddRelationInput{Feature: testFeature, ID: "worker-explains-returns", Type: requirements.RelationVerifies,
		From: "urn:change-saga:" + sagaID + ":test-case:fast", To: "urn:change-saga:" + sagaID + ":story:run:criterion:returns", Rationale: "Check run."}
	err := relationAdd(context.Background(), verifyLoadingArgs(root, input)[1:], &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "managed by complete-slide transaction history") {
		t.Fatalf("collision error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements", "relations", input.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("collision published relation: %v", err)
	}
	assertValid(t, root)

	// Even verifies authoring retains design-digest errors, although its pins
	// are revision-based. Reusing the report must not skip asset validation.
	report, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	slide := report.Decks[0].Slides[0]
	if err := os.Truncate(filepath.Join(slide.Directory, slide.Entrypoint), saga.MaxDesignDigestBytesPerTarget+1); err != nil {
		t.Fatal(err)
	}
	oldInput, newInput := input, input
	_, oldErr := defaultV5RelationPins(root, &oldInput)
	_, newErr := defaultV5RelationPinsWithGuardedSaga(root, &newInput, report)
	if oldErr == nil || newErr == nil || oldErr.Error() != newErr.Error() || !strings.Contains(newErr.Error(), "index design content:") {
		t.Fatalf("design-digest error changed: original=%v reused=%v", oldErr, newErr)
	}
}

func TestVerifyLoadingRetainsDomainValidation(t *testing.T) {
	for _, domain := range []string{"work-plan", "quality"} {
		t.Run(domain, func(t *testing.T) {
			root, input := newVerifyLoadingFixture(t)
			if domain == "work-plan" {
				writeFile(t, filepath.Join(testFeatureDir(root), "___workplan", "unexpected.json"), `{}`)
			} else {
				writeFile(t, filepath.Join(testFeatureDir(root), "___quality", "test-cases", "fast.test", "test-case.json"), `{}`)
			}
			manifest, err := saga.ReadManifest(root)
			if err != nil {
				t.Fatal(err)
			}
			relationURN := "urn:change-saga:" + manifest.ID + ":relation:" + input.ID
			// Compare the complete legacy preparation to the new CLI. Some
			// domain failures are caught by the guard, others by pin loading.
			oldErr := guardEmbeddedCriterionRelationMutation(root, relationURN)
			if oldErr == nil {
				_, oldErr = defaultV5RelationPins(root, &input)
			}
			newErr := relationAdd(context.Background(), verifyLoadingArgs(root, input)[1:], &bytes.Buffer{})
			if oldErr == nil || newErr == nil || oldErr.Error() != newErr.Error() {
				t.Fatalf("domain error changed: original=%v new=%v", oldErr, newErr)
			}
			if _, err := os.Stat(filepath.Join(testFeatureDir(root), "___requirements", "relations", input.ID+".json")); !os.IsNotExist(err) {
				t.Fatalf("invalid domain published relation: %v", err)
			}
		})
	}
}
