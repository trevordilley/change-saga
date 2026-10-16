package server

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
)

// Discovery reads authored heads only. Target health and code provenance are
// resolved by the existing inspector when the reader opens a record.
type deprecationSummary struct {
	ID, URN, State, Scope, Reason, Preview, DetailHref string
	Targets                                            []deprecationPlace
}

type deprecationPlace struct {
	Target, Element, Revision, Href, Label string
}

type deprecationDirectory struct {
	Rows                                       []deprecationSummary
	Search, State, Scope, Page, Next, Previous string
	Total                                      int
}

func deprecationSummaries(doc requirements.Document, selected map[string]bool) []deprecationSummary {
	rows := []deprecationSummary{}
	for _, dep := range doc.Deprecations {
		urn, _ := requirements.DeprecationURN(doc.SagaID, dep.Identity.ID)
		row := deprecationSummary{ID: dep.Identity.ID, URN: urn, State: "conflicted", DetailHref: "/api/deprecations?" + url.Values{"deprecation": {urn}}.Encode()}
		if dep.CurrentRevision != nil && dep.CurrentLifecycle != nil {
			row.State = string(dep.CurrentLifecycle.State)
		}
		matched := selected == nil
		var reasons []string
		for _, revision := range dep.Revisions {
			head, _ := requirements.DeprecationRevisionURN(doc.SagaID, dep.Identity.ID, revision.ID)
			if !slices.Contains(dep.RevisionHeads, head) {
				continue
			}
			matched = matched || selected[revision.Target.Target]
			if row.Scope == "" {
				row.Scope = revision.Scope
			} else if row.Scope != revision.Scope {
				row.Scope = "mixed"
			}
			reasons = append(reasons, revision.Reason)
			row.Targets = append(row.Targets, deprecationPlace{Target: revision.Target.Target, Element: revision.Target.Element, Revision: revision.Target.Revision})
		}
		if !matched {
			continue
		}
		row.Reason = strings.Join(reasons, " · ")
		preview := []rune(row.Reason)
		if len(preview) > 180 {
			preview = append(preview[:180], '…')
		}
		row.Preview = string(preview)
		rows = append(rows, row)
	}
	return rows
}

func deprecationPlaceLink(place deprecationPlace, graph *appGraph, metadata requirements.WhiteboardMetadata) deprecationPlace {
	parts := strings.Split(place.Target, ":")
	place.Label = recordLabel(place.Target)
	if len(parts) < 5 {
		return place
	}
	kind, id := parts[3], parts[4]
	switch kind {
	case "whiteboard":
		// Named URLs retain identity even when the role owner changes.
		place.Href = whiteboardMetadataHref(metadata, id, "")
		if place.Element != "" {
			// An appearance belongs to its authored revision, not an unverified
			// element at the current head.
			revision := place.Revision[strings.LastIndex(place.Revision, ":")+1:]
			place.Href = whiteboardFocusHref(whiteboardMetadataHref(metadata, id, revision), place.Element)
		}
	case "component", "system", "data-entity", "erd", "erd-overlay":
		place.Href = technicalHref(kind, id, strings.TrimPrefix(place.Revision, place.Target+":revision:"))
	default:
		if graph != nil {
			link := graph.link(place.Target)
			place.Href = link.Href
			if link.Title != "" {
				place.Label = link.Title
			}
		}
	}
	return place
}

func makeDeprecationDirectory(doc requirements.Document, graph *appGraph, metadata requirements.WhiteboardMetadata, query url.Values) *deprecationDirectory {
	view := &deprecationDirectory{Search: query.Get("q"), State: query.Get("state"), Scope: query.Get("scope"), Page: query.Get("page")}
	rows := deprecationSummaries(doc, nil)
	filtered := []deprecationSummary{}
	for _, row := range rows {
		if view.State != "" && view.State != "all" && row.State != view.State {
			continue
		}
		if view.Scope != "" && row.Scope != view.Scope && row.Scope != "mixed" {
			continue
		}
		matchedPage := view.Page == ""
		text := row.ID + " " + row.Reason
		for i, place := range row.Targets {
			place = deprecationPlaceLink(place, graph, metadata)
			row.Targets[i] = place
			text += " " + place.Target + " " + place.Label
			if href, err := url.Parse(place.Href); err == nil && href.Path == view.Page {
				matchedPage = true
			}
		}
		if !matchedPage || !strings.Contains(strings.ToLower(text), strings.ToLower(view.Search)) {
			continue
		}
		filtered = append(filtered, row)
	}
	view.Total = len(filtered)
	offset, _ := strconv.Atoi(query.Get("offset"))
	if offset < 0 || offset > len(filtered) {
		offset = 0
	}
	end := min(offset+50, len(filtered))
	view.Rows = filtered[offset:end]
	link := func(offset int) string {
		q := maps.Clone(query)
		q.Set("offset", strconv.Itoa(offset))
		return "/deprecations?" + q.Encode()
	}
	if end < len(filtered) {
		view.Next = link(end)
	}
	if offset > 0 {
		view.Previous = link(max(0, offset-50))
	}
	return view
}

