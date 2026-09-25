// Package diagram is the structured source for generated slide diagrams.
//
// A Document is the authoritative drawing: every element carries explicit
// geometry, and nothing in this package lays out, resizes, or reroutes
// anything on the author's behalf. Render turns a Document into the exact
// SVG bytes a slide publishes; Describe projects it into a compact reading
// view that deliberately omits geometry and cannot rebuild the drawing.
package diagram

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
)

const (
	// Version is the diagram source record version.
	Version = 1
	// Renderer identifies the deterministic SVG renderer. Rendering the same
	// Document with the same Renderer produces the same bytes.
	Renderer = "change-saga-diagram/1"

	MaxElements       = 500
	MaxFragmentBytes  = 64 << 10
	MaxTotalFragments = 256 << 10
	MaxLabelRunes     = 2000
	MaxDescribeLimit  = 100
	DefaultWidth      = 1280
	DefaultHeight     = 720
	defaultBackground = "#fafaf8"
)

var (
	identifier    = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	colorPattern  = regexp.MustCompile(`^(#[a-fA-F0-9]{6}|none)$`)
	reservedID    = "diagram-"
	elementKinds  = map[string]bool{"node": true, "edge": true, "text": true, "group": true, "graphic": true}
	nodeShapes    = map[string]bool{"service": true, "datastore": true, "decision": true, "rect": true, "ellipse": true, "boundary": true}
	frameShapes   = map[string]bool{"rect": true, "boundary": true}
	textAlignment = map[string]bool{"": true, "start": true, "middle": true, "end": true}
)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Box is an element-local rectangle.
type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Style is a complete, reusable presentation. Elements reference styles by
// name; graphics may use currentColor to follow the style's stroke.
type Style struct {
	Fill        string  `json:"fill"`
	Stroke      string  `json:"stroke"`
	Ink         string  `json:"ink"`
	StrokeWidth float64 `json:"stroke_width"`
	FontSize    float64 `json:"font_size"`
	Dash        bool    `json:"dash,omitempty"`
}

// Element is one explicitly positioned drawing object. Coordinates are local
// to the parent group, whose position translates its children.
type Element struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Shape       string  `json:"shape,omitempty"`
	Label       string  `json:"label,omitempty"`
	Detail      string  `json:"detail,omitempty"`
	Description string  `json:"description,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width,omitempty"`
	Height      float64 `json:"height,omitempty"`
	Z           int     `json:"z,omitempty"`
	Parent      string  `json:"parent,omitempty"`
	Style       string  `json:"style"`
	Icon        string  `json:"icon,omitempty"`
	IconSize    float64 `json:"icon_size,omitempty"`
	From        string  `json:"from,omitempty"`
	To          string  `json:"to,omitempty"`
	Points      []Point `json:"points,omitempty"`
	Path        string  `json:"path,omitempty"`
	Head        string  `json:"head,omitempty"`
	HeadSize    float64 `json:"head_size,omitempty"`
	LabelBox    *Box    `json:"label_box,omitempty"`
	Align       string  `json:"align,omitempty"`
	Wrap        bool    `json:"wrap,omitempty"`
	Fragment    string  `json:"fragment,omitempty"`
	Decorative  bool    `json:"decorative,omitempty"`
}

// Document is the complete diagram source. Elements are ordered: that order
// is the reading order of Describe and breaks ties between equal z values.
type Document struct {
	Version    int              `json:"version"`
	Width      float64          `json:"width"`
	Height     float64          `json:"height"`
	Background string           `json:"background,omitempty"`
	Styles     map[string]Style `json:"styles,omitempty"`
	Elements   []Element        `json:"elements"`
}

