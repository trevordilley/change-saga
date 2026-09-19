package saga

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/coderef"
)

const (
	// CursorName is the sync cursor of a Saga that lives in a companion
	// repository: the code commit the Saga currently documents. A Saga in its
	// code repository has no cursor file; its cursor is the commit it is read
	// at.
	CursorName      = "sync.json"
	CursorSchemaURL = "https://changesaga.dev/schema/v5/sync.schema.json"
	CursorVersion   = 5
)

// Cursor is the sync cursor record.
type Cursor struct {
	Schema  string `json:"$schema,omitempty"`
	Version int    `json:"version"`
	Commit  string `json:"commit"`
}

// ReadCursor reads the sync cursor at root. ok is false when the Saga has
// none.
func ReadCursor(root string) (Cursor, bool, error) {
	var cursor Cursor
	path := filepath.Join(root, CursorName)
	if info, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return Cursor{}, false, nil
	} else if err != nil {
		return Cursor{}, false, err
	} else if !info.Mode().IsRegular() {
		return Cursor{}, false, fmt.Errorf("%s must be a regular file", CursorName)
	}
	if err := readJSON(path, &cursor); err != nil {
		return Cursor{}, false, err
	}
	return cursor, true, ValidateCursor(cursor)
}

// ValidateCursor checks one cursor record.
func ValidateCursor(cursor Cursor) error {
	if cursor.Version != CursorVersion {
		return fmt.Errorf("sync cursor requires version %d", CursorVersion)
	}
	if !coderef.ValidCommit(cursor.Commit) {
		return fmt.Errorf("sync cursor commit must be a full commit object ID")
	}
	return nil
}
