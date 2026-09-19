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

// TermDefinition is the complete content of one term revision.
type TermDefinition struct {
	Name       string
	Definition string
	Aliases    []string
	Stories    []string
	Records    []string
	// Code references are authored by the caller, which reads each one's
	// digest from the code repository.
	Code []coderef.Reference
}

type AddTermInput struct {
	ID         string
	RevisionID string
	EventID    string
	TermDefinition
	CreatedAt time.Time
	RequestID string
}

type ReviseTermInput struct {
	Term    string
	ID      string
	Parents []string
	TermDefinition
	CreatedAt time.Time
	RequestID string
}

type SetTermStateInput struct {
	Term      string
	ID        string
	Parents   []string
	State     TermState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

func termRevision(urn, id string, parents []string, definition TermDefinition, createdAt time.Time, requestID string) TermRevision {
	trimmed := func(values []string) []string {
		result := []string{}
		for _, value := range values {
			result = append(result, strings.TrimSpace(value))
		}
		if len(result) == 0 {
			return nil
		}
		return result
	}
	code := append([]coderef.Reference(nil), definition.Code...)
	if len(code) == 0 {
		code = nil
	}
	return TermRevision{Schema: TermRevisionSchemaURL, Version: AppRecordVersion, ID: id, Term: urn, Parents: copyStrings(parents),
		Name: strings.TrimSpace(definition.Name), Definition: strings.TrimSpace(definition.Definition),
		Aliases: trimmed(definition.Aliases), Stories: trimmed(definition.Stories), Records: trimmed(definition.Records), Code: code,
		CreatedAt: createdAt, RequestID: requestID}
}

// AddTerm publishes a term's identity, first revision, and active event
// atomically.
func AddTerm(root, sagaID string, input AddTermInput) (MutationResult, error) {
	createdAt := mutationTime(input.CreatedAt)
	urn, err := TermURN(sagaID, input.ID)
	if err != nil {
		return MutationResult{}, err
	}
	identity := RecordIdentity{Schema: TermSchemaURL, Version: AppRecordVersion, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
	revision := termRevision(urn, input.RevisionID, []string{}, input.TermDefinition, createdAt, input.RequestID)
	event := TermEvent{Schema: TermEventSchemaURL, Version: AppRecordVersion, ID: input.EventID, Term: urn, Parents: []string{},
		State: TermActive, CreatedAt: createdAt, RequestID: input.RequestID}
	if err := validateRequestIDValue(input.RequestID); err != nil {
		return MutationResult{}, err
	}
	if err := validateTermRevision(revision, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	if err := validateTermEvent(event, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	revisionURN, _ := TermRevisionURN(sagaID, input.ID, input.RevisionID)
	eventURN, _ := TermEventURN(sagaID, input.ID, input.EventID)
	path := TermPackagePath(input.ID)
	paths := []string{path, path + "/revisions/" + input.RevisionID + ".json", path + "/events/" + input.EventID + ".json"}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		if existing := document.FindTerm(input.ID); existing != nil {
			identity.CreatedAt, revision.CreatedAt, event.CreatedAt = existing.Identity.CreatedAt, existing.Identity.CreatedAt, existing.Identity.CreatedAt
			if input.RequestID != "" && existing.Identity.RequestID == input.RequestID && len(existing.Revisions) > 0 && len(existing.Events) > 0 &&
				reflect.DeepEqual(existing.Identity, identity) && reflect.DeepEqual(existing.Revisions[0], revision) && reflect.DeepEqual(existing.Events[0], event) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...), Replayed: true}
				return nil
			}
			return fmt.Errorf("term id %q already exists", input.ID)
		}
		if len(document.Terms) >= MaxTerms {
			return fmt.Errorf("term limit of %d reached", MaxTerms)
		}
		if err := validateTermLinks(document, revision); err != nil {
			return err
		}
		err := createAppPackage(document.Root, filepath.Join(document.Root, filepath.FromSlash(path)), "term.json", identity, revision, event, input.RevisionID, input.EventID)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("term id %q already exists", input.ID)
		}
		if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: []string{revisionURN, eventURN}}
		return nil
	})
	return result, err
}

// ReviseTerm appends a complete term revision. Its parents must name every
// current revision head, so several parents reconcile competing heads.
func ReviseTerm(root, sagaID string, input ReviseTermInput) (MutationResult, error) {
	id, err := ParseTermURN(sagaID, input.Term)
	if err != nil {
		return MutationResult{}, err
	}
	revision := termRevision(input.Term, input.ID, input.Parents, input.TermDefinition, mutationTime(input.CreatedAt), input.RequestID)
	if err := validateTermRevision(revision, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := TermRevisionURN(sagaID, id, input.ID)
	path := TermPackagePath(id) + "/revisions/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		term := document.FindTerm(id)
		if term == nil {
			return fmt.Errorf("term %q does not exist", id)
		}
		for _, existing := range term.Revisions {
			if existing.ID != input.ID {
				continue
			}
			wanted := revision
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(term.RevisionHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", input.ID)
		}
		if !sameSet(revision.Parents, term.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, term.RevisionHeads)
		}
		if err := validateTermLinks(document, revision); err != nil {
			return err
		}
		candidate := *term
		candidate.Revisions = append(append([]TermRevision{}, term.Revisions...), revision)
		if err := resolveTerm(&candidate, sagaID); err != nil {
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

// SetTermState retires or restores a term.
func SetTermState(root, sagaID string, input SetTermStateInput) (MutationResult, error) {
	id, err := ParseTermURN(sagaID, input.Term)
	if err != nil {
		return MutationResult{}, err
	}
	event := TermEvent{Schema: TermEventSchemaURL, Version: AppRecordVersion, ID: input.ID, Term: input.Term, Parents: copyStrings(input.Parents),
		State: input.State, Reason: strings.TrimSpace(input.Reason), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	if err := validateTermEvent(event, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := TermEventURN(sagaID, id, input.ID)
	path := TermPackagePath(id) + "/events/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		term := document.FindTerm(id)
		if term == nil {
			return fmt.Errorf("term %q does not exist", id)
		}
		for _, existing := range term.Events {
			if existing.ID != input.ID {
				continue
			}
			wanted := event
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(term.LifecycleHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", input.ID)
		}
		if !sameSet(event.Parents, term.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, term.LifecycleHeads)
		}
		candidate := *term
		candidate.Events = append(append([]TermEvent{}, term.Events...), event)
		if err := resolveTerm(&candidate, sagaID); err != nil {
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
