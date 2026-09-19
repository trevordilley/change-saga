package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// comparisonCommits returns the merge-base and head commit of a saga's source
// comparison, the two commits comparison-side references are pinned at.
func comparisonCommits(t *testing.T, root, repo string) (base, head string) {
	t.Helper()
	report, err := buildReport(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	return report.BaseOID, report.HeadOID
}

const (
	rangeRepository = "https://example.test/acme/app.git"
	rangeBase       = "1111111111111111111111111111111111111111"
	rangeHead       = "2222222222222222222222222222222222222222"
)

// rangeChanges is a hand-built comparison of rangeBase..rangeHead, the same
// shape gitdiff.Read emits, so the unit tests exercise the exact input the
// derived reference path sees in production.
func rangeChanges(atoms ...gitdiff.Atom) gitdiff.ChangeSet {
	return gitdiff.ChangeSet{Repository: rangeRepository, BaseOID: rangeBase, HeadOID: rangeHead, Atoms: atoms}
}

func newLine(path string, line int) gitdiff.Atom {
	return gitdiff.Atom{Kind: "line", Path: path, Side: "new", Line: line}
}

func oldLine(path string, line int) gitdiff.Atom {
	return gitdiff.Atom{Kind: "line", Path: path, Side: "old", Line: line}
}

func eventAtom(event, path string) gitdiff.Atom {
	return gitdiff.Atom{Kind: "event", Event: event, Path: path}
}

// describeLocations renders emitted locations as a compact, order-preserving
// shape naming the comparison side, so a failure names the range that was
// wrong rather than dumping object names.
func describeLocations(t *testing.T, base, head string, locations []coderef.Location) []string {
	t.Helper()
	described := make([]string, 0, len(locations))
	for _, location := range locations {
		side := "?"
		switch location.Commit {
		case head:
			side = "new"
		case base:
			side = "old"
		}
		if location.WholeFile() {
			described = append(described, fmt.Sprintf("%s %s file", side, location.Path))
			continue
		}
		described = append(described, fmt.Sprintf("%s %s %d-%d", side, location.Path, location.Start, location.End))
	}
	return described
}

func referenceLocations(references []coderef.Reference) []coderef.Location {
	locations := make([]coderef.Location, 0, len(references))
	for _, reference := range references {
		locations = append(locations, reference.Location())
	}
	return locations
}

func TestChangedLocationsCoalesceOnlyDenseRuns(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		path  string
		side  string
		atoms []gitdiff.Atom
		want  []string
	}{
		{
			name:  "consecutive new lines become one range",
			atoms: []gitdiff.Atom{newLine("a.go", 1), newLine("a.go", 2), newLine("a.go", 3)},
			want:  []string{"new a.go 1-3"},
		},
		{
			name:  "a single line keeps a degenerate range",
			atoms: []gitdiff.Atom{newLine("a.go", 7)},
			want:  []string{"new a.go 7-7"},
		},
		{
			name:  "a one line gap splits the run",
			atoms: []gitdiff.Atom{newLine("a.go", 1), newLine("a.go", 2), newLine("a.go", 4), newLine("a.go", 5)},
			want:  []string{"new a.go 1-2", "new a.go 4-5"},
		},
		{
			name:  "nonconsecutive lines never merge",
			atoms: []gitdiff.Atom{newLine("a.go", 1), newLine("a.go", 10), newLine("a.go", 20)},
			want:  []string{"new a.go 1-1", "new a.go 10-10", "new a.go 20-20"},
		},
		{
			name:  "old and new sides stay separate, each at its own commit",
			atoms: []gitdiff.Atom{oldLine("a.go", 1), oldLine("a.go", 2), newLine("a.go", 1), newLine("a.go", 2)},
			want:  []string{"old a.go 1-2", "new a.go 1-2"},
		},
		{
			name:  "only the requested path is referenced",
			atoms: []gitdiff.Atom{newLine("a.go", 1), newLine("b.go", 2), newLine("a.go", 2)},
			want:  []string{"new a.go 1-2"},
		},
		{
			name:  "an add event is the whole file, and its lines stay line ranges",
			atoms: []gitdiff.Atom{eventAtom("add", "a.go"), newLine("a.go", 1), newLine("a.go", 2)},
			want:  []string{"new a.go file", "new a.go 1-2"},
		},
		{
			name:  "a delete event is the whole file at the merge-base",
			atoms: []gitdiff.Atom{eventAtom("delete", "a.go"), oldLine("a.go", 1), oldLine("a.go", 2)},
			want:  []string{"old a.go file", "old a.go 1-2"},
		},
		{
			name:  "two events on one path are one whole-file reference",
			atoms: []gitdiff.Atom{eventAtom("mode", "a.go"), eventAtom("type-change", "a.go")},
			want:  []string{"new a.go file"},
		},
		{
			name:  "a whole file absorbs no lines on either side",
			atoms: []gitdiff.Atom{oldLine("a.go", 1), oldLine("a.go", 2), eventAtom("mode", "a.go"), newLine("a.go", 3)},
			want:  []string{"new a.go file", "old a.go 1-2", "new a.go 3-3"},
		},
		{
			name: "either path of a rename selects both sides",
			path: "b.go",
			atoms: []gitdiff.Atom{
				{Kind: "event", Event: "rename", Path: "b.go", OldPath: "a.go", NewPath: "b.go"},
				oldLine("a.go", 3), newLine("b.go", 3), newLine("c.go", 1),
			},
			want: []string{"new b.go file", "old a.go 3-3", "new b.go 3-3"},
		},
		{
			name:  "unsorted input canonicalizes to ascending ranges",
			atoms: []gitdiff.Atom{newLine("a.go", 3), newLine("a.go", 1), newLine("a.go", 2)},
			want:  []string{"new a.go 1-3"},
		},
		{
			name:  "duplicate atoms do not widen a range",
			atoms: []gitdiff.Atom{newLine("a.go", 2), newLine("a.go", 2), newLine("a.go", 4)},
			want:  []string{"new a.go 2-2", "new a.go 4-4"},
		},
		{
			name:  "the old side filter keeps only merge-base lines",
			side:  "old",
			atoms: []gitdiff.Atom{oldLine("a.go", 1), newLine("a.go", 1), oldLine("a.go", 2)},
			want:  []string{"old a.go 1-2"},
		},
		{
			name:  "the new side filter drops a delete event",
			side:  "new",
			atoms: []gitdiff.Atom{eventAtom("delete", "a.go"), oldLine("a.go", 1)},
			want:  []string{},
		},
		{
			name:  "no atoms produce no references",
			atoms: nil,
			want:  []string{},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := firstNonEmpty(testCase.path, "a.go")
			got := describeLocations(t, rangeBase, rangeHead, changedLocations(rangeChanges(testCase.atoms...), path, testCase.side))
			if strings.Join(got, "; ") != strings.Join(testCase.want, "; ") {
				t.Fatalf("references = %v, want %v", got, testCase.want)
			}
		})
	}
}

