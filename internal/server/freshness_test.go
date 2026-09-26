package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A page asks several caches whether the Saga changed. They share one check,
// so a page walks the Saga once however many caches it reads.
func TestOnePageTakesOneFreshnessCheck(t *testing.T) {
	fixture, _, _ := boundedFixture(t)
	application := &app{root: fixture.Root, sourceDir: fixture.Repository, template: serverTemplate(t)}
	handler := newMux(application)
	for want, path := range []string{"/", "/requirements", "/"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
		}
		if application.fresh.checks != want+1 {
			t.Fatalf("after GET %s the Saga was checked %d times, want %d", path, application.fresh.checks, want+1)
		}
	}
}

// A check answers only the requests that arrived before it began, so no
// request is served a state older than itself, and a request that needs every
// file is never answered by a check that skipped some.
func TestAFreshnessCheckAnswersOnlyRequestsBeforeIt(t *testing.T) {
	root := validServerSaga(t)
	application := &app{root: root, sourceDir: root}
	at := func(arrived time.Time) context.Context {
		return context.WithValue(context.Background(), arrivalKey{}, arrived)
	}
	arrived := time.Now()
	shell := application.sagaState(at(arrived), false)
	if again := application.sagaState(at(arrived), false); again != shell {
		t.Fatal("a second cache of the same request took its own check")
	}
	if _, err := shell.filesKey(); err == nil {
		t.Fatal("a shell check claimed to fingerprint every file")
	}
	full := application.sagaState(at(arrived), true)
	if full == shell || !full.full {
		t.Fatal("a request that reads every file was answered by a shell check")
	}
	if again := application.sagaState(at(arrived), false); again != full {
		t.Fatal("a full check did not answer the shell caches of the same request")
	}
	writeServerFile(t, filepath.Join(root, "README.md"), "An edit the next request must see.\n")
	later := application.sagaState(at(time.Now()), false)
	if later == full || later.documentation == full.documentation {
		t.Fatal("a request after an edit was answered by a check from before it")
	}
}

// A shell check never enters code evidence, and a full check commits to it.
func TestTheShellCheckSkipsCodeEvidence(t *testing.T) {
	root := validServerSaga(t)
	evidence := filepath.Join(root, saga.CodeDirName, "record.json")
	writeServerFile(t, evidence, "{}\n")
	outline, documentation, _, err := sagaFingerprints(root, false)
	if err != nil {
		t.Fatal(err)
	}
	_, _, files, err := sagaFingerprints(root, true)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, evidence, "{\"changed\":true}\n")
	outlineAfter, documentationAfter, _, err := sagaFingerprints(root, false)
	if err != nil {
		t.Fatal(err)
	}
	_, _, filesAfter, err := sagaFingerprints(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if outline != outlineAfter || documentation != documentationAfter {
		t.Fatal("an evidence edit changed what the shell reads")
	}
	if files == filesAfter {
		t.Fatal("an evidence edit left every-file fingerprint unchanged")
	}
}

