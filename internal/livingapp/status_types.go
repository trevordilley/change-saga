package livingapp

import (
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Link is one persisted relation as the status assembler reads it. It is
// deliberately wider than requirements.Relation: Scope is a v5 field the v3
// record does not carry, and test-case endpoints are v5-only. Adapters fill it
// from whichever loader produced the record; StaleReasons carries whatever the
// loader already derived from pins and is extended here, never replaced.
type Link struct {
	URN               string
	Type              requirements.RelationType
	From              string
	To                string
	Scope             string
	FromRevision      string
	ToRevision        string
	FromContentDigest string
	ToContentDigest   string
	Active            bool
	Version           int
	// Currency and Reasons come from requirements.EvaluateRelation, the single
	// place relation pins are compared with current heads. Only a current
	// relation ever counts as coverage.
	Currency requirements.Currency
	Reasons  []requirements.CurrencyReason
}

// reasons splits the link's currency into the axis vocabulary: a stale pin, a
// conflicted head, or an invalid endpoint. A current link has none.
func (link Link) reasons() (stale, conflicts, invalid []string) {
	stale, conflicts, invalid = []string{}, []string{}, []string{}
	var target *[]string
	switch link.Currency {
	case requirements.CurrencyStale:
		target = &stale
	case requirements.CurrencyConflicted:
		target = &conflicts
	case requirements.CurrencyInvalid:
		target = &invalid
	default:
		return
	}
	for _, reason := range link.Reasons {
		*target = append(*target, reason.Message)
	}
	return uniqueSorted(stale), uniqueSorted(conflicts), uniqueSorted(invalid)
}

// pins reports the pinned and current value behind each currency reason.
func (link Link) pins() []Pin {
	pins := []Pin{}
	for _, reason := range link.Reasons {
		if reason.Pinned == "" {
			continue
		}
		field := reason.Endpoint + "_revision"
		if reason.Code == requirements.ReasonContentDigestChanged {
			field = reason.Endpoint + "_content_digest"
		}
		pins = append(pins, Pin{Field: field, Pinned: reason.Pinned, Current: strings.Join(reason.Current, ", ")})
	}
	return pins
}

// StatusInputs is every already-loaded fact the status projection reads. The
// assembler performs no I/O, so a test can hand it records directly.
type StatusInputs struct {
	SagaID      string
	SagaVersion int

	Stories   []requirements.Story
	Citations []requirements.Citation
	Links     []Link

	// Epics, Personas, Flags, and Gates are the app-level records: the
	// durable product domains, who the app serves, and which stories current
	// flags gate.
	Epics    []applayout.Epic
	Personas []requirements.Persona
	Flags    []requirements.Flag
	Gates    requirements.Gate

	// Overview is the overview's parts; Terms are the project's vocabulary,
	// TermCode each current term reference viewed at the head (keyed by
	// coderef.Reference.Key), and TermSuggestions the declarations a
	// comparison added that look like new vocabulary.
	Overview        OverviewStatus
	Terms           []requirements.Term
	TermCode        map[string]coderesolve.Resolution
	TermSuggestions []TermSuggestion

	Prototypes prototypes.Document
	Decks      []*saga.Deck
	// DesignDigests is the current canonical content digest of every design and
	// visual target, used to report the current side of a stale digest pin.
	DesignDigests map[string]string

	Exceptions []coverage.Exception

	Quality quality.Document

	Report  coverage.Report
	Changes gitdiff.ChangeSet
	// QualityCode is every quality-evidence code reference viewed in Changes,
	// keyed by coderef.Reference.Key.
	QualityCode map[string]coverage.ResolvedCode

	Diagnostics []Diagnostic
}

// Diagnostic is a load or composition fact that is not a gate result.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Status is the machine-readable authoring status of one Saga snapshot. Every
// section is a list of facts; nothing is reduced to a score or a percentage.
type Status struct {
	SagaID         string           `json:"saga_id"`
	SagaVersion    int              `json:"saga_version"`
	Epics          []EpicStatus     `json:"epics"`
	Personas       []PersonaStatus  `json:"personas"`
	PersonaOrphans []PersonaOrphans `json:"persona_orphans"`
	// PersonaCoverage reports the persona -> story link. It never blocks a
	// readiness gate.
	PersonaCoverage PersonaCoverage `json:"persona_coverage"`
	// Chain is the persona -> story -> design -> code chain the coverage
	// report walks; the report, not the chain, is the JSON contract.
	Chain Chain        `json:"-"`
	Flags []FlagStatus `json:"flags"`
	// Overview reports the overview's parts and which are gaps; Terms the
	// project's vocabulary and its code health; NewTerminology the growth
	// suggestions of a comparison. None of them is part of a readiness gate.
	Overview       OverviewStatus          `json:"overview"`
	Terms          []TermStatus            `json:"terms"`
	NewTerminology []TermSuggestion        `json:"new_terminology"`
	Stories        []StoryStatus           `json:"stories"`
	Prototypes     []PrototypeStatus       `json:"prototypes"`
	Axes           coverage.AxisProjection `json:"axes"`
	Quality        QualityStatus           `json:"quality"`
	Stale          []StaleRecord           `json:"stale"`
	ChangedSource  ChangedSource           `json:"changed_source"`
	Diagnostics    []Diagnostic            `json:"diagnostics"`
}

// StoryStatus is the requirement identity an author needs to act on a story.
type StoryStatus struct {
	Story string `json:"story"`
	ID    string `json:"id"`
	Epic  string `json:"epic"`
	Title string `json:"title,omitempty"`
	// Statement, Priority, and Citations complete the current revision, so a
	// suggested revision can carry every field forward.
	Statement       string            `json:"statement,omitempty"`
	Priority        string            `json:"priority,omitempty"`
	Citations       []string          `json:"citations,omitempty"`
	State           string            `json:"state"`
	Personas        []string          `json:"personas"`
	GatedBy         []string          `json:"gated_by"`
	Availability    string            `json:"availability"`
	RevisionHeads   []string          `json:"revision_heads"`
	LifecycleHeads  []string          `json:"lifecycle_heads"`
	CurrentRevision string            `json:"current_revision,omitempty"`
	Criteria        []CriterionStatus `json:"criteria"`
}

// CriterionStatus is one criterion in the current revision.
type CriterionStatus struct {
	Criterion string `json:"criterion"`
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

// PrototypeStatus is one prototype's product-discovery linkage.
type PrototypeStatus struct {
	Prototype       string   `json:"prototype"`
	Epic            string   `json:"epic"`
	State           string   `json:"state"`
	Retained        bool     `json:"retained"`
	RevisionHeads   []string `json:"revision_heads"`
	CurrentRevision string   `json:"current_revision,omitempty"`
	CurrentLinks    []string `json:"current_links"`
	StaleLinks      []string `json:"stale_links"`
}

// QualityStatus is the quality domain as it bears on readiness. A Saga with no
// quality records has no exemption: every accepted criterion's quality axis is
// then a visible gap.
type QualityStatus struct {
	Criteria  []QualityCriterion  `json:"criteria"`
	TestCases []TestCaseStatus    `json:"test_cases"`
	Facts     []QualityFactStatus `json:"facts"`
}

// QualityCriterion is one accepted criterion's quality obligation: which kinds
// its policy requires, which kinds linked tests declare, and the per-kind state.
type QualityCriterion struct {
	Criterion         string        `json:"criterion"`
	StoryRevision     string        `json:"story_revision"`
	Policy            string        `json:"policy"`
	PolicyState       string        `json:"policy_state"`
	RequiredKinds     []string      `json:"required_kinds"`
	AllowedAutomation []string      `json:"allowed_automation"`
	ObservedKinds     []string      `json:"observed_kinds"`
	Tests             []string      `json:"tests"`
	Kinds             []KindStatus  `json:"kinds"`
	Links             []TestLinkRow `json:"links"`
}

// KindStatus is one coverage kind for one criterion. State is one of covered,
// missing_kind, not_run, failed, blocked, skipped, stale, inactive, invalid,
// conflicted, or excluded.
type KindStatus struct {
	Kind      string   `json:"kind"`
	Required  bool     `json:"required"`
	State     string   `json:"state"`
	TestCases []string `json:"test_cases"`
	Reasons   []string `json:"reasons"`
}

// TestLinkRow is one verifies relation from a test case to a criterion and the
// evaluated state of the test behind it.
type TestLinkRow struct {
	Relation  string   `json:"relation"`
	TestCase  string   `json:"test_case"`
	State     string   `json:"state"`
	Kinds     []string `json:"kinds"`
	Run       string   `json:"run,omitempty"`
	RunResult string   `json:"run_result,omitempty"`
	Reasons   []string `json:"reasons"`
}

// TestCaseStatus is one test case and what it currently proves.
type TestCaseStatus struct {
	TestCase        string         `json:"test_case"`
	Epic            string         `json:"epic"`
	Title           string         `json:"title,omitempty"`
	Lifecycle       string         `json:"lifecycle"`
	RevisionHeads   []string       `json:"revision_heads"`
	CurrentRevision string         `json:"current_revision,omitempty"`
	Kinds           []string       `json:"kinds"`
	Automation      string         `json:"automation,omitempty"`
	RunHeads        []string       `json:"run_heads"`
	CurrentRun      string         `json:"current_run,omitempty"`
	RunResult       string         `json:"run_result,omitempty"`
	Verifies        []VerifiesLink `json:"verifies"`
	// Orphaned is true when no active verifies relation connects the test to
	// any criterion, so it proves nothing about any story.
	Orphaned bool `json:"orphaned"`
}

// VerifiesLink is one active verifies relation from a test case with its
// currency. A stale link needs re-pinning; only a test case with no active
// verifies relation at all is an orphan that needs linking.
type VerifiesLink struct {
	Relation  string   `json:"relation"`
	Criterion string   `json:"criterion"`
	Currency  string   `json:"currency"`
	Reasons   []string `json:"reasons"`
}

// QualityFactStatus is the readiness.QualityFact the gate evaluated, exposed so
// the gate verdict can be traced without re-deriving it.
type QualityFactStatus struct {
	Criterion        string   `json:"criterion"`
	Kind             string   `json:"kind"`
	Required         bool     `json:"required"`
	State            string   `json:"state"`
	TestCase         string   `json:"test_case,omitempty"`
	RunResult        string   `json:"run_result,omitempty"`
	RunHeads         []string `json:"run_heads"`
	EvidenceResolved bool     `json:"evidence_resolved"`
	StaleReasons     []string `json:"stale_reasons"`
}

// StaleRecord is one persisted record whose pin no longer matches the head it
// was pinned to. History says which history moved: "review" when a Saga record
// changed (a story, test, prototype, or design revision or digest), "source"
// when the source comparison moved under a diff selector or run identity.
// Staleness is derived only from pins, never from Git history.
type StaleRecord struct {
	Record  string   `json:"record"`
	Kind    string   `json:"kind"`
	History string   `json:"history"`
	Reasons []string `json:"reasons"`
	Pins    []Pin    `json:"pins"`
	Affects []string `json:"affects"`
	// Type, From, To, and Axis identify what the record asserted, so a refresh
	// can restate the same claim against the current heads.
	Type  string `json:"type,omitempty"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Axis  string `json:"axis,omitempty"`
	Scope string `json:"scope,omitempty"`
}

// Pin is one pinned value beside the current value it is compared with.
type Pin struct {
	Field   string `json:"field"`
	Pinned  string `json:"pinned"`
	Current string `json:"current,omitempty"`
}

// ChangedSource is the global omission invariant. It is kept apart from every
// per-criterion axis because transitivity proves stories reached code but can
// never prove that no other code changed: changed code no design mentions sits
// on no story's path. No coverage exception is consulted here.
type ChangedSource struct {
	Complete    bool `json:"complete"`
	SchemaValid bool `json:"schema_valid"`
	Atoms       int  `json:"atoms"`
	// Uncovered groups unowned atoms by path; the exact atoms are the version 1
	// top-level "uncovered" list, minus any listed under TestOwned.
	Uncovered      []UncoveredPath  `json:"uncovered"`
	UncoveredAtoms int              `json:"uncovered_atoms"`
	Stale          []StaleReference `json:"stale"`
	TestOwned      []TestOwned      `json:"test_owned"`
	Implicated     []Implicated     `json:"implicated"`
	Note           string           `json:"note"`

	uncoveredRefs []string
}

// UncoveredPath is one changed path with atoms no target owns.
type UncoveredPath struct {
	Path  string `json:"path"`
	Atoms int    `json:"atoms"`
}

// StaleReference is one code reference that is current at neither side of
// the comparison, with the criteria reached through its owner.
type StaleReference struct {
	Target       string            `json:"target"`
	EvidenceFile string            `json:"evidence_file"`
	Reference    int               `json:"reference"`
	Code         coderef.Reference `json:"code"`
	Reason       string            `json:"reason"`
	Affects      []string          `json:"affects"`
}

// TestOwned is a changed atom accounted for only by current test-code evidence.
type TestOwned struct {
	Atom     string `json:"atom"`
	TestCase string `json:"test_case"`
	Evidence string `json:"evidence"`
}

// Implicated names a story/criterion or test case whose evidence no longer
// matches the source comparison, so the source change must be revisited there.
type Implicated struct {
	Resource string   `json:"resource"`
	Kind     string   `json:"kind"`
	Via      []string `json:"via"`
}
