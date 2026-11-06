package gitdiff

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

func treeCacheFixture(t *testing.T) (repo, base, head string) {
	t.Helper()
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo = newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "sub", "data.txt"), "one\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base = strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "sub", "data.txt"), "two\n")
	gitTest(t, repo, "commit", "-am", "edit")
	head = strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	return
}

func TestTreeCacheSurvivesSessionsAndDetectsAttributeAndConfigEdits(t *testing.T) {
	repo, base, head := treeCacheFixture(t)
	attr := filepath.Join(repo, "sub", ".gitattributes")
	writeGitTestFile(t, attr, "*.txt  diff\n")
	info, err := os.Stat(attr)
	if err != nil {
		t.Fatal(err)
	}
	builds := 0
	read := func() []FileChange {
		t.Helper()
		ctx, end := gitexec.Begin(context.Background())
		defer end()
		changes, err := cachedTreeChanges(ctx, repo, base, head, func() ([]FileChange, error) {
			builds++
			return treeChangesOnce(ctx, repo, base, head)
		})
		if err != nil || len(changes) != 1 {
			t.Fatalf("comparison = %+v, %v", changes, err)
		}
		return changes
	}
	if read()[0].Binary || read()[0].Binary || builds != 1 {
		t.Fatalf("separate sessions must share the successful text comparison; builds=%d", builds)
	}
	// Saga edits must be assessed afresh by callers, but do not change the
	// source-only comparison cached here.
	writeGitTestFile(t, filepath.Join(repo, "change.saga", "record.json"), "{}\n")
	read()
	if builds != 1 {
		t.Fatalf("a Saga-only edit rebuilt a source comparison: %d", builds)
	}
	gitTest(t, repo, "branch", "another-agent", head)
	read()
	if builds != 1 {
		t.Fatalf("an unrelated agent branch invalidated a full-commit comparison: %d", builds)
	}
	// Same path, byte length and timestamp, different effective attributes.
	writeGitTestFile(t, attr, "*.txt -diff\n")
	if err := os.Chtimes(attr, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if !read()[0].Binary || builds != 2 {
		t.Fatalf("nested attribute edit must invalidate the text comparison; builds=%d", builds)
	}
	writeGitTestFile(t, attr, "*.txt diff=custom\n")
	gitTest(t, repo, "config", "diff.custom.binary", "false")
	if read()[0].Binary {
		t.Fatal("custom text driver was reported binary")
	}
	gitTest(t, repo, "config", "diff.custom.binary", "true")
	if !read()[0].Binary || builds != 4 {
		t.Fatalf("driver configuration edit must invalidate the comparison; builds=%d", builds)
	}
}

func TestTreeCacheRebuildsCorruptionAndFallsBackWhenUnavailable(t *testing.T) {
	repo, base, head := treeCacheFixture(t)
	builds := 0
	compute := func() ([]FileChange, error) {
		builds++
		return treeChangesOnce(context.Background(), repo, base, head)
	}
	if _, err := cachedTreeChanges(context.Background(), repo, base, head, compute); err != nil {
		t.Fatal(err)
	}
	key, ok := treeChangesKey(context.Background(), repo, base, head)
	if !ok {
		t.Fatal("fixture must be cacheable")
	}
	cache, err := snapshotcache.Default()
	if err != nil {
		t.Fatal(err)
	}
	dir, ok := cache.Lookup(key)
	if !ok {
		t.Fatal("successful comparison was not persisted")
	}
	// Valid JSON with altered geometry must fail the integrity check too.
	writeGitTestFile(t, filepath.Join(dir, treeChangesFile), `{"Format":"tree-changes/v1","Changes":[]}`)
	if changes, err := cachedTreeChanges(context.Background(), repo, base, head, compute); err != nil || len(changes) != 1 || builds != 2 {
		t.Fatalf("damaged cache was not rebuilt: %+v, %v; builds=%d", changes, err, builds)
	}
	if _, ok := readCachedChanges(dir); !ok {
		t.Fatal("damaged generation was not repaired")
	}
	blocked := filepath.Join(t.TempDir(), "file")
	writeGitTestFile(t, blocked, "not a directory")
	t.Setenv(snapshotcache.DirEnv, blocked)
	if changes, err := cachedTreeChanges(context.Background(), repo, base, head, compute); err != nil || len(changes) != 1 || builds != 3 {
		t.Fatalf("cache failure changed the answer: %+v, %v; builds=%d", changes, err, builds)
	}
}

func TestTreeCacheDoesNotPersistFailuresOrChangingInputs(t *testing.T) {
	repo, base, head := treeCacheFixture(t)
	ctx := context.Background()
	failure := errors.New("repository read failed")
	if _, err := cachedTreeChanges(ctx, repo, base, head, func() ([]FileChange, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatalf("failed comparison: %v", err)
	}
	key, _ := treeChangesKey(ctx, repo, base, head)
	cache, err := snapshotcache.Default()
	if err != nil {
		t.Fatal(err)
	}
	if _, found := cache.Lookup(key); found {
		t.Fatal("failed comparison was persisted")
	}
	_, err = cachedTreeChanges(ctx, repo, base, head, func() ([]FileChange, error) {
		changes, err := treeChangesOnce(ctx, repo, base, head)
		writeGitTestFile(t, filepath.Join(repo, "sub", ".gitattributes"), "*.txt -diff\n")
		return changes, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := cache.Lookup(key); found {
		t.Fatal("comparison was persisted under attributes that changed during its build")
	}
	if changes, err := TreeChanges(ctx, repo, base, head); err != nil || len(changes) != 1 || !changes[0].Binary {
		t.Fatalf("next comparison must observe changed attributes: %+v, %v", changes, err)
	}
}

func TestTreeCacheFollowsHistoryReplacementAndObjectAvailability(t *testing.T) {
	repo, base, head := treeCacheFixture(t)
	read := func(from string) ([]FileChange, error) {
		ctx, end := gitexec.Begin(context.Background())
		defer end()
		return TreeChanges(ctx, repo, from, head)
	}
	if changes, err := read(base); err != nil || len(changes) != 1 {
		t.Fatalf("initial comparison: %+v, %v", changes, err)
	}
	gitTest(t, repo, "replace", base, head)
	if changes, err := read(base); err != nil || len(changes) != 0 {
		t.Fatalf("replaced history served the old comparison: %+v, %v", changes, err)
	}
	gitTest(t, repo, "replace", "-d", base)
	if changes, err := read(base); err != nil || len(changes) != 1 {
		t.Fatalf("restored history: %+v, %v", changes, err)
	}
	// Custom replace namespaces also change the comparison's meaning.
	t.Setenv("GIT_REPLACE_REF_BASE", "refs/custom-replacements/")
	gitTest(t, repo, "replace", base, head)
	if changes, err := read(base); err != nil || len(changes) != 0 {
		t.Fatalf("custom replacement namespace served old history: %+v, %v", changes, err)
	}
	gitTest(t, repo, "replace", "-d", base)
	t.Setenv("GIT_REPLACE_REF_BASE", "")
	if _, ok := treeChangesKey(context.Background(), repo, base, head); ok {
		t.Fatal("an empty custom replace namespace must not be guessed cacheable")
	}
	if err := os.Unsetenv("GIT_REPLACE_REF_BASE"); err != nil {
		t.Fatal(err)
	}
	tree := strings.TrimSpace(gitTest(t, repo, "rev-parse", base+"^{tree}"))
	orphan := strings.TrimSpace(gitTest(t, repo, "commit-tree", tree, "-m", "unreachable pin"))
	if changes, err := read(orphan); err != nil || len(changes) != 1 {
		t.Fatalf("unreachable but available pin: %+v, %v", changes, err)
	}
	if err := os.Remove(filepath.Join(repo, ".git", "objects", orphan[:2], orphan[2:])); err != nil {
		t.Fatal(err)
	}
	if changes, err := read(orphan); err == nil {
		t.Fatalf("a pruned pin was answered from the cache: %+v", changes)
	}
}

func TestTreeCacheIncludesAttributesOfSagaNamedRegularFiles(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := newGitTestRepo(t)
	// The diff excludes directories ending in .saga, but a regular file with
	// that suffix still participates and its attributes must be fingerprinted.
	writeGitTestFile(t, filepath.Join(repo, "note.saga"), "one\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "note.saga"), "two\n")
	gitTest(t, repo, "commit", "-am", "edit")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	before, err := TreeChanges(context.Background(), repo, base, head)
	if err != nil || len(before) != 1 || before[0].Binary {
		t.Fatalf("text comparison: %+v, %v", before, err)
	}
	writeGitTestFile(t, filepath.Join(repo, ".gitattributes"), "*.saga -diff\n")
	after, err := TreeChanges(context.Background(), repo, base, head)
	if err != nil || len(after) != 1 || !after[0].Binary {
		t.Fatalf("regular .saga file's attributes were ignored: %+v, %v", after, err)
	}
}
