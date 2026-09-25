package diagram

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Options carries the slide-level accessible name and summary.
type Options struct {
	Title       string
	Description string
}

// widthSlack keeps text measured with the pinned font inside its box when a
// viewer substitutes a system sans-serif of slightly different width.
const widthSlack = 1.04

const lineHeight = 1.25

var measurementFont = sync.OnceValues(func() (*sfnt.Font, error) {
	data, _ := Font()
	return sfnt.Parse(data)
})

// num formats coordinates rounded to thousandths, so the output bytes do not
// depend on floating-point contraction differences between CPU architectures.
func num(n float64) string {
	rounded := math.Round(n*1000) / 1000
	if rounded == 0 {
		rounded = 0 // normalize negative zero
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// Render validates d and produces its SVG. The output is deterministic for a
// given Document, Options, and Renderer: element order, attributes, and
// numbers are serialized canonically. Every element becomes a group whose id
// is the element ID, so slide Items select elements by ID.
func Render(d Document, options Options) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	face, err := measurementFont()
	if err != nil {
		return nil, fmt.Errorf("measurement font: %w", err)
	}
	r := renderer{doc: d, face: face}
	svg := newNode("svg", "xmlns", "http://www.w3.org/2000/svg", "viewBox", fmt.Sprintf("0 0 %s %s", num(d.Width), num(d.Height)),
		"role", "img", "aria-labelledby", "diagram-title", "data-renderer", Renderer)
	svg.add("title", "id", "diagram-title").text = options.Title
	if strings.TrimSpace(options.Description) != "" {
		svg.set("aria-describedby", "diagram-desc")
		svg.add("desc", "id", "diagram-desc").text = options.Description
	}
	defs := svg.add("defs")
	defs.add("style").text = fmt.Sprintf(`@font-face{font-family:"%s";src:url("%s") format("truetype")}text{font-family:"%s",system-ui,-apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;font-weight:400}`, FontFamily, FontPath, FontFamily)
	r.defs = defs
	usesIcons := false
	for _, e := range d.Elements {
		usesIcons = usesIcons || e.Icon != ""
	}
	if usesIcons {
		svg.add("metadata", "id", "diagram-notices").text = "Lucide icons (" + LucideRevision + "):\n" + IconLicense()
	}
	if d.Background != "" && d.Background != "none" {
		svg.add("rect", "width", num(d.Width), "height", num(d.Height), "fill", d.Background)
	}
	if err := r.draw("", svg); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	svg.write(&out, 0)
	return out.Bytes(), nil
}

type renderer struct {
	doc  Document
	face *sfnt.Font
	defs *node
}

func (r *renderer) draw(parent string, into *node) error {
	type ordered struct {
		element Element
		index   int
	}
	children := []ordered{}
	for index, e := range r.doc.Elements {
		if e.Parent == parent {
			children = append(children, ordered{e, index})
		}
	}
	sort.SliceStable(children, func(i, j int) bool {
		if children[i].element.Z != children[j].element.Z {
			return children[i].element.Z < children[j].element.Z
		}
		return children[i].index < children[j].index
	})
	for _, child := range children {
		if err := r.element(child.element, into); err != nil {
			return fmt.Errorf("%s: %w", child.element.ID, err)
		}
	}
	return nil
}

func (r *renderer) element(e Element, into *node) error {
	style, _ := r.doc.Style(e.Style)
	g := into.add("g", "id", e.ID, "data-diagram-kind", e.Kind, "transform", fmt.Sprintf("translate(%s %s)", num(e.X), num(e.Y)))
	if e.Decorative {
		g.set("aria-hidden", "true")
	} else {
		g.set("role", "group")
		g.set("aria-label", accessibleName(e))
		if strings.TrimSpace(e.Description) != "" {
			g.add("title").text = e.Description
		}
	}
	switch e.Kind {
	case "group":
		if e.Shape != "" {
			frame := g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "10")
			paint(frame, style)
			if e.Label != "" {
				if err := r.text(g, e.Label, Box{X: 18, Y: 12, Width: e.Width - 36, Height: style.FontSize * lineHeight}, style.FontSize, style.Ink, false, e.Align); err != nil {
					return err
				}
			}
		}
		return r.draw(e.ID, g)
	case "graphic":
		children, err := parseFragment(e.Fragment)
		if err != nil {
			return err
		}
		g.set("color", firstColor(style.Stroke, style.Ink))
		g.children = append(g.children, children...)
		return nil
	case "edge":
		return r.edge(e, style, g)
	case "text":
		return r.text(g, e.Label, Box{Width: e.Width, Height: e.Height}, style.FontSize, style.Ink, e.Wrap, e.Align)
	}
	return r.shape(e, style, g)
}

