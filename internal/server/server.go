package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/semanticgraph"
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
	// observed is the observed Coverage graph, kept while the Saga's files
	// are unchanged.
	observed observeGraphCache
	// files is what documentation pages read from the Saga's own files,
	// kept while those files are unchanged.
	files sagaFilesCache
	// reviewCoverages is each review's coverage; see reviewcache.go.
	reviewCoverages reviewCoverageCache
	// termPlacesCache is where the terms' code is at the head.
	termPlacesCache termPlacesCache
	// fresh shares the check of whether the Saga's files or the heads
	// have changed between the requests that ask at once.
	fresh freshness
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
	// Technical is the Technical design page, and TechnicalEntity one
	// definition's canonical page.
	Technical       *technicalPageView
	TechnicalEntity *technicalEntityView
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
	// ReviewDeck makes one pull-request review use the same full-pane visual
	// treatment as an implementation deck. The deck is the page; review actions
	// are overlays and linked evidence opens from its exact Items.
	ReviewDeck bool
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
	Nav                []*navNodeView
	Diagnostic         string
	Code               *CodeReviewView
	Manifest           *CoverageManifestView
	Error              string
	Files              []*fileDiffView
	// CoverageTotals is the audit reduced to the numbers the shell states
	// outright. The audit itself stays on the Coverage tab.
	CoverageTotals *coverageTotalsView
	// ShellVersion is the state of the Saga the page was read from. The
	// sidebar and the deck viewer are loaded once and kept across pages; the
	// version they were loaded at says whether a later page still agrees
	// with them. Empty when the Saga could not be fingerprinted, which no
	// kept part ever matches.
	ShellVersion string
	// PageTitle names the page in its <title> and to a reader told that it
	// has arrived. Empty on the overview, which the Saga's title names.
	PageTitle string
	// StaleShell says the browser asking for this page as a partial holds a
	// sidebar and deck viewer from another state of the Saga, so the partial
	// replaces them too.
	StaleShell bool
}

// oobView is one part of the page as the layout composes it, or as a
// partial swaps it in out of band. Each part is defined once and says only
// whether it is swapped out of band; nothing else differs between the two.
type oobView struct {
	*pageData
	OOB bool
}

// oobPart is one part of data, swapped out of band or not. Rendering tests
// pass the page by value; the server passes a pointer.
func oobPart(data any, swap bool) (oobView, error) {
	switch page := data.(type) {
	case *pageData:
		return oobView{pageData: page, OOB: swap}, nil
	case pageData:
		return oobView{pageData: &page, OOB: swap}, nil
	}
	return oobView{}, fmt.Errorf("oob: %T is not a page", data)
}

// Page is the page the part belongs to, for parts that compose others.
func (view oobView) Page() *pageData { return view.pageData }

// navStateView is what a page decides about the sidebar, which is otherwise
// the same on every page: the rows it marks current, the places it opens,
// and the sections it hides. Each is a space-separated list of row IDs.
type navStateView struct {
	Current, Expanded, Hidden string
}

