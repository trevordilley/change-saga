package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type documentationMember struct {
	Pin    saga.DocumentationLink
	Name   string
	Status string
	Y      int
	// PageHref is the member's canonical page at its pinned revision.
	PageHref string
}
type documentationEdge struct {
	ID, From, To, Description string
	Path, CodeHref            string
	// Intent is the interaction's own intent, independent of its System's.
	Intent string
}
type documentationView struct {
	Target, Revision, Name, Kind, Explanation, Status string
	CurrentPin                                        *saga.DocumentationLink
	Members                                           []documentationMember
	Edges                                             []documentationEdge
	Height                                            int
	Code                                              []*termCodeView
	History                                           []saga.DocumentationLink
	// PageHref and UsagesHref are set in the drawer, which links the
	// definition's canonical page and loads its usages on request. The page
	// itself lists usages directly.
	PageHref, UsagesHref string
	// Selections is the opening Item's exact selections, when the drawer was
	// opened from an Item.
	Selections *itemSelectionsView
	// OnPage renders the definition on its own page: members open their own
	// pages rather than the drawer, and the page's heading names it.
	OnPage bool
	// Intent is the revision's explicit intent, or unspecified. A proposal
	// names its implemented baseline revision or says it has none; an
	// implemented revision names the delivery commit it was asserted at.
	Intent       string
	BaselineHref string
	BaselineID   string
	BaselineNone bool
	Delivery     *requirements.Delivery
	// Data-entity content: curated fields, holding resources, owned and
	// derived incoming relationships, and the ERDs listing it.
	Fields        []entityFieldView
	Holders       []entityHolderView
	Relationships []relationshipView
	Incoming      []relationshipView
	ERDs          []erdMembershipView
}

// documentationView is one exact revision of a definition, as both the drawer
// and the definition's page render it.
func (a *app) documentationView(ctx context.Context, inventory requirements.Inventory, record *requirements.TechnicalRecord, revision *requirements.TechnicalRevision, pin saga.DocumentationLink) documentationView {
	view := documentationView{Target: pin.Target, Revision: pin.Revision, Name: revision.Name, Kind: record.Kind, Explanation: revision.Explanation, Status: inventory.LinkStatus(pin), Height: len(revision.Components)*100 + 20, Intent: revision.EffectiveIntent(), Delivery: revision.Delivery}
	switch {
	case revision.Baseline == requirements.BaselineNone:
		view.BaselineNone = true
	case revision.Baseline != "":
		view.BaselineID = strings.TrimPrefix(revision.Baseline, record.Target+":revision:")
		view.BaselineHref = technicalHref(record.Kind, record.Identity.ID, view.BaselineID)
	}
	if record.Kind == requirements.KindDataEntity {
		dataEntityDetail(inventory, &view, revision, pin)
	}
	if record.CurrentRevision != nil {
		view.CurrentPin = &saga.DocumentationLink{Target: record.Target, Revision: record.Target + ":revision:" + record.CurrentRevision.ID}
	}
	for _, v := range record.Revisions {
		view.History = append(view.History, saga.DocumentationLink{Target: record.Target, Revision: record.Target + ":revision:" + v.ID})
	}
	positions := map[string]int{}
	names := map[string]string{}
	for index, member := range revision.Components {
		name := member.Target
		if component := inventory.Find(member.Target); component != nil {
			if rev := component.Revision(member.Revision); rev != nil {
				name = rev.Name
			}
		}
		positions[member.Target] = index*100 + 45
		names[member.Target] = name
		view.Members = append(view.Members, documentationMember{Pin: member, Name: name, Status: inventory.LinkStatus(member), Y: index*100 + 20, PageHref: technicalPinHref(member)})
	}
	for index, edge := range revision.Interactions {
		from, to := positions[edge.From], positions[edge.To]
		lane := 290 + (index%6)*22
		path := fmt.Sprintf("M 250 %d H %d V %d H 255", from, lane, to)
		intent := edge.Intent
		if intent == "" {
			intent = requirements.IntentUnspecified
		}
		view.Edges = append(view.Edges, documentationEdge{ID: edge.ID, Intent: intent, From: names[edge.From], To: names[edge.To], Description: edge.Description, Path: path, CodeHref: "/api/documentation?" + url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "interaction": {edge.ID}}.Encode()})
	}
	view.Code = a.documentationCode(ctx, requirements.References(revision.Code), record.Kind)
	return view
}

