package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// The shell's files are fetched once and kept: their URLs name their bytes,
// so they are immutable, and an old name never answers with new bytes.
func TestShellAssetsAreVersionedAndImmutable(t *testing.T) {
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	page := getPage(t, handler, "/reviews").Body.String()
	for _, name := range []string{"app.css", "theme.js", "htmx.min.js", "preload.min.js", "app.js"} {
		path := assetPath(name)
		if !strings.Contains(page, `"`+path+`"`) {
			t.Fatalf("the page does not link %s at %s", name, path)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
			t.Fatalf("GET %s = %d, Cache-Control %q", path, recorder.Code, recorder.Header().Get("Cache-Control"))
		}
		if recorder.Body.String() != string(shellAssets[name].body) {
			t.Fatalf("GET %s did not serve the asset's bytes", path)
		}
		stale := httptest.NewRecorder()
		handler.ServeHTTP(stale, httptest.NewRequest(http.MethodGet, "/assets/0000000000000000/"+name, nil))
		if stale.Code != http.StatusNotFound {
			t.Fatalf("an unknown digest of %s answered %d", name, stale.Code)
		}
	}
	// The styles moved out of the page; nothing is inlined in their place.
	if strings.Contains(page, "<style>") {
		t.Fatal("the page still inlines its stylesheet")
	}
	// htmx is configured without eval, inline scripts, injected styles, or
	// history snapshots, which the page's CSP and freshness both rule out.
	for _, setting := range []string{`"allowEval":false`, `"allowScriptTags":false`, `"includeIndicatorStyles":false`, `"historyCacheSize":0`} {
		if !strings.Contains(page, setting) {
			t.Fatalf("the htmx config lacks %s", setting)
		}
	}
}
