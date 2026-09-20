package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestSecureHandlerRejectsCrossOriginFetchSiteAndHost(t *testing.T) {
	called := 0
	handler := secureHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}), "127.0.0.1:7342")
	tests := []struct {
		name   string
		method string
		host   string
		header map[string]string
	}{
		{name: "origin", method: http.MethodPost, host: "127.0.0.1:7342", header: map[string]string{"Origin": "http://evil.test"}},
		{name: "fetch metadata", method: http.MethodPost, host: "127.0.0.1:7342", header: map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{name: "host", method: http.MethodGet, host: "attacker.test:7342"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://127.0.0.1:7342/", nil)
			request.Host = test.host
			for key, value := range test.header {
				request.Header.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want forbidden", recorder.Code)
			}
		})
	}
	if called != 0 {
		t.Fatalf("rejected requests reached handler %d times", called)
	}

	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7342/", nil)
	request.Host = "127.0.0.1:7342"
	request.Header.Set("Origin", "http://127.0.0.1:7342")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("same-origin request status=%d called=%d", recorder.Code, called)
	}
}

func TestHTTPServerHasBoundedResourceSettings(t *testing.T) {
	server := newHTTPServer(http.NotFoundHandler())
	if server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 || server.ReadHeaderTimeout <= 0 || server.MaxHeaderBytes <= 0 {
		t.Fatalf("server limits are incomplete: %#v", server)
	}
}

// The browser suite proves the behavior; this pins the markup contract the
// behavior depends on so a template edit cannot quietly drop it.
func TestWorkspaceTabsAndClosedDrawerCarryAccessibleSemantics(t *testing.T) {
	for _, fragment := range []string{
		`role="tablist"`,
		`role="tab" id="view-tab-saga"`,
		`aria-controls="view-saga" aria-selected="true" tabindex="0"`,
		`aria-controls="view-code" aria-selected="false" tabindex="-1"`,
		`id="view-saga" role="tabpanel" aria-labelledby="view-tab-saga"`,
		`id="view-code" role="tabpanel" aria-labelledby="view-tab-code"`,
		`id="view-manifest" role="tabpanel" aria-labelledby="view-tab-manifest"`,
		`<aside class="diff-drawer" id="review-drawer" aria-hidden="true" inert`,
		`data-open-fragment="{{.Anchor}}"`,
	} {
		if !strings.Contains(pageTemplate, fragment) {
			t.Errorf("page template is missing accessible chrome markup %q", fragment)
		}
	}
	for _, fragment := range []string{
		"tab.setAttribute('aria-selected', String(selected))",
		"tab.tabIndex = selected ? 0 : -1",
		"drawer.setAttribute('inert', '')",
		"removeAttribute('inert')",
		"openDrawer(drawerButton.dataset.openDiffs, drawerButton)",
		"openFragmentDrawer(fragmentDrawerLink.dataset.openFragment, fragmentDrawerLink)",
		"hydrateTargetCode(targetCodeButton)",
		"data-target-code-response",
		"const labels = {fragment:'Related explanation', history:'History', code:'Linked code'}",
		"openHistoryDrawer(historyButton.dataset.historyHref, historyButton)",
	} {
		if !strings.Contains(appJavaScript, fragment) {
			t.Errorf("browser script no longer maintains %q", fragment)
		}
	}
}

func TestListenRefusesNonLoopbackAddressBeforeServing(t *testing.T) {
	err := Listen(context.Background(), filepath.Join(t.TempDir(), "missing.saga"), "", "0.0.0.0:0", false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "non-loopback") {
		t.Fatalf("Listen error = %v, want explicit non-loopback refusal", err)
	}
}

func TestManagedRuntimeEndpointsRequireTokenAndSignalShutdown(t *testing.T) {
	stopped := make(chan struct{}, 1)
	application := &app{shutdownToken: "private-token", shutdown: func() { stopped <- struct{}{} }}
	handler := newMux(application)

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/runtime", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"ok":true`) {
		t.Fatalf("runtime status = %d %q", status.Code, status.Body.String())
	}

	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequest(http.MethodPost, "/api/runtime-stop", nil))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("unauthenticated stop = %d", denied.Code)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/runtime-stop", nil)
	request.Header.Set("X-Change-Saga-Shutdown", "private-token")
	accepted := httptest.NewRecorder()
	handler.ServeHTTP(accepted, request)
	if accepted.Code != http.StatusOK {
		t.Fatalf("authenticated stop = %d %q", accepted.Code, accepted.Body.String())
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("authenticated stop did not signal shutdown")
	}
}

func TestColdComparisonEndpointReportsBuildingCacheWithoutMaterializingReviewData(t *testing.T) {
	// Comparing: observing has no comparison, so its Coverage never waits on one.
	application := &app{rng: gitdiff.Range{Against: "main"}}
	application.cache.building = true
	handler := newMux(application)

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/api/coverage", nil))
	if page.Code != http.StatusAccepted || !strings.Contains(page.Body.String(), "Building review cache") {
		t.Fatalf("cold comparison endpoint = %d %q, want explicit building-cache response", page.Code, page.Body.String())
	}
	if page.Header().Get("Retry-After") == "" {
		t.Fatal("building-cache response did not tell the browser when to retry")
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/runtime", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"cache":"building"`) {
		t.Fatalf("cold runtime status = %d %q", status.Code, status.Body.String())
	}
}

func TestBrowserErrorsDoNotExposeFilesystemPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private", "missing.saga")
	recorder := httptest.NewRecorder()
	(&app{root: root}).page(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("page status = %d", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), root) || strings.Contains(recorder.Body.String(), "no such file") {
		t.Fatalf("browser error exposed an internal path: %q", recorder.Body.String())
	}
}

