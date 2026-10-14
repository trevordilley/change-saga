package server

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Capabilities describe this presentation, not what could be inferred from its
// pixels. In particular, SVG text resembling a field never establishes a field
// group, a code reference, an entity, or a relationship.
type whiteboardSourceCapability struct {
	Mode         string
	Note         string
	Bindings     int
	EntityDetail bool
	FieldDetail  bool
	Landmarks    bool
	Code         bool
	Reveal       bool
}

// enhanceWhiteboardSources is a read-only reviewer enhancement. Composition's
// persisted SVG remains deterministic and keeps opaque sources isolated as
// images. Here supported authored sources use the exact standalone ERD safety
// policy and bindings, then join the existing pinned-definition interaction.
// An unsupported source stays a whole image, never a partially sanitized SVG.
func (a *app) enhanceWhiteboardSources(inventory requirements.Inventory, resolved *requirements.ResolvedWhiteboard, visual template.HTML) (template.HTML, map[string]whiteboardSourceCapability) {
	capabilities := map[string]whiteboardSourceCapability{}
	if resolved == nil || resolved.Composition == nil {
		return visual, capabilities
	}
	description, err := resolved.Composition.Describe("")
	if err != nil {
		return visual, capabilities
	}
	// Both inputs to this tree editor have already passed the shared SVG
	// presentation policy: inlineWhiteboardSVG or makeERDViewWithPrefix. This
	// is only a markup editor; it neither interprets nor allows source content.
	board, err := readPresentationSVG(string(visual))
	if err != nil {
		return visual, capabilities
	}
	details := map[string]*presentationSVG{}
	board.walk(func(n *presentationSVG) {
		if path := n.attr("data-frame-detail"); path != "" {
			details[path] = n
		}
	})
	for _, item := range description.Elements {
		if item.Element.Kind != "frame" || item.Source == nil {
			continue
		}
		capability := whiteboardSourceCapability{Mode: "image", Note: "Whole-image preview; internal entities and relationships are not selectable."}
		detail := details[item.Path]
		if detail == nil {
			capabilities[item.Path] = capability
			continue
		}
		if !item.Opaque {
			capability.Mode = "structured"
			capability.Note = "Structured diagram; fields can be expanded. Notes and reveal steps are available only when authored; slide-fragment landmarks are not part of an ERD source."
			detail.walkSource(func(n *presentationSVG) {
				if n.attr("data-field") != "" {
					n.set("data-diagram-detail", "field")
					n.set("display", "none")
					n.set("aria-hidden", "true")
					capability.FieldDetail = true
				}
				if n.attr("data-documentation-target") != "" {
					capability.Bindings++
				}
				if n.attr("data-reveal-step") != "" {
					capability.Reveal = true
				}
				if n.attr("data-diagram-kind") == "code" {
					capability.Code = true
				}
			})
		} else {
			pin := saga.DocumentationLink{Target: item.Source.Target, Revision: item.Source.Revision}
			record, revision := inventory.Find(pin.Target), inventory.Pinned(pin)
			if record != nil && revision != nil {
				prefix := "whiteboard-source-" + base64.RawURLEncoding.EncodeToString([]byte(item.Path)) + "-"
				view := a.makeERDViewWithPrefix(inventory, record, revision, prefix)
				if view.Visual != "" {
					if source, err := readPresentationSVG(string(view.Visual)); err == nil {
						if image := detail.child("image"); image != nil {
							// Preserve the existing image viewport, including a nonzero
							// authored viewBox origin. Do not reinterpret source geometry.
							for _, name := range []string{"width", "height", "preserveAspectRatio"} {
								source.set(name, image.attr(name))
							}
							source.set("data-instance-path", item.Path)
							source.set("data-diagram-source", "authored")
							byID := map[string]*presentationSVG{}
							source.walk(func(n *presentationSVG) {
								if id := n.attr("id"); id != "" {
									byID[id] = n
								}
							})
							for _, binding := range view.Bindings {
								if node := byID[binding.Element]; node != nil {
									element := strings.TrimPrefix(binding.Element, prefix)
									node.set("data-instance-path", item.Path+"/"+element)
									node.set("data-binding", "true")
									node.set("data-documentation-target", binding.Pin.Target)
									node.set("data-documentation-revision", binding.Pin.Revision)
									node.set("aria-label", binding.Label)
									if binding.Relationship != "" {
										node.set("data-source-relationship", binding.Relationship)
									} else if node.name == "g" {
										node.set("data-diagram-entity", "true")
										node.set("data-diagram-label", binding.Label)
										capability.EntityDetail = true
									}
									capability.Bindings++
								}
							}
							// Replace the isolated image and its metadata-only proxy
							// regions together: only actual authored marks are targets.
							detail.children = []*presentationSVG{source}
							capability.Mode = "authored"
							capability.Note = "Authored SVG; only explicitly bound marks are selectable. Entity summaries use authored group bounds; expanded entities show the original artwork. Individual field semantics, slide landmarks and reveal steps are unavailable."
						}
					}
				} else if view.VisualNote != "" {
					capability.Note += " " + view.VisualNote
				}
			}
		}
		detail.set("data-diagram-presentation", capability.Mode)
		capabilities[item.Path] = capability
	}
	return template.HTML(board.markup()), capabilities
}

