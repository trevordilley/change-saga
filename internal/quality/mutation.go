package quality

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/store"
)

// MutationResult describes one committed (or replayed) quality write. Paths
// are app-relative and slash-separated, including the
// ___features/<id>.feature/ prefix of the feature holding the record; CurrentHeads are the graph heads the
// write leaves behind so the caller can name them as the next parents.
type MutationResult struct {
	URN          string   `json:"urn"`
	Path         string   `json:"path"`
	Created      []string `json:"created"`
	Paths        []string `json:"paths"`
	CurrentHeads []string `json:"current_heads"`
	Replayed     bool     `json:"replayed"`
}

// Definition is the complete authored content of one test-case revision.
type Definition struct {
	Title          string
	CoverageKinds  []CoverageKind
	Automation     Automation
	Preconditions  []string
	Steps          []Step
	ExpectedResult string
}

type AddTestCaseInput struct {
	// Feature is the feature the new test case is written into.
	Feature    string
	ID         string
	RevisionID string
	EventID    string
	Definition Definition
	CreatedAt  time.Time
	RequestID  string
}

type ReviseTestCaseInput struct {
	TestCase   string
	RevisionID string
	Parents    []string
	Definition Definition
	CreatedAt  time.Time
	RequestID  string
}

type SetTestCaseStateInput struct {
	TestCase  string
	EventID   string
	Parents   []string
	State     LifecycleState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

type SetPolicyInput struct {
	// Feature is the feature the new policy is written into.
	Feature           string
	ID                string
	Criterion         string
	StoryRevision     string
	RequiredKinds     []CoverageKind
	AllowedAutomation []Automation
	Supersedes        []string
	Rationale         string
	CreatedAt         time.Time
	RequestID         string
}

type AddEvidenceInput struct {
	ID            string
	TestCase      string
	TestRevision  string
	Role          EvidenceRole
	Code          []coderef.Reference
	Verifications []string
	Citations     []string
	Supersedes    []string
	CreatedAt     time.Time
	RequestID     string
}

type RecordRunInput struct {
	ID           string
	TestCase     string
	TestRevision string
	Parents      []string
	Source       *SourceIdentity
	Result       RunResult
	Summary      string
	Command      string
	Evidence     []string
	ExecutedAt   time.Time
	RequestID    string
}

// AddTestCase publishes a complete test-case package: identity, its first
// revision, and the proposed lifecycle root. The package appears atomically.
func AddTestCase(root string, input AddTestCaseInput) (MutationResult, error) {
	if input.EventID == "" {
		input.EventID = string(StateProposed)
	}
	var result MutationResult
	err := mutate(root, func(document *Document) error {
		sagaID := document.SagaID
		testCaseURN, err := qualityid.TestCase(sagaID, input.ID)
		if err != nil {
			return fmt.Errorf("test-case id: %w", err)
		}
		createdAt := mutationTime(input.CreatedAt)
		identity := TestCaseIdentity{Schema: TestCaseSchemaURL, Version: Version, ID: input.ID, CreatedAt: createdAt, RequestID: input.RequestID}
		revision := newRevision(input.RevisionID, testCaseURN, []string{}, input.Definition, createdAt, input.RequestID)
		event := LifecycleEvent{
			Schema: LifecycleEventSchemaURL, Version: Version, ID: input.EventID, TestCase: testCaseURN,
			Parents: []string{}, State: StateProposed, CreatedAt: createdAt, RequestID: input.RequestID,
		}
		if err := validateIdentity(identity, input.ID); err != nil {
			return err
		}
		if err := validateRevision(revision, sagaID, input.ID); err != nil {
			return err
		}
		if err := validateLifecycleEvent(event, sagaID, input.ID); err != nil {
			return err
		}
		revisionURN, _ := qualityid.Revision(sagaID, input.ID, revision.ID)
		eventURN, _ := qualityid.Event(sagaID, input.ID, event.ID)
		created := []string{testCaseURN, revisionURN, eventURN}
		paths := []string{testCasePath(input.Feature, input.ID), revisionPath(input.Feature, input.ID, revision.ID), eventPath(input.Feature, input.ID, event.ID)}
		if existing := findTestCase(document, input.ID); existing != nil {
			if existing.Feature == input.Feature && replayableTestCase(*existing, identity, revision, event) {
				result = MutationResult{URN: testCaseURN, Path: paths[0], Created: created, Paths: paths,
					CurrentHeads: append(copyStrings(existing.RevisionHeads), existing.LifecycleHeads...), Replayed: true}
				return nil
			}
			return fmt.Errorf("test case %q already exists", input.ID)
		}
		if len(document.TestCases) >= MaxTestCases {
			return fmt.Errorf("test-case limit of %d reached", MaxTestCases)
		}
		feature, err := document.feature(input.Feature)
		if err != nil {
			return err
		}
		candidate := TestCase{Identity: identity, Revisions: []Revision{revision}, Events: []LifecycleEvent{event}}
		if err := validateTestCaseGraphs(&candidate, sagaID, document.Source); err != nil {
			return err
		}
		var rollback dirRollback
		parent, err := rollback.ensure(document.Root, filepath.Join(filepath.FromSlash(feature.Rel), RootDir, "test-cases"))
		if err != nil {
			rollback.undo()
			return err
		}
		err = store.CommitDir(document.Root, filepath.Join(parent, input.ID+".test"), func(stage string) error {
			for _, dir := range []string{"revisions", "events"} {
				if err := os.Mkdir(filepath.Join(stage, dir), 0o755); err != nil {
					return err
				}
			}
			if err := store.WriteJSON(filepath.Join(stage, "test-case.json"), identity, true); err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(stage, "revisions", revision.ID+".json"), revision, true); err != nil {
				return err
			}
			return store.WriteJSON(filepath.Join(stage, "events", event.ID+".json"), event, true)
		})
		if err != nil {
			rollback.undo()
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("test case %q already exists", input.ID)
			}
			return err
		}
		result = MutationResult{URN: testCaseURN, Path: paths[0], Created: created, Paths: paths, CurrentHeads: []string{revisionURN, eventURN}}
		return nil
	})
	return result, err
}

