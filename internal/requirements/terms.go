package requirements

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
)

// Terms are the project's own vocabulary: the words a team says every day
// ("testtaker" for one sitting of an assessment) that baffle a newcomer until
// they dig through the code. A term is an app-level living record like a
// persona: an immutable identity, append-only complete revisions, and an
// append-only active/retired lifecycle. It references what it names: the
// stories it belongs to, other app records, and the exact code that defines
// it, pinned at a commit. A term's code references never count toward
// changed-line coverage; they are watched so a rename goes stale.
//
//	___overview/terms/<id>.term/{term.json, revisions/<id>.json, events/<id>.json}
const (
	TermSchemaURL         = "https://changesaga.dev/schema/v5/term.schema.json"
	TermRevisionSchemaURL = "https://changesaga.dev/schema/v5/term-revision.schema.json"
	TermEventSchemaURL    = "https://changesaga.dev/schema/v5/term-event.schema.json"

	MaxTerms = 10_000
	// MaxTermAliases, MaxTermLinks, and MaxTermCode bound one revision.
	MaxTermAliases = 32
	MaxTermLinks   = 256
	MaxTermCode    = 64
)

// TermRevision is a complete term snapshot.
type TermRevision struct {
	Schema     string   `json:"$schema"`
	Version    int      `json:"version"`
	ID         string   `json:"id"`
	Term       string   `json:"term"`
	Parents    []string `json:"parents"`
	Name       string   `json:"name"`
	Definition string   `json:"definition"`
	// Aliases are other spellings the team uses for the same thing.
	Aliases []string `json:"aliases,omitempty"`
	// Stories are the canonical URNs of the stories the term belongs to.
	Stories []string `json:"stories,omitempty"`
	// Records are canonical URNs of other app records the term names: a
	// persona, an epic, a feature flag, or another term.
	Records []string `json:"records,omitempty"`
	// Code is the exact code that defines the term, most often an enum value
	// or a constant, pinned at a commit.
	Code      []coderef.Reference `json:"code,omitempty"`
	CreatedAt time.Time           `json:"created_at"`
	RequestID string              `json:"request_id,omitempty"`
}

// TermState is active or retired. A retired term is vocabulary the project no
// longer uses; it stays as history.
type TermState string

const (
	TermActive  TermState = "active"
	TermRetired TermState = "retired"
)

