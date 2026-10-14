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

// whiteboardView is a read-only named document on the shared canvas. Its
// references lead to the existing, independently readable pinned definitions.
type whiteboardView struct {
	ID, Href, CurrentHref string
	History               []whiteboardRevisionView
	Metadata              string
	Viewport              string
	Visual                template.HTML
	Name, Revision        string
	Note                  string
	Frames                []whiteboardFrameView
	Sections              []whiteboardSectionView
	Notes                 []whiteboardNoteView
}

type whiteboardNoteView struct {
	Path    string
	Popover notePopoverView
}

// Sections are authored regions of this document, not inventory definitions.
type whiteboardSectionView struct {
	Path, Title string
}

type whiteboardFrameView struct {
	Path, Title, Summary, Href, Revision, Status string
	Target                                       string
	X, Y, Width, Height                          float64
	Opaque                                       bool
	SourceNote                                   string
}

type whiteboardGeometry struct {
	Path      string            `json:"path"`
	Bounds    *diagram.Box      `json:"bounds,omitempty"`
	LabelBox  *diagram.Box      `json:"label_box,omitempty"`
	Transform diagram.Transform `json:"transform"`
	Bends     []diagram.Point   `json:"bends,omitempty"`
	Curve     string            `json:"curve,omitempty"`
}

// A missing board is optional; a broken board is explicit without removing
// the ERD documents and entity directory that remain usable below it.
func (a *app) makeWhiteboardView(inventory requirements.Inventory) *whiteboardView {
	return a.makeNamedWhiteboardView(inventory, "data-model", "")
}

