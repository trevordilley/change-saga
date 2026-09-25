// Package technicalpolicy evaluates the first technical inventory policy slice.
// It has no persistence or coverage authority and is not connected to public
// authoring. Callers must load snapshot-consistent global and saved-view facts,
// verify saved artifact/source identity, resolve exact endpoint revisions, and
// bind the resolver to the Saga's canonical source repository before calling.
// Publication, locks, parent checks and retries remain caller responsibilities.
package technicalpolicy

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
)

// Pin identifies an exact entity revision; these are opaque caller-resolved IDs.
type Pin struct{ Target, Revision string }

func (p Pin) valid() bool {
	return strings.TrimSpace(p.Target) != "" && strings.TrimSpace(p.Revision) != ""
}

// DefinitionState distinguishes missing/unknown definitions from unique ones.
type DefinitionState string

const (
	DefinitionUnique     DefinitionState = "unique"
	DefinitionMissing    DefinitionState = "missing"
	DefinitionConflicted DefinitionState = "conflicted"
)

type Lifecycle string

const (
	Active              Lifecycle = "active"
	Retired             Lifecycle = "retired"
	LifecycleConflicted Lifecycle = "conflicted"
)

// GlobalHealth is independent of saved-view admission. Current is meaningful
// only when Definition is unique; no saved view changes these supplied facts.
type GlobalHealth struct {
	Current    Pin
	Definition DefinitionState
	Lifecycle  Lifecycle
}

// SavedView is an already resolved, immutable design/review projection, not a
// label authorizing arbitrary history. Artifact identifies the saved snapshot or
// committed artifact. Resolved attests caller loading/identity validation; this
// package cannot authenticate unprovided external facts.
type SavedView struct {
	Artifact     string
	SourceCommit string
	Resolved     bool
	Binding      Pin
	Definition   DefinitionState
	Lifecycle    Lifecycle
}

type PinRequest struct {
	Requested Pin
	Global    GlobalHealth
	// Non-nil explicitly selects a view, including a failed/missing projection.
	// There is no fallback to global currentness when that view is invalid.
	View *SavedView
}

type AdmissionBasis string

const (
	Current AdmissionBasis = "current"
	Saved   AdmissionBasis = "saved_view"
)

type PinReason string

const (
	PinInvalid            PinReason = "invalid_pin"
	PinNotCurrent         PinReason = "noncurrent_default_pin"
	ViewMissing           PinReason = "missing_view"
	ViewBindingMissing    PinReason = "missing_view_binding"
	ViewBindingMismatch   PinReason = "mismatched_view_binding"
	ViewDefinitionMissing PinReason = "missing_view_definition"
	ViewAmbiguous         PinReason = "ambiguous_view"
	ViewInactive          PinReason = "inactive_view"
	ViewUnknown           PinReason = "unknown_view_facts"
)

// Admission deliberately has no readability, coverage, or approval field.
type Admission struct {
	Admitted bool
	Reason   PinReason
	Basis    AdmissionBasis
	Global   GlobalHealth
}

// AdmitPin evaluates new-link admission only. Existing historical reads are not
// subject to this gate. Unknown facts refuse admission in the selected scope.
func AdmitPin(request PinRequest) Admission {
	result := Admission{Global: request.Global}
	refuse := func(reason PinReason) Admission { result.Reason = reason; return result }
	if !request.Requested.valid() {
		return refuse(PinInvalid)
	}
	if request.View == nil {
		if request.Global.Definition != DefinitionUnique || request.Global.Lifecycle != Active || request.Global.Current != request.Requested {
			return refuse(PinNotCurrent)
		}
		result.Admitted, result.Basis = true, Current
		return result
	}
	view := request.View
	if !view.Resolved || strings.TrimSpace(view.Artifact) == "" || !coderef.ValidCommit(view.SourceCommit) {
		return refuse(ViewMissing)
	}
	if !view.Binding.valid() {
		return refuse(ViewBindingMissing)
	}
	if view.Binding != request.Requested {
		return refuse(ViewBindingMismatch)
	}
	if view.Definition == DefinitionMissing {
		return refuse(ViewDefinitionMissing)
	}
	if view.Definition == DefinitionConflicted || view.Lifecycle == LifecycleConflicted {
		return refuse(ViewAmbiguous)
	}
	if view.Lifecycle == Retired {
		return refuse(ViewInactive)
	}
	if view.Definition != DefinitionUnique || view.Lifecycle != Active {
		return refuse(ViewUnknown)
	}
	result.Admitted, result.Basis = true, Saved
	return result
}
