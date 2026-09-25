package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestReviewDiffsReuseOnlyRawPatches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	rng := reviewstate.Range{BaseOID: "base", HeadOID: "head"}
	diffs := newReviewDiffs("nested", nil, rng)
	roots, resolutions := 0, 0
	reads := map[string]int{}
	diffs.rootLookup = func(_ context.Context, dir string, args ...string) (string, error) {
		roots++
		if dir != "nested" || !reflect.DeepEqual(args, []string{"rev-parse", "--show-toplevel"}) {
			t.Fatalf("root lookup: %s %v", dir, args)
		}
		return "root", nil
	}
	diffs.resolve = func(_ context.Context, ref coderef.Reference, head string) coderesolve.Resolution {
		resolutions++
		if head != rng.HeadOID {
			t.Fatalf("resolve head: %s", head)
		}
		if ref.Digest == "stale" {
			return coderesolve.Resolution{State: coderesolve.Stale}
		}
		location := ref.Location()
		if ref.Path == "old" {
			location.Path = "file"
		}
		return coderesolve.Resolution{State: coderesolve.Current, Location: location}
	}
	diffs.fileDiff = func(_ context.Context, repo, base, head string, paths ...string) (string, error) {
		if repo != "root" || base != rng.BaseOID || head != rng.HeadOID || len(paths) != 1 {
			t.Fatalf("patch arguments: %s %s %s %v", repo, base, head, paths)
		}
		reads[paths[0]]++
		if paths[0] == "empty" {
			return "", nil
		}
		return "@@ -1 +1 @@\n-old one\n+new one\n@@ -20 +20 @@\n-old twenty\n+new twenty\n", nil
	}
	refs := []coderef.Reference{
		{Path: "old", Start: 1, End: 1},
		{Path: "file", Start: 20, End: 20},
		{Path: "file", Start: 1, End: 1, Digest: "stale"},
		{Path: "file"},
		{Path: "empty"}, {Path: "empty", Start: 7, End: 8},
	}
	var views []*reviewDiffView
	for _, ref := range refs {
		views = append(views, diffs.referenceDiff(ctx, ref))
	}
	if roots != 1 || resolutions != len(refs) || !reflect.DeepEqual(reads, map[string]int{"file": 1, "empty": 1}) {
		t.Fatalf("root=%d resolve=%d reads=%v", roots, resolutions, reads)
	}
	if views[0].Path != "file" || views[0].Location != refs[0].Location().String() || len(views[0].Lines) != 3 || views[0].Lines[2].Text != "new one" {
		t.Fatalf("first exact reference: %+v", views[0])
	}
	if len(views[1].Lines) != 3 || views[1].Lines[2].Text != "new twenty" {
		t.Fatalf("second exact reference: %+v", views[1])
	}
	if len(views[2].Lines) != 6 || !strings.Contains(views[2].Note, "referenced lines changed") || views[3].Note != "" || len(views[3].Lines) != 6 {
		t.Fatalf("stale/whole-file: %+v %+v", views[2], views[3])
	}
	if views[4].Note != "The file is unchanged between the base and the head." || views[5].Note != "Lines 7-8 are unchanged between the base and the head." {
		t.Fatalf("empty views: %+v %+v", views[4], views[5])
	}
	views[0].Lines[2].Text = "mutated view"
	if got := diffs.referenceDiff(ctx, refs[0]); got.Lines[2].Text != "new one" {
		t.Fatal("filtered views share mutable lines")
	}
}

