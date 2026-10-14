package diagram

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode"
)

// SourceReference pins a depicted record. Element and Relationship are optional
// selectors within that revision; neither a label nor a connection invents one.
type SourceReference struct {
	Target       string `json:"target"`
	Revision     string `json:"revision"`
	Element      string `json:"element,omitempty"`
	Relationship string `json:"relationship,omitempty"`
}

// Binding supplies explicit semantics for an otherwise opaque SVG. Bounds, when
// present, are in the SVG viewBox coordinate system and permit connections.
type Binding struct {
	ID          string           `json:"id"`
	Source      *SourceReference `json:"source,omitempty"`
	Bounds      *Box             `json:"bounds,omitempty"`
	Label       string           `json:"label,omitempty"`
	Description string           `json:"description,omitempty"`
}

// ResolvedSource is caller-owned data for an immutable source revision. Exactly
// one of Document or SVG is required. SVG is embedded as an isolated image, not
// interpreted as structured diagram source. Width/Height optionally override
// its viewport dimensions (otherwise the root viewBox or dimensions are used).
type ResolvedSource struct {
	originX, originY float64
	Document         *Document
	SVG              []byte
	Width, Height    float64
	Bindings         []Binding
}

type Resolver func(SourceReference) (ResolvedSource, error)

// Transform maps local coordinates into board coordinates. Frames uniformly
// scale and center children into their authored rectangle; ordinary groups translate.
type Transform struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	ScaleX float64 `json:"scale_x"`
	ScaleY float64 `json:"scale_y"`
}

func (t Transform) point(p Point) Point { return Point{t.X + p.X*t.ScaleX, t.Y + p.Y*t.ScaleY} }
func (t Transform) box(b Box) *Box {
	p := t.point(Point{b.X, b.Y})
	return &Box{p.X, p.Y, b.Width * t.ScaleX, b.Height * t.ScaleY}
}
func (t Transform) local(p Point) Point { return Point{(p.X - t.X) / t.ScaleX, (p.Y - t.Y) / t.ScaleY} }

// Instance identifies a particular appearance, even when a source is repeated.
// From/To/About are canonical board paths; Bounds and Transform are board-space.
type Instance struct {
	Path      string           `json:"path"`
	Parent    string           `json:"parent,omitempty"`
	Element   Element          `json:"element"`
	Source    *SourceReference `json:"source,omitempty"`
	Bounds    *Box             `json:"bounds,omitempty"`
	Transform Transform        `json:"transform"`
	Opaque    bool             `json:"opaque,omitempty"`
	From      string           `json:"from,omitempty"`
	To        string           `json:"to,omitempty"`
	About     string           `json:"about,omitempty"`
	scope     string
}

type CompositionDescription struct {
	Focus       string     `json:"focus,omitempty"`
	Width       float64    `json:"width"`
	Height      float64    `json:"height"`
	Elements    []Instance `json:"elements"`
	Connections []Instance `json:"connections"`
	Context     []Instance `json:"context"`
}

// Composition is a snapshot shared by rendering and inspection. Resolution is
// bounded, detects recursive revision cycles, and never imports storage APIs.
type Composition struct {
	document  Document
	instances []Instance
	index     map[string]int
	sources   map[string]ResolvedSource
}

const maxCompositionInstances = 10000
const maxCompositionDepth = 16
const maxCompositionSVGBytes = 32 << 20