// ReviseTestCase appends one complete immutable revision. Parents must name
// every current revision head, so concurrent edits are reconciled explicitly.
func ReviseTestCase(root string, input ReviseTestCaseInput) (MutationResult, error) {
	var result MutationResult
	err := mutateTestCase(root, input.TestCase, func(document *Document, testCase *TestCase) error {
		sagaID, testCaseID := document.SagaID, testCase.Identity.ID
		revision := newRevision(input.RevisionID, input.TestCase, copyStrings(input.Parents), input.Definition, mutationTime(input.CreatedAt), input.RequestID)
		if err := validateRevision(revision, sagaID, testCaseID); err != nil {
			return err
		}
		urn, _ := qualityid.Revision(sagaID, testCaseID, revision.ID)
		path := revisionPath(testCase.Feature, testCaseID, revision.ID)
		for _, existing := range testCase.Revisions {
			if existing.ID != revision.ID {
				continue
			}
			if replayable(existing.RequestID, input.RequestID, existing, revision, "CreatedAt") {
				result = singleResult(urn, path, testCase.RevisionHeads, true)
				return nil
			}
			return fmt.Errorf("revision id %q already exists", revision.ID)
		}
		if len(testCase.Revisions) >= MaxRevisionsPerTest {
			return fmt.Errorf("revision limit of %d reached", MaxRevisionsPerTest)
		}
		if !sameSet(revision.Parents, testCase.RevisionHeads) {
			return fmt.Errorf("revision parents must name every current head (got %v, want %v)", revision.Parents, testCase.RevisionHeads)
		}
		candidate := *testCase
		candidate.Revisions = append(append([]Revision{}, testCase.Revisions...), revision)
		if err := validateCandidate(&candidate, document); err != nil {
			return err
		}
		if err := writeRecords(document.Root, pendingRecord{path: path, value: revision}); err != nil {
			return err
		}
		result = singleResult(urn, path, []string{urn}, false)
		return nil
	})
	return result, err
}

