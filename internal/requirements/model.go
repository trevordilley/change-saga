// Package requirements owns the v3 requirements-domain records of a living
// Change Saga. It intentionally has no dependency on the central saga loader,
// query registry, or CLI dispatcher so those integration points can evolve
// independently.
package requirements

import (
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
)

const (
	Version = 3

	StorySchemaURL          = "https://changesaga.dev/schema/v3/story.schema.json"
	RevisionSchemaURL       = "https://changesaga.dev/schema/v3/story-revision.schema.json"
	LifecycleEventSchemaURL = "https://changesaga.dev/schema/v3/story-event.schema.json"
	CitationSchemaURL       = "https://changesaga.dev/schema/v3/citation.schema.json"
	RelationSchemaURL       = "https://changesaga.dev/schema/v3/relation.schema.json"

	// V5RelationVersion and V5RelationSchemaURL identify the frozen v5 relation
	// record. Only a v5 Saga may contain or be written with one.
	V5RelationVersion   = 5
	V5RelationSchemaURL = "https://changesaga.dev/schema/v5/relation.schema.json"

	MaxStories           = 10_000
	MaxRevisionsPerStory = 10_000
	MaxEventsPerStory    = 10_000
	MaxCitations         = 20_000
	MaxRelations         = 50_000
	MaxRecordBytes       = 1 << 20
)

type StoryIdentity struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	RequestID string    `json:"request_id,omitempty"`
}

type Criterion struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
}

// Revision is a complete story snapshot. A reader never inherits omitted
// values from a parent, which keeps branch merges deterministic.
type Revision struct {
	Schema    string   `json:"$schema"`
	Version   int      `json:"version"`
	ID        string   `json:"id"`
	Story     string   `json:"story"`
	Parents   []string `json:"parents"`
	Title     string   `json:"title"`
	Statement string   `json:"statement"`
	Priority  string   `json:"priority"`
	// Personas names every persona this revision of the story serves. It is a
	// defining attribute of the story, so every revision records it.
	Personas           []string    `json:"personas"`
	Citations          []string    `json:"citations"`
	AcceptanceCriteria []Criterion `json:"acceptance_criteria"`
	CreatedAt          time.Time   `json:"created_at"`
	RequestID          string      `json:"request_id,omitempty"`
}

type LifecycleState string

const (
	StateProposed LifecycleState = "proposed"
	StateAccepted LifecycleState = "accepted"
	StateDeferred LifecycleState = "deferred"
	StateRejected LifecycleState = "rejected"
	StateRetired  LifecycleState = "retired"
)

