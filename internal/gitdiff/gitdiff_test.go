package gitdiff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitexec"
)

func TestParseLinesAndEvents(t *testing.T) {
	patch := []byte(`diff --git a/app.go b/app.go
index 1111111..2222222 100644
--- a/app.go
+++ b/app.go
@@ -2,2 +2,2 @@
-old one
-old two
+new one
+new two
diff --git a/old.go b/new.go
similarity index 100%
rename from old.go
rename to new.go
diff --git "a/script name.sh" "b/script name.sh"
old mode 100644
new mode 100755
diff --git a/logo.png b/logo.png
index 3333333..4444444 100644
Binary files a/logo.png and b/logo.png differ
`)
	atoms, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(atoms), 7; got != want {
		t.Fatalf("got %d atoms, want %d: %#v", got, want, atoms)
	}
	wants := []struct {
		key     string
		content string
	}{
		{"line:app.go:old:2", "old one"},
		{"line:app.go:old:3", "old two"},
		{"line:app.go:new:2", "new one"},
		{"line:app.go:new:3", "new two"},
		{"event:rename:new.go:old.go:new.go", ""},
		{"event:mode:script name.sh::", ""},
		{"event:binary:logo.png::", ""},
	}
	for i, want := range wants {
		if atoms[i].Key != want.key || atoms[i].Content != want.content {
			t.Errorf("atom %d = %#v, want key %q content %q", i, atoms[i], want.key, want.content)
		}
	}
}

func TestParseKeepsDisplayContextOutOfCoverageAtoms(t *testing.T) {
	patch := []byte(`diff --git a/app.go b/app.go
--- a/app.go
+++ b/app.go
@@ -8,3 +8,3 @@
 unchanged before
-old value
+new value
 unchanged after
`)
	atoms, lines, err := parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	if len(atoms) != 2 {
		t.Fatalf("coverage atoms = %d, want only added and removed lines", len(atoms))
	}
	if len(lines) != 4 || lines[0].Kind != "context" || lines[0].OldLine != 8 || lines[0].NewLine != 8 || lines[1].Kind != "old" || lines[2].Kind != "new" || lines[3].Kind != "context" {
		t.Fatalf("unexpected display lines: %#v", lines)
	}
}

func TestParseDoesNotConfuseChangedContentWithFileHeaders(t *testing.T) {
	patch := []byte(`diff --git a/comments.txt b/comments.txt
--- a/comments.txt
+++ b/comments.txt
@@ -1,2 +1,2 @@
--- old comment
-old tail
+++ new value
+new tail
`)
	atoms, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"line:comments.txt:old:1", "line:comments.txt:old:2",
		"line:comments.txt:new:1", "line:comments.txt:new:2",
	}
	if len(atoms) != len(want) {
		t.Fatalf("header-like source lines were lost: %#v", atoms)
	}
	for i := range want {
		if atoms[i].Key != want[i] {
			t.Errorf("atom %d = %q, want %q", i, atoms[i].Key, want[i])
		}
	}
}