func TestReviewDiffsRootFallbackAndPatchRetry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, top    string
		err          error
		roots, reads int
	}{
		{"known root", "root", nil, 1, 2},
		{"empty root", "", nil, 3, 3},
		{"failed root", "", errors.New("missing Git root"), 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			diffs := newReviewDiffs("fallback", nil, reviewstate.Range{})
			roots, reads := 0, 0
			diffs.rootLookup = func(context.Context, string, ...string) (string, error) { roots++; return tc.top, tc.err }
			diffs.resolve = func(_ context.Context, ref coderef.Reference, _ string) coderesolve.Resolution {
				return coderesolve.Resolution{State: coderesolve.Current, Location: coderef.Location{Path: "renamed", Start: 1, End: 1}}
			}
			diffs.fileDiff = func(_ context.Context, repo, _, _ string, paths ...string) (string, error) {
				reads++
				wantRepo := "fallback"
				if tc.top != "" {
					wantRepo = tc.top
				}
				if repo != wantRepo || paths[0] != "renamed" {
					t.Fatalf("fallback patch: %s %v", repo, paths)
				}
				if reads == 1 {
					return "partial patch", errors.New("unreadable object")
				}
				return "@@ -1 +1 @@\n-before\n+after\n", nil
			}
			ref := coderef.Reference{Path: "original", Start: 1, End: 1}
			first := diffs.referenceDiff(context.Background(), ref)
			if first.Path != "original" || first.Location != ref.Location().String() || first.Note != "The diff could not be read from this checkout." || len(first.Lines) != 0 {
				t.Fatalf("failure view: %+v", first)
			}
			second := diffs.referenceDiff(context.Background(), ref)
			third := diffs.referenceDiff(context.Background(), ref)
			if second.Path != "renamed" || len(second.Lines) != 3 || second.Note != "" || !reflect.DeepEqual(second, third) || roots != tc.roots || reads != tc.reads {
				t.Fatalf("retry: %+v %+v roots=%d reads=%d", second, third, roots, reads)
			}
		})
	}
}

func TestReviewDiffsMatchUncachedGitSemantics(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := t.TempDir()
	serverGit(t, repo, "init", "-b", "main")
	serverGit(t, repo, "config", "user.name", "Dev")
	serverGit(t, repo, "config", "user.email", "dev@example.test")
	var lines strings.Builder
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&lines, "line %d\n", i)
	}
	before := lines.String()
	// A path full of glob syntax must be passed to Git literally; the decoy is
	// what the path would match as a pattern. Windows forbids '*' in file
	// names, so there the bracket alone carries the glob.
	literal, decoy := "literal[1]*.txt", "literal1-other.txt"
	if runtime.GOOS == "windows" {
		literal, decoy = "literal[1].txt", "literal1.txt"
	}
	for _, path := range []string{"file", "moved", "renamed-before", "deleted", "unchanged", literal, decoy} {
		writeServerFile(t, filepath.Join(repo, path), before)
	}
	writeServerFile(t, filepath.Join(repo, "binary"), "before\x00data")
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	after := strings.ReplaceAll(strings.ReplaceAll(before, "line 5\n", "changed five\n"), "line 30\n", "changed thirty\n")
	writeServerFile(t, filepath.Join(repo, "file"), after)
	writeServerFile(t, filepath.Join(repo, literal), after)
	writeServerFile(t, filepath.Join(repo, decoy), "must not match literal path\n")
	writeServerFile(t, filepath.Join(repo, "moved"), "inserted\n"+before)
	serverGit(t, repo, "mv", "renamed-before", "renamed-after")
	serverGit(t, repo, "rm", "deleted")
	writeServerFile(t, filepath.Join(repo, "binary"), "after\x00data")
	serverGit(t, repo, "add", ".")
	serverGit(t, repo, "commit", "-m", "head")
	head := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	nested := filepath.Join(repo, "nested", "saga")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, nested)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	author := func(commit, path string, start, end int) coderef.Reference {
		t.Helper()
		ref, err := resolver.Author(ctx, coderef.Location{Commit: commit, Path: path, Start: start, End: end}, "")
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	refs := []coderef.Reference{
		author(head, "file", 5, 5), author(head, "file", 30, 30),
		author(base, "file", 5, 5), author(head, "file", 0, 0),
		author(base, "moved", 30, 30), author(base, "renamed-before", 30, 30),
		author(base, "deleted", 5, 5), author(base, "deleted", 0, 0),
		author(head, "binary", 0, 0), author(base, "binary", 0, 0),
		author(head, literal, 5, 5), author(head, "unchanged", 5, 5), author(head, "unchanged", 0, 0),
	}
	badDigest := refs[0]
	badDigest.Digest = "sha256:" + strings.Repeat("0", 64)
	missingPin := refs[0]
	missingPin.Commit = strings.Repeat("0", 40)
	refs = append(refs, badDigest, missingPin)
	for _, rng := range []reviewstate.Range{{BaseOID: base, HeadOID: head}, {BaseOID: head, HeadOID: head}, {BaseOID: base, HeadOID: strings.Repeat("0", 40)}, {BaseOID: strings.Repeat("0", 40), HeadOID: head}} {
		for _, useResolver := range []*coderesolve.Resolver{resolver, nil} {
			diffs := newReviewDiffs(nested, useResolver, rng)
			for _, ref := range refs {
				got := diffs.referenceDiff(ctx, ref)
				want := uncachedReferenceDiff(ctx, nested, useResolver, ref, rng)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("range=%+v ref=%+v\ngot=%+v\nwant=%+v", rng, ref, got, want)
				}
			}
		}
	}
	// A later request must read a newly resolved range, even for the same path.
	writeServerFile(t, filepath.Join(repo, "file"), "third revision\n")
	serverGit(t, repo, "commit", "-am", "later request")
	later := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	got := newReviewDiffs(nested, nil, reviewstate.Range{BaseOID: head, HeadOID: later}).referenceDiff(ctx, coderef.Reference{Path: "file"})
	if !strings.Contains(fmt.Sprint(got.Lines), "third revision") {
		t.Fatalf("new request reused old patch: %+v", got)
	}
	// A separate checkout with identical paths cannot inherit successful patches.
	other := t.TempDir()
	serverGit(t, other, "init", "-b", "main")
	got = newReviewDiffs(other, nil, reviewstate.Range{BaseOID: base, HeadOID: head}).referenceDiff(ctx, refs[0])
	if got.Note != "The diff could not be read from this checkout." {
		t.Fatalf("other repo reused patch: %+v", got)
	}
}

