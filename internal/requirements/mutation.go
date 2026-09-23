package requirements

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

func AddStory(root, sagaID string, input AddStoryInput) (MutationResult, error) {
	createdAt := mutationTime(input.CreatedAt)
	identity := StoryIdentity{Schema: StorySchemaURL, Version: Version, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
	storyID, urnErr := storyURN(sagaID, input.ID)
	if urnErr != nil {
		return MutationResult{}, urnErr
	}
	revision := Revision{
		Schema: RevisionSchemaURL, Version: Version, ID: input.RevisionID, Story: storyID, Parents: []string{},
		Title: strings.TrimSpace(input.Title), Statement: strings.TrimSpace(input.Statement), Priority: strings.TrimSpace(input.Priority),
		Personas: copyStrings(input.Personas), Citations: copyStrings(input.Citations), AcceptanceCriteria: copyCriteria(input.AcceptanceCriteria), CreatedAt: createdAt, RequestID: input.RequestID,
	}
	event := LifecycleEvent{
		Schema: LifecycleEventSchemaURL, Version: Version, ID: input.EventID, Story: storyID, Parents: []string{},
		State: StateProposed, CreatedAt: createdAt, RequestID: input.RequestID,
	}
	revisionID, _ := revisionURN(sagaID, input.ID, input.RevisionID)
	eventID, _ := StoryEventURN(sagaID, input.ID, input.EventID)
	created := []string{storyID, revisionID, eventID}
	paths := []string{storyPackagePath(input.Feature, input.ID), revisionPath(input.Feature, input.ID, input.RevisionID), eventPath(input.Feature, input.ID, input.EventID)}
	if err := validateIdentity(identity, input.ID); err != nil {
		return MutationResult{}, err
	}
	if err := validateRevision(revision, sagaID, input.ID); err != nil {
		return MutationResult{}, err
	}
	if err := validateEvent(event, sagaID, input.ID); err != nil {
		return MutationResult{}, err
	}

	var result MutationResult
	err := mutate(root, sagaID, func(document *Document) error {
		for _, existing := range document.Stories {
			if existing.Identity.ID != input.ID {
				continue
			}
			if input.RequestID != "" && existing.Identity.RequestID == input.RequestID && existing.Feature == input.Feature && equalStoryCreation(existing, identity, revision, event) {
				heads := append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...)
				result = MutationResult{URN: storyID, Path: storyPackagePath(input.Feature, input.ID), Created: copyStrings(created), Paths: copyStrings(paths), CurrentHeads: heads, Replayed: true}
				return nil
			}
			return fmt.Errorf("story id %q already exists", input.ID)
		}
		if len(document.Stories) >= MaxStories {
			return fmt.Errorf("story limit of %d reached", MaxStories)
		}
		feature, err := document.feature(input.Feature)
		if err != nil {
			return err
		}
		if err := requireCitations(document, revision.Citations); err != nil {
			return err
		}
		if err := requirePersonas(document, revision.Personas); err != nil {
			return err
		}
		storiesDir, err := store.EnsureDirWithin(document.Root, filepath.Join(feature.Dir, applayout.RequirementsDir, "stories"))
		if err != nil {
			return err
		}
		final := filepath.Join(storiesDir, input.ID+".story")
		if err := store.CommitDir(document.Root, final, func(stage string) error {
			if err := store.WriteJSON(filepath.Join(stage, "story.json"), identity, true); err != nil {
				return err
			}
			revisionsDir := filepath.Join(stage, "revisions")
			eventsDir := filepath.Join(stage, "events")
			if err := ensureStageDir(revisionsDir); err != nil {
				return err
			}
			if err := ensureStageDir(eventsDir); err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(revisionsDir, revision.ID+".json"), revision, true); err != nil {
				return err
			}
			return store.WriteJSON(filepath.Join(eventsDir, event.ID+".json"), event, true)
		}); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("story id %q already exists", input.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{URN: storyID, Path: storyPackagePath(input.Feature, input.ID), Created: copyStrings(created), Paths: copyStrings(paths), CurrentHeads: []string{revisionID, eventID}}
		return nil
	})
	return result, err
}

