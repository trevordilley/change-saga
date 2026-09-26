// Package sagalineage finds where one Saga lived at each commit of its
// repository. A Saga's identity is its manifest id, not its directory, so a
// Saga renamed with git mv (as app.saga became change.saga) is still the same
// Saga before the move, while an unrelated Saga that once had today's name,
// or that an earlier commit deleted as a new one was added, never is.
//
// Every reader of a Saga at a past commit (comparisons, record history,
// commit reasons, saved views, pre-integration, and sync-cursor history) asks
// this package for the path rather than assuming today's.
package sagalineage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/twentyideas/changesaga/internal/saga"
)

// Move is one commit that moved the whole Saga from one directory to another.
type Move struct {
	Commit, From, To string
}

// Lineage is one Saga's past locations as seen from a head commit.
type Lineage struct {
	// Path is the Saga's directory at head, relative to the repository.
	Path string
	// Moves are the Saga's verified moves, newest first; each ends where the
	// next newer one begins.
	Moves []Move
	// Born is the commit that created the Saga at its oldest path, or empty
	// when Git cannot say. Nothing before it is this Saga.
	Born string
}

// Era is a stretch of history during which the Saga sat at one path: the
// commits in From..To, where an empty From means from the beginning.
type Era struct {
	Path, From, To string
}

