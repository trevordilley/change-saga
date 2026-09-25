package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Technical design is the overview's shared technical vocabulary: the Systems,
// Components, and data model that implementation and review decks reuse by
// identity. It is one page with a directory per kind, and one canonical page
// per definition. Both are read-only documentation.
//
// A definition page renders an exact revision. With no revision named it shows
// the unique current revision; with competing heads it names every head and
// picks none. A saved pin opens that revision even when it is stale, retired,
// or conflicted, and says so, so following a slide's link never silently
// becomes a read of the latest definition.
//
// Proposal intent, lifecycle, and code currency are three separate facts. A
// definition authored before revisions recorded intent is unspecified, never
// inferred from its code or its age. Nothing here is a newness claim: "new"
// needs a named comparison, which this page does not have.

const technicalPath = "/technical"

var errTechnicalNotFound = errors.New("technical definition not found")

// technicalHref is a definition's canonical page. An empty revision opens the
// current definition.
func technicalHref(kind, id, revision string) string {
	href := technicalPath + "/" + url.PathEscape(kind) + "/" + url.PathEscape(id)
	if revision != "" {
		href += "?" + url.Values{"revision": {revision}}.Encode()
	}
	return href
}

// technicalPinHref opens the page of an exact pin.
func technicalPinHref(pin saga.DocumentationLink) string {
	kind, id, ok := technicalTargetParts(pin.Target)
	if !ok {
		return ""
	}
	revision := strings.TrimPrefix(pin.Revision, pin.Target+":revision:")
	return technicalHref(kind, id, revision)
}

func technicalTargetParts(target string) (string, string, bool) {
	parts := strings.Split(target, ":")
	if len(parts) != 5 || parts[0] != "urn" || parts[1] != "change-saga" {
		return "", "", false
	}
	return parts[3], parts[4], true
}

// technicalIntent is a revision's explicit proposal intent. Revisions that
// predate intent report unspecified; the page never infers it.
func technicalIntent(*requirements.TechnicalRevision) string { return "unspecified" }

// technicalLifecycle is the record's lifecycle, or conflicted when its heads
// compete.
func technicalLifecycle(record *requirements.TechnicalRecord) string {
	if record.CurrentLifecycle == nil {
		return "conflicted"
	}
	return record.CurrentLifecycle.State
}

// ----- Reverse usages -----

// maxTechnicalUsages bounds the usages listed for one definition. The count is
// always exact; only the listing is cut, and the cut is stated.
const maxTechnicalUsages = 200

// technicalUsage is one Item that pins a definition: where it is, the exact
// revision it pinned, and whether that pin is still current.
type technicalUsage struct {
	Target  string
	Pin     saga.DocumentationLink
	PinHref string
	Status  string
	// Context is "Implementation" or "Review"; Feature and Review name the
	// place that holds the deck.
	Context     string
	Feature     string
	FeatureHref string
	Review      string
	ReviewHref  string
	Deck        string
	Slide       string
	Item        string
	Description string
	// Href opens the slide at the Item.
	Href string
}

// technicalUsageIndex is every Item-level definition pin in the Saga, by
// definition target. It reads the loaded decks only; it never resolves code.
type technicalUsageIndex map[string][]technicalUsage

func indexTechnicalUsages(document *saga.Saga, inventory requirements.Inventory) technicalUsageIndex {
	index := technicalUsageIndex{}
	deckFeature := map[string]*saga.Feature{}
	for _, feature := range document.Features {
		for _, deck := range feature.Decks {
			deckFeature[deck.Target] = feature
		}
	}
	locations := indexManifestTargets(document)
	add := func(usage technicalUsage, item *saga.Item) {
		pin := *item.Documentation
		usage.Target, usage.Pin, usage.Status = item.Target, pin, inventory.LinkStatus(pin)
		usage.PinHref = technicalPinHref(pin)
		usage.Item, usage.Description = item.Label, item.Description
		if usage.Item == "" {
			usage.Item = item.ID
		}
		index[pin.Target] = append(index[pin.Target], usage)
	}
	for _, deck := range document.Decks {
		feature := deckFeature[deck.Target]
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if item.Documentation == nil {
					continue
				}
				usage := technicalUsage{Context: "Implementation", Deck: deck.Title, Slide: slide.Title}
				if feature != nil {
					usage.Feature, usage.FeatureHref = featureTitle(feature), featureHref(feature.ID)
				}
				if location, ok := locations[item.Target]; ok {
					usage.Href = "/?view=slides" + location.Href
				}
				add(usage, item)
			}
		}
	}
	for _, review := range document.Reviews {
		if review.Deck == nil {
			continue
		}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				if item.Documentation == nil {
					continue
				}
				usage := technicalUsage{Context: "Review", Review: reviewNavTitle(review), ReviewHref: reviewHref(review.ID), Deck: review.Deck.Title, Slide: slide.Title}
				usage.Href = reviewHref(review.ID) + "#" + domID(item.Target)
				add(usage, item)
			}
		}
	}
	return index
}

