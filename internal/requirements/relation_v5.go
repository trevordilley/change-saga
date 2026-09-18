package requirements

import (
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/qualityid"
)

// validateV5Relation implements the frozen v5 relation contract
// (schema/v5/relation.schema.json and SPEC.md's v5 relation matrix). It never
// changes how a v3 record is read: validateRelation dispatches on version.
func validateV5Relation(value Relation, sagaID, expectedID string) error {
	var problems validationErrors
	if value.Schema != V5RelationSchemaURL {
		problems.add("$schema must be %q", V5RelationSchemaURL)
	}
	if value.Version != V5RelationVersion {
		problems.add("version must be %d", V5RelationVersion)
	}
	if value.Scope != ScopeSelf && value.Scope != ScopeDescendants {
		problems.add("scope must be self or descendants")
	}
	from, fromErr := parseEndpoint(value.From)
	to, toErr := parseEndpoint(value.To)
	validateRelationCommon(&problems, value, sagaID, expectedID, from, to, fromErr, toErr)
	if fromErr == nil && toErr == nil {
		if value.Type == RelationConflictsWith && value.To < value.From {
			problems.add("conflicts_with endpoints must use canonical lexical order")
		}
		validateV5Scope(&problems, value, from)
		validateV5Matrix(&problems, value, from, to)
		validateV5EndpointPins(&problems, "from", from, value.FromRevision, value.FromContentDigest)
		validateV5EndpointPins(&problems, "to", to, value.ToRevision, value.ToContentDigest)
	}
	return problems.err()
}

func isRequirement(kind endpointKind) bool {
	return kind == endpointStory || kind == endpointCriterion
}

func isVisual(kind endpointKind) bool {
	return kind == endpointDeck || kind == endpointSlide || kind == endpointItem
}

// validateV5Scope admits descendant reach only when it is explicitly declared
// on a Deck or Slide source of an addresses or explains relation.
func validateV5Scope(problems *validationErrors, relation Relation, from endpoint) {
	if relation.Scope != ScopeDescendants {
		return
	}
	if from.Kind != endpointDeck && from.Kind != endpointSlide {
		problems.add("descendants scope requires a Deck or Slide source")
	}
	if relation.Type != RelationAddresses && relation.Type != RelationExplains {
		problems.add("descendants scope is valid only on addresses or explains relations")
	}
}

func validateV5Matrix(problems *validationErrors, relation Relation, from, to endpoint) {
	require := func(ok bool, description string) {
		if !ok {
			problems.add("%s relation requires %s", relation.Type, description)
		}
	}
	switch relation.Type {
	case RelationRefines:
		require(isRequirement(from.Kind) && isRequirement(to.Kind), "story or criterion endpoints")
		require(relation.FromRevision != "" && relation.ToRevision != "", "source and target story revision pins")
	case RelationAddresses:
		require((from.Kind == endpointDesign || isVisual(from.Kind)) && isRequirement(to.Kind), "a report design, Deck, Slide, or Item source and story or criterion target")
		require(relation.FromContentDigest != "" && relation.ToRevision != "", "a source content digest and target revision pin")
	case RelationImplements:
		require(from.Kind == endpointWorkItem && (to.Kind == endpointDesign || to.Kind == endpointCriterion), "a work-item source and report design or criterion target")
		require(relation.FromRevision != "", "a source work-item revision pin")
		if to.Kind == endpointDesign {
			require(relation.ToContentDigest != "", "a target content digest")
		} else {
			require(relation.ToRevision != "", "a target criterion revision pin")
		}
	case RelationExplains:
		require(isVisual(from.Kind) && isRequirement(to.Kind), "a deck, slide, or Item source and story or criterion target")
		require(relation.ToRevision != "", "a target story revision pin")
	case RelationVerifies:
		require((from.Kind == endpointClaim || from.Kind == endpointVerification || from.Kind == endpointTestCase) && to.Kind == endpointCriterion, "a claim, verification, or test-case source and criterion target")
		require(relation.ToRevision != "", "a target criterion revision pin")
		if from.Kind == endpointTestCase {
			require(relation.FromRevision != "", "a source test-case revision pin")
		}
	case RelationSupersedes:
		sameKind := from.Kind == to.Kind
		if from.Kind == endpointDesign && to.Kind == endpointDesign {
			sameKind = from.DesignKind == to.DesignKind
		}
		require(sameKind, "endpoints of the same resource kind")
	case RelationConflictsWith:
		require(isRequirement(from.Kind) && isRequirement(to.Kind), "compatible story or criterion endpoints")
		require(relation.FromRevision != "" && relation.ToRevision != "", "both story revision pins")
	}
}

// validateV5EndpointPins applies the schema's pin classes: revision-only
// endpoints may pin their own revision, design and visual endpoints may pin a
// content digest, and immutable endpoints carry no pin.
func validateV5EndpointPins(problems *validationErrors, side string, endpoint endpoint, revision, digest string) {
	switch endpoint.Kind {
	case endpointStory, endpointCriterion, endpointWorkItem, endpointTestCase, endpointWave, endpointContract:
		if revision != "" && !pinsEndpointRevision(endpoint, revision) {
			problems.add("%s_revision does not pin the %s endpoint", side, endpoint.Kind)
		}
		if digest != "" {
			problems.add("%s %s endpoint cannot use a content digest", side, endpoint.Kind)
		}
	case endpointDesign, endpointDeck, endpointSlide, endpointItem:
		if revision != "" {
			problems.add("%s design endpoint cannot use a definition revision", side)
		}
	default:
		if revision != "" || digest != "" {
			problems.add("%s endpoint kind cannot carry a revision or content digest pin", side)
		}
	}
}

// pinsEndpointRevision reports whether revision is a revision URN of the
// endpoint's own definition (a criterion is pinned by its story's revision).
func pinsEndpointRevision(endpoint endpoint, revision string) bool {
	if endpoint.Kind == endpointTestCase {
		ref, err := qualityid.Parse(revision)
		return err == nil && ref.Kind == qualityid.KindRevision && ref.SagaID == endpoint.SagaID && ref.TestCaseID == endpoint.ID
	}
	ref, err := livingid.Parse(revision)
	if err != nil || ref.Kind != livingid.KindRevision || ref.SagaID != endpoint.SagaID {
		return false
	}
	parentKind := ref.ParentKind
	if parentKind == "" {
		parentKind = livingid.KindStory
	}
	switch endpoint.Kind {
	case endpointStory:
		return parentKind == livingid.KindStory && ref.ParentID == endpoint.ID
	case endpointCriterion:
		return parentKind == livingid.KindStory && ref.ParentID == endpoint.StoryID
	case endpointWorkItem:
		return parentKind == livingid.KindWorkItem && ref.ParentID == endpoint.ID
	case endpointWave:
		return parentKind == livingid.KindWave && ref.ParentID == endpoint.ID
	case endpointContract:
		return parentKind == livingid.KindContract && ref.ParentID == endpoint.ID
	}
	return false
}
