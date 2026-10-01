package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestReviewLineCommentsBindToDisplayedDiff(t *testing.T) {
	t.Parallel()
	f := newServerReviewFixture(t)
	_, handler := reviewApp(t, f, gitdiff.Range{})
	base := strings.TrimSpace(serverGit(t, f.repo, "rev-parse", "main"))
	target := saga.ReviewItemTarget("app", "pr-7", "queue", "node")
	for _, path := range []string{"/reviews/pr-7/item-diffs?target=" + url.QueryEscape(target), "/reviews/pr-7/file-diff?file=queue.go"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("diff %s: %d %s", path, response.Code, response.Body.String())
		}
		for _, attr := range []string{`data-review-diff-head="` + f.head + `"`, `data-review-diff-base="` + base + `"`} {
			if !strings.Contains(response.Body.String(), attr) {
				t.Fatalf("diff %s missing %s", path, attr)
			}
		}
	}
	query := url.Values{"path": {"queue.go"}, "head": {f.head}, "base": {base}}
	read := func(want int) {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/reviews/pr-7/line-threads?"+query.Encode(), nil))
		if response.Code != want {
			t.Fatalf("threads: %d: %s", response.Code, response.Body.String())
		}
	}
	read(http.StatusOK)
	fields := url.Values{"token": {"review-token"}, "head": {f.head}, "base": {base}, "path": {"queue.go"}, "side": {"new"}, "line": {"3"}, "body": {"Check this return value."}}
	withoutToken := fields.Encode()
	fields.Set("token", "wrong")
	if response := postAsyncReview(t, handler, "/reviews/pr-7/comment", fields); response.Code != http.StatusForbidden {
		t.Fatalf("token: %d %s", response.Code, response.Body.String())
	}
	fields, _ = url.ParseQuery(withoutToken)
	response := postAsyncReview(t, handler, "/reviews/pr-7/comment", fields)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"saved":true`) {
		t.Fatalf("comment: %d %s", response.Code, response.Body.String())
	}
	document, validation, err := saga.Load(f.root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %+v", err, validation)
	}
	comments := document.FindReview("pr-7").Comments
	if len(comments) != 1 || comments[0].CodeLine == nil || comments[0].CodeLine.Commit != f.head {
		t.Fatalf("wrong anchor: %+v", comments)
	}
	writeServerFile(t, filepath.Join(f.repo, "queue.go"), "package queue\n\nfunc Enqueue() string { return \"changed\" }\n")
	serverGit(t, f.repo, "add", "queue.go")
	serverGit(t, f.repo, "commit", "-m", "Move review head")
	read(http.StatusConflict)
	response = postAsyncReview(t, handler, "/reviews/pr-7/comment", fields)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale comment: %d %s", response.Code, response.Body.String())
	}
	document, _, _ = saga.Load(f.root)
	if len(document.FindReview("pr-7").Comments) != 1 {
		t.Fatal("stale submission wrote a comment")
	}
}
