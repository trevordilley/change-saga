package diagram

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// revealed is the sample diagram fading in: the store enters with the author,
// the decorative lifeline has its own step, the publish edge asks for a step
// earlier than its group so it enters with alt, and the untouched decorative
// heading is present from the start.
func revealed() Document {
	d := sample()
	d.Reveal = "fade"
	steps := map[string]int{"store": 1, "lifeline": 2, "publish": 2}
	for index := range d.Elements {
		d.Elements[index].Step = steps[d.Elements[index].ID]
	}
	return d
}

func TestRevealValidation(t *testing.T) {
	if err := revealed().Validate(); err != nil {
		t.Fatalf("a valid reveal was refused: %v", err)
	}
	d := sample()
	d.Reveal = "slide"
	d.Elements[1].Step = MaxRevealStep + 1
	d.Elements[2].Step = -1
	err := d.Validate()
	for _, want := range []string{
		`reveal "slide" is not supported; use fade, or omit reveal for a static drawing`,
		"author: step must be 1-1000",
		"store: step must be 1-1000",
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
	d = sample()
	d.Elements[1].Step = 2
	d.Elements[3].Step = 1
	err = d.Validate()
	for _, id := range []string{"author", "lifeline"} {
		want := id + `: step orders the reveal, but the diagram does not set one; set it with the canvas operation {"reveal":"fade"} or remove the step`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in %v", want, err)
		}
	}
}

func TestPlanRevealResolvesSteps(t *testing.T) {
	plan := planReveal(revealed())
	want := map[string][2]int{ // id: step, animates on its own (1)
		"heading":  {0, 0},
		"author":   {1, 1},
		"store":    {1, 1},
		"lifeline": {2, 1},
		"alt":      {3, 1},
		"commit":   {4, 1},
		"publish":  {3, 0},
	}
	for id, expected := range want {
		animates := 0
		if plan.animate[id] {
			animates = 1
		}
		if got := [2]int{plan.step[id], animates}; got != expected {
			t.Errorf("%s: step, animates = %v, want %v", id, got, expected)
		}
	}
	if plan.steps != 4 {
		t.Errorf("steps = %d, want 4", plan.steps)
	}
	if plan := planReveal(sample()); plan.steps != 0 || len(plan.animate) != 0 {
		t.Errorf("a diagram without reveal must plan nothing: %+v", plan)
	}
	// Gaps between authored steps cost no time; a decorative element inside a
	// group enters with it.
	d := sample()
	d.Reveal = "fade"
	d.Elements = append(d.Elements, Element{ID: "tick", Kind: "graphic", Parent: "alt", Style: "normal", Decorative: true, Fragment: `<circle r="2"/>`})
	for index := range d.Elements {
		d.Elements[index].Step = map[string]int{"author": 10, "store": 900, "alt": 900}[d.Elements[index].ID]
	}
	plan = planReveal(d)
	if plan.step["author"] != 1 || plan.step["store"] != 2 || plan.step["alt"] != 2 || plan.step["commit"] != 2 || plan.step["tick"] != 2 || plan.animate["tick"] || plan.steps != 2 {
		t.Errorf("ranked steps: %+v", plan)
	}
}

// revealMarkup is everything reveal adds to a rendering.
var revealMarkup = regexp.MustCompile(`\n *<style>@media \(prefers-reduced-motion[^<]*</style>|\n *<g id="diagram-reveal(-replay)?"/>| data-reveal-step="\d+"`)

