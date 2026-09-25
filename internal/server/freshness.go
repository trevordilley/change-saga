package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
)

// sagaState is what one freshness check saw: a fingerprint of the Saga's
// files as each cache reads them, and the heads of the repositories the
// Saga and its code references live in. Every cache is keyed on the part
// that covers what it read, so one walk of the Saga answers all of them.
type sagaState struct {
	// outline commits to what LoadOutline reads, documentation to what the
	// documentation pages read, and files to every file of the Saga. files
	// is taken only by a full check.
	outline       string
	documentation string
	files         string
	full          bool
	sagaHead      string
	sourceHead    string
	err           error
	// started is when the check began. Every file it fingerprints was read
	// after then, so it answers for any request that arrived before it.
	started time.Time
}

// outlineKey is the outline cache's key: what LoadOutline reads, and the
// head of the repository the Saga is committed to.
func (state *sagaState) outlineKey() (string, error) {
	if state.err != nil {
		return "", state.err
	}
	return state.outline + "\x00" + state.sagaHead, nil
}

// documentationKey is the documentation files' key.
func (state *sagaState) documentationKey() (string, error) {
	return state.documentation, state.err
}

// filesKey commits to every file of the Saga. It needs a full check.
func (state *sagaState) filesKey() (string, error) {
	if !state.full && state.err == nil {
		return "", errors.New("a shell check does not fingerprint every file")
	}
	return state.files, state.err
}

// relatedKey commits to everything the related-review intersection reads:
// every file of the Saga and the source head its references resolve against.
func (state *sagaState) relatedKey() (string, error) {
	files, err := state.filesKey()
	if err != nil {
		return "", err
	}
	return files + "\x00" + state.sourceHead, nil
}

// answers says whether this check can serve a request that arrived at
// arrived and needs every file when full is set.
func (state *sagaState) answers(arrived time.Time, full bool) bool {
	return !state.started.Before(arrived) && (state.full || !full)
}

// freshness shares freshness checks between requests. A check answers every
// request that arrived before it started, so concurrent requests wait on one
// walk rather than each taking their own, and none is served a state older
// than itself.
type freshness struct {
	mutex   sync.Mutex
	latest  *sagaState
	pending *pendingCheck
	checks  int
}

type pendingCheck struct {
	started time.Time
	full    bool
	done    chan struct{}
	state   *sagaState
}

// arrivalKey carries when a request arrived, so every cache a request asks
// shares the check that answers it.
type arrivalKey struct{}

// arriving stamps a request with its arrival.
func arriving(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r.WithContext(context.WithValue(r.Context(), arrivalKey{}, time.Now())))
	}
}

// arrivedAt is when the request behind ctx arrived, or now for a caller
// outside a request.
func arrivedAt(ctx context.Context) time.Time {
	if arrived, ok := ctx.Value(arrivalKey{}).(time.Time); ok {
		return arrived
	}
	return time.Now()
}

// sagaState is the Saga's state as of this request: taken by a check that
// began no earlier than the request itself. A shell check skips code
// evidence, claims, and verifications, so the pages that read none of them do
// not scale with them; full also fingerprints every file.
func (a *app) sagaState(ctx context.Context, full bool) *sagaState {
	arrived := arrivedAt(ctx)
	f := &a.fresh
	f.mutex.Lock()
	if state := f.latest; state != nil && state.err == nil && state.answers(arrived, full) {
		f.mutex.Unlock()
		return state
	}
	if pending := f.pending; pending != nil && !pending.started.Before(arrived) && (pending.full || !full) {
		f.mutex.Unlock()
		<-pending.done
		return pending.state
	}
	pending := &pendingCheck{started: time.Now(), full: full, done: make(chan struct{})}
	f.pending = pending
	f.checks++
	f.mutex.Unlock()
	// The check is shared, so one request going away must not cut it short
	// for the others waiting on it.
	pending.state = a.checkSaga(context.WithoutCancel(ctx), pending.started, full)
	f.mutex.Lock()
	if f.pending == pending {
		f.pending = nil
	}
	if f.latest == nil || f.latest.started.Before(pending.started) || pending.full && !f.latest.full {
		f.latest = pending.state
	}
	f.mutex.Unlock()
	close(pending.done)
	return pending.state
}