// No inventory is loaded by the initial shell. This endpoint resolves the
// explicitly requested pin, preserving it even when the definition has moved.
func (a *app) documentationPage(w http.ResponseWriter, r *http.Request) {
	manifest, err := saga.ReadManifest(a.root)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	pin := saga.DocumentationLink{Target: r.URL.Query().Get("target"), Revision: r.URL.Query().Get("revision")}
	if !saga.ValidDocumentationLink(manifest.ID, pin) {
		http.Error(w, "Invalid documentation reference", 400)
		return
	}
	inventory, err := requirements.LoadInventory(a.root, manifest.ID)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	record := inventory.Find(pin.Target)
	if record == nil || record.Revision(pin.Revision) == nil {
		http.Error(w, "The referenced definition or revision is missing. The saved link has not been replaced.", 404)
		return
	}
	revision := record.Revision(pin.Revision)
	if interaction := r.URL.Query().Get("interaction"); interaction != "" {
		for _, edge := range revision.Interactions {
			if edge.ID == interaction {
				var body bytes.Buffer
				if err := a.template.ExecuteTemplate(&body, "documentation-code", a.documentationCode(r.Context(), requirements.References(edge.Code), "interaction")); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write(body.Bytes())
				return
			}
		}
		http.Error(w, "Interaction not found in the saved revision", 404)
		return
	}

	view := a.documentationView(r.Context(), inventory, record, revision, pin)
	// The drawer is read from a slide: it links the definition's own page and
	// loads its other usages only when asked.
	view.PageHref = technicalPinHref(pin)
	view.UsagesHref = "/api/technical-usages?" + url.Values{"target": {pin.Target}}.Encode()
	if item := r.URL.Query().Get("item"); item != "" {
		view.Selections = a.itemSelections(r.Context(), inventory, pin, item)
	}
	var body bytes.Buffer
	if err := a.template.ExecuteTemplate(&body, "documentation", view); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}

func (a *app) documentationCode(ctx context.Context, refs []coderef.Reference, subject string) []*termCodeView {
	views := a.referenceCode(ctx, refs, subject)
	for index, view := range views {
		if index < len(refs) {
			view.Note = refs[index].Note + " " + view.Note
		}
	}
	return views
}