func navState(nodes []*navNodeView) navStateView {
	var current, expanded, hidden []string
	var walk func([]*navNodeView)
	walk = func(nodes []*navNodeView) {
		for _, node := range nodes {
			if node.NodeID != "" {
				if node.Active {
					current = append(current, node.NodeID)
				}
				if node.Expanded && len(node.Children) > 0 {
					expanded = append(expanded, node.NodeID)
				}
				if node.Hidden {
					hidden = append(hidden, node.NodeID)
				}
			}
			walk(node.Children)
		}
	}
	walk(nodes)
	return navStateView{Current: strings.Join(current, " "), Expanded: strings.Join(expanded, " "), Hidden: strings.Join(hidden, " ")}
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
	// Hidden is a section the sidebar carries for the other side of the
	// header: it is there for the next page, not this one.
	Hidden   bool
	Children []*navNodeView
	// dormant marks a subtree the sidebar holds for other pages: a feature
	// the page does not belong to, or the other side's section. The sidebar
	// is the same on every page, so what this page marks current or opens is
	// decided outside these subtrees.
	dormant bool
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
	Deferred    bool
	DOMID       string
	URL         string
	Markdown    template.HTML
	Plain       string
	Interactive bool
	// BackgroundFrame leaves an interactive frame for the deck viewer's
	// script to load; see viewScope.backgroundFrames.
	BackgroundFrame bool
	Image           bool
	AspectRatio     string
	SectionTitle    string
	LandmarkViews   []*landmarkView
	Stories         *storyLinksView
	ChangeCount     int
	Attached        *attachedCodeView
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
	// The Saga is read in the background as soon as the server is up, and
	// again whenever it changes, so a reviewer's first page and the page
	// after an edit find it already read.
	watchCtx, stopWatching := context.WithCancel(ctx)
	defer stopWatching()
	go application.watchSaga(watchCtx)
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
	// Every route is stamped with its arrival, so the caches one request
	// asks share the freshness check that answers it.
	page := func(pattern string, handler http.HandlerFunc) { mux.HandleFunc(pattern, arriving(handler)) }
	// Everything else is a part of a page, a file, or an answer for the
	// page's script. A boosted link that reaches one is followed by the
	// browser instead of being swapped in as though it were a page.
	handle := func(pattern string, handler http.HandlerFunc) { mux.HandleFunc(pattern, arriving(notAPage(handler))) }
	page("GET /requirements/{story}/criteria/{criterion}", application.page)
	page("GET /requirements/{story}", application.page)
	page("GET /requirements", application.page)
	page("GET /terms/{term}", application.page)
	page("GET /terms", application.page)
	page("GET /chapters/{chapter}", application.page)
	page("GET /personas/{persona}", application.page)
	page("GET /personas", application.page)
	page("GET /flags", application.page)
	page("GET /design-system", application.page)
	page("GET /technical/{kind}/{id}", application.page)
	page("GET /technical/{area}", application.page)
	page("GET /technical", application.page)
	page("GET /features/{feature}", application.page)
	page("GET /features", application.page)
	page("GET /tests/{test}", application.page)
	page("GET /", application.page)
	page("GET /reviews", application.reviewIndex)
	page("GET /reviews/{id}", application.reviewPage)
	handle("GET /reviews/{id}/code", application.reviewCodeSurface)
	handle("GET /reviews/{id}/file-diff", application.reviewFileDiffSurface)
	handle("GET /reviews/{id}/coverage", application.reviewCoverageSurface)
	handle("GET /reviews/{id}/visual/{slide}", application.reviewVisual)
	handle("GET /reviews/{id}/annotations", application.reviewAnnotations)
	handle("GET /reviews/{id}/feedback", application.reviewFeedbackSurface)
	handle("POST /reviews/{id}/decision", application.reviewDecision)
	handle("POST /reviews/{id}/comment", application.reviewComment)
	handle("GET /assets/{hash}/{name}", application.shellAssetFile)
	handle("GET /decks", application.decksPage)
	handle("GET /app.js", application.javascript)
	handle("GET "+diagram.FontPath, application.diagramFont)
	handle("GET /theme.js", application.themeScript)
	handle("GET /api/documentation", application.documentationPage)
	handle("GET /api/technical-usages", application.technicalUsagesPage)
	handle("GET /api/code", application.codePage)
	handle("GET /api/coverage", application.coveragePage)
	handle("GET /api/totals", application.coverageTotalsPage)
	handle("GET /api/reference-code", application.referenceCodePage)
	handle("GET /api/layers", application.layersAPI)
	handle("GET /api/change", application.changePage)
	handle("GET /api/history", application.historyPage)
	handle("GET /api/coverage-file", application.coverageFilePage)
	handle("GET /api/coverage-target", application.coverageTargetPage)
	handle("GET /api/file-diff", application.fileDiffFragment)
	handle("GET /api/target-code", application.targetCode)
	handle("GET /api/file-owners", application.fileOwners)
	handle("GET /api/section", application.sectionBody)
	handle("GET /api/fragment", application.fragmentContent)
	handle("GET /api/locate", application.locateAnchor)
	handle("GET /api/runtime", application.runtimeStatus)
	handle("POST /api/runtime-stop", application.runtimeStop)
	handle("GET /f/{id}/{path...}", application.fragmentFile)
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
	return template.New("page").Funcs(funcs).Parse(pageTemplate + directoryTemplates + documentationTemplates + technicalTemplates + technicalERDTemplates + technicalSelectionTemplates)
}

