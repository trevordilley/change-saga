package server

import (
	"context"
	"encoding/xml"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type whiteboardStaleElement struct{ Path, Label string }

// Enhance only this board's own elements. Imported ERD sources keep their exact
// independent ownership and existing actions. These attributes are reader state,
// never persisted geometry or a new revision of the authored SVG.
func (a *app) enhanceWhiteboardElements(sagaID, boardID string, resolved *requirements.ResolvedWhiteboard, visual template.HTML) (template.HTML, []whiteboardStaleElement) {
	tree, err := readPresentationSVG(string(visual))
	if err != nil {
		return visual, nil
	}
	description, err := resolved.Composition.Describe("")
	if err != nil {
		return visual, nil
	}
	elements := map[string]diagram.Element{}
	for _, item := range append(description.Elements, description.Connections...) {
		if resolved.Revision.Document.Index(item.Element.ID) >= 0 && item.Path == boardElementPath(resolved.Revision.Document, item.Element.ID) {
			elements[item.Path] = item.Element
		}
	}
	ctx := context.Background()
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err == nil {
		defer resolver.Close()
	}
	head, _ := resolveCommit(ctx, a.sourceDir, firstNonEmptyString(a.rng.Head, "HEAD"))
	var stale []whiteboardStaleElement
	tree.walk(func(node *presentationSVG) {
		path := node.attr("data-instance-path")
		e, ok := elements[path]
		if !ok || e.Decorative || (len(e.Evidence) == 0 && len(e.CriterionLinks) == 0 && e.Record == "" && e.Documentation == nil) {
			return
		}
		node.set("data-whiteboard-element", e.ID)
		node.set("data-whiteboard-board", boardID)
		node.set("data-whiteboard-revision", resolved.Revision.ID)
		node.set("tabindex", "0")
		isStale := false
		for _, ref := range e.Evidence {
			if resolver == nil || !resolver.Resolve(ctx, ref, head).Current() {
				isStale = true
			}
		}
		if !isStale {
			return
		}
		stale = append(stale, whiteboardStaleElement{Path: path, Label: diagram.AccessibleName(e)})
		node.set("data-evidence-state", "stale")
		node.set("aria-label", diagram.AccessibleName(e)+"; stale code reference — reassessment required")
		badge := &presentationSVG{name: "text", attrs: []xml.Attr{{Name: xml.Name{Local: "x"}, Value: "8"}, {Name: xml.Name{Local: "y"}, Value: "-8"}, {Name: xml.Name{Local: "data-evidence-badge"}, Value: "stale"}}, children: []*presentationSVG{{text: "Stale"}}}
		node.children = append(node.children, badge)
	})
	return template.HTML(tree.markup()), stale
}

func boardElementPath(doc diagram.Document, id string) string {
	e, ok := doc.Element(id)
	if !ok {
		return ""
	}
	path := e.ID
	for parent := e.Parent; parent != ""; {
		p, ok := doc.Element(parent)
		if !ok {
			break
		}
		path = p.ID + "/" + path
		parent = p.Parent
	}
	return path
}

// The revision is mandatory: actions from an old page cannot accidentally open
// the current element's replacement evidence. Code rendering stays lazy and uses
// the same pinned-code renderer and drawer protocol as diagram/slide controls.
func (a *app) whiteboardElement(w http.ResponseWriter, r *http.Request) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The Saga could not be loaded.", 500)
		return
	}
	q := r.URL.Query()
	boardID, revisionID, elementID := q.Get("board"), q.Get("revision"), q.Get("element")
	if !livingid.ValidID(boardID) || !livingid.ValidID(revisionID) || !saga.ValidMarkdownAnchor(elementID) {
		http.Error(w, "An exact whiteboard revision and element are required.", 400)
		return
	}
	revision, err := requirements.LoadWhiteboard(a.root, document.Manifest.ID, boardID, revisionID)
	if err != nil || revision == nil {
		http.Error(w, "The saved whiteboard revision is unavailable. Nothing was substituted.", 404)
		return
	}
	e, ok := revision.Document.Element(elementID)
	if !ok || e.Decorative {
		http.Error(w, "The saved element is unavailable.", 404)
		return
	}
	item := requirements.WhiteboardItem(document.Manifest.ID, boardID, *revision, e)
	id := domID(item.Target + ":revision:" + revision.ID)
	if q.Get("panel") == "code" {
		refs := make([]requirements.Evidence, len(e.Evidence))
		for i, ref := range e.Evidence {
			refs[i].Reference = ref
		}
		view := diagramAnnotationCodeView{targetCodeView: targetCodeView{DOMID: id, Target: item.Target, Title: item.Label, ReferenceCount: len(refs), Observed: a.pinnedDocumentationCode(r.Context(), refs, "whiteboard element")}}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "diagram-annotation-code", view, "Linked code could not be rendered.")
		return
	}
	records, err := a.sagaFiles(r.Context()).projectedRecords(document.Manifest.ID)
	if err != nil {
		http.Error(w, "Story links could not be loaded.", 500)
		return
	}
	view := annotationAffordanceView{Label: item.Label, CodeID: id, CodeCount: len(e.Evidence), Documentation: documentationControl(e.Documentation, "", 0), Stories: whiteboardElementStories(records, *item, id)}
	if len(e.Evidence) > 0 {
		query := url.Values{"board": {boardID}, "revision": {revisionID}, "element": {elementID}, "panel": {"code"}}
		view.CodeHref = "/api/whiteboard-element?" + query.Encode()
	}
	if e.Record != "" {
		link := newAppGraph(document, records, quality.Document{}).link(e.Record)
		view.RecordLink = &link
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "annotation-affordance", view, "Element actions could not be rendered.")
}

func whiteboardElementStories(records requirements.Document, item saga.Item, id string) *storyLinksView {
	view := &storyLinksView{DOMID: id, Title: item.Label}
	for _, pin := range item.CriterionLinks {
		parts := strings.Split(pin.Criterion, ":")
		if len(parts) != 7 {
			continue
		}
		storyID := parts[4]
		storyURN, _ := livingid.Story(records.SagaID, storyID)
		row := storyLinkView{Story: storyID, StoryTarget: storyURN, StoryHref: requirementStoryHref(storyID), Criterion: parts[6], CriterionHref: requirementCriterionHref(storyID, parts[6]), Item: item.Label, Rationale: pin.Rationale, Status: requirements.CurrencyStale}
		if story := records.FindStory(storyID); story != nil && story.CurrentRevision != nil {
			row.Story = story.CurrentRevision.Title
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				if criterion.ID == parts[6] {
					row.Criterion = criterion.Statement
					if storyURN+":revision:"+story.CurrentRevision.ID == pin.StoryRevision {
						row.Status = requirements.CurrencyCurrent
					}
				}
			}
		}
		view.Links = append(view.Links, row)
	}
	view.summarize()
	return view
}