// technicalUsagesView is one definition's usages, bounded for display.
type technicalUsagesView struct {
	Target    string
	Total     int
	Current   int
	Usages    []technicalUsage
	Truncated int
}

func (index technicalUsageIndex) view(target string) technicalUsagesView {
	usages := index[target]
	view := technicalUsagesView{Target: target, Total: len(usages)}
	for _, usage := range usages {
		if usage.Status == "current" {
			view.Current++
		}
	}
	if len(usages) > maxTechnicalUsages {
		view.Truncated = len(usages) - maxTechnicalUsages
		usages = usages[:maxTechnicalUsages]
	}
	view.Usages = usages
	return view
}

// ----- The Technical design page -----

type technicalPageView struct {
	Systems    *directoryView
	Components *directoryView
	// DataModel states what the data model holds. This Saga's format has no
	// data-entity records yet, so it says so rather than drawing an empty ERD.
	DataModel string
	Query     string
}

func technicalDirectory(id, title, lede, noun, nouns, command string) *directoryView {
	return &directoryView{
		ID: id, Title: title, Action: technicalPath, Lede: lede,
		Label: "Filter " + nouns, Noun: noun, Nouns: nouns,
		Columns: []directoryColumn{
			{Title: title[:len(title)-1]}, {Title: "Explanation", Wide: true},
			{Title: "Intent"}, {Title: "Lifecycle"}, {Title: "Revision"},
			{Title: "Code references", Numeric: true}, {Title: "Used by Items", Numeric: true},
		},
		Empty:   "No " + nouns + " yet.",
		Command: command,
	}
}

func technicalPage(inventory requirements.Inventory, usages technicalUsageIndex, query string) *technicalPageView {
	view := &technicalPageView{
		Systems:    technicalDirectory("technical-systems", "Systems", "How Components interact and how data flows through them.", "System", "Systems", "change-saga system add"),
		Components: technicalDirectory("technical-components", "Components", "Identifiable units of logic or transformation, reused by identity across decks.", "Component", "Components", "change-saga component add"),
		DataModel:  "No data entities are recorded in this Saga. The data model and its authored ERD appear here once data-entity records exist.",
		Query:      query,
	}
	for index := range inventory.Records {
		record := &inventory.Records[index]
		directory := view.Components
		if record.Kind == "system" {
			directory = view.Systems
		}
		name, explanation, codeCount := record.Identity.ID, "", 0
		intent := gapCell("unknown — competing revisions")
		revision := gapCell(strconv.Itoa(len(record.RevisionHeads)) + " competing revisions")
		if current := record.CurrentRevision; current != nil {
			name, explanation, codeCount = current.Name, current.Explanation, len(current.Code)
			intent = textCell(technicalIntent(current))
			if intent.Text == "unspecified" {
				intent = gapCell("unspecified")
			}
			revision = textCell(current.ID)
		}
		lifecycle := textCell(technicalLifecycle(record))
		if record.CurrentLifecycle == nil {
			lifecycle = gapCell("conflicted")
		}
		used := usages.view(record.Target)
		usedCell := countCell(used.Total)
		if used.Total > used.Current {
			usedCell.Note = strconv.Itoa(used.Total-used.Current) + " not current"
		}
		directory.addRow(directoryRow{Key: record.Identity.ID, Cells: []directoryCell{
			{Text: name, Href: technicalHref(record.Kind, record.Identity.ID, ""), Note: record.Identity.ID, Target: record.Target},
			textCell(summarise(explanation, 140)),
			intent, lifecycle, revision, countCell(codeCount), usedCell,
		}})
	}
	view.Systems.apply(query)
	view.Components.apply(query)
	return view
}

