package requirements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
)

func addSecondEpic(t *testing.T, root string) {
	t.Helper()
	if _, err := applayout.WriteEpic(root, applayout.EpicManifest{ID: "billing", Title: "Billing", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
}

func TestStoryIDsAreUniqueAcrossEpics(t *testing.T) {
	root := newSaga(t)
	addSecondEpic(t, root)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	input := storyInput("checkout", "r1", "proposed", nil)
	input.Epic = "billing"
	if _, err := AddStory(root, "test", input); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second epic reused a story id: %v", err)
	}
	// A hand-copied package is one URN naming two records, so loading refuses it.
	from := filepath.Join(root, "___epics", "core.epic", "___requirements", "stories", "checkout.story")
	to := filepath.Join(root, "___epics", "billing.epic", "___requirements", "stories", "checkout.story")
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "test"); err == nil || !strings.Contains(err.Error(), "unique across the app") {
		t.Fatalf("duplicate story across epics loaded: %v", err)
	}
}

func TestMovingAStoryKeepsEveryRelationCurrent(t *testing.T) {
	root := newSaga(t)
	addSecondEpic(t, root)
	if _, err := AddStory(root, "test", storyInput("parent", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", storyInput("child", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRelation(root, "test", AddRelationInput{Epic: "core", ID: "child-refines-parent", Type: RelationRefines,
		From: "urn:change-saga:test:story:child", To: "urn:change-saga:test:story:parent", Rationale: "narrows it",
		FromRevision: "urn:change-saga:test:story:child:revision:r1", ToRevision: "urn:change-saga:test:story:parent:revision:r1", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	result, err := MoveStory(root, "test", MoveStoryInput{Story: "urn:change-saga:test:story:parent", Epic: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "___epics/billing.epic/___requirements/stories/parent.story" {
		t.Fatalf("moved path = %s", result.Path)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if story := document.FindStory("parent"); story == nil || story.Epic != "billing" {
		t.Fatalf("story did not move: %+v", story)
	}
	for _, currency := range EvaluateRelations(document, StaleInputs{}) {
		if !currency.Current() {
			t.Fatalf("relation went stale after a move: %+v", currency)
		}
	}
	replay, err := MoveStory(root, "test", MoveStoryInput{Story: "urn:change-saga:test:story:parent", Epic: "billing"})
	if err != nil || !replay.Replayed {
		t.Fatalf("repeat move = %+v, %v", replay, err)
	}
}

func TestStoryRevisionMustServeAnExistingPersona(t *testing.T) {
	root := newSaga(t)
	input := storyInput("checkout", "r1", "proposed", nil)
	input.Personas = nil
	if _, err := AddStory(root, "test", input); err == nil || !strings.Contains(err.Error(), "at least one persona") {
		t.Fatalf("story without personas = %v", err)
	}
	input.Personas = []string{"urn:change-saga:test:persona:ghost"}
	if _, err := AddStory(root, "test", input); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("story serving a missing persona = %v", err)
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

func TestFlagsGateStoriesAndEpics(t *testing.T) {
	root := newSaga(t)
	addSecondEpic(t, root)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	invoice := storyInput("invoice", "r1", "proposed", nil)
	invoice.Epic = "billing"
	if _, err := AddStory(root, "test", invoice); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "new-checkout", RevisionID: "r1", EventID: "off", Description: "New checkout", Targets: []string{"urn:change-saga:test:story:checkout"}, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "billing", RevisionID: "r1", EventID: "on", State: FlagOn, Description: "Billing", Targets: []string{"urn:change-saga:test:epic:billing"}, CreatedAt: testTime}); err != nil {
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
