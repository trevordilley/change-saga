package gitdiff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
	"github.com/twentyideas/changesaga/internal/store"
)

// Tree comparisons, unlike whole health reports, have a small, explicit set
// of dependencies. Cache only successful hunk geometry, never a verdict about
// a Saga or a missing object. Git still verifies every pin on each command.
// Bump this format when the canonical diff or parser semantics change.
const treeChangesFormat = "tree-changes/v1"

const treeChangesFile = "changes.json"

type cachedChanges struct {
	Format   string
	Checksum string
	Changes  []FileChange
}

func changesChecksum(changes []FileChange) string {
	data, _ := json.Marshal(changes)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readCachedChanges(dir string) ([]FileChange, bool) {
	data, err := os.ReadFile(filepath.Join(dir, treeChangesFile))
	var saved cachedChanges
	if err != nil || json.Unmarshal(data, &saved) != nil || saved.Format != treeChangesFormat || saved.Checksum != changesChecksum(saved.Changes) {
		return nil, false
	}
	return saved.Changes, true
}

func cachedTreeChanges(ctx context.Context, repo, from, to string, compute func() ([]FileChange, error)) ([]FileChange, error) {
	return cacheTreeChanges(ctx, func() (snapshotcache.Key, bool) {
		return treeChangesKey(ctx, repo, from, to)
	}, compute)
}

func cacheTreeChanges(ctx context.Context, inputs func() (snapshotcache.Key, bool), compute func() ([]FileChange, error)) ([]FileChange, error) {
	key, ok := inputs()
	if !ok {
		return compute()
	}
	cache, err := snapshotcache.Default()
	if err != nil {
		return compute()
	}
	if dir, found := cache.Lookup(key); found {
		if changes, ok := readCachedChanges(dir); ok {
			return changes, nil
		}
		// Damaged derived data is disposable; rebuild this generation alone.
		_ = os.RemoveAll(dir)
	}
	var changes []FileChange
	var computeErr error
	evaluated := false
	dir, _, buildErr := cache.Build(key, func(stage string) error {
		evaluated = true
		changes, computeErr = compute()
		if computeErr != nil {
			return computeErr
		}
		// Attribute files, configuration, and refs can be edited outside this
		// process. Publish only if the inputs still name the same comparison.
		after, valid := inputs()
		if !valid || after != key || ctx.Err() != nil {
			return errors.New("tree comparison inputs changed during build")
		}
		return store.WriteJSON(filepath.Join(stage, treeChangesFile), cachedChanges{
			Format: treeChangesFormat, Checksum: changesChecksum(changes), Changes: changes,
		}, false)
	})
	if evaluated {
		if buildErr == nil {
			// Keep several complete checks' comparisons while bounding disk use.
			_ = cache.PrunePublished(key, 128)
		}
		return changes, computeErr
	}
	if buildErr == nil {
		if changes, ok := readCachedChanges(dir); ok {
			return changes, nil
		}
	}
	// Cache availability never determines whether a report can be produced.
	return compute()
}

func treeChangesKey(ctx context.Context, repo, from, to string) (snapshotcache.Key, bool) {
	if !gitexec.IsObjectName(from) || !gitexec.IsObjectName(to) {
		return snapshotcache.Key{}, false
	}
	absolute, err := filepath.Abs(repo)
	if err != nil {
		return snapshotcache.Key{}, false
	}
	state, ok := gitexec.FreshComparisonDigest(ctx, repo)
	if !ok {
		return snapshotcache.Key{}, false
	}
	paths := map[string]bool{}
	for _, commit := range []string{from, to} {
		// Object availability is checked even on a cache hit: a fetch or gc
		// may have changed it without moving a ref.
		kind, answered := gitexec.ObjectType(ctx, repo, commit)
		if !answered {
			output, err := gitexec.Output(ctx, "-C", repo, "cat-file", "-t", commit)
			if err != nil {
				return snapshotcache.Key{}, false
			}
			kind = strings.TrimSpace(string(output))
		}
		if kind != "commit" {
			return snapshotcache.Key{}, false
		}
		output, err := gitexec.Stable(ctx, repo, []string{commit}, []string{"tree-cache-paths", state, commit}, func() ([]byte, error) {
			return exec.CommandContext(ctx, "git", "-C", repo, "ls-tree", "-r", "--name-only", "-z", commit).Output()
		})
		if err != nil {
			return snapshotcache.Key{}, false
		}
		for path := range bytes.SplitSeq(output, []byte{0}) {
			if len(path) != 0 {
				paths[string(path)] = true
			}
		}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	// Ask Git for effective attributes on every compared path. This covers
	// nested, untracked, global, info/attributes and index fallback rules,
	// without relying on mtimes or guessing where attribute files live.
	command := exec.CommandContext(ctx, "git", "-C", repo, "check-attr", "--all", "-z", "--stdin")
	if len(ordered) > 0 {
		command.Stdin = strings.NewReader(strings.Join(ordered, "\x00") + "\x00")
	}
	attributes, err := command.Output()
	if err != nil {
		return snapshotcache.Key{}, false
	}
	if after, ok := gitexec.FreshComparisonDigest(ctx, repo); !ok || after != state {
		return snapshotcache.Key{}, false
	}
	version, err := gitexec.Output(ctx, "--version")
	if err != nil {
		return snapshotcache.Key{}, false
	}
	var environment []string
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") || strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "XDG_CONFIG_HOME=") || strings.HasPrefix(entry, "USERPROFILE=") {
			environment = append(environment, entry)
		}
	}
	sort.Strings(environment)
	hash := sha256.New()
	for _, input := range [][]byte{[]byte(state), version, []byte(strings.Join(environment, "\x00")), attributes} {
		hash.Write(input)
		hash.Write([]byte{0})
	}
	return snapshotcache.Key{
		Saga:   absolute,
		Tree:   treeChangesFormat + "\x00" + hex.EncodeToString(hash.Sum(nil)),
		Source: from + "\x00" + to,
	}, true
}