// checkSaga fingerprints the Saga's files in one walk while the two heads are
// read beside it.
func (a *app) checkSaga(ctx context.Context, started time.Time, full bool) *sagaState {
	state := &sagaState{started: started, full: full}
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		state.sagaHead, _ = gitOutput(ctx, a.root, "rev-parse", "HEAD")
	}()
	go func() {
		defer wait.Done()
		state.sourceHead, _ = gitOutput(ctx, a.sourceDir, "rev-parse", "HEAD")
	}()
	state.outline, state.documentation, state.files, state.err = sagaFingerprints(a.root, full)
	wait.Wait()
	return state
}

// sagaFingerprints walks the Saga once and commits, by path, size, and
// modification time, to three sets of its files:
//
//   - outline: only the files LoadOutline consumes. Code evidence, reviews,
//     claims, verifications, landmarks, and fragment bodies are skipped, so the
//     shell does not scale with per-line evidence or attachment size.
//   - documentation: everything but code evidence, claims, and verifications,
//     which the narrative leaves unopened and the records, test cases, and
//     inventory do not live in.
//   - files: every file of the Saga, code references and reviews included.
//     Only a full walk takes it; otherwise the walk never enters the
//     directories documentation skips.
func sagaFingerprints(root string, full bool) (outline, documentation, files string, err error) {
	outlineDigest, documentationDigest, filesDigest := sha256.New(), sha256.New(), sha256.New()
	// Each set skips whole directories. The walk visits a directory's
	// entries before any path after it, so a skipped directory is left the
	// first time a path falls outside it.
	var outlineSkip, documentationSkip string
	inside := func(skip, rel string) bool {
		return skip != "" && strings.HasPrefix(rel, skip+"/")
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !inside(outlineSkip, rel) {
			outlineSkip = ""
		}
		if !inside(documentationSkip, rel) {
			documentationSkip = ""
		}
		if entry.IsDir() {
			if outlineSkip == "" {
				if path != root && skipOutlineDirectory(rel, entry.Name()) {
					outlineSkip = rel
				} else {
					fmt.Fprintf(outlineDigest, "d\x00%s\x00", rel)
				}
			}
			if documentationSkip == "" {
				if path != root && skipDocumentationDirectory(entry.Name()) {
					if !full {
						return filepath.SkipDir
					}
					documentationSkip = rel
				} else {
					fmt.Fprintf(documentationDigest, "d\x00%s\x00", rel)
				}
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		line := func(digest hash.Hash) {
			fmt.Fprintf(digest, "f\x00%s\x00%d\x00%d\x00", rel, info.Size(), info.ModTime().UnixNano())
		}
		line(filesDigest)
		if documentationSkip == "" {
			line(documentationDigest)
		}
		if outlineSkip == "" && outlineFile(rel, entry.Name()) {
			line(outlineDigest)
		}
		return nil
	})
	if err != nil {
		return "", "", "", err
	}
	if !full {
		return hex.EncodeToString(outlineDigest.Sum(nil)), hex.EncodeToString(documentationDigest.Sum(nil)), "", nil
	}
	return hex.EncodeToString(outlineDigest.Sum(nil)), hex.EncodeToString(documentationDigest.Sum(nil)), hex.EncodeToString(filesDigest.Sum(nil)), nil
}

// skipDocumentationDirectory names the directories no documentation page
// reads.
func skipDocumentationDirectory(base string) bool {
	switch base {
	case saga.CodeDirName, "___claims", "___verifications":
		return true
	}
	return false
}