// SetTestCaseState appends a lifecycle event. Naming more than one current
// head is an explicit reconciliation and may choose any state.
func SetTestCaseState(root string, input SetTestCaseStateInput) (MutationResult, error) {
	var result MutationResult
	err := mutateTestCase(root, input.TestCase, func(document *Document, testCase *TestCase) error {
		sagaID, testCaseID := document.SagaID, testCase.Identity.ID
		event := LifecycleEvent{
			Schema: LifecycleEventSchemaURL, Version: Version, ID: input.EventID, TestCase: input.TestCase,
			Parents: copyStrings(input.Parents), State: input.State, Reason: strings.TrimSpace(input.Reason),
			CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
		}
		if err := validateLifecycleEvent(event, sagaID, testCaseID); err != nil {
			return err
		}
		urn, _ := qualityid.Event(sagaID, testCaseID, event.ID)
		path := eventPath(testCase.Feature, testCaseID, event.ID)
		for _, existing := range testCase.Events {
			if existing.ID != event.ID {
				continue
			}
			if replayable(existing.RequestID, input.RequestID, existing, event, "CreatedAt") {
				result = singleResult(urn, path, testCase.LifecycleHeads, true)
				return nil
			}
			return fmt.Errorf("lifecycle event id %q already exists", event.ID)
		}
		if len(testCase.Events) >= MaxEventsPerTest {
			return fmt.Errorf("lifecycle event limit of %d reached", MaxEventsPerTest)
		}
		if !sameSet(event.Parents, testCase.LifecycleHeads) {
			return fmt.Errorf("lifecycle parents must name every current head (got %v, want %v)", event.Parents, testCase.LifecycleHeads)
		}
		candidate := *testCase
		candidate.Events = append(append([]LifecycleEvent{}, testCase.Events...), event)
		if err := validateCandidate(&candidate, document); err != nil {
			return err
		}
		if err := writeRecords(document.Root, pendingRecord{path: path, value: event}); err != nil {
			return err
		}
		result = singleResult(urn, path, []string{urn}, false)
		return nil
	})
	return result, err
}

// SetPolicy records an immutable per-criterion kind policy. Supersedes must
// name every current policy head for the same criterion and story revision so
// exactly one head remains; it may also retire policies for older revisions.
// Without an explicit ID the policy is named after its criterion pin.
func SetPolicy(root string, input SetPolicyInput) (MutationResult, error) {
	var result MutationResult
	err := mutate(root, func(document *Document) error {
		sagaID := document.SagaID
		value := Policy{
			Schema: PolicySchemaURL, Version: Version, ID: input.ID, Criterion: input.Criterion,
			StoryRevision: input.StoryRevision, RequiredKinds: sortedKinds(input.RequiredKinds),
			AllowedAutomation: sortedAutomation(input.AllowedAutomation), Supersedes: copyStrings(input.Supersedes),
			Rationale: strings.TrimSpace(input.Rationale), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
			Feature: input.Feature,
		}
		if replayed, ok := replayPolicy(document, value, input.ID == ""); ok {
			result = replayed
			return nil
		}
		if value.ID == "" {
			value.ID = defaultPolicyID(document, input.Criterion, input.StoryRevision)
		}
		if err := validatePolicy(value, sagaID, value.ID); err != nil {
			return err
		}
		urn, _ := qualityid.QualityPolicy(sagaID, value.ID)
		path := policyPath(value.Feature, value.ID)
		for _, existing := range document.Policies {
			if existing.ID == value.ID {
				return fmt.Errorf("policy id %q already exists; policies are immutable, supersede it with a new id", value.ID)
			}
		}
		if len(document.Policies) >= MaxPolicies {
			return fmt.Errorf("policy limit of %d reached", MaxPolicies)
		}
		if _, err := document.feature(value.Feature); err != nil {
			return err
		}
		heads := []string{}
		for _, set := range document.PolicySets {
			if set.Criterion == value.Criterion && set.StoryRevision == value.StoryRevision {
				heads = set.Heads
			}
		}
		named := map[string]bool{}
		for _, superseded := range value.Supersedes {
			named[superseded] = true
		}
		for _, head := range heads {
			if !named[head] {
				return fmt.Errorf("policy must supersede every current head for this criterion and story revision (missing %s)", head)
			}
		}
		superseded := map[string]bool{}
		for _, policy := range document.Policies {
			for _, parent := range policy.Supersedes {
				superseded[parent] = true
			}
		}
		for _, parent := range value.Supersedes {
			if superseded[parent] {
				return fmt.Errorf("policy %s is already superseded", parent)
			}
		}
		candidate := *document
		candidate.Policies = append(append([]Policy{}, document.Policies...), value)
		candidate.PolicySets = nil
		if err := validatePolicyGraph(&candidate); err != nil {
			return err
		}
		if err := writeRecords(document.Root, pendingRecord{path: path, value: value}); err != nil {
			return err
		}
		result = singleResult(urn, path, []string{urn}, false)
		return nil
	})
	return result, err
}

