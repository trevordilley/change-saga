package requirements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/currentrecord"
)

func TestInventoryCurrentSchemasAndLegacyGraphReader(t *testing.T) {
	root := newSaga(t)
	def := TechnicalDefinition{Name: "FlagStore", Explanation: "Owns flag values.", Code: []Evidence{{Reference: coderef.Reference{Commit: strings.Repeat("c", 40), Path: "flags.go", Start: 2, End: 3, Digest: "sha256:" + strings.Repeat("d", 64), Note: "The exact store implementation."}}}}
	pins := map[string]string{}
	for _, id := range []string{"store", "client"} {
		result, err := WriteTechnical(root, "test", "component", id, "", nil, def, true)
		if err != nil {
			t.Fatal(err)
		}
		pins[id] = result.ContentDigest
	}
	target := "urn:change-saga:test:component:store"
	original := def
	def.Explanation = "Owns the current flag values."
	updated, err := WriteTechnical(root, "test", "component", "store", "", nil, def, false)
	if err != nil {
		t.Fatal(err)
	}
	pins["store"] = updated.ContentDigest
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	// Legacy graph decoding remains covered without asking current writers
	// to manufacture a second content history.
	legacy := *d.Find(target)
	legacy.current = false
	first := TechnicalRevision{Schema: TechnicalSchema("component", "-revision"), Version: 5, ID: "r1", Record: target, Parents: []string{}, TechnicalDefinition: original, CreatedAt: testTime}
	second := first
	second.ID = "r2"
	second.Parents = []string{target + ":revision:r1"}
	second.TechnicalDefinition = def
	fork := second
	fork.ID = "fork"
	fork.Explanation = "Independently authored alternative."
	legacy.Revisions = []TechnicalRevision{first, second, fork}
	if err := resolveTechnical(&legacy, "test"); err != nil {
		t.Fatal(err)
	}
	copyInventory := d
	copyInventory.Records = append([]TechnicalRecord(nil), d.Records...)
	*copyInventory.Find(target) = legacy
	if legacy.CurrentRevision != nil || len(legacy.RevisionHeads) != 2 || copyInventory.LinkStatus(DocumentationLink{Target: target, Revision: target + ":revision:r1"}) != "conflicted" {
		t.Fatal("competing legacy heads were hidden")
	}
	resolved := fork
	resolved.ID = "merged"
	resolved.Parents = append([]string{}, legacy.RevisionHeads...)
	legacy.Revisions = append(legacy.Revisions, resolved)
	if err := resolveTechnical(&legacy, "test"); err != nil || legacy.CurrentRevision == nil || legacy.CurrentRevision.ID != "merged" {
		t.Fatalf("legacy multi-parent reader: %v", err)
	}

	system := def
	system.Name = "FeatureFlag"
	system.Components = []DocumentationLink{{Target: target, Revision: pins["store"]}, {Target: "urn:change-saga:test:component:client", Revision: pins["client"]}}
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
	// ERD revisions now reference the shared diagram source schema. Register it
	// locally so schema validation remains offline, including legacy revisions.
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "v5", "diagram.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	resource, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource("https://changesaga.dev/schema/v5/diagram.schema.json", resource); err != nil {
		t.Fatal(err)
	}
	for _, record := range d.Records {
		values := []any{record.Identity}
		if record.current {
			current, err := currentrecord.Read(filepath.Join(record.root, filepath.FromSlash(TechnicalPath(record.Kind, record.Identity.ID))))
			if err != nil {
				t.Fatal(err)
			}
			if current == nil {
				t.Fatal("current package has no atomic publication")
			}
			values = append(values, current)
		} else {
			type legacyRevision TechnicalRevision
			type legacyEvent TechnicalEvent
			for _, value := range record.Revisions {
				values = append(values, legacyRevision(value))
			}
			for _, value := range record.Events {
				values = append(values, legacyEvent(value))
			}
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