func ReviseStory(root, sagaID string, input ReviseStoryInput) (MutationResult, error) {
	storyRef, err := livingid.Parse(input.Story)
	if err != nil || storyRef.Kind != livingid.KindStory || storyRef.SagaID != sagaID {
		return MutationResult{}, fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	revision := Revision{
		Schema: RevisionSchemaURL, Version: Version, ID: input.ID, Story: input.Story, Parents: copyStrings(input.Parents),
		Title: strings.TrimSpace(input.Title), Statement: strings.TrimSpace(input.Statement), Priority: strings.TrimSpace(input.Priority),
		Personas: copyStrings(input.Personas), Citations: copyStrings(input.Citations), AcceptanceCriteria: copyCriteria(input.AcceptanceCriteria), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
	}
	if err := validateRevision(revision, sagaID, storyRef.ID); err != nil {
		return MutationResult{}, err
	}
	revisionID, _ := revisionURN(sagaID, storyRef.ID, revision.ID)
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		story := findStory(document, storyRef.ID)
		if story == nil {
			return fmt.Errorf("story %q does not exist", storyRef.ID)
		}
		for _, existing := range story.Revisions {
			if existing.ID != revision.ID {
				continue
			}
			if input.RequestID != "" && existing.RequestID == input.RequestID && equalRevisionIgnoringTime(existing, revision) {
				result = MutationResult{URN: revisionID, Path: revisionPath(story.Feature, storyRef.ID, revision.ID), Created: []string{revisionID}, Paths: []string{revisionPath(story.Feature, storyRef.ID, revision.ID)}, CurrentHeads: copyStrings(story.RevisionHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", revision.ID)
		}
		if len(story.Revisions) >= MaxRevisionsPerStory {
			return fmt.Errorf("revision limit of %d reached", MaxRevisionsPerStory)
		}
		if !sameSet(revision.Parents, story.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, story.RevisionHeads)
		}
		if err := requireCitations(document, revision.Citations); err != nil {
			return err
		}
		if err := requirePersonas(document, revision.Personas); err != nil {
			return err
		}
		candidate := *story
		candidate.Revisions = append(append([]Revision{}, story.Revisions...), revision)
		if err := validateStoryGraphs(&candidate, sagaID, document.citationIDs(), document.personaIDs()); err != nil {
			return err
		}
		path := filepath.Join(document.Root, filepath.FromSlash(revisionPath(story.Feature, storyRef.ID, revision.ID)))
		if err := store.WriteJSON(path, revision, true); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("revision id %q already exists", revision.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{URN: revisionID, Path: revisionPath(story.Feature, storyRef.ID, revision.ID), Created: []string{revisionID}, Paths: []string{revisionPath(story.Feature, storyRef.ID, revision.ID)}, CurrentHeads: []string{revisionID}}
		return nil
	})
	return result, err
}

// AddCriterion is a single-head convenience over an immutable, complete story
// revision. It performs the read/modify/write while holding the Saga writer
// lock, so the caller's explicit parent is an optimistic concurrency check.
func AddCriterion(root, sagaID string, input AddCriterionInput) (MutationResult, error) {
	if !livingid.ValidID(input.Criterion.ID) {
		return MutationResult{}, fmt.Errorf("criterion id is not a stable identifier")
	}
	if strings.TrimSpace(input.Criterion.Statement) == "" {
		return MutationResult{}, fmt.Errorf("criterion %q requires a statement", input.Criterion.ID)
	}
	return mutateCriterion(root, sagaID, criterionMutation{
		kind: criterionAdd, story: input.Story, parent: input.Parent, revisionID: input.RevisionID,
		criterionID: input.Criterion.ID, statement: input.Criterion.Statement,
		createdAt: input.CreatedAt, requestID: input.RequestID,
	})
}

// ReviseCriterion preserves criterion identity while replacing its wording in
// a new complete story snapshot.
func ReviseCriterion(root, sagaID string, input ReviseCriterionInput) (MutationResult, error) {
	return mutateCriterion(root, sagaID, criterionMutation{
		kind: criterionRevise, story: input.Story, criterion: input.Criterion,
		parent: input.Parent, revisionID: input.RevisionID, statement: input.Statement,
		createdAt: input.CreatedAt, requestID: input.RequestID,
	})
}

// RemoveCriterion omits the criterion from a new complete story snapshot. The
// reason is intentionally output-only: requirements history is represented by
// immutable revisions, not an additional tombstone record.
func RemoveCriterion(root, sagaID string, input RemoveCriterionInput) (MutationResult, error) {
	if strings.TrimSpace(input.Reason) == "" {
		return MutationResult{}, fmt.Errorf("criterion removal reason is required")
	}
	return mutateCriterion(root, sagaID, criterionMutation{
		kind: criterionRemove, story: input.Story, criterion: input.Criterion,
		parent: input.Parent, revisionID: input.RevisionID, reason: input.Reason,
		createdAt: input.CreatedAt, requestID: input.RequestID,
	})
}

type criterionMutationKind int

const (
	criterionAdd criterionMutationKind = iota
	criterionRevise
	criterionRemove
)

type criterionMutation struct {
	kind        criterionMutationKind
	story       string
	criterion   string
	parent      string
	revisionID  string
	criterionID string
	statement   string
	reason      string
	createdAt   time.Time
	requestID   string
}

func mutateCriterion(root, sagaID string, input criterionMutation) (MutationResult, error) {
	storyRef, err := livingid.Parse(input.story)
	if err != nil || storyRef.Kind != livingid.KindStory || storyRef.SagaID != sagaID {
		return MutationResult{}, fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	parentStory, err := parseStoryRevision(input.parent, sagaID)
	if err != nil || parentStory != storyRef.ID {
		return MutationResult{}, fmt.Errorf("parent must be a canonical revision URN for story %q", storyRef.ID)
	}
	if !livingid.ValidID(input.revisionID) {
		return MutationResult{}, fmt.Errorf("revision id is not a stable identifier")
	}
	criterionID := input.criterionID
	if input.kind != criterionAdd {
		criterionRef, parseErr := livingid.Parse(input.criterion)
		if parseErr != nil || criterionRef.Kind != livingid.KindCriterion || criterionRef.SagaID != sagaID || criterionRef.ParentID != storyRef.ID {
			return MutationResult{}, fmt.Errorf("criterion must be a canonical criterion URN for story %q", storyRef.ID)
		}
		criterionID = criterionRef.ID
	}
	if input.kind == criterionRevise && strings.TrimSpace(input.statement) == "" {
		return MutationResult{}, fmt.Errorf("criterion %q requires a statement", criterionID)
	}
	criterionIDURN, _ := criterionURN(sagaID, storyRef.ID, criterionID)
	revisionIDURN, _ := revisionURN(sagaID, storyRef.ID, input.revisionID)
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		story := findStory(document, storyRef.ID)
		if story == nil {
			return fmt.Errorf("story %q does not exist", storyRef.ID)
		}
		var parent *Revision
		for index := range story.Revisions {
			revisionURNValue, _ := revisionURN(sagaID, storyRef.ID, story.Revisions[index].ID)
			if revisionURNValue == input.parent {
				parent = &story.Revisions[index]
				break
			}
		}
		if parent == nil {
			return fmt.Errorf("parent revision %q does not exist", input.parent)
		}
		var existingRevision *Revision
		for index := range story.Revisions {
			if story.Revisions[index].ID == input.revisionID {
				existingRevision = &story.Revisions[index]
				break
			}
		}
		if existingRevision == nil {
			if len(story.RevisionHeads) != 1 {
				return fmt.Errorf("criterion mutation requires one current story revision head; found %v; reconcile with story revise first", story.RevisionHeads)
			}
			if story.RevisionHeads[0] != input.parent {
				return fmt.Errorf("criterion parent is stale (got %q, current head is %q)", input.parent, story.RevisionHeads[0])
			}
		}
		revision := Revision{
			Schema: RevisionSchemaURL, Version: Version, ID: input.revisionID, Story: input.story,
			Parents: []string{input.parent}, Title: parent.Title, Statement: parent.Statement,
			Priority: parent.Priority, Personas: copyStrings(parent.Personas), Citations: copyStrings(parent.Citations),
			AcceptanceCriteria: copyCriteria(parent.AcceptanceCriteria), CreatedAt: mutationTime(input.createdAt),
			RequestID: input.requestID,
		}
		found := -1
		for index := range revision.AcceptanceCriteria {
			if revision.AcceptanceCriteria[index].ID == criterionID {
				found = index
				break
			}
		}
		switch input.kind {
		case criterionAdd:
			if found >= 0 {
				return fmt.Errorf("criterion id %q already exists in the current story revision", criterionID)
			}
			revision.AcceptanceCriteria = append(revision.AcceptanceCriteria, Criterion{ID: criterionID, Statement: strings.TrimSpace(input.statement)})
		case criterionRevise:
			if found < 0 {
				return fmt.Errorf("criterion %q is not present in parent revision %q", criterionID, input.parent)
			}
			if revision.AcceptanceCriteria[found].Statement == strings.TrimSpace(input.statement) {
				return fmt.Errorf("criterion %q statement is unchanged", criterionID)
			}
			revision.AcceptanceCriteria[found].Statement = strings.TrimSpace(input.statement)
		case criterionRemove:
			if found < 0 {
				return fmt.Errorf("criterion %q is not present in parent revision %q", criterionID, input.parent)
			}
			revision.AcceptanceCriteria = append(revision.AcceptanceCriteria[:found:found], revision.AcceptanceCriteria[found+1:]...)
		}
		if err := validateRevision(revision, sagaID, storyRef.ID); err != nil {
			return err
		}
		path := revisionPath(story.Feature, storyRef.ID, revision.ID)
		if existingRevision != nil {
			if input.requestID != "" && existingRevision.RequestID == input.requestID && equalRevisionIgnoringTime(*existingRevision, revision) {
				result = MutationResult{
					URN: criterionIDURN, Path: path, Created: []string{revisionIDURN}, Paths: []string{path},
					CurrentHeads: copyStrings(story.RevisionHeads), Reason: strings.TrimSpace(input.reason), Replayed: true,
				}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", revision.ID)
		}
		if len(story.Revisions) >= MaxRevisionsPerStory {
			return fmt.Errorf("revision limit of %d reached", MaxRevisionsPerStory)
		}
		if input.kind == criterionAdd {
			for _, historical := range story.Revisions {
				for _, criterion := range historical.AcceptanceCriteria {
					if criterion.ID == criterionID {
						return fmt.Errorf("criterion id %q was used previously and cannot be reused", criterionID)
					}
				}
			}
		}
		if err := requireCitations(document, revision.Citations); err != nil {
			return err
		}
		candidate := *story
		candidate.Revisions = append(append([]Revision{}, story.Revisions...), revision)
		if err := validateStoryGraphs(&candidate, sagaID, document.citationIDs(), document.personaIDs()); err != nil {
			return err
		}
		fullPath := filepath.Join(document.Root, filepath.FromSlash(path))
		if err := store.WriteJSON(fullPath, revision, true); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("revision id %q already exists", revision.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{
			URN: criterionIDURN, Path: path, Created: []string{revisionIDURN}, Paths: []string{path},
			CurrentHeads: []string{revisionIDURN}, Reason: strings.TrimSpace(input.reason),
		}
		return nil
	})
	return result, err
}

func SetStoryState(root, sagaID string, input SetStoryStateInput) (MutationResult, error) {
	storyRef, err := livingid.Parse(input.Story)
	if err != nil || storyRef.Kind != livingid.KindStory || storyRef.SagaID != sagaID {
		return MutationResult{}, fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	event := LifecycleEvent{
		Schema: LifecycleEventSchemaURL, Version: Version, ID: input.ID, Story: input.Story, Parents: copyStrings(input.Parents),
		State: input.State, Reason: strings.TrimSpace(input.Reason), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
	}
	if err := validateEvent(event, sagaID, storyRef.ID); err != nil {
		return MutationResult{}, err
	}
	eventURN, _ := StoryEventURN(sagaID, storyRef.ID, event.ID)
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		story := findStory(document, storyRef.ID)
		if story == nil {
			return fmt.Errorf("story %q does not exist", storyRef.ID)
		}
		for _, existing := range story.Events {
			if existing.ID != event.ID {
				continue
			}
			if input.RequestID != "" && existing.RequestID == input.RequestID && equalEventIgnoringTime(existing, event) {
				result = MutationResult{URN: eventURN, Path: eventPath(story.Feature, storyRef.ID, event.ID), Created: []string{eventURN}, Paths: []string{eventPath(story.Feature, storyRef.ID, event.ID)}, CurrentHeads: copyStrings(story.LifecycleHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", event.ID)
		}
		if len(story.Events) >= MaxEventsPerStory {
			return fmt.Errorf("lifecycle event limit of %d reached", MaxEventsPerStory)
		}
		if !sameSet(event.Parents, story.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, story.LifecycleHeads)
		}
		candidate := *story
		candidate.Events = append(append([]LifecycleEvent{}, story.Events...), event)
		if err := validateStoryGraphs(&candidate, sagaID, document.citationIDs(), document.personaIDs()); err != nil {
			return err
		}
		path := filepath.Join(document.Root, filepath.FromSlash(eventPath(story.Feature, storyRef.ID, event.ID)))
		if err := store.WriteJSON(path, event, true); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("lifecycle event id %q already exists", event.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{URN: eventURN, Path: eventPath(story.Feature, storyRef.ID, event.ID), Created: []string{eventURN}, Paths: []string{eventPath(story.Feature, storyRef.ID, event.ID)}, CurrentHeads: []string{eventURN}}
		return nil
	})
	return result, err
}

func AddCitation(root, sagaID string, input AddCitationInput) (MutationResult, error) {
	value := Citation{
		Schema: CitationSchemaURL, Version: Version, ID: input.ID, Kind: input.Kind,
		Title: strings.TrimSpace(input.Title), Reference: strings.TrimSpace(input.Reference),
		CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID, Feature: input.Feature,
	}
	if err := validateCitation(value, sagaID, input.ID); err != nil {
		return MutationResult{}, err
	}
	urn, _ := citationURN(sagaID, input.ID)
	var result MutationResult
	err := mutate(root, sagaID, func(document *Document) error {
		for _, existing := range document.Citations {
			if existing.ID != input.ID {
				continue
			}
			if input.RequestID != "" && existing.RequestID == input.RequestID && equalCitationIgnoringTime(existing, value) {
				result = MutationResult{URN: urn, Path: citationPath(existing.Feature, input.ID), Replayed: true}
				return nil
			}
			return fmt.Errorf("citation id %q already exists; citations are immutable", input.ID)
		}
		if len(document.Citations) >= MaxCitations {
			return fmt.Errorf("citation limit of %d reached", MaxCitations)
		}
		feature, err := document.feature(input.Feature)
		if err != nil {
			return err
		}
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(feature.Dir, applayout.RequirementsDir, "citations"))
		if err != nil {
			return err
		}
		path := filepath.Join(dir, input.ID+".json")
		if err := store.WriteJSON(path, value, true); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("citation id %q already exists; citations are immutable", input.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: citationPath(input.Feature, input.ID)}
		return nil
	})
	return result, err
}

func AddRelation(root, sagaID string, input AddRelationInput) (MutationResult, error) {
	if input.Type == RelationConflictsWith && input.To < input.From {
		input.From, input.To = input.To, input.From
		input.FromRevision, input.ToRevision = input.ToRevision, input.FromRevision
		input.FromContentDigest, input.ToContentDigest = input.ToContentDigest, input.FromContentDigest
	}
	value := Relation{
		Schema: V5RelationSchemaURL, Version: V5RelationVersion, ID: input.ID, Type: input.Type,
		From: input.From, To: input.To, Rationale: strings.TrimSpace(input.Rationale),
		FromRevision: input.FromRevision, ToRevision: input.ToRevision,
		FromContentDigest: input.FromContentDigest, ToContentDigest: input.ToContentDigest,
		State: RelationActive, CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
		Scope: input.Scope, Feature: input.Feature,
	}
	if value.Scope == "" {
		value.Scope = ScopeSelf
	}
	if err := validateRelation(value, sagaID, input.ID); err != nil {
		return MutationResult{}, err
	}
	urn, _ := relationURN(sagaID, input.ID)
	var result MutationResult
	err := mutate(root, sagaID, func(document *Document) error {
		for _, existing := range document.Relations {
			if existing.ID != input.ID {
				continue
			}
			if input.RequestID != "" && existing.RequestID == input.RequestID && equalRelationIgnoringTime(existing, value) {
				result = MutationResult{URN: urn, Path: relationPath(existing.Feature, input.ID), Replayed: true}
				return nil
			}
			return fmt.Errorf("relation id %q already exists", input.ID)
		}
		if len(document.Relations) >= MaxRelations {
			return fmt.Errorf("relation limit of %d reached", MaxRelations)
		}
		// Preserve the frozen reader contract and idempotent replays of old
		// records, but require precise ownership for every new visual link.
		if value.Type == RelationExplains || value.Type == RelationAddresses {
			from, _ := parseEndpoint(value.From)
			if from.Kind == endpointDeck || from.Kind == endpointSlide {
				return fmt.Errorf("story links must originate from a slide Item, not a whole deck or slide; link each relevant element (scope self); the slide summary is derived from its Items")
			}
		}
		feature, err := document.feature(input.Feature)
		if err != nil {
			return err
		}
		candidate := *document
		candidate.Relations = append(append([]Relation{}, document.Relations...), value)
		if err := validateRelationSet(&candidate); err != nil {
			return err
		}
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(feature.Dir, applayout.RequirementsDir, "relations"))
		if err != nil {
			return err
		}
		path := filepath.Join(dir, input.ID+".json")
		if err := store.WriteJSON(path, value, true); errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("relation id %q already exists", input.ID)
		} else if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: relationPath(input.Feature, input.ID)}
		return nil
	})
	return result, err
}

// SupersedeRelation explicitly deactivates one relation without retargeting it.
// The replacement is atomic, serialized, and preserves all endpoint pins.
func SupersedeRelation(root, sagaID, relation string, at time.Time, requestID string) (MutationResult, error) {
	ref, err := livingid.Parse(relation)
	if err != nil || ref.Kind != livingid.KindRelation || ref.SagaID != sagaID {
		return MutationResult{}, fmt.Errorf("relation must be a canonical relation URN in saga %q", sagaID)
	}
	if requestID != "" && !livingid.ValidID(requestID) {
		return MutationResult{}, fmt.Errorf("request_id must be a stable identifier")
	}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		var existing *Relation
		for index := range document.Relations {
			if document.Relations[index].ID == ref.ID {
				existing = &document.Relations[index]
				break
			}
		}
		if existing == nil {
			return fmt.Errorf("relation %q does not exist", ref.ID)
		}
		if existing.State == RelationSuperseded {
			if requestID != "" && existing.SupersedeRequestID == requestID {
				result = MutationResult{URN: relation, Path: relationPath(existing.Feature, ref.ID), Replayed: true}
				return nil
			}
			return fmt.Errorf("relation %q is already superseded", ref.ID)
		}
		when := mutationTime(at)
		existing.State = RelationSuperseded
		existing.SupersededAt = &when
		existing.SupersedeRequestID = requestID
		if err := validateRelation(*existing, sagaID, ref.ID); err != nil {
			return err
		}
		path := filepath.Join(document.Root, filepath.FromSlash(relationPath(existing.Feature, ref.ID)))
		if err := store.WriteJSON(path, *existing, false); err != nil {
			return err
		}
		result = MutationResult{URN: relation, Path: relationPath(existing.Feature, ref.ID)}
		return nil
	})
	return result, err
}

