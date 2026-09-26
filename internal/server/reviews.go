package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A pull request's review is the one place the reviewer offers approval and
// comments. It is a slide deck viewed against the review's base: each
// Item's code references render as the diff of the referenced lines between
// the merge-base and the pull request's head, and each slide shows every
// reviewer's decision with an out-of-date marker when the slide or its code
// changed since. The living documentation is never approvable.

type reviewIndexView struct {
	Saga    *saga.Saga
	Reviews []reviewSummaryView
	// Directory is every review as one filterable table. It is the first
	// thing the page shows, because a reviewer arriving at Reviews is looking
	// for one review and not for all of them at once.
	Directory *directoryView
}

type reviewSummaryView struct {
	Report reviewstate.Report
	Href   string
	// Hidden marks a review the directory's filter ruled out, so the detail
	// beneath the table shows the same reviews the table does.
	Hidden bool
	// Matches is set when the review's head is the head this reviewer was
	// opened to compare, so its slides sit beside the living layers.
	Matches bool
}

type reviewPageView struct {
	Saga          *saga.Saga
	Review        *saga.Review
	Report        reviewstate.Report
	Frozen        bool
	Comparing     bool
	Slides        []*reviewSlideView
	MutationToken string
	Notice        string
}

// reviewCoverageView is how completely the deck explains the review's range:
// the counts, every uncovered change as a diff row, and stale references.
// Like the Code view's gaps, it is reported, never a verdict.
type reviewCoverageView struct {
	Summary coverage.Summary
	Files   []reviewGapFile
	Stale   []coverage.StaleReference
}

// reviewGapFile is one file's uncovered changes.
type reviewGapFile struct {
	Path      string
	Locations []string
	Lines     []reviewDiffLine
	Events    []string
}

type reviewSlideView struct {
	Snapshot    string
	Slide       *saga.Slide
	DOMID       string
	VisualURL   string
	Interactive bool
	Position    int
	Report      reviewstate.SlideReport
	OutOfDate   bool
	Items       []*reviewItemView
	Threads     []*reviewThreadView
}

type reviewItemView struct {
	Item        *saga.Item
	DOMID       string
	Region      *saga.LandmarkRegion
	RecordHref  string
	RecordLabel string
	RecordKind  string
	Diffs       []*reviewDiffView
	Threads     []*reviewThreadView
}

type reviewDiffView struct {
	Path     string
	Location string
	Note     string
	Lines    []reviewDiffLine
}

type reviewDiffLine struct {
	Kind string
	Old  string
	New  string
	Text string
}

type reviewThreadView struct {
	ID              string
	Target          string
	State           string
	Comments        []reviewCommentView
	Anchor          *saga.ReviewAnchor
	AnchorJSON      string
	Annotation      bool
	Deleted         bool
	ReviewID, Token string
	Frozen          bool
}

type reviewCommentView struct {
	ID        string
	Author    string
	Body      template.HTML
	CreatedAt time.Time
	State     string
}

func reviewHref(id string) string { return reviewsIndexPath + "/" + id }

// matchingReview reports whether a review's current head is the head this
// reviewer compares: the pull request it is the review of.
func matchingReview(report reviewstate.Report, headOID string) bool {
	if headOID == "" || report.Range == nil {
		return false
	}
	if report.Range.HeadOID == headOID {
		return true
	}
	return report.Merged != nil && report.Merged.Landed == headOID
}

// reviewReports builds the slide-by-slide report of reviews against the
// source checkout. It never fails: a review whose head cannot be read is
// reported with that diagnostic.
func (a *app) reviewReports(ctx context.Context, document *saga.Saga, reviews []*saga.Review) []reviewstate.Report {
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err == nil {
		defer resolver.Close()
	} else {
		resolver = nil
	}
	repository := document.Manifest.Source.Repository
	build := func(review *saga.Review) reviewstate.Report {
		report := reviewstate.Build(ctx, review, reviewstate.Options{Checkout: a.sourceDir, SagaRoot: document.Root, Resolver: resolver, Repository: repository, SkipCoverage: true})
		if report.Range != nil && resolver != nil {
			if covered, err := a.reviewCoverage(ctx, review, *report.Range, repository, resolver); err != nil {
				report.Diagnostics = append(report.Diagnostics, "the review's coverage could not be read: "+err.Error())
			} else {
				report.Coverage = covered
			}
		}
		return report
	}
	// Each review resolves its own range with Git, which is waiting rather
	// than work, so a few are built at once. The resolver serializes its
	// own reads.
	reports := make([]reviewstate.Report, len(reviews))
	next := make(chan int)
	var workers sync.WaitGroup
	for range min(reviewReportWorkers, len(reviews)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range next {
				reports[index] = build(reviews[index])
			}
		}()
	}
	for index := range reviews {
		next <- index
	}
	close(next)
	workers.Wait()
	return reports
}

// reviewReportWorkers bounds how many reviews' reports are built at once.
const reviewReportWorkers = 4

// comparedHead is the head commit this reviewer was opened to compare, or ""
// when it observes.
func (a *app) comparedHead(ctx context.Context) string {
	if a.rng.Observe() {
		return ""
	}
	head, err := resolveCommit(ctx, a.sourceDir, a.rng.HeadRevision())
	if err != nil {
		return ""
	}
	return head
}

func (a *app) loadReviewDocument(w http.ResponseWriter) *saga.Saga {
	document, validation, err := saga.Load(a.root)
	if err != nil || !validation.Valid {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return nil
	}
	return document
}

func (a *app) reviewIndex(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	head := a.comparedHead(r.Context())
	view := reviewIndexView{Saga: document}
	for _, report := range a.reviewReports(r.Context(), document, document.Reviews) {
		view.Reviews = append(view.Reviews, reviewSummaryView{Report: report, Href: reviewHref(report.ID), Matches: matchingReview(report, head)})
	}
	view.Directory = reviewsDirectory(view.Reviews, directoryQuery(r))
	for index, row := range view.Directory.Rows {
		view.Reviews[index].Hidden = row.Hidden
	}
	a.inShell(w, r, "review-index", view, reviewSurfaces{deckLabel: "Reviews", title: "Reviews"})
}

