package diagram

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func float(v float64) *float64 { return &v }

// sample is a small sequence-style diagram exercising frames, external
// labels, graphics with currentColor, alignment, and edges with labels.
func sample() Document {
	d := New()
	d.Styles = map[string]Style{"reply": {Fill: "none", Stroke: "#64748b", Ink: "#526174", StrokeWidth: 1.5, FontSize: 16, Dash: true}}
	d.Elements = []Element{
		{ID: "heading", Kind: "text", Label: "Publishing one batch", X: 60, Y: 28, Width: 900, Height: 48, Style: "title", Decorative: true},
		{ID: "author", Kind: "node", Shape: "rect", Label: "Author", Icon: "lucide:user", X: 76, Y: 110, Width: 200, Height: 64, Style: "normal"},
		{ID: "store", Kind: "node", Shape: "datastore", Label: "Store", Detail: "One record", X: 900, Y: 100, Width: 220, Height: 110, Style: "normal"},
		{ID: "lifeline", Kind: "graphic", X: 176, Y: 174, Z: -1, Style: "secondary", Decorative: true, Fragment: `<line x1="0" y1="0" x2="0" y2="300" stroke="currentColor" stroke-dasharray="6 6"/>`},
		{ID: "alt", Kind: "group", Shape: "boundary", Label: "alt  [snapshot matches]", X: 110, Y: 300, Width: 1070, Height: 200, Style: "boundary"},
		{ID: "commit", Kind: "node", Shape: "ellipse", Parent: "alt", X: 40, Y: 80, Width: 20, Height: 20, Style: "primary", Label: "merge", LabelBox: &Box{X: -30, Y: 24, Width: 80, Height: 28}, Align: "middle", Description: "Merge after review"},
		{ID: "publish", Kind: "edge", From: "author", To: "store", Parent: "alt", Points: []Point{{66, 150}, {790, 150}}, Head: "arrow", Label: "publish  record", LabelBox: &Box{X: 80, Y: 120, Width: 200, Height: 24}, Style: "reply"},
	}
	return d
}

