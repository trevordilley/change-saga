package diagram

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
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
	w.WriteString(indent)
	// Text content is whitespace-significant under xml:space="preserve", so a
	// text element and its tspans are written inline without indentation.
	if n.tag == "text" {
		n.writeInline(w)
		w.WriteString("\n")
		return
	}
	n.open(w)
	switch {
	case len(n.children) == 0 && n.text == "":
		w.WriteString("/>\n")
	case len(n.children) == 0:
		w.WriteString(">")
		escape(w, n.text, false)
		w.WriteString("</" + n.tag + ">\n")
	default:
		w.WriteString(">\n")
		for _, child := range n.children {
			child.write(w, depth+1)
		}
		w.WriteString(indent + "</" + n.tag + ">\n")
	}
}

func (n *node) open(w *bytes.Buffer) {
	w.WriteString("<" + n.tag)
	for _, attr := range n.attrs {
		w.WriteString(" " + attr[0] + `="`)
		escape(w, attr[1], true)
		w.WriteString(`"`)
	}
}

func (n *node) writeInline(w *bytes.Buffer) {
	n.open(w)
	if len(n.children) == 0 && n.text == "" {
		w.WriteString("/>")
		return
	}
	w.WriteString(">")
	escape(w, n.text, false)
	for _, child := range n.children {
		child.writeInline(w)
	}
	w.WriteString("</" + n.tag + ">")
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

// maxFragmentDepth bounds nesting so a small fragment cannot expand into an
// enormous indented rendering.
const maxFragmentDepth = 24

var (
	numberValue    = `[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?`
	lengthValue    = regexp.MustCompile(`^` + numberValue + `(px|%)?$`)
	numberList     = regexp.MustCompile(`^(\s*` + numberValue + `\s*,?)*$`)
	colorValue     = regexp.MustCompile(`^(#[0-9a-fA-F]{3}|#[0-9a-fA-F]{6}|none|currentColor|[a-z]{3,20})$`)
	transformValue = regexp.MustCompile(`^(\s*(matrix|translate|scale|rotate|skewX|skewY)\s*\((\s*` + numberValue + `\s*,?)*\)\s*,?)*\s*$`)
	dashValue      = regexp.MustCompile(`^(none|(\s*` + numberValue + `\s*,?)+)$`)
)

func enum(values ...string) *regexp.Regexp {
	return regexp.MustCompile(`^(` + strings.Join(values, "|") + `)$`)
}

// fragmentAttributes gives every allowed attribute its own value grammar.
// Values are matched after XML decoding, so no escape sequence, entity, url(),
// or script can hide inside an otherwise allowed attribute.
var fragmentAttributes = map[string]*regexp.Regexp{
	"d": pathData, "points": numberList, "transform": transformValue,
	"x": lengthValue, "y": lengthValue, "x1": lengthValue, "y1": lengthValue, "x2": lengthValue, "y2": lengthValue,
	"cx": lengthValue, "cy": lengthValue, "r": lengthValue, "rx": lengthValue, "ry": lengthValue, "dx": lengthValue, "dy": lengthValue,
	"width": lengthValue, "height": lengthValue, "stroke-width": lengthValue, "font-size": lengthValue,
	"opacity": lengthValue, "fill-opacity": lengthValue, "stroke-opacity": lengthValue,
	"fill": colorValue, "stroke": colorValue, "stroke-dasharray": dashValue,
	"stroke-linecap": enum("butt", "round", "square"), "stroke-linejoin": enum("miter", "round", "bevel"),
	"fill-rule": enum("nonzero", "evenodd"), "font-weight": enum("normal", "bold", "[1-9]00"),
	"text-anchor": enum("start", "middle", "end"), "dominant-baseline": enum("auto", "middle", "central", "hanging", "alphabetic", "text-top", "text-bottom"),
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
			if len(stack) > maxFragmentDepth {
				return nil, nil, fmt.Errorf("fragment nests deeper than %d elements", maxFragmentDepth)
			}
			child := &node{tag: t.Name.Local}
			seen := map[string]bool{}
			for _, attr := range t.Attr {
				grammar := fragmentAttributes[attr.Name.Local]
				if attr.Name.Space != "" || grammar == nil {
					return nil, nil, fmt.Errorf("fragment attribute %s is not supported", attr.Name.Local)
				}
				if seen[attr.Name.Local] {
					return nil, nil, fmt.Errorf("fragment attribute %s is repeated", attr.Name.Local)
				}
				seen[attr.Name.Local] = true
				value := strings.TrimSpace(attr.Value)
				if !grammar.MatchString(value) {
					return nil, nil, fmt.Errorf("fragment attribute %s has an unsupported value %q", attr.Name.Local, attr.Value)
				}
				child.attrs = append(child.attrs, [2]string{attr.Name.Local, value})
			}
			parent := stack[len(stack)-1]
			if parent.text != "" {
				return nil, nil, fmt.Errorf("fragment text may not mix characters and child elements")
			}
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
				if len(current.children) > 0 {
					return nil, nil, fmt.Errorf("fragment text may not mix characters and child elements")
				}
				current.text += text
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return nil, nil, fmt.Errorf("fragment may not contain comments, processing instructions, or directives")
		}
	}
	return rootAttrs, root.children, nil
}
