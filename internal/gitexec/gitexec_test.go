package gitexec

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// history builds commits exercising what a diff can report: additions,
// edits, a rename, a binary file, a deletion, and a change under .saga.
func history(t *testing.T) (repo string, commits []string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "config", "user.name", "Test")
	git(t, repo, "config", "user.email", "test@example.test")
	commit := func(message string) {
		git(t, repo, "add", "-A")
		git(t, repo, "commit", "-q", "--allow-empty", "-m", message)
		commits = append(commits, git(t, repo, "rev-parse", "HEAD"))
	}
	write(t, filepath.Join(repo, "a.go"), "package a\n\nconst A = 1\nconst B = 2\n")
	write(t, filepath.Join(repo, "docs", "notes.md"), strings.Repeat("line\n", 40))
	commit("base")
	write(t, filepath.Join(repo, "a.go"), "package a\n\nconst A = 10\nconst B = 2\nconst C = 3\n")
	write(t, filepath.Join(repo, "app.saga", "record.json"), "{}\n")
	commit("edit")
	git(t, repo, "mv", "docs/notes.md", "docs/renamed notes.md")
	write(t, filepath.Join(repo, "image.bin"), "\x00\x01\x02binary\x00")
	commit("rename")
	if err := os.Remove(filepath.Join(repo, "a.go")); err != nil {
		t.Fatal(err)
	}
	commit("delete")
	git(t, repo, "tag", "-a", "-m", "release", "v1", commits[1])
	return repo, commits
}

var diffConfig = []string{
	"-c", "core.quotePath=true", "-c", "diff.algorithm=myers", "-c", "diff.renames=true",
}