// A permalink can name a heading or a marked place inside a chapter
// that has not been fetched yet. The browser cannot scroll to what is not there,
// so the server answers where one anchor lives — and answers it for the derived
// anchors too, because a heading id and a landmark id are both suffixes of the
// explanation that owns them.
func TestDeferredAnchorsResolveToTheirChapterAndExplanation(t *testing.T) {
	root := validServerSaga(t)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "chapter.json"), `{"version":2,"id":"alpha","title":"Alpha"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "story.fragment", "fragment.json"), `{"version":2,"id":"alpha-story","title":"Alpha story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "story.fragment", "content.md"), "# Deep heading {#deep}\n\nAlpha narrative.\n")
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "story.fragment", "___landmarks", "place.landmark", "landmark.json"),
		`{"version":2,"id":"place","label":"A marked place","selector":{"type":"text","exact":"Alpha"},"target":""}`)
	chapterTarget := saga.ChapterTarget("test", "alpha")
	fragmentTarget := "urn:change-saga:test:fragment:alpha-story"
	application := &app{root: root, sourceDir: root, template: serverTemplate(t)}

	place := func(anchor string) (map[string]string, int) {
		recorder := httptest.NewRecorder()
		application.locateAnchor(recorder, httptest.NewRequest(http.MethodGet, "/api/locate?anchor="+url.QueryEscape(anchor), nil))
		found := map[string]string{}
		if recorder.Code == http.StatusOK {
			if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
				t.Fatalf("the anchor response was not JSON: %v", err)
			}
		}
		return found, recorder.Code
	}

	fragmentID := domID(fragmentTarget)
	for name, anchor := range map[string]string{
		"the explanation itself": fragmentID,
		"a heading inside it":    fragmentID + "--deep",
		"a marked place":         fragmentID + "--place",
	} {
		found, status := place(anchor)
		if status != http.StatusOK {
			t.Fatalf("%s did not resolve: status=%d", name, status)
		}
		if found["chapter"] != domID(chapterTarget) || found["fragment"] != fragmentID {
			t.Fatalf("%s resolved to %#v, want chapter %s and explanation %s", name, found, domID(chapterTarget), fragmentID)
		}
	}

	// A chapter's own anchor needs no explanation fetched, and the overview
	// belongs to no chapter at all.
	if found, status := place(domID(chapterTarget)); status != http.StatusOK || found["chapter"] != domID(chapterTarget) || found["fragment"] != "" {
		t.Fatalf("a chapter anchor resolved to %#v (status %d)", found, status)
	}
	if found, status := place(domID(saga.SagaTarget("test"))); status != http.StatusOK || found["chapter"] != "" {
		t.Fatalf("the overview was placed inside a chapter: %#v (status %d)", found, status)
	}
	if _, status := place("target-not-a-real-anchor-abcdef"); status != http.StatusNotFound {
		t.Fatalf("an unknown anchor returned %d, want 404", status)
	}
	recorder := httptest.NewRecorder()
	application.locateAnchor(recorder, httptest.NewRequest(http.MethodGet, "/api/locate", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("a missing anchor returned %d, want 400", recorder.Code)
	}
}

// fragmentRequest asks for one explanation's content the way the browser does
// once the shell has told it the explanation exists.
func fragmentRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/fragment?target="+url.QueryEscape(target), nil)
}

// sectionRequest asks for one chapter's body the way the browser does when a
// reviewer opens that chapter.
func sectionRequest(target string) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/section?target="+url.QueryEscape(target), nil)
}

