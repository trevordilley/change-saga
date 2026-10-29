package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/prototypes"
)

// A prototype opens on its own page: the app framed in a sandbox, its
// revisions, and the annotations that pin it to requirements. Its files are
// served on the authored-content policy, so even opened in its own tab the
// app cannot reach the reviewer.
func TestPrototypePageFramesTheAppAndServesItSandboxed(t *testing.T) {
	t.Parallel()
	root := writeAppNavSaga(t)
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"shop","title":"Shop","source":{"repository":"https://example.test/acme/shop.git"}}`)
	dir := applayout.FeatureDir(root, "billing")
	writeServerFile(t, filepath.Join(dir, applayout.FeatureManifestName), fmt.Sprintf(`{"$schema":%q,"version":5,"id":"billing","title":"Billing","created_at":"2026-08-21T12:00:00Z"}`, applayout.FeatureSchemaURL))
	source := t.TempDir()
	writeServerFile(t, filepath.Join(source, "index.html"), `<!doctype html><link rel="stylesheet" href="assets/app.css"><h1 id="pay">Pay</h1>`)
	writeServerFile(t, filepath.Join(source, "assets", "app.css"), `h1{color:red}`)
	for _, revision := range []string{"r1", "r2"} {
		var err error
		if revision == "r1" {
			_, err = prototypes.AddHTML(root, "shop", prototypes.AddHTMLInput{Feature: "billing", ID: "checkout", RevisionID: "r1", Title: "Checkout", State: prototypes.StateDraft, SourcePath: source})
		} else {
			writeServerFile(t, filepath.Join(source, "index.html"), `<!doctype html><link rel="stylesheet" href="assets/app.css"><h1 id="pay">Pay now</h1>`)
			_, err = prototypes.Revise(root, "shop", prototypes.ReviseInput{Prototype: "urn:change-saga:shop:prototype:checkout", ID: "r2", Parents: []string{"urn:change-saga:shop:prototype:checkout:revision:r1"}, Title: "Checkout v2", State: prototypes.StateReady, HTMLSourcePath: source})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		newMux(&app{root: root, sourceDir: root, template: serverTemplate(t)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder
	}

	page := get("/prototypes/checkout")
	if page.Code != http.StatusOK {
		t.Fatalf("GET prototype page = %d: %s", page.Code, page.Body.String())
	}
	html := page.Body.String()
	for _, want := range []string{
		`<iframe class="prototype-frame" sandbox="allow-scripts" src="/p/checkout/r2/html/index.html"`,
		`href="/p/checkout/r2/html/index.html" target="_blank" rel="noopener" data-prototype-open`,
		`href="/prototypes/checkout?revision=r1"`,
		`<h1>Checkout v2</h1>`,
		// The sidebar row now opens the page.
		`href="/prototypes/checkout"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("prototype page is missing %s", want)
		}
	}
	if older := get("/prototypes/checkout?revision=r1").Body.String(); !strings.Contains(older, `src="/p/checkout/r1/html/index.html"`) {
		t.Fatal("?revision= must frame the named revision")
	}
	if get("/prototypes/checkout?revision=r9").Code != http.StatusNotFound || get("/prototypes/missing").Code != http.StatusNotFound {
		t.Fatal("an unknown prototype or revision must be not found")
	}

	file := get("/p/checkout/r1/html/index.html")
	if file.Code != http.StatusOK || !strings.Contains(file.Body.String(), ">Pay<") {
		t.Fatalf("GET r1 entrypoint = %d: %s", file.Code, file.Body.String())
	}
	if policy := file.Header().Get("Content-Security-Policy"); !strings.HasPrefix(policy, "sandbox allow-scripts;") || !strings.Contains(policy, "connect-src 'none'") {
		t.Fatalf("prototype files must load on an opaque sandboxed origin: %q", policy)
	}
	if asset := get("/p/checkout/r2/html/assets/app.css"); asset.Code != http.StatusOK || !strings.Contains(asset.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("relative asset = %d %q", asset.Code, asset.Header().Get("Content-Type"))
	}
	for _, escape := range []string{"/p/checkout/r1/revision.json", "/p/checkout/r1/html/../revision.json", "/p/checkout/r1/html/%2e%2e/revision.json"} {
		if code := get(escape).Code; code == http.StatusOK {
			t.Fatalf("%s escaped the html package", escape)
		}
	}
}
