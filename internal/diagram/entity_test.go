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

func TestEntityEdgesTakeCrowsFeet(t *testing.T) {
	d := gallery()
	places := d.Index("places")
	d.Elements[places].Head, d.Elements[places].Tail = "only-one", "zero-or-many"
	svg, err := Render(d, Options{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), `data-from-field="customer_id" data-to-field="id"`) || !strings.Contains(string(svg), "marker-start") {
		t.Fatal("a field edge must keep its field ends and draw its terminators")
	}
	var b strings.Builder
	Describe(d, 0, 30).WriteText(&b)
	if want := `  places: orders.customer_id -> customers.id "places" tail=zero-or-many head=only-one` + "\n"; !strings.Contains(b.String(), want) {
		t.Fatalf("missing %q in:\n%s", want, b.String())
	}
}

func TestAuthoredEntitySummaryPreservesExactFieldArtwork(t *testing.T) {
	doc := New()
	doc.Elements = []Element{{ID: "record", Kind: "node", Shape: "entity", Label: "Saved record", Width: 500, Height: 220, Style: "normal", Fields: Fields{{Name: "id", Type: "uuid", Key: "pk"}, {Name: "body", Type: "text"}}}}
	attr := func(n *node, name string) string {
		for _, a := range n.attrs {
			if a[0] == name {
				return a[1]
			}
		}
		return ""
	}
	var find func(*node, string, string) *node
	find = func(n *node, name, value string) *node {
		if attr(n, name) == value {
			return n
		}
		for _, c := range n.children {
			if found := find(c, name, value); found != nil {
				return found
			}
		}
		return nil
	}
	original, err := renderTree(doc, Options{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	entity := find(original, "id", "record")
	var expected bytes.Buffer
	for _, child := range entity.children {
		if attr(child, "data-field") != "" || attr(child, "stroke-opacity") == ".35" {
			child.write(&expected, 0)
		}
	}
	doc.Elements[0].Summary = "Preserves the accepted edit for shared access."
	rendered, err := renderTree(doc, Options{}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	detail, summary := find(rendered, "data-diagram-detail", "entity"), find(rendered, "data-diagram-summary", "entity")
	if detail == nil || summary == nil {
		t.Fatal("authored summary and exact field body missing")
	}
	var actual bytes.Buffer
	for _, child := range detail.children {
		child.write(&actual, 0)
	}
	if actual.String() != expected.String() {
		t.Fatal("summary altered fixed field metrics, keys or divider artwork")
	}
	if attr(summary, "style") != "display:none" || attr(detail, "style") != "" {
		t.Fatal("standalone default must retain full fields")
	}
	if len(summary.children) != 1 || attr(summary.children[0], "font-size") != "17.6" || attr(summary.children[0], "font-weight") != "normal" {
		t.Fatal("summary must use shared wrapped text at authored style scale")
	}
	doc.Elements[0].Summary = strings.Repeat("exceeds available space ", 30)
	if _, err := Render(doc, Options{}); err == nil || !strings.Contains(err.Error(), "entity summary: text overflow") {
		t.Fatalf("summary overflow was hidden: %v", err)
	}
}
