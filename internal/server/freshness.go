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
	"os"
	"path/filepath"
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

// answers says whether this check can serve a request that accepts checks
// begun at floor or later and needs every file when full is set.
func (state *sagaState) answers(floor time.Time, full bool) bool {
	return !state.started.Before(floor) && (state.full || !full)
}

// freshness shares freshness checks between requests. A check answers every
// request that arrived before it started, so concurrent requests wait on one
// walk rather than each taking their own, and none is served a state older
// than itself.
//
// A running server also watches the Saga: a background check every
// pollInterval while reviewers are using it. When one sees a change, what the
// change invalidated is read again in the background, so the page after an
// edit usually finds it already read. A page never waits on the watcher, and
// is never answered by a check older than itself: an edit saved before a
// request arrives is always in its page.
type freshness struct {
	mutex   sync.Mutex
	latest  *sagaState
	pending *pendingCheck
	checks  int
	// asked is when a request last asked, so polling stops while nobody is
	// reviewing.
	asked time.Time
	// sagaHead and sourceHead read the heads of the repositories the Saga
	// and its code references live in.
	sagaHead, sourceHead headReader
}

const (
	// pollInterval is how soon after an edit the watcher starts reading it.
	// Requests take their own checks, so it bounds only how early the
	// reading starts, not what any page shows.
	pollInterval = 500 * time.Millisecond
	// pollIdle is how long after the last request polling continues.
	pollIdle = 2 * time.Minute
)

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
	return a.checkedSince(ctx, arrivedAt(ctx), full, true)
}

