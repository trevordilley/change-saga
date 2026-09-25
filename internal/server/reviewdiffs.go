package server

import (
	"context"
	"fmt"

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
	view := &reviewDiffView{Path: reference.Path, Location: reference.Location().String()}
	start, end := reference.Start, reference.End
	path := reference.Path
	if diffs.resolve != nil {
		if resolution := diffs.resolve(ctx, reference, diffs.rng.HeadOID); resolution.Current() {
			path, start, end = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
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
	if len(view.Lines) == 0 {
		if start > 0 {
			view.Note = fmt.Sprintf("Lines %d-%d are unchanged between the base and the head.", start, end)
		} else if view.Note == "" {
			view.Note = "The file is unchanged between the base and the head."
		}
	}
	return view
}