// technicalCount is the overview directory's count for Technical design.
func technicalCount(inventory requirements.Inventory) (int, int) {
	systems, components := 0, 0
	for _, record := range inventory.Records {
		if record.Kind == "system" {
			systems++
		} else {
			components++
		}
	}
	return systems, components
}

// technicalOverviewPart is the overview directory's row for Technical design.
func technicalOverviewPart(inventory requirements.Inventory) overviewPartView {
	systems, components := technicalCount(inventory)
	part := overviewPartView{Title: "Technical design", Href: technicalPath,
		Count: plural(systems, "System", "Systems") + " · " + plural(components, "Component", "Components"),
		Note:  "The Systems, Components, and data model decks reuse by identity."}
	if systems+components == 0 {
		part.Gap = true
		part.Note = "No technical definitions yet. Run change-saga component add to record a reusable Component."
	}
	return part
}

// technicalNav is the sidebar's Technical design rows: Systems, then
// Components, one row per definition opening its canonical page.
func technicalNav(inventory requirements.Inventory) []*navNodeView {
	var nodes []*navNodeView
	for _, kind := range []string{"system", "component"} {
		for _, record := range inventory.Records {
			if record.Kind != kind {
				continue
			}
			title := record.Identity.ID
			if record.CurrentRevision != nil {
				title = record.CurrentRevision.Name
			}
			node := &navNodeView{Title: title, Href: technicalHref(kind, record.Identity.ID, ""), NodeID: "nav-" + domID(record.Target), Icon: "implementation"}
			if kind == "system" {
				node.Icon = "design"
			}
			switch {
			case record.CurrentRevision == nil || record.CurrentLifecycle == nil:
				node.Gap, node.Note = true, "conflicted"
			case record.CurrentLifecycle.State == "retired":
				node.Note = "retired"
			}
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// ----- One definition -----

type technicalEntityView struct {
	Kind, KindTitle, ID, Target string
	Name                        string
	// Revision is the exact revision shown, and Requested whether a pin
	// named it rather than the current definition.
	Revision  string
	Requested bool
	Intent    string
	Lifecycle string
	// PinStatus is the shown revision's status as a pin: current, stale,
	// retired, or conflicted.
	PinStatus string
	// Heads are every competing revision head, listed when there is no
	// unique current revision.
	Heads         []technicalRevisionLink
	CurrentHref   string
	Definition    *documentationView
	Usages        technicalUsagesView
	History       []technicalRevisionLink
	LifecycleNote string
}

type technicalRevisionLink struct {
	ID, Href string
	Current  bool
	Shown    bool
}

func (a *app) technicalEntityPage(ctx context.Context, inventory requirements.Inventory, usages technicalUsageIndex, kind, id, revisionID string) (*technicalEntityView, error) {
	target, err := requirements.TechnicalURN(inventory.SagaID, kind, id)
	if err != nil {
		return nil, errTechnicalNotFound
	}
	record := inventory.Find(target)
	if record == nil {
		return nil, errTechnicalNotFound
	}
	view := &technicalEntityView{Kind: kind, KindTitle: strings.ToUpper(kind[:1]) + kind[1:], ID: id, Target: target, Name: id, Lifecycle: technicalLifecycle(record), Requested: revisionID != ""}
	current := ""
	if record.CurrentRevision != nil {
		current = record.CurrentRevision.ID
		view.CurrentHref = technicalHref(kind, id, "")
	}
	if revisionID == "" {
		revisionID = current
	}
	for _, head := range record.RevisionHeads {
		headID := strings.TrimPrefix(head, target+":revision:")
		view.Heads = append(view.Heads, technicalRevisionLink{ID: headID, Href: technicalHref(kind, id, headID), Shown: headID == revisionID})
	}
	for _, revision := range record.Revisions {
		view.History = append(view.History, technicalRevisionLink{ID: revision.ID, Href: technicalHref(kind, id, revision.ID), Current: revision.ID == current, Shown: revision.ID == revisionID})
	}
	view.Usages = usages.view(target)
	if record.CurrentLifecycle != nil && record.CurrentLifecycle.Reason != "" {
		view.LifecycleNote = record.CurrentLifecycle.Reason
	}
	if revisionID == "" {
		// Competing heads and no pin: name them all and choose none.
		return view, nil
	}
	revision := record.Revision(target + ":revision:" + revisionID)
	if revision == nil {
		return nil, errTechnicalNotFound
	}
	pin := saga.DocumentationLink{Target: target, Revision: target + ":revision:" + revisionID}
	view.Revision, view.Name, view.Intent = revisionID, revision.Name, technicalIntent(revision)
	view.PinStatus = inventory.LinkStatus(pin)
	definition := a.documentationView(ctx, inventory, record, revision, pin)
	definition.OnPage = true
	view.Definition = &definition
	return view, nil
}

// technicalRequest serves the page's routes inside the app shell.
func (a *app) technicalShell(ctx context.Context, document *saga.Saga, route appRoute, query url.Values) (*technicalPageView, *technicalEntityView, error) {
	inventory, err := requirements.LoadInventory(a.root, document.Manifest.ID)
	if err != nil {
		return nil, nil, err
	}
	usages := indexTechnicalUsages(document, inventory)
	if route.kind == "technical" {
		return technicalPage(inventory, usages, query.Get("q")), nil, nil
	}
	entity, err := a.technicalEntityPage(ctx, inventory, usages, route.id, route.sub, query.Get("revision"))
	return nil, entity, err
}

// technicalUsagesPage is the drawer's lazily loaded usage list for one
// definition: every Item that pins it, at whichever revision it pinned.
func (a *app) technicalUsagesPage(w http.ResponseWriter, r *http.Request) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return
	}
	target := r.URL.Query().Get("target")
	if _, _, ok := technicalTargetParts(target); !ok || !strings.HasPrefix(target, "urn:change-saga:"+document.Manifest.ID+":") {
		http.Error(w, "Invalid definition reference", http.StatusBadRequest)
		return
	}
	inventory, err := requirements.LoadInventory(a.root, document.Manifest.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if inventory.Find(target) == nil {
		http.Error(w, "The definition is missing.", http.StatusNotFound)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "technical-usages", indexTechnicalUsages(document, inventory).view(target), "The usages could not be rendered.")
}

const technicalTemplates = `
{{define "technical-usages"}}<div class="technical-usages" data-technical-usages="{{.Target}}">{{if .Usages}}<p class="technical-usage-count">{{.Total}} {{if eq .Total 1}}Item pins{{else}}Items pin{{end}} this definition{{if lt .Current .Total}}; {{.Current}} at its current revision{{end}}.</p><ul class="trace-links technical-usage-list">{{range .Usages}}<li data-technical-usage="{{.Target}}" data-usage-status="{{.Status}}">{{if .Href}}<a href="{{.Href}}" data-usage-item="{{.Target}}">{{.Item}}</a>{{else}}<span>{{.Item}}</span>{{end}} <small class="trace-kind">{{.Context}} Item</small><small class="trace-note">{{if .Feature}}<a href="{{.FeatureHref}}">{{.Feature}}</a> · {{end}}{{if .Review}}<a href="{{.ReviewHref}}">{{.Review}}</a> · {{end}}{{.Deck}} · {{.Slide}}</small><small class="technical-pin">pins <a href="{{.PinHref}}"><code>{{.Pin.Revision}}</code></a>{{if ne .Status "current"}} · <span class="gap">{{.Status}}</span>{{end}}</small>{{if .Description}}<p class="trace-rationale">{{.Description}}</p>{{end}}</li>{{end}}</ul>{{if .Truncated}}<p class="gap" role="status">{{.Truncated}} more usages are not listed here.</p>{{end}}{{else}}<p class="term-empty">No implementation or review Item pins this definition yet.</p>{{end}}</div>{{end}}

{{define "technical-page"}}<section class="app-page technical-page" data-technical-page><nav class="requirements-breadcrumbs" aria-label="Technical design breadcrumb"><a href="/">Overview</a><span>/</span><strong>Technical design</strong></nav><header class="page-heading"><h1>Technical design</h1><p class="app-lede">The application's shared technical vocabulary: Systems, Components, and the data model that implementation and review decks reuse by identity. Intent, lifecycle, and code currency are separate facts; none of them is a review approval.</p></header>
<nav class="technical-jump" aria-label="Technical design sections"><a href="#technical-systems-section">Systems</a><a href="#technical-components-section">Components</a><a href="#technical-data-model">Data model</a></nav>
<section class="app-page-section" id="technical-systems-section" data-technical-kind="system"><h2>Systems</h2><p class="app-lede">{{.Systems.Lede}}</p>{{template "directory" .Systems}}</section>
<section class="app-page-section" id="technical-components-section" data-technical-kind="component"><h2>Components</h2><p class="app-lede">{{.Components.Lede}}</p>{{template "directory" .Components}}</section>
<section class="app-page-section" id="technical-data-model" data-technical-data-model><h2>Data model</h2><p class="app-empty">{{.DataModel}}</p></section>
</section>{{end}}

{{define "technical-entity-page"}}<section class="app-page technical-entity-page" data-technical-entity="{{.Target}}"{{if .Revision}} data-technical-revision="{{.Revision}}"{{end}}><nav class="requirements-breadcrumbs" aria-label="Definition breadcrumb"><a href="/">Overview</a><span>/</span><a href="/technical">Technical design</a><span>/</span><strong>{{.Name}}</strong></nav>
<header class="requirement-story-hero"><div><p class="app-page-kind">{{.KindTitle}}</p><h1>{{.Name}}</h1><p class="technical-facts"><span data-technical-fact="revision">Revision <code>{{if .Revision}}{{.Revision}}{{else}}none chosen{{end}}</code></span><span data-technical-fact="intent">Intent: {{if eq .Intent "unspecified"}}<span class="gap">unspecified</span>{{else if .Intent}}{{.Intent}}{{else}}<span class="gap">unknown</span>{{end}}</span><span data-technical-fact="lifecycle">Lifecycle: {{if eq .Lifecycle "conflicted"}}<span class="gap">conflicted</span>{{else}}{{.Lifecycle}}{{end}}</span></p>
{{if and .Revision (ne .PinStatus "current")}}<p role="status" class="gap" data-technical-pin-status="{{.PinStatus}}">{{if .Requested}}You are reading saved revision <code>{{.Revision}}</code>. It is {{.PinStatus}}; nothing has been repinned.{{else}}This definition is {{.PinStatus}}.{{end}}{{if and .CurrentHref .Requested}} <a href="{{.CurrentHref}}">Read the current definition</a>{{end}}</p>{{end}}
{{if gt (len .Heads) 1}}<div role="status" class="gap" data-technical-conflict><p>This definition has {{len .Heads}} competing revisions. None is chosen as current; reconciling them names every head.</p><ul>{{range .Heads}}<li><a href="{{.Href}}"{{if .Shown}} aria-current="page"{{end}}><code>{{.ID}}</code></a></li>{{end}}</ul></div>{{end}}
{{if .LifecycleNote}}<p class="term-state">{{.LifecycleNote}}</p>{{end}}</div></header>
{{with .Definition}}{{template "documentation" .}}{{end}}
<section class="app-page-section" data-technical-used-by><h2>Used by</h2>{{template "technical-usages" .Usages}}</section>
<section class="app-page-section" data-technical-history><h2>Revision history</h2><ul class="technical-history">{{range .History}}<li><a href="{{.Href}}"{{if .Shown}} aria-current="page"{{end}}><code>{{.ID}}</code></a>{{if .Current}} <small>current</small>{{end}}{{if .Shown}} <small>shown</small>{{end}}</li>{{end}}</ul><p class="term-empty">Opening another revision reads it only; it changes no slide's pin.</p></section>
</section>{{end}}
`

const technicalStyles = `
.technical-jump{display:flex;flex-wrap:wrap;gap:6px 18px;margin:0 0 8px;font:500 13px var(--ui)}
.technical-facts{display:flex;flex-wrap:wrap;gap:4px 18px;margin:6px 0 0;color:var(--muted);font:13px var(--ui)}
.technical-facts code{color:var(--ink)}
.technical-usage-list small{display:inline-block;margin-right:8px}
.technical-pin code{font-size:11px}
.technical-history{list-style:none;padding:0;display:flex;flex-wrap:wrap;gap:6px 16px}
.technical-entity-page [data-technical-conflict] ul{margin:4px 0 0}
.documentation-page-link{margin:4px 0 12px;font:500 13px var(--ui)}
`
