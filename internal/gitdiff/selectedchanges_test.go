package gitdiff

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/snapshotcache"
)

func TestSelectedChangesPreserveGlobalRenamesAndFileGeometry(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := newGitTestRepo(t)
	var original strings.Builder
	for line := range 40 {
		fmt.Fprintf(&original, "unique original line %d\n", line)
	}
	unusualPaths := []string{"a spaced path.go"}
	// Windows forbids double quotes in filenames. Keep space handling covered
	// everywhere and exercise Git's quoted-path escaping where it is supported.
	if runtime.GOOS != "windows" {
		unusualPaths = append(unusualPaths, "a \" quoted.go")
	}
	paths := append([]string{"src/rename.go", "src/edit.go", "gone.go", "mode.sh", "binary.dat"}, unusualPaths...)
	for _, path := range paths {
		writeGitTestFile(t, filepath.Join(repo, path), original.String())
	}
	writeGitTestFile(t, filepath.Join(repo, "binary.dat"), "\x00one\n")
	writeGitTestFile(t, filepath.Join(repo, "unrelated.go"), "unrelated\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "mv", "src/rename.go", "src/moved.go")
	writeGitTestFile(t, filepath.Join(repo, "src", "moved.go"), "inserted\n"+original.String())
	// Reoccupying the old name must not steal the original file's reference.
	writeGitTestFile(t, filepath.Join(repo, "src", "rename.go"), "entirely new file at the old path\n")
	writeGitTestFile(t, filepath.Join(repo, "src", "edit.go"), "inserted\n"+original.String())
	for _, path := range unusualPaths {
		writeGitTestFile(t, filepath.Join(repo, path), original.String()+"appended\n")
	}
	writeGitTestFile(t, filepath.Join(repo, "binary.dat"), "\x00two\n")
	writeGitTestFile(t, filepath.Join(repo, "unrelated.go"), "different unrelated code\n")
	if err := os.Remove(filepath.Join(repo, "gone.go")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "update-index", "--chmod=+x", "mode.sh")
	gitTest(t, repo, "commit", "-m", "edit, rename, replace, delete, mode, binary")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	for _, attributes := range []string{"", "src/rename.go -diff\n", "src/moved.go -diff\n", "*.go diff=custom\n"} {
		writeGitTestFile(t, filepath.Join(repo, ".gitattributes"), attributes)
		gitTest(t, repo, "config", "diff.custom.binary", "true")
		for _, batched := range []bool{false, true} {
			ctx := context.Background()
			end := func() {}
			if batched {
				ctx, end = gitexec.Begin(ctx)
			}
			full, err := TreeChanges(ctx, repo, base, head)
			if err != nil {
				end()
				t.Fatal(err)
			}
			selected, err := TreeChangesForPaths(ctx, repo, base, head, paths)
			end()
			if err != nil {
				t.Fatal(err)
			}
			byOld := map[string]FileChange{}
			for _, change := range full {
				if change.OldPath != "" {
					if change.Deleted {
						change.Hunks = nil // deletion itself establishes staleness
					}
					byOld[change.OldPath] = change
				}
			}
			if len(selected) != len(paths) {
				t.Fatalf("selected %d records, want %d: %+v", len(selected), len(paths), selected)
			}
			for _, change := range selected {
				if !reflect.DeepEqual(change, byOld[change.OldPath]) {
					t.Fatalf("attributes=%q batched=%v: selected %+v, full %+v", attributes, batched, change, byOld[change.OldPath])
				}
			}
		}
	}
}

func TestSelectedChangesRebuildWhenTheRequestedPathsExpand(t *testing.T) {
	repo, base, head := treeCacheFixture(t)
	first, err := TreeChangesForPaths(context.Background(), repo, base, head, []string{"not-referenced.txt"})
	if err != nil || len(first) != 0 {
		t.Fatalf("empty selection: %+v, %v", first, err)
	}
	second, err := TreeChangesForPaths(context.Background(), repo, base, head, []string{"not-referenced.txt", "sub/data.txt"})
	if err != nil || len(second) != 1 || len(second[0].Hunks) == 0 {
		t.Fatalf("expanded selection reused the narrower result: %+v, %v", second, err)
	}
}

func TestSelectedChangesPreserveTypeChangeSemantics(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "typed"), "original regular file\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "regular file")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	if err := os.Remove(filepath.Join(repo, "typed")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target", filepath.Join(repo, "typed")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "symlink")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	full, err := TreeChanges(context.Background(), repo, base, head)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := TreeChangesForPaths(context.Background(), repo, base, head, []string{"typed"})
	if err != nil || !reflect.DeepEqual(selected, full) {
		t.Fatalf("type change: selected %+v, %v; full %+v", selected, err, full)
	}
}

func TestSelectedChangesHandleManyLongPathsAcrossArgumentChunks(t *testing.T) {
	t.Setenv(snapshotcache.DirEnv, t.TempDir())
	repo := newGitTestRepo(t)
	var paths []string
	for index := range 180 {
		path := fmt.Sprintf("file-%03d-%s.txt", index, strings.Repeat("p", 75))
		paths = append(paths, path)
		writeGitTestFile(t, filepath.Join(repo, path), "before\n")
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "before")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	for _, path := range paths {
		writeGitTestFile(t, filepath.Join(repo, path), "after\n")
	}
	gitTest(t, repo, "commit", "-am", "after")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	changes, err := TreeChangesForPaths(context.Background(), repo, base, head, paths)
	if err != nil || len(changes) != len(paths) {
		t.Fatalf("selected %d of %d paths: %v", len(changes), len(paths), err)
	}
	for _, change := range changes {
		if len(change.Hunks) != 1 || change.Hunks[0].OldCount != 1 || change.Hunks[0].NewCount != 1 {
			t.Fatalf("missing geometry for %q: %+v", change.OldPath, change.Hunks)
		}
	}
}

// These benchmarks measure the uncached algorithm: all data comes from Git
// afresh, with one small referenced file and increasing unrelated patch size.
func BenchmarkSelectedChanges(b *testing.B) {
	for _, lines := range []int{10_000, 100_000, 1_000_000} {
		b.Run(fmt.Sprintf("unrelated-lines-%d", lines), func(b *testing.B) {
			repo := b.TempDir()
			git := func(args ...string) string {
				b.Helper()
				data, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
				if err != nil {
					b.Fatalf("git: %v: %s", err, data)
				}
				return strings.TrimSpace(string(data))
			}
			git("init", "-q", "-b", "main")
			git("config", "user.name", "T")
			git("config", "user.email", "t@example.test")
			for _, prefix := range []string{"before", "after"} {
				var body strings.Builder
				for line := range lines {
					fmt.Fprintf(&body, "%s payload %08d\n", prefix, line)
				}
				if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte(body.String()), 0o644); err != nil {
					b.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(repo, "referenced.txt"), []byte(prefix+"\n"), 0o644); err != nil {
					b.Fatal(err)
				}
				git("add", ".")
				git("commit", "-qm", prefix)
			}
			base, head := git("rev-parse", "HEAD~1"), git("rev-parse", "HEAD")
			for _, selected := range []bool{false, true} {
				name := "full-patches"
				if selected {
					name = "selected-patches"
				}
				b.Run(name, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						ctx, end := gitexec.Begin(context.Background())
						var err error
						if selected {
							_, err = selectedChangesOnce(ctx, repo, base, head, []string{"referenced.txt"})
						} else {
							_, err = treeChangesOnce(ctx, repo, base, head)
						}
						end()
						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