func TestFragmentFileRejectsSymlinkOutsidePackage(t *testing.T) {
	root := validServerSaga(t)
	outside := filepath.Join(filepath.Dir(root), "secret.txt")
	writeServerFile(t, outside, "secret")
	fragmentDir := filepath.Join(serverFeatureDir(root), "overview.fragment")
	if err := os.Symlink(outside, filepath.Join(fragmentDir, "secret.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	application := &app{root: root}
	request := httptest.NewRequest(http.MethodGet, "/f/overview/secret.txt", nil)
	request.SetPathValue("id", "overview")
	request.SetPathValue("path", "secret.txt")
	recorder := httptest.NewRecorder()
	application.fragmentFile(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("fragment status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestInteractiveFragmentIsServedWithSandboxCSP(t *testing.T) {
	root := validServerSaga(t)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "demo.fragment", "fragment.json"), `{"version":2,"id":"demo","media_type":"text/html","entrypoint":"index.html"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "demo.fragment", "index.html"), `<button onclick="this.textContent='ok'">Run</button>`)
	application := &app{root: root}
	request := httptest.NewRequest(http.MethodGet, "/f/demo/index.html", nil)
	request.SetPathValue("id", "demo")
	request.SetPathValue("path", "index.html")
	recorder := httptest.NewRecorder()
	application.fragmentFile(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "onclick") {
		t.Fatalf("interactive fragment was not served: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	csp := recorder.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src") || !strings.Contains(csp, "connect-src 'none'") {
		t.Fatalf("unexpected fragment CSP: %s", csp)
	}
}

func TestPageTemplateAndMarkdown(t *testing.T) {
	tmpl := serverTemplate(t)
	fragmentDir := t.TempDir()
	writeServerFile(t, filepath.Join(fragmentDir, "content.md"), "# Story {#story}\n")
	landmarkTarget := saga.LandmarkTarget("test", "overview", "story-text")
	fragment := &saga.Fragment{ID: "overview", Title: "Overview", Target: "urn:change-saga:test:fragment:overview", Directory: fragmentDir, MediaType: "text/markdown", Entrypoint: "content.md", Landmarks: []saga.Landmark{{Version: 2, ID: "story-text", Label: "Story text", Target: landmarkTarget, Selector: saga.LandmarkSelector{Type: "text", Exact: "Story"}}}}
	emptyFragment := &saga.Fragment{ID: "empty", Title: "No changes", Target: "urn:change-saga:test:fragment:empty", Directory: fragmentDir, MediaType: "text/plain", Entrypoint: "missing.txt"}
	section := &saga.Section{Kind: "chapter", ID: "root", Title: "Test", Target: "urn:change-saga:test:saga", Path: "private/root.chapter", Fragments: []*saga.Fragment{fragment, emptyFragment}}
	lineRef := testLocation(testHeadCommit, "app.go", 1, 1)
	fragment.Code = []saga.CodeFile{{Version: 2, References: []coderef.Reference{testReference(testHeadCommit, "app.go", 1, 1, "Adds the package entrypoint so the example compiles.")}}}
	manifestFiles := []*ManifestFileView{{Path: "internal/app.go", AtomCount: 1, Added: 1, Covered: 1, HasDiff: true, Chunks: []*ManifestChunkView{{Label: "+1", Path: "internal/app.go", AtomCount: 1, Excerpt: "package app", Href: CodeDiffURL("internal/app.go", lineRef), Covered: true, Owners: []*ManifestOwnerView{{Title: "Overview", Kind: "Fragment", Chapter: "Test", Href: "#overview"}}}}}}
	manifestFixture := &CoverageManifestView{
		Complete: true, Total: 1, Covered: 1, MappingCount: 1, Files: manifestFiles, Tree: makeManifestTree(manifestFiles),
		Targets: []*ManifestTargetView{{ManifestOwnerView: ManifestOwnerView{Title: "Overview", Kind: "Fragment", Chapter: "Test", Href: "#overview"}, AtomCount: 1, Chunks: []*ManifestChunkView{{Label: "+1", Path: "internal/app.go", Excerpt: "package app", Href: CodeDiffURL("internal/app.go", lineRef)}}, Files: []*ManifestTargetFileView{{Path: "internal/app.go", AtomCount: 1, Added: 1, Href: CodeDiffURL("internal/app.go", ""), HasDiff: true, Chunks: []*ManifestChunkView{{Label: "+1", Path: "internal/app.go", AtomCount: 1, Href: CodeDiffURL("internal/app.go", lineRef)}}}}}},
	}
	data := pageData{
		Saga: &saga.Saga{Manifest: saga.Manifest{ID: "test", Title: "Test", Source: saga.Source{Repository: "https://example.test/a.git"}}, Section: section},
		Root: makeSectionView(section, viewScope{
			changes: map[string][]gitdiff.Atom{
				fragment.Target: {{Kind: "line", Ref: lineRef, Path: "app.go", Side: "new", Line: 1, Content: "package app"}},
				landmarkTarget:  {{Kind: "line", Ref: lineRef, Path: "app.go", Side: "new", Line: 1, Content: "package app"}},
			},
		}),
		Code: &CodeReviewView{}, Manifest: manifestFixture,
	}
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "page", data); err != nil {
		t.Fatal(err)
	}
	renderedPage := output.String()
	// The page no longer carries diff rows, so the code locations it must keep
	// unmangled are asserted where those rows are now produced, in
	// TestFileDiffEndpointServesCoverageAndTargetedBodies.
	if strings.Contains(renderedPage, "ZgotmplZ") {
		t.Fatal("template produced an unsafe URL sentinel")
	}
	for _, expected := range []string{"/app.js", `id="` + domID(fragment.Target) + `--story"`} {
		if !strings.Contains(renderedPage, expected) {
			t.Fatalf("template output is missing %q", expected)
		}
	}
	// The Saga is documentation: no approval, comment, or annotation control
	// is rendered on it in any mode.
	for _, control := range []string{"annotation-toolbox", "data-annotation-tools", "data-review-progress", "data-review-controls", "data-review-decision", "data-review-comment", "data-shared-review-form", "/api/thread", "/api/review", "/api/reply", "/api/diff-review", "data-activity"} {
		if strings.Contains(renderedPage, control) {
			t.Fatalf("documentation rendered review control %q", control)
		}
	}
	if !strings.Contains(renderedPage, `body data-saga-id="test"`) || !strings.Contains(renderedPage, `data-open-history`) {
		t.Fatal("the page lost its identity or history controls")
	}
	if !strings.Contains(renderedPage, `data-view-tab="manifest"`) || !strings.Contains(renderedPage, `data-review-surface="manifest"`) || !strings.Contains(renderedPage, `data-surface-href="/api/coverage"`) {
		t.Fatal("bounded coverage navigation was not rendered")
	}
	if strings.Contains(renderedPage, `data-manifest-panel="code"`) || strings.Contains(renderedPage, `class="manifest-range"`) {
		t.Fatal("the root page eagerly rendered coverage details")
	}
	// Coverage is an invariant, so a complete report earns no praise banner —
	// only failures and stale references are worth a reviewer's attention.
	for _, celebration := range []string{"Everything is accounted for", "Every source change has a live", "complete\"", "manifest-verdict"} {
		if strings.Contains(renderedPage, celebration) {
			t.Fatalf("complete coverage still congratulates the reviewer: %q", celebration)
		}
	}
	if strings.Contains(renderedPage, "Attached code") || strings.Contains(renderedPage, "Linked diffs</h2>") || !strings.Contains(renderedPage, `<strong id="review-drawer-title">Linked code</strong>`) {
		t.Fatal("attached-code drawer retained redundant header chrome")
	}
	if !strings.Contains(renderedPage, `class="attached-file" data-file-diff-href=`) || !strings.Contains(renderedPage, `data-file-diff-rows`) || !strings.Contains(renderedPage, "Adds the package entrypoint so the example compiles.") || !strings.Contains(renderedPage, "Open in Code Diff") || strings.Contains(renderedPage, `<details class="attached-file" open`) || strings.Contains(renderedPage, "Linked ranges only") {
		t.Fatal("attached code was not presented as a collapsed, explained file list")
	}
	if !strings.Contains(renderedPage, `class="attached-code-summary"`) || !strings.Contains(renderedPage, `class="diff-counts"`) || !strings.Contains(renderedPage, "linked line") || !strings.Contains(renderedPage, "highlighted") {
		t.Fatal("attached code did not distinguish linked-change counts from full-file context")
	}
	if !strings.Contains(renderedPage, `data-landmark-type="text"`) || !strings.Contains(renderedPage, `data-exact="Story"`) || !strings.Contains(renderedPage, `data-prefix=""`) || !strings.Contains(renderedPage, `>Story text</a>`) {
		t.Fatal("fragment landmarks were not exposed as deep links")
	}
	if !strings.Contains(renderedPage, `data-open-diffs="diffs-`+domID(fragment.Target)+`--story-text"`) {
		t.Fatal("landmark-related code was not exposed in place")
	}
	if strings.Contains(renderedPage, `name="author"`) || strings.Contains(renderedPage, "Your name") || strings.Contains(renderedPage, "reviewer-name") {
		t.Fatal("review UI asked for editable author identity")
	}
	if strings.Contains(renderedPage, "text/markdown") || strings.Contains(renderedPage, "text/plain") || strings.Contains(renderedPage, "private/root.chapter") || strings.Contains(renderedPage, "format v") || strings.Contains(renderedPage, ">Chapter<") {
		t.Fatal("reviewer-facing format metadata leaked into the page")
	}
	if strings.Contains(renderedPage, `data-open-diffs="diffs-`+domID(emptyFragment.Target)+`"`) || strings.Contains(renderedPage, `id="diffs-`+domID(emptyFragment.Target)+`"`) {
		t.Fatal("fragment without linked changes rendered a diff action")
	}
	rendered := string(markdown("# Heading {#stable-heading}\n\n- one\n- <script>bad</script>"))
	if strings.Contains(rendered, "<script>") || strings.Contains(rendered, "{#stable-heading}") || !strings.Contains(rendered, "&lt;script&gt;") || !strings.Contains(rendered, `id="heading--stable-heading"`) || !strings.Contains(rendered, `data-copy-link="#heading--stable-heading"`) {
		t.Fatalf("unexpected Markdown rendering: %s", rendered)
	}
}

func TestSVGAspectRatioKeepsHotspotsAligned(t *testing.T) {
	if got := svgAspectRatio(`<svg viewBox="0 0 1200 640"></svg>`); got != "1.87500000" {
		t.Fatalf("aspect ratio = %q", got)
	}
	if got := svgAspectRatio(`<svg></svg>`); got != "" {
		t.Fatalf("missing viewBox ratio = %q", got)
	}
}

func TestMarkdownRendersSafeGFMWithStablePermalinks(t *testing.T) {
	rendered := string(markdownWithAnchors(`# Story {#stable-story}

| Before | After |
| --- | --- |
| **slow** | `+"`fast`"+` |

1. First
2. Second

The lease is renewed before its midpoint.[^lease-renewal]

[^lease-renewal]: The heartbeat path renews the lease before half its TTL elapses.

[safe](https://example.test) [unsafe](javascript:alert(1))

<script>alert("no")</script>
`, "fragment"))
	for _, expected := range []string{
		`id="fragment--stable-story"`,
		`data-copy-link="#fragment--stable-story"`,
		`<table>`,
		`<strong>slow</strong>`,
		`<code>fast</code>`,
		`<ol>`,
		`href="https://example.test"`,
		`id="fragment--fnref:1"`,
		`href="#fragment--fn:1"`,
		`class="footnote-ref"`,
		`The heartbeat path renews the lease before half its TTL elapses.`,
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("Markdown output is missing %q:\n%s", expected, rendered)
		}
	}
	if strings.Contains(rendered, `<script>`) || strings.Contains(rendered, `href="javascript:`) {
		t.Fatalf("Markdown output contains unsafe content:\n%s", rendered)
	}
	other := string(markdownWithAnchors("Another claim.[^lease-renewal]\n\n[^lease-renewal]: Other evidence.\n", "other-fragment"))
	if !strings.Contains(other, `id="other-fragment--fnref:1"`) || strings.Contains(other, `id="fragment--fnref:1"`) {
		t.Fatalf("footnote IDs were not namespaced per fragment:\n%s", other)
	}
}

// The first load is a shell: saga identity, coverage totals, the overview's
// explanations as descriptors, one summary per chapter, and the navigation
// outline. Everything below that is fetched from a bounded endpoint as the
// reviewer reaches it, so the page describes the story instead of containing it.
func TestPageHandlerShipsAChapterShellAndRedirectsLegacyRoutes(t *testing.T) {
	root := validServerSaga(t)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "content.md"), "Root-only introduction\n")
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "chapter.json"), `{"version":2,"id":"alpha","title":"Alpha"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "alpha.fragment", "fragment.json"), `{"version":2,"id":"alpha-story","title":"Alpha story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "alpha.chapter", "alpha.fragment", "content.md"), "Alpha-exclusive narrative\n")
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "beta.chapter", "chapter.json"), `{"version":2,"id":"beta","title":"Beta"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "beta.chapter", "beta.fragment", "fragment.json"), `{"version":2,"id":"beta-story","title":"Beta story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "beta.chapter", "beta.fragment", "content.md"), "Beta-exclusive narrative\n")
	application := &app{root: root, sourceDir: root, template: serverTemplate(t)}

	// The chapters belong to the fixture's feature, so its page is their shell.
	overview := httptest.NewRecorder()
	newMux(application).ServeHTTP(overview, httptest.NewRequest(http.MethodGet, featureHref(serverFeature), nil))
	if overview.Code != http.StatusOK {
		t.Fatalf("overview status = %d: %s", overview.Code, overview.Body.String())
	}
	overviewBody := overview.Body.String()
	alphaTarget := saga.ChapterTarget("test", "alpha")
	overviewFragment := "urn:change-saga:test:fragment:overview"
	// The shell names every chapter and every explanation and carries the
	// content of none of them.
	for _, narrative := range []string{"Root-only introduction", "Alpha-exclusive narrative", "Beta-exclusive narrative"} {
		if strings.Contains(overviewBody, narrative) {
			t.Fatalf("first load carried narrative content it was only asked to describe: %q", narrative)
		}
	}
	if !strings.Contains(overviewBody, `href="`+featureHref(serverFeature)+`#`+domID(alphaTarget)+`"`) ||
		!strings.Contains(overviewBody, `data-section-href="/api/section?target=`+template.HTMLEscapeString(url.QueryEscape(alphaTarget))+`"`) {
		t.Fatal("the shell did not describe its chapters as fetchable summaries")
	}
	if strings.Contains(overviewBody, `data-review-target=`) || !strings.Contains(overviewBody, `data-history-href="/api/history?target=`+template.HTMLEscapeString(url.QueryEscape(alphaTarget))+`"`) {
		t.Fatal("the chapter bar carried review controls or lost its history")
	}
	if strings.Contains(overviewBody, `data-chapter-review-directory`) {
		t.Fatal("the first-load shell eagerly carried a chapter review directory")
	}
	if !strings.Contains(overviewBody, `data-fragment-href="/api/fragment?target=`+template.HTMLEscapeString(url.QueryEscape(overviewFragment))+`"`) {
		t.Fatal("the overview did not describe its explanations as fetchable descriptors")
	}
	if strings.Count(overviewBody, `data-chapter-body hidden`) != 2 || strings.Count(overviewBody, `data-chapter-toggle aria-expanded="false"`) != 2 {
		t.Fatal("summarised chapters did not start collapsed")
	}

	// Opening a chapter fetches that chapter and nothing else, and the
	// explanations it names are themselves still descriptors.
	alpha := httptest.NewRecorder()
	application.sectionBody(alpha, sectionRequest(alphaTarget))
	if alpha.Code != http.StatusOK {
		t.Fatalf("chapter body status = %d: %s", alpha.Code, alpha.Body.String())
	}
	alphaBody := alpha.Body.String()
	alphaFragment := "urn:change-saga:test:fragment:alpha-story"
	if !strings.Contains(alphaBody, `data-fragment-href="/api/fragment?target=`+template.HTMLEscapeString(url.QueryEscape(alphaFragment))+`"`) {
		t.Fatalf("chapter body did not describe its explanations: %s", alphaBody)
	}
	if strings.Contains(alphaBody, `data-chapter-review-directory`) || strings.Contains(alphaBody, `data-review-target=`) {
		t.Fatalf("chapter body carried approval controls on documentation: %s", alphaBody)
	}
	if strings.Contains(alphaBody, "Alpha-exclusive narrative") || strings.Contains(alphaBody, "Beta-exclusive narrative") {
		t.Fatal("a chapter body carried explanation content, or content from another chapter")
	}

	// Only the explanation endpoint produces content, and only for the one
	// explanation it was asked for.
	story := httptest.NewRecorder()
	application.fragmentContent(story, fragmentRequest(alphaFragment))
	if story.Code != http.StatusOK {
		t.Fatalf("explanation status = %d: %s", story.Code, story.Body.String())
	}
	if !strings.Contains(story.Body.String(), "Alpha-exclusive narrative") || strings.Contains(story.Body.String(), "Beta-exclusive narrative") {
		t.Fatalf("explanation response was not exactly the one explanation: %s", story.Body.String())
	}

	missingSection := httptest.NewRecorder()
	application.sectionBody(missingSection, sectionRequest(saga.ChapterTarget("test", "nowhere")))
	missingFragment := httptest.NewRecorder()
	application.fragmentContent(missingFragment, fragmentRequest("urn:change-saga:test:fragment:nowhere"))
	if missingSection.Code != http.StatusNotFound || missingFragment.Code != http.StatusNotFound {
		t.Fatalf("unknown targets did not 404: section=%d fragment=%d", missingSection.Code, missingFragment.Code)
	}

	chapterRequest := httptest.NewRequest(http.MethodGet, "/chapters/alpha", nil)
	chapterRequest.SetPathValue("chapter", "alpha")
	chapter := httptest.NewRecorder()
	application.page(chapter, chapterRequest)
	if chapter.Code != http.StatusFound || chapter.Header().Get("Location") != featureHref(serverFeature)+"#"+domID(alphaTarget) {
		t.Fatalf("legacy chapter route did not redirect to its in-page target: status=%d location=%q", chapter.Code, chapter.Header().Get("Location"))
	}

	missingRequest := httptest.NewRequest(http.MethodGet, "/chapters/missing", nil)
	missingRequest.SetPathValue("chapter", "missing")
	missing := httptest.NewRecorder()
	application.page(missing, missingRequest)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing chapter status = %d, want 404", missing.Code)
	}
}

