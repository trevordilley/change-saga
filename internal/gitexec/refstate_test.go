package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resolveIn resolves revision the way one command would: in a fresh session.
func resolveIn(t *testing.T, repo, revision string) (string, bool) {
	t.Helper()
	ctx, end := Begin(context.Background())
	defer end()
	return ResolveCommit(ctx, repo, revision)
}

// Every way refs move must reach the next command: a new commit, a checkout,
// a reset, packing refs, deleting a branch, and a ref only a linked worktree
// sees.
func TestRememberedRevisionsFollowRefChanges(t *testing.T) {
	repo, commits := history(t)
	expect := func(revision, want string) {
		t.Helper()
		for _, ctx := range []context.Context{nil, context.Background()} {
			var got string
			var ok bool
			if ctx == nil {
				got, ok = resolveIn(t, repo, revision)
			} else {
				got, ok = ResolveCommit(ctx, repo, revision)
			}
			if want == "" && ok {
				t.Fatalf("%s resolved to %s; want no answer", revision, got)
			}
			if want != "" && (!ok || got != want) {
				t.Fatalf("%s = %q, %v; want %s", revision, got, ok, want)
			}
		}
	}
	expect("HEAD", commits[3])
	expect("main~1", commits[2])

	write(t, filepath.Join(repo, "next.txt"), "next\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "next")
	next := git(t, repo, "rev-parse", "HEAD")
	expect("HEAD", next)
	expect("main~1", commits[3])

	git(t, repo, "checkout", "-q", "-b", "side", commits[1])
	expect("HEAD", commits[1])
	expect("side", commits[1])

	git(t, repo, "pack-refs", "--all")
	expect("side", commits[1])
	git(t, repo, "update-ref", "refs/heads/side", commits[2])
	expect("side", commits[2])
	expect("HEAD", commits[2])

	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "branch", "-q", "-D", "side")
	expect("side", "")

	git(t, repo, "reset", "-q", "--hard", commits[0])
	expect("HEAD", commits[0])

	linked := filepath.Join(t.TempDir(), "linked")
	git(t, repo, "worktree", "add", "-q", "-b", "elsewhere", linked, commits[2])
	expect("HEAD", commits[0])
	got, ok := resolveIn(t, linked, "HEAD")
	if !ok || got != commits[2] {
		t.Fatalf("linked worktree HEAD = %q, %v; want %s", got, ok, commits[2])
	}
	git(t, linked, "reset", "-q", "--hard", commits[3])
	if got, ok := resolveIn(t, linked, "HEAD"); !ok || got != commits[3] {
		t.Fatalf("linked worktree HEAD after reset = %q, %v; want %s", got, ok, commits[3])
	}
	expect("HEAD", commits[0])
}

func TestRememberedConfigurationFollowsConfigChanges(t *testing.T) {
	repo, _ := history(t)
	global := filepath.Join(t.TempDir(), "gitconfig")
	write(t, global, "")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	origin := func() string {
		t.Helper()
		ctx, end := Begin(context.Background())
		defer end()
		output, err := ConfigOutput(ctx, repo, "remote", "get-url", "origin")
		if err != nil {
			return "error"
		}
		return strings.TrimSpace(string(output))
	}
	if got := origin(); got != "error" {
		t.Fatalf("origin before one exists = %q", got)
	}
	git(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	if got := origin(); got != "https://example.test/a.git" {
		t.Fatalf("origin = %q", got)
	}
	git(t, repo, "remote", "set-url", "origin", "https://example.test/b.git")
	if got := origin(); got != "https://example.test/b.git" {
		t.Fatalf("origin after set-url = %q", got)
	}
	// A global rewrite rule changes what get-url reports without touching
	// the repository.
	write(t, global, "[url \"https://mirror.test/\"]\n\tinsteadOf = https://example.test/\n")
	if got := origin(); got != "https://mirror.test/b.git" {
		t.Fatalf("origin after a global insteadOf = %q", got)
	}
}

func TestRememberedTopLevelFollowsRepositoryLayout(t *testing.T) {
	repo, _ := history(t)
	nested := filepath.Join(repo, "docs", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	top := func(dir string) string {
		t.Helper()
		value, err := TopLevel(context.Background(), dir)
		if err != nil {
			return "error"
		}
		return value
	}
	want := git(t, repo, "rev-parse", "--show-toplevel")
	if got := top(nested); got != want {
		t.Fatalf("TopLevel = %q; want %q", got, want)
	}
	// A repository created between the directory and the old top level
	// takes it over.
	git(t, filepath.Join(repo, "docs"), "init", "-q")
	inner := git(t, filepath.Join(repo, "docs"), "rev-parse", "--show-toplevel")
	if got := top(nested); got != inner {
		t.Fatalf("TopLevel after an inner init = %q; want %q", got, inner)
	}
	if err := os.RemoveAll(filepath.Join(repo, "docs", ".git")); err != nil {
		t.Fatal(err)
	}
	if got := top(nested); got != want {
		t.Fatalf("TopLevel after removing the inner repository = %q; want %q", got, want)
	}
	if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	if got := top(nested); got == want {
		t.Fatal("TopLevel still answered after the repository was removed")
	}
}

func TestRefCacheableRevisions(t *testing.T) {
	for revision, want := range map[string]bool{
		"HEAD": true, "main": true, "feature/x": true, "HEAD~2": true, "main^1": true, "v1.0": true,
		strings.Repeat("a", 40): false, "abc1234": false, "abc1234~1": false, "HEAD@{1}": false,
		"@{upstream}": false, "HEAD:path": false, "": false, "deadbeef": false, "bad": true,
	} {
		if got := refCacheable(revision); got != want {
			t.Errorf("refCacheable(%q) = %v; want %v", revision, got, want)
		}
	}
}

// With nothing changed, a later command answers from memory without
// starting Git at all.
func TestRememberedAnswersStartNoGitWhenNothingChanged(t *testing.T) {
	repo, commits := history(t)
	git(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	command := func() {
		t.Helper()
		ctx, end := Begin(context.Background())
		defer end()
		if top, err := TopLevel(ctx, repo); err != nil || top == "" {
			t.Fatalf("TopLevel = %q, %v", top, err)
		}
		if _, err := ConfigOutput(ctx, repo, "remote", "get-url", "origin"); err != nil {
			t.Fatal(err)
		}
		if got, ok := ResolveCommit(ctx, repo, "main"); !ok || got != commits[3] {
			t.Fatalf("main = %q, %v", got, ok)
		}
	}
	spawns := func() int {
		data, _ := os.ReadFile(trace)
		return strings.Count(string(data), "trace: built-in: git")
	}
	command()
	first := spawns()
	if first == 0 {
		t.Fatal("the first command started no Git; the trace is not counting")
	}
	command()
	if again := spawns() - first; again != 0 {
		t.Fatalf("an unchanged repository cost the second command %d Git processes", again)
	}
}
