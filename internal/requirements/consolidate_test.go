package requirements

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestConsolidationRequiresCurrentExternalPinsAndWritesFreshPins(t *testing.T) {
	root, duplicateURN, canonicalURN, duplicateCriterion, canonicalCriterion := consolidationFixture(t)
	item := "urn:change-saga:test:slide:flow:item:validator"
	digest := "sha256:" + strings.Repeat("a", 64)
	if _, err := AddRelation(root, "test", AddRelationInput{
		Feature: "core", ID: "validator-addresses-duplicate", Type: RelationAddresses,
		From: item, To: duplicateCriterion, FromContentDigest: digest,
		ToRevision: duplicateURN + ":revision:r1", Rationale: "The current Item addresses this exact criterion.",
	}); err != nil {
		t.Fatal(err)
	}
	input := ConsolidateInput{
		Duplicate: duplicateURN, Canonical: canonicalURN, EventID: "consolidated", Parents: []string{duplicateURN + ":event:proposed"}, Reason: "duplicate",
		CriterionMap: map[string]string{duplicateCriterion: canonicalCriterion},
	}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "no supplied current value") {
		t.Fatalf("missing digest refusal = %v", err)
	}
	input.CurrencyInputs.CurrentContentDigests = map[string]string{item: "sha256:" + strings.Repeat("b", 64)}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "relation is stale") {
		t.Fatalf("stale digest refusal = %v", err)
	}
	input.CurrencyInputs.CurrentContentDigests[item] = digest
	if _, err := ConsolidateProposal(root, "test", input); err != nil {
		t.Fatal(err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, relation := range document.Relations {
		if relation.State == RelationActive {
			if relation.To != canonicalCriterion || relation.ToRevision != canonicalURN+":revision:r1" || relation.FromContentDigest != digest {
				t.Fatalf("replacement did not carry verified current pins: %#v", relation)
			}
		}
	}
}

func TestConsolidationRefusesConflictedAffectedRelation(t *testing.T) {
	root, duplicateURN, canonicalURN, duplicateCriterion, canonicalCriterion := consolidationFixture(t)
	workItem := "urn:change-saga:test:work-item:implement-checkout"
	workRevision := workItem + ":revision:r1"
	if _, err := AddRelation(root, "test", AddRelationInput{
		Feature: "core", ID: "work-implements-duplicate", Type: RelationImplements,
		From: workItem, To: duplicateCriterion, FromRevision: workRevision,
		ToRevision: duplicateURN + ":revision:r1", Rationale: "The work item implements the criterion.",
	}); err != nil {
		t.Fatal(err)
	}
	input := ConsolidateInput{
		Duplicate: duplicateURN, Canonical: canonicalURN, EventID: "consolidated", Parents: []string{duplicateURN + ":event:proposed"}, Reason: "duplicate",
		CriterionMap:   map[string]string{duplicateCriterion: canonicalCriterion},
		CurrencyInputs: StaleInputs{ConflictedRevisions: map[string][]string{workItem: {workRevision, workItem + ":revision:r2"}}},
	}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "relation is conflicted") {
		t.Fatalf("conflicted relation refusal = %v", err)
	}
}

func TestConsolidationRefusesTransactionOwnedCriterionLinks(t *testing.T) {
	root, duplicateURN, canonicalURN, duplicateCriterion, canonicalCriterion := consolidationFixture(t)
	slides := filepath.Join(root, "___features", "core.feature", "___slides")
	if err := os.MkdirAll(slides, 0o755); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"current":"sha256:current","revisions":[{"snapshot":"sha256:current","items":[{"criterion_links":[{"id":"validator-link","criterion":%q,"story_revision":%q,"rationale":"Exact link"}]}]}]}`,
		duplicateCriterion, duplicateURN+":revision:r1")
	path := filepath.Join(slides, "25-t-aaaaaaaaaaaa-bbbbbbbbbbbb.json")
	if err := os.WriteFile(path, []byte(record), 0o644); err != nil {
		t.Fatal(err)
	}
	input := ConsolidateInput{
		Duplicate: duplicateURN, Canonical: canonicalURN, EventID: "consolidated", Parents: []string{duplicateURN + ":event:proposed"}, Reason: "duplicate",
		CriterionMap: map[string]string{duplicateCriterion: canonicalCriterion},
	}
	if _, err := PreviewConsolidation(root, "test", input); err == nil || !strings.Contains(err.Error(), "partial graph retirement") || !strings.Contains(err.Error(), "validator-link") || !strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("transaction-owned link refusal = %v", err)
	}
}

func consolidationFixture(t *testing.T) (root, duplicateURN, canonicalURN, duplicateCriterion, canonicalCriterion string) {
	t.Helper()
	root = newSaga(t)
	if _, err := AddStory(root, "test", storyInput("duplicate", "r1", "proposed", []Criterion{{ID: "works", Statement: "The flow works"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", storyInput("canonical", "r1", "proposed", []Criterion{{ID: "completes", Statement: "The flow completes"}})); err != nil {
		t.Fatal(err)
	}
	duplicateURN = "urn:change-saga:test:story:duplicate"
	canonicalURN = "urn:change-saga:test:story:canonical"
	duplicateCriterion = duplicateURN + ":criterion:works"
	canonicalCriterion = canonicalURN + ":criterion:completes"
	return
}