// Coalescing is only sound if the emitted references account for exactly the
// atoms they were built from: every changed line lies inside exactly one line
// reference, every file event is its own whole-file reference, and every line
// a range spans is itself a changed line.
func TestChangedLocationsPreserveExactAtomIdentity(t *testing.T) {
	changes := rangeChanges(
		oldLine("a.go", 4), oldLine("a.go", 5),
		newLine("a.go", 4), newLine("a.go", 5), newLine("a.go", 6),
		newLine("a.go", 40),
		newLine("b.go", 1),
		eventAtom("add", "c.go"), newLine("c.go", 1), newLine("c.go", 2),
	)
	for _, path := range []string{"a.go", "b.go", "c.go"} {
		locations := changedLocations(changes, path, "")
		changed := map[string]bool{}
		for _, atom := range changes.Atoms {
			if atom.Path != path {
				continue
			}
			location := changes.Location(atom)
			changed[location.String()] = true
			inside := 0
			for _, reference := range locations {
				// A whole-file reference accounts only for its file's events.
				if reference.WholeFile() == (atom.Kind == "event") && reference.Contains(location) {
					inside++
				}
			}
			if inside != 1 {
				t.Fatalf("%s atom %s lies inside %d references, want exactly 1: %v", path, location, inside, locations)
			}
		}
		for _, reference := range locations {
			if reference.WholeFile() {
				continue
			}
			for line := reference.Start; line <= reference.End; line++ {
				location := coderef.Location{Commit: reference.Commit, Path: reference.Path, Start: line, End: line}
				if !changed[location.String()] {
					t.Fatalf("range %s widened over unchanged line %d", reference, line)
				}
			}
		}
	}
}

