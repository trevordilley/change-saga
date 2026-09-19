package livingapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

const (
	coverageExceptionsDir      = "coverage-exceptions"
	coverageExceptionSchemaURL = "https://changesaga.dev/schema/v5/coverage-exception.schema.json"
	maxCoverageExceptions      = 20_000
	maxCoverageExceptionBytes  = 1 << 20
)

// StatusOptions names the Saga and the source comparison status already read.
// Document is optional; when supplied it must be the same snapshot the
// comparison was evaluated against.
type StatusOptions struct {
	SagaRoot string
	Document *saga.Saga
	Report   coverage.Report
	Changes  gitdiff.ChangeSet
	// Resolver views quality-evidence and term code references in Changes.
	// Without it every such reference is reported stale. When it can also read
	// file content (Blobs), a comparison suggests new terminology.
	Resolver coverage.Resolver
}

// LoadStatus reads every living record strictly and read-only, then assembles
// the status projection. It never writes, executes a test, or fetches a URL.
func LoadStatus(ctx context.Context, options StatusOptions) (Status, error) {
	inputs, err := LoadStatusInputs(ctx, options)
	if err != nil {
		return Status{}, err
	}
	return Assemble(inputs), nil
}

// LoadStatusInputs loads the records Assemble reads.
func LoadStatusInputs(ctx context.Context, options StatusOptions) (StatusInputs, error) {
	root, err := filepath.Abs(options.SagaRoot)
	if err != nil {
		return StatusInputs{}, err
	}
	doc := options.Document
	if doc == nil {
		doc, _, err = saga.Load(root)
		if err != nil {
			return StatusInputs{}, err
		}
	}
	inputs := StatusInputs{
		SagaID: doc.Manifest.ID, SagaVersion: doc.Manifest.Version, Decks: doc.Decks,
		Stories: []requirements.Story{}, Citations: []requirements.Citation{}, Links: []Link{},
		Prototypes: prototypes.Document{SagaID: doc.Manifest.ID, Prototypes: []prototypes.Prototype{}, Annotations: []prototypes.Annotation{}},
		Exceptions: []coverage.Exception{}, Report: options.Report, Changes: options.Changes, Diagnostics: []Diagnostic{},
		Quality: quality.Document{SagaID: doc.Manifest.ID, TestCases: []quality.TestCase{}, Policies: []quality.Policy{}, PolicySets: []quality.PolicySet{}},
	}
	inputs.Epics, err = applayout.Epics(root)
	if err != nil {
		return StatusInputs{}, err
	}
	inputs.Quality, err = quality.Load(root)
	if err != nil {
		return StatusInputs{}, fmt.Errorf("load quality: %w", err)
	}
	inputs.QualityCode = map[string]coverage.ResolvedCode{}
	for _, testCase := range inputs.Quality.TestCases {
		for _, evidence := range testCase.Evidence {
			for _, reference := range evidence.Code {
				if options.Resolver == nil {
					inputs.QualityCode[reference.Key()] = coverage.ResolvedCode{Reason: "no source repository is available to resolve the reference"}
					continue
				}
				inputs.QualityCode[reference.Key()] = coverage.Resolve(ctx, reference, options.Changes, options.Resolver)
			}
		}
	}
	inputs.Exceptions, err = LoadCoverageExceptions(root, doc.Manifest.ID)
	if err != nil {
		return StatusInputs{}, err
	}
	if requirementsAdopted(root) {
		heads := map[string][]string{}
		for _, testCase := range inputs.Quality.TestCases {
			heads[testCase.Identity.ID] = copyStrings(testCase.RevisionHeads)
		}
		graph, err := loadLivingGraph(root, doc, heads)
		if err != nil {
			return StatusInputs{}, err
		}
		inputs.Stories = graph.requirements.Stories
		inputs.Citations = graph.requirements.Citations
		inputs.Personas = graph.requirements.Personas
		inputs.Flags = graph.requirements.Flags
		inputs.Terms = graph.requirements.Terms
		inputs.Gates = graph.requirements.Gates()
		inputs.DesignDigests = graph.designDigests
		inputs.Links = LinksFromCurrency(graph.requirements, graph.currency)
		inputs.Prototypes, err = prototypes.Load(root, doc.Manifest.ID)
		if err != nil {
			return StatusInputs{}, fmt.Errorf("load prototypes: %w", err)
		}
	}
	inputs.TermCode = resolveTermCode(ctx, inputs.Terms, options.Changes, options.Resolver)
	blobs, _ := options.Resolver.(Blobs)
	inputs.TermSuggestions = SuggestTerms(ctx, inputs.Terms, options.Changes, options.Resolver, blobs)
	inputs.Overview = overviewStatus(doc, inputs.Terms)
	return inputs, nil
}

