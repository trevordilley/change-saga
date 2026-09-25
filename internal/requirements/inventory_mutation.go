package requirements

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/store"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// TechnicalWrite is one complete revision publication. Repository and
// Resolver are required for every revision with explicit intent: the resolver
// must be bound to the Saga's canonical source repository, and Repository is
// that repository's declared identity.
type TechnicalWrite struct {
	Kind, ID, RevisionID string
	Parents              []string
	Definition           TechnicalDefinition
	Create               bool
	Repository           string
	Resolver             technicalpolicy.EvidenceResolver
	// Visual holds the SVG bytes named by Definition.Visual (ERD kinds only).
	Visual []byte
}

// ErrFormatRequired reports content that needs deliberate format adoption.
var ErrFormatRequired = errors.New("requires inventory format 2; adopt it explicitly with `change-saga inventory adopt-format --format 2`")

// PolicyError carries technicalpolicy diagnostics for a refused candidate.
type PolicyError struct{ Diagnostics []technicalpolicy.Diagnostic }

func (e *PolicyError) Error() string {
	parts := make([]string, 0, len(e.Diagnostics))
	for _, d := range e.Diagnostics {
		where := "revision"
		if d.EdgeID != "" || d.EdgeIndex >= 0 {
			where = "edge " + d.EdgeID
		}
		if d.ReferenceIndex >= 0 {
			where += fmt.Sprintf(" reference %d", d.ReferenceIndex)
		}
		parts = append(parts, string(d.Code)+" ("+where+")")
	}
	return "implementation policy refused the revision: " + strings.Join(parts, "; ")
}

// AdoptInventoryFormat writes the format-2 marker. It never rewrites
// existing records; legacy revisions keep reading as unspecified intent.
func AdoptInventoryFormat(root, sagaID string, format int) (MutationResult, error) {
	if format != CurrentInventoryFormat {
		return MutationResult{}, fmt.Errorf("only inventory format %d can be adopted", CurrentInventoryFormat)
	}
	result := MutationResult{URN: "urn:change-saga:" + sagaID + ":inventory-format:" + fmt.Sprint(format), Path: InventoryDir + "/" + InventoryFormatName}
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		d, err := LoadInventory(root, sagaID)
		if err != nil {
			return err
		}
		if d.Format == format {
			result.Replayed = true
			return nil
		}
		if _, err := store.EnsureDirWithin(root, filepath.Join(root, InventoryDir)); err != nil {
			return err
		}
		marker := InventoryFormat{Schema: TechnicalSchema("inventory-format", ""), Format: format, CreatedAt: mutationTime(time.Time{})}
		if err := store.WriteJSON(filepath.Join(root, InventoryDir, InventoryFormatName), marker, true); err != nil {
			return err
		}
		result.Created = []string{result.Path}
		return nil
	})
	return result, err
}

// WriteTechnical publishes a legacy (format 1) Component/System revision.
func WriteTechnical(root, sagaID, kind, id, revisionID string, parents []string, definition TechnicalDefinition, create bool) (MutationResult, error) {
	return WriteTechnicalRevision(context.Background(), root, sagaID, TechnicalWrite{Kind: kind, ID: id, RevisionID: revisionID, Parents: parents, Definition: definition, Create: create})
}