const documentationTemplates = `
{{define "documentation-control"}}{{with .}}<button type="button" class="icon-button" data-documentation-target="{{.Target}}" data-documentation-revision="{{.Revision}}"{{if .Item}} data-documentation-item="{{.Item}}"{{end}}{{if .Selections}} data-documentation-selections="{{.Selections}}"{{end}} aria-label="Open technical explanation" title="Open technical explanation"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-book"></use></svg></button>{{end}}{{end}}
{{define "documentation-code"}}{{range .}}<figure class="term-code{{if .Stale}} stale{{end}}" data-file-path="{{.Path}}"><figcaption><code>{{.Location}}</code>{{if .Stale}} · stale{{end}}</figcaption>{{if .Note}}<p class="term-code-note">{{.Note}}</p>{{end}}<table class="term-code-lines"><tbody>{{range .Lines}}<tr{{if .Referenced}} class="referenced"{{end}}><th scope="row">{{.Number}}</th><td><code data-code>{{.Text}}</code></td></tr>{{end}}</tbody></table></figure>{{end}}{{end}}
{{define "documentation"}}<article class="documentation-explanation" data-documentation-view="{{.Target}}" data-documentation-pin="{{.Revision}}">
{{if not .OnPage}}<p class="trace-kind">{{kindTitle .Kind}}</p><h2>{{.Name}}</h2>
{{end}}{{if and (ne .Status "current") (not .OnPage)}}<p role="status" class="gap" data-documentation-status="{{.Status}}">This reference is {{.Status}}. You are reading the saved revision; it has not been repinned.</p>{{with .CurrentPin}}<button type="button" data-documentation-target="{{.Target}}" data-documentation-revision="{{.Revision}}">Read current definition</button>{{end}}{{end}}
<p class="documentation-intent" data-documentation-intent="{{.Intent}}"><span class="technical-intent intent-{{.Intent}}">{{.Intent}}</span>{{if eq .Intent "unspecified"}} <small>This revision predates recorded intent; it is neither assumed proposed nor implemented.</small>{{else if eq .Intent "proposed"}} <small>{{if .BaselineHref}}Proposed successor of the implemented baseline <a href="{{.BaselineHref}}"><code>{{.BaselineID}}</code></a>, which is retained.{{else if .BaselineNone}}A wholly new proposal: no implemented baseline.{{end}} A proposal is not implemented code.</small>{{else if eq .Intent "implemented"}}{{with .Delivery}} <small>Asserted at delivery commit <code>{{short .Commit}}</code>. Implementation is not verification or review approval.</small>{{end}}{{end}}</p>
<p class="documentation-prose">{{.Explanation}}</p>{{if .PageHref}}<p class="documentation-page-link"><a href="{{.PageHref}}" data-documentation-page>Open this revision's full page</a></p>{{end}}
{{if .Members}}<h3>Component interactions</h3><svg class="documentation-diagram" viewBox="0 0 430 {{.Height}}" role="group" aria-label="Directed component interactions; explanations and exact code follow."><defs><marker id="documentation-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor"/></marker></defs>{{range .Edges}}<path d="{{.Path}}" fill="none" stroke="currentColor" stroke-width="2"{{if eq .Intent "proposed"}} stroke-dasharray="6 4"{{end}} marker-end="url(#documentation-arrow)"><title>{{.From}} → {{.To}}: {{.Description}}{{if eq .Intent "proposed"}} (proposed){{end}}</title></path>{{end}}{{range .Members}}<a {{if $.OnPage}}href="{{.PageHref}}"{{else}}href="/api/documentation?target={{.Pin.Target}}&amp;revision={{.Pin.Revision}}" data-documentation-target="{{.Pin.Target}}" data-documentation-revision="{{.Pin.Revision}}"{{end}} aria-label="Open {{.Name}}"><g transform="translate(20 {{.Y}})"><rect width="230" height="50" fill="var(--bg,white)" stroke="currentColor"/><text x="12" y="30" font-size="14" fill="currentColor">{{.Name}}</text></g></a>{{end}}</svg>
<ul class="documentation-members">{{range .Members}}<li>{{if $.OnPage}}<a href="{{.PageHref}}">{{.Name}}</a>{{else}}<button type="button" data-documentation-target="{{.Pin.Target}}" data-documentation-revision="{{.Pin.Revision}}">{{.Name}}</button>{{end}}{{if ne .Status "current"}} <span class="gap">{{.Status}}</span>{{end}}</li>{{end}}</ul>
<ol>{{range .Edges}}<li data-interaction="{{.ID}}" data-interaction-intent="{{.Intent}}"><h4>{{.From}} → {{.To}}{{if ne .Intent "unspecified"}} <small class="technical-intent intent-{{.Intent}}">{{.Intent}}</small>{{end}}</h4><p>{{.Description}}</p><details data-lazy-href="{{.CodeHref}}"><summary>Interaction code</summary><div data-lazy-body>Code loads when opened.</div></details></li>{{end}}</ol>{{end}}
{{with .Selections}}{{template "item-selections" .}}{{end}}{{if eq .Kind "data-entity"}}{{template "data-entity-detail" .}}{{end}}
<h3>{{if .Selections}}All of this definition's code{{else}}Exact code{{end}}</h3>{{if .Code}}{{template "documentation-code" .Code}}{{else}}<p class="term-empty" data-documentation-no-code>{{if eq .Intent "proposed"}}No code yet. A proposal remains visible and usable before implementation.{{else}}No code references.{{end}}</p>{{end}}
{{if .UsagesHref}}<details class="documentation-usages" data-lazy-href="{{.UsagesHref}}"><summary>Used by</summary><div data-lazy-body>Usages load when opened.</div></details>{{end}}{{if not .OnPage}}<details><summary>Definition history</summary><p>Viewing <code>{{.Revision}}</code>. Opening another revision does not update this slide.</p><ul>{{range .History}}<li><button type="button" data-documentation-target="{{.Target}}" data-documentation-revision="{{.Revision}}">{{.Revision}}</button></li>{{end}}</ul></details>{{end}}
</article>{{end}}
`
