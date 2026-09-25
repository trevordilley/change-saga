// Package draft is an experimental, explicitly positioned diagram document.
// It is not a production Saga format and does not infer requirements relations.
package draft

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const Renderer = "diagram-spike/1-etree-1.6.0"

var identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var colorPattern = regexp.MustCompile(`^(#[a-fA-F0-9]{6}|none)$`)

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Style struct {
	Fill        string  `json:"fill"`
	Stroke      string  `json:"stroke"`
	Ink         string  `json:"ink"`
	StrokeWidth float64 `json:"stroke_width"`
	FontSize    float64 `json:"font_size"`
	Dash        bool    `json:"dash,omitempty"`
}
type Asset struct {
	Digest  string `json:"digest"`
	Origin  string `json:"origin"`
	License string `json:"license"`
}
type Element struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Shape       string  `json:"shape,omitempty"`
	Label       string  `json:"label,omitempty"`
	Detail      string  `json:"detail,omitempty"`
	Description string  `json:"description,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	W           float64 `json:"width,omitempty"`
	H           float64 `json:"height,omitempty"`
	Z           int     `json:"z"`
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
	LabelAt     *Point  `json:"label_at,omitempty"`
	Wrap        bool    `json:"wrap,omitempty"`
	Fragment    string  `json:"fragment,omitempty"`
	Decorative  bool    `json:"decorative,omitempty"`
	// These existing Saga payloads remain opaque here. Actual publication must run
	// ApplySlideTransaction's repository, digest, criterion and selector checks.
	Evidence       []json.RawMessage `json:"evidence,omitempty"`
	CriterionLinks []json.RawMessage `json:"criterion_links,omitempty"`
}
type Document struct {
	Version    int                `json:"version"`
	Renderer   string             `json:"renderer"`
	ID         string             `json:"id"`
	Title      string             `json:"title"`
	Width      float64            `json:"width"`
	Height     float64            `json:"height"`
	Background string             `json:"background"`
	Styles     map[string]Style   `json:"styles"`
	Assets     map[string]Asset   `json:"assets"`
	Elements   map[string]Element `json:"elements"`
}
type Operation struct {
	Op      string          `json:"op"`
	ID      string          `json:"id,omitempty"`
	Element *Element        `json:"element,omitempty"`
	Set     json.RawMessage `json:"set,omitempty"`
	Style   *Style          `json:"style,omitempty"`
	DX      float64         `json:"dx,omitempty"`
	DY      float64         `json:"dy,omitempty"`
	Cascade bool            `json:"cascade,omitempty"`
	IDs     []string        `json:"ids,omitempty"`
	Axis    string          `json:"axis,omitempty"`
	Value   float64         `json:"value,omitempty"`
}
type Request struct {
	Version    int         `json:"version"`
	RequestID  string      `json:"request_id"`
	Expected   string      `json:"expected_snapshot"`
	Source     *Document   `json:"source,omitempty"`
	Operations []Operation `json:"operations,omitempty"`
}

func Decode(data []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}
func Hash(b []byte) string       { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }
func Snapshot(d Document) string { b, _ := json.Marshal(d); return Hash(b) }
func clone(d Document) Document {
	b, _ := json.Marshal(d)
	var c Document
	_ = json.Unmarshal(b, &c)
	return c
}
func keys[T any](m map[string]T) []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	sort.Strings(a)
	return a
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) < 1e7 }

func New(id, title string) Document {
	return Document{Version: 1, Renderer: Renderer, ID: id, Title: title, Width: 1280, Height: 720, Background: "#fafaf8", Elements: map[string]Element{}, Assets: map[string]Asset{}, Styles: map[string]Style{
		"normal":    {Fill: "#ffffff", Stroke: "#64748b", Ink: "#172033", StrokeWidth: 2, FontSize: 22},
		"primary":   {Fill: "#edf5ff", Stroke: "#2563eb", Ink: "#153869", StrokeWidth: 2, FontSize: 22},
		"emphasis":  {Fill: "none", Stroke: "#2563eb", Ink: "#153869", StrokeWidth: 4, FontSize: 18},
		"secondary": {Fill: "none", Stroke: "#94a3b8", Ink: "#526174", StrokeWidth: 2, FontSize: 18},
		"warning":   {Fill: "#fff7e6", Stroke: "#b7791f", Ink: "#805015", StrokeWidth: 2, FontSize: 19},
		"boundary":  {Fill: "#f0f3f6", Stroke: "#c3cdd8", Ink: "#526174", StrokeWidth: 1, FontSize: 18, Dash: true},
		"title":     {Fill: "none", Stroke: "none", Ink: "#172033", StrokeWidth: 1, FontSize: 34},
	}}
}

func (d Document) Validate() error {
	if d.Version != 1 || d.Renderer != Renderer {
		return fmt.Errorf("unsupported document version/renderer")
	}
	if !identifier.MatchString(d.ID) || strings.HasPrefix(d.ID, "draft-") {
		return fmt.Errorf("invalid document id")
	}
	if !finite(d.Width) || !finite(d.Height) || d.Width <= 0 || d.Height <= 0 || d.Width > 10000 || d.Height > 10000 {
		return fmt.Errorf("invalid canvas dimensions")
	}
	if !colorPattern.MatchString(d.Background) {
		return fmt.Errorf("invalid background")
	}
	if len(d.Elements) > 500 {
		return fmt.Errorf("prototype limit: 500 elements")
	}
	for _, name := range keys(d.Styles) {
		s := d.Styles[name]
		if !identifier.MatchString(name) || !colorPattern.MatchString(s.Fill) || !colorPattern.MatchString(s.Stroke) || !colorPattern.MatchString(s.Ink) || !finite(s.FontSize) || s.FontSize < 8 || s.FontSize > 200 || !finite(s.StrokeWidth) || s.StrokeWidth < 0 || s.StrokeWidth > 50 {
			return fmt.Errorf("invalid style %s", name)
		}
	}
	for _, id := range keys(d.Elements) {
		e := d.Elements[id]
		if !identifier.MatchString(id) || strings.HasPrefix(id, "draft-") || e.ID != id {
			return fmt.Errorf("invalid or mismatched element ID %s", id)
		}
		if _, ok := d.Styles[e.Style]; !ok {
			return fmt.Errorf("%s: unknown style %s", id, e.Style)
		}
		if len(e.Label) > 2000 || len(e.Detail) > 2000 || len(e.Description) > 4000 {
			return fmt.Errorf("%s: text too long", id)
		}
		for _, n := range []float64{e.X, e.Y, e.W, e.H, e.IconSize, e.HeadSize} {
			if !finite(n) {
				return fmt.Errorf("%s: non-finite geometry", id)
			}
		}
		if e.Icon != "" {
			if _, ok := d.Assets[e.Icon]; !ok {
				return fmt.Errorf("%s: missing pinned icon %s", id, e.Icon)
			}
			if e.IconSize < 0 || e.IconSize > 256 {
				return fmt.Errorf("%s: invalid icon size", id)
			}
		}
		switch e.Kind {
		case "node":
			switch e.Shape {
			case "service", "datastore", "decision", "rect", "ellipse", "boundary":
			default:
				return fmt.Errorf("%s: unknown shape %s", id, e.Shape)
			}
			if e.W <= 0 || e.H <= 0 {
				return fmt.Errorf("%s: node needs positive size", id)
			}
		case "edge":
			if e.From == "" || e.To == "" {
				return fmt.Errorf("%s: edge needs explicit from/to", id)
			}
			for _, end := range []string{e.From, e.To} {
				n, ok := d.Elements[end]
				if !ok || n.Kind != "node" {
					return fmt.Errorf("%s: missing node %s", id, end)
				}
			}
			if (len(e.Points) < 2) == (e.Path == "") {
				return fmt.Errorf("%s: supply points or path, exclusively", id)
			}
			for _, p := range e.Points {
				if !finite(p.X) || !finite(p.Y) {
					return fmt.Errorf("%s: invalid point", id)
				}
			}
			if e.Head != "" && e.Head != "none" && e.Head != "arrow" {
				return fmt.Errorf("%s: unsupported head", id)
			}
			if e.HeadSize < 0 || e.HeadSize > 100 {
				return fmt.Errorf("%s: invalid head size", id)
			}
			if e.Label != "" && e.LabelAt == nil {
				return fmt.Errorf("%s: edge labels require label_at", id)
			}
		case "group":
		case "text":
			if e.W <= 0 || e.H <= 0 {
				return fmt.Errorf("%s: text needs an explicit box", id)
			}
		case "graphic":
			if e.Fragment == "" {
				return fmt.Errorf("%s: empty custom SVG", id)
			}
		default:
			return fmt.Errorf("%s: unsupported kind %s", id, e.Kind)
		}
		if e.Kind != "edge" && (e.From != "" || e.To != "" || len(e.Points) > 0 || e.Path != "") {
			return fmt.Errorf("%s: edge fields on non-edge", id)
		}
		if e.LabelAt != nil && (!finite(e.LabelAt.X) || !finite(e.LabelAt.Y)) {
			return fmt.Errorf("%s: invalid label position", id)
		}
		seen := map[string]bool{id: true}
		p := e.Parent
		for p != "" {
			if seen[p] {
				return fmt.Errorf("%s: group cycle", id)
			}
			seen[p] = true
			g, ok := d.Elements[p]
			if !ok || g.Kind != "group" {
				return fmt.Errorf("%s: missing parent group %s", id, p)
			}
			p = g.Parent
		}
	}
	for name, a := range d.Assets {
		if !digestPattern.MatchString(a.Digest) || a.Origin == "" || a.License == "" {
			return fmt.Errorf("invalid asset pin %s", name)
		}
	}
	return nil
}

// Edit changes only requested properties. A connected node move never reroutes an edge.
func Edit(d Document, ops []Operation) (Document, error) {
	d = clone(d)
	for i, op := range ops {
		fail := func(msg string) (Document, error) {
			return Document{}, fmt.Errorf("operation %d (%s): %s", i, op.Op, msg)
		}
		switch op.Op {
		case "style":
			if op.Style == nil {
				return fail("style value required")
			}
			if d.Styles == nil {
				d.Styles = map[string]Style{}
			}
			d.Styles[op.ID] = *op.Style
		case "add":
			if op.Element == nil {
				return fail("element required")
			}
			e := *op.Element
			if _, ok := d.Elements[e.ID]; ok {
				return fail("duplicate id " + e.ID)
			}
			d.Elements[e.ID] = e
		case "update":
			e, ok := d.Elements[op.ID]
			if !ok {
				return fail("unknown id")
			}
			b, _ := json.Marshal(e)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(b, &fields)
			var patch map[string]json.RawMessage
			if err := Decode(op.Set, &patch); err != nil || patch == nil {
				return fail("set must be an object")
			}
			for k, v := range patch {
				if k == "id" || k == "kind" || k == "evidence" || k == "criterion_links" {
					return fail("immutable identity/evidence field " + k)
				}
				fields[k] = v
			}
			b, _ = json.Marshal(fields)
			var updated Element
			if err := Decode(b, &updated); err != nil {
				return fail(err.Error())
			}
			d.Elements[op.ID] = updated
		case "move":
			e, ok := d.Elements[op.ID]
			if !ok {
				return fail("unknown id")
			}
			if !finite(op.DX) || !finite(op.DY) {
				return fail("invalid delta")
			}
			e.X += op.DX
			e.Y += op.DY
			d.Elements[op.ID] = e
		case "remove":
			if _, ok := d.Elements[op.ID]; !ok {
				return fail("unknown id")
			}
			remove := map[string]bool{op.ID: true}
			for again := true; again; {
				again = false
				for id, e := range d.Elements {
					if !remove[id] && (remove[e.Parent] || remove[e.From] || remove[e.To]) {
						if !op.Cascade {
							return fail("dependency " + id + "; use explicit cascade")
						}
						remove[id] = true
						again = true
					}
				}
			}
			for id := range remove {
				e := d.Elements[id]
				if len(e.Evidence) > 0 || len(e.CriterionLinks) > 0 {
					return fail("refusing removal of linked Item " + id)
				}
				delete(d.Elements, id)
			}
		case "align", "distribute":
			if len(op.IDs) < 2 || (op.Axis != "x" && op.Axis != "y") || !finite(op.Value) {
				return fail("need >=2 IDs, x/y axis and finite value")
			}
			ids := map[string]bool{}
			parent := ""
			for j, id := range op.IDs {
				e, ok := d.Elements[id]
				if !ok || ids[id] {
					return fail("unknown/duplicate selection")
				}
				ids[id] = true
				if j == 0 {
					parent = e.Parent
				}
				if e.Parent != parent {
					return fail("selected elements must share a coordinate space")
				}
			}
			pos := op.Value
			for _, id := range op.IDs {
				e := d.Elements[id]
				if op.Axis == "x" {
					if op.Op == "align" {
						e.X = op.Value
					} else {
						if id == op.IDs[0] {
							pos = e.X
						}
						e.X = pos
						pos += e.W + op.Value
					}
				} else {
					if op.Op == "align" {
						e.Y = op.Value
					} else {
						if id == op.IDs[0] {
							pos = e.Y
						}
						e.Y = pos
						pos += e.H + op.Value
					}
				}
				d.Elements[id] = e
			}
		default:
			return fail("unknown operation")
		}
	}
	return d, nil
}

type Summary struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Label          string `json:"label,omitempty"`
	Detail         string `json:"detail,omitempty"`
	Description    string `json:"description,omitempty"`
	From           string `json:"from,omitempty"`
	To             string `json:"to,omitempty"`
	Parent         string `json:"parent,omitempty"`
	Icon           string `json:"icon,omitempty"`
	CodeLinks      int    `json:"code_links,omitempty"`
	CriterionLinks int    `json:"criterion_links,omitempty"`
}

// Description is the single reading projection behind every describe format.
type Description struct {
	ID              string    `json:"id"`
	Title           string    `json:"title"`
	Snapshot        string    `json:"snapshot"`
	Elements        []Summary `json:"elements"`
	Offset          int       `json:"offset"`
	Total           int       `json:"total"`
	NextOffset      int       `json:"next_offset"`
	HasMore         bool      `json:"has_more"`
	Omitted         []string  `json:"omitted"`
	Reconstructable bool      `json:"reconstructable"`
}

func Describe(d Document, offset, limit int) Description {
	ids := keys(d.Elements)
	all := []Summary{}
	for _, id := range ids {
		e := d.Elements[id]
		if !e.Decorative {
			all = append(all, Summary{ID: id, Kind: e.Kind, Label: e.Label, Detail: e.Detail, Description: e.Description, From: e.From, To: e.To, Parent: e.Parent, Icon: e.Icon, CodeLinks: len(e.Evidence), CriterionLinks: len(e.CriterionLinks)})
		}
	}
	offset = min(offset, len(all))
	end := min(offset+limit, len(all))
	return Description{ID: d.ID, Title: d.Title, Snapshot: Snapshot(d), Elements: all[offset:end], Offset: offset, Total: len(all), NextOffset: end, HasMore: end < len(all), Omitted: []string{"geometry", "styles", "decorative_elements", "asset_bytes", "evidence_bodies"}, Reconstructable: false}
}

var textSections = []struct{ kind, heading string }{{"node", "Nodes"}, {"edge", "Edges"}, {"group", "Groups"}, {"text", "Text"}, {"graphic", "Graphics"}}

// Text renders the description as compact, Graphviz-like reading text. It is a
// view of the same projection as the JSON format, not an editable syntax.
func (v Description) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Diagram: %s %s\nSnapshot: %s\n", v.ID, strconv.Quote(v.Title), v.Snapshot)
	fmt.Fprintf(&b, "View: semantic; omits %s; cannot reconstruct the drawing\n", strings.Join(v.Omitted, ", "))
	for _, section := range textSections {
		heading := false
		for _, e := range v.Elements {
			if e.Kind != section.kind {
				continue
			}
			if !heading {
				fmt.Fprintf(&b, "\n%s:\n", section.heading)
				heading = true
			}
			b.WriteString("  " + e.ID)
			if e.Kind == "edge" {
				b.WriteString(": " + orUnset(e.From) + " -> " + orUnset(e.To))
			}
			if e.Label != "" {
				b.WriteString(" " + strconv.Quote(e.Label))
			}
			for _, attr := range [][2]string{{"in", e.Parent}, {"icon", e.Icon}} {
				if attr[1] != "" {
					fmt.Fprintf(&b, " %s=%s", attr[0], attr[1])
				}
			}
			for _, count := range []struct {
				name string
				n    int
			}{{"code_links", e.CodeLinks}, {"criterion_links", e.CriterionLinks}} {
				if count.n > 0 {
					fmt.Fprintf(&b, " %s=%d", count.name, count.n)
				}
			}
			b.WriteString("\n")
			for _, field := range [][2]string{{"detail", e.Detail}, {"description", e.Description}} {
				if field[1] != "" {
					fmt.Fprintf(&b, "    %s: %s\n", field[0], plain(field[1]))
				}
			}
		}
	}
	switch {
	case v.Total == 0:
		b.WriteString("\nNo semantic elements.\n")
	case len(v.Elements) == 0:
		fmt.Fprintf(&b, "\nNo elements at offset %d of %d.\n", v.Offset, v.Total)
	case v.HasMore:
		fmt.Fprintf(&b, "\nShowing %d-%d of %d; continue with --offset %d.\n", v.Offset+1, v.NextOffset, v.Total, v.NextOffset)
	default:
		fmt.Fprintf(&b, "\nShowing %d-%d of %d.\n", v.Offset+1, v.NextOffset, v.Total)
	}
	return b.String()
}

func orUnset(id string) string {
	if id == "" {
		return "(unconnected)"
	}
	return id
}

// plain leaves ordinary prose unquoted but quotes anything that could break the
// line structure or be misread, such as newlines or surrounding spaces.
func plain(s string) string {
	if q := strconv.Quote(s); q[1:len(q)-1] == s && strings.TrimSpace(s) == s {
		return s
	}
	return strconv.Quote(s)
}