// WriteTechnicalRevision publishes an identity + initial revision/event
// atomically, or appends one full revision. Every parent must be an observed
// current head. All validation, including implementation policy against the
// delivery commit, runs under the Saga lock before any file is written.
func WriteTechnicalRevision(ctx context.Context, root, sagaID string, w TechnicalWrite) (MutationResult, error) {
	urn, err := TechnicalURN(sagaID, w.Kind, w.ID)
	if err != nil {
		return MutationResult{}, err
	}
	now := mutationTime(time.Time{})
	v := TechnicalRevision{Schema: TechnicalSchema(w.Kind, "-revision"), Version: 5, ID: w.RevisionID, Record: urn, Parents: copyStrings(w.Parents), TechnicalDefinition: w.Definition, CreatedAt: now}
	if err := validateTechnicalRevision(v, w.Kind, urn, w.RevisionID); err != nil {
		return MutationResult{}, err
	}
	if (v.Visual == nil) != (len(w.Visual) == 0) {
		return MutationResult{}, fmt.Errorf("visual bytes must accompany exactly the declared visual")
	}
	if v.Visual != nil {
		if err := ValidateVisual(w.Visual, v.Visual.Digest, v.Bindings); err != nil {
			return MutationResult{}, err
		}
	}
	result := MutationResult{URN: urn, Path: TechnicalPath(w.Kind, w.ID)}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		d, err := LoadInventory(root, sagaID)
		if err != nil {
			return err
		}
		if err := formatAdmits(d.Format, w.Kind, v); err != nil {
			return err
		}
		found := d.Find(urn)
		if found != nil {
			if old := found.Revision(urn + ":revision:" + w.RevisionID); old != nil {
				v.CreatedAt = old.CreatedAt
				if reflect.DeepEqual(*old, v) && (w.Create == (len(old.Parents) == 0)) {
					result.Replayed = true
					result.CurrentHeads = found.RevisionHeads
					return nil
				}
				return fmt.Errorf("revision %s already exists with different content", w.RevisionID)
			}
		}
		if w.Create {
			if found != nil {
				return fmt.Errorf("%s already exists", urn)
			}
			if len(w.Parents) != 0 {
				return fmt.Errorf("initial revision cannot have parents")
			}
			if len(d.Records) >= MaxInventoryRecords {
				return fmt.Errorf("inventory record limit reached")
			}
		} else {
			if found == nil {
				return fmt.Errorf("%s does not exist", urn)
			}
			if !sameSet(w.Parents, found.RevisionHeads) {
				return fmt.Errorf("parents must name every current revision head: %v", found.RevisionHeads)
			}
			if len(found.Revisions) >= MaxRevisionsPerStory {
				return fmt.Errorf("revision limit reached")
			}
		}
		// The candidate must resolve in its record's history (baseline ancestry).
		candidate := TechnicalRecord{Kind: w.Kind, Target: urn, Identity: RecordIdentity{ID: w.ID}}
		if found != nil {
			candidate = *found
			candidate.Revisions = append(append([]TechnicalRevision{}, found.Revisions...), v)
		} else {
			candidate.Revisions = []TechnicalRevision{v}
			candidate.Events = []TechnicalEvent{{ID: "active", Parents: []string{}, State: "active"}}
		}
		if err := resolveTechnical(&candidate, sagaID); err != nil {
			return err
		}
		self := DocumentationLink{Target: urn, Revision: urn + ":revision:" + w.RevisionID}
		if err := admitDefinitionPins(&d, found, self, v); err != nil {
			return err
		}
		if err := checkViewReferences(&d, v); err != nil {
			return err
		}
		if v.Intent != "" {
			if err := checkImplementationPolicy(ctx, &d, self, v, w); err != nil {
				return err
			}
		}
		path := filepath.Join(root, filepath.FromSlash(result.Path))
		if w.Create {
			identity := RecordIdentity{Schema: TechnicalSchema(w.Kind, ""), Version: 5, ID: w.ID, CreatedAt: now}
			event := TechnicalEvent{Schema: TechnicalSchema(w.Kind, "-event"), Version: 5, ID: "active", Record: urn, Parents: []string{}, State: "active", CreatedAt: now}
			if err := createTechnicalPackage(root, path, w.Kind, identity, v, event, w.Visual); err != nil {
				return err
			}
		} else {
			created, err := writeVisualAsset(root, path, v.Visual, w.Visual)
			if err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(path, "revisions", w.RevisionID+".json"), v, true); err != nil {
				if created != "" {
					_ = os.Remove(created)
				}
				return err
			}
		}
		result.Created = []string{self.Revision}
		result.CurrentHeads = result.Created
		return nil
	})
	return result, err
}

// formatAdmits enforces deliberate adoption. After adoption every new
// Component/System revision states its intent; legacy shape remains readable
// but is no longer authored, so a new revision is never silently unspecified.
func formatAdmits(format int, kind string, v TechnicalRevision) error {
	needsTwo := v.Intent != "" || kind == KindDataEntity || kind == KindERD || kind == KindERDOverlay
	if needsTwo && format < 2 {
		return ErrFormatRequired
	}
	if format >= 2 && v.Intent == "" && (kind == "component" || kind == "system") {
		return fmt.Errorf("inventory format 2 requires explicit intent (%s or %s) on every new %s revision", IntentProposed, IntentImplemented, kind)
	}
	return nil
}

// definitionPins lists every exact pin a revision declares.
func definitionPins(v *TechnicalRevision) []DocumentationLink {
	pins := append([]DocumentationLink{}, v.Components...)
	for _, h := range v.Holders {
		pins = append(pins, h.Component)
	}
	for _, r := range v.Relationships {
		pins = append(pins, r.Destination)
	}
	pins = append(pins, v.Directory...)
	pins = append(pins, v.Pins...)
	if v.ERD != nil {
		pins = append(pins, *v.ERD)
	}
	return pins
}