func TestRenderIsDeterministicAndMatchesGolden(t *testing.T) {
	d := sample()
	first, err := Render(d, Options{Title: "Publishing one batch", Description: "One record publishes source and SVG together."})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(d, Options{Title: "Publishing one batch", Description: "One record publishes source and SVG together."})
	if !bytes.Equal(first, second) {
		t.Fatal("rendering is not deterministic")
	}
	golden := filepath.Join("testdata", "sample.svg")
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
		t.Fatal("renderer output changed; if intended, bump Renderer when stored diagrams would re-render differently and run with -update")
	}
	svg := string(first)
	for _, want := range []string{
		`viewBox="0 0 1280 720"`, `id="author"`, `id="publish"`, `id="commit"`, `data-from="author"`,
		`src:url("` + FontPath, `<desc id="diagram-desc">`, `xml:space="preserve"`, `text-anchor="middle"`,
		`id="lifeline" data-diagram-kind="graphic"`, `color="#94a3b8"`, `stroke-dasharray="7 6"`, `id="diagram-notices"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	if strings.Contains(svg, "base64") {
		t.Error("SVG must reference the shared font, not embed it")
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	d := New()
	d.Elements = []Element{
		{ID: "a", Kind: "node", Shape: "hexagon", Width: 10, Height: 10, Style: "normal"},
		{ID: "b", Kind: "edge", From: "a", To: "missing", Points: []Point{{0, 0}, {1, 1}}, Label: "x", Style: "nope"},
		{ID: "frame", Kind: "group", Style: "normal", Decorative: true},
		{ID: "hidden", Kind: "text", Parent: "frame", Label: "Seen", Width: 100, Height: 30, Style: "normal"},
		{ID: "diagram-title", Kind: "text", Width: 1, Height: 1, Style: "normal"},
		{ID: "a", Kind: "text", Width: 1, Height: 1, Style: "normal"},
	}
	err := d.Validate()
	if err == nil {
		t.Fatal("expected problems")
	}
	for _, want := range []string{"a: node shape", "b: unknown style", `b: edge endpoint "missing"`, "b: an edge label needs an explicit label_box", "hidden: semantic element inside decorative group frame", `element "diagram-title"`, "a: duplicate id"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestFragmentAllowlist(t *testing.T) {
	for _, fragment := range []string{
		`<script>alert(1)</script>`, `<rect id="author" width="1" height="1"/>`, `<rect style="fill:red" width="1" height="1"/>`,
		`<rect fill="url(#x)" width="1" height="1"/>`, `<image href="x.png"/>`, `<a href="javascript:x"><rect/></a>`,
		`<rect onclick="x"/>`, `<!-- note --><rect/>`, `stray text`, `<rect xlink:href="x"/>`,
	} {
		if _, err := parseFragment(fragment); err == nil {
			t.Errorf("fragment accepted: %s", fragment)
		}
	}
	children, err := parseFragment(`<g transform="translate(1 2)"><circle r="3" fill="currentColor"/><text x="1" font-size="12">Hi</text></g>`)
	if err != nil || len(children) != 1 || len(children[0].children) != 2 || children[0].children[1].text != "Hi" {
		t.Fatalf("valid fragment rejected or mangled: %v", err)
	}
}

func TestTextOverflowAndMissingGlyphsRefuse(t *testing.T) {
	d := New()
	d.Elements = []Element{{ID: "box", Kind: "node", Shape: "rect", Label: "This label is far too long for its box", Width: 120, Height: 60, Style: "normal"}}
	if _, err := Render(d, Options{Title: "x"}); err == nil || !strings.Contains(err.Error(), "text overflow") {
		t.Fatalf("expected overflow, got %v", err)
	}
	d.Elements[0] = Element{ID: "box", Kind: "text", Label: "漢字", Width: 400, Height: 60, Style: "normal"}
	if _, err := Render(d, Options{Title: "x"}); err == nil || !strings.Contains(err.Error(), "no glyph") {
		t.Fatalf("expected missing glyph, got %v", err)
	}
	d.Elements[0] = Element{ID: "box", Kind: "text", Label: "Wraps across several short lines", Wrap: true, Width: 120, Height: 90, Style: "caption"}
	if _, err := Render(d, Options{Title: "x"}); err != nil {
		t.Fatalf("wrapping text should fit: %v", err)
	}
}

func TestEditOperations(t *testing.T) {
	d := sample()
	edge, _ := d.Element("publish")
	moved, changed, err := Edit(d, []Operation{{Op: "move", ID: "author", DX: 20, DY: 30}})
	if err != nil || strings.Join(changed, ",") != "author" {
		t.Fatalf("move: %v %v", changed, err)
	}
	if after, _ := moved.Element("publish"); !equalJSON(after, edge) {
		t.Fatal("moving a node rerouted its edge")
	}
	if author, _ := moved.Element("author"); author.X != 96 || author.Y != 140 {
		t.Fatalf("move offset wrong: %+v", author)
	}
	if _, _, err := Edit(d, []Operation{{Op: "remove", ID: "author"}}); err == nil || !strings.Contains(err.Error(), "publish depends on author") {
		t.Fatalf("removal must refuse dependents without cascade: %v", err)
	}
	removed, changed, err := Edit(d, []Operation{{Op: "remove", ID: "alt", Cascade: true}})
	if err != nil || removed.Index("commit") >= 0 || removed.Index("publish") >= 0 || strings.Join(changed, ",") != "alt,commit,publish" {
		t.Fatalf("cascade removal: %v %v", changed, err)
	}
	inserted, _, err := Edit(d, []Operation{{Op: "add", Before: "author", Element: &Element{ID: "actor", Kind: "text", Label: "Actor", Width: 80, Height: 30, Style: "normal"}}})
	if err != nil || inserted.Index("actor") != 1 {
		t.Fatalf("add before: %v", err)
	}
	if _, _, err := Edit(d, []Operation{{Op: "update", ID: "author", Set: json.RawMessage(`{"id":"x"}`)}}); err == nil {
		t.Fatal("id must be immutable")
	}
	updated, _, err := Edit(d, []Operation{{Op: "update", ID: "author", Set: json.RawMessage(`{"label":"Writer","icon":null}`)}})
	if a, _ := updated.Element("author"); err != nil || a.Label != "Writer" || a.Icon != "" {
		t.Fatalf("update with null removal: %+v %v", a, err)
	}
	aligned, _, err := Edit(d, []Operation{
		{Op: "align", IDs: []string{"author", "store"}, Axis: "y", Value: float(200)},
		{Op: "distribute", IDs: []string{"author", "store"}, Axis: "x", Gap: float(40), Start: float(60)},
	})
	author, _ := aligned.Element("author")
	store, _ := aligned.Element("store")
	if err != nil || author.Y != 200 || store.Y != 200 || author.X != 60 || store.X != 300 {
		t.Fatalf("align/distribute: %+v %+v %v", author, store, err)
	}
	if _, _, err := Edit(d, []Operation{{Op: "align", IDs: []string{"author", "commit"}, Axis: "x", Value: float(1)}}); err == nil {
		t.Fatal("align across coordinate spaces must refuse")
	}
	styled, changed, err := Edit(d, []Operation{{Op: "style", ID: "reply", Style: &Style{Fill: "none", Stroke: "#000000", Ink: "#000000", StrokeWidth: 1, FontSize: 16}}})
	if err != nil || strings.Join(changed, ",") != "publish" || styled.Styles["reply"].Stroke != "#000000" {
		t.Fatalf("style consumers: %v %v", changed, err)
	}
	canvas, _, err := Edit(d, []Operation{{Op: "canvas", Set: json.RawMessage(`{"width":1024,"height":576}`)}})
	if err != nil || canvas.Width != 1024 || canvas.Height != 576 {
		t.Fatalf("canvas: %v", err)
	}
}

func TestFramedGroupTranslatesChildren(t *testing.T) {
	d := sample()
	moved, _, err := Edit(d, []Operation{{Op: "move", ID: "alt", DY: 50}})
	if err != nil {
		t.Fatal(err)
	}
	commit, _ := moved.Element("commit")
	if commit.Y != 80 {
		t.Fatal("children keep local coordinates; the frame's translation moves them")
	}
	svg, err := Render(moved, Options{Title: "x"})
	if err != nil || !strings.Contains(string(svg), `id="alt" data-diagram-kind="group" transform="translate(110 350)"`) {
		t.Fatalf("framed group not translated: %v", err)
	}
}

func TestDescribeText(t *testing.T) {
	var b strings.Builder
	Describe(sample(), 0, 30).WriteText(&b)
	text := b.String()
	for _, line := range []string{
		"Groups:\n  alt \"alt  [snapshot matches]\" shape=boundary\n",
		"  author \"Author\" shape=rect icon=lucide:user\n",
		"  store \"Store\" shape=datastore\n    detail: One record\n",
		"  commit \"merge\" shape=ellipse in=alt\n    description: Merge after review\n",
		"  publish: author -> store \"publish  record\" in=alt\n",
		"Showing elements 1-5 of 5.\n",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("missing %q in:\n%s", line, text)
		}
	}
	if strings.Contains(text, "heading") || strings.Contains(text, "lifeline") {
		t.Error("decorative elements must be omitted")
	}
	b.Reset()
	Describe(sample(), 1, 2).WriteText(&b)
	if !strings.Contains(b.String(), "Showing elements 2-3 of 5; continue with --offset 3.") {
		t.Fatal(b.String())
	}
	if Plain("two\nlines") != `"two\nlines"` || Plain("plain prose") != "plain prose" {
		t.Fatal("Plain quoting")
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	if _, err := Decode([]byte(`{"version":1,"width":1,"height":1,"elements":[],"extra":1}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := DecodeOperations([]byte(`[{"op":"move","id":"a","dz":1}]`)); err == nil {
		t.Fatal("unknown operation field accepted")
	}
}

