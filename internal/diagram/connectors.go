package diagram

import (
	"fmt"
	"sort"
	"strings"
)

// terminator is one end marker drawn in a 10x10 box whose x axis runs along
// the edge toward the end it marks, with the edge's end point at x=10. A tail
// uses the same drawing turned to face outward, so every terminator reads the
// same at either end. Hollow parts are filled with the canvas background so
// the line does not show through them.
type terminator struct {
	// size is the default marker size at a stroke width of 2.
	size  float64
	parts []terminatorPart
}

type terminatorPart struct {
	tag    string // path or circle
	d      string
	cx, cy float64
	r      float64
	filled bool // filled with the stroke colour
	hollow bool // filled with the canvas background
}

func linePart(d string) terminatorPart   { return terminatorPart{tag: "path", d: d} }
func filledPart(d string) terminatorPart { return terminatorPart{tag: "path", d: d, filled: true} }
func hollowPart(d string) terminatorPart { return terminatorPart{tag: "path", d: d, hollow: true} }
func dotPart(cx, r float64) terminatorPart {
	return terminatorPart{tag: "circle", cx: cx, cy: 5, r: r, filled: true}
}
func ringPart(cx, r float64) terminatorPart {
	return terminatorPart{tag: "circle", cx: cx, cy: 5, r: r, hollow: true}
}
func crowsFoot(from float64) terminatorPart {
	return linePart(fmt.Sprintf("M%s 5L10 0M%s 5L10 10M%s 5L10 5", num(from), num(from), num(from)))
}

// terminators are the head and tail vocabulary: directional arrows, UML ends,
// and the ERD crow's-foot cardinalities.
var terminators = map[string]terminator{
	"arrow":          {10, []terminatorPart{filledPart("M0 0L10 5L0 10Z")}},
	"open":           {12, []terminatorPart{linePart("M0 0L10 5L0 10")}},
	"triangle":       {18, []terminatorPart{hollowPart("M0 0L10 5L0 10Z")}},
	"diamond":        {20, []terminatorPart{hollowPart("M0 5L5 1L10 5L5 9Z")}},
	"filled-diamond": {20, []terminatorPart{filledPart("M0 5L5 1L10 5L5 9Z")}},
	"circle":         {12, []terminatorPart{ringPart(5, 4)}},
	"dot":            {12, []terminatorPart{dotPart(5, 4)}},
	"bar":            {12, []terminatorPart{linePart("M8 0L8 10")}},
	"one":            {24, []terminatorPart{linePart("M6 0L6 10")}},
	"only-one":       {24, []terminatorPart{linePart("M4 0L4 10M7 0L7 10")}},
	"zero-or-one":    {24, []terminatorPart{ringPart(2.5, 2), linePart("M7 0L7 10")}},
	"many":           {24, []terminatorPart{crowsFoot(4)}},
	"one-or-many":    {24, []terminatorPart{linePart("M2 0L2 10"), crowsFoot(4)}},
	"zero-or-many":   {24, []terminatorPart{ringPart(2, 1.8), crowsFoot(4.5)}},
}

