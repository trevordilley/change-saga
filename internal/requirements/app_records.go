package requirements

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
)

// Personas and feature flags are app-level living records. Like stories they
// have an immutable identity, append-only complete revisions, and an
// append-only lifecycle graph; the current value is the unique head of each
// graph and several heads are a visible conflict, never ordered by time.
//
//	___personas/<id>.persona/{persona.json, revisions/<id>.json, events/<id>.json}
//	___featureflags/<id>.flag/{flag.json, revisions/<id>.json, events/<id>.json}
const (
	AppRecordVersion = 5

	PersonaSchemaURL         = "https://changesaga.dev/schema/v5/persona.schema.json"
	PersonaRevisionSchemaURL = "https://changesaga.dev/schema/v5/persona-revision.schema.json"
	PersonaEventSchemaURL    = "https://changesaga.dev/schema/v5/persona-event.schema.json"

	FlagSchemaURL         = "https://changesaga.dev/schema/v5/flag.schema.json"
	FlagRevisionSchemaURL = "https://changesaga.dev/schema/v5/flag-revision.schema.json"
	FlagEventSchemaURL    = "https://changesaga.dev/schema/v5/flag-event.schema.json"

	MaxPersonas = 1_000
	MaxFlags    = 10_000
)

// RecordIdentity is the immutable identity file of a persona or flag.
type RecordIdentity struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	RequestID string    `json:"request_id,omitempty"`
}

// PersonaRevision is a complete persona snapshot.
type PersonaRevision struct {
	Schema      string    `json:"$schema"`
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Persona     string    `json:"persona"`
	Parents     []string  `json:"parents"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	RequestID   string    `json:"request_id,omitempty"`
}

// PersonaState is active or retired. Only an active persona must be served by
// an accepted story.
type PersonaState string

const (
	PersonaActive  PersonaState = "active"
	PersonaRetired PersonaState = "retired"
)

type PersonaEvent struct {
	Schema    string       `json:"$schema"`
	Version   int          `json:"version"`
	ID        string       `json:"id"`
	Persona   string       `json:"persona"`
	Parents   []string     `json:"parents"`
	State     PersonaState `json:"state"`
	Reason    string       `json:"reason,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	RequestID string       `json:"request_id,omitempty"`
}

type Persona struct {
	Identity  RecordIdentity
	Revisions []PersonaRevision
	Events    []PersonaEvent

	RevisionHeads    []string
	LifecycleHeads   []string
	CurrentRevision  *PersonaRevision
	CurrentLifecycle *PersonaEvent
}

// Active reports whether the persona's unique current lifecycle is active.
func (persona Persona) Active() bool {
	return persona.CurrentLifecycle != nil && persona.CurrentLifecycle.State == PersonaActive
}

// FlagRevision is a complete flag snapshot: what the flag is for and the
// stories and epics it gates.
type FlagRevision struct {
	Schema      string    `json:"$schema"`
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Flag        string    `json:"flag"`
	Parents     []string  `json:"parents"`
	Description string    `json:"description"`
	Targets     []string  `json:"targets"`
	CreatedAt   time.Time `json:"created_at"`
	RequestID   string    `json:"request_id,omitempty"`
}

// FlagState says whether gated work is enabled. A retired flag gates nothing.
type FlagState string

const (
	FlagOff     FlagState = "off"
	FlagOn      FlagState = "on"
	FlagRetired FlagState = "retired"
)

