package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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
