package requirements

import (
	"encoding/json"
	"fmt"
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

type Interaction struct {
	ID          string              `json:"id"`
	From        string              `json:"from"`
	To          string              `json:"to"`
	Description string              `json:"description"`
	Code        []coderef.Reference `json:"code"`
}

type TechnicalDefinition struct {
	Name         string              `json:"name"`
	Explanation  string              `json:"explanation"`
	Code         []coderef.Reference `json:"code"`
	Components   []DocumentationLink `json:"components,omitempty"`
	Interactions []Interaction       `json:"interactions,omitempty"`
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
	SagaID  string
	Records []TechnicalRecord
}

func TechnicalSchema(kind, suffix string) string {
	return "https://changesaga.dev/schema/v5/" + kind + suffix + ".schema.json"
}
func TechnicalURN(sagaID, kind, id string) (string, error) {
	if kind != "component" && kind != "system" {
		return "", fmt.Errorf("kind must be component or system")
	}
	return appRecordURN(sagaID, kind, id)
}
func TechnicalPath(kind, id string) string { return InventoryDir + "/" + kind + "s/" + id + "." + kind }
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
	d := Inventory{SagaID: sagaID, Records: []TechnicalRecord{}}
	dir := filepath.Join(root, InventoryDir)
	if present, err := realDirectory(dir); err != nil {
		return d, err
	} else if present {
		entries, err := boundedReadDir(dir, 2)
		if err != nil {
			return d, err
		}
		for _, e := range entries {
			if (e.Name() != "components" && e.Name() != "systems") || !e.IsDir() {
				return d, fmt.Errorf("%s: unknown inventory entry %s", InventoryDir, e.Name())
			}
		}
	}
	for _, kind := range []string{"component", "system"} {
		packages, err := loadAppPackages(root, filepath.Join(dir, kind+"s"), "."+kind, kind+".json", TechnicalSchema(kind, ""), MaxInventoryRecords)
		if err != nil {
			return d, err
		}
		for _, p := range packages {
			urn, _ := TechnicalURN(sagaID, kind, p.id)
			r := TechnicalRecord{Kind: kind, Target: urn, Identity: p.identity}
			if err := readInventoryJSON(filepath.Join(p.dir, kind+".json"), &r.Identity); err != nil {
				return d, err
			}
			for _, path := range p.revisions {
				var v TechnicalRevision
				if err := readInventoryJSON(path, &v); err != nil {
					return d, err
				}
				if err := validateTechnicalRevision(v, kind, urn, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
					return d, fmt.Errorf("%s: %w", path, err)
				}
				r.Revisions = append(r.Revisions, v)
			}
			for _, path := range p.events {
				var v TechnicalEvent
				if err := readInventoryJSON(path, &v); err != nil {
					return d, err
				}
				if err := validateTechnicalEvent(v, kind, urn, strings.TrimSuffix(filepath.Base(path), ".json")); err != nil {
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

func exactTechnicalCode(code []coderef.Reference) error {
	if len(code) == 0 || len(code) > 64 {
		return fmt.Errorf("requires 1 to 64 exact code references")
	}
	seen := map[string]bool{}
	for _, ref := range code {
		if err := coderef.Validate(ref); err != nil {
			return err
		}
		if ref.Start == 0 || strings.TrimSpace(ref.Note) == "" {
			return fmt.Errorf("code requires exact line ranges and a meaningful note")
		}
		key := ref.Location().String()
		if seen[key] {
			return fmt.Errorf("duplicate code reference %s", key)
		}
		seen[key] = true
	}
	return nil
}
func validateTechnicalRevision(v TechnicalRevision, kind, urn, id string) error {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(encoded)+1 > MaxRecordBytes {
		return fmt.Errorf("revision exceeds one MiB")
	}

	if v.Schema != TechnicalSchema(kind, "-revision") || v.Version != 5 || !livingid.ValidID(v.ID) || v.ID != id || v.Record != urn || v.Parents == nil || v.CreatedAt.IsZero() {
		return fmt.Errorf("invalid %s revision identity, parents, schema, version or time", kind)
	}
	if strings.TrimSpace(v.Name) == "" || v.Name != strings.TrimSpace(v.Name) || len(v.Name) > 200 || strings.TrimSpace(v.Explanation) == "" {
		return fmt.Errorf("name (up to 200 bytes) and meaningful explanation are required")
	}
	if err := exactTechnicalCode(v.Code); err != nil {
		return err
	}
	if kind == "component" {
		if len(v.Components)+len(v.Interactions) > 0 {
			return fmt.Errorf("components cannot contain a system graph")
		}
		return nil
	}
	if len(v.Components) < 2 || len(v.Components) > 32 || len(v.Interactions) == 0 || len(v.Interactions) > 64 {
		return fmt.Errorf("a System requires 2 to 32 Components and 1 to 64 interactions")
	}
	members := map[string]bool{}
	sagaID := strings.Split(urn, ":")[2]
	for _, pin := range v.Components {
		if !saga.ValidDocumentationLink(sagaID, pin) || !strings.Contains(pin.Target, ":component:") {
			return fmt.Errorf("system members must pin Component revisions")
		}
		if members[pin.Target] {
			return fmt.Errorf("duplicate Component %s", pin.Target)
		}
		members[pin.Target] = true
	}
	edges := map[string]bool{}
	for _, edge := range v.Interactions {
		if !livingid.ValidID(edge.ID) || edges[edge.ID] || !members[edge.From] || !members[edge.To] || strings.TrimSpace(edge.Description) == "" {
			return fmt.Errorf("interaction requires unique ID, member endpoints and a description")
		}
		edges[edge.ID] = true
		if err := exactTechnicalCode(edge.Code); err != nil {
			return fmt.Errorf("interaction %s: %w", edge.ID, err)
		}
	}
	return nil
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
	heads, _ := resolveAppGraph(&problems, "revision", sagaID, r.Kind, r.Identity.ID, "revision", revs)
	r.RevisionHeads = memberURNs(sagaID, r.Kind, r.Identity.ID, "revision", heads)
	r.CurrentRevision = nil
	if len(heads) == 1 {
		r.CurrentRevision = r.Revision(r.RevisionHeads[0])
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
