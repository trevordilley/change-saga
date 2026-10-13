package server

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// annotationAffordanceView is presentation, not a slide Item. Both slides and
// native diagram instances use the same action strip and delegated controls.
type annotationAffordanceView struct {
	DOMID                   string
	Surprise                bool
	Documentation           *documentationControlView
	Stories                 *storyLinksView
	Code                    *landmarkView
	CodeHref, CodeID, Label string
	CodeCount               int
}

func (v *landmarkView) Affordance() annotationAffordanceView {
	result := annotationAffordanceView{DOMID: v.DOMID, Stories: v.Stories, Code: v}
	if v.ItemMeta != nil {
		result.Surprise = v.ItemMeta.Kind == "callout"
		result.Documentation = documentationControl(v.ItemMeta.Documentation, v.Target, len(v.ItemMeta.Selections))
	}
	return result
}

// diagramAnnotation reads only metadata until the reader opens linked code.
// It accepts existing saved pins, never labels or guessed slide Item identities.
func (a *app) diagramAnnotation(w http.ResponseWriter, r *http.Request) {
	files := a.sagaFiles(r.Context())
	document := files.narrative()
	if document == nil {
		http.Error(w, "The saga could not be loaded.", 500)
		return
	}
	pin := saga.DocumentationLink{Target: r.URL.Query().Get("target"), Revision: r.URL.Query().Get("revision")}
	if !saga.ValidDocumentationLink(document.Manifest.ID, pin) && !saga.ValidPinOfKinds(document.Manifest.ID, pin, requirements.KindERD) {
		http.Error(w, "Invalid documentation reference", 400)
		return
	}
	inventory, err := files.inventory(document.Manifest.ID)
	if err != nil {
		http.Error(w, "The inventory could not be loaded.", 500)
		return
	}
	revision := inventory.Pinned(pin)
	if revision == nil {
		http.Error(w, "The saved definition is missing. Nothing was substituted.", 404)
		return
	}
	label, evidence := revision.Name, revision.Code
	relationship := r.URL.Query().Get("relationship")
	if relationship != "" {
		found := false
		for _, edge := range revision.Relationships {
			if edge.ID == relationship {
				label, evidence, found = edge.Label, edge.Code, true
				break
			}
		}
		if !found {
			http.Error(w, "The relationship is missing from the saved revision.", 404)
			return
		}
	}
	id := domID(pin.Revision + ":relationship:" + relationship)
	if r.URL.Query().Get("panel") == "code" {
		view := targetCodeView{DOMID: id, Target: pin.Target, Title: label, ReferenceCount: len(evidence), Observed: a.pinnedDocumentationCode(r.Context(), evidence, "diagram element")}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "diagram-annotation-code", view, "Linked code could not be rendered.")
		return
	}
	query := url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "panel": {"code"}}
	if relationship != "" {
		query.Set("relationship", relationship)
	}
	view := annotationAffordanceView{Documentation: documentationControl(&pin, "", 0), Label: label, CodeID: id, CodeCount: len(evidence)}
	view.Documentation.Relationship = relationship
	if len(evidence) > 0 {
		view.CodeHref = "/api/diagram-annotation?" + query.Encode()
	}
	// Entity-wide Item links are not relationship-specific evidence.
	if relationship == "" {
		records, err := files.projectedRecords(document.Manifest.ID)
		if err != nil {
			http.Error(w, "Recorded story links could not be loaded.", 500)
			return
		}
		view.Stories, err = diagramAnnotationStories(document, records, pin, id, label)
		if err != nil {
			http.Error(w, "Recorded story links could not be resolved.", 500)
			return
		}
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "annotation-affordance", view, "Element actions could not be rendered.")
}

// Story provenance remains the real implementation Item whose documentation
// pin exactly matches this appearance, not a relation invented for the board.
func diagramAnnotationStories(document *saga.Saga, records requirements.Document, pin saga.DocumentationLink, id, label string) (*storyLinksView, error) {
	result := &storyLinksView{DOMID: "diagram-" + id, Title: label, Provenance: "Stories linked by implementation elements that pin this exact saved definition. These links belong to those elements, not to the whiteboard."}
	decorator := &storyLinkDecorator{document: document, records: records}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			fragment := findFragmentByTarget(document, slide.Target)
			if fragment == nil {
				continue
			}
			view := &fragmentView{Fragment: fragment, DOMID: domID(fragment.Target)}
			for _, landmark := range fragment.Landmarks {
				if landmark.ItemMeta == nil || landmark.ItemMeta.Documentation == nil || *landmark.ItemMeta.Documentation != pin {
					continue
				}
				view.LandmarkViews = append(view.LandmarkViews, &landmarkView{Landmark: landmark, DOMID: view.DOMID + "--" + landmark.ID, Title: landmark.Label})
			}
			if len(view.LandmarkViews) == 0 {
				continue
			}
			if err := decorator.decorate(view); err != nil {
				return nil, fmt.Errorf("story links: %w", err)
			}
			for _, owner := range view.LandmarkViews {
				for _, link := range owner.Stories.Links {
					link.ItemHref = "/?view=slides#" + owner.DOMID
					result.Links = append(result.Links, link)
				}
			}
		}
	}
	result.summarize()
	return result, nil
}

const diagramAnnotationTemplates = `
{{define "annotation-affordance"}}<span class="landmark-affordance" data-landmark-affordance>{{if .Surprise}}<span class="callout-mark" title="Surprise">{{template "icon" "alert"}}</span>{{end}}{{if .DOMID}}{{template "permalink" .DOMID}}{{end}}{{template "documentation-control" .Documentation}}{{with .Stories}}{{if .Count}}{{template "story-control" .}}{{end}}{{end}}{{if .CodeHref}}<button type="button" class="icon-button diff-button" data-target-code-href="{{.CodeHref}}" data-target-code-template="diffs-{{.CodeID}}" aria-label="Open {{.CodeCount}} linked code references for {{.Label}}" title="Linked code">{{template "icon" "implementation"}}<span>{{.CodeCount}}</span></button>{{else}}{{with .Code}}{{template "diff-control" .}}{{end}}{{end}}</span>{{end}}
{{define "diagram-annotation-code"}}<span data-target-code-response data-target-code-target="{{.Target}}" data-target-code-count="{{.ReferenceCount}}">{{template "observed-code-open-button" .}}<template id="diffs-{{.DOMID}}"><div class="attached-code observed-code" data-attached-title="{{.Title}}"><header class="attached-code-summary">{{template "icon" "implementation"}}<span>{{.ReferenceCount}} linked code references at their saved locations</span></header>{{template "documentation-code" .Observed}}</div></template></span>{{end}}
`
