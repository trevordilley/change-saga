package diagram

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"strings"
)

// Render draws the exact tree used by Describe without resolving again.
func (c *Composition) Render(options Options) ([]byte, error) {
	svg, err := renderTree(c.document, options, c, "")
	if err != nil {
		return nil, err
	}
	svg.set("data-whiteboard", "true")
	var out bytes.Buffer
	svg.write(&out, 0)
	return out.Bytes(), nil
}
func (r *renderer) instance(e Element) Instance {
	return r.composition.instances[r.composition.index[joinInstance(r.scope, elementPath(r.doc, e))]]
}
func sourceAttributes(g *node, s *SourceReference) {
	if s == nil {
		return
	}
	g.set("data-documentation-target", s.Target)
	g.set("data-documentation-revision", s.Revision)
	if s.Element != "" {
		g.set("data-source-element", s.Element)
	}
	if s.Relationship != "" {
		g.set("data-source-relationship", s.Relationship)
	}
}
func (r *renderer) compositionMetadata(e Element, g *node) {
	i := r.instance(e)
	g.set("data-instance-path", i.Path)
	sourceAttributes(g, i.Source)
	if e.Visibility != "" {
		g.set("data-visibility", e.Visibility)
	}
	if i.From != "" {
		g.set("data-from", i.From)
	}
	if i.To != "" {
		g.set("data-to", i.To)
	}
	if i.About != "" {
		g.set("data-about", i.About)
	}
}
func (r *renderer) compositionFrame(e Element, style Style, g *node) error {
	i := r.instance(e)
	source := r.composition.sources[i.Path]
	threshold := e.DetailScale
	if threshold == 0 {
		threshold = 0.6
	}
	g.set("data-frame-path", i.Path)
	g.set("data-detail-scale", num(threshold))
	g.set("data-frame-width", num(e.Width))
	g.set("data-frame-height", num(e.Height))
	g.set("data-opaque", fmt.Sprint(i.Opaque))
	border := g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "8")
	paint(border, style)
	scale, dx, dy := frameFit(e, source)
	detail := g.add("g", "data-frame-detail", i.Path, "transform", fmt.Sprintf("translate(%s %s) scale(%s)", num(dx), num(dy), num(scale)))
	if source.Document != nil {
		child, err := renderTree(*source.Document, Options{Title: e.Label, Description: e.Summary}, r.composition, i.Path)
		if err != nil {
			return err
		}
		child.set("width", num(source.Width))
		child.set("height", num(source.Height))
		child.set("preserveAspectRatio", "none")
		namespaceSVG(child, "instance-"+base64.RawURLEncoding.EncodeToString([]byte(i.Path))+"-")
		detail.children = append(detail.children, child)
	} else {
		detail.add("image", "role", "img", "width", num(source.Width), "height", num(source.Height), "preserveAspectRatio", "none", "href", "data:image/svg+xml;base64,"+base64.StdEncoding.EncodeToString(source.SVG), "aria-label", e.Label)
		// Invisible but inspectable binding regions carry provenance and focus boxes.
		for _, b := range source.Bindings {
			path := joinInstance(i.Path, b.ID)
			bg := detail.add("g", "data-instance-path", path, "data-binding", "true", "role", "group", "aria-label", b.Label)
			sourceAttributes(bg, b.Source)
			if b.Bounds != nil {
				box := b.Bounds
				bg.add("rect", "x", num(box.X-source.originX), "y", num(box.Y-source.originY), "width", num(box.Width), "height", num(box.Height), "fill", "transparent")
			}
		}
	}
	overview := g.add("g", "data-frame-overview", i.Path, "style", "display:none")
	title := e.Label
	if title == "" {
		title = e.ID
	}
	padding := math.Min(16, e.Width/10)
	titleHeight := style.FontSize * lineHeight
	if err := r.text(overview, title, Box{X: padding, Y: padding, Width: e.Width - 2*padding, Height: titleHeight}, style.FontSize, style.Ink, true, "start"); err != nil {
		return fmt.Errorf("frame title: %w", err)
	}
	if e.Summary != "" {
		if err := r.text(overview, e.Summary, Box{X: padding, Y: padding + titleHeight + 8, Width: e.Width - 2*padding, Height: e.Height - 2*padding - titleHeight - 8}, style.FontSize, style.Ink, true, "start"); err != nil {
			return fmt.Errorf("frame summary: %w", err)
		}
	}
	return nil
}

