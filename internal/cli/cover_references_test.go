package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

const (
	modifiedBase = "package service\n\nconst A = 1\nconst B = 2\nconst C = 3\nconst D = 4\nconst E = 5\nconst F = 6\n"
	modifiedHead = "package service\n\nconst A = 10\nconst B = 20\nconst C = 3\nconst D = 4\nconst E = 5\nconst F = 60\n"
	addedFile    = "package service\n\nconst Added = true\n"
	deletedFile  = "package service\n\nconst Deleted = true\n"
	renamedFile  = "package service\n\nconst Renamed = 1\nconst Kept = 2\n"
	movedBase    = "package service\n\nconst Moved = 1\nconst Unchanged = 2\nconst Tail = 3\n"
	movedHead    = "package service\n\nconst Moved = 100\nconst Unchanged = 2\nconst Tail = 3\n"
)

// fileEventSaga is a comparison holding every kind of file change: a modified
// file with two separated edits, an added file, a deleted file, a pure rename,
// and a rename with an edit.
func fileEventSaga(t *testing.T) (root, repo, base, head string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", rangeRepository)
	writeFile(t, filepath.Join(repo, "service", "modified.go"), modifiedBase)
	writeFile(t, filepath.Join(repo, "service", "deleted.go"), deletedFile)
	writeFile(t, filepath.Join(repo, "service", "before.go"), renamedFile)
	writeFile(t, filepath.Join(repo, "service", "moved.go"), movedBase)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	base = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repo, "service", "modified.go"), modifiedHead)
	writeFile(t, filepath.Join(repo, "service", "added.go"), addedFile)
	git(t, repo, "rm", "-q", "service/deleted.go")
	git(t, repo, "mv", "service/before.go", "service/after.go")
	git(t, repo, "mv", "service/moved.go", "service/relocated.go")
	writeFile(t, filepath.Join(repo, "service", "relocated.go"), movedHead)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "feature")
	head = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	root = filepath.Join(t.TempDir(), "events.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", rangeRepository, root}, &output); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(overviewFragment(root), "content.md"), "# Events {#events}\n\nEvery kind of file change.\n")
	return root, repo, base, head
}

func coverJSON(t *testing.T, args ...string) coverageMutationOutput {
	t.Helper()
	output, err := runCover(t, "", append([]string{"--json"}, args...)...)
	if err != nil {
		t.Fatalf("cover %v: %v\n%s", args, err, output)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &fields); err != nil {
		t.Fatalf("decode cover output: %v\n%s", err, output)
	}
	if _, ok := fields["references"]; !ok {
		t.Fatalf("cover --json omitted references:\n%s", output)
	}
	if _, ok := fields["selectors"]; ok {
		t.Fatalf("cover --json still reports selectors:\n%s", output)
	}
	var result coverageMutationOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// Line references name a comparison side: new pins the head commit, old pins
// the merge-base where the deleted lines still exist. Each records the digest
// of exactly the referenced bytes, read from the repository.
func TestCoverSideLineReferencesPinEachSideWithItsDigest(t *testing.T) {
	root, repo, base, head := fileEventSaga(t)
	coverJSON(t, "--repo", repo, "--path", "service/modified.go", "--side", "new", "--lines", "3-4,8", "--name", "new-side", root)
	coverJSON(t, "--repo", repo, "--path", "service/modified.go", "--side", "old", "--lines", "3-4,8", "--name", "old-side", root)

	newSide := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "new-side.json"))
	wantNew := []coderef.Reference{
		{Commit: head, Path: "service/modified.go", Start: 3, End: 4, Digest: sha256Digest("const A = 10\nconst B = 20\n")},
		{Commit: head, Path: "service/modified.go", Start: 8, End: 8, Digest: sha256Digest("const F = 60\n")},
	}
	if !sameReferences(newSide, wantNew) {
		t.Fatalf("new-side references = %#v\nwant %#v", newSide, wantNew)
	}
	oldSide := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "old-side.json"))
	wantOld := []coderef.Reference{
		{Commit: base, Path: "service/modified.go", Start: 3, End: 4, Digest: sha256Digest("const A = 1\nconst B = 2\n")},
		{Commit: base, Path: "service/modified.go", Start: 8, End: 8, Digest: sha256Digest("const F = 6\n")},
	}
	if !sameReferences(oldSide, wantOld) {
		t.Fatalf("old-side references = %#v\nwant %#v", oldSide, wantOld)
	}

	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	for _, atom := range report.Uncovered {
		if atom.Path == "service/modified.go" {
			t.Fatalf("both sides of the modified file should be covered, %v is not", atom)
		}
	}
	if report.Summary.Stale != 0 || report.Summary.Overlapping != 0 {
		t.Fatalf("side references were stale or overlapping: %#v", report.Summary)
	}
	assertValid(t, root)
}