func (a *app) reviewPage(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil || review.Deck == nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	report := a.reviewReports(ctx, document, []*saga.Review{review})[0]
	view := reviewPageView{
		Saga: document, Review: review, Report: report, Frozen: review.Merged != nil,
		Comparing: !a.rng.Observe(), MutationToken: a.mutationToken, Notice: r.URL.Query().Get("notice"),
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err == nil {
		defer resolver.Close()
	} else {
		resolver = nil
	}
	var diffs *reviewDiffs
	if report.Range != nil {
		diffs = newReviewDiffs(a.sourceDir, resolver, *report.Range)
	}
	threads := reviewstate.Threads(review.Comments)
	slideReports := map[string]reviewstate.SlideReport{}
	for _, slide := range report.Slides {
		slideReports[slide.ID] = slide
	}
	for index, slide := range review.Deck.Slides {
		slideView := &reviewSlideView{
			Snapshot: a.reviewSnapshot(slide, report.Range), Slide: slide, DOMID: domID(slide.Target), VisualURL: reviewHref(review.ID) + "/visual/" + slide.ID,
			Interactive: slide.MediaType == "text/html" || slide.MediaType == "image/svg+xml", Position: index + 1, Report: slideReports[slide.ID],
		}
		if slide.MediaType == "image/svg+xml" {
			if data, readErr := os.ReadFile(filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))); readErr == nil {
				if aspect := svgAspectRatio(string(data)); aspect != "" {
					slideView.VisualURL += "?saga_aspect=" + aspect
				}
			}
		}
		for _, decision := range slideView.Report.Decisions {
			slideView.OutOfDate = slideView.OutOfDate || decision.Currency == reviewstate.OutOfDate
		}
		slideView.Threads = threadViewsFor(threads, slide.Target, review.ID, a.mutationToken, view.Frozen)
		for _, item := range slide.Items {
			region := item.Hotspot
			if region == nil && item.Selector.Type == "region" {
				region = &saga.LandmarkRegion{X: item.Selector.X, Y: item.Selector.Y, Width: item.Selector.Width, Height: item.Selector.Height}
			}
			itemView := &reviewItemView{Item: item, DOMID: domID(item.Target), Region: region, Threads: threadViewsFor(threads, item.Target, review.ID, a.mutationToken, view.Frozen)}
			if item.Record != "" {
				itemView.RecordHref, itemView.RecordLabel, itemView.RecordKind = recordHref(document, item.Record), recordLabel(item.Record), reviewRecordKind(item.Record)
			}
			if report.Range != nil {
				for _, file := range item.Code {
					for _, reference := range file.References {
						itemView.Diffs = append(itemView.Diffs, diffs.referenceDiff(ctx, reference))
					}
				}
			}
			slideView.Items = append(slideView.Items, itemView)
		}
		view.Slides = append(view.Slides, slideView)
	}
	// Opening a review gives the deck the whole review pane. Code and coverage
	// remain the same secondary surfaces implementation decks use; exact Items
	// carry reviewers to the review-range diff and affected Saga records.
	a.inShell(w, r, "review-page", view, reviewSurfaces{
		deckLabel: "Deck", reviewDeck: true, title: reviewNavTitle(review),
		codeHref: reviewHref(review.ID) + "/code", coverageHref: reviewHref(review.ID) + "/coverage",
	})
}

// reviewSurfaces is what the Review side offers beside the deck: where its
// Code Diff and its coverage load from. A review's own range on a review's
// page; the comparison the reviewer was opened with on the index, which has
// no single range of its own.
type reviewSurfaces struct {
	deckLabel, codeHref, coverageHref, title string
	reviewDeck                               bool
}

// inShell renders a review surface inside the app shell, so the reviews sit
// beside the sidebar and the tabs like every other page.
func (a *app) inShell(w http.ResponseWriter, r *http.Request, name string, view any, surfaces reviewSurfaces) {
	var body bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&body, name, view); err != nil {
		http.Error(w, "The review could not be rendered.", http.StatusInternalServerError)
		return
	}
	data, err := a.shell(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if surfaces.deckLabel != "" {
		data.DeckLabel = surfaces.deckLabel
	}
	data.ReviewDeck = surfaces.reviewDeck
	data.PageTitle = surfaces.title
	if surfaces.codeHref != "" || surfaces.coverageHref != "" {
		data.ReviewCodeHref, data.ReviewCoverageHref = surfaces.codeHref, surfaces.coverageHref
	}
	data.Reviews = template.HTML(body.String())
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	a.renderPage(w, r, data, "The review could not be rendered.")
}

// reviewCoverage renders every uncovered change as the diff row it is: a
// deleted line at the merge-base, an added line at the head, or a file event.
func reviewCoverage(covered *reviewstate.Coverage) *reviewCoverageView {
	view := &reviewCoverageView{Summary: covered.Summary, Stale: covered.StaleReferences}
	files := map[string]*reviewGapFile{}
	for _, file := range covered.UncoveredFiles {
		view.Files = append(view.Files, reviewGapFile{Path: file.Path, Locations: file.Locations})
	}
	for index := range view.Files {
		files[view.Files[index].Path] = &view.Files[index]
	}
	for _, atom := range covered.Uncovered {
		path := atom.Path
		if atom.Kind != "line" && atom.NewPath != "" {
			path = atom.NewPath
		}
		file := files[path]
		if file == nil {
			if file = files[atom.OldPath]; file == nil {
				continue
			}
		}
		if atom.Kind != "line" {
			file.Events = append(file.Events, coverage.DescribeAtom(atom))
			continue
		}
		line := reviewDiffLine{Kind: "add", New: strconv.Itoa(atom.Line), Text: atom.Content}
		if atom.Side == "old" {
			line = reviewDiffLine{Kind: "del", Old: strconv.Itoa(atom.Line), Text: atom.Content}
		}
		file.Lines = append(file.Lines, line)
	}
	return view
}

func threadViewsFor(threads []*reviewstate.Thread, target, reviewID, token string, frozen bool) []*reviewThreadView {
	var result []*reviewThreadView
	for _, thread := range threads {
		if thread.Root.Target != target {
			continue
		}
		view := &reviewThreadView{ID: thread.Root.ID, Target: thread.Root.Target, State: thread.State, ReviewID: reviewID, Token: token, Frozen: frozen}
		for _, comment := range append([]saga.ReviewComment{thread.Root}, thread.Replies...) {
			switch comment.AnnotationAction {
			case "create", "update":
				view.Annotation, view.Deleted, view.Anchor = true, false, comment.Anchor
			case "delete":
				view.Annotation, view.Deleted = true, true
			}
			view.Comments = append(view.Comments, reviewCommentView{ID: comment.ID, Author: reviewerSeat(comment.Reviewer), Body: markdown(comment.Body), CreatedAt: comment.CreatedAt, State: comment.State})
		}
		if view.Anchor != nil {
			if data, err := json.Marshal(view.Anchor); err == nil {
				view.AnchorJSON = string(data)
			}
		}
		result = append(result, view)
	}
	return result
}

func reviewerSeat(reviewer saga.ReviewerIdentity) string {
	if reviewer.Kind == "ai" {
		return fmt.Sprintf("AI %s · %s · %s", reviewer.Name, reviewer.Agent, reviewer.Model)
	}
	return "Human"
}

// recordHref links a review Item's record into the living Saga: a report or
// deck node opens in place, a story opens its requirement page, and a term
// opens its page in the overview's vocabulary.
func recordHref(document *saga.Saga, record string) string {
	prefix := "urn:change-saga:" + document.Manifest.ID + ":"
	if id, ok := strings.CutPrefix(record, prefix+"story:"); ok {
		return "/requirements/" + id
	}
	if id, ok := strings.CutPrefix(record, prefix+"term:"); ok && !strings.Contains(id, ":") {
		return termHref(id)
	}
	return "/" + sagaHref(record)
}

func recordLabel(record string) string {
	parts := strings.Split(record, ":")
	if len(parts) < 2 {
		return record
	}
	return parts[len(parts)-2] + " " + parts[len(parts)-1]
}

func reviewRecordKind(record string) string {
	parts := strings.Split(record, ":")
	if len(parts) < 2 {
		return "Record"
	}
	switch parts[len(parts)-2] {
	case "story":
		return "Story"
	case "feature":
		return "Feature"
	default:
		return "Record"
	}
}

// referenceDiff is the diff of one Item's code reference against the
// review's base: the hunks between the merge-base and the head that touch the
// referenced lines as they are at the head.
func (a *app) referenceDiff(ctx context.Context, resolver *coderesolve.Resolver, reference coderef.Reference, rng reviewstate.Range) *reviewDiffView {
	return newReviewDiffs(a.sourceDir, resolver, rng).referenceDiff(ctx, reference)
}