func mutate(root, sagaID string, operation func(*Document) error) error {
	if _, err := Load(root, sagaID); err != nil {
		return fmt.Errorf("cannot mutate requirements: %w", err)
	}
	return store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, err := Load(root, sagaID)
		if err != nil {
			return fmt.Errorf("cannot mutate requirements after locking: %w", err)
		}
		return operation(&document)
	})
}

func requireCitations(document *Document, citations []string) error {
	known := map[string]bool{}
	for _, citation := range document.Citations {
		urn, _ := citationURN(document.SagaID, citation.ID)
		known[urn] = true
	}
	for _, citation := range citations {
		if !known[citation] {
			return fmt.Errorf("citation %q does not exist", citation)
		}
	}
	return nil
}

// requirePersonas checks the personas a new story revision names: each must
// exist and be active. A retired persona may stay on older revisions, where it
// is reported, but is never newly assigned.
func requirePersonas(document *Document, personas []string) error {
	for _, persona := range personas {
		id, err := ParsePersonaURN(document.SagaID, persona)
		if err != nil {
			return err
		}
		found := document.FindPersona(id)
		if found == nil {
			return fmt.Errorf("persona %q does not exist", persona)
		}
		if found.CurrentLifecycle != nil && found.CurrentLifecycle.State == PersonaRetired {
			return fmt.Errorf("persona %q is retired; a story cannot newly serve it", persona)
		}
	}
	return nil
}

