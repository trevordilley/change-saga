package requirements

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

const (
	// RelationRepinSchemaURL identifies an appended relation repin. A repin is
	// only ever written beside a v5 relation.
	RelationRepinSchemaURL = "https://changesaga.dev/schema/v5/relation-repin.schema.json"

	// RepinsDir is the sibling root under ___requirements that holds repins,
	// one directory per relation.
	RepinsDir = "relation-repins"

	MaxRepinsPerRelation = 10_000
)

// RelationRepin is one immutable confirmation that a relation still holds
// against the pins it names. Repins are appended and never edited, so the
// relation keeps its id, its rationale, and every revision anyone read it
// against; the relation record itself is never rewritten to advance a pin.
//
// A repin states only the pins it moves. Anything it omits keeps the value the
// relation, or an earlier repin, already had.
type RelationRepin struct {
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

	// Feature is the feature whose directory holds the record.
	Feature string `json:"-"`
}

// RelationRepinURN names one repin. Like a lifecycle event it is owned by this
// package rather than the shared livingid parser.
func RelationRepinURN(sagaID, relationID, repinID string) (string, error) {
	for name, value := range map[string]string{"saga": sagaID, "relation": relationID, "repin": repinID} {
		if !livingid.ValidID(value) {
			return "", fmt.Errorf("%s ID is not a stable identifier", name)
		}
	}
	return fmt.Sprintf("urn:change-saga:%s:relation:%s:repin:%s", sagaID, relationID, repinID), nil
}

func repinPath(feature, relationID, repinID string) string {
	return applayout.FeatureRel(feature) + "/" + applayout.RequirementsDir + "/" + RepinsDir + "/" + relationID + "/" + repinID + ".json"
}

// Confirmed folds the relation's appended repins over its recorded pins and
// returns what each endpoint is currently confirmed against. The relation's own
// pins are what its author confirmed; each repin in turn is what a later reader
// confirmed.
func (relation Relation) Confirmed() Relation {
	result := relation
	for _, repin := range relation.Repins {
		for target, value := range map[*string]string{
			&result.FromRevision:      repin.FromRevision,
			&result.ToRevision:        repin.ToRevision,
			&result.FromContentDigest: repin.FromContentDigest,
			&result.ToContentDigest:   repin.ToContentDigest,
		} {
			if value != "" {
				*target = value
			}
		}
	}
	return result
}

// loadRelationRepins reads every repin one feature holds and attaches it to its
// relation. Repins are ordered oldest first, by recorded time then id, so the
// fold is the same for every reader of the same bytes.
func loadRelationRepins(document *Document, feature applayout.Feature) error {
	dir := filepath.Join(feature.Dir, applayout.RequirementsDir, RepinsDir)
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxRelations)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
			return fmt.Errorf("relation-repins entry %q must be a real directory named for its relation", entry.Name())
		}
		if !livingid.ValidID(entry.Name()) {
			return fmt.Errorf("relation-repins entry %q is not a stable relation id", entry.Name())
		}
		relation := document.relation(entry.Name())
		if relation == nil {
			return fmt.Errorf("relation-repins/%s: relation %q does not exist", entry.Name(), entry.Name())
		}
		if relation.Schema == currentrecord.RelationSchema {
			continue // published current pins take precedence over interrupted cleanup
		}
		repins, err := loadRepinsOf(document, feature, filepath.Join(dir, entry.Name()), entry.Name())
		if err != nil {
			return fmt.Errorf("relation-repins/%s: %w", entry.Name(), err)
		}
		relation.Repins = repins
		// A repin never makes a relation invalid, whoever wrote the bytes.
		if err := validateRelation(relation.Confirmed(), document.SagaID, relation.ID); err != nil {
			return fmt.Errorf("relation-repins/%s: %w", entry.Name(), err)
		}
	}
	return nil
}

func loadRepinsOf(document *Document, feature applayout.Feature, dir, relationID string) ([]RelationRepin, error) {
	files, err := boundedReadDir(dir, MaxRepinsPerRelation)
	if err != nil {
		return nil, err
	}
	repins := make([]RelationRepin, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		if file.Type()&fs.ModeSymlink != 0 || !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".json" {
			return nil, fmt.Errorf("repin entry %q must be a real JSON file", file.Name())
		}
		var value RelationRepin
		if err := readStrictJSON(filepath.Join(dir, file.Name()), &value); err != nil {
			return nil, fmt.Errorf("repin %s: %w", file.Name(), err)
		}
		id := strings.TrimSuffix(file.Name(), ".json")
		if err := validateRelationRepin(value, document.SagaID, relationID, id); err != nil {
			return nil, fmt.Errorf("repin %s: %w", file.Name(), err)
		}
		if seen[id] {
			return nil, fmt.Errorf("repin id %q appears twice", id)
		}
		seen[id] = true
		value.Feature = feature.ID
		repins = append(repins, value)
	}
	sort.Slice(repins, func(i, j int) bool {
		if !repins[i].CreatedAt.Equal(repins[j].CreatedAt) {
			return repins[i].CreatedAt.Before(repins[j].CreatedAt)
		}
		return repins[i].ID < repins[j].ID
	})
	return repins, nil
}

