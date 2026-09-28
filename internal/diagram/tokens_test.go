package diagram

import (
	"regexp"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/theme"
)

// themed is a small board whose author style references tokens by name.
func themed() Document {
	d := New()
	d.Styles = map[string]Style{
		"brand":  {Fill: "@diagram-primary-fill", Stroke: "@accent", Ink: "@ink", StrokeWidth: 2, FontSize: 20},
		"custom": {Fill: "#123456", Stroke: "none", Ink: "#abcdef", StrokeWidth: 1, FontSize: 18},
	}
	d.Elements = []Element{
		{ID: "api", Kind: "node", Shape: "rect", Label: "API", X: 40, Y: 40, Width: 200, Height: 80, Style: "brand"},
		{ID: "db", Kind: "node", Shape: "rect", Label: "DB", X: 400, Y: 40, Width: 200, Height: 80, Style: "custom"},
		{ID: "call", Kind: "edge", From: "api", To: "db", Points: []Point{{240, 80}, {400, 80}}, Head: "arrow", Tail: "diamond", Style: "normal"},
		{ID: "pin", Kind: "annotation", Shape: "pin", Label: "1", X: 20, Y: 20, Width: 32, Style: "normal", About: "api"},
		{ID: "lane", Kind: "group", Shape: "section", Label: "Lane", Color: "green", X: 20, Y: 200, Width: 400, Height: 200, Style: "normal"},
		{ID: "memo", Kind: "sticky", Label: "Remember", X: 500, Y: 200, Width: 160, Height: 160, Style: "normal", About: "db"},
	}
	return d
}

func TestStyleColoursMayReferenceColourTokens(t *testing.T) {
	svg, err := Render(themed(), Options{Title: "Themed"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(svg)
	for _, want := range []string{
		// An author's token reference resolves to its light value and a var().
		`fill="#edf5ff" stroke="#0969da" stroke-width="2" style="fill:var(--diagram-primary-fill,#edf5ff);stroke:var(--accent,#0969da)"`,
		`fill="#1f2328" xml:space="preserve" style="fill:var(--ink,#1f2328)"`,
		// A custom hex colour stays exactly as authored.
		`fill="#123456" stroke="none" stroke-width="1"/>`,
		// The default canvas, markers, palette, and white-on-accent text all read tokens.
		`fill="#fafaf8" style="fill:var(--diagram-canvas,#fafaf8)"`,
		`fill="#64748b" style="fill:var(--diagram-normal-stroke,#64748b)"`,
		`style="fill:var(--diagram-canvas,#fafaf8);stroke:var(--diagram-normal-stroke,#64748b)"`,
		`style="fill:var(--diagram-gray-accent,#475569);stroke:var(--diagram-on-accent,#ffffff)"`,
		`style="fill:var(--diagram-green-accent,#15803d)"`,
		`style="fill:var(--diagram-yellow-sticky,#fff3a3)"`,
		`style="flood-color:var(--diagram-shadow,#0f172a)"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("themed SVG lacks %s", want)
		}
	}
	if strings.Contains(out, `="@`) {
		t.Error("a token reference reached the SVG unresolved")
	}
	// Every var() names a token the SVG declares, so no colour relies on the
	// host page, and every light fallback is the contract's light value.
	declared := regexp.MustCompile(`--([a-z0-9-]+):`).FindAllStringSubmatch(out, -1)
	names := map[string]bool{}
	for _, match := range declared {
		names[match[1]] = true
	}
	for _, match := range regexp.MustCompile(`var\(--([a-z0-9-]+),([^)]+)\)`).FindAllStringSubmatch(out, -1) {
		token, ok := theme.Lookup(match[1])
		if !ok || !names[match[1]] || token.Light != match[2] {
			t.Errorf("var(--%s,%s) is undeclared or not the token's light value", match[1], match[2])
		}
	}
}

func TestStandaloneSVGDeclaresDarkTokens(t *testing.T) {
	svg, err := Render(themed(), Options{Title: "Themed"})
	if err != nil {
		t.Fatal(err)
	}
	out := string(svg)
	light, dark, found := strings.Cut(out, "@media (prefers-color-scheme:dark){:root{")
	if !found {
		t.Fatal("the SVG declares no dark scheme")
	}
	if !strings.Contains(light, ":root{color-scheme:light dark;") {
		t.Error("the SVG does not opt into both color schemes")
	}
	dark, _, _ = strings.Cut(dark, "}}")
	for _, name := range []string{"diagram-canvas", "diagram-primary-fill", "accent", "ink", "diagram-on-accent", "diagram-green-accent", "diagram-yellow-sticky"} {
		token, _ := theme.Lookup(name)
		if !strings.Contains(light, "--"+name+":"+token.Light+";") {
			t.Errorf("light declarations lack %s", name)
		}
		if !strings.Contains(dark, "--"+name+":"+token.Dark+";") {
			t.Errorf("dark declarations lack %s", name)
		}
	}
	if strings.Contains(dark, "--diagram-pink-") {
		t.Error("the SVG declares tokens it never uses")
	}
}

func TestTokenReferencesAreValidatedAgainstTheContract(t *testing.T) {
	for _, bad := range []string{"@nope", "@diagram-", "@mono", "@radius", "@Accent", "accent", "#fff"} {
		d := themed()
		d.Styles["brand"] = Style{Fill: bad, Stroke: "none", Ink: "@ink", StrokeWidth: 1, FontSize: 20}
		if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "colour token") {
			t.Errorf("style fill %q: want a colour-token error, got %v", bad, err)
		}
	}
	d := themed()
	d.Background = "@bg-subtle"
	if err := d.Validate(); err != nil {
		t.Fatalf("a token background: %v", err)
	}
	d.Background = "@ui"
	if err := d.Validate(); err == nil {
		t.Fatal("a font token is not a background colour")
	}
	schema := compileSchema(t)
	for value, ok := range map[string]bool{"@diagram-primary-fill": true, "#a1b2c3": true, "none": true, "@": false, "red": false} {
		d := themed()
		d.Background = value
		data, _ := Encode(d)
		instance, _ := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
		if err := schema.Validate(instance); (err == nil) != ok {
			t.Errorf("schema on background %q: %v", value, err)
		}
	}
}
