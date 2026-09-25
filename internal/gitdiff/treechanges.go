package gitdiff

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitexec"
)

// Hunk is one zero-context hunk header. OldCount zero is a pure insertion after
// OldStart; NewCount zero is a pure deletion.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
}

// FileChange is how one product file changed between two commits, reduced to
// what remapping a line range needs: identity on each side and hunk geometry.
type FileChange struct {
	OldPath    string
	NewPath    string
	Added      bool
	Deleted    bool
	Binary     bool
	ModeChange bool
	Hunks      []Hunk
}

// TreeChanges compares two commits' product code (every path outside a .saga
// directory) with zero lines of context. Commits that differ only in Saga
// files therefore produce no changes at all.
func TreeChanges(ctx context.Context, repo, from, to string) ([]FileChange, error) {
	if from == to {
		return nil, nil
	}
	output, err := diffCommits(ctx, repo, []string{"-p", "--unified=0"}, from, to, ".", ":(exclude,glob)**/*.saga/**")
	if err != nil {
		var exitErr *exec.ExitError
		if errorAs(err, &exitErr) {
			return nil, fmt.Errorf("git diff %s..%s: %s", from, to, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git diff %s..%s: %w", from, to, err)
	}
	return ParseTreeChanges(output)
}

// FileDiff returns the patch of the named paths between two commits, for
// showing a reader what changed since a reference was pinned.
func FileDiff(ctx context.Context, repo, from, to string, paths ...string) (string, error) {
	if gitexec.NamesObjects(from, to) {
		key := append([]string{"file-diff", from, to, attributesIdentity(repo)}, paths...)
		output, err := gitexec.Stable(ctx, repo, []string{from, to}, key, func() ([]byte, error) {
			patch, err := fileDiffOnce(ctx, repo, from, to, paths...)
			return []byte(patch), err
		})
		return string(output), err
	}
	return fileDiffOnce(ctx, repo, from, to, paths...)
}

func fileDiffOnce(ctx context.Context, repo, from, to string, paths ...string) (string, error) {
	args := canonicalDiffArgs(repo, "--unified=3", from, to, "--")
	for _, path := range paths {
		args = append(args, ":(literal)"+path)
	}
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s..%s: %w", from, to, err)
	}
	return string(output), nil
}

// ParseTreeChanges reads a zero-context patch into per-file hunk geometry.
func ParseTreeChanges(patch []byte) ([]FileChange, error) {
	scanner := bufio.NewScanner(bytes.NewReader(patch))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var changes []FileChange
	var current *FileChange
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			oldPath, newPath := parseDiffHeader(strings.TrimPrefix(line, "diff --git "))
			changes = append(changes, FileChange{OldPath: oldPath, NewPath: newPath})
			current = &changes[len(changes)-1]
		case current == nil:
		case strings.HasPrefix(line, "new file mode "):
			current.Added, current.OldPath = true, ""
		case strings.HasPrefix(line, "deleted file mode "):
			current.Deleted, current.NewPath = true, ""
		case strings.HasPrefix(line, "rename from "):
			current.OldPath = unquoteGitPath(strings.TrimPrefix(line, "rename from "))
		case strings.HasPrefix(line, "rename to "):
			current.NewPath = unquoteGitPath(strings.TrimPrefix(line, "rename to "))
		case strings.HasPrefix(line, "old mode "), strings.HasPrefix(line, "new mode "):
			current.ModeChange = true
		case strings.HasPrefix(line, "GIT binary patch"), strings.HasPrefix(line, "Binary files "):
			current.Binary = true
		case strings.HasPrefix(line, "--- "):
			if value := parseHeaderPath(strings.TrimPrefix(line, "--- "), "a/"); value != "" {
				current.OldPath = value
			}
		case strings.HasPrefix(line, "+++ "):
			if value := parseHeaderPath(strings.TrimPrefix(line, "+++ "), "b/"); value != "" {
				current.NewPath = value
			}
		case strings.HasPrefix(line, "@@ "):
			match := hunkPattern.FindStringSubmatch(line)
			if match == nil {
				return nil, fmt.Errorf("parse hunk header %q", line)
			}
			hunk := Hunk{OldCount: 1, NewCount: 1}
			hunk.OldStart, _ = strconv.Atoi(match[1])
			if match[2] != "" {
				hunk.OldCount, _ = strconv.Atoi(match[2])
			}
			hunk.NewStart, _ = strconv.Atoi(match[3])
			if match[4] != "" {
				hunk.NewCount, _ = strconv.Atoi(match[4])
			}
			current.Hunks = append(current.Hunks, hunk)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan Git diff: %w", err)
	}
	return changes, nil
}
