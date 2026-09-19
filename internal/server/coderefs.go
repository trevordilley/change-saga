package server

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
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

// fileLocation is the whole-file code location a changed file is reviewed at:
// the head commit, or the merge-base when the file was deleted.
func fileLocation(baseOID, headOID, path string, deleted bool) string {
	commit := headOID
	if deleted {
		commit = baseOID
	}
	return coderef.Location{Commit: commit, Path: path}.String()
}