// templateFuncs is shared by the server and its rendering tests so a new
// presentation helper cannot be wired into one and forgotten in the other.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"comparing":            func() bool { return true },
		"short":                shortCommit,
		"kindTitle":            technicalKindTitle,
		"documentationControl": documentationControl,
		"roleTitle":            technicalRoleTitle,
		"join":                 strings.Join,
		"markdown":             markdown,
		"domID":                domID,
		"fileIcon":             fileIcon,
		"lower":                strings.ToLower,
		"asset":                assetPath,
		"oob":                  oobPart,
		"navState":             navState,
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
	a.renderPage(w, r, data, "The review page could not be rendered.")
}

// notAPage answers a boosted request, which htmx makes for a link it expects
// to be a page, by sending the browser to the URL itself.
func notAPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HX-Boosted") == "true" {
			w.Header().Set("HX-Redirect", r.URL.RequestURI())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// shellVersion names the state the kept parts of the shell were read from:
// the Saga's documentation files, and the source head the code they link
// resolves against. Empty when the Saga cannot be fingerprinted.
func (a *app) shellVersion(ctx context.Context, files *sagaFiles) string {
	if files.fingerprint == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(files.fingerprint + "\x00" + a.sagaState(ctx, false).sourceHead))
	return hex.EncodeToString(digest[:12])
}

// renderPage answers for one page of the app: the whole page, or, when htmx
// asks for it to replace the page on screen, the page's parts. Both are
// rendered from the same blocks, so a page reached by a link and the same
// URL loaded afresh cannot differ.
func (a *app) renderPage(w http.ResponseWriter, r *http.Request, data *pageData, failure string) {
	w.Header().Add("Vary", pageVary)
	name := "page"
	if partialPageRequest(r) {
		name = "page-partial"
		data.StaleShell = data.ShellVersion == "" || r.Header.Get("X-Saga-Shell") != data.ShellVersion
	}
	renderHTML(w, a.template, name, data, failure)
}

// pageVary names the request headers a page's answer depends on.
const pageVary = "HX-Request, HX-Target, HX-History-Restore-Request, X-Saga-Shell"

