package requirements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/coderef"
)

func TestInventoryHistorySchemas(t *testing.T) {
	root := newSaga(t)
	def := TechnicalDefinition{Name: "FlagStore", Explanation: "Owns flag values.", Code: []Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("c", 40), Path: "flags.go", Start: 2, End: 3, Digest: "sha256:" + strings.Repeat("d", 64), Note: "The exact store implementation."}}}}
	for _, id := range []string{"store", "client"} {
		if _, err := WriteTechnical(root, "test", "component", id, "r1", nil, def, true); err != nil {
			t.Fatal(err)
		}
	}
	target := "urn:change-saga:test:component:store"
	if _, err := WriteTechnical(root, "test", "component", "store", "r2", []string{target + ":revision:r1"}, def, false); err != nil {
		t.Fatal(err)
	}
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := d.Find(target)
	fork := r.Revisions[1]
	fork.ID = "fork"
	// A merge can preserve competing immutable revisions; the reader must not
	// choose whichever file happened to be read last.
	r.Revisions = append(r.Revisions, fork)
	if err := resolveTechnical(r, "test"); err != nil {
		t.Fatal(err)
	}
	if r.CurrentRevision != nil || len(r.RevisionHeads) != 2 || d.LinkStatus(DocumentationLink{Target: target, Revision: target + ":revision:r1"}) != "conflicted" {
		t.Fatal("competing heads were hidden")
	}
	reconciled := fork
	reconciled.ID = "merged"
	reconciled.Parents = append([]string{}, r.RevisionHeads...)
	r.Revisions = append(r.Revisions, reconciled)
	if err := resolveTechnical(r, "test"); err != nil || r.CurrentRevision == nil || r.CurrentRevision.ID != "merged" {
		t.Fatalf("reconciliation: %v", err)
	}
	system := def
	system.Name = "FeatureFlag"
	system.Components = []DocumentationLink{{Target: target, Revision: target + ":revision:r2"}, {Target: "urn:change-saga:test:component:client", Revision: "urn:change-saga:test:component:client:revision:r1"}}
	system.Interactions = []Interaction{{ID: "read", From: system.Components[1].Target, To: target, Description: "Read the flag store.", Code: def.Code}}
	if _, err := WriteTechnical(root, "test", "system", "feature-flag", "r1", nil, system, true); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTechnicalState(root, "test", "system", "feature-flag", "retired", "retired", "Replaced", []string{"urn:change-saga:test:system:feature-flag:event:active"}); err != nil {
		t.Fatal(err)
	}
	d, err = LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	validateInventorySchemas(t, d)
	// Malformed dependencies remain visible as missing instead of synthesizing
	// an identity or widening evidence to cover them.
	missing := DocumentationLink{Target: "urn:change-saga:test:component:missing", Revision: "urn:change-saga:test:component:missing:revision:r1"}
	if d.LinkStatus(missing) != "missing" {
		t.Fatal("missing link not reported")
	}
	def.Code[0].Start = 0
	def.Code[0].End = 0
	if _, err := WriteTechnical(root, "test", "component", "wide", "r1", nil, def, true); err == nil {
		t.Fatal("whole file evidence accepted")
	}
	if _, err := os.Stat(filepath.Join(root, TechnicalPath("component", "wide"))); !os.IsNotExist(err) {
		t.Fatal("invalid publication left metadata")
	}
}

func TestInventoryRejectsAmbiguousJSON(t *testing.T) {
	for _, data := range []string{`{"name":"A","name":"B"}`, `{"name":"A"} {}`, `{"name":"A","surprise":true}`} {
		var value TechnicalDefinition
		if err := DecodeInventoryJSON([]byte(data), &value); err == nil {
			t.Fatalf("ambiguous or unknown JSON accepted: %s", data)
		}
	}
}

// validateInventorySchemas checks every loaded record against its published schema.
func validateInventorySchemas(t *testing.T, d Inventory) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, record := range d.Records {
		values := []any{record.Identity}
		for _, v := range record.Revisions {
			values = append(values, v)
		}
		for _, v := range record.Events {
			values = append(values, v)
		}
		for _, value := range values {
			data, _ := json.Marshal(value)
			var obj map[string]any
			if err := json.Unmarshal(data, &obj); err != nil {
				t.Fatal(err)
			}
			name := strings.TrimPrefix(obj["$schema"].(string), "https://changesaga.dev/schema/")
			schema, err := compiler.Compile(filepath.Join("..", "..", "schema", filepath.FromSlash(name)))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(obj); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
}
