package server

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/prototypes"
)

// A prototype is the fourth reading surface beside essays, decks, and
// whiteboards: an interactive HTML app a reader runs rather than reads. Its
// page frames one revision and lists the annotations that pin parts of it to
// stories and criteria. The app itself is served from /p/, on the same opaque
// sandboxed origin as an interactive slide, so it runs its own scripts but
// cannot reach the reviewer that frames it, even when opened in its own tab.

func prototypeHref(id string) string { return "/prototypes/" + url.PathEscape(id) }

func prototypeRevisionHref(id, revision string) string {
	return prototypeHref(id) + "?revision=" + url.QueryEscape(revision)
}

// prototypeFileHref is where one revision's files are served. Relative links
// inside the app resolve beneath it, so its own assets load as authored.
func prototypeFileHref(id, revision, entry string) string {
	return "/p/" + url.PathEscape(id) + "/" + url.PathEscape(revision) + "/" + entry
}

type prototypePageView struct {
	ID        string
	Target    string
	Title     string
	State     string
	FeatureID string
	Feature   traceLink
	Conflict  bool
	Revisions []prototypeRevisionView
	// Revision is the one shown: the current head unless ?revision= names
	// another.
	Revision prototypeRevisionView
	// FrameURL is the sandboxed app; OpenURL opens the same app in its own
	// tab. Both are empty for a prototype that lives outside the Saga.
	FrameURL string
	OpenURL  string
	// ExternalURL is a prototype hosted elsewhere, shown as a link out.
	ExternalURL string
	// Location and Commit say where a repository-backed prototype lives.
	Location string
	Commit   string
	// Stale says why a repository-backed prototype no longer matches its pin.
	Stale       string
	Unavailable string
	Annotations []prototypeAnnotationView
}

type prototypeRevisionView struct {
	ID       string
	Title    string
	State    string
	Href     string
	Created  string
	Current  bool
	Selected bool
}

type prototypeAnnotationView struct {
	ID         string
	Rationale  string
	Selector   string
	Target     traceLink
	Revision   string
	OnRevision bool
}

func (a *app) prototypePage(graph *appGraph, sagaID, id, requested string) (*prototypePageView, error) {
	document, note := a.prototypeDocument(sagaID)
	if note != "" {
		return nil, fmt.Errorf("The prototypes %s.", note)
	}
	prototype := document.Find(id)
	if prototype == nil {
		return nil, errAppPageNotFound
	}
	target, _ := prototypes.PrototypeURN(sagaID, id)
	view := &prototypePageView{ID: id, Target: target, Title: id, FeatureID: prototype.Feature, Conflict: prototype.RevisionConflict()}
	if prototype.Feature != "" {
		view.Feature = graph.link(applayout.FeatureURN(sagaID, prototype.Feature))
	}
	current := ""
	if prototype.CurrentRevision != nil {
		current = prototype.CurrentRevision.ID
	}
	selected := current
	if requested != "" {
		if prototype.Revision(requested) == nil {
			return nil, errAppPageNotFound
		}
		selected = requested
	}
	revisions := append([]prototypes.Revision(nil), prototype.Revisions...)
	sort.SliceStable(revisions, func(i, j int) bool { return revisions[i].CreatedAt.Before(revisions[j].CreatedAt) })
	for _, revision := range revisions {
		row := prototypeRevisionView{
			ID: revision.ID, Title: revision.Title, State: string(revision.State),
			Href: prototypeRevisionHref(id, revision.ID), Created: revision.CreatedAt.Format("2006-01-02"),
			Current: revision.ID == current, Selected: revision.ID == selected,
		}
		if row.Current {
			row.Href = prototypeHref(id)
		}
		view.Revisions = append(view.Revisions, row)
		if row.Selected {
			view.Revision = row
		}
	}
	if revision := prototype.Revision(selected); revision != nil {
		view.Title, view.State = revision.Title, string(revision.State)
		switch revision.Source.Kind {
		case prototypes.SourceHTML:
			view.FrameURL = prototypeFileHref(id, revision.ID, revision.Source.Entrypoint)
			view.OpenURL = view.FrameURL
		case prototypes.SourceExternal, prototypes.SourceEmbed:
			view.ExternalURL = revision.Source.URL
		default:
			view.Unavailable = "This revision's source cannot be shown here."
		}
	} else {
		view.Unavailable = "This prototype has no single current revision to show."
	}
	selectedURN, _ := prototypes.RevisionURN(sagaID, id, selected)
	for _, annotation := range document.Annotations {
		if annotation.Prototype != target {
			continue
		}
		row := prototypeAnnotationView{
			ID: annotation.ID, Rationale: annotation.Rationale, Selector: prototypeSelectorLabel(annotation.Selector),
			Target: graph.link(annotation.Target), OnRevision: annotation.PrototypeRevision == "" || annotation.PrototypeRevision == selectedURN,
		}
		if revision := annotation.PrototypeRevision; revision != "" {
			row.Revision = revision[strings.LastIndex(revision, ":")+1:]
		}
		view.Annotations = append(view.Annotations, row)
	}
	return view, nil
}

