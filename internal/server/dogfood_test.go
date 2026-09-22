package server

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func dogfoodRecords(t *testing.T) (*saga.Saga, requirements.Document, quality.Document) {
	t.Helper()
	requireDogfoodSaga(t)
	document, _, err := saga.LoadNarrative(dogfoodSaga)
	if err != nil {
		t.Fatal(err)
	}
	records, err := requirements.Load(dogfoodSaga, document.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := quality.Load(dogfoodSaga)
	if err != nil {
		t.Fatal(err)
	}
	return document, records, tests
}

// The repository's own app Saga is the real Saga these regressions were
// found on. The tests assert shapes that hold for any content it grows.
var dogfoodSaga = filepath.Join("..", "..", "app.saga")

func requireDogfoodSaga(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dogfoodSaga, saga.ManifestName)); os.IsNotExist(err) {
		t.Skip("the repository's app Saga is intentionally absent during initial-Saga setup")
	} else if err != nil {
		t.Fatal(err)
	}
}

// dogfoodPage renders one reviewer path of the repository's app Saga,
// observing HEAD.
func dogfoodPage(t *testing.T, path string) (int, string) {
	t.Helper()
	requireDogfoodSaga(t)
	tmpl, err := newPageTemplateFor(gitdiff.Range{})
	if err != nil {
		t.Fatal(err)
	}
	application := &app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl}
	recorder := httptest.NewRecorder()
	newMux(application).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code, recorder.Body.String()
}

// dogfoodSidebar is the Contents navigation of one rendered page. The sidebar
// shows one feature at a time, so a row that belongs to a feature is asserted on a
// page inside that feature rather than on the app overview.
func dogfoodSidebar(t *testing.T, path string) string {
	t.Helper()
	_, rest, ok := strings.Cut(dogfoodOK(t, path), `<nav class="doc-tree"`)
	if !ok {
		t.Fatalf("GET %s rendered no sidebar", path)
	}
	sidebar, _, ok := strings.Cut(rest, "</nav>")
	if !ok {
		t.Fatalf("GET %s rendered an unterminated sidebar", path)
	}
	return sidebar
}

func dogfoodOK(t *testing.T, path string) string {
	t.Helper()
	code, body := dogfoodPage(t, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d\n%s", path, code, body)
	}
	return body
}

// disconnectedWriter is a browser that went away: every body write fails.
// It counts status writes, which net/http reports as superfluous after the
// first.
type disconnectedWriter struct {
	header  http.Header
	headers int
}

func (w *disconnectedWriter) Header() http.Header { return w.header }
func (w *disconnectedWriter) WriteHeader(int)     { w.headers++ }

// Write sends the implicit 200 first, as net/http does.
func (w *disconnectedWriter) Write([]byte) (int, error) {
	if w.headers == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return 0, errors.New("broken pipe")
}