type FlagEvent struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Flag      string    `json:"flag"`
	Parents   []string  `json:"parents"`
	State     FlagState `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	RequestID string    `json:"request_id,omitempty"`
}

type Flag struct {
	Identity  RecordIdentity
	Revisions []FlagRevision
	Events    []FlagEvent

	RevisionHeads    []string
	LifecycleHeads   []string
	CurrentRevision  *FlagRevision
	CurrentLifecycle *FlagEvent
}

// PersonaURN returns the canonical URN of a persona.
func PersonaURN(sagaID, personaID string) (string, error) {
	return appRecordURN(sagaID, "persona", personaID)
}

// FlagURN returns the canonical URN of a feature flag.
func FlagURN(sagaID, flagID string) (string, error) {
	return appRecordURN(sagaID, "flag", flagID)
}

// PersonaRevisionURN, PersonaEventURN, FlagRevisionURN, and FlagEventURN name
// the immutable members of each record's graphs.
func PersonaRevisionURN(sagaID, personaID, id string) (string, error) {
	return appMemberURN(sagaID, "persona", personaID, "revision", id)
}
func PersonaEventURN(sagaID, personaID, id string) (string, error) {
	return appMemberURN(sagaID, "persona", personaID, "event", id)
}
func FlagRevisionURN(sagaID, flagID, id string) (string, error) {
	return appMemberURN(sagaID, "flag", flagID, "revision", id)
}
func FlagEventURN(sagaID, flagID, id string) (string, error) {
	return appMemberURN(sagaID, "flag", flagID, "event", id)
}

// ParsePersonaURN returns the persona ID named by a canonical persona URN.
func ParsePersonaURN(sagaID, value string) (string, error) {
	return parseAppRecordURN(sagaID, "persona", value)
}

// ParseFlagURN returns the flag ID named by a canonical flag URN.
func ParseFlagURN(sagaID, value string) (string, error) {
	return parseAppRecordURN(sagaID, "flag", value)
}

func appRecordURN(sagaID, kind, id string) (string, error) {
	if !livingid.ValidID(sagaID) || !livingid.ValidID(id) {
		return "", fmt.Errorf("%s ID is not a stable identifier", kind)
	}
	return "urn:change-saga:" + sagaID + ":" + kind + ":" + id, nil
}

func appMemberURN(sagaID, kind, id, member, memberID string) (string, error) {
	base, err := appRecordURN(sagaID, kind, id)
	if err != nil {
		return "", err
	}
	if !livingid.ValidID(memberID) {
		return "", fmt.Errorf("%s ID is not a stable identifier", member)
	}
	return base + ":" + member + ":" + memberID, nil
}

func parseAppRecordURN(sagaID, kind, value string) (string, error) {
	prefix := "urn:change-saga:" + sagaID + ":" + kind + ":"
	id := strings.TrimPrefix(value, prefix)
	if id == value || !livingid.ValidID(id) {
		return "", fmt.Errorf("%q is not a canonical %s URN in saga %q", value, kind, sagaID)
	}
	return id, nil
}

// parseMemberURN returns the member ID of a revision or event URN of one
// record, or "" when value names another record or kind.
func parseMemberURN(sagaID, kind, id, member, value string) string {
	prefix := "urn:change-saga:" + sagaID + ":" + kind + ":" + id + ":" + member + ":"
	memberID := strings.TrimPrefix(value, prefix)
	if memberID == value || !livingid.ValidID(memberID) {
		return ""
	}
	return memberID
}

func (document *Document) personaIDs() map[string]bool {
	ids := make(map[string]bool, len(document.Personas))
	for _, persona := range document.Personas {
		ids[persona.Identity.ID] = true
	}
	return ids
}

// FindPersona returns the persona with id, or nil.
func (document *Document) FindPersona(id string) *Persona {
	for index := range document.Personas {
		if document.Personas[index].Identity.ID == id {
			return &document.Personas[index]
		}
	}
	return nil
}

// FindFlag returns the flag with id, or nil.
func (document *Document) FindFlag(id string) *Flag {
	for index := range document.Flags {
		if document.Flags[index].Identity.ID == id {
			return &document.Flags[index]
		}
	}
	return nil
}

// FindStory returns the story with id, or nil.
func (document *Document) FindStory(id string) *Story { return findStory(document, id) }

// appPackage is one loaded <id>.<kind> directory before its members are typed.
type appPackage struct {
	id        string
	dir       string
	identity  RecordIdentity
	revisions []string
	events    []string
}

// loadAppPackages strictly reads every <id><suffix> package beneath dir: an
// identity file plus revisions/ and events/ directories of <id>.json records.
func loadAppPackages(root, dir, suffix, identityName, schemaURL string, maximum int) ([]appPackage, error) {
	present, err := realDirectory(dir)
	if err != nil || !present {
		return nil, err
	}
	entries, err := boundedReadDir(dir, maximum)
	if err != nil {
		return nil, err
	}
	packages := make([]appPackage, 0, len(entries))
	for _, entry := range entries {
		rel := relative(root, filepath.Join(dir, entry.Name()))
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			return nil, fmt.Errorf("%s: entries must be real <id>%s directories", rel, suffix)
		}
		id := strings.TrimSuffix(entry.Name(), suffix)
		if !livingid.ValidID(id) {
			return nil, fmt.Errorf("%s: package name is not a stable id", rel)
		}
		value := appPackage{id: id, dir: filepath.Join(dir, entry.Name())}
		members, err := boundedReadDir(value.dir, 3)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			switch {
			case member.Name() == identityName && member.Type().IsRegular():
			case (member.Name() == "revisions" || member.Name() == "events") && member.IsDir() && member.Type()&fs.ModeSymlink == 0:
			default:
				return nil, fmt.Errorf("%s: unknown entry %q", rel, member.Name())
			}
		}
		identityPath := filepath.Join(value.dir, identityName)
		if err := readStrictJSON(identityPath, &value.identity); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, identityPath), err)
		}
		var problems validationErrors
		if value.identity.Schema != schemaURL {
			problems.add("$schema must be %q", schemaURL)
		}
		if value.identity.Version != AppRecordVersion {
			problems.add("version must be %d", AppRecordVersion)
		}
		if value.identity.ID != id {
			problems.add("id must match its package name")
		}
		validateTime(&problems, value.identity.CreatedAt)
		validateRequestID(&problems, value.identity.RequestID)
		if err := problems.err(); err != nil {
			return nil, fmt.Errorf("%s: %w", relative(root, identityPath), err)
		}
		for _, member := range []string{"revisions", "events"} {
			memberDir := filepath.Join(value.dir, member)
			present, err := realDirectory(memberDir)
			if err != nil {
				return nil, err
			}
			if !present {
				return nil, fmt.Errorf("%s: missing %s directory", rel, member)
			}
			files, err := boundedReadDir(memberDir, MaxRevisionsPerStory)
			if err != nil {
				return nil, err
			}
			if len(files) == 0 {
				return nil, fmt.Errorf("%s: requires at least one %s record", rel, strings.TrimSuffix(member, "s"))
			}
			for _, file := range files {
				if file.Type()&fs.ModeSymlink != 0 || !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".json" {
					return nil, fmt.Errorf("%s: %s entry %q must be a real JSON file", rel, member, file.Name())
				}
				path := filepath.Join(memberDir, file.Name())
				if member == "revisions" {
					value.revisions = append(value.revisions, path)
				} else {
					value.events = append(value.events, path)
				}
			}
		}
		packages = append(packages, value)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].id < packages[j].id })
	return packages, nil
}

// graphMember is the part of a revision or event the graph rules need.
type graphMember struct {
	id      string
	parents []string
}

// resolveAppGraph applies the story graph rules to one persona or flag graph:
// unique IDs, parents that exist, exactly one root, and no cycle. It returns
// the head IDs.
func resolveAppGraph(problems *validationErrors, label, sagaID, kind, recordID, member string, members []graphMember) (heads []string, parentsOf map[string][]string) {
	byID := map[string]bool{}
	for _, value := range members {
		if byID[value.id] {
			problems.add("%s id %q is duplicated", label, value.id)
		}
		byID[value.id] = true
	}
	parentsOf = map[string][]string{}
	for _, value := range members {
		seen := map[string]bool{}
		for _, parent := range value.parents {
			parentID := parseMemberURN(sagaID, kind, recordID, member, parent)
			switch {
			case parentID == "":
				problems.add("%s %q parent %q is not a %s of this %s", label, value.id, parent, member, kind)
			case seen[parentID]:
				problems.add("%s %q parent %q is duplicated", label, value.id, parent)
			case !byID[parentID]:
				problems.add("%s %q names missing parent %q", label, value.id, parent)
			default:
				parentsOf[value.id] = append(parentsOf[value.id], parentID)
			}
			seen[parentID] = true
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	heads, roots, cycle := graphHeads(parentsOf, ids)
	if len(roots) != 1 {
		problems.add("%s graph must have exactly one initial %s; found %d", label, label, len(roots))
	}
	if cycle {
		problems.add("%s graph must be acyclic", label)
	}
	return heads, parentsOf
}

func memberURNs(sagaID, kind, recordID, member string, ids []string) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		urn, _ := appMemberURN(sagaID, kind, recordID, member, id)
		values = append(values, urn)
	}
	return values
}

func loadPersonas(document *Document) error {
	packages, err := loadAppPackages(document.Root, filepath.Join(document.Root, applayout.PersonasDir), ".persona", "persona.json", PersonaSchemaURL, MaxPersonas)
	if err != nil {
		return err
	}
	for _, value := range packages {
		persona := Persona{Identity: value.identity}
		for _, path := range value.revisions {
			var revision PersonaRevision
			if err := readStrictJSON(path, &revision); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validatePersonaRevision(revision, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			persona.Revisions = append(persona.Revisions, revision)
		}
		for _, path := range value.events {
			var event PersonaEvent
			if err := readStrictJSON(path, &event); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validatePersonaEvent(event, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			persona.Events = append(persona.Events, event)
		}
		if err := resolvePersona(&persona, document.SagaID); err != nil {
			return fmt.Errorf("persona %q: %w", value.id, err)
		}
		document.Personas = append(document.Personas, persona)
	}
	return nil
}

func validatePersonaRevision(value PersonaRevision, sagaID, personaID, expectedID string) error {
	var problems validationErrors
	if value.Schema != PersonaRevisionSchemaURL {
		problems.add("$schema must be %q", PersonaRevisionSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("revision id must be a stable identifier matching its filename")
	}
	if want, _ := PersonaURN(sagaID, personaID); value.Persona != want {
		problems.add("persona must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if strings.TrimSpace(value.Name) == "" {
		problems.add("name is required")
	}
	if strings.TrimSpace(value.Description) == "" {
		problems.add("description is required")
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validatePersonaEvent(value PersonaEvent, sagaID, personaID, expectedID string) error {
	var problems validationErrors
	if value.Schema != PersonaEventSchemaURL {
		problems.add("$schema must be %q", PersonaEventSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("event id must be a stable identifier matching its filename")
	}
	if want, _ := PersonaURN(sagaID, personaID); value.Persona != want {
		problems.add("persona must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if value.State != PersonaActive && value.State != PersonaRetired {
		problems.add("state must be active or retired")
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

// resolvePersona computes the heads of both graphs. A persona starts active;
// it may be retired and later restored.
func resolvePersona(persona *Persona, sagaID string) error {
	var problems validationErrors
	id := persona.Identity.ID
	revisions := make([]graphMember, 0, len(persona.Revisions))
	for _, revision := range persona.Revisions {
		revisions = append(revisions, graphMember{revision.ID, revision.Parents})
	}
	heads, _ := resolveAppGraph(&problems, "revision", sagaID, "persona", id, "revision", revisions)
	persona.RevisionHeads = memberURNs(sagaID, "persona", id, "revision", heads)
	persona.CurrentRevision = nil
	if len(heads) == 1 {
		for index := range persona.Revisions {
			if persona.Revisions[index].ID == heads[0] {
				value := persona.Revisions[index]
				persona.CurrentRevision = &value
			}
		}
	}
	events := make([]graphMember, 0, len(persona.Events))
	byID := map[string]PersonaEvent{}
	for _, event := range persona.Events {
		events = append(events, graphMember{event.ID, event.Parents})
		byID[event.ID] = event
	}
	eventHeads, parents := resolveAppGraph(&problems, "lifecycle event", sagaID, "persona", id, "event", events)
	for child, values := range parents {
		if len(values) == 1 && byID[values[0]].State == byID[child].State {
			problems.add("lifecycle event %q repeats state %s", child, byID[child].State)
		}
	}
	for _, event := range persona.Events {
		if len(event.Parents) == 0 && event.State != PersonaActive {
			problems.add("initial lifecycle state must be active")
		}
	}
	persona.LifecycleHeads = memberURNs(sagaID, "persona", id, "event", eventHeads)
	persona.CurrentLifecycle = nil
	if len(eventHeads) == 1 {
		value := byID[eventHeads[0]]
		persona.CurrentLifecycle = &value
	}
	return problems.err()
}

func loadFlags(document *Document) error {
	packages, err := loadAppPackages(document.Root, filepath.Join(document.Root, applayout.FeatureFlagsDir), ".flag", "flag.json", FlagSchemaURL, MaxFlags)
	if err != nil {
		return err
	}
	for _, value := range packages {
		flag := Flag{Identity: value.identity}
		for _, path := range value.revisions {
			var revision FlagRevision
			if err := readStrictJSON(path, &revision); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateFlagRevision(revision, document, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			flag.Revisions = append(flag.Revisions, revision)
		}
		for _, path := range value.events {
			var event FlagEvent
			if err := readStrictJSON(path, &event); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			if err := validateFlagEvent(event, document.SagaID, value.id, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
				return fmt.Errorf("%s: %w", relative(document.Root, path), err)
			}
			flag.Events = append(flag.Events, event)
		}
		if err := resolveFlag(&flag, document.SagaID); err != nil {
			return fmt.Errorf("flag %q: %w", value.id, err)
		}
		document.Flags = append(document.Flags, flag)
	}
	return nil
}

// FlagTarget is one parsed gate target.
type FlagTarget struct {
	Kind string // "story" or "epic"
	ID   string
}

// ParseFlagTarget accepts a canonical story or epic URN of sagaID.
func ParseFlagTarget(sagaID, value string) (FlagTarget, error) {
	if ref, err := livingid.Parse(value); err == nil && ref.Kind == livingid.KindStory && ref.SagaID == sagaID {
		return FlagTarget{Kind: "story", ID: ref.ID}, nil
	}
	prefix := "urn:change-saga:" + sagaID + ":epic:"
	if id := strings.TrimPrefix(value, prefix); id != value && livingid.ValidID(id) {
		return FlagTarget{Kind: "epic", ID: id}, nil
	}
	return FlagTarget{}, fmt.Errorf("target %q must be a canonical story or epic URN in saga %q", value, sagaID)
}

func validateFlagRevision(value FlagRevision, document *Document, flagID, expectedID string) error {
	var problems validationErrors
	sagaID := document.SagaID
	if value.Schema != FlagRevisionSchemaURL {
		problems.add("$schema must be %q", FlagRevisionSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("revision id must be a stable identifier matching its filename")
	}
	if want, _ := FlagURN(sagaID, flagID); value.Flag != want {
		problems.add("flag must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if strings.TrimSpace(value.Description) == "" {
		problems.add("description is required")
	}
	if len(value.Targets) == 0 {
		problems.add("targets must name at least one story or epic")
	}
	seen := map[string]bool{}
	for _, target := range value.Targets {
		parsed, err := ParseFlagTarget(sagaID, target)
		switch {
		case err != nil:
			problems.add("%s", err.Error())
		case seen[target]:
			problems.add("target %q is duplicated", target)
		case parsed.Kind == "story" && findStory(document, parsed.ID) == nil:
			problems.add("target story %q does not exist", target)
		case parsed.Kind == "epic":
			if _, ok := applayout.Find(document.Epics, parsed.ID); !ok {
				problems.add("target epic %q does not exist", target)
			}
		}
		seen[target] = true
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateFlagEvent(value FlagEvent, sagaID, flagID, expectedID string) error {
	var problems validationErrors
	if value.Schema != FlagEventSchemaURL {
		problems.add("$schema must be %q", FlagEventSchemaURL)
	}
	if value.Version != AppRecordVersion {
		problems.add("version must be %d", AppRecordVersion)
	}
	if !livingid.ValidID(value.ID) || (expectedID != "" && value.ID != expectedID) {
		problems.add("event id must be a stable identifier matching its filename")
	}
	if want, _ := FlagURN(sagaID, flagID); value.Flag != want {
		problems.add("flag must be %q", want)
	}
	if value.Parents == nil {
		problems.add("parents is required")
	}
	if value.State != FlagOff && value.State != FlagOn && value.State != FlagRetired {
		problems.add("state must be off, on, or retired")
	}
	validateTime(&problems, value.CreatedAt)
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

// resolveFlag computes the heads of both graphs. A flag starts off or on and
// toggles between them; retired is terminal.
func resolveFlag(flag *Flag, sagaID string) error {
	var problems validationErrors
	id := flag.Identity.ID
	revisions := make([]graphMember, 0, len(flag.Revisions))
	for _, revision := range flag.Revisions {
		revisions = append(revisions, graphMember{revision.ID, revision.Parents})
	}
	heads, _ := resolveAppGraph(&problems, "revision", sagaID, "flag", id, "revision", revisions)
	flag.RevisionHeads = memberURNs(sagaID, "flag", id, "revision", heads)
	flag.CurrentRevision = nil
	if len(heads) == 1 {
		for index := range flag.Revisions {
			if flag.Revisions[index].ID == heads[0] {
				value := flag.Revisions[index]
				flag.CurrentRevision = &value
			}
		}
	}
	events := make([]graphMember, 0, len(flag.Events))
	byID := map[string]FlagEvent{}
	for _, event := range flag.Events {
		events = append(events, graphMember{event.ID, event.Parents})
		byID[event.ID] = event
	}
	eventHeads, parents := resolveAppGraph(&problems, "lifecycle event", sagaID, "flag", id, "event", events)
	for child, values := range parents {
		if len(values) != 1 {
			continue
		}
		from, to := byID[values[0]].State, byID[child].State
		if from == to || from == FlagRetired {
			problems.add("lifecycle event %q cannot transition from %s to %s", child, from, to)
		}
	}
	for _, event := range flag.Events {
		if len(event.Parents) == 0 && event.State == FlagRetired {
			problems.add("initial flag state must be off or on")
		}
	}
	flag.LifecycleHeads = memberURNs(sagaID, "flag", id, "event", eventHeads)
	flag.CurrentLifecycle = nil
	if len(eventHeads) == 1 {
		value := byID[eventHeads[0]]
		flag.CurrentLifecycle = &value
	}
	return problems.err()
}

// Gate is the projection of every current flag onto the stories it gates. A
// story is gated when a current off flag targets it or its epic; it is enabled
// otherwise. Conflicted flags gate conservatively, as if off.
type Gate struct {
	// Flags lists the current off (or conflicted) flag URNs gating each story.
	Off map[string][]string
	// On lists the on flag URNs that name each story or its epic.
	On map[string][]string
}

// Gates projects the current flags onto story IDs.
func (document *Document) Gates() Gate {
	gate := Gate{Off: map[string][]string{}, On: map[string][]string{}}
	storiesByEpic := map[string][]string{}
	for _, story := range document.Stories {
		storiesByEpic[story.Epic] = append(storiesByEpic[story.Epic], story.Identity.ID)
	}
	for _, flag := range document.Flags {
		urn, _ := FlagURN(document.SagaID, flag.Identity.ID)
		state := FlagOff
		if flag.CurrentLifecycle != nil {
			state = flag.CurrentLifecycle.State
		}
		if state == FlagRetired {
			continue
		}
		targets := map[string]bool{}
		for _, revision := range flag.Revisions {
			for _, head := range flag.RevisionHeads {
				if parseMemberURN(document.SagaID, "flag", flag.Identity.ID, "revision", head) != revision.ID {
					continue
				}
				for _, target := range revision.Targets {
					parsed, err := ParseFlagTarget(document.SagaID, target)
					if err != nil {
						continue
					}
					if parsed.Kind == "story" {
						targets[parsed.ID] = true
					} else {
						for _, story := range storiesByEpic[parsed.ID] {
							targets[story] = true
						}
					}
				}
			}
		}
		for story := range targets {
			if state == FlagOn {
				gate.On[story] = append(gate.On[story], urn)
			} else {
				gate.Off[story] = append(gate.Off[story], urn)
			}
		}
	}
	for _, values := range []map[string][]string{gate.Off, gate.On} {
		for key := range values {
			sort.Strings(values[key])
		}
	}
	return gate
}
