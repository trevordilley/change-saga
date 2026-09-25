package inventoryview

import (
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

type Intent = technicalpolicy.Intent

// RevisionIntent reports the explicit proposed/implemented assessment of one
// revision. Legacy revisions read as unspecified; intent is never inferred
// from code evidence, lifecycle, age or name.
func RevisionIntent(rev *requirements.TechnicalRevision) Intent {
	if rev == nil {
		return technicalpolicy.Unspecified
	}
	return Intent(rev.EffectiveIntent())
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

// OwnedEvidence is one code reference a revision owns: its own code (Edge "")
// or an interaction's or relationship's, with that edge's explicit intent.
type OwnedEvidence struct {
	Edge     string
	Intent   Intent
	Evidence requirements.Evidence
}

// Suffix is the owner suffix appended to a record target: "" or "#edge".
func (o OwnedEvidence) Suffix() string {
	if o.Edge == "" {
		return ""
	}
	return "#" + o.Edge
}

// Evidence lists every code reference a revision owns, in declaration order.
func Evidence(rev *requirements.TechnicalRevision) []OwnedEvidence {
	out := []OwnedEvidence{}
	if rev == nil {
		return out
	}
	own := RevisionIntent(rev)
	for _, e := range rev.Code {
		out = append(out, OwnedEvidence{"", own, e})
	}
	for _, edge := range rev.Interactions {
		for _, e := range edge.Code {
			out = append(out, OwnedEvidence{edge.ID, edgeIntent(edge.Intent), e})
		}
	}
	for _, edge := range rev.Relationships {
		for _, e := range edge.Code {
			out = append(out, OwnedEvidence{edge.ID, edgeIntent(edge.Intent), e})
		}
	}
	return out
}

func edgeIntent(value string) Intent {
	if value == "" {
		return technicalpolicy.Unspecified
	}
	return Intent(value)
}
