// Package applayout defines the physical layout of an app Saga: one saga.json,
// the app-level roots, and the durable epics beneath ___epics. It is a leaf
// package so every domain loader can locate its records the same way without
// depending on any other domain.
//
//	app.saga/
//	  saga.json
//	  ___overview/        report content: the elevator pitch for the app
//	  ___personas/        persona records
//	  ___designsystem/    report content: Figma links and references
//	  ___onboarding/      a deck whose Items reference records
//	  ___featureflags/    flags and the epics and stories each one gates
//	  ___epics/<id>.epic/
//	    epic.json
//	    <report content, ___requirements, ___design, ___slides, ___quality, ___workplan>
//
// Resource identity is independent of epic membership: a story, test case, or
// deck URN never names its epic. Loaders therefore read every epic into one
// app-wide model and reject an ID that two epics both use.
package applayout

import (
	"bytes"
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
	EpicsDir        = "___epics"

	EpicSuffix       = ".epic"
	EpicManifestName = "epic.json"

	// EpicVersion versions epic.json. App-level records are part of the one
	// version-5 format.
	EpicVersion   = 5
	EpicSchemaURL = "https://changesaga.dev/schema/v5/epic.schema.json"

	MaxEpics       = 1_000
	maxRecordBytes = 1 << 20
)

// Epic-level content roots. They are the roots a single-change Saga held at
// its top level before epics existed.
const (
	RequirementsDir = "___requirements"
	DesignDir       = "___design"
	SlidesDir       = "___slides"
	QualityDir      = "___quality"
	WorkplanDir     = "___workplan"
)

// AppRootDirs are the reserved directories allowed directly beneath an app
// Saga root, besides the review overlay roots the report loader owns.
var AppRootDirs = []string{OverviewDir, PersonasDir, DesignSystemDir, OnboardingDir, FeatureFlagsDir, EpicsDir}

// EpicRootDirs are the reserved directories allowed directly beneath an epic.
var EpicRootDirs = []string{RequirementsDir, DesignDir, SlidesDir, QualityDir, WorkplanDir}

var stableID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidID reports whether value is a stable identifier.
func ValidID(value string) bool { return stableID.MatchString(value) }

// EpicManifest is the immutable identity of one durable product domain.
type EpicManifest struct {
	Schema      string    `json:"$schema"`
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	RequestID   string    `json:"request_id,omitempty"`
}

// Epic is one loaded epic directory.
type Epic struct {
	EpicManifest
	// Dir is the absolute epic directory.
	Dir string
	// Rel is the slash path of the epic directory relative to the app root.
	Rel string
}

// EpicRel returns the app-relative slash path of epic id.
func EpicRel(id string) string { return EpicsDir + "/" + id + EpicSuffix }

// EpicDir returns the absolute directory of epic id beneath root.
func EpicDir(root, id string) string {
	return filepath.Join(root, EpicsDir, id+EpicSuffix)
}

// EpicURN is the stable target of an epic.
func EpicURN(sagaID, epicID string) string {
	return "urn:change-saga:" + sagaID + ":epic:" + epicID
}

// EpicFromURN returns the epic ID named by value, which may be a bare ID or a
// canonical epic URN of sagaID.
func EpicFromURN(sagaID, value string) (string, bool) {
	if ValidID(value) {
		return value, true
	}
	prefix := "urn:change-saga:" + sagaID + ":epic:"
	if id := strings.TrimPrefix(value, prefix); id != value && ValidID(id) {
		return id, true
	}
	return "", false
}

// Epics lists every epic beneath root in ID order. A missing ___epics
// directory is an app with no epics yet. Every entry must be a real
// <id>.epic directory whose epic.json names the same ID.
func Epics(root string) ([]Epic, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(abs, EpicsDir)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Epic{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%s must be a real directory", EpicsDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > MaxEpics {
		return nil, fmt.Errorf("%s has %d entries; maximum is %d", EpicsDir, len(entries), MaxEpics)
	}
	epics := make([]Epic, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		rel := EpicsDir + "/" + name
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(name, EpicSuffix) {
			return nil, fmt.Errorf("%s: epics must be real <id>%s directories", rel, EpicSuffix)
		}
		id := strings.TrimSuffix(name, EpicSuffix)
		if !ValidID(id) {
			return nil, fmt.Errorf("%s: epic directory name is not a stable id", rel)
		}
		var manifest EpicManifest
		if err := ReadStrictJSON(filepath.Join(dir, name, EpicManifestName), &manifest); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", rel, EpicManifestName, err)
		}
		if err := ValidateEpicManifest(manifest); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", rel, EpicManifestName, err)
		}
		if manifest.ID != id {
			return nil, fmt.Errorf("%s/%s: epic id %q must match its directory", rel, EpicManifestName, manifest.ID)
		}
		epics = append(epics, Epic{EpicManifest: manifest, Dir: filepath.Join(dir, name), Rel: rel})
	}
	sort.Slice(epics, func(i, j int) bool { return epics[i].ID < epics[j].ID })
	return epics, nil
}

