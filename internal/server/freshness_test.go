package server

import (
	"context"
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

// Within a running server's window a recent check answers the request, and
// the server's own write is seen by the very next request however recent
// that check was.
func TestTheServersOwnWriteIsSeenAtOnce(t *testing.T) {
	fixture, _, _ := boundedFixture(t)
	application := &app{root: fixture.Root, sourceDir: fixture.Repository, template: serverTemplate(t)}
	application.fresh.window = time.Hour
	handler := newMux(application)
	open := func() {
		t.Helper()
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET / = %d: %s", recorder.Code, recorder.Body.String())
		}
	}
	open()
	open()
	if application.fresh.checks != 1 {
		t.Fatalf("a request inside the window took its own check: %d checks", application.fresh.checks)
	}
	first := application.files.current
	writeServerFile(t, filepath.Join(fixture.Root, "README.md"), "Written by the server.\n")
	application.fresh.wrote()
	open()
	if application.fresh.checks != 2 || application.files.current == first {
		t.Fatalf("the request after the server's write was answered by a check from before it: %d checks", application.fresh.checks)
	}
}

// A running server reads the Saga, related reviews included, before anyone
// asks, and again after an edit made outside it, within the window.
func TestAWatchedSagaIsReadBeforeAnyoneAsks(t *testing.T) {
	fixture := newServerReviewFixture(t)
	documentTheFixture(t, fixture)
	application, handler := reviewApp(t, fixture, gitdiff.Range{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go application.watchSaga(ctx)
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
