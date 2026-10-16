package server

import (
	"net/http"
	"net/url"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type whiteboardDocumentView struct{ ID, Name, Href, Note, Role string }
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
	metadata := a.whiteboardMetadata()
	if r.PathValue("board") == "data-model" && (r.URL.Query().Get("revision") != "" || metadata.Roles["erd"] == "data-model") {
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
	metadata, err := requirements.LoadWhiteboardMetadata(a.root, sagaID)
	if err != nil {
		return nil, err
	}
	var documents []whiteboardDocumentView
	for _, id := range ids {
		if metadata.Boards[id].Archived {
			continue
		}
		row := whiteboardDocumentView{ID: id, Name: id, Role: metadata.Role(id), Href: whiteboardMetadataHref(metadata, id, "")}
		revision, err := requirements.LoadWhiteboard(a.root, sagaID, id, "")
		if err != nil {
			row.Note = "This whiteboard could not be opened."
		} else if revision != nil {
			row.Name = revision.Name
		}
		if metadata.Boards[id].Name != "" {
			row.Name = metadata.Boards[id].Name
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
	metadata, _ := requirements.LoadWhiteboardMetadata(a.root, sagaID)
	history := make([]whiteboardRevisionView, 0, len(ids))
	for _, revision := range ids {
		history = append(history, whiteboardRevisionView{ID: revision, Href: whiteboardMetadataHref(metadata, id, revision), Shown: revision == shown})
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

// Named document URLs retain identity after assignment changes. The historical
// ERD alias continues to address the original data-model document.
func whiteboardMetadataHref(metadata requirements.WhiteboardMetadata, id, revision string) string {
	if id != "data-model" || revision != "" || metadata.Roles["erd"] == id {
		return whiteboardHref(id, revision)
	}
	href := technicalPath + "/whiteboards/" + url.PathEscape(id)
	return href
}

func (a *app) whiteboardMetadata() requirements.WhiteboardMetadata {
	manifest, err := saga.ReadManifest(a.root)
	if err != nil {
		return requirements.WhiteboardMetadata{}
	}
	metadata, _ := requirements.LoadWhiteboardMetadata(a.root, manifest.ID)
	return metadata
}

// Only the three technical roles occupy top-level navigation rows. Unassigned
// documents remain discoverable on the overview and directly addressable.
func applyWhiteboardRoles(view *technicalPageView, rows []*navNodeView, documents []whiteboardDocumentView, metadata requirements.WhiteboardMetadata) {
	for i, design := range technicalDesignBoards {
		role := design.ID
		if role == "data-model" {
			role = "erd"
		}
		row := rows[i]
		assigned := metadata.Roles[role]
		if assigned == "" && metadata.Stored {
			row.Href = technicalPath + "?role=" + role
			row.Children = nil
		}
		for _, document := range documents {
			if document.ID != assigned {
				continue
			}
			if row.Href != document.Href {
				row.Children = nil
			}
			row.Href, row.Note = document.Href, document.Note
			if view != nil {
				selected := view.Whiteboard
				if view.DataModel != nil && view.DataModel.Whiteboard != nil {
					selected = view.DataModel.Whiteboard
				}
				if selected != nil && selected.ID == assigned {
					row.Href = selected.Href
					row.Children = nil
					for _, section := range selected.Sections {
						row.Children = append(row.Children, &navNodeView{Title: section.Title, Href: whiteboardFocusHref(selected.Href, section.Path), NodeID: row.NodeID + "-" + domID(section.Path), Icon: "split"})
					}
				}
			}
		}
		if view != nil {
			for j := range view.Areas {
				if view.Areas[j].Segment == design.ID {
					view.Areas[j].Href = row.Href
					if assigned != "" {
						view.Areas[j].Count = "Whiteboard"
						view.Areas[j].Gap = row.Note != ""
						if row.Note != "" {
							view.Areas[j].Note = row.Note
						}
					}
				}
			}
		}
	}
	if view != nil {
		for _, document := range documents {
			if document.Role == "" {
				view.Whiteboards = append(view.Whiteboards, document)
			}
		}
	}
}

func expandTechnicalNav(nodes []*navNodeView) bool {
	found := false
	for _, node := range nodes {
		if node.NodeID == "nav-technical" || expandTechnicalNav(node.Children) {
			node.Expanded = true
			found = true
		}
	}
	return found
}
