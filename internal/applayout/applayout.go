// Package applayout defines the physical layout of an app Saga: one saga.json,
// the app-level roots, and the durable features beneath ___features. It is a leaf
// package so every domain loader can locate its records the same way without
// depending on any other domain.
//
//	app.saga/
//	  saga.json
//	  ___overview/        the project's name (saga.json title), elevator pitch,
//	                      description, and terms and vocabulary
//	    pitch.fragment/
//	    description.fragment/
//	    terms/<id>.term/
//	  ___personas/        persona records
//	  ___designsystem/    report content: Figma links and references
//	  ___onboarding/      a deck whose Items reference records
//	  ___featureflags/    flags and the features and stories each one gates
//	  ___features/<id>.feature/
//	    feature.json
//	    <report content, ___requirements, ___design, ___slides, ___quality, ___workplan>
//
// Resource identity is independent of feature membership: a story, test case, or
// deck URN never names its feature. Loaders therefore read every feature into one
// app-wide model and reject an ID that two features both use.
package applayout

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/store"
)

const (
	ManifestName = "saga.json"

	OverviewDir     = "___overview"
	PersonasDir     = "___personas"
	DesignSystemDir = "___designsystem"
	OnboardingDir   = "___onboarding"
	FeatureFlagsDir = "___featureflags"
	FeaturesDir     = "___features"

	// The overview's formal parts. The project name is saga.json's title; the
	// elevator pitch and description are fixed report fragments; terms are
	// living records. Every part is optional.
	OverviewPitch       = "pitch.fragment"
	OverviewDescription = "description.fragment"
	OverviewTerms       = "terms"
	TermsDir            = OverviewDir + "/" + OverviewTerms

	FeatureSuffix       = ".feature"
	FeatureManifestName = "feature.json"

	// FeatureVersion versions feature.json. App-level records are part of the one
	// version-5 format.
	FeatureVersion   = 5
	FeatureSchemaURL = "https://changesaga.dev/schema/v5/feature.schema.json"

	MaxFeatures    = 1_000
	maxRecordBytes = 1 << 20
)

// Feature-level content roots. They are the roots a single-change Saga held at
// its top level before features existed.
const (
	RequirementsDir = "___requirements"
	DesignDir       = "___design"
	SlidesDir       = "___slides"
	QualityDir      = "___quality"
	WorkplanDir     = "___workplan"
)

// AppRootDirs are the reserved directories allowed directly beneath an app
// Saga root, besides the review overlay roots the report loader owns.
var AppRootDirs = []string{OverviewDir, PersonasDir, DesignSystemDir, OnboardingDir, FeatureFlagsDir, FeaturesDir, "___inventory"}

// FeatureRootDirs are the reserved directories allowed directly beneath a feature.
var FeatureRootDirs = []string{RequirementsDir, DesignDir, SlidesDir, QualityDir, WorkplanDir}

var (
	stableID          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	unsafeQualifiedID = regexp.MustCompile(`[^a-z0-9]+`)
)

// ValidID reports whether value is a stable identifier.
func ValidID(value string) bool { return stableID.MatchString(value) }

// FeatureQualifiedID returns the deterministic ID used when feature-qualified
// generation is requested for a deck or slide without an explicit ID.
// Existing and explicitly supplied IDs are never rewritten. The double hyphen
// keeps the feature and local portions visually distinct while remaining
// inside the stable-ID grammar used by every existing URN.
func FeatureQualifiedID(feature, local string) string {
	feature = qualifiedIDPart(feature, 60)
	local = qualifiedIDPart(local, 60)
	const separator = "--"
	maximumLocal := 128 - len(feature) - len(separator)
	if len(local) > maximumLocal {
		local = strings.Trim(local[:maximumLocal], "-")
	}
	if local == "" {
		local = "item"
	}
	return feature + separator + local
}

func qualifiedIDPart(value string, maximum int) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = unsafeQualifiedID.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		value = "item"
	}
	if len(value) <= maximum {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return strings.Trim(value[:maximum-9], "-") + "-" + fmt.Sprintf("%x", sum[:4])
}