// DefaultStyles are available to every Document. A Document style with the
// same name replaces the default completely.
func DefaultStyles() map[string]Style {
	return map[string]Style{
		"normal":    {Fill: "#ffffff", Stroke: "#64748b", Ink: "#172033", StrokeWidth: 2, FontSize: 22},
		"primary":   {Fill: "#edf5ff", Stroke: "#2563eb", Ink: "#153869", StrokeWidth: 2, FontSize: 22},
		"emphasis":  {Fill: "none", Stroke: "#2563eb", Ink: "#153869", StrokeWidth: 4, FontSize: 18},
		"secondary": {Fill: "none", Stroke: "#94a3b8", Ink: "#526174", StrokeWidth: 2, FontSize: 18},
		"warning":   {Fill: "#fff7e6", Stroke: "#b7791f", Ink: "#805015", StrokeWidth: 2, FontSize: 19},
		"boundary":  {Fill: "#f0f3f6", Stroke: "#c3cdd8", Ink: "#526174", StrokeWidth: 1, FontSize: 18, Dash: true},
		"title":     {Fill: "none", Stroke: "none", Ink: "#172033", StrokeWidth: 1, FontSize: 34},
		"caption":   {Fill: "none", Stroke: "none", Ink: "#526174", StrokeWidth: 1, FontSize: 15},
	}
}

// New returns an empty 16:9 Document.
func New() Document {
	return Document{Version: Version, Width: DefaultWidth, Height: DefaultHeight, Background: defaultBackground, Elements: []Element{}}
}

// Style resolves name against the Document's styles, then the defaults.
func (d Document) Style(name string) (Style, bool) {
	if style, ok := d.Styles[name]; ok {
		return style, true
	}
	style, ok := DefaultStyles()[name]
	return style, ok
}

// Index returns the position of id in Elements, or -1.
func (d Document) Index(id string) int {
	for index, element := range d.Elements {
		if element.ID == id {
			return index
		}
	}
	return -1
}

// Element returns the element named id.
func (d Document) Element(id string) (Element, bool) {
	if index := d.Index(id); index >= 0 {
		return d.Elements[index], true
	}
	return Element{}, false
}

// Decode strictly reads exactly one Document.
func Decode(data []byte) (Document, error) {
	var d Document
	if err := decodeStrict(data, &d); err != nil {
		return Document{}, err
	}
	return d, nil
}

