package requirements

import (
	"github.com/twentyideas/changesaga/internal/coderef"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentStoryEditsWithoutRevisionIDs(t *testing.T) {
	root := newSaga(t)
	input := storyInput("editable", "", "", nil)
	if _, err := AddStory(root, "test", input); err != nil {
		t.Fatal(err)
	}
	storyURN := "urn:change-saga:test:story:editable"
	for _, title := range []string{"First edit", "Second edit"} {
		if _, err := ReviseStory(root, "test", ReviseStoryInput{Story: storyURN, Title: title, Statement: "As a buyer I can edit documentation", AcceptanceCriteria: input.AcceptanceCriteria}); err != nil {
			t.Fatal(err)
		}
	}
	doc, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	story := doc.FindStory("editable")
	if story.CurrentRevision.Title != "Second edit" || len(story.Revisions) != 1 {
		t.Fatalf("current story: %+v", story)
	}
	dir := filepath.Join(root, filepath.FromSlash(storyPackagePath("core", "editable")))
	data, err := os.ReadFile(filepath.Join(dir, "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{`"parents"`, `"created_at"`, `"request_id"`, `"revisions"`} {
		if strings.Contains(string(data), old) {
			t.Errorf("current content retained %s", old)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "revisions")); !os.IsNotExist(err) {
		t.Fatalf("history retained: %v", err)
	}
}

func TestCurrentAppStateAndStableElementIDs(t *testing.T) {
	root := newSaga(t)
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "switch", Description: "Gate the feature", Targets: []string{"urn:change-saga:test:feature:core"}, State: FlagOff}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetFlagState(root, "test", SetFlagStateInput{Flag: "urn:change-saga:test:flag:switch", State: FlagOn, Reason: "Ready"}); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	flag := doc.FindFlag("switch")
	if flag.CurrentLifecycle.State != FlagOn || flag.CurrentLifecycle.Reason != "Ready" {
		t.Fatalf("flag: %+v", flag)
	}
	if _, err := RevisePersona(root, "test", RevisePersonaInput{Persona: "urn:change-saga:test:persona:buyer", Name: "Buyer", Description: "Current description"}); err != nil {
		t.Fatal(err)
	}
	doc, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if doc.FindPersona("buyer").CurrentRevision.Description != "Current description" {
		t.Fatal("persona edit lost")
	}
}

func TestCurrentStateReasonCanBeEditedAndRetried(t *testing.T) {
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("clarified", "", "", nil)); err != nil {
		t.Fatal(err)
	}
	input := SetStoryStateInput{Story: "urn:change-saga:test:story:clarified", State: StateProposed, Reason: "Clarified current intent"}
	if _, err := SetStoryState(root, "test", input); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(storyPackagePath("core", "clarified")), "current.json")
	before, _ := os.ReadFile(path)
	if _, err := SetStoryState(root, "test", input); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("same-value state retry changed the publication")
	}
}

func TestCurrentDeprecationDigestPinCurrency(t *testing.T) {
	root := newSaga(t)
	definition := TechnicalDefinition{Name: "Store", Explanation: "Current store", Code: []Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("a", 40), Path: "source.go", Start: 1, End: 1, Digest: "sha256:" + strings.Repeat("a", 64), Note: "Exact declaration"}}}}
	first, err := WriteTechnical(root, "test", "component", "store", "", nil, definition, true)
	if err != nil {
		t.Fatal(err)
	}
	pin := DeprecationTarget{Target: first.URN, ContentDigest: first.ContentDigest}
	observer := DeprecationTargets{Root: root, SagaID: "test"}
	if health := observer.Observe(pin); health.Status != "current" {
		t.Fatalf("exact content digest marked noncurrent: %+v", health)
	}
	definition.Explanation = "Edited store"
	second, err := WriteTechnical(root, "test", "component", "store", "", nil, definition, false)
	if err != nil {
		t.Fatal(err)
	}
	observer = DeprecationTargets{Root: root, SagaID: "test"}
	health := observer.Observe(pin)
	if health.Status != "stale" || health.Current == nil || health.Current.ContentDigest != second.ContentDigest {
		t.Fatalf("old assertion silently repinned: %+v", health)
	}
}

func TestCurrentCreationRetriesCompareAuthoredValues(t *testing.T) {
	root := newSaga(t)
	cases := []struct {
		name   string
		create func() (MutationResult, error)
		change func() (MutationResult, error)
	}{
		{"story", func() (MutationResult, error) { return AddStory(root, "test", storyInput("retry-story", "", "", nil)) }, func() (MutationResult, error) {
			input := storyInput("retry-story", "", "", nil)
			input.Title = "Different"
			return AddStory(root, "test", input)
		}},
		{"persona", func() (MutationResult, error) {
			return AddPersona(root, "test", AddPersonaInput{ID: "retry-persona", Name: "Reader", Description: "Reads documentation"})
		}, func() (MutationResult, error) {
			return AddPersona(root, "test", AddPersonaInput{ID: "retry-persona", Name: "Writer", Description: "Writes documentation"})
		}},
		{"flag", func() (MutationResult, error) {
			return AddFlag(root, "test", AddFlagInput{ID: "retry-flag", Description: "Gate current intent", Targets: []string{"urn:change-saga:test:feature:core"}, State: FlagOff})
		}, func() (MutationResult, error) {
			return AddFlag(root, "test", AddFlagInput{ID: "retry-flag", Description: "Changed gate", Targets: []string{"urn:change-saga:test:feature:core"}, State: FlagOff})
		}},
		{"term", func() (MutationResult, error) {
			return AddTerm(root, "test", AddTermInput{ID: "retry-term", TermDefinition: TermDefinition{Name: "Record", Definition: "A saved documentation record"}})
		}, func() (MutationResult, error) {
			return AddTerm(root, "test", AddTermInput{ID: "retry-term", TermDefinition: TermDefinition{Name: "Record", Definition: "A changed definition"}})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := tc.create()
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(first.Path), "current.json")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := tc.create()
			if err != nil || !replay.Replayed {
				t.Fatalf("value retry: %+v %v", replay, err)
			}
			if _, err := tc.change(); err == nil {
				t.Fatal("creation replaced a stable identity with different content")
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatal("retry or refused creation changed current publication")
			}
		})
	}
}

func TestCurrentTechnicalStateNeedsNoInitialEventGraph(t *testing.T) {
	root := newSaga(t)
	definition := TechnicalDefinition{Name: "Store", Explanation: "Current store", Code: []Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("a", 40), Path: "source.go", Start: 1, End: 1, Digest: "sha256:" + strings.Repeat("a", 64), Note: "Exact declaration"}}}}
	created, err := WriteTechnical(root, "test", "component", "store", "", nil, definition, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"Replaced", "Clarified replacement", "Clarified replacement"} {
		if _, err := SetTechnicalState(root, "test", "component", "store", "", "retired", reason, nil); err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	record := inventory.Find(created.URN)
	if record.CurrentLifecycle.State != "retired" || record.CurrentLifecycle.Reason != "Clarified replacement" || len(record.Events) != 1 {
		t.Fatalf("current state: %+v", record)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(TechnicalPath("component", "store")), "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"parents"`) || strings.Contains(string(data), `"created_at"`) {
		t.Fatal("state edit recreated graph metadata")
	}
}
