package requirements

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/store"
)

type AddPersonaInput struct {
	ID          string
	RevisionID  string
	EventID     string
	Name        string
	Description string
	CreatedAt   time.Time
	RequestID   string
}

type RevisePersonaInput struct {
	Persona     string
	ID          string
	Parents     []string
	Name        string
	Description string
	CreatedAt   time.Time
	RequestID   string
}

type SetPersonaStateInput struct {
	Persona   string
	ID        string
	Parents   []string
	State     PersonaState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

type AddFlagInput struct {
	ID          string
	RevisionID  string
	EventID     string
	Description string
	Targets     []string
	State       FlagState
	CreatedAt   time.Time
	RequestID   string
}

type ReviseFlagInput struct {
	Flag        string
	ID          string
	Parents     []string
	Description string
	Targets     []string
	CreatedAt   time.Time
	RequestID   string
}

type SetFlagStateInput struct {
	Flag      string
	ID        string
	Parents   []string
	State     FlagState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

// MoveStoryInput moves a story package to another feature. Identity, revisions,
// lifecycle, and every relation, pin, and reference to the story are
// unchanged; only the directory that holds it moves.
type MoveStoryInput struct {
	Story   string
	Feature string
}

func personaPackagePath(id string) string {
	return applayout.PersonasDir + "/" + id + ".persona"
}

func flagPackagePath(id string) string {
	return applayout.FeatureFlagsDir + "/" + id + ".flag"
}

// createAppPackage publishes one identity, revision, and event atomically.
func createAppPackage(root, final, identityName string, identity, revision, event any, revisionID, eventID string) error {
	return store.CommitDir(root, final, func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		if err := store.WriteJSON(filepath.Join(stage, identityName), identity, true); err != nil {
			return err
		}
		for _, dir := range []string{"revisions", "events"} {
			if err := ensureStageDir(filepath.Join(stage, dir)); err != nil {
				return err
			}
		}
		if err := store.WriteJSON(filepath.Join(stage, "revisions", revisionID+".json"), revision, true); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(stage, "events", eventID+".json"), event, true)
	})
}

func AddPersona(root, sagaID string, input AddPersonaInput) (MutationResult, error) {
	createdAt := mutationTime(input.CreatedAt)
	urn, err := PersonaURN(sagaID, input.ID)
	if err != nil {
		return MutationResult{}, err
	}
	identity := RecordIdentity{Schema: PersonaSchemaURL, Version: AppRecordVersion, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
	revision := PersonaRevision{Schema: PersonaRevisionSchemaURL, Version: AppRecordVersion, ID: input.RevisionID, Persona: urn, Parents: []string{},
		Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), CreatedAt: createdAt, RequestID: input.RequestID}
	event := PersonaEvent{Schema: PersonaEventSchemaURL, Version: AppRecordVersion, ID: input.EventID, Persona: urn, Parents: []string{},
		State: PersonaActive, CreatedAt: createdAt, RequestID: input.RequestID}
	if err := validateRequestIDValue(input.RequestID); err != nil {
		return MutationResult{}, err
	}
	if err := validatePersonaRevision(revision, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	if err := validatePersonaEvent(event, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	revisionURN, _ := PersonaRevisionURN(sagaID, input.ID, input.RevisionID)
	eventURN, _ := PersonaEventURN(sagaID, input.ID, input.EventID)
	path := personaPackagePath(input.ID)
	paths := []string{path, path + "/revisions/" + input.RevisionID + ".json", path + "/events/" + input.EventID + ".json"}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		if existing := document.FindPersona(input.ID); existing != nil {
			identity.CreatedAt, revision.CreatedAt, event.CreatedAt = existing.Identity.CreatedAt, existing.Identity.CreatedAt, existing.Identity.CreatedAt
			if input.RequestID != "" && existing.Identity.RequestID == input.RequestID && len(existing.Revisions) > 0 && len(existing.Events) > 0 &&
				reflect.DeepEqual(existing.Identity, identity) && reflect.DeepEqual(existing.Revisions[0], revision) && reflect.DeepEqual(existing.Events[0], event) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...), Replayed: true}
				return nil
			}
			return fmt.Errorf("persona id %q already exists", input.ID)
		}
		if len(document.Personas) >= MaxPersonas {
			return fmt.Errorf("persona limit of %d reached", MaxPersonas)
		}
		err := createAppPackage(document.Root, filepath.Join(document.Root, filepath.FromSlash(path)), "persona.json", identity, revision, event, input.RevisionID, input.EventID)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("persona id %q already exists", input.ID)
		}
		if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: []string{revisionURN, eventURN}}
		return nil
	})
	return result, err
}