func accessibleName(e Element) string {
	switch {
	case strings.TrimSpace(e.Label) != "":
		return e.Label
	case e.Kind == "edge":
		return e.From + " to " + e.To
	default:
		return e.ID
	}
}

func firstColor(values ...string) string {
	for _, value := range values {
		if value != "" && value != "none" {
			return value
		}
	}
	return "currentColor"
}

func paint(n *node, style Style) {
	n.set("fill", style.Fill)
	n.set("stroke", style.Stroke)
	n.set("stroke-width", num(style.StrokeWidth))
	if style.Dash {
		n.set("stroke-dasharray", "7 6")
	}
}

func (r *renderer) edge(e Element, style Style, g *node) error {
	g.set("data-from", e.From)
	g.set("data-to", e.To)
	path := e.Path
	if path == "" {
		var b strings.Builder
		for index, p := range e.Points {
			if index == 0 {
				b.WriteString("M")
			} else {
				b.WriteString(" L")
			}
			b.WriteString(num(p.X) + " " + num(p.Y))
		}
		path = b.String()
	}
	line := g.add("path", "d", path, "fill", "none", "stroke", style.Stroke, "stroke-width", num(style.StrokeWidth), "stroke-linejoin", "round")
	if style.Dash {
		line.set("stroke-dasharray", "7 6")
	}
	if e.Head == "arrow" {
		size := e.HeadSize
		if size == 0 {
			size = 10
		}
		id := "diagram-head-" + e.ID
		marker := r.defs.add("marker", "id", id, "viewBox", "0 0 10 10", "refX", "9", "refY", "5", "markerWidth", num(size), "markerHeight", num(size), "markerUnits", "userSpaceOnUse", "orient", "auto")
		marker.add("path", "d", "M0 0L10 5L0 10Z", "fill", style.Stroke)
		line.set("marker-end", "url(#"+id+")")
	}
	if e.Label != "" {
		return r.text(g, e.Label, *e.LabelBox, style.FontSize, style.Ink, e.Wrap, e.Align)
	}
	return nil
}

func (r *renderer) shape(e Element, style Style, g *node) error {
	var body *node
	switch e.Shape {
	case "ellipse":
		body = g.add("ellipse", "cx", num(e.Width/2), "cy", num(e.Height/2), "rx", num(e.Width/2), "ry", num(e.Height/2))
	case "decision":
		body = g.add("path", "d", fmt.Sprintf("M%s 0L%s %sL%s %sL0 %sZ", num(e.Width/2), num(e.Width), num(e.Height/2), num(e.Width/2), num(e.Height), num(e.Height/2)))
	case "datastore":
		ry := 14.0
		body = g.add("path", "d", fmt.Sprintf("M0 %sC0 -%s %s -%s %s %sV%sC%s %s 0 %s 0 %sZ", num(ry), num(ry/3), num(e.Width), num(ry/3), num(e.Width), num(ry), num(e.Height-ry), num(e.Width), num(e.Height+ry/3), num(e.Height+ry/3), num(e.Height-ry)))
	default:
		body = g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "8")
	}
	paint(body, style)
	if e.Shape == "datastore" {
		rim := g.add("path", "d", fmt.Sprintf("M0 14C0 33 %s 33 %s 14", num(e.Width), num(e.Width)), "fill", "none", "stroke", style.Stroke, "stroke-width", num(style.StrokeWidth))
		if style.Dash {
			rim.set("stroke-dasharray", "7 6")
		}
	}
	left, top := 18.0, 18.0
	switch e.Shape {
	case "datastore":
		top = 40
	case "decision":
		left, top = e.Width*.25, e.Height*.35
	}
	if e.Icon != "" {
		size := e.IconSize
		if size == 0 {
			size = 24
		}
		icon, err := r.icon(e.Icon, left, top, size, style.Ink)
		if err != nil {
			return err
		}
		g.children = append(g.children, icon)
		left += size + 10
	}
	if e.LabelBox != nil {
		if err := r.text(g, e.Label, *e.LabelBox, style.FontSize, style.Ink, e.Wrap, e.Align); err != nil {
			return err
		}
		if e.Detail != "" {
			return r.text(g, e.Detail, Box{X: 18, Y: top, Width: e.Width - 36, Height: e.Height - top - 8}, 15, style.Ink, e.Wrap, e.Align)
		}
		return nil
	}
	width := e.Width - left - 16
	if e.Shape == "decision" {
		width = e.Width * .5
	}
	labelHeight := e.Height - top - 12
	if e.Detail != "" {
		labelHeight = math.Min(labelHeight, 32)
	}
	if err := r.text(g, e.Label, Box{X: left, Y: top, Width: width, Height: labelHeight}, style.FontSize, style.Ink, e.Wrap, e.Align); err != nil {
		return err
	}
	if e.Detail != "" {
		return r.text(g, e.Detail, Box{X: 18, Y: top + 40, Width: e.Width - 36, Height: e.Height - top - 48}, 15, style.Ink, e.Wrap, e.Align)
	}
	return nil
}