func TestParseRangesCanonicalizesEquivalentManualSelectors(t *testing.T) {
	ranges, err := parseRanges("9-10, 1, 3-5, 2-3, 8, 10")
	if err != nil {
		t.Fatal(err)
	}
	want := []lineRange{{Start: 1, End: 5}, {Start: 8, End: 10}}
	if fmt.Sprint(ranges) != fmt.Sprint(want) {
		t.Fatalf("canonical ranges = %v, want %v", ranges, want)
	}
}

// changedLinesSaga has one added file, one modified file whose changed lines are
// deliberately nonconsecutive on both sides, and one deleted file, so a single
// comparison exercises gaps, both sides, and several event kinds.
func changedLinesSaga(t *testing.T) (root, repo string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", rangeRepository)
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	writeFile(t, filepath.Join(repo, "internal", "service", "handler.go"), "package service\n\nconst A = 1\nconst B = 2\nconst C = 3\nconst D = 4\nconst E = 5\nconst F = 6\nconst G = 7\n")
	writeFile(t, filepath.Join(repo, "internal", "service", "legacy.go"), "package service\n\nconst Legacy = true\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "feature")
	// Two separated edits: lines 3-4 and line 8 change, everything else is
	// untouched context, so the derived selectors must show a gap.
	writeFile(t, filepath.Join(repo, "internal", "service", "handler.go"), "package service\n\nconst A = 10\nconst B = 20\nconst C = 3\nconst D = 4\nconst E = 5\nconst F = 60\nconst G = 7\n")
	writeFile(t, filepath.Join(repo, "internal", "service", "added.go"), "package service\n\nconst New = 1\n")
	if err := os.Remove(filepath.Join(repo, "internal", "service", "legacy.go")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "feature")

	root = filepath.Join(t.TempDir(), "ranges.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", rangeRepository, "--base", base, "--head", "HEAD", root}, &output); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(overviewFragment(root), "content.md"), "# Range change {#range-change}\n\nThe canonical range coverage test change.\n")
	return root, repo
}

func TestCoverChangedLinesEmitsCanonicalRangesWithGaps(t *testing.T) {
	root, repo := changedLinesSaga(t)
	output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--note", "the modified constants", "--name", "handler", "--json", root)
	if err != nil {
		t.Fatalf("changed-lines cover: %v\n%s", err, output)
	}
	var result coverageMutationOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output)
	}
	references := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "handler.json"))
	base, head := comparisonCommits(t, root, repo)
	got := describeLocations(t, base, head, referenceLocations(references))
	// Each side is emitted once, at the position of its first atom, with its
	// dense runs ascending: the two separated edits stay two ranges per side,
	// deleted lines pinned at the merge-base and added lines at the head.
	want := []string{"old internal/service/handler.go 3-4", "old internal/service/handler.go 8-8", "new internal/service/handler.go 3-4", "new internal/service/handler.go 8-8"}
	if strings.Join(got, "; ") != strings.Join(want, "; ") {
		t.Fatalf("derived references = %v, want %v", got, want)
	}
	if result.References != len(want) {
		t.Fatalf("reference count = %d, want %d", result.References, len(want))
	}
	// Every reference the record produced must carry the record's note; a range
	// stands in for the lines it replaced, not for a different annotation.
	for _, reference := range references {
		if reference.Note != "the modified constants" {
			t.Fatalf("range lost the record note: %#v", reference)
		}
	}
}

