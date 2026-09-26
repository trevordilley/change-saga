package changeview

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/saga"
)

// sagaMove is one commit that moved the whole Saga to another directory, as
// renaming app.saga to change.saga does. Its manifest is what moved.
type sagaMove struct {
	commit, from, to string
}

// sagaMoves lists, newest first, the commits that moved the Saga now at
// location.Path, found by following its manifest through renames. Each move
// ends where the next newer one begins.
func sagaMoves(ctx context.Context, location Location) []sagaMove {
	head, err := revParse(ctx, location.Repo, "HEAD")
	if err != nil {
		return nil
	}
	// The history beneath a commit never changes, so its moves are read once.
	key := location.Repo + "\x00" + location.Path + "\x00" + head
	if cached, ok := sagaMoveCache.Load(key); ok {
		return cached.([]sagaMove)
	}
	manifest := path.Join(location.Path, saga.ManifestName)
	output, err := exec.CommandContext(ctx, "git", "-C", location.Repo, "log", "--follow", "--diff-filter=R", "--name-status", "-z", "--format=%x1e%H", head, "--", manifest).Output()
	if err != nil {
		return nil
	}
	moves := []sagaMove{}
	want := location.Path
	for _, record := range strings.Split(string(output), "\x1e") {
		fields := strings.Split(strings.Trim(record, "\x00\n"), "\x00")
		if len(fields) < 4 {
			continue
		}
		commit := strings.TrimSpace(fields[0])
		status, from, to := strings.TrimSpace(fields[1]), fields[2], fields[3]
		if !strings.HasPrefix(status, "R") || path.Base(from) != saga.ManifestName || path.Base(to) != saga.ManifestName || path.Dir(to) != want {
			break
		}
		moves = append(moves, sagaMove{commit: commit, from: path.Dir(from), to: path.Dir(to)})
		want = path.Dir(from)
	}
	sagaMoveCache.Store(key, moves)
	return moves
}

var sagaMoveCache sync.Map

// sagaPathAt is where the Saga now at location.Path was at commit: before
// each move that commit does not descend from, the Saga was where that move
// took it from. A commit older than a move therefore never reads an
// unrelated Saga that once had today's name.
func sagaPathAt(ctx context.Context, location Location, commit string) string {
	at := location.Path
	for _, move := range sagaMoves(ctx, location) {
		err := exec.CommandContext(ctx, "git", "-C", location.Repo, "merge-base", "--is-ancestor", move.commit, commit).Run()
		var exit *exec.ExitError
		if err == nil || !errors.As(err, &exit) || exit.ExitCode() != 1 {
			break
		}
		at = move.from
	}
	return at
}

// movedFiles maps each file the move carried unchanged to its path before the
// move. A file the move also edited, added, or left behind is absent, so its
// history is not joined to a different file's.
func movedFiles(ctx context.Context, repo string, move sagaMove) map[string]string {
	moved := map[string]string{}
	output, err := exec.CommandContext(ctx, "git", "-C", repo, "diff", "--name-status", "-z", "-M100%", move.commit+"^1", move.commit, "--", move.from, move.to).Output()
	if err != nil {
		return moved
	}
	fields := bytes.Split(output, []byte{0})
	for index := 0; index < len(fields); index++ {
		status := string(fields[index])
		switch {
		case status == "R100" && index+2 < len(fields):
			moved[string(fields[index+2])] = string(fields[index+1])
			index += 2
		case strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C"):
			index += 2
		case status != "":
			index++
		}
	}
	return moved
}

// recordCommits is readCommits over a record's files that follows the Saga
// through the moves that carried those files unchanged. The move itself
// changed nothing the record says, so it is not one of the record's commits.
func recordCommits(ctx context.Context, location Location, files []string) ([]commitInfo, error) {
	current := make([]string, 0, len(files))
	for _, file := range files {
		current = append(current, path.Join(location.Path, file))
	}
	to := "HEAD"
	eras := [][]commitInfo{}
	for _, move := range sagaMoves(ctx, location) {
		moved := movedFiles(ctx, location.Repo, move)
		older := make([]string, 0, len(current))
		for _, file := range current {
			if before, ok := moved[file]; ok {
				older = append(older, before)
			}
		}
		if len(older) != len(current) {
			break
		}
		era, err := readCommits(ctx, location.Repo, move.commit, to, current...)
		if err != nil {
			return nil, err
		}
		eras = append(eras, era)
		current, to = older, move.commit+"^1"
	}
	commits, err := readCommits(ctx, location.Repo, "", to, current...)
	if err != nil {
		return nil, err
	}
	for index := len(eras) - 1; index >= 0; index-- {
		commits = append(commits, eras[index]...)
	}
	return commits, nil
}