// A page whose client disconnects mid-response writes its status once. The
// page used to report the failed write with http.Error, a second WriteHeader
// that the server logged as superfluous.
func TestPageWritesItsStatusOnceWhenTheClientGoesAway(t *testing.T) {
	root := validServerSaga(t)
	writer := &disconnectedWriter{header: http.Header{}}
	(&app{root: root, sourceDir: root, template: serverTemplate(t)}).page(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	if writer.headers > 1 {
		t.Fatalf("page wrote its status %d times", writer.headers)
	}
}

// Only a review's slides are review slides. The slide viewer names an
// implementation deck's and the onboarding deck's slides for what they are.
func TestSlideViewerNamesSlidesByTheirDeckRole(t *testing.T) {
	if strings.Contains(pageStyles, "Review slide") {
		t.Fatal("the slide viewer still labels documentation slides as review slides")
	}
	for _, label := range []string{"'Implementation slide'", "[data-deck-role=onboarding] .fragment-head::before{content:'Onboarding slide'}"} {
		if !strings.Contains(pageStyles, label) {
			t.Fatalf("styles lack %s", label)
		}
	}
	page := dogfoodOK(t, "/")
	for _, role := range []string{`data-deck-role="change"`, `data-deck-role="onboarding"`} {
		if !strings.Contains(page, role) {
			t.Fatalf("slide viewer lacks %s", role)
		}
	}
}

// Sidebar titles wrap instead of truncating, and a row's note never squeezes
// its title: "Design system" read as "Design syste" beside its gap note.
func TestSidebarTitlesWrapInsteadOfTruncating(t *testing.T) {
	for _, rule := range []string{
		".doc-tree .doc-link{white-space:normal;",
		".doc-row:has(>.doc-note){flex-wrap:wrap}",
		".doc-row:has(>.doc-note)>.doc-link{flex:0 0 auto;",
		".doc-tree a.doc-link:has(>.i){display:flex;",
	} {
		if !strings.Contains(pageStyles, rule) {
			t.Fatalf("styles lack %s", rule)
		}
	}
}

// Terms and vocabulary stays shut away from the terms pages, so a story page's
// sidebar still shows the features.
func TestVocabularyOpensOnlyOnTheTermsPages(t *testing.T) {
	vocabularyOpen := func(page string) bool {
		return strings.Contains(page, `aria-expanded="true" aria-controls="nav-terms"`)
	}
	if vocabularyOpen(dogfoodOK(t, "/requirements")) {
		t.Fatal("the vocabulary opened on the requirements page")
	}
	if !vocabularyOpen(dogfoodOK(t, "/terms")) {
		t.Fatal("the vocabulary is shut on its own page")
	}
}

func TestSlideThumbnailCaptionsWrap(t *testing.T) {
	if strings.Contains(pageStyles, ".slide-thumbnail-title{display:block;min-width:0;flex:1;color:inherit;font:11.5px/1.3 var(--ui);overflow:hidden;text-overflow:ellipsis") {
		t.Fatal("slide captions still truncate")
	}
}

// Finding 28: every persona has a page with its description, the stories
// that serve it, and its terms, and the sidebar links to it.
func TestEveryPersonaHasAPage(t *testing.T) {
	_, records, _ := dogfoodRecords(t)
	if len(records.Personas) == 0 {
		t.Skip("the app Saga names no personas")
	}
	root := dogfoodOK(t, "/")
	for _, persona := range records.Personas {
		href := personaHref(persona.Identity.ID)
		if !strings.Contains(root, `href="`+href+`"`) {
			t.Fatalf("the sidebar does not link %s", href)
		}
		page := dogfoodOK(t, href)
		for _, want := range []string{"data-persona-page", "data-persona-served", "data-persona-terms", template.HTMLEscapeString(persona.CurrentRevision.Description)} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s lacks %q", href, want)
			}
		}
	}
	if code, _ := dogfoodPage(t, personaHref("nobody")); code != http.StatusNotFound {
		t.Fatalf("an unknown persona = %d", code)
	}
}

// Findings 28 and 30: every feature has a page with its description, stories,
// and summary, and a feature's design chapters render there, not as top-level
// chapters of the app overview.
func TestEveryFeatureHasAPageHoldingItsDesign(t *testing.T) {
	document, records, tests := dogfoodRecords(t)
	root := dogfoodOK(t, "/")
	for _, feature := range document.Features {
		href := featureHref(feature.ID)
		if !strings.Contains(root, `href="`+href+`"`) {
			t.Fatalf("the sidebar does not link %s", href)
		}
		page := dogfoodOK(t, href)
		for _, want := range []string{"data-feature-page", "data-feature-summary"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s lacks %s", href, want)
			}
		}
		hasStories, hasTests := false, false
		for _, story := range records.Stories {
			hasStories = hasStories || story.Feature == feature.ID
		}
		for _, testCase := range tests.TestCases {
			hasTests = hasTests || testCase.Feature == feature.ID
		}
		hasDesign := feature.Design != nil && len(feature.Design.Children)+len(feature.Design.Fragments) > 0
		for marker, want := range map[string]bool{
			"data-feature-stories":        hasStories,
			"data-feature-design":         hasDesign,
			"data-feature-quality":        hasTests,
			"data-feature-implementation": len(feature.Decks) > 0,
		} {
			if got := strings.Contains(page, marker); got != want {
				t.Fatalf("%s renders %s = %v, want %v", href, marker, got, want)
			}
		}
		for _, manifest := range records.Features {
			if manifest.ID == feature.ID && manifest.Description != "" && !strings.Contains(page, template.HTMLEscapeString(manifest.Description)) {
				t.Fatalf("%s lacks its description", href)
			}
		}
		if feature.Design == nil {
			continue
		}
		for _, chapter := range feature.Design.Children {
			fetch := `data-section-href="/api/section?target=` + template.HTMLEscapeString(url.QueryEscape(chapter.Target)) + `"`
			if !strings.Contains(page, fetch) {
				t.Fatalf("%s does not hold its chapter %s", href, chapter.ID)
			}
			if strings.Contains(root, fetch) {
				t.Fatalf("the overview still lists the feature chapter %s", chapter.ID)
			}
			// The feature's own page is where its chapters are in the sidebar:
			// that page's feature is the one the sidebar shows.
			if !strings.Contains(dogfoodSidebar(t, href), `href="`+href+`#`+domID(chapter.Target)+`"`) {
				t.Fatalf("the sidebar does not open %s on its feature's page", chapter.ID)
			}
		}
	}
}