type TermEvent struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Term      string    `json:"term"`
	Parents   []string  `json:"parents"`
	State     TermState `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	RequestID string    `json:"request_id,omitempty"`
}

type Term struct {
	Identity  RecordIdentity
	Revisions []TermRevision
	Events    []TermEvent

	RevisionHeads    []string
	LifecycleHeads   []string
	CurrentRevision  *TermRevision
	CurrentLifecycle *TermEvent
}

// Active reports whether the term's unique current lifecycle is active.
func (term Term) Active() bool {
	return term.CurrentLifecycle != nil && term.CurrentLifecycle.State == TermActive
}

// TermURN returns the canonical URN of a term.
func TermURN(sagaID, termID string) (string, error) {
	return appRecordURN(sagaID, "term", termID)
}

// TermRevisionURN and TermEventURN name the immutable members of a term.
func TermRevisionURN(sagaID, termID, id string) (string, error) {
	return appMemberURN(sagaID, "term", termID, "revision", id)
}
func TermEventURN(sagaID, termID, id string) (string, error) {
	return appMemberURN(sagaID, "term", termID, "event", id)
}

// ParseTermURN returns the term ID named by a canonical term URN.
func ParseTermURN(sagaID, value string) (string, error) {
	return parseAppRecordURN(sagaID, "term", value)
}

// TermPackagePath is the app-relative slash path of a term package.
func TermPackagePath(id string) string {
	return applayout.TermsDir + "/" + id + ".term"
}

// FindTerm returns the term with id, or nil.
func (document *Document) FindTerm(id string) *Term {
	for index := range document.Terms {
		if document.Terms[index].Identity.ID == id {
			return &document.Terms[index]
		}
	}
	return nil
}

// TermRecordKinds are the record kinds a term's records may name.
var TermRecordKinds = []string{"persona", "epic", "flag", "term"}

// termRecord parses a canonical persona, epic, flag, or term URN of sagaID.
func termRecord(sagaID, value string) (kind, id string, err error) {
	for _, kind := range TermRecordKinds {
		prefix := "urn:change-saga:" + sagaID + ":" + kind + ":"
		if id := strings.TrimPrefix(value, prefix); id != value && livingid.ValidID(id) {
			return kind, id, nil
		}
	}
	return "", "", fmt.Errorf("record %q must be a canonical persona, epic, flag, or term URN in saga %q", value, sagaID)
}

func loadTerms(document *Document) error {
	packages, err := loadAppPackages(document.Root, filepath.Join(document.Root, filepath.FromSlash(applayout.TermsDir)), ".term", "term.json", TermSchemaURL, MaxTerms)
	if err != nil {
		return err
	}
	for _, value := range packages {
		term := Term{Identity: value.identity}
		for _, path := range value.revisions {
			var revision TermRevision
			if err := readStrictJSON(path, &revision); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateTermRevision(revision, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			term.Revisions = append(term.Revisions, revision)
		}
		for _, path := range value.events {
			var event TermEvent
			if err := readStrictJSON(path, &event); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateTermEvent(event, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			term.Events = append(term.Events, event)
		}
		if err := resolveTerm(&term, document.SagaID); err != nil {
			return fmt.Errorf("term %q: %w", value.id, err)
		}
		document.Terms = append(document.Terms, term)
	}
	// Links are checked once every term is loaded, because a term may name
	// another term.
	for _, term := range document.Terms {
		for _, revision := range term.Revisions {
			if err := validateTermLinks(document, revision); err != nil {
				path := TermPackagePath(term.Identity.ID) + "/revisions/" + revision.ID + ".json"
				return fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	return nil
}

// validateTermRevision checks one revision's own shape. Whether the stories
// and records it names exist is checked by validateTermLinks.
func validateTermRevision(value TermRevision, sagaID, termID, expectedID string) error {
	var problems validationErrors
	if value.Schema != TermRevisionSchemaURL {
		problems.add("$schema must be %q", TermRevisionSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("revision id must be a stable identifier matching its filename")
	}
	want, _ := TermURN(sagaID, termID)
	if value.Term != want {
		problems.add("term must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if strings.TrimSpace(value.Name) == "" || value.Name != strings.TrimSpace(value.Name) {
		problems.add("name is required and has no surrounding space")
	}
	if strings.TrimSpace(value.Definition) == "" {
		problems.add("definition is required")
	}
	if len(value.Aliases) > MaxTermAliases {
		problems.add("a term has at most %d aliases", MaxTermAliases)
	}
	spellings := map[string]bool{strings.ToLower(value.Name): true}
	for _, alias := range value.Aliases {
		switch {
		case strings.TrimSpace(alias) == "" || alias != strings.TrimSpace(alias):
			problems.add("alias %q must be non-empty with no surrounding space", alias)
		case spellings[strings.ToLower(alias)]:
			problems.add("alias %q repeats the name or another alias", alias)
		}
		spellings[strings.ToLower(alias)] = true
	}
	if len(value.Stories)+len(value.Records) > MaxTermLinks {
		problems.add("a term names at most %d stories and records", MaxTermLinks)
	}
	seen := map[string]bool{}
	for _, story := range value.Stories {
		if ref, err := livingid.Parse(story); err != nil || ref.Kind != livingid.KindStory || ref.SagaID != sagaID {
			problems.add("story %q must be a canonical story URN in saga %q", story, sagaID)
		} else if seen[story] {
			problems.add("story %q is duplicated", story)
		}
		seen[story] = true
	}
	for _, record := range value.Records {
		if _, _, err := termRecord(sagaID, record); err != nil {
			problems.add("%s", err.Error())
		} else if record == want {
			problems.add("a term cannot name itself")
		} else if seen[record] {
			problems.add("record %q is duplicated", record)
		}
		seen[record] = true
	}
	if len(value.Code) > MaxTermCode {
		problems.add("a term has at most %d code references", MaxTermCode)
	}
	locations := map[string]bool{}
	for index, reference := range value.Code {
		if err := coderef.Validate(reference); err != nil {
			problems.add("code reference %d: %s", index+1, err.Error())
		} else if locations[reference.Location().String()] {
			problems.add("code reference %d duplicates %s", index+1, reference.Location())
		}
		locations[reference.Location().String()] = true
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

// validateTermLinks checks that every story and record a revision names
// exists.
func validateTermLinks(document *Document, value TermRevision) error {
	var problems validationErrors
	for _, story := range value.Stories {
		ref, err := livingid.Parse(story)
		if err == nil && findStory(document, ref.ID) == nil {
			problems.add("story %q does not exist", story)
		}
	}
	for _, record := range value.Records {
		kind, id, err := termRecord(document.SagaID, record)
		if err != nil {
			continue
		}
		exists := false
		switch kind {
		case "persona":
			exists = document.FindPersona(id) != nil
		case "epic":
			_, exists = applayout.Find(document.Epics, id)
		case "flag":
			exists = document.FindFlag(id) != nil
		case "term":
			exists = document.FindTerm(id) != nil
		}
		if !exists {
			problems.add("%s %q does not exist", kind, record)
		}
	}
	return problems.err()
}

func validateTermEvent(value TermEvent, sagaID, termID, expectedID string) error {
	var problems validationErrors
	if value.Schema != TermEventSchemaURL {
		problems.add("$schema must be %q", TermEventSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("event id must be a stable identifier matching its filename")
	}
	if want, _ := TermURN(sagaID, termID); value.Term != want {
		problems.add("term must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if value.State != TermActive && value.State != TermRetired {
		problems.add("state must be active or retired")
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

// resolveTerm computes the heads of both graphs. A term starts active; it may
// be retired and later restored.
func resolveTerm(term *Term, sagaID string) error {
	var problems validationErrors
	id := term.Identity.ID
	revisions := make([]graphMember, 0, len(term.Revisions))
	for _, revision := range term.Revisions {
		revisions = append(revisions, graphMember{revision.ID, revision.Parents})
	}
	heads, _ := resolveAppGraph(&problems, "revision", sagaID, "term", id, "revision", revisions)
	term.RevisionHeads = memberURNs(sagaID, "term", id, "revision", heads)
	term.CurrentRevision = nil
	if len(heads) == 1 {
		for index := range term.Revisions {
			if term.Revisions[index].ID == heads[0] {
				value := term.Revisions[index]
				term.CurrentRevision = &value
			}
		}
	}
	events := make([]graphMember, 0, len(term.Events))
	byID := map[string]TermEvent{}
	for _, event := range term.Events {
		events = append(events, graphMember{event.ID, event.Parents})
		byID[event.ID] = event
	}
	eventHeads, parents := resolveAppGraph(&problems, "lifecycle event", sagaID, "term", id, "event", events)
	for child, values := range parents {
		if len(values) == 1 && byID[values[0]].State == byID[child].State {
			problems.add("lifecycle event %q repeats state %s", child, byID[child].State)
		}
	}
	for _, event := range term.Events {
		if len(event.Parents) == 0 && event.State != TermActive {
			problems.add("initial lifecycle state must be active")
		}
	}
	term.LifecycleHeads = memberURNs(sagaID, "term", id, "event", eventHeads)
	term.CurrentLifecycle = nil
	if len(eventHeads) == 1 {
		value := byID[eventHeads[0]]
		term.CurrentLifecycle = &value
	}
	return problems.err()
}

// TermsNaming returns, for every story or record URN a current term revision
// names, the URNs of the terms that name it: the reverse of a term's links,
// so a story page can show the vocabulary it defines.
func (document *Document) TermsNaming() map[string][]string {
	index := map[string][]string{}
	for _, term := range document.Terms {
		if term.CurrentRevision == nil {
			continue
		}
		urn, _ := TermURN(document.SagaID, term.Identity.ID)
		for _, target := range append(append([]string{}, term.CurrentRevision.Stories...), term.CurrentRevision.Records...) {
			index[target] = append(index[target], urn)
		}
	}
	for key := range index {
		sort.Strings(index[key])
	}
	return index
}
