package server

import (
	"bytes"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/theme"
)

// themePreviewView is the /theme page: the theme's problems, if any, and the
// same samples in a light and a dark pane side by side.
type themePreviewView struct {
	Present  bool
	Problems []theme.Problem
	Panes    []themePaneView
}

// themePaneView is one scheme. Tokens declares every token's resolved value
// on the pane, so its samples read that scheme whatever the page's own mode.
type themePaneView struct {
	Scheme   string
	Tokens   template.CSS
	Groups   []themeGroupView
	Failures []theme.ContrastResult
}

type themeGroupView struct {
	Name   string
	Tokens []themeSwatchView
}

type themeSwatchView struct {
	Name, Kind, Value string
}

// themePreview renders the page `change-saga theme preview` opens. It is a
// page of its own rather than a reviewer tab: it reads no review state, and
// each pane scopes the tokens to itself.
func (a *app) themePreview(w http.ResponseWriter, _ *http.Request) {
	view := themePreviewView{}
	file, err := theme.Load(a.root)
	var invalid *theme.InvalidError
	switch {
	case errors.As(err, &invalid):
		view.Present, view.Problems = true, invalid.Problems
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	default:
		view.Present = file != nil
	}
	failures := map[string][]theme.ContrastResult{}
	for _, result := range file.Contrast() {
		if !result.Passes {
			failures[result.Scheme] = append(failures[result.Scheme], result)
		}
	}
	for _, scheme := range []string{"light", "dark"} {
		values := file.Resolved(scheme)
		var declarations strings.Builder
		pane := themePaneView{Scheme: scheme, Failures: failures[scheme]}
		for _, token := range theme.Tokens() {
			declarations.WriteString("--" + token.Name + ":" + values[token.Name] + ";")
			if len(pane.Groups) == 0 || pane.Groups[len(pane.Groups)-1].Name != token.Group {
				pane.Groups = append(pane.Groups, themeGroupView{Name: token.Group})
			}
			group := &pane.Groups[len(pane.Groups)-1]
			group.Tokens = append(group.Tokens, themeSwatchView{Name: token.Name, Kind: token.Kind, Value: values[token.Name]})
		}
		// Every value is the contract's default or a validated,
		// re-serialized override, so it is safe as CSS.
		pane.Tokens = template.CSS(declarations.String() + "color-scheme:" + scheme)
		view.Panes = append(view.Panes, pane)
	}
	var page bytes.Buffer
	if err := themePreviewTemplate.Execute(&page, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page.Bytes())
}

// themePreviewDiagram serves the preview's diagram like any slide visual,
// with the theme's tokens injected, so it shows what a generated slide
// will look like.
func (a *app) themePreviewDiagram(w http.ResponseWriter, r *http.Request) {
	svg, err := previewDiagramSVG()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body := injectFrameTheme(svg, "svg", a.sagaTheme().FrameCSS())
	w.Header().Set("Content-Security-Policy", authoredContentPolicy)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", contentETag(body))
	http.ServeContent(w, r, "theme.svg", time.Time{}, bytes.NewReader(body))
}

