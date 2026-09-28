package diagram

import (
	"fmt"
	"math"
	"strings"
)

// Sections, stickies, and annotations are the board layer of a diagram: a
// section is a group presented as a tinted frame with a title tab, a sticky is
// a coloured square of wrapped text, and an annotation is a speech bubble,
// numbered pin, translucent highlight, or bracket. Their colours come from a
// small renderer-owned palette rather than author styles, so text on them
// always has contrast; their style supplies only the font size, stroke width,
// and dash. A sticky or annotation may name the element it is about, which
// describe prints so a reader learns what each note annotates.

var (
	annotationShapes = map[string]bool{"bubble": true, "pin": true, "highlight": true, "bracket": true}
	bracketSides     = map[string]bool{"left": true, "right": true, "top": true, "bottom": true}
)

// Swatch is one palette colour's renderer-owned tones, each a reference to
// its diagram-<colour>-<tone> token.
type Swatch struct {
	Sticky string // a sticky's paper
	Tint   string // a section's or bubble's fill
	Mark   string // a highlight, drawn translucent
	Accent string // frames, tabs, pins, and brackets; onAccent text reads on it
	Ink    string // text on paper, tint, or mark
}

var palette = map[string]Swatch{}

func init() {
	for _, name := range paletteOrder {
		tone := func(role string) string { return "@diagram-" + name + "-" + role }
		palette[name] = Swatch{Sticky: tone("sticky"), Tint: tone("tint"), Mark: tone("mark"), Accent: tone("accent"), Ink: tone("ink")}
	}
}

// onAccent is the text on a section tab or pin, and the ring around a pin.
const onAccent = "@diagram-on-accent"

var paletteOrder = []string{"yellow", "pink", "blue", "green", "purple", "gray"}

// PaletteNames lists the colours a sticky, section, or annotation may use.
func PaletteNames() []string { return append([]string{}, paletteOrder...) }

// swatch resolves e's colour: stickies and highlights default to yellow,
// everything else to gray.
func swatch(e Element) Swatch {
	if e.Color != "" {
		return palette[e.Color]
	}
	if e.Kind == "sticky" || e.Shape == "highlight" {
		return palette["yellow"]
	}
	return palette["gray"]
}

const (
	shadowID   = "diagram-shadow"
	pinMaxText = 3
	bubbleMin  = 48.0
	bubbleR    = 10.0
)

func isSection(e Element) bool { return e.Kind == "group" && e.Shape == "section" }

// isNote reports whether e is described as a note about another element.
func isNote(e Element) bool { return e.Kind == "sticky" || e.Kind == "annotation" }

func labelBoxAnnotation(e Element) bool {
	return e.Kind == "annotation" && (e.Shape == "highlight" || e.Shape == "bracket")
}

// validateAnnotations reports every problem with e's section, sticky, and
// annotation fields. byID holds every element with a valid, unique id.
func validateAnnotations(d Document, e Element, byID map[string]Element) []string {
	problems := []string{}
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	board := isNote(e) || isSection(e)
	if e.Color != "" {
		if !board {
			add("color applies only to stickies, sections, and annotations")
		} else if _, ok := palette[e.Color]; !ok {
			add("color %q is not in the palette (%s)", e.Color, strings.Join(paletteOrder, ", "))
		}
	}
	if e.About != "" {
		target, ok := byID[e.About]
		switch {
		case !isNote(e):
			add("about applies only to stickies and annotations")
		case e.Decorative:
			add("a decorative element is hidden from readers, so what it is about would never be read; remove about or make the element semantic")
		case e.About == e.ID:
			add("about names the element itself")
		case !ok:
			add("about names unknown element %q", e.About)
		case target.Decorative:
			add("about names decorative element %s, which readers never see", e.About)
		}
	}
	if e.Target != nil && !(e.Kind == "annotation" && e.Shape == "bubble") {
		add("target applies only to bubble annotations")
	}
	if e.Side != "" && !(e.Kind == "annotation" && e.Shape == "bracket") {
		add("side applies only to bracket annotations")
	}
	if isSection(e) && strings.TrimSpace(e.Label) == "" {
		add("a section needs a label for its title tab")
	}
	if !isNote(e) {
		return problems
	}
	if e.Shape == "pin" {
		if e.Width <= 0 {
			add("a pin needs a positive width, its diameter")
		}
	} else if e.Width <= 0 || e.Height <= 0 {
		add("%s needs a positive width and height", noteKind(e))
	}
	if e.Detail != "" {
		add("%s takes no detail; put it in the label or a note", noteKind(e))
	}
	if e.Kind == "sticky" {
		if e.Shape != "" {
			add("a sticky takes no shape")
		}
		if strings.TrimSpace(e.Label) == "" {
			add("a sticky needs label text")
		}
		return problems
	}
	switch e.Shape {
	case "bubble":
		if strings.TrimSpace(e.Label) == "" {
			add("a bubble needs label text")
		}
		if e.Width < bubbleMin || e.Height < bubbleMin {
			add("a bubble needs a width and height of at least %s to fit its pointer", num(bubbleMin))
		}
		if e.Target == nil && e.About == "" {
			add("a bubble needs a target point or an about element for its pointer")
		}
		if e.Target != nil && (!finite(e.Target.X) || !finite(e.Target.Y)) {
			add("target must be finite")
		} else if len(problems) == 0 {
			if _, err := pointerTip(d, e); err != nil {
				add("%v", err)
			}
		}
	case "pin":
		if count := len([]rune(strings.TrimSpace(e.Label))); count == 0 || count > pinMaxText {
			add("a pin needs a label of 1-%d characters, such as its number", pinMaxText)
		}
		if e.Height != 0 && e.Height != e.Width {
			add("a pin is a circle: set height equal to width or omit it")
		}
	case "highlight", "bracket":
		if e.Label != "" && e.LabelBox == nil {
			add("a %s label needs an explicit label_box", e.Shape)
		}
		if e.Shape == "bracket" && !bracketSides[e.Side] {
			add("a bracket needs side left, right, top, or bottom: the side its point faces")
		}
	default:
		add("annotation shape must be bubble, pin, highlight, or bracket")
	}
	return problems
}

