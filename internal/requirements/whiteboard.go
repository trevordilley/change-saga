package requirements

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/store"
)

const WhiteboardDir = "___whiteboards"
const MaxWhiteboards = 10_000
const WhiteboardSchemaURL = "https://changesaga.dev/schema/v5/whiteboard-revision.schema.json"

// WhiteboardRevision is an immutable, complete composition snapshot. Parents
// are board revision IDs, while embedded source references use exact URN pins.
type WhiteboardRevision struct {
	Schema    string           `json:"$schema"`
	Version   int              `json:"version"`
	ID        string           `json:"id"`
	Parents   []string         `json:"parents"`
	Name      string           `json:"name"`
	Document  diagram.Document `json:"document"`
	CreatedAt time.Time        `json:"created_at"`
}

type ResolvedWhiteboard struct {
	Revision    WhiteboardRevision   `json:"revision"`
	Composition *diagram.Composition `json:"-"`
	SVG         []byte               `json:"-"`
}

// ListWhiteboardIDs lists named documents without loading their revision histories.
// The optional root and every entry must be real directories with stable IDs.
// Store staging directories are unpublished and do not consume document slots.
func ListWhiteboardIDs(root string) ([]string, error) {
	dir := filepath.Join(root, WhiteboardDir)
	present, err := realDirectory(dir)
	if err != nil || !present {
		return nil, err
	}
	// Allow bounded staging debris as well as the document limit. CommitDir
	// stages beside the final board, including while unlocked readers list it.
	entries, err := boundedReadDir(dir, 2*MaxWhiteboards+1)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name() == WhiteboardMetadataName && entry.Type().IsRegular() {
			continue
		}
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".change-saga-write-") {
			continue
		}
		if !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("whiteboard entry %q must be a real directory with a valid board ID", entry.Name())
		}
		if strings.HasPrefix(entry.Name(), ".change-saga-stage-") {
			continue
		}
		if !livingid.ValidID(entry.Name()) {
			return nil, fmt.Errorf("whiteboard entry %q must have a valid board ID", entry.Name())
		}
		ids = append(ids, entry.Name())
		if len(ids) > MaxWhiteboards {
			return nil, fmt.Errorf("whiteboard document count exceeds maximum of %d", MaxWhiteboards)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

func whiteboardHistory(root, sagaID, boardID string) (map[string]WhiteboardRevision, []string, error) {
	if !livingid.ValidID(sagaID) || !livingid.ValidID(boardID) {
		return nil, nil, fmt.Errorf("whiteboard requires valid saga and board IDs")
	}
	for _, dir := range []string{filepath.Join(root, WhiteboardDir), filepath.Join(root, WhiteboardDir, boardID)} {
		present, err := realDirectory(dir)
		if err != nil || !present {
			return nil, nil, err
		}
	}
	dir := filepath.Join(root, WhiteboardDir, boardID)
	present, err := realDirectory(filepath.Join(dir, "revisions"))
	if err != nil {
		return nil, nil, err
	}
	if !present {
		return nil, nil, fmt.Errorf("whiteboard directory %s is missing", filepath.Join(dir, "revisions"))
	}
	entries, err := boundedReadDir(dir, 1)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range entries {
		if entry.Name() != "revisions" {
			return nil, nil, fmt.Errorf("unknown whiteboard entry %s", entry.Name())
		}
	}
	dir = filepath.Join(dir, "revisions")
	entries, err = whiteboardRevisionFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("whiteboard has no revisions")
	}
	revisions := map[string]WhiteboardRevision{}
	parents := map[string][]string{}
	for _, entry := range entries {
		var v WhiteboardRevision
		if err := readInventoryJSON(filepath.Join(dir, entry.Name()), &v); err != nil {
			return nil, nil, fmt.Errorf("whiteboard %s: %w", entry.Name(), err)
		}
		if err := validateWhiteboardRevision(v, strings.TrimSuffix(entry.Name(), ".json")); err != nil {
			return nil, nil, err
		}
		revisions[v.ID], parents[v.ID] = v, v.Parents
	}
	for id, v := range revisions {
		for _, parent := range v.Parents {
			if _, ok := revisions[parent]; !ok {
				return nil, nil, fmt.Errorf("whiteboard revision %s has missing parent %s", id, parent)
			}
		}
	}
	heads, roots, cycle := graphHeads(parents, mapKeys(revisions))
	if cycle || len(roots) != 1 {
		return nil, nil, fmt.Errorf("whiteboard revisions require an acyclic graph with one root")
	}
	return revisions, heads, nil
}

