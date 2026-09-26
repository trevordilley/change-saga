package changeview

import (
	"context"
	"path"

	"github.com/twentyideas/changesaga/internal/sagalineage"
)

// sagaPathAt is where the Saga now at location.Path was at commit, and false
// when the Saga did not exist there. A commit before a move reads the Saga
// where it was then, and a commit before the Saga never reads an unrelated
// one that had its name.
func sagaPathAt(ctx context.Context, location Location, commit string) (string, bool) {
	return sagalineage.Of(ctx, location.Repo, location.Path).PathAt(ctx, location.Repo, commit)
}

// recordCommits is readCommits over a record's files, oldest first, followed
// back through the Saga's moves to its birth and no further. A move that
// carried a file unchanged is not one of its commits; a move that created or
// edited it is, and history continues at the old path for every file the
// move carried, edited or not, so a squash-merged rename that also revised
// records still reaches back to where they began.
func recordCommits(ctx context.Context, location Location, files []string) ([]commitInfo, error) {
	lineage := sagalineage.Of(ctx, location.Repo, location.Path)
	eras := lineage.Eras(ctx, location.Repo, "HEAD")
	current := make([]string, 0, len(files))
	for _, file := range files {
		current = append(current, path.Join(location.Path, file))
	}
	chunks := [][]commitInfo{}
	for index, era := range eras {
		commits, err := readCommits(ctx, location.Repo, era.From, era.To, current...)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, commits)
		if index == len(lineage.Moves) {
			break
		}
		move := lineage.Moves[index]
		carried := sagalineage.CarriedBy(ctx, location.Repo, move)
		older, changed := []string{}, []string{}
		for _, file := range current {
			before, exact, ok := carried.Moved(move, file)
			if !exact {
				changed = append(changed, file)
			}
			if ok {
				older = append(older, before)
			}
		}
		if len(changed) > 0 {
			commits, err := readCommits(ctx, location.Repo, move.Commit+"^1", move.Commit, changed...)
			if err != nil {
				return nil, err
			}
			chunks = append(chunks, commits)
		}
		if len(older) == 0 {
			break
		}
		current = older
	}
	result := []commitInfo{}
	for index := len(chunks) - 1; index >= 0; index-- {
		result = append(result, chunks[index]...)
	}
	return result, nil
}
