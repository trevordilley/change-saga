package changeview

import (
	"context"
	"path"
	"sort"
	"strings"

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
		joined := path.Join(location.Path, file)
		if strings.HasSuffix(file, "/") {
			joined += "/"
		}
		current = append(current, joined)
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
			before, exact, ok := movedRecordPath(carried, move, file)
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

// A current record replaces files that existed only in older Git trees. Query
// its whole package so deleting revisions during normalization does not erase
// those commits from the history view. Ordinary single-file records stay exact.
func recordHistoryFiles(files []string) []string {
	paths := map[string]bool{}
	for _, file := range files {
		parts := strings.Split(file, "/")
		if len(parts) >= 3 && parts[0] == "___whiteboards" {
			file = strings.Join(parts[:2], "/") + "/"
		}
		for i, part := range parts[:len(parts)-1] {
			switch path.Ext(part) {
			case ".story", ".persona", ".term", ".flag", ".deprecation", ".component", ".system", ".data-entity", ".erd", ".erd-overlay", ".prototype", ".test":
				file = strings.Join(parts[:i+1], "/") + "/"
			}
			if strings.HasSuffix(file, "/") {
				break
			}
		}
		paths[file] = true
	}
	result := make([]string, 0, len(paths))
	for file := range paths {
		result = append(result, file)
	}
	sort.Strings(result)
	return result
}

func movedRecordPath(carried sagalineage.Carried, move sagalineage.Move, file string) (string, bool, bool) {
	if !strings.HasSuffix(file, "/") {
		return carried.Moved(move, file)
	}
	relative := strings.TrimPrefix(file, strings.TrimSuffix(move.To, "/")+"/")
	if relative == file {
		return "", false, false
	}
	exact, exists := true, false
	for name, blob := range carried.Before {
		if strings.HasPrefix(name, relative) {
			exists = true
			if carried.After[name] != blob {
				exact = false
			}
		}
	}
	for name, blob := range carried.After {
		if strings.HasPrefix(name, relative) && carried.Before[name] != blob {
			exact = false
		}
	}
	return path.Join(move.From, relative) + "/", exact, exists
}
