package server

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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

// The repository's own Saga is the real Saga these regressions were
// found on. The tests assert shapes that hold for any content it grows.
var dogfoodSaga = filepath.Join("..", "..", "change.saga")

func requireDogfoodSaga(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dogfoodSaga, saga.ManifestName)); os.IsNotExist(err) {
		t.Skip("the repository's Saga is intentionally absent during initial-Saga setup")
	} else if err != nil {
		t.Fatal(err)
	}
}

// dogfoodServer is one reviewer observing HEAD of the repository's Saga,
// shared by every dogfood request as a running server is shared by a
// reviewer's requests. Its caches are keyed on what they read, so a warm
// cache serves exactly what a cold one would build; sharing it only stops
// each request from rebuilding the derived indexes of the whole Saga.
var dogfoodServer struct {
	once    sync.Once
	handler http.Handler
	err     error
}

// dogfoodPage renders one reviewer path of the repository's Saga,
// observing HEAD.
func dogfoodPage(t *testing.T, path string) (int, string) {
	t.Helper()
	requireDogfoodSaga(t)
	dogfoodServer.once.Do(func() {
		tmpl, err := newPageTemplateFor(gitdiff.Range{})
		if err != nil {
			dogfoodServer.err = err
			return
		}
		dogfoodServer.handler = newMux(&app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl})
	})
	if dogfoodServer.err != nil {
		t.Fatal(dogfoodServer.err)
	}
	recorder := httptest.NewRecorder()
	dogfoodServer.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder.Code, recorder.Body.String()
}

// dogfoodLinks is the Saga's records joined the way its pages read them. A check
// of every record is made against this, because rendering every page tests the
// same templates hundreds of times; each kind's page is rendered once.
var dogfoodLinks struct {
	once  sync.Once
	graph *appGraph
	code  []documentedCode
	err   error
}

func dogfoodGraph(t *testing.T) (*appGraph, []documentedCode) {
	t.Helper()
	requireDogfoodSaga(t)
	dogfoodLinks.once.Do(func() {
		dogfoodLinks.graph, dogfoodLinks.code, dogfoodLinks.err = loadObserveGraph(dogfoodSaga)
	})
	if dogfoodLinks.err != nil {
		t.Fatal(dogfoodLinks.err)
	}
	return dogfoodLinks.graph, dogfoodLinks.code
}

// richestStory is the story whose page has the most to show.
func richestStory(graph *appGraph) *requirements.Story {
	var richest *requirements.Story
	best := -1
	for index := range graph.requirements.Stories {
		story := &graph.requirements.Stories[index]
		if story.CurrentRevision == nil {
			continue
		}
		urn := "urn:change-saga:" + graph.requirements.SagaID + ":story:" + story.Identity.ID
		trace := graph.traceTo(urn)
		score := len(story.CurrentRevision.Personas) + len(story.CurrentRevision.Citations) + len(story.CurrentRevision.AcceptanceCriteria) + len(trace.Design) + len(trace.Slides) + len(trace.Tests)
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			crit := graph.traceTo(urn + ":criterion:" + criterion.ID)
			score += len(crit.Design) + len(crit.Slides) + len(crit.Tests)
		}
		if score > best {
			richest, best = story, score
		}
	}
	return richest
}

var sidebarNavTags = regexp.MustCompile(`<nav\b[^>]*>|</nav\s*>`)

// pageSidebar is the Contents navigation of page, already rendered at path.
// The sidebar shows one feature at a time, so a row that belongs to a feature
// is asserted on a page inside that feature rather than on the app overview.
func pageSidebar(t *testing.T, path, page string) string {
	t.Helper()
	start := strings.Index(page, `<nav class="doc-tree"`)
	if start < 0 {
		t.Fatalf("GET %s rendered no sidebar", path)
	}
	// The feature-grouping menu is a nested nav. Match the outer Contents
	// navigation so assertions also inspect feature rows after that menu.
	depth := 0
	for cursor := start; cursor < len(page); {
		span := sidebarNavTags.FindStringIndex(page[cursor:])
		if span == nil {
			break
		}
		tag := page[cursor+span[0] : cursor+span[1]]
		cursor += span[1]
		if strings.HasPrefix(tag, "</") {
			depth--
		} else {
			depth++
		}
		if depth == 0 {
			return page[start:cursor]
		}
	}
	t.Fatalf("GET %s rendered an unterminated sidebar", path)
	return ""
}