func validateRelationRepin(value RelationRepin, sagaID, relationID, expectedID string) error {
	var problems validationErrors
	if value.Schema != RelationRepinSchemaURL {
		problems.add("$schema must be %q", RelationRepinSchemaURL)
	}
	if value.Version != V5RelationVersion {
		problems.add("version must be %d", V5RelationVersion)
	}
	if value.ID != expectedID {
		problems.add("id must match the file name %q", expectedID)
	}
	urn, err := relationURN(sagaID, relationID)
	if err != nil || value.Relation != urn {
		problems.add("relation must be %q, the relation whose repins hold it", urn)
	}
	if strings.TrimSpace(value.Rationale) == "" {
		problems.add("rationale must say why the relation still holds")
	}
	if value.CreatedAt.IsZero() {
		problems.add("created_at is required")
	}
	if value.FromRevision == "" && value.ToRevision == "" && value.FromContentDigest == "" && value.ToContentDigest == "" {
		problems.add("a repin must move at least one pin")
	}
	if value.RequestID != "" && !livingid.ValidID(value.RequestID) {
		problems.add("request_id must be a stable identifier")
	}
	return problems.err()
}

// relation returns the loaded relation with this id, or nil.
func (document *Document) relation(id string) *Relation {
	for index := range document.Relations {
		if document.Relations[index].ID == id {
			return &document.Relations[index]
		}
	}
	return nil
}

// RepinRelationInput advances one relation's pins. Omitted pins are left where
// the relation, or an earlier repin, already had them.
type RepinRelationInput struct {
	Relation          string
	ID                string
	FromRevision      string
	ToRevision        string
	FromContentDigest string
	ToContentDigest   string
	Rationale         string
	CreatedAt         time.Time
	RequestID         string
}

// RepinRelation updates the current relation after the author reassesses it.
// Git records the previous pins and rationale; no confirmation overlay is saved.
func RepinRelation(root, sagaID string, input RepinRelationInput) (MutationResult, error) {
	if err := currentrecord.NormalizeValue(root, &input); err != nil {
		return MutationResult{}, err
	}
	ref, err := livingid.Parse(input.Relation)
	if err != nil || ref.Kind != livingid.KindRelation || ref.SagaID != sagaID {
		return MutationResult{}, fmt.Errorf("relation must be a canonical relation URN in saga %q", sagaID)
	}
	if strings.TrimSpace(input.Rationale) == "" {
		return MutationResult{}, fmt.Errorf("rationale must explain the reassessment")
	}
	var result MutationResult
	err = mutate(root, sagaID, func(document *Document) error {
		existing := document.relation(ref.ID)
		if existing == nil || existing.State != RelationActive {
			return fmt.Errorf("relation %q must exist and be active", ref.ID)
		}
		candidate := existing.Confirmed()
		candidate.Schema = currentrecord.RelationSchema
		candidate.Repins = nil
		for target, value := range map[*string]string{
			&candidate.FromRevision: input.FromRevision, &candidate.ToRevision: input.ToRevision,
			&candidate.FromContentDigest: input.FromContentDigest, &candidate.ToContentDigest: input.ToContentDigest,
		} {
			if value != "" {
				*target = value
			}
		}
		if samePins(candidate, existing.Confirmed()) {
			return fmt.Errorf("relation %q already has those pins; there is nothing to re-pin", ref.ID)
		}
		candidate.Rationale = strings.TrimSpace(input.Rationale)
		if err := validateRelation(candidate, sagaID, ref.ID); err != nil {
			return err
		}
		*existing = candidate
		if err := validateRelationSet(document); err != nil {
			return err
		}
		path := relationPath(existing.Feature, ref.ID)
		if err := store.WriteJSON(filepath.Join(root, filepath.FromSlash(path)), candidate, false); err != nil {
			return err
		}
		result = MutationResult{URN: input.Relation, Path: path}
		return nil
	})
	return result, err
}

// samePins reports whether two folds of a relation confirm the identical pins.
func samePins(left, right Relation) bool {
	return left.FromRevision == right.FromRevision && left.ToRevision == right.ToRevision &&
		left.FromContentDigest == right.FromContentDigest && left.ToContentDigest == right.ToContentDigest
}
