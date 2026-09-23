package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
	"github.com/twentyideas/changesaga/internal/store"
)

type app struct {
	root      string
	sourceDir string
	// rng is how the reviewer was opened: observe one commit, or compare a
	// change against its merge-base. It never comes from the Saga.
	rng           gitdiff.Range
	template      *template.Template
	shutdownToken string
	// mutationToken authorizes the review page's decision and comment forms.
	// It is random per server process and rendered only into those forms.
	mutationToken string
	shutdown      func()
	cache         snapshotCache
	outline       outlineCache
	catalog       sourceCatalogCache
	evidence      evidenceOwnerCache
	layers        layersCache
	// related is the derived related-reviews index, kept while nothing it
	// reads has changed.
	related relatedReviewCache
	// comparisonLoader is the injectable boundary around the expensive source
	// diff and coverage build. Root and narrative shell handlers must never call
	// it; focused comparison endpoints reach it through snapshot().
	comparisonLoader func(context.Context) (*reviewSnapshot, error)
	// catalogLoader is the bounded changed-file metadata seam. Code navigation
	// uses it instead of comparisonLoader so opening the tab cannot construct
	// every source atom or the coverage ownership graph.
	catalogLoader func(context.Context, saga.Manifest) (gitdiff.Catalog, error)
	// layersLoader is the injectable boundary around the comparison's layers,
	// which the page marks read-only beside the pull request's review.
	layersLoader func(context.Context) (*changeview.Layers, error)
	generations  *snapshotcache.Store
}

// ManagedOptions lets the CLI supervise a detached loopback server without
// weakening the ordinary foreground server. The shutdown token is random,
// stored only in the user's private runtime directory, and never rendered into
// saga content.
type ManagedOptions struct {
	// Range is how the Saga is opened; the zero value observes HEAD.
	Range         gitdiff.Range
	ShutdownToken string
	OnReady       func(string) error
}

type lockedWriter struct {
	mutex sync.Mutex
	value io.Writer
}

func (w *lockedWriter) Write(data []byte) (int, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	return w.value.Write(data)
}

// OpenBrowser opens a trusted loopback URL using the platform launcher. It is
// exported for the CLI's detached-server path, where readiness is observed in
// a different process from the server itself.
func OpenBrowser(rawURL string) error { return launchBrowser(rawURL) }

type pageData struct {
	// Opening says how the reviewer was opened; Comparing is compare mode.
	Opening          string
	Comparing        bool
	Saga             *saga.Saga
	EmbeddedDecks    bool
	RequirementsMode bool
	Requirements     *requirementsPageView
	// TermsMode shows the overview's Terms and vocabulary, or one term.
	TermsMode bool
	Terms     *termsPageView
	// Persona, Feature, and TestCase are the app-level record pages, and
	// Features, Personas, and Flags the directories that list them. At most one
	// is set.
	Persona  *personaPageView
	Feature  *featurePageView
	TestCase *testCasePageView
	Features *directoryView
	Personas *directoryView
	Flags    *directoryView
	// DesignSystemMode is the design system's own page; DesignSystem is its
	// content, which is absent until an author records some.
	DesignSystemMode bool
	DesignSystem     *sectionView
	// OverviewParts is the overview's directory: its parts and what each one
	// holds.
	OverviewParts []overviewPartView
	// PageFeature is the feature this page belongs to, and so the one feature the
	// sidebar opens. Empty when the page belongs to none.
	PageFeature string
	// Reviews is a review surface rendered inside the app shell.
	Reviews template.HTML
	// ReviewSide says the page is on the Review side of the header rather
	// than the Documentation side. The two sides are what the header carries:
	// Documentation is the overview and the feature set, Review the current
	// and completed reviews.
	ReviewSide bool
	// DeckLabel names the Review side's first view: the reviews themselves,
	// or one review's deck.
	DeckLabel string
	// ReviewCodeHref and ReviewCoverageHref are where the Review side's Code Diff and
	// coverage load from: a review's own range on a review's page, and the
	// comparison the reviewer was opened with on the index. Empty means the
	// view has nothing to show and its tab is not offered.
	ReviewCodeHref     string
	ReviewCoverageHref string
	Root               *sectionView
	SlideRoot          *sectionView
	Nav                []*navNodeView
	Diagnostic         string
	Code               *CodeReviewView
	Manifest           *CoverageManifestView
	Error              string
	Files              []*fileDiffView
	// CoverageTotals is the audit reduced to the numbers the shell states
	// outright. The audit itself stays on the Coverage tab.
	CoverageTotals *coverageTotalsView
}

// coverageTotalsView is the coverage state a reviewer needs before deciding
// whether to open the audit: how much changed, how much of it the story
// explains, and whether anything is still unaccounted for.
type coverageTotalsView struct {
	Files       int
	Total       int
	Covered     int
	Uncovered   int
	Overlapping int
	Orphaned    int
	Mappings    int
	Complete    bool
}

func makeCoverageTotals(manifest *CoverageManifestView) *coverageTotalsView {
	if manifest == nil {
		return nil
	}
	return &coverageTotalsView{
		Files: len(manifest.Files), Total: manifest.Total, Covered: manifest.Covered,
		Uncovered: manifest.Uncovered, Overlapping: manifest.Overlapping,
		Orphaned: manifest.Orphaned, Mappings: manifest.MappingCount, Complete: manifest.Complete,
	}
}

// navNodeView is the sidebar documentation tree. It exposes titles, links and a
// quiet review state only: never counts, never the storage hierarchy.
type navNodeView struct {
	Title  string
	Href   string
	NodeID string
	Icon   string
	// IconPlaceholder remains as a defensive rendering fallback for callers
	// outside the app navigation builder. App navigation assigns real icons.
	IconPlaceholder bool
	Requirement     bool
	Deck            bool
	// Group marks a named place in the stable information architecture rather
	// than an authored destination: it discloses what it holds instead of
	// linking anywhere of its own.
	Group bool
	// Gap marks an authored record whose state is incomplete (for example, a
	// persona with no accepted story). Empty sections never reach this tree.
	Gap      bool
	Note     string
	Slide    *SlideReferenceView
	Active   bool
	Expanded bool
	Children []*navNodeView
}

type sectionView struct {
	*saga.Section
	// Deferred marks a chapter summary whose body has not been rendered. The
	// body arrives from /api/section the first time the chapter is opened.
	Deferred bool
	// DeckRole is a deck's role, which names its slides: an implementation
	// deck's slides are not review slides; only a review's are.
	DeckRole      string
	DOMID         string
	ChangeCount   int
	Attached      *attachedCodeView
	FragmentViews []*fragmentView
	ChildViews    []*sectionView
}

type fragmentView struct {
	*saga.Fragment
	// Deferred marks a descriptor: the fragment is named, linked, and
	// reviewable, and its content arrives from /api/fragment.
	Deferred      bool
	DOMID         string
	URL           string
	Markdown      template.HTML
	Plain         string
	Interactive   bool
	Image         bool
	AspectRatio   string
	SectionTitle  string
	LandmarkViews []*landmarkView
	Stories       *storyLinksView
	ChangeCount   int
	Attached      *attachedCodeView
}

type landmarkView struct {
	saga.Landmark
	Stories     *storyLinksView
	DOMID       string
	Title       string
	ChangeCount int
	Attached    *attachedCodeView
	Region      *saga.LandmarkRegion
}

