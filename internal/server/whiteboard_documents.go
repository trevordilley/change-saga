package server

import (
	"net/http"
	"net/url"

	"github.com/twentyideas/changesaga/internal/requirements"
)

type whiteboardDocumentView struct{ ID, Name, Href, Note string }
type whiteboardRevisionView struct {
	ID, Href string
	Shown    bool
}

func whiteboardHref(id, revision string) string {
	href := technicalPath + "/whiteboards/" + url.PathEscape(id)
	if id == "data-model" {
		href = technicalPath + "/erd"
	}
	if revision != "" {
		href += "?" + url.Values{"revision": {revision}}.Encode()
	}
	return href
}

func (a *app) namedWhiteboardPage(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("board") == "data-model" {
		destination := whiteboardHref("data-model", "")
		if r.URL.RawQuery != "" {
			destination += "?" + r.URL.RawQuery
		}
		if r.Header.Get("HX-Boosted") == "true" {
			w.Header().Set("HX-Location", destination)
			w.WriteHeader(http.StatusNoContent)
		} else {
			http.Redirect(w, r, destination, http.StatusFound)
		}
		return
	}
	a.page(w, r)
}

// Discovery reads document identities and titles, never renders all canvases or
// reclassifies arbitrary diagrams as inventory definitions.
func (a *app) whiteboardDocuments(sagaID string) ([]whiteboardDocumentView, error) {
	ids, err := requirements.ListWhiteboardIDs(a.root)
	if err != nil {
		return nil, err
	}
	var documents []whiteboardDocumentView
	for _, id := range ids {
		row := whiteboardDocumentView{ID: id, Name: id, Href: whiteboardHref(id, "")}
		revision, err := requirements.LoadWhiteboard(a.root, sagaID, id, "")
		if err != nil {
			row.Note = "This whiteboard could not be opened."
		} else if revision != nil {
			row.Name = revision.Name
		}
		documents = append(documents, row)
	}
	return documents, nil
}

func (files *sagaFiles) whiteboardDocuments(build func() ([]whiteboardDocumentView, error)) ([]whiteboardDocumentView, error) {
	files.whiteboardDocumentsOnce.Do(func() { files.whiteboardDocumentViews, files.whiteboardDocumentsErr = build() })
	return files.whiteboardDocumentViews, files.whiteboardDocumentsErr
}

func (a *app) whiteboardHistoryView(sagaID, id, shown string) []whiteboardRevisionView {
	ids, err := requirements.ListWhiteboardRevisionIDs(a.root, sagaID, id)
	if err != nil {
		return nil
	}
	history := make([]whiteboardRevisionView, 0, len(ids))
	for _, revision := range ids {
		history = append(history, whiteboardRevisionView{ID: revision, Href: whiteboardHref(id, revision), Shown: revision == shown})
	}
	return history
}

func whiteboardFocusHref(href, path string) string {
	destination, _ := url.Parse(href)
	query := destination.Query()
	query.Set("focus", path)
	destination.RawQuery = query.Encode()
	return destination.String()
}
