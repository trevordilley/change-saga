package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
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
}

type reviewSummaryView struct {
	Report reviewstate.Report
	Href   string
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

type reviewSlideView struct {
	Slide       *saga.Slide
	DOMID       string
	VisualURL   string
	Interactive bool
	Report      reviewstate.SlideReport
	OutOfDate   bool
	Items       []*reviewItemView
	Threads     []*reviewThreadView
}

type reviewItemView struct {
	Item        *saga.Item
	DOMID       string
	RecordHref  string
	RecordLabel string
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
	ID       string
	State    string
	Comments []reviewCommentView
}

type reviewCommentView struct {
	ID        string
	Author    string
	Body      template.HTML
	CreatedAt time.Time
	State     string
}

func reviewHref(id string) string { return "/reviews/" + id }

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
	reports := make([]reviewstate.Report, 0, len(reviews))
	for _, review := range reviews {
		reports = append(reports, reviewstate.Build(ctx, review, reviewstate.Options{Checkout: a.sourceDir, SagaRoot: document.Root, Resolver: resolver}))
	}
	return reports
}

// comparedHead is the head commit this reviewer was opened to compare, or ""
// when it observes.
func (a *app) comparedHead(ctx context.Context) string {
	if a.rng.Observe() {
		return ""
	}
	head, err := gitOutput(ctx, a.sourceDir, "rev-parse", "--verify", "--quiet", a.rng.HeadRevision()+"^{commit}")
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
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	if err := reviewTemplates.ExecuteTemplate(w, "review-index", view); err != nil {
		http.Error(w, "The reviews could not be rendered.", http.StatusInternalServerError)
	}
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
	threads := reviewstate.Threads(review.Comments)
	slideReports := map[string]reviewstate.SlideReport{}
	for _, slide := range report.Slides {
		slideReports[slide.ID] = slide
	}
	for _, slide := range review.Deck.Slides {
		slideView := &reviewSlideView{
			Slide: slide, DOMID: domID(slide.Target), VisualURL: reviewHref(review.ID) + "/visual/" + slide.ID,
			Interactive: slide.MediaType == "text/html", Report: slideReports[slide.ID],
		}
		for _, decision := range slideView.Report.Decisions {
			slideView.OutOfDate = slideView.OutOfDate || decision.Currency == reviewstate.OutOfDate
		}
		slideView.Threads = threadViewsFor(threads, slide.Target)
		for _, item := range slide.Items {
			itemView := &reviewItemView{Item: item, DOMID: domID(item.Target), Threads: threadViewsFor(threads, item.Target)}
			if item.Record != "" {
				itemView.RecordHref, itemView.RecordLabel = recordHref(document, item.Record), recordLabel(item.Record)
			}
			if report.Range != nil {
				for _, file := range item.Code {
					for _, reference := range file.References {
						itemView.Diffs = append(itemView.Diffs, a.referenceDiff(ctx, resolver, reference, *report.Range))
					}
				}
			}
			slideView.Items = append(slideView.Items, itemView)
		}
		view.Slides = append(view.Slides, slideView)
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	if err := reviewTemplates.ExecuteTemplate(w, "review-page", view); err != nil {
		http.Error(w, "The review could not be rendered.", http.StatusInternalServerError)
	}
}

func threadViewsFor(threads []*reviewstate.Thread, target string) []*reviewThreadView {
	var result []*reviewThreadView
	for _, thread := range threads {
		if thread.Root.Target != target {
			continue
		}
		view := &reviewThreadView{ID: thread.Root.ID, State: thread.State}
		for _, comment := range append([]saga.ReviewComment{thread.Root}, thread.Replies...) {
			view.Comments = append(view.Comments, reviewCommentView{ID: comment.ID, Author: reviewerSeat(comment.Reviewer), Body: markdown(comment.Body), CreatedAt: comment.CreatedAt, State: comment.State})
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

// referenceDiff is the diff of one Item's code reference against the
// review's base: the hunks between the merge-base and the head that touch the
// referenced lines as they are at the head.
func (a *app) referenceDiff(ctx context.Context, resolver *coderesolve.Resolver, reference coderef.Reference, rng reviewstate.Range) *reviewDiffView {
	view := &reviewDiffView{Path: reference.Path, Location: reference.Location().String()}
	start, end := reference.Start, reference.End
	path := reference.Path
	if resolver != nil {
		if resolution := resolver.Resolve(ctx, reference, rng.HeadOID); resolution.Current() {
			path, start, end = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
		} else if !reference.WholeFile() {
			view.Note = "The referenced lines changed after the reference was written; showing every change to the file."
			start, end = 0, 0
		}
	}
	// Pathspecs are relative to the working directory, and a Saga served from
	// inside its code repository has the Saga directory as its source dir.
	repo := a.sourceDir
	if top, err := gitOutput(ctx, a.sourceDir, "rev-parse", "--show-toplevel"); err == nil && top != "" {
		repo = top
	}
	patch, err := gitdiff.FileDiff(ctx, repo, rng.BaseOID, rng.HeadOID, path)
	if err != nil {
		view.Note = "The diff could not be read from this checkout."
		return view
	}
	view.Path = path
	view.Lines = diffLinesTouching(patch, start, end)
	if len(view.Lines) == 0 {
		if start > 0 {
			view.Note = fmt.Sprintf("Lines %d-%d are unchanged between the base and the head.", start, end)
		} else if view.Note == "" {
			view.Note = "The file is unchanged between the base and the head."
		}
	}
	return view
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
	w.Header().Set("Content-Security-Policy", "default-src 'self' data: blob:; script-src 'self' 'unsafe-inline' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
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
	_, err := reviewstore.Decide(a.root, reviewstore.Decision{
		Review: review.ID, Slide: slide, State: r.PostForm.Get("state"),
		Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: head, Body: r.PostForm.Get("body"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	comment, err := reviewstore.Comment(a.root, reviewstore.Remark{
		Review: review.ID, Target: r.PostForm.Get("target"), ReplyTo: r.PostForm.Get("reply_to"),
		Body: r.PostForm.Get("body"), State: r.PostForm.Get("state"),
		Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: head,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, reviewHref(review.ID)+"#"+domID(comment.Target), http.StatusSeeOther)
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
	"reviewer": func(decision reviewstate.DecisionReport) string {
		if decision.Reviewer.Kind == "ai" {
			return fmt.Sprintf("AI %s (%s, %s) for %s", decision.Reviewer.Name, decision.Reviewer.Agent, decision.Reviewer.Model, decision.Author)
		}
		return decision.Author
	},
}).Parse(reviewTemplateSource))

// reviewTemplateSource renders the review index and one review. Plain forms
// post decisions and comments, so the page works without script.
const reviewTemplateSource = `{{define "review-head"}}<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.}} · Change Saga</title><style>` + pageStyles + reviewStyles + `</style><script src="/theme.js"></script></head>{{end}}
{{define "review-summary"}}<article class="review-summary{{if .Matches}} matching{{end}}" data-review-summary="{{.Report.ID}}"><header><a href="{{.Href}}"><strong>{{.Report.Title}}</strong></a>{{with .Report.PullRequest}}{{if .Number}} <span class="review-pr">#{{.Number}}</span>{{end}}{{end}}{{if .Report.Merged}} <span class="review-badge merged">merged</span>{{else}} <span class="review-badge open">open</span>{{end}}</header>{{template "review-range" .Report}}<ol class="review-slide-states">{{range .Report.Slides}}<li data-review-slide-state="{{.ID}}"><span class="review-slide-title">{{.Title}}</span>{{range .Decisions}}<span class="review-decision-chip {{.State}}{{if eq .Currency "out_of_date"}} out-of-date{{end}}" data-decision-state="{{.State}}" data-currency="{{.Currency}}">{{reviewState .State}}{{if eq .Currency "out_of_date"}} · out of date{{end}}</span>{{else}}<span class="review-decision-chip none">no decision</span>{{end}}{{if .OpenThreads}}<span class="review-threads">{{.OpenThreads}} open {{if eq .OpenThreads 1}}thread{{else}}threads{{end}}</span>{{end}}</li>{{end}}</ol></article>{{end}}
{{define "review-range"}}<p class="review-range">{{with .Range}}{{if .Frozen}}Frozen at <code>{{short .BaseOID}}</code>..<code>{{short .HeadOID}}</code>{{else}}<code>{{short .BaseOID}}</code>..<code>{{short .HeadOID}}</code> · head follows <code>{{.Following}}</code>{{end}}{{end}}{{with .Merged}} · landed as <code>{{short .Landed}}</code>{{end}}{{range .Diagnostics}}<span class="review-diagnostic">{{.}}</span>{{end}}</p>{{end}}
{{define "review-index"}}{{template "review-head" "Reviews"}}<body class="review-surface"><header class="review-top"><a href="/">← {{.Saga.Manifest.Title}}</a><h1>Reviews</h1><p>Each pull request has one review: a slide deck explaining what the change did and why. Approvals and comments happen only here, per slide. The Saga itself is documentation.</p></header><main class="review-main">{{range .Reviews}}{{template "review-summary" .}}{{else}}<p class="review-empty">No reviews yet. Create one for a pull request with <code>change-saga review create</code>.</p>{{end}}</main></body></html>{{end}}
{{define "review-diff"}}<figure class="review-diff" data-review-diff="{{.Location}}"><figcaption><code>{{.Path}}</code> <span class="review-location">{{.Location}}</span></figcaption>{{if .Note}}<p class="review-note">{{.Note}}</p>{{end}}{{if .Lines}}<table><tbody>{{range .Lines}}<tr class="review-line {{.Kind}}">{{if eq .Kind "hunk"}}<td colspan="3" class="review-hunk">{{.Text}}</td>{{else}}<td class="review-lineno">{{.Old}}</td><td class="review-lineno">{{.New}}</td><td class="review-code"><code>{{if eq .Kind "add"}}+{{else if eq .Kind "del"}}-{{else}} {{end}}{{.Text}}</code></td>{{end}}</tr>{{end}}</tbody></table>{{end}}</figure>{{end}}
{{define "review-threads"}}{{range .}}<article class="review-thread {{.State}}" id="thread-{{.ID}}" data-review-thread="{{.ID}}" data-thread-state="{{.State}}">{{range .Comments}}<div class="review-comment" id="comment-{{.ID}}"><div class="review-comment-meta">{{.Author}} · <time datetime="{{.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}">{{.CreatedAt.Format "2006-01-02 15:04 MST"}}</time>{{if .State}} · {{.State}}{{end}}</div><div class="review-comment-body">{{.Body}}</div></div>{{end}}</article>{{end}}{{end}}
{{define "review-comment-form"}}{{if not .Frozen}}<form class="review-comment-form" method="post" action="/reviews/{{.ReviewID}}/comment" data-review-comment-form="{{.Target}}"><input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="target" value="{{.Target}}"><label><span>Comment on {{.Label}}</span><textarea name="body" required rows="2"></textarea></label><button type="submit">Comment</button></form>{{end}}{{end}}
{{define "review-page"}}{{template "review-head" .Review.Title}}<body class="review-surface" data-review="{{.Review.ID}}"><header class="review-top"><a href="/reviews">← Reviews</a> · <a href="/">{{.Saga.Manifest.Title}}</a>{{if .Comparing}} · <a href="/?view=change">Changed, Affected, and Code (read-only)</a>{{end}}<h1>{{.Review.Title}}</h1>{{with .Review.PullRequest}}<p class="review-pr">{{if .URL}}<a href="{{.URL}}">{{if .Number}}Pull request #{{.Number}}{{else}}{{.URL}}{{end}}</a>{{else}}Pull request #{{.Number}}{{end}}</p>{{end}}{{template "review-range" .Report}}<p class="review-rule">{{if .Frozen}}This review is history: its change has landed. It is shown exactly as it was reviewed.{{else}}Decide slide by slide. A decision records the head it was given at and goes out of date when the slide or the code it references changes. The tool records decisions; your team decides what it requires.{{end}}</p></header>
<main class="review-main">{{$page := .}}{{range .Slides}}<section class="review-slide{{if .OutOfDate}} out-of-date{{end}}" id="{{.DOMID}}" data-review-slide="{{.Slide.ID}}"><header class="review-slide-head"><h2>{{.Slide.Title}}</h2><p class="review-takeaway">{{.Slide.Takeaway}}</p></header><div class="review-slide-body"><div class="review-visual">{{if .Interactive}}<iframe sandbox="allow-scripts" src="{{.VisualURL}}" title="{{.Slide.Title}}"></iframe>{{else}}<img src="{{.VisualURL}}" alt="{{.Slide.Title}}">{{end}}</div>
<aside class="review-decisions" aria-label="Decisions on {{.Slide.Title}}"><h3>Decisions</h3><ul>{{range .Report.Decisions}}<li class="review-decision-row {{.State}}{{if eq .Currency "out_of_date"}} out-of-date{{end}}" data-decision-state="{{.State}}" data-currency="{{.Currency}}"><strong>{{reviewState .State}}</strong> by {{reviewer .}} at <code>{{short .Commit}}</code>{{if eq .Currency "out_of_date"}} <span class="review-out-of-date" data-out-of-date>Out of date</span>{{else if eq .Currency "unknown"}} <span class="review-unknown">currency unknown</span>{{end}}{{if .Reasons}}<ul class="review-reasons">{{range .Reasons}}<li>{{.}}</li>{{end}}</ul>{{end}}{{if .Body}}<p class="review-body">{{.Body}}</p>{{end}}</li>{{else}}<li class="review-decision-row none">No decision yet</li>{{end}}</ul>{{if not $page.Frozen}}<form class="review-decision-form" method="post" action="/reviews/{{$page.Review.ID}}/decision" data-review-decision-form="{{.Slide.ID}}"><input type="hidden" name="token" value="{{$page.MutationToken}}"><input type="hidden" name="slide" value="{{.Slide.ID}}"><label><span>Note</span><textarea name="body" rows="2" placeholder="Required when requesting changes"></textarea></label><div class="review-decision-buttons"><button type="submit" name="state" value="approved" data-review-approve>Approve slide</button><button type="submit" name="state" value="changes_requested" data-review-request-changes>Request changes</button><button type="submit" name="state" value="none" data-review-withdraw>Withdraw</button></div></form>{{end}}</aside></div>
<div class="review-items">{{range .Items}}<article class="review-item" id="{{.DOMID}}" data-review-item="{{.Item.ID}}"><h3>{{.Item.Label}}</h3><p>{{.Item.Description}}</p>{{if .RecordHref}}<p class="review-record">Documentation: <a href="{{.RecordHref}}" data-review-record="{{.Item.Record}}">{{.RecordLabel}}</a></p>{{end}}{{range .Diffs}}{{template "review-diff" .}}{{end}}{{template "review-threads" .Threads}}{{template "review-comment-form" (reviewCommentForm $page .Item.Target .Item.Label)}}</article>{{end}}</div>
<div class="review-slide-threads">{{template "review-threads" .Threads}}{{template "review-comment-form" (reviewCommentForm $page .Slide.Target .Slide.Title)}}</div></section>{{end}}</main></body></html>{{end}}`

// reviewCommentFormView carries what a comment form needs from its page.
type reviewCommentFormView struct {
	ReviewID, Token, Target, Label string
	Frozen                         bool
}

const reviewStyles = `
.review-surface{max-width:1180px;margin:0 auto;padding:24px;font:15px/1.5 system-ui,sans-serif}
.review-top h1{margin:8px 0 4px}.review-range code,.review-decisions code{font-size:12px}
.review-rule{color:var(--muted,#555)}.review-diagnostic{display:block;color:#a15c00}
.review-summary{border:1px solid var(--line,#ddd);border-radius:10px;padding:12px 16px;margin:12px 0}
.review-summary.matching{border-color:#2f6fdc}.review-badge{font-size:12px;padding:1px 8px;border-radius:9px;background:#eee;color:#333}
.review-slide-states{list-style:none;padding:0}.review-slide-states li{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin:4px 0}
.review-decision-chip{font-size:12px;padding:1px 8px;border-radius:9px;background:#eef}.review-decision-chip.approved{background:#dcf5e3;color:#14532d}
.review-decision-chip.changes_requested{background:#fde2e1;color:#7f1d1d}.review-decision-chip.out-of-date{outline:2px dashed #b45309}
.review-slide{border:1px solid var(--line,#ddd);border-radius:12px;padding:16px;margin:24px 0}
.review-slide.out-of-date{border-color:#b45309}
.review-slide-body{display:grid;grid-template-columns:minmax(0,2fr) minmax(260px,1fr);gap:16px}
.review-visual img,.review-visual iframe{width:100%;border:1px solid var(--line,#ddd);border-radius:8px;background:#fff;aspect-ratio:16/9}
.review-decisions ul{list-style:none;padding:0;margin:0}.review-decision-row{margin:6px 0}
.review-out-of-date{font-size:12px;font-weight:700;color:#fff;background:#b45309;border-radius:9px;padding:1px 8px}
.review-reasons{font-size:13px;color:#7c2d12}
.review-decision-form textarea,.review-comment-form textarea{width:100%;box-sizing:border-box}
.review-decision-buttons{display:flex;gap:6px;flex-wrap:wrap;margin-top:6px}
.review-item{border-top:1px solid var(--line,#eee);padding-top:10px;margin-top:10px}
.review-diff{margin:8px 0}.review-diff table{border-collapse:collapse;width:100%;font:12px/1.4 ui-monospace,monospace}
.review-line.add{background:#e6ffed}.review-line.del{background:#ffeef0}.review-hunk{color:#57606a;background:#f6f8fa}
.review-lineno{width:3em;text-align:right;color:#8b949e;padding-right:6px}.review-code code{white-space:pre}
.review-thread{border-left:3px solid #2f6fdc;padding-left:8px;margin:8px 0}.review-thread.resolved{border-color:#999;opacity:.8}
.review-comment-meta{font-size:12px;color:#666}
`
