package gitdiff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

// TreeChangesForPaths resolves only the old paths a caller references.
// Rename detection still sees the entire product comparison. Hunk generation
// is restricted to the selected files, so unrelated large patches are never
// constructed. Deleted files need no hunks to establish that a pin is stale.
func TreeChangesForPaths(ctx context.Context, repo, from, to string, paths []string) ([]FileChange, error) {
	if from == to || len(paths) == 0 {
		return nil, nil
	}
	paths = slices.Clone(paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	selection := sha256.Sum256([]byte(strings.Join(paths, "\x00")))
	return cacheTreeChanges(ctx, func() (snapshotcache.Key, bool) {
		key, ok := treeChangesKey(ctx, repo, from, to)
		key.Tree += "\x00selected-paths/v1\x00" + hex.EncodeToString(selection[:])
		return key, ok
	}, func() ([]FileChange, error) {
		return selectedChangesOnce(ctx, repo, from, to, paths)
	})
}

type rawChange struct {
	FileChange
	oldObject, newObject string
	status               byte
}

func selectedChangesOnce(ctx context.Context, repo, from, to string, paths []string) ([]FileChange, error) {
	metadata, err := diffCommitsOnce(ctx, repo, []string{"--raw", "--no-abbrev", "-z"}, from, to, ".", ":(exclude,glob)**/*.saga/**")
	if err != nil {
		return treeChangesOnce(ctx, repo, from, to)
	}
	catalog, err := parseRawChanges(metadata)
	if err != nil {
		return treeChangesOnce(ctx, repo, from, to)
	}
	wanted := make(map[string]bool, len(paths))
	for _, path := range paths {
		wanted[path] = true
	}
	var selected []rawChange
	var patchPaths []string
	for _, change := range catalog {
		if !wanted[change.OldPath] {
			continue
		}
		if change.status == 'T' {
			// Git emits a type change as a deletion followed by an addition;
			// retain the established resolver semantics for that uncommon case.
			return treeChangesOnce(ctx, repo, from, to)
		}
		selected = append(selected, change)
		if !change.Deleted && change.OldPath == change.NewPath && change.oldObject != change.newObject {
			patchPaths = append(patchPaths, ":(literal)"+change.OldPath)
		}
	}
	geometry := map[string]FileChange{}
	for len(patchPaths) > 0 {
		// Bound argv size on Windows and on repositories with many long
		// referenced paths.
		end, size := 0, 0
		for end < len(patchPaths) && (end == 0 || size+len(patchPaths[end])+1 <= 8<<10) {
			size += len(patchPaths[end]) + 1
			end++
		}
		// Global metadata has already established that these paths remain in
		// place. Disabling local rename detection prevents a restricted diff
		// from choosing a different rename pairing than the global comparison.
		// Each selection has different argv. Use a one-shot process here so
		// hundreds of historical pins cannot leave hundreds of idle batch
		// processes resident for the remainder of the command.
		args := canonicalDiffArgs(repo, "-p", "--unified=0", "--no-renames", from, to, "--")
		args = append(args, patchPaths[:end]...)
		patch, err := exec.CommandContext(ctx, "git", args...).Output()
		if err != nil {
			return treeChangesOnce(ctx, repo, from, to)
		}
		changes, err := ParseTreeChanges(patch)
		if err != nil {
			return treeChangesOnce(ctx, repo, from, to)
		}
		for _, change := range changes {
			geometry[change.OldPath] = change
		}
		patchPaths = patchPaths[end:]
	}
	var result []FileChange
	for _, change := range selected {
		value := change.FileChange
		if !change.Deleted && change.oldObject != change.newObject {
			patch, found := geometry[change.OldPath]
			if change.OldPath != change.NewPath {
				// Extended object names preserve both original path contexts,
				// including their attributes. Diff exactly the blobs selected
				// by global rename detection, without rerunning that heuristic.
				patch, err = renamedGeometry(ctx, repo, from, to, change)
				if err != nil {
					return treeChangesOnce(ctx, repo, from, to)
				}
				found = true
			}
			if !found {
				// Unexpected Git output must never turn an unexamined change
				// into a current reference. Fall back to the complete comparison.
				return treeChangesOnce(ctx, repo, from, to)
			}
			value.Binary, value.Hunks = patch.Binary, patch.Hunks
		}
		result = append(result, value)
	}
	return result, nil
}

func renamedGeometry(ctx context.Context, repo, from, to string, change rawChange) (FileChange, error) {
	args := canonicalDiffArgs(repo, "--unified=0", from+":"+change.OldPath, to+":"+change.NewPath, "--")
	patch, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return FileChange{}, fmt.Errorf("read renamed source patch: %w", err)
	}
	changes, err := ParseTreeChanges(patch)
	if err != nil {
		return FileChange{}, err
	}
	if len(changes) != 1 {
		return FileChange{}, fmt.Errorf("expected one renamed source patch, got %d", len(changes))
	}
	return changes[0], nil
}

func parseRawChanges(data []byte) ([]rawChange, error) {
	var result []rawChange
	for len(data) > 0 {
		header, rest, ok := bytes.Cut(data, []byte{0})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || fields[4] == "" {
			return nil, fmt.Errorf("invalid source change metadata header %q", header)
		}
		path, rest, ok := bytes.Cut(rest, []byte{0})
		if !ok || len(path) == 0 {
			return nil, fmt.Errorf("invalid source change metadata path")
		}
		change := rawChange{
			FileChange: FileChange{OldPath: string(path), NewPath: string(path), ModeChange: strings.TrimPrefix(fields[0], ":") != fields[1]},
			oldObject:  fields[2], newObject: fields[3], status: fields[4][0],
		}
		switch change.status {
		case 'A':
			change.OldPath, change.Added = "", true
			change.ModeChange = false
		case 'D':
			change.NewPath, change.Deleted = "", true
			change.ModeChange = false
		case 'R', 'C':
			newPath, remaining, ok := bytes.Cut(rest, []byte{0})
			if !ok || len(newPath) == 0 {
				return nil, fmt.Errorf("invalid renamed source metadata path")
			}
			change.NewPath, rest = string(newPath), remaining
		case 'M', 'T':
		default:
			return nil, fmt.Errorf("unsupported source change metadata status %q", fields[4])
		}
		result = append(result, change)
		data = rest
	}
	return result, nil
}
