package requirements

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

// WhiteboardElementTarget is the stable owner of an element's evidence and links.
// It deliberately excludes the revision: publishing reassessed content keeps its identity.
func WhiteboardElementTarget(sagaID, boardID, elementID string) string {
	return "urn:change-saga:" + sagaID + ":whiteboard:" + boardID + ":element:" + elementID
}

// WhiteboardItem projects only semantic metadata. Geometry and drawing kinds stay
// in the diagram; this is not a synthetic slide or a second persisted Item.
func WhiteboardItem(sagaID, boardID string, revision WhiteboardRevision, e diagram.Element) *saga.Item {
	path := filepath.ToSlash(filepath.Join(WhiteboardDir, boardID, "revisions", revision.ID+".json"))
	return &saga.Item{ItemManifest: saga.ItemManifest{ID: e.ID, Label: diagram.AccessibleName(e), Description: e.Description, Record: e.Record, Documentation: e.Documentation, DocumentationView: e.DocumentationView, Selections: e.Selections}, Target: WhiteboardElementTarget(sagaID, boardID, e.ID), Path: path,
		Code: []saga.CodeFile{{Path: path + "#element=" + e.ID, References: e.Evidence}}, CriterionLinks: e.CriterionLinks, HasCode: len(e.Evidence) > 0}
}

// CurrentWhiteboardItems visits unique current heads only. Historical revisions
// remain inspectable but never inflate live coverage or keep repaired debt alive.
func CurrentWhiteboardItems(root, sagaID string) ([]*saga.Item, error) {
	ids, err := ListWhiteboardIDs(root)
	if err != nil {
		return nil, err
	}
	metadata, err := LoadWhiteboardMetadata(root, sagaID)
	if err != nil {
		return nil, err
	}
	var items []*saga.Item
	for _, id := range ids {
		if metadata.Boards[id].Archived {
			continue
		}
		revision, err := LoadWhiteboard(root, sagaID, id, "")
		if err != nil {
			// A broken board must not hide independently readable siblings. Full
			// history validation reports the error; no replacement head is invented.
			continue
		}
		if revision == nil {
			continue
		}
		for _, e := range revision.Document.Elements {
			if !e.Decorative {
				items = append(items, WhiteboardItem(sagaID, id, *revision, e))
			}
		}
	}
	return items, nil
}

func validateWhiteboardItems(sagaID string, doc diagram.Document) error {
	seen := map[string]bool{}
	for _, e := range doc.Elements {
		if e.Documentation != nil && !saga.ValidDocumentationLink(sagaID, *e.Documentation) {
			return fmt.Errorf("%s: documentation needs an exact Component, System or data-entity revision", e.ID)
		}
		if e.Record != "" {
			valid := false
			for _, kind := range saga.RecordReferenceKinds {
				prefix := "urn:change-saga:" + sagaID + ":" + kind + ":"
				if strings.HasPrefix(e.Record, prefix) && saga.ValidID(strings.TrimPrefix(e.Record, prefix)) {
					valid = true
				}
			}
			if !valid {
				return fmt.Errorf("%s: invalid record link", e.ID)
			}
		}
		for _, problem := range saga.ItemSelectionProblems(sagaID, saga.ItemManifest{Documentation: e.Documentation, DocumentationView: e.DocumentationView, Selections: e.Selections}) {
			return fmt.Errorf("%s: %s", e.ID, problem)
		}
		for _, link := range e.CriterionLinks {
			if err := saga.ValidateCriterionLink(sagaID, link); err != nil {
				return fmt.Errorf("%s: %w", e.ID, err)
			}
			if seen[link.ID] {
				return fmt.Errorf("duplicate criterion link %s", link.ID)
			}
			seen[link.ID] = true
		}
	}
	return nil
}

// Publication verifies that authored pins name saved definitions. Later removal
// or revision remains a currency problem, so historical reads do not call this.
func validateWhiteboardItemPins(root, sagaID, boardID string, doc diagram.Document) error {
	candidateIDs := map[string]bool{}
	for _, e := range doc.Elements {
		for _, link := range e.CriterionLinks {
			candidateIDs[link.ID] = true
		}
	}
	if len(candidateIDs) > 0 {
		records, err := Load(root, sagaID)
		if err != nil {
			return err
		}
		for _, relation := range records.Relations {
			if candidateIDs[relation.ID] {
				return fmt.Errorf("criterion link id %s is already a persisted relation", relation.ID)
			}
		}
		document, _, err := saga.Load(root)
		if err != nil {
			return err
		}
		for _, feature := range document.Features {
			for _, deck := range feature.Decks {
				for _, slide := range deck.Slides {
					for _, item := range slide.Items {
						for _, link := range item.CriterionLinks {
							if candidateIDs[link.ID] {
								return fmt.Errorf("criterion link id %s is already owned by %s", link.ID, item.Target)
							}
						}
					}
				}
			}
		}
		items, err := CurrentWhiteboardItems(root, sagaID)
		if err != nil {
			return err
		}
		prefix := "urn:change-saga:" + sagaID + ":whiteboard:" + boardID + ":element:"
		for _, item := range items {
			if strings.HasPrefix(item.Target, prefix) {
				continue
			}
			for _, link := range item.CriterionLinks {
				if candidateIDs[link.ID] {
					return fmt.Errorf("criterion link id %s is already owned by %s", link.ID, item.Target)
				}
			}
		}
	}

	var inventory *Inventory
	var records *Document
	for _, e := range doc.Elements {
		if e.Documentation != nil {
			if inventory == nil {
				value, err := LoadInventory(root, sagaID)
				if err != nil {
					return err
				}
				inventory = &value
			}
			if inventory.Pinned(*e.Documentation) == nil {
				return fmt.Errorf("%s: documentation revision %s is missing", e.ID, e.Documentation.Revision)
			}
		}
		for _, selection := range e.Selections {
			if e.Documentation == nil {
				return fmt.Errorf("%s: selections require documentation", e.ID)
			}
			if inventory == nil {
				value, err := LoadInventory(root, sagaID)
				if err != nil {
					return err
				}
				inventory = &value
			}
			if inventory.Format != 2 {
				return fmt.Errorf("%s: selections require inventory format 2", e.ID)
			}
			if _, err := inventory.ResolveSelection(*e.Documentation, selection); err != nil {
				return fmt.Errorf("%s: %w", e.ID, err)
			}
		}
		for _, link := range e.CriterionLinks {
			if records == nil {
				value, err := Load(root, sagaID)
				if err != nil {
					return err
				}
				records = &value
			}
			parts := strings.Split(link.Criterion, ":")
			story := records.FindStory(parts[4])
			found := false
			if story != nil {
				for _, revision := range story.Revisions {
					if link.StoryRevision == "urn:change-saga:"+sagaID+":story:"+story.Identity.ID+":revision:"+revision.ID {
						for _, criterion := range revision.AcceptanceCriteria {
							if criterion.ID == parts[6] {
								found = true
							}
						}
					}
				}
			}
			if !found {
				return fmt.Errorf("%s: criterion %s is missing from saved story revision %s", e.ID, link.Criterion, link.StoryRevision)
			}
		}
	}
	return nil
}
