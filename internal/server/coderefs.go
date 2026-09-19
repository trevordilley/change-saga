package server

import "github.com/twentyideas/changesaga/internal/coderef"

// fileLocation is the whole-file code location a changed file is reviewed at:
// the head commit, or the merge-base when the file was deleted.
func fileLocation(baseOID, headOID, path string, deleted bool) string {
	commit := headOID
	if deleted {
		commit = baseOID
	}
	return coderef.Location{Commit: commit, Path: path}.String()
}
