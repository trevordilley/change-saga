package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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
	if _, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: "core", Title: "Core"}); err != nil {
		t.Fatal(err)
	}
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "base")
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := requirements.AddStory(root, "test", requirements.AddStoryInput{Feature: "core", ID: "sit", RevisionID: "r1", EventID: "proposed",
		Title: "Sit an assessment", Statement: "As a candidate, I sit an assessment", Priority: "high", CreatedAt: created}); err != nil {
		t.Fatal(err)
	}
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.AddTerm(root, "test", requirements.AddTermInput{ID: "testtaker", RevisionID: "r1", EventID: "active", CreatedAt: created,
		TermDefinition: requirements.TermDefinition{Name: "Testtaker", Definition: "One sitting of an assessment, not the person taking it.",
			DefinitionMaturity: requirements.DefinitionMaturityAccepted, ImplementationEvidence: requirements.ImplementationEvidencePresent,
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
	for _, want := range []string{`id="nav-overview"`, `title="Name"`, `href="/terms"`, `href="/terms/testtaker"`, "Assessments"} {
		if !strings.Contains(html, want) {
			t.Fatalf("the overview is missing %s", want)
		}
	}
	for _, absent := range []string{`title="Elevator pitch"`, `title="Description"`, "not written yet"} {
		if strings.Contains(html, absent) {
			t.Fatalf("the overview renders empty part %s", absent)
		}
	}
}

func TestATermPageShowsItsDefinitionStoriesAndCode(t *testing.T) {
	root, repo := termSaga(t)
	html := termPage(t, root, repo, "/terms/testtaker")
	for _, want := range []string{
		"<h1>Testtaker</h1>", "One sitting of an assessment, not the person taking it.", "sitting",
		"Definition maturity", "accepted", "Implementation evidence", "present", "does not prove the concept is implemented", "linked code evidence is current",
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
	if !strings.Contains(stale, "linked code evidence is stale") || !strings.Contains(stale, ">present<") {
		t.Fatalf("stale code must not rewrite evidence availability:\n%s", stale)
	}
}

func TestTermPagesDistinguishObservedGapsUnknownsAndRevisionConflicts(t *testing.T) {
	root, repo := termSaga(t)
	created := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	if _, err := requirements.AddTerm(root, "test", requirements.AddTermInput{ID: "review-annotation", RevisionID: "r1", EventID: "active", CreatedAt: created,
		TermDefinition: requirements.TermDefinition{Name: "Review annotation", Definition: "A note pinned to an exact review Item.",
			DefinitionMaturity: requirements.DefinitionMaturityAccepted, ImplementationEvidence: requirements.ImplementationEvidenceAbsent,
			Stories: []string{"urn:change-saga:test:story:sit"}}}); err != nil {
		t.Fatal(err)
	}
	intent := termPage(t, root, repo, "/terms/review-annotation")
	for _, want := range []string{"accepted", ">absent<", "Observed implementation gap", "No code references this term yet"} {
		if !strings.Contains(intent, want) {
			t.Fatalf("intent-only term is missing %q:\n%s", want, intent)
		}
	}
	if index := termPage(t, root, repo, "/terms"); !strings.Contains(index, "absent — observed gap") {
		t.Fatalf("term directory does not distinguish the observed gap:\n%s", index)
	}
	if _, err := requirements.AddTerm(root, "test", requirements.AddTermInput{ID: "legacy-meaning", RevisionID: "r1", EventID: "active", CreatedAt: created,
		TermDefinition: requirements.TermDefinition{Name: "Legacy meaning", Definition: "A definition from before semantic axes existed."}}); err != nil {
		t.Fatal(err)
	}
	unknown := termPage(t, root, repo, "/terms/legacy-meaning")
	if !strings.Contains(unknown, "unknown — not assessed") || !strings.Contains(unknown, "Unverified: no implementation-evidence assessment") || strings.Contains(unknown, "Observed implementation gap") {
		t.Fatalf("unknown must remain unverified, not an observed gap:\n%s", unknown)
	}

	termURN := "urn:change-saga:test:term:review-annotation"
	if _, err := requirements.ReviseTerm(root, "test", requirements.ReviseTermInput{Term: termURN, ID: "r2", Parents: []string{termURN + ":revision:r1"}, CreatedAt: created,
		TermDefinition: requirements.TermDefinition{Name: "Review annotation", Definition: "A proposed competing meaning.", DefinitionMaturity: requirements.DefinitionMaturityProposed, ImplementationEvidence: requirements.ImplementationEvidencePartial}}); err != nil {
		t.Fatal(err)
	}
	r2Path := filepath.Join(root, "___overview", "terms", "review-annotation.term", "revisions", "r2.json")
	data, err := os.ReadFile(r2Path)
	if err != nil {
		t.Fatal(err)
	}
	r3 := strings.Replace(string(data), `"id": "r2"`, `"id": "r3"`, 1)
	if r3 == string(data) {
		t.Fatalf("could not derive competing revision from:\n%s", data)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(r2Path), "r3.json"), []byte(r3), 0o644); err != nil {
		t.Fatal(err)
	}
	conflict := termPage(t, root, repo, "/terms/review-annotation")
	for _, want := range []string{"Conflicting term revisions", "unknown — not assessed", "competing revision heads must be reconciled", "No current definition is available"} {
		if !strings.Contains(conflict, want) {
			t.Fatalf("conflicted term is missing %q:\n%s", want, conflict)
		}
	}
	index := termPage(t, root, repo, "/terms")
	for _, want := range []string{"Definition maturity", "Implementation evidence", "unknown — revision conflict", "unknown — unverified"} {
		if !strings.Contains(index, want) {
			t.Fatalf("term directory is missing %q:\n%s", want, index)
		}
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

// The terms table places each term's code at the head. That follows from the
// head commit and the references alone, so it is kept under both: an
// unchanged head reuses it, and a new commit places the code again.
func TestTermPlacesAreKeptPerHeadAndReferences(t *testing.T) {
	root, repo := termSaga(t)
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	document, err := requirements.Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	first := application.termPlaces(t.Context(), document)
	if len(first["testtaker"]) != 1 || first["testtaker"][0].Stale {
		t.Fatalf("the term's code was not placed: %#v", first)
	}
	firstKey := application.termPlacesCache.key
	second := application.termPlaces(t.Context(), document)
	if application.termPlacesCache.key != firstKey || reflect.ValueOf(second).Pointer() != reflect.ValueOf(first).Pointer() {
		t.Fatal("an unchanged head and unchanged references placed the code again")
	}
	// A new commit that rewrites the constant makes the reference stale.
	writeServerFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(serverKinds, `"testtaker"`, `"candidate"`, 1))
	serverGit(t, repo, "commit", "-am", "rename")
	moved := application.termPlaces(t.Context(), document)
	if application.termPlacesCache.key == firstKey || len(moved["testtaker"]) != 1 || !moved["testtaker"][0].Stale {
		t.Fatalf("a new head did not place the code again: %#v", moved)
	}
}

// A place that rests on a pinned commit the repository lacks may be answered
// differently after a fetch, so it is shown but not kept.
func TestTermPlacesKeepNoProvisionalAnswer(t *testing.T) {
	root, repo := termSaga(t)
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	document, err := requirements.Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	for index := range document.Terms {
		if revision := document.Terms[index].CurrentRevision; revision != nil {
			for code := range revision.Code {
				revision.Code[code].Commit = strings.Repeat("0", 40)
			}
		}
	}
	// The code is still found, by its content, but only provisionally.
	if places := application.termPlaces(t.Context(), document); len(places["testtaker"]) != 1 {
		t.Fatalf("a reference to a missing commit was not placed: %#v", places)
	}
	if application.termPlacesCache.key != "" {
		t.Fatal("a provisional answer was kept")
	}
}
