package server

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A selected file streams its entire model. Requesting the already-supported
// maximum reduces repeated validation/diff work without sharing mutable state.
func TestReviewCodeStreamBatchesWithoutChangingRowsOrBounds(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	path := "stream & [literal].txt"
	var content strings.Builder
	for i := 0; i < 572; i++ {
		fmt.Fprintf(&content, "line %d\n", i)
	}
	writeServerFile(t, filepath.Join(fixture.repo, path), content.String())
	serverGit(t, fixture.repo, "add", path)
	serverGit(t, fixture.repo, "commit", "-m", "large selected file")
	_, handler := reviewApp(t, fixture, gitdiff.Range{Against: "main", Head: "feature/pg"})
	request := func(rawURL string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", rawURL, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: HTTP %d: %s", rawURL, w.Code, w.Body.String())
		}
		return w
	}
	code := request("/reviews/pr-7/code?file=" + url.QueryEscape(path))
	match := regexp.MustCompile(`data-file-diff-href="([^"]+)"`).FindStringSubmatch(code.Body.String())
	if len(match) != 2 {
		t.Fatal("selected-file stream URL missing")
	}
	stream, err := url.Parse(html.UnescapeString(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if stream.Query().Get("file") != path || stream.Query().Get("limit") != "200" {
		t.Fatalf("selected-file stream lost its literal path or explicit batch limit: %s", stream)
	}
	comparison := request("/api/code?file=" + url.QueryEscape(path))
	comparisonMatch := regexp.MustCompile(`data-file-diff-href="([^"]+)"`).FindStringSubmatch(comparison.Body.String())
	if len(comparisonMatch) != 2 || strings.Contains(comparisonMatch[1], "limit=") {
		t.Fatal("ordinary comparison stream changed its page-size request")
	}
	base := "/reviews/pr-7/file-diff?file=" + url.QueryEscape(path)
	if got := request(base).Header().Get("X-Change-Saga-Returned"); got != "50" {
		t.Fatalf("default response bound changed: %s", got)
	}
	if got := request(base + "&limit=20000").Header().Get("X-Change-Saga-Returned"); got != "200" {
		t.Fatalf("maximum response bound changed: %s", got)
	}
	walk := func(limit int) (string, int) {
		t.Helper()
		var rows strings.Builder
		cursor, count, pages := "", 0, 0
		seen := map[string]bool{}
		for {
			w := request(base + "&limit=" + strconv.Itoa(limit) + "&cursor=" + url.QueryEscape(cursor))
			returned, err := strconv.Atoi(w.Header().Get("X-Change-Saga-Returned"))
			if err != nil || returned < 1 || returned > limit {
				t.Fatalf("invalid row count: %d (%v)", returned, err)
			}
			count += returned
			pages++
			body := w.Body.String()
			_, body, ok := strings.Cut(body, `<div data-page-items="lines">`)
			if !ok || !strings.HasSuffix(body, "</div></div>") {
				t.Fatal("diff rows missing")
			}
			rows.WriteString(strings.TrimSuffix(body, "</div></div>"))
			cursor = w.Header().Get("X-Change-Saga-Next-Cursor")
			if cursor == "" {
				if strconv.Itoa(count) != w.Header().Get("X-Change-Saga-Total") || count != 573 {
					t.Fatalf("incomplete stream: %d rows", count)
				}
				break
			}
			if seen[cursor] {
				t.Fatal("cursor repeated")
			}
			seen[cursor] = true
		}
		return rows.String(), pages
	}
	ordinary, ordinaryPages := walk(50)
	batched, batchedPages := walk(200)
	if ordinaryPages != 12 || batchedPages != 3 || ordinary != batched {
		t.Fatalf("batching changed exact rows/actions: %d vs %d pages; equal=%v", ordinaryPages, batchedPages, ordinary == batched)
	}
	first := request(base + "&limit=200")
	// A dirty edit with unchanged size/mtime must still reach full validation
	// on the next page; no timestamp generation may mask it.
	manifest := filepath.Join(fixture.root, saga.ManifestName)
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, manifest, "!"+string(data[1:]))
	if err := os.Chtimes(manifest, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	continuation := base + "&limit=200&cursor=" + url.QueryEscape(first.Header().Get("X-Change-Saga-Next-Cursor"))
	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest("GET", continuation, nil))
	if invalid.Code != http.StatusInternalServerError {
		t.Fatalf("dirty invalid Saga escaped validation: HTTP %d", invalid.Code)
	}
	writeServerFile(t, manifest, string(data))
	if err := os.Chtimes(manifest, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	request(continuation) // Retry succeeds immediately after repair.
	writeServerFile(t, filepath.Join(fixture.repo, path), content.String()+"new head\n")
	serverGit(t, fixture.repo, "commit", "-am", "advance review head")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", base+"&limit=200&cursor="+url.QueryEscape(first.Header().Get("X-Change-Saga-Next-Cursor")), nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("old cursor accepted after source changed: HTTP %d", w.Code)
	}
}
