package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// layersCache keeps the layers of the current comparison generation. The
// comparison itself is cached by the snapshot; the layers add the Saga at the
// merge-base, so they are derived once per generation.
type layersCache struct {
	mutex sync.Mutex
	key   string
	value *changeview.Layers
	err   error
}

// layersResponse is /api/layers: how the reviewer was opened and, comparing,
// which records the change edited or affected. The page marks those records
// read-only; approvals happen only in the pull request's review.
type layersResponse struct {
	Mode     string   `json:"mode"`
	Against  string   `json:"against,omitempty"`
	Head     string   `json:"head"`
	BaseOID  string   `json:"base_oid,omitempty"`
	HeadOID  string   `json:"head_oid,omitempty"`
	Changed  []string `json:"changed"`
	Affected []string `json:"affected"`
	// DOM maps each Changed and Affected record to its element id, so the
	// page can mark it and quiet everything else.
	DOM map[string]string `json:"dom"`
}

// comparisonLayers returns the layers of the current generation, or nil with
// the HTTP response already written (202 while the comparison builds).
func (a *app) comparisonLayers(w http.ResponseWriter, r *http.Request) *changeview.Layers {
	if a.layersLoader != nil {
		layers, err := a.layersLoader(r.Context())
		if err != nil {
			http.Error(w, "The comparison layers could not be derived: "+err.Error(), http.StatusInternalServerError)
			return nil
		}
		return layers
	}
	current := a.requestSnapshot(w, r)
	if current == nil {
		return nil
	}
	if current.diffErr != nil {
		http.Error(w, "The source comparison could not be loaded.", http.StatusInternalServerError)
		return nil
	}
	layers, err := a.layersFor(r.Context(), current)
	if err != nil {
		http.Error(w, "The comparison layers could not be derived: "+err.Error(), http.StatusInternalServerError)
		return nil
	}
	return layers
}

func (a *app) layersFor(ctx context.Context, current *reviewSnapshot) (*changeview.Layers, error) {
	a.layers.mutex.Lock()
	defer a.layers.mutex.Unlock()
	a.cache.mutex.Lock()
	key := current.identity + "\x00" + a.cache.saga
	a.cache.mutex.Unlock()
	if a.layers.key == key && (a.layers.value != nil || a.layers.err != nil) {
		return a.layers.value, a.layers.err
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		return nil, err
	}
	defer resolver.Close()
	layers, _, err := changeview.Open(ctx, changeview.OpenOptions{
		SagaRoot: a.root, Document: current.document, Checkout: a.sourceDir,
		Changes: current.changes, Report: current.report, Resolver: resolver,
	})
	if err != nil && ctx.Err() != nil {
		// The request went away while git was running. That is a fact about
		// the request, not the comparison, so the next request derives again.
		return nil, err
	}
	a.layers.key, a.layers.value, a.layers.err = key, nil, err
	if err == nil {
		a.layers.value = &layers
	}
	return a.layers.value, a.layers.err
}

func (a *app) layersAPI(w http.ResponseWriter, r *http.Request) {
	response := layersResponse{Mode: a.rng.Mode(), Against: a.rng.Against, Head: a.rng.HeadRevision(), Changed: []string{}, Affected: []string{}, DOM: map[string]string{}}
	if !a.rng.Observe() {
		layers := a.comparisonLayers(w, r)
		if layers == nil {
			return
		}
		response.BaseOID, response.HeadOID = layers.BaseOID, layers.HeadOID
		for _, change := range layers.Changed {
			response.Changed = append(response.Changed, change.URN)
		}
		for _, affected := range layers.Affected {
			response.Affected = append(response.Affected, affected.URN)
		}
		for _, urn := range append(append([]string{}, response.Changed...), response.Affected...) {
			response.DOM[urn] = domID(urn)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response)
}

// changeView is the Change tab: the three layers of the comparison.
type changeView struct {
	Layers *changeview.Layers
	Titles map[string]string
	// Reviews are the pull request reviews of the compared head. The layers
	// are read-only documentation; approval happens on these review slides.
	Reviews []reviewSummaryView
}

func (a *app) changePage(w http.ResponseWriter, r *http.Request) {
	if a.rng.Observe() {
		http.Error(w, "Observing has no change; open the Saga with --against to compare.", http.StatusNotFound)
		return
	}
	layers := a.comparisonLayers(w, r)
	if layers == nil {
		return
	}
	titles := map[string]string{}
	for _, change := range layers.Changed {
		titles[change.URN] = change.Title
	}
	for _, affected := range layers.Affected {
		titles[affected.URN] = affected.Title
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	view := changeView{Layers: layers, Titles: titles}
	if document, validation, err := saga.Load(a.root); err == nil && validation.Valid {
		view.Reviews = a.reviewsForHead(r.Context(), document, layers.HeadOID)
	}
	renderHTML(w, a.template, "change-view", view, "The change could not be rendered.")
}

// historyView is one record's history in the drawer.
type historyView struct {
	History changeview.History
	Saga    string
}

func (a *app) historyPage(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "target is required", http.StatusBadRequest)
		return
	}
	history, err := changeview.NodeHistory(r.Context(), a.root, target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "history-view", historyView{History: history, Saga: a.root}, "The history could not be rendered.")
}

// openingLabel describes how the reviewer was opened, for the shell.
func openingLabel(rng gitdiff.Range) string {
	if rng.Observe() {
		return "Observing " + rng.HeadRevision()
	}
	return "Comparing " + rng.HeadRevision() + " against " + rng.Against
}

func shortCommit(value string) string {
	if len(value) > 12 && !strings.ContainsAny(value, "/~^") {
		return value[:12]
	}
	return value
}