// Of reads the lineage of the Saga at sagaPath, as of the repository's HEAD.
// The history beneath a commit never changes, so it is read once per head.
func Of(ctx context.Context, repo, sagaPath string) Lineage {
	lineage := Lineage{Path: sagaPath}
	if sagaPath == "" || sagaPath == "." {
		return lineage
	}
	head, err := output(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	head = strings.TrimSpace(head)
	if err != nil || head == "" {
		return lineage
	}
	key := repo + "\x00" + sagaPath + "\x00" + head
	if cached, ok := lineages.Load(key); ok {
		return cached.(Lineage)
	}
	lineage.Moves = moves(ctx, repo, sagaPath, head)
	oldest, before := sagaPath, head
	if count := len(lineage.Moves); count > 0 {
		oldest, before = lineage.Moves[count-1].From, lineage.Moves[count-1].Commit+"^1"
	}
	if born, err := output(ctx, repo, "log", "-1", "--diff-filter=A", "--format=%H", before, "--", path.Join(oldest, saga.ManifestName)); err == nil {
		lineage.Born = strings.TrimSpace(born)
	}
	lineages.Store(key, lineage)
	return lineage
}

var lineages sync.Map

// moves follows the manifest back through renames and keeps each one only
// while it is verifiably the same Saga moving: the manifest id is unchanged
// and most of the Saga's files arrived byte for byte at the same relative
// path. Git calls one near-identical manifest a rename of another, so a Saga
// deleted in the commit that added a new one would otherwise pass for it.
func moves(ctx context.Context, repo, sagaPath, head string) []Move {
	manifest := path.Join(sagaPath, saga.ManifestName)
	log, err := output(ctx, repo, "log", "--follow", "--diff-filter=R", "--name-status", "-z", "--format=%x1e%H", head, "--", manifest)
	if err != nil {
		return nil
	}
	result := []Move{}
	want := sagaPath
	for _, record := range strings.Split(log, "\x1e") {
		fields := strings.Split(strings.Trim(record, "\x00\n"), "\x00")
		if len(fields) < 4 {
			continue
		}
		commit := strings.TrimSpace(fields[0])
		status, from, to := strings.TrimSpace(fields[1]), fields[2], fields[3]
		if !strings.HasPrefix(status, "R") || path.Base(from) != saga.ManifestName || path.Base(to) != saga.ManifestName || path.Dir(to) != want {
			break
		}
		move := Move{Commit: commit, From: path.Dir(from), To: path.Dir(to)}
		if !sameSaga(ctx, repo, move) {
			break
		}
		result = append(result, move)
		want = move.From
	}
	return result
}

func sameSaga(ctx context.Context, repo string, move Move) bool {
	before, after := manifestID(ctx, repo, move.Commit+"^1", move.From), manifestID(ctx, repo, move.Commit, move.To)
	if before == "" || before != after {
		return false
	}
	carried := CarriedBy(ctx, repo, move)
	return len(carried.Before) > 0 && 2*carried.Exact > len(carried.Before)
}

func manifestID(ctx context.Context, repo, commit, sagaPath string) string {
	data, err := output(ctx, repo, "show", commit+":"+path.Join(sagaPath, saga.ManifestName))
	if err != nil {
		return ""
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(data), &manifest) != nil {
		return ""
	}
	return manifest.ID
}

// Carried is what one move did to each of the Saga's files, by path relative
// to the Saga.
type Carried struct {
	// Before and After map each relative path to its blob before and after
	// the move.
	Before, After map[string]string
	// Exact counts the files that arrived unchanged.
	Exact int
}

// Moved reports the file's path before the move and whether the move left
// its content unchanged. A file the move created was not carried at all.
func (c Carried) Moved(move Move, file string) (before string, exact, ok bool) {
	relative := strings.TrimPrefix(file, strings.TrimSuffix(move.To, "/")+"/")
	blob, existed := c.Before[relative]
	if relative == file || !existed {
		return "", false, false
	}
	return path.Join(move.From, relative), c.After[relative] == blob, true
}

// CarriedBy compares the Saga's files on both sides of a move.
func CarriedBy(ctx context.Context, repo string, move Move) Carried {
	key := repo + "\x00" + move.Commit + "\x00" + move.From + "\x00" + move.To
	if cached, ok := carriedCache.Load(key); ok {
		return cached.(Carried)
	}
	carried := Carried{Before: blobs(ctx, repo, move.Commit+"^1", move.From), After: blobs(ctx, repo, move.Commit, move.To)}
	for relative, blob := range carried.Before {
		if carried.After[relative] == blob {
			carried.Exact++
		}
	}
	carriedCache.Store(key, carried)
	return carried
}

var carriedCache sync.Map

func blobs(ctx context.Context, repo, commit, dir string) map[string]string {
	result := map[string]string{}
	listing, err := output(ctx, repo, "ls-tree", "-r", "-z", "--full-tree", commit, "--", dir)
	if err != nil {
		return result
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	for _, entry := range strings.Split(listing, "\x00") {
		meta, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" || !strings.HasPrefix(name, prefix) {
			continue
		}
		result[strings.TrimPrefix(name, prefix)] = fields[2]
	}
	return result
}

// PathAt is where the Saga was at commit, and false when the Saga did not
// exist there: the commit is not descended from its birth. Before each move
// the commit does not descend from, the Saga was where that move took it
// from.
func (l Lineage) PathAt(ctx context.Context, repo, commit string) (string, bool) {
	at := l.Path
	for _, move := range l.Moves {
		if isAncestor(ctx, repo, move.Commit, commit) {
			break
		}
		at = move.From
	}
	if l.Born != "" && !isAncestor(ctx, repo, l.Born, commit) {
		return "", false
	}
	return at, true
}

// Eras divides the history up to head into the stretches the Saga spent at
// each path, newest first. A move commit belongs to no era, because moving
// the Saga changes nothing it says; a caller that needs what else the move
// did asks CarriedBy. The oldest era starts at the Saga's birth.
func (l Lineage) Eras(ctx context.Context, repo, head string) []Era {
	eras := []Era{}
	to, at := head, l.Path
	for _, move := range l.Moves {
		eras = append(eras, Era{Path: at, From: move.Commit, To: to})
		to, at = move.Commit+"^1", move.From
	}
	from := ""
	if l.Born != "" {
		if _, err := output(ctx, repo, "rev-parse", "--verify", "--quiet", l.Born+"^1^{commit}"); err == nil {
			from = l.Born + "^1"
		}
	}
	return append(eras, Era{Path: at, From: from, To: to})
}

// Range is the era's revision range for git log.
func (e Era) Range() string {
	if e.From == "" {
		return e.To
	}
	return e.From + ".." + e.To
}

// isAncestor reports whether commit descends from ancestor. Only Git's
// explicit "no" is false: a commit Git cannot answer for is read where the
// Saga is today rather than declared absent.
func isAncestor(ctx context.Context, repo, ancestor, commit string) bool {
	err := exec.CommandContext(ctx, "git", "-C", repo, "merge-base", "--is-ancestor", ancestor, commit).Run()
	var exit *exec.ExitError
	return !(errors.As(err, &exit) && exit.ExitCode() == 1)
}

func output(ctx context.Context, repo string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	err := command.Run()
	return stdout.String(), err
}
