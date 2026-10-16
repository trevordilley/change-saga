package applayout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const LabelsDir = "___labels"
const MilestonesDir = "___milestones"

// Classification is one shared label or milestone. Features store its canonical
// URN, so renaming a title never rewrites feature identities or memberships.
type Classification struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
}

type Classifications struct {
	Labels     []Classification `json:"labels"`
	Milestones []Classification `json:"milestones"`
}

func ClassificationDir(kind string) string {
	if kind == "label" {
		return LabelsDir
	}
	return MilestonesDir
}
func ClassificationSchema(kind string) string {
	return "https://changesaga.dev/schema/v5/" + kind + ".schema.json"
}
func ClassificationURN(sagaID, kind, id string) string {
	return "urn:change-saga:" + sagaID + ":" + kind + ":" + id
}
func ClassificationID(sagaID, kind, value string) (string, bool) {
	if ValidID(value) {
		return value, true
	}
	prefix := ClassificationURN(sagaID, kind, "")
	id := strings.TrimPrefix(value, prefix)
	return id, id != value && ValidID(id)
}
func (c Classifications) Records(kind string) []Classification {
	if kind == "label" {
		return c.Labels
	}
	return c.Milestones
}
func (c Classifications) Resolve(sagaID, kind, selector string) (Classification, error) {
	id, ok := ClassificationID(sagaID, kind, selector)
	if ok {
		for _, record := range c.Records(kind) {
			if record.ID == id {
				return record, nil
			}
		}
	}
	return Classification{}, fmt.Errorf("%s %q does not exist in this Saga; create it with %s add first", kind, selector, kind)
}
func (c Classifications) Title(sagaID, kind, ref string) string {
	if !strings.HasPrefix(ref, "urn:") {
		return ref // Never reinterpret a legacy milestone as an ID.
	}
	if record, err := c.Resolve(sagaID, kind, ref); err == nil {
		return record.Title
	}
	return ref // Legacy free-text milestones remain readable.
}
func ValidateClassification(record Classification, kind string) error {
	if record.Schema != ClassificationSchema(kind) || record.Version != 5 || !ValidID(record.ID) {
		return fmt.Errorf("%s requires its v5 schema, version and stable id", kind)
	}
	if record.Title == "" || record.Title != strings.TrimSpace(record.Title) || len(record.Title) > 1000 || strings.ContainsAny(record.Title, "\r\n\t") {
		return fmt.Errorf("%s title must be a trimmed single-line name of at most 1000 bytes", kind)
	}
	return nil
}
func ReadClassifications(root string) (Classifications, error) {
	result := Classifications{Labels: []Classification{}, Milestones: []Classification{}}
	for _, kind := range []string{"label", "milestone"} {
		dir := filepath.Join(root, ClassificationDir(kind))
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("%s must be a real directory", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return result, err
		}
		if len(entries) > MaxFeatures {
			return result, fmt.Errorf("too many %s records", kind)
		}
		records := []Classification{}
		titles := map[string]bool{}
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), ".json")
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || id == entry.Name() || !ValidID(id) {
				return result, fmt.Errorf("%s records must be regular <id>.json files", kind)
			}
			var record Classification
			if err := ReadStrictJSON(filepath.Join(dir, entry.Name()), &record); err != nil {
				return result, err
			}
			if record.ID != id {
				return result, fmt.Errorf("%s id must match filename", kind)
			}
			if err := ValidateClassification(record, kind); err != nil {
				return result, err
			}
			key := strings.ToLower(record.Title)
			if titles[key] {
				return result, fmt.Errorf("duplicate %s title %q", kind, record.Title)
			}
			titles[key] = true
			records = append(records, record)
		}
		if kind == "label" {
			result.Labels = records
		} else {
			result.Milestones = records
		}
	}
	return result, nil
}
func (c Classifications) ValidatePlan(sagaID string, plan FeaturePlan) error {
	for _, ref := range plan.Labels {
		record, err := c.Resolve(sagaID, "label", ref)
		if err != nil {
			return err
		}
		if ref != ClassificationURN(sagaID, "label", record.ID) {
			return fmt.Errorf("feature labels must be canonical label URNs")
		}
	}
	if strings.HasPrefix(plan.Milestone, "urn:") {
		record, err := c.Resolve(sagaID, "milestone", plan.Milestone)
		if err != nil {
			return err
		}
		if plan.Milestone != ClassificationURN(sagaID, "milestone", record.ID) {
			return fmt.Errorf("milestone must be a canonical milestone URN")
		}
	}
	return nil
}
