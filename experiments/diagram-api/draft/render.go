package draft

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/beevik/etree"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type InputReader func(Asset) ([]byte, error)

func num(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }
func el(parent *etree.Element, tag string, attrs ...string) *etree.Element {
	e := parent.CreateElement(tag)
	for i := 0; i < len(attrs); i += 2 {
		e.CreateAttr(attrs[i], attrs[i+1])
	}
	return e
}
func safeFragment(s string) (*etree.Element, error) {
	d := etree.NewDocument()
	if err := d.ReadFromString("<g>" + s + "</g>"); err != nil {
		return nil, err
	}
	var walk func(*etree.Element) error
	walk = func(e *etree.Element) error {
		switch e.Tag {
		case "g", "svg", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "tspan", "title", "desc":
		default:
			return fmt.Errorf("custom SVG element %s unsupported in spike", e.Tag)
		}
		for _, a := range e.Attr {
			if a.Space != "" || strings.HasPrefix(strings.ToLower(a.Key), "on") || a.Key == "id" || a.Key == "href" || a.Key == "style" || strings.Contains(a.Value, "url(") {
				return fmt.Errorf("custom SVG attribute %s unsupported in spike", a.Key)
			}
		}
		for _, c := range e.ChildElements() {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	return d.Root(), walk(d.Root())
}

func Render(d Document, read InputReader) ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	fpin, ok := d.Assets["font:go-regular"]
	if !ok {
		return nil, fmt.Errorf("missing font pin")
	}
	fb, err := read(fpin)
	if err != nil {
		return nil, err
	}
	f, err := sfnt.Parse(fb)
	if err != nil {
		return nil, err
	}
	doc := etree.NewDocument()
	svg := doc.CreateElement("svg")
	svg.CreateAttr("xmlns", "http://www.w3.org/2000/svg")
	svg.CreateAttr("viewBox", fmt.Sprintf("0 0 %s %s", num(d.Width), num(d.Height)))
	svg.CreateAttr("role", "img")
	svg.CreateAttr("aria-labelledby", "draft-title")
	el(svg, "title", "id", "draft-title").SetText(d.Title)
	defs := el(svg, "defs")
	css := el(defs, "style")
	css.SetText("@font-face{font-family:DraftGo;src:url(data:font/ttf;base64," + base64.StdEncoding.EncodeToString(fb) + ") format('truetype')}text{font-family:DraftGo;font-weight:400}")
	notices := map[string]bool{}
	for _, a := range d.Assets {
		notices[a.License] = true
	}
	el(svg, "metadata", "id", "draft-notices").SetText(strings.Join(keys(notices), "\n"))
	el(svg, "rect", "width", num(d.Width), "height", num(d.Height), "fill", d.Background)
	var draw func(string, *etree.Element) error
	draw = func(parent string, into *etree.Element) error {
		ids := []string{}
		for id, e := range d.Elements {
			if e.Parent == parent {
				ids = append(ids, id)
			}
		}
		sort.Slice(ids, func(i, j int) bool {
			a, b := d.Elements[ids[i]], d.Elements[ids[j]]
			if a.Z != b.Z {
				return a.Z < b.Z
			}
			return a.ID < b.ID
		})
		for _, id := range ids {
			e := d.Elements[id]
			s := d.Styles[e.Style]
			g := el(into, "g", "id", id, "data-item-id", id, "data-kind", e.Kind, "transform", fmt.Sprintf("translate(%s %s)", num(e.X), num(e.Y)))
			if e.Decorative {
				g.CreateAttr("aria-hidden", "true")
			} else {
				g.CreateAttr("role", "group")
				g.CreateAttr("aria-label", e.Label)
				el(g, "title").SetText(e.Description)
			}
			if e.Kind == "group" {
				if err := draw(id, g); err != nil {
					return err
				}
				continue
			}
			if e.Kind == "graphic" {
				frag, err := safeFragment(e.Fragment)
				if err != nil {
					return err
				}
				g.AddChild(frag)
				continue
			}
			if e.Kind == "edge" {
				g.CreateAttr("data-from", e.From)
				g.CreateAttr("data-to", e.To)
				path := e.Path
				if path == "" {
					for i, p := range e.Points {
						cmd := " L"
						if i == 0 {
							cmd = "M"
						}
						path += cmd + num(p.X) + " " + num(p.Y)
					}
				}
				p := el(g, "path", "d", path, "fill", "none", "stroke", s.Stroke, "stroke-width", num(s.StrokeWidth), "stroke-linejoin", "round")
				if s.Dash {
					p.CreateAttr("stroke-dasharray", "7 6")
				}
				if e.Head == "arrow" {
					size := e.HeadSize
					if size == 0 {
						size = 10
					}
					mid := "draft-head-" + id
					m := el(defs, "marker", "id", mid, "viewBox", "0 0 10 10", "refX", "9", "refY", "5", "markerWidth", num(size), "markerHeight", num(size), "markerUnits", "userSpaceOnUse", "orient", "auto")
					el(m, "path", "d", "M0 0L10 5L0 10Z", "fill", s.Stroke)
					p.CreateAttr("marker-end", "url(#"+mid+")")
				}
				if e.Label != "" {
					if err := drawText(g, f, e.Label, e.LabelAt.X, e.LabelAt.Y, e.W, max(e.H, 30), s.FontSize, s.Ink, false); err != nil {
						return nilElementError(id, err)
					}
				}
				continue
			}
			if e.Kind == "text" {
				if err := drawText(g, f, e.Label, 0, 0, e.W, e.H, s.FontSize, s.Ink, e.Wrap); err != nil {
					return nilElementError(id, err)
				}
				continue
			}
			var body *etree.Element
			switch e.Shape {
			case "ellipse":
				body = el(g, "ellipse", "cx", num(e.W/2), "cy", num(e.H/2), "rx", num(e.W/2), "ry", num(e.H/2))
			case "decision":
				body = el(g, "path", "d", fmt.Sprintf("M%s 0L%s %sL%s %sL0 %sZ", num(e.W/2), num(e.W), num(e.H/2), num(e.W/2), num(e.H), num(e.H/2)))
			case "datastore":
				ry := 14.0
				body = el(g, "path", "d", fmt.Sprintf("M0 %sC0 -%s %s -%s %s %sV%sC%s %s 0 %s 0 %sZ", num(ry), num(ry/3), num(e.W), num(ry/3), num(e.W), num(ry), num(e.H-ry), num(e.W), num(e.H+ry/3), num(e.H+ry/3), num(e.H-ry)))
			default:
				body = el(g, "rect", "width", num(e.W), "height", num(e.H), "rx", "8")
			}
			body.CreateAttr("fill", s.Fill)
			body.CreateAttr("stroke", s.Stroke)
			body.CreateAttr("stroke-width", num(s.StrokeWidth))
			if s.Dash {
				body.CreateAttr("stroke-dasharray", "7 6")
			}
			if e.Shape == "datastore" {
				el(g, "path", "d", fmt.Sprintf("M0 14C0 33 %s 33 %s 14", num(e.W), num(e.W)), "fill", "none", "stroke", s.Stroke, "stroke-width", num(s.StrokeWidth))
			}
			left, top := 18.0, 18.0
			if e.Shape == "datastore" {
				top = 40
			}
			if e.Shape == "decision" {
				left = e.W * .25
				top = e.H * .35
			}
			if e.Icon != "" {
				b, err := read(d.Assets[e.Icon])
				if err != nil {
					return err
				}
				iconDoc := etree.NewDocument()
				if err := iconDoc.ReadFromBytes(b); err != nil {
					return err
				}
				icon := iconDoc.Root()
				if icon == nil || icon.Tag != "svg" {
					return fmt.Errorf("invalid icon SVG")
				}
				size := e.IconSize
				if size == 0 {
					size = 24
				}
				icon.CreateAttr("x", num(left))
				icon.CreateAttr("y", num(top))
				icon.CreateAttr("width", num(size))
				icon.CreateAttr("height", num(size))
				icon.CreateAttr("color", s.Ink)
				icon.CreateAttr("aria-hidden", "true")
				g.AddChild(icon.Copy())
				left += size + 10
			}
			width := e.W - left - 16
			if e.Shape == "decision" {
				width = e.W * .5
			}
			labelHeight := e.H - top - 12
			if e.Detail != "" {
				labelHeight = min(labelHeight, 32)
			}
			if err := drawText(g, f, e.Label, left, top, width, labelHeight, s.FontSize, s.Ink, e.Wrap); err != nil {
				return nilElementError(id, err)
			}
			if e.Detail != "" {
				if err := drawText(g, f, e.Detail, 18, top+40, e.W-36, e.H-top-48, 15, s.Ink, e.Wrap); err != nil {
					return nilElementError(id, err)
				}
			}
		}
		return nil
	}
	if err := draw("", svg); err != nil {
		return nil, err
	}
	doc.Indent(2)
	b, err := doc.WriteToBytes()
	if err != nil {
		return nil, err
	}
	if err := CheckSelectors(d, b); err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
func nilElementError(id string, err error) error { return fmt.Errorf("%s: %w", id, err) }
func textWidth(f *sfnt.Font, text string, size float64) (float64, error) {
	var buf sfnt.Buffer
	var w fixed.Int26_6
	for _, r := range text {
		idx, err := f.GlyphIndex(&buf, r)
		if err != nil {
			return 0, err
		}
		if idx == 0 && !unicode.IsSpace(r) {
			return 0, fmt.Errorf("font lacks glyph %U", r)
		}
		a, err := f.GlyphAdvance(&buf, idx, fixed.Int26_6(math.Round(size*64)), font.HintingNone)
		if err != nil {
			return 0, err
		}
		w += a
	}
	return float64(w) / 64, nil
}
func drawText(g *etree.Element, f *sfnt.Font, text string, x, y, w, h, size float64, ink string, wrap bool) error {
	if text == "" {
		return nil
	}
	if w <= 0 || h <= 0 {
		return fmt.Errorf("empty text box")
	}
	lines := []string{}
	for _, line := range strings.Split(text, "\n") {
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
			width, err := textWidth(f, next, size)
			if err != nil {
				return err
			}
			if width > w && current != "" {
				lines = append(lines, current)
				current = word
			} else {
				current = next
			}
		}
		lines = append(lines, current)
	}
	if float64(len(lines))*size*1.25 > h+.01 {
		return fmt.Errorf("text overflow: need height %.1f, have %.1f", float64(len(lines))*size*1.25, h)
	}
	t := el(g, "text", "x", num(x), "y", num(y+size), "font-size", num(size), "fill", ink)
	for i, line := range lines {
		width, err := textWidth(f, line, size)
		if err != nil {
			return err
		}
		if width > w+.01 {
			return fmt.Errorf("text overflow: %q needs width %.1f, have %.1f", line, width, w)
		}
		el(t, "tspan", "x", num(x), "y", num(y+size+float64(i)*size*1.25)).SetText(line)
	}
	return nil
}
func CheckSelectors(d Document, b []byte) error {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(b); err != nil {
		return err
	}
	ids := map[string]int{}
	var walk func(*etree.Element)
	walk = func(e *etree.Element) {
		if id := e.SelectAttrValue("id", ""); id != "" {
			ids[id]++
		}
		for _, c := range e.ChildElements() {
			walk(c)
		}
	}
	if doc.Root() == nil {
		return io.ErrUnexpectedEOF
	}
	walk(doc.Root())
	for id, n := range ids {
		if n != 1 {
			return fmt.Errorf("duplicate output ID %s", id)
		}
	}
	for id := range d.Elements {
		if ids[id] != 1 {
			return fmt.Errorf("broken Item selector #%s", id)
		}
	}
	return nil
}

// StripFont removes only the embedded font bytes for fair authoring-traffic comparisons.
// The actual published SVG always remains self-contained.
func StripFont(svg []byte) []byte {
	start := bytes.Index(svg, []byte("base64,"))
	if start < 0 {
		return svg
	}
	start += len("base64,")
	end := bytes.IndexByte(svg[start:], ')')
	if end < 0 {
		return svg
	}
	out := append([]byte{}, svg[:start]...)
	out = append(out, []byte("[PINNED-FONT]")...)
	return append(out, svg[start+end:]...)
}
