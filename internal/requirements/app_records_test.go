package requirements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
)

func addSecondFeature(t *testing.T, root string) {
	t.Helper()
	if _, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: "billing", Title: "Billing", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
}

func TestStoryIDsAreUniqueAcrossFeatures(t *testing.T) {
	root := newSaga(t)
	addSecondFeature(t, root)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	input := storyInput("checkout", "r1", "proposed", nil)
	input.Feature = "billing"
	if _, err := AddStory(root, "test", input); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second feature reused a story id: %v", err)
	}
	// A hand-copied package is one URN naming two records, so loading refuses it.
	from := filepath.Join(root, "___features", "core.feature", "___requirements", "stories", "checkout.story")
	to := filepath.Join(root, "___features", "billing.feature", "___requirements", "stories", "checkout.story")
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "test"); err == nil || !strings.Contains(err.Error(), "unique across the app") {
		t.Fatalf("duplicate story across features loaded: %v", err)
	}
}

func TestMovingAStoryKeepsEveryRelationCurrent(t *testing.T) {
	root := newSaga(t)
	addSecondFeature(t, root)
	if _, err := AddStory(root, "test", storyInput("parent", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", storyInput("child", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRelation(root, "test", AddRelationInput{Feature: "core", ID: "child-refines-parent", Type: RelationRefines,
		From: "urn:change-saga:test:story:child", To: "urn:change-saga:test:story:parent", Rationale: "narrows it",
		FromRevision: "urn:change-saga:test:story:child:revision:r1", ToRevision: "urn:change-saga:test:story:parent:revision:r1", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	result, err := MoveStory(root, "test", MoveStoryInput{Story: "urn:change-saga:test:story:parent", Feature: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "___features/billing.feature/___requirements/stories/parent.story" {
		t.Fatalf("moved path = %s", result.Path)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if story := document.FindStory("parent"); story == nil || story.Feature != "billing" {
		t.Fatalf("story did not move: %+v", story)
	}
	for _, currency := range EvaluateRelations(document, StaleInputs{}) {
		if !currency.Current() {
			t.Fatalf("relation went stale after a move: %+v", currency)
		}
	}
	replay, err := MoveStory(root, "test", MoveStoryInput{Story: "urn:change-saga:test:story:parent", Feature: "billing"})
	if err != nil || !replay.Replayed {
		t.Fatalf("repeat move = %+v, %v", replay, err)
	}
}

// Personas are optional: a first story needs none. Any persona a story does
// name must exist, and a retired persona is never newly assigned.
func TestStoryPersonasAreOptionalButMustExistAndBeActive(t *testing.T) {
	root := newSaga(t)
	input := storyInput("checkout", "r1", "proposed", nil)
	input.Personas = nil
	if _, err := AddStory(root, "test", input); err != nil {
		t.Fatalf("story without personas = %v", err)
	}
	ghost := storyInput("ghost", "r1", "proposed", nil)
	ghost.Personas = []string{"urn:change-saga:test:persona:ghost"}
	if _, err := AddStory(root, "test", ghost); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("story serving a missing persona = %v", err)
	}
	persona := "urn:change-saga:test:persona:buyer"
	if _, err := SetPersonaState(root, "test", SetPersonaStateInput{Persona: persona, ID: "retired", Parents: []string{persona + ":event:active"}, State: PersonaRetired, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	retired := storyInput("refund", "r1", "proposed", nil)
	if _, err := AddStory(root, "test", retired); err == nil || !strings.Contains(err.Error(), "retired") {
		t.Fatalf("story newly serving a retired persona = %v", err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if story := document.FindStory("checkout"); story == nil || len(story.CurrentRevision.Personas) != 0 {
		t.Fatalf("story without personas = %+v", story)
	}
}

func TestPersonaLifecycleAndRevisions(t *testing.T) {
	root := newSaga(t)
	persona := "urn:change-saga:test:persona:buyer"
	if _, err := RevisePersona(root, "test", RevisePersonaInput{Persona: persona, ID: "r2", Parents: []string{persona + ":revision:r1"}, Name: "Buyer", Description: "Buys and returns things", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetPersonaState(root, "test", SetPersonaStateInput{Persona: persona, ID: "retired", Parents: []string{persona + ":event:active"}, State: PersonaRetired, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetPersonaState(root, "test", SetPersonaStateInput{Persona: persona, ID: "again", Parents: []string{persona + ":event:retired"}, State: PersonaRetired, CreatedAt: testTime}); err == nil {
		t.Fatal("repeating a persona state was accepted")
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	found := document.FindPersona("buyer")
	if found.Active() || found.CurrentRevision.ID != "r2" {
		t.Fatalf("persona = %+v", found)
	}
}

func TestFlagsGateStoriesAndFeatures(t *testing.T) {
	root := newSaga(t)
	addSecondFeature(t, root)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	invoice := storyInput("invoice", "r1", "proposed", nil)
	invoice.Feature = "billing"
	if _, err := AddStory(root, "test", invoice); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "new-checkout", RevisionID: "r1", EventID: "off", Description: "New checkout", Targets: []string{"urn:change-saga:test:story:checkout"}, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "billing", RevisionID: "r1", EventID: "on", State: FlagOn, Description: "Billing", Targets: []string{"urn:change-saga:test:feature:billing"}, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "ghost", RevisionID: "r1", EventID: "off", Description: "Ghost", Targets: []string{"urn:change-saga:test:story:ghost"}, CreatedAt: testTime}); err == nil {
		t.Fatal("a flag gating a missing story was accepted")
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	gates := document.Gates()
	if len(gates.Off["checkout"]) != 1 || len(gates.On["invoice"]) != 1 || len(gates.Off["invoice"]) != 0 {
		t.Fatalf("gates = %+v", gates)
	}
	flag := "urn:change-saga:test:flag:new-checkout"
	if _, err := SetFlagState(root, "test", SetFlagStateInput{Flag: flag, ID: "on", Parents: []string{flag + ":event:off"}, State: FlagOn, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	document, _ = Load(root, "test")
	if gates := document.Gates(); len(gates.Off["checkout"]) != 0 {
		t.Fatalf("enabled flag still gates: %+v", gates)
	}
}