func TestIsSagaPath(t *testing.T) {
	tests := map[string]bool{
		"pr-12.saga/title.md":              true,
		"docs/reviews/pr-12.saga/a/x.json": true,
		"docs/saga/title.md":               false,
		"service.saga.go":                  false,
	}
	for path, want := range tests {
		if got := IsSagaPath(path); got != want {
			t.Errorf("IsSagaPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestSagaOnlyCommitsLeaveProductAtomsUnchanged(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.test")
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeGitTestFile(t, filepath.Join(repo, "README.md"), "base\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "app.go"), "package app\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "product")
	productHead := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	before, err := Read(context.Background(), repo, "https://example.test/acme/app.git", base, productHead)
	if err != nil {
		t.Fatal(err)
	}
	writeGitTestFile(t, filepath.Join(repo, "pr-1.saga", "note.txt"), "review metadata\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "review")
	after, err := Read(context.Background(), repo, "https://example.test/acme/app.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// The head is now a commit, so its identity moves; the product atoms and
	// their line numbers do not, which is what keeps references current.
	if before.HeadOID == after.HeadOID || len(before.Atoms) != 2 || len(after.Atoms) != 2 || before.Atoms[0].Key != after.Atoms[0].Key || before.Atoms[1].Key != after.Atoms[1].Key {
		t.Fatalf("saga-only commit changed product atoms: before=%#v after=%#v", before, after)
	}
	if changes, err := TreeChanges(context.Background(), repo, productHead, after.HeadOID); err != nil || len(changes) != 0 {
		t.Fatalf("saga-only commit changed product code: %v %#v", err, changes)
	}
	if len(after.SagaChanges) != 2 {
		t.Fatalf("saga-only change should still be reported: %#v", after.SagaChanges)
	}
}

func TestReadCatalogMatchesComparisonIdentityWithoutPatchAtoms(t *testing.T) {
	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/catalog.git")
	writeGitTestFile(t, filepath.Join(repo, "rename-me.txt"), "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n")
	writeGitTestFile(t, filepath.Join(repo, "modify.txt"), "before\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	gitTest(t, repo, "mv", "rename-me.txt", "renamed.txt")
	writeGitTestFile(t, filepath.Join(repo, "modify.txt"), "after one\nafter two\n")
	writeGitTestFile(t, filepath.Join(repo, "review.saga", "note.md"), "review-only metadata\n")
	gitTest(t, repo, "add", "-A")
	gitTest(t, repo, "commit", "-m", "feature")

	changes, err := Read(context.Background(), repo, "https://example.test/acme/catalog.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadCatalog(context.Background(), repo, "https://example.test/acme/catalog.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Repository != changes.Repository || catalog.BaseOID != changes.BaseOID || catalog.HeadOID != changes.HeadOID {
		t.Fatalf("catalog identity differs from full comparison: catalog=%#v changes=%#v", catalog, changes)
	}
	if len(catalog.Files) != 2 {
		t.Fatalf("catalog files = %#v, want only the two product files", catalog.Files)
	}
	if got := catalog.Files[0]; got.Path != "modify.txt" || got.Added != 2 || got.Deleted != 1 {
		t.Fatalf("modified file summary = %#v", got)
	}
	if got := catalog.Files[1]; got.Path != "renamed.txt" || got.OldPath != "rename-me.txt" || got.NewPath != "renamed.txt" || got.Added != 0 || got.Deleted != 0 {
		t.Fatalf("rename summary = %#v", got)
	}
	selected, err := ReadFile(context.Background(), repo, catalog, catalog.Files[0])
	if err != nil {
		t.Fatal(err)
	}
	if selected.Repository != changes.Repository || selected.BaseOID != changes.BaseOID || selected.HeadOID != changes.HeadOID {
		t.Fatalf("selected-file identity differs from its catalog: %#v", selected)
	}
	if len(selected.Atoms) != 3 {
		t.Fatalf("selected-file atoms = %#v, want only modify.txt", selected.Atoms)
	}
	for _, atom := range selected.Atoms {
		if atom.Path != "modify.txt" {
			t.Fatalf("selected-file read leaked %q", atom.Path)
		}
	}
	renamed, err := ReadFile(context.Background(), repo, catalog, catalog.Files[1])
	if err != nil {
		t.Fatal(err)
	}
	if len(renamed.Atoms) != 1 || renamed.Atoms[0].Event != "rename" || renamed.Atoms[0].OldPath != "rename-me.txt" || renamed.Atoms[0].NewPath != "renamed.txt" {
		t.Fatalf("focused rename lost whole-comparison identity: %#v", renamed.Atoms)
	}
}

func TestParseNumstatHandlesBinaryAndNULTerminatedRenamePaths(t *testing.T) {
	files, err := parseNumstat([]byte("-\t-\timage.bin\x001\t2\t\x00old name.txt\x00new name.txt\x00"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].Binary || files[0].Path != "image.bin" {
		t.Fatalf("binary summary = %#v", files)
	}
	if got := files[1]; got.Path != "new name.txt" || got.OldPath != "old name.txt" || got.NewPath != "new name.txt" || got.Added != 1 || got.Deleted != 2 {
		t.Fatalf("rename summary = %#v", got)
	}
}

func TestParseFileLifecycleAndDefensiveModify(t *testing.T) {
	patch := []byte(`diff --git a/empty-new b/empty-new
new file mode 100644
index 0000000..e69de29
diff --git a/empty-old b/empty-old
deleted file mode 100644
index e69de29..0000000
diff --git a/node b/node
old mode 100644
new mode 120000
diff --git a/metadata-only b/metadata-only
index 1111111..2222222 100644
`)
	atoms, err := Parse(patch)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"event:add:empty-new::",
		"event:delete:empty-old::",
		"event:type-change:node::",
		"event:modify:metadata-only::",
	}
	if len(atoms) != len(want) {
		t.Fatalf("atoms = %#v, want keys %v", atoms, want)
	}
	for i := range want {
		if atoms[i].Key != want[i] {
			t.Errorf("atom %d key = %q, want %q", i, atoms[i].Key, want[i])
		}
	}
}

func TestAdversarialGitFixtureCorpus(t *testing.T) {
	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/corpus.git")
	writeGitTestFile(t, filepath.Join(repo, "empty-delete"), "")
	writeGitTestFile(t, filepath.Join(repo, "text-delete.txt"), "old one\nold two\n")
	writeGitTestFile(t, filepath.Join(repo, "text-modify.txt"), "before\n")
	writeGitTestFile(t, filepath.Join(repo, "rename-clean.txt"), "unchanged rename\n")
	writeGitTestFile(t, filepath.Join(repo, "rename-edited.txt"), "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n")
	writeGitTestFile(t, filepath.Join(repo, "script.sh"), "#!/bin/sh\necho ok\n")
	writeGitTestFile(t, filepath.Join(repo, "node"), "regular node\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "fixture base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))

	if err := os.Remove(filepath.Join(repo, "empty-delete")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(repo, "text-delete.txt")); err != nil {
		t.Fatal(err)
	}
	writeGitTestFile(t, filepath.Join(repo, "empty-add"), "")
	writeGitTestFile(t, filepath.Join(repo, "text-add.txt"), "new one\nnew two\n")
	writeGitTestFile(t, filepath.Join(repo, "text-modify.txt"), "after\n")
	writeGitTestBytes(t, filepath.Join(repo, "binary-new.bin"), []byte{0, 1, 2, 3, 0xff})
	gitTest(t, repo, "mv", "rename-clean.txt", "renamed clean.txt")
	gitTest(t, repo, "mv", "rename-edited.txt", "renamed-edited.txt")
	writeGitTestFile(t, filepath.Join(repo, "renamed-edited.txt"), "one\ntwo changed\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n")
	writeGitTestBytes(t, filepath.Join(repo, "crlf.txt"), []byte("first\r\nsecond\r\n"))
	writeGitTestFile(t, filepath.Join(repo, "no-final-newline.txt"), "last line")
	writeGitTestFile(t, filepath.Join(repo, "unicodé space.txt"), "snowman ☃\n")
	writeGitTestFile(t, filepath.Join(repo, "review.saga", "note.md"), "saga only\n")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(repo, "script.sh"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(repo, "node")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("text-add.txt", filepath.Join(repo, "node")); err != nil {
			t.Fatal(err)
		}
	}
	gitTest(t, repo, "add", "-A")
	gitTest(t, repo, "commit", "-m", "adversarial feature")
	baseline, err := Read(context.Background(), repo, "https://example.test/acme/corpus.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	// Poison common diff settings. Read must still produce the same canonical
	// prefixes, quoting, rename detection, submodule form, and algorithm.
	gitTest(t, repo, "config", "core.quotePath", "false")
	gitTest(t, repo, "config", "diff.noprefix", "true")
	gitTest(t, repo, "config", "diff.srcPrefix", "OLD/")
	gitTest(t, repo, "config", "diff.dstPrefix", "NEW/")
	gitTest(t, repo, "config", "diff.submodule", "log")
	gitTest(t, repo, "config", "diff.algorithm", "histogram")
	gitTest(t, repo, "config", "diff.context", "50")
	gitTest(t, repo, "config", "diff.interHunkContext", "50")

	changes, err := Read(context.Background(), repo, "https://example.test/acme/corpus.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if changes.HeadOID != baseline.HeadOID || len(changes.Atoms) != len(baseline.Atoms) {
		t.Fatalf("repository diff config changed canonical comparison: before=%#v after=%#v", baseline, changes)
	}
	keys := map[string]bool{}
	paths := map[string]bool{}
	for _, atom := range changes.Atoms {
		keys[atom.Key] = true
		paths[atom.Path] = true
		paths[atom.OldPath] = true
		paths[atom.NewPath] = true
		if location, err := coderef.ParseLocation(atom.Ref); err != nil || location != changes.Location(atom) {
			t.Errorf("atom %q has invalid code location %q: %v", atom.Key, atom.Ref, err)
		}
	}
	for _, key := range []string{
		"event:rename:empty-add:empty-delete:empty-add",
		"event:add:text-add.txt::", "event:delete:text-delete.txt::",
		"event:binary:binary-new.bin::",
		"event:rename:renamed clean.txt:rename-clean.txt:renamed clean.txt",
		"event:rename:renamed-edited.txt:rename-edited.txt:renamed-edited.txt",
		"line:text-modify.txt:old:1", "line:text-modify.txt:new:1",
		"line:crlf.txt:new:1", "line:no-final-newline.txt:new:1",
		"line:unicodé space.txt:new:1",
	} {
		if !keys[key] {
			t.Errorf("fixture corpus omitted %q; keys=%v", key, keys)
		}
	}
	if runtime.GOOS != "windows" {
		for _, key := range []string{"event:mode:script.sh::", "event:type-change:node::"} {
			if !keys[key] {
				t.Errorf("fixture corpus omitted %q", key)
			}
		}
	}
	for _, productPath := range []string{
		"empty-add", "empty-delete", "text-add.txt", "text-delete.txt", "text-modify.txt",
		"binary-new.bin", "renamed clean.txt", "renamed-edited.txt", "crlf.txt",
		"no-final-newline.txt", "unicodé space.txt",
	} {
		if !paths[productPath] {
			t.Errorf("Git-reported product path %q yielded no coverage atom", productPath)
		}
	}
	if len(changes.SagaChanges) == 0 {
		t.Fatal("saga-only fixture was not classified separately")
	}
	assertSessionReadsMatch(t, repo, "https://example.test/acme/corpus.git", base, "HEAD")
}

// A gitexec session reads diffs through a shared diff-tree process. Every
// reader must return exactly what it returns without one.
func assertSessionReadsMatch(t *testing.T, repo, repository, base, head string) {
	t.Helper()
	plain := context.Background()
	session, end := gitexec.Begin(plain)
	defer end()
	wantChanges, err := Read(plain, repo, repository, base, head)
	if err != nil {
		t.Fatal(err)
	}
	wantCatalog, err := ReadCatalog(plain, repo, repository, base, head)
	if err != nil {
		t.Fatal(err)
	}
	wantTree, err := TreeChanges(plain, repo, wantChanges.BaseOID, wantChanges.HeadOID)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		gotChanges, err := Read(session, repo, repository, base, head)
		if err != nil {
			t.Fatal(err)
		}
		gotCatalog, err := ReadCatalog(session, repo, repository, base, head)
		if err != nil {
			t.Fatal(err)
		}
		gotTree, err := TreeChanges(session, repo, wantChanges.BaseOID, wantChanges.HeadOID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotChanges, wantChanges) {
			t.Fatalf("Read in a session differs:\n got %#v\nwant %#v", gotChanges, wantChanges)
		}
		if !reflect.DeepEqual(gotCatalog, wantCatalog) {
			t.Fatalf("ReadCatalog in a session differs:\n got %#v\nwant %#v", gotCatalog, wantCatalog)
		}
		if !reflect.DeepEqual(gotTree, wantTree) {
			t.Fatalf("TreeChanges in a session differs:\n got %#v\nwant %#v", gotTree, wantTree)
		}
	}
}

func TestEmptyFileAddAndDeleteProduceLifecycleAtoms(t *testing.T) {
	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/empty.git")
	writeGitTestFile(t, filepath.Join(repo, "empty-delete"), "")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "checkout", "-b", "delete-empty")
	if err := os.Remove(filepath.Join(repo, "empty-delete")); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "-A")
	gitTest(t, repo, "commit", "-m", "delete empty")
	deleted, err := Read(context.Background(), repo, "https://example.test/acme/empty.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !hasAtomKey(deleted.Atoms, "event:delete:empty-delete::") {
		t.Fatalf("empty delete yielded no lifecycle atom: %#v", deleted.Atoms)
	}

	gitTest(t, repo, "checkout", "-b", "add-empty", base)
	writeGitTestFile(t, filepath.Join(repo, "empty-add"), "")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "add empty")
	added, err := Read(context.Background(), repo, "https://example.test/acme/empty.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if !hasAtomKey(added.Atoms, "event:add:empty-add::") {
		t.Fatalf("empty add yielded no lifecycle atom: %#v", added.Atoms)
	}
}

func TestCommittedComparisonsUseActualMergeBase(t *testing.T) {
	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/topology.git")
	writeGitTestFile(t, filepath.Join(repo, "root.txt"), "root\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "root")
	root := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "checkout", "-b", "feature")
	writeGitTestFile(t, filepath.Join(repo, "feature.txt"), "committed\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "feature")
	feature := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, repo, "checkout", "main")
	writeGitTestFile(t, filepath.Join(repo, "advanced-base.txt"), "not part of feature\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "advanced base")

	committed, err := Read(context.Background(), repo, "https://example.test/acme/topology.git", "main", feature)
	if err != nil {
		t.Fatal(err)
	}
	if committed.BaseOID != root || committed.HeadOID != feature || hasAtomPath(committed.Atoms, "advanced-base.txt") || !hasAtomPath(committed.Atoms, "feature.txt") {
		t.Fatalf("committed comparison did not use merge base %s: %#v", root, committed)
	}

	// References pin commits, so uncommitted work cannot be compared.
	if _, err := Read(context.Background(), repo, "https://example.test/acme/topology.git", "main", "WORKTREE"); err == nil || !strings.Contains(err.Error(), "WORKTREE is not supported") {
		t.Fatalf("WORKTREE head was accepted: %v", err)
	}
}

func TestRenameAcrossSagaBoundaryRemainsAProductAtom(t *testing.T) {
	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/boundary.git")
	writeGitTestFile(t, filepath.Join(repo, "product.txt"), "one\ntwo\nthree\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	if err := os.MkdirAll(filepath.Join(repo, "review.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "mv", "product.txt", "review.saga/product.txt")
	gitTest(t, repo, "commit", "-m", "move product into saga")
	changes, err := Read(context.Background(), repo, "https://example.test/acme/boundary.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	key := "event:rename:review.saga/product.txt:product.txt:review.saga/product.txt"
	if !hasAtomKey(changes.Atoms, key) || !hasAtomKey(changes.SagaChanges, key) {
		t.Fatalf("cross-boundary rename must be both product and saga-visible: %#v", changes)
	}
}

func TestReadVerifiesCheckoutRepositoryWithExplicitOverride(t *testing.T) {
	repo := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "base.txt"), "base\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "change.txt"), "change\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "change")
	gitTest(t, repo, "remote", "add", "origin", "https://user:secret@example.test/acme/actual.git")
	_, err := Read(context.Background(), repo, "https://example.test/acme/other.git", base, "HEAD")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("repository mismatch was not safely rejected: %v", err)
	}
	changes, err := ReadWithOptions(context.Background(), repo, "https://example.test/acme/other.git", base, "HEAD", ReadOptions{AllowRepositoryMismatch: true})
	if err != nil || len(changes.Atoms) == 0 {
		t.Fatalf("explicit repository mismatch override failed: %#v %v", changes, err)
	}
}