// AddEvidence records one immutable evidence record.
func AddEvidence(root string, input AddEvidenceInput) (MutationResult, error) {
	results, err := AddEvidenceBatch(root, []AddEvidenceInput{input})
	if err != nil {
		return MutationResult{}, err
	}
	return results[0], nil
}

// AddEvidenceBatch preflights and validates the complete set against one
// locked snapshot before the first write, and removes every record it wrote
// if a later write fails. Replayed requests are reported, not rewritten.
func AddEvidenceBatch(root string, inputs []AddEvidenceInput) ([]MutationResult, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("evidence batch is empty")
	}
	var results []MutationResult
	err := mutate(root, func(document *Document) error {
		sagaID := document.SagaID
		candidates := map[string]*TestCase{}
		pending := []pendingRecord{}
		results = make([]MutationResult, 0, len(inputs))
		for index, input := range inputs {
			label := fmt.Sprintf("evidence %d", index+1)
			testCase, err := resolveTestCase(document, input.TestCase)
			if err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			testCaseID := testCase.Identity.ID
			if candidates[testCaseID] == nil {
				copy := *testCase
				copy.Evidence = append([]Evidence{}, testCase.Evidence...)
				candidates[testCaseID] = &copy
			}
			candidate := candidates[testCaseID]
			value := Evidence{
				Schema: EvidenceSchemaURL, Version: Version, ID: input.ID, TestCase: input.TestCase,
				TestRevision: input.TestRevision, Role: input.Role, Code: append([]coderef.Reference{}, input.Code...),
				Verifications: copyStrings(input.Verifications), Citations: copyStrings(input.Citations),
				Supersedes: copyStrings(input.Supersedes), CreatedAt: mutationTime(input.CreatedAt), RequestID: input.RequestID,
			}
			if value.TestRevision == "" {
				if value.TestRevision, err = uniqueHead(testCase.RevisionHeads, "revision"); err != nil {
					return fmt.Errorf("%s: %w", label, err)
				}
			}
			if replayedValue, ok := findReplay(testCase.Evidence, value, input.ID == "", func(e Evidence) (string, string) { return e.ID, e.RequestID }); ok {
				urn, _ := qualityid.Evidence(sagaID, testCaseID, replayedValue.ID)
				results = append(results, singleResult(urn, evidencePath(testCase.Feature, testCaseID, replayedValue.ID), testCase.EvidenceHeads, true))
				continue
			}
			if value.ID == "" {
				value.ID = nextID(string(value.Role), func(id string) bool { return hasEvidence(candidate.Evidence, id) })
			}
			if err := validateEvidence(value, sagaID, testCaseID); err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
			if hasEvidence(candidate.Evidence, value.ID) {
				return fmt.Errorf("%s: evidence id %q already exists", label, value.ID)
			}
			if len(candidate.Evidence) >= MaxEvidencePerTest {
				return fmt.Errorf("%s: evidence limit of %d reached", label, MaxEvidencePerTest)
			}
			if !contains(testCase.RevisionHeads, value.TestRevision) {
				return fmt.Errorf("%s: test_revision must be a current revision head %v", label, testCase.RevisionHeads)
			}
			for _, superseded := range value.Supersedes {
				if !contains(candidateEvidenceHeads(candidate, sagaID), superseded) {
					return fmt.Errorf("%s: can only supersede current evidence heads; %s is not one", label, superseded)
				}
			}
			candidate.Evidence = append(candidate.Evidence, value)
			urn, _ := qualityid.Evidence(sagaID, testCaseID, value.ID)
			path := evidencePath(testCase.Feature, testCaseID, value.ID)
			pending = append(pending, pendingRecord{path: path, value: value})
			results = append(results, MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}})
		}
		for _, candidate := range candidates {
			if err := validateCandidate(candidate, document); err != nil {
				return err
			}
		}
		for index := range results {
			if results[index].Replayed {
				continue
			}
			ref, _ := qualityid.Parse(results[index].URN)
			results[index].CurrentHeads = candidateEvidenceHeads(candidates[ref.TestCaseID], sagaID)
		}
		return writeRecords(document.Root, pending...)
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// RecordRun appends one immutable run/result event. Parents must name every
// current run head: a failed or conflicting head stays in history and is
// only ever succeeded by a later run, never replaced.
func RecordRun(root string, input RecordRunInput) (MutationResult, error) {
	var result MutationResult
	err := mutateTestCase(root, input.TestCase, func(document *Document, testCase *TestCase) error {
		sagaID, testCaseID := document.SagaID, testCase.Identity.ID
		source := document.Source
		if input.Source != nil {
			source.Commit = input.Source.Commit
			if input.Source.Repository != "" {
				source.Repository = input.Source.Repository
			}
		}
		value := Run{
			Schema: RunSchemaURL, Version: Version, ID: input.ID, TestCase: input.TestCase,
			TestRevision: input.TestRevision, Parents: copyStrings(input.Parents), Source: source,
			Result: input.Result, Summary: strings.TrimSpace(input.Summary), Command: strings.TrimSpace(input.Command),
			Evidence: copyStrings(input.Evidence), ExecutedAt: mutationTime(input.ExecutedAt), RequestID: input.RequestID,
		}
		if value.TestRevision == "" {
			var err error
			if value.TestRevision, err = uniqueHead(testCase.RevisionHeads, "revision"); err != nil {
				return err
			}
		}
		if replayedValue, ok := findReplay(testCase.Runs, value, input.ID == "", func(r Run) (string, string) { return r.ID, r.RequestID }); ok {
			urn, _ := qualityid.Run(sagaID, testCaseID, replayedValue.ID)
			result = singleResult(urn, runPath(testCase.Feature, testCaseID, replayedValue.ID), testCase.RunHeads, true)
			return nil
		}
		if value.ID == "" {
			value.ID = store.EventID(value.ExecutedAt)
		}
		if err := validateRun(value, sagaID, testCaseID); err != nil {
			return err
		}
		for _, existing := range testCase.Runs {
			if existing.ID == value.ID {
				return fmt.Errorf("run id %q already exists; runs are immutable", value.ID)
			}
		}
		if len(testCase.Runs) >= MaxRunsPerTest {
			return fmt.Errorf("run limit of %d reached", MaxRunsPerTest)
		}
		if !sameSet(value.Parents, testCase.RunHeads) {
			return fmt.Errorf("run parents must name every current run head (got %v, want %v)", value.Parents, testCase.RunHeads)
		}
		candidate := *testCase
		candidate.Runs = append(append([]Run{}, testCase.Runs...), value)
		if err := validateCandidate(&candidate, document); err != nil {
			return err
		}
		urn, _ := qualityid.Run(sagaID, testCaseID, value.ID)
		path := runPath(testCase.Feature, testCaseID, value.ID)
		if err := writeRecords(document.Root, pendingRecord{path: path, value: value}); err != nil {
			return err
		}
		result = singleResult(urn, path, []string{urn}, false)
		return nil
	})
	return result, err
}

