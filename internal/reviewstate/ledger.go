package reviewstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/gitexec"
)

// CacheDirEnv redirects the ledger's directory, as it redirects the snapshot
// cache: tests set it so they never touch the user's real cache.
const CacheDirEnv = "CHANGE_SAGA_CACHE_DIR"

// Ledger keeps, across runs, the reviews already found merged. A merge never
// comes undone, so a review found merged at one commit of its base is merged
// at every later one: a repository with a thousand reviews asks Git about
// only the ones still open, and one ancestry check carries the rest forward.
// Open and unknown reviews are never kept; they are asked again.
//
// The ledger is derived, disposable state, like the snapshot cache: it lives
// under the user's cache directory, never inside a Saga, and deleting it only
// makes the next read slower. One repository shares one ledger across its
// worktrees, since record paths and commits are the repository's.
type Ledger struct {
	path    string
	mu      sync.Mutex
	entries map[string]ledgerEntry
	dirty   bool
}

// ledgerEntry is one review found merged: the base commit it was verified at
// and the ref that commit was read from. It is keyed by the review record's
// path in the repository.
type ledgerEntry struct {
	Base     string `json:"base"`
	LandedIn string `json:"landed_in"`
}

type ledgerFile struct {
	Version int                    `json:"version"`
	Merged  map[string]ledgerEntry `json:"merged"`
}

const ledgerVersion = 1

// OpenLedger reads the ledger of checkout's repository, or starts an empty
// one. It returns nil when there is nowhere to keep one, and a nil Ledger
// keeps nothing.
func OpenLedger(ctx context.Context, checkout string) *Ledger {
	_, common, ok := gitexec.GitDirs(ctx, checkout)
	if !ok {
		return nil
	}
	root := strings.TrimSpace(os.Getenv(CacheDirEnv))
	if root == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return nil
		}
		root = filepath.Join(cache, "change-saga")
	}
	if resolved, err := filepath.EvalSymlinks(common); err == nil {
		common = resolved
	}
	sum := sha256.Sum256([]byte(common))
	ledger := &Ledger{path: filepath.Join(root, "review-states", hex.EncodeToString(sum[:16])+".json"), entries: map[string]ledgerEntry{}}
	var file ledgerFile
	if data, err := os.ReadFile(ledger.path); err == nil && json.Unmarshal(data, &file) == nil && file.Version == ledgerVersion {
		for record, entry := range file.Merged {
			if gitexec.IsObjectName(entry.Base) {
				ledger.entries[record] = entry
			}
		}
	}
	return ledger
}

func (ledger *Ledger) lookup(record string) (ledgerEntry, bool) {
	if ledger == nil {
		return ledgerEntry{}, false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	entry, ok := ledger.entries[record]
	return entry, ok
}

func (ledger *Ledger) remember(record, base, ref string) {
	if ledger == nil {
		return
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if entry, ok := ledger.entries[record]; ok && entry.Base == base {
		return
	}
	ledger.entries[record] = ledgerEntry{Base: base, LandedIn: ref}
	ledger.dirty = true
}

// Save writes what was learned since the ledger was read. It replaces the
// file in one rename, so a reader sees the old ledger or the new one; two
// writers racing lose at most what the other learned, which is asked again.
func (ledger *Ledger) Save() error {
	if ledger == nil {
		return nil
	}
	ledger.mu.Lock()
	if !ledger.dirty {
		ledger.mu.Unlock()
		return nil
	}
	file := ledgerFile{Version: ledgerVersion, Merged: make(map[string]ledgerEntry, len(ledger.entries))}
	for record, entry := range ledger.entries {
		file.Merged[record] = entry
	}
	ledger.dirty = false
	ledger.mu.Unlock()
	data, err := json.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(ledger.path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(ledger.path), ".review-states-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), ledger.path)
}
