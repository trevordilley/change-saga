package requirements

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
)

// Deprecations are authored assertions, separate from their targets and code coverage.
const (
	DeprecationSchemaURL         = "https://changesaga.dev/schema/v5/deprecation.schema.json"
	DeprecationRevisionSchemaURL = "https://changesaga.dev/schema/v5/deprecation-revision.schema.json"
	DeprecationEventSchemaURL    = "https://changesaga.dev/schema/v5/deprecation-event.schema.json"

	MaxDeprecations = 10_000
)

// DeprecationTarget pins exactly one content identity; scope never expands through links.
type DeprecationTarget struct {
	Target        string `json:"target"`
	Revision      string `json:"revision,omitempty"`
	ContentDigest string `json:"content_digest,omitempty"`
	// Element is a whiteboard appearance path, pinned to that board revision.
	Element string `json:"element,omitempty"`
}

type DeprecationRevision struct {
	Schema      string              `json:"$schema"`
	Version     int                 `json:"version"`
	ID          string              `json:"id"`
	Deprecation string              `json:"deprecation"`
	Parents     []string            `json:"parents"`
	Reason      string              `json:"reason"`
	Scope       string              `json:"scope"`
	Target      DeprecationTarget   `json:"target"`
	Replacement *DeprecationTarget  `json:"replacement,omitempty"`
	Code        []coderef.Reference `json:"code,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	RequestID   string              `json:"request_id,omitempty"`
}

// DeprecationState is active or withdrawn. Withdrawal preserves the assertion as history.
type DeprecationState string

const (
	DeprecationActive    DeprecationState = "active"
	DeprecationWithdrawn DeprecationState = "withdrawn"
)

type DeprecationEvent struct {
	Schema      string           `json:"$schema"`
	Version     int              `json:"version"`
	ID          string           `json:"id"`
	Deprecation string           `json:"deprecation"`
	Parents     []string         `json:"parents"`
	State       DeprecationState `json:"state"`
	Reason      string           `json:"reason,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	RequestID   string           `json:"request_id,omitempty"`
}

type Deprecation struct {
	Identity  RecordIdentity        `json:"identity"`
	Revisions []DeprecationRevision `json:"revisions"`
	Events    []DeprecationEvent    `json:"events"`

	RevisionHeads    []string             `json:"revision_heads"`
	LifecycleHeads   []string             `json:"lifecycle_heads"`
	CurrentRevision  *DeprecationRevision `json:"current_revision"`
	CurrentLifecycle *DeprecationEvent    `json:"current_lifecycle"`
}

// Active reports whether the deprecation's unique current lifecycle is active.
func (deprecation Deprecation) Active() bool {
	return deprecation.CurrentLifecycle != nil && deprecation.CurrentLifecycle.State == DeprecationActive
}

// DeprecationURN returns the canonical URN of a deprecation.
func DeprecationURN(sagaID, deprecationID string) (string, error) {
	return appRecordURN(sagaID, "deprecation", deprecationID)
}

// DeprecationRevisionURN and DeprecationEventURN name the immutable members of a deprecation.
func DeprecationRevisionURN(sagaID, deprecationID, id string) (string, error) {
	return appMemberURN(sagaID, "deprecation", deprecationID, "revision", id)
}
func DeprecationEventURN(sagaID, deprecationID, id string) (string, error) {
	return appMemberURN(sagaID, "deprecation", deprecationID, "event", id)
}

// ParseDeprecationURN returns the deprecation ID named by a canonical deprecation URN.
func ParseDeprecationURN(sagaID, value string) (string, error) {
	return parseAppRecordURN(sagaID, "deprecation", value)
}

// DeprecationPackagePath is the app-relative slash path of a deprecation package.
func DeprecationPackagePath(id string) string {
	return "___deprecations" + "/" + id + ".deprecation"
}

// FindDeprecation returns the deprecation with id, or nil.
func (document *Document) FindDeprecation(id string) *Deprecation {
	for index := range document.Deprecations {
		if document.Deprecations[index].Identity.ID == id {
			return &document.Deprecations[index]
		}
	}
	return nil
}

