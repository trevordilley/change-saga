package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
)

// The dogfooding repro, end to end through status: eleven app.saga stories
// gained a persona and changed nothing else, and seven true relations were
// called stale. A relation to a criterion asserts something about that
// criterion; a revision that leaves the criterion alone carries its pin
// forward, reports that it did, and asks nothing of anyone.
func TestAddingAPersonaToAStoryStalesNothingAndSaysWhatItCarried(t *testing.T) {
	ctx := context.Background()
	root, repo := coveredSaga(t)
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
	var output bytes.Buffer
	mustRun := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output.String())
		}
	}
	mustRun("accept", Story(ctx, []string{"set-state", root, "--story", story, "--event", "accepted",
		"--parent", story + ":event:proposed", "--state", "accepted", "--json"}, &output))
	runQuality(t, "", "test-case", "add", root, "--feature", testFeature, "--id", "fast", "--title", "Fast checkout", "--kind", "positive",
		"--automation", "automated", "--step", `{"id":"s1","action":"Check out","expected_result":"Done"}`, "--expected-result", "Done")
	mustRun("relation add", Relation(ctx, []string{"add", root, "--feature", testFeature, "--id", "fast-verifies", "--type", "verifies",
		"--from", testCase, "--to", story + ":criterion:fast", "--rationale", "Exercises the fast path.", "--json"}, &output))

	// Exactly the repro: one more persona, every criterion byte-identical.
	mustRun("add persona", Persona(ctx, []string{"add", root, "--id", "newcomer", "--revision", "r1", "--event", "active",
		"--name", "Newcomer", "--description", "Reads the app for the first time", "--json"}, &output))
	mustRun("story revise", Story(ctx, []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1",
		"--persona", personaURNFor(document.SagaID), "--persona", prefix + ":persona:newcomer",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--json"}, &output))

	output.Reset()
	_ = Status(ctx, []string{"--against", "main", "--json", "--repo", repo, root}, &output)
	var status struct {
		Stale          []struct{} `json:"stale"`
		CarriedForward []struct {
			Record    string `json:"record"`
			Endpoint  string `json:"endpoint"`
			Code      string `json:"code"`
			Confirmed string `json:"confirmed"`
			Current   string `json:"current"`
		} `json:"carried_forward"`
		NextActions []struct {
			ID       string `json:"id"`
			Category string `json:"category"`
		} `json:"next_actions"`
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatalf("status: %v\n%s", err, output.String())
	}
	if len(status.Stale) != 0 {
		t.Fatalf("a persona changed no criterion, so nothing is stale: %s", output.String())
	}
	for _, action := range status.NextActions {
		if action.Category == "stale" {
			t.Fatalf("a persona changed no criterion, so nothing is asked: %v", action.ID)
		}
	}
	if len(status.CarriedForward) != 1 || status.CarriedForward[0].Record != relation ||
		status.CarriedForward[0].Code != "criterion_unchanged" ||
		status.CarriedForward[0].Confirmed != story+":revision:r1" || status.CarriedForward[0].Current != story+":revision:r2" {
		t.Fatalf("status says which pin it carried and from where: %#v", status.CarriedForward)
	}

	// The text report says it too: a reader can tell the revision somebody
	// confirmed from the one the tool carried the relation past.
	output.Reset()
	_ = Status(ctx, []string{"--against", "main", "--repo", repo, root}, &output)
	if !strings.Contains(output.String(), "Carried-forward pins: 1 relations still hold, and were not re-confirmed by anyone") ||
		!strings.Contains(output.String(), "confirmed at "+story+":revision:r1, carried to "+story+":revision:r2") {
		t.Fatalf("status text = %s", output.String())
	}
}