// diffLinesTouching keeps the hunks of a unified patch whose new-side range
// overlaps start..end (every hunk for a whole file, start 0).
func diffLinesTouching(patch string, start, end int) []reviewDiffLine {
	var result, hunk []reviewDiffLine
	keep := false
	oldLine, newLine := 0, 0
	flush := func() {
		if keep {
			result = append(result, hunk...)
		}
		hunk, keep = nil, false
	}
	scanner := bufio.NewScanner(strings.NewReader(patch))
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@@") {
			flush()
			inHunk = true
			oldStart, oldCount, newStart, newCount := parseHunkHeader(line)
			oldLine, newLine = oldStart, newStart
			last := newStart + newCount - 1
			if newCount == 0 {
				last = newStart
			}
			keep = start == 0 || (newStart <= end && last >= start) || (oldCount > 0 && newCount == 0 && newStart >= start-1 && newStart <= end)
			hunk = append(hunk, reviewDiffLine{Kind: "hunk", Text: line})
			continue
		}
		if !inHunk || strings.HasPrefix(line, "\\") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			hunk = append(hunk, reviewDiffLine{Kind: "add", New: strconv.Itoa(newLine), Text: line[1:]})
			newLine++
		case strings.HasPrefix(line, "-"):
			hunk = append(hunk, reviewDiffLine{Kind: "del", Old: strconv.Itoa(oldLine), Text: line[1:]})
			oldLine++
		case strings.HasPrefix(line, " "):
			hunk = append(hunk, reviewDiffLine{Kind: "ctx", Old: strconv.Itoa(oldLine), New: strconv.Itoa(newLine), Text: line[1:]})
			oldLine++
			newLine++
		default:
			inHunk = false
		}
	}
	flush()
	return result
}

func parseHunkHeader(line string) (oldStart, oldCount, newStart, newCount int) {
	fields := strings.Fields(line)
	parse := func(value string) (int, int) {
		value = strings.TrimLeft(value, "-+")
		first, second, found := strings.Cut(value, ",")
		start, _ := strconv.Atoi(first)
		count := 1
		if found {
			count, _ = strconv.Atoi(second)
		}
		return start, count
	}
	if len(fields) >= 3 {
		oldStart, oldCount = parse(fields[1])
		newStart, newCount = parse(fields[2])
	}
	return
}

// reviewVisual serves one review slide's visual with the fragment CSP.
func (a *app) reviewVisual(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil || review.Deck == nil {
		http.NotFound(w, r)
		return
	}
	slide := review.Slide(r.PathValue("slide"))
	if slide == nil {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))
	if filepath.Dir(path) != filepath.Clean(slide.Directory) {
		http.Error(w, "invalid slide visual", http.StatusBadRequest)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Security-Policy", authoredContentPolicy)
	w.Header().Set("Cache-Control", "no-store")
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
}

// reviewMutation checks the session token and form of a review write. The
// browser reviewer records its human user's own decisions; AI reviewers
// record theirs through the CLI with their seat.
func (a *app) reviewMutation(w http.ResponseWriter, r *http.Request) (*saga.Review, string, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return nil, "", false
	}
	if a.mutationToken == "" || r.PostForm.Get("token") != a.mutationToken {
		http.Error(w, "missing or invalid mutation token", http.StatusForbidden)
		return nil, "", false
	}
	document := a.loadReviewDocument(w)
	if document == nil {
		return nil, "", false
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil {
		http.NotFound(w, r)
		return nil, "", false
	}
	if review.Merged != nil {
		http.Error(w, "This review is history: its change has landed.", http.StatusConflict)
		return nil, "", false
	}
	rng, err := reviewstate.ResolveRange(r.Context(), a.sourceDir, review)
	if err != nil {
		http.Error(w, "The review's head could not be read: "+err.Error(), http.StatusConflict)
		return nil, "", false
	}
	return review, rng.HeadOID, true
}

func (a *app) reviewDecision(w http.ResponseWriter, r *http.Request) {
	review, head, ok := a.reviewMutation(w, r)
	if !ok {
		return
	}
	slide := r.PostForm.Get("slide")
	decision, err := reviewstore.Decide(a.root, reviewstore.Decision{
		CheckSnapshot: a.reviewSnapshotCheck(r, head), Review: review.ID, Slide: slide, State: r.PostForm.Get("state"),
		Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: head, Body: r.PostForm.Get("body"),
	})
	if err != nil {
		reviewWriteError(w, err)
		return
	}
	savedTarget := slide
	if found := review.Slide(decision.Slide); found != nil {
		savedTarget = found.Target
	}
	if a.reviewSaved(w, r, review.ID, decision.ID, savedTarget) {
		return
	}
	target := ""
	if found := review.Slide(slide); found != nil {
		target = "#" + domID(found.Target)
	}
	http.Redirect(w, r, reviewHref(review.ID)+target, http.StatusSeeOther)
}

func (a *app) reviewComment(w http.ResponseWriter, r *http.Request) {
	review, head, ok := a.reviewMutation(w, r)
	if !ok {
		return
	}
	var anchor *saga.ReviewAnchor
	if source := strings.TrimSpace(r.PostForm.Get("anchor")); source != "" {
		var decoded saga.ReviewAnchor
		if err := json.Unmarshal([]byte(source), &decoded); err != nil {
			http.Error(w, "The annotation anchor is invalid.", http.StatusBadRequest)
			return
		}
		anchor = &decoded
	}
	comment, err := reviewstore.Comment(a.root, reviewstore.Remark{
		CheckSnapshot: a.reviewSnapshotCheck(r, head), Review: review.ID, Target: r.PostForm.Get("target"), ReplyTo: r.PostForm.Get("reply_to"),
		Body: r.PostForm.Get("body"), State: r.PostForm.Get("state"),
		Anchor: anchor, AnnotationAction: r.PostForm.Get("annotation_action"),
		Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: head,
	})
	if err != nil {
		reviewWriteError(w, err)
		return
	}
	if a.reviewSaved(w, r, review.ID, comment.ID, comment.Target) {
		return
	}
	http.Redirect(w, r, reviewHref(review.ID)+"#"+domID(comment.Target), http.StatusSeeOther)
}

// reviewAnnotations returns only the visual collaboration layer. Keeping this
// projection separate lets the authored slide remain the first paint while
// preserving exact, append-only annotation records in the Saga repository.
func (a *app) reviewAnnotations(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil {
		http.NotFound(w, r)
		return
	}
	type message struct{ Author, Body, CreatedAt string }
	type annotation struct {
		ID, Target string
		Anchor     *saga.ReviewAnchor
		Deleted    bool
		Messages   []message
	}
	response := struct {
		Frozen      bool
		Annotations []annotation
	}{Frozen: review.Merged != nil, Annotations: []annotation{}}
	for _, thread := range reviewstate.Threads(review.Comments) {
		if thread.Root.AnnotationAction != "create" || thread.Root.Anchor == nil {
			continue
		}
		entry := annotation{ID: thread.Root.ID, Target: thread.Root.Target, Anchor: thread.Root.Anchor}
		for _, comment := range append([]saga.ReviewComment{thread.Root}, thread.Replies...) {
			switch comment.AnnotationAction {
			case "create", "update":
				entry.Anchor, entry.Deleted = comment.Anchor, false
			case "delete":
				entry.Deleted = true
			}
			if comment.AnnotationAction == "" || comment.AnnotationAction == "create" {
				entry.Messages = append(entry.Messages, message{Author: reviewerSeat(comment.Reviewer), Body: comment.Body, CreatedAt: comment.CreatedAt.Format(time.RFC3339)})
			}
		}
		response.Annotations = append(response.Annotations, entry)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "The annotations could not be rendered.", http.StatusInternalServerError)
	}
}