// previewDiagramSVG draws every default style and every palette colour: a
// section of styled nodes, a sticky, a bubble, a pin, and a highlight in
// each colour, and two entities joined by an ERD connector.
var previewDiagramSVG = sync.OnceValues(func() ([]byte, error) {
	document := diagram.New()
	add := func(element diagram.Element) { document.Elements = append(document.Elements, element) }
	add(diagram.Element{ID: "title", Kind: "text", Label: "Theme preview", X: 40, Y: 24, Width: 700, Height: 48, Style: "title"})
	add(diagram.Element{ID: "caption", Kind: "text", Label: "Every default style, then every palette colour", X: 40, Y: 72, Width: 700, Height: 24, Style: "caption"})
	add(diagram.Element{ID: "styles", Kind: "group", Shape: "section", Label: "Styles", Color: "blue", X: 24, Y: 110, Width: 1232, Height: 150, Style: "secondary"})
	styles := []string{"normal", "primary", "emphasis", "secondary", "warning", "boundary"}
	for i, style := range styles {
		shape := "service"
		if style == "boundary" {
			shape = "boundary"
		}
		add(diagram.Element{ID: "style-" + style, Kind: "node", Shape: shape, Label: style, Parent: "styles", X: 24 + float64(i)*200, Y: 44, Width: 176, Height: 80, Style: style})
	}
	add(diagram.Element{ID: "flow", Kind: "edge", From: "style-normal", To: "style-primary", Points: []diagram.Point{{X: 224, Y: 194}, {X: 248, Y: 194}}, Style: "secondary"})
	for i, color := range diagram.PaletteNames() {
		x := 40 + float64(i)*205
		add(diagram.Element{ID: "sticky-" + color, Kind: "sticky", Label: color + " sticky", Color: color, X: x, Y: 280, Width: 180, Height: 90, Style: "normal"})
		add(diagram.Element{ID: "bubble-" + color, Kind: "annotation", Shape: "bubble", Label: color, Color: color, About: "sticky-" + color, X: x, Y: 394, Width: 140, Height: 56, Style: "normal"})
		add(diagram.Element{ID: "pin-" + color, Kind: "annotation", Shape: "pin", Label: string(rune('1' + i)), Color: color, About: "bubble-" + color, X: x + 150, Y: 406, Width: 30, Style: "normal"})
		add(diagram.Element{ID: "mark-" + color, Kind: "annotation", Shape: "highlight", Color: color, About: "sticky-" + color, X: x, Y: 466, Width: 180, Height: 24, Style: "normal"})
	}
	fields := diagram.Fields{{Name: "id", Type: "uuid", Key: "pk"}, {Name: "name", Type: "text"}}
	add(diagram.Element{ID: "customer", Kind: "node", Shape: "entity", Label: "customer", Fields: fields, X: 40, Y: 520, Width: 240, Height: 110, Style: "normal"})
	add(diagram.Element{ID: "order", Kind: "node", Shape: "entity", Label: "order", Fields: diagram.Fields{{Name: "id", Type: "uuid", Key: "pk"}, {Name: "customer_id", Type: "uuid", Key: "fk"}}, X: 420, Y: 520, Width: 240, Height: 110, Style: "primary"})
	add(diagram.Element{ID: "places", Kind: "edge", From: "customer", To: "order", Tail: "only-one", Head: "zero-or-many", Points: []diagram.Point{{X: 280, Y: 575}, {X: 420, Y: 575}}, Style: "normal"})
	add(diagram.Element{ID: "entity-note", Kind: "sticky", Label: "An ERD edge: one customer places many orders", Color: "green", About: "places", X: 720, Y: 520, Width: 300, Height: 130, Style: "normal"})
	return diagram.Render(document, diagram.Options{Title: "Theme preview", Description: "Every default diagram style and palette colour."})
})