func noteKind(e Element) string {
	if e.Kind == "sticky" {
		return "a sticky"
	}
	return "an annotation"
}

// annotationName is the accessible name of a sticky or annotation: its label,
// with a pin's number spelled out, or what it is about.
func annotationName(e Element) string {
	kind := e.Shape
	if e.Kind == "sticky" {
		kind = "sticky"
	}
	label := strings.TrimSpace(e.Label)
	switch {
	case e.Shape == "pin" && e.About != "":
		return "Pin " + label + " on " + e.About
	case e.Shape == "pin":
		return "Pin " + label
	case label != "":
		return e.Label
	case e.About != "":
		return kind + " on " + e.About
	}
	return e.ID
}

// origin returns the canvas position of id's coordinate system origin: the
// sum of its ancestors' positions, bounded so a parent cycle, reported by
// Validate, cannot loop.
func origin(d Document, parent string) (float64, float64) {
	x, y := 0.0, 0.0
	for depth := 0; parent != "" && depth <= len(d.Elements); depth++ {
		group, ok := d.Element(parent)
		if !ok {
			break
		}
		x, y = x+group.X, y+group.Y
		parent = group.Parent
	}
	return x, y
}

// pointerTip returns the bubble-local point its pointer reaches: the explicit
// target, or else the point of its about element's box nearest the bubble's
// centre. The tip must lie outside the bubble.
func pointerTip(d Document, e Element) (Point, error) {
	var tip Point
	if e.Target != nil {
		tip = *e.Target
	} else {
		about, _ := d.Element(e.About)
		if about.Width <= 0 || about.Height <= 0 {
			return Point{}, fmt.Errorf("bubble points at %s, which has no box; set an explicit target", e.About)
		}
		bx, by := origin(d, e.Parent)
		ax, ay := origin(d, about.Parent)
		// Work in bubble-local coordinates.
		left, top := ax+about.X-bx-e.X, ay+about.Y-by-e.Y
		tip = Point{X: clamp(e.Width/2, left, left+about.Width), Y: clamp(e.Height/2, top, top+about.Height)}
	}
	if tip.X >= 0 && tip.X <= e.Width && tip.Y >= 0 && tip.Y <= e.Height {
		return Point{}, fmt.Errorf("bubble pointer tip (%s, %s) lies inside the bubble; move the bubble clear of what it points at", num(tip.X), num(tip.Y))
	}
	return tip, nil
}

func clamp(v, low, high float64) float64 { return math.Max(low, math.Min(high, v)) }

