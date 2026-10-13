package server

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/twentyideas/changesaga/internal/coderef"
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

// ERD scope evidence preserves owners even when several members cite the same
// saved bytes. Notes belong to those owners, not to a synthesized Item.
type diagramEvidenceOwner struct {
	erdEvidenceMember
	Note string
}

type diagramEvidenceGroup struct {
	Owners    []diagramEvidenceOwner
	Code      []*termCodeView
	reference coderef.Reference
}

type diagramERDScope struct {
	Groups      []diagramEvidenceGroup
	WithoutCode []erdEvidenceMember
}

type diagramAnnotationCodeView struct {
	targetCodeView
	Scope *diagramERDScope
}

// Collection and deduplication use saved metadata only. A distinct reference
// means one location and digest; every distinct owner and note is retained.
func diagramERDEvidence(inventory requirements.Inventory, revision *requirements.TechnicalRevision) *diagramERDScope {
	scope := &diagramERDScope{}
	groups := map[string]int{}
	type ownerKey struct {
		Pin                saga.DocumentationLink
		Relationship, Note string
	}
	owners := map[string]map[ownerKey]bool{}
	without := map[ownerKey]bool{}
	for _, member := range erdEvidenceMembers(inventory, revision).Members {
		if len(member.references) == 0 {
			key := ownerKey{Pin: member.Pin, Relationship: member.Relationship}
			if !without[key] {
				scope.WithoutCode = append(scope.WithoutCode, member)
				without[key] = true
			}
		}
		for _, ref := range member.references {
			key := ref.Key()
			index, exists := groups[key]
			if !exists {
				index = len(scope.Groups)
				groups[key] = index
				original := ref
				original.Note = ""
				scope.Groups = append(scope.Groups, diagramEvidenceGroup{reference: original})
				owners[key] = map[ownerKey]bool{}
			}
			owner := ownerKey{Pin: member.Pin, Relationship: member.Relationship, Note: ref.Note}
			if !owners[key][owner] {
				scope.Groups[index].Owners = append(scope.Groups[index].Owners, diagramEvidenceOwner{erdEvidenceMember: member, Note: ref.Note})
				owners[key][owner] = true
			}
		}
	}
	return scope
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
	var scope *diagramERDScope
	count := len(evidence)
	if relationship == "" && inventory.Find(pin.Target).Kind == requirements.KindERD {
		scope = diagramERDEvidence(inventory, revision)
		count = len(scope.Groups)
	}
	id := domID(pin.Revision + ":relationship:" + relationship)
	if r.URL.Query().Get("panel") == "code" {
		view := diagramAnnotationCodeView{targetCodeView: targetCodeView{DOMID: id, Target: pin.Target, Title: label, ReferenceCount: count}, Scope: scope}
		if scope == nil {
			view.Observed = a.pinnedDocumentationCode(r.Context(), evidence, "diagram element")
		} else if count > 0 {
			refs := make([]requirements.Evidence, count)
			for i, group := range scope.Groups {
				refs[i].Reference = group.reference
			}
			code := a.pinnedDocumentationCode(r.Context(), refs, "data model member")
			for i := range scope.Groups {
				scope.Groups[i].Code = code[i : i+1]
			}
		}
		writeIncrementalHeaders(w, "text/html; charset=utf-8")
		renderHTML(w, a.template, "diagram-annotation-code", view, "Linked code could not be rendered.")
		return
	}
	query := url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "panel": {"code"}}
	if relationship != "" {
		query.Set("relationship", relationship)
	}
	view := annotationAffordanceView{Documentation: documentationControl(&pin, "", 0), Label: label, CodeID: id, CodeCount: count}
	view.Documentation.Relationship = relationship
	if scope != nil {
		view.Label = "ERD scope: " + label
	}
	if count > 0 {
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
{{define "diagram-annotation-code"}}<span data-target-code-response data-target-code-target="{{.Target}}" data-target-code-count="{{.ReferenceCount}}">{{template "observed-code-open-button" .}}<template id="diffs-{{.DOMID}}"><div class="attached-code observed-code" data-attached-title="{{.Title}}"><header class="attached-code-summary">{{template "icon" "implementation"}}<span>{{.ReferenceCount}} {{if .Scope}}distinct saved code references from this ERD scope{{else}}linked code references at their saved locations{{end}}</span></header>{{if .Scope}}{{template "diagram-erd-scope-code" .Scope}}{{else}}{{template "documentation-code" .Observed}}{{end}}</div></template></span>{{end}}
{{define "diagram-evidence-owner"}}<button type="button" data-documentation-target="{{.Pin.Target}}" data-documentation-revision="{{.Pin.Revision}}"{{if .Relationship}} data-source-relationship="{{.Relationship}}"{{end}}>{{if .Relationship}}Relationship: {{end}}{{.Name}}</button> <code>{{.Pin.Revision}}</code> · definition pin {{.PinStatus}}{{end}}
{{define "diagram-erd-scope-code"}}<section class="documentation-explanation" data-diagram-erd-scope><p>Broader pinned ERD scope: these references belong to the exact directory members and their recorded relationships listed below. The scope can include more than the depicted concept; it is not code owned by this diagram element or the ERD revision. No latest revisions or relationship destinations were substituted.</p>{{range .Groups}}<section data-diagram-evidence-group><h3>Recorded evidence owners</h3><ul>{{range .Owners}}<li data-evidence-member="{{.Pin.Revision}}"{{if .Relationship}} data-evidence-relationship="{{.Relationship}}"{{end}}>{{template "diagram-evidence-owner" .}}{{if .Note}}<p>{{.Note}}</p>{{end}}</li>{{end}}</ul>{{template "documentation-code" .Code}}</section>{{end}}{{with .WithoutCode}}<h3>Members without linked code</h3><ul>{{range .}}<li data-evidence-member="{{.Pin.Revision}}" data-evidence-pin-status="{{.PinStatus}}"{{if .Relationship}} data-evidence-relationship="{{.Relationship}}"{{end}}>{{template "diagram-evidence-owner" .}} — {{if eq .PinStatus "missing"}}The saved member revision is unavailable. Nothing was substituted.{{else}}No code references are recorded for this member.{{end}}</li>{{end}}</ul>{{end}}</section>{{end}}
`