func findStory(document *Document, id string) *Story {
	for index := range document.Stories {
		if document.Stories[index].Identity.ID == id {
			return &document.Stories[index]
		}
	}
	return nil
}

func sameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a, b := copyStrings(left), copyStrings(right)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

func mutationTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func ensureStageDir(path string) error {
	// CommitDir owns and cleans the staging tree; these two fixed subdirectories
	// cannot escape it and do not need the saga-root path builder.
	return os.Mkdir(path, 0o755)
}

func storyPackagePath(feature, id string) string {
	return applayout.FeatureRel(feature) + "/" + applayout.RequirementsDir + "/stories/" + id + ".story"
}
func revisionPath(feature, storyID, id string) string {
	return storyPackagePath(feature, storyID) + "/revisions/" + id + ".json"
}
func eventPath(feature, storyID, id string) string {
	return storyPackagePath(feature, storyID) + "/events/" + id + ".json"
}
func citationPath(feature, id string) string {
	return applayout.FeatureRel(feature) + "/" + applayout.RequirementsDir + "/citations/" + id + ".json"
}
func relationPath(feature, id string) string {
	return applayout.FeatureRel(feature) + "/" + applayout.RequirementsDir + "/relations/" + id + ".json"
}

// feature resolves the feature an authoring operation writes new content into.
func (document *Document) feature(id string) (applayout.Feature, error) {
	if strings.TrimSpace(id) == "" {
		return applayout.Feature{}, fmt.Errorf("a feature is required; new feature content is never written to an implied feature")
	}
	feature, ok := applayout.Find(document.Features, id)
	if !ok {
		return applayout.Feature{}, fmt.Errorf("feature %q does not exist", id)
	}
	return feature, nil
}

