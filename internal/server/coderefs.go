package server

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// authorCode turns a location a reviewer selected into a code reference by
// reading its content digest from the source repository. Browsers send only
// the location; the digest is never taken from a request.
func (a *app) authorCode(ctx context.Context, location coderef.Location) (coderef.Reference, error) {
	resolver, err := coderesolve.New(ctx, a.sourceDir)
	if err != nil {
		return coderef.Reference{}, err
	}
	defer resolver.Close()
	return resolver.Author(ctx, location, "")
}

// authorAnchor completes a code anchor submitted by a browser.
func (a *app) authorAnchor(ctx context.Context, anchor *saga.Anchor) error {
	if anchor.Type != "code" || anchor.Code == nil {
		return nil
	}
	reference, err := a.authorCode(ctx, anchor.Code.Location())
	if err != nil {
		return err
	}
	anchor.Code = &reference
	return nil
}

// codeThreadKeys returns the comparison atom keys a code-anchored thread sits
// on. A comparison's added lines live at its head and deleted lines at its
// merge-base, so the anchor's commit decides the side; its line numbers are
// unchanged by commits that only touch Saga files.
func codeThreadKeys(anchor *coderef.Reference, baseOID string) []string {
	if anchor == nil || anchor.WholeFile() {
		return nil
	}
	side := "new"
	if anchor.Commit == baseOID {
		side = "old"
	}
	keys := make([]string, 0, anchor.End-anchor.Start+1)
	for line := anchor.Start; line <= anchor.End; line++ {
		keys = append(keys, gitdiff.Key(gitdiff.Atom{Kind: "line", Path: anchor.Path, Side: side, Line: line}))
	}
	return keys
}

// latestFileReviews keeps the latest reviewed/unreviewed mark per file path.
// Ties on created_at resolve by id so the result does not depend on directory
// listing order.
func latestFileReviews(reviews []saga.FileReview) map[string]saga.FileReview {
	latest := map[string]saga.FileReview{}
	for _, review := range reviews {
		path := review.Code.Path
		if previous, ok := latest[path]; !ok || previous.CreatedAt.Before(review.CreatedAt) ||
			previous.CreatedAt.Equal(review.CreatedAt) && previous.ID < review.ID {
			latest[path] = review
		}
	}
	return latest
}

// fileLocation is the whole-file code location a changed file is reviewed at:
// the head commit, or the merge-base when the file was deleted.
func fileLocation(baseOID, headOID, path string, deleted bool) string {
	commit := headOID
	if deleted {
		commit = baseOID
	}
	return coderef.Location{Commit: commit, Path: path}.String()
}