// Finding 27: every test case is a sidebar row under its feature's Quality and
// has a page with its definition, the criteria it verifies, its evidence
// code, and its runs.
func TestEveryTestCaseHasARowAndAPage(t *testing.T) {
	_, records, tests := dogfoodRecords(t)
	if len(tests.TestCases) == 0 {
		t.Skip("the app Saga has no test cases")
	}
	for _, testCase := range tests.TestCases {
		href := testCaseHref(testCase.Identity.ID)
		// Quality lists the test cases of the feature the sidebar shows, so the
		// row is asserted on the test case's own page.
		sidebar := dogfoodSidebar(t, href)
		if strings.Contains(sidebar, "no test cases yet") {
			t.Fatalf("the feature of %s still says it has no test cases", href)
		}
		if !strings.Contains(sidebar, `href="`+href+`"`) {
			t.Fatalf("the sidebar does not list %s", href)
		}
		page := dogfoodOK(t, href)
		for _, want := range []string{"data-test-definition", "data-test-verifies", "data-test-evidence", "data-test-runs", template.HTMLEscapeString(testCase.CurrentRevision.Title)} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s lacks %q", href, want)
			}
		}
		for _, step := range testCase.CurrentRevision.Steps {
			if !strings.Contains(page, template.HTMLEscapeString(step.Action)) {
				t.Fatalf("%s lacks step %q", href, step.Action)
			}
		}
		urn := "urn:change-saga:" + tests.SagaID + ":test-case:" + testCase.Identity.ID
		for _, relation := range records.Relations {
			if relation.From == urn && relation.State == requirements.RelationActive {
				parts := strings.Split(relation.To, ":")
				if len(parts) == 7 && !strings.Contains(page, `href="`+requirementCriterionHref(parts[4], parts[6])+`"`) {
					t.Fatalf("%s does not link the criterion it verifies: %s", href, relation.To)
				}
			}
		}
		if len(testCase.Evidence) > 0 && !strings.Contains(page, "data-test-code") {
			t.Fatalf("%s renders none of its evidence code", href)
		}
		for _, run := range testCase.Runs {
			if !strings.Contains(page, `data-test-run="`+run.ID+`"`) {
				t.Fatalf("%s lacks run %s", href, run.ID)
			}
		}
		if featurePage := dogfoodOK(t, featureHref(testCase.Feature)); !strings.Contains(featurePage, `href="`+href+`"`) {
			t.Fatalf("the feature page does not list %s", href)
		}
	}
}

