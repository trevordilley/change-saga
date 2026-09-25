package diagram

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// node is a minimal SVG element tree with deterministic serialization. The
// renderer never serializes author-supplied markup directly: fragments are
// parsed into this tree through an allowlist first.
type node struct {
	tag      string
	attrs    [][2]string
	text     string
	children []*node
}

func newNode(tag string, attrs ...string) *node {
	n := &node{tag: tag}
	for i := 0; i+1 < len(attrs); i += 2 {
		n.attrs = append(n.attrs, [2]string{attrs[i], attrs[i+1]})
	}
	return n
}

func (n *node) add(tag string, attrs ...string) *node {
	child := newNode(tag, attrs...)
	n.children = append(n.children, child)
	return child
}

func (n *node) set(name, value string) { n.attrs = append(n.attrs, [2]string{name, value}) }

func (n *node) write(w *bytes.Buffer, depth int) {
	indent := strings.Repeat("  ", depth)
	w.WriteString(indent + "<" + n.tag)
	for _, attr := range n.attrs {
		w.WriteString(" " + attr[0] + `="`)
		escape(w, attr[1], true)
		w.WriteString(`"`)
	}
	switch {
	case len(n.children) == 0 && n.text == "":
		w.WriteString("/>\n")
	case len(n.children) == 0:
		w.WriteString(">")
		escape(w, n.text, false)
		w.WriteString("</" + n.tag + ">\n")
	default:
		w.WriteString(">\n")
		if n.text != "" {
			w.WriteString(indent + "  ")
			escape(w, n.text, false)
			w.WriteString("\n")
		}
		for _, child := range n.children {
			child.write(w, depth+1)
		}
		w.WriteString(indent + "</" + n.tag + ">\n")
	}
}

// escape writes value with only the escapes XML requires, replacing
// characters XML cannot represent with U+FFFD.
func escape(w *bytes.Buffer, value string, attribute bool) {
	for _, r := range value {
		switch {
		case r == '&':
			w.WriteString("&amp;")
		case r == '<':
			w.WriteString("&lt;")
		case r == '>':
			w.WriteString("&gt;")
		case attribute && r == '"':
			w.WriteString("&quot;")
		case attribute && (r == '\n' || r == '\r' || r == '\t'):
			fmt.Fprintf(w, "&#%d;", r)
		case r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF):
			w.WriteRune(r)
		default:
			w.WriteRune('\uFFFD')
		}
	}
}

const svgNamespace = "http://www.w3.org/2000/svg"

var fragmentElements = map[string]bool{
	"g": true, "path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true, "title": true, "desc": true,
}

var fragmentAttributes = map[string]bool{
	"d": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true,
	"r": true, "rx": true, "ry": true, "dx": true, "dy": true, "width": true, "height": true, "points": true,
	"fill": true, "stroke": true, "stroke-width": true, "stroke-dasharray": true, "stroke-linecap": true,
	"stroke-linejoin": true, "fill-rule": true, "opacity": true, "fill-opacity": true, "stroke-opacity": true,
	"transform": true, "font-size": true, "font-weight": true, "text-anchor": true, "dominant-baseline": true,
}

// parseFragment accepts a bounded subset of SVG drawing markup. It refuses
// scripts, links, styles, IDs, external references, and anything unlisted,
// so a graphic can never import behavior or collide with Item selectors.
func parseFragment(fragment string) ([]*node, error) {
	_, children, err := parseMarkup("<fragment>" + fragment + "</fragment>")
	return children, err
}

// parseMarkup sanitizes the children of markup's single root element through
// the fragment allowlist and returns the root's attributes unfiltered for the
// caller to select from.
func parseMarkup(markup string) ([]xml.Attr, []*node, error) {
	decoder := xml.NewDecoder(strings.NewReader(markup))
	decoder.Strict = true
	root := &node{tag: "root"}
	var rootAttrs []xml.Attr
	stack := []*node{root}
	wrapperOpen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("fragment is not well-formed SVG: %v", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if !wrapperOpen {
				wrapperOpen, rootAttrs = true, t.Attr
				continue
			}
			if (t.Name.Space != "" && t.Name.Space != svgNamespace) || !fragmentElements[t.Name.Local] {
				return nil, nil, fmt.Errorf("fragment element <%s> is not supported", t.Name.Local)
			}
			child := &node{tag: t.Name.Local}
			for _, attr := range t.Attr {
				value := strings.TrimSpace(attr.Value)
				lower := strings.ToLower(value)
				if attr.Name.Space != "" || !fragmentAttributes[attr.Name.Local] || strings.Contains(lower, "url(") || strings.Contains(lower, "javascript:") {
					return nil, nil, fmt.Errorf("fragment attribute %s is not supported", attr.Name.Local)
				}
				child.attrs = append(child.attrs, [2]string{attr.Name.Local, attr.Value})
			}
			parent := stack[len(stack)-1]
			parent.children = append(parent.children, child)
			stack = append(stack, child)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if text := strings.TrimSpace(string(t)); text != "" {
				current := stack[len(stack)-1]
				if current == root || (current.tag != "text" && current.tag != "tspan" && current.tag != "title" && current.tag != "desc") {
					return nil, nil, fmt.Errorf("fragment text must be inside text, tspan, title, or desc")
				}
				current.text += text
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return nil, nil, fmt.Errorf("fragment may not contain comments, processing instructions, or directives")
		}
	}
	return rootAttrs, root.children, nil
}
