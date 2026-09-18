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
	coverageExceptionsDir       = "coverage-exceptions"
	coverageExceptionSchemaURL  = "https://changesaga.dev/schema/v5/coverage-exception.schema.json"
	maxCoverageExceptions       = 20_000
	maxCoverageExceptionBytes   = 1 << 20
	qualityRequiresV5ReasonText = "quality records are v5; this is a v%d Saga (adopt with `change-saga upgrade --to 5`)"
)

// StatusOptions names the Saga and the source comparison status already read.
// Document is optional; when supplied it must be the same snapshot the
// comparison was evaluated against.
type StatusOptions struct {
	SagaRoot string
	Document *saga.Saga
	Policy   string
	Report   coverage.Report
	Changes  gitdiff.ChangeSet
}

// LoadStatus reads every living record strictly and read-only, then assembles
// the status projection. It never writes, executes a test, or fetches a URL.
func LoadStatus(_ context.Context, options StatusOptions) (Status, error) {
	inputs, err := LoadStatusInputs(options)
	if err != nil {
		return Status{}, err
	}
	return Assemble(inputs), nil
}

// LoadStatusInputs loads the records Assemble reads.
func LoadStatusInputs(options StatusOptions) (StatusInputs, error) {
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
	version := doc.Manifest.Version
	inputs := StatusInputs{
		SagaID: doc.Manifest.ID, SagaVersion: version, Policy: options.Policy, Decks: doc.Decks,
		Stories: []requirements.Story{}, Citations: []requirements.Citation{}, Links: []Link{},
		Prototypes: prototypes.Document{SagaID: doc.Manifest.ID, Prototypes: []prototypes.Prototype{}, Annotations: []prototypes.Annotation{}},
		Exceptions: []coverage.Exception{}, Report: options.Report, Changes: options.Changes, Diagnostics: []Diagnostic{},
		Quality: quality.Document{SagaID: doc.Manifest.ID, Adoption: quality.NotAdopted, TestCases: []quality.TestCase{}, Policies: []quality.Policy{}, PolicySets: []quality.PolicySet{}},
	}
	living := version == saga.CurrentSagaVersion || version == quality.Version
	if living && livingRootPresent(root, "___requirements") {
		graph, err := loadLivingGraph(root, doc)
		if err != nil {
			return StatusInputs{}, err
		}
		inputs.RequirementsAdopted = true
		inputs.Stories = graph.requirements.Stories
		inputs.Citations = graph.requirements.Citations
		inputs.DesignDigests = graph.designDigests
		for _, relation := range graph.requirements.Relations {
			inputs.Links = append(inputs.Links, LinkFromRelation(graph.requirements.SagaID, relation))
		}
		inputs.Prototypes, err = prototypes.Load(root, doc.Manifest.ID)
		if err != nil {
			return StatusInputs{}, fmt.Errorf("load prototypes: %w", err)
		}
		criteria, _ := (&session{requirements: graph.requirements, plan: graph.plan, saga: doc, adopted: true}).criterionInputs(Filters{})
		inputs.PeerReview = criteria
	}
	if version == quality.Version {
		inputs.Quality, err = quality.Load(root)
		if err != nil {
			return StatusInputs{}, fmt.Errorf("load quality: %w", err)
		}
		inputs.Exceptions, inputs.ExceptionsAdopted, err = LoadCoverageExceptions(root, doc.Manifest.ID)
		if err != nil {
			return StatusInputs{}, err
		}
	} else {
		inputs.QualityReason = fmt.Sprintf(qualityRequiresV5ReasonText, version)
	}
	return inputs, nil
}

// LinkFromRelation adapts a loaded relation record. A v3 record carries no
// scope: every relation is self-scoped except a legacy explains relation from a
// deck or slide, which keeps its established descendant trace behavior.
func LinkFromRelation(sagaID string, relation requirements.Relation) Link {
	urn, _ := livingid.Relation(sagaID, relation.ID)
	scope := scopeSelf
	if relation.Type == requirements.RelationExplains && (strings.Contains(relation.From, ":deck:") || !strings.Contains(relation.From, ":item:") && strings.Contains(relation.From, ":slide:")) {
		scope = scopeDescendants
	}
	return Link{
		URN: urn, Type: relation.Type, From: relation.From, To: relation.To, Scope: scope,
		FromRevision: relation.FromRevision, ToRevision: relation.ToRevision,
		FromContentDigest: relation.FromContentDigest, ToContentDigest: relation.ToContentDigest,
		Active: relation.State == requirements.RelationActive, StaleReasons: copyStrings(relation.StaleReasons),
	}
}

type livingGraph struct {
	requirements  requirements.Document
	plan          workplan.Plan
	designDigests map[string]string
}

// loadLivingGraph is the one composition path for requirements, work plan,
// and design digests, shared by query sessions and status.
func loadLivingGraph(root string, doc *saga.Saga) (livingGraph, error) {
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
	stale := requirements.StaleInputs{CurrentRevisions: map[string]string{}, CurrentContentDigests: designDigests, Missing: map[string]bool{}}
	for _, id := range sortedKeys(plan.WorkItems) {
		item := plan.WorkItems[id]
		if item.CurrentRevision != nil {
			stale.CurrentRevisions[item.CurrentRevision.WorkItem] = item.Heads[0]
		}
	}
	document, err := requirements.LoadWithOptions(root, plan.SagaID, requirements.LoadOptions{StaleInputs: stale})
	if err != nil {
		return livingGraph{}, appError(CodeInvalidSaga, "the requirements could not be loaded", false, nil, err)
	}
	evaluateCrossDomainStaleness(&document, plan, doc, designDigests)
	return livingGraph{requirements: document, plan: plan, designDigests: designDigests}, nil
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

// LoadCoverageExceptions strictly reads ___requirements/coverage-exceptions.
// Structural errors fail the load. An unknown axis, blank rationale, missing or
// unresolved citation, stale pin, supersession, or competing head is projected
// by coverage.ProjectAxes as a visible cell state instead, so the author sees
// why a recorded decision did not take.
func LoadCoverageExceptions(root, sagaID string) ([]coverage.Exception, bool, error) {
	dir := filepath.Join(root, "___requirements", coverageExceptionsDir)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []coverage.Exception{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, false, fmt.Errorf("coverage exceptions must be a real directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	if len(entries) > maxCoverageExceptions {
		return nil, false, fmt.Errorf("coverage exceptions exceed %d records", maxCoverageExceptions)
	}
	result := []coverage.Exception{}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, false, fmt.Errorf("coverage exception %q must be a regular .json file", entry.Name())
		}
		record, err := readCoverageException(path)
		if err != nil {
			return nil, false, fmt.Errorf("coverage exception %s: %w", entry.Name(), err)
		}
		if err := validateCoverageException(record, sagaID, strings.TrimSuffix(entry.Name(), ".json")); err != nil {
			return nil, false, fmt.Errorf("coverage exception %s: %w", entry.Name(), err)
		}
		urn, _ := qualityid.CoverageException(sagaID, record.ID)
		result = append(result, coverage.Exception{
			URN: urn, Axis: coverage.Axis(record.Axis), Criterion: record.Criterion, StoryRevision: record.Revision,
			Rationale: record.Rationale, Citations: copyStrings(record.Citations), Supersedes: copyStrings(record.Supersedes), CreatedAt: record.CreatedAt,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URN < result[j].URN })
	return result, true, nil
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