// Finding 29: a story page names its feature, personas, and citations, and the
// design, slides, and test cases linked to it and to each criterion; each
// criterion has its own traceability view; the sidebar names stories by
// title, not by ordinal.
func TestStoriesAndCriteriaShowTheirTraceability(t *testing.T) {
	document, records, _ := dogfoodRecords(t)
	graph := newAppGraph(document, records, quality.Document{})
	for _, story := range records.Stories {
		if story.CurrentRevision == nil {
			continue
		}
		href := requirementStoryHref(story.Identity.ID)
		page := dogfoodOK(t, href)
		// A story's page shows that story's feature, so its Requirements row is
		// in that page's own sidebar.
		sidebar := dogfoodSidebar(t, href)
		if strings.Contains(sidebar, ">Story 0") || strings.Contains(sidebar, "Story 01 ·") {
			t.Fatal("the sidebar still names stories by ordinal")
		}
		if !strings.Contains(sidebar, `title="`+template.HTMLEscapeString(story.CurrentRevision.Title)+`"`) {
			t.Fatalf("the sidebar does not name %s by its title", href)
		}
		for _, want := range []string{"data-story-context", "data-story-trace", `href="` + featureHref(story.Feature) + `"`} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s lacks %s", href, want)
			}
		}
		for _, persona := range story.CurrentRevision.Personas {
			if !strings.Contains(page, `data-story-persona="`+persona+`"`) {
				t.Fatalf("%s does not name persona %s", href, persona)
			}
		}
		if got := strings.Count(page, "data-story-citation"); got != len(story.CurrentRevision.Citations) {
			t.Fatalf("%s shows %d citations, want %d", href, got, len(story.CurrentRevision.Citations))
		}
		storyURN := "urn:change-saga:" + records.SagaID + ":story:" + story.Identity.ID
		for _, link := range append(append(graph.traceTo(storyURN).Design, graph.traceTo(storyURN).Slides...), graph.traceTo(storyURN).Tests...) {
			if !strings.Contains(page, `href="`+template.HTMLEscapeString(link.Href)+`"`) {
				t.Fatalf("%s does not link %s", href, link.Target)
			}
		}
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			criterionPage := dogfoodOK(t, requirementCriterionHref(story.Identity.ID, criterion.ID))
			if !strings.Contains(criterionPage, "data-criterion-page") || !strings.Contains(criterionPage, template.HTMLEscapeString(criterion.Statement)) {
				t.Fatalf("criterion %s/%s has no traceability view", story.Identity.ID, criterion.ID)
			}
			trace := graph.traceTo(storyURN + ":criterion:" + criterion.ID)
			for _, link := range append(append(trace.Design, trace.Slides...), trace.Tests...) {
				if !strings.Contains(criterionPage, `href="`+template.HTMLEscapeString(link.Href)+`"`) || !strings.Contains(page, `href="`+template.HTMLEscapeString(link.Href)+`"`) {
					t.Fatalf("criterion %s/%s does not link %s on both pages", story.Identity.ID, criterion.ID, link.Target)
				}
			}
		}
	}
}

// Finding 31: /requirements groups stories under their features and no longer
// titles the elevator pitch "Rationale".
func TestRequirementsOverviewIsGroupedByFeature(t *testing.T) {
	_, records, _ := dogfoodRecords(t)
	page := dogfoodOK(t, "/requirements")
	page = page[strings.Index(page, "data-requirements-page"):]
	if strings.Contains(page, "Rationale") {
		t.Fatal("the requirements overview still has a Rationale heading")
	}
	for _, story := range records.Stories {
		if story.CurrentRevision == nil {
			continue
		}
		group := strings.Index(page, `data-requirements-feature="urn:change-saga:`+records.SagaID+`:feature:`+story.Feature+`"`)
		card := strings.Index(page, `href="`+requirementStoryHref(story.Identity.ID)+`"`)
		if group < 0 || card < group {
			t.Fatalf("story %s is not listed under its feature %s", story.Identity.ID, story.Feature)
		}
		if next := strings.Index(page[group+1:], "data-requirements-feature="); next >= 0 && card > group+1+next {
			t.Fatalf("story %s is listed under another feature", story.Identity.ID)
		}
	}
}