func TestFragmentContentNeverLoadsTheSourceComparison(t *testing.T) {
	root := validServerSaga(t)
	application := &app{root: root, sourceDir: root, template: serverTemplate(t)}
	application.comparisonLoader = func(context.Context) (*reviewSnapshot, error) {
		t.Fatal("narrative fragment requested the source comparison")
		return nil, nil
	}

	recorder := httptest.NewRecorder()
	application.fragmentContent(recorder, fragmentRequest("urn:change-saga:test:fragment:overview"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("explanation status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "Story") {
		t.Fatal("comparison-independent fragment lost its narrative content")
	}
}

func TestPageHandlerRendersRealGitComparison(t *testing.T) {
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Test")
	serverGit(t, repo, "config", "user.email", "test@example.test")
	writeServerFile(t, filepath.Join(repo, "base.txt"), "base\n")
	serverGit(t, repo, "add", "base.txt")
	serverGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	writeServerFile(t, filepath.Join(repo, "app.go"), "package app\n")
	writeServerFile(t, filepath.Join(repo, "web", "view.js"), "export const ready = true\n")
	serverGit(t, repo, "add", "app.go")
	serverGit(t, repo, "add", "web/view.js")
	serverGit(t, repo, "commit", "-m", "feature")
	root := filepath.Join(repo, "pr-1.saga")
	repository, err := coderef.FileRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"Test","source":{"repository":"`+repository+`"}}`)
	writeServerFeature(t, root)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "content.md"), "# Story\n")
	application := &app{root: root, sourceDir: repo, rng: gitdiff.Range{Against: base}, template: serverTemplate(t)}
	handler := newMux(application)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Overview") || !strings.Contains(recorder.Body.String(), "Code Diff") || strings.Contains(recorder.Body.String(), `<code>web/view.js</code>`) {
		t.Fatalf("root did not render the comparison-free shell: status=%d", recorder.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/code?file=web%2Fview.js&limit=200", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "app.go") || !strings.Contains(recorder.Body.String(), `data-file-path="web/view.js"`) {
		t.Fatalf("incremental code page did not render expected file navigation: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	changes, err := gitdiff.Read(t.Context(), repo, repository, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	var selectedRef string
	for _, atom := range changes.Atoms {
		if atom.Path == "web/view.js" {
			selectedRef = atom.Ref
			break
		}
	}
	if selectedRef == "" {
		t.Fatal("missing web/view.js atom")
	}
	request = httptest.NewRequest(http.MethodGet, "/api/code?ref="+url.QueryEscape(selectedRef), nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `data-file-path="web/view.js"`) {
		t.Fatalf("exact diff handler selection: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/code?file=missing.go", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown focused file status=%d", recorder.Code)
	}
}

func TestTargetCodeLoadsOneNarrativeMappingWithoutGlobalSnapshot(t *testing.T) {
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Test")
	serverGit(t, repo, "config", "user.email", "test@example.test")
	writeServerFile(t, filepath.Join(repo, "base.txt"), "base\n")
	serverGit(t, repo, "add", "base.txt")
	serverGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	writeServerFile(t, filepath.Join(repo, "app.go"), "package app\n\nfunc Ready() bool { return true }\n")
	writeServerFile(t, filepath.Join(repo, "unrelated.go"), "package app\n")
	serverGit(t, repo, "add", "app.go", "unrelated.go")
	serverGit(t, repo, "commit", "-m", "feature")
	repository, err := coderef.FileRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := gitdiff.Read(t.Context(), repo, repository, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	var appRef string
	for _, atom := range changes.Atoms {
		if atom.Path == "app.go" && atom.Kind == "line" {
			appRef = atom.Ref
			break
		}
	}
	if appRef == "" {
		t.Fatal("fixture has no app.go change")
	}

	root := filepath.Join(repo, "linked.saga")
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"linked","title":"Linked","source":{"repository":"`+repository+`"}}`)
	writeServerFeature(t, root)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "story.fragment", "fragment.json"), `{"version":2,"id":"story","title":"Story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "story.fragment", "content.md"), "# Story\n")
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "story.fragment", saga.CodeDirName, "app.json"), codeRecordJSON(t, repo, [2]string{appRef, "Implements the ready path."}))
	target := saga.FragmentTarget("linked", "story")
	application := &app{root: root, sourceDir: repo, rng: gitdiff.Range{Against: base}, template: serverTemplate(t)}
	application.comparisonLoader = func(context.Context) (*reviewSnapshot, error) {
		t.Fatal("target-scoped linked code requested the global comparison")
		return nil, nil
	}
	handler := newMux(application)

	fragment := httptest.NewRecorder()
	handler.ServeHTTP(fragment, fragmentRequest(target))
	if fragment.Code != http.StatusOK || !strings.Contains(fragment.Body.String(), `data-target-code-href="/api/target-code?target=`) || strings.Contains(fragment.Body.String(), `data-open-diffs=`) {
		t.Fatalf("narrative did not render a lazy linked-code control: status=%d body=%s", fragment.Code, fragment.Body.String())
	}

	summary := httptest.NewRecorder()
	handler.ServeHTTP(summary, httptest.NewRequest(http.MethodGet, "/api/target-code?target="+url.QueryEscape(target), nil))
	if summary.Code != http.StatusOK || !strings.Contains(summary.Body.String(), `data-open-diffs="diffs-`+domID(target)+`"`) || !strings.Contains(summary.Body.String(), `data-target-code-count="3"`) || !strings.Contains(summary.Body.String(), `aria-label="Open linked code with 3 additions and 0 deletions"`) || strings.Count(summary.Body.String(), `<span class="diff-counts"><span class="add">+3</span><span class="del">−0</span></span>`) < 3 || !strings.Contains(summary.Body.String(), "1 linked line highlighted") || !strings.Contains(summary.Body.String(), "app.go") || !strings.Contains(summary.Body.String(), "Implements the ready path.") || strings.Contains(summary.Body.String(), "unrelated.go") {
		t.Fatalf("target code was not scoped to the authored mapping: status=%d body=%s", summary.Code, summary.Body.String())
	}

	file := httptest.NewRecorder()
	handler.ServeHTTP(file, httptest.NewRequest(http.MethodGet, "/api/file-diff?file=app.go&target="+url.QueryEscape(target), nil))
	if file.Code != http.StatusOK || !strings.Contains(file.Body.String(), "linked-evidence") || !strings.Contains(file.Body.String(), `data-target="`+target+`"`) || strings.Contains(file.Body.String(), "unrelated.go") {
		t.Fatalf("target file body lost its scoped evidence: status=%d body=%s", file.Code, file.Body.String())
	}
}

