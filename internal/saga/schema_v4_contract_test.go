package saga

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadV4Schema(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../schema/v4", name))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

// The v4 deck, slide, and Item record schemas still describe the records of
// every embedded deck under ___slides/.
func TestV4DeckRecordSchemasAreClosedAndVersioned(t *testing.T) {
	for _, name := range []string{"deck.schema.json", "slide.schema.json", "item.schema.json"} {
		t.Run(name, func(t *testing.T) {
			schema := loadV4Schema(t, name)
			if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["additionalProperties"] != false {
				t.Fatalf("v4 schema is not closed draft 2020-12: %#v", schema)
			}
			if schema["$id"] != "https://changesaga.dev/schema/v4/"+name {
				t.Fatalf("v4 %s $id = %v", name, schema["$id"])
			}
		})
	}
}

func TestV4ItemSchemaMakesCalloutAnEvidenceCapableItemKind(t *testing.T) {
	schema := loadV4Schema(t, "item.schema.json")
	kinds := dig(t, schema, "properties", "kind", "enum").([]any)
	found := false
	for _, kind := range kinds {
		found = found || kind == "callout"
	}
	if !found {
		t.Fatal("callout is not an Item kind")
	}
	if _, ok := schema["properties"].(map[string]any)["diffs"]; ok {
		t.Fatal("evidence belongs in independent flat 40-e records, not item.json")
	}
	if _, ok := schema["properties"].(map[string]any)["rank"]; !ok {
		t.Fatal("Items require a compact sortable rank")
	}
	if _, ok := schema["properties"].(map[string]any)["slide"]; !ok {
		t.Fatal("flat Items must name their semantic parent slide")
	}
	if _, ok := loadV4Schema(t, "slide.schema.json")["properties"].(map[string]any)["deck"]; !ok {
		t.Fatal("flat slides must name their semantic parent deck")
	}
}