// Finding 32: observing, Coverage shows the documented code. Every Item that
// references code is a Saga → Code row whose code renders at the head, and
// every referenced file is a Code → Saga row.
func TestObservedCoverageShowsTheDocumentedCode(t *testing.T) {
	requireDogfoodSaga(t)
	document, _, err := saga.Load(dogfoodSaga)
	if err != nil {
		t.Fatal(err)
	}
	var items []*saga.Item
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if len(item.Code) > 0 {
					items = append(items, item)
				}
			}
		}
	}
	if len(items) == 0 {
		t.Skip("no Item references code")
	}
	sagaToCode := dogfoodOK(t, "/api/coverage?mode=saga")
	if !strings.Contains(sagaToCode, "data-observe-coverage") {
		t.Fatal("observing did not render the documented-code coverage")
	}
	for _, item := range items {
		if !strings.Contains(sagaToCode, `data-observe-target="`+item.Target+`"`) {
			t.Fatalf("Saga → Code lacks the Item %s", item.Target)
		}
		code := dogfoodOK(t, "/api/reference-code?target="+url.QueryEscape(item.Target))
		if !strings.Contains(code, "data-reference-code") || !strings.Contains(code, "<code data-code>") {
			t.Fatalf("the Item %s rendered no code", item.Target)
		}
		codeToSaga := dogfoodOK(t, "/api/coverage?mode=code")
		for _, file := range item.Code {
			for _, reference := range file.References {
				if !strings.Contains(codeToSaga, `data-observe-file="`+reference.Path+`"`) && !strings.Contains(sagaToCode, "stale") {
					t.Fatalf("Code → Saga lacks %s", reference.Path)
				}
			}
		}
	}
	if strings.Contains(sagaToCode, "0 files · 0 changes") {
		t.Fatal("observed coverage still reports an empty comparison")
	}
}

// Finding 32: the overview's coverage line is replaced once it can be read,
// instead of saying "Building the review index…" forever.
func TestOverviewCoverageLineIsFilledIn(t *testing.T) {
	root := dogfoodOK(t, "/")
	if !strings.Contains(root, `data-totals-href="/api/totals"`) || strings.Contains(root, "Building the review index") {
		t.Fatal("the observed overview does not ask for its coverage line")
	}
	if !strings.Contains(appJavaScript, "loadCoverageTotals()") || !strings.Contains(appJavaScript, "hydrateLazyDetails(details)") {
		t.Fatal("the page script never fills in the coverage line or opens reference code")
	}
	if totals := dogfoodOK(t, "/api/totals"); !strings.Contains(totals, "data-observe-totals") || !strings.Contains(totals, "records reference") && !strings.Contains(totals, "record references") {
		t.Fatalf("coverage totals = %s", totals)
	}
}

// Comparing, the coverage line waits for the comparison: the totals endpoint
// asks the browser to retry while it builds, so the line updates when ready.
func TestComparedCoverageLineRetriesWhileTheComparisonBuilds(t *testing.T) {
	application := &app{rng: gitdiff.Range{Against: "main"}}
	application.cache.building = true
	recorder := httptest.NewRecorder()
	newMux(application).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/totals", nil))
	if recorder.Code != http.StatusAccepted || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("totals while building = %d", recorder.Code)
	}
}

// The documentation carries no approval or comment control on any page,
// including the new ones; only a review's slides do.
func TestDocumentationPagesHaveNoApprovalOrCommentControls(t *testing.T) {
	document, records, tests := dogfoodRecords(t)
	paths := []string{"/", "/requirements", "/terms"}
	for _, story := range records.Stories {
		paths = append(paths, requirementStoryHref(story.Identity.ID))
		if story.CurrentRevision != nil && len(story.CurrentRevision.AcceptanceCriteria) > 0 {
			paths = append(paths, requirementCriterionHref(story.Identity.ID, story.CurrentRevision.AcceptanceCriteria[0].ID))
		}
	}
	for _, persona := range records.Personas {
		paths = append(paths, personaHref(persona.Identity.ID))
	}
	for _, feature := range document.Features {
		paths = append(paths, featureHref(feature.ID))
	}
	for _, testCase := range tests.TestCases {
		paths = append(paths, testCaseHref(testCase.Identity.ID))
	}
	paths = append(paths, "/personas", "/flags", "/features", designSystemPath)
	for _, path := range paths {
		page := dogfoodOK(t, path)
		for _, control := range []string{`<form method="post"`, "data-review-decision", "data-review-comment", "Approve slide", "Request changes", "<textarea"} {
			if strings.Contains(page, control) {
				t.Fatalf("%s carries a review control: %s", path, control)
			}
		}
		// A directory's filter is the one form documentation carries, and it
		// only ever reads: every form on a documentation page is a GET.
		for _, form := range strings.Split(page, "<form")[1:] {
			if !strings.HasPrefix(form, ` class="directory-filter" method="get"`) {
				t.Fatalf("%s carries a form that is not a directory filter: <form%s", path, form[:min(len(form), 80)])
			}
		}
	}
}
