package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
)

// Re-affirming a relation keeps it. Before repin the only way to say "yes, it
// still holds" was to retire a true relation and add a renamed copy; now the
// relation keeps its id, its rationale, and the record of every revision
// anyone read it against.
func TestRelationRepinKeepsTheRelationAndRecordsWhatWasConfirmed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := newLivingSaga(t)
	if err := addCheckoutStory(t, root, "checkout"); err != nil {
		t.Fatal(err)
	}
	document, err := requirements.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	prefix := "urn:change-saga:" + document.SagaID
	story, testCase := prefix+":story:checkout", prefix+":test-case:fast"
	relation := prefix + ":relation:fast-verifies"
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast checkout", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	if err := Relation(ctx, []string{"add", root, "--feature", testFeature, "--id", "fast-verifies", "--type", "verifies", "--from", testCase,
		"--to", story + ":criterion:fast", "--rationale", "Exercises the fast path.", "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	// Nothing has moved, so there is nothing to confirm.
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--rationale", "Still true."}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "nothing to re-pin") {
		t.Fatalf("repin of a current relation = %v", err)
	}

	// Reword the criterion; now a person has to read it.
	if err := Story(ctx, []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1",
		"--persona", testPersonaURN, "--title", "Checkout", "--statement", "As a buyer I can check out quickly", "--priority", "must",
		"--criterion", "fast=Checkout finishes in two seconds", "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := Relation(ctx, []string{"repin", root, "--relation", relation,
		"--rationale", "Two seconds is the promptness the case already asserts.", "--request-id", "repin-1", "--json"}, &output); err != nil {
		t.Fatalf("repin: %v\n%s", err, output.String())
	}
	var repinned relationAddOutput
	if err := json.Unmarshal(output.Bytes(), &repinned); err != nil {
		t.Fatal(err)
	}
	wantURN := relation + ":repin:r2"
	if !repinned.OK || repinned.Resource != wantURN || len(repinned.DefaultedPins) != 1 ||
		!strings.Contains(repinned.DefaultedPins[0], "to_revision from "+story+":revision:r1 to the current head "+story+":revision:r2") {
		t.Fatalf("repin output = %s", output.String())
	}

	// The relation record is untouched: it still names the revision its author
	// pinned, and the confirmation is a record of its own beside it.
	record, err := os.ReadFile(filepath.Join(testFeatureDir(root), "___requirements", "relations", "fast-verifies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(record, []byte(story+":revision:r1")) || bytes.Contains(record, []byte("repin")) {
		t.Fatalf("the relation record was rewritten: %s", record)
	}
	if _, err := os.ReadFile(filepath.Join(testFeatureDir(root), "___requirements", "relation-repins", "fast-verifies", "r2.json")); err != nil {
		t.Fatalf("the repin is not beside its relation: %v", err)
	}
	assertValid(t, root)

	// The relation is current again, and says so through the confirmed pin.
	var text bytes.Buffer
	if err := Relation(ctx, []string{"status", root, "--relation", relation, "--json"}, &text); err != nil {
		t.Fatal(err)
	}
	var status relationStatusOutput
	if err := json.Unmarshal(text.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Relations) != 1 || !status.Relations[0].Current() || len(status.Relations[0].CarriedForward) != 0 {
		t.Fatalf("repinned relation = %s", text.String())
	}

	// Naming the pin explicitly is no way around the no-op check. This is the
	// command the stale next action prints, so running it twice has to refuse
	// the second time; a repin record is immutable, so the noise is permanent.
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--to-revision", story + ":revision:r2",
		"--rationale", "Saying it again."}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nothing to re-pin") {
		t.Fatalf("repin of the pin already confirmed = %v", err)
	}

	// Replaying the same request writes nothing new; a second repin under the
	// same id is refused rather than editing the record.
	output.Reset()
	if err := Relation(ctx, []string{"repin", root, "--relation", relation,
		"--rationale", "Two seconds is the promptness the case already asserts.", "--request-id", "repin-1", "--json"}, &output); err != nil {
		t.Fatalf("replayed repin: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), `"replayed": true`) {
		t.Fatalf("replayed repin output = %s", output.String())
	}
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--id", "r2",
		"--to-revision", story + ":revision:r2", "--rationale", "Different words."}, &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second repin under one id = %v", err)
	}

	// A superseded relation is retired, not re-affirmed.
	if err := Relation(ctx, []string{"supersede", root, "--relation", relation, "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := Relation(ctx, []string{"repin", root, "--relation", relation, "--to-revision", story + ":revision:r2",
		"--rationale", "Too late."}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("repin of a superseded relation = %v", err)
	}
}