type diffAtomView struct {
	gitdiff.Atom
	Target   string
	Selected bool
}

type fileDiffView = FileDiffView

func Listen(ctx context.Context, root, sourceDir, addr string, openBrowser bool, out io.Writer) error {
	return ListenManaged(ctx, root, sourceDir, addr, openBrowser, out, ManagedOptions{})
}

func ListenManaged(ctx context.Context, root, sourceDir, addr string, openBrowser bool, out io.Writer, options ManagedOptions) error {
	out = &lockedWriter{value: out}
	if !loopbackListenAddress(addr) {
		return fmt.Errorf("refusing non-loopback listen address %q; remote serving is disabled", addr)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if info, err := os.Stat(abs); err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("%s is not a saga directory", root)
	}
	if sourceDir == "" {
		sourceDir = abs
	}
	if _, validation, err := saga.LoadMutationIndex(abs); err != nil {
		return err
	} else if !validation.Valid {
		return fmt.Errorf("saga is structurally invalid; run change-saga validate")
	}
	tmpl, err := newPageTemplateFor(options.Range)
	if err != nil {
		return err
	}
	generations, err := snapshotcache.Default()
	if err != nil {
		return fmt.Errorf("open review cache: %w", err)
	}
	stopCh := make(chan struct{}, 1)
	mutationToken, err := newMutationToken()
	if err != nil {
		return err
	}
	application := &app{root: abs, sourceDir: sourceDir, rng: options.Range, template: tmpl, shutdownToken: options.ShutdownToken, mutationToken: mutationToken, generations: generations}
	application.shutdown = func() {
		select {
		case stopCh <- struct{}{}:
		default:
		}
	}
	mux := newMux(application)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	server := newHTTPServer(secureHandler(mux, listener.Addr().String()))
	serverURL := "http://" + listener.Addr().String()
	if host, port, err := net.SplitHostPort(listener.Addr().String()); err == nil && host == "127.0.0.1" {
		serverURL = "http://127.0.0.1:" + port
	}
	if options.OnReady != nil {
		if err := options.OnReady(serverURL); err != nil {
			_ = listener.Close()
			return fmt.Errorf("publish managed server state: %w", err)
		}
	}
	fmt.Fprintf(out, "Change Saga is available at %s\nPress Ctrl-C to stop.\n", serverURL)
	if openBrowser {
		if err := launchBrowser(serverURL); err != nil {
			fmt.Fprintf(out, "Could not open a browser automatically: %v\n", err)
		}
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case <-stopCh:
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func newMux(application *app) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /requirements/{story}/criteria/{criterion}", application.page)
	mux.HandleFunc("GET /requirements/{story}", application.page)
	mux.HandleFunc("GET /requirements", application.page)
	mux.HandleFunc("GET /terms/{term}", application.page)
	mux.HandleFunc("GET /terms", application.page)
	mux.HandleFunc("GET /chapters/{chapter}", application.page)
	mux.HandleFunc("GET /personas/{persona}", application.page)
	mux.HandleFunc("GET /personas", application.page)
	mux.HandleFunc("GET /flags", application.page)
	mux.HandleFunc("GET /design-system", application.page)
	mux.HandleFunc("GET /features/{feature}", application.page)
	mux.HandleFunc("GET /features", application.page)
	mux.HandleFunc("GET /tests/{test}", application.page)
	mux.HandleFunc("GET /", application.page)
	mux.HandleFunc("GET /reviews", application.reviewIndex)
	mux.HandleFunc("GET /reviews/{id}", application.reviewPage)
	mux.HandleFunc("GET /reviews/{id}/code", application.reviewCodeSurface)
	mux.HandleFunc("GET /reviews/{id}/file-diff", application.reviewFileDiffSurface)
	mux.HandleFunc("GET /reviews/{id}/coverage", application.reviewCoverageSurface)
	mux.HandleFunc("GET /reviews/{id}/visual/{slide}", application.reviewVisual)
	mux.HandleFunc("POST /reviews/{id}/decision", application.reviewDecision)
	mux.HandleFunc("POST /reviews/{id}/comment", application.reviewComment)
	mux.HandleFunc("GET /app.js", application.javascript)
	mux.HandleFunc("GET /theme.js", application.themeScript)
	mux.HandleFunc("GET /api/code", application.codePage)
	mux.HandleFunc("GET /api/coverage", application.coveragePage)
	mux.HandleFunc("GET /api/totals", application.coverageTotalsPage)
	mux.HandleFunc("GET /api/reference-code", application.referenceCodePage)
	mux.HandleFunc("GET /api/layers", application.layersAPI)
	mux.HandleFunc("GET /api/change", application.changePage)
	mux.HandleFunc("GET /api/history", application.historyPage)
	mux.HandleFunc("GET /api/coverage-file", application.coverageFilePage)
	mux.HandleFunc("GET /api/coverage-target", application.coverageTargetPage)
	mux.HandleFunc("GET /api/file-diff", application.fileDiffFragment)
	mux.HandleFunc("GET /api/target-code", application.targetCode)
	mux.HandleFunc("GET /api/file-owners", application.fileOwners)
	mux.HandleFunc("GET /api/section", application.sectionBody)
	mux.HandleFunc("GET /api/fragment", application.fragmentContent)
	mux.HandleFunc("GET /api/locate", application.locateAnchor)
	mux.HandleFunc("GET /api/runtime", application.runtimeStatus)
	mux.HandleFunc("POST /api/runtime-stop", application.runtimeStop)
	mux.HandleFunc("GET /f/{id}/{path...}", application.fragmentFile)
	return mux
}

func (a *app) runtimeStatus(w http.ResponseWriter, _ *http.Request) {
	state, _ := a.snapshotState()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": state != "error", "cache": state})
}

// fileDiffFragment renders one complete changed file on demand. It is the only
// place a diff body is produced: the page ships file summaries, and the linked
// code drawer and the coverage audit both ask for a body when a reviewer opens
// one file. Inlining every body instead made the document grow with the whole
// comparison, twice over, for markup no reviewer had asked to see.
//
// `target` scopes the body to one narrative owner, so the rows that target
// explains are marked as its evidence and any comment written from the drawer
// is attributed to it. `view=manifest` returns the read-only rows the coverage
// audit shows, which carry no per-line review actions.
func (a *app) fileDiffFragment(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		http.Error(w, "missing changed file", http.StatusBadRequest)
		return
	}
	// Target-scoped drawers still need the mapping generation to mark the exact
	// rows owned by that explanation. Ordinary Code and Coverage file bodies do
	// not: read only the requested catalog entry and leave mapping independent.
	if r.URL.Query().Get("target") != "" {
		a.mappedFileDiffFragment(w, r)
		return
	}
	document := a.sourceReviewDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
		return
	}
	catalog, err := a.sourceCatalog(r.Context(), document.Manifest)
	if err != nil {
		http.Error(w, "The source comparison could not be loaded.", http.StatusInternalServerError)
		return
	}
	file, ok := catalogFile(catalog, filePath)
	if !ok {
		http.Error(w, "changed file not found", http.StatusNotFound)
		return
	}
	changes, err := gitdiff.ReadFile(r.Context(), a.sourceDir, catalog, file)
	if err != nil {
		http.Error(w, "The file diff could not be loaded.", http.StatusInternalServerError)
		return
	}
	manifestView := r.URL.Query().Get("view") == "manifest"
	files := makeFileViews(changes, saga.SagaTarget(document.Manifest.ID))
	var selected *FileDiffView
	for _, candidate := range files {
		if candidate.Path == filePath {
			selected = candidate
			break
		}
	}
	if selected == nil {
		// Binary and mode-only entries can have catalog metadata without text
		// rows. They still render a stable, reviewable file shell.
		selected = catalogFileView(catalog, file)
	}
	total := len(selected.Lines)
	window, err := pageRequest(r, "file-diff\x00"+sourceCatalogIdentity(catalog)+"\x00"+filePath+"\x00\x00"+r.URL.Query().Get("view"), total, defaultDiffPageLimit, maxDiffPageLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	selected.Lines = selected.Lines[window.start:window.end]
	name := "file-diff-page"
	if manifestView {
		name = "manifest-file-diff-page"
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	writePageHeaders(w, window)
	page := fileDiffPageView{File: selected, NextCursor: window.next, HasMore: window.hasMore(), Returned: window.end - window.start}
	renderHTML(w, a.template, name, page, "The file diff could not be rendered.")
}

func (a *app) mappedFileDiffFragment(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file")
	document := a.sourceReviewDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
		return
	}
	manifestView := r.URL.Query().Get("view") == "manifest"
	target := r.URL.Query().Get("target")
	if target != "" && !targetExists(document, target) {
		http.Error(w, "unknown narrative target", http.StatusBadRequest)
		return
	}
	selection, err := a.selectTargetCode(r.Context(), document, target, filePath)
	if err != nil {
		http.Error(w, "The linked file diff could not be loaded.", http.StatusInternalServerError)
		return
	}
	files := makeFileViews(selection.changes, target)
	var selected *FileDiffView
	for _, candidate := range files {
		if candidate.Path == filePath {
			selected = candidate
			break
		}
	}
	if selected == nil {
		file, ok := catalogFile(selection.catalog, filePath)
		if !ok {
			http.Error(w, "changed file not found", http.StatusNotFound)
			return
		}
		selected = catalogFileView(selection.catalog, file)
	}
	linked := make(map[string]bool, len(selection.matched))
	for _, atom := range selection.matched {
		linked[atom.Ref] = true
	}
	for _, line := range selected.Lines {
		line.Linked = line.Atom != nil && linked[line.Atom.Ref]
	}
	total := len(selected.Lines)
	window, err := pageRequest(r, "file-diff\x00"+sourceCatalogIdentity(selection.catalog)+"\x00"+filePath+"\x00"+target+"\x00"+r.URL.Query().Get("view"), total, defaultDiffPageLimit, maxDiffPageLimit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	selected.Lines = selected.Lines[window.start:window.end]
	name := "file-diff-page"
	if manifestView {
		name = "manifest-file-diff-page"
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	writePageHeaders(w, window)
	page := fileDiffPageView{File: selected, NextCursor: window.next, HasMore: window.hasMore(), Returned: window.end - window.start}
	renderHTML(w, a.template, name, page, "The file diff could not be rendered.")
}

// sectionBody renders one chapter's body on demand: its comments, its
// explanations as descriptors, and the sections nested inside it. It is bounded
// by that one chapter, and it renders at the same scope the page renders its
// root at, so an opened chapter reads exactly as the shell around it.
func (a *app) sectionBody(w http.ResponseWriter, r *http.Request) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
		return
	}
	section := findSection(document, r.URL.Query().Get("target"))
	if section == nil {
		http.Error(w, "unknown section", http.StatusNotFound)
		return
	}
	scope := viewScope{}.shell()
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "section-body", makeSectionView(section, scope), "The chapter could not be rendered.")
}

