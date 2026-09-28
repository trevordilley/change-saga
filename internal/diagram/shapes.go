package diagram

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// libraryShapes draw in a renderer-owned text area and center their content.
// The original shapes keep their own layout in shape, byte for byte.
var libraryShapes = map[string]bool{
	"triangle": true, "hexagon": true, "parallelogram": true, "document": true, "cloud": true,
	"actor": true, "queue": true, "circle": true, "star": true,
}

// geometricShapes say nothing about what a node is, so describe omits them:
// a reader follows relationships, not outlines.
var geometricShapes = map[string]bool{"triangle": true, "hexagon": true, "parallelogram": true, "circle": true, "star": true}

// detailSize is the font size of a node's detail, as in shape.
const detailSize = 15.0

// The library and entity shapes register alongside the original node shapes.
func init() {
	for shape := range libraryShapes {
		nodeShapes[shape] = true
	}
	nodeShapes["entity"] = true
}

func shapeNames() []string {
	names := []string{}
	for name := range nodeShapes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// describedShape is the shape describe prints: one that says what a node is.
func describedShape(shape string) string {
	if geometricShapes[shape] {
		return ""
	}
	return shape
}

// validateShape checks shape-specific geometry and, through validateEntity,
// entity fields and the field an edge ends on.
func validateShape(d Document, e Element, byID map[string]Element) []string {
	problems := validateEntity(d, e, byID)
	if e.Kind == "node" && e.Shape == "circle" && e.Width != e.Height {
		problems = append(problems, "a circle needs equal width and height; use ellipse for an oval")
	}
	return problems
}

// textArea is the renderer-owned box inside a library shape where its icon,
// label, and detail sit.
func textArea(shape string, w, h float64) Box {
	const pad = 8.0
	switch shape {
	case "triangle":
		return Box{X: w/4 + pad, Y: h / 2, Width: w/2 - 2*pad, Height: h/2 - pad}
	case "hexagon":
		inset := hexagonInset(w, h)
		return Box{X: inset + 4, Y: pad, Width: w - 2*inset - 8, Height: h - 2*pad}
	case "parallelogram":
		slant := parallelogramSlant(w, h)
		return Box{X: slant + 4, Y: pad, Width: w - 2*slant - 8, Height: h - 2*pad}
	case "document":
		return Box{X: pad + 8, Y: pad, Width: w - 2*pad - 16, Height: h - 2*documentWave(h) - 2*pad}
	case "cloud":
		return Box{X: w * .2, Y: h * .38, Width: w * .6, Height: h * .52}
	case "actor":
		figure := actorFigure(h)
		return Box{X: 0, Y: figure + 6, Width: w, Height: h - figure - 6}
	case "queue":
		cap := queueCap(w, h)
		return Box{X: cap + 4, Y: pad, Width: w - 3*cap - 8, Height: h - 2*pad}
	case "circle":
		r := w / 2
		return Box{X: .2*r + 4, Y: .4*r + 4, Width: 1.6*r - 8, Height: 1.2*r - 8}
	case "star":
		rx, ry := starRadii(w, h)
		halfWidth, halfHeight := .8*starInner*math.Cos(math.Pi/5)*rx, .6*starInner*math.Cos(math.Pi/5)*ry
		return Box{X: w/2 - halfWidth, Y: ry - halfHeight, Width: 2 * halfWidth, Height: 2 * halfHeight}
	}
	return Box{X: pad, Y: pad, Width: w - 2*pad, Height: h - 2*pad}
}

func hexagonInset(w, h float64) float64       { return math.Min(h*.29, w*.25) }
func parallelogramSlant(w, h float64) float64 { return math.Min(h*.5, w*.2) }
func documentWave(h float64) float64          { return math.Min(h*.08, 14) }
func queueCap(w, h float64) float64           { return math.Min(h*.25, w*.15) }
func actorFigure(h float64) float64           { return h * .6 }

// starInner is the ratio of a star's inner to outer radius.
const starInner = .5

// starRadii scale a five-pointed star so its points touch every side of the
// box: it spans 2*sin(72°) of its radius across and 1+cos(36°) down.
func starRadii(w, h float64) (float64, float64) {
	return w / (2 * math.Sin(2*math.Pi/5)), h / (1 + math.Cos(math.Pi/5))
}

// cloudArcs trace a cloud clockwise from its bottom-left as fractions of the
// box: each arc's radii, then its end point.
var cloudArcs = [][4]float64{
	{.1419, .2243, .052, .6022},
	{.1589, .2512, .2563, .2434},
	{.227, .3588, .6649, .1716},
	{.1702, .2691, .9145, .4945},
	{.1702, .2691, .8237, .9969},
}

// outline is a library shape's closed path.
func outline(shape string, w, h float64) string {
	points := func(xy ...float64) string {
		var b strings.Builder
		for index := 0; index+1 < len(xy); index += 2 {
			if index == 0 {
				b.WriteString("M")
			} else {
				b.WriteString("L")
			}
			b.WriteString(num(xy[index]) + " " + num(xy[index+1]))
		}
		b.WriteString("Z")
		return b.String()
	}
	switch shape {
	case "triangle":
		return points(w/2, 0, w, h, 0, h)
	case "hexagon":
		inset := hexagonInset(w, h)
		return points(inset, 0, w-inset, 0, w, h/2, w-inset, h, inset, h, 0, h/2)
	case "parallelogram":
		slant := parallelogramSlant(w, h)
		return points(slant, 0, w, 0, w-slant, h, 0, h)
	case "document":
		// A cubic from (w, c) to (0, c) with controls c-k and c+k swings
		// 0.2887k either side of c, so k sets the wave's amplitude.
		wave := documentWave(h)
		c, k := h-wave, wave/.2887
		return fmt.Sprintf("M0 0H%sV%sC%s %s %s %s 0 %sZ", num(w), num(c), num(w*2/3), num(c-k), num(w/3), num(c+k), num(c))
	case "cloud":
		var b strings.Builder
		b.WriteString("M" + num(.1655*w) + " " + num(.9969*h))
		for _, arc := range cloudArcs {
			fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", num(arc[0]*w), num(arc[1]*h), num(arc[2]*w), num(arc[3]*h))
		}
		b.WriteString("Z")
		return b.String()
	case "queue":
		cap := queueCap(w, h)
		return fmt.Sprintf("M%s 0H%sA%s %s 0 0 1 %s %sH%sA%s %s 0 0 1 %s 0Z", num(cap), num(w-cap), num(cap), num(h/2), num(w-cap), num(h), num(cap), num(cap), num(h/2), num(cap))
	case "star":
		rx, ry := starRadii(w, h)
		xy := []float64{}
		for index := range 10 {
			radius := 1.0
			if index%2 == 1 {
				radius = starInner
			}
			angle := -math.Pi/2 + float64(index)*math.Pi/5
			xy = append(xy, w/2+radius*rx*math.Cos(angle), ry+radius*ry*math.Sin(angle))
		}
		return points(xy...)
	}
	return ""
}

func stroke(n *node, style Style) {
	n.set("fill", "none")
	n.set("stroke", style.Stroke)
	n.set("stroke-width", num(style.StrokeWidth))
	if style.Dash {
		n.set("stroke-dasharray", "7 6")
	}
}

// library draws a library shape and centers its icon, label, and detail in
// the shape's text area. Overflow is refused, as everywhere else.
func (r *renderer) library(e Element, style Style, g *node) error {
	w, h := e.Width, e.Height
	switch e.Shape {
	case "circle":
		paint(g.add("circle", "cx", num(w/2), "cy", num(h/2), "r", num(w/2)), style)
	case "actor":
		figure := actorFigure(h)
		unit := math.Min(figure/10, w/5)
		cx := w / 2
		paint(g.add("circle", "cx", num(cx), "cy", num(1.5*unit), "r", num(1.4*unit)), style)
		limbs := g.add("path", "d", fmt.Sprintf("M%s %sV%sM%s %sH%sM%s %sL%s %sM%s %sL%s %s",
			num(cx), num(2.9*unit), num(6.2*unit),
			num(cx-2.4*unit), num(4.2*unit), num(cx+2.4*unit),
			num(cx), num(6.2*unit), num(cx-2*unit), num(9.8*unit),
			num(cx), num(6.2*unit), num(cx+2*unit), num(9.8*unit)))
		stroke(limbs, style)
		limbs.set("stroke-linecap", "round")
	default:
		paint(g.add("path", "d", outline(e.Shape, w, h), "stroke-linejoin", "round"), style)
	}
	if e.Shape == "queue" {
		cap := queueCap(w, h)
		stroke(g.add("path", "d", fmt.Sprintf("M%s 0A%s %s 0 0 0 %s %s", num(w-cap), num(cap), num(h/2), num(w-cap), num(h))), style)
	}
	area := textArea(e.Shape, w, h)
	align := e.Align
	if align == "" {
		align = "middle"
	}
	if e.LabelBox != nil {
		if err := r.text(g, e.Label, *e.LabelBox, style.FontSize, style.Ink, e.Wrap, align); err != nil {
			return err
		}
	}
	if e.Icon == "" && e.Detail == "" && (e.Label == "" || e.LabelBox != nil) {
		return nil
	}
	if area.Width <= 0 || area.Height <= 0 {
		return fmt.Errorf("the %s is too small for text: its text area is %sx%s", e.Shape, num(area.Width), num(area.Height))
	}
	iconSize := e.IconSize
	if iconSize == 0 {
		iconSize = 24
	}
	var label, detail []string
	var err error
	if e.Label != "" && e.LabelBox == nil {
		if label, err = r.lines(e.Label, area.Width, style.FontSize, e.Wrap); err != nil {
			return err
		}
	}
	if e.Detail != "" {
		if detail, err = r.lines(e.Detail, area.Width, detailSize, e.Wrap); err != nil {
			return err
		}
	}
	need := float64(len(label))*style.FontSize*lineHeight + float64(len(detail))*detailSize*lineHeight
	if e.Icon != "" {
		need += iconSize
		if len(label)+len(detail) > 0 {
			need += 8
		}
	}
	if len(label) > 0 && len(detail) > 0 {
		need += 6
	}
	if need > area.Height+.01 {
		return fmt.Errorf("text overflow: the %s's icon, label, and detail need height %.1f, its text area has %.1f", e.Shape, need, area.Height)
	}
	y := area.Y
	if e.Shape != "actor" {
		y += (area.Height - need) / 2
	}
	if e.Icon != "" {
		x := area.X
		switch align {
		case "middle":
			x += (area.Width - iconSize) / 2
		case "end":
			x += area.Width - iconSize
		}
		icon, err := r.icon(e.Icon, x, y, iconSize, style.Ink)
		if err != nil {
			return err
		}
		g.children = append(g.children, icon)
		y += iconSize + 8
	}
	if len(label) > 0 {
		height := float64(len(label)) * style.FontSize * lineHeight
		if err := r.text(g, e.Label, Box{X: area.X, Y: y, Width: area.Width, Height: height}, style.FontSize, style.Ink, e.Wrap, align); err != nil {
			return err
		}
		y += height + 6
	}
	if len(detail) > 0 {
		height := float64(len(detail)) * detailSize * lineHeight
		return r.text(g, e.Detail, Box{X: area.X, Y: y, Width: area.Width, Height: height}, detailSize, style.Ink, e.Wrap, align)
	}
	return nil
}