func TestReadRequiresOverrideWhenCheckoutOriginIsUnavailable(t *testing.T) {
	repo := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "base.txt"), "base\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "change.txt"), "change\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "change")
	_, err := Read(context.Background(), repo, "https://example.test/acme/declared.git", base, "HEAD")
	if err == nil || !strings.Contains(err.Error(), "no origin") {
		t.Fatalf("unverifiable checkout was accepted: %v", err)
	}
	if _, err := ReadWithOptions(context.Background(), repo, "https://example.test/acme/declared.git", base, "HEAD", ReadOptions{AllowRepositoryMismatch: true}); err != nil {
		t.Fatalf("explicit override did not permit originless checkout: %v", err)
	}
}

func TestRepositoryCorrespondenceIgnoresTransportAndGitSuffix(t *testing.T) {
	if !sameRepository("https://example.test/acme/app", "ssh://example.test/acme/app.git") {
		t.Fatal("equivalent HTTPS and SSH repository identities did not correspond")
	}
	if sameRepository("https://example.test/acme/app", "https://example.test/other/app") {
		t.Fatal("different repository paths corresponded")
	}
}

func TestSubmoduleGitlinkChangeProducesAtoms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local file-transport submodule fixture is verified on Unix CI")
	}
	child := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(child, "child.txt"), "one\n")
	gitTest(t, child, "add", ".")
	gitTest(t, child, "commit", "-m", "one")
	firstChild := strings.TrimSpace(gitTest(t, child, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(child, "child.txt"), "two\n")
	gitTest(t, child, "add", ".")
	gitTest(t, child, "commit", "-m", "two")

	repo := newGitTestRepo(t)
	gitTest(t, repo, "remote", "add", "origin", "https://example.test/acme/submodule.git")
	gitTest(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", child, "deps/child")
	gitTest(t, filepath.Join(repo, "deps/child"), "checkout", firstChild)
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "submodule base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	gitTest(t, filepath.Join(repo, "deps/child"), "checkout", "main")
	gitTest(t, repo, "add", "deps/child")
	gitTest(t, repo, "commit", "-m", "advance gitlink")
	changes, err := Read(context.Background(), repo, "https://example.test/acme/submodule.git", base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes.Atoms) == 0 || !hasAtomPath(changes.Atoms, "deps/child") {
		t.Fatalf("submodule change yielded no coverage atoms: %#v", changes)
	}
	assertSessionReadsMatch(t, repo, "https://example.test/acme/submodule.git", base, "HEAD")
}

func hasAtomPath(atoms []Atom, path string) bool {
	for _, atom := range atoms {
		if atom.Path == path || atom.OldPath == path || atom.NewPath == path {
			return true
		}
	}
	return false
}

func hasAtomKey(atoms []Atom, key string) bool {
	for _, atom := range atoms {
		if atom.Key == key {
			return true
		}
	}
	return false
}

func newGitTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init", "-b", "main")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.test")
	return repo
}

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func writeGitTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeGitTestBytes(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Git reads attributes from the checkout even when diffing two commits, so an
// uncommitted attribute edit must not be answered from an earlier diff.
func TestCachedCommitDiffFollowsCheckoutAttributes(t *testing.T) {
	repo := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "data.txt"), "one\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "data.txt"), "two\n")
	gitTest(t, repo, "commit", "-am", "edit")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	before, err := TreeChanges(context.Background(), repo, base, head)
	if err != nil || len(before) != 1 || before[0].Binary || len(before[0].Hunks) != 1 {
		t.Fatalf("text diff = %#v, %v", before, err)
	}
	writeGitTestFile(t, filepath.Join(repo, ".gitattributes"), "*.txt -diff\n")
	after, err := TreeChanges(context.Background(), repo, base, head)
	if err != nil || len(after) != 1 || !after[0].Binary {
		t.Fatalf("diff after marking the file -diff = %#v, %v; want it binary", after, err)
	}
}