// admitDefinitionPins applies today's new-link rule to every declared pin: it
// must be the current revision of an active, unconflicted record, unless the
// identical pin is retained from the record's current revision or names the
// candidate revision itself (a self relationship).
func admitDefinitionPins(d *Inventory, found *TechnicalRecord, self DocumentationLink, v TechnicalRevision) error {
	retained := map[DocumentationLink]bool{}
	if found != nil && found.CurrentRevision != nil {
		for _, pin := range definitionPins(found.CurrentRevision) {
			retained[pin] = true
		}
	}
	for _, pin := range definitionPins(&v) {
		if retained[pin] || pin == self {
			continue
		}
		if status := d.LinkStatus(pin); status != "current" {
			return fmt.Errorf("pinned %s is %s; new pins must be the current revision of an active record", pin.Revision, status)
		}
	}
	return nil
}

// checkViewReferences validates ERD bindings and overlays against the exact
// saved revisions they name.
func checkViewReferences(d *Inventory, v TechnicalRevision) error {
	directory := v.Directory
	if v.ERD != nil {
		base := d.Pinned(*v.ERD)
		if base == nil {
			return fmt.Errorf("overlay baseline %s is missing", v.ERD.Revision)
		}
		baseline := map[string]DocumentationLink{}
		for _, pin := range base.Directory {
			baseline[pin.Target] = pin
		}
		for _, removal := range v.Removals {
			if _, ok := baseline[removal.Target]; !ok {
				return fmt.Errorf("removal %s is not in the baseline ERD directory", removal.Target)
			}
		}
		for _, pin := range v.Pins {
			if baseline[pin.Target] == pin {
				return fmt.Errorf("overlay pin %s repeats the baseline; pin a different revision or omit it", pin.Revision)
			}
		}
		composed, err := d.ComposeOverlay(&v)
		if err != nil {
			return err
		}
		directory = composed
	}
	members := map[DocumentationLink]bool{}
	for _, pin := range directory {
		members[pin] = true
	}
	for _, b := range v.Bindings {
		if b.Entity != nil && !members[*b.Entity] {
			return fmt.Errorf("binding %s names %s outside the directory", b.ID, b.Entity.Revision)
		}
		if b.Relationship != nil {
			owner := d.Pinned(b.Relationship.Owner)
			if !members[b.Relationship.Owner] || owner == nil {
				return fmt.Errorf("binding %s relationship owner is not in the directory", b.ID)
			}
			found := false
			for _, edge := range owner.Relationships {
				found = found || edge.ID == b.Relationship.ID
			}
			if !found {
				return fmt.Errorf("binding %s names missing relationship %s of %s", b.ID, b.Relationship.ID, b.Relationship.Owner.Revision)
			}
		}
	}
	return nil
}

// checkImplementationPolicy evaluates explicit intent through technicalpolicy:
// original bytes and delivery currency for every implemented owner/edge, and
// exact endpoint intent without cascading promotion.
func checkImplementationPolicy(ctx context.Context, d *Inventory, self DocumentationLink, v TechnicalRevision, w TechnicalWrite) error {
	if w.Resolver == nil || strings.TrimSpace(w.Repository) == "" {
		return fmt.Errorf("explicit intent requires a source resolver bound to the Saga's canonical repository")
	}
	if v.Delivery != nil && v.Delivery.Repository != w.Repository {
		return fmt.Errorf("delivery repository %q is not the Saga's canonical source %q", v.Delivery.Repository, w.Repository)
	}
	endpoint := func(pin DocumentationLink) technicalpolicy.Endpoint {
		e := technicalpolicy.Endpoint{Pin: technicalpolicy.Pin{Target: pin.Target, Revision: pin.Revision}}
		if rev := d.Pinned(pin); rev != nil {
			e.Resolved, e.Intent = true, technicalpolicy.Intent(rev.EffectiveIntent())
		}
		return e
	}
	candidate := technicalpolicy.Candidate{Pin: technicalpolicy.Pin{Target: self.Target, Revision: self.Revision}, Intent: technicalpolicy.Intent(v.Intent), Evidence: References(v.Code)}
	if v.Delivery != nil {
		candidate.DeliveryOID = v.Delivery.Commit
	}
	var extra []technicalpolicy.Diagnostic
	members := map[string]DocumentationLink{}
	for _, pin := range v.Components {
		members[pin.Target] = pin
	}
	for index, edge := range v.Interactions {
		candidate.Edges = append(candidate.Edges, technicalpolicy.Edge{ID: edge.ID, Intent: technicalpolicy.Intent(edge.Intent), Destination: endpoint(members[edge.To]), Evidence: References(edge.Code)})
		// An interaction has two member endpoints; the policy checks the
		// destination, so the source member is checked with the same rule.
		if edge.Intent == IntentImplemented {
			if from := endpoint(members[edge.From]); !from.Resolved || from.Intent != technicalpolicy.Implemented {
				extra = append(extra, technicalpolicy.Diagnostic{Owner: candidate.Pin, EdgeID: edge.ID, EdgeIndex: index, ReferenceIndex: -1, Code: technicalpolicy.EndpointNotImplemented})
			}
		}
	}
	for _, edge := range v.Relationships {
		destination := endpoint(edge.Destination)
		if edge.Destination == self {
			destination = technicalpolicy.Endpoint{Pin: candidate.Pin, Resolved: true, Intent: candidate.Intent}
		}
		candidate.Edges = append(candidate.Edges, technicalpolicy.Edge{ID: edge.ID, Intent: technicalpolicy.Intent(edge.Intent), Destination: destination, Evidence: References(edge.Code)})
	}
	outcome := technicalpolicy.ValidateCandidate(ctx, candidate, w.Resolver)
	if diagnostics := append(outcome.Diagnostics, extra...); len(diagnostics) > 0 {
		return &PolicyError{Diagnostics: diagnostics}
	}
	return nil
}