func TestPageSidebarIncludesRowsAfterNestedNavigation(t *testing.T) {
	for _, menu := range []string{"", `<nav class="feature-grouping"><a href="/features?group=all">None</a></nav>`} {
		page := `<nav class="doc-tree" aria-label="Contents">` + menu + `<a href="/requirements/story">Story title</a></nav><nav><a href="/outside">Outside</a></nav>`
		sidebar := pageSidebar(t, "/", page)
		if !strings.Contains(sidebar, `href="/requirements/story"`) || strings.Contains(sidebar, `href="/outside"`) {
			t.Fatalf("wrong Contents boundary: %s", sidebar)
		}
	}
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
	t.Parallel()
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
	t.Parallel()
	if strings.Contains(pageStyles, "Review slide") {
		t.Fatal("the slide viewer still labels documentation slides as review slides")
	}
	for _, label := range []string{"'Technical slide'", "[data-deck-role=onboarding] .fragment-head::before{content:'Onboarding slide'}"} {
		if !strings.Contains(pageStyles, label) {
			t.Fatalf("styles lack %s", label)
		}
	}
	page := dogfoodOK(t, "/decks")
	for _, role := range []string{`data-deck-role="change"`, `data-deck-role="onboarding"`} {
		if !strings.Contains(page, role) {
			t.Fatalf("slide viewer lacks %s", role)
		}
	}
}

// Sidebar titles wrap instead of truncating, and a row's note never squeezes
// its title: "Design system" read as "Design syste" beside its gap note.
func TestSidebarTitlesWrapInsteadOfTruncating(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	if strings.Contains(pageStyles, ".slide-thumbnail-title{display:block;min-width:0;flex:1;color:inherit;font:11.5px/1.3 var(--ui);overflow:hidden;text-overflow:ellipsis") {
		t.Fatal("slide captions still truncate")
	}
}