var iconRootAttributes = map[string]bool{"viewBox": true, "fill": true, "stroke": true, "stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true}

func (r *renderer) icon(name string, x, y, size float64, ink string) (*node, error) {
	data, err := Icon(name)
	if err != nil {
		return nil, err
	}
	attrs, children, err := parseMarkup(string(data))
	if err != nil {
		return nil, fmt.Errorf("icon %s: %w", name, err)
	}
	icon := newNode("svg", "x", num(x), "y", num(y), "width", num(size), "height", num(size), "color", ink, "aria-hidden", "true")
	for _, attr := range attrs {
		if attr.Name.Space == "" && iconRootAttributes[attr.Name.Local] {
			icon.set(attr.Name.Local, attr.Value)
		}
	}
	icon.children = children
	return icon, nil
}

// text draws s inside box, refusing overflow instead of shrinking or moving
// anything. Newlines are explicit line breaks; wrap breaks at spaces.
func (r *renderer) text(g *node, s string, box Box, size float64, ink string, wrap bool, align string) error {
	if s == "" {
		return nil
	}
	if box.Width <= 0 || box.Height <= 0 {
		return fmt.Errorf("text %q has no room: its box is %sx%s", s, num(box.Width), num(box.Height))
	}
	lines := []string{}
	for _, line := range strings.Split(s, "\n") {
		if !wrap {
			lines = append(lines, line)
			continue
		}
		current := ""
		for _, word := range strings.Fields(line) {
			next := word
			if current != "" {
				next = current + " " + word
			}
			width, err := r.measure(next, size)
			if err != nil {
				return err
			}
			if width*widthSlack > box.Width && current != "" {
				lines = append(lines, current)
				current = word
			} else {
				current = next
			}
		}
		lines = append(lines, current)
	}
	if need := float64(len(lines)) * size * lineHeight; need > box.Height+.01 {
		return fmt.Errorf("text overflow: %q needs height %.1f, its box has %.1f", s, need, box.Height)
	}
	x, anchor := box.X, ""
	switch align {
	case "middle":
		x, anchor = box.X+box.Width/2, "middle"
	case "end":
		x, anchor = box.X+box.Width, "end"
	}
	t := g.add("text", "x", num(x), "y", num(box.Y+size), "font-size", num(size), "fill", ink, "xml:space", "preserve")
	if anchor != "" {
		t.set("text-anchor", anchor)
	}
	for index, line := range lines {
		width, err := r.measure(line, size)
		if err != nil {
			return err
		}
		if width*widthSlack > box.Width+.01 {
			return fmt.Errorf("text overflow: %q needs width %.1f, its box has %.1f", line, width*widthSlack, box.Width)
		}
		t.add("tspan", "x", num(x), "y", num(box.Y+size+float64(float64(index)*size*lineHeight))).text = line
	}
	return nil
}

func (r *renderer) measure(text string, size float64) (float64, error) {
	var buffer sfnt.Buffer
	var width fixed.Int26_6
	for _, char := range text {
		index, err := r.face.GlyphIndex(&buffer, char)
		if err != nil {
			return 0, err
		}
		if index == 0 && !unicode.IsSpace(char) {
			return 0, fmt.Errorf("the diagram font has no glyph for %q (U+%04X)", char, char)
		}
		advance, err := r.face.GlyphAdvance(&buffer, index, fixed.Int26_6(math.Round(size*64)), font.HintingNone)
		if err != nil {
			return 0, err
		}
		width += advance
	}
	return float64(width) / 64, nil
}
