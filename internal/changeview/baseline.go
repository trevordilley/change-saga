package changeview

import (
	"context"
	"errors"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// IsSagaAbsent reports that a committed side has no Saga because it did not
// exist yet, as opposed to a Saga that exists but could not be read.
func IsSagaAbsent(err error) bool { return errors.Is(err, errAbsent) }

// BaseSide selects the committed Saga snapshot documenting a comparison's base
// with Open's rules: the base commit itself when the Saga shares the code
// repository, or the companion Saga commit whose sync cursor documents it. A
// side with an empty Source means no snapshot is known; callers must report
// the baseline as unknown rather than treating every record as added.
func BaseSide(ctx context.Context, sagaRoot, checkout string, changes gitdiff.ChangeSet) (SagaSide, []Diagnostic) {
	location, err := Locate(ctx, sagaRoot)
	if err != nil {
		return SagaSide{}, []Diagnostic{{Code: "saga_history_unavailable", Message: err.Error()}}
	}
	if sameRepository(ctx, location.Repo, checkout) {
		return SagaSide{Commit: changes.BaseOID, Source: SideGit}, nil
	}
	found, err := companionSides(ctx, location, checkout, changes)
	if err != nil {
		return SagaSide{}, []Diagnostic{{Code: "saga_history_unavailable", Message: err.Error()}}
	}
	diagnostics := []Diagnostic{}
	for _, d := range found.diagnostics {
		if d.Code != "saga_cursor_behind" {
			diagnostics = append(diagnostics, d)
		}
	}
	if found.base == "" {
		return SagaSide{}, diagnostics
	}
	return SagaSide{Commit: found.base, Source: SideGit}, diagnostics
}