func whiteboardRevisionFiles(dir string) ([]fs.DirEntry, error) {
	// WriteFile publishes through a regular same-directory temporary file and
	// an atomic link. Bound temporary debris separately from saved revisions.
	entries, err := boundedReadDir(dir, 2*MaxRevisionsPerStory)
	if err != nil {
		return nil, err
	}
	files := entries[:0]
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), ".change-saga-write-") {
			continue
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("whiteboard revision %s must be a regular JSON file", entry.Name())
		}
		files = append(files, entry)
		if len(files) > MaxRevisionsPerStory {
			return nil, fmt.Errorf("whiteboard revision count exceeds maximum of %d", MaxRevisionsPerStory)
		}
	}
	return files, nil
}

func validateWhiteboardRevision(v WhiteboardRevision, id string) error {
	if v.Schema != WhiteboardSchemaURL || v.Version != 1 || !livingid.ValidID(v.ID) || v.ID != id || v.Parents == nil || len(v.Parents) > 1 || v.CreatedAt.IsZero() {
		return fmt.Errorf("invalid whiteboard revision identity, schema, parents, version or time")
	}
	if strings.TrimSpace(v.Name) == "" || strings.TrimSpace(v.Name) != v.Name || len(v.Name) > 200 {
		return fmt.Errorf("whiteboard name must contain 1 to 200 bytes")
	}
	for _, p := range v.Parents {
		if !livingid.ValidID(p) || p == v.ID {
			return fmt.Errorf("invalid whiteboard parent %q", p)
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > MaxRecordBytes {
		return fmt.Errorf("whiteboard revision exceeds one MiB")
	}
	return v.Document.Validate()
}

// ListWhiteboardRevisionIDs validates a document's stored revision history once
// and lists its IDs from the unique current head to the root. An absent board
// returns nil,nil. Like LoadWhiteboard, it does not resolve or render source pins.
func ListWhiteboardRevisionIDs(root, sagaID, boardID string) ([]string, error) {
	revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
	if err != nil || len(revisions) == 0 {
		return nil, err
	}
	if len(heads) != 1 {
		return nil, fmt.Errorf("whiteboard has conflicting revision heads: %v", heads)
	}
	ids := make([]string, 0, len(revisions))
	for id := heads[0]; ; {
		ids = append(ids, id)
		parents := revisions[id].Parents
		if len(parents) == 0 {
			return ids, nil
		}
		id = parents[0]
	}
}

// LoadWhiteboard selects an exact revision or the unique head. Missing optional
// board storage returns nil,nil. An explicit revision never falls back to head.
func LoadWhiteboard(root, sagaID, boardID, revision string) (*WhiteboardRevision, error) {
	revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
	if err != nil {
		return nil, err
	}
	if revision == "" {
		if len(revisions) == 0 {
			return nil, nil
		}
		if len(heads) != 1 {
			return nil, fmt.Errorf("whiteboard has conflicting revision heads: %v", heads)
		}
		revision = heads[0]
	}
	v, ok := revisions[revision]
	if !ok {
		return nil, fmt.Errorf("whiteboard revision %q does not exist", revision)
	}
	return &v, nil
}

// PublishWhiteboard validates every source before appending a complete snapshot.
// Updating requires the observed current parent; child revisions never advance
// implicitly. Retrying identical content is idempotent.
func PublishWhiteboard(ctx context.Context, root, sagaID, boardID, revision, parent, name string, document diagram.Document) (*WhiteboardRevision, error) {
	v := WhiteboardRevision{Schema: WhiteboardSchemaURL, Version: 1, ID: revision, Parents: []string{}, Name: name, Document: document, CreatedAt: mutationTime(time.Time{})}
	if parent != "" {
		v.Parents = []string{parent}
	}
	if err := validateWhiteboardRevision(v, revision); err != nil {
		return nil, err
	}
	err := store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
		if err != nil {
			return err
		}
		if old, ok := revisions[revision]; ok {
			v.CreatedAt = old.CreatedAt
			if reflect.DeepEqual(v, old) {
				return nil
			}
			return fmt.Errorf("whiteboard revision %s already exists with different content", revision)
		}
		if len(revisions) >= MaxRevisionsPerStory {
			return fmt.Errorf("whiteboard revision limit reached")
		}
		if len(revisions) == 0 && parent != "" || len(revisions) > 0 && (len(heads) != 1 || parent != heads[0]) {
			return fmt.Errorf("whiteboard parent must name the current head: %v", heads)
		}
		if _, err := resolveWhiteboard(root, sagaID, v); err != nil {
			return err
		}
		if len(revisions) == 0 {
			ids, err := ListWhiteboardIDs(root)
			if err != nil {
				return err
			}
			if len(ids) >= MaxWhiteboards {
				return fmt.Errorf("whiteboard document limit of %d reached", MaxWhiteboards)
			}
			boardRoot := filepath.Join(root, WhiteboardDir)
			present, err := realDirectory(boardRoot)
			if err != nil {
				return err
			}
			// Atomically expose the first root or a complete new board inside an
			// existing root. Failed publications never leave an empty board or
			// replace any sibling's immutable history.
			final, relative := filepath.Join(boardRoot, boardID), "revisions"
			if !present {
				final, relative = boardRoot, filepath.Join(boardID, "revisions")
			}
			return store.CommitDir(root, final, func(stage string) error {
				dir := filepath.Join(stage, relative)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				return store.WriteJSON(filepath.Join(dir, revision+".json"), v, true)
			})
		}
		return store.WriteJSON(filepath.Join(root, WhiteboardDir, boardID, "revisions", revision+".json"), v, true)
	})
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func LoadResolvedWhiteboard(root, sagaID, boardID, revision string) (*ResolvedWhiteboard, error) {
	v, err := LoadWhiteboard(root, sagaID, boardID, revision)
	if err != nil || v == nil {
		return nil, err
	}
	return resolveWhiteboard(root, sagaID, *v)
}

func resolveWhiteboard(root, sagaID string, v WhiteboardRevision) (*ResolvedWhiteboard, error) {
	inventory, err := LoadInventory(root, sagaID)
	if err != nil {
		return nil, err
	}
	composition, err := resolveInventoryComposition(root, &inventory, v.Document, nil)
	if err != nil {
		return nil, err
	}
	svg, err := composition.Render(diagram.Options{Title: v.Name})
	if err != nil {
		return nil, err
	}
	return &ResolvedWhiteboard{Revision: v, Composition: composition, SVG: svg}, nil
}

// ValidateWhiteboards validates every named document's complete history,
// including source pins hidden by newer revisions. The subtree is optional.
func ValidateWhiteboards(root, sagaID string) error {
	if _, err := LoadWhiteboardMetadata(root, sagaID); err != nil {
		return err
	}
	if !livingid.ValidID(sagaID) {
		return fmt.Errorf("whiteboard requires a valid saga ID")
	}
	ids, err := ListWhiteboardIDs(root)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := ValidateWhiteboard(root, sagaID, id); err != nil {
			return fmt.Errorf("whiteboard %s: %w", id, err)
		}
	}
	return nil
}

// ValidateWhiteboard validates one document's complete history and exact source
// pins. A missing optional document is valid, including beside existing boards.
func ValidateWhiteboard(root, sagaID, boardID string) error {
	revisions, heads, err := whiteboardHistory(root, sagaID, boardID)
	if err != nil {
		return err
	}
	if len(revisions) > 0 && len(heads) != 1 {
		return fmt.Errorf("whiteboard has conflicting revision heads: %v", heads)
	}
	ids := mapKeys(revisions)
	slices.Sort(ids)
	for _, id := range ids {
		if _, err := resolveWhiteboard(root, sagaID, revisions[id]); err != nil {
			return fmt.Errorf("whiteboard revision %s: %w", id, err)
		}
	}
	return nil
}