// fragmentContent renders one explanation's narrative content, marked places,
// annotations, and review records. Linked source summaries are deliberately a
// separate lazy surface: reading prose must never start or wait for a source
// comparison or coverage build.
func (a *app) fragmentContent(w http.ResponseWriter, r *http.Request) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
		return
	}
	fragment := findFragmentByTarget(document, r.URL.Query().Get("target"))
	if fragment == nil {
		http.Error(w, "unknown fragment", http.StatusNotFound)
		return
	}
	scope := viewScope{}
	view := makeFragmentView(fragment, scope)
	if strings.Contains(fragment.Target, ":slide:") {
		records, err := requirements.Load(a.root, document.Manifest.ID)
		if err != nil {
			http.Error(w, "Story links could not be loaded.", http.StatusInternalServerError)
			return
		}
		if err := decorateFragmentStories(document, records, view); err != nil {
			http.Error(w, "Story links could not be resolved.", http.StatusInternalServerError)
			return
		}
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "fragment", view, "The explanation could not be rendered.")
}

// locateAnchor answers where a page anchor lives. A permalink can name a
// heading, a marked place, or a comment inside a chapter nobody has opened yet,
// and the browser has to know which chapter to fetch before it can scroll to it.
// Answering here costs one small request on a deep link; shipping the same
// answer as an index would cost every reviewer the whole document on every load.
func (a *app) locateAnchor(w http.ResponseWriter, r *http.Request) {
	anchor := r.URL.Query().Get("anchor")
	if anchor == "" {
		http.Error(w, "missing anchor", http.StatusBadRequest)
		return
	}
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded.", http.StatusInternalServerError)
		return
	}
	place, ok := locateAnchorIn(document, anchor)
	if !ok {
		http.Error(w, "unknown anchor", http.StatusNotFound)
		return
	}
	response := map[string]string{}
	if place.chapter != "" {
		response["chapter"] = domID(place.chapter)
	}
	if place.fragment != "" {
		response["fragment"] = domID(place.fragment)
		// A feature's explanation renders on its feature's page. Elsewhere the
		// browser fetches it by target to show it in the drawer.
		response["target"] = place.fragment
	}
	writeIncrementalHeaders(w, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "The anchor could not be resolved.", http.StatusInternalServerError)
	}
}

// writeIncrementalHeaders answers a request for part of the page. What comes
// back carries live review state — decisions, comments, and the identity behind
// them — so it is never reused from a cache: a reviewer would otherwise open a
// chapter and read it as it was before their own last comment.
func writeIncrementalHeaders(w http.ResponseWriter, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
}

// anchorPlace names the two things a deferred anchor needs before it can be
// scrolled to: the chapter whose body must be fetched, and the fragment whose
// content must be rendered. Either can be empty — an anchor in the overview has
// no chapter, and a chapter's own anchor has no fragment.
type anchorPlace struct {
	chapter  string
	fragment string
}

// locateAnchorIn resolves an anchor exactly when the document names it, and
// otherwise by the "owner--detail" shape every derived anchor uses: a heading,
// a footnote, a marked place, and a comment bubble are all suffixes of the DOM
// id of the thing that owns them.
func locateAnchorIn(document *saga.Saga, anchor string) (anchorPlace, bool) {
	places := anchorPlaces(document)
	if place, ok := places[anchor]; ok {
		return place, true
	}
	for cut := strings.LastIndex(anchor, "--"); cut > 0; cut = strings.LastIndex(anchor[:cut], "--") {
		if place, ok := places[anchor[:cut]]; ok {
			return place, true
		}
	}
	return anchorPlace{}, false
}

