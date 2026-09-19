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

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A test case becomes coverage only through a pinned v5 verifies relation.
// The writer defaults each omitted pin to the unique current head and says
// so; a later story revision makes the link stale with a concrete reason.
func TestV5RelationLinksTestCaseToCriterionAndGoesStaleOnRevision(t *testing.T) {
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
	runQuality(t, "", "test-case", "add", root, "--epic", testEpic, "--id", "fast", "--title", "Fast checkout", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")

	var output bytes.Buffer
	if err := Relation(ctx, []string{"add", root, "--epic", testEpic, "--id", "fast-verifies", "--type", "verifies", "--from", testCase,
		"--to", story + ":criterion:fast", "--rationale", "Exercises the fast path.", "--json"}, &output); err != nil {
		t.Fatalf("relation add: %v\n%s", err, output.String())
	}
	var added relationAddOutput
	if err := json.Unmarshal(output.Bytes(), &added); err != nil {
		t.Fatal(err)
	}
	wantDefaults := []string{
		"from_revision to current head " + testCase + ":revision:r1",
		"to_revision to current head " + story + ":revision:r1",
	}
	if !added.OK || added.Resource != prefix+":relation:fast-verifies" || !reflect.DeepEqual(added.DefaultedPins, wantDefaults) {
		t.Fatalf("relation add output = %s", output.String())
	}
	record, err := os.ReadFile(filepath.Join(testEpicDir(root), "___requirements", "relations", "fast-verifies.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(record, []byte(requirements.V5RelationSchemaURL)) || !bytes.Contains(record, []byte(`"scope": "self"`)) {
		t.Fatalf("relation record = %s", record)
	}
	assertValid(t, root)

	status := func() requirements.RelationCurrency {
		t.Helper()
		var output bytes.Buffer
		if err := Relation(ctx, []string{"status", root, "--json"}, &output); err != nil {
			t.Fatalf("relation status: %v", err)
		}
		var result relationStatusOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil || len(result.Relations) != 1 {
			t.Fatalf("relation status = %s (%v)", output.String(), err)
		}
		return result.Relations[0]
	}
	if got := status(); !got.Current() {
		t.Fatalf("fresh relation = %#v", got)
	}

	if err := Story(ctx, []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1",
		"--persona", testPersonaURN, "--title", "Checkout", "--statement", "As a buyer I can check out quickly", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--json"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := status()
	if got.Status != requirements.CurrencyStale || len(got.Reasons) != 1 || got.Reasons[0].Code != requirements.ReasonRevisionChanged ||
		got.Reasons[0].Pinned != story+":revision:r1" || !reflect.DeepEqual(got.Reasons[0].Current, []string{story + ":revision:r2"}) {
		t.Fatalf("revised relation = %#v", got)
	}
	var text bytes.Buffer
	if err := Relation(ctx, []string{"status", root}, &text); err != nil || !strings.Contains(text.String(), "stale\n  revision_changed: to revision changed") {
		t.Fatalf("text status = %v\n%s", err, text.String())
	}

	// A link to a test case the quality domain does not know is refused.
	if err := Relation(ctx, []string{"add", root, "--epic", testEpic, "--id", "ghost", "--type", "verifies", "--from", prefix + ":test-case:ghost",
		"--to", story + ":criterion:fast", "--rationale", "No such case."}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("ghost test case error = %v", err)
	}
}

func addCheckoutStory(t *testing.T, root, id string) error {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	err = Story(context.Background(), []string{
		"add", root, "--epic", testEpic, "--persona", personaURNFor(manifest.ID), "--id", id, "--revision", "r1", "--event", "proposed",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--request-id", id + "-request", "--json",
	}, &output)
	if err != nil {
		return fmt.Errorf("%w: %s", err, output.String())
	}
	return nil
}
