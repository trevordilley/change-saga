package cli

import (
	"context"
	"errors"
	"flag"
	"io"
	"time"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// reviewThreadsResult is "query review-threads": one review's discussion,
// thread by thread, so a reviewer agent reads what was said on each slide,
// Item, and code line without reading the review's files.
type reviewThreadsResult struct {
	Review string             `json:"review"`
	Range  *reviewstate.Range `json:"range,omitempty"`
	// Diagnostics say why placements are unknown, such as a head this
	// checkout does not have.
	Diagnostics []string            `json:"diagnostics"`
	Threads     []reviewThreadEntry `json:"threads"`
}

type reviewThreadEntry struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	State  string `json:"state"`
	// CodeLine is the lines the thread was made on, and Placement where they
	// show in the review's current range and whether they changed since.
	CodeLine  *saga.ReviewCodeLine      `json:"code_line,omitempty"`
	Placement *reviewThreadPlacement    `json:"placement,omitempty"`
	Comments  []reviewThreadCommentView `json:"comments"`
}

type reviewThreadPlacement struct {
	Currency string `json:"currency"`
	Location string `json:"location"`
	Path     string `json:"path"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Moved    bool   `json:"moved,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type reviewThreadCommentView struct {
	ID        string                `json:"id"`
	ReplyTo   string                `json:"reply_to,omitempty"`
	Body      string                `json:"body"`
	State     string                `json:"state,omitempty"`
	Reviewer  saga.ReviewerIdentity `json:"reviewer"`
	Commit    string                `json:"commit,omitempty"`
	CreatedAt time.Time             `json:"created_at"`
}

func queryReviewThreads(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query review-threads", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sagaRoot := flags.String("saga", "", "saga root")
	sourceDir := flags.String("repo", "", "source repository checkout")
	reviewID := flags.String("review", "", "review id")
	path := flags.String("path", "", "only threads on code lines of this repository path")
	state := flags.String("state", "", "only open or only resolved threads")
	lines := flags.Bool("lines", false, "only threads on code lines")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return writeQuerySuccess(out, "", queryHelpFor("review-threads"), nil)
		}
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *sagaRoot == "" || *reviewID == "" || flags.NArg() != 0 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "usage: " + queryUsage["review-threads"]})
	}
	if *state != "" && *state != saga.CommentOpen && *state != saga.CommentResolved {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "--state must be open or resolved"})
	}
	document, validation, err := saga.Load(*sagaRoot)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if !validation.Valid {
		return writeQueryFailure(out, &queryError{Code: "invalid_saga", Message: "the Saga is invalid; run change-saga validate"})
	}
	review := document.FindReview(*reviewID)
	if review == nil {
		return writeQueryFailure(out, &queryError{Code: "not_found", Message: "review " + *reviewID + " does not exist", Details: map[string]any{"kind": "review", "selector": *reviewID}})
	}
	result := reviewThreadsResult{Review: review.Target, Diagnostics: []string{}, Threads: []reviewThreadEntry{}}
	checkout := firstNonEmpty(*sourceDir, document.Root)
	if rng, err := reviewstate.ResolveRange(ctx, checkout, review); err != nil {
		result.Diagnostics = append(result.Diagnostics, err.Error())
	} else {
		result.Range = &rng
	}
	var resolver *coderesolve.Resolver
	if result.Range != nil {
		if resolver, err = coderesolve.New(ctx, checkout); err == nil {
			defer resolver.Close()
		} else {
			resolver = nil
			result.Diagnostics = append(result.Diagnostics, err.Error())
		}
	}
	for _, thread := range reviewstate.Threads(review.Comments) {
		root := thread.Root
		if (*state != "" && thread.State != *state) || ((*lines || *path != "") && root.CodeLine == nil) {
			continue
		}
		entry := reviewThreadEntry{ID: root.ID, Target: root.Target, State: thread.State, CodeLine: root.CodeLine, Comments: []reviewThreadCommentView{}}
		if root.CodeLine != nil {
			view := reviewstate.ViewCodeLine(ctx, resolver, result.Range, *root.CodeLine)
			if *path != "" && root.CodeLine.Path != *path && view.Path != *path {
				continue
			}
			entry.Placement = &reviewThreadPlacement{Currency: view.Currency, Location: view.Location, Path: view.Path, Start: view.Start, End: view.End, Moved: view.Moved, Reason: view.Reason}
		}
		for _, comment := range append([]saga.ReviewComment{root}, thread.Replies...) {
			entry.Comments = append(entry.Comments, reviewThreadCommentView{ID: comment.ID, ReplyTo: comment.ReplyTo, Body: comment.Body, State: comment.State, Reviewer: comment.Reviewer, Commit: comment.Commit, CreatedAt: comment.CreatedAt})
		}
		result.Threads = append(result.Threads, entry)
	}
	// Threads read the Saga's own records and the review's range, not a
	// comparison.
	snapshot, err := reviewapp.Snapshot(ctx, *sagaRoot, gitdiff.ChangeSet{})
	if err != nil {
		return writeQueryFailure(out, &queryError{Code: "internal", Message: err.Error()})
	}
	return writeQuerySuccess(out, snapshot, result, nil)
}