func decodeStrict(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

// Encode returns the canonical stored bytes for d.
func Encode(d Document) ([]byte, error) {
	if d.Elements == nil {
		d.Elements = []Element{}
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) < 1e7 }

// Validate reports every structural problem at once, so an author can repair
// a whole batch instead of discovering violations one per attempt.
func (d Document) Validate() error {
	var problems []error
	add := func(format string, args ...any) { problems = append(problems, fmt.Errorf(format, args...)) }
	if d.Version != Version {
		add("diagram version must be %d", Version)
	}
	if !finite(d.Width) || !finite(d.Height) || d.Width <= 0 || d.Height <= 0 || d.Width > 10000 || d.Height > 10000 {
		add("canvas width and height must be positive and at most 10000")
	}
	if d.Background != "" && !colorPattern.MatchString(d.Background) {
		add("background must be #rrggbb or none")
	}
	if len(d.Elements) > MaxElements {
		add("a diagram may contain at most %d elements", MaxElements)
	}
	styleNames := make([]string, 0, len(d.Styles))
	for name := range d.Styles {
		styleNames = append(styleNames, name)
	}
	sort.Strings(styleNames)
	for _, name := range styleNames {
		style := d.Styles[name]
		if !identifier.MatchString(name) || !colorPattern.MatchString(style.Fill) || !colorPattern.MatchString(style.Stroke) || !colorPattern.MatchString(style.Ink) ||
			!finite(style.FontSize) || style.FontSize < 8 || style.FontSize > 200 || !finite(style.StrokeWidth) || style.StrokeWidth < 0 || style.StrokeWidth > 50 {
			add("style %s: needs a stable name, #rrggbb or none colors, font_size 8-200, and stroke_width 0-50", name)
		}
	}
	fragmentBytes := 0
	for _, e := range d.Elements {
		fragmentBytes += len(e.Fragment)
	}
	if fragmentBytes > MaxTotalFragments {
		add("graphic fragments total %d bytes; a diagram may carry at most %d", fragmentBytes, MaxTotalFragments)
	}
	byID := map[string]Element{}
	for _, e := range d.Elements {
		if !identifier.MatchString(e.ID) || strings.HasPrefix(e.ID, reservedID) {
			add("element %q: id must match %s and not start with %q", e.ID, identifier, reservedID)
			continue
		}
		if _, repeated := byID[e.ID]; repeated {
			add("element %s: duplicate id", e.ID)
		}
		byID[e.ID] = e
	}
	for _, e := range d.Elements {
		fail := func(format string, args ...any) { add("%s: "+format, append([]any{e.ID}, args...)...) }
		if !elementKinds[e.Kind] {
			fail("unsupported kind %q (use node, edge, text, group, or graphic)", e.Kind)
			continue
		}
		if _, ok := d.Style(e.Style); !ok {
			fail("unknown style %q", e.Style)
		}
		if len([]rune(e.Label)) > MaxLabelRunes || len([]rune(e.Detail)) > MaxLabelRunes || len([]rune(e.Description)) > 2*MaxLabelRunes {
			fail("text is too long")
		}
		for _, n := range []float64{e.X, e.Y, e.Width, e.Height, e.IconSize, e.HeadSize} {
			if !finite(n) {
				fail("geometry must be finite")
				break
			}
		}
		if e.Width < 0 || e.Height < 0 {
			fail("width and height must not be negative")
		}
		if !textAlignment[e.Align] {
			fail("align must be start, middle, or end")
		}
		if e.Icon != "" && e.Kind != "node" {
			fail("only nodes take an icon")
		}
		if e.Icon != "" {
			if !IconExists(e.Icon) {
				fail("unknown icon %q; list bundled icons with `change-saga diagram icons`", e.Icon)
			}
			if e.IconSize < 0 || e.IconSize > 256 {
				fail("icon_size must be 0-256")
			}
		}
		if e.LabelBox != nil {
			box := *e.LabelBox
			if !finite(box.X) || !finite(box.Y) || !finite(box.Width) || !finite(box.Height) || box.Width <= 0 || box.Height <= 0 {
				fail("label_box needs finite coordinates and a positive size")
			}
			if e.Kind != "node" && e.Kind != "edge" {
				fail("label_box applies only to nodes and edges")
			}
		}
		if e.Kind != "edge" && (e.From != "" || e.To != "" || len(e.Points) > 0 || e.Path != "" || e.Head != "" || e.HeadSize != 0) {
			fail("edge fields (from, to, points, path, head) are only valid on edges")
		}
		if e.Kind != "graphic" && e.Fragment != "" {
			fail("fragment is only valid on graphics")
		}
		switch e.Kind {
		case "node":
			if !nodeShapes[e.Shape] {
				fail("node shape must be service, datastore, decision, rect, ellipse, or boundary")
			}
			if e.Width <= 0 || e.Height <= 0 {
				fail("node needs a positive width and height")
			}
		case "edge":
			for _, end := range []string{e.From, e.To} {
				if end == "" {
					fail("edge needs explicit from and to nodes")
					break
				}
				if n, ok := byID[end]; !ok || n.Kind != "node" {
					fail("edge endpoint %q is not a node", end)
				}
			}
			if hasPoints := len(e.Points) > 0; hasPoints == (e.Path != "") || (hasPoints && len(e.Points) < 2) {
				fail("edge needs either at least two points or a path, not both")
			}
			for _, p := range e.Points {
				if !finite(p.X) || !finite(p.Y) {
					fail("edge points must be finite")
					break
				}
			}
			if e.Path != "" && !validPathData(e.Path) {
				fail("edge path must contain only SVG path commands and numbers")
			}
			if e.Head != "" && e.Head != "none" && e.Head != "arrow" {
				fail("edge head must be arrow or none")
			}
			if e.HeadSize < 0 || e.HeadSize > 100 {
				fail("head_size must be 0-100")
			}
			if e.Label != "" && e.LabelBox == nil {
				fail("an edge label needs an explicit label_box")
			}
			if e.Shape != "" {
				fail("edges do not take a shape")
			}
		case "text":
			if e.Width <= 0 || e.Height <= 0 {
				fail("text needs an explicit positive width and height")
			}
			if e.Shape != "" {
				fail("text does not take a shape")
			}
		case "group":
			if e.Shape != "" && !frameShapes[e.Shape] {
				fail("a group frame shape must be rect or boundary")
			}
			if e.Shape != "" && (e.Width <= 0 || e.Height <= 0) {
				fail("a framed group needs a positive width and height")
			}
			if e.Shape == "" && e.Label != "" && e.Decorative {
				fail("a decorative unframed group label is never shown or read; remove it")
			}
		case "graphic":
			if strings.TrimSpace(e.Fragment) == "" {
				fail("graphic needs an SVG fragment")
			} else if len(e.Fragment) > MaxFragmentBytes {
				fail("fragment exceeds %d bytes", MaxFragmentBytes)
			} else if _, err := parseFragment(e.Fragment); err != nil {
				fail("%v", err)
			}
		}
		seen := map[string]bool{e.ID: true}
		for parent := e.Parent; parent != ""; {
			if seen[parent] {
				fail("group cycle through %s", parent)
				break
			}
			seen[parent] = true
			group, ok := byID[parent]
			if !ok || group.Kind != "group" {
				fail("parent %q is not a group", parent)
				break
			}
			// A decorative group is aria-hidden and absent from Describe, which
			// would hide a semantic child from readers and assistive technology.
			if group.Decorative && !e.Decorative {
				fail("semantic element inside decorative group %s", parent)
			}
			parent = group.Parent
		}
	}
	return errors.Join(problems...)
}

var pathData = regexp.MustCompile(`^[MmLlHhVvCcSsQqTtAaZz0-9eE.,+\-\s]+$`)

func validPathData(path string) bool { return pathData.MatchString(path) }

// Contract is the diagram vocabulary change-saga spec publishes, so an author
// can discover the source format from the installed CLI without the network.
func Contract() map[string]any {
	styles := []string{}
	for name := range DefaultStyles() {
		styles = append(styles, name)
	}
	sort.Strings(styles)
	sorted := func(values map[string]bool) []string {
		result := []string{}
		for value := range values {
			if value != "" {
				result = append(result, value)
			}
		}
		sort.Strings(result)
		return result
	}
	return map[string]any{
		"schema":         "https://changesaga.dev/schema/v5/diagram.schema.json",
		"version":        Version,
		"renderer":       Renderer,
		"request_field":  "diagram, instead of asset, in an apply-slide request",
		"storage":        "24-a-<digest-prefix>.json beside the rendered 24-a-<digest-prefix>.svg, pinned by the revision's diagram field",
		"element_kinds":  sorted(elementKinds),
		"node_shapes":    sorted(nodeShapes),
		"frame_shapes":   sorted(frameShapes),
		"alignments":     sorted(textAlignment),
		"default_styles": styles,
		"fields":         []string{"id", "kind", "shape", "label", "detail", "description", "x", "y", "width", "height", "z", "parent", "style", "icon", "icon_size", "from", "to", "points", "path", "head", "head_size", "label_box", "align", "wrap", "fragment", "decorative"},
		"operations":     OperationNames,
		"font":           FontPath,
		"limits":         map[string]int{"elements": MaxElements, "fragment_bytes": MaxFragmentBytes, "label_runes": MaxLabelRunes},
		"rules": []string{
			"every coordinate is explicit and local to the parent group; nothing is laid out, resized, or rerouted",
			"element order is reading order and breaks ties between equal z values",
			"each element renders with its id as the SVG id; Items select semantic elements by id",
			"text that does not fit its box is refused",
			"a framed group draws its frame and parents its contents",
			"decorative elements are hidden from describe and assistive technology and may not contain semantic ones",
			"graphics accept allowlisted drawing markup; currentColor follows the style's stroke",
		},
	}
}
