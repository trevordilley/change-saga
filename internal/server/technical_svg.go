package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// An authored ERD is shown as drawn, but the author's bytes never reach the
// HTML parser verbatim. The drawing is parsed as XML and written back from
// its tokens, keeping only static drawing elements and presentation
// attributes. Links are same-document fragments only. Every id is namespaced
// to the view, so a drawing cannot shadow the page's own ids and two drawings
// cannot collide; bindings are rewritten to the same namespaced ids.
//
// A drawing that needs anything outside the allowlist is not shown in part:
// silently altering what the author drew would misrepresent it. The page says
// the drawing is unavailable and why, and the directory still lists every
// entity.

var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true, "symbol": true, "use": true,
	"rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true, "path": true,
	"text": true, "tspan": true, "textPath": true, "marker": true, "clipPath": true, "mask": true, "pattern": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
}

var svgAttributes = map[string]bool{
	"id": true, "x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true,
	"r": true, "rx": true, "ry": true, "width": true, "height": true, "d": true, "points": true,
	"viewBox": true, "preserveAspectRatio": true, "transform": true, "dx": true, "dy": true, "rotate": true,
	"textLength": true, "lengthAdjust": true, "text-anchor": true, "dominant-baseline": true, "alignment-baseline": true,
	"font-family": true, "font-size": true, "font-weight": true, "font-style": true, "letter-spacing": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true, "stroke-width": true, "stroke-opacity": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-miterlimit": true,
	"opacity": true, "visibility": true, "display": true, "vector-effect": true, "clip-path": true, "clip-rule": true, "mask": true,
	"marker-start": true, "marker-mid": true, "marker-end": true, "markerWidth": true, "markerHeight": true,
	"markerUnits": true, "refX": true, "refY": true, "orient": true, "offset": true, "stop-color": true, "stop-opacity": true,
	"gradientUnits": true, "gradientTransform": true, "spreadMethod": true, "fx": true, "fy": true,
	"patternUnits": true, "patternContentUnits": true, "patternTransform": true, "clipPathUnits": true,
	"maskUnits": true, "maskContentUnits": true, "href": true, "role": true, "aria-label": true, "aria-hidden": true,
}

// svgURL is a url() reference; only same-document fragments are kept.
var svgURL = regexp.MustCompile(`url\(\s*['"]?([^'")\s]*)['"]?\s*\)`)

// sanitizeSVG re-serializes an authored drawing, namespacing its ids with
// prefix. It returns the safe markup, and the reasons it refused, if any.
func sanitizeSVG(data []byte, prefix string) (string, []string) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	var out strings.Builder
	var refused []string
	refuse := func(format string, args ...any) { refused = append(refused, fmt.Sprintf(format, args...)) }
	skip, roots := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", []string{"it is not well-formed XML"}
		}
		switch t := token.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			name := t.Name.Local
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				refuse("element %s from another namespace", name)
				skip = 1
				continue
			}
			if !svgElements[name] {
				refuse("element <%s>", name)
				skip = 1
				continue
			}
			if name == "svg" {
				roots++
			}
			out.WriteString("<" + name)
			if name == "svg" && roots == 1 {
				out.WriteString(` xmlns="http://www.w3.org/2000/svg"`)
			}
			attributes := append([]xml.Attr(nil), t.Attr...)
			sort.SliceStable(attributes, func(i, j int) bool { return attributes[i].Name.Local < attributes[j].Name.Local })
			for _, attribute := range attributes {
				key, value := attribute.Name.Local, attribute.Value
				switch {
				case attribute.Name.Space == "xmlns" || (attribute.Name.Space == "" && key == "xmlns"):
					continue // namespace declarations are restated, not copied
				case attribute.Name.Space != "" && !(attribute.Name.Space == "http://www.w3.org/1999/xlink" && key == "href"):
					refuse("attribute %s:%s", attribute.Name.Space, key)
					continue
				case !svgAttributes[key]:
					refuse("attribute %s on <%s>", key, name)
					continue
				}
				switch key {
				case "id":
					value = prefix + value
				case "href":
					if !strings.HasPrefix(value, "#") {
						refuse("a link that is not a same-document fragment on <%s>", name)
						continue
					}
					value = "#" + prefix + value[1:]
				}
				if strings.Contains(strings.ToLower(value), "url(") {
					bad := false
					value = svgURL.ReplaceAllStringFunc(value, func(match string) string {
						target := svgURL.FindStringSubmatch(match)[1]
						if !strings.HasPrefix(target, "#") {
							bad = true
							return match
						}
						return "url(#" + prefix + target[1:] + ")"
					})
					if bad {
						refuse("a url() that is not a same-document fragment on <%s>", name)
						continue
					}
				}
				out.WriteString(" " + key + `="`)
				xml.EscapeText(&escapeWriter{&out}, []byte(value))
				out.WriteString(`"`)
			}
			out.WriteString(">")
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			out.WriteString("</" + t.Name.Local + ">")
		case xml.CharData:
			if skip == 0 {
				xml.EscapeText(&escapeWriter{&out}, t)
			}
		case xml.Directive:
			refuse("a document directive")
		case xml.ProcInst:
			if t.Target != "xml" {
				refuse("a processing instruction")
			}
		}
	}
	if roots != 1 {
		refuse("it must have exactly one <svg> root")
	}
	if len(refused) > 0 {
		return "", refused
	}
	return out.String(), nil
}

type escapeWriter struct{ b *strings.Builder }

func (w *escapeWriter) Write(p []byte) (int, error) { return w.b.Write(p) }
