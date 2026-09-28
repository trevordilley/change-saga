package theme

import (
	"fmt"
	"math"
)

// MinimumContrast is WCAG 2 AA's ratio for body text.
const MinimumContrast = 4.5

// Pair is one text colour on the background it is read against.
type Pair struct {
	Ink        string `json:"ink"`
	Background string `json:"background"`
}

// ContrastPairs are the text and background pairs a theme must keep
// readable: the reviewer's ink and muted text on its background, the primary
// button, each review decision state's text on its chip, each diagram
// style's text on its fill, and each palette colour's ink on its sticky note
// and the text on its accent (a section's title tab, a pin's number).
func ContrastPairs() []Pair {
	pairs := []Pair{
		{"ink", "bg"}, {"muted", "bg"}, {"ink", "bg-subtle"}, {"primary-ink", "primary-bg"},
		{"diagram-normal-ink", "diagram-normal-fill"}, {"diagram-primary-ink", "diagram-primary-fill"},
		{"diagram-warning-ink", "diagram-warning-fill"}, {"diagram-secondary-ink", "diagram-canvas"},
		{"diagram-secondary-ink", "diagram-boundary-fill"}, {"diagram-title-ink", "diagram-canvas"},
		{"review-approved-ink", "review-approved-bg"}, {"review-changes-ink", "review-changes-bg"},
		{"review-none-ink", "review-none-bg"}, {"review-stale-ink", "review-stale-bg"}, {"review-stale-ink", "bg"},
	}
	for _, color := range PaletteColors {
		pairs = append(pairs, Pair{"diagram-" + color + "-ink", "diagram-" + color + "-sticky"}, Pair{"diagram-on-accent", "diagram-" + color + "-accent"})
	}
	return pairs
}

// PaletteColors are the diagram palette's colour names, in contract order.
var PaletteColors = []string{"yellow", "pink", "blue", "green", "purple", "gray"}

// ContrastResult is one pair's ratio in one scheme.
type ContrastResult struct {
	Scheme     string  `json:"scheme"`
	Ink        string  `json:"ink"`
	Background string  `json:"background"`
	InkValue   string  `json:"ink_value"`
	BgValue    string  `json:"background_value"`
	Ratio      float64 `json:"ratio"`
	Passes     bool    `json:"passes"`
}

func (result ContrastResult) String() string {
	return fmt.Sprintf("%s: --%s %s on --%s %s is %.2f:1, below %.1f:1", result.Scheme, result.Ink, result.InkValue, result.Background, result.BgValue, result.Ratio, MinimumContrast)
}

// Contrast measures every key pair in light and dark mode with the theme
// applied. A translucent colour is composited over what it sits on: the
// background over the page (and the page over white or black), the ink over
// its background.
func (file *File) Contrast() []ContrastResult {
	var results []ContrastResult
	for _, scheme := range []string{"light", "dark"} {
		values := file.Resolved(scheme)
		base := Color{R: 1, G: 1, B: 1, A: 1}
		if scheme == "dark" {
			base = Color{A: 1}
		}
		color := func(name string) Color {
			parsed, err := ParseColor(values[name])
			if err != nil {
				return Color{}
			}
			return parsed
		}
		page := color("bg").over(base)
		for _, pair := range ContrastPairs() {
			background := color(pair.Background).over(page)
			ratio := contrastRatio(color(pair.Ink).over(background), background)
			results = append(results, ContrastResult{
				Scheme: scheme, Ink: pair.Ink, Background: pair.Background,
				InkValue: values[pair.Ink], BgValue: values[pair.Background],
				Ratio: math.Floor(ratio*100) / 100, Passes: ratio >= MinimumContrast,
			})
		}
	}
	return results
}

// over composites color onto an opaque base.
func (color Color) over(base Color) Color {
	mix := func(top, bottom float64) float64 { return top*color.A + bottom*(1-color.A) }
	return Color{R: mix(color.R, base.R), G: mix(color.G, base.G), B: mix(color.B, base.B), A: 1}
}

func luminance(color Color) float64 {
	linear := func(v float64) float64 {
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(color.R) + 0.7152*linear(color.G) + 0.0722*linear(color.B)
}

func contrastRatio(a, b Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
