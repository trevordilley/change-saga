package requirements

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Inventory is deliberately loaded separately: a deck shell needs only its
// explicit pins, not every technical definition and its historical graph.
const InventoryDir = "___inventory"
const MaxInventoryRecords = 10000

type DocumentationLink = saga.DocumentationLink

// Evidence is one exact code reference. Inventory format 2 revisions give each
// reference an ID unique across the whole revision, so a selection names
// (revision pin, evidence ID) rather than a mutable array position. Legacy
// references have no ID and are not selectable.
type Evidence struct {
	ID string `json:"id,omitempty"`
	coderef.Reference
}

// References returns the plain code references, for resolvers and renderers.
func References(evidence []Evidence) []coderef.Reference {
	out := make([]coderef.Reference, len(evidence))
	for i := range evidence {
		out[i] = evidence[i].Reference
	}
	return out
}

type Interaction struct {
	ID          string     `json:"id"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	Description string     `json:"description"`
	Intent      string     `json:"intent,omitempty"`
	Code        []Evidence `json:"code,omitempty"`
}

// Delivery names the source repository and full commit at which an
// implemented revision's evidence was validated. It is the assertion's source
// view, not a mutable application setting.
type Delivery struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

// Field is one author-selected field of a data entity; the list is curated,
// never an exhaustive schema mirror.
type Field struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Type string   `json:"type,omitempty"`
	Keys []string `json:"keys,omitempty"`
	Note string   `json:"note,omitempty"`
}

// Holder pins a Component (queue, store, ...) that carries or stores a data
// entity. It is a distinct resource, not a second identity for the entity.
type Holder struct {
	Component DocumentationLink `json:"component"`
	Role      string            `json:"role"`
}

// Cardinality is declared per association endpoint; "unknown" is explicit.
type Cardinality struct {
	Owner       string `json:"owner"`
	Destination string `json:"destination"`
}

// Relationship is an outgoing edge owned by a data-entity revision. Incoming
// relationships are derived by readers and never duplicated.
type Relationship struct {
	ID          string            `json:"id"`
	Meaning     string            `json:"meaning"`
	Destination DocumentationLink `json:"destination"`
	Label       string            `json:"label,omitempty"`
	Explanation string            `json:"explanation"`
	Cardinality *Cardinality      `json:"cardinality,omitempty"`
	Flow        string            `json:"flow,omitempty"`
	Intent      string            `json:"intent"`
	Code        []Evidence        `json:"code,omitempty"`
}

// Visual is an immutable, offline SVG asset stored in the record package.
type Visual struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type"`
	Digest    string `json:"digest"`
}

// RelationshipRef names one relationship by its owning entity revision.
type RelationshipRef struct {
	Owner DocumentationLink `json:"owner"`
	ID    string            `json:"id"`
}

// Removal proposes removing a baseline directory entity in an overlay. It
// does not retire the entity; that remains an explicit lifecycle event.
type Removal struct {
	Target      string `json:"target"`
	Explanation string `json:"explanation"`
}

// Binding attaches one SVG element to exactly one entity or relationship.
type Binding struct {
	ID           string             `json:"id"`
	Element      string             `json:"element"`
	Entity       *DocumentationLink `json:"entity,omitempty"`
	Relationship *RelationshipRef   `json:"relationship,omitempty"`
}

// TechnicalDefinition is the complete authored content of one revision. Kinds
// use disjoint subsets: Component/System (code, components, interactions),
// data-entity (fields, holders, relationships), ERD (visual, directory,
// bindings) and ERD overlay (erd, feature, pins, optional visual/bindings).
// Intent, Baseline and Delivery are inventory format 2 and absent on legacy
// revisions, which read as unspecified intent.
type TechnicalDefinition struct {
	Name          string              `json:"name"`
	Explanation   string              `json:"explanation"`
	Intent        string              `json:"intent,omitempty"`
	Baseline      string              `json:"baseline,omitempty"`
	Delivery      *Delivery           `json:"delivery,omitempty"`
	Code          []Evidence          `json:"code,omitempty"`
	Components    []DocumentationLink `json:"components,omitempty"`
	Interactions  []Interaction       `json:"interactions,omitempty"`
	Fields        []Field             `json:"fields,omitempty"`
	Holders       []Holder            `json:"holders,omitempty"`
	Relationships []Relationship      `json:"relationships,omitempty"`
	ERD           *DocumentationLink  `json:"erd,omitempty"`
	Feature       string              `json:"feature,omitempty"`
	Pins          []DocumentationLink `json:"pins,omitempty"`
	Removals      []Removal           `json:"removals,omitempty"`
	Visual        *Visual             `json:"visual,omitempty"`
	Directory     []DocumentationLink `json:"directory,omitempty"`
	Bindings      []Binding           `json:"bindings,omitempty"`
}