func TestSlideTargetCodeRollsUpItemFiles(t *testing.T) {
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Test")
	serverGit(t, repo, "config", "user.email", "test@example.test")
	writeServerFile(t, filepath.Join(repo, "base.txt"), "base\n")
	serverGit(t, repo, "add", "base.txt")
	serverGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	writeServerFile(t, filepath.Join(repo, "app.go"), "package app\n\nfunc Ready() bool { return true }\n")
	writeServerFile(t, filepath.Join(repo, "guide.md"), "# Guide\n\nReady.\n")
	serverGit(t, repo, "add", "app.go", "guide.md")
	serverGit(t, repo, "commit", "-m", "feature")
	repository, err := coderef.FileRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := gitdiff.Read(t.Context(), repo, repository, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	refByPath := map[string]string{}
	for _, atom := range changes.Atoms {
		if atom.Kind == "line" && refByPath[atom.Path] == "" {
			refByPath[atom.Path] = atom.Ref
		}
	}

	root := filepath.Join(repo, "slides.saga")
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"slides","title":"Slides","source":{"repository":"`+repository+`"}}`)
	writeServerFeature(t, root)
	bundle := filepath.Join(serverFeatureDir(root), saga.EmbeddedSlidesDir, "review"+saga.EmbeddedDeckSuffix)
	deckTarget := saga.DeckTarget("slides", "review")
	deckName, _ := saga.FlatDeckFilename(deckTarget, 0)
	writeServerFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"review","title":"Review","role":"change","rank":0,"objective":"Review the change."}`)
	slideTarget := saga.SlideTarget("slides", "summary")
	slideName, _ := saga.FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeServerFile(t, filepath.Join(bundle, slideName), `{"version":4,"id":"summary","deck":"review","title":"Changed files","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":"`+assetName+`","takeaway":"Both files support this slide.","reading_order":["code","guide"]}`)
	writeServerFile(t, filepath.Join(bundle, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><g id="code"/><g id="guide"/></svg>`)
	for rank, fixture := range []struct {
		id, label, path, note string
	}{
		{id: "code", label: "Code", path: "app.go", note: "Implements readiness."},
		{id: "guide", label: "Guide", path: "guide.md", note: "Documents readiness."},
	} {
		itemTarget := saga.ItemTarget("slides", "summary", fixture.id)
		itemName, _ := saga.FlatItemFilename(slideTarget, itemTarget, rank*10)
		writeServerFile(t, filepath.Join(bundle, itemName), fmt.Sprintf(`{"version":4,"id":%q,"slide":"summary","rank":%d,"kind":"node","label":%q,"description":%q,"selector":{"type":"element","element_id":%q}}`, fixture.id, rank*10, fixture.label, fixture.note, fixture.id))
		evidence := codeRecordJSON(t, repo, [2]string{refByPath[fixture.path], fixture.note})
		if fixture.id == "guide" {
			// Repeating one exact reference on a second Item must not inflate
			// the slide's file totals, even though coverage validation will
			// surface the overlapping ownership to the author.
			evidence = codeRecordJSON(t, repo, [2]string{refByPath[fixture.path], fixture.note}, [2]string{refByPath["app.go"], "Also mentioned by the guide."})
		}
		writeServerFile(t, filepath.Join(bundle, saga.FlatEvidenceFilename(itemTarget, fixture.id)), evidence)
	}

	application := &app{root: root, sourceDir: repo, rng: gitdiff.Range{Against: base}, template: serverTemplate(t)}
	application.comparisonLoader = func(context.Context) (*reviewSnapshot, error) {
		t.Fatal("slide-scoped linked code requested the global comparison")
		return nil, nil
	}
	handler := newMux(application)

	summary := httptest.NewRecorder()
	handler.ServeHTTP(summary, httptest.NewRequest(http.MethodGet, "/api/target-code?target="+url.QueryEscape(slideTarget), nil))
	body := summary.Body.String()
	if summary.Code != http.StatusOK || !strings.Contains(body, `data-target-code-count="6"`) || !strings.Contains(body, `aria-label="Open linked code with 6 additions and 0 deletions"`) || !strings.Contains(body, "app.go") || !strings.Contains(body, "guide.md") || !strings.Contains(body, "Implements readiness.") || !strings.Contains(body, "Documents readiness.") {
		t.Fatalf("slide linked-code summary did not aggregate its Item files: status=%d body=%s", summary.Code, body)
	}

	file := httptest.NewRecorder()
	handler.ServeHTTP(file, httptest.NewRequest(http.MethodGet, "/api/file-diff?file=guide.md&target="+url.QueryEscape(slideTarget), nil))
	if file.Code != http.StatusOK || !strings.Contains(file.Body.String(), `data-file-path="guide.md"`) || !strings.Contains(file.Body.String(), "linked-evidence") || strings.Contains(file.Body.String(), "app.go") {
		t.Fatalf("slide linked-file body was not scoped to the requested referenced file: status=%d body=%s", file.Code, file.Body.String())
	}
}

func TestNarrativeEvidenceTargetsFindsTheRequestedDeck(t *testing.T) {
	target := saga.SlideTarget("slides", "summary")
	itemTarget := saga.ItemTarget("slides", "summary", "code")
	root := &saga.Section{Children: []*saga.Section{
		{Kind: "deck", Fragments: []*saga.Fragment{{Target: saga.SlideTarget("slides", "other"), SlideMeta: &saga.SlideManifest{}}}},
		{Kind: "deck", Fragments: []*saga.Fragment{{Target: target, SlideMeta: &saga.SlideManifest{}, Landmarks: []saga.Landmark{{Target: itemTarget, ItemMeta: &saga.ItemManifest{}}}}}},
	}}
	got := narrativeEvidenceTargets(root, target)
	if len(got) != 2 || got[0] != target || got[1] != itemTarget {
		t.Fatalf("slide evidence targets = %#v", got)
	}
	if direct := narrativeEvidenceTargets(root, itemTarget); len(direct) != 1 || direct[0] != itemTarget {
		t.Fatalf("Item target unexpectedly widened to %#v", direct)
	}
}

// Stylesheet rules and the markup they target drift apart silently: a rule that
// matches nothing, or one that matches more than intended, breaks the design
// without breaking a render. These two pairs have both regressed before.
func TestStylesheetSelectorsMatchTheMarkupTheyTarget(t *testing.T) {
	// The disclosure chevron in the linked-code drawer is the shared glyph, so
	// the rule that sizes and rotates it must name that class.
	if !strings.Contains(pageStyles, ".attached-file[open]>summary .twisty{transform:rotate(90deg)}") || strings.Contains(pageStyles, ".attached-file-marker") {
		t.Fatal("linked-code drawer chevron rule does not match the rendered glyph")
	}
	// The chapter eyebrow is monospace; the fragment excerpt beneath it is
	// prose. A `.related-chapter>a` rule would silently capture both.
	if strings.Contains(pageStyles, ".related-chapter>a") {
		t.Fatal("chapter link rule also matches the prose excerpt beneath it")
	}
	if !strings.Contains(pageStyles, ".related-chapter-link{") || !strings.Contains(pageTemplate, `class="related-chapter-link"`) {
		t.Fatal("chapter link class is not applied in both the stylesheet and the template")
	}
}

// The sidebar is documentation navigation, not a view of storage: it lists the
// overview and every preloaded chapter while keeping chapter outlines collapsed.
func TestNavigationTreeReadsAsCollapsedDocumentationOutline(t *testing.T) {
	systemMap := &saga.Fragment{ID: "system-map", Title: "System map"}
	root := &saga.Section{ID: "root", Title: "Scaffold", Target: saga.SagaTarget("test"),
		Fragments: []*saga.Fragment{{ID: "overview", Title: "Overview"}, systemMap, {ID: "untitled"}},
		Children: []*saga.Section{
			{Kind: "chapter", ID: "format", Title: "Format", Target: saga.ChapterTarget("test", "format")},
			{Kind: "chapter", ID: "ui", Title: "Reviewer", Target: saga.ChapterTarget("test", "ui"),
				Fragments: []*saga.Fragment{{ID: "shell", Title: "Reviewer"}, systemMap}},
		}}

	nodes := makeNavTree(root)
	if len(nodes) != 3 || nodes[0].Title != "Overview" || nodes[1].Title != "Format" || nodes[2].Title != "Reviewer" {
		t.Fatalf("unexpected navigation nodes: %#v", nodes)
	}
	// "Overview > Overview" is noise, and untitled content must not be exposed
	// under an internal identifier.
	if len(nodes[0].Children) != 1 || nodes[0].Children[0].Title != "System map" {
		t.Fatalf("overview outline was not deduplicated: %#v", nodes[0].Children)
	}
	if !nodes[0].Expanded || nodes[1].Expanded || nodes[2].Expanded {
		t.Fatal("only the open page may be expanded")
	}
	if nodes[1].Href != sagaHref(saga.ChapterTarget("test", "format")) {
		t.Fatalf("chapter node is not navigation: %#v", nodes[1])
	}
	if nodes[2].Expanded || len(nodes[2].Children) != 1 || nodes[2].Children[0].Title != "System map" {
		t.Fatalf("collapsed chapter did not retain its navigable outline: %#v", nodes[2])
	}
}

func TestDeckNavigationNamesDecksAndExpandsToRenderedSlideThumbnails(t *testing.T) {
	root := &saga.Section{ID: "root", Title: "Slides", Target: saga.SagaTarget("test"), Children: []*saga.Section{
		{Kind: "deck", ID: "flow", Title: "Request flow", Target: saga.DeckTarget("test", "flow"), Fragments: []*saga.Fragment{
			{ID: "happy", Title: "Happy path", Target: saga.SlideTarget("test", "happy"), SlideMeta: &saga.SlideManifest{Section: "Request"}},
			{ID: "failure", Title: "Failure path", Target: saga.SlideTarget("test", "failure"), SlideMeta: &saga.SlideManifest{Section: "Errors"}},
		}},
	}}

	nodes := makeDeckNavTree(root)
	if len(nodes) != 1 || nodes[0].Title != "Request flow" || !nodes[0].Deck || nodes[0].Icon != "deck" {
		t.Fatalf("deck was not projected as a named sidebar disclosure: %#v", nodes)
	}
	if nodes[0].Expanded || len(nodes[0].Children) != 2 {
		t.Fatalf("deck should begin collapsed with both slide destinations available: %#v", nodes[0])
	}
	if got := nodes[0].Children[1]; got.Title != "Failure path" || got.Slide == nil || got.Slide.Target != saga.SlideTarget("test", "failure") || !strings.Contains(got.Href, "?view=slides#") {
		t.Fatalf("rendered slide destination = %#v", got)
	}
	if nodes[0].Children[0].Slide.Section != "Request" || nodes[0].Children[1].Slide.Section != "Errors" {
		t.Fatalf("slide section boundaries were not retained: %#v", nodes[0].Children)
	}
}

func TestDOMIDIsStableAndCollisionResistantAfterReadablePrefix(t *testing.T) {
	prefix := "urn:change-saga:test:fragment:" + strings.Repeat("shared-prefix", 10)
	first := domID(prefix + "-one")
	second := domID(prefix + "-two")
	if first == second || first != domID(prefix+"-one") {
		t.Fatalf("DOM IDs must be stable and collision resistant: %q %q", first, second)
	}
}

func TestFileViewsGroupRenameAndUseDistinctAnchors(t *testing.T) {
	changes := gitdiff.ChangeSet{
		Repository: "https://example.test/a.git", BaseOID: "aaa", HeadOID: "bbb",
		Atoms: []gitdiff.Atom{
			{Kind: "event", Event: "rename", OldPath: "old.go", NewPath: "new.go", Path: "new.go"},
			{Kind: "line", Path: "old.go", Side: "old", Line: 1, Content: "old"},
			{Kind: "line", Path: "new.go", Side: "new", Line: 1, Content: "new"},
			{Kind: "line", Path: "another.go", Side: "new", Line: 1, Content: "new"},
		},
	}
	files := makeFileViews(changes, "urn:change-saga:test:saga")
	if len(files) != 2 {
		t.Fatalf("files = %d, want renamed file plus another file", len(files))
	}
	if files[0].ID == files[1].ID {
		t.Fatal("file DOM anchors must be collision resistant")
	}
	for _, file := range files {
		if file.Path == "new.go" && len(file.Atoms) != 3 {
			t.Fatalf("renamed file has %d atoms, want 3", len(file.Atoms))
		}
	}
}

// codeRecordJSON authors an evidence record from {location, note} pairs,
// reading each digest from repo exactly as an author's tooling would.
func codeRecordJSON(t *testing.T, repo string, entries ...[2]string) string {
	t.Helper()
	resolver, err := coderesolve.New(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	record := saga.CodeFile{Version: 2}
	for _, entry := range entries {
		location, err := coderef.ParseLocation(entry[0])
		if err != nil {
			t.Fatal(err)
		}
		reference, err := resolver.Author(t.Context(), location, entry[1])
		if err != nil {
			t.Fatal(err)
		}
		record.References = append(record.References, reference)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func validServerSaga(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "test.saga")
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"Test","source":{"repository":"https://example.test/a.git"}}`)
	writeServerFeature(t, root)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "content.md"), "# Story\n")
	return root
}

