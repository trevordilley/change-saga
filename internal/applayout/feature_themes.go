package applayout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const FeatureThemesDir = "___feature-themes"
const FeatureThemeSchemaURL = "https://changesaga.dev/schema/v5/feature-theme.schema.json"

// FeatureTheme is an optional product categorization. Membership references
// features; it never changes their identity or physical location. Git records
// edits to this current snapshot, including its authored Markdown overview.
type FeatureTheme struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Overview  string    `json:"overview"`
	Features  []string  `json:"features"`
	CreatedAt time.Time `json:"created_at"`
}

func FeatureThemeRel(id string) string { return FeatureThemesDir + "/" + id + ".theme" }
func FeatureThemeURN(sagaID, id string) string {
	return "urn:change-saga:" + sagaID + ":feature-theme:" + id
}
func FeatureThemeID(sagaID, value string) (string, bool) {
	if ValidID(value) {
		return value, true
	}
	prefix := FeatureThemeURN(sagaID, "")
	id := strings.TrimPrefix(value, prefix)
	return id, id != value && ValidID(id)
}

func ValidateFeatureTheme(value FeatureTheme, sagaID string, features []Feature) error {
	if value.Schema != FeatureThemeSchemaURL || value.Version != 5 || !ValidID(value.ID) || strings.TrimSpace(value.Title) == "" || value.CreatedAt.IsZero() || strings.TrimSpace(value.Overview) == "" || value.Features == nil {
		return fmt.Errorf("feature theme requires its v5 schema, stable id, title, authored overview, feature array, and created_at")
	}
	if len(value.Title) > 1000 || len(value.Overview) > 256*1024 || len(value.Features) > MaxFeatures {
		return fmt.Errorf("feature theme exceeds title, overview, or membership limits")
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(data)+1 > maxRecordBytes {
		return fmt.Errorf("feature theme exceeds record size limit")
	}
	seen := map[string]bool{}
	for _, ref := range value.Features {
		id, ok := FeatureFromURN(sagaID, ref)
		if !ok || ref != FeatureURN(sagaID, id) {
			return fmt.Errorf("theme feature %q must be a canonical feature URN of this Saga", ref)
		}
		if _, ok := Find(features, id); !ok {
			return fmt.Errorf("theme feature %q does not exist", ref)
		}
		if seen[ref] {
			return fmt.Errorf("duplicate theme feature %q", ref)
		}
		seen[ref] = true
	}
	return nil
}

// FeatureThemes rejects links and non-regular records before reading, with
// bounded record sizes and counts just like Features. Missing means unadopted.
func FeatureThemes(root, sagaID string) ([]FeatureTheme, error) {
	dir := filepath.Join(root, FeatureThemesDir)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return []FeatureTheme{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s must be a real directory", FeatureThemesDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > MaxFeatures {
		return nil, fmt.Errorf("too many feature themes (maximum %d)", MaxFeatures)
	}
	features, err := Features(root)
	if err != nil {
		return nil, err
	}
	result := []FeatureTheme{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".theme")
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || id == entry.Name() || !ValidID(id) {
			return nil, fmt.Errorf("%s: themes must be real <id>.theme directories", entry.Name())
		}
		var value FeatureTheme
		if err := ReadStrictJSON(filepath.Join(dir, entry.Name(), "theme.json"), &value); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if value.ID != id {
			return nil, fmt.Errorf("theme id must match directory %s", entry.Name())
		}
		if err := ValidateFeatureTheme(value, sagaID, features); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		result = append(result, value)
	}
	return result, nil
}