var themePreviewTemplate = template.Must(template.New("theme").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Theme preview · change-saga</title>
<link rel="stylesheet" href="` + assetPath("app.css") + `"><link rel="stylesheet" href="/theme.css"><script src="` + assetPath("theme.js") + `"></script>
<style>
.theme-preview{padding:20px 24px;max-width:1600px;margin:0 auto}
.theme-preview>header h1{margin:0 0 4px;font-size:20px}.theme-preview>header p{margin:0 0 16px;color:var(--muted)}
.theme-panes{display:grid;grid-template-columns:repeat(auto-fit,minmax(560px,1fr));gap:16px}
.theme-pane{background:var(--bg);color:var(--ink);font:13px/1.55 var(--ui);border:1px solid var(--line);border-radius:var(--radius);padding:16px;min-width:0}
.theme-pane h2{margin:0 0 12px;font-size:15px}.theme-pane h3{margin:16px 0 6px;font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}
.theme-swatches{display:grid;grid-template-columns:repeat(auto-fill,minmax(150px,1fr));gap:6px}
.theme-swatch{display:flex;gap:8px;align-items:center;min-width:0}.theme-swatch>span:first-child{flex:none;width:28px;height:28px;border:1px solid var(--line);border-radius:var(--radius)}
.theme-swatch code{display:block;font:11px/1.3 var(--mono);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.theme-swatch small{color:var(--muted);font:11px var(--mono)}
.theme-chrome{display:grid;gap:10px}.theme-chrome .sample-bar{display:flex;align-items:center;gap:10px;height:var(--top);padding:0 12px;background:var(--toolbar-bg);border-bottom:1px solid var(--line)}
.sample-row{display:flex;flex-wrap:wrap;gap:8px;align-items:center}
.sample-note{padding:8px 10px;border:1px solid var(--warning-line);border-left:3px solid var(--amber);border-radius:var(--radius);background:var(--warning-bg)}
.sample-danger{padding:8px 10px;border:1px solid var(--danger-line);border-left:3px solid var(--red);border-radius:var(--radius);background:var(--danger-bg)}
.sample-code{margin:0;border:1px solid var(--line);border-radius:var(--radius);background:var(--code-bg);font:12px/1.5 var(--mono);overflow:hidden}
.sample-code>span{display:block;padding:0 8px}.sample-code .add{background:var(--add-bg);box-shadow:inset 3px 0 var(--add-line)}.sample-code .del{background:var(--del-bg);box-shadow:inset 3px 0 var(--del-line)}
.sample-card{padding:10px 12px;background:var(--bg-subtle);border-radius:var(--radius);box-shadow:var(--shadow)}
.sample-muted{color:var(--muted)}.sample-faint{color:var(--faint)}.sample-accent{color:var(--accent)}.sample-status{font-weight:600}
.theme-diagram{display:block;width:100%;height:auto;margin-top:8px;border:1px solid var(--line);border-radius:var(--radius)}
.theme-problems{margin:0 0 16px}.theme-problems li,.theme-failures li{font:12px/1.5 var(--mono)}
</style></head>
<body><main class="theme-preview">
<header><h1>Theme preview</h1>
<p>{{if .Present}}This Saga's <code>theme.css</code> applied to the default tokens{{else}}The default tokens: this Saga has no <code>theme.css</code>; <code>change-saga theme init</code> writes one{{end}}. <code>change-saga theme check</code> validates it and measures contrast.</p>
{{if .Problems}}<div class="sample-danger theme-problems" role="alert"><strong>The theme is invalid and is not applied:</strong><ul>{{range .Problems}}<li>theme.css:{{.Line}}: {{.Message}}</li>{{end}}</ul></div>{{end}}
</header>
<div class="theme-panes">{{range .Panes}}
<section class="theme-pane" data-scheme="{{.Scheme}}" style="{{.Tokens}}" aria-label="{{.Scheme}} mode">
<h2>{{if eq .Scheme "light"}}Light{{else}}Dark{{end}}</h2>
{{if .Failures}}<div class="sample-note theme-failures"><strong>Below WCAG AA contrast:</strong><ul>{{range .Failures}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
<h3>Interface</h3>
<div class="theme-chrome">
<div class="sample-bar"><strong>change-saga</strong><span class="sample-muted">Review pr-23</span><span class="sample-accent">Open diff</span></div>
<div class="sample-row"><button type="button" class="btn">Comment</button><button type="button" class="btn btn-primary">Approve</button><span class="sample-status" style="color:var(--green)">approved</span><span class="sample-status" style="color:var(--red)">changes requested</span><span class="sample-status" style="color:var(--amber)">stale</span></div>
<p style="margin:0">Body text in ink, <span class="sample-muted">muted notes</span>, <span class="sample-faint">faint hints</span>, and <a href="#">a link</a>.</p>
<div class="sample-note">A warning callout explains a surprise.</div>
<div class="sample-danger">A danger callout names a gap.</div>
<pre class="sample-code"><span class="del">- <span class="tok-keyword">return</span> <span class="tok-string">"sqs"</span></span><span class="add">+ <span class="tok-keyword">return</span> <span class="tok-string">"postgres"</span> <span class="tok-comment">// one transaction</span></span><span><span class="tok-type">int</span> <span class="tok-property">count</span> <span class="tok-punctuation">=</span> <span class="tok-number">42</span></span></pre>
<div class="sample-card">A card on the subtle background, with the shadow and radius tokens.</div>
</div>
<h3>Diagram</h3>
<img class="theme-diagram" src="/theme/diagram.svg" alt="A diagram in every default style and palette colour" style="color-scheme:{{.Scheme}}" width="1280" height="720">
{{range .Groups}}<h3>{{.Name}}</h3><div class="theme-swatches">{{range .Tokens}}<div class="theme-swatch">{{if eq .Kind "color"}}<span style="background:var(--{{.Name}})"></span>{{else}}<span style="{{if eq .Kind "shadow"}}box-shadow:var(--{{.Name}}){{else if eq .Kind "length"}}border-radius:var(--{{.Name}}){{end}}"></span>{{end}}<span><code>--{{.Name}}</code><small>{{.Value}}</small></span></div>{{end}}</div>{{end}}
</section>{{end}}
</div>
</main></body></html>`))
