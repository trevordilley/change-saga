package coverage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

const (
	testRepository = "https://example.test/acme/app.git"
	testBase       = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testHead       = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

var testDigest = coderef.DigestBytes(nil)

func BenchmarkEvaluateLargeMappedDiff(b *testing.B) {
	const (
		atomCount    = 10_000
		mappingCount = 2_000
	)
	atoms := make([]gitdiff.Atom, atomCount)
	for i := range atoms {
		atoms[i] = gitdiff.Atom{Key: fmt.Sprintf("line:large.go:new:%d", i+1), Kind: "line", Path: "large.go", Side: "new", Line: i + 1}
	}
	references := make([]coderef.Reference, mappingCount)
	for i := range references {
		references[i] = lineReference(testHead, "large.go", i*5+1, i*5+1)
	}
	document := &saga.Saga{Section: &saga.Section{
		Target: "urn:change-saga:benchmark:saga",
		Code:   []saga.CodeFile{{Path: "___code/large.json", References: references}},
	}}
	changes := gitdiff.ChangeSet{Repository: testRepository, BaseOID: testBase, HeadOID: testHead, Atoms: atoms}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		report := Evaluate(context.Background(), document, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
		if report.Summary.Covered != mappingCount {
			b.Fatalf("covered = %d, want %d", report.Summary.Covered, mappingCount)
		}
	}
}

func lineReference(commit, path string, start, end int) coderef.Reference {
	return coderef.Reference{Commit: commit, Path: path, Start: start, End: end, Digest: testDigest}
}

func fileReference(commit, path string) coderef.Reference {
	return coderef.Reference{Commit: commit, Path: path, Digest: testDigest}
}

func lineAtom(path, side string, line int) gitdiff.Atom {
	return gitdiff.Atom{Key: fmt.Sprintf("line:%s:%s:%d", path, side, line), Kind: "line", Path: path, Side: side, Line: line}
}

func eventAtom(event, path string) gitdiff.Atom {
	return gitdiff.Atom{Key: "event:" + event + ":" + path + "::", Kind: "event", Event: event, Path: path}
}

func documentWith(targets map[string][]coderef.Reference) *saga.Saga {
	root := &saga.Section{Target: "urn:change-saga:test:saga"}
	for target, references := range targets {
		if target == root.Target {
			root.Code = append(root.Code, saga.CodeFile{Path: "___code/root.json", References: references})
			continue
		}
		root.Fragments = append(root.Fragments, &saga.Fragment{Target: target, Code: []saga.CodeFile{{Path: target + "/___code/e.json", References: references}}})
	}
	return &saga.Saga{Section: root}
}

func TestEvaluateMatchesEachSideAtItsOwnCommit(t *testing.T) {
	changes := gitdiff.ChangeSet{BaseOID: testBase, HeadOID: testHead, Atoms: []gitdiff.Atom{
		lineAtom("app.go", "new", 4), lineAtom("app.go", "new", 5), lineAtom("app.go", "old", 4),
		eventAtom("add", "new.go"), lineAtom("new.go", "new", 1),
		eventAtom("delete", "gone.go"), lineAtom("gone.go", "old", 1),
		lineAtom("app.go", "new", 9),
	}}
	document := documentWith(map[string][]coderef.Reference{
		"urn:change-saga:test:fragment:a": {
			lineReference(testHead, "app.go", 4, 5),
			// A deletion references the base commit's code.
			lineReference(testBase, "app.go", 4, 4),
			// A whole file accounts for its event; its lines need line references.
			fileReference(testHead, "new.go"),
			lineReference(testHead, "new.go", 1, 1),
			fileReference(testBase, "gone.go"),
			lineReference(testBase, "gone.go", 1, 1),
		},
		"urn:change-saga:test:fragment:b": {
			lineReference(testHead, "app.go", 5, 5),
			// Pinned elsewhere: current at neither side, so stale.
			lineReference(strings.Repeat("c", 40), "app.go", 1, 2),
		},
	})
	report := Evaluate(context.Background(), document, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if report.Summary.Covered != 7 || report.Summary.Uncovered != 1 || report.Uncovered[0].Line != 9 {
		t.Fatalf("summary %+v uncovered %+v", report.Summary, report.Uncovered)
	}
	if len(report.Overlaps) != 1 || report.Overlaps[0].Atom.Line != 5 {
		t.Fatalf("overlaps %+v", report.Overlaps)
	}
	if len(report.StaleReferences) != 1 || report.StaleReferences[0].Assignment.Reference != 2 || report.Complete {
		t.Fatalf("stale %+v complete %v", report.StaleReferences, report.Complete)
	}
	summary := EvaluateSummary(context.Background(), document, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if summary.Summary != report.Summary {
		t.Fatalf("summary evaluation %+v != %+v", summary.Summary, report.Summary)
	}
	selected := SelectTarget(context.Background(), document.Section.Fragments[1].Code, changes, coderesolve.Pinned{})
	if len(selected) != 1 || selected[0].Line != 5 {
		t.Fatalf("selected %+v", selected)
	}
}

// File events and lines are accounted for separately, so ownership is never
// wider than what a reference names.
func TestEvaluateKeepsFileEventsAndLinesSeparate(t *testing.T) {
	changes := gitdiff.ChangeSet{BaseOID: testBase, HeadOID: testHead, Atoms: []gitdiff.Atom{eventAtom("add", "new.go"), lineAtom("new.go", "new", 1)}}
	lines := documentWith(map[string][]coderef.Reference{"urn:change-saga:test:saga": {lineReference(testHead, "new.go", 1, 1)}})
	report := Evaluate(context.Background(), lines, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if report.Complete || len(report.Uncovered) != 1 || report.Uncovered[0].Event != "add" {
		t.Fatalf("file events need whole-file references: %+v", report.Uncovered)
	}
	file := documentWith(map[string][]coderef.Reference{"urn:change-saga:test:saga": {fileReference(testHead, "new.go")}})
	report = Evaluate(context.Background(), file, saga.Validation{Valid: true}, changes, coderesolve.Pinned{})
	if report.Complete || len(report.Uncovered) != 1 || report.Uncovered[0].Kind != "line" {
		t.Fatalf("a whole-file reference must not account for the file's lines: %+v", report.Uncovered)
	}
}

// The point of references: evidence authored at one commit keeps covering its
// lines after unrelated commits shift them, and goes stale only when they change.
func TestEvaluateRemapsAcrossLaterCommits(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	run(t, repo, "init", "-q", "-b", "main")
	writeFile(t, repo, "app.go", "package app\n\nfunc A() {}\n")
	base := commit(t, repo, "base")
	run(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, repo, "app.go", "package app\n\nfunc A() {}\n\nfunc B() {}\n")
	pinned := commit(t, repo, "add B")

	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	reference, err := resolver.Author(ctx, coderef.Location{Commit: pinned, Path: "app.go", Start: 4, End: 5}, "")
	if err != nil {
		t.Fatal(err)
	}
	document := documentWith(map[string][]coderef.Reference{"urn:change-saga:test:saga": {reference}})
	evaluate := func() Report {
		changes, err := gitdiff.ReadWithOptions(ctx, repo, testRepository, base, "HEAD", gitdiff.ReadOptions{AllowRepositoryMismatch: true})
		if err != nil {
			t.Fatal(err)
		}
		return Evaluate(ctx, document, saga.Validation{Valid: true}, changes, resolver)
	}
	if report := evaluate(); !report.Complete {
		t.Fatalf("at the pin: %+v %+v", report.Summary, report.Uncovered)
	}

	writeFile(t, repo, "app.go", "package app\n\n// A does nothing.\nfunc A() {}\n\nfunc B() {}\n")
	commit(t, repo, "document A")
	writeFile(t, repo, "notes.saga/saga.json", "{}\n")
	commit(t, repo, "saga only")
	report := evaluate()
	if report.Summary.Uncovered != 1 || report.Uncovered[0].Content != "// A does nothing." || report.Summary.Remapped != 1 || len(report.StaleReferences) != 0 {
		t.Fatalf("after an unrelated shift: %+v %+v", report.Summary, report.Uncovered)
	}

	writeFile(t, repo, "app.go", "package app\n\n// A does nothing.\nfunc A() {}\n\nfunc B() int { return 1 }\n")
	commit(t, repo, "change B")
	report = evaluate()
	if len(report.StaleReferences) != 1 || !strings.Contains(report.StaleReferences[0].Reason, "changed") {
		t.Fatalf("after changing the referenced lines: %+v", report.StaleReferences)
	}
}

func TestReportCollectionsAreNeverNullInJSON(t *testing.T) {
	covered := documentWith(map[string][]coderef.Reference{"urn:change-saga:test:saga": {lineReference(testHead, "app.go", 1, 1)}})
	empty := &saga.Saga{Section: &saga.Section{Target: "urn:change-saga:contract:saga"}}
	for _, test := range []struct {
		name     string
		document *saga.Saga
		changes  gitdiff.ChangeSet
		fields   []string
	}{
		{"fully covered comparison", covered, gitdiff.ChangeSet{BaseOID: testBase, HeadOID: testHead, Atoms: []gitdiff.Atom{lineAtom("app.go", "new", 1)}},
			[]string{"uncovered", "overlaps", "stale_references", "saga_changes", "schema_issues"}},
		{"empty comparison", empty, gitdiff.ChangeSet{BaseOID: testBase, HeadOID: testHead},
			[]string{"uncovered", "overlaps", "stale_references", "targets", "saga_changes", "schema_issues"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			report := Evaluate(context.Background(), test.document, saga.Validation{Valid: true}, test.changes, coderesolve.Pinned{})
			if !report.Complete {
				t.Fatalf("expected a complete report: %#v", report.Summary)
			}
			encoded, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var decoded map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			for _, field := range test.fields {
				if value, present := decoded[field]; !present || string(value) != "[]" {
					t.Fatalf("%q encoded as %s, want []", field, value)
				}
			}
		})
	}
}

// An empty changed line has no content, and omitting the field made a consumer
// that reads atom["content"] unconditionally fail on exactly those atoms.
func TestAtomAlwaysCarriesContent(t *testing.T) {
	encoded, err := json.Marshal(lineAtom("app.go", "new", 1))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"content":""`)) {
		t.Fatalf("a blank changed line dropped its content field: %s", encoded)
	}
}

func writeFile(t *testing.T, repo, name, content string) {
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
	run(t, repo, "add", "-A")
	run(t, repo, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", message)
	return strings.TrimSpace(run(t, repo, "rev-parse", "HEAD"))
}

func run(t *testing.T, repo string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}