// Density is per identity, so ranges must not span the side they belong to.
func TestCoverChangedLinesRespectsSideFilter(t *testing.T) {
	for _, side := range []string{"old", "new"} {
		t.Run(side, func(t *testing.T) {
			root, repo := changedLinesSaga(t)
			if output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--side", side, "--name", "handler", root); err != nil {
				t.Fatalf("changed-lines cover: %v\n%s", err, output)
			}
			references := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "handler.json"))
			base, head := comparisonCommits(t, root, repo)
			got := describeLocations(t, base, head, referenceLocations(references))
			want := []string{
				fmt.Sprintf("%s internal/service/handler.go 3-4", side),
				fmt.Sprintf("%s internal/service/handler.go 8-8", side),
			}
			if strings.Join(got, "; ") != strings.Join(want, "; ") {
				t.Fatalf("%s-side references = %v, want %v", side, got, want)
			}
		})
	}
}

// File events are whole-file references on the side the file exists, their
// lines are dense line ranges, and covering every path must still close the
// saga exactly.
func TestCoverChangedLinesKeepsEventsSeparateAndCoverageExact(t *testing.T) {
	root, repo := changedLinesSaga(t)
	batch := strings.Join([]string{
		`{"path":"internal/service/handler.go","changed_lines":true,"name":"handler","note":"modified constants"}`,
		`{"path":"internal/service/added.go","changed_lines":true,"name":"added","note":"the new file"}`,
		`{"path":"internal/service/legacy.go","changed_lines":true,"name":"legacy","note":"the removed file"}`,
	}, "\n")
	if output, err := runCover(t, batch, "--repo", repo, "--batch", "-", root); err != nil {
		t.Fatalf("batch changed-lines cover: %v\n%s", err, output)
	}

	base, head := comparisonCommits(t, root, repo)
	added := describeLocations(t, base, head, referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, "added.json"))))
	if strings.Join(added, "; ") != "new internal/service/added.go file; new internal/service/added.go 1-3" {
		t.Fatalf("added file references = %v", added)
	}
	deleted := describeLocations(t, base, head, referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, "legacy.json"))))
	if strings.Join(deleted, "; ") != "old internal/service/legacy.go file; old internal/service/legacy.go 1-3" {
		t.Fatalf("deleted file references = %v", deleted)
	}

	report, err := buildReport(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	// Ranged references must own exactly the atoms per-line references owned:
	// nothing uncovered, nothing double-owned, nothing stale.
	if !report.Complete || report.Summary.Uncovered != 0 || report.Summary.Overlapping != 0 || report.Summary.Stale != 0 {
		t.Fatalf("canonical ranges changed coverage: %#v", report.Summary)
	}
	assertValid(t, root)
}

// A range built from consecutive atoms must never reach a line that belongs to
// another target, which is what "dense" buys over "widened".
func TestCoverChangedLinesRangesDoNotStealNeighbouringAtoms(t *testing.T) {
	root, repo := changedLinesSaga(t)
	if output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "handler", root); err != nil {
		t.Fatalf("changed-lines cover: %v\n%s", err, output)
	}
	// Line 5 of the added file is not a changed atom at all; covering the added
	// file's own dense range must leave the handler ranges untouched.
	if output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/added.go", "--changed-lines", "--name", "added", root); err != nil {
		t.Fatalf("added cover: %v\n%s", err, output)
	}
	report, err := buildReport(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Overlapping != 0 {
		t.Fatalf("dense ranges overlapped: %#v", report.Overlaps)
	}
	// The deleted file was intentionally left uncovered, so its atoms must still
	// be reported: a range in another file cannot absorb them.
	if report.Complete || report.Summary.Uncovered == 0 {
		t.Fatalf("uncovered atoms disappeared behind ranges: %#v", report.Summary)
	}
	for _, atom := range report.Uncovered {
		if atom.Path != "internal/service/legacy.go" {
			t.Fatalf("unexpected uncovered atom: %#v", atom)
		}
	}
}
