package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

const serverKinds = "package assessment\n\ntype Kind string\n\nconst (\n\tKindTesttaker Kind = \"testtaker\"\n\tKindProctor   Kind = \"proctor\"\n)\n"

// termSaga is a code repository holding kinds.go and an app Saga with one
// story and the term Testtaker, whose code is the KindTesttaker constant.
func termSaga(t *testing.T) (root, repo string) {
	t.Helper()
	repo = t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Test")
	serverGit(t, repo, "config", "user.email", "test@example.test")
	writeServerFile(t, filepath.Join(repo, "kinds.go"), serverKinds)
	root = filepath.Join(repo, "app.saga")
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"test","title":"Assessments","source":{"repository":"https://example.test/a.git"}}`)
	if _, err := applayout.WriteEpic(root, applayout.EpicManifest{ID: "core", Title: "Core"}); err != nil {
		t.Fatal(err)
	}
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "base")
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := requirements.AddStory(root, "test", requirements.AddStoryInput{Epic: "core", ID: "sit", RevisionID: "r1", EventID: "proposed",
		Title: "Sit an assessment", Statement: "As a candidate, I sit an assessment", Priority: "high", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.AddTerm(root, "test", requirements.AddTermInput{ID: "testtaker", RevisionID: "r1", EventID: "active", CreatedAt: created,
		TermDefinition: requirements.TermDefinition{Name: "Testtaker", Definition: "One sitting of an assessment, not the person taking it.",
			Aliases: []string{"sitting"}, Stories: []string{"urn:change-saga:test:story:sit"},
			Code: []coderef.Reference{{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest}}}}); err != nil {
		t.Fatal(err)
	}
	return root, repo
}

func termPage(t *testing.T, root, repo, path string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func TestTheOverviewExpandsToItsPartsAndEveryTerm(t *testing.T) {
	root, repo := termSaga(t)
	html := termPage(t, root, repo, "/")
	for _, want := range []string{`id="nav-overview"`, `title="Name"`, `title="Elevator pitch"`, `title="Description"`, `href="/terms"`, `href="/terms/testtaker"`, "Assessments"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the overview is missing %s", want)
		}
	}
	if strings.Count(html, "not written yet") != 2 {
		t.Fatal("the absent pitch and description are stated gaps")
	}
}

func TestATermPageShowsItsDefinitionStoriesAndCode(t *testing.T) {
	root, repo := termSaga(t)
	html := termPage(t, root, repo, "/terms/testtaker")
	for _, want := range []string{
		"<h1>Testtaker</h1>", "One sitting of an assessment, not the person taking it.", "sitting",
		`href="/requirements/sit"`, "Sit an assessment",
		`data-file-path="kinds.go"`, `<tr class="referenced"><th scope="row">6</th><td><code data-code>	KindTesttaker Kind = &#34;testtaker&#34;</code>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("the term page is missing %s:\n%s", want, html)
		}
	}
	for _, absent := range []string{"approve", "data-comment", "request-changes"} {
		if strings.Contains(strings.ToLower(html[strings.Index(html, "data-terms-page"):]), absent) {
			t.Fatalf("documentation carries no %s control", absent)
		}
	}
	if index := termPage(t, root, repo, "/terms"); !strings.Contains(index, "Terms and vocabulary</h1>") || !strings.Contains(index, `href="/terms/testtaker"`) {
		t.Fatal("the vocabulary page lists every term")
	}
	recorder := httptest.NewRecorder()
	newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/terms/missing", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("an unknown term = %d", recorder.Code)
	}

	// Renaming the constant leaves the term's code stale: the page says so
	// and shows the code as it was pinned.
	writeServerFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(serverKinds, "KindTesttaker", "KindCandidate", 1))
	serverGit(t, repo, "commit", "-am", "rename")
	stale := termPage(t, root, repo, "/terms/testtaker")
	if !strings.Contains(stale, `class="term-code stale"`) || !strings.Contains(stale, "KindTesttaker") || !strings.Contains(stale, "changed after the term was written") {
		t.Fatalf("a renamed constant must show the term's code as stale:\n%s", stale)
	}
}

func TestAStoryPageLinksBackToItsTerms(t *testing.T) {
	root, repo := termSaga(t)
	html := termPage(t, root, repo, "/requirements/sit")
	if !strings.Contains(html, `data-requirement-terms`) || !strings.Contains(html, `<a href="/terms/testtaker" data-term-target="urn:change-saga:test:term:testtaker">Testtaker</a>`) {
		t.Fatalf("the story page does not link to its term:\n%s", html)
	}
	if _, err := os.Stat(filepath.Join(root, "___overview", "terms", "testtaker.term")); err != nil {
		t.Fatal(err)
	}
}

func TestTheCodeViewNamesTheTermsAFileDefines(t *testing.T) {
	root, repo := termSaga(t)
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "saga")
	serverGit(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	serverGit(t, repo, "checkout", "-b", "grader")
	writeServerFile(t, filepath.Join(repo, "kinds.go"), serverKinds+"\n// graders arrive next\n")
	serverGit(t, repo, "commit", "-am", "note")
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t), rng: gitdiff.Range{Against: "main"}}
	recorder := httptest.NewRecorder()
	newMux(application).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/file-owners?file=kinds.go", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `<a href="/terms/testtaker" data-term-target="urn:change-saga:test:term:testtaker"`) {
		t.Fatalf("file owners = %d %s", recorder.Code, recorder.Body.String())
	}
	if href := recordHref(&saga.Saga{Manifest: saga.Manifest{ID: "test"}}, "urn:change-saga:test:term:testtaker"); href != "/terms/testtaker" {
		t.Fatalf("a review Item's term record opens the term page: %s", href)
	}
}