func serverTemplate(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	return tmpl
}

func serverGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeServerFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// serverFeature is the one feature the server fixtures hold their report content in.
// Review records, claims, and verifications stay at the app root.
const serverFeature = "core"

// serverFeatureDir is the fixture feature's directory beneath an app Saga root.
func serverFeatureDir(root string) string { return applayout.FeatureDir(root, serverFeature) }

// writeServerFeature writes the fixture feature's manifest.
func writeServerFeature(t *testing.T, root string) {
	t.Helper()
	writeServerFile(t, filepath.Join(serverFeatureDir(root), applayout.FeatureManifestName),
		`{"$schema":"https://changesaga.dev/schema/v5/feature.schema.json","version":5,"id":"core","title":"Core","created_at":"2026-08-21T12:00:00Z"}`)
}

// Documentation has no approve, reject, or comment control in either mode;
// each record offers its history, and comparing adds the Change tab.
func TestObservingRendersNoApprovalControls(t *testing.T) {
	root := validServerSaga(t)
	render := func(rng gitdiff.Range) string {
		tmpl, err := newPageTemplateFor(rng)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		(&app{root: root, sourceDir: root, rng: rng, template: tmpl}).page(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("page = %d", recorder.Code)
		}
		return recorder.Body.String()
	}
	observed := render(gitdiff.Range{})
	if strings.Contains(observed, "data-review-decision=") || strings.Contains(observed, "data-review-progress") || !strings.Contains(observed, "data-open-history") || !strings.Contains(observed, `data-opening="observe"`) {
		t.Fatal("observe mode rendered approval controls or no history")
	}
	compared := render(gitdiff.Range{Against: "main"})
	if strings.Contains(compared, "data-review-decision=") || strings.Contains(compared, "data-approval-gate") || strings.Contains(compared, "data-review-comment") || !strings.Contains(compared, `data-view-tab="change"`) || !strings.Contains(compared, `data-opening="compare"`) {
		t.Fatal("compare mode rendered approval controls or lost the Change tab")
	}
}