func (document *Document) citationIDs() map[string]bool {
	ids := map[string]bool{}
	for _, citation := range document.Citations {
		ids[citation.ID] = true
	}
	return ids
}

func copyStrings(values []string) []string        { return append([]string{}, values...) }
func copyCriteria(values []Criterion) []Criterion { return append([]Criterion{}, values...) }

func equalStoryCreation(existing Story, identity StoryIdentity, revision Revision, event LifecycleEvent) bool {
	identity.CreatedAt = existing.Identity.CreatedAt
	if !reflect.DeepEqual(existing.Identity, identity) {
		return false
	}
	var storedRevision *Revision
	for index := range existing.Revisions {
		if existing.Revisions[index].ID == revision.ID {
			storedRevision = &existing.Revisions[index]
			break
		}
	}
	var storedEvent *LifecycleEvent
	for index := range existing.Events {
		if existing.Events[index].ID == event.ID {
			storedEvent = &existing.Events[index]
			break
		}
	}
	if storedRevision == nil || storedEvent == nil {
		return false
	}
	revision.CreatedAt = storedRevision.CreatedAt
	event.CreatedAt = storedEvent.CreatedAt
	return reflect.DeepEqual(*storedRevision, revision) && reflect.DeepEqual(*storedEvent, event)
}

func equalRevisionIgnoringTime(existing, wanted Revision) bool {
	wanted.CreatedAt = existing.CreatedAt
	return reflect.DeepEqual(existing, wanted)
}

func equalEventIgnoringTime(existing, wanted LifecycleEvent) bool {
	wanted.CreatedAt = existing.CreatedAt
	return reflect.DeepEqual(existing, wanted)
}

func equalCitationIgnoringTime(existing, wanted Citation) bool {
	wanted.CreatedAt = existing.CreatedAt
	return reflect.DeepEqual(existing, wanted)
}

func equalRelationIgnoringTime(existing, wanted Relation) bool {
	wanted.CreatedAt = existing.CreatedAt
	existing.State = RelationActive
	existing.SupersededAt = nil
	existing.SupersedeRequestID = ""
	existing.Stale = false
	existing.StaleReasons = nil
	return reflect.DeepEqual(existing, wanted)
}
