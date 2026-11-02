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
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type whiteboardStaleElement struct{ Path, Label string }

// Enhance only this board's own elements. Imported ERD sources keep their exact
// independent ownership and existing actions. These attributes are reader state,
// never persisted geometry or a new revision of the authored SVG.
func (a *app) enhanceWhiteboardElements(inventory requirements.Inventory, boardID string, resolved *requirements.ResolvedWhiteboard, visual template.HTML) (template.HTML, []whiteboardStaleElement) {

	hasMetadata, hasEvidence := false, false
	for _, e := range resolved.Revision.Document.Elements {
		hasMetadata = hasMetadata || len(e.Selections) > 0 || len(e.Evidence) > 0 || len(e.CriterionLinks) > 0 || e.Record != "" || e.Documentation != nil
		hasEvidence = hasEvidence || len(e.Evidence) > 0 || len(e.Selections) > 0
	}
	if !hasMetadata {
		return visual, nil
	}
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

	var resolver *coderesolve.Resolver
	head := ""
	if hasEvidence {
		resolver, _ = coderesolve.New(ctx, a.sourceDir)
		if resolver != nil {
			defer resolver.Close()
		}
		head, _ = resolveCommit(ctx, a.sourceDir, firstNonEmptyString(a.rng.Head, "HEAD"))
	}

	var stale []whiteboardStaleElement
	tree.walk(func(node *presentationSVG) {
		path := node.attr("data-instance-path")
		e, ok := elements[path]
		if !ok || e.Decorative || (len(e.Evidence) == 0 && len(e.CriterionLinks) == 0 && e.Record == "" && e.Documentation == nil && len(e.Selections) == 0) {
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
		for _, selection := range e.Selections {
			if e.Documentation == nil {
				isStale = true
				continue
			}
			var selectionResolver inventoryview.SelectionResolver
			if resolver != nil {
				selectionResolver = resolver
			}
			result := inventoryview.ResolveSelection(ctx, &inventory, *e.Documentation, selection, head, selectionResolver)
			if !result.Eligible || len(result.Reasons) > 0 {
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
	appearance := "urn:change-saga:" + document.Manifest.ID + ":whiteboard:" + boardID + ":revision:" + revisionID + ":element:" + elementID
	count := len(e.Evidence) + len(e.Selections)
	id := domID(item.Target + ":revision:" + revision.ID)
	if q.Get("panel") == "code" {
		refs := make([]requirements.Evidence, len(e.Evidence))
		for i, ref := range e.Evidence {
			refs[i].Reference = ref
		}
		view := diagramAnnotationCodeView{targetCodeView: targetCodeView{DOMID: id, Target: item.Target, Title: item.Label, ReferenceCount: count, Observed: a.pinnedDocumentationCode(r.Context(), refs, "whiteboard element")}}
		if len(e.Selections) > 0 && e.Documentation != nil {
			inventory, err := a.sagaFiles(r.Context()).inventory(document.Manifest.ID)
			if err != nil {
				http.Error(w, "The inventory could not be loaded.", 500)
				return
			}
			view.Selections = a.itemSelections(r.Context(), inventory, *e.Documentation, appearance)
		}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "diagram-annotation-code", view, "Linked code could not be rendered.")
		return
	}
	records, err := a.sagaFiles(r.Context()).projectedRecords(document.Manifest.ID)
	if err != nil {
		http.Error(w, "Story links could not be loaded.", 500)
		return
	}
	pin := e.Documentation
	if pin == nil && e.Source != nil {
		pin = &saga.DocumentationLink{Target: e.Source.Target, Revision: e.Source.Revision}
	}
	view := annotationAffordanceView{Label: item.Label, CodeID: id, CodeCount: count, Documentation: documentationControl(pin, appearance, len(e.Selections)), Stories: whiteboardElementStories(records, *item, id)}
	if view.Documentation != nil && e.Source != nil && e.Documentation == nil {
		view.Documentation.Relationship = e.Source.Relationship
	}
	if count > 0 {
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
	evaluator := requirements.NewRelationEvaluator(records)
	for _, pin := range item.CriterionLinks {
		parts := strings.Split(pin.Criterion, ":")
		if len(parts) != 7 {
			continue
		}
		storyID := parts[4]
		storyURN, _ := livingid.Story(records.SagaID, storyID)
		currency := evaluator.Evaluate(requirements.Relation{Version: requirements.V5RelationVersion, ID: pin.ID, Type: requirements.RelationExplains, From: item.Target, To: pin.Criterion, ToRevision: pin.StoryRevision, Scope: requirements.ScopeSelf, State: requirements.RelationActive}, requirements.StaleInputs{})
		row := storyLinkView{Story: storyID, StoryTarget: storyURN, StoryHref: requirementStoryHref(storyID), Criterion: parts[6], CriterionHref: requirementCriterionHref(storyID, parts[6]), Item: item.Label, Rationale: pin.Rationale, Status: currency.Status}
		if story := records.FindStory(storyID); story != nil && story.CurrentRevision != nil {
			row.Story = story.CurrentRevision.Title
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				if criterion.ID == parts[6] {
					row.Criterion = criterion.Statement
				}
			}
		}
		view.Links = append(view.Links, row)
	}
	view.summarize()
	return view
}