// bubbleDependents lists the bubbles whose derived pointer may re-aim because
// their about element, or either one's enclosing group, is in changed, so an
// edit reports them as changed too.
func bubbleDependents(d Document, changed map[string]bool) []string {
	moved := func(id string) bool {
		for depth := 0; id != "" && depth <= len(d.Elements); depth++ {
			if changed[id] {
				return true
			}
			e, _ := d.Element(id)
			id = e.Parent
		}
		return false
	}
	ids := []string{}
	for _, e := range d.Elements {
		if e.Kind == "annotation" && e.Shape == "bubble" && e.Target == nil && e.About != "" && !changed[e.ID] && (moved(e.About) || moved(e.Parent)) {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// board draws a sticky or an annotation into its element group g.
func (r *renderer) board(e Element, style Style, g *node) error {
	if e.About != "" {
		g.set("data-about", e.About)
	}
	colors := swatch(e)
	if e.Kind == "sticky" {
		r.ensureShadow()
		g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "3", "fill", colors.Sticky, "filter", "url(#"+shadowID+")")
		return r.text(g, e.Label, Box{X: 16, Y: 16, Width: e.Width - 32, Height: e.Height - 32}, style.FontSize, colors.Ink, true, e.Align)
	}
	g.set("data-annotation", e.Shape)
	switch e.Shape {
	case "bubble":
		tip, err := pointerTip(r.doc, e)
		if err != nil {
			return err
		}
		body := g.add("path", "d", bubblePath(e.Width, e.Height, tip), "fill", colors.Tint, "stroke", colors.Accent, "stroke-width", num(style.StrokeWidth), "stroke-linejoin", "round")
		if style.Dash {
			body.set("stroke-dasharray", "7 6")
		}
		return r.text(g, e.Label, Box{X: 14, Y: 12, Width: e.Width - 28, Height: e.Height - 24}, style.FontSize, colors.Ink, true, e.Align)
	case "pin":
		radius := e.Width / 2
		g.add("circle", "cx", num(radius), "cy", num(radius), "r", num(radius), "fill", colors.Accent, "stroke", onAccent, "stroke-width", "2")
		label := strings.TrimSpace(e.Label)
		size := math.Round(e.Width*.5*10) / 10
		width, err := r.measure(label, size)
		if err != nil {
			return err
		}
		if width*widthSlack > e.Width*.8 {
			return fmt.Errorf("text overflow: pin label %q needs width %.1f, a %s-wide pin has %.1f", label, width*widthSlack, num(e.Width), e.Width*.8)
		}
		g.add("text", "x", num(radius), "y", num(radius+size*.35), "font-size", num(size), "fill", onAccent, "text-anchor", "middle", "font-weight", "bold").text = label
		return nil
	case "highlight":
		g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "6", "fill", colors.Mark, "fill-opacity", "0.35")
	case "bracket":
		line := g.add("path", "d", bracketPath(e.Width, e.Height, e.Side), "fill", "none", "stroke", colors.Accent, "stroke-width", num(style.StrokeWidth), "stroke-linecap", "round", "stroke-linejoin", "round")
		if style.Dash {
			line.set("stroke-dasharray", "7 6")
		}
	}
	if e.Label != "" {
		return r.text(g, e.Label, *e.LabelBox, style.FontSize, colors.Ink, e.Wrap, e.Align)
	}
	return nil
}

// section draws a section's tinted frame and the title tab holding its label,
// inside the frame's top-left corner.
func (r *renderer) section(e Element, style Style, g *node) error {
	colors := swatch(e)
	frame := g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "10", "fill", colors.Tint, "stroke", colors.Accent, "stroke-width", num(style.StrokeWidth))
	if style.Dash {
		frame.set("stroke-dasharray", "7 6")
	}
	size := style.FontSize
	textWidth, err := r.measure(e.Label, size)
	if err != nil {
		return err
	}
	tabWidth, tabHeight := math.Min(textWidth*widthSlack+24, e.Width), size*lineHeight+12
	if tabHeight > e.Height {
		return fmt.Errorf("text overflow: section title tab needs height %.1f, the section has %.1f", tabHeight, e.Height)
	}
	g.add("path", "d", fmt.Sprintf("M0 %sV10A10 10 0 0 1 10 0H%sA10 10 0 0 1 %s 10V%sZ", num(tabHeight), num(tabWidth-10), num(tabWidth), num(tabHeight)), "fill", colors.Accent)
	return r.text(g, e.Label, Box{X: 12, Y: 6, Width: tabWidth - 24, Height: size * lineHeight}, size, onAccent, false, "")
}

