package cli

import (
	"fmt"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// CompactStatusSchema names the default `status --json` document: what an agent
// decides on, with every list bounded. `status --json --detail` is the complete
// StatusSchema document, unchanged. A field the compact document keeps has the
// same path and meaning as in the complete one, so a rule over the counts reads
// either; a list it drops is absent rather than empty, so a rule that walks one
// fails loudly instead of passing on nothing.
const CompactStatusSchema = "change-saga.status/v4"

// compactListLimit is how many rows of each list `status --json` prints unless
// --max says otherwise.
const compactListLimit = 10

// statusTextLimit is the same default for text status, which is read by people
// and agents alike.
const statusTextLimit = 20

// compactStatus is the default `status --json` contract. Each bounded list has
// a sibling _total, so a reader can always tell a whole list from its head.
type compactStatus struct {
	Schema string `json:"status_schema"`
	// ListLimit is the row limit applied to every bounded list; zero means none.
	ListLimit int `json:"list_limit"`
	// Expand names the commands that return what this document leaves out.
	Expand            compactExpansion              `json:"expand"`
	Opening           opening                       `json:"opening"`
	Complete          bool                          `json:"complete"`
	CoverageScope     string                        `json:"coverage_scope"`
	Summary           coverage.Summary              `json:"summary"`
	Repository        string                        `json:"repository"`
	Base              string                        `json:"base"`
	Head              string                        `json:"head"`
	BaseOID           string                        `json:"base_oid"`
	HeadOID           string                        `json:"head_oid"`
	SagaID            string                        `json:"saga_id"`
	SagaVersion       int                           `json:"saga_version"`
	SchemaValid       bool                          `json:"schema_valid"`
	SchemaIssues      []saga.Issue                  `json:"schema_issues"`
	SchemaIssuesTotal int                           `json:"schema_issues_total"`
	Coverage          compactCoverage               `json:"coverage"`
	Counts            compactCounts                 `json:"counts"`
	Documentation     documentationState            `json:"documentation"`
	Overview          livingapp.OverviewStatus      `json:"overview"`
	ChangedSource     compactChangedSource          `json:"changed_source"`
	Comparison        *compactComparison            `json:"comparison,omitempty"`
	StaleByChange     *compactChangeStaleness       `json:"stale_by_change,omitempty"`
	StaleReferences   []compactStaleReference       `json:"stale_references"`
	StaleRefsTotal    int                           `json:"stale_references_total"`
	Stale             []compactStaleRecord          `json:"stale"`
	StaleTotal        int                           `json:"stale_total"`
	Dangling          []livingapp.DanglingReference `json:"dangling"`
	DanglingTotal     int                           `json:"dangling_total"`
	OverviewRevisions []compactOverviewRevision     `json:"overview_revisions"`
	OverviewRevTotal  int                           `json:"overview_revisions_total"`
	Features          []compactFeature              `json:"features"`
	FeaturesTotal     int                           `json:"features_total"`
	Diagnostics       []livingapp.Diagnostic        `json:"diagnostics"`
	Reviews           []compactReview               `json:"reviews"`
	ReviewsTotal      int                           `json:"reviews_total"`
	ChangeReviews     []string                      `json:"change_reviews"`
	RewrittenReviews  []string                      `json:"rewritten_reviews,omitempty"`
	NextActions       []compactAction               `json:"next_actions"`
	NextActionsTotal  int                           `json:"next_actions_total"`
	AuthoringLoop     nextaction.Loop               `json:"authoring_loop"`
}

// compactExpansion is the way back to everything the compact document omits.
type compactExpansion struct {
	Rows     string `json:"rows"`
	Action   string `json:"action"`
	Area     string `json:"area"`
	Review   string `json:"review"`
	Complete string `json:"complete"`
}

var statusExpansion = compactExpansion{
	Rows:     "status --json --max N for N rows of each list; --max 0 for every row",
	Action:   "status --json --action ID for one next action's practice, options, and commands",
	Area:     "check --covers AREA --max 0 for every gap of one area",
	Review:   "review list --review ID for one review's slides, decisions, and callouts",
	Complete: "status --json --detail for the complete document, which can be megabytes",
}

// compactCoverage mirrors areas.Report without the covered entries, so
// .coverage.areas.<area>.complete, .total, .covered, and .uncovered read the
// same as in the complete document.
type compactCoverage struct {
	Scope areas.Scope  `json:"scope"`
	Areas compactAreas `json:"areas"`
}

type compactAreas struct {
	Review         compactArea `json:"review"`
	Implementation compactArea `json:"implementation"`
	Stories        compactArea `json:"stories"`
	Personas       compactArea `json:"personas"`
	Design         compactArea `json:"design"`
	Quality        compactArea `json:"quality"`
	Health         compactArea `json:"health"`
}

type compactArea struct {
	Area                  areas.Name   `json:"area"`
	Unit                  areas.Unit   `json:"unit"`
	Total                 int          `json:"total"`
	Covered               int          `json:"covered"`
	Uncovered             int          `json:"uncovered"`
	Complete              bool         `json:"complete"`
	Note                  string       `json:"note,omitempty"`
	UncoveredEntries      []compactGap `json:"uncovered_entries"`
	UncoveredEntriesTotal int          `json:"uncovered_entries_total"`
}

// compactGap is one uncovered entry without the lists of what could cover it.
type compactGap struct {
	Resource string `json:"resource"`
	Title    string `json:"title,omitempty"`
	Feature  string `json:"feature,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Side     string `json:"side,omitempty"`
	Lines    string `json:"lines,omitempty"`
	Event    string `json:"event,omitempty"`
	Count    int    `json:"count"`
	Reason   string `json:"reason,omitempty"`
}

// compactCounts sizes the record lists the compact document leaves out.
type compactCounts struct {
	Features       int `json:"features"`
	Personas       int `json:"personas"`
	PersonaGaps    int `json:"persona_gaps"`
	Stories        int `json:"stories"`
	Terms          int `json:"terms"`
	NewTerminology int `json:"new_terminology"`
	Prototypes     int `json:"prototypes"`
	Flags          int `json:"flags"`
	Criteria       int `json:"criteria"`
	TestCases      int `json:"test_cases"`
	AxisGaps       int `json:"axis_gaps"`
	CarriedForward int `json:"carried_forward"`
}

type compactChangedSource struct {
	Complete       bool   `json:"complete"`
	SchemaValid    bool   `json:"schema_valid"`
	Atoms          int    `json:"atoms"`
	UncoveredAtoms int    `json:"uncovered_atoms"`
	Stale          int    `json:"stale"`
	TestOwned      int    `json:"test_owned"`
	Implicated     int    `json:"implicated"`
	Note           string `json:"note"`
}

// compactComparison keeps the layer counts; query layers reads the layers.
type compactComparison struct {
	Summary     changeview.Summary      `json:"summary"`
	Diagnostics []changeview.Diagnostic `json:"diagnostics"`
}

type compactChangeStaleness struct {
	BaseOID         string            `json:"base_oid"`
	HeadOID         string            `json:"head_oid"`
	Count           int               `json:"count"`
	Proposed        int               `json:"proposed"`
	DeletedSide     int               `json:"deleted_side"`
	PreExisting     *int              `json:"pre_existing,omitempty"`
	References      []compactStaleRow `json:"references"`
	ReferencesTotal int               `json:"references_total"`
}

// compactStaleRow is one reference a change made stale: where it was pinned
// and the command that accepts its proposal, without the diff. reconcile and
// references --stale show the diff to read before accepting.
type compactStaleRow struct {
	Pinned coderef.Location `json:"pinned"`
	Reason string           `json:"reason"`
	Accept []string         `json:"accept,omitempty"`
}

type compactStaleReference struct {
	Assignment coverage.Assignment `json:"assignment"`
	Location   coderef.Location    `json:"location"`
	Reason     string              `json:"reason"`
}

type compactStaleRecord struct {
	Record  string   `json:"record"`
	Kind    string   `json:"kind"`
	History string   `json:"history"`
	Reasons []string `json:"reasons"`
	Affects int      `json:"affects"`
}

type compactOverviewRevision struct {
	Feature string `json:"feature"`
	Fix     string `json:"fix"`
}

type compactFeature struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Stories   int    `json:"stories"`
	TestCases int    `json:"test_cases"`
	Decks     int    `json:"decks"`
}

// compactReview names one review and its coverage counts. Its slides, with
// each reviewer's decision, are in the complete document and review list.
type compactReview struct {
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Target          string                 `json:"target"`
	PullRequest     *saga.PullRequest      `json:"pull_request,omitempty"`
	Base            string                 `json:"base"`
	Head            string                 `json:"head,omitempty"`
	Merged          *saga.ReviewMerge      `json:"merged,omitempty"`
	State           string                 `json:"state"`
	StateSource     string                 `json:"state_source"`
	SlidesTotal     int                    `json:"slides_total"`
	OpenLineThreads int                    `json:"open_line_threads"`
	Coverage        *compactReviewCoverage `json:"coverage,omitempty"`
	Diagnostics     []string               `json:"diagnostics"`
}

// compactReviewCoverage counts a review's coverage and names the first files it
// leaves uncovered; review list --uncovered gives their line ranges.
type compactReviewCoverage struct {
	Summary             coverage.Summary       `json:"summary"`
	UncoveredFiles      []compactUncoveredFile `json:"uncovered_files"`
	UncoveredFilesTotal int                    `json:"uncovered_files_total"`
}

type compactUncoveredFile struct {
	Path  string `json:"path"`
	Atoms int    `json:"atoms"`
}

// compactAction is one next action without the practice it teaches or each
// answer's commands; status --json --action ID returns the action whole.
type compactAction struct {
	ID       string              `json:"id"`
	Kind     nextaction.Kind     `json:"kind"`
	Category nextaction.Category `json:"category"`
	Area     string              `json:"area,omitempty"`
	Resource string              `json:"resource,omitempty"`
	Feature  string              `json:"feature,omitempty"`
	Axis     string              `json:"axis,omitempty"`
	Reason   string              `json:"reason"`
	Command  *compactInvocation  `json:"command,omitempty"`
	Question *compactQuestion    `json:"question,omitempty"`
}

type compactInvocation struct {
	Command string             `json:"command"`
	Inputs  []grammar.Argument `json:"inputs"`
	Argv    []string           `json:"argv"`
}

type compactQuestion struct {
	Text    string          `json:"text"`
	Needs   nextaction.Need `json:"needs"`
	Answers []string        `json:"answers"`
}

// firstRows returns at most limit rows, never nil; a limit of zero means all.
func firstRows[T any](rows []T, limit int) []T {
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	if rows == nil {
		return []T{}
	}
	return rows
}

// mapRows bounds rows and converts each one.
func mapRows[T, R any](rows []T, limit int, convert func(T) R) []R {
	head := firstRows(rows, limit)
	result := make([]R, 0, len(head))
	for _, row := range head {
		result = append(result, convert(row))
	}
	return result
}

// compactStatusDocument reduces the complete status document to what an agent
// decides on. It reads the document and never changes it, so status's exit
// status and check are untouched.
func compactStatusDocument(status statusDocument, limit int) compactStatus {
	personaGaps := 0
	for _, persona := range status.Personas {
		if persona.Gap {
			personaGaps++
		}
	}
	document := compactStatus{
		Schema: CompactStatusSchema, ListLimit: limit, Expand: statusExpansion,
		Opening: status.Opening, Complete: status.Report.Complete, CoverageScope: status.CoverageScope,
		Summary: status.Summary, Repository: status.Repository, Base: status.Base, Head: status.Head,
		BaseOID: status.BaseOID, HeadOID: status.HeadOID, SagaID: status.SagaID, SagaVersion: status.SagaVersion,
		SchemaValid: status.SchemaValid, SchemaIssues: boundedSchemaIssues(status.SchemaIssues, limit), SchemaIssuesTotal: len(status.SchemaIssues),
		Coverage: compactCoverage{Scope: status.Coverage.Scope, Areas: compactAreas{
			Review:         compactAreaOf(status.Coverage.Areas.Review, limit),
			Implementation: compactAreaOf(status.Coverage.Areas.Implementation, limit),
			Stories:        compactAreaOf(status.Coverage.Areas.Stories, limit),
			Personas:       compactAreaOf(status.Coverage.Areas.Personas, limit),
			Design:         compactAreaOf(status.Coverage.Areas.Design, limit),
			Quality:        compactAreaOf(status.Coverage.Areas.Quality, limit),
			Health:         compactAreaOf(status.Coverage.Areas.Health, limit),
		}},
		Counts: compactCounts{
			Features: len(status.Features), Personas: len(status.Personas), PersonaGaps: personaGaps,
			Stories: len(status.Stories), Terms: len(status.Terms), NewTerminology: len(status.NewTerminology),
			Prototypes: len(status.Prototypes), Flags: len(status.Flags), Criteria: len(status.Quality.Criteria),
			TestCases: len(status.Quality.TestCases), AxisGaps: len(status.Axes.Gaps), CarriedForward: len(status.CarriedForward),
		},
		Documentation: status.Documentation, Overview: status.Overview,
		ChangedSource: compactChangedSource{
			Complete: status.ChangedSource.Complete, SchemaValid: status.ChangedSource.SchemaValid,
			Atoms: status.ChangedSource.Atoms, UncoveredAtoms: status.ChangedSource.UncoveredAtoms,
			Stale: len(status.ChangedSource.Stale), TestOwned: len(status.ChangedSource.TestOwned),
			Implicated: len(status.ChangedSource.Implicated), Note: status.ChangedSource.Note,
		},
		StaleReferences: mapRows(status.StaleReferences, limit, func(stale coverage.StaleReference) compactStaleReference {
			return compactStaleReference{Assignment: stale.Assignment, Location: stale.Reference.Location(), Reason: stale.Reason}
		}),
		StaleRefsTotal: len(status.StaleReferences),
		Stale: mapRows(status.Status.Stale, limit, func(stale livingapp.StaleRecord) compactStaleRecord {
			return compactStaleRecord{Record: stale.Record, Kind: stale.Kind, History: stale.History, Reasons: stale.Reasons, Affects: len(stale.Affects)}
		}),
		StaleTotal: len(status.Status.Stale), Dangling: firstRows(status.Dangling, limit), DanglingTotal: len(status.Dangling),
		OverviewRevisions: mapRows(status.OverviewRevisions, limit, func(revision livingapp.OverviewRevisionStatus) compactOverviewRevision {
			return compactOverviewRevision{Feature: revision.Feature, Fix: revision.Fix}
		}),
		OverviewRevTotal: len(status.OverviewRevisions),
		Features: mapRows(status.Features, limit, func(feature livingapp.FeatureStatus) compactFeature {
			return compactFeature{ID: feature.ID, Title: feature.Title, Stories: len(feature.Stories), TestCases: len(feature.TestCases), Decks: len(feature.Decks)}
		}),
		FeaturesTotal: len(status.Features), Diagnostics: firstRows(status.Diagnostics, 0),
		Reviews:      mapRows(status.Reviews, limit, func(review reviewstate.Report) compactReview { return compactReviewOf(review, limit) }),
		ReviewsTotal: len(status.Reviews), ChangeReviews: status.ChangeReviews, RewrittenReviews: status.RewrittenReviews,
		NextActions: mapRows(status.NextActions, limit, compactActionOf), NextActionsTotal: len(status.NextActions),
		AuthoringLoop: status.AuthoringLoop,
	}
	if status.Comparison != nil {
		document.Comparison = &compactComparison{Summary: status.Comparison.Summary, Diagnostics: firstRows(status.Comparison.Diagnostics, limit)}
	}
	if stale := status.StaleByChange; stale != nil {
		document.StaleByChange = &compactChangeStaleness{
			BaseOID: stale.BaseOID, HeadOID: stale.HeadOID, Count: stale.Count, Proposed: stale.Proposed,
			DeletedSide: stale.DeletedSide, PreExisting: stale.PreExisting, ReferencesTotal: len(stale.References),
			References: mapRows(stale.References, limit, func(row staleRow) compactStaleRow {
				compact := compactStaleRow{Pinned: row.Pinned, Reason: row.Reason}
				if row.Accept != nil {
					compact.Accept = row.Accept.Argv
				}
				return compact
			}),
		}
	}
	return document
}

// boundedSchemaIssues lists every error, since an error is what makes a report
// untrustworthy, then warnings up to the limit; a limit of zero means all.
func boundedSchemaIssues(issues []saga.Issue, limit int) []saga.Issue {
	if issues == nil {
		return []saga.Issue{}
	}
	if limit <= 0 || len(issues) <= limit {
		return issues
	}
	shown := make([]saga.Issue, 0, limit)
	for _, issue := range issues {
		if issue.Severity == "error" {
			shown = append(shown, issue)
		}
	}
	for _, issue := range issues {
		if issue.Severity != "error" && len(shown) < limit {
			shown = append(shown, issue)
		}
	}
	return shown
}

func compactAreaOf(area areas.Area, limit int) compactArea {
	return compactArea{
		Area: area.Area, Unit: area.Unit, Total: area.Total, Covered: area.Covered, Uncovered: area.Uncovered,
		Complete: area.Complete, Note: area.Note, UncoveredEntriesTotal: len(area.UncoveredEntries),
		UncoveredEntries: mapRows(area.UncoveredEntries, limit, func(entry areas.Entry) compactGap {
			return compactGap{
				Resource: entry.Resource, Title: entry.Title, Feature: entry.Feature, Kind: entry.Kind,
				Side: entry.Side, Lines: entry.Lines, Event: entry.Event, Count: entry.Count, Reason: entry.Reason,
			}
		}),
	}
}

func compactReviewOf(review reviewstate.Report, limit int) compactReview {
	compact := compactReview{
		ID: review.ID, Title: review.Title, Target: review.Target, PullRequest: review.PullRequest,
		Base: review.Base, Head: review.Head, Merged: review.Merged, State: review.State, StateSource: review.StateSource,
		SlidesTotal: len(review.Slides), OpenLineThreads: review.OpenLineThreads, Diagnostics: firstRows(review.Diagnostics, limit),
	}
	if review.Coverage != nil {
		compact.Coverage = &compactReviewCoverage{
			Summary: review.Coverage.Summary,
			UncoveredFiles: mapRows(review.Coverage.UncoveredFiles, limit, func(file reviewstate.UncoveredFile) compactUncoveredFile {
				return compactUncoveredFile{Path: file.Path, Atoms: file.Atoms}
			}),
			UncoveredFilesTotal: len(review.Coverage.UncoveredFiles),
		}
	}
	return compact
}

func compactActionOf(action nextaction.Action) compactAction {
	compact := compactAction{
		ID: action.ID, Kind: action.Kind, Category: action.Category, Area: action.Area, Resource: action.Resource,
		Feature: action.Feature, Axis: action.Axis, Reason: action.Reason,
	}
	if action.Command != nil {
		compact.Command = &compactInvocation{Command: action.Command.Command, Inputs: action.Command.Inputs, Argv: action.Command.Argv}
	}
	if action.Question != nil {
		question := &compactQuestion{Text: action.Question.Text, Needs: action.Question.Needs, Answers: []string{}}
		for _, option := range action.Question.Options {
			question.Answers = append(question.Answers, option.Answer)
		}
		compact.Question = question
	}
	return compact
}

// findNextAction returns one next action whole, for status --json --action.
func findNextAction(actions []nextaction.Action, id string) (nextaction.Action, error) {
	for _, action := range actions {
		if action.ID == id {
			return action, nil
		}
	}
	return nextaction.Action{}, fmt.Errorf("status has no next action %q; status --json lists them", id)
}
