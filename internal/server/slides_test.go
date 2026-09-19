package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestEmbeddedDeckRendersInTheDeckViewer(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	assetName := writeEmbeddedSlideFixture(t, root)
	index, indexValidation, err := saga.LoadMutationIndex(root)
	if err != nil || !indexValidation.Valid {
		t.Fatalf("load embedded-deck mutation index: valid=%v err=%v issues=%#v", indexValidation.Valid, err, indexValidation.Issues)
	}
	before, err := indexedReviewFingerprint(t.Context(), index)
	if err != nil {
		t.Fatal(err)
	}
	itemTarget := saga.ItemTarget("visual", "change", "premise")
	if _, err := reviewstore.AddThread(root, itemTarget, "Keep the surprise visible.", saga.Anchor{Type: "target"}, "comment", "", nil); err != nil {
		t.Fatal(err)
	}
	after, err := indexedReviewFingerprint(t.Context(), index)
	if err != nil || after == before {
		t.Fatalf("embedded flat review did not advance fingerprint: before=%q after=%q err=%v", before, after, err)
	}
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
		Nav: append(makeNavTree(reportRoot, nil), makeDeckNavTree(slideRoot)...),
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

	for _, contract := range []string{"requestFullscreen", "fullscreenchange", "presentation-mode", "data-slide-thumbnail", "updateSlideReviewState"} {
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
	if !strings.Contains(appJavaScript, `if (q('.diff-drawer.open')) closeDrawer(false);`) {
		t.Fatal("presentation mode did not close the review drawer before entering fullscreen")
	}
}

// The asset route resolves an embedded slide's asset through its slide
// target, even though the slide lives in a deck bundle under ___slides/.
func TestEmbeddedDeckAssetRouteResolvesTheSlideTarget(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	assetName := writeEmbeddedSlideFixture(t, root)

	request := httptest.NewRequest(http.MethodGet, "/f/change/"+assetName, nil)
	recorder := httptest.NewRecorder()
	newMux(&app{root: root}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Complex flow") {
		t.Fatalf("embedded slide asset was not served: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestEmbeddedDeckCoverageAndActivityUseTheFlatReviewOverlay(t *testing.T) {
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
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"visual","title":"Visual review","source":{"repository":"`+repository+`","base":"`+base+`","head":"HEAD"}}`)
	writeServerEpic(t, root)

	index, validation, err := saga.LoadMutationIndex(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load mutation index: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	before, err := indexedReviewFingerprint(t.Context(), index)
	if err != nil {
		t.Fatalf("fingerprint empty flat review state: %v", err)
	}

	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	handler := newMux(application)
	coverage := httptest.NewRecorder()
	handler.ServeHTTP(coverage, httptest.NewRequest(http.MethodGet, "/api/coverage", nil))
	if coverage.Code != http.StatusOK || !strings.Contains(coverage.Body.String(), `data-review-surface-response="manifest"`) {
		t.Fatalf("flat coverage status=%d body=%s", coverage.Code, coverage.Body.String())
	}

	slideTarget := saga.SlideTarget("visual", "change")
	itemTarget := saga.ItemTarget("visual", "change", "premise")
	decision := url.Values{"target": {slideTarget}, "state": {"approved"}, "body": {"The slide is clear."}}
	decisionResponse := httptest.NewRecorder()
	decisionRequest := httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(decision.Encode()))
	decisionRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.ServeHTTP(decisionResponse, decisionRequest)
	if decisionResponse.Code != http.StatusSeeOther {
		t.Fatalf("flat slide decision status=%d body=%s", decisionResponse.Code, decisionResponse.Body.String())
	}
	itemDecision := url.Values{"target": {itemTarget}, "state": {"approved"}}
	itemDecisionRequest := httptest.NewRequest(http.MethodPost, "/api/review", strings.NewReader(itemDecision.Encode()))
	itemDecisionRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	itemDecisionResponse := httptest.NewRecorder()
	handler.ServeHTTP(itemDecisionResponse, itemDecisionRequest)
	if itemDecisionResponse.Code != http.StatusBadRequest {
		t.Fatalf("flat Item approval status=%d, want 400", itemDecisionResponse.Code)
	}
	if _, err := reviewstore.AddThread(root, itemTarget, "Keep this callout", saga.Anchor{Type: "target"}, "comment", "", nil); err != nil {
		t.Fatal(err)
	}
	after, err := indexedReviewFingerprint(t.Context(), index)
	if err != nil || after == before {
		t.Fatalf("flat review records did not advance fingerprint: before=%q after=%q err=%v", before, after, err)
	}

	outline, outlineValidation, err := saga.LoadOutline(root)
	if err != nil || !outlineValidation.Valid || len(outline.Decks[0].Slides[0].Items) != 1 {
		t.Fatalf("outline lost Item review targets: valid=%v err=%v issues=%#v", outlineValidation.Valid, err, outlineValidation.Issues)
	}
	coverage = httptest.NewRecorder()
	handler.ServeHTTP(coverage, httptest.NewRequest(http.MethodGet, "/api/coverage", nil))
	if coverage.Code != http.StatusOK {
		t.Fatalf("coverage failed after flat review mutation: status=%d body=%s", coverage.Code, coverage.Body.String())
	}

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-review-decided="1"`) || !strings.Contains(page.Body.String(), `data-review-total="3"`) || !strings.Contains(page.Body.String(), `data-slide-review-status data-review-target="`+slideTarget+`" data-review-state="approved"`) {
		t.Fatalf("slide approval did not reach progress and thumbnail state: status=%d body=%s", page.Code, page.Body.String())
	}

	activity := httptest.NewRecorder()
	handler.ServeHTTP(activity, httptest.NewRequest(http.MethodGet, "/api/activity", nil))
	if activity.Code != http.StatusOK {
		t.Fatalf("flat activity status=%d body=%s", activity.Code, activity.Body.String())
	}
	for _, expected := range []string{"The slide is clear.", "Keep this callout", "Complex flow", "Slide", "Surprise", "Item"} {
		if !strings.Contains(activity.Body.String(), expected) {
			t.Fatalf("flat activity is missing %q: %s", expected, activity.Body.String())
		}
	}
	if got := strings.Count(activity.Body.String(), `class="activity-slide-preview"`); got != 2 {
		t.Fatalf("slide and Item activity should both carry their visual slide reference; got %d: %s", got, activity.Body.String())
	}

}

func writeEmbeddedSlideFixture(t *testing.T, root string) string {
	t.Helper()
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"visual","title":"Visual review","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"feature"}}`)
	writeServerEpic(t, root)
	writeServerFile(t, filepath.Join(serverEpicDir(root), "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Living overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(serverEpicDir(root), "overview.fragment", "content.md"), "# Living overview {#living-overview}\n")
	bundle := filepath.Join(serverEpicDir(root), saga.EmbeddedSlidesDir, "flow"+saga.EmbeddedDeckSuffix)
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