// mutate validates the Saga before and after taking the writer lock so a
// structurally invalid tree is refused without waiting, and the operation
// always sees the freshest committed state.
func mutate(root string, operation func(*Document) error) error {
	if _, err := Load(root); err != nil {
		return fmt.Errorf("cannot mutate quality: %w", err)
	}
	return store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, err := Load(root)
		if err != nil {
			return fmt.Errorf("cannot mutate quality after locking: %w", err)
		}
		return operation(&document)
	})
}

func mutateTestCase(root, testCaseURN string, operation func(*Document, *TestCase) error) error {
	return mutate(root, func(document *Document) error {
		testCase, err := resolveTestCase(document, testCaseURN)
		if err != nil {
			return err
		}
		return operation(document, testCase)
	})
}

func resolveTestCase(document *Document, urn string) (*TestCase, error) {
	ref, err := qualityid.Parse(urn)
	if err != nil || ref.Kind != qualityid.KindTestCase || ref.SagaID != document.SagaID {
		return nil, fmt.Errorf("test case must be a canonical test-case URN in saga %q", document.SagaID)
	}
	testCase := findTestCase(document, ref.ID)
	if testCase == nil {
		return nil, fmt.Errorf("test case %q does not exist", ref.ID)
	}
	return testCase, nil
}