// Preserve the pre-reuse implementation as a semantic oracle. It deliberately
// resolves the checkout and reads the patch again for every reference.
func uncachedReferenceDiff(ctx context.Context, sourceDir string, resolver *coderesolve.Resolver, reference coderef.Reference, rng reviewstate.Range) *reviewDiffView {
	view := &reviewDiffView{Path: reference.Path, Location: reference.Location().String()}
	start, end := reference.Start, reference.End
	path := reference.Path
	if resolver != nil {
		if resolution := resolver.Resolve(ctx, reference, rng.HeadOID); resolution.Current() {
			path, start, end = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
		} else if !reference.WholeFile() {
			view.Note = "The referenced lines changed after the reference was written; showing every change to the file."
			start, end = 0, 0
		}
	}
	// Pathspecs are relative to the working directory, and a Saga served from
	// inside its code repository has the Saga directory as its source dir.
	repo := sourceDir
	if top, err := gitOutput(ctx, sourceDir, "rev-parse", "--show-toplevel"); err == nil && top != "" {
		repo = top
	}
	patch, err := gitdiff.FileDiff(ctx, repo, rng.BaseOID, rng.HeadOID, path)
	if err != nil {
		view.Note = "The diff could not be read from this checkout."
		return view
	}
	view.Path = path
	view.Lines = diffLinesTouching(patch, start, end)
	if len(view.Lines) == 0 {
		if start > 0 {
			view.Note = fmt.Sprintf("Lines %d-%d are unchanged between the base and the head.", start, end)
		} else if view.Note == "" {
			view.Note = "The file is unchanged between the base and the head."
		}
	}
	return view
}