// partialPageRequest says htmx is asking for a page to swap into the one on
// screen: a boosted link or form, which targets #page, or a step back or
// forward, which restores into it.
func partialPageRequest(r *http.Request) bool {
	if r.Header.Get("HX-Request") != "true" {
		return false
	}
	return r.Header.Get("HX-Target") == "page" || r.Header.Get("HX-History-Restore-Request") == "true"
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
	// sub is a second path identity, such as a definition's ID beneath its
	// kind.
	sub string
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
	case path == technicalPath:
		return appRoute{kind: "technical"}, true
	case strings.HasPrefix(path, technicalPath+"/") && r.PathValue("area") != "":
		if _, ok := technicalAreaOf(r.PathValue("area"), ""); ok {
			return appRoute{kind: "technical-area", id: r.PathValue("area")}, true
		}
	case strings.HasPrefix(path, technicalPath+"/") && r.PathValue("kind") != "" && r.PathValue("id") != "":
		return appRoute{kind: "technical-entity", id: r.PathValue("kind"), sub: r.PathValue("id")}, true
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
	if route.kind == "feature" || route.kind == "requirements" {
		// These pages also ask for the related reviews, which read every
		// file, so the one check this request takes covers every file.
		a.sagaState(r.Context(), true)
	}
	document := a.outlineDocument(r.Context())
	if document == nil {
		return nil, errors.New("The saga could not be loaded. Run change-saga validate for details.")
	}
	// One fingerprint of the Saga's files serves every part the page reads.
	files := a.sagaFiles(r.Context())
	narrated := len(document.Decks)+len(document.Onboarding) > 0
	if narrated {
		document = files.narrative()
		if document == nil {
			return nil, errors.New("The slide deck could not be loaded. Run change-saga validate for details.")
		}
	}
	scope := viewScope{}
	reportRoot, slideRoot := splitReportAndDeckSections(document.Section)
	requirementsView, _, requirementsDocument, err := loadRequirementsSurface(files, document.Manifest.ID, r)
	if err != nil {
		if errors.Is(err, errRequirementNotFound) {
			return nil, err
		}
		return nil, errors.New("The requirements could not be loaded. Run change-saga validate for details.")
	}
	// The page reads the records with the complete slides' links projected
	// in. Projected from the narrative, they are the same for every page of
	// one state of the files, so they are projected once for all of them.
	if narrated {
		requirementsDocument, err = files.projectedRecords(document.Manifest.ID)
	} else {
		err = semanticgraph.ProjectSlideCriterionLinks(document, &requirementsDocument)
	}
	if err != nil {
		return nil, errors.New("The complete-slide criterion links could not be loaded. Run change-saga validate for details.")
	}
	tests, err := files.tests()
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
		ShellVersion:  a.shellVersion(r.Context(), files),
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
	case "technical", "technical-area", "technical-entity":
		if data.Technical, data.TechnicalEntity, err = a.technicalShell(r.Context(), document, route, r.URL.Query()); err != nil {
			if errors.Is(err, errTechnicalNotFound) {
				return nil, errAppPageNotFound
			}
			return nil, err
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
	// The inventory is small, identity-only reading here: no code resolves.
	inventory, inventoryErr := files.inventory(document.Manifest.ID)
	var technicalRows []*navNodeView
	if inventoryErr == nil {
		technicalRows = technicalNav(inventory)
	}
	if route.kind == "overview" {
		data.OverviewParts = overviewDirectory(document, requirementsDocument, onboarding)
		if inventoryErr == nil {
			data.OverviewParts = append(data.OverviewParts, technicalOverviewPart(inventory))
		} else {
			data.OverviewParts = append(data.OverviewParts, overviewPartView{Title: "Technical design", Href: technicalPath, Count: "unreadable", Note: "The technical inventory could not be read: " + inventoryErr.Error(), Gap: true})
		}
	}
	prototypeDocument, prototypeNote := a.prototypeDocument(document.Manifest.ID)
	data.Nav = makeAppNavTree(appNavSources{
		document: document, requirements: requirementsDocument, page: requirementsView,
		quality:    tests,
		prototypes: prototypeDocument, prototypeNote: prototypeNote,
		decks: makeDeckNavTree(slideRoot), overviewActive: overviewActive,
		pageFeature: data.PageFeature, reviewSide: data.ReviewSide,
		technical: technicalRows,
	})
	// The sidebar is the same on every page, so it outlives the page it was
	// loaded with: an in-page anchor names the overview's path rather than
	// whichever page is showing.
	rootNavLinks(data.Nav)
	if route.kind != "overview" && !data.TermsMode {
		markActiveNav(data.Nav, technicalNavPath(route, r.URL.Path))
	}
	data.PageTitle = pageTitle(data)
	return data, nil
}

// pageTitle names the page as its heading does, so a reader switching tabs,
// or told by a screen reader that a page has arrived, knows which one it is.
func pageTitle(data *pageData) string {
	switch {
	case data.RequirementsMode && data.Requirements.FocusedCriterion != nil && data.Requirements.Story != nil:
		return data.Requirements.FocusedCriterion.Label + " · " + data.Requirements.Story.Title
	case data.RequirementsMode && data.Requirements.Story != nil:
		return data.Requirements.Story.Title
	case data.RequirementsMode:
		return "Requirements"
	case data.TermsMode && data.Terms.Term != nil:
		return data.Terms.Term.Name
	case data.TermsMode:
		return "Terms and vocabulary"
	case data.Persona != nil:
		return data.Persona.Name
	case data.Feature != nil:
		return data.Feature.Title
	case data.TestCase != nil:
		return data.TestCase.Title
	case data.Features != nil:
		return data.Features.Title
	case data.Personas != nil:
		return data.Personas.Title
	case data.Flags != nil:
		return data.Flags.Title
	case data.TechnicalEntity != nil:
		return data.TechnicalEntity.Name
	case data.Technical != nil && data.Technical.Area.Title != "":
		return data.Technical.Area.Title
	case data.Technical != nil:
		return "Technical design"
	case data.DesignSystemMode:
		return "Design system"
	}
	return ""
}

// decks is the deck viewer: every slide of every embedded deck, as every page
// shows them. It is the largest thing the reviewer renders and the same for
// every page, so the pages leave it out and the shell loads it once, after
// its first paint, from /decks. Built from the narrative and the records with
// the complete slides' links projected in, it is built once for each state
// of the Saga's files, whose fingerprint it returns.
func (a *app) decks(ctx context.Context) (string, template.HTML, error) {
	files := a.sagaFiles(ctx)
	document := files.narrative()
	if document == nil {
		return "", "", errors.New("The slide deck could not be loaded. Run change-saga validate for details.")
	}
	if len(document.Decks)+len(document.Onboarding) == 0 {
		return files.fingerprint, "", nil
	}
	_, html, err := files.slides(func() (*sectionView, template.HTML, error) {
		records, err := files.projectedRecords(document.Manifest.ID)
		if err != nil {
			return nil, "", fmt.Errorf("the complete-slide criterion links could not be loaded: %w", err)
		}
		_, slideRoot := splitReportAndDeckSections(document.Section)
		root := makeSectionView(slideRoot, viewScope{backgroundFrames: true})
		storyLinks := &storyLinkDecorator{document: document, records: records}
		for _, deck := range root.ChildViews {
			for _, slide := range deck.FragmentViews {
				if err := storyLinks.decorate(slide); err != nil {
					return nil, "", fmt.Errorf("story links could not be resolved: %w", err)
				}
			}
		}
		labelDeckRoles(root, document)
		var rendered bytes.Buffer
		if err := a.template.ExecuteTemplate(&rendered, "deck-viewer", root); err != nil {
			return nil, "", fmt.Errorf("the decks could not be rendered: %w", err)
		}
		return root, template.HTML(rendered.String()), nil
	})
	return files.fingerprint, html, err
}

// decksPage serves the deck viewer to the shell. Its validator is the state
// of the Saga it was built from, so a new session on an unchanged Saga
// revalidates it rather than downloading it again, and a changed Saga is
// never served from a cache.
func (a *app) decksPage(w http.ResponseWriter, r *http.Request) {
	fingerprint, html, err := a.decks(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if fingerprint == "" {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		etag := `"` + fingerprint + `"`
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = io.WriteString(w, string(html))
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
			if node.dormant {
				continue
			}
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
	return a.sagaFiles(ctx).narrative()
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
	// backgroundFrames names each slide's frame without loading it: the deck
	// viewer holds every slide, hidden, and its script loads the frames a few
	// at a time in the background, the slide on screen first, so they never
	// crowd out the reader's next page.
	backgroundFrames bool
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
	view := &fragmentView{Fragment: fragment, DOMID: domID(fragment.Target), BackgroundFrame: scope.backgroundFrames}
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
	// A page embeds every slide's files, so the index they are found in is
	// read once for each state of the Saga's files rather than per file.
	index, validation, err := a.sagaFiles(r.Context()).mutation()
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
	w.Header().Set("Content-Security-Policy", authoredContentPolicy)
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
	// The browser may keep the file but must ask before each use; an
	// unchanged file is answered by its modification time alone.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(realPath), info.ModTime(), file)
}

// diagramFont serves the font generated diagram SVGs measure their text
// with. Slides render in sandboxed, opaque-origin frames and browsers fetch
// fonts in CORS mode, so the response must allow any origin; the bytes are
// public and fixed for this binary.
func (a *app) diagramFont(w http.ResponseWriter, _ *http.Request) {
	data, _ := diagram.Font()
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
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

// authoredContentPolicy governs author-provided fragments and review visuals.
// Their script needs 'unsafe-inline', so the sandbox directive gives the
// response an opaque origin wherever it loads, as the sandboxed iframe that
// embeds it already does. Without it, opening the URL directly would run that
// script on the app's own origin, beside pages carrying review tokens.
const authoredContentPolicy = "sandbox allow-scripts; default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

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
