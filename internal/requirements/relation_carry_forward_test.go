package requirements

import (
	"reflect"
	"testing"
	"time"
)

// The real repro: eleven app.saga stories gained a persona and changed nothing
// else, and every relation pinned to a criterion of those stories was called
// stale. A relation to a criterion asserts something about that criterion, so
// it is stale only when the criterion's own statement changed or the criterion
// left the story; a revision that touches anything else carries the pin
// forward, and says so.
func TestRelationToUnchangedCriterionSurvivesAnUnrelatedRevision(t *testing.T) {
	root := newV5Saga(t)
	if _, err := AddRelation(root, "test", verifiesInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := AddPersona(root, "test", AddPersonaInput{ID: "newcomer", RevisionID: "r1", EventID: "active",
		Name: "Newcomer", Description: "Reads the app for the first time", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	// Exactly the repro: one more persona, every criterion byte-identical.
	if _, err := ReviseStory(root, "test", ReviseStoryInput{
		Story: v5Story, ID: "r2", Parents: []string{v5StoryR1}, Title: "Checkout",
		Statement: "As a buyer, I can check out", Priority: "high",
		Personas:           append(append([]string{}, testPersonas...), "urn:change-saga:test:persona:newcomer"),
		AcceptanceCriteria: []Criterion{{ID: "fast", Statement: "Checkout finishes promptly"}},
		CreatedAt:          testTime.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	inputs := StaleInputs{}
	inputs.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR1}})
	document, err := LoadWithOptions(root, "test", LoadOptions{StaleInputs: inputs})
	if err != nil {
		t.Fatal(err)
	}
	got := EvaluateRelations(document, inputs)
	if len(got) != 1 {
		t.Fatalf("currencies = %#v", got)
	}
	if !got[0].Current() || len(got[0].Reasons) != 0 {
		t.Fatalf("relation to an unchanged criterion = %#v", got[0])
	}
	// The pin still names the revision a person confirmed, and the report says
	// the rest was carried forward rather than affirmed.
	want := []CarriedForward{{Endpoint: "to", Code: CarriedCriterionUnchanged, Confirmed: v5StoryR1,
		Current: "urn:change-saga:test:story:checkout:revision:r2",
		Message: "to criterion statement is unchanged since the confirmed revision"}}
	if !reflect.DeepEqual(got[0].CarriedForward, want) {
		t.Fatalf("carried forward = %#v", got[0].CarriedForward)
	}
	if document.Relations[0].Stale {
		t.Fatalf("load projection still calls it stale: %v", document.Relations[0].StaleReasons)
	}
}

// The signal must still fire: reword the criterion and the relation is stale,
// and says which criterion changed under it.
func TestRelationGoesStaleWhenTheCriterionStatementChanges(t *testing.T) {
	root := newV5Saga(t)
	if _, err := AddRelation(root, "test", verifiesInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviseStory(root, "test", ReviseStoryInput{Personas: testPersonas,
		Story: v5Story, ID: "r2", Parents: []string{v5StoryR1}, Title: "Checkout",
		Statement:          "As a buyer, I can check out",
		AcceptanceCriteria: []Criterion{{ID: "fast", Statement: "Checkout finishes in two seconds"}},
		CreatedAt:          testTime.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	inputs := StaleInputs{}
	inputs.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR1}})
	got := EvaluateRelations(document, inputs)[0]
	if got.Status != CurrencyStale || len(got.CarriedForward) != 0 {
		t.Fatalf("reworded criterion = %#v", got)
	}
	if got.Reasons[0].Code != ReasonCriterionStatementChanged {
		t.Fatalf("reason = %#v", got.Reasons)
	}
}

// Whitespace and typo fixes are judgment calls by definition, so they stale.
func TestCosmeticCriterionEditsAreStillStale(t *testing.T) {
	for name, statement := range map[string]string{
		"trailing whitespace": "Checkout finishes promptly ",
		"typo fix":            "Checkout finishes promptly.",
		"case":                "checkout finishes promptly",
	} {
		t.Run(name, func(t *testing.T) {
			root := newV5Saga(t)
			if _, err := AddRelation(root, "test", verifiesInput()); err != nil {
				t.Fatal(err)
			}
			if _, err := ReviseStory(root, "test", ReviseStoryInput{Personas: testPersonas,
				Story: v5Story, ID: "r2", Parents: []string{v5StoryR1}, Title: "Checkout",
				Statement:          "As a buyer, I can check out",
				AcceptanceCriteria: []Criterion{{ID: "fast", Statement: statement}},
				CreatedAt:          testTime.Add(time.Minute),
			}); err != nil {
				t.Fatal(err)
			}
			document, err := Load(root, "test")
			if err != nil {
				t.Fatal(err)
			}
			inputs := StaleInputs{}
			inputs.SetTestCaseHeads("test", map[string][]string{"fast-path": {v5TestCaseR1}})
			if got := EvaluateRelations(document, inputs)[0]; got.Status != CurrencyStale {
				t.Fatalf("%s = %#v", name, got)
			}
		})
	}
}