func loadDeprecations(document *Document) error {
	packages, err := loadAppPackages(document.Root, filepath.Join(document.Root, filepath.FromSlash("___deprecations")), ".deprecation", "deprecation.json", DeprecationSchemaURL, MaxDeprecations)
	if err != nil {
		return err
	}
	for _, value := range packages {
		deprecation := Deprecation{Identity: value.identity}
		for _, path := range value.revisions {
			var revision DeprecationRevision
			if err := readStrictJSON(path, &revision); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateDeprecationRevision(revision, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			deprecation.Revisions = append(deprecation.Revisions, revision)
		}
		for _, path := range value.events {
			var event DeprecationEvent
			if err := readStrictJSON(path, &event); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateDeprecationEvent(event, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			deprecation.Events = append(deprecation.Events, event)
		}
		if err := resolveDeprecation(&deprecation, document.SagaID); err != nil {
			return fmt.Errorf("deprecation %q: %w", value.id, err)
		}
		document.Deprecations = append(document.Deprecations, deprecation)
	}

	return nil
}

// validateDeprecationRevision checks one revision's shape. Target currency is
// observed separately so historical assertions survive changed or missing targets.
func validateDeprecationRevision(value DeprecationRevision, sagaID, deprecationID, expectedID string) error {
	var problems validationErrors
	if value.Schema != DeprecationRevisionSchemaURL {
		problems.add("$schema must be %q", DeprecationRevisionSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("revision id must be a stable identifier matching its filename")
	}
	want, _ := DeprecationURN(sagaID, deprecationID)
	if value.Deprecation != want {
		problems.add("deprecation must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if strings.TrimSpace(value.Reason) == "" {
		problems.add("reason is required")
	}
	if value.Scope != "content" && value.Scope != "code" {
		problems.add("scope must be content or code")
	}
	if value.Scope == "code" && len(value.Code) == 0 {
		problems.add("code scope requires explicit exact code references")
	}
	if err := ValidateDeprecationTarget(sagaID, value.Target); err != nil {
		problems.add("target: %s", err)
	}
	if value.Replacement != nil {
		if err := ValidateDeprecationTarget(sagaID, *value.Replacement); err != nil {
			problems.add("replacement: %s", err)
		}
		if *value.Replacement == value.Target {
			problems.add("replacement cannot equal target")
		}
	}
	if len(value.Code) > 64 {
		problems.add("at most 64 code references")
	}
	seen := map[string]bool{}
	for _, ref := range value.Code {
		if err := coderef.Validate(ref); err != nil {
			problems.add("code: %s", err)
		}
		key := ref.Location().String()
		if seen[key] {
			problems.add("duplicate code reference %s", key)
		}
		seen[key] = true
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateDeprecationEvent(value DeprecationEvent, sagaID, deprecationID, expectedID string) error {
	var problems validationErrors
	if value.Schema != DeprecationEventSchemaURL {
		problems.add("$schema must be %q", DeprecationEventSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("event id must be a stable identifier matching its filename")
	}
	if want, _ := DeprecationURN(sagaID, deprecationID); value.Deprecation != want {
		problems.add("deprecation must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if value.State != DeprecationActive && value.State != DeprecationWithdrawn {
		problems.add("state must be active or withdrawn")
	}
	if len(value.Parents) > 0 && strings.TrimSpace(value.Reason) == "" {
		problems.add("lifecycle changes require a reason")
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

// resolveDeprecation computes the heads of both graphs. A deprecation starts active; it may
// be withdrawn and later restored.
func resolveDeprecation(deprecation *Deprecation, sagaID string) error {
	var problems validationErrors
	id := deprecation.Identity.ID
	revisions := make([]graphMember, 0, len(deprecation.Revisions))
	for _, revision := range deprecation.Revisions {
		revisions = append(revisions, graphMember{revision.ID, revision.Parents})
	}
	heads, _ := resolveAppGraph(&problems, "revision", sagaID, "deprecation", id, "revision", revisions)
	deprecation.RevisionHeads = memberURNs(sagaID, "deprecation", id, "revision", heads)
	deprecation.CurrentRevision = nil
	if len(heads) == 1 {
		for index := range deprecation.Revisions {
			if deprecation.Revisions[index].ID == heads[0] {
				value := deprecation.Revisions[index]
				deprecation.CurrentRevision = &value
			}
		}
	}
	events := make([]graphMember, 0, len(deprecation.Events))
	byID := map[string]DeprecationEvent{}
	for _, event := range deprecation.Events {
		events = append(events, graphMember{event.ID, event.Parents})
		byID[event.ID] = event
	}
	eventHeads, parents := resolveAppGraph(&problems, "lifecycle event", sagaID, "deprecation", id, "event", events)
	for child, values := range parents {
		if len(values) == 1 && byID[values[0]].State == byID[child].State {
			problems.add("lifecycle event %q repeats state %s", child, byID[child].State)
		}
	}
	for _, event := range deprecation.Events {
		if len(event.Parents) == 0 && event.State != DeprecationActive {
			problems.add("initial lifecycle state must be active")
		}
	}
	deprecation.LifecycleHeads = memberURNs(sagaID, "deprecation", id, "event", eventHeads)
	deprecation.CurrentLifecycle = nil
	if len(eventHeads) == 1 {
		value := byID[eventHeads[0]]
		deprecation.CurrentLifecycle = &value
	}
	return problems.err()
}