// A running server reads the Saga, related reviews included, before anyone
// asks, and again after an edit, before anyone asks for it.
func TestAWatchedSagaIsReadBeforeAnyoneAsks(t *testing.T) {
	fixture := newServerReviewFixture(t)
	documentTheFixture(t, fixture)
	application, handler := reviewApp(t, fixture, gitdiff.Range{})
	// The watcher has stopped reading the Saga before its directory is
	// removed.
	defer application.startWatching(context.Background())()
	builds := func() int {
		application.related.mutex.Lock()
		defer application.related.mutex.Unlock()
		return application.related.builds
	}
	waitFor := func(what string, done func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for !done() {
			if time.Now().After(deadline) {
				t.Fatalf("the watched Saga never %s", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor("built its related reviews", func() bool { return builds() == 1 })
	path := requirementStoryHref("place-an-order")
	if body := documentationPage(t, handler, path); !strings.Contains(body, `data-related-review="pr-7"`) {
		t.Fatalf("%s lost its related review to the background build", path)
	}
	if builds() != 1 {
		t.Fatalf("the page rebuilt the related reviews the watcher had built: %d builds", builds())
	}
	storyFile := filepath.Join(serverFeatureDir(fixture.root), applayout.RequirementsDir, "stories", "place-an-order.story", "story.json")
	info, err := os.Stat(storyFile)
	if err != nil {
		t.Fatal(err)
	}
	later := info.ModTime().Add(time.Second)
	if err := os.Chtimes(storyFile, later, later); err != nil {
		t.Fatal(err)
	}
	waitFor("rebuilt after an edit", func() bool { return builds() == 2 })
}

// Every page shows the same decks, so their view is built once for each state
// of the Saga's files and never outlives it.
func TestTheDecksAreBuiltOncePerStateOfTheSaga(t *testing.T) {
	requireDogfoodSaga(t)
	tmpl, err := newPageTemplateFor(gitdiff.Range{})
	if err != nil {
		t.Fatal(err)
	}
	application := &app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl}
	handler := newMux(application)
	dogfoodGet := func(path string) {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, recorder.Code)
		}
	}
	dogfoodGet("/")
	files := application.files.current
	if files == nil || files.slidesView == nil {
		t.Fatal("the app Saga's decks were not built through the Saga files")
	}
	built := files.slidesView
	dogfoodGet("/features")
	if application.files.current != files || files.slidesView != built {
		t.Fatal("a second page built the unchanged decks again")
	}
}

// Several directories are listed at once, and the fingerprints are exactly
// those one walk of the Saga in name order takes.
func TestTheListedFingerprintsAreTheWalkedOnes(t *testing.T) {
	roots := []string{validServerSaga(t)}
	if _, err := os.Stat(filepath.Join(dogfoodSaga, saga.ManifestName)); err == nil {
		roots = append(roots, dogfoodSaga)
	}
	fixture, _, _ := boundedFixture(t)
	roots = append(roots, fixture.Root)
	for _, root := range roots {
		for _, full := range []bool{false, true} {
			outline, documentation, files, err := sagaFingerprints(root, full)
			if err != nil {
				t.Fatal(err)
			}
			wantOutline, wantDocumentation, wantFiles, err := walkedSagaFingerprints(root, full)
			if err != nil {
				t.Fatal(err)
			}
			if outline != wantOutline || documentation != wantDocumentation || files != wantFiles {
				t.Fatalf("%s (full %v): listed fingerprints differ from the walked ones", root, full)
			}
		}
	}
	if _, _, _, err := sagaFingerprints(filepath.Join(t.TempDir(), "missing"), true); err == nil {
		t.Fatal("a missing Saga was fingerprinted")
	}
}

// walkedSagaFingerprints is one sequential walk of the Saga, the reference
// the listed fingerprints are held to.
func walkedSagaFingerprints(root string, full bool) (outline, documentation, files string, err error) {
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

// A file replaced by renaming a temporary one over it, or a directory a
// checkout removes, can vanish between being listed and being read. The check
// leaves it out, as a listing a moment later would, rather than failing every
// page waiting on it.
func TestAnEntryThatVanishesMidCheckIsLeftOut(t *testing.T) {
	root := validServerSaga(t)
	writeServerFile(t, filepath.Join(root, "notes", "kept.md"), "kept\n")
	outline, documentation, files, err := sagaFingerprints(root, true)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, filepath.Join(root, ".change-saga-write-1"), "being renamed\n")
	writeServerFile(t, filepath.Join(root, "gone", "file.md"), "being removed\n")
	realRead, realInfo := readSagaDir, sagaEntryInfo
	t.Cleanup(func() { readSagaDir, sagaEntryInfo = realRead, realInfo })
	sagaEntryInfo = func(entry fs.DirEntry) (fs.FileInfo, error) {
		if entry.Name() == ".change-saga-write-1" {
			return nil, fs.ErrNotExist
		}
		return realInfo(entry)
	}
	readSagaDir = func(dir string) ([]fs.DirEntry, error) {
		if filepath.Base(dir) == "gone" {
			return nil, fs.ErrNotExist
		}
		return realRead(dir)
	}
	_, _, vanishedFiles, err := sagaFingerprints(root, true)
	if err != nil {
		t.Fatalf("a vanished entry failed the check: %v", err)
	}
	// The files that vanished are simply not there: every file is
	// fingerprinted as before they were written. (The emptied directory
	// itself was listed, so the directory-aware sets name it.)
	if vanishedFiles != files {
		t.Fatal("a vanished file was fingerprinted as if it were still there")
	}
	_, _ = outline, documentation
}

// Stopping the watcher waits for it: once stop returns, it checks nothing
// more and renders nothing more.
func TestStoppingTheWatcherWaitsForIt(t *testing.T) {
	fixture, _, _ := boundedFixture(t)
	application := &app{root: fixture.Root, sourceDir: fixture.Repository, template: serverTemplate(t)}
	stop := application.startWatching(context.Background())
	deadline := time.Now().Add(30 * time.Second)
	for {
		application.fresh.mutex.Lock()
		checks := application.fresh.checks
		application.fresh.mutex.Unlock()
		if checks > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the watcher never checked the Saga")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	application.fresh.mutex.Lock()
	stopped := application.fresh.checks
	application.fresh.mutex.Unlock()
	time.Sleep(3 * pollInterval)
	application.fresh.mutex.Lock()
	defer application.fresh.mutex.Unlock()
	if application.fresh.checks != stopped || application.fresh.pending != nil {
		t.Fatalf("the watcher kept checking after it was stopped: %d checks, then %d", stopped, application.fresh.checks)
	}
}