// FeatureManifest is the immutable identity of one durable product domain.
type FeatureManifest struct {
	Schema      string    `json:"$schema"`
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	RequestID   string    `json:"request_id,omitempty"`
}

// Feature is one loaded feature directory.
type Feature struct {
	FeatureManifest
	// Dir is the absolute feature directory.
	Dir string
	// Rel is the slash path of the feature directory relative to the app root.
	Rel string
}

// FeatureRel returns the app-relative slash path of feature id.
func FeatureRel(id string) string { return FeaturesDir + "/" + id + FeatureSuffix }

// FeatureDir returns the absolute directory of feature id beneath root.
func FeatureDir(root, id string) string {
	return filepath.Join(root, FeaturesDir, id+FeatureSuffix)
}

// FeatureURN is the stable target of a feature.
func FeatureURN(sagaID, featureID string) string {
	return "urn:change-saga:" + sagaID + ":feature:" + featureID
}

// FeatureFromURN returns the feature ID named by value, which may be a bare ID or a
// canonical feature URN of sagaID.
func FeatureFromURN(sagaID, value string) (string, bool) {
	if ValidID(value) {
		return value, true
	}
	prefix := "urn:change-saga:" + sagaID + ":feature:"
	if id := strings.TrimPrefix(value, prefix); id != value && ValidID(id) {
		return id, true
	}
	return "", false
}

// Features lists every feature beneath root in ID order. A missing ___features
// directory is an app with no features yet. Every entry must be a real
// <id>.feature directory whose feature.json names the same ID.
func Features(root string) ([]Feature, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(abs, FeaturesDir)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Feature{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%s must be a real directory", FeaturesDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > MaxFeatures {
		return nil, fmt.Errorf("%s has %d entries; maximum is %d", FeaturesDir, len(entries), MaxFeatures)
	}
	features := make([]Feature, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		rel := FeaturesDir + "/" + name
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(name, FeatureSuffix) {
			return nil, fmt.Errorf("%s: features must be real <id>%s directories", rel, FeatureSuffix)
		}
		id := strings.TrimSuffix(name, FeatureSuffix)
		if !ValidID(id) {
			return nil, fmt.Errorf("%s: feature directory name is not a stable id", rel)
		}
		var manifest FeatureManifest
		if err := ReadStrictJSON(filepath.Join(dir, name, FeatureManifestName), &manifest); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", rel, FeatureManifestName, err)
		}
		if err := ValidateFeatureManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", rel, FeatureManifestName, err)
		}
		if manifest.ID != id {
			return nil, fmt.Errorf("%s/%s: feature id %q must match its directory", rel, FeatureManifestName, manifest.ID)
		}
		features = append(features, Feature{FeatureManifest: manifest, Dir: filepath.Join(dir, name), Rel: rel})
	}
	sort.Slice(features, func(i, j int) bool { return features[i].ID < features[j].ID })
	return features, nil
}

// InCreationOrder returns features in the order they were created, which is how
// an author introduced the app's domains and how they are presented; features
// created at the same instant keep ID order.
func InCreationOrder(features []Feature) []Feature {
	result := append([]Feature{}, features...)
	sort.SliceStable(result, func(i, j int) bool {
		if !result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].CreatedAt.Before(result[j].CreatedAt)
		}
		return result[i].ID < result[j].ID
	})
	return result
}