// --file references the whole file at --path on a side; its digest is the
// digest of the whole file, and it accounts for the file event but not the
// file's lines.
func TestCoverFileReferencesTheWholeFileOnASide(t *testing.T) {
	root, repo, base, head := fileEventSaga(t)
	coverJSON(t, "--repo", repo, "--path", "service/added.go", "--file", "--name", "added", root)
	coverJSON(t, "--repo", repo, "--path", "service/deleted.go", "--side", "old", "--file", "--name", "deleted", root)

	added := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "added.json"))
	if !sameReferences(added, []coderef.Reference{{Commit: head, Path: "service/added.go", Digest: sha256Digest(addedFile)}}) {
		t.Fatalf("added whole-file reference = %#v", added)
	}
	deleted := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "deleted.json"))
	if !sameReferences(deleted, []coderef.Reference{{Commit: base, Path: "service/deleted.go", Digest: sha256Digest(deletedFile)}}) {
		t.Fatalf("deleted whole-file reference = %#v", deleted)
	}
	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, atom := range report.Uncovered {
		if atom.Path != "service/added.go" && atom.Path != "service/deleted.go" {
			continue
		}
		if atom.Kind == "event" {
			t.Fatalf("a whole-file reference must cover its file event; %v is uncovered", atom)
		}
		lines++
	}
	if lines != 6 {
		t.Fatalf("a whole-file reference must leave the file's 6 changed lines to line references; %d are uncovered", lines)
	}

	// A deleted file does not exist at the head, so the new side cannot
	// reference it.
	if _, err := runCover(t, "", "--repo", repo, "--path", "service/deleted.go", "--file", "--name", "missing", root); err == nil || !strings.Contains(err.Error(), "does not exist at commit") {
		t.Fatalf("a whole-file reference to a missing file was accepted: %v", err)
	}
}

// --commit pins a reference at any revision instead of a comparison side, and
// --ref takes a full location; both are digested at authoring time.
func TestCoverCommitAndRefPinOutsideTheComparison(t *testing.T) {
	root, repo, base, head := fileEventSaga(t)
	coverJSON(t, "--repo", repo, "--path", "service/modified.go", "--commit", "main", "--lines", "1", "--name", "at-main", root)
	atMain := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "at-main.json"))
	if !sameReferences(atMain, []coderef.Reference{{Commit: base, Path: "service/modified.go", Start: 1, End: 1, Digest: sha256Digest("package service\n")}}) {
		t.Fatalf("--commit main did not pin the resolved main commit: %#v", atMain)
	}

	result := coverJSON(t, "--repo", repo, "--ref", head+":service/after.go#L3-L4", "--ref", base+":service/before.go", "--note", "renamed constants", "--name", "refs", root)
	if result.Records != 1 || result.References != 2 {
		t.Fatalf("two --ref flags should write one record with two references: %#v", result)
	}
	refs := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "refs.json"))
	want := []coderef.Reference{
		{Commit: head, Path: "service/after.go", Start: 3, End: 4, Digest: sha256Digest("const Renamed = 1\nconst Kept = 2\n"), Note: "renamed constants"},
		{Commit: base, Path: "service/before.go", Digest: sha256Digest(renamedFile), Note: "renamed constants"},
	}
	if !sameReferences(refs, want) {
		t.Fatalf("--ref references = %#v\nwant %#v", refs, want)
	}
	assertValid(t, root)
}

