package server

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"regexp"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// whiteboardView is a read-only composition on the Data model page. Its
// references lead to the existing, independently readable pinned definitions.
type whiteboardView struct {
	Metadata       string
	Visual         template.HTML
	Name, Revision string
	Note           string
	Frames         []whiteboardFrameView
}

type whiteboardFrameView struct {
	Path, Title, Summary, Href, Revision, Status string
	X, Y, Width, Height                          float64
	Opaque                                       bool
}

type whiteboardGeometry struct {
	Path      string            `json:"path"`
	Bounds    *diagram.Box      `json:"bounds,omitempty"`
	Transform diagram.Transform `json:"transform"`
	Bends     []diagram.Point   `json:"bends,omitempty"`
	Curve     string            `json:"curve,omitempty"`
}

// A missing board is optional; a broken board is explicit without removing
// the ERD documents and entity directory that remain usable below it.
func (a *app) makeWhiteboardView(inventory requirements.Inventory) *whiteboardView {
	resolved, err := requirements.LoadResolvedWhiteboard(a.root, inventory.SagaID, "erd", "")
	if err != nil {
		return &whiteboardView{Note: "The whiteboard could not be opened: " + err.Error()}
	}
	if resolved == nil {
		return nil
	}
	description, err := resolved.Composition.Describe("")
	if err != nil {
		return &whiteboardView{Note: "The whiteboard could not be opened: " + err.Error()}
	}
	visual, err := inlineWhiteboardSVG(resolved.SVG)
	if err != nil {
		return &whiteboardView{Note: "The whiteboard could not be opened: " + err.Error()}
	}
	view := &whiteboardView{Visual: template.HTML(visual), Name: resolved.Revision.Name, Revision: resolved.Revision.ID}
	geometry := []whiteboardGeometry{}
	for _, item := range append(description.Elements, description.Connections...) {
		geometry = append(geometry, whiteboardGeometry{Path: item.Path, Bounds: item.Bounds, Transform: item.Transform, Bends: item.Element.Bends, Curve: item.Element.Curve})
	}
	metadata, _ := json.Marshal(geometry)
	view.Metadata = string(metadata)
	for _, item := range description.Elements {
		if item.Element.Kind != "frame" || item.Bounds == nil || item.Source == nil {
			continue
		}
		pin := saga.DocumentationLink{Target: item.Source.Target, Revision: item.Source.Revision}
		frame := whiteboardFrameView{Path: item.Path, Title: item.Element.Label, Summary: item.Element.Summary,
			Href: technicalPinHref(pin), Revision: pin.Revision, Status: inventory.LinkStatus(pin), Opaque: item.Opaque,
			X: item.Bounds.X, Y: item.Bounds.Y, Width: item.Bounds.Width, Height: item.Bounds.Height}
		if frame.Title == "" {
			frame.Title = entityName(inventory, pin)
		}
		view.Frames = append(view.Frames, frame)
	}
	return view
}

// Composition.Render is the trusted renderer: opaque source SVGs are isolated
// by that renderer. Namespace its IDs before inlining, and drop its standalone
// stylesheet so diagram defaults cannot override the application's theme.
func inlineWhiteboardSVG(data []byte) (string, error) {
	return inlineDiagramSVG(data, "whiteboard-")
}

var diagramPaintStyle = regexp.MustCompile(`^(?:(?:fill|stroke|flood-color|color):var\(--[a-z][a-z0-9-]*,#[0-9a-fA-F]{6}\)|display:none)(?:;(?:fill|stroke|flood-color|color):var\(--[a-z][a-z0-9-]*,#[0-9a-fA-F]{6}\))*$`)

var diagramMetadata = func() map[string]bool {
	result := map[string]bool{}
	for _, name := range strings.Fields("data-about data-annotation data-code-highlight data-code-line data-language data-connection-detail data-connection-overview data-detail-scale data-documentation-revision data-documentation-target data-frame-detail data-frame-height data-frame-overview data-frame-path data-frame-width data-from data-from-frame data-instance-path data-opaque data-source-element data-source-relationship data-to data-to-frame data-visibility data-whiteboard data-field data-key data-diagram-kind data-from-field data-renderer data-to-field data-reveal-step data-binding aria-labelledby aria-describedby filter color-interpolation-filters stdDeviation flood-color flood-opacity text-decoration overflow") {
		result[name] = true
	}
	return result
}()

