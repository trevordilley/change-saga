package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

type serverReviewFixture struct {
	repo, root, head string
}

func writeServerJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, path, string(data))
}

// newServerReviewFixture is a repository whose feature branch changes one
// file, holding an app Saga with a review of that branch: one slide whose
// Item references the changed line and points at an epic.
func newServerReviewFixture(t *testing.T) serverReviewFixture {
	t.Helper()
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Dev")
	serverGit(t, repo, "config", "user.email", "dev@example.test")
	serverGit(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeServerFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"sqs\" }\n")
	root := filepath.Join(repo, "app.saga")
	writeServerJSON(t, filepath.Join(root, saga.ManifestName), saga.Manifest{Schema: saga.SagaSchemaURL, Version: saga.SagaVersion, ID: "app", Title: "App", Source: saga.Source{Repository: "https://example.test/acme/app.git"}})
	writeServerEpic(t, root)
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "base")
	serverGit(t, repo, "checkout", "-b", "feature/pg")
	writeServerFile(t, filepath.Join(repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres\" }\n")
	serverGit(t, repo, "commit", "-am", "Move the queue to Postgres")
	head := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))

	if err := reviewstore.Create(root, saga.ReviewManifest{ID: "pr-7", Title: "Move the queue to Postgres", Base: "main", Head: "feature/pg", PullRequest: &saga.PullRequest{Number: 7, URL: "https://github.com/acme/app/pull/7"}}, saga.DeckManifest{ID: "pr-7", Title: "PR 7", Objective: "Explain the move."}); err != nil {
		t.Fatal(err)
	}
	deckDir := filepath.Join(saga.ReviewDir(root, "pr-7"), saga.ReviewDeckDir)
	deck, slide, item := saga.ReviewDeckTarget("app", "pr-7", "pr-7"), saga.ReviewSlideTarget("app", "pr-7", "queue"), saga.ReviewItemTarget("app", "pr-7", "queue", "node")
	slideName, _ := saga.FlatSlideFilename(deck, slide, 0)
	asset, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeServerJSON(t, filepath.Join(deckDir, slideName), saga.SlideManifest{Version: saga.DeckRecordVersion, ID: "queue", DeckID: "pr-7", Title: "Queue moves to Postgres", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Entrypoint: asset, Takeaway: "One transaction.", ReadingOrder: []string{"node"}})
	writeServerFile(t, filepath.Join(deckDir, asset), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect id="node" width="5" height="5"/></svg>`)
	itemName, _ := saga.FlatItemFilename(slide, item, 0)
	writeServerJSON(t, filepath.Join(deckDir, itemName), saga.ItemManifest{Version: saga.DeckRecordVersion, ID: "node", SlideID: "queue", Kind: "node", Label: "Enqueue", Description: "Enqueue writes to Postgres.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "node"}, Record: "urn:change-saga:app:epic:" + serverEpic})
	resolver, err := coderesolve.New(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(context.Background(), coderef.Location{Commit: head, Path: "queue.go", Start: 3, End: 3}, "")
	if err != nil {
		t.Fatal(err)
	}
	writeServerJSON(t, filepath.Join(deckDir, saga.FlatEvidenceFilename(item, "queue")), saga.CodeFile{Version: saga.CurrentVersion, References: []coderef.Reference{reference}})
	if _, validation, err := saga.Load(root); err != nil || !validation.Valid {
		t.Fatalf("fixture: %v %#v", err, validation.Issues)
	}
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "Review deck")
	return serverReviewFixture{repo: repo, root: root, head: strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))}
}

func reviewApp(t *testing.T, fixture serverReviewFixture, rng gitdiff.Range) (*app, http.Handler) {
	t.Helper()
	tmpl, err := newPageTemplateFor(rng)
	if err != nil {
		t.Fatal(err)
	}
	application := &app{root: fixture.root, sourceDir: fixture.repo, rng: rng, template: tmpl, mutationToken: "review-token"}
	return application, newMux(application)
}

func postReview(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestReviewPageShowsDiffsDecisionsAndCurrency(t *testing.T) {
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	if refused := postReview(t, handler, "/reviews/pr-7/decision", url.Values{"slide": {"queue"}, "state": {"approved"}}); refused.Code != http.StatusForbidden {
		t.Fatalf("a decision without the session token = %d", refused.Code)
	}
	if entries, _ := os.ReadDir(filepath.Join(saga.ReviewDir(fixture.root, "pr-7"), saga.ReviewApprovalsDir)); len(entries) != 0 {
		t.Fatal("a refused decision wrote a record")
	}
	approved := postReview(t, handler, "/reviews/pr-7/decision", url.Values{"token": {"review-token"}, "slide": {"queue"}, "state": {"approved"}, "body": {"Clear"}})
	if approved.Code != http.StatusSeeOther {
		t.Fatalf("approve = %d %s", approved.Code, approved.Body.String())
	}
	commented := postReview(t, handler, "/reviews/pr-7/comment", url.Values{"token": {"review-token"}, "target": {"queue/node"}, "body": {"Index on status?"}})
	if commented.Code != http.StatusSeeOther {
		t.Fatalf("comment = %d %s", commented.Code, commented.Body.String())
	}
	if documentation := postReview(t, handler, "/reviews/pr-7/comment", url.Values{"token": {"review-token"}, "target": {saga.FragmentTarget("app", "overview")}, "body": {"no"}}); documentation.Code != http.StatusBadRequest {
		t.Fatalf("a comment on documentation = %d", documentation.Code)
	}

	page := getPage(t, handler, "/reviews/pr-7")
	body := page.Body.String()
	for _, want := range []string{
		`data-review-slide="queue"`, `data-decision-state="approved" data-currency="current"`, `data-review-decision-form="queue"`,
		`data-review-diff=`, `review-line add`, `postgres`, `review-line del`, `sqs`,
		`data-review-record="urn:change-saga:app:epic:` + serverEpic + `"`, `Index on status?`, `/reviews/pr-7/visual/queue`,
		`https://github.com/acme/app/pull/7`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review page is missing %q:\n%s", want, body)
		}
	}
	if visual := getPage(t, handler, "/reviews/pr-7/visual/queue"); !strings.Contains(visual.Body.String(), `<rect id="node"`) {
		t.Fatalf("slide visual = %s", visual.Body.String())
	}

	// A push that changes the referenced code marks the approval out of date.
	serverGit(t, fixture.repo, "add", ".")
	serverGit(t, fixture.repo, "commit", "-m", "decisions")
	writeServerFile(t, filepath.Join(fixture.repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"postgres-v2\" }\n")
	serverGit(t, fixture.repo, "commit", "-am", "rename")
	body = getPage(t, handler, "/reviews/pr-7").Body.String()
	if !strings.Contains(body, `data-currency="out_of_date"`) || !strings.Contains(body, `data-out-of-date>Out of date`) || !strings.Contains(body, "queue.go lines 3-3 changed") {
		t.Fatalf("the approval was not marked out of date:\n%s", body)
	}
	index := getPage(t, handler, "/reviews").Body.String()
	if !strings.Contains(index, `data-review-summary="pr-7"`) || !strings.Contains(index, `data-currency="out_of_date"`) {
		t.Fatalf("review index = %s", index)
	}
}