// --changed-lines closes every kind of file change on its own: dense line
// ranges for edits, and whole-file references for file events on the side the
// file exists, plus line ranges for that file's changed lines.
func TestCoverChangedLinesCompletesEveryFileEvent(t *testing.T) {
	root, repo, base, head := fileEventSaga(t)
	for _, test := range []struct {
		path string
		want []string
	}{
		{"service/modified.go", []string{"old service/modified.go 3-4", "old service/modified.go 8-8", "new service/modified.go 3-4", "new service/modified.go 8-8"}},
		{"service/added.go", []string{"new service/added.go file", "new service/added.go 1-3"}},
		{"service/deleted.go", []string{"old service/deleted.go file", "old service/deleted.go 1-3"}},
		{"service/after.go", []string{"new service/after.go file"}},
		// Either path of an edited rename selects the rename event and the
		// changed lines on both sides.
		{"service/moved.go", []string{"new service/relocated.go file", "old service/moved.go 3-3", "new service/relocated.go 3-3"}},
	} {
		name := strings.TrimSuffix(filepath.Base(test.path), ".go")
		coverJSON(t, "--repo", repo, "--path", test.path, "--changed-lines", "--name", name, root)
		got := describeLocations(t, base, head, referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, name+".json"))))
		if strings.Join(got, "; ") != strings.Join(test.want, "; ") {
			t.Fatalf("%s references = %v, want %v", test.path, got, test.want)
		}
	}
	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete || report.Summary.Uncovered != 0 || report.Summary.Overlapping != 0 || report.Summary.Stale != 0 {
		t.Fatalf("changed-lines coverage of every file event is not exact: %#v uncovered=%v", report.Summary, report.Uncovered)
	}
	// Every whole-file digest is the digest of the file on its side.
	for name, content := range map[string]string{"added": addedFile, "deleted": deletedFile, "after": renamedFile, "moved": movedHead} {
		references := readCodeFile(t, filepath.Join(root, saga.CodeDirName, name+".json"))
		if references[0].Digest != sha256Digest(content) {
			t.Fatalf("%s whole-file digest %s does not match the file", name, references[0].Digest)
		}
	}
	// The new path of the rename selects the same references as the old one.
	moved := referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, "moved.json")))
	if err := os.Remove(filepath.Join(root, saga.CodeDirName, "moved.json")); err != nil {
		t.Fatal(err)
	}
	coverJSON(t, "--repo", repo, "--path", "service/relocated.go", "--changed-lines", "--name", "relocated", root)
	if relocated := referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, "relocated.json"))); !reflect.DeepEqual(relocated, moved) {
		t.Fatalf("the new path of a rename references %v, the old path %v", relocated, moved)
	}
	assertValid(t, root)
}

// A renamed file is one file: its new path selects the deleted lines at the
// old path too, exactly as its old path does.
func TestCoverChangedLinesSelectsBothPathsOfARename(t *testing.T) {
	for _, path := range []string{"service/relocated.go", "service/moved.go"} {
		t.Run(path, func(t *testing.T) {
			root, repo, base, head := fileEventSaga(t)
			coverJSON(t, "--repo", repo, "--path", path, "--changed-lines", "--name", "rename", root)
			got := describeLocations(t, base, head, referenceLocations(readCodeFile(t, filepath.Join(root, saga.CodeDirName, "rename.json"))))
			if want := "new service/relocated.go file; old service/moved.go 3-3; new service/relocated.go 3-3"; strings.Join(got, "; ") != want {
				t.Fatalf("references = %v, want %s", got, want)
			}
			report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
			if err != nil {
				t.Fatal(err)
			}
			for _, atom := range report.Uncovered {
				if atom.Path == "service/moved.go" || atom.Path == "service/relocated.go" {
					t.Fatalf("the rename left %s uncovered", atom.Ref)
				}
			}
		})
	}
}

