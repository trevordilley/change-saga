package server

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// HEAD read from the repository's files is what git rev-parse HEAD reports,
// on a branch, detached, after the branch's ref is packed, in a linked
// worktree, and before the first commit.
func TestHeadReaderAgreesWithGit(t *testing.T) {
	root := t.TempDir()
	git := func(dir string, args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(repo, "init", "-q", "-b", "main")
	check := func(name, dir, want string) {
		t.Helper()
		reader := &headReader{}
		if got := reader.head(context.Background(), dir); got != want {
			t.Fatalf("%s: HEAD read as %q, git says %q", name, got, want)
		}
		if _, direct := reader.read(); want != "" && (!reader.plain || !direct) {
			t.Fatalf("%s: HEAD was read by asking git rather than from the repository's files", name)
		}
	}
	check("unborn branch", repo, "")
	writeServerFile(t, filepath.Join(repo, "a.txt"), "a\n")
	git(repo, "add", ".")
	git(repo, "commit", "-q", "-m", "first")
	check("branch", repo, git(repo, "rev-parse", "HEAD"))
	git(repo, "pack-refs", "--all")
	if _, err := os.Stat(filepath.Join(repo, ".git", "refs", "heads", "main")); !os.IsNotExist(err) {
		t.Fatalf("the branch ref was not packed: %v", err)
	}
	check("packed branch", repo, git(repo, "rev-parse", "HEAD"))
	writeServerFile(t, filepath.Join(repo, "b.txt"), "b\n")
	git(repo, "add", ".")
	git(repo, "commit", "-q", "-m", "second")
	check("loose ref over a packed one", repo, git(repo, "rev-parse", "HEAD"))
	first := git(repo, "rev-parse", "HEAD~1")
	git(repo, "checkout", "-q", "--detach", first)
	check("detached", repo, first)
	git(repo, "checkout", "-q", "main")
	linked := filepath.Join(root, "linked")
	git(repo, "worktree", "add", "-q", "-b", "side", linked, first)
	check("linked worktree", linked, first)
	check("subdirectory", filepath.Join(linked), first)
	check("not a repository", t.TempDir(), "")
	// One reader follows HEAD as it moves.
	reader := &headReader{}
	before := reader.head(context.Background(), repo)
	git(repo, "commit", "-q", "--allow-empty", "-m", "third")
	if after := reader.head(context.Background(), repo); after == before || after != git(repo, "rev-parse", "HEAD") {
		t.Fatalf("the reader did not follow a new commit: before %s after %s", before, after)
	}
}