func inlineDiagramSVG(data []byte, prefix string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out strings.Builder
	skip := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("invalid composed drawing: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			if skip > 0 {
				skip++
				continue
			}
			if t.Name.Local == "style" {
				skip = 1
				continue
			}
			if t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg" {
				return "", fmt.Errorf("drawing uses a foreign namespace")
			}
			if !svgElements[t.Name.Local] && t.Name.Local != "image" && t.Name.Local != "filter" && t.Name.Local != "feDropShadow" {
				return "", fmt.Errorf("unsupported drawing element %s", t.Name.Local)
			}
			out.WriteString("<" + t.Name.Local)
			for _, attr := range t.Attr {
				name, value := attr.Name.Local, attr.Value
				if attr.Name.Space == "xmlns" || name == "xmlns" {
					continue
				}
				if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && name == "space" && value == "preserve" {
					out.WriteString(` xml:space="preserve"`)
					continue
				}
				if attr.Name.Space != "" && !(attr.Name.Space == "http://www.w3.org/1999/xlink" && name == "href") {
					return "", fmt.Errorf("drawing uses a foreign attribute")
				}
				switch name {
				case "style":
					if !diagramPaintStyle.MatchString(value) {
						return "", fmt.Errorf("unsupported drawing style")
					}
				case "class":
					if value != "diagram-code" {
						return "", fmt.Errorf("unsupported drawing class")
					}
				default:
					if !svgAttributes[name] && !diagramMetadata[name] {
						return "", fmt.Errorf("unsupported drawing attribute %s", name)
					}
				}
				if name == "href" && !strings.HasPrefix(value, "#") && !(t.Name.Local == "image" && strings.HasPrefix(value, "data:image/svg+xml;base64,")) {
					return "", fmt.Errorf("drawing link must be local")
				}
				for _, match := range svgURL.FindAllStringSubmatch(value, -1) {
					if !strings.HasPrefix(match[1], "#") {
						return "", fmt.Errorf("drawing paint reference must be local")
					}
				}
				switch name {
				case "id":
					value = prefix + value
				case "href":
					if strings.HasPrefix(value, "#") {
						value = "#" + prefix + value[1:]
					}
				case "aria-labelledby", "aria-describedby":
					ids := strings.Fields(value)
					for i := range ids {
						ids[i] = prefix + ids[i]
					}
					value = strings.Join(ids, " ")
				}
				value = svgURL.ReplaceAllStringFunc(value, func(match string) string {
					target := svgURL.FindStringSubmatch(match)[1]
					if strings.HasPrefix(target, "#") {
						return "url(#" + prefix + target[1:] + ")"
					}
					return match
				})
				out.WriteString(" " + name + `="`)
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
		case xml.Directive:
			return "", fmt.Errorf("drawing directives are unsupported")
		case xml.ProcInst:
			if t.Target != "xml" {
				return "", fmt.Errorf("drawing processing instructions are unsupported")
			}
		case xml.CharData:
			if skip == 0 {
				xml.EscapeText(&escapeWriter{&out}, t)
			}
		}
	}
	return out.String(), nil
}