var diffFlags = []string{"--no-color", "--no-ext-diff", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/", "--find-renames=50%"}

// The batch process must print exactly what one `git diff` per pair prints,
// because coverage is anchored to those bytes.
func TestDiffTreeMatchesGitDiff(t *testing.T) {
	repo, commits := history(t)
	ctx, end := Begin(context.Background())
	defer end()
	for _, mode := range [][]string{{"-p", "--unified=0"}, {"-p", "--unified=20"}, {"--numstat", "-z"}} {
		for _, pathspec := range [][]string{nil, {".", ":(exclude,glob)**/*.saga/**"}} {
			treeArgs := append(append(append(append([]string{}, diffConfig...), "-C", repo, "diff-tree", "--stdin", "--no-commit-id", "-r"), diffFlags...), mode...)
			treeArgs = append(append(treeArgs, "--"), pathspec...)
			for _, pair := range [][2]string{{commits[0], commits[1]}, {commits[1], commits[2]}, {commits[0], commits[3]}, {commits[3], commits[0]}, {commits[2], commits[2]}} {
				diffArgs := append(append(append(append([]string{}, diffConfig...), "-C", repo, "diff"), diffFlags...), mode[len(mode)-1])
				if mode[0] == "--numstat" {
					diffArgs = append(diffArgs, "--numstat")
				}
				diffArgs = append(append(diffArgs, pair[0], pair[1], "--"), pathspec...)
				want, err := exec.Command("git", diffArgs...).Output()
				if err != nil {
					t.Fatal(err)
				}
				got, ok := DiffTree(ctx, treeArgs, pair[0], pair[1])
				if !ok {
					t.Fatalf("DiffTree %v %v could not answer", mode, pair)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("DiffTree %v %v %v differs from git diff:\n--- got\n%s\n--- want\n%s", mode, pathspec, pair, got, want)
				}
			}
		}
	}
}

func TestDiffTreeDeclinesWhatItCannotAnswer(t *testing.T) {
	repo, commits := history(t)
	args := []string{"-C", repo, "diff-tree", "--stdin", "--no-commit-id", "-r", "-p"}
	if _, ok := DiffTree(context.Background(), args, commits[0], commits[1]); ok {
		t.Fatal("DiffTree answered without a session")
	}
	ctx, end := Begin(context.Background())
	defer end()
	if _, ok := DiffTree(ctx, args, "main", commits[1]); ok {
		t.Fatal("DiffTree answered for a revision that is not a full object name")
	}
	if _, ok := DiffTree(ctx, args, strings.Repeat("a", 40), commits[1]); ok {
		t.Fatal("DiffTree answered for a missing commit")
	}
	// The failed process is replaced rather than poisoning the session.
	if _, ok := DiffTree(ctx, args, commits[0], commits[1]); !ok {
		t.Fatal("DiffTree did not recover after a failed request")
	}
}

func TestResolveCommitMatchesRevParse(t *testing.T) {
	repo, commits := history(t)
	ctx, end := Begin(context.Background())
	defer end()
	for _, revision := range []string{"HEAD", "main", "HEAD~2", "v1", commits[0], commits[0][:12], "refs/heads/main"} {
		want := git(t, repo, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
		got, ok := ResolveCommit(ctx, repo, revision)
		if !ok || got != want {
			t.Fatalf("ResolveCommit(%q) = %q, %v; want %q", revision, got, ok, want)
		}
	}
	blob := git(t, repo, "rev-parse", commits[0]+":a.go")
	for _, revision := range []string{"missing", strings.Repeat("a", 40), blob, "--help", "HEAD\nHEAD", ""} {
		if got, ok := ResolveCommit(ctx, repo, revision); ok {
			t.Fatalf("ResolveCommit(%q) = %q; want no answer", revision, got)
		}
	}
	// A declined request does not break later ones.
	if got, ok := ResolveCommit(ctx, repo, "HEAD"); !ok || got != commits[3] {
		t.Fatalf("ResolveCommit after a miss = %q, %v", got, ok)
	}
}

func TestOutputIsMemoizedWithinASessionOnly(t *testing.T) {
	repo, _ := history(t)
	counter := filepath.Join(t.TempDir(), "count")
	// GIT_TRACE logs one line per Git process, which counts the spawns.
	t.Setenv("GIT_TRACE", counter)
	spawns := func() int {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "trace: built-in: git rev-parse")
	}
	ctx, end := Begin(context.Background())
	for range 3 {
		output, err := Output(ctx, "-C", repo, "rev-parse", "--show-toplevel")
		if err != nil || strings.TrimSpace(string(output)) == "" {
			t.Fatalf("Output = %q, %v", output, err)
		}
	}
	_, missingErr := CombinedOutput(ctx, "-C", repo, "rev-parse", "--verify", "missing")
	_, repeatErr := CombinedOutput(ctx, "-C", repo, "rev-parse", "--verify", "missing")
	end()
	if missingErr == nil || repeatErr == nil {
		t.Fatal("a failing query succeeded")
	}
	if got := spawns(); got != 2 {
		t.Fatalf("a session spawned rev-parse %d times; want one per distinct query", got)
	}
	for range 2 {
		if _, err := Output(context.Background(), "-C", repo, "rev-parse", "--show-toplevel"); err != nil {
			t.Fatal(err)
		}
	}
	if got := spawns(); got != 4 {
		t.Fatalf("without a session rev-parse ran %d times in total; want every call to spawn", got)
	}
}

func TestNestedBeginSharesTheSessionAndEndStopsProcesses(t *testing.T) {
	repo, _ := history(t)
	outer, end := Begin(context.Background())
	inner, innerEnd := Begin(outer)
	if sessionFrom(inner) != sessionFrom(outer) {
		t.Fatal("a nested Begin started a second session")
	}
	if _, ok := ResolveCommit(inner, repo, "HEAD"); !ok {
		t.Fatal("ResolveCommit failed")
	}
	innerEnd()
	session := sessionFrom(outer)
	if len(session.batches) != 1 {
		t.Fatalf("the nested end stopped the shared session's processes: %d live", len(session.batches))
	}
	var processes []*batch
	for _, process := range session.batches {
		processes = append(processes, process)
	}
	end()
	for _, process := range processes {
		select {
		case <-process.exited:
		case <-time.After(10 * time.Second):
			t.Fatal("ending the session left its Git process running")
		}
	}
	if _, ok := ResolveCommit(outer, repo, "HEAD"); ok {
		t.Fatal("an ended session started a new process")
	}
}

func TestCancelledRequestDoesNotHang(t *testing.T) {
	repo, _ := history(t)
	session, end := Begin(context.Background())
	defer end()
	ctx, cancel := context.WithCancel(session)
	cancel()
	if _, ok := ResolveCommit(ctx, repo, "HEAD"); ok {
		t.Fatal("a cancelled request was answered")
	}
	if got, ok := ResolveCommit(session, repo, "HEAD"); !ok || got == "" {
		t.Fatal("the session did not recover after a cancelled request")
	}
}

func TestReadObjectMatchesCatFile(t *testing.T) {
	repo, commits := history(t)
	ctx, end := Begin(context.Background())
	defer end()
	for _, name := range []string{commits[0] + ":a.go", commits[2] + ":image.bin", commits[2] + ":docs", commits[1]} {
		wantType := git(t, repo, "cat-file", "-t", name)
		want, err := exec.Command("git", "-C", repo, "cat-file", wantType, name).Output()
		if err != nil {
			t.Fatal(err)
		}
		gotType, got, ok := ReadObject(ctx, repo, name)
		if !ok || gotType != wantType || !bytes.Equal(got, want) {
			t.Fatalf("ReadObject(%q) = %q, %q, %v; want %q, %q", name, gotType, got, ok, wantType, want)
		}
	}
	for _, name := range []string{commits[0] + ":absent", commits[0] + ":name with spaces"} {
		if gotType, _, ok := ReadObject(ctx, repo, name); !ok || gotType != "missing" {
			t.Fatalf("ReadObject(%q) = %q, %v; want missing", name, gotType, ok)
		}
	}
	// Revisions and objects share one cat-file process per repository.
	if _, ok := ResolveCommit(ctx, repo, "HEAD"); !ok {
		t.Fatal("ResolveCommit failed after reads")
	}
	if got := len(sessionFrom(ctx).batches); got != 1 {
		t.Fatalf("the session runs %d cat-file processes for one repository; want 1", got)
	}
	if _, _, ok := ReadObject(context.Background(), repo, commits[0]); ok {
		t.Fatal("ReadObject answered without a session")
	}
}

// A Git that cannot serve an invocation falls back to one-shot commands
// instead of starting a process for every question.
func TestBrokenBatchInvocationIsRetiredAfterRepeatedFailures(t *testing.T) {
	repo, commits := history(t)
	ctx, end := Begin(context.Background())
	defer end()
	args := []string{"-C", repo, "diff-tree", "--stdin", "--no-such-option"}
	for range maxBatchFailures + 2 {
		if _, ok := DiffTree(ctx, args, commits[0], commits[1]); ok {
			t.Fatal("an invocation Git rejects answered")
		}
	}
	session := sessionFrom(ctx)
	if got := session.failures[strings.Join(args, "\x00")]; got != maxBatchFailures {
		t.Fatalf("failures = %d; want the invocation retired after %d", got, maxBatchFailures)
	}
	if len(session.batches) != 0 {
		t.Fatalf("a retired invocation left %d processes", len(session.batches))
	}
}