func anchorPlaces(document *saga.Saga) map[string]anchorPlace {
	places, byTarget := map[string]anchorPlace{}, map[string]anchorPlace{}
	var walk func(*saga.Section, string)
	walk = func(section *saga.Section, chapter string) {
		if section.Kind == "chapter" {
			chapter = section.Target
		}
		place := anchorPlace{chapter: chapter}
		byTarget[section.Target], places[domID(section.Target)] = place, place
		for _, fragment := range section.Fragments {
			within := anchorPlace{chapter: chapter, fragment: fragment.Target}
			byTarget[fragment.Target], places[domID(fragment.Target)] = within, within
			for index := range fragment.Landmarks {
				byTarget[fragment.Landmarks[index].Target] = within
			}
		}
		for _, child := range section.Children {
			walk(child, chapter)
		}
	}
	walk(document.Section, "")
	return places
}

func findSection(document *saga.Saga, target string) *saga.Section {
	if target == "" {
		return nil
	}
	var found *saga.Section
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Target == target {
			found = section
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return found
}

func findFragmentByTarget(document *saga.Saga, target string) *saga.Fragment {
	if target == "" {
		return nil
	}
	var found *saga.Fragment
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		for _, fragment := range section.Fragments {
			if fragment.Target == target {
				found = fragment
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return found
}

// markLinkedEvidence flags the rows of a whole-file diff that a single
// narrative target actually explains, so the drawer keeps showing the reviewer
// which lines its explanation is answerable for once the surrounding file
// arrives. The page used to carry those rows twice — once as the target's
// evidence and once inside the file — purely so the browser could compare them.
func markLinkedEvidence(file *FileDiffView, linked []gitdiff.Atom) {
	if len(linked) == 0 {
		return
	}
	keys := make(map[string]bool, len(linked))
	for _, atom := range linked {
		keys[atom.Key] = true
	}
	for _, line := range file.Lines {
		if line.Atom != nil && keys[line.Atom.Key] {
			line.Linked = true
		}
	}
}

func (a *app) runtimeStop(w http.ResponseWriter, r *http.Request) {
	provided := r.Header.Get("X-Change-Saga-Shutdown")
	if a.shutdownToken == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(a.shutdownToken)) != 1 {
		http.Error(w, "Missing or invalid shutdown token.", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`+"\n")
	if a.shutdown != nil {
		go a.shutdown()
	}
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           securityHeaders(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func secureHandler(next http.Handler, listenerAddress string) http.Handler {
	crossOrigin := http.NewCrossOriginProtection()
	crossOrigin.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Cross-origin request rejected.", http.StatusForbidden)
	}))
	return validateHost(listenerAddress, crossOrigin.Handler(next))
}

func validateHost(listenerAddress string, next http.Handler) http.Handler {
	allowed := allowedListenerHosts(listenerAddress)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowed[strings.ToLower(r.Host)] {
			http.Error(w, "Invalid request host.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedListenerHosts(listenerAddress string) map[string]bool {
	host, port, err := net.SplitHostPort(listenerAddress)
	if err != nil {
		return map[string]bool{}
	}
	allowed := map[string]bool{strings.ToLower(net.JoinHostPort(host, port)): true}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		allowed[net.JoinHostPort("localhost", port)] = true
	}
	return allowed
}

func loopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// newPageTemplate is the single definition of the renderer's template funcs so
// tests exercise exactly the helpers the served page uses. It renders a
// compared Saga; newPageTemplateFor renders the way a reviewer was opened.
func newPageTemplate() (*template.Template, error) {
	return newPageTemplateFor(gitdiff.Range{Against: "HEAD"})
}

// newPageTemplateFor renders a reviewer opened with rng: comparing adds the
// Change tab. The documentation it renders carries no approval or comment
// control in either mode; those live on the pull request's review pages.
func newPageTemplateFor(rng gitdiff.Range) (*template.Template, error) {
	funcs := templateFuncs()
	comparing := !rng.Observe()
	funcs["comparing"] = func() bool { return comparing }
	return template.New("page").Funcs(funcs).Parse(pageTemplate + directoryTemplates)
}

// templateFuncs is shared by the server and its rendering tests so a new
// presentation helper cannot be wired into one and forgotten in the other.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"comparing": func() bool { return true },
		"short":     shortCommit,
		"join":      strings.Join,
		"markdown":  markdown,
		"domID":     domID,
		"fileIcon":  fileIcon,
		"lower":     strings.ToLower,
		"reviewDiffSurface": func(path, codeHref string) reviewDiffSurfaceView {
			return reviewDiffSurfaceView{Path: path, CodeHref: codeHref}
		},
		"relatedReviews": func(kind string, reviews []relatedReviewView) relatedReviewsView {
			return relatedReviewsView{Kind: kind, Reviews: reviews}
		},
	}
}

func (a *app) page(w http.ResponseWriter, r *http.Request) {
	if chapterID, chapterRoute := requestedChapter(r); chapterRoute {
		a.chapterRedirect(w, r, chapterID)
		return
	}
	data, err := a.shell(r)
	switch {
	case errors.Is(err, errRequirementNotFound), errors.Is(err, errTermNotFound), errors.Is(err, errAppPageNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderHTML(w, a.template, "page", data, "The review page could not be rendered.")
}

// chapterRedirect keeps /chapters/{id} links working: an app chapter opens on
// the overview, and a feature's chapter on that feature's page.
func (a *app) chapterRedirect(w http.ResponseWriter, r *http.Request, chapterID string) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return
	}
	for _, child := range document.Section.Children {
		if child.Kind != "chapter" || child.ID != chapterID {
			continue
		}
		destination := "/#" + domID(child.Target)
		for _, feature := range document.Features {
			if feature.Design != nil && containsSection(feature.Design, child) || feature.Report != nil && containsSection(feature.Report, child) {
				destination = featureHref(feature.ID) + "#" + domID(child.Target)
			}
		}
		http.Redirect(w, r, destination, http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

func containsSection(root, wanted *saga.Section) bool {
	if root.Target == wanted.Target {
		return true
	}
	for _, child := range root.Children {
		if containsSection(child, wanted) {
			return true
		}
	}
	return false
}

// appRoute says which page of the app shell a path asks for.
type appRoute struct {
	kind string
	id   string
}

func routeOf(r *http.Request) (appRoute, bool) {
	path := r.URL.Path
	switch {
	case path == "/":
		return appRoute{kind: "overview"}, true
	case isRequirementsPath(path):
		return appRoute{kind: "requirements"}, true
	case isTermsPath(path):
		return appRoute{kind: "terms"}, true
	case path == "/personas":
		return appRoute{kind: "personas"}, true
	case strings.HasPrefix(path, "/personas/") && r.PathValue("persona") != "":
		return appRoute{kind: "persona", id: r.PathValue("persona")}, true
	case path == "/flags":
		return appRoute{kind: "flags"}, true
	case path == designSystemPath:
		return appRoute{kind: "designsystem"}, true
	case path == "/features":
		return appRoute{kind: "features"}, true
	case strings.HasPrefix(path, "/features/") && r.PathValue("feature") != "":
		return appRoute{kind: "feature", id: r.PathValue("feature")}, true
	case strings.HasPrefix(path, "/tests/") && r.PathValue("test") != "":
		return appRoute{kind: "test", id: r.PathValue("test")}, true
	case path == "/reviews" || strings.HasPrefix(path, "/reviews/"):
		return appRoute{kind: "reviews"}, true
	}
	return appRoute{}, false
}

// shell builds the app shell for one request: the topbar, the sidebar, and
// the page the path names. The overview is itself a shell: identity,
// coverage totals, the overview's fragments as descriptors, one summary per
// app chapter, and the navigation outline. Everything below that arrives from
// /api/section and /api/fragment as a reviewer opens it.
func (a *app) shell(r *http.Request) (*pageData, error) {
	route, ok := routeOf(r)
	if !ok {
		return nil, errAppPageNotFound
	}
	document := a.outlineDocument(r.Context())
	if document == nil {
		return nil, errors.New("The saga could not be loaded. Run change-saga validate for details.")
	}
	if len(document.Decks)+len(document.Onboarding) > 0 {
		document = a.narrativeDocument(r.Context())
		if document == nil {
			return nil, errors.New("The slide deck could not be loaded. Run change-saga validate for details.")
		}
	}
	scope := viewScope{}
	reportRoot, slideRoot := splitReportAndDeckSections(document.Section)
	requirementsView, _, requirementsDocument, err := loadRequirementsSurface(a.root, document.Manifest.ID, r)
	if err != nil {
		if errors.Is(err, errRequirementNotFound) {
			return nil, err
		}
		return nil, errors.New("The requirements could not be loaded. Run change-saga validate for details.")
	}
	tests, err := quality.Load(a.root)
	if err != nil {
		tests = quality.Document{SagaID: document.Manifest.ID}
	}
	graph := newAppGraph(document, requirementsDocument, tests)
	// A feature's chapters and explanations belong to its page, and the design
	// system's to its own; the overview holds only the app's own.
	appReport := *reportRoot
	appReport.Children, appReport.Fragments = nil, nil
	designRoot := *reportRoot
	designRoot.Children, designRoot.Fragments = nil, nil
	for _, fragment := range reportRoot.Fragments {
		if _, feature := graph.targetFeature[fragment.Target]; feature {
			continue
		}
		if saga.IsDesignSystemPath(fragment.Path) {
			designRoot.Fragments = append(designRoot.Fragments, fragment)
			continue
		}
		appReport.Fragments = append(appReport.Fragments, fragment)
	}
	for _, child := range reportRoot.Children {
		if _, feature := graph.featureChapter(child); feature {
			continue
		}
		if saga.IsDesignSystemPath(child.Path) {
			designRoot.Children = append(designRoot.Children, child)
			continue
		}
		appReport.Children = append(appReport.Children, child)
	}
	data := &pageData{
		Opening:       openingLabel(a.rng),
		Comparing:     !a.rng.Observe(),
		Saga:          document,
		EmbeddedDecks: len(document.Decks)+len(document.Onboarding) > 0,
		Root:          makeSectionView(&appReport, scope.shell()),
		Requirements:  requirementsView,
	}
	// Change totals describe a comparison; observing has none, so its line
	// arrives from /api/totals as the documented code instead.
	if data.Comparing {
		data.CoverageTotals = a.cachedCoverageTotals()
	}
	// The header carries the distinction between the two sides. Code Diff and
	// coverage of a change are comparison views, so they belong to Review;
	// Documentation keeps the documented code, which is the same references
	// resolved at the head rather than a diff.
	data.ReviewSide = route.kind == "reviews"
	data.DeckLabel = "Reviews"
	if data.ReviewSide && data.Comparing {
		data.ReviewCodeHref, data.ReviewCoverageHref = "/api/code", "/api/coverage"
	}
	data.RequirementsMode = requirementsView.Active
	if data.RequirementsMode {
		graph.decorateRequirements(requirementsView)
	}
	storyTitles := map[string]string{}
	for _, story := range requirementsView.Stories {
		storyTitles[story.Target] = story.Title
	}
	data.Terms, err = a.makeTermsPage(r.Context(), requirementsDocument, storyTitles, r.URL.Path, r.PathValue("term"), directoryQuery(r))
	if err != nil {
		return nil, err
	}
	data.TermsMode = data.Terms.Active
	switch route.kind {
	case "persona":
		if data.Persona, err = graph.personaPage(route.id); err != nil {
			return nil, err
		}
	case "personas":
		data.Personas = personasDirectory(requirementsDocument, directoryQuery(r))
	case "flags":
		data.Flags = flagsDirectory(graph, directoryQuery(r))
	case "designsystem":
		data.DesignSystemMode = true
		if len(designRoot.Fragments)+len(designRoot.Children) > 0 {
			data.DesignSystem = makeSectionView(&designRoot, scope.shell())
		}
	case "feature":
		if data.Feature, err = graph.featurePage(route.id); err != nil {
			return nil, err
		}
	case "test":
		if data.TestCase, err = a.testCasePage(r.Context(), graph, route.id); err != nil {
			return nil, err
		}
	}
	// Related reviews are a footnote on a feature, a story, and a criterion,
	// derived from the code those records reference rather than authored. Only
	// those pages ask for the index, and it is held until the Saga or the
	// source head changes, so the documentation pages do not pay the cross
	// product again on every request.
	if route.kind == "feature" || route.kind == "requirements" {
		attachRelatedReviews(a.relatedReviews(r.Context(), document, requirementsDocument), data)
	}
	overviewActive := ""
	switch {
	case data.TermsMode && data.Terms.Term != nil:
		overviewActive = data.Terms.Term.ID
	case data.TermsMode:
		overviewActive = "/terms"
	case route.kind != "overview":
		overviewActive = "-"
	}
	// The sidebar lists every feature and opens the one this page belongs to.
	// A page that belongs to none, the features table among them, opens none:
	// which feature a reader is in is a fact about the page, never a preference
	// kept about the reader.
	data.PageFeature = pageFeature(route, requirementsView, tests)
	if route.kind == "features" {
		data.Features = featuresDirectory(document, graph, data.PageFeature, directoryQuery(r))
	}
	onboarding := onboardingHref(document)
	if route.kind == "overview" {
		data.OverviewParts = overviewDirectory(document, requirementsDocument, onboarding)
	}
	prototypeDocument, prototypeNote := a.prototypeDocument(document.Manifest.ID)
	data.Nav = makeAppNavTree(appNavSources{
		document: document, requirements: requirementsDocument, page: requirementsView,
		quality:    tests,
		prototypes: prototypeDocument, prototypeNote: prototypeNote,
		decks: makeDeckNavTree(slideRoot), overviewActive: overviewActive,
		pageFeature: data.PageFeature, reviewSide: data.ReviewSide,
	})
	if route.kind != "overview" {
		// Off the overview, an in-page anchor would point into a page that is
		// not there. Every such link opens the overview instead.
		rootNavLinks(data.Nav)
		if !data.TermsMode {
			markActiveNav(data.Nav, r.URL.Path)
		}
	}
	if data.EmbeddedDecks {
		data.SlideRoot = makeSectionView(slideRoot, scope)
		storyLinks := &storyLinkDecorator{document: document, records: requirementsDocument}
		for _, deck := range data.SlideRoot.ChildViews {
			for _, slide := range deck.FragmentViews {
				if err := storyLinks.decorate(slide); err != nil {
					return nil, fmt.Errorf("story links could not be resolved: %w", err)
				}
			}
		}
		labelDeckRoles(data.SlideRoot, document)
	}
	return data, nil
}

// rootNavLinks points the sidebar's in-page anchors at the overview.
func rootNavLinks(nodes []*navNodeView) {
	for _, node := range nodes {
		if strings.HasPrefix(node.Href, "#") || strings.HasPrefix(node.Href, "?") {
			node.Href = "/" + node.Href
		}
		if node.Slide != nil && strings.HasPrefix(node.Slide.Href, "?") {
			node.Slide.Href = "/" + node.Slide.Href
		}
		rootNavLinks(node.Children)
	}
}

// markActiveNav makes the rows that open path the sidebar's current rows,
// and opens the places around them.
func markActiveNav(nodes []*navNodeView, path string) {
	clearActiveNav(nodes)
	var mark func([]*navNodeView)
	mark = func(nodes []*navNodeView) {
		for _, node := range nodes {
			node.Active = node.Href == path
			mark(node.Children)
		}
	}
	mark(nodes)
	for _, node := range nodes {
		revealActive(node)
	}
}

// labelDeckRoles records each projected deck's role on its view, so the slide
// viewer can say what kind of slide it shows.
func labelDeckRoles(root *sectionView, document *saga.Saga) {
	roles := map[string]string{}
	for _, deck := range append(append([]*saga.Deck{}, document.Decks...), document.Onboarding...) {
		roles[deck.Target] = deck.Role
	}
	for _, deck := range root.ChildViews {
		deck.DeckRole = roles[deck.Target]
	}
}

func splitReportAndDeckSections(root *saga.Section) (*saga.Section, *saga.Section) {
	if root == nil {
		return root, root
	}
	report := *root
	slides := *root
	report.Children = nil
	slides.Fragments = nil
	slides.Children = nil
	for _, child := range root.Children {
		if child.Kind == "deck" {
			slides.Children = append(slides.Children, child)
		} else {
			report.Children = append(report.Children, child)
		}
	}
	return &report, &slides
}

func (a *app) narrativeDocument(ctx context.Context) *saga.Saga {
	document, validation, err := saga.LoadNarrative(a.root)
	if err != nil || !validation.Valid {
		return nil
	}
	return document
}

// sourceReviewDocument is the narrative generation code and file responses
// read; it never opens authored coverage mappings.
func (a *app) sourceReviewDocument(ctx context.Context) *saga.Saga {
	document, validation, err := saga.LoadNarrative(a.root)
	if err != nil || !validation.Valid {
		return nil
	}
	return document
}

func requestedChapter(r *http.Request) (string, bool) {
	if value := r.PathValue("chapter"); value != "" {
		return value, true
	}
	const prefix = "/chapters/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(r.URL.Path, prefix)
	return value, value != "" && !strings.Contains(value, "/")
}

// makeNavTree builds a documentation outline for the one-page saga. It reads the
// document rather than the rendered views: the page ships chapter summaries, and
// the outline still has to name every destination beneath them so a reviewer can
// navigate into a chapter that has not been fetched yet. Titles and targets come
// from the saga's own manifests, so building the whole outline reads no content.
func makeNavTree(root *saga.Section) []*navNodeView {
	overview := &navNodeView{Title: "Overview", Href: sagaHref(root.Target), NodeID: "nav-overview", Icon: "book", Active: true}
	overview.Children = withoutRedundantLead(fragmentOutline(root), overview.Title)
	overview.Expanded = len(overview.Children) > 0
	nodes := []*navNodeView{overview}
	for _, child := range root.Children {
		// Design chapters are navigation too, but they belong under Design >
		// Technical rather than beside the narrative chapters.
		if child.Kind != "chapter" || designSection(child) {
			continue
		}
		nodes = append(nodes, makeChapterNav(child))
	}
	return nodes
}

// makeChapterNav is one chapter as a sidebar destination with its collapsed
// outline beneath it, wherever the architecture places that chapter.
func makeChapterNav(chapter *saga.Section) *navNodeView {
	node := &navNodeView{Title: chapter.Title, Href: sagaHref(chapter.Target), NodeID: "nav-" + domID(chapter.Target), Icon: "book"}
	node.Children = withoutRedundantLead(documentOutline(chapter), node.Title)
	return node
}

// makeDeckNavTree projects embedded review decks into the same sidebar as the
// living documentation. Decks are disclosure nodes, while their slides are
// destinations that switch the main pane from report reading to visual review.
// splitDeckNavByRole then places each deck in the architecture; the decks no
// longer occupy a sidebar path of their own.
func makeDeckNavTree(root *saga.Section) []*navNodeView {
	if root == nil {
		return nil
	}
	var nodes []*navNodeView
	for _, deck := range root.Children {
		if deck.Kind != "deck" {
			continue
		}
		node := &navNodeView{
			Title: deck.Title, NodeID: "nav-" + domID(deck.Target), Icon: "deck", Deck: true,
		}
		previousSection := ""
		for _, slide := range deck.Fragments {
			if slide.Title == "" {
				continue
			}
			section := ""
			if slide.SlideMeta != nil {
				section = strings.TrimSpace(slide.SlideMeta.Section)
			}
			sectionStart := ""
			if section != "" && section != previousSection {
				sectionStart = section
			}
			previousSection = section
			node.Children = append(node.Children, &navNodeView{
				Title:  slide.Title,
				Href:   "?view=slides#" + domID(slide.Target),
				NodeID: "nav-" + domID(slide.Target),
				Slide: &SlideReferenceView{
					ID: slide.ID, Title: slide.Title, Section: sectionStart, Target: slide.Target,
					Anchor: domID(slide.Target), Href: "?view=slides#" + domID(slide.Target),
					URL: fragmentAssetURL(slide), MediaType: slide.MediaType,
				},
			})
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// documentOutline turns the open page into headings a reader recognises. Titled
// content becomes an entry; untitled content is skipped rather than exposed
// under an internal identifier.
func documentOutline(section *saga.Section) []*navNodeView {
	nodes := fragmentOutline(section)
	for _, child := range section.Children {
		if child.Title == "" {
			continue
		}
		id := domID(child.Target)
		node := &navNodeView{Title: child.Title, Href: "#" + id, NodeID: "nav-" + id, Icon: "list"}
		node.Children = documentOutline(child)
		node.Expanded = len(node.Children) > 0
		nodes = append(nodes, node)
	}
	return nodes
}

// fragmentOutline lists one section's own explanations, which is the whole of
// the overview's outline: the overview is the root, and its chapters are
// separate top-level entries rather than children of it.
func fragmentOutline(section *saga.Section) []*navNodeView {
	var nodes []*navNodeView
	for _, fragment := range section.Fragments {
		// A lead-in that repeats the page title is not a separate destination.
		if fragment.Title == "" || strings.EqualFold(fragment.Title, section.Title) {
			continue
		}
		id := domID(fragment.Target)
		nodes = append(nodes, &navNodeView{Title: fragment.Title, Href: "#" + id, NodeID: "nav-" + id, Icon: "list"})
	}
	return nodes
}

// withoutRedundantLead drops a leading entry that only repeats the label of the
// page it sits under. A tree that reads "Overview > Overview" tells the reader
// nothing, and the parent row already links to that content.
func withoutRedundantLead(nodes []*navNodeView, label string) []*navNodeView {
	if len(nodes) == 0 || len(nodes[0].Children) > 0 || !strings.EqualFold(nodes[0].Title, label) {
		return nodes
	}
	return nodes[1:]
}

// viewScope carries everything a narrative view needs from the snapshot, and how
// much of the tree this render is allowed to materialise. The page renders a
// shell — the overview, its fragments as descriptors, and one summary per
// chapter — because rendering the whole document eagerly made first load grow
// with the size of the story rather than with what a reviewer can see. The
// bounded /api/section and /api/fragment endpoints render one node each, and
// /api/section reuses the page's own scope so a chapter body is built by
// exactly the code that built the page around it.
type viewScope struct {
	changes  map[string][]gitdiff.Atom
	snapshot *reviewSnapshot
	// summary stops the render at this section's own head: its body arrives
	// from /api/section when a reviewer opens it.
	summary bool
	// summarizeChapters turns this section's direct chapter children into
	// summaries. It applies to one level only, so a chapter body still renders
	// the sections nested inside it as the page always did.
	summarizeChapters bool
	// deferContent renders every fragment as a descriptor whose content arrives
	// from /api/fragment.
	deferContent bool
	// directoryManaged removes duplicate inline decision controls for targets
	// whose chapter directory owns those controls.
	directoryManaged bool
}

// shell is the scope both the page and /api/section render at: this node in
// full, its fragments as descriptors, and any chapter beneath it as a summary.
func (scope viewScope) shell() viewScope {
	scope.summary, scope.summarizeChapters, scope.deferContent = false, true, true
	return scope
}

func makeSectionView(section *saga.Section, scope viewScope) *sectionView {
	changeCount, attached := scopedAttachedCode(scope, section.Title, section.Target, section.Code)
	changeCount = lazyChangeCount(section.HasCode, changeCount)
	view := &sectionView{
		Section: section, DOMID: domID(section.Target), ChangeCount: changeCount,
		Attached: attached,
	}
	if scope.summary {
		view.Deferred = true
		return view
	}
	previousSlideSection := ""
	for _, fragment := range section.Fragments {
		fragmentView := makeFragmentView(fragment, scope)
		if fragment.SlideMeta != nil {
			currentSection := strings.TrimSpace(fragment.SlideMeta.Section)
			if currentSection != "" && currentSection != previousSlideSection {
				fragmentView.SectionTitle = currentSection
			}
			previousSlideSection = currentSection
		}
		view.FragmentViews = append(view.FragmentViews, fragmentView)
	}
	for _, child := range section.Children {
		childScope := scope
		childScope.summary, childScope.summarizeChapters = scope.summarizeChapters && child.Kind == "chapter", false
		view.ChildViews = append(view.ChildViews, makeSectionView(child, childScope))
	}
	return view
}

func makeFragmentView(fragment *saga.Fragment, scope viewScope) *fragmentView {
	title := fragment.Title
	if title == "" {
		title = fragment.ID
	}
	view := &fragmentView{Fragment: fragment, DOMID: domID(fragment.Target)}
	view.URL = fragmentAssetURL(fragment)
	if scope.deferContent {
		// A descriptor names the explanation and carries its review controls.
		// The content, its landmarks, and its linked code arrive from
		// /api/fragment once the reviewer can actually see this fragment.
		view.Deferred = true
		return view
	}
	view.ChangeCount, view.Attached = scopedAttachedCode(scope, title, fragment.Target, fragment.Code)
	view.ChangeCount = lazyChangeCount(fragment.HasCode, view.ChangeCount)
	for _, landmark := range fragment.Landmarks {
		region := landmark.Hotspot
		if region == nil && landmark.Selector.Type == "region" {
			region = &saga.LandmarkRegion{X: landmark.Selector.X, Y: landmark.Selector.Y, Width: landmark.Selector.Width, Height: landmark.Selector.Height}
		}
		changeCount, attached := scopedAttachedCode(scope, landmark.Label, landmark.Target, landmark.Code)
		changeCount = lazyChangeCount(landmark.HasCode, changeCount)
		landmarkView := &landmarkView{
			Landmark: landmark, DOMID: view.DOMID + "--" + landmark.ID, Title: landmark.Label,
			ChangeCount: changeCount,
			Attached:    attached,
			Region:      region,
		}
		view.LandmarkViews = append(view.LandmarkViews, landmarkView)
	}
	switch fragment.MediaType {
	case "text/markdown":
		if data, err := os.ReadFile(filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))); err == nil {
			view.Markdown = markdownWithAnchors(string(data), view.DOMID)
		}
	case "text/plain":
		if data, err := os.ReadFile(filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))); err == nil {
			view.Plain = string(data)
		}
	case "text/html", "image/svg+xml":
		view.Interactive = true
		if fragment.MediaType == "image/svg+xml" {
			if data, err := os.ReadFile(filepath.Join(fragment.Directory, filepath.FromSlash(fragment.Entrypoint))); err == nil {
				view.AspectRatio = svgAspectRatio(string(data))
				if view.AspectRatio != "" {
					view.URL += "?saga_aspect=" + url.QueryEscape(view.AspectRatio)
				}
			}
		}
	default:
		view.Image = strings.HasPrefix(fragment.MediaType, "image/")
	}
	return view
}

func fragmentAssetURL(fragment *saga.Fragment) string {
	return "/f/" + url.PathEscape(fragment.ID) + "/" + strings.Join(pathEscapeParts(fragment.Entrypoint), "/")
}

// A negative count is an internal render state: authored evidence exists, but
// its exact current-source match count belongs to the lazy target-code request.
func lazyChangeCount(hasDiffs bool, count int) int {
	if count == 0 && hasDiffs {
		return -1
	}
	return count
}

func scopedAttachedCode(scope viewScope, title, target string, evidence []saga.CodeFile) (int, *attachedCodeView) {
	if scope.snapshot != nil {
		indexes := scope.snapshot.targetAtoms[target]
		attached := makeAttachedCodeViewIndexed(title, target, scope.snapshot, indexes, evidence)
		if attached == nil {
			return 0, nil
		}
		return attached.ChangeCount, attached
	}
	atoms := scope.changes[target]
	attached := makeAttachedCodeView(title, target, atoms, atoms, evidence)
	if attached == nil {
		return 0, nil
	}
	return attached.ChangeCount, attached
}

var svgViewBoxPattern = regexp.MustCompile(`(?i)\bviewBox\s*=\s*["']([^"']+)["']`)

func svgAspectRatio(source string) string {
	match := svgViewBoxPattern.FindStringSubmatch(source)
	if len(match) != 2 {
		return ""
	}
	parts := strings.Fields(match[1])
	if len(parts) != 4 {
		return ""
	}
	width, widthErr := strconv.ParseFloat(parts[2], 64)
	height, heightErr := strconv.ParseFloat(parts[3], 64)
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return ""
	}
	return strconv.FormatFloat(width/height, 'f', 8, 64)
}

func makeFileViews(changes gitdiff.ChangeSet, target string) []*fileDiffView {
	byPath := map[string]*fileDiffView{}
	renameTo := map[string]string{}
	for _, atom := range changes.Atoms {
		if atom.Kind == "event" && atom.Event == "rename" && atom.OldPath != "" && atom.NewPath != "" {
			renameTo[atom.OldPath] = atom.NewPath
		}
	}
	deleted := map[string]bool{}
	for _, atom := range changes.Atoms {
		if atom.Kind == "event" && atom.Event == "delete" {
			deleted[atom.Path] = true
		}
	}
	for _, atom := range changes.Atoms {
		path := atom.Path
		if path == "" {
			path = atom.NewPath
		}
		if renamed, ok := renameTo[path]; ok {
			path = renamed
		}
		file := byPath[path]
		if file == nil {
			digest := sha256.Sum256([]byte(path))
			file = &fileDiffView{ID: fmt.Sprintf("diff-%x", digest[:8]), Path: path, Ref: fileLocation(changes.BaseOID, changes.HeadOID, path, deleted[path])}
			byPath[path] = file
		}
		file.Atoms = append(file.Atoms, &diffAtomView{Atom: atom, Target: target})
		if atom.Side == "new" {
			file.Added++
		} else if atom.Side == "old" {
			file.Deleted++
		}
	}
	// Keep a stable key lookup so renderer-only context lines can point back to
	// the exact changed atom used by comments, suggestions, and coverage.
	atomsByKey := map[string]*diffAtomView{}
	for _, file := range byPath {
		for _, atom := range file.Atoms {
			atomsByKey[atom.Key] = atom
		}
	}
	for _, line := range changes.DisplayLines {
		linePath := line.Path
		if renamed, ok := renameTo[linePath]; ok {
			linePath = renamed
		}
		file := byPath[linePath]
		if file == nil {
			continue
		}
		file.Lines = append(file.Lines, &DiffLineView{
			Kind: line.Kind, Path: linePath, OldLine: line.OldLine, NewLine: line.NewLine,
			Content: line.Content, Event: line.Event, OldPath: line.OldPath, NewPath: line.NewPath,
			Atom: atomsByKey[line.AtomKey],
		})
	}
	// Manually constructed ChangeSets (and older callers) have no display
	// context. Fall back to the changed atoms without weakening their actions.
	for _, file := range byPath {
		if len(file.Lines) != 0 {
			continue
		}
		for _, atom := range file.Atoms {
			line := &DiffLineView{Kind: atom.Side, Path: file.Path, Content: atom.Content, Event: atom.Event, OldPath: atom.OldPath, NewPath: atom.NewPath, Atom: atom}
			if atom.Kind == "event" {
				line.Kind = "event"
			} else if atom.Side == "old" {
				line.OldLine = atom.Line
			} else {
				line.NewLine = atom.Line
			}
			file.Lines = append(file.Lines, line)
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]*fileDiffView, 0, len(paths))
	for _, path := range paths {
		result = append(result, byPath[path])
	}
	return result
}

func (a *app) fragmentFile(w http.ResponseWriter, r *http.Request) {
	index, validation, err := saga.LoadMutationIndex(a.root)
	if err != nil || !validation.Valid {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return
	}
	assetTarget := saga.FragmentTarget(index.Manifest.ID, r.PathValue("id"))
	if slideTarget := saga.SlideTarget(index.Manifest.ID, r.PathValue("id")); index.FlatTargets[slideTarget] {
		assetTarget = slideTarget
	}
	fragmentDir, ok := index.Targets[assetTarget]
	if !ok {
		http.NotFound(w, r)
		return
	}
	rel := filepath.Clean(filepath.FromSlash(r.PathValue("path")))
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || hasReservedPart(rel) {
		http.Error(w, "invalid fragment path", http.StatusBadRequest)
		return
	}
	path := filepath.Join(fragmentDir, rel)
	realRoot, rootErr := filepath.EvalSymlinks(fragmentDir)
	realPath, pathErr := filepath.EvalSymlinks(path)
	if rootErr != nil || pathErr != nil {
		http.NotFound(w, r)
		return
	}
	realRel, err := filepath.Rel(realRoot, realPath)
	if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) {
		http.Error(w, "fragment file escapes its package", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
	file, err := os.Open(realPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(realPath))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, filepath.Base(realPath), info.ModTime(), file)
}

func (a *app) javascript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, appJavaScript)
}

// themeScript is served as its own file rather than inlined in the head
// because the page's Content-Security-Policy allows script-src 'self' only.
// It stays out of app.js, and stays render-blocking, so the theme is settled
// before the first paint instead of one deferred script later.
func (a *app) themeScript(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, themeBoot)
}

func findFragment(document *saga.Saga, id string) *saga.Fragment {
	var found *saga.Fragment
	matches := 0
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		for _, fragment := range section.Fragments {
			if fragment.ID == id {
				found = fragment
				matches++
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	if matches != 1 {
		return nil
	}
	return found
}

func targetExists(document *saga.Saga, target string) bool {
	if target == saga.SagaTarget(document.Manifest.ID) {
		return true
	}
	found := false
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Target == target {
			found = true
		}
		for _, fragment := range section.Fragments {
			if fragment.Target == target {
				found = true
			}
			for index := range fragment.Landmarks {
				if fragment.Landmarks[index].Target == target {
					found = true
				}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return found
}

func findTargetDirectory(document *saga.Saga, target string) string {
	if target == saga.SagaTarget(document.Manifest.ID) {
		return document.Root
	}
	result := ""
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Target == target {
			result = filepath.Join(document.Root, filepath.FromSlash(section.Path))
		}
		for _, fragment := range section.Fragments {
			if fragment.Target == target {
				result = fragment.Directory
			}
			for index := range fragment.Landmarks {
				landmark := &fragment.Landmarks[index]
				if landmark.Target == target {
					result = landmark.Directory
				}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return result
}

func pathEscapeParts(path string) []string {
	parts := strings.Split(path, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return parts
}

func hasReservedPart(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if strings.HasPrefix(part, "___") || part == "fragment.json" {
			return true
		}
	}
	return false
}

func domID(value string) string {
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("target-%s-%x", store.Slug(value), digest[:6])
}

func sagaHref(target string) string { return "#" + domID(target) }

const defaultAnnotationColor = "#d04832"

// Sticky notes default to the warm amber already used for landmark highlights so
// a placed note reads as paper rather than as a drawing stroke.
const defaultNoteColor = "#f2bd4b"

func markdown(source string) template.HTML {
	return markdownWithAnchors(source, "heading")
}

func launchBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	return exec.Command(command, args...).Start()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !strings.HasPrefix(r.URL.Path, "/f/") {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; form-action 'self'; frame-ancestors 'none'; object-src 'none'")
		}
		next.ServeHTTP(w, r)
	})
}

// renderHTML executes one template into memory and only then writes it. A
// template that failed halfway, or a browser that went away mid-response,
// used to leave a 200 already sent when the handler reported the failure,
// which net/http logs as a superfluous WriteHeader. Rendering first means a
// failure is a clean 500 and a disconnect is nothing at all.
func renderHTML(w http.ResponseWriter, tmpl *template.Template, name string, data any, failure string) {
	var body bytes.Buffer
	if err := tmpl.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, failure, http.StatusInternalServerError)
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	_, _ = body.WriteTo(w)
}
