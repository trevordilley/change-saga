package requirements

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
)

type sagaIdentity struct {
	Schema  string          `json:"$schema,omitempty"`
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	PR      json.RawMessage `json:"pr,omitempty"`
	Source  json.RawMessage `json:"source"`
}

// sagaVersion is the one Change Saga container format. Its requirement
// components are version-3 records.
const sagaVersion = 5

func Load(root, sagaID string) (Document, error) {
	return LoadWithOptions(root, sagaID, LoadOptions{})
}

// LoadWithOptions strictly loads only the bounded requirements-domain roots.
// It never follows symlinks and never opens narrative, design, work-plan,
// review, or diff content.
func LoadWithOptions(root, sagaID string, options LoadOptions) (Document, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Document{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Document{}, fmt.Errorf("open saga: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Document{}, fmt.Errorf("saga root must be a real directory")
	}
	manifestPath := filepath.Join(abs, "saga.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
		return Document{}, fmt.Errorf("saga.json must be a real regular file")
	}
	var identity sagaIdentity
	if err := readStrictJSON(manifestPath, &identity); err != nil {
		return Document{}, fmt.Errorf("read saga.json: %w", err)
	}
	if identity.Version != sagaVersion {
		return Document{}, fmt.Errorf("saga.json: unsupported Saga version %d; change-saga reads only version %d", identity.Version, sagaVersion)
	}
	if !livingid.ValidID(identity.ID) {
		return Document{}, fmt.Errorf("saga.json contains an invalid saga id")
	}
	if sagaID == "" {
		sagaID = identity.ID
	}
	if sagaID != identity.ID {
		return Document{}, fmt.Errorf("requested saga id %q does not match saga.json id %q", sagaID, identity.ID)
	}
	features, err := applayout.Features(abs)
	if err != nil {
		return Document{}, err
	}
	if err := applayout.RejectFeatureRootsAtAppRoot(abs); err != nil {
		return Document{}, err
	}
	document := Document{Root: abs, SagaID: sagaID, Features: features, Personas: []Persona{}, Stories: []Story{}, Citations: []Citation{}, Relations: []Relation{}, Flags: []Flag{}, Terms: []Term{}}
	if err := loadPersonas(&document); err != nil {
		return Document{}, err
	}
	stories, citations, relations := applayout.NewUniqueIDs("story"), applayout.NewUniqueIDs("citation"), applayout.NewUniqueIDs("relation")
	for _, feature := range features {
		requirementsRoot := filepath.Join(feature.Dir, applayout.RequirementsDir)
		present, err := realDirectory(requirementsRoot)
		if err != nil {
			return Document{}, err
		}
		if !present {
			continue
		}
		entries, err := boundedReadDir(requirementsRoot, 6)
		if err != nil {
			return Document{}, err
		}
		for _, entry := range entries {
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
				return Document{}, fmt.Errorf("%s: requirements entry %q must be a real directory", feature.Rel, entry.Name())
			}
			switch entry.Name() {
			case "stories", "citations", "relations":
			// prototypes is the sibling capability root owned by internal/prototypes.
			// It shares ___requirements so the two merge independently; this loader
			// deliberately never reads it.
			case "prototypes":
			// coverage-exceptions holds immutable decisions owned by the coverage
			// exception domain. Like prototypes it is a sibling root this loader
			// deliberately never reads.
			case "coverage-exceptions":
			// relation-repins holds the appended confirmations of relations in
			// this feature, one directory per relation. They are read after
			// every relation is loaded, so a repin can never name one that is
			// not there.
			case RepinsDir:
			default:
				return Document{}, fmt.Errorf("%s: unknown requirements entry %q", feature.Rel, entry.Name())
			}
		}
		if err := loadCitations(&document, feature, citations); err != nil {
			return Document{}, err
		}
		if err := loadStories(&document, feature, stories); err != nil {
			return Document{}, err
		}
		if err := loadRelations(&document, feature, relations); err != nil {
			return Document{}, err
		}
	}
	for _, feature := range features {
		if err := loadRelationRepins(&document, feature); err != nil {
			return Document{}, fmt.Errorf("%s: %w", feature.Rel, err)
		}
	}
	if len(document.Stories) > MaxStories {
		return Document{}, fmt.Errorf("the app has %d stories; maximum is %d", len(document.Stories), MaxStories)
	}
	sort.Slice(document.Stories, func(i, j int) bool { return document.Stories[i].Identity.ID < document.Stories[j].Identity.ID })
	sort.Slice(document.Citations, func(i, j int) bool { return document.Citations[i].ID < document.Citations[j].ID })
	sort.Slice(document.Relations, func(i, j int) bool { return document.Relations[i].ID < document.Relations[j].ID })
	if err := loadFlags(&document); err != nil {
		return Document{}, err
	}
	if err := loadTerms(&document); err != nil {
		return Document{}, err
	}
	citationIDs := map[string]bool{}
	for _, citation := range document.Citations {
		citationIDs[citation.ID] = true
	}
	personaIDs := document.personaIDs()
	for index := range document.Stories {
		story := &document.Stories[index]
		if err := validateStoryGraphs(story, sagaID, citationIDs, personaIDs); err != nil {
			return Document{}, fmt.Errorf("story %q: %w", story.Identity.ID, err)
		}
	}
	if err := validateRelationSet(&document); err != nil {
		return Document{}, fmt.Errorf("relations: %w", err)
	}
	evaluateStaleness(&document, options.StaleInputs)
	return document, nil
}

