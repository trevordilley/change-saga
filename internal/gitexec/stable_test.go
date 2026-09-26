package gitexec

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableRemembersSuccessesWithinSessions(t *testing.T) {
	repo, commits := history(t)
	key := []string{"test", t.Name()}
	calls := 0
	failing := func() ([]byte, error) { calls++; return nil, errors.New("not yet") }
	answering := func() ([]byte, error) { calls++; return []byte("answer"), nil }
	for range 2 {
		if _, err := Stable(context.Background(), repo, commits[:1], key, answering); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("without a session compute ran %d times; want every call", calls)
	}
	calls = 0
	for range 2 {
		ctx, end := Begin(context.Background())
		if _, err := Stable(ctx, repo, commits[:1], key, failing); err == nil {
			t.Fatal("a failure was reported as an answer")
		}
		end()
	}
	for range 3 {
		ctx, end := Begin(context.Background())
		if value, err := Stable(ctx, repo, commits[:1], key, answering); err != nil || string(value) != "answer" {
			t.Fatalf("Stable = %q, %v", value, err)
		}
		end()
	}
	if calls != 3 {
		t.Fatalf("compute ran %d times; want every failure and one success across sessions", calls)
	}
}

// gc prunes unreachable commits, as after a squash merge. An answer about a
// commit that is gone must not outlive it.
func TestStableForgetsAnswersAboutPrunedCommits(t *testing.T) {
	repo, commits := history(t)
	tree := git(t, repo, "rev-parse", commits[0]+"^{tree}")
	orphan := git(t, repo, "commit-tree", "-m", "unreachable", tree)
	key := []string{"test", t.Name()}
	calls := 0
	compute := func() ([]byte, error) { calls++; return []byte("answer"), nil }
	ctx, end := Begin(context.Background())
	if _, err := Stable(ctx, repo, []string{orphan}, key, compute); err != nil {
		t.Fatal(err)
	}
	end()
	if err := os.Remove(filepath.Join(repo, ".git", "objects", orphan[:2], orphan[2:])); err != nil {
		t.Fatal(err)
	}
	ctx, end = Begin(context.Background())
	defer end()
	if _, err := Stable(ctx, repo, []string{orphan}, key, compute); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("an answer about a pruned commit was served from memory (compute ran %d times)", calls)
	}
}

func TestAnswerCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := &answerCache{entries: map[string]*list.Element{}, order: list.New(), limit: 64}
	cache.put("a", []byte("0123"))
	cache.put("b", []byte("0123"))
	cache.get("a")
	cache.put("big", []byte(strings.Repeat("x", 5))) // over limit/16: never stored
	for index := range 10 {
		cache.put(fmt.Sprintf("k%d", index), []byte("0123"))
	}
	if _, ok := cache.get("big"); ok {
		t.Fatal("an answer larger than a sixteenth of the cache was stored")
	}
	if cache.size > cache.limit {
		t.Fatalf("cache holds %d bytes over its %d limit", cache.size, cache.limit)
	}
	if _, ok := cache.get("b"); ok {
		t.Fatal("the least recently used answer survived eviction")
	}
}

// A replace ref or graft makes a commit's ID stop fixing its parents; an
// answer remembered before one appears must not outlive it.
func TestStableIgnoresRepositoriesThatRewriteHistory(t *testing.T) {
	repo, commits := history(t)
	parent := func() string {
		t.Helper()
		ctx, end := Begin(context.Background())
		defer end()
		output, err := Stable(ctx, repo, commits[2:3], []string{"first-parent", commits[2]}, func() ([]byte, error) {
			return []byte(git(t, repo, "rev-parse", commits[2]+"^1")), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(output)
	}
	if got := parent(); got != commits[1] {
		t.Fatalf("first parent = %q; want %s", got, commits[1])
	}
	git(t, repo, "replace", "--graft", commits[2], commits[0])
	if got := parent(); got != commits[0] {
		t.Fatalf("first parent after a graft replace = %q; want %s", got, commits[0])
	}
}

// A replace ref added while a session runs must not let that session
// remember what the replacement makes Git report.
func TestStableDoesNotRememberAcrossAReplaceAddedMidSession(t *testing.T) {
	repo, commits := history(t)
	ctx, end := Begin(context.Background())
	parent := func(ctx context.Context) string {
		t.Helper()
		output, err := Stable(ctx, repo, commits[2:3], []string{"first-parent", commits[2]}, func() ([]byte, error) {
			return []byte(git(t, repo, "rev-parse", commits[2]+"^1")), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(output)
	}
	// The session checks for rewritten history before any replace exists.
	if _, err := Stable(ctx, repo, commits[1:2], []string{"probe", commits[1]}, func() ([]byte, error) { return []byte("x"), nil }); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "replace", "--graft", commits[2], commits[0])
	if got := parent(ctx); got != commits[0] {
		t.Fatalf("first parent with the replace = %q", got)
	}
	end()
	git(t, repo, "replace", "-d", commits[2])
	fresh, endFresh := Begin(context.Background())
	defer endFresh()
	if got := parent(fresh); got != commits[1] {
		t.Fatalf("after the replace was removed, first parent = %q; want %s", got, commits[1])
	}
}
