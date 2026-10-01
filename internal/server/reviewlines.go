package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A reviewer comments on a line of any diff inside a review, as on a pull
// request. The diffs themselves are cached page by page and shared by every
// reader, so a diff never carries its discussion: the threads of an Item's
// drawer, or of one file of the Code Diff, are read separately and placed
// under their lines as each page of the diff arrives.

// reviewLineThreadsView is the code-line threads of one Item or one file,
// with the range they were placed in and a composer for new ones.
type reviewLineThreadsView struct {
	ReviewID, Token string
	// Target is the Item a new comment from this drawer is filed under; it
	// is empty in the Code Diff, where the server picks the Item.
	Target     string
	Head, Base string
	Frozen     bool
	Threads    []*reviewLineThreadView
}

// reviewLineThreadView is one code-line thread where it shows now.
type reviewLineThreadView struct {
	*reviewThreadView
	Line reviewstate.LineThread
	// Original is the code the thread was made on, shown once its lines
	// changed.
	Original []referenceCodeLine
	// OriginalMore counts the lines of a long range left out of Original.
	OriginalMore int
	Head, Base   string
}

// referenceCodeLine is one numbered line of code.
type referenceCodeLine struct {
	Number int
	Text   string
}

// Outdated reports whether the thread's lines changed since it was made.
func (view *reviewLineThreadView) Outdated() bool {
	return view.Line.Currency != reviewstate.Current
}

// LineLabel names the lines where the thread shows.
func (view *reviewLineThreadView) LineLabel() string {
	if view.Line.End != view.Line.Start {
		return fmt.Sprintf("lines %d–%d", view.Line.Start, view.Line.End)
	}
	return fmt.Sprintf("line %d", view.Line.Start)
}

// reviewLineOriginalCap bounds the code an outdated thread quotes.
const reviewLineOriginalCap = 20