func findTestCase(document *Document, id string) *TestCase {
	for index := range document.TestCases {
		if document.TestCases[index].Identity.ID == id {
			return &document.TestCases[index]
		}
	}
	return nil
}

// validateCandidate runs the same graph rules the loader enforces against a
// copy that includes the proposed record, so a write can never produce a
// Saga that Load would reject.
func validateCandidate(candidate *TestCase, document *Document) error {
	if err := validateTestCaseGraphs(candidate, document.SagaID, document.Source); err != nil {
		return fmt.Errorf("test case %q: %w", candidate.Identity.ID, err)
	}
	return nil
}

func candidateEvidenceHeads(candidate *TestCase, sagaID string) []string {
	parents := map[string][]string{}
	ids := make([]string, 0, len(candidate.Evidence))
	for _, value := range candidate.Evidence {
		ids = append(ids, value.ID)
		for _, parent := range value.Supersedes {
			if id, ok := nestedID(parent, qualityid.KindEvidence, sagaID, candidate.Identity.ID); ok {
				parents[value.ID] = append(parents[value.ID], id)
			}
		}
	}
	heads, _, _ := graphHeads(parents, ids)
	return buildNestedURNs(sagaID, candidate.Identity.ID, qualityid.KindEvidence, heads)
}

func newRevision(id, testCase string, parents []string, definition Definition, createdAt time.Time, requestID string) Revision {
	preconditions := []string{}
	for _, value := range definition.Preconditions {
		preconditions = append(preconditions, strings.TrimSpace(value))
	}
	steps := []Step{}
	for _, step := range definition.Steps {
		steps = append(steps, Step{ID: step.ID, Action: strings.TrimSpace(step.Action), ExpectedResult: strings.TrimSpace(step.ExpectedResult)})
	}
	return Revision{
		Schema: RevisionSchemaURL, Version: Version, ID: id, TestCase: testCase, Parents: parents,
		Title: strings.TrimSpace(definition.Title), CoverageKinds: sortedKinds(definition.CoverageKinds),
		Automation: definition.Automation, Preconditions: preconditions, Steps: steps,
		ExpectedResult: strings.TrimSpace(definition.ExpectedResult), CreatedAt: createdAt, RequestID: requestID,
	}
}

// DefinitionOf returns the authored content of a stored revision so callers
// can build a complete successor from a single parent.
func DefinitionOf(revision Revision) Definition {
	return Definition{
		Title: revision.Title, CoverageKinds: append([]CoverageKind{}, revision.CoverageKinds...),
		Automation: revision.Automation, Preconditions: copyStrings(revision.Preconditions),
		Steps: append([]Step{}, revision.Steps...), ExpectedResult: revision.ExpectedResult,
	}
}

func replayableTestCase(existing TestCase, identity TestCaseIdentity, revision Revision, event LifecycleEvent) bool {
	if identity.RequestID == "" || existing.Identity.RequestID != identity.RequestID {
		return false
	}
	identity.CreatedAt = existing.Identity.CreatedAt
	if !reflect.DeepEqual(existing.Identity, identity) {
		return false
	}
	matchedRevision, matchedEvent := false, false
	for _, stored := range existing.Revisions {
		if stored.ID == revision.ID {
			matchedRevision = replayable(stored.RequestID, revision.RequestID, stored, revision, "CreatedAt")
		}
	}
	for _, stored := range existing.Events {
		if stored.ID == event.ID {
			matchedEvent = replayable(stored.RequestID, event.RequestID, stored, event, "CreatedAt")
		}
	}
	return matchedRevision && matchedEvent
}

