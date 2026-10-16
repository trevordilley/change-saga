package requirements

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/saga"
)

var deprecationAppearanceID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]{0,127}$`)

var deprecationDigest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// DeprecationTargetKind admits only existing addressable content kinds. A
// native whiteboard appearance is a revision-local path, not a new entity.
func DeprecationTargetKind(sagaID, target string) (string, error) {
	p := strings.Split(target, ":")
	if len(p) < 5 || p[0] != "urn" || p[1] != "change-saga" || p[2] != sagaID || !livingid.ValidID(sagaID) {
		return "", fmt.Errorf("target must be a canonical content URN in this Saga")
	}
	for i := 4; i < len(p); i += 2 {
		if !livingid.ValidID(p[i]) {
			return "", fmt.Errorf("target has an invalid stable ID")
		}
	}
	if len(p) == 5 {
		switch p[3] {
		case "fragment", "deck", "slide", "component", "system", "data-entity", "erd", "erd-overlay", "whiteboard":
			return p[3], nil
		}
	}
	if len(p) == 7 && ((p[3] == "fragment" && p[5] == "landmark") || (p[3] == "slide" && p[5] == "item")) {
		return p[5], nil
	}
	if p[3] == "review" && ((len(p) == 7 && (p[5] == "deck" || p[5] == "slide")) || (len(p) == 9 && p[5] == "slide" && p[7] == "item")) {
		return p[len(p)-2], nil
	}
	return "", fmt.Errorf("unsupported deprecation target %q", target)
}

func WhiteboardURN(sagaID, id string) (string, error) { return appRecordURN(sagaID, "whiteboard", id) }

func ValidateDeprecationTarget(sagaID string, pin DeprecationTarget) error {
	kind, err := DeprecationTargetKind(sagaID, pin.Target)
	if err != nil {
		return err
	}
	if kind == "whiteboard" || isTechnicalDeprecationKind(kind) {
		if pin.ContentDigest != "" {
			return fmt.Errorf("revision target cannot carry a content digest")
		}
		prefix := pin.Target + ":revision:"
		if !strings.HasPrefix(pin.Revision, prefix) || !livingid.ValidID(strings.TrimPrefix(pin.Revision, prefix)) {
			return fmt.Errorf("revision must pin the target's own revision")
		}
	} else if pin.Revision != "" || !deprecationDigest.MatchString(pin.ContentDigest) {
		return fmt.Errorf("content target requires a SHA-256 content digest, not a revision")
	}
	if pin.Element != "" {
		if kind != "whiteboard" || len(pin.Element) > 2048 {
			return fmt.Errorf("element requires a bounded whiteboard appearance path")
		}
		for _, part := range strings.Split(pin.Element, "/") {
			if !deprecationAppearanceID.MatchString(part) {
				return fmt.Errorf("invalid whiteboard appearance path")
			}
		}
	}
	return nil
}
func isTechnicalDeprecationKind(kind string) bool { _, ok := technicalKindDir(kind); return ok }

// DeprecationTargets lazily loads each target domain once for one observation.
// Missing and conflicted content remains readable through the assertion's pin.
type DeprecationTargets struct {
	Root, SagaID string
	narrative    *saga.Saga
	inventory    *Inventory
}

type DeprecationTargetHealth struct {
	Status    string             `json:"status"`
	Reason    string             `json:"reason,omitempty"`
	Current   *DeprecationTarget `json:"current,omitempty"`
	Lifecycle string             `json:"lifecycle,omitempty"`
}

func (r *DeprecationTargets) Observe(pin DeprecationTarget) DeprecationTargetHealth {
	current := DeprecationTarget{Target: pin.Target, Element: pin.Element}
	result := DeprecationTargetHealth{Status: "current"}
	kind, err := DeprecationTargetKind(r.SagaID, pin.Target)
	if err != nil {
		return DeprecationTargetHealth{Status: "invalid", Reason: err.Error()}
	}
	switch {
	case kind == "whiteboard":
		id := strings.TrimPrefix(pin.Target, "urn:change-saga:"+r.SagaID+":whiteboard:")
		revisions, heads, e := whiteboardHistory(r.Root, r.SagaID, id)
		if e != nil {
			return DeprecationTargetHealth{Status: "unresolved", Reason: e.Error()}
		}
		if len(revisions) == 0 {
			return DeprecationTargetHealth{Status: "missing", Reason: "whiteboard is missing"}
		}
		if len(heads) != 1 {
			return DeprecationTargetHealth{Status: "conflicted", Reason: "whiteboard has competing revision heads"}
		}
		current.Revision = pin.Target + ":revision:" + heads[0]
		if pin.Element != "" {
			board, e := LoadResolvedWhiteboard(r.Root, r.SagaID, id, heads[0])
			if e != nil {
				return DeprecationTargetHealth{Status: "unresolved", Reason: e.Error()}
			}
			if _, e = board.Composition.Describe(pin.Element); e != nil {
				return DeprecationTargetHealth{Status: "missing", Reason: "whiteboard appearance is missing: " + e.Error()}
			}
		}
	case isTechnicalDeprecationKind(kind):
		if r.inventory == nil {
			inv, e := LoadInventory(r.Root, r.SagaID)
			if e != nil {
				return DeprecationTargetHealth{Status: "unresolved", Reason: e.Error()}
			}
			r.inventory = &inv
		}
		record := r.inventory.Find(pin.Target)
		if record == nil {
			return DeprecationTargetHealth{Status: "missing", Reason: "technical identity is missing"}
		}
		if record.CurrentRevision == nil {
			return DeprecationTargetHealth{Status: "conflicted", Reason: "technical identity has competing revision heads"}
		}
		current.Revision = record.RevisionHeads[0]
		if record.CurrentLifecycle != nil {
			result.Lifecycle = string(record.CurrentLifecycle.State)
		} else {
			result.Lifecycle = "conflicted"
		}
	default:
		if r.narrative == nil {
			doc, validation, e := saga.LoadNarrative(r.Root)
			if e != nil {
				return DeprecationTargetHealth{Status: "unresolved", Reason: e.Error()}
			}
			if !validation.Valid {
				return DeprecationTargetHealth{Status: "unresolved", Reason: "narrative validation failed"}
			}
			r.narrative = doc
		}
		digest, found, conflict, e := saga.AnnotationContentDigest(r.narrative, pin.Target)
		if e != nil {
			return DeprecationTargetHealth{Status: "unresolved", Reason: e.Error()}
		}
		if !found {
			return DeprecationTargetHealth{Status: "missing", Reason: "content identity is missing"}
		}
		if conflict {
			return DeprecationTargetHealth{Status: "conflicted", Reason: "content has competing authoring heads"}
		}
		current.ContentDigest = digest
	}
	result.Current = &current
	if (pin.Revision != "" && pin.Revision != current.Revision) || (pin.ContentDigest != "" && pin.ContentDigest != current.ContentDigest) {
		result.Status = "stale"
		result.Reason = "target changed since the authored assertion"
	}
	return result
}

// Pin authors against a unique current target, without modifying that target.
func (r *DeprecationTargets) Pin(target, element string) (DeprecationTarget, error) {
	health := r.Observe(DeprecationTarget{Target: target, Element: element})
	if health.Status != "current" || health.Current == nil {
		return DeprecationTarget{}, fmt.Errorf("%s: %s: %s", target, health.Status, health.Reason)
	}
	if err := ValidateDeprecationTarget(r.SagaID, *health.Current); err != nil {
		return DeprecationTarget{}, err
	}
	return *health.Current, nil
}

// ValidateDeprecationPins checks new assertions only. Historical pins are never
// required to stay current or even present; readers report their health.
func ValidateDeprecationPins(root, sagaID string, revision DeprecationRevision) error {
	targets := DeprecationTargets{Root: root, SagaID: sagaID}
	pins := []DeprecationTarget{revision.Target}
	if revision.Replacement != nil {
		pins = append(pins, *revision.Replacement)
	}
	for _, pin := range pins {
		health := targets.Observe(pin)
		if health.Status != "current" {
			return fmt.Errorf("%s is %s: %s", pin.Target, health.Status, health.Reason)
		}
	}
	return nil
}

// RelatedCode returns direct evidence on a target, without containment or link
// expansion. These references explain content; none asserts code deprecation.
func (r *DeprecationTargets) RelatedCode(target string) ([]saga.CodeFile, error) {
	kind, err := DeprecationTargetKind(r.SagaID, target)
	if err != nil {
		return nil, err
	}
	if isTechnicalDeprecationKind(kind) {
		if r.inventory == nil {
			inv, e := LoadInventory(r.Root, r.SagaID)
			if e != nil {
				return nil, e
			}
			r.inventory = &inv
		}
		record := r.inventory.Find(target)
		if record == nil || record.CurrentRevision == nil {
			return nil, nil
		}
		revision := record.CurrentRevision
		refs := References(revision.Code)
		for _, v := range revision.Interactions {
			refs = append(refs, References(v.Code)...)
		}
		for _, v := range revision.Relationships {
			refs = append(refs, References(v.Code)...)
		}
		return []saga.CodeFile{{Path: record.RevisionHeads[0], References: refs}}, nil
	}
	if kind == "whiteboard" {
		return nil, nil
	}
	if r.narrative == nil {
		doc, _, e := saga.LoadNarrative(r.Root)
		if e != nil {
			return nil, e
		}
		r.narrative = doc
	}
	index := saga.MutationIndexFromDocument(r.narrative)
	// Review evidence is independent of living coverage but is still directly related.
	for _, review := range r.narrative.Reviews {
		if review.Deck != nil {
			for _, slide := range review.Deck.Slides {
				for _, item := range slide.Items {
					if item.Target == target {
						index.Targets[target] = item.Directory
						index.FlatTargets[target] = true
					}
				}
			}
		}
	}
	if _, exists := index.Targets[target]; !exists {
		return nil, nil
	}
	code, validation, err := saga.LoadTargetCode(index, target)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		return nil, fmt.Errorf("target evidence is invalid")
	}
	return code, nil
}
