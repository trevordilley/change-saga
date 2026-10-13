package diagram

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"reflect"
	"strings"
	"testing"
)

func compositionDoc(w, h float64, e ...Element) Document {
	return Document{Version: Version, Width: w, Height: h, Elements: e}
}
func compositionFrame(id, target string, x, y, w, h float64) Element {
	return Element{ID: id, Kind: "frame", X: x, Y: y, Width: w, Height: h, Style: "normal", Label: id, Summary: "Overview explanation", Source: &SourceReference{Target: target, Revision: "revision-1"}}
}
func compositionNode(id string, x, y float64) Element {
	return Element{ID: id, Kind: "node", Shape: "rect", X: x, Y: y, Width: 40, Height: 30, Style: "normal"}
}

func TestCompositionNestedTransformsRepeatedInstancesAndFocus(t *testing.T) {
	entity := compositionNode("organization", 10, 20)
	entity.Source = &SourceReference{Target: "data-entity:organization", Revision: "entity-1"}
	leaf := compositionDoc(100, 100, entity)
	inner := compositionFrame("identity", "erd:identity", 20, 30, 200, 200)
	child := compositionDoc(400, 300, inner)
	outer := compositionFrame("cloud", "erd:cloud", 100, 50, 800, 600)
	outer.Parent = "region"
	region := Element{ID: "region", Kind: "group", X: 10, Y: 15, Style: "normal"}
	duplicate := compositionFrame("replica", "erd:identity", 1100, 50, 200, 200)
	connection := Element{ID: "sync", Kind: "connection", From: "region/cloud/identity/organization", To: "replica/organization", Bends: []Point{{900, 250}}, Style: "secondary", Head: "arrow", Source: &SourceReference{Target: "erd:cloud", Revision: "revision-1", Relationship: "replication"}}
	note := Element{ID: "explain", Kind: "sticky", X: 20, Y: 700, Width: 180, Height: 180, Style: "caption", Label: "Shared organization", About: "replica", Source: entity.Source, Visibility: "overview"}
	board := compositionDoc(1600, 1000, region, outer, duplicate, connection, note)
	calls := map[string]int{}
	resolver := func(s SourceReference) (ResolvedSource, error) {
		calls[s.Target]++
		if s.Target == "erd:cloud" {
			return ResolvedSource{Document: &child}, nil
		}
		return ResolvedSource{Document: &leaf}, nil
	}
	c, err := ResolveComposition(board, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if calls["erd:identity"] != 1 {
		t.Fatalf("repeated source resolved %d times", calls["erd:identity"])
	}
	focus, err := c.Describe("region/cloud/identity/organization")
	if err != nil {
		t.Fatal(err)
	}
	if len(focus.Elements) != 1 || len(focus.Context) != 3 || len(focus.Connections) != 1 {
		t.Fatalf("focus missing context or connections: %+v", focus)
	}
	want := Box{X: 190, Y: 205, Width: 160, Height: 120}
	if got := *focus.Elements[0].Bounds; got != want {
		t.Fatalf("nested transform %+v, want %+v", got, want)
	}
	if focus.Elements[0].Source.Target != "data-entity:organization" || focus.Connections[0].From != "region/cloud/identity/organization" {
		t.Fatalf("lost provenance or semantic endpoint: %+v", focus)
	}
	full, err := c.Describe("")
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range full.Elements {
		if i.Path == "explain" && i.About != "replica" {
			t.Fatalf("annotation about: %+v", i)
		}
	}
	svg, err := c.Render(Options{Title: "Board"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`data-frame-path="region/cloud/identity"`, `data-instance-path="replica/organization"`, `data-documentation-target="data-entity:organization"`, `data-source-relationship="replication"`, `data-from="region/cloud/identity/organization"`, `data-from-frame="region/cloud"`, `data-visibility="overview"`, `data-connection-detail="sync"`, `data-connection-overview="sync"`} {
		if !strings.Contains(string(svg), want) {
			t.Errorf("missing %s", want)
		}
	}
	assertUniqueSVGIDs(t, svg)
	// Resolution snapshots input and callback data; Describe also returns a copy.
	leaf.Elements[0].Label = "Mutated"
	focus.Elements[0].Source.Target = "mutated"
	again, err := c.Render(Options{Title: "Board"})
	if err != nil {
		t.Fatal(err)
	}
	if string(svg) != string(again) {
		t.Fatal("render depended on mutable input")
	}
	check, _ := c.Describe("region/cloud/identity/organization")
	if check.Elements[0].Source.Target == "mutated" {
		t.Fatal("Describe leaked mutable source")
	}
}

func assertUniqueSVGIDs(t *testing.T, data []byte) {
	t.Helper()
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	ids := map[string]bool{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok {
			attrs := map[string]bool{}
			for _, a := range start.Attr {
				if attrs[a.Name.Local] {
					t.Fatalf("duplicate attribute %s on %s", a.Name.Local, start.Name.Local)
				}
				attrs[a.Name.Local] = true
				if a.Name.Local == "id" {
					if ids[a.Value] {
						t.Fatalf("duplicate SVG id %s", a.Value)
					}
					ids[a.Value] = true
				}
			}
		}
	}
}

func TestCompositionOpaqueSVGAndExplicitBindings(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="10 20 200 100"><script>alert(1)</script><rect id="unbound" x="10" y="20" width="100" height="50"/></svg>`)
	frame := compositionFrame("legacy", "erd:legacy", 100, 100, 400, 400)
	board := compositionDoc(800, 800, frame, compositionNode("outside", 600, 100))
	resolver := func(SourceReference) (ResolvedSource, error) { return ResolvedSource{SVG: svg}, nil }
	c, err := ResolveComposition(board, resolver)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := c.Describe("legacy")
	if len(d.Elements) != 1 || !d.Elements[0].Opaque {
		t.Fatalf("opaque frame fabricated structure: %+v", d)
	}
	if _, err := c.Describe("legacy/unbound"); err == nil {
		t.Fatal("SVG ID became an inferred instance")
	}
	rendered, err := c.Render(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "data:image/svg+xml;base64,") || strings.Contains(string(rendered), "<script") || strings.Contains(string(rendered), `id="unbound"`) {
		t.Fatal("SVG was not isolated")
	}
	if !strings.Contains(string(rendered), `<image role="img"`) {
		t.Fatal("opaque image lacks explicit accessible role")
	}
	connection := Element{ID: "link", Kind: "connection", From: "legacy/bound", To: "outside", Style: "normal"}
	board.Elements = append(board.Elements, connection)
	binding := Binding{ID: "bound", Source: &SourceReference{Target: "data-entity:bound", Revision: "revision-1"}, Label: "Bound entity"}
	resolver = func(SourceReference) (ResolvedSource, error) {
		return ResolvedSource{SVG: svg, Bindings: []Binding{binding}}, nil
	}
	if _, err := ResolveComposition(board, resolver); err == nil || !strings.Contains(err.Error(), "no explicit bounds") {
		t.Fatalf("unlocated binding accepted: %v", err)
	}
	binding.Bounds = &Box{X: 10, Y: 20, Width: 50, Height: 40}
	c, err = ResolveComposition(board, resolver)
	if err != nil {
		t.Fatal(err)
	}
	d, _ = c.Describe("legacy/bound")
	// Uniform 2x scale, vertically centered (100px letterbox), viewBox origin removed.
	if got := *d.Elements[0].Bounds; got != (Box{X: 100, Y: 200, Width: 100, Height: 80}) {
		t.Fatalf("SVG viewBox transform: %+v", got)
	}
	if len(d.Connections) != 1 {
		t.Fatal("binding focus omitted incident connection")
	}
	rendered, err = c.Render(Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertUniqueSVGIDs(t, rendered)
}

func TestCompositionValidationAndCompatibility(t *testing.T) {
	old := sample()
	before, err := Render(old, Options{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := ResolveComposition(old, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := c.Render(Options{Title: "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) == 0 || len(before) == 0 {
		t.Fatal("empty SVG")
	}
	if _, err := ResolveComposition(compositionDoc(100, 100, compositionFrame("f", "erd:x", 0, 0, 100, 100)), nil); err == nil {
		t.Fatal("unresolved frame accepted")
	}
	frame := compositionFrame("f", "erd:x", 0, 0, 200, 200)
	for _, test := range []struct {
		name string
		edit func(*Element)
	}{
		{"missing revision", func(e *Element) { e.Source.Revision = "" }},
		{"missing target", func(e *Element) { e.Source.Target = "" }},
		{"frame selector", func(e *Element) { e.Source.Element = "table" }},
		{"visibility", func(e *Element) { e.Visibility = "secret" }},
		{"zero width", func(e *Element) { e.Width = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := frame
			source := *frame.Source
			e.Source = &source
			test.edit(&e)
			if compositionDoc(300, 300, e).Validate() == nil {
				t.Fatal("invalid frame accepted")
			}
		})
	}
	loop := compositionDoc(300, 300, frame)
	if _, err := ResolveComposition(loop, func(SourceReference) (ResolvedSource, error) { return ResolvedSource{Document: &loop}, nil }); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error: %v", err)
	}
	if _, err := Render(loop, Options{}); err == nil {
		t.Fatal("plain renderer silently ignored frame")
	}
	data, _ := Encode(loop)
	decoded, err := Decode(data)
	if err != nil || !reflect.DeepEqual(decoded, loop) {
		t.Fatalf("round trip: %v", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if err := compileSchema(t).Validate(value); err != nil {
		t.Fatal(err)
	}
	described := Describe(loop, 0, 10)
	if described.Elements[0].Source.Target != "erd:x" || described.Elements[0].Summary == "" {
		t.Fatal("legacy describe drops frame metadata")
	}
	for _, bad := range []string{`<g/>`, `<svg/><svg/>`, `<svg>`, `<!DOCTYPE svg><svg/>`} {
		if _, _, err := svgSize([]byte(bad)); err == nil {
			t.Errorf("invalid SVG accepted: %s", bad)
		}
	}
}

func TestCompositionFocusIncludesCalloutsAndNestedConnectionGeometry(t *testing.T) {
	a, b := compositionNode("a", 10, 20), compositionNode("b", 150, 20)
	callout := Element{ID: "why", Kind: "annotation", Shape: "bubble", X: 10, Y: 100, Width: 160, Height: 70, Style: "caption", Label: "Why this entity exists", About: "a", Source: &SourceReference{Target: "data-entity:a", Revision: "r1"}}
	link := Element{ID: "link", Kind: "connection", From: "a", To: "b", X: 5, Y: 10, Bends: []Point{{100, 80}}, Style: "normal", Head: "arrow"}
	child := compositionDoc(300, 200, a, b, callout, link)
	frame := compositionFrame("area", "erd:area", 100, 100, 600, 600)
	boardCallout := Element{ID: "board-note", Kind: "annotation", Shape: "bubble", X: 750, Y: 100, Width: 200, Height: 90, Style: "caption", Label: "Cross-frame explanation", About: "area/a"}
	board := compositionDoc(1000, 900, frame, boardCallout)
	c, err := ResolveComposition(board, func(SourceReference) (ResolvedSource, error) { return ResolvedSource{Document: &child}, nil })
	if err != nil {
		t.Fatal(err)
	}
	d, err := c.Describe("area/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Elements) != 3 || d.Elements[1].Path != "area/why" || d.Elements[2].Path != "board-note" {
		t.Fatalf("focus omitted relevant callouts: %+v", d.Elements)
	}
	if len(d.Connections) != 1 || d.Connections[0].From != "area/a" {
		t.Fatalf("focus connections: %+v", d.Connections)
	}
	svg, err := c.Render(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), `translate(0 100) scale(2)`) {
		t.Fatal("letterbox transform missing")
	}
	// Authored bend remains local to the connector, regardless of enclosing scale.
	if !strings.Contains(string(svg), `100 80`) {
		t.Fatal("authored bend lost")
	}
	assertUniqueSVGIDs(t, svg)
}

func TestCompositionRejectsInvalidViewportsAndSupportsLegacyBindingIDs(t *testing.T) {
	for _, v := range []string{"NaN 0 100 100", "0 +Inf 100 100", "0 0 -1 100", "0 0 100 0"} {
		if _, _, err := svgSize([]byte(`<svg viewBox="` + v + `"/>`)); err == nil {
			t.Errorf("invalid viewBox accepted: %s", v)
		}
	}
	board := compositionDoc(500, 500, compositionFrame("old", "erd:old", 0, 0, 400, 400))
	c, err := ResolveComposition(board, func(SourceReference) (ResolvedSource, error) {
		return ResolvedSource{SVG: []byte(`<svg viewBox="0 0 100 100"/>`), Bindings: []Binding{{ID: "Entity_Account.v2", Source: &SourceReference{Target: "data-entity:account", Revision: "r1"}}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Describe("old/Entity_Account.v2"); err != nil {
		t.Fatal(err)
	}
}