// prototypeSelectorLabel names the part of the app an annotation points at.
func prototypeSelectorLabel(selector prototypes.Selector) string {
	switch selector.Kind {
	case prototypes.SelectorElement:
		return "#" + selector.ElementID
	case prototypes.SelectorText:
		return "“" + selector.ExactText + "”"
	case prototypes.SelectorRegion:
		if region := selector.Region; region != nil {
			return fmt.Sprintf("region %.0f%%, %.0f%%", region.X*100, region.Y*100)
		}
		return "region"
	case prototypes.SelectorProvider:
		return "provider node " + selector.ProviderID
	}
	return string(selector.Kind)
}

// prototypeFile serves one file of one prototype revision. The response
// carries the authored-content policy, whose sandbox directive gives it an
// opaque origin wherever it loads: framed in the page or opened in its own
// tab, the app's scripts run but can neither read the reviewer nor call it.
func (a *app) prototypeFile(w http.ResponseWriter, r *http.Request) {
	index, validation, err := a.sagaFiles(r.Context()).mutation()
	if err != nil || !validation.Valid {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return
	}
	document, note := a.prototypeDocument(index.Manifest.ID)
	if note != "" {
		http.Error(w, "The prototypes "+note+".", http.StatusInternalServerError)
		return
	}
	prototype := document.Find(r.PathValue("prototype"))
	if prototype == nil {
		http.NotFound(w, r)
		return
	}
	revision := prototype.Revision(r.PathValue("revision"))
	if revision == nil {
		http.NotFound(w, r)
		return
	}
	rel := path.Clean("/" + r.PathValue("path"))[1:]
	if rel == "" || hasReservedPart(filepath.FromSlash(rel)) {
		http.Error(w, "invalid prototype path", http.StatusBadRequest)
		return
	}
	switch revision.Source.Kind {
	case prototypes.SourceHTML:
		a.servePrototypePackageFile(w, r, document, *prototype, revision.ID, rel)
	default:
		http.NotFound(w, r)
	}
}

// servePrototypePackageFile serves a file stored inside the revision's
// html/ package. The package is digest-checked when it loads, so its bytes
// are the ones the revision pinned.
func (a *app) servePrototypePackageFile(w http.ResponseWriter, r *http.Request, document prototypes.Document, prototype prototypes.Prototype, revisionID, rel string) {
	if rel != "html" && !strings.HasPrefix(rel, "html/") {
		http.NotFound(w, r)
		return
	}
	dir, err := document.RevisionDir(prototype, revisionID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	root := filepath.Join(dir, "html")
	full := filepath.Join(dir, filepath.FromSlash(rel))
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realPath, pathErr := filepath.EvalSymlinks(full)
	if rootErr != nil || pathErr != nil {
		http.NotFound(w, r)
		return
	}
	if within, err := filepath.Rel(realRoot, realPath); err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		http.Error(w, "prototype file escapes its package", http.StatusForbidden)
		return
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(realPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	writePrototypeFile(w, r, realPath, data, info.ModTime())
}

func writePrototypeFile(w http.ResponseWriter, r *http.Request, name string, data []byte, modified time.Time) {
	w.Header().Set("Content-Security-Policy", authoredContentPolicy)
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(name), modified, bytes.NewReader(data))
}
