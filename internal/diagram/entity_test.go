package diagram

import (
	"bytes"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

func TestEntityAndShapeValidationReportsEveryProblem(t *testing.T) {
	d := New()
	d.Elements = []Element{
		{ID: "round", Kind: "node", Shape: "circle", Width: 100, Height: 80, Style: "normal"},
		{ID: "table", Kind: "node", Shape: "entity", Detail: "x", LabelBox: &Box{Width: 1, Height: 1}, Width: 200, Height: 60, Style: "normal",
			Fields: []Field{{Name: "id", Key: "primary"}, {Name: "id"}, {Name: " pad"}, {Name: "note", Type: "two\nlines"}}},
		{ID: "box", Kind: "node", Shape: "rect", Width: 10, Height: 10, Style: "normal", Fields: []Field{{Name: "id"}}},
		{ID: "link", Kind: "edge", From: "box", To: "table", FromField: "id", ToField: "missing", Points: []Point{{0, 0}, {1, 1}}, Style: "normal"},
		{ID: "words", Kind: "text", Width: 10, Height: 10, Style: "normal", ToField: "id"},
	}
	err := d.Validate()
	if err == nil {
		t.Fatal("expected problems")
	}
	for _, want := range []string{
		"round: a circle needs equal width and height",
		"table: an entity needs a label naming it",
		"table: an entity lists fields instead of a detail",
		"table: an entity names itself in its header and takes no label_box",
		"table: field id: key must be pk, fk, or pk,fk",
		"table: field id is repeated",
		"table: field 3 needs a name",
		"table: field note: type must be at most 64 characters on one line",
		"table: entity needs height 155.5 for its header and 4 field rows of 28; it has 60",
		"box: fields are only valid on an entity node",
		`link: from_field needs its endpoint "box" to be an entity`,
		`link: to_field names "missing", which entity table does not list`,
		"words: from_field and to_field are only valid on edges",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestSchemaAcceptsShapesAndEntities(t *testing.T) {
	schema := compileSchema(t)
	data, err := Encode(gallery())
	if err != nil {
		t.Fatal(err)
	}
	value, _ := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("gallery does not satisfy the schema: %v", err)
	}
	for _, bad := range []string{
		`{"version":1,"width":10,"height":10,"elements":[{"id":"t","kind":"node","shape":"entity","x":0,"y":0,"style":"normal","fields":[{"name":"id","key":"primary"}]}]}`,
		`{"version":1,"width":10,"height":10,"elements":[{"id":"t","kind":"node","shape":"entity","x":0,"y":0,"style":"normal","fields":[{"type":"uuid"}]}]}`,
		`{"version":1,"width":10,"height":10,"elements":[{"id":"t","kind":"node","shape":"entity","x":0,"y":0,"style":"normal","fields":[{"name":"id","nullable":true}]}]}`,
	} {
		value, _ := jsonschema.UnmarshalJSON(strings.NewReader(bad))
		if schema.Validate(value) == nil {
			t.Errorf("schema accepted %s", bad)
		}
	}
}