type LifecycleEvent struct {
	Schema    string         `json:"$schema"`
	Version   int            `json:"version"`
	ID        string         `json:"id"`
	Story     string         `json:"story"`
	Parents   []string       `json:"parents"`
	State     LifecycleState `json:"state"`
	Reason    string         `json:"reason,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	RequestID string         `json:"request_id,omitempty"`
}

type CitationKind string

const (
	CitationURL              CitationKind = "url"
	CitationRepositoryCommit CitationKind = "repository_commit"
	CitationIssue            CitationKind = "issue"
	CitationDocument         CitationKind = "document"
	CitationDecision         CitationKind = "decision"
)

// Citation is immutable. Reference is the authoritative locator: an absolute
// URL, repository-and-commit notation, issue key/URL, document identity, or
// recorded-decision identity according to Kind.
type Citation struct {
	Schema    string       `json:"$schema"`
	Version   int          `json:"version"`
	ID        string       `json:"id"`
	Kind      CitationKind `json:"kind"`
	Title     string       `json:"title"`
	Reference string       `json:"reference"`
	CreatedAt time.Time    `json:"created_at"`
	RequestID string       `json:"request_id,omitempty"`

	// Epic is the epic whose directory holds the record. It is where the
	// record lives, never part of its identity.
	Epic string `json:"-"`
}

type RelationType string

const (
	RelationRefines       RelationType = "refines"
	RelationAddresses     RelationType = "addresses"
	RelationImplements    RelationType = "implements"
	RelationExplains      RelationType = "explains"
	RelationVerifies      RelationType = "verifies"
	RelationSupersedes    RelationType = "supersedes"
	RelationConflictsWith RelationType = "conflicts_with"
)

// RelationScope is persisted only on v5 relations. Self asserts only the named
// source; descendants explicitly permits Deck/Slide containment traversal.
type RelationScope string

const (
	ScopeSelf        RelationScope = "self"
	ScopeDescendants RelationScope = "descendants"
)

type RelationState string

const (
	RelationActive     RelationState = "active"
	RelationSuperseded RelationState = "superseded"
)

// Relation pins mutable endpoints separately from their stable identities.
// Computed stale fields are projections and are never persisted. Version is 3
// or 5; Scope is present exactly on v5 records, so v3 bytes are unchanged.
type Relation struct {
	Schema             string        `json:"$schema"`
	Version            int           `json:"version"`
	ID                 string        `json:"id"`
	Type               RelationType  `json:"type"`
	From               string        `json:"from"`
	To                 string        `json:"to"`
	Scope              RelationScope `json:"scope,omitempty"`
	Rationale          string        `json:"rationale"`
	FromRevision       string        `json:"from_revision,omitempty"`
	ToRevision         string        `json:"to_revision,omitempty"`
	FromContentDigest  string        `json:"from_content_digest,omitempty"`
	ToContentDigest    string        `json:"to_content_digest,omitempty"`
	State              RelationState `json:"state"`
	CreatedAt          time.Time     `json:"created_at"`
	RequestID          string        `json:"request_id,omitempty"`
	SupersededAt       *time.Time    `json:"superseded_at,omitempty"`
	SupersedeRequestID string        `json:"supersede_request_id,omitempty"`

	Stale        bool     `json:"-"`
	StaleReasons []string `json:"-"`
	// Epic is the epic whose directory holds the record.
	Epic string `json:"-"`
}

type Story struct {
	// Epic is the epic whose directory holds the story. Moving a story
	// between epics changes only this; its URN and every pin stay the same.
	Epic      string
	Identity  StoryIdentity
	Revisions []Revision
	Events    []LifecycleEvent

	RevisionHeads    []string
	LifecycleHeads   []string
	CurrentRevision  *Revision
	CurrentLifecycle *LifecycleEvent
}

func (story Story) RevisionConflict() bool  { return len(story.RevisionHeads) > 1 }
func (story Story) LifecycleConflict() bool { return len(story.LifecycleHeads) > 1 }

type Document struct {
	Root      string
	SagaID    string
	Epics     []applayout.Epic
	Personas  []Persona
	Flags     []Flag
	Stories   []Story
	Citations []Citation
	Relations []Relation
}

// StaleInputs supplies current values owned outside this package. Keys are
// stable endpoint URNs. Missing marks identities known to have disappeared;
// an absent map entry alone means "unknown", not stale. Test cases are the
// exception: a pinned test-case endpoint is never current unless its heads
// were supplied through SetTestCaseHeads.
type StaleInputs struct {
	CurrentRevisions      map[string]string
	CurrentContentDigests map[string]string
	Missing               map[string]bool
	// ConflictedRevisions lists the competing revision heads of an endpoint
	// that has more than one.
	ConflictedRevisions map[string][]string
	// TestCasesSupplied records that SetTestCaseHeads supplied every test case
	// in the Saga, so an unlisted test-case endpoint is missing.
	TestCasesSupplied bool
}

type LoadOptions struct {
	StaleInputs StaleInputs
}

type AddStoryInput struct {
	// Epic is the epic the new story is written into.
	Epic               string
	ID                 string
	RevisionID         string
	EventID            string
	Title              string
	Statement          string
	Priority           string
	Personas           []string
	Citations          []string
	AcceptanceCriteria []Criterion
	CreatedAt          time.Time
	RequestID          string
}

type ReviseStoryInput struct {
	Story              string
	ID                 string
	Parents            []string
	Title              string
	Statement          string
	Priority           string
	Personas           []string
	Citations          []string
	AcceptanceCriteria []Criterion
	CreatedAt          time.Time
	RequestID          string
}

// AddCriterionInput appends one criterion to the complete current story
// snapshot. Parent is deliberately singular: criterion conveniences are not a
// reconciliation surface and therefore refuse a story with multiple heads.
type AddCriterionInput struct {
	Story      string
	Parent     string
	RevisionID string
	Criterion  Criterion
	CreatedAt  time.Time
	RequestID  string
}

// ReviseCriterionInput changes the wording of one stable criterion without
// changing its identity or any other part of the story snapshot.
type ReviseCriterionInput struct {
	Story      string
	Criterion  string
	Parent     string
	RevisionID string
	Statement  string
	CreatedAt  time.Time
	RequestID  string
}

// RemoveCriterionInput omits one criterion from the next complete story
// snapshot. Reason is returned to the caller as commit guidance; the durable
// history remains the immutable parent/child revision pair.
type RemoveCriterionInput struct {
	Story      string
	Criterion  string
	Parent     string
	RevisionID string
	Reason     string
	CreatedAt  time.Time
	RequestID  string
}

type SetStoryStateInput struct {
	Story     string
	ID        string
	Parents   []string
	State     LifecycleState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

type AddCitationInput struct {
	Epic      string
	ID        string
	Kind      CitationKind
	Title     string
	Reference string
	CreatedAt time.Time
	RequestID string
}

type AddRelationInput struct {
	Epic              string
	ID                string
	Type              RelationType
	From              string
	To                string
	Rationale         string
	FromRevision      string
	ToRevision        string
	FromContentDigest string
	ToContentDigest   string
	// Scope applies only on a v5 Saga, where an empty value means self.
	Scope     RelationScope
	CreatedAt time.Time
	RequestID string
}

type MutationResult struct {
	URN          string
	Path         string
	Created      []string
	Paths        []string
	CurrentHeads []string
	Reason       string
	Replayed     bool
}
