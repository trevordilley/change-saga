package diagram

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaPath = "../../schema/v5/diagram.schema.json"

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	file, err := os.Open(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	document, err := jsonschema.UnmarshalJSON(file)
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("https://changesaga.dev/schema/v5/diagram.schema.json", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("https://changesaga.dev/schema/v5/diagram.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestSchemaAcceptsStoredSourceAndRejectsUnknownFields(t *testing.T) {
	schema := compileSchema(t)
	data, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	value, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("stored source does not satisfy its schema: %v", err)
	}
	for _, bad := range []string{
		`{"version":1,"width":10,"height":10,"elements":[],"extra":true}`,
		`{"version":1,"width":10,"height":10,"elements":[{"id":"diagram-x","kind":"text","x":0,"y":0,"style":"normal"}]}`,
		`{"version":1,"width":10,"height":10,"styles":{"Bad":{"fill":"none","stroke":"none","ink":"none","stroke_width":1,"font_size":12}},"elements":[]}`,
	} {
		value, _ := jsonschema.UnmarshalJSON(strings.NewReader(bad))
		if schema.Validate(value) == nil {
			t.Errorf("schema accepted %s", bad)
		}
	}
}

func TestSchemaEnumsMatchRuntime(t *testing.T) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs struct {
			Element struct {
				Properties map[string]struct {
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"element"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	check := func(field string, runtime map[string]bool) {
		want := []string{}
		for key := range runtime {
			if key != "" {
				want = append(want, key)
			}
		}
		got := append([]string{}, schema.Defs.Element.Properties[field].Enum...)
		sort.Strings(want)
		sort.Strings(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s enum: schema %v, runtime %v", field, got, want)
		}
	}
	check("kind", elementKinds)
	check("shape", nodeShapes)
	check("align", textAlignment)
}

func TestContractFieldsMatchElement(t *testing.T) {
	want := []string{}
	elementType := reflect.TypeOf(Element{})
	for index := range elementType.NumField() {
		want = append(want, strings.Split(elementType.Field(index).Tag.Get("json"), ",")[0])
	}
	if got := Contract()["fields"].([]string); !reflect.DeepEqual(got, want) {
		t.Fatalf("contract fields %v, element fields %v", got, want)
	}
}
