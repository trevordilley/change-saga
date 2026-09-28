package diagram

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// board is a small review board: nested sections, a sticky about a store, a
// bubble pointing at an element's box and one at an explicit point, numbered
// pins, a highlight, and a labelled bracket.
func board() Document {
	d := New()
	d.Elements = []Element{
		{ID: "heading", Kind: "text", Label: "Retries stay idempotent", X: 60, Y: 28, Width: 900, Height: 48, Style: "title", Decorative: true},
		{ID: "ingest", Kind: "group", Shape: "section", Label: "Ingest", Color: "blue", X: 60, Y: 100, Width: 560, Height: 360, Style: "normal"},
		{ID: "queue", Kind: "node", Shape: "rect", Label: "Queue", Parent: "ingest", X: 30, Y: 70, Width: 200, Height: 80, Style: "normal"},
		{ID: "retry", Kind: "group", Shape: "section", Label: "Retry", Color: "purple", Parent: "ingest", X: 280, Y: 60, Width: 250, Height: 270, Style: "normal"},
		{ID: "backoff", Kind: "node", Shape: "rect", Label: "Backoff", Parent: "retry", X: 25, Y: 70, Width: 200, Height: 80, Style: "normal"},
		{ID: "store", Kind: "node", Shape: "datastore", Label: "Store", X: 820, Y: 130, Width: 240, Height: 120, Style: "normal"},
		{ID: "write", Kind: "edge", From: "backoff", To: "store", Points: []Point{{565, 270}, {700, 270}, {700, 200}, {820, 200}}, Head: "arrow", Style: "secondary"},
		{ID: "why", Kind: "sticky", Label: "Writes are keyed by batch, so a retry overwrites itself.", About: "store", X: 840, Y: 420, Width: 200, Height: 200, Style: "normal", Note: "The key is the batch digest."},
		{ID: "hot", Kind: "annotation", Shape: "bubble", Label: "Hot path", About: "store", X: 1080, Y: 300, Width: 180, Height: 90, Style: "normal"},
		{ID: "slow", Kind: "annotation", Shape: "bubble", Label: "Waits up to 30s", About: "write", Target: &Point{60, -85}, Color: "pink", X: 640, Y: 320, Width: 160, Height: 70, Style: "caption"},
		{ID: "first", Kind: "annotation", Shape: "pin", Label: "1", About: "queue", Color: "blue", X: 272, Y: 152, Width: 32, Style: "normal"},
		{ID: "second", Kind: "annotation", Shape: "pin", Label: "2", About: "backoff", Color: "purple", X: 547, Y: 212, Width: 32, Height: 32, Style: "normal"},
		{ID: "focus", Kind: "annotation", Shape: "highlight", About: "backoff", Parent: "retry", X: 15, Y: 60, Width: 220, Height: 100, Z: -1, Style: "normal"},
		{ID: "span", Kind: "annotation", Shape: "bracket", Side: "right", Label: "One record", About: "store", Color: "green", X: 1070, Y: 130, Width: 24, Height: 120, LabelBox: &Box{X: 32, Y: 46, Width: 150, Height: 28}, Style: "caption"},
	}
	return d
}