// Finding 28: every persona has a page with its description, the stories
// that serve it, and its terms, and the sidebar links to it.
func TestEveryPersonaHasAPage(t *testing.T) {
	t.Parallel()
	_, records, _ := dogfoodRecords(t)
	if len(records.Personas) == 0 {
		t.Skip("the Saga names no personas")
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
	t.Parallel()
	graph, _ := dogfoodGraph(t)
	root := dogfoodOK(t, "/")
	abstractOf := func(feature *saga.Feature) string {
		abstract := ""
		for _, manifest := range graph.requirements.Features {
			if manifest.ID == feature.ID {
				abstract = manifest.Description
			}
		}
		// An authored Essay abstract takes precedence over the manifest's
		// fallback description; assert the actual reading contract in both cases.
		if overview := feature.Overview(); overview != nil && overview.Abstract != "" {
			abstract = overview.Abstract
		}
		return abstract
	}
	expected := func(feature *saga.Feature) map[string]bool {
		hasStories, hasTests := false, false
		for _, story := range graph.requirements.Stories {
			hasStories = hasStories || story.Feature == feature.ID
		}
		for _, testCase := range graph.quality.TestCases {
			hasTests = hasTests || testCase.Feature == feature.ID
		}
		return map[string]bool{
			"data-feature-stories":        hasStories,
			"data-feature-design":         feature.Design != nil && len(feature.Design.Children)+len(feature.Design.Fragments) > 0,
			"data-feature-quality":        hasTests,
			"data-feature-implementation": len(feature.Decks) > 0,
		}
	}
	chapterFetch := func(chapter *saga.Section) string {
		return `data-section-href="/api/section?target=` + template.HTMLEscapeString(url.QueryEscape(chapter.Target)) + `"`
	}
	var fullest, sparsest *saga.Feature
	shown := func(feature *saga.Feature) int {
		count := 0
		for _, present := range expected(feature) {
			if present {
				count++
			}
		}
		return count
	}
	for _, feature := range graph.document.Features {
		href := featureHref(feature.ID)
		if !strings.Contains(root, `href="`+href+`"`) {
			t.Fatalf("the sidebar does not link %s", href)
		}
		view, err := graph.featurePage(feature.ID)
		if err != nil {
			t.Fatalf("%s: %v", href, err)
		}
		want := expected(feature)
		for marker, got := range map[string]bool{
			"data-feature-stories":        len(view.Stories) > 0,
			"data-feature-design":         view.Design != nil,
			"data-feature-quality":        len(view.Tests) > 0,
			"data-feature-implementation": len(view.Decks) > 0,
		} {
			if got != want[marker] {
				t.Fatalf("%s holds %s = %v, want %v", href, marker, got, want[marker])
			}
		}
		if view.Abstract != abstractOf(feature) {
			t.Fatalf("%s reads the abstract %q, want %q", href, view.Abstract, abstractOf(feature))
		}
		if feature.Design != nil {
			for _, chapter := range feature.Design.Children {
				if strings.Contains(root, chapterFetch(chapter)) {
					t.Fatalf("the overview still lists the feature chapter %s", chapter.ID)
				}
			}
		}
		if fullest == nil || shown(feature) > shown(fullest) || shown(feature) == shown(fullest) && feature.Design != nil && len(feature.Design.Children) > 0 && (fullest.Design == nil || len(fullest.Design.Children) == 0) {
			fullest = feature
		}
		if sparsest == nil || shown(feature) < shown(sparsest) {
			sparsest = feature
		}
	}
	// The template follows those fields: one page with the most to show, and one
	// with the least, so a section appears only when it has something.
	for _, feature := range []*saga.Feature{fullest, sparsest} {
		href := featureHref(feature.ID)
		page := dogfoodOK(t, href)
		for _, want := range []string{"data-feature-page", "data-feature-summary"} {
			if !strings.Contains(page, want) {
				t.Fatalf("%s lacks %s", href, want)
			}
		}
		for marker, want := range expected(feature) {
			if got := strings.Contains(page, marker); got != want {
				t.Fatalf("%s renders %s = %v, want %v", href, marker, got, want)
			}
		}
		if abstract := abstractOf(feature); abstract != "" && !strings.Contains(page, string(essayAbstract(abstract, feature.Target))) {
			t.Fatalf("%s lacks its abstract", href)
		}
		if feature.Design == nil {
			continue
		}
		for _, chapter := range feature.Design.Children {
			if !strings.Contains(page, chapterFetch(chapter)) {
				t.Fatalf("%s does not hold its chapter %s", href, chapter.ID)
			}
			// The feature's own page is where its chapters are in the sidebar:
			// that page's feature is the one the sidebar shows.
			if !strings.Contains(pageSidebar(t, href, page), `href="`+href+`#`+domID(chapter.Target)+`"`) {
				t.Fatalf("the sidebar does not open %s on its feature's page", chapter.ID)
			}
		}
	}
}

// Finding 27: every test case is a sidebar row under its feature's Quality and
// has a page with its definition, the criteria it verifies, its evidence
// code, and its runs.
func TestEveryTestCaseHasARowAndAPage(t *testing.T) {
	t.Parallel()
	graph, _ := dogfoodGraph(t)
	tests := graph.quality
	if len(tests.TestCases) == 0 {
		t.Skip("the Saga has no test cases")
	}
	listed := func(nodes []*navNodeView, href string) bool {
		for _, node := range nodes {
			if node.Href == href {
				return true
			}
		}
		return false
	}
	var richest *quality.TestCase
	links := func(testCase *quality.TestCase) []requirements.Relation {
		return graph.outbound["urn:change-saga:"+tests.SagaID+":test-case:"+testCase.Identity.ID]
	}
	for index := range tests.TestCases {
		testCase := &tests.TestCases[index]
		href := testCaseHref(testCase.Identity.ID)
		if testCase.CurrentRevision == nil || testCase.CurrentRevision.Title == "" {
			t.Fatalf("%s has no definition to show", href)
		}
		// Quality lists the test cases of the feature the sidebar shows, and the
		// feature's own page lists them too.
		if !listed(testCaseNav(tests, testCase.Feature), href) {
			t.Fatalf("the sidebar does not list %s", href)
		}
		view, err := graph.featurePage(testCase.Feature)
		if err != nil {
			t.Fatalf("%s: %v", href, err)
		}
		onFeature := false
		for _, link := range view.Tests {
			onFeature = onFeature || link.Href == href
		}
		if !onFeature {
			t.Fatalf("the feature page does not list %s", href)
		}
		for _, relation := range links(testCase) {
			if relation.State != requirements.RelationActive {
				continue
			}
			parts := strings.Split(relation.To, ":")
			if len(parts) == 7 && graph.link(relation.To).Href != requirementCriterionHref(parts[4], parts[6]) {
				t.Fatalf("%s does not link the criterion it verifies: %s", href, relation.To)
			}
		}
		if richest == nil || len(links(testCase))+len(testCase.Evidence)+len(testCase.Runs) > len(links(richest))+len(richest.Evidence)+len(richest.Runs) {
			richest = testCase
		}
	}
	// One page shows all of that: its definition, the criteria it verifies, its
	// evidence code, and its runs.
	testCase := richest
	href := testCaseHref(testCase.Identity.ID)
	page := dogfoodOK(t, href)
	sidebar := pageSidebar(t, href, page)
	if strings.Contains(sidebar, "no test cases yet") {
		t.Fatalf("the feature of %s still says it has no test cases", href)
	}
	if !strings.Contains(sidebar, `href="`+href+`"`) {
		t.Fatalf("the sidebar does not list %s", href)
	}
	for _, want := range []string{"data-test-definition", "data-test-verifies", "data-test-evidence", "data-test-definition-stage", template.HTMLEscapeString(testCase.CurrentRevision.Title)} {
		if !strings.Contains(page, want) {
			t.Fatalf("%s lacks %q", href, want)
		}
	}
	for _, step := range testCase.CurrentRevision.Steps {
		if !strings.Contains(page, template.HTMLEscapeString(step.Action)) {
			t.Fatalf("%s lacks step %q", href, step.Action)
		}
	}
	for _, relation := range links(testCase) {
		if parts := strings.Split(relation.To, ":"); relation.State == requirements.RelationActive && len(parts) == 7 && !strings.Contains(page, `href="`+requirementCriterionHref(parts[4], parts[6])+`"`) {
			t.Fatalf("%s does not link the criterion it verifies: %s", href, relation.To)
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

// Finding 29: a story page names its feature, personas, and citations, and the
// design, slides, and test cases linked to it and to each criterion; each
// criterion has its own traceability view; the sidebar names stories by
// title, not by ordinal.
func TestStoriesAndCriteriaShowTheirTraceability(t *testing.T) {
	t.Parallel()
	graph, _ := dogfoodGraph(t)
	records := graph.requirements
	surface, nav, err := makeRequirementsSurface(records, requirementRoute{})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]string{}
	for _, node := range nav.Children {
		if strings.HasPrefix(node.Title, "Story 0") {
			t.Fatal("the sidebar still names stories by ordinal")
		}
		rows[node.Href] = node.Title
	}
	graph.decorateRequirements(surface)
	viewOf := map[string]*requirementStoryView{}
	for _, view := range surface.Stories {
		viewOf[view.ID] = view
	}
	for _, story := range records.Stories {
		if story.CurrentRevision == nil {
			continue
		}
		href := requirementStoryHref(story.Identity.ID)
		if rows[href] != story.CurrentRevision.Title {
			t.Fatalf("the sidebar names %s %q, not by its title", href, rows[href])
		}
		if viewOf[story.Identity.ID].FeatureLink.Href != featureHref(story.Feature) {
			t.Fatalf("%s does not link its feature", href)
		}
		storyURN := "urn:change-saga:" + records.SagaID + ":story:" + story.Identity.ID
		targets := []string{storyURN}
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			targets = append(targets, storyURN+":criterion:"+criterion.ID)
		}
		for _, target := range targets {
			trace := graph.traceTo(target)
			for _, link := range append(append(trace.Design, trace.Slides...), trace.Tests...) {
				if link.Href == "" {
					t.Fatalf("%s traces to %s, which goes nowhere", target, link.Target)
				}
			}
		}
	}
	// The pages of the story with the most links show what those records hold.
	story := richestStory(graph)
	href := requirementStoryHref(story.Identity.ID)
	page := dogfoodOK(t, href)
	// A story's page shows that story's feature, so its Requirements row is
	// in that page's own sidebar.
	if sidebar := pageSidebar(t, href, page); !strings.Contains(sidebar, `title="`+template.HTMLEscapeString(story.CurrentRevision.Title)+`"`) {
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
	trace := graph.traceTo(storyURN)
	for _, link := range append(append(trace.Design, trace.Slides...), trace.Tests...) {
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

// Finding 31: /requirements groups stories under their features and no longer
// titles the elevator pitch "Rationale".
func TestRequirementsOverviewIsGroupedByFeature(t *testing.T) {
	t.Parallel()
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
	// It only reads the shared dogfood server, which serves concurrent
	// requests as a reviewer's server does, so it runs beside the others.
	t.Parallel()
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
	// Code → Saga is one page for the whole Saga; every Item is checked
	// against the same rendering.
	codeToSaga := dogfoodOK(t, "/api/coverage?mode=code")
	for _, item := range items {
		if !strings.Contains(sagaToCode, `data-observe-target="`+item.Target+`"`) {
			t.Fatalf("Saga → Code lacks the Item %s", item.Target)
		}
		for _, file := range item.Code {
			for _, reference := range file.References {
				if !strings.Contains(codeToSaga, `data-observe-file="`+reference.Path+`"`) && !strings.Contains(sagaToCode, "stale") {
					t.Fatalf("Code → Saga lacks %s", reference.Path)
				}
			}
		}
	}
	// Whether each reference still resolves is what health checks of every record
	// on every pull request. Here the first and last Item show their code renders.
	for _, item := range []*saga.Item{items[0], items[len(items)-1]} {
		code := dogfoodOK(t, "/api/reference-code?target="+url.QueryEscape(item.Target))
		if !strings.Contains(code, "data-reference-code") || !strings.Contains(code, "<code data-code>") {
			t.Fatalf("the Item %s rendered no code:\n%s", item.Target, code)
		}
	}
	if strings.Contains(sagaToCode, "0 files · 0 changes") {
		t.Fatal("observed coverage still reports an empty comparison")
	}
}

// Finding 32: the overview's coverage line is replaced once it can be read,
// instead of saying "Building the review index…" forever.
func TestOverviewCoverageLineIsFilledIn(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	graph, _ := dogfoodGraph(t)
	// A review control can only come from a template: authored Markdown is
	// escaped, which TestEssayMarkdownDialectMatchesDeckBodies holds. So one page
	// of each kind answers for every record of that kind.
	paths := []string{"/", "/requirements", "/terms", "/personas", "/flags", "/features", designSystemPath}
	if story := richestStory(graph); story != nil {
		paths = append(paths, requirementStoryHref(story.Identity.ID))
		if criteria := story.CurrentRevision.AcceptanceCriteria; len(criteria) > 0 {
			paths = append(paths, requirementCriterionHref(story.Identity.ID, criteria[0].ID))
		}
	}
	if personas := graph.requirements.Personas; len(personas) > 0 {
		paths = append(paths, personaHref(personas[0].Identity.ID))
	}
	if features := graph.document.Features; len(features) > 0 {
		paths = append(paths, featureHref(features[0].ID))
	}
	if cases := graph.quality.TestCases; len(cases) > 0 {
		paths = append(paths, testCaseHref(cases[0].Identity.ID))
	}
	for _, path := range paths {
		page := dogfoodOK(t, path)
		if control := documentationReviewControl(page); control != "" {
			t.Fatalf("%s carries a review control: %s", path, control)
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

// Documentation can explain review actions in stories and onboarding slides.
// Their labels are ordinary prose, not evidence that an interactive review
// control leaked onto a documentation page. The form-shape check above also
// rejects any form other than the read-only directory filter.
func documentationReviewControl(page string) string {
	for _, control := range []string{`<form method="post"`, "data-review-decision", "data-review-comment", "<textarea"} {
		if strings.Contains(page, control) {
			return control
		}
	}
	return ""
}

func TestDocumentationControlDetectionAllowsReviewActionProse(t *testing.T) {
	t.Parallel()
	if got := documentationReviewControl(`<p>Choosing Request changes records a decision; Approve slide records approval.</p>`); got != "" {
		t.Fatalf("review-action prose was treated as a control: %s", got)
	}
	for _, markup := range []string{
		`<form method="post" action="/review"><button>Approve slide</button></form>`,
		`<button data-review-decision="approve">Approve slide</button>`,
		`<button data-review-comment>Comment</button>`,
		`<textarea name="comment"></textarea>`,
	} {
		if documentationReviewControl(markup) == "" {
			t.Errorf("missed review control: %s", markup)
		}
	}
}
