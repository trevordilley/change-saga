package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/deprecationview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type deprecationPanel struct {
	Records   []deprecationview.Record
	Code      bool
	Note      string
	Truncated bool
}

// deprecations is a lazy, read-only inspection surface shared by narrative,
// diagrams and exact code. It preserves conflicting candidates and never
// labels related explanation evidence as deprecated code.
func (a *app) deprecations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	target := q.Get("target")
	panel := deprecationPanel{Code: q.Get("file") != "" || q.Get("ref") != ""}
	render := func() {
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "deprecation-panel", panel, "Deprecations could not be rendered.")
	}
	manifest, err := saga.ReadManifest(a.root)
	if err != nil {
		panel.Note = "Deprecation records could not be read."
		render()
		return
	}
	files := a.sagaFiles(r.Context())
	doc, err := files.records(manifest.ID)
	if err != nil {
		panel.Note = "Deprecation records are invalid or unavailable; their absence cannot be established."
		render()
		return
	}
	if target == "" && !panel.Code && q.Get("deprecation") == "" {
		http.Error(w, "requires target, ref or file", 400)
		return
	}
	if target != "" {
		if _, err := requirements.DeprecationTargetKind(manifest.ID, target); err != nil {
			http.Error(w, "invalid target", 400)
			return
		}
	}
	filter := deprecationview.Filter{ID: q.Get("deprecation")}
	var resolver *coderesolve.Resolver
	head := ""
	if panel.Code {
		catalog, e := a.sourceCatalog(r.Context(), manifest)
		if e != nil {
			panel.Note = "Code deprecation could not be checked: the source repository is unavailable."
			render()
			return
		}
		head = catalog.HeadOID
		location := coderef.Location{Commit: head, Path: q.Get("file")}
		if q.Get("ref") != "" {
			location, err = coderef.ParseLocation(q.Get("ref"))
			if err != nil {
				http.Error(w, "invalid exact reference", 400)
				return
			}
		}
		if err = location.Validate(); err != nil {
			http.Error(w, "invalid code location", 400)
			return
		}
		filter.Ref = &location
		resolver, err = coderesolve.New(r.Context(), a.sourceDir)
		if err != nil {
			panel.Note = "Code deprecation could not be checked: the source repository is unavailable."
			render()
			return
		}
		defer resolver.Close()
	}
	var evidenceResolver deprecationview.Resolver
	if resolver != nil {
		evidenceResolver = resolver
	}
	selected := map[string]bool{target: true}
	if target != "" && q.Get("children") == "1" {
		// Visibility roll-up only: each child keeps its exact authored scope.
		document := files.narrative()
		if document == nil {
			// Annotation inspection also admits an invalid narrative's readable
			// children, as before; ordinary pages require a valid narrative.
			if partial, _, err := saga.LoadNarrative(a.root); err == nil {
				document = partial
			}
		}
		if document != nil {
			fragment := findFragmentByTarget(document, target)
			if fragment != nil {
				for _, landmark := range fragment.Landmarks {
					selected[landmark.Target] = true
				}
			}
			for _, review := range document.Reviews {
				if review.Deck != nil {
					for _, slide := range review.Deck.Slides {
						if slide.Target == target {
							for _, item := range slide.Items {
								selected[item.Target] = true
							}
						}
					}
				}
			}
		}
		prefix := "urn:change-saga:" + manifest.ID + ":whiteboard:"
		if strings.HasPrefix(target, prefix) {
			revision := q.Get("revision")
			if board, e := requirements.LoadResolvedWhiteboard(a.root, manifest.ID, strings.TrimPrefix(target, prefix), revision); e == nil && board != nil {
				if description, e := board.Composition.Describe(""); e == nil {
					for _, instance := range append(description.Elements, description.Connections...) {
						if instance.Source != nil {
							selected[instance.Source.Target] = true
						}
					}
				}
			}
		}
	}
	if q.Get("compact") == "1" && !panel.Code {
		a.compactDeprecations(w, doc, selected, q)
		return
	}
	if panel.Code || q.Get("deprecation") != "" {
		panel.Records = deprecationview.Records(r.Context(), doc, head, evidenceResolver, filter)
	} else {
		panel.Records = files.contentDeprecations(doc).Records(r.Context(), selected)
	}
	if q.Get("compact") == "1" {
		allowed := map[string]bool{}
		for _, row := range panel.Records {
			allowed[row.Identity.ID] = true
		}
		summaries := deprecationSummaries(doc, nil)
		compact := summaries[:0]
		for _, row := range summaries {
			if allowed[row.ID] {
				detail := r.URL.Query()
				detail.Del("compact")
				detail.Set("deprecation", row.URN)
				row.DetailHref = "/api/deprecations?" + detail.Encode()
				compact = append(compact, row)
			}
		}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "deprecation-summary", compact, "Deprecations could not be listed.")
		return
	}
	if len(panel.Records) > 100 {
		panel.Records = panel.Records[:100]
		panel.Truncated = true
	}
	if q.Get("revision") != "" && len(panel.Records) > 0 {
		panel.Note = fmt.Sprintf("Reading saved revision %s. Annotations retain their authored pins; current target health is shown separately.", q.Get("revision"))
	}
	render()
}

