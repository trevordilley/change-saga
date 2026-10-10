package saga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDataModelOverviewSchemaReusesClosedFragmentContract(t *testing.T) {
	t.Parallel()
	compiler := jsonschema.NewCompiler()
	// Register both published IDs so compilation and validation stay offline.
	for _, path := range []string{"v2/fragment.schema.json", "v5/data-model-overview.schema.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "schema", path))
		if err != nil {
			t.Fatal(err)
		}
		schema, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("https://changesaga.dev/schema/"+path, schema); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("https://changesaga.dev/schema/v5/data-model-overview.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, body string
		valid      bool
	}{
		{"Markdown", `{"version":2,"id":"shared-model","title":"Data model overview","media_type":"text/markdown","entrypoint":"content.md","order":30}`, true},
		{"HTML", `{"version":2,"id":"shared-model","media_type":"text/html","entrypoint":"content.html"}`, false},
		{"SVG", `{"version":2,"id":"shared-model","media_type":"image/svg+xml","entrypoint":"model.svg"}`, false},
		{"traversal", `{"version":2,"id":"shared-model","media_type":"text/markdown","entrypoint":"../content.md"}`, false},
		{"unknown field", `{"version":2,"id":"shared-model","media_type":"text/markdown","entrypoint":"content.md","erds":[]}`, false},
		{"wrong package version", `{"version":5,"id":"shared-model","media_type":"text/markdown","entrypoint":"content.md"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			instance, err := jsonschema.UnmarshalJSON(strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(instance); (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
}