func TestRenderWithRevealIsDeterministicAndMatchesGolden(t *testing.T) {
	options := Options{Title: "Publishing one batch", Description: "One record publishes source and SVG together."}
	first, err := Render(revealed(), options)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(revealed(), options)
	if !bytes.Equal(first, second) {
		t.Fatal("rendering with a reveal is not deterministic")
	}
	golden := filepath.Join("testdata", "reveal.svg")
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
		t.Fatal("renderer output with a reveal changed; if intended, run with -update")
	}
	svg := string(first)
	for _, want := range []string{
		`<g id="diagram-reveal"/>`, `<g id="diagram-reveal-replay"/>`,
		`@media (prefers-reduced-motion:no-preference){#diagram-reveal:target~* [data-reveal-step],#diagram-reveal:target~[data-reveal-step]{animation-name:diagram-reveal-fade;`,
		`[data-reveal-step="4"]{animation-delay:1050ms}`, `@keyframes diagram-reveal-replay-fade{from{opacity:0}}`,
		`id="author" data-diagram-kind="node" transform="translate(76 110)" data-reveal-step="1"`,
		`id="commit" data-diagram-kind="node" transform="translate(40 80)" data-reveal-step="4"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	for _, absent := range []string{`id="heading" data-diagram-kind="text" transform="translate(60 28)" data-reveal-step`, `id="publish" data-diagram-kind="edge" transform="translate(0 0)" data-reveal-step`} {
		if strings.Contains(svg, absent) {
			t.Errorf("SVG must not animate %s", absent)
		}
	}
	if strings.Index(svg, `<g id="diagram-reveal-replay"/>`) > strings.Index(svg, `<g id="heading"`) {
		t.Error("the anchors must precede every element, which the sibling selectors rely on")
	}
	// Reveal adds only its stylesheet, anchors, and step attributes: removing
	// them restores the exact bytes of the static drawing.
	plain, err := Render(sample(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(revealMarkup.ReplaceAll(first, nil), plain) {
		t.Fatal("a reveal changed more of the rendering than its own markup")
	}
}

func TestRevealStaggerFitsLongDiagrams(t *testing.T) {
	d := New()
	d.Reveal = "fade"
	for index := range 40 {
		d.Elements = append(d.Elements, Element{ID: "n" + string(rune('a'+index/26)) + string(rune('a'+index%26)), Kind: "node", Shape: "rect", Width: 10, Height: 10, Style: "normal"})
	}
	svg, err := Render(d, Options{Title: "Many"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(svg), `[data-reveal-step="40"]{animation-delay:4875ms}`) {
		t.Fatal("the last of 40 steps must start within the reveal span")
	}
}

func TestRevealDescribeAndEditRoundTrip(t *testing.T) {
	var b strings.Builder
	Describe(revealed(), 0, 30).WriteText(&b)
	text := b.String()
	for _, want := range []string{
		"\nReveal: fade in 4 steps; step=N is when an element appears, equal steps together.\n",
		"  alt \"alt  [snapshot matches]\" shape=boundary step=3\n",
		"  author \"Author\" shape=rect icon=lucide:user step=1\n",
		"  publish: author -> store \"publish  record\" in=alt step=3\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q in:\n%s", want, text)
		}
	}
	data, _ := json.Marshal(Describe(revealed(), 0, 30))
	for _, want := range []string{`"id":"commit","kind":"node","shape":"ellipse","label":"merge","description":"Merge after review","parent":"alt","step":4}`, `"reveal":"fade","steps":4}`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("describe JSON lacks %s: %s", want, data)
		}
	}
	var plain strings.Builder
	Describe(sample(), 0, 30).WriteText(&plain)
	if data, _ := json.Marshal(Describe(sample(), 0, 30)); strings.Contains(plain.String(), "step") || strings.Contains(plain.String(), "Reveal") || strings.Contains(string(data), "step") || strings.Contains(string(data), "reveal") {
		t.Fatal("describe of a static diagram must not mention a reveal")
	}

	on, changed, err := Edit(sample(), []Operation{{Op: "canvas", Set: json.RawMessage(`{"reveal":"fade"}`)}, {Op: "update", ID: "store", Set: json.RawMessage(`{"step":1}`)}})
	if err != nil || on.Reveal != "fade" || len(changed) != len(on.Elements) {
		t.Fatalf("canvas reveal: %v %v", changed, err)
	}
	if store, _ := on.Element("store"); store.Step != 1 || on.Validate() != nil {
		t.Fatalf("step: %+v %v", store, on.Validate())
	}
	encoded, _ := Encode(on)
	if decoded, err := Decode(encoded); err != nil || decoded.Reveal != "fade" || decoded.Elements[2].Step != 1 {
		t.Fatalf("stored source must round-trip the reveal: %v", err)
	}
	if _, changed, _ := Edit(on, []Operation{{Op: "canvas", Set: json.RawMessage(`{"reveal":"fade","width":1280}`)}}); len(changed) != 0 {
		t.Fatalf("an unchanged reveal re-renders nothing: %v", changed)
	}
	off, _, err := Edit(on, []Operation{{Op: "canvas", Set: json.RawMessage(`{"reveal":""}`)}})
	if err != nil || off.Validate() == nil || !strings.Contains(off.Validate().Error(), "store: step orders the reveal") {
		t.Fatalf("clearing the reveal must name the leftover step: %v %v", err, off.Validate())
	}
	off, _, _ = Edit(off, []Operation{{Op: "update", ID: "store", Set: json.RawMessage(`{"step":null}`)}})
	if encoded, _ := Encode(off); off.Validate() != nil || strings.Contains(string(encoded), "reveal") || strings.Contains(string(encoded), "step") {
		t.Fatalf("a cleared reveal must leave no field in the stored source: %v\n%s", off.Validate(), encoded)
	}
}

func TestSchemaBoundsReveal(t *testing.T) {
	schema := compileSchema(t)
	data, _ := Encode(revealed())
	value, _ := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("a revealed source does not satisfy its schema: %v", err)
	}
	for _, bad := range []string{
		`{"version":1,"width":10,"height":10,"reveal":"slide","elements":[]}`,
		`{"version":1,"width":10,"height":10,"reveal":"fade","elements":[{"id":"a","kind":"text","x":0,"y":0,"style":"normal","step":0}]}`,
		`{"version":1,"width":10,"height":10,"reveal":"fade","elements":[{"id":"a","kind":"text","x":0,"y":0,"style":"normal","step":1.5}]}`,
	} {
		value, _ := jsonschema.UnmarshalJSON(strings.NewReader(bad))
		if schema.Validate(value) == nil {
			t.Errorf("schema accepted %s", bad)
		}
	}
	raw, _ := os.ReadFile(schemaPath)
	var document struct {
		Properties struct {
			Reveal struct {
				Enum []string `json:"enum"`
			} `json:"reveal"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || strings.Join(document.Properties.Reveal.Enum, ",") != strings.Join(RevealModes(), ",") {
		t.Fatalf("reveal enum: schema %v, runtime %v (%v)", document.Properties.Reveal.Enum, RevealModes(), err)
	}
}