// presentationSVG edits already validated markup. It deliberately defines no
// SVG allowlist or source policy: those remain in sanitizeSVG/inlineDiagramSVG.
// Mixed content stays ordered so text/tspan/desc retain authored text exactly.
type presentationSVG struct {
	name     string
	attrs    []xml.Attr
	text     string
	children []*presentationSVG
}

func readPresentationSVG(markup string) (*presentationSVG, error) {
	decoder := xml.NewDecoder(strings.NewReader(markup))
	var root *presentationSVG
	var stack []*presentationSVG
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			n := &presentationSVG{name: t.Name.Local, attrs: t.Attr}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			} else if root == nil && n.name == "svg" {
				root = n
			} else {
				return nil, fmt.Errorf("presentation requires one SVG root")
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, &presentationSVG{text: string(t)})
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("presentation has no SVG root")
	}
	return root, nil
}

func (n *presentationSVG) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}
func (n *presentationSVG) set(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].Name.Local == name {
			n.attrs[i].Value = value
			return
		}
	}
	n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}
func (n *presentationSVG) child(name string) *presentationSVG {
	for _, child := range n.children {
		if child.name == name {
			return child
		}
	}
	return nil
}
func (n *presentationSVG) walk(visit func(*presentationSVG)) {
	visit(n)
	for _, child := range n.children {
		child.walk(visit)
	}
}

// walkSource excludes nested source frames, whose capabilities belong to their
// own exact source pin and will be visited independently.
func (n *presentationSVG) walkSource(visit func(*presentationSVG)) {
	visit(n)
	for _, child := range n.children {
		if child.attr("data-frame-path") == "" {
			child.walkSource(visit)
		}
	}
}
func (n *presentationSVG) markup() string {
	var out strings.Builder
	var write func(*presentationSVG)
	write = func(n *presentationSVG) {
		if n.name == "" {
			xml.EscapeText(&escapeWriter{&out}, []byte(n.text))
			return
		}
		out.WriteString("<" + n.name)
		for _, a := range n.attrs {
			name := a.Name.Local
			if a.Name.Space == "http://www.w3.org/XML/1998/namespace" {
				name = "xml:" + name
			}
			out.WriteString(" " + name + `="`)
			xml.EscapeText(&escapeWriter{&out}, []byte(a.Value))
			out.WriteString(`"`)
		}
		out.WriteString(">")
		for _, child := range n.children {
			write(child)
		}
		out.WriteString("</" + n.name + ">")
	}
	write(n)
	return out.String()
}
