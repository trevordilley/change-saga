package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/semanticgraph"
)

// Observing has no change, so its Coverage is the documented code itself:
// every code reference the Saga holds, resolved at the observed head.
// Saga → Code lists each documenting record and the code it references;
// Code → Saga lists each referenced file and the records that explain it.
// Stale references are health warnings. Like status, it counts and never
// passes a verdict: code no record references is not a gap when observing,
// because ownership accumulates as changes land.

type observeCoverageView struct {
	Mode       string
	References int
	Stale      int
	Targets    []*observeTargetView
	Files      []*observeFileView
}

type observeTargetView struct {
	traceLink
	References []observeReferenceView
	Stale      int
	CodeHref   string
}

type observeReferenceView struct {
	Path     string
	Location string
	Stale    bool
	Reason   string
}

type observeFileView struct {
	Path    string
	Owners  []observeOwnerView
	Stale   int
	Ranges  int
	Targets int
}

type observeOwnerView struct {
	traceLink
	Location string
	Stale    bool
}

// documentedCode is one record and the code references it holds.
type documentedCode struct {
	target     string
	references []coderef.Reference
}

// collectDocumentedCode gathers every code reference in the Saga: the
// documentation tree's sections, explanations, and Items, test-case evidence,
// and terms.
func collectDocumentedCode(document *saga.Saga, records requirements.Document, tests quality.Document) []documentedCode {
	var result []documentedCode
	add := func(target string, files []saga.CodeFile) {
		var references []coderef.Reference
		for _, file := range files {
			references = append(references, file.References...)
		}
		if len(references) > 0 {
			result = append(result, documentedCode{target: target, references: references})
		}
	}
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		add(section.Target, section.Code)
		for _, fragment := range section.Fragments {
			add(fragment.Target, fragment.Code)
			for _, landmark := range fragment.Landmarks {
				add(landmark.Target, landmark.Code)
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	if document.Section != nil {
		walk(document.Section)
	}
	for _, testCase := range tests.TestCases {
		urn, _ := qualityid.TestCase(tests.SagaID, testCase.Identity.ID)
		var references []coderef.Reference
		for _, evidence := range testCase.Evidence {
			references = append(references, evidence.Code...)
		}
		if len(references) > 0 {
			result = append(result, documentedCode{target: urn, references: references})
		}
	}
	for _, term := range records.Terms {
		if term.CurrentRevision == nil || len(term.CurrentRevision.Code) == 0 {
			continue
		}
		urn, _ := requirements.TermURN(records.SagaID, term.Identity.ID)
		result = append(result, documentedCode{target: urn, references: term.CurrentRevision.Code})
	}
	return result
}

// observeGraphCache keeps the observed graph while no file of the Saga has
// changed. Every observed Coverage row a reviewer opens asks for one record's
// code, and loading the whole Saga with its code for each of them costs far
// more than the code itself.
type observeGraphCache struct {
	mutex       sync.Mutex
	fingerprint string
	graph       *appGraph
	documented  []documentedCode
	builds      int
}

// observeGraph is the full Saga, with its code, and the records around it.
// Callers only read what it returns.
func (a *app) observeGraph() (*appGraph, []documentedCode, error) {
	a.observed.mutex.Lock()
	defer a.observed.mutex.Unlock()
	fingerprint, fingerprintErr := sagaFilesFingerprint(a.root)
	if fingerprintErr == nil && a.observed.graph != nil && fingerprint == a.observed.fingerprint {
		return a.observed.graph, a.observed.documented, nil
	}
	graph, documented, err := loadObserveGraph(a.root)
	if err != nil {
		return nil, nil, err
	}
	// Fingerprint after loading, as the outline does: an edit made while the
	// Saga was read produces a miss on the next request, never a stale hit.
	if after, err := sagaFilesFingerprint(a.root); fingerprintErr == nil && err == nil && after == fingerprint {
		a.observed.fingerprint, a.observed.graph, a.observed.documented = fingerprint, graph, documented
		a.observed.builds++
	}
	return graph, documented, nil
}

func loadObserveGraph(root string) (*appGraph, []documentedCode, error) {
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		return nil, nil, errors.New("The saga could not be loaded. Run change-saga validate for details.")
	}
	records, err := requirements.Load(root, document.Manifest.ID)
	if err != nil {
		return nil, nil, errors.New("The requirements could not be loaded. Run change-saga validate for details.")
	}
	if err := semanticgraph.ProjectSlideCriterionLinks(document, &records); err != nil {
		return nil, nil, errors.New("The complete-slide criterion links could not be loaded. Run change-saga validate for details.")
	}
	tests, err := quality.Load(root)
	if err != nil {
		tests = quality.Document{SagaID: document.Manifest.ID}
	}
	return newAppGraph(document, records, tests), collectDocumentedCode(document, records, tests), nil
}

func (a *app) observeCoverage(ctx context.Context, mode string) (*observeCoverageView, error) {
	graph, documented, err := a.observeGraph()
	if err != nil {
		return nil, err
	}
	view := &observeCoverageView{Mode: mode}
	resolver, resolveErr := coderesolve.New(ctx, a.sourceDir)
	headOID := ""
	if resolveErr == nil {
		defer resolver.Close()
		headOID, _ = resolveCommit(ctx, a.sourceDir, firstNonEmptyString(a.rng.Head, "HEAD"))
	}
	files := map[string]*observeFileView{}
	for _, record := range documented {
		target := &observeTargetView{traceLink: graph.link(record.target), CodeHref: "/api/reference-code?target=" + url.QueryEscape(record.target)}
		seen := map[string]bool{}
		for _, reference := range record.references {
			row := observeReferenceView{Path: reference.Path, Location: reference.Location().String()}
			switch {
			case resolveErr != nil:
				row.Stale, row.Reason = true, "the code repository is not available to this reviewer"
			default:
				if at := resolver.Resolve(ctx, reference, headOID); at.Current() {
					row.Path, row.Location = at.Location.Path, at.Location.String()
				} else {
					row.Stale, row.Reason = true, at.Reason
				}
			}
			view.References++
			if row.Stale {
				view.Stale++
				target.Stale++
			}
			target.References = append(target.References, row)
			file := files[row.Path]
			if file == nil {
				file = &observeFileView{Path: row.Path}
				files[row.Path] = file
			}
			file.Ranges++
			if row.Stale {
				file.Stale++
			}
			file.Owners = append(file.Owners, observeOwnerView{traceLink: target.traceLink, Location: row.Location, Stale: row.Stale})
			if !seen[row.Path] {
				seen[row.Path] = true
				file.Targets++
			}
		}
		view.Targets = append(view.Targets, target)
	}
	for _, file := range files {
		view.Files = append(view.Files, file)
	}
	sort.Slice(view.Files, func(i, j int) bool { return view.Files[i].Path < view.Files[j].Path })
	return view, nil
}

// observeCoveragePage serves the Coverage tab when observing.
func (a *app) observeCoveragePage(w http.ResponseWriter, r *http.Request, mode string) {
	view, err := a.observeCoverage(r.Context(), mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "observe-coverage-page", view, "The coverage page could not be rendered.")
}

// referenceCodePage renders one record's code references as code at the
// head, for an observed Coverage row a reviewer opened.
func (a *app) referenceCodePage(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	_, documented, err := a.observeGraph()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, record := range documented {
		if record.target != target {
			continue
		}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "reference-code", a.referenceCode(r.Context(), record.references, "record"), "The code could not be rendered.")
		return
	}
	http.Error(w, "no documented code for this target", http.StatusNotFound)
}

// coverageTotalsPage answers the overview's coverage line once it can: the
// comparison's totals when comparing, the documented code when observing.
// While the comparison builds it asks the browser to retry.
func (a *app) coverageTotalsPage(w http.ResponseWriter, r *http.Request) {
	if a.rng.Observe() {
		view, err := a.observeCoverage(r.Context(), "saga")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "observe-totals", view, "The coverage totals could not be rendered.")
		return
	}
	current := a.requestSnapshot(w, r)
	if current == nil {
		return
	}
	if current.diffErr != nil {
		http.Error(w, "The source comparison could not be loaded.", http.StatusInternalServerError)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "coverage-totals", a.cachedCoverageTotals(), "The coverage totals could not be rendered.")
}
