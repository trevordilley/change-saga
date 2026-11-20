package currentrecord

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoricalContentComesFromGitByDigest(t *testing.T) {
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Saga test", "-c", "user.email=saga@example.test"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	legacy(t, root, "r1", nil, "Original")
	state(t, root)
	git("add", ".")
	git("commit", "--quiet", "-m", "Original content")
	original := git("rev-parse", "HEAD")
	index, err := AuditPins(root)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := index.Resolve("urn:change-saga:test:story:a:revision:r1")
	if err != nil {
		t.Fatal(err)
	}
	legacy(t, root, "r2", []string{"r1"}, "Edited")
	git("add", ".")
	git("commit", "--quiet", "-m", "Current content")
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "Git owns history")
	found, err := FindGitContent(root, "a.story", pin)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(found.Content), "Original") || found.Commit == "" || found.File == "" {
		t.Fatalf("historical content: %+v", found)
	}
	_ = original
	if _, err := FindGitContent(root, "a.story", "sha256:"+strings.Repeat("f", 64)); err == nil {
		t.Fatal("unknown content silently resolved to current")
	}
}

func TestHistoricalPinsFollowSagaMovesAndRespectSavedCommit(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Saga test", "-c", "user.email=saga@example.test"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet", "-b", "main")
	root := filepath.Join(repo, "app.saga")
	put(t, filepath.Join(root, "saga.json"), map[string]any{"id": "test"})
	legacy(t, root, "r1", nil, "Original")
	state(t, root)
	git("add", ".")
	git("commit", "--quiet", "-m", "Original")
	first := git("rev-parse", "HEAD")
	pins, _ := AuditPins(root)
	old, err := pins.Resolve("urn:change-saga:test:story:a:revision:r1")
	if err != nil {
		t.Fatal(err)
	}
	legacy(t, root, "r2", []string{"r1"}, "Edited")
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	record, _ := Read(filepath.Join(root, "a.story"))
	newPin, _ := Digest(record.Content)
	git("add", ".")
	git("commit", "--quiet", "-m", "Normalize")
	git("mv", "app.saga", "change.saga")
	git("commit", "--quiet", "-m", "Move")
	found, err := FindGitContent(filepath.Join(repo, "change.saga"), "a.story", old)
	if err != nil || found.Commit != first || !strings.Contains(string(found.Content), "Original") {
		t.Fatalf("historical pin across move: %+v %v", found, err)
	}
	if _, err := FindGitContentAt(GitSource{Repository: repo, SagaPath: "change.saga", Commit: first}, "a.story", newPin); err == nil {
		t.Fatal("a saved view read content introduced by a future commit")
	}
}