// LinksFromCurrency adapts loaded relations and their currency, which must be
// requirements.EvaluateRelations(document, ...) in document order. Scope is
// read from the record; a v3 record carries none, so it is self except for a
// legacy explains relation from a deck or slide, which keeps its established
// descendant trace behavior.
func LinksFromCurrency(document requirements.Document, currency []requirements.RelationCurrency) []Link {
	links := make([]Link, 0, len(document.Relations))
	for index, relation := range document.Relations {
		urn, _ := livingid.Relation(document.SagaID, relation.ID)
		scope := string(relation.Scope)
		if scope == "" {
			scope = scopeSelf
			if relation.Type == requirements.RelationExplains && (strings.Contains(relation.From, ":deck:") || !strings.Contains(relation.From, ":item:") && strings.Contains(relation.From, ":slide:")) {
				scope = scopeDescendants
			}
		}
		link := Link{
			URN: urn, Type: relation.Type, From: relation.From, To: relation.To, Scope: scope, Version: relation.Version,
			FromRevision: relation.FromRevision, ToRevision: relation.ToRevision,
			FromContentDigest: relation.FromContentDigest, ToContentDigest: relation.ToContentDigest,
			Active: relation.State == requirements.RelationActive, Currency: requirements.CurrencyCurrent, Reasons: []requirements.CurrencyReason{},
		}
		if index < len(currency) && currency[index].Relation == urn {
			link.Currency = currency[index].Status
			link.Reasons = append(link.Reasons, currency[index].Reasons...)
		}
		links = append(links, link)
	}
	return links
}

type livingGraph struct {
	requirements  requirements.Document
	plan          workplan.Plan
	designDigests map[string]string
	currency      []requirements.RelationCurrency
}

// testCaseHeads returns every test case's revision heads, keyed by test-case
// ID.
func testCaseHeads(root string) (map[string][]string, error) {
	document, err := quality.Load(root)
	if err != nil {
		return nil, err
	}
	return revisionHeads(document), nil
}

func revisionHeads(document quality.Document) map[string][]string {
	heads := map[string][]string{}
	for _, testCase := range document.TestCases {
		heads[testCase.Identity.ID] = copyStrings(testCase.RevisionHeads)
	}
	return heads
}

// loadLivingGraph is the one composition path for requirements, work plan,
// design digests, and relation currency, shared by query sessions and status.
// Relation currency comes only from requirements.EvaluateRelations; this
// function supplies the heads other domains own, including test-case heads,
// so a test-case link is never judged without them.
func loadLivingGraph(root string, doc *saga.Saga, testCases map[string][]string) (livingGraph, error) {
	plan, validation, err := workplan.Load(root)
	if err != nil {
		return livingGraph{}, appError(CodeInvalidSaga, "the work plan could not be loaded", false, nil, err)
	}
	if !validation.Valid {
		return livingGraph{}, appError(CodeInvalidSaga, "the work plan is invalid", false, map[string]any{"issues": sanitizePlanIssues(validation.Issues)}, nil)
	}
	designDigests, err := saga.CurrentDesignContentDigests(doc)
	if err != nil {
		return livingGraph{}, appError(CodeInvalidSaga, "the technical design could not be indexed", false, nil, err)
	}
	stale := requirements.StaleInputs{CurrentRevisions: map[string]string{}, CurrentContentDigests: designDigests, Missing: map[string]bool{}, ConflictedRevisions: map[string][]string{}}
	for _, id := range sortedKeys(plan.WorkItems) {
		item := plan.WorkItems[id]
		if item.CurrentRevision != nil && len(item.Heads) == 1 {
			stale.CurrentRevisions[item.CurrentRevision.WorkItem] = item.Heads[0]
		}
	}
	if testCases != nil {
		stale.SetTestCaseHeads(doc.Manifest.ID, testCases)
	}
	document, err := requirements.LoadWithOptions(root, plan.SagaID, requirements.LoadOptions{StaleInputs: stale})
	if err != nil {
		return livingGraph{}, appError(CodeInvalidSaga, "the requirements could not be loaded", false, nil, err)
	}
	crossDomainInputs(document, plan, doc, designDigests, &stale)
	currency := requirements.EvaluateRelations(document, stale)
	for index := range document.Relations {
		relation := &document.Relations[index]
		reasons := []string{}
		for _, reason := range currency[index].Reasons {
			reasons = append(reasons, reason.Message)
		}
		relation.StaleReasons = reasons
		relation.Stale = len(reasons) > 0
	}
	return livingGraph{requirements: document, plan: plan, designDigests: designDigests, currency: currency}, nil
}

type coverageExceptionRecord struct {
	Schema     string    `json:"$schema"`
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	Axis       string    `json:"axis"`
	Criterion  string    `json:"criterion"`
	Revision   string    `json:"story_revision"`
	Rationale  string    `json:"rationale"`
	Citations  []string  `json:"citations"`
	Supersedes []string  `json:"supersedes"`
	CreatedAt  time.Time `json:"created_at"`
	RequestID  string    `json:"request_id,omitempty"`
}

