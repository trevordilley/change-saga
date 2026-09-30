package reviewstate

import (
	"context"
	"fmt"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A thread on code lines is current while its lines are unchanged in the
// review's current range, and outdated once they change: it then stays at
// the line it was made on, with the code it was made on, as a pull request's
// outdated comment does. Unknown means its commit is not in this checkout.
const Outdated = "outdated"

// AuthorCodeLine anchors lines start..end of path on side of the review's
// range: the head's lines for the new side and the merge-base's for the old.
// The digest is read from the repository, so the lines must exist there.
func AuthorCodeLine(ctx context.Context, resolver *coderesolve.Resolver, rng Range, path, side string, start, end int) (saga.ReviewCodeLine, error) {
	if side == "" {
		side = saga.CodeLineNew
	}
	if end == 0 {
		end = start
	}
	commit, where := rng.HeadOID, "the head"
	if side == saga.CodeLineOld {
		commit, where = rng.BaseOID, "the merge-base"
	}
	line := saga.ReviewCodeLine{Commit: commit, Path: path, Side: side, Start: start, End: end, Digest: coderef.DigestPrefix + strings.Repeat("0", 64)}
	if err := saga.ValidateReviewCodeLine(line); err != nil {
		return saga.ReviewCodeLine{}, err
	}
	if resolver == nil {
		return saga.ReviewCodeLine{}, fmt.Errorf("the review's repository cannot be read, so its lines cannot be anchored")
	}
	reference, err := resolver.Author(ctx, line.Reference().Location(), "")
	if err != nil {
		return saga.ReviewCodeLine{}, fmt.Errorf("lines %d-%d of %s cannot be commented on at %s (%s): %w", start, end, path, where, short(commit), err)
	}
	line.Digest = reference.Digest
	return line, nil
}

// LineTarget is what a code-line comment made without a target is filed
// under: the first Item, in deck order, whose code reference holds the lines
// at their side, so the comment joins the explanation of that code. Lines no
// Item explains are commented on the review itself.
func LineTarget(ctx context.Context, resolver *coderesolve.Resolver, review *saga.Review, line saga.ReviewCodeLine) string {
	if resolver == nil || review.Deck == nil {
		return review.Target
	}
	want := coderef.Location{Commit: line.Commit, Path: line.Path, Start: line.Start, End: line.End}
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for _, file := range item.Code {
				for _, reference := range file.References {
					if reference.Path != line.Path {
						continue
					}
					if resolution := resolver.Resolve(ctx, reference, line.Commit); resolution.Current() && resolution.Location.Contains(want) {
						return item.Target
					}
				}
			}
		}
	}
	return review.Target
}

// LineThread is one thread on code lines, viewed at the review's current
// range. Path, Start, and End are where it shows: where its lines are now
// when current, and the lines it was made on otherwise.
type LineThread struct {
	ID       string              `json:"id"`
	Target   string              `json:"target"`
	State    string              `json:"state"`
	CodeLine saga.ReviewCodeLine `json:"code_line"`
	// Location is the anchor's canonical <commit>:<path>#L<start>[-L<end>].
	Location string `json:"location"`
	Currency string `json:"currency"`
	Path     string `json:"path"`
	Start    int    `json:"start"`
	End      int    `json:"end"`
	// Moved marks current lines that now sit elsewhere, because lines were
	// added or removed above them; the thread follows them and says so.
	Moved    bool   `json:"moved,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Comments int    `json:"comments"`
	// Summary is the first line of the thread's first comment.
	Summary string `json:"summary"`
}

// ViewCodeLine places line in rng, the review's current range. rng is nil
// when the range cannot be read, and resolver when the repository cannot.
func ViewCodeLine(ctx context.Context, resolver *coderesolve.Resolver, rng *Range, line saga.ReviewCodeLine) LineThread {
	view := LineThread{CodeLine: line, Location: line.Reference().Location().String(), Currency: Unknown, Path: line.Path, Start: line.Start, End: line.End}
	if rng == nil || resolver == nil {
		view.Reason = "the review's range cannot be read here"
		return view
	}
	at := rng.HeadOID
	if line.Side == saga.CodeLineOld {
		at = rng.BaseOID
	}
	resolution := resolver.Resolve(ctx, line.Reference(), at)
	switch {
	case resolution.Current():
		view.Currency, view.Moved = Current, resolution.Moved
		view.Path, view.Start, view.End = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
		if resolution.Moved && resolution.Location.Start != line.Start {
			view.Reason = fmt.Sprintf("the lines moved from line %d to %d", line.Start, resolution.Location.Start)
		} else if resolution.Moved {
			view.Reason = fmt.Sprintf("the file moved from %s", line.Path)
		}
	case resolution.Provisional:
		view.Reason = resolution.Reason
	default:
		view.Currency, view.Reason = Outdated, resolution.Reason
	}
	return view
}

// LineThreads is every code-line thread of review, in time order.
func LineThreads(ctx context.Context, review *saga.Review, rng *Range, resolver *coderesolve.Resolver) []LineThread {
	result := []LineThread{}
	for _, thread := range Threads(review.Comments) {
		if thread.Root.CodeLine == nil {
			continue
		}
		view := ViewCodeLine(ctx, resolver, rng, *thread.Root.CodeLine)
		view.ID, view.Target, view.State = thread.Root.ID, thread.Root.Target, thread.State
		view.Comments = 1 + len(thread.Replies)
		view.Summary, _, _ = strings.Cut(strings.TrimSpace(thread.Root.Body), "\n")
		result = append(result, view)
	}
	return result
}