// BenchmarkReviewDiffs compares the production request helper with the original
// per-reference path on the same disposable realistic fixture. Equality and
// call counts are checked outside timing; no Saga records are written.
func BenchmarkReviewDiffs(b *testing.B) {
	root, repo, id := os.Getenv("PR_PERF_SAGA"), os.Getenv("PR_PERF_REPO"), os.Getenv("PR_PERF_REVIEW")
	if root == "" || repo == "" || id == "" {
		b.Skip("set PR_PERF_SAGA, PR_PERF_REPO and PR_PERF_REVIEW")
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		b.Fatalf("load: %v; validation: %+v", err, validation)
	}
	review := doc.FindReview(id)
	if review == nil || review.Deck == nil {
		b.Fatal("review with deck required")
	}
	ctx := context.Background()
	rng, err := reviewstate.ResolveRange(ctx, repo, review)
	if err != nil {
		b.Fatal(err)
	}
	var refs []coderef.Reference
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for _, file := range item.Code {
				refs = append(refs, file.References...)
			}
		}
	}
	if len(refs) == 0 {
		b.Fatal("exact references required")
	}
	roots, reads, resolves := 0, map[string]int{}, 0
	run := func(reuse, count bool) []*reviewDiffView {
		resolver, err := coderesolve.New(ctx, repo)
		if err != nil {
			b.Fatal(err)
		}
		defer resolver.Close()
		diffs := newReviewDiffs(repo, resolver, rng)
		if count {
			diffs.rootLookup = func(ctx context.Context, dir string, args ...string) (string, error) {
				roots++
				return gitOutput(ctx, dir, args...)
			}
			diffs.fileDiff = func(ctx context.Context, dir, base, head string, paths ...string) (string, error) {
				reads[paths[0]]++
				return gitdiff.FileDiff(ctx, dir, base, head, paths...)
			}
			diffs.resolve = func(ctx context.Context, ref coderef.Reference, head string) coderesolve.Resolution {
				resolves++
				return resolver.Resolve(ctx, ref, head)
			}
		}
		result := make([]*reviewDiffView, 0, len(refs))
		for _, ref := range refs {
			if reuse {
				result = append(result, diffs.referenceDiff(ctx, ref))
			} else {
				result = append(result, uncachedReferenceDiff(ctx, repo, resolver, ref, rng))
			}
		}
		return result
	}
	if want, got := run(false, false), run(true, true); !reflect.DeepEqual(want, got) {
		b.Fatal("request reuse changed reference views")
	}
	if roots != 1 || resolves != len(refs) {
		b.Fatalf("root=%d resolves=%d refs=%d", roots, resolves, len(refs))
	}
	for path, count := range reads {
		if count != 1 {
			b.Fatalf("patch %s read %d times", path, count)
		}
	}
	for _, reuse := range []bool{false, true} {
		name := "uncached"
		if reuse {
			name = "request_local"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				run(reuse, false)
			}
			b.ReportMetric(float64(len(refs)), "references/op")
			b.ReportMetric(float64(len(reads)), "files/op")
		})
	}
}

func TestReviewDiffsRootLookupRecovers(t *testing.T) {
	t.Parallel()
	for _, firstErr := range []error{nil, errors.New("transient root failure")} {
		t.Run(fmt.Sprint(firstErr), func(t *testing.T) {
			diffs := newReviewDiffs("nested", nil, reviewstate.Range{})
			roots, reads := 0, 0
			diffs.rootLookup = func(context.Context, string, ...string) (string, error) {
				roots++
				if roots == 1 {
					return "", firstErr
				}
				return "root", nil
			}
			diffs.fileDiff = func(_ context.Context, repo, _, _ string, _ ...string) (string, error) {
				reads++
				if repo == "nested" {
					return "", nil
				}
				if repo != "root" {
					t.Fatalf("unexpected repo %s", repo)
				}
				return "@@ -1 +1 @@\n-old\n+new\n", nil
			}
			ref := coderef.Reference{Path: "file"}
			first := diffs.referenceDiff(context.Background(), ref)
			second := diffs.referenceDiff(context.Background(), ref)
			third := diffs.referenceDiff(context.Background(), ref)
			if len(first.Lines) != 0 || len(second.Lines) != 3 || !reflect.DeepEqual(second, third) || roots != 2 || reads != 2 {
				t.Fatalf("root recovery: %+v %+v %+v roots=%d reads=%d", first, second, third, roots, reads)
			}
		})
	}
}