// ValidateEpicManifest checks one epic identity record.
func ValidateEpicManifest(manifest EpicManifest) error {
	switch {
	case manifest.Schema != EpicSchemaURL:
		return fmt.Errorf("$schema must be %s", EpicSchemaURL)
	case manifest.Version != EpicVersion:
		return fmt.Errorf("version must be %d", EpicVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	}
	return nil
}

// Find returns the epic with id.
func Find(epics []Epic, id string) (Epic, bool) {
	for _, epic := range epics {
		if epic.ID == id {
			return epic, true
		}
	}
	return Epic{}, false
}

// Require resolves the epic an authoring command targets. value may be a bare
// epic ID or its URN; an empty value is refused with the known epics listed,
// because epic content is never written to an implied epic.
func Require(root, sagaID, value string) (Epic, error) {
	epics, err := Epics(root)
	if err != nil {
		return Epic{}, err
	}
	if strings.TrimSpace(value) == "" {
		return Epic{}, fmt.Errorf("--epic is required; %s", known(epics))
	}
	id, ok := EpicFromURN(sagaID, value)
	if !ok {
		return Epic{}, fmt.Errorf("--epic %q is not an epic id or URN", value)
	}
	epic, found := Find(epics, id)
	if !found {
		return Epic{}, fmt.Errorf("epic %q does not exist; %s", id, known(epics))
	}
	return epic, nil
}

func known(epics []Epic) string {
	if len(epics) == 0 {
		return "the app has no epics yet; add one with change-saga epic add"
	}
	ids := make([]string, 0, len(epics))
	for _, epic := range epics {
		ids = append(ids, epic.ID)
	}
	return "known epics: " + strings.Join(ids, ", ")
}

// EpicOfPath returns the epic ID containing an app-relative slash path, or ""
// for app-level paths.
func EpicOfPath(rel string) string {
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, EpicsDir+"/") {
		return ""
	}
	name := strings.SplitN(strings.TrimPrefix(rel, EpicsDir+"/"), "/", 2)[0]
	if !strings.HasSuffix(name, EpicSuffix) {
		return ""
	}
	return strings.TrimSuffix(name, EpicSuffix)
}

// UniqueIDs records the epic that owns each app-unique resource ID and reports
// a second owner as an error. Identity is independent of epic membership, so
// the same ID in two epics would be one URN naming two records.
type UniqueIDs struct {
	kind   string
	owners map[string]string
}

// NewUniqueIDs returns an empty ID index for kind, such as "story".
func NewUniqueIDs(kind string) *UniqueIDs {
	return &UniqueIDs{kind: kind, owners: map[string]string{}}
}

// Claim records that epic owns id.
func (u *UniqueIDs) Claim(id, epic string) error {
	if owner, ok := u.owners[id]; ok {
		if owner == epic {
			return fmt.Errorf("duplicate %s id %q in epic %q", u.kind, id, epic)
		}
		return fmt.Errorf("%s id %q is used by epics %q and %q; %s IDs are unique across the app", u.kind, id, owner, epic, u.kind)
	}
	u.owners[id] = epic
	return nil
}

// Owner returns the epic owning id.
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

// WriteEpic creates the epic directory and its manifest. It is the one writer
// of epic.json, shared by the CLI and test fixtures.
func WriteEpic(root string, manifest EpicManifest) (Epic, error) {
	if manifest.Schema == "" {
		manifest.Schema = EpicSchemaURL
	}
	if manifest.Version == 0 {
		manifest.Version = EpicVersion
	}
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	if err := ValidateEpicManifest(manifest); err != nil {
		return Epic{}, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Epic{}, err
	}
	dir := EpicDir(abs, manifest.ID)
	err = store.CommitDir(abs, dir, func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(stage, EpicManifestName), manifest, true)
	})
	if errors.Is(err, fs.ErrExist) {
		return Epic{}, fmt.Errorf("epic %q already exists", manifest.ID)
	}
	if err != nil {
		return Epic{}, err
	}
	return Epic{EpicManifest: manifest, Dir: dir, Rel: EpicRel(manifest.ID)}, nil
}

// RejectEpicRootsAtAppRoot refuses the single-change layout, in which
// requirements, design, quality, the work plan, and the deck sat directly
// beneath saga.json. That content now belongs to an epic.
func RejectEpicRootsAtAppRoot(root string) error {
	for _, name := range EpicRootDirs {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			return fmt.Errorf("%s belongs in an epic (%s/<id>%s/%s), not at the app root", name, EpicsDir, EpicSuffix, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
