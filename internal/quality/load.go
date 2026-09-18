package quality

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/diffuri"
	"github.com/twentyideas/changesaga/internal/qualityid"
)

type manifest struct {
	Schema  string         `json:"$schema,omitempty"`
	Version int            `json:"version"`
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	PR      *manifestPR    `json:"pr,omitempty"`
	Source  SourceIdentity `json:"source"`
}

type manifestPR struct {
	Number *int   `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
}

// Load strictly reads only saga.json and the bounded ___quality subtree. It
// never follows symlinks, writes files, executes commands, fetches URLs, or
// resolves referenced resources outside the Saga.
func Load(root string) (Document, error) {
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

	var identity manifest
	manifestPath := filepath.Join(abs, "saga.json")
	if err := readStrictJSON(manifestPath, &identity); err != nil {
		return Document{}, fmt.Errorf("read saga.json: %w", err)
	}
	if err := requireJSONFields(manifestPath, "version", "id", "title", "source"); err != nil {
		return Document{}, fmt.Errorf("read saga.json: %w", err)
	}
	if err := validateManifest(identity); err != nil {
		return Document{}, fmt.Errorf("saga.json: %w", err)
	}
	document := Document{
		Root: abs, SagaID: identity.ID, Source: identity.Source, Adoption: NotAdopted,
		TestCases: []TestCase{}, Policies: []Policy{}, PolicySets: []PolicySet{},
	}

	qualityRoot := filepath.Join(abs, RootDir)
	present, err := realDirectory(qualityRoot)
	if err != nil {
		return Document{}, err
	}
	if !present {
		return document, nil
	}
	document.Adoption = AdoptedEmpty
	entries, err := boundedReadDir(qualityRoot, 2)
	if err != nil {
		return Document{}, err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
			return Document{}, fmt.Errorf("quality entry %q must be a real directory", entry.Name())
		}
		switch entry.Name() {
		case "policies", "test-cases":
		default:
			return Document{}, fmt.Errorf("unknown quality entry %q", entry.Name())
		}
	}

	if err := loadPolicies(&document); err != nil {
		return Document{}, err
	}
	if err := loadTestCases(&document); err != nil {
		return Document{}, err
	}
	if len(document.TestCases) > 0 {
		document.Adoption = Adopted
	}
	if err := validateDocument(&document); err != nil {
		return Document{}, err
	}
	return document, nil
}

func loadPolicies(document *Document) error {
	dir := filepath.Join(document.Root, RootDir, "policies")
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxPolicies)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return fmt.Errorf("policy entry %q must be a real JSON file", entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		var value Policy
		if err := readStrictJSON(path, &value); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		if err := requireJSONFields(path, "$schema", "version", "id", "criterion", "story_revision", "required_kinds", "allowed_automation", "supersedes", "rationale", "created_at"); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		expectedID := strings.TrimSuffix(entry.Name(), ".json")
		if err := validatePolicy(value, document.SagaID, expectedID); err != nil {
			return fmt.Errorf("%s: %w", relative(document.Root, path), err)
		}
		document.Policies = append(document.Policies, value)
	}
	sort.Slice(document.Policies, func(i, j int) bool { return document.Policies[i].ID < document.Policies[j].ID })
	return nil
}

func loadTestCases(document *Document) error {
	dir := filepath.Join(document.Root, RootDir, "test-cases")
	present, err := realDirectory(dir)
	if err != nil || !present {
		return err
	}
	entries, err := boundedReadDir(dir, MaxTestCases)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".test") {
			return fmt.Errorf("test-case entry %q must be a real <id>.test directory", entry.Name())
		}
		testCaseID := strings.TrimSuffix(entry.Name(), ".test")
		if !qualityid.ValidID(testCaseID) {
			return fmt.Errorf("test-case package %q has an invalid id", entry.Name())
		}
		value, err := loadTestCasePackage(document.Root, document.SagaID, path, testCaseID)
		if err != nil {
			return err
		}
		document.TestCases = append(document.TestCases, value)
	}
	sort.Slice(document.TestCases, func(i, j int) bool { return document.TestCases[i].Identity.ID < document.TestCases[j].Identity.ID })
	return nil
}

func loadTestCasePackage(root, sagaID, dir, testCaseID string) (TestCase, error) {
	entries, err := boundedReadDir(dir, 5)
	if err != nil {
		return TestCase{}, err
	}
	foundIdentity := false
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 {
			return TestCase{}, fmt.Errorf("test case %q contains symlink %q", testCaseID, entry.Name())
		}
		switch entry.Name() {
		case "test-case.json":
			if !entry.Type().IsRegular() {
				return TestCase{}, fmt.Errorf("test case %q identity must be a regular file", testCaseID)
			}
			foundIdentity = true
		case "revisions", "events", "evidence", "runs":
			if !entry.IsDir() {
				return TestCase{}, fmt.Errorf("test case %q %s must be a real directory", testCaseID, entry.Name())
			}
		default:
			return TestCase{}, fmt.Errorf("test case %q contains unknown entry %q", testCaseID, entry.Name())
		}
	}
	if !foundIdentity {
		return TestCase{}, fmt.Errorf("test case %q is missing test-case.json", testCaseID)
	}

	var testCase TestCase
	identityPath := filepath.Join(dir, "test-case.json")
	if err := readStrictJSON(identityPath, &testCase.Identity); err != nil {
		return TestCase{}, fmt.Errorf("%s: %w", relative(root, identityPath), err)
	}
	if err := requireJSONFields(identityPath, "$schema", "version", "id", "created_at"); err != nil {
		return TestCase{}, fmt.Errorf("%s: %w", relative(root, identityPath), err)
	}
	if err := validateIdentity(testCase.Identity, testCaseID); err != nil {
		return TestCase{}, fmt.Errorf("%s: %w", relative(root, identityPath), err)
	}
	if testCase.Revisions, err = loadRevisions(root, sagaID, testCaseID, filepath.Join(dir, "revisions")); err != nil {
		return TestCase{}, err
	}
	if testCase.Events, err = loadEvents(root, sagaID, testCaseID, filepath.Join(dir, "events")); err != nil {
		return TestCase{}, err
	}
	if testCase.Evidence, err = loadEvidence(root, sagaID, testCaseID, filepath.Join(dir, "evidence")); err != nil {
		return TestCase{}, err
	}
	if testCase.Runs, err = loadRuns(root, sagaID, testCaseID, filepath.Join(dir, "runs")); err != nil {
		return TestCase{}, err
	}
	if len(testCase.Revisions) == 0 || len(testCase.Events) == 0 {
		return TestCase{}, fmt.Errorf("test case %q requires at least one revision and lifecycle event", testCaseID)
	}
	return testCase, nil
}

func loadRevisions(root, sagaID, testCaseID, dir string) ([]Revision, error) {
	return loadRequiredRecords(root, testCaseID, dir, "revision", MaxRevisionsPerTest, func(path, filenameID string) (Revision, error) {
		var value Revision
		if err := readStrictJSON(path, &value); err != nil {
			return value, err
		}
		if err := requireJSONFields(path, "$schema", "version", "id", "test_case", "parents", "title", "coverage_kinds", "automation", "preconditions", "steps", "expected_result", "created_at"); err != nil {
			return value, err
		}
		if value.ID != filenameID {
			return value, fmt.Errorf("revision id must match its filename")
		}
		return value, validateRevision(value, sagaID, testCaseID)
	})
}

func loadEvents(root, sagaID, testCaseID, dir string) ([]LifecycleEvent, error) {
	return loadRequiredRecords(root, testCaseID, dir, "lifecycle event", MaxEventsPerTest, func(path, filenameID string) (LifecycleEvent, error) {
		var value LifecycleEvent
		if err := readStrictJSON(path, &value); err != nil {
			return value, err
		}
		if err := requireJSONFields(path, "$schema", "version", "id", "test_case", "parents", "state", "created_at"); err != nil {
			return value, err
		}
		if value.ID != filenameID {
			return value, fmt.Errorf("lifecycle event id must match its filename")
		}
		return value, validateLifecycleEvent(value, sagaID, testCaseID)
	})
}

func loadEvidence(root, sagaID, testCaseID, dir string) ([]Evidence, error) {
	return loadOptionalRecords(root, dir, "evidence", MaxEvidencePerTest, func(path, filenameID string) (Evidence, error) {
		var value Evidence
		if err := readStrictJSON(path, &value); err != nil {
			return value, err
		}
		if err := requireJSONFields(path, "$schema", "version", "id", "test_case", "test_revision", "role", "diffs", "verifications", "citations", "supersedes", "created_at"); err != nil {
			return value, err
		}
		if value.ID != filenameID {
			return value, fmt.Errorf("evidence id must match its filename")
		}
		return value, validateEvidence(value, sagaID, testCaseID)
	})
}

func loadRuns(root, sagaID, testCaseID, dir string) ([]Run, error) {
	return loadOptionalRecords(root, dir, "run", MaxRunsPerTest, func(path, filenameID string) (Run, error) {
		var value Run
		if err := readStrictJSON(path, &value); err != nil {
			return value, err
		}
		if err := requireJSONFields(path, "$schema", "version", "id", "test_case", "test_revision", "parents", "source", "result", "summary", "evidence", "executed_at"); err != nil {
			return value, err
		}
		if err := validateOptionalNonblankString(path, "command"); err != nil {
			return value, err
		}
		if value.ID != filenameID {
			return value, fmt.Errorf("run id must match its filename")
		}
		return value, validateRun(value, sagaID, testCaseID)
	})
}

func loadRequiredRecords[T any](root, testCaseID, dir, kind string, maximum int, decode func(string, string) (T, error)) ([]T, error) {
	present, err := realDirectory(dir)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("test case %q is missing %s directory", testCaseID, filepath.Base(dir))
	}
	return loadRecords(root, dir, kind, maximum, decode)
}

func loadOptionalRecords[T any](root, dir, kind string, maximum int, decode func(string, string) (T, error)) ([]T, error) {
	present, err := realDirectory(dir)
	if err != nil || !present {
		return []T{}, err
	}
	return loadRecords(root, dir, kind, maximum, decode)
}

func loadRecords[T any](root, dir, kind string, maximum int, decode func(string, string) (T, error)) ([]T, error) {
	entries, err := boundedReadDir(dir, maximum)
	if err != nil {
		return nil, err
	}
	values := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("%s entry %q must be a real JSON file", kind, entry.Name())
		}
		path := filepath.Join(dir, entry.Name())
		value, err := decode(path, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, path), err)
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		left, _ := json.Marshal(values[i])
		right, _ := json.Marshal(values[j])
		return bytes.Compare(left, right) < 0
	})
	return values, nil
}

func validateManifest(value manifest) error {
	var problems validationErrors
	if value.Version != Version {
		problems.add("unsupported Saga version %d; change-saga reads only version %d", value.Version, Version)
	}
	if !qualityid.ValidID(value.ID) {
		problems.add("saga id is invalid")
	}
	if strings.TrimSpace(value.Title) == "" {
		problems.add("title is required")
	}
	if canonical, err := diffuri.CanonicalRepository(value.Source.Repository); err != nil || canonical != value.Source.Repository {
		problems.add("source.repository must be a canonical absolute repository URI")
	}
	if strings.TrimSpace(value.Source.Base) == "" || strings.TrimSpace(value.Source.Head) == "" {
		problems.add("source.base and source.head are required")
	}
	if value.PR != nil {
		if value.PR.Number != nil && *value.PR.Number < 1 {
			problems.add("pr.number must be positive")
		}
		if value.PR.URL != "" {
			parsed, err := url.Parse(value.PR.URL)
			if err != nil || !parsed.IsAbs() {
				problems.add("pr.url must be an absolute URI")
			}
		}
	}
	return problems.err()
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
	var generic any
	if err := json.Unmarshal(data, &generic); err != nil {
		return err
	}
	if containsJSONNull(generic) {
		return fmt.Errorf("null is not allowed")
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

func requireJSONFields(path string, names ...string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, name := range names {
		if _, exists := fields[name]; !exists {
			return fmt.Errorf("required field %q is missing", name)
		}
	}
	return nil
}

func validateOptionalNonblankString(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	raw, exists := fields[name]
	if !exists {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil || strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must be a nonblank string when present", name)
	}
	return nil
}

func containsJSONNull(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if containsJSONNull(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if containsJSONNull(item) {
				return true
			}
		}
	}
	return false
}

func relative(root, path string) string {
	value, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(value)
}
