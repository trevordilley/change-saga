package diagram

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// connected is an ERD and UML sampler: crow's-foot cardinalities with end
// labels, UML ends, dashed, dotted and smooth lines, and a thick arrow.
func connected() Document {
	d := New()
	d.Styles = map[string]Style{"thick": {Fill: "none", Stroke: "#2563eb", Ink: "#153869", StrokeWidth: 6, FontSize: 18}}
	box := func(x, y float64) *Box { return &Box{X: x, Y: y, Width: 40, Height: 28} }
	d.Elements = []Element{
		{ID: "customers", Kind: "node", Shape: "rect", Label: "customers", X: 60, Y: 80, Width: 220, Height: 80, Style: "normal"},
		{ID: "orders", Kind: "node", Shape: "rect", Label: "orders", X: 560, Y: 80, Width: 220, Height: 80, Style: "normal"},
		{ID: "base", Kind: "node", Shape: "rect", Label: "Shape", X: 60, Y: 360, Width: 220, Height: 80, Style: "normal"},
		{ID: "square", Kind: "node", Shape: "rect", Label: "Square", X: 560, Y: 360, Width: 220, Height: 80, Style: "normal"},
		{ID: "cache", Kind: "node", Shape: "rect", Label: "Cache", X: 960, Y: 360, Width: 220, Height: 80, Style: "normal"},
		{ID: "placed", Kind: "edge", From: "orders", To: "customers", Points: []Point{{560, 120}, {280, 120}}, Tail: "zero-or-many", Head: "only-one",
			Label: "placed by", LabelBox: &Box{X: 360, Y: 84, Width: 120, Height: 28}, TailLabel: "N", TailLabelBox: box(500, 124), HeadLabel: "1", HeadLabelBox: box(290, 124), Style: "normal"},
		{ID: "extends", Kind: "edge", From: "square", To: "base", Points: []Point{{560, 400}, {280, 400}}, Head: "triangle", Style: "normal", Line: "dashed"},
		{ID: "owns", Kind: "edge", From: "square", To: "cache", Points: []Point{{780, 400}, {960, 400}}, Tail: "filled-diamond", Head: "open", Line: "dotted", Style: "normal"},
		{ID: "flows", Kind: "edge", From: "customers", To: "cache", Points: []Point{{170, 160}, {400, 280}, {900, 250}, {1070, 360}}, Curve: "smooth", Head: "arrow", Style: "thick"},
	}
	return d
}

func TestConnectorsRenderDeterministicallyAndMatchGolden(t *testing.T) {
	first, err := Render(connected(), Options{Title: "Connectors"})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(connected(), Options{Title: "Connectors"})
	if !bytes.Equal(first, second) {
		t.Fatal("connector rendering is not deterministic")
	}
	golden := filepath.Join("testdata", "connectors.svg")
	if *update {
		if err := os.WriteFile(golden, first, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/diagram -update)", err)
	}
	if !bytes.Equal(first, want) {
		t.Fatal("connector rendering changed; if intended, run with -update")
	}
	svg := string(first)
	for _, want := range []string{
		`marker-start="url(#diagram-marker-placed-tail)" marker-end="url(#diagram-marker-placed-head)"`,
		`id="diagram-marker-placed-tail" viewBox="0 0 10 10" refX="10" refY="5" markerWidth="24" markerHeight="24" markerUnits="userSpaceOnUse" orient="auto-start-reverse" overflow="visible"`,
		`stroke-dasharray="8 6"`, `stroke-linecap="round"`, ` C`,
		// A thick edge's legacy arrow keeps its definition but grows with the stroke.
		`id="diagram-head-flows" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="30" markerHeight="30"`,
		`>N</tspan>`, `>1</tspan>`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
}

func TestConnectorValidation(t *testing.T) {
	d := connected()
	d.Elements = append(d.Elements,
		Element{ID: "bad-head", Kind: "edge", From: "base", To: "cache", Points: []Point{{0, 0}, {1, 1}}, Head: "crow", Style: "normal"},
		Element{ID: "bad-label", Kind: "edge", From: "base", To: "cache", Points: []Point{{0, 0}, {1, 1}}, HeadLabel: "1", Style: "normal"},
		Element{ID: "bad-curve", Kind: "edge", From: "base", To: "cache", Path: "M0 0L1 1", Curve: "smooth", Line: "wavy", Style: "normal"},
		Element{ID: "bad-node", Kind: "node", Shape: "rect", Width: 10, Height: 10, Tail: "one", Style: "normal"},
	)
	err := d.Validate()
	if err == nil {
		t.Fatal("expected connector problems")
	}
	for _, want := range []string{
		"bad-head: head must be none or one of arrow, bar, circle",
		"bad-label: head_label needs an explicit head_label_box",
		"bad-curve: curve smooth applies to points, not to an explicit path",
		"bad-curve: line must be solid, dashed, or dotted",
		"bad-node: connector fields (tail, head_label, tail_label, curve, line) are only valid on edges",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestConnectorsDescribeTheirCardinality(t *testing.T) {
	var b strings.Builder
	Describe(connected(), 0, 30).WriteText(&b)
	text := b.String()
	for _, want := range []string{
		"  placed: orders -> customers \"placed by\" tail=zero-or-many head=only-one tail_label=\"N\" head_label=\"1\"\n",
		"  extends: square -> base head=triangle\n",
		"  owns: square -> cache tail=filled-diamond head=open\n",
		"  flows: customers -> cache\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q in:\n%s", want, text)
		}
	}
	data, _ := json.Marshal(Describe(connected(), 0, 30))
	if !strings.Contains(string(data), `"tail":"zero-or-many","head":"only-one","tail_label":"N","head_label":"1"`) {
		t.Fatalf("describe JSON lacks the cardinality: %s", data)
	}
}

func TestConnectorEditRoundTrip(t *testing.T) {
	edited, _, err := Edit(connected(), []Operation{{Op: "update", ID: "extends", Set: json.RawMessage(`{"head":"one-or-many","tail":"one","line":null}`)}})
	if err != nil {
		t.Fatal(err)
	}
	extends, _ := edited.Element("extends")
	if extends.Head != "one-or-many" || extends.Tail != "one" || extends.Line != "" {
		t.Fatalf("update did not set terminators: %+v", extends)
	}
	if err := edited.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSchemaTerminatorsMatchRuntime(t *testing.T) {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs struct {
			Terminator struct {
				Enum []string `json:"enum"`
			} `json:"terminator"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	got := append([]string{}, schema.Defs.Terminator.Enum...)
	sort.Strings(got)
	if want := TerminatorNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("schema terminators %v, runtime %v", got, want)
	}
	compiled := compileSchema(t)
	encoded, _ := Encode(connected())
	value, _ := jsonschema.UnmarshalJSON(strings.NewReader(string(encoded)))
	if err := compiled.Validate(value); err != nil {
		t.Fatalf("a connected source does not satisfy its schema: %v", err)
	}
}
