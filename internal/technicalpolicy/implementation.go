package technicalpolicy

import (
	"context"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
)

const MaxEdges = 64
const MaxReferences = 64

type Intent string

const (
	Proposed    Intent = "proposed"
	Implemented Intent = "implemented"
	// Unspecified is readable legacy intent, never a new assessment.
	Unspecified Intent = "unspecified"
)

// EvidenceResolver is satisfied directly by *coderesolve.Resolver. Author must
// read the original bytes, Resolve must evaluate delivery currency, and
// CommitExists must establish a commit object, not merely an OID-shaped string.
// Implementations are trusted read-only dependencies, scoped to one source.
type EvidenceResolver interface {
	CommitExists(context.Context, string) (bool, error)
	Author(context.Context, coderef.Location, string) (coderef.Reference, error)
	Resolve(context.Context, coderef.Reference, string) coderesolve.Resolution
}

// Endpoint carries facts about exactly Pin, never its latest successor.
type Endpoint struct {
	Pin      Pin
	Resolved bool
	Intent   Intent
}

type Edge struct {
	ID          string
	Intent      Intent
	Destination Endpoint
	Evidence    []coderef.Reference
}

type Candidate struct {
	Pin         Pin
	Intent      Intent
	DeliveryOID string
	Evidence    []coderef.Reference
	Edges       []Edge
}

type Code string

const (
	InvalidOwner           Code = "invalid_owner"
	InvalidIntent          Code = "invalid_intent"
	InvalidDelivery        Code = "invalid_delivery"
	DeliveryUnavailable    Code = "delivery_unavailable"
	ResolverMissing        Code = "resolver_missing"
	TooManyEdges           Code = "too_many_edges"
	InvalidEdgeID          Code = "invalid_edge_id"
	DuplicateEdgeID        Code = "duplicate_edge_id"
	MissingEvidence        Code = "missing_evidence"
	TooManyReferences      Code = "too_many_references"
	InvalidReference       Code = "invalid_reference"
	DuplicateReference     Code = "duplicate_reference"
	InvalidOriginal        Code = "invalid_original_evidence"
	StaleDelivery          Code = "stale_delivery_evidence"
	EndpointNotImplemented Code = "endpoint_not_implemented"
)

// Diagnostic positions are zero-based; -1 denotes the entity or whole owner.
// Codes and positions are deterministic; resolver error text is not exposed.
type Diagnostic struct {
	Owner          Pin
	EdgeID         string
	EdgeIndex      int
	ReferenceIndex int
	Code           Code
}

type Result struct {
	Diagnostics []Diagnostic
	// ProposedEdges preserves input order even if some other check refuses the
	// candidate. It is remaining work, not a successful publication projection.
	ProposedEdges []string
}

func (r Result) Valid() bool { return len(r.Diagnostics) == 0 }

// ValidateCandidate assesses explicit candidate intent without modifying inputs
// or publishing changes. Diagnostics are entity-first, then edge input order,
// with each owner's references in input order. Oversized collections are refused
// without traversing them. Proposed evidence is shape-checked but does not assert
// currency; every implemented edge (including retained edges) is revalidated.
func ValidateCandidate(ctx context.Context, candidate Candidate, resolver EvidenceResolver) Result {
	result := Result{}
	add := func(edgeIndex int, edgeID string, refIndex int, code Code) {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{candidate.Pin, edgeID, edgeIndex, refIndex, code})
	}
	if !candidate.Pin.valid() {
		add(-1, "", -1, InvalidOwner)
	}
	if candidate.Intent != Proposed && candidate.Intent != Implemented {
		add(-1, "", -1, InvalidIntent)
	}
	if len(candidate.Edges) > MaxEdges {
		add(-1, "", -1, TooManyEdges)
		return result
	}
	needsDelivery := candidate.Intent == Implemented
	for _, edge := range candidate.Edges {
		needsDelivery = needsDelivery || edge.Intent == Implemented
	}
	deliveryOK := false
	if needsDelivery || candidate.DeliveryOID != "" {
		switch {
		case !coderef.ValidCommit(candidate.DeliveryOID):
			add(-1, "", -1, InvalidDelivery)
		case resolver == nil:
			add(-1, "", -1, ResolverMissing)
		default:
			exists, err := resolver.CommitExists(ctx, candidate.DeliveryOID)
			if err != nil || !exists {
				add(-1, "", -1, DeliveryUnavailable)
			} else {
				deliveryOK = true
			}
		}
	}
	checkEvidence := func(refs []coderef.Reference, implemented bool, edgeIndex int, edgeID string) {
		if implemented && len(refs) == 0 {
			add(edgeIndex, edgeID, -1, MissingEvidence)
		}
		if len(refs) > MaxReferences {
			add(edgeIndex, edgeID, -1, TooManyReferences)
			return
		}
		seen := map[string]bool{}
		for index, ref := range refs {
			if coderef.Validate(ref) != nil || ref.WholeFile() || strings.TrimSpace(ref.Note) == "" {
				add(edgeIndex, edgeID, index, InvalidReference)
				continue
			}
			if seen[ref.Key()] {
				add(edgeIndex, edgeID, index, DuplicateReference)
				continue
			}
			seen[ref.Key()] = true
			if !implemented || !deliveryOK {
				continue
			}
			exists, err := resolver.CommitExists(ctx, ref.Commit)
			if err != nil || !exists {
				add(edgeIndex, edgeID, index, InvalidOriginal)
				continue
			}
			original, err := resolver.Author(ctx, ref.Location(), ref.Note)
			if err != nil || original != ref {
				add(edgeIndex, edgeID, index, InvalidOriginal)
				continue
			}
			resolution := resolver.Resolve(ctx, ref, candidate.DeliveryOID)
			if resolution.State != coderesolve.Current || resolution.Location.Commit != candidate.DeliveryOID || resolution.Location.Validate() != nil || resolution.Location.WholeFile() {
				add(edgeIndex, edgeID, index, StaleDelivery)
			}
		}
	}
	checkEvidence(candidate.Evidence, candidate.Intent == Implemented, -1, "")
	seen := map[string]bool{}
	for index, edge := range candidate.Edges {
		if strings.TrimSpace(edge.ID) == "" {
			add(index, edge.ID, -1, InvalidEdgeID)
		} else if seen[edge.ID] {
			add(index, edge.ID, -1, DuplicateEdgeID)
		}
		seen[edge.ID] = true
		switch edge.Intent {
		case Proposed:
			result.ProposedEdges = append(result.ProposedEdges, edge.ID)
		case Implemented:
			destinationImplemented := edge.Destination.Resolved && edge.Destination.Pin.valid() && edge.Destination.Intent == Implemented
			// A self edge to the candidate revision uses its post-transition intent.
			// A baseline revision of the same identity still needs exact resolved facts.
			if edge.Destination.Pin == candidate.Pin && candidate.Pin.valid() {
				destinationImplemented = candidate.Intent == Implemented
			}
			if candidate.Intent != Implemented || !destinationImplemented {
				add(index, edge.ID, -1, EndpointNotImplemented)
			}
		default:
			add(index, edge.ID, -1, InvalidIntent)
		}
		checkEvidence(edge.Evidence, edge.Intent == Implemented, index, edge.ID)
	}
	return result
}
