package requirements

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

const WhiteboardMetadataName = ".metadata.json"
const WhiteboardMetadataSchemaURL = "https://changesaga.dev/schema/v5/whiteboard-metadata.schema.json"

// WhiteboardMetadata is mutable presentation metadata, separate from immutable
// source revisions. Empty role values explicitly leave that reading view unassigned.
type WhiteboardMetadata struct {
	Stored  bool                            `json:"-"`
	Schema  string                          `json:"$schema"`
	Version int                             `json:"version"`
	SagaID  string                          `json:"saga_id"`
	Roles   map[string]string               `json:"roles"`
	Boards  map[string]WhiteboardProperties `json:"boards"`
}

type WhiteboardProperties struct {
	Name     string `json:"name,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

var whiteboardRoleDefaults = map[string]string{"erd": "data-model", "infrastructure": "infrastructure", "data-flows": "data-flows"}

// LoadWhiteboardMetadata preserves the legacy default IDs until a metadata
// operation explicitly records the role assignments. It never renders a board.
func LoadWhiteboardMetadata(root, sagaID string) (WhiteboardMetadata, error) {
	metadata := WhiteboardMetadata{Schema: WhiteboardMetadataSchemaURL, Version: 1, SagaID: sagaID, Roles: map[string]string{}, Boards: map[string]WhiteboardProperties{}}
	if !livingid.ValidID(sagaID) {
		return metadata, fmt.Errorf("whiteboard requires a valid saga ID")
	}
	ids, err := ListWhiteboardIDs(root)
	if err != nil {
		return metadata, err
	}
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
	}
	path := filepath.Join(root, WhiteboardDir, WhiteboardMetadataName)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		for role, id := range whiteboardRoleDefaults {
			metadata.Roles[role] = ""
			if known[id] {
				metadata.Roles[role] = id
			}
		}
		return metadata, nil
	} else if err != nil {
		return metadata, err
	}
	metadata = WhiteboardMetadata{}
	if err := readInventoryJSON(path, &metadata); err != nil {
		return metadata, err
	}
	if metadata.Schema != WhiteboardMetadataSchemaURL || metadata.Version != 1 || metadata.SagaID != sagaID || metadata.Roles == nil || metadata.Boards == nil || len(metadata.Roles) != len(whiteboardRoleDefaults) {
		return metadata, fmt.Errorf("invalid whiteboard metadata schema, version, ownership or roles")
	}
	metadata.Stored = true
	assigned := map[string]bool{}
	for role, id := range metadata.Roles {
		if _, ok := whiteboardRoleDefaults[role]; !ok {
			return metadata, fmt.Errorf("unknown whiteboard role %q", role)
		}
		if id == "" {
			continue
		}
		if !known[id] || assigned[id] || metadata.Boards[id].Archived {
			return metadata, fmt.Errorf("whiteboard role %s must name an existing, active, uniquely assigned board", role)
		}
		assigned[id] = true
	}
	for id, properties := range metadata.Boards {
		if !known[id] {
			return metadata, fmt.Errorf("whiteboard metadata names missing board %q", id)
		}
		if properties.Name != "" && !validWhiteboardName(properties.Name) {
			return metadata, fmt.Errorf("whiteboard name must contain 1 to 200 bytes without surrounding whitespace")
		}
	}
	return metadata, nil
}

func validWhiteboardName(name string) bool {
	return name != "" && strings.TrimSpace(name) == name && len(name) <= 200
}

func (m WhiteboardMetadata) Role(id string) string {
	for role, board := range m.Roles {
		if id != "" && board == id {
			return role
		}
	}
	return ""
}

// WhiteboardMetadataChange reports the complete before/after state so replacing
// a role also accounts for the displaced board and the selected board's old role.
type WhiteboardMetadataChange struct {
	Operation string             `json:"operation"`
	Target    string             `json:"target"`
	Path      string             `json:"path"`
	DryRun    bool               `json:"dry_run"`
	Changed   bool               `json:"changed"`
	Before    WhiteboardMetadata `json:"before"`
	After     WhiteboardMetadata `json:"after"`
}

type WhiteboardMetadataWrite struct {
	Operation, ID, Role, Expected, Name, ExpectedSnapshot string
	DryRun, Move                                          bool
}

// UpdateWhiteboardMetadata validates ownership, containment and role preconditions
// under the Saga lock, then publishes a single atomic metadata replacement.
// Archiving only removes discovery; direct references and all revisions remain valid.
func UpdateWhiteboardMetadata(ctx context.Context, root, sagaID string, w WhiteboardMetadataWrite) (WhiteboardMetadataChange, error) {
	result := WhiteboardMetadataChange{Operation: w.Operation, Path: WhiteboardDir + "/" + WhiteboardMetadataName, DryRun: w.DryRun}
	if !livingid.ValidID(w.ID) {
		return result, fmt.Errorf("invalid whiteboard ID")
	}
	result.Target, _ = WhiteboardURN(sagaID, w.ID)
	operation := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if present, err := realDirectory(root); err != nil {
			return err
		} else if !present {
			return fmt.Errorf("Saga root does not exist")
		}
		var identity sagaIdentity
		if err := readStrictJSON(filepath.Join(root, "saga.json"), &identity); err != nil {
			return err
		}
		if identity.Version != sagaVersion || identity.ID != sagaID {
			return fmt.Errorf("whiteboard metadata requires matching v5 Saga ownership")
		}
		before, err := LoadWhiteboardMetadata(root, sagaID)
		if err != nil {
			return err
		}
		board, err := LoadWhiteboard(root, sagaID, w.ID, "")
		if err != nil {
			return err
		}
		if board == nil {
			return fmt.Errorf("whiteboard %q does not exist", w.ID)
		}
		if w.ExpectedSnapshot != "" && w.ExpectedSnapshot != before.Snapshot() {
			return fmt.Errorf("whiteboard metadata snapshot changed")
		}
		result.Before = before
		after := before
		after.Roles = map[string]string{}
		for role, id := range before.Roles {
			after.Roles[role] = id
		}
		after.Boards = map[string]WhiteboardProperties{}
		for id, properties := range before.Boards {
			after.Boards[id] = properties
		}
		properties := after.Boards[w.ID]
		switch w.Operation {
		case "assign-role", "unassign-role":
			if _, ok := whiteboardRoleDefaults[w.Role]; !ok {
				return fmt.Errorf("--role must be erd, infrastructure or data-flows")
			}
			if w.Operation == "unassign-role" {
				if after.Roles[w.Role] != w.ID {
					return fmt.Errorf("role %s is assigned to %q, not %q", w.Role, after.Roles[w.Role], w.ID)
				}
				after.Roles[w.Role] = ""
			} else {
				if w.Expected == "" {
					return fmt.Errorf("assign-role requires --expect ID|URN|none naming the observed role assignment")
				}
				expected := w.Expected
				if strings.HasPrefix(expected, "urn:") {
					prefix := "urn:change-saga:" + sagaID + ":whiteboard:"
					if !strings.HasPrefix(expected, prefix) || !livingid.ValidID(strings.TrimPrefix(expected, prefix)) {
						return fmt.Errorf("expected owner must be a whiteboard ID or canonical URN in this Saga")
					}
					expected = strings.TrimPrefix(expected, prefix)
				} else if expected == "none" {
					expected = ""
				}
				if after.Roles[w.Role] != expected {
					return fmt.Errorf("role %s assignment changed: expected %q, current %q", w.Role, expected, after.Roles[w.Role])
				}
				if properties.Archived {
					return fmt.Errorf("restore whiteboard %s before assigning a role", w.ID)
				}
				if oldRole := after.Role(w.ID); oldRole != "" && oldRole != w.Role && (!w.Move || w.ExpectedSnapshot == "") {
					return fmt.Errorf("whiteboard %s is assigned to %s; use --move and --expected-snapshot to move that observed assignment", w.ID, oldRole)
				}
				for role, id := range after.Roles {
					if id == w.ID {
						after.Roles[role] = ""
					}
				}
				after.Roles[w.Role] = w.ID
			}
		case "rename":
			if !validWhiteboardName(w.Name) {
				return fmt.Errorf("whiteboard name must contain 1 to 200 bytes without surrounding whitespace")
			}
			properties.Name = w.Name
			after.Boards[w.ID] = properties
		case "archive":
			if role := after.Role(w.ID); role != "" {
				return fmt.Errorf("whiteboard %s is assigned to %s; unassign or reassign the role before archiving", w.ID, role)
			}
			properties.Archived = true
			after.Boards[w.ID] = properties
		case "restore":
			properties.Archived = false
			after.Boards[w.ID] = properties
		default:
			return fmt.Errorf("unsupported whiteboard metadata operation %q", w.Operation)
		}
		result.After = after
		result.Changed = !before.Stored || !reflect.DeepEqual(before, after)
		data, err := json.MarshalIndent(after, "", "  ")
		if err != nil {
			return err
		}
		if len(data)+1 > MaxRecordBytes {
			return fmt.Errorf("whiteboard metadata exceeds one MiB")
		}
		if w.DryRun || !result.Changed {
			return nil
		}
		return store.WriteFile(filepath.Join(root, filepath.FromSlash(result.Path)), append(data, '\n'), 0644, false)
	}
	// Dry runs perform the identical preflight without creating lock files.
	if w.DryRun {
		err := operation()
		return result, err
	}
	err := store.WithSagaLock(root, store.DefaultLockTimeout, operation)
	return result, err
}

// Snapshot identifies the complete effective metadata, including legacy role defaults.
func (m WhiteboardMetadata) Snapshot() string {
	data, _ := json.Marshal(m)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
