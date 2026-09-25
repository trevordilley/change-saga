package server

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// pageParts fetches path the way htmx asks for a page to swap in, holding a
// shell loaded at shell.
func pageParts(t *testing.T, handler http.Handler, path, shell string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("HX-Request", "true")
	request.Header.Set("HX-Target", "page")
	request.Header.Set("X-Saga-Shell", shell)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s as a partial = %d: %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder
}

var (
	shellVersionAttr   = regexp.MustCompile(`data-saga-shell="([^"]*)"`)
)

// A page reached by a link is the page's own parts: its content, and its
// tabs, surfaces, and sidebar state out of band. What every page shares is
// already on screen and is not sent again. The content is the same markup a
// fresh load of the URL renders.
func TestAPartialIsThePagesOwnParts(t *testing.T) {
	root := writeAppNavSaga(t)
	handler := newMux(&app{root: root, sourceDir: root, template: serverTemplate(t)})
	for _, path := range []string{"/", featureHref("billing"), "/personas", "/terms", "/features"} {
		full := getPlain(t, handler, path)
		shell := shellVersionAttr.FindStringSubmatch(full)
		if shell == nil || shell[1] == "" {
			t.Fatalf("%s names no shell version", path)
		}
		recorder := pageParts(t, handler, path, shell[1])
		partial := recorder.Body.String()
		for _, part := range []string{
			`<title>`, `data-page-root`, `id="view-saga"`,
			`<div class="page-tabs" id="page-tabs" hx-swap-oob="true">`,
			`<div class="page-surfaces" id="page-surfaces" hx-swap-oob="true">`,
			`<div class="code-side" id="code-side" data-code-sidebar hidden hx-swap-oob="true">`,
			`<div id="nav-state" hidden data-nav-current=`,
		} {
			if !strings.Contains(partial, part) {
				t.Fatalf("the partial for %s lacks %q:\n%s", path, part, partial)
			}
		}
		for _, shared := range []string{"<html", "<head>", `class="doc-tree"`, "data-deck-viewer", `id="i-book"`, "diff-drawer"} {
			if strings.Contains(partial, shared) {
				t.Fatalf("the partial for %s sends the shared %q again", path, shared)
			}
		}
		if got := recorder.Header().Values("Vary"); strings.Join(got, ", ") != pageVary {
			t.Fatalf("the partial for %s varies by %v", path, got)
		}
		fullContent := pageContent(full)
		if fullContent == "" || !strings.Contains(partial, "\n"+fullContent+"\n") {
			t.Fatalf("the partial for %s renders different content than the full page:\n%s\n----\n%s", path, fullContent, partial)
		}
	}
}

// A browser holding a sidebar and deck viewer from another state of the
// Saga gets them again with the page, so nothing on screen is older than the
// request.
func TestAPartialReplacesAStaleShell(t *testing.T) {
	root := writeAppNavSaga(t)
	handler := newMux(&app{root: root, sourceDir: root, template: serverTemplate(t)})
	partial := pageParts(t, handler, featureHref("billing"), "an-older-saga").Body.String()
	for _, part := range []string{
		`<aside class="sidebar" id="changed-files-panel" data-saga-shell="`,
		`hx-swap-oob="true"><a class="sidebar-title"`,
		`<div class="view" id="view-slides" hx-swap-oob="true"`,
		`class="doc-tree"`,
	} {
		if !strings.Contains(partial, part) {
			t.Fatalf("the partial for a stale shell lacks %q", part)
		}
	}
	// The fresh sidebar carries its own state; it is not sent twice.
	if strings.Count(partial, `id="nav-state"`) != 1 || strings.Count(partial, `id="code-side"`) != 1 {
		t.Fatal("the stale-shell partial repeats the sidebar's parts")
	}
}

// Only a request to replace the page gets its parts: another htmx request, or
// one that asks for no target, is a whole page.
func TestOnlyAPageSwapGetsThePartial(t *testing.T) {
	root := writeAppNavSaga(t)
	handler := newMux(&app{root: root, sourceDir: root, template: serverTemplate(t)})
	for _, headers := range []map[string]string{
		{},
		{"HX-Request": "true"},
		{"HX-Request": "true", "HX-Target": "elsewhere"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if !strings.HasPrefix(recorder.Body.String(), "<!doctype html>") {
			t.Fatalf("%v got a partial", headers)
		}
	}
	restore := httptest.NewRequest(http.MethodGet, "/", nil)
	restore.Header.Set("HX-Request", "true")
	restore.Header.Set("HX-History-Restore-Request", "true")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, restore)
	if strings.HasPrefix(recorder.Body.String(), "<!doctype html>") || !strings.Contains(recorder.Body.String(), "data-page-root") {
		t.Fatal("a history restore did not get the page's parts")
	}
}

// pageContent is what a full page renders inside #page.
func pageContent(full string) string {
	start := strings.Index(full, `<div id="page">`)
	end := strings.Index(full, `</div><div class="view" id="view-slides"`)
	if end < 0 {
		end = strings.Index(full, `</div></main>`)
	}
	if start < 0 || end < start {
		return ""
	}
	return full[start+len(`<div id="page">`) : end]
}

func getPlain(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}