func loadStories(document *Document, feature applayout.Feature, ids *applayout.UniqueIDs) error {
	dir := filepath.Join(feature.Dir, applayout.RequirementsDir, "stories")
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxStories)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".story") {
			return fmt.Errorf("story entry %q must be a real <id>.story directory", entry.Name())
		}
		storyID := strings.TrimSuffix(entry.Name(), ".story")
		if !livingid.ValidID(storyID) {
			return fmt.Errorf("story package %q has an invalid id", entry.Name())
		}
		if err := ids.Claim(storyID, feature.ID); err != nil {
			return err
		}
		story, err := loadStoryPackage(document.Root, document.SagaID, path, storyID)
		if err != nil {
			return err
		}
		story.Feature = feature.ID
		document.Stories = append(document.Stories, story)
	}
	return nil
}

func loadStoryPackage(root, sagaID, dir, storyID string) (Story, error) {
	entries, err := boundedReadDir(dir, 3)
	if err != nil {
		return Story{}, err
	}
	foundIdentity := false
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 {
			return Story{}, fmt.Errorf("story %q contains symlink %q", storyID, entry.Name())
		}
		switch entry.Name() {
		case "story.json":
			if !entry.Type().IsRegular() {
				return Story{}, fmt.Errorf("story %q identity must be a regular file", storyID)
			}
			foundIdentity = true
		case "revisions", "events":
			if !entry.IsDir() {
				return Story{}, fmt.Errorf("story %q %s must be a real directory", storyID, entry.Name())
			}
		default:
			return Story{}, fmt.Errorf("story %q contains unknown entry %q", storyID, entry.Name())
		}
		_ = path
	}
	if !foundIdentity {
		return Story{}, fmt.Errorf("story %q is missing story.json", storyID)
	}
	var story Story
	identityPath := filepath.Join(dir, "story.json")
	if err := readStrictJSON(identityPath, &story.Identity); err != nil {
		return Story{}, fmt.Errorf("%s: %w", relative(root, identityPath), err)
	}
	if err := validateIdentity(story.Identity, storyID); err != nil {
		return Story{}, fmt.Errorf("%s: %w", relative(root, identityPath), err)
	}
	story.Revisions, err = loadRevisions(root, sagaID, storyID, filepath.Join(dir, "revisions"))
	if err != nil {
		return Story{}, err
	}
	story.Events, err = loadEvents(root, sagaID, storyID, filepath.Join(dir, "events"))
	if err != nil {
		return Story{}, err
	}
	if len(story.Revisions) == 0 || len(story.Events) == 0 {
		return Story{}, fmt.Errorf("story %q requires at least one revision and lifecycle event", storyID)
	}
	return story, nil
}

