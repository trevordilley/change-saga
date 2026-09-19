package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
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
// which records the change edited or affected. Approval controls appear only
// on those records.
type layersResponse struct {
	Mode       string   `json:"mode"`
	Against    string   `json:"against,omitempty"`
	Head       string   `json:"head"`
	BaseOID    string   `json:"base_oid,omitempty"`
	HeadOID    string   `json:"head_oid,omitempty"`
	Changed    []string `json:"changed"`
	Affected   []string `json:"affected"`
	Approvable []string `json:"approvable"`
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
	a.layers.key, a.layers.value, a.layers.err = key, nil, err
	if err == nil {
		a.layers.value = &layers
	}
	return a.layers.value, a.layers.err
}

// awaitLayers is comparisonLayers for a write: an approval cannot be asked
// to retry, so it waits, bounded, for a comparison that is still building.
func (a *app) awaitLayers(w http.ResponseWriter, r *http.Request) *changeview.Layers {
	if a.layersLoader == nil {
		deadline := time.Now().Add(2 * time.Minute)
		for a.snapshot(r.Context()) == nil && time.Now().Before(deadline) {
			if state, _ := a.snapshotState(); state != "building" {
				break
			}
			select {
			case <-r.Context().Done():
				return nil
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	return a.comparisonLayers(w, r)
}

// approvable is the set of records a compared reviewer may approve or reject:
// the Changed and Affected layers.
func approvable(layers *changeview.Layers) map[string]bool {
	result := map[string]bool{}
	if layers == nil {
		return result
	}
	for _, change := range layers.Changed {
		if !change.Removed {
			result[change.URN] = true
		}
	}
	for _, affected := range layers.Affected {
		result[affected.URN] = true
	}
	return result
}

func (a *app) layersAPI(w http.ResponseWriter, r *http.Request) {
	response := layersResponse{Mode: a.rng.Mode(), Against: a.rng.Against, Head: a.rng.HeadRevision(), Changed: []string{}, Affected: []string{}, Approvable: []string{}, DOM: map[string]string{}}
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
		for urn := range approvable(layers) {
			response.Approvable = append(response.Approvable, urn)
		}
		sort.Strings(response.Approvable)
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
	if err := a.template.ExecuteTemplate(w, "change-view", changeView{Layers: layers, Titles: titles}); err != nil {
		http.Error(w, "The change could not be rendered.", http.StatusInternalServerError)
	}
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
	if err := a.template.ExecuteTemplate(w, "history-view", historyView{History: history, Saga: a.root}); err != nil {
		http.Error(w, "The history could not be rendered.", http.StatusInternalServerError)
	}
}

// reviewAllowed enforces goal 6 on every approval write: approval exists only
// in compare mode, and only for records in the Changed or Affected layer.
func (a *app) reviewAllowed(w http.ResponseWriter, r *http.Request, target string) bool {
	if a.rng.Observe() {
		http.Error(w, "Approval belongs to a change: open the Saga with --against to approve or reject.", http.StatusForbidden)
		return false
	}
	layers := a.awaitLayers(w, r)
	if layers == nil {
		return false
	}
	if !approvable(layers)[target] {
		http.Error(w, "Only records this change edited or affected can be approved or rejected.", http.StatusForbidden)
		return false
	}
	return true
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
