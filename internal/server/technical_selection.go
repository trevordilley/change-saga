package server

import (
	"context"
	"errors"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// An Item that pins a definition may also select the exact part of it the
// slide explains: a declared path of pinned revisions ending in one evidence
// reference, and a subset of that reference's lines. Opened from that Item,
// the drawer shows the selection before the definition's complete code, so a
// reader sees what this use is about without mistaking the rest of the
// definition's code for it.
//
// Three facts stay separate. The path either resolves against the saved
// revisions or reports the stable reason it does not; the selected lines are
// current or stale at the observed head; and the containing reference may be
// stale even when the selected lines are not. None of these is coverage or
// approval, and nothing is repinned or widened to make a selection resolve.

// documentationControlView is the book control on a slide Item.
type documentationControlView struct {
	Target, Revision string
	// Item is the Item's URN, so the drawer can show its own selections.
	Item       string
	Selections int
}

func documentationControl(pin *saga.DocumentationLink, item string, selections int) *documentationControlView {
	if pin == nil {
		return nil
	}
	return &documentationControlView{Target: pin.Target, Revision: pin.Revision, Item: item, Selections: selections}
}

type selectionHopView struct {
	Name, Kind, Href, Status string
}

type selectionView struct {
	ID       string
	Hops     []selectionHopView
	Evidence string
	// OwnerEdge names the interaction or relationship whose evidence holds
	// the selection; empty for the last hop's own code.
	OwnerEdge string
	// Problem is the stable reason the path does not resolve, with its text.
	Problem, ProblemCode string
	// Selected is the selected lines at the observed head; Containing is the
	// reference it lies within, reported by currency only.
	Selected   []*termCodeView
	Containing string
	// ContainingStale is independent of Selected's currency.
	ContainingStale bool
	// Integrity is whether the saved digest matches the selected bytes at
	// the selection's own commit.
	Integrity string
}

// itemSelectionsView is what the drawer shows for the Item it was opened from.
type itemSelectionsView struct {
	Item, Label string
	// Mismatch says the Item no longer pins the revision the drawer shows;
	// its selections are not presented against a different revision.
	Mismatch bool
	// View is the saved Saga view commit that admitted a non-current pin.
	View       string
	Selections []selectionView
}

// findItem locates one Item in an implementation or review deck.
func findItem(document *saga.Saga, target string) *saga.Item {
	var decks []*saga.Deck
	decks = append(decks, document.Decks...)
	for _, review := range document.Reviews {
		if review.Deck != nil {
			decks = append(decks, review.Deck)
		}
	}
	for _, deck := range decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if item.Target == target {
					return item
				}
			}
		}
	}
	return nil
}

// itemSelections resolves an Item's selections for the drawer. It reads the
// Item as saved; a pin that differs from the drawer's shows no selections.
func (a *app) itemSelections(ctx context.Context, inventory requirements.Inventory, pin saga.DocumentationLink, itemTarget string) *itemSelectionsView {
	document := a.narrativeDocument(ctx)
	if document == nil {
		return nil
	}
	item := findItem(document, itemTarget)
	if item == nil || item.Documentation == nil {
		return nil
	}
	view := &itemSelectionsView{Item: item.Target, Label: item.Label, View: item.DocumentationView}
	if view.Label == "" {
		view.Label = item.ID
	}
	if *item.Documentation != pin {
		view.Mismatch = true
		return view
	}
	var resolver *coderesolve.Resolver
	if len(item.Selections) > 0 {
		if r, err := coderesolve.New(ctx, a.sourceDir); err == nil {
			resolver = r
			defer resolver.Close()
		}
	}
	for _, selection := range item.Selections {
		sv := selectionView{ID: selection.ID, Evidence: selection.Evidence}
		for _, hop := range selection.Path {
			kind, _, _ := technicalTargetParts(hop.Target)
			sv.Hops = append(sv.Hops, selectionHopView{Name: entityName(inventory, hop), Kind: technicalKindTitle(kind), Href: technicalPinHref(hop), Status: inventory.LinkStatus(hop)})
		}
		resolution, err := inventory.ResolveSelection(pin, selection)
		var problem *requirements.SelectionError
		switch {
		case errors.As(err, &problem):
			sv.ProblemCode, sv.Problem = string(problem.Code), problem.Message
		case err != nil:
			sv.ProblemCode, sv.Problem = "unresolved", err.Error()
		default:
			sv.OwnerEdge = resolution.OwnerEdge
			containing := a.referenceCode(ctx, []coderef.Reference{resolution.Evidence.Reference}, "selection")
			if len(containing) > 0 {
				sv.Containing, sv.ContainingStale = containing[0].Location, containing[0].Stale
			}
		}
		// The selected lines are shown even when the path does not resolve:
		// they are the saved selection, and hiding them would hide the gap.
		sv.Selected = a.documentationCode(ctx, []coderef.Reference{selection.Code}, "selection")
		switch {
		case resolver == nil:
			sv.Integrity = "unknown: the code repository is not available"
		case requirements.VerifySelectedBytes(ctx, resolver.Author, selection) != nil:
			sv.Integrity = "differs: the saved digest does not match the selected lines at their commit"
		default:
			sv.Integrity = "matches"
		}
		view.Selections = append(view.Selections, sv)
	}
	return view
}

const technicalSelectionTemplates = `
{{define "item-selections"}}<section class="item-selections" data-item-selections="{{.Item}}"><h3>Selected for “{{.Label}}”</h3>{{if .Mismatch}}<p role="status" class="gap" data-selection-mismatch>This Item now pins a different revision, so its selections are not shown against this one.</p>{{else}}{{if .View}}<p class="term-empty" data-selection-view>This pin was admitted from the saved Saga view at <code>{{short .View}}</code>; its global status is reported above, separately.</p>{{end}}{{range .Selections}}<article class="item-selection" data-selection="{{.ID}}"{{if .ProblemCode}} data-selection-problem="{{.ProblemCode}}"{{end}}><ol class="selection-path" aria-label="Declared path">{{range .Hops}}<li><a href="{{.Href}}">{{.Name}}</a> <small class="trace-kind">{{.Kind}}</small>{{if ne .Status "current"}} <small class="gap">{{.Status}}</small>{{end}}</li>{{end}}</ol>{{if .ProblemCode}}<p role="status" class="gap">This selection does not resolve ({{.ProblemCode}}): {{.Problem}} Nothing was substituted.</p>{{else}}<p class="selection-evidence">Evidence <code>{{.Evidence}}</code>{{if .OwnerEdge}} of <code>{{.OwnerEdge}}</code>{{end}} within <code>{{.Containing}}</code>{{if .ContainingStale}} · <span class="gap" data-containing-stale>the containing reference is stale</span>{{end}}</p>{{end}}{{template "documentation-code" .Selected}}<p class="selection-integrity" data-selection-integrity="{{.Integrity}}">Selected lines at their commit: {{.Integrity}}.</p></article>{{else}}<p class="term-empty" data-selection-none>This Item links the whole definition and selects no code subset. The link alone grants no coverage.</p>{{end}}{{end}}</section>{{end}}
`