// A batch record spells every cover flag as a field, and each record is
// resolved exactly as the equivalent invocation would be.
func TestCoverBatchRecordSpellsEveryReferenceFlag(t *testing.T) {
	root, repo, base, head := fileEventSaga(t)
	batch := strings.Join([]string{
		`{"path":"service/modified.go","side":"old","lines":"3-4","name":"old-lines","note":"old constants"}`,
		`{"path":"service/added.go","file":true,"name":"added-file"}`,
		`{"path":"service/modified.go","commit":"main","lines":"8","name":"at-main"}`,
		`{"refs":["` + head + `:service/after.go"],"name":"refs"}`,
		`{"path":"service/deleted.go","changed_lines":true,"name":"changed"}`,
	}, "\n")
	output, err := runCover(t, batch, "--repo", repo, "--batch", "-", "--json", root)
	if err != nil {
		t.Fatalf("batch: %v\n%s", err, output)
	}
	var result coverageMutationOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil || result.Records != 5 || result.References != 6 {
		t.Fatalf("batch summary = %#v err=%v\n%s", result, err, output)
	}
	for name, want := range map[string]coderef.Reference{
		"old-lines":  {Commit: base, Path: "service/modified.go", Start: 3, End: 4, Digest: sha256Digest("const A = 1\nconst B = 2\n"), Note: "old constants"},
		"added-file": {Commit: head, Path: "service/added.go", Digest: sha256Digest(addedFile)},
		"at-main":    {Commit: base, Path: "service/modified.go", Start: 8, End: 8, Digest: sha256Digest("const F = 6\n")},
		"refs":       {Commit: head, Path: "service/after.go", Digest: sha256Digest(renamedFile)},
	} {
		got := readCodeFile(t, filepath.Join(root, saga.CodeDirName, name+".json"))
		if !sameReferences(got, []coderef.Reference{want}) {
			t.Fatalf("batch record %s = %#v, want %#v", name, got, want)
		}
	}
	changed := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "changed.json"))
	if !sameReferences(changed, []coderef.Reference{
		{Commit: base, Path: "service/deleted.go", Digest: sha256Digest(deletedFile)},
		{Commit: base, Path: "service/deleted.go", Start: 1, End: 3, Digest: sha256Digest(deletedFile)},
	}) {
		t.Fatalf("batch record changed = %#v", changed)
	}
	assertValid(t, root)
}

func TestCoverRejectsContradictoryReferenceFlags(t *testing.T) {
	root, repo, _, head := fileEventSaga(t)
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"lines without a side", []string{"--path", "service/modified.go", "--lines", "3"}, "need --side new or old"},
		{"unknown side", []string{"--path", "service/modified.go", "--side", "left", "--lines", "3"}, "--side must be old or new"},
		{"side with commit", []string{"--path", "service/modified.go", "--side", "new", "--commit", "main", "--lines", "3"}, "cannot be combined with --commit"},
		{"file with lines", []string{"--path", "service/modified.go", "--side", "new", "--file", "--lines", "3"}, "cannot be combined with --lines"},
		{"lines without a path", []string{"--side", "new", "--lines", "3"}, "provide --ref or --path"},
		{"file without a path", []string{"--file", "--ref", head + ":service/added.go"}, "require --path"},
		{"commit without a path", []string{"--commit", "main", "--ref", head + ":service/added.go"}, "require --path"},
		{"changed lines without a path", []string{"--changed-lines"}, "--changed-lines requires --path"},
		{"changed lines with lines", []string{"--path", "service/modified.go", "--changed-lines", "--lines", "3"}, "cannot be combined"},
		{"changed lines of an unchanged path", []string{"--path", "README.md", "--changed-lines"}, "has no changed atoms"},
		{"unknown commit", []string{"--path", "service/modified.go", "--commit", "no-such-branch", "--lines", "1"}, "resolve --commit"},
		{"abbreviated ref", []string{"--ref", head[:12] + ":service/added.go"}, "invalid --ref"},
		{"noncanonical ref", []string{"--ref", head + ":service/added.go#L2-L2"}, "invalid --ref"},
		{"lines past the end", []string{"--path", "service/added.go", "--side", "new", "--lines", "3-9"}, "outside the file"},
		{"retired event flag", []string{"--path", "service/added.go", "--event", "add"}, "flag provided but not defined"},
		{"retired uri flag", []string{"--uri", "saga-diff://v1/file?x"}, "flag provided but not defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runCover(t, "", append(append([]string{"--repo", repo}, test.args...), root)...)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q\n%s", err, test.want, output)
			}
			if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 0 {
				t.Fatalf("a refused invocation wrote %v", names)
			}
		})
	}
}

// A JSON failure keeps the success shape, with references rather than the
// retired selectors count.
func TestCoverJSONFailureReportsReferencesField(t *testing.T) {
	root, repo, _, _ := fileEventSaga(t)
	var output bytes.Buffer
	err := Cover(context.Background(), []string{"--against", "main", "--repo", repo, "--path", "service/modified.go", "--lines", "3", "--json", root}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("JSON failure status = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["references"]) != "0" || fields["selectors"] != nil {
		t.Fatalf("failure JSON = %s", output.String())
	}
}

func sameReferences(got, want []coderef.Reference) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