// checkedSince is a check begun no earlier than floor. asked says a request
// is asking, rather than the watcher.
func (a *app) checkedSince(ctx context.Context, floor time.Time, full, asked bool) *sagaState {
	f := &a.fresh
	f.mutex.Lock()
	if asked {
		f.asked = time.Now()
	}
	if state := f.latest; state != nil && state.err == nil && state.answers(floor, full) {
		f.mutex.Unlock()
		return state
	}
	if pending := f.pending; pending != nil && !pending.started.Before(floor) && (pending.full || !full) {
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
		state.sagaHead = a.fresh.sagaHead.head(ctx, a.root)
	}()
	go func() {
		defer wait.Done()
		state.sourceHead = a.fresh.sourceHead.head(ctx, a.sourceDir)
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
	listings, err := listSaga(root, full)
	if err != nil {
		return "", "", "", err
	}
	outlineDigest, documentationDigest, filesDigest := sha256.New(), sha256.New(), sha256.New()
	// The digests are taken in the order filepath.WalkDir visits the tree:
	// a directory, then its entries by name, each directory's entries before
	// the next entry. outlineOn and documentationOn say whether the set
	// still reads the directory being visited.
	var visit func(rel string, outlineOn, documentationOn bool)
	visit = func(rel string, outlineOn, documentationOn bool) {
		for _, entry := range listings[rel] {
			path := entry.name
			if rel != "." {
				path = rel + "/" + entry.name
			}
			if entry.dir {
				childOutline := outlineOn && !skipOutlineDirectory(path, entry.name)
				childDocumentation := documentationOn && !skipDocumentationDirectory(entry.name)
				if childOutline {
					fmt.Fprintf(outlineDigest, "d\x00%s\x00", path)
				}
				if childDocumentation {
					fmt.Fprintf(documentationDigest, "d\x00%s\x00", path)
				}
				if childDocumentation || full {
					visit(path, childOutline, childDocumentation)
				}
				continue
			}
			line := func(digest hash.Hash) {
				fmt.Fprintf(digest, "f\x00%s\x00%d\x00%d\x00", path, entry.size, entry.modTime)
			}
			line(filesDigest)
			if documentationOn {
				line(documentationDigest)
			}
			if outlineOn && outlineFile(path, entry.name) {
				line(outlineDigest)
			}
		}
	}
	fmt.Fprintf(outlineDigest, "d\x00.\x00")
	fmt.Fprintf(documentationDigest, "d\x00.\x00")
	visit(".", true, true)
	if !full {
		return hex.EncodeToString(outlineDigest.Sum(nil)), hex.EncodeToString(documentationDigest.Sum(nil)), "", nil
	}
	return hex.EncodeToString(outlineDigest.Sum(nil)), hex.EncodeToString(documentationDigest.Sum(nil)), hex.EncodeToString(filesDigest.Sum(nil)), nil
}

// sagaEntry is one entry of a Saga directory as a check listed it.
type sagaEntry struct {
	name    string
	dir     bool
	size    int64
	modTime int64
}

// sagaListers is how many directories a check lists at once. Listing is
// mostly waiting on the file system, which serves several directories
// together far faster than one after another.
const sagaListers = 8

// readSagaDir and sagaEntryInfo are how a listing reads the file system;
// tests replace them to make an entry vanish at a chosen moment.
var (
	readSagaDir   = os.ReadDir
	sagaEntryInfo = fs.DirEntry.Info
)

// listSaga lists every directory of the Saga, several at once, by its path
// relative to root; the root is ".". Entries are in name order. Unless full is
// set it never enters the directories documentation skips.
//
// A file or directory removed between being listed and being read is left
// out, as a listing taken a moment later would: writers replace files by
// renaming a temporary one over them, and a checkout removes whole trees.
// Any other entry that cannot be read fails the whole listing.
func listSaga(root string, full bool) (map[string][]sagaEntry, error) {
	listings := map[string][]sagaEntry{}
	var (
		mutex    sync.Mutex
		wait     sync.WaitGroup
		firstErr error
	)
	slots := make(chan struct{}, sagaListers)
	var list func(rel string)
	list = func(rel string) {
		defer wait.Done()
		slots <- struct{}{}
		entries, err := readSagaDir(filepath.Join(root, filepath.FromSlash(rel)))
		if rel != "." && errors.Is(err, fs.ErrNotExist) {
			entries, err = nil, nil
		}
		listed := make([]sagaEntry, 0, len(entries))
		for _, entry := range entries {
			if err != nil {
				break
			}
			item := sagaEntry{name: entry.Name(), dir: entry.IsDir()}
			if !item.dir {
				info, infoErr := sagaEntryInfo(entry)
				if errors.Is(infoErr, fs.ErrNotExist) {
					continue
				}
				if err = infoErr; err == nil {
					item.size, item.modTime = info.Size(), info.ModTime().UnixNano()
				}
			}
			listed = append(listed, item)
		}
		<-slots
		mutex.Lock()
		listings[rel] = listed
		if err != nil && firstErr == nil {
			firstErr = err
		}
		failed := firstErr != nil
		mutex.Unlock()
		if failed {
			return
		}
		for _, item := range listed {
			if item.dir && (full || !skipDocumentationDirectory(item.name)) {
				child := item.name
				if rel != "." {
					child = rel + "/" + item.name
				}
				wait.Add(1)
				go list(child)
			}
		}
	}
	wait.Add(1)
	go list(".")
	wait.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return listings, nil
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

// startWatching runs watchSaga until the returned stop is called. stop
// returns only once the watcher, and any page it was rendering, has finished,
// so nothing reads the Saga or starts Git after it.
func (a *app) startWatching(ctx context.Context) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	watching := make(chan struct{})
	go func() {
		defer close(watching)
		a.watchSaga(ctx)
	}()
	return func() {
		cancel()
		<-watching
	}
}

// watchSaga checks the Saga every pollInterval while reviewers are using the
// server, and rebuilds what a change invalidated before the next request
// asks for it. It returns when ctx is done, once its warming has finished.
func (a *app) watchSaga(ctx context.Context) {
	handler := newMux(a)
	warm := make(chan struct{}, 1)
	// The warming goroutine is waited for, so nothing reads the Saga once
	// the watcher has returned.
	var warming sync.WaitGroup
	defer warming.Wait()
	warming.Add(1)
	go func() {
		defer warming.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case <-warm:
				a.warmCaches(ctx, handler)
			}
		}
	}()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	var seen string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		a.fresh.mutex.Lock()
		idle := time.Since(a.fresh.asked) > pollIdle
		a.fresh.mutex.Unlock()
		if idle && seen != "" {
			continue
		}
		state := a.checkedSince(ctx, time.Now(), true, false)
		key, err := state.relatedKey()
		if err != nil {
			continue
		}
		if outline, _ := state.outlineKey(); key+outline != seen {
			seen = key + outline
			select {
			case warm <- struct{}{}:
			default:
			}
		}
	}
}

// warmCaches renders a documentation page nobody asked for, so the page after
// a change finds everything it reads already read: the Saga's files, the
// decks, and the related reviews, which the requirements page reads as every
// feature and story page does. Rendering a real page keeps the warming
// exactly what a page reads.
func (a *app) warmCaches(ctx context.Context, handler http.Handler) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "/requirements", nil)
	if err != nil {
		return
	}
	handler.ServeHTTP(&discardResponse{header: http.Header{}}, request)
}

// discardResponse is a response nobody reads.
type discardResponse struct{ header http.Header }

func (response *discardResponse) Header() http.Header            { return response.header }
func (response *discardResponse) Write(data []byte) (int, error) { return len(data), nil }
func (response *discardResponse) WriteHeader(int)                {}