// compactDeprecations is embedded beside content. It contains no expanded
// prose or provenance and does not perform code resolution.
func (a *app) compactDeprecations(w http.ResponseWriter, doc requirements.Document, selected map[string]bool, query url.Values) {
	rows := deprecationSummaries(doc, selected)
	for i := range rows {
		q := maps.Clone(query)
		q.Del("compact")
		q.Set("deprecation", rows[i].URN)
		rows[i].DetailHref = "/api/deprecations?" + q.Encode()
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "deprecation-summary", rows, "Deprecations could not be listed.")
}

const deprecationDiscoveryTemplates = `
{{define "deprecation-summary"}}{{if .}}<div class="deprecation-summary" data-deprecation-summary><button type="button" class="deprecation-badge" data-deprecation-list aria-label="Deprecations on this content">Deprecations · {{len .}}</button><div hidden data-deprecation-entries>{{range .}}<div data-deprecation-entry data-deprecation-id="{{.URN}}" data-deprecation-state="{{.State}}" data-deprecation-href="{{.DetailHref}}" data-deprecation-preview="{{.Preview}}">{{range .Targets}}<span data-deprecation-anchor="{{domID .Target}}" data-deprecation-target="{{.Target}}" data-deprecation-element="{{.Element}}" data-deprecation-revision="{{.Revision}}"></span>{{end}}<button type="button" class="deprecation-entry-button" data-deprecation-detail="{{.DetailHref}}">{{.Preview}}</button><small>{{.Scope}} · {{.State}}</small></div>{{end}}</div></div>{{end}}{{end}}
{{define "deprecation-directory"}}<article class="deprecation-directory reading-surface"><header class="page-heading"><h1>Deprecations</h1><p>Recorded reasons to stop using content or specific code. Open a record for its current health, replacement and history.</p></header><form method="get" action="/deprecations" class="deprecation-filters"><label>Search <input type="search" aria-label="Search" name="q" value="{{.Search}}" placeholder="Reason or target"></label><label>State <select name="state" aria-label="State"><option value="all">All states</option><option value="active"{{if eq .State "active"}} selected{{end}}>Active</option><option value="withdrawn"{{if eq .State "withdrawn"}} selected{{end}}>Withdrawn</option><option value="conflicted"{{if eq .State "conflicted"}} selected{{end}}>Conflicted</option></select></label><label>Scope <select name="scope" aria-label="Scope"><option value="">Content and code</option><option value="content"{{if eq .Scope "content"}} selected{{end}}>Content</option><option value="code"{{if eq .Scope "code"}} selected{{end}}>Code</option></select></label>{{if .Page}}<input type="hidden" name="page" value="{{.Page}}">{{end}}<button type="submit">Filter</button></form>{{if .Page}}<p>On {{.Page}} · <a href="/deprecations">Show the whole Saga</a></p>{{end}}<p role="status">{{.Total}} recorded {{if eq .Total 1}}deprecation{{else}}deprecations{{end}}</p><ul class="deprecation-results">{{range .Rows}}<li data-deprecation-result="{{.ID}}"><button type="button" class="deprecation-entry-button" data-deprecation-detail="{{.DetailHref}}">{{.Preview}}</button><p>{{.Scope}} · {{.State}}</p>{{range .Targets}}<p>{{if .Href}}<a href="{{.Href}}">Show {{.Label}}{{if .Element}} · {{.Element}}{{end}}</a>{{else}}Target no longer available: <code>{{.Target}}</code>{{end}}</p>{{end}}</li>{{else}}<li>No recorded deprecations match these filters.</li>{{end}}</ul><nav aria-label="Deprecation pages">{{if .Previous}}<a href="{{.Previous}}">Previous</a>{{end}} {{if .Next}}<a href="{{.Next}}">Next</a>{{end}}</nav></article>{{end}}
`
