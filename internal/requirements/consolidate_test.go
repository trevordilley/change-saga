package requirements

import (
	"fmt"
	"strings"
	"testing"
)

func TestConsolidationPreviewsThenRemapsSevenExactItemLinks(t *testing.T) {
	root := newSaga(t)
	duplicate := storyInput("duplicate", "r1", "proposed", []Criterion{{ID: "works", Statement: "The flow works"}})
	duplicate.Title, duplicate.Statement = "Shared checkout", "As a buyer I complete checkout"
	canonical := storyInput("canonical", "r1", "proposed", []Criterion{{ID: "completes", Statement: "The flow completes"}})
	canonical.Title, canonical.Statement = "Shared checkout", "As a buyer I complete checkout"
	if _, err := AddStory(root, "test", duplicate); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", canonical); err != nil {
		t.Fatal(err)
	}
	duplicateURN := "urn:change-saga:test:story:duplicate"
	canonicalURN := "urn:change-saga:test:story:canonical"
	duplicateCriterion := duplicateURN + ":criterion:works"
	canonicalCriterion := canonicalURN + ":criterion:completes"
	for index := 1; index <= 7; index++ {
		_, err := AddRelation(root, "test", AddRelationInput{
			Feature: "core", ID: fmt.Sprintf("item-%d-explains-duplicate", index), Type: RelationExplains,
			From: fmt.Sprintf("urn:change-saga:test:slide:flow:item:item-%d", index), To: duplicateCriterion,
			ToRevision: duplicateURN + ":revision:r1", Rationale: "This exact diagram item explains the criterion.",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	input := ConsolidateInput{
		Duplicate: duplicateURN, Canonical: canonicalURN, EventID: "consolidated",
		Parents: []string{duplicateURN + ":event:proposed"}, Reason: "Both proposals describe the same confirmed checkout intent",
		CriterionMap: map[string]string{duplicateCriterion: canonicalCriterion},
	}
	preview, err := PreviewConsolidation(root, "test", input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || preview.LifecycleState != StateRejected || len(preview.RelationRemaps) != 7 {
		t.Fatalf("preview = %#v", preview)
	}
	before, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Relations) != 7 || len(before.FindStory("duplicate").Events) != 1 {
		t.Fatal("preview changed the Saga")
	}
	applied, err := ConsolidateProposal(root, "test", input)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || !applied.PreservesIntent || len(applied.RelationRemaps) != 7 {
		t.Fatalf("applied = %#v", applied)
	}
	after, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	story := after.FindStory("duplicate")
	if story.CurrentLifecycle == nil || story.CurrentLifecycle.State != StateRejected || len(story.Events) != 2 {
		t.Fatalf("duplicate lifecycle = %#v, history %d", story.CurrentLifecycle, len(story.Events))
	}
	active, superseded := 0, 0
	for _, relation := range after.Relations {
		switch relation.State {
		case RelationActive:
			active++
			if relation.To != canonicalCriterion || relation.ToRevision != canonicalURN+":revision:r1" || !strings.Contains(relation.From, ":item:item-") {
				t.Fatalf("unsafe replacement = %#v", relation)
			}
		case RelationSuperseded:
			superseded++
			if relation.To != duplicateCriterion {
				t.Fatalf("historical link was rewritten = %#v", relation)
			}
		}
	}
	if active != 7 || superseded != 7 {
		t.Fatalf("active/superseded = %d/%d", active, superseded)
	}
}

func TestConsolidationRefusesAmbiguousMappingAndAcceptedIntentLoss(t *testing.T) {
	root := newSaga(t)
	duplicate := storyInput("duplicate", "r1", "proposed", []Criterion{{ID: "one", Statement: "One"}, {ID: "two", Statement: "Two"}})
	canonical := storyInput("canonical", "r1", "proposed", []Criterion{{ID: "kept", Statement: "Kept"}})
	if _, err := AddStory(root, "test", duplicate); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", canonical); err != nil {
		t.Fatal(err)
	}
	duplicateURN := "urn:change-saga:test:story:duplicate"
	canonicalURN := "urn:change-saga:test:story:canonical"
	parent := duplicateURN + ":event:proposed"
	target := canonicalURN + ":criterion:kept"
	input := ConsolidateInput{Duplicate: duplicateURN, Canonical: canonicalURN, EventID: "consolidated", Parents: []string{parent}, Reason: "duplicate", CriterionMap: map[string]string{
		duplicateURN + ":criterion:one": target,
		duplicateURN + ":criterion:two": target,
	}}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous map error = %v", err)
	}
	if _, err := SetStoryState(root, "test", SetStoryStateInput{Story: duplicateURN, ID: "accepted", Parents: []string{parent}, State: StateAccepted}); err != nil {
		t.Fatal(err)
	}
	input.Parents = []string{duplicateURN + ":event:accepted"}
	input.CriterionMap = map[string]string{duplicateURN + ":criterion:one": target, duplicateURN + ":criterion:two": canonicalURN + ":criterion:missing"}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "accepted duplicate intent") {
		t.Fatalf("accepted-intent refusal = %v", err)
	}
	after, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if after.FindStory("duplicate").CurrentLifecycle.State != StateAccepted || len(after.FindStory("duplicate").Events) != 2 {
		t.Fatal("failed consolidation changed accepted lifecycle")
	}
}