func createTechnicalPackage(root, final, kind string, identity RecordIdentity, revision TechnicalRevision, event TechnicalEvent, visual []byte) error {
	return store.CommitDir(root, final, func(stage string) error {
		if err := os.Chmod(stage, 0o755); err != nil {
			return err
		}
		if err := store.WriteJSON(filepath.Join(stage, kind+".json"), identity, true); err != nil {
			return err
		}
		for _, dir := range []string{"revisions", "events"} {
			if err := ensureStageDir(filepath.Join(stage, dir)); err != nil {
				return err
			}
		}
		if revision.Visual != nil {
			if err := ensureStageDir(filepath.Join(stage, "assets")); err != nil {
				return err
			}
			if err := store.WriteFile(filepath.Join(stage, filepath.FromSlash(revision.Visual.Path)), visual, 0o644, true); err != nil {
				return err
			}
		}
		if err := store.WriteJSON(filepath.Join(stage, "revisions", revision.ID+".json"), revision, true); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(stage, "events", event.ID+".json"), event, true)
	})
}

// writeVisualAsset stores a content-addressed SVG. An existing asset must be
// byte-identical. It returns the path it created (for rollback), or "".
func writeVisualAsset(root, packageDir string, visual *Visual, data []byte) (string, error) {
	if visual == nil {
		return "", nil
	}
	sum := sha256.Sum256(data)
	if visual.Digest != coderef.DigestPrefix+hex.EncodeToString(sum[:]) {
		return "", fmt.Errorf("visual digest does not match its bytes")
	}
	path := filepath.Join(packageDir, filepath.FromSlash(visual.Path))
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, data) {
			return "", fmt.Errorf("visual asset %s already exists with different bytes", visual.Path)
		}
		return "", nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if _, err := store.EnsureDirWithin(root, filepath.Dir(path)); err != nil {
		return "", err
	}
	if err := store.WriteFile(path, data, 0o644, true); err != nil {
		return "", err
	}
	return path, nil
}

func SetTechnicalState(root, sagaID, kind, id, eventID, state, reason string, parents []string) (MutationResult, error) {
	urn, err := TechnicalURN(sagaID, kind, id)
	if err != nil {
		return MutationResult{}, err
	}
	v := TechnicalEvent{Schema: TechnicalSchema(kind, "-event"), Version: 5, ID: eventID, Record: urn, Parents: copyStrings(parents), State: state, Reason: reason, CreatedAt: mutationTime(time.Time{})}
	if err := validateTechnicalEvent(v, kind, urn, eventID); err != nil {
		return MutationResult{}, err
	}
	result := MutationResult{URN: urn, Path: TechnicalPath(kind, id)}
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		d, err := LoadInventory(root, sagaID)
		if err != nil {
			return err
		}
		found := d.Find(urn)
		if found == nil {
			return fmt.Errorf("%s does not exist", urn)
		}
		for _, old := range found.Events {
			if old.ID == eventID {
				v.CreatedAt = old.CreatedAt
				if reflect.DeepEqual(old, v) {
					result.Replayed = true
					result.CurrentHeads = found.LifecycleHeads
					return nil
				}
				return fmt.Errorf("event already exists with different content")
			}
		}
		if !sameSet(parents, found.LifecycleHeads) {
			return fmt.Errorf("parents must name every current lifecycle head: %v", found.LifecycleHeads)
		}
		if found.CurrentLifecycle != nil && found.CurrentLifecycle.State == state {
			return fmt.Errorf("lifecycle state is already %s", state)
		}
		if len(found.Events) >= MaxRevisionsPerStory {
			return fmt.Errorf("lifecycle limit reached")
		}
		if err := store.WriteJSON(filepath.Join(root, filepath.FromSlash(result.Path), "events", eventID+".json"), v, true); err != nil {
			return err
		}
		result.Created = []string{urn + ":event:" + eventID}
		result.CurrentHeads = result.Created
		return nil
	})
	return result, err
}
