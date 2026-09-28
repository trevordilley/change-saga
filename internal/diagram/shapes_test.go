package diagram

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// gallery draws every library shape and an ERD with an edge that ends on a
// field row.
func gallery() Document {
	d := New()
	d.Elements = []Element{
		{ID: "heading", Kind: "text", Label: "Shapes", X: 40, Y: 20, Width: 400, Height: 44, Style: "title", Decorative: true},
		{ID: "triangle", Kind: "node", Shape: "triangle", Label: "Warn", X: 40, Y: 90, Width: 180, Height: 150, Style: "warning"},
		{ID: "hexagon", Kind: "node", Shape: "hexagon", Label: "Prepare", X: 260, Y: 110, Width: 200, Height: 110, Style: "normal"},
		{ID: "input", Kind: "node", Shape: "parallelogram", Label: "Read input", X: 500, Y: 110, Width: 220, Height: 110, Style: "normal"},
		{ID: "report", Kind: "node", Shape: "document", Label: "Report", Detail: "weekly", Icon: "lucide:file-text", X: 760, Y: 90, Width: 200, Height: 150, Style: "primary"},
		{ID: "cdn", Kind: "node", Shape: "cloud", Label: "CDN", Icon: "lucide:globe", X: 1000, Y: 90, Width: 240, Height: 150, Style: "normal"},
		{ID: "customer", Kind: "node", Shape: "actor", Label: "Customer", X: 60, Y: 280, Width: 140, Height: 170, Style: "normal"},
		{ID: "jobs", Kind: "node", Shape: "queue", Label: "Jobs", Detail: "at least once", X: 240, Y: 310, Width: 260, Height: 110, Style: "normal"},
		{ID: "start", Kind: "node", Shape: "circle", Label: "Start", X: 540, Y: 290, Width: 140, Height: 140, Style: "primary"},
		{ID: "favorite", Kind: "node", Shape: "star", Label: "Top", X: 720, Y: 280, Width: 170, Height: 162, Style: "warning"},
		{ID: "customers", Kind: "node", Shape: "entity", Label: "customers", Icon: "lucide:table", X: 60, Y: 480, Width: 300, Height: 128, Style: "normal",
			Fields: []Field{{Name: "id", Type: "uuid", Key: "pk"}, {Name: "email", Type: "text"}}},
		{ID: "orders", Kind: "node", Shape: "entity", Label: "orders", X: 600, Y: 480, Width: 320, Height: 184, Style: "primary",
			Fields: []Field{{Name: "id", Type: "uuid", Key: "pk"}, {Name: "customer_id", Type: "uuid", Key: "fk"}, {Name: "total", Type: "numeric(10,2)"}, {Name: "placed_at", Type: "timestamptz"}}},
		{ID: "places", Kind: "edge", From: "orders", To: "customers", FromField: "customer_id", ToField: "id", Head: "arrow", Label: "places",
			Points: []Point{{600, 565.5}, {480, 565.5}, {480, 537.5}, {360, 537.5}}, LabelBox: &Box{X: 490, Y: 570, Width: 90, Height: 30}, Style: "secondary"},
	}
	return d
}

func TestLibraryShapesRenderAndMatchGolden(t *testing.T) {
	svg, err := Render(gallery(), Options{Title: "Shapes"})
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("testdata", "shapes.svg")
	if *update {
		if err := os.WriteFile(golden, svg, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/diagram -update)", err)
	}
	if !bytes.Equal(svg, want) {
		t.Fatal("shape rendering changed; if intended, run with -update")
	}
	for _, want := range []string{
		`id="places" data-diagram-kind="edge"`, `data-from-field="customer_id" data-to-field="id"`,
		`<g data-field="customer_id" data-key="fk">`, `text-decoration="underline"`, `id="diagram-notices"`,
	} {
		if !strings.Contains(string(svg), want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
}

func TestShapeValidationReportsEveryProblem(t *testing.T) {
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

func TestLibraryShapeTextStillRefusesOverflow(t *testing.T) {
	for _, e := range []Element{
		{ID: "a", Kind: "node", Shape: "star", Label: "Far too long for a star", Width: 160, Height: 150, Style: "normal"},
		{ID: "a", Kind: "node", Shape: "circle", Label: "Two", Detail: "lines of detail\nand more\nand more", Icon: "lucide:user", Width: 100, Height: 100, Style: "normal"},
		{ID: "a", Kind: "node", Shape: "entity", Label: "t", Width: 120, Height: 100, Style: "normal", Fields: []Field{{Name: "a_very_long_column", Type: "timestamptz"}}},
		{ID: "a", Kind: "node", Shape: "triangle", Label: "x", Width: 20, Height: 20, Style: "normal"},
	} {
		d := New()
		d.Elements = []Element{e}
		if _, err := Render(d, Options{Title: "x"}); err == nil || !(strings.Contains(err.Error(), "overflow") || strings.Contains(err.Error(), "too small")) {
			t.Errorf("%s: expected refusal, got %v", e.Shape, err)
		}
	}
}

func TestDescribeReadsEntitiesAndOmitsGeometricShapes(t *testing.T) {
	var b strings.Builder
	Describe(gallery(), 0, 30).WriteText(&b)
	text := b.String()
	for _, line := range []string{
		"  triangle \"Warn\"\n", "  start \"Start\"\n", "  favorite \"Top\"\n",
		"  customer \"Customer\" shape=actor\n", "  jobs \"Jobs\" shape=queue\n", "  cdn \"CDN\" shape=cloud icon=lucide:globe\n",
		"  orders \"orders\" shape=entity\n    fields: id uuid pk, customer_id uuid fk, total \"numeric(10,2)\", placed_at timestamptz\n",
		"  places: orders.customer_id -> customers.id \"places\"\n",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("missing %q in:\n%s", line, text)
		}
	}
	if Describe(gallery(), 0, 30).Elements[1].Shape != "" {
		t.Error("the JSON reading omits geometric shapes too")
	}
	if got := (Fields{{Name: "order date", Type: "date"}}).String(); got != `"order date" date` {
		t.Errorf("a spaced name must be quoted: %s", got)
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