const whiteboardTemplates = `
{{define "erd-whiteboard"}}<section class="app-page-section whiteboard" data-whiteboard-view aria-label="Data model whiteboard"><h2>{{if .Name}}{{.Name}}{{else}}Whiteboard{{end}}</h2>{{if .Revision}}<p class="whiteboard-help">Whiteboard · saved revision <code>{{.Revision}}</code></p>{{end}}
{{if .Note}}<p class="gap" role="status" data-whiteboard-note>{{.Note}}</p>{{end}}
{{if .Visual}}<div hidden data-whiteboard-metadata="{{.Metadata}}"></div><div class="whiteboard-toolbar" role="toolbar" aria-label="Whiteboard navigation" hidden data-whiteboard-toolbar>
<button type="button" data-whiteboard-zoom="-1" aria-label="Zoom out whiteboard" title="Zoom out (-)">−</button><output data-whiteboard-level aria-live="polite"></output><button type="button" data-whiteboard-zoom="1" aria-label="Zoom in whiteboard" title="Zoom in (+)">+</button><button type="button" data-whiteboard-fit title="Fit board (0)">Fit board</button>
<label>Focus diagram <select data-whiteboard-focus><option value="">Whole board</option>{{range .Frames}}<option value="{{.Path}}">{{.Title}}</option>{{end}}</select></label><span data-whiteboard-mode aria-live="polite">Overview</span></div>
<div class="whiteboard-viewport" data-whiteboard-viewport tabindex="0" role="region" aria-label="Data model whiteboard. Drag to pan, plus and minus to zoom, zero to fit the board. Arrow keys pan.">{{.Visual}}</div>
<p class="whiteboard-help">Drag to pan · + / − to zoom · 0 to fit · Select a diagram to focus, or an entity to inspect its saved definition.</p>
<details class="whiteboard-sources"><summary>Diagram sources and saved revisions</summary><ul>{{range .Frames}}<li data-whiteboard-frame="{{.Path}}" data-x="{{.X}}" data-y="{{.Y}}" data-width="{{.Width}}" data-height="{{.Height}}"><button type="button" data-whiteboard-focus-frame="{{.Path}}">Focus {{.Title}}</button> <a href="{{.Href}}">{{.Title}} · {{.Revision}}</a>{{if ne .Status "current"}} <span class="gap">{{.Status}}</span>{{end}}<p>{{.Summary}}{{if .Opaque}} <span class="term-empty">Whole SVG; open the source ERD for entity inspection.</span>{{end}}</p></li>{{end}}</ul></details>
<p class="whiteboard-inspection" data-whiteboard-inspection role="status" hidden></p>{{end}}</section>{{end}}
`

const whiteboardStyles = `
@font-face{font-family:"Change Saga Diagram";src:url("/_diagram/fonts/go-regular.ttf") format("truetype")}
.whiteboard-viewport text,.erd-viewport svg[data-renderer] text{font-family:"Change Saga Diagram",system-ui,-apple-system,"Segoe UI",sans-serif}
.whiteboard-toolbar{display:flex;flex-wrap:wrap;align-items:center;gap:6px;margin:8px 0;font:13px var(--ui)}
.whiteboard-toolbar[hidden]{display:none}
.whiteboard-toolbar button,.whiteboard-toolbar select,.whiteboard-sources button{background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:3px;padding:6px 9px;font:inherit;cursor:pointer}
.whiteboard-toolbar label{max-width:100%;display:flex;align-items:center;gap:6px;margin-left:10px}
.whiteboard-toolbar select{min-width:0;max-width:100%}
.whiteboard-toolbar output{min-width:44px;text-align:center;font-variant-numeric:tabular-nums}
.whiteboard-toolbar [data-whiteboard-mode]{margin-left:auto;color:var(--muted)}
.whiteboard-viewport{height:min(68vh,760px);min-height:360px;overflow:hidden;border:1px solid var(--line);background:var(--bg);cursor:grab;touch-action:none}
.whiteboard-viewport>svg{display:block;width:100%;height:100%;max-width:none}
.whiteboard-viewport:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.whiteboard-viewport.whiteboard-dragging{cursor:grabbing;user-select:none}
.whiteboard-viewport [data-whiteboard-selectable]{cursor:pointer}
.whiteboard-viewport [data-whiteboard-selectable]:focus-visible{outline:3px solid var(--accent)}
.whiteboard-help{font:12px var(--ui);color:var(--muted);margin:6px 0 12px}
.whiteboard-sources{font:13px var(--ui)}
.whiteboard-sources ul{list-style:none;padding:0}
.whiteboard-sources li{overflow-wrap:anywhere;border-bottom:1px solid var(--line);padding:8px 0}
.whiteboard-sources li p{margin:5px 0;color:var(--muted)}
.whiteboard-selected-relationship{outline:2px solid var(--accent);outline-offset:3px}
.whiteboard-inspection{border-left:2px solid var(--accent);padding:8px 12px;font:13px var(--ui)}
@media(max-width:640px){.whiteboard-toolbar label{margin-left:0}.whiteboard-viewport{min-height:320px}.whiteboard-toolbar button{min-height:44px}}
`
