package requirements

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

const (
	IntentProposed    = string(technicalpolicy.Proposed)
	IntentImplemented = string(technicalpolicy.Implemented)
	IntentUnspecified = string(technicalpolicy.Unspecified)
	// BaselineNone explicitly states a proposal has no implemented baseline.
	BaselineNone = "none"

	// InventoryFormatName marks deliberate adoption of inventory format 2.
	InventoryFormatName    = "format.json"
	CurrentInventoryFormat = 2

	MaxEvidencePerOwner = technicalpolicy.MaxReferences
	MaxRelationships    = technicalpolicy.MaxEdges
	MaxEntityFields     = 64
	MaxEntityHolders    = 16
	MaxERDDirectory     = 512
	MaxERDBindings      = 512
	MaxOverlayPins      = 256
	MaxVisualBytes      = 1 << 20
	MaxERDLabelBytes    = 80
)

var (
	fieldKeys       = map[string]bool{"primary": true, "foreign": true, "unique": true, "partition": true}
	cardinalities   = map[string]bool{"unknown": true, "0..1": true, "1": true, "0..many": true, "1..many": true}
	productionFlows = map[string]bool{"owner_to_destination": true, "destination_to_owner": true}
	svgElementID    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]{0,127}$`)
	visualAssetPath = regexp.MustCompile(`^assets/([0-9a-f]{64})\.svg$`)
)

// InventoryFormat is the ___inventory/format.json marker. Its absence means
// format 1 (Components and Systems without intent). Older readers reject the
// marker as an unknown inventory entry rather than discard format-2 content.
type InventoryFormat struct {
	Schema    string    `json:"$schema"`
	Format    int       `json:"format"`
	CreatedAt time.Time `json:"created_at"`
}

func readInventoryFormat(root string) (int, error) {
	path := filepath.Join(root, InventoryDir, InventoryFormatName)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%s/%s must be a regular file", InventoryDir, InventoryFormatName)
	}
	var marker InventoryFormat
	if err := readInventoryJSON(path, &marker); err != nil {
		return 0, fmt.Errorf("%s/%s: %w", InventoryDir, InventoryFormatName, err)
	}
	if marker.Schema != TechnicalSchema("inventory-format", "") || marker.CreatedAt.IsZero() {
		return 0, fmt.Errorf("%s/%s: invalid schema or time", InventoryDir, InventoryFormatName)
	}
	if marker.Format != CurrentInventoryFormat {
		return 0, fmt.Errorf("%s/%s: unsupported inventory format %d; this change-saga reads formats 1 and %d", InventoryDir, InventoryFormatName, marker.Format, CurrentInventoryFormat)
	}
	return marker.Format, nil
}

func validPin(sagaID string, link DocumentationLink, kinds ...string) bool {
	return saga.ValidPinOfKinds(sagaID, link, kinds...)
}

// evidenceList validates one owner's references. IDs are required (and
// recorded in ids, unique across the revision) exactly when withIDs is set.
func evidenceList(refs []Evidence, minimum int, withIDs bool, ids map[string]bool) error {
	if len(refs) < minimum || len(refs) > MaxEvidencePerOwner {
		return fmt.Errorf("requires %d to %d exact code references", minimum, MaxEvidencePerOwner)
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if err := coderef.Validate(ref.Reference); err != nil {
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
		switch {
		case !withIDs && ref.ID != "":
			return fmt.Errorf("evidence IDs require explicit revision intent (inventory format 2)")
		case withIDs && !livingid.ValidID(ref.ID):
			return fmt.Errorf("evidence %s requires a stable id", key)
		case withIDs && ids[ref.ID]:
			return fmt.Errorf("evidence id %q is not unique in the revision", ref.ID)
		}
		if withIDs {
			ids[ref.ID] = true
		}
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
	sagaID := strings.Split(urn, ":")[2]
	entity := len(v.Fields)+len(v.Holders)+len(v.Relationships) > 0
	view := v.ERD != nil || v.Feature != "" || len(v.Pins)+len(v.Removals)+len(v.Directory)+len(v.Bindings) > 0 || v.Visual != nil
	graph := len(v.Components)+len(v.Interactions) > 0
	switch kind {
	case "component", "system":
		if entity || view {
			return fmt.Errorf("a %s cannot contain data-entity or ERD content", kind)
		}
		if v.Intent == "" {
			return validateLegacyTechnical(v, kind, sagaID)
		}
	case KindDataEntity:
		if graph || view {
			return fmt.Errorf("a data entity cannot contain a System graph or ERD content")
		}
		if v.Intent == "" {
			return fmt.Errorf("a data entity requires explicit proposed or implemented intent")
		}
	case KindERD, KindERDOverlay:
		return validateERDRevision(v, kind, sagaID)
	default:
		return fmt.Errorf("unknown inventory kind %s", kind)
	}
	return validateIntentRevision(v, kind, urn, sagaID)
}

// validateLegacyTechnical keeps the original format-1 contract unchanged.
func validateLegacyTechnical(v TechnicalRevision, kind, sagaID string) error {
	if v.Baseline != "" || v.Delivery != nil {
		return fmt.Errorf("baseline and delivery require explicit revision intent")
	}
	if err := evidenceList(v.Code, 1, false, nil); err != nil {
		return err
	}
	if kind == "component" {
		if len(v.Components)+len(v.Interactions) > 0 {
			return fmt.Errorf("components cannot contain a system graph")
		}
		return nil
	}
	members, err := systemMembers(v, sagaID)
	if err != nil {
		return err
	}
	edges := map[string]bool{}
	for _, edge := range v.Interactions {
		if !livingid.ValidID(edge.ID) || edges[edge.ID] || !members[edge.From] || !members[edge.To] || strings.TrimSpace(edge.Description) == "" {
			return fmt.Errorf("interaction requires unique ID, member endpoints and a description")
		}
		edges[edge.ID] = true
		if edge.Intent != "" {
			return fmt.Errorf("interaction %s: intent requires explicit revision intent", edge.ID)
		}
		if err := evidenceList(edge.Code, 1, false, nil); err != nil {
			return fmt.Errorf("interaction %s: %w", edge.ID, err)
		}
	}
	return nil
}

func systemMembers(v TechnicalRevision, sagaID string) (map[string]bool, error) {
	if len(v.Components) < 2 || len(v.Components) > 32 || len(v.Interactions) == 0 || len(v.Interactions) > 64 {
		return nil, fmt.Errorf("a System requires 2 to 32 Components and 1 to 64 interactions")
	}
	members := map[string]bool{}
	for _, pin := range v.Components {
		if !validPin(sagaID, pin, "component") {
			return nil, fmt.Errorf("system members must pin Component revisions")
		}
		if members[pin.Target] {
			return nil, fmt.Errorf("duplicate Component %s", pin.Target)
		}
		members[pin.Target] = true
	}
	return members, nil
}

// validateIntentRevision applies the format-2 rules shared by Components,
// Systems and data entities. Cross-record facts (endpoint intent, delivery
// currency) are checked by the public writer through technicalpolicy.
func validateIntentRevision(v TechnicalRevision, kind, urn, sagaID string) error {
	switch v.Intent {
	case IntentProposed:
		if v.Baseline == "" {
			return fmt.Errorf("a proposed revision requires baseline: an implemented revision URN of this record or %q", BaselineNone)
		}
		if v.Baseline != BaselineNone {
			id := strings.TrimPrefix(v.Baseline, urn+":revision:")
			if id == v.Baseline || !livingid.ValidID(id) || id == v.ID {
				return fmt.Errorf("baseline must be %q or another revision URN of %s", BaselineNone, urn)
			}
			if len(v.Parents) == 0 {
				return fmt.Errorf("an initial proposed revision has no implemented baseline; use %q", BaselineNone)
			}
		}
		if v.Delivery != nil {
			return fmt.Errorf("a proposed revision cannot record a delivery commit")
		}
	case IntentImplemented:
		if v.Baseline != "" {
			return fmt.Errorf("baseline applies only to proposed revisions")
		}
		if v.Delivery == nil || strings.TrimSpace(v.Delivery.Repository) == "" || !coderef.ValidCommit(v.Delivery.Commit) {
			return fmt.Errorf("an implemented revision requires delivery {repository, full commit}")
		}
	default:
		return fmt.Errorf("intent must be %s or %s", IntentProposed, IntentImplemented)
	}
	implemented := v.Intent == IntentImplemented
	minimum := 0
	if implemented {
		minimum = 1
	}
	ids := map[string]bool{}
	if err := evidenceList(v.Code, minimum, true, ids); err != nil {
		return err
	}
	switch kind {
	case "component":
		if len(v.Components)+len(v.Interactions) > 0 {
			return fmt.Errorf("components cannot contain a system graph")
		}
	case "system":
		members, err := systemMembers(v, sagaID)
		if err != nil {
			return err
		}
		edges := map[string]bool{}
		for _, edge := range v.Interactions {
			if !livingid.ValidID(edge.ID) || edges[edge.ID] || !members[edge.From] || !members[edge.To] || strings.TrimSpace(edge.Description) == "" {
				return fmt.Errorf("interaction requires unique ID, member endpoints and a description")
			}
			edges[edge.ID] = true
			if err := edgeIntent(edge.ID, edge.Intent, implemented); err != nil {
				return err
			}
			if err := evidenceList(edge.Code, edgeMinimum(edge.Intent), true, ids); err != nil {
				return fmt.Errorf("interaction %s: %w", edge.ID, err)
			}
		}
	case KindDataEntity:
		return validateDataEntity(v, sagaID, implemented, ids)
	}
	return nil
}

func edgeIntent(id, intent string, ownerImplemented bool) error {
	switch intent {
	case IntentProposed:
		return nil
	case IntentImplemented:
		if !ownerImplemented {
			return fmt.Errorf("edge %s: a proposed owner cannot assert an implemented relationship", id)
		}
		return nil
	}
	return fmt.Errorf("edge %s: intent must be %s or %s", id, IntentProposed, IntentImplemented)
}

func edgeMinimum(intent string) int {
	if intent == IntentImplemented {
		return 1
	}
	return 0
}

func validateDataEntity(v TechnicalRevision, sagaID string, implemented bool, ids map[string]bool) error {
	if len(v.Fields) > MaxEntityFields || len(v.Holders) > MaxEntityHolders || len(v.Relationships) > MaxRelationships {
		return fmt.Errorf("a data entity has at most %d fields, %d holders and %d relationships", MaxEntityFields, MaxEntityHolders, MaxRelationships)
	}
	fields := map[string]bool{}
	for _, field := range v.Fields {
		if !livingid.ValidID(field.ID) || fields[field.ID] || strings.TrimSpace(field.Name) == "" || field.Name != strings.TrimSpace(field.Name) || len(field.Name) > 200 || len(field.Type) > 200 {
			return fmt.Errorf("field requires a unique stable id and a name up to 200 bytes")
		}
		fields[field.ID] = true
		keys := map[string]bool{}
		for _, key := range field.Keys {
			if !fieldKeys[key] || keys[key] {
				return fmt.Errorf("field %s: keys must be unique primary, foreign, unique or partition roles", field.ID)
			}
			keys[key] = true
		}
	}
	holders := map[string]bool{}
	for _, holder := range v.Holders {
		if !validPin(sagaID, holder.Component, "component") || holders[holder.Component.Target] || strings.TrimSpace(holder.Role) == "" {
			return fmt.Errorf("holder requires a unique Component revision pin and a role")
		}
		holders[holder.Component.Target] = true
	}
	edges := map[string]bool{}
	for _, edge := range v.Relationships {
		if !livingid.ValidID(edge.ID) || edges[edge.ID] {
			return fmt.Errorf("relationship requires a unique stable id")
		}
		edges[edge.ID] = true
		if !validPin(sagaID, edge.Destination, KindDataEntity) || strings.TrimSpace(edge.Explanation) == "" || len(edge.Label) > MaxERDLabelBytes {
			return fmt.Errorf("relationship %s requires a data-entity revision destination, an explanation and a label up to %d bytes", edge.ID, MaxERDLabelBytes)
		}
		switch edge.Meaning {
		case "association":
			if edge.Cardinality == nil || !cardinalities[edge.Cardinality.Owner] || !cardinalities[edge.Cardinality.Destination] || edge.Flow != "" {
				return fmt.Errorf("association %s requires declared (possibly unknown) cardinality at both ends and no flow", edge.ID)
			}
		case "production":
			if edge.Cardinality != nil || !productionFlows[edge.Flow] || strings.TrimSpace(edge.Label) == "" {
				return fmt.Errorf("production %s requires a flow direction and label, and no cardinality", edge.ID)
			}
		default:
			return fmt.Errorf("relationship %s meaning must be association or production", edge.ID)
		}
		if err := edgeIntent(edge.ID, edge.Intent, implemented); err != nil {
			return err
		}
		if err := evidenceList(edge.Code, edgeMinimum(edge.Intent), true, ids); err != nil {
			return fmt.Errorf("relationship %s: %w", edge.ID, err)
		}
	}
	return nil
}

func validateERDRevision(v TechnicalRevision, kind, sagaID string) error {
	if v.Intent != "" || v.Baseline != "" || v.Delivery != nil || len(v.Code) > 0 || len(v.Components)+len(v.Interactions)+len(v.Fields)+len(v.Holders)+len(v.Relationships) > 0 {
		return fmt.Errorf("an ERD view carries no intent, code or entity content; entities carry their own")
	}
	members := map[DocumentationLink]bool{}
	if kind == KindERD {
		if v.Visual == nil || v.ERD != nil || v.Feature != "" || len(v.Pins)+len(v.Removals) > 0 {
			return fmt.Errorf("an ERD requires a visual and cannot name an overlay baseline or pins")
		}
		if len(v.Directory) == 0 || len(v.Directory) > MaxERDDirectory {
			return fmt.Errorf("an ERD directory requires 1 to %d data-entity pins", MaxERDDirectory)
		}
		if err := uniqueEntityPins(sagaID, v.Directory, members); err != nil {
			return err
		}
	} else {
		if v.ERD == nil || !validPin(sagaID, *v.ERD, KindERD) || len(v.Directory) > 0 {
			return fmt.Errorf("an ERD overlay requires an exact ERD revision baseline and no directory of its own")
		}
		if v.Feature != "" && !strings.HasPrefix(v.Feature, "urn:change-saga:"+sagaID+":feature:") || v.Feature != "" && !livingid.ValidID(strings.TrimPrefix(v.Feature, "urn:change-saga:"+sagaID+":feature:")) {
			return fmt.Errorf("overlay feature must be a canonical feature URN")
		}
		if len(v.Pins)+len(v.Removals) == 0 || len(v.Pins) > MaxOverlayPins || len(v.Removals) > MaxOverlayPins {
			return fmt.Errorf("an ERD overlay requires 1 to %d pins or removals", MaxOverlayPins)
		}
		if err := uniqueEntityPins(sagaID, v.Pins, map[DocumentationLink]bool{}); err != nil {
			return err
		}
		touched := map[string]bool{}
		for _, pin := range v.Pins {
			touched[pin.Target] = true
		}
		for _, removal := range v.Removals {
			if !validPin(sagaID, DocumentationLink{Target: removal.Target, Revision: removal.Target + ":revision:r"}, KindDataEntity) || touched[removal.Target] || strings.TrimSpace(removal.Explanation) == "" {
				return fmt.Errorf("removal requires a data-entity target not otherwise pinned and an explanation")
			}
			touched[removal.Target] = true
		}
		if v.Visual == nil && len(v.Bindings) > 0 {
			return fmt.Errorf("overlay bindings require an overlay visual")
		}
		members = nil // bindings resolve against the composed directory
	}
	if v.Visual != nil {
		match := visualAssetPath.FindStringSubmatch(v.Visual.Path)
		if match == nil || v.Visual.MediaType != "image/svg+xml" || v.Visual.Digest != coderef.DigestPrefix+match[1] {
			return fmt.Errorf("visual must be image/svg+xml at assets/<sha256>.svg with its digest")
		}
	}
	if len(v.Bindings) > MaxERDBindings {
		return fmt.Errorf("an ERD has at most %d bindings", MaxERDBindings)
	}
	ids, elements := map[string]bool{}, map[string]bool{}
	for _, b := range v.Bindings {
		if !livingid.ValidID(b.ID) || ids[b.ID] || !svgElementID.MatchString(b.Element) || elements[b.Element] {
			return fmt.Errorf("binding requires a unique stable id and a unique SVG element id")
		}
		ids[b.ID], elements[b.Element] = true, true
		var pin DocumentationLink
		switch {
		case b.Entity != nil && b.Relationship == nil:
			pin = *b.Entity
		case b.Relationship != nil && b.Entity == nil && livingid.ValidID(b.Relationship.ID):
			pin = b.Relationship.Owner
		default:
			return fmt.Errorf("binding %s must name exactly one entity or relationship", b.ID)
		}
		if !validPin(sagaID, pin, KindDataEntity) {
			return fmt.Errorf("binding %s must pin a data-entity revision", b.ID)
		}
		if members != nil && !members[pin] {
			return fmt.Errorf("binding %s names %s outside the directory; the visual is a subset of the directory", b.ID, pin.Revision)
		}
	}
	return nil
}

func uniqueEntityPins(sagaID string, pins []DocumentationLink, members map[DocumentationLink]bool) error {
	targets := map[string]bool{}
	for _, pin := range pins {
		if !validPin(sagaID, pin, KindDataEntity) || targets[pin.Target] {
			return fmt.Errorf("pins must name unique data-entity revisions")
		}
		targets[pin.Target] = true
		members[pin] = true
	}
	return nil
}

// checkVisualAsset verifies the pinned offline SVG exists with its digest,
// references nothing external, and contains every bound element.
func checkVisualAsset(packageDir string, visual Visual, bindings []Binding) error {
	path := filepath.Join(packageDir, filepath.FromSlash(visual.Path))
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("visual %s: %w", visual.Path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > MaxVisualBytes {
		return fmt.Errorf("visual %s must be a regular file up to one MiB", visual.Path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return ValidateVisual(data, visual.Digest, bindings)
}

// ValidateVisual checks an SVG against its digest and bound element IDs. The
// renderer inlines the SVG for hit-testing, so authoring refuses anything
// active or external: scripts, foreign objects, event handlers, non-fragment
// links, external url()/@import in styles and DTD/entity directives. Every ID
// must be unique so a binding names exactly one element.
func ValidateVisual(data []byte, digest string, bindings []Binding) error {
	if len(data) > MaxVisualBytes {
		return fmt.Errorf("visual exceeds one MiB")
	}
	sum := sha256.Sum256(data)
	if digest != "" && digest != coderef.DigestPrefix+hex.EncodeToString(sum[:]) {
		return fmt.Errorf("visual digest does not match its bytes")
	}
	decoder := xml.NewDecoder(io.LimitReader(bytes.NewReader(data), MaxVisualBytes))
	decoder.Strict = true
	elements := map[string]int{}
	depth, roots, inStyle := 0, 0, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("visual is not well-formed SVG: %w", err)
		}
		switch value := token.(type) {
		case xml.Directive:
			return fmt.Errorf("visual cannot contain DOCTYPE or entity directives")
		case xml.ProcInst:
			if value.Target != "xml" {
				return fmt.Errorf("visual cannot contain processing instructions")
			}
		case xml.CharData:
			if inStyle && unsafeStyle(string(value)) {
				return fmt.Errorf("visual styles cannot import or reference external content")
			}
		case xml.EndElement:
			depth--
			inStyle = false
		case xml.StartElement:
			if depth == 0 {
				roots++
				if value.Name.Local != "svg" {
					return fmt.Errorf("visual root element must be svg")
				}
			}
			depth++
			name := strings.ToLower(value.Name.Local)
			if name == "script" || name == "foreignobject" || name == "iframe" || name == "object" || name == "embed" {
				return fmt.Errorf("visual cannot contain %s elements", value.Name.Local)
			}
			inStyle = name == "style"
			for _, attr := range value.Attr {
				local := strings.ToLower(attr.Name.Local)
				switch {
				case strings.HasPrefix(local, "on"):
					return fmt.Errorf("visual cannot contain event handler attributes")
				case local == "href" || local == "src":
					if !strings.HasPrefix(strings.TrimSpace(attr.Value), "#") {
						return fmt.Errorf("visual links must be same-document #fragments")
					}
				case local == "style" || strings.Contains(strings.ToLower(attr.Value), "url("):
					if unsafeStyle(attr.Value) {
						return fmt.Errorf("visual must be offline; %s references external content", attr.Name.Local)
					}
				}
				if local == "id" && attr.Name.Space == "" {
					elements[attr.Value]++
				}
			}
		}
	}
	if roots != 1 {
		return fmt.Errorf("visual must contain exactly one svg root element")
	}
	for id, count := range elements {
		if count > 1 {
			return fmt.Errorf("visual element id %q is not unique", id)
		}
	}
	for _, b := range bindings {
		if elements[b.Element] != 1 {
			return fmt.Errorf("binding %s names missing SVG element %q", b.ID, b.Element)
		}
	}
	return nil
}

var cssURL = regexp.MustCompile(`(?i)url\(\s*['"]?\s*([^'")\s]*)`)

// unsafeStyle reports @import or any url() that is not a same-document fragment.
func unsafeStyle(value string) bool {
	if strings.Contains(strings.ToLower(value), "@import") {
		return true
	}
	for _, match := range cssURL.FindAllStringSubmatch(value, -1) {
		if !strings.HasPrefix(match[1], "#") {
			return true
		}
	}
	return false
}