// replayable reports whether wanted repeats an idempotent request that
// produced existing. Only the named time field may differ.
func replayable[T any](existingRequest, wantedRequest string, existing, wanted T, timeField string) bool {
	if wantedRequest == "" || existingRequest != wantedRequest {
		return false
	}
	return equalIgnoring(existing, wanted, timeField)
}

func equalIgnoring[T any](existing, wanted T, fields ...string) bool {
	left := reflect.ValueOf(&existing).Elem()
	right := reflect.ValueOf(&wanted).Elem()
	for _, name := range append(fields, "Current", "StaleReasons") {
		if field := right.FieldByName(name); field.IsValid() {
			field.Set(left.FieldByName(name))
		}
	}
	return reflect.DeepEqual(left.Interface(), right.Interface())
}

// findReplay locates an earlier record written by the same request. When the
// caller let the writer generate the ID, the stored ID is adopted.
func findReplay[T any](values []T, wanted T, generatedID bool, key func(T) (string, string)) (T, bool) {
	var zero T
	wantedID, wantedRequest := key(wanted)
	if wantedRequest == "" {
		return zero, false
	}
	for _, existing := range values {
		id, request := key(existing)
		if request != wantedRequest || (!generatedID && id != wantedID) {
			continue
		}
		ignore := []string{"CreatedAt", "ExecutedAt"}
		if generatedID {
			ignore = append(ignore, "ID")
		}
		if equalIgnoring(existing, wanted, ignore...) {
			return existing, true
		}
	}
	return zero, false
}

func replayPolicy(document *Document, value Policy, generatedID bool) (MutationResult, bool) {
	existing, ok := findReplay(document.Policies, value, generatedID, func(p Policy) (string, string) { return p.ID, p.RequestID })
	if !ok {
		return MutationResult{}, false
	}
	urn, _ := qualityid.QualityPolicy(document.SagaID, existing.ID)
	heads := []string{}
	for _, set := range document.PolicySets {
		if set.Criterion == existing.Criterion && set.StoryRevision == existing.StoryRevision {
			heads = set.Heads
		}
	}
	return singleResult(urn, policyPath(existing.Feature, existing.ID), heads, true), true
}

func defaultPolicyID(document *Document, criterion, storyRevision string) string {
	base := "policy"
	criterionRef, criterionErr := livingid.Parse(criterion)
	revisionRef, revisionErr := livingid.Parse(storyRevision)
	if criterionErr == nil && revisionErr == nil {
		base = criterionRef.ParentID + "." + criterionRef.ID + "." + revisionRef.ID
	}
	if len(base) > 120 {
		base = strings.TrimRight(base[:120], "._-")
	}
	return nextID(base, func(id string) bool {
		for _, policy := range document.Policies {
			if policy.ID == id {
				return true
			}
		}
		return false
	})
}

// nextID returns base, or base-N for the first unused N >= 2, keeping
// generated identifiers readable and deterministic under the writer lock.
func nextID(base string, exists func(string) bool) string {
	base = strings.ReplaceAll(base, "_", "-")
	if !exists(base) {
		return base
	}
	for n := 2; ; n++ {
		id := base + "-" + strconv.Itoa(n)
		if !exists(id) {
			return id
		}
	}
}

func hasEvidence(values []Evidence, id string) bool {
	for _, value := range values {
		if value.ID == id {
			return true
		}
	}
	return false
}

func uniqueHead(heads []string, kind string) (string, error) {
	if len(heads) != 1 {
		return "", fmt.Errorf("test case has %d %s heads %v; name the test revision explicitly or reconcile first", len(heads), kind, heads)
	}
	return heads[0], nil
}

func singleResult(urn, path string, heads []string, replayed bool) MutationResult {
	return MutationResult{URN: urn, Path: path, Created: []string{urn}, Paths: []string{path}, CurrentHeads: copyStrings(heads), Replayed: replayed}
}

type pendingRecord struct {
	path  string
	value any
}

