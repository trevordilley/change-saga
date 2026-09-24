package requirements

import (
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/twentyideas/changesaga/internal/store"
)

// WriteTechnical publishes an identity + initial revision/event atomically, or
// appends one full revision. Every parent must be an observed current head.
func WriteTechnical(root, sagaID, kind, id, revisionID string, parents []string, definition TechnicalDefinition, create bool) (MutationResult, error) {
	urn, err := TechnicalURN(sagaID, kind, id)
	if err != nil {
		return MutationResult{}, err
	}
	now := mutationTime(time.Time{})
	v := TechnicalRevision{Schema: TechnicalSchema(kind, "-revision"), Version: 5, ID: revisionID, Record: urn, Parents: copyStrings(parents), TechnicalDefinition: definition, CreatedAt: now}
	if err := validateTechnicalRevision(v, kind, urn, revisionID); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{URN: urn, Path: TechnicalPath(kind, id)}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		d, err := LoadInventory(root, sagaID)
		if err != nil {
			return err
		}
		found := d.Find(urn)
		if found != nil {
			if old := found.Revision(urn + ":revision:" + revisionID); old != nil {
				v.CreatedAt = old.CreatedAt
				if reflect.DeepEqual(*old, v) && (create == (len(old.Parents) == 0)) {
					result.Replayed = true
					result.CurrentHeads = found.RevisionHeads
					return nil
				}
				return fmt.Errorf("revision %s already exists with different content", revisionID)
			}
		}
		if create {
			if found != nil {
				return fmt.Errorf("%s already exists", urn)
			}
			if len(parents) != 0 {
				return fmt.Errorf("initial revision cannot have parents")
			}
			if len(d.Records) >= MaxInventoryRecords {
				return fmt.Errorf("inventory record limit reached")
			}
		} else {
			if found == nil {
				return fmt.Errorf("%s does not exist", urn)
			}
			if !sameSet(parents, found.RevisionHeads) {
				return fmt.Errorf("parents must name every current revision head: %v", found.RevisionHeads)
			}
		}
		for _, pin := range definition.Components {
			retained := false
			if found != nil && found.CurrentRevision != nil {
				for _, old := range found.CurrentRevision.Components {
					if old == pin {
						retained = true
					}
				}
			}
			if retained {
				continue
			}
			if status := d.LinkStatus(pin); status != "current" {
				return fmt.Errorf("Component %s is %s", pin.Target, status)
			}
		}
		path := filepath.Join(root, filepath.FromSlash(result.Path))
		if create {
			identity := RecordIdentity{Schema: TechnicalSchema(kind, ""), Version: 5, ID: id, CreatedAt: now}
			event := TechnicalEvent{Schema: TechnicalSchema(kind, "-event"), Version: 5, ID: "active", Record: urn, Parents: []string{}, State: "active", CreatedAt: now}
			if err := createAppPackage(root, path, kind+".json", identity, v, event, revisionID, "active"); err != nil {
				return err
			}
		} else {
			if len(found.Revisions) >= MaxRevisionsPerStory {
				return fmt.Errorf("revision limit reached")
			}
			if err := store.WriteJSON(filepath.Join(path, "revisions", revisionID+".json"), v, true); err != nil {
				return err
			}
		}
		result.Created = []string{urn + ":revision:" + revisionID}
		result.CurrentHeads = result.Created
		return nil
	})
	return result, err
}

func SetTechnicalState(root, sagaID, kind, id, eventID, state, reason string, parents []string) (MutationResult, error) {
	urn, err := TechnicalURN(sagaID, kind, id)
	if err != nil {
		return MutationResult{}, err
	}
	v := TechnicalEvent{Schema: TechnicalSchema(kind, "-event"), Version: 5, ID: eventID, Record: urn, Parents: copyStrings(parents), State: state, Reason: reason, CreatedAt: mutationTime(time.Time{})}
	if err := validateTechnicalEvent(v, kind, urn, eventID); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{URN: urn, Path: TechnicalPath(kind, id)}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		d, err := LoadInventory(root, sagaID)
		if err != nil {
			return err
		}
		found := d.Find(urn)
		if found == nil {
			return fmt.Errorf("%s does not exist", urn)
		}
		for _, old := range found.Events {
			if old.ID == eventID {
				v.CreatedAt = old.CreatedAt
				if reflect.DeepEqual(old, v) {
					result.Replayed = true
					result.CurrentHeads = found.LifecycleHeads
					return nil
				}
				return fmt.Errorf("event already exists with different content")
			}
		}
		if !sameSet(parents, found.LifecycleHeads) {
			return fmt.Errorf("parents must name every current lifecycle head: %v", found.LifecycleHeads)
		}
		if found.CurrentLifecycle != nil && found.CurrentLifecycle.State == state {
			return fmt.Errorf("lifecycle state is already %s", state)
		}
		if len(found.Events) >= MaxRevisionsPerStory {
			return fmt.Errorf("lifecycle limit reached")
		}
		if err := store.WriteJSON(filepath.Join(root, filepath.FromSlash(result.Path), "events", eventID+".json"), v, true); err != nil {
			return err
		}
		result.Created = []string{urn + ":event:" + eventID}
		result.CurrentHeads = result.Created
		return nil
	})
	return result, err
}