// ValidateFeatureManifest checks one feature identity record.
func ValidateFeatureManifest(manifest FeatureManifest) error {
	switch {
	case manifest.Schema != FeatureSchemaURL:
		return fmt.Errorf("$schema must be %s", FeatureSchemaURL)
	case manifest.Version != FeatureVersion:
		return fmt.Errorf("version must be %d", FeatureVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	}
	return nil
}

// Find returns the feature with id.
func Find(features []Feature, id string) (Feature, bool) {
	for _, feature := range features {
		if feature.ID == id {
			return feature, true
		}
	}
	return Feature{}, false
}

// Require resolves the feature an authoring command targets. value may be a bare
// feature ID or its URN; an empty value is refused with the known features listed,
// because feature content is never written to an implied feature.
func Require(root, sagaID, value string) (Feature, error) {
	features, err := Features(root)
	if err != nil {
		return Feature{}, err
	}
	if strings.TrimSpace(value) == "" {
		return Feature{}, fmt.Errorf("--feature is required; %s", known(features))
	}
	id, ok := FeatureFromURN(sagaID, value)
	if !ok {
		return Feature{}, fmt.Errorf("--feature %q is not a feature id or URN", value)
	}
	feature, found := Find(features, id)
	if !found {
		return Feature{}, fmt.Errorf("feature %q does not exist; %s", id, known(features))
	}
	return feature, nil
}

func known(features []Feature) string {
	if len(features) == 0 {
		return "the app has no features yet; add one with change-saga feature add"
	}
	ids := make([]string, 0, len(features))
	for _, feature := range features {
		ids = append(ids, feature.ID)
	}
	return "known features: " + strings.Join(ids, ", ")
}

// FeatureOfPath returns the feature ID containing an app-relative slash path, or ""
// for app-level paths.
func FeatureOfPath(rel string) string {
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, FeaturesDir+"/") {
		return ""
	}
	name := strings.SplitN(strings.TrimPrefix(rel, FeaturesDir+"/"), "/", 2)[0]
	if !strings.HasSuffix(name, FeatureSuffix) {
		return ""
	}
	return strings.TrimSuffix(name, FeatureSuffix)
}

// UniqueIDs records the feature that owns each app-unique resource ID and reports
// a second owner as an error. Identity is independent of feature membership, so
// the same ID in two features would be one URN naming two records.
type UniqueIDs struct {
	kind   string
	owners map[string]string
}

// NewUniqueIDs returns an empty ID index for kind, such as "story".
func NewUniqueIDs(kind string) *UniqueIDs {
	return &UniqueIDs{kind: kind, owners: map[string]string{}}
}

// Claim records that feature owns id.
func (u *UniqueIDs) Claim(id, feature string) error {
	if owner, ok := u.owners[id]; ok {
		if owner == feature {
			return fmt.Errorf("duplicate %s id %q in feature %q", u.kind, id, feature)
		}
		return fmt.Errorf("%s id %q is used by features %q and %q; %s IDs are unique across the app", u.kind, id, owner, feature, u.kind)
	}
	u.owners[id] = feature
	return nil
}

// Owner returns the feature owning id.
func (u *UniqueIDs) Owner(id string) (string, bool) {
	owner, ok := u.owners[id]
	return owner, ok
}

// ReadStrictJSON decodes exactly one JSON value from a real regular file of at
// most one MiB, rejecting unknown fields.
func ReadStrictJSON(path string, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("must be a real regular file")
	}
	if info.Size() > maxRecordBytes {
		return fmt.Errorf("record is %d bytes; maximum is %d", info.Size(), maxRecordBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("record must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

// WriteFeature creates the feature directory and its manifest. It is the one writer
// of feature.json, shared by the CLI and test fixtures.
func WriteFeature(root string, manifest FeatureManifest) (Feature, error) {
	if manifest.Schema == "" {
		manifest.Schema = FeatureSchemaURL
	}
	if manifest.Version == 0 {
		manifest.Version = FeatureVersion
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	if err := ValidateFeatureManifest(manifest); err != nil {
		return Feature{}, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Feature{}, err
	}
	dir := FeatureDir(abs, manifest.ID)
	err = store.CommitDir(abs, dir, func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(stage, FeatureManifestName), manifest, true)
	})
	if errors.Is(err, fs.ErrExist) {
		return Feature{}, fmt.Errorf("feature %q already exists", manifest.ID)
	}
	if err != nil {
		return Feature{}, err
	}
	return Feature{FeatureManifest: manifest, Dir: dir, Rel: FeatureRel(manifest.ID)}, nil
}

// RejectFeatureRootsAtAppRoot refuses the single-change layout, in which
// requirements, design, quality, the work plan, and the deck sat directly
// beneath saga.json. That content now belongs to a feature.
func RejectFeatureRootsAtAppRoot(root string) error {
	for _, name := range FeatureRootDirs {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return fmt.Errorf("%s belongs in a feature (%s/<id>%s/%s), not at the app root", name, FeaturesDir, FeatureSuffix, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