func TestIcons(t *testing.T) {
	if names := Icons("data"); len(names) != 1 || names[0] != "lucide:database" {
		t.Fatalf("icon search: %v", names)
	}
	if _, err := Icon("lucide:database"); err != nil {
		t.Fatal(err)
	}
	if IconExists("lucide:nope") {
		t.Fatal("unknown icon reported")
	}
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func TestReviewRegressions(t *testing.T) {
	deep := strings.Repeat("<g>", 30) + "<rect width=\"1\" height=\"1\"/>" + strings.Repeat("</g>", 30)
	for name, fragment := range map[string]string{
		"deep nesting":        deep,
		"css escape":          `<rect fill="\75rl(http://host/p.svg#g)" width="1" height="1"/>`,
		"entity escape":       `<rect fill="&#92;75rl(#g)" width="1" height="1"/>`,
		"url in transform":    `<g transform="translate(url(#x))"><rect/></g>`,
		"duplicate attribute": `<rect x="1" x="2" width="3" height="4"/>`,
		"mixed content":       `<text>a<tspan>b</tspan>c</text>`,
		"named color call":    `<rect fill="rgb(1,2,3)" width="1" height="1"/>`,
	} {
		if _, err := parseFragment(fragment); err == nil {
			t.Errorf("%s accepted: %s", name, fragment)
		}
	}
	if _, err := parseFragment(`<g transform="translate(1 2) rotate(45)"><path d="M0 0L10 10" stroke="currentColor" stroke-dasharray="4 4" stroke-linecap="round"/></g>`); err != nil {
		t.Fatalf("ordinary drawing markup refused: %v", err)
	}

	d := New()
	for index := 0; index < 5; index++ {
		d.Elements = append(d.Elements, Element{ID: "g" + string(rune('a'+index)), Kind: "graphic", Style: "normal", Decorative: true, Fragment: `<rect width="1" height="1"/>` + strings.Repeat(" ", 60<<10)})
	}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "graphic fragments total") {
		t.Fatalf("total fragment cap not enforced: %v", err)
	}

	if got := num(38.925000000000004); got != "38.925" {
		t.Fatalf("num must round away floating-point contraction noise, got %s", got)
	}
	if got := num(-0.0001); got != "0" {
		t.Fatalf("negative zero must normalize, got %s", got)
	}

	d = New()
	d.Styles = map[string]Style{"odd": {Fill: "none", Stroke: "none", Ink: "#000000", StrokeWidth: 1, FontSize: 17.3}}
	d.Elements = []Element{{ID: "note", Kind: "text", Label: "one\ntwo\nthree", Width: 300, Height: 90, Style: "odd", Align: "middle"}}
	svg, err := Render(d, Options{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(svg), "0000000") {
		t.Fatalf("coordinates carry floating-point noise:\n%s", svg)
	}
	if regexp.MustCompile(`<text[^>]*>\s|</tspan>\s+<tspan`).Match(svg) {
		t.Fatalf("whitespace inside preserved text shifts alignment:\n%s", svg)
	}

	sample := sample()
	if _, _, err := Edit(sample, []Operation{{Op: "update", ID: "author", Set: json.RawMessage(`{"Label":"x"}`)}}); err == nil || !strings.Contains(err.Error(), `unknown element field "Label"`) {
		t.Fatalf("case-variant keys must be refused: %v", err)
	}
	bad := New()
	bad.Elements = []Element{
		{ID: "a", Kind: "node", Shape: "rect", Width: 10, Height: 10, Style: "normal"},
		{ID: "e", Kind: "edge", From: "a", To: "a", Path: "M0 0L1 1", Points: []Point{{1, 1}}, Style: "normal"},
		{ID: "frame", Kind: "group", Icon: "lucide:user", Style: "normal"},
	}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "e: edge needs either") || !strings.Contains(err.Error(), "frame: only nodes take an icon") {
		t.Fatalf("path+point edge and group icon must be refused: %v", err)
	}
	encoded, _ := Encode(Document{Version: Version, Width: 1, Height: 1})
	if !strings.Contains(string(encoded), `"elements": []`) {
		t.Fatalf("stored source must carry an elements array: %s", encoded)
	}
}