type TechnicalRevision struct {
	Schema  string   `json:"$schema"`
	Version int      `json:"version"`
	ID      string   `json:"id"`
	Record  string   `json:"record"`
	Parents []string `json:"parents"`
	TechnicalDefinition
	CreatedAt time.Time `json:"created_at"`
}

type TechnicalEvent struct {
	Schema    string    `json:"$schema"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Record    string    `json:"record"`
	Parents   []string  `json:"parents"`
	State     string    `json:"state"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type TechnicalRecord struct {
	Kind             string              `json:"kind"`
	Target           string              `json:"target"`
	Identity         RecordIdentity      `json:"identity"`
	Revisions        []TechnicalRevision `json:"revisions"`
	Events           []TechnicalEvent    `json:"events"`
	RevisionHeads    []string            `json:"revision_heads"`
	LifecycleHeads   []string            `json:"lifecycle_heads"`
	CurrentRevision  *TechnicalRevision  `json:"current_revision"`
	CurrentLifecycle *TechnicalEvent     `json:"current_lifecycle"`
}

type Inventory struct {
	SagaID string
	// Format is 1 when ___inventory/format.json is absent, else its format.
	Format  int
	Records []TechnicalRecord
}

// TechnicalKinds lists every inventory kind in load order, with its
// directory and the minimum inventory format that may contain it.
var TechnicalKinds = []struct {
	Kind, Dir string
	Format    int
}{
	{"component", "components", 1}, {"system", "systems", 1}, {KindDataEntity, "data-entities", 2}, {KindERD, "erds", 2}, {KindERDOverlay, "erd-overlays", 2},
}

const (
	KindDataEntity = "data-entity"
	KindERD        = "erd"
	KindERDOverlay = "erd-overlay"
)

func technicalKindDir(kind string) (string, bool) {
	for _, k := range TechnicalKinds {
		if k.Kind == kind {
			return k.Dir, true
		}
	}
	return "", false
}

func TechnicalSchema(kind, suffix string) string {
	return "https://changesaga.dev/schema/v5/" + kind + suffix + ".schema.json"
}
func TechnicalURN(sagaID, kind, id string) (string, error) {
	if _, ok := technicalKindDir(kind); !ok {
		return "", fmt.Errorf("kind must be component, system, data-entity, erd or erd-overlay")
	}
	return appRecordURN(sagaID, kind, id)
}
func TechnicalPath(kind, id string) string {
	dir, _ := technicalKindDir(kind)
	return InventoryDir + "/" + dir + "/" + id + "." + kind
}
func (d *Inventory) Find(target string) *TechnicalRecord {
	for i := range d.Records {
		if d.Records[i].Target == target {
			return &d.Records[i]
		}
	}
	return nil
}
func (r *TechnicalRecord) Revision(pin string) *TechnicalRevision {
	for i := range r.Revisions {
		if r.Target+":revision:"+r.Revisions[i].ID == pin {
			return &r.Revisions[i]
		}
	}
	return nil
}

// Pinned returns the exact pinned revision, never a successor.
func (d *Inventory) Pinned(link DocumentationLink) *TechnicalRevision {
	if r := d.Find(link.Target); r != nil {
		return r.Revision(link.Revision)
	}
	return nil
}

// EffectiveIntent reports the explicit intent, or unspecified for legacy
// revisions. It is never inferred from code, lifecycle or age.
func (v *TechnicalRevision) EffectiveIntent() string {
	if v.Intent == "" {
		return IntentUnspecified
	}
	return v.Intent
}

// EvidenceByID finds a stable evidence ID anywhere in the revision. The
// returned owner is "" for the entity's own code, else the interaction or
// relationship ID that owns the reference.
func (v *TechnicalRevision) EvidenceByID(id string) (*Evidence, string) {
	if id == "" {
		return nil, ""
	}
	for i := range v.Code {
		if v.Code[i].ID == id {
			return &v.Code[i], ""
		}
	}
	for i := range v.Interactions {
		for j := range v.Interactions[i].Code {
			if v.Interactions[i].Code[j].ID == id {
				return &v.Interactions[i].Code[j], v.Interactions[i].ID
			}
		}
	}
	for i := range v.Relationships {
		for j := range v.Relationships[i].Code {
			if v.Relationships[i].Code[j].ID == id {
				return &v.Relationships[i].Code[j], v.Relationships[i].ID
			}
		}
	}
	return nil, ""
}

// LinkStatus never picks a winner, repins a reference, or grants coverage.
func (d *Inventory) LinkStatus(link DocumentationLink) string {
	r := d.Find(link.Target)
	if r == nil || r.Revision(link.Revision) == nil {
		return "missing"
	}
	if r.CurrentRevision == nil || r.CurrentLifecycle == nil {
		return "conflicted"
	}
	if r.CurrentLifecycle.State == "retired" {
		return "retired"
	}
	if r.Target+":revision:"+r.CurrentRevision.ID != link.Revision {
		return "stale"
	}
	return "current"
}

func LoadInventory(root, sagaID string) (Inventory, error) {
	d := Inventory{SagaID: sagaID, Format: 1, Records: []TechnicalRecord{}}
	dir := filepath.Join(root, InventoryDir)
	if present, err := realDirectory(dir); err != nil {
		return d, err
	} else if present {
		entries, err := boundedReadDir(dir, len(TechnicalKinds)+1)
		if err != nil {
			return d, err
		}
		for _, e := range entries {
			if e.Name() == InventoryFormatName && e.Type().IsRegular() {
				continue
			}
			if _, ok := kindForDir(e.Name()); !ok || !e.IsDir() || e.Type()&fs.ModeSymlink != 0 {
				return d, fmt.Errorf("%s: unknown inventory entry %s", InventoryDir, e.Name())
			}
		}
		format, err := readInventoryFormat(root)
		if err != nil {
			return d, err
		}
		d.Format = format
	}
	for _, k := range TechnicalKinds {
		extra := []string{}
		if k.Kind == KindERD || k.Kind == KindERDOverlay {
			extra = append(extra, "assets")
		}
		packages, err := loadAppPackages(root, filepath.Join(dir, k.Dir), "."+k.Kind, k.Kind+".json", TechnicalSchema(k.Kind, ""), MaxInventoryRecords, extra...)
		if err != nil {
			return d, err
		}
		if len(packages) > 0 && d.Format < k.Format {
			return d, fmt.Errorf("%s: %s records require inventory format %d; run inventory adopt-format explicitly", InventoryDir, k.Kind, k.Format)
		}
		for _, p := range packages {
			urn, _ := TechnicalURN(sagaID, k.Kind, p.id)
			r := TechnicalRecord{Kind: k.Kind, Target: urn, Identity: p.identity}
			if err := readInventoryJSON(filepath.Join(p.dir, k.Kind+".json"), &r.Identity); err != nil {
				return d, err
			}
			for _, path := range p.revisions {
				var v TechnicalRevision
				if err := readInventoryJSON(path, &v); err != nil {
					return d, err
				}
				if err := validateTechnicalRevision(v, k.Kind, urn, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
					return d, fmt.Errorf("%s: %w", path, err)
				}
				if v.Intent != "" && d.Format < 2 {
					return d, fmt.Errorf("%s: revision intent requires inventory format 2; run inventory adopt-format explicitly", path)
				}
				if v.Visual != nil {
					if err := checkVisualAsset(p.dir, *v.Visual, v.Bindings); err != nil {
						return d, fmt.Errorf("%s: %w", path, err)
					}
				}
				r.Revisions = append(r.Revisions, v)
			}
			for _, path := range p.events {
				var v TechnicalEvent
				if err := readInventoryJSON(path, &v); err != nil {
					return d, err
				}
				if err := validateTechnicalEvent(v, k.Kind, urn, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
					return d, fmt.Errorf("%s: %w", path, err)
				}
				r.Events = append(r.Events, v)
			}
			if err := resolveTechnical(&r, sagaID); err != nil {
				return d, err
			}
			d.Records = append(d.Records, r)
			if len(d.Records) > MaxInventoryRecords {
				return d, fmt.Errorf("inventory record limit reached")
			}
		}
	}
	return d, nil
}

func kindForDir(name string) (string, bool) {
	for _, k := range TechnicalKinds {
		if k.Dir == name {
			return k.Kind, true
		}
	}
	return "", false
}

func validateTechnicalEvent(v TechnicalEvent, kind, urn, id string) error {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(encoded)+1 > MaxRecordBytes {
		return fmt.Errorf("event exceeds one MiB")
	}

	if v.Schema != TechnicalSchema(kind, "-event") || v.Version != 5 || !livingid.ValidID(v.ID) || v.ID != id || v.Record != urn || v.Parents == nil || v.CreatedAt.IsZero() || (v.State != "active" && v.State != "retired") {
		return fmt.Errorf("invalid %s lifecycle event", kind)
	}
	if len(v.Parents) == 0 && v.State != "active" {
		return fmt.Errorf("initial lifecycle must be active")
	}
	if len(v.Parents) > 0 && strings.TrimSpace(v.Reason) == "" {
		return fmt.Errorf("lifecycle change requires a reason")
	}
	return nil
}
func resolveTechnical(r *TechnicalRecord, sagaID string) error {
	var problems validationErrors
	revs := []graphMember{}
	events := []graphMember{}
	for _, v := range r.Revisions {
		revs = append(revs, graphMember{v.ID, v.Parents})
	}
	for _, v := range r.Events {
		events = append(events, graphMember{v.ID, v.Parents})
	}
	heads, parentsOf := resolveAppGraph(&problems, "revision", sagaID, r.Kind, r.Identity.ID, "revision", revs)
	r.RevisionHeads = memberURNs(sagaID, r.Kind, r.Identity.ID, "revision", heads)
	r.CurrentRevision = nil
	if len(heads) == 1 {
		r.CurrentRevision = r.Revision(r.RevisionHeads[0])
	}
	// A proposed revision's implemented baseline is an explicit ancestor pin of
	// the same identity, retained separately from its parents.
	for _, v := range r.Revisions {
		if v.Baseline == "" || v.Baseline == BaselineNone {
			continue
		}
		base := r.Revision(v.Baseline)
		if base == nil || base.Intent != IntentImplemented {
			problems.add("revision %q baseline must pin an implemented revision of the same record", v.ID)
			continue
		}
		if !technicalAncestor(parentsOf, r.Target+":revision:", v.ID, base.ID) {
			problems.add("revision %q baseline %s is not one of its ancestors", v.ID, v.Baseline)
		}
	}
	heads, _ = resolveAppGraph(&problems, "lifecycle", sagaID, r.Kind, r.Identity.ID, "event", events)
	eventStates := map[string]string{}
	for _, v := range r.Events {
		eventStates[r.Target+":event:"+v.ID] = v.State
	}
	for _, v := range r.Events {
		if len(v.Parents) == 1 && eventStates[v.Parents[0]] == v.State {
			problems.add("lifecycle event %q repeats state %s", v.ID, v.State)
		}
	}

	r.LifecycleHeads = memberURNs(sagaID, r.Kind, r.Identity.ID, "event", heads)
	r.CurrentLifecycle = nil
	if len(heads) == 1 {
		for i := range r.Events {
			if r.Events[i].ID == heads[0] {
				r.CurrentLifecycle = &r.Events[i]
			}
		}
	}
	return problems.err()
}

// technicalAncestor walks declared parents (bounded by the record's revisions).
func technicalAncestor(parentsOf map[string][]string, prefix, from, target string) bool {
	seen := map[string]bool{}
	queue := append([]string{}, parentsOf[from]...)
	for len(queue) > 0 {
		next := strings.TrimPrefix(queue[0], prefix)
		queue = queue[1:]
		if next == target {
			return true
		}
		if seen[next] {
			continue
		}
		seen[next] = true
		queue = append(queue, parentsOf[next]...)
	}
	return false
}
