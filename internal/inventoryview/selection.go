package inventoryview

import (
	"context"
	"errors"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Resolver views a code reference at a commit; *coderesolve.Resolver and the
// coverage package's Resolver interface satisfy it.
type Resolver interface {
	Resolve(ctx context.Context, reference coderef.Reference, view string) coderesolve.Resolution
}

// SelectionResolver also authors the original bytes of a reference, so the
// saved selected-byte digest can be verified. *coderesolve.Resolver fits.
type SelectionResolver interface {
	Resolver
	Author(ctx context.Context, location coderef.Location, note string) (coderef.Reference, error)
}

// Structural reason codes come from requirements.ResolveSelection and
// requirements.VerifySelectedBytes. Pin and byte health codes are ours:
// conflicted pins leave a selection unresolved; the others keep it resolved
// but ineligible (or, for outside_subset_changed, flag semantic review only).
const (
	ReasonConflictedPin    = "conflicted_pin"
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
	Pin    Pin    `json:"pin"`
	Status string `json:"status"`
	Kind   string `json:"kind,omitempty"`
}

// SelectionResult keeps structural resolution, pin health, selected-byte
// health and containing-evidence health as separate facts. Current selected
// bytes never establish that the containing entity is healthy or accurate.
type SelectionResult struct {
	Item             string                  `json:"item,omitempty"`
	Selection        saga.ItemSelection      `json:"selection"`
	State            string                  `json:"state"` // resolved | unresolved
	View             string                  `json:"view"`
	Hops             []Hop                   `json:"hops"`
	EvidenceOwner    string                  `json:"evidence_owner,omitempty"`
	Containing       *coderef.Reference      `json:"containing,omitempty"`
	ContainingHealth *coderesolve.Resolution `json:"containing_health,omitempty"`
	SelectedHealth   *coderesolve.Resolution `json:"selected_health,omitempty"`
	PinsCurrent      bool                    `json:"pins_current"`
	// Eligible: resolved, every pin current, and selected bytes current at View.
	Eligible bool     `json:"eligible"`
	Reasons  []Reason `json:"reasons"`
}

// ResolveSelection delegates structure (declared hops at saved revisions,
// evidence ID, containment) to requirements.ResolveSelection, verifies the
// selected digest at its own commit, then reports pin health and views the
// selected bytes and containing evidence at view separately. It never
// substitutes a newer revision, widens a range or traverses all members.
// Without a resolver or a full view commit only structure and pins are known.
func ResolveSelection(ctx context.Context, inventory *requirements.Inventory, documentation Pin, sel saga.ItemSelection, view string, resolver SelectionResolver) SelectionResult {
	result := SelectionResult{Selection: sel, State: "unresolved", View: view, Hops: []Hop{}, Reasons: []Reason{}}
	add := func(code string, hop int, detail string) {
		result.Reasons = append(result.Reasons, Reason{code, hop, detail})
	}
	if inventory == nil {
		add(string(requirements.SelectionInvalid), -1, "no inventory")
		return result
	}
	blocked := false
	pinsCurrent := true
	for i, pin := range sel.Path {
		hop := Hop{Pin: pin, Status: inventory.LinkStatus(pin)}
		if r := inventory.Find(pin.Target); r != nil {
			hop.Kind = r.Kind
		}
		switch hop.Status {
		case "conflicted":
			add(ReasonConflictedPin, i, pin.Target)
			blocked = true
		case "retired":
			add(ReasonRetiredPin, i, pin.Target)
			pinsCurrent = false
		case "stale":
			add(ReasonNoncurrentPin, i, pin.Revision)
			pinsCurrent = false
		}
		result.Hops = append(result.Hops, hop)
	}
	result.PinsCurrent = pinsCurrent
	structural, err := inventory.ResolveSelection(documentation, sel)
	if err != nil {
		var selErr *requirements.SelectionError
		if errors.As(err, &selErr) {
			add(string(selErr.Code), selErr.Hop, selErr.Message)
		} else {
			add(string(requirements.SelectionInvalid), -1, err.Error())
		}
		return result
	}
	containing := structural.Evidence.Reference
	result.Containing, result.EvidenceOwner = &containing, structural.OwnerEdge
	if blocked {
		return result
	}
	if resolver == nil || !coderef.ValidCommit(view) {
		result.State = "resolved"
		return result
	}
	if err := requirements.VerifySelectedBytes(ctx, resolver.Author, sel); err != nil {
		var selErr *requirements.SelectionError
		if errors.As(err, &selErr) {
			add(string(selErr.Code), selErr.Hop, selErr.Message)
		}
		return result
	}
	result.State = "resolved"
	containingHealth := resolver.Resolve(ctx, containing, view)
	selectedHealth := resolver.Resolve(ctx, sel.Code, view)
	result.ContainingHealth, result.SelectedHealth = &containingHealth, &selectedHealth
	if !selectedHealth.Current() {
		add(ReasonSelectedStale, -1, selectedHealth.Reason)
	}
	if !containingHealth.Current() {
		if selectedHealth.Current() {
			// The selected bytes survived; the change lies outside the subset
			// and still requires semantic reassessment of the entity.
			add(ReasonSemanticReassess, -1, containingHealth.Reason)
		} else {
			add(ReasonContainingStale, -1, containingHealth.Reason)
		}
	}
	result.Eligible = pinsCurrent && selectedHealth.Current()
	return result
}
