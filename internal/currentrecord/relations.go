package currentrecord

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/livingid"

	"github.com/twentyideas/changesaga/internal/store"
)

const RelationSchema = "https://changesaga.dev/schema/v5/relation-current.schema.json"

// prepareRelations preserves the legacy effective current pins. The old format
// explicitly folds confirmations by created_at then ID; this is an existing
// projection, not a rule for choosing between competing content heads. The
// atomic current publication precedes removal of those obsolete confirmations.
func (p *Pins) prepareRelations() error {
	p.relations = map[string]json.RawMessage{}
	p.repinDirs = nil
	paths := make([]string, 0, len(p.files))
	for path := range p.files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		var relation map[string]json.RawMessage
		if err := json.Unmarshal(p.files[path], &relation); err != nil {
			return err
		}
		var schema, id string
		_ = json.Unmarshal(relation["$schema"], &schema)
		if schema != RelationSchema && schema != "https://changesaga.dev/schema/v5/relation.schema.json" {
			continue
		}
		_ = json.Unmarshal(relation["id"], &id)
		if id == "" || filepath.Base(filepath.Dir(path)) != "relations" {
			continue
		}
		dir := filepath.Join(filepath.Dir(filepath.Dir(path)), "relation-repins", id)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			entries = nil
		} else if err != nil {
			return err
		} else {
			p.repinDirs = append(p.repinDirs, dir)
		}
		if schema == RelationSchema {
			continue // a successfully published value wins over cleanup remnants
		}
		type confirmation struct {
			Schema            string    `json:"$schema"`
			Version           int       `json:"version"`
			ID                string    `json:"id"`
			Relation          string    `json:"relation"`
			FromRevision      string    `json:"from_revision,omitempty"`
			ToRevision        string    `json:"to_revision,omitempty"`
			FromContentDigest string    `json:"from_content_digest,omitempty"`
			ToContentDigest   string    `json:"to_content_digest,omitempty"`
			Rationale         string    `json:"rationale"`
			CreatedAt         time.Time `json:"created_at"`
			RequestID         string    `json:"request_id,omitempty"`
		}
		confirmations := []confirmation{}
		var from string
		_ = json.Unmarshal(relation["from"], &from)
		endpoint := strings.Split(from, ":")
		for _, entry := range entries {
			file := filepath.Join(dir, entry.Name())
			if !entry.Type().IsRegular() || filepath.Ext(file) != ".json" {
				return fmt.Errorf("%s must contain regular legacy confirmations", dir)
			}
			data := p.files[file]
			if len(data) == 0 {
				return fmt.Errorf("%s is not a recognized legacy confirmation", file)
			}
			var value confirmation
			if err := Decode(data, &value); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
			expectedID := strings.TrimSuffix(entry.Name(), ".json")
			if len(endpoint) < 5 || endpoint[0] != "urn" || endpoint[1] != "change-saga" ||
				value.Schema != "https://changesaga.dev/schema/v5/relation-repin.schema.json" || value.Version != 5 ||
				!livingid.ValidID(value.ID) || value.ID != expectedID ||
				value.Relation != "urn:change-saga:"+endpoint[2]+":relation:"+id ||
				value.CreatedAt.IsZero() || strings.TrimSpace(value.Rationale) == "" ||
				value.RequestID != "" && !livingid.ValidID(value.RequestID) ||
				value.FromRevision == "" && value.ToRevision == "" && value.FromContentDigest == "" && value.ToContentDigest == "" {
				return fmt.Errorf("%s: invalid legacy relation confirmation; preserve its bytes and correct the record before normalization", file)
			}
			confirmations = append(confirmations, value)
		}
		slices.SortFunc(confirmations, func(a, b confirmation) int {
			if a.CreatedAt.Before(b.CreatedAt) {
				return -1
			}
			if b.CreatedAt.Before(a.CreatedAt) {
				return 1
			}
			return strings.Compare(a.ID, b.ID)
		})
		// Fold the original pin representation before conversion, exactly as
		// Relation.Confirmed does. Omitted fields never erase an earlier pin.
		for _, value := range confirmations {
			for key, pin := range map[string]string{"from_revision": value.FromRevision, "to_revision": value.ToRevision, "from_content_digest": value.FromContentDigest, "to_content_digest": value.ToContentDigest} {
				if pin != "" {
					relation[key], _ = json.Marshal(pin)
				}
			}
		}

		relation["$schema"], _ = json.Marshal(RelationSchema)
		data, err := json.Marshal(relation)
		if err != nil {
			return err
		}
		p.relations[path] = data
	}
	return nil
}

func (p *Pins) cleanupRelations() error {
	for _, dir := range p.repinDirs {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		// Leave the optional, now empty relation-repins root for legacy readers
		// of an interrupted migration; it carries no content or history.
		if err := store.SyncDir(filepath.Dir(dir)); err != nil {
			return err
		}
	}
	return nil
}