func RevisePersona(root, sagaID string, input RevisePersonaInput) (MutationResult, error) {
	id, err := ParsePersonaURN(sagaID, input.Persona)
	if err != nil {
		return MutationResult{}, err
	}
	revision := PersonaRevision{Schema: PersonaRevisionSchemaURL, Version: AppRecordVersion, ID: input.ID, Persona: input.Persona, Parents: copyStrings(input.Parents),
		Name: strings.TrimSpace(input.Name), Description: strings.TrimSpace(input.Description), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	if err := validatePersonaRevision(revision, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := PersonaRevisionURN(sagaID, id, input.ID)
	path := personaPackagePath(id) + "/revisions/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		persona := document.FindPersona(id)
		if persona == nil {
			return fmt.Errorf("persona %q does not exist", id)
		}
		for _, existing := range persona.Revisions {
			if existing.ID != input.ID {
				continue
			}
			wanted := revision
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(persona.RevisionHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", input.ID)
		}
		if !sameSet(revision.Parents, persona.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, persona.RevisionHeads)
		}
		candidate := *persona
		candidate.Revisions = append(append([]PersonaRevision{}, persona.Revisions...), revision)
		if err := resolvePersona(&candidate, sagaID); err != nil {
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

func SetPersonaState(root, sagaID string, input SetPersonaStateInput) (MutationResult, error) {
	id, err := ParsePersonaURN(sagaID, input.Persona)
	if err != nil {
		return MutationResult{}, err
	}
	event := PersonaEvent{Schema: PersonaEventSchemaURL, Version: AppRecordVersion, ID: input.ID, Persona: input.Persona, Parents: copyStrings(input.Parents),
		State: input.State, Reason: strings.TrimSpace(input.Reason), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	if err := validatePersonaEvent(event, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := PersonaEventURN(sagaID, id, input.ID)
	path := personaPackagePath(id) + "/events/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		persona := document.FindPersona(id)
		if persona == nil {
			return fmt.Errorf("persona %q does not exist", id)
		}
		for _, existing := range persona.Events {
			if existing.ID != input.ID {
				continue
			}
			wanted := event
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(persona.LifecycleHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", input.ID)
		}
		if !sameSet(event.Parents, persona.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, persona.LifecycleHeads)
		}
		candidate := *persona
		candidate.Events = append(append([]PersonaEvent{}, persona.Events...), event)
		if err := resolvePersona(&candidate, sagaID); err != nil {
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

func AddFlag(root, sagaID string, input AddFlagInput) (MutationResult, error) {
	createdAt := mutationTime(input.CreatedAt)
	urn, err := FlagURN(sagaID, input.ID)
	if err != nil {
		return MutationResult{}, err
	}
	if input.State == "" {
		input.State = FlagOff
	}
	identity := RecordIdentity{Schema: FlagSchemaURL, Version: AppRecordVersion, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
	revision := FlagRevision{Schema: FlagRevisionSchemaURL, Version: AppRecordVersion, ID: input.RevisionID, Flag: urn, Parents: []string{},
		Description: strings.TrimSpace(input.Description), Targets: copyStrings(input.Targets), CreatedAt: createdAt, RequestID: input.RequestID}
	event := FlagEvent{Schema: FlagEventSchemaURL, Version: AppRecordVersion, ID: input.EventID, Flag: urn, Parents: []string{},
		State: input.State, CreatedAt: createdAt, RequestID: input.RequestID}
	if err := validateRequestIDValue(input.RequestID); err != nil {
		return MutationResult{}, err
	}
	if err := validateFlagEvent(event, sagaID, input.ID, ""); err != nil {
		return MutationResult{}, err
	}
	if input.State == FlagRetired {
		return MutationResult{}, fmt.Errorf("a new flag starts off or on")
	}
	revisionURN, _ := FlagRevisionURN(sagaID, input.ID, input.RevisionID)
	eventURN, _ := FlagEventURN(sagaID, input.ID, input.EventID)
	path := flagPackagePath(input.ID)
	paths := []string{path, path + "/revisions/" + input.RevisionID + ".json", path + "/events/" + input.EventID + ".json"}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		if err := validateFlagRevision(revision, document, input.ID, ""); err != nil {
			return err
		}
		if existing := document.FindFlag(input.ID); existing != nil {
			identity.CreatedAt, revision.CreatedAt, event.CreatedAt = existing.Identity.CreatedAt, existing.Identity.CreatedAt, existing.Identity.CreatedAt
			if input.RequestID != "" && existing.Identity.RequestID == input.RequestID && len(existing.Revisions) > 0 && len(existing.Events) > 0 &&
				reflect.DeepEqual(existing.Identity, identity) && reflect.DeepEqual(existing.Revisions[0], revision) && reflect.DeepEqual(existing.Events[0], event) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...), Replayed: true}
				return nil
			}
			return fmt.Errorf("flag id %q already exists", input.ID)
		}
		if len(document.Flags) >= MaxFlags {
			return fmt.Errorf("flag limit of %d reached", MaxFlags)
		}
		err := createAppPackage(document.Root, filepath.Join(document.Root, filepath.FromSlash(path)), "flag.json", identity, revision, event, input.RevisionID, input.EventID)
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("flag id %q already exists", input.ID)
		}
		if err != nil {
			return err
		}
		result = MutationResult{URN: urn, Path: path, Created: []string{urn, revisionURN, eventURN}, Paths: paths, CurrentHeads: []string{revisionURN, eventURN}}
		return nil
	})
	return result, err
}

func ReviseFlag(root, sagaID string, input ReviseFlagInput) (MutationResult, error) {
	id, err := ParseFlagURN(sagaID, input.Flag)
	if err != nil {
		return MutationResult{}, err
	}
	revision := FlagRevision{Schema: FlagRevisionSchemaURL, Version: AppRecordVersion, ID: input.ID, Flag: input.Flag, Parents: copyStrings(input.Parents),
		Description: strings.TrimSpace(input.Description), Targets: copyStrings(input.Targets), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	urn, _ := FlagRevisionURN(sagaID, id, input.ID)
	path := flagPackagePath(id) + "/revisions/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		if err := validateFlagRevision(revision, document, id, ""); err != nil {
			return err
		}
		flag := document.FindFlag(id)
		if flag == nil {
			return fmt.Errorf("flag %q does not exist", id)
		}
		for _, existing := range flag.Revisions {
			if existing.ID != input.ID {
				continue
			}
			wanted := revision
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(flag.RevisionHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("revision id %q already exists", input.ID)
		}
		if !sameSet(revision.Parents, flag.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, flag.RevisionHeads)
		}
		candidate := *flag
		candidate.Revisions = append(append([]FlagRevision{}, flag.Revisions...), revision)
		if err := resolveFlag(&candidate, sagaID); err != nil {
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

func SetFlagState(root, sagaID string, input SetFlagStateInput) (MutationResult, error) {
	id, err := ParseFlagURN(sagaID, input.Flag)
	if err != nil {
		return MutationResult{}, err
	}
	event := FlagEvent{Schema: FlagEventSchemaURL, Version: AppRecordVersion, ID: input.ID, Flag: input.Flag, Parents: copyStrings(input.Parents),
		State: input.State, Reason: strings.TrimSpace(input.Reason), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID}
	if err := validateFlagEvent(event, sagaID, id, ""); err != nil {
		return MutationResult{}, err
	}
	urn, _ := FlagEventURN(sagaID, id, input.ID)
	path := flagPackagePath(id) + "/events/" + input.ID + ".json"
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		flag := document.FindFlag(id)
		if flag == nil {
			return fmt.Errorf("flag %q does not exist", id)
		}
		for _, existing := range flag.Events {
			if existing.ID != input.ID {
				continue
			}
			wanted := event
			wanted.CreatedAt = existing.CreatedAt
			if input.RequestID != "" && existing.RequestID == input.RequestID && reflect.DeepEqual(existing, wanted) {
				result = MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(flag.LifecycleHeads), Replayed: true}
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", input.ID)
		}
		if !sameSet(event.Parents, flag.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, flag.LifecycleHeads)
		}
		candidate := *flag
		candidate.Events = append(append([]FlagEvent{}, flag.Events...), event)
		if err := resolveFlag(&candidate, sagaID); err != nil {
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

// MoveStory renames the story package into another feature's stories directory.
// Moving a story into the feature that already holds it is a replay.
func MoveStory(root, sagaID string, input MoveStoryInput) (MutationResult, error) {
	id, err := parseStoryURN(sagaID, input.Story)
	if err != nil {
		return MutationResult{}, err
	}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		story := findStory(document, id)
		if story == nil {
			return fmt.Errorf("story %q does not exist", id)
		}
		target, err := document.feature(input.Feature)
		if err != nil {
			return err
		}
		to := storyPackagePath(target.ID, id)
		if story.Feature == target.ID {
			result = MutationResult{URN: input.Story, Path: to, Paths: []string{to}, Replayed: true}
			return nil
		}
		from := storyPackagePath(story.Feature, id)
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(target.Dir, applayout.RequirementsDir, "stories"))
		if err != nil {
			return err
		}
		final := filepath.Join(dir, id+".story")
		if _, err := os.Lstat(final); err == nil {
			return fmt.Errorf("%s already exists", to)
		}
		if err := os.Rename(filepath.Join(document.Root, filepath.FromSlash(from)), final); err != nil {
			return err
		}
		result = MutationResult{URN: input.Story, Path: to, Paths: []string{from, to}}
		return nil
	})
	return result, err
}

func parseStoryURN(sagaID, value string) (string, error) {
	prefix := "urn:change-saga:" + sagaID + ":story:"
	id := strings.TrimPrefix(value, prefix)
	if id == value || !applayout.ValidID(id) {
		return "", fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	return id, nil
}

func validateRequestIDValue(value string) error {
	var problems validationErrors
	validateRequestID(&problems, value)
	return problems.err()
}