// reviewsForHead summarizes the reviews of the compared head for the Change
// tab, which shows the living layers read-only beside them.
func (a *app) reviewsForHead(ctx context.Context, document *saga.Saga, headOID string) []reviewSummaryView {
	var result []reviewSummaryView
	for _, report := range a.reviewReports(ctx, document, document.Reviews) {
		if matchingReview(report, headOID) {
			result = append(result, reviewSummaryView{Report: report, Href: reviewHref(report.ID), Matches: true})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Report.ID < result[j].Report.ID })
	return result
}

func newMutationToken() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

var reviewTemplates = template.Must(template.New("reviews").Funcs(templateFuncs()).Funcs(template.FuncMap{
	"reviewState": func(state string) string { return strings.ReplaceAll(state, "_", " ") },
	"reviewCommentForm": func(page reviewPageView, target, label string) reviewCommentFormView {
		return reviewCommentFormView{ReviewID: page.Review.ID, Token: page.MutationToken, Target: target, Label: label, Frozen: page.Frozen}
	},
	"reviewSlideMenu": func(page reviewPageView, slide *reviewSlideView) reviewSlideMenuView {
		return reviewSlideMenuView{Page: page, Slide: slide}
	},
	"reviewer": func(decision reviewstate.DecisionReport) string {
		if decision.Reviewer.Kind == "ai" {
			return fmt.Sprintf("AI %s (%s, %s) for %s", decision.Reviewer.Name, decision.Reviewer.Agent, decision.Reviewer.Model, decision.Author)
		}
		return decision.Author
	},
}).Parse(reviewTemplateSource + directoryTemplates + documentationTemplates + technicalERDTemplates + technicalSelectionTemplates))

