package saga

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDeckOverviewAndFrontPublishedSchemas(t *testing.T) {
	for _, test := range []struct{ path, body string }{
		{"v4/deck.schema.json", `{"version":4,"id":"main","title":"Main","role":"change","rank":0,"objective":"Explain","overview":{"body":"[Flow](annotation:a)","annotations":[{"id":"a","label":"Flow","slide":"flow","item":"node"}]}}`},
		{"v4/slide.schema.json", `{"version":4,"id":"flow","deck":"main","title":"Flow","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":"slide.svg","takeaway":"Summary","front":["Authored"],"reading_order":[]}`},
		{"v5/slide-transaction-request.schema.json", `{"version":1,"operation":"create","request_id":"request","review":"pr-1","expected_snapshot":"absent","slide":{"id":"flow","title":"Flow","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","takeaway":"Summary","front":["Authored"],"reading_order":["node"]},"asset":{"path":"slide.svg"},"items":[{"id":"node","rank":0,"kind":"node","label":"Node","description":"Explanation","selector":{"type":"element","element_id":"node"},"evidence":[],"criterion_links":[]}]}`},
	} {
		t.Run(test.path, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			diagram, err := os.ReadFile(filepath.Join("..", "..", "schema", "v5", "diagram.schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			resource, err := jsonschema.UnmarshalJSON(strings.NewReader(string(diagram)))
			if err != nil {
				t.Fatal(err)
			}
			if err := compiler.AddResource("https://changesaga.dev/schema/v5/diagram.schema.json", resource); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(filepath.Join("..", "..", "schema", test.path))
			if err != nil {
				t.Fatal(err)
			}
			value, err := jsonschema.UnmarshalJSON(strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); err != nil {
				t.Fatal(err)
			}
			object := value.(map[string]any)
			if strings.Contains(test.path, "deck.schema") {
				overview := object["overview"].(map[string]any)
				overview["code"] = []any{}
				if schema.Validate(value) == nil {
					t.Fatal("overview accepts independent evidence")
				}
				delete(object, "overview")
			} else {
				slide := object
				if transaction, ok := object["slide"]; ok {
					slide = transaction.(map[string]any)
				}
				slide["front"] = []any{" "}
				if schema.Validate(value) == nil {
					t.Fatal("blank front accepted")
				}
				delete(slide, "front")
			}
			if err := schema.Validate(value); err != nil {
				t.Fatalf("legacy content rejected: %v", err)
			}
		})
	}
}