// reviewLineThreads renders the code-line threads of one review Item
// (?target=) or of one repository path (?path=).
func (a *app) reviewLineThreads(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	review := document.FindReview(r.PathValue("id"))
	if review == nil {
		http.NotFound(w, r)
		return
	}
	target, path := r.URL.Query().Get("target"), r.URL.Query().Get("path")
	if (target == "") == (path == "") {
		http.Error(w, "name one Item target or one path", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	view := reviewLineThreadsView{ReviewID: review.ID, Token: a.mutationToken, Target: target, Frozen: review.Merged != nil}
	rng, resolver := a.reviewLineRange(ctx, review)
	if resolver != nil {
		defer resolver.Close()
	}
	if rng != nil {
		view.Head, view.Base = rng.HeadOID, rng.BaseOID
	}
	if view.Head == "" || r.URL.Query().Get("head") != view.Head || r.URL.Query().Get("base") != view.Base {
		http.Error(w, errReviewLinesChanged.Error(), http.StatusConflict)
		return
	}
	for _, thread := range reviewstate.Threads(review.Comments) {
		line := thread.Root.CodeLine
		if line == nil || (target != "" && thread.Root.Target != target) {
			continue
		}
		lineView := a.lineThreadView(ctx, review, thread, rng, resolver)
		if path != "" && line.Path != path && lineView.Line.Path != path {
			continue
		}
		view.Threads = append(view.Threads, lineView)
	}
	var body bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&body, "review-line-threads", view); err != nil {
		http.Error(w, "The line comments could not be rendered.", http.StatusInternalServerError)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	_, _ = w.Write(body.Bytes())
}

// reviewLineRange is the review's current range and a resolver for it, or
// nil for either that cannot be read. The caller closes the resolver.
func (a *app) reviewLineRange(ctx context.Context, review *saga.Review) (*reviewstate.Range, *coderesolve.Resolver) {
	rng, err := reviewstate.ResolveRange(ctx, a.sourceDir, review)
	if err != nil {
		return nil, nil
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		return &rng, nil
	}
	return &rng, resolver
}

func (a *app) lineThreadView(ctx context.Context, review *saga.Review, thread *reviewstate.Thread, rng *reviewstate.Range, resolver *coderesolve.Resolver) *reviewLineThreadView {
	line := reviewstate.ViewCodeLine(ctx, resolver, rng, *thread.Root.CodeLine)
	line.ID, line.Target, line.State = thread.Root.ID, thread.Root.Target, thread.State
	view := &reviewLineThreadView{reviewThreadView: threadView(thread, review.ID, a.mutationToken, review.Merged != nil), Line: line}
	if rng != nil {
		view.Head, view.Base = rng.HeadOID, rng.BaseOID
	}
	if view.Outdated() && resolver != nil {
		anchor := thread.Root.CodeLine
		if content, found, err := resolver.Blob(ctx, anchor.Commit, anchor.Path); err == nil && found {
			lines := coderef.Lines(content)
			for number := anchor.Start; number <= anchor.End && number <= len(lines); number++ {
				if len(view.Original) == reviewLineOriginalCap {
					view.OriginalMore = anchor.End - number + 1
					break
				}
				view.Original = append(view.Original, referenceCodeLine{Number: number, Text: strings.TrimRight(string(lines[number-1]), "\r\n")})
			}
		}
	}
	return view
}

// reviewLineComment records a comment on code lines, or a reply that
// resolves or reopens a code-line thread. The reviewer commented on the diff
// they were shown, so the range they saw must still be the review's range.
func (a *app) reviewLineComment(w http.ResponseWriter, r *http.Request, review *saga.Review) {
	ctx := r.Context()
	rng, err := reviewstate.ResolveRange(ctx, a.sourceDir, review)
	if err != nil {
		http.Error(w, "The review's head could not be read: "+err.Error(), http.StatusConflict)
		return
	}
	if r.PostForm.Get("head") != rng.HeadOID || r.PostForm.Get("base") != rng.BaseOID {
		http.Error(w, errReviewLinesChanged.Error(), http.StatusConflict)
		return
	}
	remark := reviewstore.Remark{
		Review: review.ID, ReplyTo: r.PostForm.Get("reply_to"), Body: r.PostForm.Get("body"), State: r.PostForm.Get("state"),
		Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: rng.HeadOID,
		CheckSnapshot: func(current *saga.Review, _ string) error {
			if now, err := reviewstate.ResolveRange(ctx, a.sourceDir, current); err != nil || current.Merged != nil || now.HeadOID != rng.HeadOID || now.BaseOID != rng.BaseOID {
				return errReviewLinesChanged
			}
			return nil
		},
	}
	if remark.ReplyTo != "" && strings.TrimSpace(remark.Body) == "" {
		// Resolving or reopening needs no words.
		switch remark.State {
		case saga.CommentResolved:
			remark.Body = "Resolved."
		case saga.CommentOpen:
			remark.Body = "Reopened."
		}
	}
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		http.Error(w, "The review's repository could not be read.", http.StatusConflict)
		return
	}
	defer resolver.Close()
	if remark.ReplyTo == "" {
		start, _ := strconv.Atoi(r.PostForm.Get("line"))
		end, _ := strconv.Atoi(r.PostForm.Get("end_line"))
		line, err := reviewstate.AuthorCodeLine(ctx, resolver, rng, r.PostForm.Get("path"), r.PostForm.Get("side"), start, end)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		remark.CodeLine, remark.Target = &line, r.PostForm.Get("target")
		if remark.Target == "" {
			remark.Target = reviewstate.LineTarget(ctx, resolver, review, line)
		}
	}
	comment, err := reviewstore.Comment(a.root, remark)
	if err != nil {
		if err == errReviewLinesChanged {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		reviewWriteError(w, err)
		return
	}
	root := comment.ID
	if !asyncReviewRequest(r) {
		for _, thread := range reviewstate.Threads(append(append([]saga.ReviewComment{}, review.Comments...), comment)) {
			for _, reply := range thread.Replies {
				if reply.ID == comment.ID {
					root = thread.Root.ID
				}
			}
		}
		http.Redirect(w, r, reviewHref(review.ID)+"#line-thread-"+root, http.StatusSeeOther)
		return
	}
	response := struct {
		Saved   bool           `json:"saved"`
		EventID string         `json:"event_id"`
		Target  string         `json:"target"`
		Thread  string         `json:"thread,omitempty"`
		Counts  map[string]int `json:"counts,omitempty"`
		Warning string         `json:"warning,omitempty"`
	}{Saved: true, EventID: comment.ID, Target: comment.Target}
	if thread, counts, err := a.savedLineThread(ctx, review.ID, comment.ID, &rng, resolver); err != nil {
		response.Warning = "Saved. Its display could not be refreshed; reload to see it."
	} else {
		response.Thread, response.Counts = thread, counts
	}
	writeIncrementalHeaders(w, "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

var errReviewLinesChanged = fmt.Errorf("The review's diff changed since it was shown. Reload to comment on its current lines. Your draft is retained.")

// savedLineThread renders the thread that holds comment as it is now, and
// the open code-line threads of its Item and slide.
func (a *app) savedLineThread(ctx context.Context, reviewID, commentID string, rng *reviewstate.Range, resolver *coderesolve.Resolver) (string, map[string]int, error) {
	document, validation, err := saga.Load(a.root)
	if err != nil || !validation.Valid {
		return "", nil, fmt.Errorf("the saved comment could not be loaded")
	}
	review := document.FindReview(reviewID)
	if review == nil {
		return "", nil, fmt.Errorf("the review is unavailable")
	}
	threads := reviewstate.Threads(review.Comments)
	for _, thread := range threads {
		holds := thread.Root.ID == commentID
		for _, reply := range thread.Replies {
			holds = holds || reply.ID == commentID
		}
		if !holds || thread.Root.CodeLine == nil {
			continue
		}
		var body bytes.Buffer
		if err := reviewTemplates.ExecuteTemplate(&body, "review-line-thread", a.lineThreadView(ctx, review, thread, rng, resolver)); err != nil {
			return "", nil, err
		}
		return body.String(), openLineThreadCounts(review, threads, thread.Root.Target), nil
	}
	return "", nil, fmt.Errorf("the saved comment is not on code lines")
}

// openLineThreadCounts is how many open code-line threads target holds, and
// its slide with the slide's Items, keyed by target.
func openLineThreadCounts(review *saga.Review, threads []*reviewstate.Thread, target string) map[string]int {
	counts := map[string]int{target: 0}
	slide := reviewTargetSlide(review, target)
	if slide != nil {
		counts[slide.Target] = 0
	}
	for _, thread := range threads {
		if thread.Root.CodeLine == nil || thread.State != saga.CommentOpen {
			continue
		}
		if thread.Root.Target == target {
			counts[target]++
		}
		if slide != nil && target != slide.Target && (thread.Root.Target == slide.Target || strings.HasPrefix(thread.Root.Target, slide.Target+":item:")) {
			counts[slide.Target]++
		}
	}
	return counts
}

// openLineThreads counts the open code-line threads of each target.
func openLineThreads(threads []*reviewstate.Thread) map[string]int {
	counts := map[string]int{}
	for _, thread := range threads {
		if thread.Root.CodeLine != nil && thread.State == saga.CommentOpen {
			counts[thread.Root.Target]++
		}
	}
	return counts
}

// reviewLineTemplates render code-line threads and their composer. Each
// thread says where it was made and, once outdated, quotes the code it was
// made on. The composer is a template the page's script fills in with the
// line a reviewer chose.
const reviewLineTemplates = `
{{define "review-line-threads"}}<section class="review-line-threads" data-review-line-threads data-review-head="{{.Head}}" data-review-base="{{.Base}}"{{if .Frozen}} data-review-frozen{{end}} aria-label="Line comments"><h3 class="review-line-threads-heading" data-review-line-unplaced-heading hidden>Comments on lines not shown here</h3><div data-review-line-unplaced>{{range .Threads}}{{template "review-line-thread" .}}{{end}}</div>{{if and (not .Frozen) .Head}}<template data-review-line-composer><form hx-boost="false" class="review-line-comment-form" method="post" action="/reviews/{{.ReviewID}}/comment" data-review-line-composer-form><input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="head" value="{{.Head}}"><input type="hidden" name="base" value="{{.Base}}"><input type="hidden" name="target" value="{{.Target}}"><input type="hidden" name="path" value=""><input type="hidden" name="side" value=""><input type="hidden" name="line" value=""><input type="hidden" name="end_line" value=""><label><span data-review-line-composer-label>Comment on this line</span><textarea name="body" required rows="3" placeholder="Leave a comment. Markdown is supported."></textarea></label><p class="review-line-form-status" role="status" hidden></p><div class="review-line-form-buttons"><button type="submit">Comment</button><button type="button" data-review-line-cancel>Cancel</button></div></form></template>{{end}}</section>{{end}}
{{define "review-line-thread"}}<article class="review-thread review-line-thread {{.State}}{{if .Outdated}} outdated{{end}}" id="line-thread-{{.ID}}" tabindex="-1" data-review-line-thread="{{.ID}}" data-review-target="{{.Target}}" data-thread-state="{{.State}}" data-line-path="{{.Line.Path}}" data-line-side="{{.Line.CodeLine.Side}}" data-line-start="{{.Line.Start}}" data-line-end="{{.Line.End}}" data-currency="{{.Line.Currency}}" aria-label="Comment thread on {{.LineLabel}} of {{.Line.Path}}"><header class="review-thread-head"><strong>{{.LineLabel}}{{if eq .Line.CodeLine.Side "old"}} (deleted side){{end}}</strong><span class="review-line-thread-state">{{if eq .Line.Currency "outdated"}}<span class="review-line-outdated" data-line-outdated>Outdated</span> · {{else if eq .Line.Currency "unknown"}}<span class="review-line-outdated">Currency unknown</span> · {{end}}{{.State}}</span></header>{{if .Line.Moved}}<p class="review-note review-line-moved">{{.Line.Reason}}; it was made on <code>{{.Line.Location}}</code>.</p>{{end}}{{if .Outdated}}<div class="review-line-original" data-line-original><p class="review-note">{{if eq .Line.Currency "outdated"}}The code changed since this was written{{else}}This comment's code cannot be compared here{{end}}{{with .Line.Reason}}: {{.}}{{end}}. It was made on <code>{{.Line.Location}}</code>:</p>{{if .Original}}<table><tbody>{{range .Original}}<tr><td class="review-lineno">{{.Number}}</td><td class="review-code"><code>{{.Text}}</code></td></tr>{{end}}</tbody></table>{{end}}{{if .OriginalMore}}<p class="review-note">{{.OriginalMore}} more lines.</p>{{end}}</div>{{end}}{{range .Comments}}<div class="review-comment" id="comment-{{.ID}}"><div class="review-comment-meta">{{.Author}} · <time datetime="{{.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}">{{.CreatedAt.Format "2006-01-02 15:04 MST"}}</time>{{if .State}} · {{.State}}{{end}}</div><div class="review-comment-body">{{.Body}}</div></div>{{end}}{{if and (not .Frozen) .Head}}<form hx-boost="false" class="review-line-comment-form review-line-reply" method="post" action="/reviews/{{.ReviewID}}/comment" data-review-line-reply-form="{{.ID}}"><input type="hidden" name="token" value="{{.Token}}"><input type="hidden" name="head" value="{{.Head}}"><input type="hidden" name="base" value="{{.Base}}"><input type="hidden" name="reply_to" value="{{.ID}}"><label><span>Reply on {{.LineLabel}}</span><textarea name="body" rows="2" placeholder="Reply. Markdown is supported."></textarea></label><p class="review-line-form-status" role="status" hidden></p><div class="review-line-form-buttons"><button type="submit" data-review-line-reply>Reply</button>{{if eq .State "resolved"}}<button type="submit" name="state" value="open" data-review-line-reopen>Reopen</button>{{else}}<button type="submit" name="state" value="resolved" data-review-line-resolve>Resolve</button>{{end}}</div></form>{{end}}</article>{{end}}
`
