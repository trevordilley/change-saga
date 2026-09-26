package coderesolve

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

func TestRemap(t *testing.T) {
	cases := []struct {
		name       string
		hunks      []gitdiff.Hunk
		start, end int
		wantStart  int
		wantEnd    int
		ok         bool
	}{
		{"no hunks", nil, 5, 9, 5, 9, true},
		{"insert before", []gitdiff.Hunk{{OldStart: 2, OldCount: 0, NewStart: 3, NewCount: 3}}, 5, 9, 8, 12, true},
		{"insert just before", []gitdiff.Hunk{{OldStart: 4, OldCount: 0, NewStart: 5, NewCount: 1}}, 5, 9, 6, 10, true},
		{"insert just after", []gitdiff.Hunk{{OldStart: 9, OldCount: 0, NewStart: 10, NewCount: 2}}, 5, 9, 5, 9, true},
		{"insert inside", []gitdiff.Hunk{{OldStart: 6, OldCount: 0, NewStart: 7, NewCount: 1}}, 5, 9, 0, 0, false},
		{"delete before", []gitdiff.Hunk{{OldStart: 1, OldCount: 2, NewStart: 0, NewCount: 0}}, 5, 9, 3, 7, true},
		{"replace before", []gitdiff.Hunk{{OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 5}}, 5, 9, 8, 12, true},
		{"change first line", []gitdiff.Hunk{{OldStart: 5, OldCount: 1, NewStart: 5, NewCount: 1}}, 5, 9, 0, 0, false},
		{"change after", []gitdiff.Hunk{{OldStart: 10, OldCount: 1, NewStart: 10, NewCount: 4}}, 5, 9, 5, 9, true},
	}
	for _, test := range cases {
		start, end, ok := Remap(test.hunks, test.start, test.end)
		if ok != test.ok || ok && (start != test.wantStart || end != test.wantEnd) {
			t.Errorf("%s: got %d-%d %v, want %d-%d %v", test.name, start, end, ok, test.wantStart, test.wantEnd, test.ok)
		}
	}
}

func TestResolveRemapsMovedLinesAndReportsChangedLinesStale(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "app.go", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\n")
	pin := commit(t, repo, "pin")

	resolver, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "app.go", Start: 3, End: 4}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve(ctx, reference, pin); !got.Current() || got.Moved {
		t.Fatalf("at pin: %+v", got)
	}

	// Only a Saga changes: the code is identical, so nothing moves.
	write(t, repo, "x.saga/saga.json", "{}\n")
	sagaOnly := commit(t, repo, "saga only")
	if got := resolver.Resolve(ctx, reference, sagaOnly); !got.Current() || got.Moved || got.Location.Start != 3 {
		t.Fatalf("saga-only commit: %+v", got)
	}

	// Unrelated insertion above the range shifts it.
	write(t, repo, "app.go", "new1\nnew2\na\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\n")
	shifted := commit(t, repo, "shift")
	got := resolver.Resolve(ctx, reference, shifted)
	if !got.Current() || !got.Moved || got.Location.Start != 5 || got.Location.End != 6 {
		t.Fatalf("shifted: %+v", got)
	}

	// A rename with the same content keeps the reference current at the new path.
	git(t, repo, "mv", "app.go", "main.go")
	renamed := commit(t, repo, "rename")
	got = resolver.Resolve(ctx, reference, renamed)
	if !got.Current() || got.Location.Path != "main.go" || got.Location.Start != 5 {
		t.Fatalf("renamed: %+v", got)
	}

	// Changing a referenced line makes it stale, with a reason and a diff.
	write(t, repo, "main.go", "new1\nnew2\na\nb\nC!\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\n")
	changed := commit(t, repo, "change")
	got = resolver.Resolve(ctx, reference, changed)
	if got.Current() || !strings.Contains(got.Reason, "lines 3-4 of app.go changed") {
		t.Fatalf("changed: %+v", got)
	}
	diff, err := resolver.DiffSince(ctx, reference, changed)
	if err != nil || !strings.Contains(diff, "+C!") {
		t.Fatalf("diff since pin: %v %q", err, diff)
	}

	// A tampered digest is stale at the pin itself.
	tampered := reference
	tampered.Digest = coderef.DigestBytes([]byte("other"))
	if got := resolver.Resolve(ctx, tampered, pin); got.Current() {
		t.Fatalf("tampered digest resolved: %+v", got)
	}
}

func TestResolveWholeFileAndMissingCommitFallback(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "logo.bin", "\x00\x01binary")
	write(t, repo, "lib.go", "one\ntwo\nthree\n")
	pin := commit(t, repo, "pin")
	resolver, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	file, err := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "logo.bin"}, "")
	if err != nil {
		t.Fatal(err)
	}
	write(t, repo, "logo.bin", "\x00\x02binary")
	next := commit(t, repo, "change binary")
	if got := resolver.Resolve(ctx, file, next); got.Current() {
		t.Fatalf("changed binary resolved: %+v", got)
	}

	lines, _ := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "lib.go", Start: 2, End: 3}, "")
	lines.Commit = strings.Repeat("f", 40)
	write(t, repo, "lib.go", "zero\none\ntwo\nthree\n")
	later := commit(t, repo, "shift")
	got := resolver.Resolve(ctx, lines, later)
	if !got.Current() || got.Location.Start != 3 || got.Location.End != 4 || !strings.Contains(got.Reason, "found by content digest") {
		t.Fatalf("digest fallback: %+v", got)
	}
}

// An answer that follows from the reference and the two commits alone is
// settled; one that rests on a missing pinned commit or a read that failed,
// such as one cut short by its caller, is provisional, since a fetch or a
// retry may answer it differently.
func TestResolveMarksAnswersThatMayChangeProvisional(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "main")
	write(t, repo, "lib.go", "one\ntwo\nthree\n")
	pin := commit(t, repo, "pin")
	write(t, repo, "lib.go", "one\nTWO\nthree\n")
	next := commit(t, repo, "change")
	resolver, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(ctx, coderef.Location{Commit: pin, Path: "lib.go", Start: 2, End: 2}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := resolver.Resolve(ctx, reference, next); got.Current() || got.Provisional {
		t.Fatalf("changed lines: %+v, want stale and settled", got)
	}
	if got := resolver.Resolve(ctx, reference, pin); !got.Current() || got.Provisional {
		t.Fatalf("at its pin: %+v, want current and settled", got)
	}
	missing := reference
	missing.Commit = strings.Repeat("f", 40)
	if got := resolver.Resolve(ctx, missing, next); !got.Provisional {
		t.Fatalf("missing pinned commit: %+v, want provisional", got)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	fresh, err := New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got := fresh.Resolve(cancelled, reference, next); got.Current() || !got.Provisional {
		t.Fatalf("cancelled read: %+v, want stale and provisional", got)
	}
}

func write(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, repo, message string) string {
	t.Helper()
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", message)
	return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

func git(t *testing.T, repo string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