// reviewTemplateSource renders the review index and one review. Plain forms
// post decisions and comments, so the page works without script.
const reviewTemplateSource = `{{define "review-summary"}}<article class="review-summary{{if .Matches}} matching{{end}}"{{if .Hidden}} hidden{{end}} data-review-summary="{{.Report.ID}}" data-directory-linked="{{.Report.ID}}"><header><a href="{{.Href}}"><strong>{{.Report.Title}}</strong></a>{{with .Report.PullRequest}}{{if .Number}} <span class="review-pr">#{{.Number}}</span>{{end}}{{end}}{{if .Report.Merged}} <span class="review-badge merged">merged</span>{{else}} <span class="review-badge open">open</span>{{end}}</header>{{template "review-range" .Report}}{{with .Report.Coverage}}<p class="coverage-totals" data-review-coverage-summary data-uncovered="{{.Summary.Uncovered}}">{{.Summary.Covered}} of {{.Summary.Total}} changed lines explained by the deck{{if .Summary.Uncovered}} · <span class="gap">{{.Summary.Uncovered}} unexplained</span>{{end}}{{if .Summary.Stale}} · <span class="gap">{{.Summary.Stale}} stale</span>{{end}}</p>{{end}}<ol class="review-slide-states">{{range .Report.Slides}}<li data-review-slide-state="{{.ID}}"><span class="review-slide-title">{{.Title}}</span>{{range .Decisions}}<span class="review-decision-chip {{.State}}{{if eq .Currency "out_of_date"}} out-of-date{{end}}" data-decision-state="{{.State}}" data-currency="{{.Currency}}">{{reviewState .State}}{{if eq .Currency "out_of_date"}} · out of date{{end}}</span>{{else}}<span class="review-decision-chip none">no decision</span>{{end}}{{if .OpenThreads}}<span class="review-threads">{{.OpenThreads}} open {{if eq .OpenThreads 1}}thread{{else}}threads{{end}}</span>{{end}}</li>{{end}}</ol></article>{{end}}
{{define "review-range"}}<p class="review-range">{{with .Range}}{{if .Frozen}}Frozen at <code>{{short .BaseOID}}</code>..<code>{{short .HeadOID}}</code>{{else}}<code>{{short .BaseOID}}</code>..<code>{{short .HeadOID}}</code> · head follows <code>{{.Following}}</code>{{end}}{{end}}{{with .Merged}} · landed as <code>{{short .Landed}}</code>{{end}}{{range .Diagnostics}}<span class="review-diagnostic">{{.}}</span>{{end}}</p>{{end}}
{{define "review-index"}}<div class="review-surface" data-review-index><header class="review-top"><h1>Reviews</h1><p>Each pull request has one review: a slide deck explaining what the change did and why. Approvals and comments happen only here, per slide. The Saga itself is documentation.</p></header>{{template "directory" .Directory}}{{if .Reviews}}<h2 class="review-detail-heading">Slide by slide</h2>{{end}}<main class="review-main">{{range .Reviews}}{{template "review-summary" .}}{{end}}</main></div>{{end}}
{{define "review-diff"}}<figure class="review-diff" data-review-diff="{{.Location}}"><figcaption><code>{{.Path}}</code> <span class="review-location">{{.Location}}</span></figcaption>{{if .Note}}<p class="review-note">{{.Note}}</p>{{end}}{{if .Lines}}<table><tbody>{{range .Lines}}<tr class="review-line {{.Kind}}">{{if eq .Kind "hunk"}}<td colspan="3" class="review-hunk">{{.Text}}</td>{{else}}<td class="review-lineno">{{.Old}}</td><td class="review-lineno">{{.New}}</td><td class="review-code"><code>{{if eq .Kind "add"}}+{{else if eq .Kind "del"}}-{{else}} {{end}}{{.Text}}</code></td>{{end}}</tr>{{end}}</tbody></table>{{end}}</figure>{{end}}
{{define "review-threads"}}{{range .}}<article class="review-thread {{.State}}" id="thread-{{.ID}}" data-review-thread="{{.ID}}" data-review-target="{{.Target}}" data-thread-state="{{.State}}"><header class="review-thread-head"><strong>Discussion</strong><span>{{.State}}</span></header>{{range .Comments}}<div class="review-comment" id="comment-{{.ID}}"><div class="review-comment-meta">{{.Author}} · <time datetime="{{.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}">{{.CreatedAt.Format "2006-01-02 15:04 MST"}}</time>{{if .State}} · {{.State}}{{end}}</div><div class="review-comment-body">{{.Body}}</div></div>{{end}}{{if not .Frozen}}<details class="review-compose review-reply"><summary>Reply</summary><form hx-boost="false" class="review-comment-form" method="post" action="/reviews/{{.ReviewID}}/comment" data-review-reply-form="{{.ID}}"><input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="reply_to" value="{{.ID}}"><label><span>Reply to discussion</span><textarea name="body" required rows="3"></textarea></label><button type="submit">Reply</button></form></details>{{end}}</article>{{end}}{{end}}
{{define "review-comment-form"}}{{if not .Frozen}}<details class="review-compose"><summary>Add comment</summary><form hx-boost="false" class="review-comment-form" method="post" action="/reviews/{{.ReviewID}}/comment" data-review-comment-form="{{.Target}}"><input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="target" value="{{.Target}}"><label><span>Comment on {{.Label}}</span><textarea name="body" required rows="3"></textarea></label><button type="submit">Comment</button></form></details>{{end}}{{end}}
{{define "review-item-panel"}}<section class="review-item-panel" data-review-item-panel="{{.Item.ID}}" data-review-target="{{.Item.Target}}"><header><p class="eyebrow">Slide element</p><h2>{{.Item.Label}}</h2><p>{{.Item.Description}}</p></header>{{if .RecordHref}}<p class="review-record">Affected documentation: <a href="{{.RecordHref}}" data-review-record="{{.Item.Record}}">{{.RecordLabel}}</a></p>{{end}}{{range .Diffs}}{{template "review-diff" .}}{{else}}<p class="review-note">This element links no changed code.</p>{{end}}<div data-review-threads-for="{{.Item.Target}}">{{template "review-threads" .Threads}}</div></section>{{end}}
{{define "review-item-affordance"}}{{template "documentation-control" (documentationControl .Item.Documentation .Item.Target (len .Item.Selections))}}<span class="landmark-affordance review-item-affordance" data-landmark-affordance><a class="permalink" href="#{{.DOMID}}" aria-label="Link to {{.Item.Label}}" title="Copy link"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-link"></use></svg></a>{{if .RecordHref}}<button type="button" class="icon-button story-button" data-open-stories="review-record-{{.DOMID}}" aria-label="Open affected documentation for {{.Item.Label}}" title="Affected {{.RecordKind}}"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-story"></use></svg></button>{{end}}{{if .Diffs}}<button type="button" class="icon-button diff-button" data-open-diffs="review-item-{{.DOMID}}" aria-label="Open {{len .Diffs}} code {{if eq (len .Diffs) 1}}reference{{else}}references{{end}} for {{.Item.Label}}" title="Linked code"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-diff"></use></svg><span>{{len .Diffs}}</span></button>{{else}}<button type="button" class="icon-button diff-button" data-open-diffs="review-item-{{.DOMID}}" aria-label="Open details for {{.Item.Label}}" title="Item details"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-more"></use></svg></button>{{end}}</span>{{end}}
{{define "review-slide-menu"}}{{if not .Page.Frozen}}<form hx-boost="false" class="review-decision-quick" method="post" action="/reviews/{{.Page.Review.ID}}/decision" data-review-decision-form="{{.Slide.Slide.ID}}"><input type="hidden" name="token" value="{{.Page.MutationToken}}"><input type="hidden" name="slide" value="{{.Slide.Slide.ID}}"><input type="hidden" name="snapshot" value="{{.Slide.Snapshot}}"><button class="review-decision approve" type="submit" name="state" value="approved" data-review-approve aria-label="Approve {{.Slide.Slide.Title}}" title="Approve"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-approve"></use></svg></button></form>{{end}}<details class="review-slide-menu"><summary class="{{if .Page.Frozen}}icon-button review-history{{else}}review-decision reject{{end}}" aria-label="{{if .Page.Frozen}}Review decisions for{{else}}Request changes on{{end}} {{.Slide.Slide.Title}}" title="{{if .Page.Frozen}}Review decisions{{else}}Request changes{{end}}">{{if .Page.Frozen}}<svg class="i" aria-hidden="true" focusable="false"><use href="#i-clock"></use></svg>{{else}}<svg class="i" aria-hidden="true" focusable="false"><use href="#i-reject"></use></svg>{{end}}</summary><div class="review-slide-panel"><header><p class="eyebrow">Review slide</p><h2>{{.Slide.Slide.Title}}</h2><p>{{.Slide.Slide.Takeaway}}</p></header><ul class="review-decision-list">{{range .Slide.Report.Decisions}}<li class="review-decision-row {{.State}}{{if eq .Currency "out_of_date"}} out-of-date{{end}}" data-decision-state="{{.State}}" data-currency="{{.Currency}}"><strong>{{reviewState .State}}</strong> by {{reviewer .}} at <code>{{short .Commit}}</code>{{if eq .Currency "out_of_date"}} <span class="review-out-of-date" data-out-of-date>Out of date</span>{{else if eq .Currency "unknown"}} <span class="review-unknown">currency unknown</span>{{end}}{{if .Reasons}}<ul class="review-reasons">{{range .Reasons}}<li>{{.}}</li>{{end}}</ul>{{end}}{{if .Body}}<p class="review-body">{{.Body}}</p>{{end}}</li>{{else}}<li class="review-decision-row none">No decision yet</li>{{end}}</ul>{{if not .Page.Frozen}}<form hx-boost="false" class="review-decision-form" method="post" action="/reviews/{{.Page.Review.ID}}/decision"><input type="hidden" name="token" value="{{.Page.MutationToken}}"><input type="hidden" name="slide" value="{{.Slide.Slide.ID}}"><input type="hidden" name="snapshot" value="{{.Slide.Snapshot}}"><label><span>What needs to change?</span><textarea name="body" rows="3" required></textarea></label><div class="review-decision-buttons"><button type="submit" name="state" value="changes_requested" data-review-request-changes>Request changes</button><button type="submit" name="state" value="none" data-review-withdraw>Withdraw decision</button></div></form>{{end}}<details class="review-source"><summary>Source and currency</summary>{{template "review-range" .Page.Report}}</details></div></details><details class="review-slide-comment"><summary class="icon-button review-comment" aria-label="{{if .Page.Frozen}}Discussion on{{else}}Comment on{{end}} {{.Slide.Slide.Title}}" title="{{if .Page.Frozen}}Discussion{{else}}Comment{{end}}"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-comment"></use></svg></summary><div class="review-slide-panel"><div data-review-threads-for="{{.Slide.Slide.Target}}">{{template "review-threads" .Slide.Threads}}</div>{{template "review-comment-form" (reviewCommentForm .Page .Slide.Slide.Target .Slide.Slide.Title)}}</div></details>{{end}}
{{define "review-page"}}{{$page := .}}<div class="review-deck-page" data-review="{{.Review.ID}}" data-review-token="{{.MutationToken}}" data-review-deck><nav class="review-deck-rail slide-side" aria-label="Review slides"><a class="sidebar-title" href="/reviews"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-book"></use></svg><span>{{.Review.Title}}</span></a>{{with .Review.PullRequest}}<p class="review-deck-pr">{{if .URL}}<a href="{{.URL}}">{{if .Number}}Pull request #{{.Number}}{{else}}{{.URL}}{{end}}</a>{{else}}Pull request #{{.Number}}{{end}}</p>{{end}}<section class="slide-rail-deck"><h2>Review deck</h2><ol class="review-thumbnail-list slide-thumbnail-list">{{range $index,$slide := .Slides}}<li class="slide-thumbnail-card{{if eq $index 0}} active{{end}}" data-slide-thumbnail-card><div class="slide-thumbnail-preview" aria-hidden="true">{{if .Interactive}}<iframe tabindex="-1" sandbox="allow-scripts" loading="lazy" src="{{.VisualURL}}" title=""></iframe>{{else}}<img loading="lazy" src="{{.VisualURL}}" alt="">{{end}}</div><span class="slide-thumbnail-caption"><span class="slide-thumbnail-title">{{.Slide.Title}}</span><span class="slide-thumbnail-status" role="img" data-review-state="{{range .Report.Decisions}}{{.State}}{{else}}none{{end}}" aria-label="{{range .Report.Decisions}}{{reviewState .State}}{{if eq .Currency "out_of_date"}}; out of date{{end}}{{else}}Not reviewed{{end}}{{if .Report.OpenThreads}}; {{.Report.OpenThreads}} open threads{{end}}" title="{{range .Report.Decisions}}{{reviewState .State}}{{if eq .Currency "out_of_date"}} · out of date{{end}}{{else}}Not reviewed{{end}}{{if .Report.OpenThreads}} · {{.Report.OpenThreads}} open{{end}}"></span></span><button type="button" class="slide-thumbnail-hit" data-slide-thumbnail data-slide-target="{{.Slide.Target}}" aria-current="{{if eq $index 0}}true{{else}}false{{end}}" aria-label="Show slide: {{.Slide.Title}}"></button></li>{{end}}</ol></section></nav><section class="deck-viewer review-deck-viewer" data-deck-viewer aria-label="{{.Review.Title}}"><div class="deck-viewer-stage"><header class="deck-viewer-header" aria-live="polite"><div><strong data-current-slide-title>{{(index .Slides 0).Slide.Title}}</strong></div><span data-slide-position></span></header>{{range $index,$slide := .Slides}}<section class="deck-viewer-slide review-deck-slide{{if eq $index 0}} active{{end}}{{if .OutOfDate}} out-of-date{{end}}" data-deck-slide data-deck-target="{{$page.Review.Deck.Target}}" data-deck-role="review" data-deck-title="{{$page.Review.Title}}" data-slide-title="{{.Slide.Title}}" data-review-snapshot="{{.Snapshot}}" data-slide-target="{{.Slide.Target}}"{{if ne $index 0}} hidden{{end}}><article class="fragment review-fragment" id="{{.DOMID}}" data-target="{{.Slide.Target}}" data-fragment-title="{{.Slide.Title}}" tabindex="0"><div class="fragment-head"><div class="fragment-actions">{{if .Items}}<details class="landmark-menu"><summary aria-label="Jump to a marked place" title="Marked places"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-list"></use></svg></summary><div class="landmark-list">{{range .Items}}<div><a href="#{{.DOMID}}">{{.Item.Label}}</a>{{template "review-item-affordance" .}}</div>{{end}}</div></details>{{end}}<a class="permalink" href="#{{.DOMID}}" aria-label="Link to {{.Slide.Title}}" title="Copy link"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-link"></use></svg></a>{{if not $page.Frozen}}<button class="icon-button annotation-tools-toggle" type="button" data-review-annotation-toggle="{{.Slide.Target}}" aria-expanded="false" aria-controls="review-annotation-toolbox" aria-label="Show annotation tools for {{.Slide.Title}}" title="Annotate"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-marker"></use></svg></button>{{end}}{{template "review-slide-menu" (reviewSlideMenu $page .)}}</div></div>{{range .Items}}<span id="{{.DOMID}}" class="landmark-target" data-review-item="{{.Item.ID}}" data-landmark-target data-landmark-anchor="{{.DOMID}}" data-landmark-type="{{.Item.Selector.Type}}" data-element-id="{{.Item.Selector.ElementID}}" data-heading-id="{{.Item.Selector.HeadingID}}" data-exact="{{.Item.Selector.Exact}}" data-prefix="{{.Item.Selector.Prefix}}" data-suffix="{{.Item.Selector.Suffix}}"><template data-landmark-affordance-template>{{template "review-item-affordance" .}}</template></span><template id="review-item-{{.DOMID}}">{{template "review-item-panel" .}}{{template "review-comment-form" (reviewCommentForm $page .Item.Target .Item.Label)}}</template>{{if .RecordHref}}<template id="review-record-{{.DOMID}}"><section class="story-links"><h2>Affected documentation</h2><p>This exact slide element links to the living Saga record below.</p><ul><li><strong><a href="{{.RecordHref}}" data-review-record="{{.Item.Record}}">{{.RecordLabel}}</a></strong><p>{{.Item.Description}}</p></li></ul></section></template>{{end}}{{end}}<div class="fragment-stage">{{if .Interactive}}<iframe class="fragment-frame" data-fragment-frame sandbox="allow-scripts" src="{{.VisualURL}}" title="{{.Slide.Title}}"></iframe>{{else}}<img class="fragment-image" src="{{.VisualURL}}" alt="{{.Slide.Title}}">{{end}}{{range .Items}}{{if .Region}}<div class="landmark-hotspot" data-landmark-visual="{{.DOMID}}" data-x="{{.Region.X}}" data-y="{{.Region.Y}}" data-width="{{.Region.Width}}" data-height="{{.Region.Height}}">{{template "review-item-affordance" .}}</div>{{end}}{{end}}</div></article></section>{{end}}<nav class="deck-viewer-controls" aria-label="Slide navigation"><button type="button" class="slide-step" data-slide-previous aria-label="Previous slide" title="Previous slide">‹</button><button type="button" class="slide-step" data-slide-next aria-label="Next slide" title="Next slide">›</button></nav><button type="button" class="slide-exit-presentation" data-slide-exit-presentation hidden>Exit presentation</button></div></section></div>{{end}}
{{define "review-coverage-surface"}}<div data-review-surface-response="manifest"><div class="review-surface review-coverage-surface">{{template "review-range" .Report}}{{if .Coverage}}{{template "review-coverage" .Coverage}}{{else}}<p class="review-note">The review's range could not be read, so its coverage is unknown.</p>{{end}}</div></div>{{end}}
{{define "review-coverage"}}{{if .}}<aside class="review-coverage{{if .Summary.Uncovered}} has-gap{{end}}" aria-label="Coverage of the change" data-review-coverage data-total="{{.Summary.Total}}" data-covered="{{.Summary.Covered}}" data-uncovered="{{.Summary.Uncovered}}" data-stale="{{.Summary.Stale}}" data-overlapping="{{.Summary.Overlapping}}"><h2>Coverage of the change</h2><p class="coverage-totals">{{.Summary.Total}} changed {{if eq .Summary.Total 1}}line{{else}}lines{{end}} · {{.Summary.Covered}} explained by the deck{{if .Summary.Uncovered}} · <span class="gap">{{.Summary.Uncovered}} unexplained</span>{{end}}{{if .Summary.Stale}} · <span class="gap">{{.Summary.Stale}} stale {{if eq .Summary.Stale 1}}reference{{else}}references{{end}}</span>{{end}}{{if .Summary.Overlapping}} · {{.Summary.Overlapping}} explained twice{{end}}</p>{{if .Files}}<p class="review-note">No review Item explains these changes. Cover them from the Item that does: <code>change-saga cover --target &lt;review Item&gt; --path &lt;file&gt; --changed-lines</code></p>{{range .Files}}<figure class="review-diff review-gap" data-review-gap="{{.Path}}"><figcaption><code>{{.Path}}</code>{{range .Locations}} <span class="review-location">{{.}}</span>{{end}}</figcaption>{{range .Events}}<p class="review-note">{{.}}</p>{{end}}{{if .Lines}}<table><tbody>{{range .Lines}}<tr class="review-line {{.Kind}}"><td class="review-lineno">{{.Old}}</td><td class="review-lineno">{{.New}}</td><td class="review-code"><code>{{if eq .Kind "add"}}+{{else}}-{{end}}{{.Text}}</code></td></tr>{{end}}</tbody></table>{{end}}</figure>{{end}}{{else}}<p class="review-note">Every changed line of the review's range is explained by the deck.</p>{{end}}{{if .Stale}}<h3>Stale references</h3><ul class="review-reasons">{{range .Stale}}<li data-review-stale="{{.Assignment.Target}}"><code>{{.Reference.Location}}</code>: {{.Reason}}</li>{{end}}</ul>{{end}}</aside>{{end}}{{end}}`

