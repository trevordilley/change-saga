package requirements

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestDeprecationSchemasLifecycleAndTechnicalPins(t *testing.T) {
	root := newSaga(t)
	record := rawTechnical(t, root, "component", "old", TechnicalDefinition{Name: "Old", Explanation: "Legacy worker", Code: []Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("a", 40), Path: "old.go", Start: 1, End: 1, Note: "Defines the legacy worker.", Digest: "sha256:" + strings.Repeat("b", 64)}}}})
	targets := DeprecationTargets{Root: root, SagaID: "test"}
	pin, err := targets.Pin(record, "")
	if err != nil {
		t.Fatal(err)
	}
	input := AddDeprecationInput{ID: "legacy", RevisionID: "r1", EventID: "active", DeprecationDefinition: DeprecationDefinition{Reason: "Use the successor", Scope: "content", Target: pin}, CreatedAt: testTime}
	if _, err := AddDeprecation(root, "test", input); err != nil {
		t.Fatal(err)
	}
	urn, _ := DeprecationURN("test", "legacy")
	if _, err := SetDeprecationState(root, "test", SetDeprecationStateInput{Deprecation: urn, ID: "withdraw", Parents: []string{urn + ":event:active"}, State: DeprecationWithdrawn, Reason: "Mistaken scope", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetDeprecationState(root, "test", SetDeprecationStateInput{Deprecation: urn, ID: "restore", Parents: []string{urn + ":event:withdraw"}, State: DeprecationActive, Reason: "Confirmed with the owner", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	input.DeprecationDefinition.Scope = "code"
	input.DeprecationDefinition.Code = []coderef.Reference{{Commit: strings.Repeat("a", 40), Path: "src/old.go", Start: 2, End: 4, Digest: "sha256:" + strings.Repeat("b", 64)}}
	if _, err := ReviseDeprecation(root, "test", ReviseDeprecationInput{Deprecation: urn, ID: "r2", Parents: []string{urn + ":revision:r1"}, DeprecationDefinition: input.DeprecationDefinition, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddStory(root, "test", storyInput("context", "r1", "proposed", nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := AddRelation(root, "test", AddRelationInput{Feature: "core", ID: "context", Type: RelationExplains, From: urn, To: "urn:change-saga:test:story:context", FromRevision: urn + ":revision:r2", ToRevision: "urn:change-saga:test:story:context:revision:r1", Rationale: "Migration context", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	schemaRoot, err := filepath.Abs("../../schema")
	if err != nil {
		t.Fatal(err)
	}
	checked := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var record map[string]any
		if err = json.Unmarshal(data, &record); err != nil {
			return err
		}
		schemaURL, _ := record["$schema"].(string)
		if !strings.Contains(schemaURL, "deprecation") && !strings.HasSuffix(schemaURL, "v5/relation.schema.json") {
			return nil
		}
		name := strings.TrimPrefix(schemaURL, "https://changesaga.dev/schema/")
		schema, err := compiler.Compile(filepath.Join(schemaRoot, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			return err
		}
		if err := schema.Validate(value); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		checked[name] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checked) != 4 {
		t.Fatalf("missing schema coverage: %v", checked)
	}
	// A concurrent withdrawal is kept as a conflict, even when both branches
	// agree on the state. Resolving it explicitly must name every event head.
	doc, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	event := doc.FindDeprecation("legacy").Events[0]
	event.ID = "parallel"
	event.Parents = []string{urn + ":event:withdraw"}
	event.State = DeprecationActive
	event.Reason = "Parallel restoration"
	if err := store.WriteJSON(filepath.Join(root, DeprecationPackagePath("legacy"), "events/parallel.json"), event, true); err != nil {
		t.Fatal(err)
	}
	doc, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if doc.FindDeprecation("legacy").CurrentLifecycle != nil {
		t.Fatal("chose conflicting lifecycle head")
	}
	if _, err := SetDeprecationState(root, "test", SetDeprecationStateInput{Deprecation: urn, ID: "resolve", Parents: doc.FindDeprecation("legacy").LifecycleHeads, State: DeprecationWithdrawn, Reason: "Resolve both branches", CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
}

func TestDeprecationRejectsGraphAndStorageCorruption(t *testing.T) {
	for _, test := range []string{"cycle", "missing-parent", "unknown-field", "symlink"} {
		t.Run(test, func(t *testing.T) {
			root := newSaga(t)
			doc := diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "a", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 100, Style: "normal", Label: "A"}}}
			if _, err := PublishWhiteboard(context.Background(), root, "test", "board", "b1", "", "Board", doc); err != nil {
				t.Fatal(err)
			}
			targets := DeprecationTargets{Root: root, SagaID: "test"}
			pin, err := targets.Pin("urn:change-saga:test:whiteboard:board", "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := AddDeprecation(root, "test", AddDeprecationInput{ID: "a", RevisionID: "r1", EventID: "active", DeprecationDefinition: DeprecationDefinition{Reason: "Obsolete", Scope: "content", Target: pin}, CreatedAt: testTime}); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, DeprecationPackagePath("a"), "revisions/r1.json")
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			json.Unmarshal(bytes, &value)
			switch test {
			case "cycle":
				value["parents"] = []string{"urn:change-saga:test:deprecation:a:revision:r1"}
			case "missing-parent":
				value["parents"] = []string{"urn:change-saga:test:deprecation:a:revision:absent"}
			case "unknown-field":
				value["deprecated"] = true
			case "symlink":
				external := filepath.Join(t.TempDir(), "revision.json")
				os.WriteFile(external, bytes, 0600)
				os.Remove(path)
				if err := os.Symlink(external, path); err != nil {
					t.Fatal(err)
				}
			}
			if test != "symlink" {
				bytes, _ = json.Marshal(value)
				os.WriteFile(path, bytes, 0600)
			}
			if _, err := Load(root, "test"); err == nil {
				t.Fatalf("accepted %s", test)
			}
		})
	}
}