// Comparing, the Change tab shows the living layers read-only beside the
// review of the compared head, and the documentation offers no approval.
func TestComparingShowsTheMatchingReviewBesideTheLayers(t *testing.T) {
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{Against: "main", Head: "feature/pg"})
	change := getPage(t, handler, "/api/change")
	for attempt := 0; change.Code == http.StatusAccepted && attempt < 200; attempt++ {
		change = getPage(t, handler, "/api/change")
	}
	body := change.Body.String()
	if change.Code != http.StatusOK || !strings.Contains(body, `data-change-review`) || !strings.Contains(body, `data-review-summary="pr-7"`) || !strings.Contains(body, `href="/reviews/pr-7"`) {
		t.Fatalf("the Change tab did not show the matching review: %d\n%s", change.Code, body)
	}
	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	root := rootResponse.Body.String()
	for _, control := range []string{"data-review-decision", "data-review-comment", "/api/review", "/api/thread"} {
		if strings.Contains(root, control) {
			t.Fatalf("the documentation offered %q", control)
		}
	}
	if !strings.Contains(root, `href="/reviews"`) {
		t.Fatal("the Saga does not link to its reviews")
	}
}

func TestFrozenReviewIsViewableButTakesNoDecisions(t *testing.T) {
	fixture := newServerReviewFixture(t)
	if err := reviewstore.Freeze(fixture.root, "pr-7", saga.ReviewMerge{Base: strings.TrimSpace(serverGit(t, fixture.repo, "rev-parse", "main")), Head: fixture.head, Landed: fixture.head, MergedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	body := getPage(t, handler, "/reviews/pr-7").Body.String()
	if !strings.Contains(body, "This review is history") || strings.Contains(body, "data-review-decision-form") || !strings.Contains(body, "review-line add") {
		t.Fatalf("frozen review page:\n%s", body)
	}
	if refused := postReview(t, handler, "/reviews/pr-7/decision", url.Values{"token": {"review-token"}, "slide": {"queue"}, "state": {"approved"}}); refused.Code != http.StatusConflict {
		t.Fatalf("a decision on a frozen review = %d", refused.Code)
	}
}
