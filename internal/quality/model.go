// Package quality owns the read-only v5 quality-domain model, strict loader,
// validation, and graph projections. It intentionally has no dependency on
// the central saga loader, query dispatcher, CLI, or server.
package quality

import (
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
)

const (
	Version = 5
	RootDir = "___quality"

	ManifestSchemaURL       = "https://changesaga.dev/schema/v5/saga.schema.json"
	TestCaseSchemaURL       = "https://changesaga.dev/schema/v5/test-case.schema.json"
	RevisionSchemaURL       = "https://changesaga.dev/schema/v5/test-case-revision.schema.json"
	LifecycleEventSchemaURL = "https://changesaga.dev/schema/v5/test-case-event.schema.json"
	EvidenceSchemaURL       = "https://changesaga.dev/schema/v5/quality-evidence.schema.json"
	RunSchemaURL            = "https://changesaga.dev/schema/v5/test-run.schema.json"
	PolicySchemaURL         = "https://changesaga.dev/schema/v5/quality-policy.schema.json"

	MaxTestCases           = 10_000
	MaxPolicies            = 20_000
	MaxRevisionsPerTest    = 10_000
	MaxEventsPerTest       = 10_000
	MaxEvidencePerTest     = 20_000
	MaxRunsPerTest         = 50_000
	MaxStepsPerRevision    = 10_000
	MaxReferencesPerRecord = 50_000
	MaxRecordBytes         = 1 << 20
)

type AdoptionState string

const (
	NotAdopted   AdoptionState = "not_adopted"
	AdoptedEmpty AdoptionState = "adopted_empty"
	Adopted      AdoptionState = "adopted"
)

type SourceIdentity struct {
	Repository string `json:"repository"`
	Base       string `json:"base"`
	Head       string `json:"head"`
}

type TestCaseIdentity struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	RequestID string    `json:"request_id,omitempty"`
}

type CoverageKind string

const (
	CoveragePositive CoverageKind = "positive"
	CoverageNegative CoverageKind = "negative"
	CoverageEdge     CoverageKind = "edge"
)

type Automation string

const (
	AutomationManual    Automation = "manual"
	AutomationAutomated Automation = "automated"
	AutomationHybrid    Automation = "hybrid"
)

type Step struct {
	ID             string `json:"id"`
	Action         string `json:"action"`
	ExpectedResult string `json:"expected_result"`
}

// Revision is a complete, ordered test-case definition snapshot.
type Revision struct {
	Schema         string         `json:"$schema"`
	Version        int            `json:"version"`
	ID             string         `json:"id"`
	TestCase       string         `json:"test_case"`
	Parents        []string       `json:"parents"`
	Title          string         `json:"title"`
	CoverageKinds  []CoverageKind `json:"coverage_kinds"`
	Automation     Automation     `json:"automation"`
	Preconditions  []string       `json:"preconditions"`
	Steps          []Step         `json:"steps"`
	ExpectedResult string         `json:"expected_result"`
	CreatedAt      time.Time      `json:"created_at"`
	RequestID      string         `json:"request_id,omitempty"`
}

type LifecycleState string

const (
	StateProposed   LifecycleState = "proposed"
	StateActive     LifecycleState = "active"
	StateDeprecated LifecycleState = "deprecated"
	StateRetired    LifecycleState = "retired"
)

type LifecycleEvent struct {
	Schema    string         `json:"$schema"`
	Version   int            `json:"version"`
	ID        string         `json:"id"`
	TestCase  string         `json:"test_case"`
	Parents   []string       `json:"parents"`
	State     LifecycleState `json:"state"`
	Reason    string         `json:"reason,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	RequestID string         `json:"request_id,omitempty"`
}

type EvidenceRole string

const (
	EvidenceTestImplementation      EvidenceRole = "test_implementation"
	EvidenceImplementationUnderTest EvidenceRole = "implementation_under_test"
	EvidenceExecutionArtifact       EvidenceRole = "execution_artifact"
)

type Evidence struct {
	Schema        string              `json:"$schema"`
	Version       int                 `json:"version"`
	ID            string              `json:"id"`
	TestCase      string              `json:"test_case"`
	TestRevision  string              `json:"test_revision"`
	Role          EvidenceRole        `json:"role"`
	Code          []coderef.Reference `json:"code"`
	Verifications []string            `json:"verifications"`
	Citations     []string            `json:"citations"`
	Supersedes    []string            `json:"supersedes"`
	CreatedAt     time.Time           `json:"created_at"`
	RequestID     string              `json:"request_id,omitempty"`

	Current      bool     `json:"-"`
	StaleReasons []string `json:"-"`
}

type RunResult string

const (
	RunPassed  RunResult = "passed"
	RunFailed  RunResult = "failed"
	RunBlocked RunResult = "blocked"
	RunSkipped RunResult = "skipped"
)

type Run struct {
	Schema       string         `json:"$schema"`
	Version      int            `json:"version"`
	ID           string         `json:"id"`
	TestCase     string         `json:"test_case"`
	TestRevision string         `json:"test_revision"`
	Parents      []string       `json:"parents"`
	Source       SourceIdentity `json:"source"`
	Result       RunResult      `json:"result"`
	Summary      string         `json:"summary"`
	Command      string         `json:"command,omitempty"`
	Evidence     []string       `json:"evidence"`
	ExecutedAt   time.Time      `json:"executed_at"`
	RequestID    string         `json:"request_id,omitempty"`

	Current      bool     `json:"-"`
	StaleReasons []string `json:"-"`
}

type Policy struct {
	Schema            string         `json:"$schema"`
	Version           int            `json:"version"`
	ID                string         `json:"id"`
	Criterion         string         `json:"criterion"`
	StoryRevision     string         `json:"story_revision"`
	RequiredKinds     []CoverageKind `json:"required_kinds"`
	AllowedAutomation []Automation   `json:"allowed_automation"`
	Supersedes        []string       `json:"supersedes"`
	Rationale         string         `json:"rationale"`
	CreatedAt         time.Time      `json:"created_at"`
	RequestID         string         `json:"request_id,omitempty"`
}

type TestCase struct {
	Identity  TestCaseIdentity
	Revisions []Revision
	Events    []LifecycleEvent
	Evidence  []Evidence
	Runs      []Run

	RevisionHeads  []string
	LifecycleHeads []string
	EvidenceHeads  []string
	RunHeads       []string

	CurrentRevision  *Revision
	CurrentLifecycle *LifecycleEvent
	HeadRun          *Run
	CurrentRun       *Run
}

func (testCase TestCase) RevisionConflict() bool  { return len(testCase.RevisionHeads) > 1 }
func (testCase TestCase) LifecycleConflict() bool { return len(testCase.LifecycleHeads) > 1 }
func (testCase TestCase) RunConflict() bool       { return len(testCase.RunHeads) > 1 }

type PolicySet struct {
	Criterion     string
	StoryRevision string
	Policies      []Policy
	Heads         []string
	Current       *Policy
}

func (set PolicySet) Conflict() bool { return len(set.Heads) > 1 }

type Document struct {
	Root       string
	SagaID     string
	Source     SourceIdentity
	Adoption   AdoptionState
	TestCases  []TestCase
	Policies   []Policy
	PolicySets []PolicySet
}