// reviewCommentFormView carries what a comment form needs from its page.
type reviewCommentFormView struct {
	ReviewID, Token, Target, Label string
	Frozen                         bool
}

type reviewSlideMenuView struct {
	Page  reviewPageView
	Slide *reviewSlideView
}

const reviewStyles = `
.review-surface{max-width:1560px;margin:0 auto;font:15px/1.5 var(--ui)}
.review-top{padding:0 4px}.review-top h1{margin:8px 0 2px}.review-range code{font-size:12px}.review-diagnostic{display:block;color:#a15c00}
.review-summary{border:1px solid var(--line,#ddd);border-radius:10px;padding:12px 16px;margin:12px 0}
.review-summary.matching{border-color:#2f6fdc}.review-badge{font-size:12px;padding:1px 8px;border-radius:9px;background:#eee;color:#333}
.review-slide-states{list-style:none;padding:0}.review-slide-states li{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin:4px 0}
.review-decision-chip{font-size:12px;padding:1px 8px;border-radius:9px;background:#eef}.review-decision-chip.approved{background:#dcf5e3;color:#14532d}
.review-decision-chip.changes_requested{background:#fde2e1;color:#7f1d1d}.review-decision-chip.out-of-date{outline:2px dashed #b45309}
.review-out-of-date{font-size:12px;font-weight:700;color:#fff;background:#b45309;border-radius:9px;padding:1px 8px}
.review-reasons{font-size:13px;color:#7c2d12}
.review-decision-form textarea,.review-comment-form textarea{width:100%;box-sizing:border-box}
.review-decision-buttons{display:flex;gap:6px;flex-wrap:wrap;margin-top:6px}
.review-diff{margin:8px 0;overflow-x:auto}.review-diff table{border-collapse:collapse;width:100%;font:12px/1.4 var(--mono)}
.review-surface{min-width:0}
.review-line.add{background:var(--add-bg)}.review-line.del{background:var(--del-bg)}.review-hunk{color:var(--muted);background:var(--bg-subtle)}
.review-lineno{width:3em;text-align:right;color:#8b949e;padding-right:6px}.review-code code{white-space:pre}
.review-thread{border-left:3px solid var(--accent);padding:8px 0 8px 12px;margin:12px 0;background:var(--bg-subtle)}.review-thread.resolved{border-color:var(--muted);opacity:.8}.review-thread-head{display:flex;justify-content:space-between;gap:12px;text-transform:capitalize}.review-thread-head span{font-size:12px;color:var(--muted,#666)}.review-comment+.review-comment{border-top:1px solid var(--line,#ddd);margin-top:8px;padding-top:8px}.review-comment-meta{font-size:12px;color:var(--muted,#666)}.review-comment-body>:first-child{margin-top:2px}.review-comment-body>:last-child{margin-bottom:2px}.review-compose{margin-top:10px}.review-comment-form{margin-top:8px;max-width:680px}.review-comment-form button{margin-top:6px}
.review-coverage{border:1px solid var(--line,#ddd);border-radius:12px;padding:12px 16px;margin:24px 0}
.review-coverage.has-gap{border-color:#b45309}.review-coverage h2{margin:0 0 4px;font-size:17px}
.review-coverage .gap,.review-summary .gap{color:#b45309;font-weight:600}.review-gap figcaption{display:flex;flex-wrap:wrap;gap:4px 8px}
/* A pull-request review is the implementation-deck renderer with review data. */
.review-deck-shell:not(.code-mode){grid-template-columns:minmax(0,1fr)}
.review-deck-shell:not(.code-mode)>.sidebar{display:none}
.review-deck-shell.slide-mode{height:calc(100vh - var(--top));min-height:0;overflow:hidden;background:#111}
.review-deck-shell.slide-mode>.content{width:100%;height:100%;padding:0;overflow:hidden}
.review-deck-shell.slide-mode #view-saga{height:100%}
.slide-present[hidden]{display:none}
.review-deck-page{display:grid;grid-template-columns:210px minmax(0,1fr);width:100%;height:100%;min-height:0;background:#111;font:13px/1.55 var(--ui)}
.review-deck-rail{height:100%;min-height:0;overflow:auto;padding:8px 10px 32px;background:var(--bg-subtle);border-right:1px solid var(--line);counter-reset:slide-thumbnail}
.review-deck-rail>.sidebar-title{align-items:flex-start;margin-bottom:3px;font-size:12px;line-height:1.35}.review-deck-pr{margin:0 0 12px 22px;color:var(--faint);font-size:10.5px;overflow-wrap:anywhere}.review-deck-rail .slide-rail-deck>h2{margin:9px 0 7px 22px;color:var(--faint);font:600 9px/1.2 var(--ui);letter-spacing:.045em;text-transform:uppercase}
.review-thumbnail-list{margin:0;padding:0;list-style:none}.review-thumbnail-list .slide-thumbnail-card{min-width:0}
.slide-thumbnail-status{position:relative;z-index:3;display:grid;place-items:center;width:16px;height:16px;flex:none;border:1px solid var(--line);border-radius:50%;background:var(--bg);color:var(--faint);font:700 10px/1 var(--ui);pointer-events:none}.slide-thumbnail-status::before{content:'·'}.slide-thumbnail-status[data-review-state=approved]{border-color:var(--green);background:var(--approve-bg);color:var(--green)}.slide-thumbnail-status[data-review-state=approved]::before{content:'✓'}.slide-thumbnail-status[data-review-state=changes_requested]{border-color:var(--red);background:var(--reject-bg);color:var(--red)}.slide-thumbnail-status[data-review-state=changes_requested]::before{content:'!'}
.review-deck-viewer{min-width:0}.review-deck-viewer .deck-viewer-stage{width:min(100%,calc(177.7778vh - 78.2222px))}
.review-deck-slide .fragment-head::before{content:'Review slide'}
.review-deck-slide.out-of-date .fragment-head::before{content:'Review slide · decision out of date';color:#b45309}
.review-deck-slide .fragment-actions{display:flex;align-items:center;gap:2px}.review-deck-slide .fragment-actions>.permalink,.review-deck-slide .fragment-actions>.landmark-menu{opacity:.55}.review-deck-slide .fragment-actions:hover>.permalink,.review-deck-slide .fragment-actions:focus-within>.permalink,.review-deck-slide .fragment-actions:hover>.landmark-menu,.review-deck-slide .fragment-actions:focus-within>.landmark-menu{opacity:1}
.review-decision-quick{display:inline-flex}.review-decision{display:grid;place-items:center;width:25px;height:23px;padding:0;border:0;border-radius:5px}.review-decision .i{width:15px;height:15px}.review-decision.approve{background:var(--approve-bg);color:var(--approve-ink)}.review-decision.reject{background:var(--reject-bg);color:var(--reject-ink)}.review-decision.approve:hover{background:var(--approve-hover);color:var(--green)}.review-decision.reject:hover{background:var(--reject-hover);color:var(--red)}
.review-slide-menu,.review-slide-comment{position:relative}.review-slide-menu>summary,.review-slide-comment>summary{list-style:none}.review-slide-menu>summary::-webkit-details-marker,.review-slide-comment>summary::-webkit-details-marker{display:none}.review-slide-comment>summary{display:grid;place-items:center;width:25px;height:23px;padding:0;color:var(--faint)}
.review-slide-panel{position:absolute;z-index:30;right:0;top:calc(100% + 6px);width:min(430px,82vw);max-height:min(680px,calc(100vh - var(--top) - 100px));overflow:auto;padding:14px 16px;background:var(--bg);border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow);color:var(--ink);text-shadow:none}.review-slide-panel h2{margin:2px 0;font-size:18px}.review-slide-panel header>p:last-child{margin:4px 0 12px;color:var(--muted)}
.review-decision-list{list-style:none;margin:0 0 10px;padding:0}.review-decision-row{margin:6px 0}.review-decision-form{padding-top:10px;border-top:1px solid var(--line)}
.review-deck-slide .landmark-hotspot{border-color:transparent;background:transparent}
.review-item-affordance{gap:1px;padding:1px}.review-item-affordance .icon-button,.review-item-affordance .permalink{display:inline-flex;align-items:center;justify-content:center;min-width:24px;height:24px;padding:2px 4px;border:0;background:transparent;color:var(--muted);font:10px var(--mono)}.review-item-affordance .icon-button:hover,.review-item-affordance .permalink:hover{background:var(--bg-inset);color:var(--ink)}.review-item-affordance .diff-button span{margin-left:1px}
.review-item-panel{padding:16px}.review-item-panel h2{margin:2px 0}.review-item-panel header>p:last-child{color:var(--muted)}
.review-source{margin-top:12px}.review-source summary,.review-compose summary{cursor:pointer;color:var(--accent);font-weight:600}.review-source .review-range{margin:8px 0 0;padding:8px 10px;border:1px solid var(--line);background:var(--bg-subtle)}
.review-annotation-layer{position:absolute;z-index:6;inset:0;pointer-events:none}.review-annotation-layer.drawing{pointer-events:auto;cursor:crosshair;touch-action:none;background:#0969da08}.review-annotation{position:absolute;inset:0;pointer-events:none}.review-annotation-svg{position:absolute;inset:0;width:100%;height:100%;overflow:visible}.review-annotation-svg>*{pointer-events:visiblePainted;cursor:grab}.review-annotation-svg>*:active{cursor:grabbing}.review-annotation.selected .review-annotation-svg>*{filter:drop-shadow(0 0 4px #fff);stroke-dasharray:10 7}.review-sticky-note{position:absolute;width:18%;min-height:12%;padding:10px;border:1px solid #7a5b13;border-radius:3px;box-shadow:0 4px 12px #0003;color:#2b210b;text-align:left;transform:translate(-8%,-8%);pointer-events:auto;cursor:move}.review-annotation-bubble{position:absolute;z-index:16;pointer-events:auto;transform:translate(-30%,-70%)}.review-annotation-bubble>summary{display:inline-flex;align-items:center;justify-content:center;width:25px;height:21px;border:1px solid var(--line);border-radius:11px;background:var(--bg);color:var(--muted);box-shadow:var(--shadow);font:700 11px var(--mono);list-style:none;cursor:pointer}.review-annotation-bubble>summary::-webkit-details-marker{display:none}.review-annotation-discussion{position:absolute;right:0;top:26px;width:min(360px,80vw);max-height:360px;overflow:auto;padding:12px;background:var(--bg);border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow);color:var(--ink)}.review-annotation-message+.review-annotation-message{margin-top:8px;padding-top:8px;border-top:1px solid var(--line)}.review-annotation-message small{color:var(--muted)}.review-annotation-message p{margin:3px 0}.review-annotation-reply textarea{width:100%}
.review-annotation-draft{position:absolute;z-index:2;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none}.review-annotation-draft>*{stroke-dasharray:7 5}
.review-annotation-resize-handle{position:absolute;z-index:18;width:18px;height:18px;padding:0;border:2px solid #fff;border-radius:50%;background:var(--accent);box-shadow:var(--shadow);transform:translate(-50%,-50%);pointer-events:auto;cursor:nwse-resize}.review-annotation-resize-handle::after{content:'';display:block;width:5px;height:5px;margin:auto;border-right:1px solid #fff;border-bottom:1px solid #fff}
.annotation-tools-toggle{color:var(--faint)}.annotation-tools-toggle[aria-expanded=true]{background:var(--accent-soft);color:var(--accent)}
.review-annotation-toolbox{position:absolute;z-index:50;top:calc(100% + 5px);right:0;display:flex;align-items:center;gap:1px;max-width:min(620px,calc(100vw - 32px));padding:4px;background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:9px;box-shadow:var(--shadow)}.review-annotation-toolbox[hidden]{display:none}.review-annotation-toolbox button{display:grid;place-items:center;width:28px;height:28px;padding:0;border:0;border-radius:var(--radius);background:transparent;color:var(--muted)}.review-annotation-toolbox button:hover{background:var(--bg-inset);color:var(--ink)}.review-annotation-toolbox button[aria-pressed=true]{background:var(--accent-soft);color:var(--accent)}.review-annotation-toolbox button:disabled{opacity:.35;cursor:not-allowed}.review-annotation-toolbox input[type=color]{width:18px;height:18px;padding:0;border:0;background:transparent;cursor:pointer}.review-annotation-toolbox .tool-divider{width:1px;height:18px;margin:0 3px;background:var(--line)}.review-annotation-toolbox [data-selection]{display:flex;align-items:center;gap:1px;padding-left:3px;border-left:1px solid var(--line)}.review-annotation-toolbox [data-selection][hidden]{display:none}
.review-annotation-compose{position:fixed;z-index:70;right:18px;bottom:18px;width:min(440px,calc(100vw - 36px));padding:14px;border:1px solid var(--line);border-radius:9px;background:var(--bg);box-shadow:var(--shadow)}.review-annotation-compose[hidden]{display:none}.review-annotation-compose textarea{width:100%}.review-annotation-compose>div{display:flex;justify-content:flex-end;gap:6px;margin-top:6px}
.review-annotation-status{position:fixed;z-index:75;right:18px;bottom:18px;max-width:min(520px,calc(100vw - 24px));margin:0;padding:7px 11px;border:1px solid var(--line);border-radius:7px;background:var(--bg);box-shadow:var(--shadow);color:var(--ink);font:600 12px var(--ui)}.review-annotation-status.failed{border-color:#b45309}.review-annotation-status[hidden]{display:none}
body.presentation-mode .review-deck-page{grid-template-columns:minmax(0,1fr)}body.presentation-mode .review-deck-rail,body.presentation-mode .review-annotation-layer,body.presentation-mode .review-sticky-note,body.presentation-mode .review-annotation-bubble{display:none}
@media(max-width:780px){.review-deck-page{grid-template-columns:min(210px,42vw) minmax(0,1fr)}.review-deck-rail{padding:8px 6px 24px}.review-sticky-note{width:28%;min-width:88px;padding:5px;font-size:10px}.review-annotation-discussion{position:fixed;left:8px;right:8px;top:auto;bottom:58px;width:auto;max-height:45vh}.review-annotation-compose{right:8px;bottom:8px;width:calc(100vw - 16px)}.review-annotation-toolbox{right:0;overflow-x:auto;justify-content:flex-start;max-width:min(620px,calc(100vw - 8px))}.review-slide-panel{position:fixed;right:8px;top:calc(var(--top) + 8px);width:calc(100vw - 16px);max-height:calc(100vh - var(--top) - 16px)}}
`