func loadRevisions(root, sagaID, storyID, dir string) ([]Revision, error) {
	present, err := realDirectory(dir)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("story %q is missing revisions directory", storyID)
	}
	entries, err := boundedReadDir(dir, MaxRevisionsPerStory)
	if err != nil {
		return nil, err
	}
	values := make([]Revision, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("revision entry %q must be a real JSON file", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		var value Revision
		if err := readStrictJSON(path, &value); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, path), err)
		}
		if strings.TrimSuffix(entry.Name(), ".json") != value.ID {
			return nil, fmt.Errorf("%s: revision id must match its filename", relative(root, path))
		}
		if err := validateRevision(value, sagaID, storyID); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, path), err)
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func loadEvents(root, sagaID, storyID, dir string) ([]LifecycleEvent, error) {
	present, err := realDirectory(dir)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("story %q is missing events directory", storyID)
	}
	entries, err := boundedReadDir(dir, MaxEventsPerStory)
	if err != nil {
		return nil, err
	}
	values := make([]LifecycleEvent, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("lifecycle entry %q must be a real JSON file", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		var value LifecycleEvent
		if err := readStrictJSON(path, &value); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, path), err)
		}
		if strings.TrimSuffix(entry.Name(), ".json") != value.ID {
			return nil, fmt.Errorf("%s: lifecycle event id must match its filename", relative(root, path))
		}
		if err := validateEvent(value, sagaID, storyID); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, path), err)
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func loadCitations(document *Document, feature applayout.Feature, ids *applayout.UniqueIDs) error {
	dir := filepath.Join(feature.Dir, applayout.RequirementsDir, "citations")
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxCitations)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("citation entry %q must be a real JSON file", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		var value Citation
		if err := readStrictJSON(path, &value); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		expectedID := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateCitation(value, document.SagaID, expectedID); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		if err := ids.Claim(value.ID, feature.ID); err != nil {
			return err
		}
		value.Feature = feature.ID
		document.Citations = append(document.Citations, value)
	}
	if len(document.Citations) > MaxCitations {
		return fmt.Errorf("the app has more than %d citations", MaxCitations)
	}
	return nil
}

func loadRelations(document *Document, feature applayout.Feature, ids *applayout.UniqueIDs) error {
	dir := filepath.Join(feature.Dir, applayout.RequirementsDir, "relations")
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxRelations)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("relation entry %q must be a real JSON file", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		var value Relation
		if err := readStrictJSON(path, &value); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		expectedID := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateRelation(value, document.SagaID, expectedID); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		if err := ids.Claim(value.ID, feature.ID); err != nil {
			return err
		}
		value.Feature = feature.ID
		document.Relations = append(document.Relations, value)
	}
	if len(document.Relations) > MaxRelations {
		return fmt.Errorf("the app has more than %d relations", MaxRelations)
	}
	return nil
}

// evaluateStaleness projects each relation's currency onto its legacy
// Stale/StaleReasons fields. The comparison itself lives in EvaluateRelation.
func evaluateStaleness(document *Document, inputs StaleInputs) {
	heads := newCurrencyHeads(*document, inputs)
	for index := range document.Relations {
		relation := &document.Relations[index]
		if relation.State != RelationActive {
			relation.Stale = false
			relation.StaleReasons = nil
			continue
		}
		currency := heads.evaluate(*relation)
		reasons := make([]string, 0, len(currency.Reasons))
		for _, reason := range currency.Reasons {
			reasons = append(reasons, reason.Message)
		}
		relation.StaleReasons = reasons
		relation.Stale = len(reasons) > 0
	}
}

func realDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("%s must be a real directory", path)
	}
	return true, nil
}

func boundedReadDir(path string, maximum int) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	if len(entries) > maximum {
		return nil, fmt.Errorf("%s contains %d entries; maximum is %d", path, len(entries), maximum)
	}
	return entries, nil
}

func readStrictJSON(path string, target any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("must be a real regular file")
	}
	if info.Size() > MaxRecordBytes {
		return fmt.Errorf("record is %d bytes; maximum is %d", info.Size(), MaxRecordBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(io.LimitReader(bytes.NewReader(data), MaxRecordBytes+1))
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

func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(value)
}