// Remembered diffs key on the top-level attribute files only. A process
// that outlives an uncommitted edit to a nested .gitattributes, as the
// review server does, runs isolated sessions and must see the new patch.
func TestIsolatedSessionsFollowNestedAttributes(t *testing.T) {
	repo := newGitTestRepo(t)
	writeGitTestFile(t, filepath.Join(repo, "sub", "data.txt"), "one\n")
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	writeGitTestFile(t, filepath.Join(repo, "sub", "data.txt"), "two\n")
	gitTest(t, repo, "commit", "-am", "edit")
	head := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	changes := func(begin func(context.Context) (context.Context, func())) []FileChange {
		t.Helper()
		ctx, end := begin(context.Background())
		defer end()
		result, err := TreeChanges(ctx, repo, base, head)
		if err != nil || len(result) != 1 {
			t.Fatalf("TreeChanges = %#v, %v", result, err)
		}
		return result
	}
	if changes(gitexec.Begin)[0].Binary {
		t.Fatal("a text edit was reported binary")
	}
	writeGitTestFile(t, filepath.Join(repo, "sub", ".gitattributes"), "*.txt -diff\n")
	if changes(gitexec.Begin)[0].Binary {
		t.Fatal("nested attributes became part of the diff key; tighten this test")
	}
	if !changes(gitexec.BeginIsolated)[0].Binary {
		t.Fatal("an isolated session served a diff remembered before the nested attribute edit")
	}
}
