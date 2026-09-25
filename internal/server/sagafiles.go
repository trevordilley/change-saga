package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// sagaFilesCache keeps what documentation pages read from the Saga's own
// files while none of them has changed: the narrative, the requirements, the
// test cases, and the technical inventory. Every documentation page reads all
// of them, and decoding them from a large Saga costs several times the stat
// walk that proves none of them changed.
type sagaFilesCache struct {
	mutex   sync.Mutex
	current *sagaFiles
	builds  int
}

// sagaFiles is one fingerprint of the Saga's files and what has been read
// from them. Each part is read the first time a request asks for it. The
// fingerprint is taken before anything is read, so an edit made while a part
// is read changes the next request's fingerprint and is never served from
// here. Callers only read what it holds.
type sagaFiles struct {
	root        string
	fingerprint string

	narrativeOnce sync.Once
	narrativeDoc  *saga.Saga

	recordsOnce sync.Once
	recordsID   string
	recordsDoc  requirements.Document
	recordsErr  error

	testsOnce sync.Once
	testsDoc  quality.Document
	testsErr  error

	inventoryOnce sync.Once
	inventoryID   string
	inventoryDoc  requirements.Inventory
	inventoryErr  error
}

// sagaFiles is the current Saga's files. When they cannot be fingerprinted
// nothing is kept, and each part is read afresh.
func (a *app) sagaFiles() *sagaFiles {
	fingerprint, err := documentationFingerprint(a.root)
	if err != nil {
		return &sagaFiles{root: a.root}
	}
	a.files.mutex.Lock()
	defer a.files.mutex.Unlock()
	if a.files.current == nil || a.files.current.fingerprint != fingerprint {
		a.files.current = &sagaFiles{root: a.root, fingerprint: fingerprint}
		a.files.builds++
	}
	return a.files.current
}

// narrative is the Saga with its complete narrative, or nil when it does
// not load valid.
func (files *sagaFiles) narrative() *saga.Saga {
	files.narrativeOnce.Do(func() {
		document, validation, err := saga.LoadNarrative(files.root)
		if err == nil && validation.Valid {
			files.narrativeDoc = document
		}
	})
	return files.narrativeDoc
}

// records is the Saga's requirements. The relations are the caller's own
// copy, since pages project complete-slide links into them.
func (files *sagaFiles) records(sagaID string) (requirements.Document, error) {
	files.recordsOnce.Do(func() {
		files.recordsID = sagaID
		files.recordsDoc, files.recordsErr = requirements.Load(files.root, sagaID)
	})
	if files.recordsID != sagaID {
		return requirements.Load(files.root, sagaID)
	}
	document := files.recordsDoc
	document.Relations = slices.Clone(document.Relations)
	return document, files.recordsErr
}

func (files *sagaFiles) tests() (quality.Document, error) {
	files.testsOnce.Do(func() {
		files.testsDoc, files.testsErr = quality.Load(files.root)
	})
	return files.testsDoc, files.testsErr
}

func (files *sagaFiles) inventory(sagaID string) (requirements.Inventory, error) {
	files.inventoryOnce.Do(func() {
		files.inventoryID = sagaID
		files.inventoryDoc, files.inventoryErr = requirements.LoadInventory(files.root, sagaID)
	})
	if files.inventoryID != sagaID {
		return requirements.LoadInventory(files.root, sagaID)
	}
	return files.inventoryDoc, files.inventoryErr
}

// documentationFingerprint commits to every file the parts above read, by
// path, size, and modification time. It skips the code evidence, claims, and
// verifications directories: the narrative leaves them unopened, and the
// records, test cases, and inventory live elsewhere. A documentation page's
// freshness check therefore does not scale with per-line evidence.
func documentationFingerprint(root string) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			switch entry.Name() {
			case saga.CodeDirName, "___claims", "___verifications":
				if path != root {
					return filepath.SkipDir
				}
			}
			fmt.Fprintf(digest, "d\x00%s\x00", rel)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(digest, "f\x00%s\x00%d\x00%d\x00", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
