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

// Configuration counts every file Git reads: included files, followed
// through relative paths and nested includes, and the system file. An edit
// to any of them reaches the next command.
func TestRememberedConfigurationFollowsIncludedAndSystemFiles(t *testing.T) {
	repo, _ := history(t)
	git(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	write(t, filepath.Join(dir, "first"), "[includeIf \"gitdir:/nowhere/\"]\n\tpath = nested\n[include]\n\tpath = nested ; a comment\n")
	write(t, filepath.Join(dir, "gitconfig"), "[include]\n\tpath = \""+filepath.ToSlash(filepath.Join(dir, "first"))+"\"\n")
	system := filepath.Join(dir, "system")
	write(t, system, "")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, "gitconfig"))
	t.Setenv("GIT_CONFIG_SYSTEM", system)
	origin := func() string {
		t.Helper()
		ctx, end := Begin(context.Background())
		defer end()
		output, err := ConfigOutput(ctx, repo, "remote", "get-url", "origin")
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(output))
	}
	rule := func(mirror string) string {
		return "[url \"" + mirror + "\"]\n\tinsteadOf = https://example.test/\n"
	}
	write(t, nested, rule("https://one.test/"))
	if got := origin(); got != "https://one.test/a.git" {
		t.Fatalf("origin = %q", got)
	}
	write(t, nested, rule("https://two.test/"))
	if got := origin(); got != "https://two.test/a.git" {
		t.Fatalf("origin after editing a nested include = %q", got)
	}
	write(t, nested, "")
	write(t, system, rule("https://system.test/"))
	if got := origin(); got != "https://system.test/a.git" {
		t.Fatalf("origin after editing the system file = %q", got)
	}
}

// core.worktree moves the top level without touching the directory tree.
func TestRememberedTopLevelFollowsConfiguration(t *testing.T) {
	repo, _ := history(t)
	elsewhere := t.TempDir()
	first, err := TopLevel(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	git(t, repo, "config", "core.worktree", elsewhere)
	moved, err := TopLevel(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if moved == first {
		t.Fatalf("TopLevel stayed %q after core.worktree moved it", first)
	}
	if want := git(t, repo, "rev-parse", "--show-toplevel"); moved != want {
		t.Fatalf("TopLevel = %q; want %q", moved, want)
	}
}

func TestIncludedConfigFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "config")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := includedConfigFiles(file, []byte("[user]\n\tname = path = no\n[Include]\n\tPath = relative\n[includeIf \"onbranch:main\"] path = ~/cond # note\n[include]\n\tpath = \"/abs/quoted\"\n"))
	want := []string{filepath.Join(dir, "relative"), filepath.Join(home, "cond"), filepath.FromSlash("/abs/quoted")}
	if !ok || strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("includedConfigFiles = %q, %v; want %q", got, ok, want)
	}
	for _, unsure := range []string{"[include]\n\tpath = a\\\\b\n", "[include]\n\tpath = %(prefix)/etc/x\n", "[include\n"} {
		if _, ok := includedConfigFiles(file, []byte(unsure)); ok {
			t.Fatalf("includedConfigFiles accepted %q, which it cannot resolve exactly", unsure)
		}
	}
}

// GIT_TRACE and Git's warnings write to standard error; they must not
// break finding the repository.
func TestTopLevelIgnoresStandardError(t *testing.T) {
	repo, _ := history(t)
	want := git(t, repo, "rev-parse", "--show-toplevel")
	t.Setenv("GIT_TRACE", "1")
	got, err := TopLevel(context.Background(), repo)
	if err != nil {
		t.Fatalf("TopLevel with GIT_TRACE=1: %v", err)
	}
	if got != want {
		t.Fatalf("TopLevel = %q; want %q", got, want)
	}
}

// Reached through a symlink, a checkout's git directories must still be
// found where they are. Placed wrongly, the digest hashes missing files and
// never sees the repository's configuration or packed refs change.
func TestRememberedAnswersThroughASymlinkFollowTheRepository(t *testing.T) {
	repo, commits := history(t)
	git(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	inner := filepath.Join(repo, "d", "e")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(inner, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	origin := func() string {
		t.Helper()
		ctx, end := Begin(context.Background())
		defer end()
		output, err := ConfigOutput(ctx, link, "remote", "get-url", "origin")
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(output))
	}
	if got := origin(); got != "https://example.test/a.git" {
		t.Fatalf("origin through the link = %q", got)
	}
	git(t, repo, "remote", "set-url", "origin", "https://example.test/b.git")
	if got := origin(); got != "https://example.test/b.git" {
		t.Fatalf("origin through the link after set-url = %q", got)
	}
	// Packed refs live in the common directory too.
	git(t, repo, "branch", "-q", "feature", commits[1])
	git(t, repo, "pack-refs", "--all")
	if got, ok := resolveIn(t, link, "feature"); !ok || got != commits[1] {
		t.Fatalf("feature through the link = %q, %v", got, ok)
	}
	git(t, repo, "branch", "-q", "-f", "feature", commits[2])
	git(t, repo, "pack-refs", "--all")
	if got, ok := resolveIn(t, link, "feature"); !ok || got != commits[2] {
		t.Fatalf("feature through the link after repacking = %q, %v; want %s", got, ok, commits[2])
	}
}

// A configuration change while Git answers must not be remembered as the
// answer for the configuration that follows it.
func TestDiscoveryIsNotRememberedAcrossAConcurrentChange(t *testing.T) {
	repo, _ := history(t)
	elsewhere := t.TempDir()
	afterDiscovery = func() { git(t, repo, "config", "core.worktree", elsewhere) }
	t.Cleanup(func() { afterDiscovery = nil })
	if _, err := TopLevel(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	afterDiscovery = nil
	abs, _ := filepath.Abs(repo)
	if _, ok := locations.Load(gitEnvironment() + "\x00" + abs); ok {
		t.Fatal("an answer given before a configuration change was remembered under the changed configuration")
	}
	got, err := TopLevel(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if want := git(t, repo, "rev-parse", "--show-toplevel"); got != want {
		t.Fatalf("TopLevel after core.worktree moved = %q; want %q", got, want)
	}
}

// An answer given while refs move must not be filed under the state before
// the move: the repository can return to that state, and the answer would
// then describe where it went instead.
func TestRefAnswersAreNotFiledAcrossAConcurrentChange(t *testing.T) {
	repo, commits := history(t)
	git(t, repo, "branch", "-q", "feature", commits[1])
	ctx, end := Begin(context.Background())
	defer end()
	// The session takes its digest while main is checked out.
	if got, ok := ResolveCommit(ctx, repo, "HEAD"); !ok || got != commits[3] {
		t.Fatalf("HEAD = %q, %v", got, ok)
	}
	// Another key is answered after a checkout moves HEAD to feature.
	moved, err := rememberRefs(ctx, repo, true, []string{"test", t.Name()}, func() ([]byte, error) {
		git(t, repo, "checkout", "-q", "feature")
		return []byte(git(t, repo, "rev-parse", "HEAD")), nil
	})
	if err != nil || string(moved) != commits[1] {
		t.Fatalf("answer = %q, %v", moved, err)
	}
	git(t, repo, "checkout", "-q", "main")
	again, err := rememberRefs(context.Background(), repo, true, []string{"test", t.Name()}, func() ([]byte, error) {
		return []byte(git(t, repo, "rev-parse", "HEAD")), nil
	})
	if err != nil || string(again) != commits[3] {
		t.Fatalf("with main checked out again the answer was %q; want %s", again, commits[3])
	}
}