// namespaceSVG scopes all generated IDs and internal references when embedding
// repeated documents. Data attributes retain canonical semantic instance paths.
func namespaceSVG(root *node, prefix string) {
	ids := map[string]string{}
	var collect func(*node)
	collect = func(n *node) {
		for _, a := range n.attrs {
			if a[0] == "id" {
				ids[a[1]] = prefix + a[1]
			}
		}
		for _, c := range n.children {
			collect(c)
		}
	}
	collect(root)
	var rewrite func(*node)
	rewrite = func(n *node) {
		for j, a := range n.attrs {
			switch a[0] {
			case "id":
				n.attrs[j][1] = ids[a[1]]
			case "aria-labelledby", "aria-describedby":
				parts := strings.Fields(a[1])
				for k, p := range parts {
					if v, ok := ids[p]; ok {
						parts[k] = v
					}
				}
				n.attrs[j][1] = strings.Join(parts, " ")
			case "href", "xlink:href":
				if v, ok := ids[strings.TrimPrefix(a[1], "#")]; ok && strings.HasPrefix(a[1], "#") {
					n.attrs[j][1] = "#" + v
				}
			default:
				if strings.HasPrefix(a[1], "url(#") && strings.HasSuffix(a[1], ")") {
					if v, ok := ids[a[1][5:len(a[1])-1]]; ok {
						n.attrs[j][1] = "url(#" + v + ")"
					}
				}
			}
		}
		for _, c := range n.children {
			rewrite(c)
		}
	}
	rewrite(root)
}
func boxCenter(b *Box) Point { return Point{b.X + b.Width/2, b.Y + b.Height/2} }
func boxBoundary(b *Box, toward Point) Point {
	center := boxCenter(b)
	dx, dy := toward.X-center.X, toward.Y-center.Y
	if dx == 0 && dy == 0 {
		return center
	}
	scale := math.Inf(1)
	if dx != 0 {
		scale = math.Min(scale, b.Width/2/math.Abs(dx))
	}
	if dy != 0 {
		scale = math.Min(scale, b.Height/2/math.Abs(dy))
	}
	return Point{center.X + dx*scale, center.Y + dy*scale}
}

// overviewEndpoint collapses onto the outermost frame in the connection's
// document. Original instance endpoints remain on the connection metadata.
func (c *Composition) overviewEndpoint(path, scope string) Instance {
	out := c.instances[c.index[path]]
	for parent := out.Parent; parent != "" && parent != scope; {
		i := c.instances[c.index[parent]]
		if i.Element.Kind == "frame" {
			out = i
		}
		parent = i.Parent
	}
	return out
}
func (r *renderer) compositionConnection(e Element, style Style, g *node) error {
	i := r.instance(e)
	c := r.composition
	from, to := c.instances[c.index[i.From]], c.instances[c.index[i.To]]
	draw := func(a, b Instance, group *node, suffix string) error {
		startToward, endToward := boxCenter(b.Bounds), boxCenter(a.Bounds)
		if len(e.Bends) > 0 {
			startToward = i.Transform.point(e.Bends[0])
			endToward = i.Transform.point(e.Bends[len(e.Bends)-1])
		}
		edge := e
		edge.ID = "diagram-composition-" + e.ID + suffix
		edge.From, edge.To = i.From, i.To
		edge.Kind = "edge"
		edge.Points = []Point{i.Transform.local(boxBoundary(a.Bounds, startToward))}
		edge.Points = append(edge.Points, e.Bends...)
		edge.Points = append(edge.Points, i.Transform.local(boxBoundary(b.Bounds, endToward)))
		return r.edge(edge, style, group)
	}
	if err := draw(from, to, g.add("g", "data-connection-detail", i.Path), "-detail"); err != nil {
		return err
	}
	a, b := c.overviewEndpoint(i.From, i.scope), c.overviewEndpoint(i.To, i.scope)
	overview := g.add("g", "data-connection-overview", i.Path, "style", "display:none", "data-from-frame", a.Path, "data-to-frame", b.Path)
	return draw(a, b, overview, "-overview")
}