// LoadCoverageExceptions strictly reads every epic's
// ___requirements/coverage-exceptions. Exception IDs are unique across the app.
// Structural errors fail the load. An unknown axis, blank rationale, missing or
// unresolved citation, stale pin, supersession, or competing head is projected
// by coverage.ProjectAxes as a visible cell state instead, so the author sees
// why a recorded decision did not take.
func LoadCoverageExceptions(root, sagaID string) ([]coverage.Exception, error) {
	epics, err := applayout.Epics(root)
	if err != nil {
		return nil, err
	}
	ids := applayout.NewUniqueIDs("coverage exception")
	result := []coverage.Exception{}
	for _, epic := range epics {
		loaded, err := loadEpicCoverageExceptions(filepath.Join(epic.Dir, applayout.RequirementsDir, coverageExceptionsDir), sagaID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", epic.Rel, err)
		}
		for _, exception := range loaded {
			if err := ids.Claim(exception.URN, epic.ID); err != nil {
				return nil, err
			}
		}
		result = append(result, loaded...)
		if len(result) > maxCoverageExceptions {
			return nil, fmt.Errorf("coverage exceptions exceed %d records", maxCoverageExceptions)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URN < result[j].URN })
	return result, nil
}

func loadEpicCoverageExceptions(dir, sagaID string) ([]coverage.Exception, error) {
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []coverage.Exception{}, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("coverage exceptions must be a real directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxCoverageExceptions {
		return nil, fmt.Errorf("coverage exceptions exceed %d records", maxCoverageExceptions)
	}
	result := []coverage.Exception{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("coverage exception %q must be a regular .json file", entry.Name())
		}
		record, err := readCoverageException(path)
		if err != nil {
			return nil, fmt.Errorf("coverage exception %s: %w", entry.Name(), err)
		}
		if err := validateCoverageException(record, sagaID, strings.TrimSuffix(entry.Name(), ".json")); err != nil {
			return nil, fmt.Errorf("coverage exception %s: %w", entry.Name(), err)
		}
		urn, _ := qualityid.CoverageException(sagaID, record.ID)
		result = append(result, coverage.Exception{
			URN: urn, Axis: coverage.Axis(record.Axis), Criterion: record.Criterion, StoryRevision: record.Revision,
			Rationale: record.Rationale, Citations: copyStrings(record.Citations), Supersedes: copyStrings(record.Supersedes), CreatedAt: record.CreatedAt,
		})
	}
	return result, nil
}

// requirementsAdopted reports whether the app records any requirements: a
// persona, or a ___requirements root in any epic.
func requirementsAdopted(root string) bool {
	if livingRootPresent(root, applayout.PersonasDir) || livingRootPresent(root, filepath.FromSlash(applayout.TermsDir)) {
		return true
	}
	epics, err := applayout.Epics(root)
	if err != nil {
		// A broken epic list must surface through the loaders, not be
		// mistaken for an app without requirements.
		return true
	}
	for _, epic := range epics {
		if livingRootPresent(epic.Dir, applayout.RequirementsDir) {
			return true
		}
	}
	return false
}

func readCoverageException(path string) (coverageExceptionRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return coverageExceptionRecord{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCoverageExceptionBytes+1))
	if err != nil {
		return coverageExceptionRecord{}, err
	}
	if len(data) > maxCoverageExceptionBytes {
		return coverageExceptionRecord{}, fmt.Errorf("record exceeds %d bytes", maxCoverageExceptionBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record coverageExceptionRecord
	if err := decoder.Decode(&record); err != nil {
		return coverageExceptionRecord{}, err
	}
	if decoder.More() {
		return coverageExceptionRecord{}, fmt.Errorf("trailing data after the record")
	}
	return record, nil
}

func validateCoverageException(record coverageExceptionRecord, sagaID, expectedID string) error {
	problems := []string{}
	if record.Schema != coverageExceptionSchemaURL {
		problems = append(problems, "$schema must be "+coverageExceptionSchemaURL)
	}
	if record.Version != quality.Version {
		problems = append(problems, "version must be 5")
	}
	if record.ID != expectedID || !livingid.ValidID(record.ID) {
		problems = append(problems, "id must be stable and match its filename")
	}
	if ref, err := livingid.Parse(record.Criterion); err != nil || ref.Kind != livingid.KindCriterion || ref.SagaID != sagaID {
		problems = append(problems, "criterion must be a criterion URN in this Saga")
	}
	if ref, err := livingid.Parse(record.Revision); err != nil || ref.Kind != livingid.KindRevision || ref.SagaID != sagaID {
		problems = append(problems, "story_revision must be a story revision URN in this Saga")
	}
	if record.Supersedes == nil {
		problems = append(problems, "supersedes is required")
	}
	if record.CreatedAt.IsZero() {
		problems = append(problems, "created_at is required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
