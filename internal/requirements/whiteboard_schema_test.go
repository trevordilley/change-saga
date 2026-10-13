package requirements

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestWhiteboardAndStructuredERDPublishedSchemas(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	board, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b1", "", "Jobs", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"diagram", "whiteboard-revision", "erd-revision", "technical-definition"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "schema", "v5", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("https://changesaga.dev/schema/v5/"+name+".schema.json", value); err != nil {
			t.Fatal(err)
		}
	}
	for name, value := range map[string]any{"whiteboard-revision": board, "erd-revision": inventory.Pinned(pin), "technical-definition": inventory.Pinned(pin).TechnicalDefinition} {
		schema, err := compiler.Compile("https://changesaga.dev/schema/v5/" + name + ".schema.json")
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(value)
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(instance); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}