// ensureShadow adds the renderer-owned soft shadow a sticky casts, once.
func (r *renderer) ensureShadow() {
	for _, child := range r.defs.children {
		for _, attr := range child.attrs {
			if attr == [2]string{"id", shadowID} {
				return
			}
		}
	}
	filter := r.defs.add("filter", "id", shadowID, "x", "-20%", "y", "-20%", "width", "140%", "height", "150%", "color-interpolation-filters", "sRGB")
	filter.add("feDropShadow", "dx", "0", "dy", "4", "stdDeviation", "5", "flood-color", "@diagram-shadow", "flood-opacity", "0.2")
}

// bubblePath outlines a rounded bubble whose pointer leaves the side facing
// tip. The pointer's base slides along that side toward the tip.
func bubblePath(w, h float64, tip Point) string {
	// Pick the side the tip lies beyond, weighing each overshoot by the
	// bubble's extent so a wide bubble points sideways only when the tip is
	// really beside it.
	dx, dy := tip.X-w/2, tip.Y-h/2
	side := "bottom"
	switch {
	case math.Abs(dx)/w > math.Abs(dy)/h && dx > 0:
		side = "right"
	case math.Abs(dx)/w > math.Abs(dy)/h:
		side = "left"
	case dy < 0:
		side = "top"
	}
	base := func(along, length float64) (float64, float64) {
		half := math.Min(12, (length-2*bubbleR)/4)
		center := clamp(along, bubbleR+half, length-bubbleR-half)
		return center - half, center + half
	}
	var b strings.Builder
	point := func(x, y float64) { fmt.Fprintf(&b, "L%s %s", num(x), num(y)) }
	pointer := func(x1, y1, x2, y2 float64) {
		point(x1, y1)
		point(tip.X, tip.Y)
		point(x2, y2)
	}
	arc := func(x, y float64) { fmt.Fprintf(&b, "A%s %s 0 0 1 %s %s", num(bubbleR), num(bubbleR), num(x), num(y)) }
	fmt.Fprintf(&b, "M%s 0", num(bubbleR))
	if side == "top" {
		a, c := base(tip.X, w)
		pointer(a, 0, c, 0)
	}
	point(w-bubbleR, 0)
	arc(w, bubbleR)
	if side == "right" {
		a, c := base(tip.Y, h)
		pointer(w, a, w, c)
	}
	point(w, h-bubbleR)
	arc(w-bubbleR, h)
	if side == "bottom" {
		a, c := base(tip.X, w)
		pointer(c, h, a, h)
	}
	point(bubbleR, h)
	arc(0, h-bubbleR)
	if side == "left" {
		a, c := base(tip.Y, h)
		pointer(0, c, 0, a)
	}
	point(0, bubbleR)
	arc(bubbleR, 0)
	b.WriteString("Z")
	return b.String()
}

// bracketPath draws a curly brace spanning its box whose point faces side.
func bracketPath(w, h float64, side string) string {
	length, depth := h, w
	if side == "top" || side == "bottom" {
		length, depth = w, h
	}
	middle := depth / 2
	curl := math.Min(middle, length/4)
	// at maps a point along and away from the brace's back to box coordinates.
	at := func(along, away float64) string {
		switch side {
		case "left":
			return num(w-away) + " " + num(along)
		case "top":
			return num(along) + " " + num(h-away)
		case "bottom":
			return num(along) + " " + num(away)
		}
		return num(away) + " " + num(along)
	}
	half := length / 2
	return "M" + at(0, 0) +
		"Q" + at(0, middle) + " " + at(curl, middle) +
		"L" + at(half-curl, middle) +
		"Q" + at(half, middle) + " " + at(half, depth) +
		"Q" + at(half, middle) + " " + at(half+curl, middle) +
		"L" + at(length-curl, middle) +
		"Q" + at(length, middle) + " " + at(length, 0)
}

// sectionMembers lists a section's direct semantic children in reading order.
func sectionMembers(d Document, e Element) []string {
	if !isSection(e) {
		return nil
	}
	members := []string{}
	for _, child := range d.Elements {
		if child.Parent == e.ID && !child.Decorative {
			members = append(members, child.ID)
		}
	}
	return members
}

// textSection names the describe heading e is listed under.
func textSection(e Summary) string {
	switch {
	case e.Kind == "group" && e.Shape == "section":
		return "section"
	case e.Kind == "sticky" || e.Kind == "annotation":
		return "note"
	}
	return e.Kind
}

// noteWord is the word describe leads a note with: sticky, or its shape.
func noteWord(e Summary) string {
	if e.Kind == "sticky" {
		return "sticky"
	}
	return e.Shape
}