func terminatorKeys() []string {
	names := []string{}
	for name := range terminators {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TerminatorNames lists the head and tail vocabulary for discovery.
func TerminatorNames() []string {
	names := append(terminatorKeys(), "none")
	sort.Strings(names)
	return names
}

var edgeCurves = map[string]bool{"": true, "straight": true, "smooth": true}
var edgeLines = map[string]bool{"": true, "solid": true, "dashed": true, "dotted": true}

// validateConnector reports problems with an edge's terminators, end labels,
// curve, and line, and refuses those fields on anything but an edge.
func validateConnector(e Element, fail func(string, ...any)) {
	if e.Kind != "edge" {
		if e.Tail != "" || e.TailLabel != "" || e.TailLabelBox != nil || e.HeadLabel != "" || e.HeadLabelBox != nil || e.Curve != "" || e.Line != "" {
			fail("connector fields (tail, head_label, tail_label, curve, line) are only valid on edges")
		}
		return
	}
	for _, end := range [][2]string{{"head", e.Head}, {"tail", e.Tail}} {
		if _, known := terminators[end[1]]; end[1] != "" && end[1] != "none" && !known {
			fail("%s must be none or one of %s", end[0], strings.Join(terminatorKeys(), ", "))
		}
	}
	for _, label := range []struct {
		name string
		text string
		box  *Box
	}{{"head_label", e.HeadLabel, e.HeadLabelBox}, {"tail_label", e.TailLabel, e.TailLabelBox}} {
		if label.text != "" && label.box == nil {
			fail("%s needs an explicit %s_box", label.name, label.name)
		}
		if label.box != nil {
			box := *label.box
			if !finite(box.X) || !finite(box.Y) || !finite(box.Width) || !finite(box.Height) || box.Width <= 0 || box.Height <= 0 {
				fail("%s_box needs finite coordinates and a positive size", label.name)
			}
		}
		if len([]rune(label.text)) > MaxLabelRunes {
			fail("%s is too long", label.name)
		}
	}
	if !edgeCurves[e.Curve] {
		fail("curve must be straight or smooth")
	}
	if e.Curve == "smooth" && e.Path != "" {
		fail("curve smooth applies to points, not to an explicit path")
	}
	if !edgeLines[e.Line] {
		fail("line must be solid, dashed, or dotted")
	}
}

// edgePath is the path an edge's points describe: straight segments, or a
// smooth curve through every point.
func edgePath(e Element) string {
	if e.Path != "" {
		return e.Path
	}
	var b strings.Builder
	points := e.Points
	b.WriteString("M" + num(points[0].X) + " " + num(points[0].Y))
	if e.Curve != "smooth" || len(points) < 3 {
		for _, p := range points[1:] {
			b.WriteString(" L" + num(p.X) + " " + num(p.Y))
		}
		return b.String()
	}
	// A Catmull-Rom spline through the points, as cubic Bezier segments.
	at := func(index int) Point { return points[max(0, min(len(points)-1, index))] }
	for index := 0; index < len(points)-1; index++ {
		p0, p1, p2, p3 := at(index-1), at(index), at(index+1), at(index+2)
		c1 := Point{p1.X + (p2.X-p0.X)/6, p1.Y + (p2.Y-p0.Y)/6}
		c2 := Point{p2.X - (p3.X-p1.X)/6, p2.Y - (p3.Y-p1.Y)/6}
		fmt.Fprintf(&b, " C%s %s %s %s %s %s", num(c1.X), num(c1.Y), num(c2.X), num(c2.Y), num(p2.X), num(p2.Y))
	}
	return b.String()
}

// markerSize is a terminator's size: the author's head_size, or the
// terminator's default grown in proportion to a stroke thicker than 2.
func markerSize(e Element, style Style, base float64) float64 {
	if e.HeadSize > 0 {
		return e.HeadSize
	}
	return base * max(1, style.StrokeWidth/2)
}

// connectorMarker defines the marker for one end of e and returns its
// reference. A legacy filled arrow head keeps its original definition, so
// diagrams drawn before terminators render unchanged.
func (r *renderer) connectorMarker(e Element, style Style, end, name string) string {
	shape := terminators[name]
	size := markerSize(e, style, shape.size)
	if end == "head" && name == "arrow" {
		id := "diagram-head-" + e.ID
		marker := r.defs.add("marker", "id", id, "viewBox", "0 0 10 10", "refX", "9", "refY", "5", "markerWidth", num(size), "markerHeight", num(size), "markerUnits", "userSpaceOnUse", "orient", "auto")
		marker.add("path", "d", "M0 0L10 5L0 10Z", "fill", style.Stroke)
		return "url(#" + id + ")"
	}
	orient := "auto"
	if end == "tail" {
		orient = "auto-start-reverse"
	}
	id := "diagram-marker-" + e.ID + "-" + end
	marker := r.defs.add("marker", "id", id, "viewBox", "0 0 10 10", "refX", "10", "refY", "5", "markerWidth", num(size), "markerHeight", num(size), "markerUnits", "userSpaceOnUse", "orient", orient, "overflow", "visible")
	// Marker content is scaled by size/10, so this keeps its lines as thick as
	// the edge's.
	width := num(style.StrokeWidth * 10 / size)
	background := r.doc.Background
	if background == "" || background == "none" {
		background = "#ffffff"
	}
	for _, part := range shape.parts {
		fill := "none"
		switch {
		case part.filled:
			fill = style.Stroke
		case part.hollow:
			fill = background
		}
		var n *node
		if part.tag == "circle" {
			n = marker.add("circle", "cx", num(part.cx), "cy", num(part.cy), "r", num(part.r))
		} else {
			n = marker.add("path", "d", part.d)
		}
		n.set("fill", fill)
		n.set("stroke", style.Stroke)
		n.set("stroke-width", width)
		n.set("stroke-linejoin", "round")
	}
	return "url(#" + id + ")"
}

// edgeDash is the dash pattern an edge's line asks for, or its style's.
func edgeDash(e Element, style Style) (dash, cap string) {
	switch e.Line {
	case "dashed":
		return "8 6", ""
	case "dotted":
		return "0.1 " + num(max(4, style.StrokeWidth*2.5)), "round"
	case "solid":
		return "", ""
	}
	if style.Dash {
		return "7 6", ""
	}
	return "", ""
}

// connectorLabels draws an edge's end labels in their explicit boxes.
func (r *renderer) connectorLabels(e Element, style Style, g *node) error {
	for _, label := range []struct {
		text string
		box  *Box
	}{{e.TailLabel, e.TailLabelBox}, {e.HeadLabel, e.HeadLabelBox}} {
		if label.text == "" {
			continue
		}
		if err := r.text(g, label.text, *label.box, style.FontSize, style.Ink, false, "middle"); err != nil {
			return err
		}
	}
	return nil
}
