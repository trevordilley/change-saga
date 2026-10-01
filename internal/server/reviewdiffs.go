package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
)

// reviewDiffs belongs to one request and one resolved review range. Only raw
// successful patches are shared: every reference is still resolved and filtered
// separately, including duplicate locations with different digests. It is used
// sequentially and must not be retained on app or shared between requests.
type reviewDiffs struct {
	repo         string
	rng          reviewstate.Range
	rootResolved bool
	patches      map[string]string
	resolve      func(context.Context, coderef.Reference, string) coderesolve.Resolution
	rootLookup   func(context.Context, string, ...string) (string, error)
	fileDiff     func(context.Context, string, string, string, ...string) (string, error)
}

// The caller owns the resolver and closes it after rendering the request.
func newReviewDiffs(sourceDir string, resolver *coderesolve.Resolver, rng reviewstate.Range) *reviewDiffs {
	diffs := &reviewDiffs{
		repo: sourceDir, rng: rng, patches: make(map[string]string),
		rootLookup: gitOutput, fileDiff: gitdiff.FileDiff,
	}
	if resolver != nil {
		diffs.resolve = resolver.Resolve
	}
	return diffs
}

func (diffs *reviewDiffs) patch(ctx context.Context, path string) (string, error) {
	if !diffs.rootResolved {
		// Pathspecs are relative to the working directory; the source directory
		// can be a Saga nested inside the code checkout. Preserve the original
		// fallback to that directory when Git cannot report its root.
		if top, err := diffs.rootLookup(ctx, diffs.repo, "rev-parse", "--show-toplevel"); err == nil && top != "" {
			diffs.repo = top
			diffs.rootResolved = true
		}
	}
	if patch, ok := diffs.patches[path]; ok {
		return patch, nil
	}
	patch, err := diffs.fileDiff(ctx, diffs.repo, diffs.rng.BaseOID, diffs.rng.HeadOID, path)
	if err == nil && diffs.rootResolved {
		// An empty patch is a successful read too. Do not memoize failures:
		// another reference can retry, retaining the existing per-reference
		// diagnostic if its read fails. A fallback-directory patch must also
		// be retried while root lookup can still recover.
		diffs.patches[path] = patch
	}
	return patch, err
}

func (diffs *reviewDiffs) referenceDiff(ctx context.Context, reference coderef.Reference) *reviewDiffView {
	view := &reviewDiffView{Path: reference.Path, Location: reference.Location().String(), Base: diffs.rng.BaseOID, Head: diffs.rng.HeadOID}
	start, end := reference.Start, reference.End
	path := reference.Path
	// Only lines resolved at the head are numbered on the new side, the side
	// a diff can be trimmed to.
	atHead := false
	if diffs.resolve != nil {
		if resolution := diffs.resolve(ctx, reference, diffs.rng.HeadOID); resolution.Current() {
			path, start, end = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
			atHead = true
		} else if !reference.WholeFile() {
			view.Note = "The referenced lines changed after the reference was written; showing every change to the file."
			start, end = 0, 0
		}
	}
	patch, err := diffs.patch(ctx, path)
	if err != nil {
		view.Note = "The diff could not be read from this checkout."
		return view
	}
	view.Path = path
	view.Lines = diffLinesTouching(patch, start, end)
	if atHead && start > 0 {
		view.Lines = trimDiffToRange(view.Lines, start, end, reviewDiffContext)
	}
	if len(view.Lines) == 0 {
		if start > 0 {
			view.Note = fmt.Sprintf("Lines %d-%d are unchanged between the base and the head.", start, end)
		} else if view.Note == "" {
			view.Note = "The file is unchanged between the base and the head."
		}
	}
	return view
}

// reviewDiffContext is how many lines around a referenced range its diff
// shows. A hunk can be a whole new file, and a reference to three of its
// lines should show those lines, not the file.
const reviewDiffContext = 3

// trimDiffToRange keeps the diff lines within context lines of start..end
// on the new side, under their hunk's header. Added and context lines are
// kept by their own line. A run of deleted lines is kept exactly when a
// line that replaces it is, so a change is never shown as a pure deletion
// or a pure addition; a run replaced by nothing sits after the new line
// before it. Where lines are left out, a marker says so. A hunk that
// already fits is kept exactly.
func trimDiffToRange(lines []reviewDiffLine, start, end, context int) []reviewDiffLine {
	low, high := start-context, end+context
	within := func(position int) bool { return position >= low && position <= high }
	var kept []reviewDiffLine
	var header *reviewDiffLine
	position, gap := 0, false
	keep := func(line reviewDiffLine) {
		if header != nil {
			kept = append(kept, *header)
			header = nil
		}
		if gap {
			kept = append(kept, reviewDiffLine{Kind: "hunk", Text: "⋯"})
		}
		gap = false
		kept = append(kept, line)
	}
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		switch line.Kind {
		case "hunk":
			// A hunk's deleted lines before its first new line sit just
			// before the new line it starts at.
			_, _, newStart, _ := parseHunkHeader(line.Text)
			position, gap, header = newStart-1, false, &lines[index]
		case "del":
			last := index
			for last+1 < len(lines) && lines[last+1].Kind == "del" {
				last++
			}
			replaced, shown := false, false
			for next := last + 1; next < len(lines) && lines[next].Kind == "add"; next++ {
				replaced = true
				added, _ := strconv.Atoi(lines[next].New)
				shown = shown || within(added)
			}
			if !replaced {
				shown = within(position)
			}
			for ; index <= last; index++ {
				if shown {
					keep(lines[index])
				} else {
					gap = true
				}
			}
			index = last
		default:
			position, _ = strconv.Atoi(line.New)
			if within(position) {
				keep(line)
			} else {
				gap = true
			}
		}
	}
	return kept
}