func (a *app) makeNamedWhiteboardView(inventory requirements.Inventory, id, revision string) *whiteboardView {
	resolved, err := requirements.LoadResolvedWhiteboard(a.root, inventory.SagaID, id, revision)
	if err != nil {
		return &whiteboardView{ID: id, Href: whiteboardHref(id, revision), CurrentHref: whiteboardHref(id, ""), Note: "The whiteboard could not be opened: " + err.Error()}
	}
	if resolved == nil {
		return nil
	}
	description, err := resolved.Composition.Describe("")
	if err != nil {
		return &whiteboardView{ID: id, Href: whiteboardHref(id, revision), CurrentHref: whiteboardHref(id, ""), Note: "The whiteboard could not be opened: " + err.Error()}
	}
	visual, err := inlineWhiteboardSVG(resolved.SVG)
	if err != nil {
		return &whiteboardView{ID: id, Href: whiteboardHref(id, revision), CurrentHref: whiteboardHref(id, ""), Note: "The whiteboard could not be opened: " + err.Error()}
	}
	enhanced, capabilities := a.enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
	view := &whiteboardView{ID: id, Href: whiteboardHref(id, revision), CurrentHref: whiteboardHref(id, ""), Visual: enhanced, Name: resolved.Revision.Name, Revision: resolved.Revision.ID}
	if resolved.Revision.Document.Viewport != nil {
		camera, _ := json.Marshal(resolved.Revision.Document.Viewport)
		view.Viewport = string(camera)
	}
	view.History = a.whiteboardHistoryView(inventory.SagaID, id, resolved.Revision.ID)
	geometry := []whiteboardGeometry{}
	for _, item := range append(description.Elements, description.Connections...) {
		if item.Element.Note != "" && !item.Element.Decorative {
			view.Notes = append(view.Notes, whiteboardNoteView{Path: item.Path, Popover: notePopoverView{Label: diagram.AccessibleName(item.Element), Note: markdownWithAnchors(item.Element.Note, "whiteboard-note-"+domID(item.Path))}})
		}
		geometry = append(geometry, whiteboardGeometry{Path: item.Path, Bounds: item.Bounds, LabelBox: item.Element.LabelBox, Transform: item.Transform, Bends: item.Element.Bends, Curve: item.Element.Curve})
	}
	metadata, _ := json.Marshal(geometry)
	view.Metadata = string(metadata)
	for _, item := range description.Elements {
		if item.Element.Kind == "group" && item.Element.Shape == "section" && item.Bounds != nil {
			title := item.Element.Label
			if title == "" {
				title = item.Element.ID
			}
			view.Sections = append(view.Sections, whiteboardSectionView{Path: item.Path, Title: title})
		}
		if item.Element.Kind != "frame" || item.Bounds == nil || item.Source == nil {
			continue
		}
		pin := saga.DocumentationLink{Target: item.Source.Target, Revision: item.Source.Revision}
		frame := whiteboardFrameView{Path: item.Path, Title: item.Element.Label, Summary: item.Element.Summary,
			Target: pin.Target, Href: technicalPinHref(pin), Revision: pin.Revision, Status: inventory.LinkStatus(pin), Opaque: item.Opaque,
			X: item.Bounds.X, Y: item.Bounds.Y, Width: item.Bounds.Width, Height: item.Bounds.Height}
		if capability, ok := capabilities[item.Path]; ok {
			switch capability.Mode {
			case "authored":
				frame.SourceNote = "Select bound entities and relationships to inspect their evidence. Details restores the original drawing."
			case "image":
				frame.SourceNote = "Embedded SVG; internal entities are not individually selectable. Use Evidence to inspect its saved directory."
			}
		}
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
	for _, name := range strings.Fields("data-about data-annotation data-code-highlight data-code-line data-language data-connection-detail data-connection-overview data-diagram-summary data-diagram-detail data-detail-scale data-documentation-revision data-documentation-target data-frame-detail data-frame-height data-frame-overview data-frame-path data-frame-width data-from data-from-frame data-instance-path data-opaque data-source-element data-source-relationship data-to data-to-frame data-visibility data-whiteboard data-field data-key data-diagram-kind data-from-field data-renderer data-to-field data-reveal-step data-binding aria-labelledby aria-describedby filter color color-interpolation-filters stdDeviation flood-color flood-opacity text-decoration overflow") {
		result[name] = true
	}
	return result
}()

func inlineDiagramSVG(data []byte, prefix string) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out strings.Builder
	skip := 0
	inMetadata := false
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
			// Shared renderer metadata carries license text, never markup. Check
			// before stylesheet skipping so no child element escapes this rule.
			if inMetadata {
				return "", fmt.Errorf("drawing metadata must contain only text, not child elements")
			}
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
			if !svgElements[t.Name.Local] && t.Name.Local != "image" && t.Name.Local != "filter" && t.Name.Local != "feDropShadow" && t.Name.Local != "metadata" {
				return "", fmt.Errorf("unsupported drawing element %s", t.Name.Local)
			}
			inMetadata = t.Name.Local == "metadata"
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
			inMetadata = false
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
{{define "erd-whiteboard"}}{{$model := .}}{{with .Whiteboard}}
<section class="whiteboard whiteboard-canvas" data-whiteboard-view data-whiteboard-id="{{.ID}}" data-whiteboard-url="{{.Href}}" data-whiteboard-revision="{{.Revision}}" data-technical-page="{{if eq .ID "data-model"}}erd{{else}}whiteboard{{end}}"{{if eq .ID "data-model"}} data-technical-data-model{{end}} aria-label="{{.Name}} whiteboard">
<div hidden data-whiteboard-metadata="{{.Metadata}}" data-whiteboard-opening="{{.Viewport}}">{{range .Sections}}<span data-whiteboard-section="{{.Path}}"></span>{{end}}{{range .Notes}}<template data-diagram-note-template="{{.Path}}">{{template "element-note-popover" .Popover}}</template>{{end}}</div>
<div class="whiteboard-viewport" data-whiteboard-viewport tabindex="0" role="region" aria-label="Whiteboard. Scroll or drag to pan, Control or Command plus scroll to zoom. Plus and minus zoom, zero fits the board, arrow keys pan.">{{.Visual}}</div>
<header class="whiteboard-heading whiteboard-overlay">
<h1>{{if .Name}}{{.Name}}{{else}}Whiteboard{{end}}</h1>
<details class="whiteboard-description"><summary>Description</summary><div class="whiteboard-description-body">
{{with $model.Overview}}{{template "fragment" .}}{{else}}<p>Focus a section to explore this document. Linked entities open their saved definitions.</p>{{end}}
<p class="whiteboard-help">Scroll or drag to pan · Shift + scroll pans sideways · Ctrl/Cmd + scroll or + / − to zoom · 0 to fit</p>
<p class="whiteboard-help">Whiteboard · saved revision <code>{{.Revision}}</code></p>
{{if .History}}<nav aria-label="Whiteboard revision history"><a href="{{.CurrentHref}}">Current document</a>{{range .History}} · <a href="{{.Href}}"{{if .Shown}} aria-current="page"{{end}}>Revision {{.ID}}</a>{{end}}</nav>{{end}}
</div></details></header>
<div class="whiteboard-toolbar whiteboard-overlay" role="toolbar" aria-label="Whiteboard navigation" hidden data-whiteboard-toolbar>
<button type="button" data-whiteboard-zoom="-1" aria-label="Zoom out whiteboard" title="Zoom out (-)">−</button>
<output data-whiteboard-level aria-live="polite"></output>
<button type="button" data-whiteboard-zoom="1" aria-label="Zoom in whiteboard" title="Zoom in (+)">+</button>
{{if .Viewport}}<button type="button" data-whiteboard-home>Start here</button>{{end}}
<button type="button" data-whiteboard-replay hidden>Replay</button>
<button type="button" data-whiteboard-fields aria-pressed="false" title="Show fields and original authored detail">Details</button>
<button type="button" data-whiteboard-fit title="Fit board (0)">Fit board</button>
<label>Focus section <select data-whiteboard-focus><option value="">{{if .Viewport}}Start here{{else}}Whole board{{end}}</option>{{range .Sections}}<option value="{{.Path}}">{{.Title}}</option>{{end}}{{if .Frames}}<optgroup label="Imported sources"{{if .Sections}} hidden{{end}}>{{range .Frames}}<option value="{{.Path}}">{{.Title}}</option>{{end}}</optgroup>{{end}}</select></label>
<button type="button" data-whiteboard-evidence hidden>Evidence</button>
<span data-whiteboard-mode aria-live="polite">Overview</span>
</div>
{{if .Frames}}<details class="whiteboard-sources whiteboard-overlay"><summary>Source provenance</summary><div class="whiteboard-sources-body"><ul>
{{range .Frames}}<li data-whiteboard-frame="{{.Path}}" data-whiteboard-source-href="{{.Href}}" data-source-target="{{.Target}}" data-source-revision="{{.Revision}}" data-x="{{.X}}" data-y="{{.Y}}" data-width="{{.Width}}" data-height="{{.Height}}">
<button type="button" data-whiteboard-focus-frame="{{.Path}}">Focus {{.Title}}</button>
<button type="button" data-whiteboard-evidence data-documentation-target="{{.Target}}" data-documentation-revision="{{.Revision}}" aria-label="Evidence for {{.Title}}">Evidence</button>
<span class="whiteboard-source-revision">{{.Revision}}</span>{{if ne .Status "current"}} <span class="gap">{{.Status}}</span>{{end}}
<p>{{.Summary}}{{with .SourceNote}} <span class="term-empty">{{.}}</span>{{end}}</p>
</li>{{end}}</ul>

</div></details>{{end}}
</section>{{end}}{{end}}
`

const whiteboardStyles = `
@font-face{font-family:"Change Saga Diagram";src:url("/_diagram/fonts/go-regular.ttf") format("truetype")}
.whiteboard-viewport text,.erd-viewport svg[data-renderer] text{font-family:"Change Saga Diagram",system-ui,-apple-system,"Segoe UI",sans-serif}
/* Scope the canvas shell to the active page, including boosted navigation. */
body:has(#view-saga.active .whiteboard-canvas){--top:0px}
body:has(#view-saga.active .whiteboard-canvas)>.topbar{display:none}
.shell:has(#view-saga.active .whiteboard-canvas){height:100dvh;min-height:0;overflow:hidden;display:grid}
.shell:has(#view-saga.active .whiteboard-canvas)>.sidebar{position:relative;top:auto;height:100%;max-height:none}
.shell:has(#view-saga.active .whiteboard-canvas)>.content{width:100%;height:100%;min-height:0;padding:0;overflow:hidden}
.shell:has(.whiteboard-canvas) #view-saga{height:100%}
.whiteboard-canvas{position:relative;height:100%;isolation:isolate;overflow:hidden}
.whiteboard-viewport{position:absolute;inset:0;overflow:hidden;background:var(--bg);cursor:grab;touch-action:none}
.whiteboard-canvas:not([data-whiteboard-fields="true"]) .whiteboard-viewport [data-field],.whiteboard-canvas:not([data-whiteboard-fields="true"]) .whiteboard-viewport [data-diagram-detail="field"]{display:none}
.whiteboard-toolbar button[aria-pressed="true"]{background:var(--accent-soft);border-color:var(--accent)}
.whiteboard-viewport>svg{display:block;width:100%;height:100%;max-width:none}
.whiteboard-viewport [data-diagram-summary] rect{fill:var(--bg);stroke:var(--accent);stroke-width:1.5}
.whiteboard-viewport [data-diagram-summary] text:not([font-weight]){fill:var(--ink);font-weight:600}
.whiteboard-viewport [data-frame-detail]>svg{overflow:visible}
.whiteboard-viewport:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
.whiteboard-viewport.whiteboard-dragging{cursor:grabbing;user-select:none}
.whiteboard-viewport [data-whiteboard-selectable]{cursor:pointer}
.whiteboard-viewport [data-frame-path]>rect:first-of-type{fill:transparent;stroke:transparent}
.whiteboard-viewport [data-frame-path]:hover>rect:first-of-type{stroke:var(--accent)}
.whiteboard-source-revision{display:block;margin-top:4px;color:var(--muted);font:11px/1.4 var(--mono);overflow-wrap:anywhere}
.whiteboard-viewport [data-whiteboard-selectable]:focus-visible{outline:3px solid var(--accent)}
.whiteboard-overlay{position:absolute;z-index:1;background:var(--bg);border:1px solid var(--line-soft);border-radius:10px;box-shadow:0 3px 14px #1f232812;font:13px/1.5 var(--ui)}
.whiteboard-heading{top:20px;left:20px;width:max-content;max-width:min(460px,calc(100% - 40px));padding:12px 16px}
.whiteboard-heading h1{margin:0;font:600 18px/1.35 var(--ui);overflow-wrap:anywhere}
.whiteboard-description{margin-top:4px;color:var(--muted)}
.whiteboard-description summary,.whiteboard-sources summary{cursor:pointer}
.whiteboard-description-body{max-height:calc(100dvh - 220px);overflow:auto;color:var(--ink)}
.whiteboard-description-body .fragment{margin:12px 0;padding:0}
.whiteboard-toolbar{bottom:20px;left:20px;right:20px;width:fit-content;max-width:calc(100% - 40px);margin:0 auto;display:flex;flex-wrap:wrap;align-items:center;gap:6px;padding:8px}
.whiteboard-toolbar[hidden]{display:none}
.whiteboard-toolbar button,.whiteboard-toolbar select,.whiteboard-sources button{background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:5px;padding:6px 9px;font:inherit;cursor:pointer}
.whiteboard-toolbar label{max-width:100%;display:flex;align-items:center;gap:6px;margin-left:10px}
.whiteboard-toolbar select{min-width:0;max-width:220px;flex:1}
.whiteboard-toolbar output{min-width:44px;text-align:center;font-variant-numeric:tabular-nums}
.whiteboard-toolbar [data-whiteboard-mode]{color:var(--muted);font-size:11px}
.whiteboard-help{font:12px/1.5 var(--ui);color:var(--muted);margin:8px 0}
.whiteboard-sources{top:20px;right:20px;max-width:min(420px,calc(100% - 40px));padding:10px 14px}
.whiteboard-sources[open]{width:420px;z-index:2}
.whiteboard-sources-body{max-height:calc(100dvh - 180px);overflow:auto;margin-top:8px}
.whiteboard-sources ul{list-style:none;padding:0}
.whiteboard-sources li{overflow-wrap:anywhere;border-bottom:1px solid var(--line);padding:8px 0}
.whiteboard-sources li p{margin:5px 0;color:var(--muted)}
.whiteboard-sources .app-page-section{margin-top:18px}
.whiteboard-selected-relationship{outline:2px solid var(--accent);outline-offset:3px}
@media(max-width:1200px){.whiteboard-sources{top:auto;bottom:88px}.whiteboard-sources-body{max-height:calc(100dvh - 240px)}.whiteboard-toolbar [data-whiteboard-mode]{display:none}}
@media(max-width:760px){
.shell:has(#view-saga.active .whiteboard-canvas){grid-template-columns:min(190px,30vw) minmax(0,1fr)}
.whiteboard-heading{top:12px;left:12px;max-width:calc(100% - 24px);padding:10px 12px}
.whiteboard-heading h1{font-size:16px}
.whiteboard-toolbar{bottom:12px;left:12px;right:12px;max-width:calc(100% - 24px)}
.whiteboard-toolbar label{margin-left:0;flex:1 1 100%}
.whiteboard-toolbar select{max-width:100%;width:0}
.whiteboard-toolbar button{min-height:44px}
.whiteboard-sources{bottom:136px;right:12px;max-width:calc(100% - 24px)}
}
`
