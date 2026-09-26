package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestEmbeddedDeckRendersInTheDeckViewer(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "visual.saga")
	assetName := writeEmbeddedSlideFixture(t, root)
	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load embedded-deck saga: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	reportRoot, slideRoot := splitReportAndDeckSections(document.Section)
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	data := pageData{
		Saga: document, EmbeddedDecks: true,
		Root: makeSectionView(reportRoot, viewScope{}), SlideRoot: makeSectionView(slideRoot, viewScope{}),
		Nav: append(makeNavTree(reportRoot), makeDeckNavTree(slideRoot)...),
	}
	if err := tmpl.ExecuteTemplate(&rendered, "page", data); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	slideTarget := saga.SlideTarget("visual", "change")
	for _, contract := range []string{`id="view-slides"`, `class="sidebar-slide-surface"`, `data-deck-viewer`, `data-deck-slide`, `data-deck-target=`, `data-slide-present`, `data-slide-exit-presentation`, `data-deck-toggle`, `class="doc-node doc-slide-thumbnail"`, `class="slide-section-divider" data-slide-section>Architecture`, `data-slide-thumbnail`, `data-slide-target="` + slideTarget + `"`, `#i-deck`, `/f/change/` + assetName} {
		if !strings.Contains(html, contract) {
			t.Fatalf("embedded deck contract %q missing:\n%s", contract, html)
		}
	}
	if strings.Contains(html, `data-view-tab="slides"`) || strings.Contains(html, `class="embedded-slide-surface"`) {
		t.Fatalf("embedded decks should live in the report sidebar instead of a second tab or rail:\n%s", html)
	}
	if !strings.Contains(html, "Living overview") || !strings.Contains(html, "Complex flow") {
		t.Fatalf("report or deck surface disappeared:\n%s", html)
	}

	for _, contract := range []string{"requestFullscreen", "fullscreenchange", "presentation-mode", "data-slide-thumbnail"} {
		if !strings.Contains(appJavaScript, contract) {
			t.Fatalf("slide interaction contract %q missing", contract)
		}
	}
	for _, contract := range []string{
		`.slide-thumbnail-hit:hover,.slide-thumbnail-hit:active{background:transparent}`,
		`body.presentation-mode .diff-drawer,body.presentation-mode .drawer-backdrop{display:none}`,
	} {
		if !strings.Contains(pageStyles, contract) {
			t.Fatalf("slide overlay isolation contract %q missing", contract)
		}
	}
	if !strings.Contains(appJavaScript, `if (q('.diff-drawer.open')) closeDrawer();`) {
		t.Fatal("presentation mode did not close the review drawer before entering fullscreen")
	}
}

// The asset route resolves an embedded slide's asset through its slide
// target, even though the slide lives in a deck bundle under ___slides/.
func TestEmbeddedDeckAssetRouteResolvesTheSlideTarget(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "visual.saga")
	assetName := writeEmbeddedSlideFixture(t, root)

	request := httptest.NewRequest(http.MethodGet, "/f/change/"+assetName, nil)
	recorder := httptest.NewRecorder()
	newMux(&app{root: root}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Complex flow") {
		t.Fatalf("embedded slide asset was not served: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEmbeddedDeckCoverageCarriesNoReviewControls(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Test")
	serverGit(t, repo, "config", "user.email", "test@example.test")
	writeServerFile(t, filepath.Join(repo, "app.go"), "package app\n")
	serverGit(t, repo, "add", "app.go")
	serverGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	writeServerFile(t, filepath.Join(repo, "app.go"), "package app\n\nfunc Ready() bool { return true }\n")
	serverGit(t, repo, "add", "app.go")
	serverGit(t, repo, "commit", "-m", "feature")

	root := filepath.Join(repo, "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	repository, err := coderef.FileRepository(repo)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"visual","title":"Visual review","source":{"repository":"`+repository+`"}}`)
	writeServerFeature(t, root)

	application := &app{root: root, sourceDir: repo, rng: gitdiff.Range{Against: base}, template: serverTemplate(t)}
	handler := newMux(application)
	coverage := httptest.NewRecorder()
	handler.ServeHTTP(coverage, httptest.NewRequest(http.MethodGet, "/api/coverage", nil))
	if coverage.Code != http.StatusOK || !strings.Contains(coverage.Body.String(), `data-review-surface-response="manifest"`) {
		t.Fatalf("flat coverage status=%d body=%s", coverage.Code, coverage.Body.String())
	}

	outline, outlineValidation, err := saga.LoadOutline(root)
	if err != nil || !outlineValidation.Valid || len(outline.Decks[0].Slides[0].Items) != 1 {
		t.Fatalf("outline lost Item targets: valid=%v err=%v issues=%#v", outlineValidation.Valid, err, outlineValidation.Issues)
	}
	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), "data-review-decision") || strings.Contains(page.Body.String(), "/api/thread") {
		t.Fatalf("the documentation deck carried approval or comment controls: status=%d body=%s", page.Code, page.Body.String())
	}
}

func writeEmbeddedSlideFixture(t *testing.T, root string) string {
	t.Helper()
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"visual","title":"Visual review","source":{"repository":"https://example.test/acme/app.git"}}`)
	writeServerFeature(t, root)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Living overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverFeatureDir(root), "overview.fragment", "content.md"), "# Living overview {#living-overview}\n")
	bundle := filepath.Join(serverFeatureDir(root), saga.EmbeddedSlidesDir, "flow"+saga.EmbeddedDeckSuffix)
	deckTarget := saga.DeckTarget("visual", "flow")
	deckName, _ := saga.FlatDeckFilename(deckTarget, 0)
	writeServerFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"flow","title":"Complex flow","role":"change","rank":0,"objective":"Explain the complex implementation."}`)
	slideTarget := saga.SlideTarget("visual", "change")
	slideName, _ := saga.FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeServerFile(t, filepath.Join(bundle, slideName), `{"version":4,"id":"change","deck":"flow","title":"Complex flow","rank":0,"section":"Architecture","intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":"`+assetName+`","takeaway":"The implementation path is explicit.","reading_order":["premise"]}`)
	writeServerFile(t, filepath.Join(bundle, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><text id="premise">Complex flow</text></svg>`)
	itemTarget := saga.ItemTarget("visual", "change", "premise")
	itemName, _ := saga.FlatItemFilename(slideTarget, itemTarget, 0)
	writeServerFile(t, filepath.Join(bundle, itemName), `{"version":4,"id":"premise","slide":"change","rank":0,"kind":"callout","label":"Surprise","description":"The non-obvious implementation path.","selector":{"type":"element","element_id":"premise"},"body":"The implementation path is not the expected one."}`)
	return assetName
}