// writeRecords exclusively creates every record or none: directories and
// files created before a failure are removed in reverse order.
func writeRecords(root string, records ...pendingRecord) error {
	var rollback dirRollback
	for _, record := range records {
		dir, err := rollback.ensure(root, filepath.Dir(filepath.FromSlash(record.path)))
		if err == nil {
			full := filepath.Join(dir, filepath.Base(record.path))
			err = store.WriteJSON(full, record.value, true)
			if err == nil {
				rollback.files = append(rollback.files, full)
			} else if errors.Is(err, fs.ErrExist) {
				err = fmt.Errorf("%s already exists; quality records are immutable", record.path)
			}
		}
		if err != nil {
			rollback.undo()
			return err
		}
	}
	return nil
}

type dirRollback struct {
	dirs  []string
	files []string
}

// ensure creates a Saga-relative directory, remembering each component it
// created so a failed mutation can restore the prior tree exactly.
func (r *dirRollback) ensure(root, relative string) (string, error) {
	current := root
	for _, part := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if _, err := os.Lstat(current); errors.Is(err, fs.ErrNotExist) {
			if _, err := store.EnsureDirWithin(root, current); err != nil {
				return "", err
			}
			r.dirs = append(r.dirs, current)
		} else if err != nil {
			return "", err
		}
	}
	return store.EnsureDirWithin(root, filepath.Join(root, relative))
}

func (r *dirRollback) undo() {
	for index := len(r.files) - 1; index >= 0; index-- {
		_ = os.Remove(r.files[index])
	}
	for index := len(r.dirs) - 1; index >= 0; index-- {
		_ = os.Remove(r.dirs[index])
	}
}

func testCasePath(feature, id string) string {
	return applayout.FeatureRel(feature) + "/" + RootDir + "/test-cases/" + id + ".test"
}
func revisionPath(feature, testCaseID, id string) string {
	return testCasePath(feature, testCaseID) + "/revisions/" + id + ".json"
}
func eventPath(feature, testCaseID, id string) string {
	return testCasePath(feature, testCaseID) + "/events/" + id + ".json"
}
func evidencePath(feature, testCaseID, id string) string {
	return testCasePath(feature, testCaseID) + "/evidence/" + id + ".json"
}
func runPath(feature, testCaseID, id string) string {
	return testCasePath(feature, testCaseID) + "/runs/" + id + ".json"
}
func policyPath(feature, id string) string {
	return applayout.FeatureRel(feature) + "/" + RootDir + "/policies/" + id + ".json"
}

// feature resolves the feature an authoring operation writes new content into.
func (document *Document) feature(id string) (applayout.Feature, error) {
	if strings.TrimSpace(id) == "" {
		return applayout.Feature{}, fmt.Errorf("a feature is required; new feature content is never written to an implied feature")
	}
	feature, ok := applayout.Find(document.Features, id)
	if !ok {
		return applayout.Feature{}, fmt.Errorf("feature %q does not exist", id)
	}
	return feature, nil
}

func mutationTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC().Truncate(time.Second)
	}
	return value.UTC()
}

func copyStrings(values []string) []string { return append([]string{}, values...) }

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a, b := copyStrings(left), copyStrings(right)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

// sortedKinds canonicalizes a kind set to positive, negative, edge order so
// equal sets are byte-identical regardless of flag order.
func sortedKinds(values []CoverageKind) []CoverageKind {
	order := map[CoverageKind]int{CoveragePositive: 0, CoverageNegative: 1, CoverageEdge: 2}
	result := append([]CoverageKind{}, values...)
	sort.SliceStable(result, func(i, j int) bool { return rank(order, result[i]) < rank(order, result[j]) })
	return result
}

func sortedAutomation(values []Automation) []Automation {
	order := map[Automation]int{AutomationAutomated: 0, AutomationHybrid: 1, AutomationManual: 2}
	result := append([]Automation{}, values...)
	sort.SliceStable(result, func(i, j int) bool { return rank(order, result[i]) < rank(order, result[j]) })
	return result
}

func rank[T comparable](order map[T]int, value T) int {
	if position, ok := order[value]; ok {
		return position
	}
	return len(order)
}