func TestBoardRenderIsDeterministicAndMatchesGolden(t *testing.T) {
	options := Options{Title: "Retries stay idempotent", Description: "A board of sections, stickies, and annotations."}
	first, err := Render(board(), options)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(board(), options)
	if !bytes.Equal(first, second) {
		t.Fatal("rendering a board is not deterministic")
	}
	golden := filepath.Join("testdata", "annotations.svg")
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
		t.Fatal("board rendering changed; if intended, run with -update")
	}
	svg := string(first)
	for _, want := range []string{
		`<filter id="diagram-shadow"`, `filter="url(#diagram-shadow)"`,
		`id="why" data-diagram-kind="sticky"`, `data-about="store"`, `data-annotation="bubble"`,
		`aria-label="Pin 1 on queue"`, `aria-label="highlight on backoff"`, `fill-opacity="0.35"`,
		`fill="#fff3a3"`, `fill="#e8f1ff"`, `fill="#6d28d9"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	if count := strings.Count(svg, `<filter `); count != 1 {
		t.Errorf("the shadow filter is defined %d times", count)
	}
}

func TestBoardFeaturesAreOptIn(t *testing.T) {
	svg, err := Render(sample(), Options{Title: "Publishing one batch"})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"diagram-shadow", "data-about", "data-annotation", "<filter"} {
		if bytes.Contains(svg, []byte(absent)) {
			t.Errorf("a diagram without board elements renders %s", absent)
		}
	}
}

func TestBubblePointsAtItsAboutElement(t *testing.T) {
	d := board()
	hot, _ := d.Element("hot")
	tip, err := pointerTip(d, hot)
	// The store's nearest corner to the bubble, (1060, 250), in bubble-local
	// coordinates.
	if err != nil || tip != (Point{-20, -50}) {
		t.Fatalf("tip %+v %v", tip, err)
	}
	backoff := Element{ID: "q", Kind: "annotation", Shape: "bubble", About: "backoff", X: 400, Y: 150, Width: 100, Height: 60}
	if tip, err := pointerTip(d, backoff); err != nil || tip != (Point{50, 80}) {
		// backoff, nested in two sections, spans (365, 230)-(565, 310) on the
		// canvas; the bubble's centre (450, 180) lies above its top edge.
		t.Fatalf("nested tip %+v %v", tip, err)
	}
	for _, side := range []struct {
		tip  Point
		want string
	}{{Point{50, -40}, "L50 -40"}, {Point{150, 20}, "L150 20"}, {Point{50, 90}, "L50 90"}, {Point{-40, 20}, "L-40 20"}} {
		if path := bubblePath(100, 50, side.tip); !strings.Contains(path, side.want) || !strings.HasSuffix(path, "Z") {
			t.Errorf("bubble toward %+v: %s", side.tip, path)
		}
	}
}

func TestBoardValidationReportsEveryProblem(t *testing.T) {
	d := New()
	d.Elements = []Element{
		{ID: "box", Kind: "node", Shape: "rect", Label: "Box", Width: 100, Height: 60, Style: "normal", Color: "blue", About: "box"},
		{ID: "flourish", Kind: "text", Label: "x", Width: 100, Height: 40, Style: "normal", Decorative: true},
		{ID: "untitled", Kind: "group", Shape: "section", Width: 300, Height: 200, Style: "normal", Color: "orange"},
		{ID: "blank", Kind: "sticky", Shape: "rect", Width: 0, Height: 100, Style: "normal", Detail: "more"},
		{ID: "self", Kind: "sticky", Label: "Me", About: "self", Width: 100, Height: 100, Style: "normal"},
		{ID: "ghost", Kind: "sticky", Label: "Boo", About: "missing", Width: 100, Height: 100, Style: "normal"},
		{ID: "hidden", Kind: "sticky", Label: "Hi", About: "flourish", Width: 100, Height: 100, Style: "normal"},
		{ID: "quiet", Kind: "sticky", Label: "Hi", About: "box", Width: 100, Height: 100, Style: "normal", Decorative: true},
		{ID: "lost", Kind: "annotation", Shape: "bubble", Label: "Where?", Width: 40, Height: 60, Style: "normal"},
		{ID: "inside", Kind: "annotation", Shape: "bubble", Label: "Here", Target: &Point{10, 10}, Width: 100, Height: 60, Style: "normal"},
		{ID: "edgeless", Kind: "edge", From: "box", To: "box", Points: []Point{{0, 0}, {1, 1}}, Style: "normal"},
		{ID: "boxless", Kind: "annotation", Shape: "bubble", Label: "At the edge", About: "edgeless", X: 300, Width: 100, Height: 60, Style: "normal"},
		{ID: "wordy", Kind: "annotation", Shape: "pin", Label: "1234", Width: 30, Height: 20, Style: "normal", Target: &Point{0, 0}},
		{ID: "loose", Kind: "annotation", Shape: "bracket", Label: "Spans", Width: 20, Height: 100, Style: "normal"},
		{ID: "odd", Kind: "annotation", Shape: "arrow", Width: 20, Height: 20, Style: "normal", Side: "up"},
		{ID: "marked", Kind: "annotation", Shape: "highlight", Width: 20, Height: 20, Style: "normal", LabelBox: &Box{Width: 10, Height: 10}},
	}
	err := d.Validate()
	if err == nil {
		t.Fatal("an invalid board was accepted")
	}
	for _, want := range []string{
		"box: color applies only to stickies, sections, and annotations",
		"box: about applies only to stickies and annotations",
		`untitled: color "orange" is not in the palette (yellow, pink, blue, green, purple, gray)`,
		"untitled: a section needs a label for its title tab",
		"blank: a sticky needs a positive width and height",
		"blank: a sticky takes no detail",
		"blank: a sticky takes no shape",
		"blank: a sticky needs label text",
		"self: about names the element itself",
		`ghost: about names unknown element "missing"`,
		"hidden: about names decorative element flourish, which readers never see",
		"quiet: a decorative element is hidden from readers, so what it is about would never be read",
		"lost: a bubble needs a width and height of at least 48",
		"lost: a bubble needs a target point or an about element for its pointer",
		"inside: bubble pointer tip (10, 10) lies inside the bubble",
		"boxless: bubble points at edgeless, which has no box; set an explicit target",
		"wordy: target applies only to bubble annotations",
		"wordy: a pin needs a label of 1-3 characters",
		"wordy: a pin is a circle: set height equal to width or omit it",
		"loose: a bracket label needs an explicit label_box",
		"loose: a bracket needs side left, right, top, or bottom",
		"odd: side applies only to bracket annotations",
		"odd: annotation shape must be bubble, pin, highlight, or bracket",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation lacks %q in:\n%v", want, err)
		}
	}
	if strings.Contains(err.Error(), "marked:") {
		t.Errorf("a highlight may carry a label_box: %v", err)
	}
}

func TestBoardRenderRefusesOverflow(t *testing.T) {
	d := board()
	for index := range d.Elements {
		switch d.Elements[index].ID {
		case "first":
			d.Elements[index].Label = "999"
		}
	}
	if _, err := Render(d, Options{}); err == nil || !strings.Contains(err.Error(), `pin label "999"`) {
		t.Fatalf("a pin label wider than its pin must be refused: %v", err)
	}
	d = board()
	d.Elements[d.Index("retry")].Label = "A section title far wider than its section"
	if _, err := Render(d, Options{}); err == nil || !strings.Contains(err.Error(), "retry: text overflow") {
		t.Fatalf("a section title wider than its section must be refused: %v", err)
	}
}

func TestBoardDescribe(t *testing.T) {
	var b strings.Builder
	Describe(board(), 0, 30).WriteText(&b)
	want := `
Sections:
  ingest "Ingest"
    members: queue, retry
  retry "Retry" in=ingest
    members: backoff, focus

Nodes:
  queue "Queue" shape=rect in=ingest
  backoff "Backoff" shape=rect in=retry
  store "Store" shape=datastore

Edges:
  write: backoff -> store

Notes:
  why: sticky about store "Writes are keyed by batch, so a retry overwrites itself."
    note: The key is the batch digest.
  hot: bubble about store "Hot path"
  slow: bubble about write "Waits up to 30s"
  first: pin about queue "1"
  second: pin about backoff "2"
  focus: highlight about backoff in=retry
  span: bracket about store "One record"

Showing elements 1-13 of 13.
`
	if got := b.String(); got != want {
		t.Fatalf("describe:\n%s\nwant:\n%s", got, want)
	}
	data, _ := json.Marshal(Describe(board(), 0, 30))
	for _, want := range []string{
		`{"id":"ingest","kind":"group","shape":"section","label":"Ingest","members":["queue","retry"]}`,
		`{"id":"first","kind":"annotation","shape":"pin","label":"1","about":"queue"}`,
		`{"id":"why","kind":"sticky","label":"Writes are keyed by batch, so a retry overwrites itself.","note":"The key is the batch digest.","about":"store"}`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("describe JSON lacks %s in %s", want, data)
		}
	}
	if strings.Contains(string(data), "color") || strings.Contains(string(data), "target") {
		t.Errorf("describe JSON must omit presentation and geometry: %s", data)
	}
}

func TestBoardEdit(t *testing.T) {
	d := board()
	if _, _, err := Edit(d, []Operation{{Op: "remove", ID: "store"}}); err == nil || !strings.Contains(err.Error(), "depends on store") {
		t.Fatalf("removing an annotated element must name its notes: %v", err)
	}
	removed, changed, err := Edit(d, []Operation{{Op: "remove", ID: "queue", Cascade: true}})
	if err != nil || removed.Index("first") >= 0 || strings.Join(changed, ",") != "first,queue" {
		t.Fatalf("cascade through about: %v %v", changed, err)
	}
	// Moving the section that holds the store's neighbour leaves the bubble
	// alone, but moving the store moves the tip of the bubble pointing at it.
	moved, changed, err := Edit(d, []Operation{{Op: "move", ID: "store", DX: -20}})
	if err != nil || strings.Join(changed, ",") != "hot,store" {
		t.Fatalf("a derived pointer follows its element: %v %v", changed, err)
	}
	if err := moved.Validate(); err != nil {
		t.Fatal(err)
	}
	_, changed, _ = Edit(d, []Operation{{Op: "move", ID: "ingest", DX: 5}})
	if strings.Join(changed, ",") != "ingest" {
		t.Fatalf("only bubbles without a target follow: %v", changed)
	}
	nested := board()
	nested.Elements = append(nested.Elements, Element{ID: "aside", Kind: "annotation", Shape: "bubble", Label: "Queued", About: "store", Parent: "retry", X: 25, Y: 180, Width: 200, Height: 60, Style: "caption"})
	if err := nested.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, changed, _ = Edit(nested, []Operation{{Op: "move", ID: "ingest", DX: 5}}); strings.Join(changed, ",") != "aside,ingest" {
		t.Fatalf("a bubble whose own section moves re-aims: %v", changed)
	}
	recolored, _, err := Edit(d, []Operation{{Op: "update", ID: "why", Set: json.RawMessage(`{"color":"green","about":null}`)}})
	if why, _ := recolored.Element("why"); err != nil || why.Color != "green" || why.About != "" {
		t.Fatalf("update color and clear about: %+v %v", why, err)
	}
	encoded, _ := Encode(d)
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if slow, _ := decoded.Element("slow"); slow.Target == nil || *slow.Target != (Point{60, -85}) || slow.About != "write" {
		t.Fatalf("stored source must round-trip board fields: %+v", slow)
	}
}

func TestSchemaAcceptsBoard(t *testing.T) {
	schema := compileSchema(t)
	data, _ := Encode(board())
	value, _ := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("a board source does not satisfy its schema: %v", err)
	}
	bad := strings.Replace(string(data), `"color": "blue"`, `"color": "orange"`, 1)
	value, _ = jsonschema.UnmarshalJSON(strings.NewReader(bad))
	if schema.Validate(value) == nil {
		t.Fatal("schema accepted a colour outside the palette")
	}
}

// The skill's board example is what an agent copies, so its elements must
// validate and render beside the nodes they annotate.
func TestSkillBoardExampleRenders(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "change-saga", "references", "diagrams.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	start := strings.Index(text, "- Board elements annotate a drawing.")
	start += strings.Index(text[start:], "```json\n") + len("```json\n")
	var elements []Element
	if err := json.Unmarshal([]byte("["+text[start:start+strings.Index(text[start:], "```")]+"]"), &elements); err != nil {
		t.Fatalf("board example: %v", err)
	}
	d := New()
	d.Elements = append([]Element{
		{ID: "queue", Kind: "node", Shape: "rect", Label: "Queue", Parent: "ingest", X: 30, Y: 70, Width: 200, Height: 80, Style: "normal"},
		{ID: "store", Kind: "node", Shape: "datastore", Label: "Store", X: 820, Y: 130, Width: 240, Height: 120, Style: "normal"},
	}, elements...)
	if _, err := Render(d, Options{Title: "Board"}); err != nil {
		t.Fatalf("the skill's board example does not render: %v", err)
	}
}