const deprecationTemplates = `
{{define "deprecation-target"}}<div class="deprecation-load" data-deprecation-load hx-get="/api/deprecations?compact=1&amp;target={{urlquery .}}&amp;children=1" hx-trigger="load, saga:deprecations" hx-target="this" hx-swap="innerHTML" aria-live="polite"></div>{{end}}
{{define "deprecation-panel"}}
{{if or .Records .Note .Code}}<aside class="deprecation-panel" data-deprecations aria-label="Recorded deprecations">
{{if .Note}}<p role="status">{{.Note}}</p>{{end}}
{{if .Code}}<p class="deprecation-caveat">Only explicitly listed code is asserted deprecated. Related explanations do not deprecate their code.</p>{{end}}
{{range .Records}}{{$record := .}}<details class="deprecation-record" data-deprecation-state="{{.State}}" open><summary><strong>{{if eq .State "withdrawn"}}Deprecation withdrawn{{else if eq .State "conflicted"}}Deprecation has conflicting heads{{else}}Deprecation recorded{{end}}</strong></summary>
{{range .Assertions}}<section data-deprecation-scope="{{.Definition.Scope}}" data-target-health="{{.TargetHealth.Status}}"><p><strong>{{if eq .Definition.Scope "code"}}Explicit code scope{{else}}Content scope{{end}}</strong> · target {{.TargetHealth.Status}}</p><p>{{.Definition.Reason}}</p><p class="deprecation-target-name">Target: <code>{{.Definition.Target.Target}}</code>{{with .Definition.Target.Element}} · appearance <code>{{.}}</code>{{end}}</p>
{{with .TargetHealth.Reason}}<p>{{.}}</p>{{end}}
{{with .Definition.Replacement}}<p>Suggested replacement: <code>{{.Target}}</code>{{with .Element}} · <code>{{.}}</code>{{end}}</p>{{end}}{{with .ReplacementHealth}}<p>Replacement: {{.Status}}{{with .Reason}} · {{.}}{{end}}</p>{{end}}
{{with .RelatedCodeWarning}}<p>Related code could not be inspected: {{.}}</p>{{end}}
{{if .Code}}<ul class="deprecation-evidence">{{range .Code}}<li data-code-role="{{.Role}}"><strong>{{if eq .Role "deprecated_code"}}Authored deprecated code{{else}}Related code only{{end}}</strong>: <code>{{.Reference.Location}}</code> · {{.Resolution.State}}{{with .Match}} · {{.}}{{end}}{{if .Provisional}} · provisional{{end}}{{with .Resolution.Reason}}<p>{{.}}</p>{{end}}<details><summary>Evidence provenance</summary><p>Owner: <code>{{.Owner}}</code></p><p>Record: <code>{{.Record}}</code></p><p>Digest: <code>{{.Reference.Digest}}</code></p></details></li>{{end}}</ul>{{end}}
<details><summary>Authored assertion and target pin</summary><p><code>{{.Revision}}</code></p><p>Authored {{.Definition.CreatedAt}}</p><p>{{with .Definition.Target.Revision}}<code>{{.}}</code>{{end}}{{with .Definition.Target.ContentDigest}}<code>{{.}}</code>{{end}}</p></details></section>{{end}}
{{with .CurrentLifecycle}}{{if .Reason}}<p>Lifecycle reason: {{.Reason}}</p>{{end}}{{end}}
{{if .Stories}}<details><summary>Related stories (optional context)</summary><ul>{{range .Stories}}<li><code>{{.Relation.To}}</code> · story {{.Lifecycle}} · relation {{.Currency.Status}}<p>{{.Relation.Rationale}}</p>{{range .Currency.Reasons}}<p>{{.Message}}</p>{{end}}</li>{{end}}</ul></details>{{end}}
</details>{{else}}{{if .Code}}{{if not .Note}}<p data-no-recorded-deprecation>No recorded deprecation for this code. This is not proof of non-deprecation.</p>{{end}}{{end}}{{end}}
{{if .Truncated}}<p>Showing the first 100 annotations. Use the paginated deprecation query for the complete list.</p>{{end}}
</aside>{{end}}{{end}}
`