var bindingID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]{0,127}$`)

func validateSource(s *SourceReference) error {
	if s == nil || strings.TrimSpace(s.Target) == "" || strings.TrimSpace(s.Revision) == "" {
		return fmt.Errorf("source requires explicit target and pinned revision")
	}
	for _, value := range []string{s.Target, s.Revision, s.Element, s.Relationship} {
		if len(value) > 2048 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("source references must be bounded single-line strings")
		}
	}
	if s.Element != "" && s.Relationship != "" {
		return fmt.Errorf("source selects either element or relationship, not both")
	}
	return nil
}
func validInstancePath(path string) bool {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if !bindingID.MatchString(part) {
			return false
		}
	}
	return true
}
func validateCompositionElement(e Element, fail func(string, ...any)) {
	if e.Source != nil {
		if err := validateSource(e.Source); err != nil {
			fail("%v", err)
		}
	}
	if e.Visibility != "" && e.Visibility != "overview" && e.Visibility != "detail" {
		fail("visibility must be overview or detail")
	}
	if len([]rune(e.Summary)) > MaxLabelRunes {
		fail("summary is too long")
	}
	if !finite(e.DetailScale) || e.DetailScale < 0 || e.DetailScale > 100 {
		fail("detail_scale must be finite and between 0 and 100")
	}
	if e.DetailScale != 0 && e.Kind != "frame" {
		fail("detail_scale applies only to frames")
	}
	if e.Kind == "frame" {
		if e.Decorative {
			fail("frame sources must remain semantic; decorative frames are not supported")
		}
		if e.Source == nil {
			fail("frame requires a pinned source")
		}
		if e.Source != nil && (e.Source.Element != "" || e.Source.Relationship != "") {
			fail("frame source must reference a whole diagram")
		}
		if e.Width <= 0 || e.Height <= 0 {
			fail("frame needs positive width and height")
		}
		if e.Shape != "" {
			fail("frame does not take a shape")
		}
	}
	if e.Kind != "connection" && len(e.Bends) > 0 {
		fail("bends apply only to connections")
	}
	if e.Kind == "connection" {
		if !validInstancePath(e.From) || !validInstancePath(e.To) {
			fail("connection requires from and to instance paths")
		}
		if len(e.Points) > 0 || e.Path != "" || e.Shape != "" {
			fail("connection uses optional bends, not points, path, or shape")
		}
		if e.HeadSize < 0 || e.HeadSize > 100 {
			fail("head_size must be 0-100")
		}
		if e.Label != "" && e.LabelBox == nil {
			fail("connection label requires label_box")
		}
		if len(e.Bends) > 100 {
			fail("connection permits at most 100 bends")
		}
		for _, p := range e.Bends {
			if !finite(p.X) || !finite(p.Y) {
				fail("bends must be finite")
			}
		}
	}
}

func joinInstance(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "/" + child
}
func elementPath(d Document, e Element) string {
	path := e.ID
	for parent := e.Parent; parent != ""; {
		p, _ := d.Element(parent)
		path = p.ID + "/" + path
		parent = p.Parent
	}
	return path
}

// ResolveComposition snapshots d and callback results. Callbacks run once per
// target/revision pair; repeated placements have separate instance identities.
func ResolveComposition(d Document, resolver Resolver) (*Composition, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var documentCopy Document
	if err = json.Unmarshal(data, &documentCopy); err != nil {
		return nil, err
	}
	d = documentCopy
	c := &Composition{document: d, index: map[string]int{}, sources: map[string]ResolvedSource{}}
	totalSVGBytes := 0
	cache := map[string]ResolvedSource{}
	active := map[string]bool{}
	var visit func(Document, string, Transform, int) error
	visit = func(doc Document, scope string, t Transform, depth int) error {
		if depth > maxCompositionDepth {
			return fmt.Errorf("composition exceeds %d nested frames", maxCompositionDepth)
		}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("%s: %w", scope, err)
		}
		for _, e := range doc.Elements {
			path := joinInstance(scope, elementPath(doc, e))
			parent := scope
			if e.Parent != "" {
				p, _ := doc.Element(e.Parent)
				parent = joinInstance(scope, elementPath(doc, p))
			}
			ox, oy := origin(doc, e.ID)
			world := t.point(Point{ox, oy})
			transform := Transform{world.X, world.Y, t.ScaleX, t.ScaleY}
			inst := Instance{Path: path, Parent: parent, Element: e, Source: e.Source, Transform: transform, scope: scope}
			if e.Width > 0 && e.Height > 0 {
				inst.Bounds = transform.box(Box{Width: e.Width, Height: e.Height})
			}
			if err := c.add(inst); err != nil {
				return err
			}
			if e.Kind != "frame" {
				continue
			}
			if resolver == nil {
				return fmt.Errorf("%s: frame requires a resolver", path)
			}
			key := e.Source.Target + "\x00" + e.Source.Revision
			if active[key] {
				return fmt.Errorf("%s: recursive source cycle at %s revision %s", path, e.Source.Target, e.Source.Revision)
			}
			resolved, ok := cache[key]
			if !ok {
				resolved, err = resolver(*e.Source)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				if (resolved.Document == nil) == (len(resolved.SVG) == 0) {
					return fmt.Errorf("%s: resolver must return exactly one Document or SVG", path)
				}
				// Copy caller-owned slices, pointers, and maps before retaining them.
				snapshot, err := json.Marshal(resolved)
				if err != nil {
					return err
				}
				var resolvedCopy ResolvedSource
				if err = json.Unmarshal(snapshot, &resolvedCopy); err != nil {
					return err
				}
				resolved = resolvedCopy
				if resolved.Document != nil {
					resolved.Width = resolved.Document.Width
					resolved.Height = resolved.Document.Height
				} else {
					width, height, err := svgSize(resolved.SVG)
					resolved.originX, resolved.originY = svgOrigin(resolved.SVG)
					if err != nil {
						return fmt.Errorf("%s: %w", path, err)
					}
					if resolved.Width == 0 {
						resolved.Width = width
					}
					if resolved.Height == 0 {
						resolved.Height = height
					}
				}
				if !finite(resolved.Width) || !finite(resolved.Height) || resolved.Width <= 0 || resolved.Height <= 0 {
					return fmt.Errorf("%s: child requires positive finite dimensions", path)
				}
				cache[key] = resolved
			}
			totalSVGBytes += len(resolved.SVG)
			if totalSVGBytes > maxCompositionSVGBytes {
				return fmt.Errorf("composition SVG images exceed %d bytes", maxCompositionSVGBytes)
			}
			c.sources[path] = resolved
			scale, dx, dy := frameFit(e, resolved)
			childTransform := Transform{world.X + t.ScaleX*(dx-scale*resolved.originX), world.Y + t.ScaleY*(dy-scale*resolved.originY), t.ScaleX * scale, t.ScaleY * scale}
			if !finite(childTransform.X) || !finite(childTransform.Y) || !finite(childTransform.ScaleX) || !finite(childTransform.ScaleY) {
				return fmt.Errorf("%s: composed transform exceeds geometry limits", path)
			}
			if resolved.Document != nil {
				active[key] = true
				if err := visit(*resolved.Document, path, childTransform, depth+1); err != nil {
					return err
				}
				delete(active, key)
			} else {
				c.instances[c.index[path]].Opaque = true
			}
			seenBindings := map[string]bool{}
			for _, b := range resolved.Bindings {
				if !validInstancePath(b.ID) || strings.HasPrefix(b.ID, "/") || (resolved.Document == nil && !bindingID.MatchString(b.ID)) {
					return fmt.Errorf("%s: invalid binding ID %q", path, b.ID)
				}
				if err := validateSource(b.Source); err != nil {
					return fmt.Errorf("%s/%s: %w", path, b.ID, err)
				}
				bindingPath := c.reference(path, b.ID)
				if seenBindings[bindingPath] {
					return fmt.Errorf("%s: duplicate binding", bindingPath)
				}
				seenBindings[bindingPath] = true
				if idx, ok := c.index[bindingPath]; ok && resolved.Document != nil {
					if existing := c.instances[idx].Source; existing != nil && *existing != *b.Source {
						return fmt.Errorf("%s: binding conflicts with explicit source", bindingPath)
					}
					c.instances[idx].Source = b.Source
					continue
				}
				if resolved.Document != nil {
					return fmt.Errorf("%s: binding names unknown structured element %s", path, b.ID)
				}
				binding := Instance{Path: bindingPath, Parent: path, Element: Element{ID: b.ID, Kind: "binding", Label: b.Label, Description: b.Description}, Source: b.Source, Transform: childTransform, scope: path}
				if b.Bounds != nil {
					box := *b.Bounds
					if !finite(box.X) || !finite(box.Y) || !finite(box.Width) || !finite(box.Height) || box.Width <= 0 || box.Height <= 0 {
						return fmt.Errorf("%s: binding requires finite positive bounds", bindingPath)
					}
					binding.Bounds = childTransform.box(box)
				}
				if err := c.add(binding); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(d, "", Transform{ScaleX: 1, ScaleY: 1}, 0); err != nil {
		return nil, err
	}
	for i := range c.instances {
		inst := &c.instances[i]
		for _, field := range []struct {
			raw  string
			dest *string
		}{{inst.Element.From, &inst.From}, {inst.Element.To, &inst.To}, {inst.Element.About, &inst.About}} {
			if field.raw == "" {
				continue
			}
			path := c.reference(inst.scope, field.raw)
			target, ok := c.index[path]
			if !ok {
				return nil, fmt.Errorf("%s: unknown instance %q (SVG children require explicit bindings)", inst.Path, field.raw)
			}
			if c.instances[target].Element.Decorative {
				return nil, fmt.Errorf("%s: endpoint %s is decorative", inst.Path, path)
			}
			*field.dest = path
			if (inst.Element.Kind == "connection" || (inst.Element.Shape == "bubble" && inst.Element.Target == nil)) && c.instances[target].Bounds == nil {
				return nil, fmt.Errorf("%s: endpoint %s has no explicit bounds", inst.Path, path)
			}
		}
	}
	return c, nil
}
func (c *Composition) add(i Instance) error {
	if !finite(i.Transform.X) || !finite(i.Transform.Y) || !finite(i.Transform.ScaleX) || !finite(i.Transform.ScaleY) || i.Transform.ScaleX <= 0 || i.Transform.ScaleY <= 0 {
		return fmt.Errorf("%s: invalid composed transform", i.Path)
	}
	if i.Bounds != nil && (!finite(i.Bounds.X) || !finite(i.Bounds.Y) || !finite(i.Bounds.Width) || !finite(i.Bounds.Height)) {
		return fmt.Errorf("%s: composed bounds exceed geometry limits", i.Path)
	}
	if len(c.instances) >= maxCompositionInstances {
		return fmt.Errorf("composition exceeds %d instances", maxCompositionInstances)
	}
	if _, ok := c.index[i.Path]; ok {
		return fmt.Errorf("duplicate instance %s", i.Path)
	}
	c.index[i.Path] = len(c.instances)
	c.instances = append(c.instances, i)
	return nil
}
func (c *Composition) reference(scope, ref string) string {
	if strings.HasPrefix(ref, "/") {
		return strings.TrimPrefix(ref, "/")
	}
	path := joinInstance(scope, ref)
	if _, ok := c.index[path]; ok {
		return path
	}
	// Legacy local IDs remain valid when nodes live in groups.
	for _, i := range c.instances {
		if i.scope == scope && i.Element.ID == ref {
			return i.Path
		}
	}
	return path
}

// Describe returns the focused subtree, enclosing context, and incident
// connections. Opaque frames are explicitly marked and never inferred from SVG.
func (c *Composition) Describe(focus string) (CompositionDescription, error) {
	focus = strings.TrimPrefix(focus, "/")
	out := CompositionDescription{Focus: focus, Width: c.document.Width, Height: c.document.Height, Elements: []Instance{}, Connections: []Instance{}, Context: []Instance{}}
	if focus != "" {
		if _, ok := c.index[focus]; !ok {
			return out, fmt.Errorf("unknown focus %q; SVG internals require explicit bindings", focus)
		}
	}
	selected := func(p string) bool { return focus == "" || p == focus || strings.HasPrefix(p, focus+"/") }
	for _, i := range c.instances {
		if i.Element.Decorative {
			continue
		}
		// Remove drawing-only fragments from the inspection projection.
		i.Element.Fragment = ""
		if selected(i.Path) || (i.About != "" && selected(i.About)) {
			out.Elements = append(out.Elements, i)
		} else if strings.HasPrefix(focus, i.Path+"/") {
			out.Context = append(out.Context, i)
		}
		if (i.Element.Kind == "edge" || i.Element.Kind == "connection") && (selected(i.Path) || selected(i.From) || selected(i.To)) {
			out.Connections = append(out.Connections, i)
		}
	}
	// Do not return mutable references into the shared resolved tree.
	data, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	var result CompositionDescription
	err = json.Unmarshal(data, &result)
	return result, err
}

// svgSize verifies a single whole SVG document without inspecting or promoting
// its internal elements to semantic instances. Image embedding isolates scripts,
// external styles, IDs, and event handlers from the surrounding document.
func svgSize(data []byte) (float64, float64, error) {
	if len(data) > 8<<20 {
		return 0, 0, fmt.Errorf("SVG exceeds 8 MiB")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	var width, height float64
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, fmt.Errorf("invalid whole SVG: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || t.Name.Local != "svg" || (t.Name.Space != "" && t.Name.Space != svgNamespace) {
					return 0, 0, fmt.Errorf("expected one whole SVG root")
				}
				var viewbox string
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "viewBox":
						viewbox = a.Value
					case "width":
						fmt.Sscanf(a.Value, "%f", &width)
					case "height":
						fmt.Sscanf(a.Value, "%f", &height)
					}
				}
				if viewbox != "" {
					var x, y float64
					if n, _ := fmt.Sscanf(strings.ReplaceAll(viewbox, ",", " "), "%f %f %f %f", &x, &y, &width, &height); n != 4 || !finite(x) || !finite(y) || !finite(width) || !finite(height) || width <= 0 || height <= 0 {
						return 0, 0, fmt.Errorf("invalid SVG viewBox")
					}
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return 0, 0, fmt.Errorf("text outside SVG root")
			}
		case xml.Directive:
			return 0, 0, fmt.Errorf("SVG directives are not supported")
		}
	}
	if roots != 1 || depth != 0 {
		return 0, 0, fmt.Errorf("expected one complete SVG")
	}
	return width, height, nil
}

func frameFit(e Element, s ResolvedSource) (scale, dx, dy float64) {
	scale = math.Min(e.Width/s.Width, e.Height/s.Height)
	return scale, (e.Width - s.Width*scale) / 2, (e.Height - s.Height*scale) / 2
}
func svgOrigin(data []byte) (x, y float64) {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		t, err := d.Token()
		if err != nil {
			return
		}
		if root, ok := t.(xml.StartElement); ok {
			for _, a := range root.Attr {
				if a.Name.Local == "viewBox" {
					fmt.Sscanf(strings.ReplaceAll(a.Value, ",", " "), "%f %f", &x, &y)
				}
			}
			return
		}
	}
}
