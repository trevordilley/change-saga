package inventoryview

import (
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

type Intent = technicalpolicy.Intent

// RevisionIntent reports the explicit proposed/implemented assessment of one
// revision. Today's persisted revisions carry no intent, so every legacy
// revision reads as unspecified; intent is never inferred from code evidence,
// lifecycle, age or name.
func RevisionIntent(rev *requirements.TechnicalRevision) Intent {
	if rev == nil {
		return technicalpolicy.Unspecified
	}
	return technicalpolicy.Unspecified
}

// Newness classifications are relative to one named comparison baseline.
const (
	NewnessNew      = "new"
	NewnessExisting = "existing"
	NewnessUnknown  = "unknown"
)

// Baseline is the inventory read at a named comparison base. Known is false
// when the baseline could not be read; Absent means the Saga (and therefore
// every technical identity) did not exist at that base.
type Baseline struct {
	Known     bool
	Absent    bool
	Inventory *requirements.Inventory
	Commit    string
}

// Newness classifies identity introduction only. A revised existing identity
// is existing; an unreadable baseline is unknown, never "everything is new".
func Newness(target string, baseline Baseline) string {
	switch {
	case !baseline.Known:
		return NewnessUnknown
	case baseline.Absent || baseline.Inventory == nil || baseline.Inventory.Find(target) == nil:
		return NewnessNew
	default:
		return NewnessExisting
	}
}
