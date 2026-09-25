package inventoryview

import (
	"context"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// Resolver views a code reference at a commit; *coderesolve.Resolver and the
// coverage package's Resolver interface satisfy it.
type Resolver interface {
	Resolve(ctx context.Context, reference coderef.Reference, view string) coderesolve.Resolution
}

const MaxPathHops = 8

// Selection names an exact subset of one evidence reference reached through
// declared pins. Path[0] is the referencing Item's own documentation pin and
// the final pin owns the evidence. EvidenceOwner is empty for the entity's own
// code or names a System interaction (later: an owned relationship).
type Selection struct {
	ID            string            `json:"id"`
	Path          []Pin             `json:"path"`
	EvidenceOwner string            `json:"evidence_owner,omitempty"`
	Evidence      string            `json:"evidence"`
	Selected      coderef.Reference `json:"selected"`
}

// Selection reason codes. Blocking reasons leave the selection unresolved;
// health reasons keep it resolved but ineligible for current coverage.
const (
	ReasonInvalidSelection   = "invalid_selection"
	ReasonPathTooLong        = "path_too_long"
	ReasonMissingPin         = "missing_pin"
	ReasonConflictedPin      = "conflicted_pin"
	ReasonUndeclaredHop      = "undeclared_hop"
	ReasonMissingOwner       = "missing_evidence_owner"
	ReasonMissingEvidence    = "missing_evidence"
	ReasonWholeFileSelection = "whole_file_selection"
	ReasonOutsideEvidence    = "outside_evidence"
	ReasonDigestMismatch     = "selected_digest_mismatch"

	ReasonRetiredPin       = "retired_pin"
	ReasonNoncurrentPin    = "noncurrent_pin"
	ReasonSelectedStale    = "selected_bytes_stale"
	ReasonContainingStale  = "containing_evidence_stale"
	ReasonSemanticReassess = "outside_subset_changed"
)

type Reason struct {
	Code   string `json:"code"`
	Hop    int    `json:"hop"`
	Detail string `json:"detail,omitempty"`
}

type Hop struct {
	Pin      Pin    `json:"pin"`
	Status   string `json:"status"`
	Declared bool   `json:"declared"`
}

// SelectionResult keeps structural resolution, pin health, selected-byte
// health and containing-evidence health as separate facts. Current selected
// bytes never establish that the containing entity is healthy or accurate.
type SelectionResult struct {
	Selection        Selection               `json:"selection"`
	State            string                  `json:"state"` // resolved | unresolved
	View             string                  `json:"view"`
	Hops             []Hop                   `json:"hops"`
	Containing       *coderef.Reference      `json:"containing,omitempty"`
	ContainingHealth *coderesolve.Resolution `json:"containing_health,omitempty"`
	SelectedHealth   *coderesolve.Resolution `json:"selected_health,omitempty"`
	PinsCurrent      bool                    `json:"pins_current"`
	// Eligible: resolved, every pin current, and selected bytes current at View.
	Eligible bool     `json:"eligible"`
	Reasons  []Reason `json:"reasons"`
}

// EvidenceID is the read-only identity of a legacy reference that has no
// persisted ID: its exact pinned location, unique within one owner revision.
func EvidenceID(ref coderef.Reference) string { return ref.Location().String() }

// ResolveSelection validates every adjacent hop at its saved revision and the
// subset against its containing reference, then views both at view. It never
// substitutes a newer revision, expands a range or traverses all members.
func ResolveSelection(ctx context.Context, inventory *requirements.Inventory, sel Selection, view string, resolver Resolver) SelectionResult {
	result := SelectionResult{Selection: sel, State: "unresolved", View: view, Hops: []Hop{}, Reasons: []Reason{}}
	block := func(code string, hop int, detail string) {
		result.Reasons = append(result.Reasons, Reason{code, hop, detail})
	}
	if len(sel.Path) == 0 || strings.TrimSpace(sel.Evidence) == "" || coderef.Validate(sel.Selected) != nil || inventory == nil {
		block(ReasonInvalidSelection, -1, "selection requires a path, evidence identity and a valid exact reference")
		return result
	}
	if len(sel.Path) > MaxPathHops {
		block(ReasonPathTooLong, -1, "")
		return result
	}
	blocked := false
	pinsCurrent := true
	var last *requirements.TechnicalRevision
	for i, pin := range sel.Path {
		hop := Hop{Pin: pin, Status: inventory.LinkStatus(pin), Declared: i == 0}
		record := inventory.Find(pin.Target)
		var rev *requirements.TechnicalRevision
		if record != nil {
			rev = record.Revision(pin.Revision)
		}
		if i > 0 && last != nil {
			for _, member := range last.Components {
				if member == pin {
					hop.Declared = true
				}
			}
		}
		switch hop.Status {
		case "missing":
			block(ReasonMissingPin, i, pin.Revision)
			blocked = true
		case "conflicted":
			block(ReasonConflictedPin, i, pin.Target)
			blocked = true
		case "retired":
			block(ReasonRetiredPin, i, pin.Target)
			pinsCurrent = false
		case "stale":
			block(ReasonNoncurrentPin, i, pin.Revision)
			pinsCurrent = false
		}
		if !hop.Declared {
			block(ReasonUndeclaredHop, i, "not declared by "+sel.Path[i-1].Revision)
			blocked = true
		}
		result.Hops = append(result.Hops, hop)
		last = rev
	}
	result.PinsCurrent = pinsCurrent
	if blocked || last == nil {
		return result
	}
	refs := last.Code
	if sel.EvidenceOwner != "" {
		refs = nil
		found := false
		for _, edge := range last.Interactions {
			if edge.ID == sel.EvidenceOwner {
				refs, found = edge.Code, true
			}
		}
		if !found {
			block(ReasonMissingOwner, len(sel.Path)-1, sel.EvidenceOwner)
			return result
		}
	}
	for i := range refs {
		if EvidenceID(refs[i]) == sel.Evidence {
			ref := refs[i]
			result.Containing = &ref
		}
	}
	if result.Containing == nil {
		block(ReasonMissingEvidence, len(sel.Path)-1, sel.Evidence)
		return result
	}
	if sel.Selected.WholeFile() {
		block(ReasonWholeFileSelection, -1, "")
		return result
	}
	if !result.Containing.Location().Contains(sel.Selected.Location()) {
		block(ReasonOutsideEvidence, -1, sel.Selected.Location().String()+" is not within "+result.Containing.Location().String())
		return result
	}
	if resolver == nil || !coderef.ValidCommit(view) {
		result.State = "resolved"
		return result
	}
	// The selected digest must match the pinned bytes themselves; a subset
	// digest that was never true is invalid, not drift.
	if pinned := resolver.Resolve(ctx, sel.Selected, sel.Selected.Commit); !pinned.Current() {
		block(ReasonDigestMismatch, -1, pinned.Reason)
		return result
	}
	result.State = "resolved"
	containing := resolver.Resolve(ctx, *result.Containing, view)
	selected := resolver.Resolve(ctx, sel.Selected, view)
	result.ContainingHealth, result.SelectedHealth = &containing, &selected
	if !selected.Current() {
		block(ReasonSelectedStale, -1, selected.Reason)
	}
	if !containing.Current() {
		detail := containing.Reason
		if selected.Current() {
			// The selected bytes survived; the change lies outside the subset
			// and still requires semantic reassessment of the entity.
			block(ReasonSemanticReassess, -1, detail)
		} else {
			block(ReasonContainingStale, -1, detail)
		}
	}
	result.Eligible = pinsCurrent && selected.Current()
	return result
}
