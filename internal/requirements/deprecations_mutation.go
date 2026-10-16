package requirements

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/store"
)

// DeprecationDefinition is the complete content of one deprecation revision.
type DeprecationDefinition struct {
	Reason      string
	Scope       string
	Target      DeprecationTarget
	Replacement *DeprecationTarget
	Code        []coderef.Reference
}

type AddDeprecationInput struct {
	ID         string
	RevisionID string
	EventID    string
	DeprecationDefinition
	CreatedAt time.Time
	RequestID string
}

type ReviseDeprecationInput struct {
	Deprecation string
	ID          string
	Parents     []string
	DeprecationDefinition
	CreatedAt time.Time
	RequestID string
}

type SetDeprecationStateInput struct {
	Deprecation string
	ID          string
	Parents     []string
	State       DeprecationState
	Reason      string
	CreatedAt   time.Time
	RequestID   string
}

func deprecationRevision(urn, id string, parents []string, definition DeprecationDefinition, createdAt time.Time, requestID string) DeprecationRevision {
	return DeprecationRevision{Schema: DeprecationRevisionSchemaURL, Version: AppRecordVersion, ID: id, Deprecation: urn, Parents: copyStrings(parents), Reason: strings.TrimSpace(definition.Reason), Scope: definition.Scope, Target: definition.Target, Replacement: definition.Replacement, Code: definition.Code, CreatedAt: createdAt, RequestID: requestID}
}

// AddDeprecation publishes a deprecation's identity, first revision, and active event
// atomically.
func AddDeprecation(root, sagaID string, input AddDeprecationInput) (MutationResult, error) {
	createdAt := mutationTime(input.CreatedAt)
	urn, err := DeprecationURN(sagaID, input.ID)
	if err != nil {
		return MutationResult{}, err
	}
	identity := RecordIdentity{Schema: DeprecationSchemaURL, Version: AppRecordVersion, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
	revision := deprecationRevision(urn, input.RevisionID, []string{}, input.DeprecationDefinition, createdAt, input.RequestID)
	event := DeprecationEvent{Schema: DeprecationEventSchemaURL, Version: AppRecordVersion, ID: input.EventID, Deprecation: urn, Parents: []string{},
		State: DeprecationActive, CreatedAt: createdAt, RequestID: input.RequestID}
	if err := validateRequestIDValue(input.RequestID); err != nil {
		return MutationResult{}, err
	}
	if err := validateDeprecationRevision(revision, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	if err := validateDeprecationEvent(event, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	revisionURN, _ := DeprecationRevisionURN(sagaID, input.ID, input.RevisionID)
	eventURN, _ := DeprecationEventURN(sagaID, input.ID, input.EventID)
	path := DeprecationPackagePath(input.ID)
	paths := []string{path, path + "/revisions/" + input.RevisionID + ".json", path + "/events/" + input.EventID + ".json"}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		if existing := document.FindDeprecation(input.ID); existing != nil {
			identity.CreatedAt, revision.CreatedAt, event.CreatedAt = existing.Identity.CreatedAt, existing.Identity.CreatedAt, existing.Identity.CreatedAt
			// Files are sorted by ID, not chronology. A later correction or
			// withdrawal must not prevent replaying the original creation.
			revisionMatches, eventMatches := false, false
			for _, old := range existing.Revisions {
				revisionMatches = revisionMatches || reflect.DeepEqual(old, revision)
			}
			for _, old := range existing.Events {
				eventMatches = eventMatches || reflect.DeepEqual(old, event)
			}
			if input.RequestID != "" && existing.Identity.RequestID == input.RequestID &&
				reflect.DeepEqual(existing.Identity, identity) && revisionMatches && eventMatches {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...), Replayed: true}
				return nil
			}
			return fmt.Errorf("deprecation id %q already exists", input.ID)
		}
		if len(document.Deprecations) >= MaxDeprecations {
			return fmt.Errorf("deprecation limit of %d reached", MaxDeprecations)
		}
		if err := ValidateDeprecationPins(document.Root, document.SagaID, revision); err != nil {
			return err
		}
		err := createAppPackage(document.Root, filepath.Join(document.Root, filepath.FromSlash(path)), "deprecation.json", identity, revision, event, input.RevisionID, input.EventID)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("deprecation id %q already exists", input.ID)
		}
		if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: []string{revisionURN, eventURN}}
		return nil
	})
	return result, err
}

// ReviseDeprecation appends a complete deprecation revision. Its parents must name every
// current revision head, so several parents reconcile competing heads.
func ReviseDeprecation(root, sagaID string, input ReviseDeprecationInput) (MutationResult, error) {
	id, err := ParseDeprecationURN(sagaID, input.Deprecation)
	if err != nil {
		return MutationResult{}, err
	}
	revision := deprecationRevision(input.Deprecation, input.ID, input.Parents, input.DeprecationDefinition, mutationTime(input.CreatedAt), input.RequestID)
	if err := validateDeprecationRevision(revision, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := DeprecationRevisionURN(sagaID, id, input.ID)
	path := DeprecationPackagePath(id) + "/revisions/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		deprecation := document.FindDeprecation(id)
		if deprecation == nil {
			return fmt.Errorf("deprecation %q does not exist", id)
		}
		for _, existing := range deprecation.Revisions {
			if existing.ID != input.ID {
				continue
			}
			wanted := revision
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(deprecation.RevisionHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", input.ID)
		}
		if !sameSet(revision.Parents, deprecation.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, deprecation.RevisionHeads)
		}
		if err := ValidateDeprecationPins(document.Root, document.SagaID, revision); err != nil {
			return err
		}
		candidate := *deprecation
		candidate.Revisions = append(append([]DeprecationRevision{}, deprecation.Revisions...), revision)
		if err := resolveDeprecation(&candidate, sagaID); err != nil {
			return err
		}
		if err := store.WriteJSON(filepath.Join(document.Root, filepath.FromSlash(path)), revision, true); err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: []string{urn}}
		return nil
	})
	return result, err
}

// SetDeprecationState retires or restores a deprecation.
func SetDeprecationState(root, sagaID string, input SetDeprecationStateInput) (MutationResult, error) {
	id, err := ParseDeprecationURN(sagaID, input.Deprecation)
	if err != nil {
		return MutationResult{}, err
	}
	event := DeprecationEvent{Schema: DeprecationEventSchemaURL, Version: AppRecordVersion, ID: input.ID, Deprecation: input.Deprecation, Parents: copyStrings(input.Parents),
		State: input.State, Reason: strings.TrimSpace(input.Reason), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	if err := validateDeprecationEvent(event, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := DeprecationEventURN(sagaID, id, input.ID)
	path := DeprecationPackagePath(id) + "/events/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		deprecation := document.FindDeprecation(id)
		if deprecation == nil {
			return fmt.Errorf("deprecation %q does not exist", id)
		}
		for _, existing := range deprecation.Events {
			if existing.ID != input.ID {
				continue
			}
			wanted := event
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(deprecation.LifecycleHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", input.ID)
		}
		if !sameSet(event.Parents, deprecation.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, deprecation.LifecycleHeads)
		}
		candidate := *deprecation
		candidate.Events = append(append([]DeprecationEvent{}, deprecation.Events...), event)
		if err := resolveDeprecation(&candidate, sagaID); err != nil {
			return err
		}
		if err := store.WriteJSON(filepath.Join(document.Root, filepath.FromSlash(path)), event, true); err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: []string{urn}}
		return nil
	})
	return result, err
}
