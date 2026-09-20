package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

// coveredSaga returns a saga whose source comparison contains a single added
// file, plus the checkout to read it from.
func coveredSaga(t *testing.T) (root, repo string) {
	t.Helper()
	repo = shortTempDir(t)
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	git(t, repo, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repo, "internal", "service", "handler.go"), "package service\n\nconst A = 1\nconst B = 2\nconst C = 3\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "feature")

	root = filepath.Join(shortTempDir(t), "batch.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	writeFile(t, filepath.Join(overviewFragment(root), "content.md"), "# Batch change {#batch-change}\n\nThe focused coverage test change.\n")
	return root, repo
}

func runCover(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	err := cover(context.Background(), againstMain(args), &output, strings.NewReader(stdin))
	return output.String(), err
}

// againstMain compares with main, the branch every fixture forks from, unless
// the test chooses how to open the Saga itself.
func againstMain(args []string) []string {
	for _, arg := range args {
		if arg == "--against" || arg == "--head" {
			return args
		}
	}
	return append([]string{"--against", "main"}, args...)
}

func diffRecords(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// A batch must attach exactly the atoms each record names. Batching is a
// delivery optimization; it must never let one record's selector widen to cover
// another record's atoms.
func TestCoverBatchAttachesExactAtomsPerRecord(t *testing.T) {
	root, repo := coveredSaga(t)
	batch := strings.Join([]string{
		`{"path":"internal/service/handler.go","side":"new","lines":"1-2","note":"package declaration","name":"package-line"}`,
		`{"path":"internal/service/handler.go","side":"new","lines":"3-4","note":"first constants","name":"constants"}`,
	}, "\n")

	output, err := runCover(t, batch, "--repo", repo, "--batch", "-", root)
	if err != nil {
		t.Fatalf("batch cover: %v\n%s", err, output)
	}
	if lines := strings.Count(strings.TrimSpace(output), "\n") + 1; lines != 2 {
		t.Fatalf("expected one confirmation per record:\n%s", output)
	}
	assertValid(t, root)

	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	// Line 5 was deliberately left out of the batch, so the saga must still be
	// reported as incomplete rather than quietly covered by a widened range. The
	// add event stays uncovered too: only a whole-file reference covers it.
	if report.Complete || report.Summary.Uncovered != 2 || report.Summary.Overlapping != 0 {
		t.Fatalf("batch coverage was not exact: %#v", report.Summary)
	}
	var uncovered []string
	for _, atom := range report.Uncovered {
		uncovered = append(uncovered, fmt.Sprintf("%s:%s:%d", atom.Kind, atom.Event, atom.Line))
	}
	if strings.Join(uncovered, " ") != "event:add:0 line::5" && strings.Join(uncovered, " ") != "line::5 event:add:0" {
		t.Fatalf("the wrong atoms were left uncovered: %v", uncovered)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 2 {
		t.Fatalf("expected two coverage records, got %v", names)
	}
}

func TestCoverBatchAcceptsJSONArray(t *testing.T) {
	root, repo := coveredSaga(t)
	batch := `[{"path":"internal/service/handler.go","side":"new","lines":"1","name":"one"},
	           {"path":"internal/service/handler.go","side":"new","lines":"3","name":"two"}]`
	if output, err := runCover(t, batch, "--repo", repo, "--batch", "-", root); err != nil {
		t.Fatalf("array batch: %v\n%s", err, output)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 2 {
		t.Fatalf("expected two coverage records, got %v", names)
	}
	assertValid(t, root)
}

func TestCoverChangedLinesSelectsExactFileAtomsAndAddEvent(t *testing.T) {
	root, repo := coveredSaga(t)
	output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "whole-file", "--json", root)
	if err != nil {
		t.Fatalf("changed-lines cover: %v\n%s", err, output)
	}
	var result coverageMutationOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output)
	}
	// An added file is a file event, which only a whole-file reference covers,
	// and five added lines, which one dense line range covers.
	if !result.OK || result.Records != 1 || result.References != 2 {
		t.Fatalf("changed-lines summary = %#v, want a whole-file reference and a line range", result)
	}
	references := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "whole-file.json"))
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if len(references) != 2 || references[0].Location() != (coderef.Location{Commit: head, Path: "internal/service/handler.go"}) ||
		references[1].Location() != (coderef.Location{Commit: head, Path: "internal/service/handler.go", Start: 1, End: 5}) {
		t.Fatalf("added file was not referenced as the whole file and its lines at the head commit: %#v", references)
	}
	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Complete {
		t.Fatalf("all exact file atoms should be covered: %#v", report.Summary)
	}
}

func TestCoverQuietSuppressesLargeBatchOutput(t *testing.T) {
	root, repo := coveredSaga(t)
	batch := `[{"path":"internal/service/handler.go","side":"new","lines":"1","name":"one"},
{"path":"internal/service/handler.go","side":"new","lines":"3","name":"two"}]`
	output, err := runCover(t, batch, "--repo", repo, "--batch", "-", "--quiet", root)
	if err != nil {
		t.Fatal(err)
	}
	if output != "" {
		t.Fatalf("--quiet wrote output: %q", output)
	}
}

func TestCoverJSONReportsFailureWithoutPartialOutput(t *testing.T) {
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	err := Cover(context.Background(), []string{"--against", "main", "--repo", repo, "--path", "internal/service/handler.go", "--side", "sideways", "--lines", "1", "--json", root}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("JSON failure status = %#v", err)
	}
	var result mutationFailureOutput
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil || result.OK || len(result.Failures) != 1 || !strings.Contains(result.Failures[0].Message, "--side must be old or new") {
		t.Fatalf("JSON failure = %#v, err=%v\n%s", result, decodeErr, output.String())
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 0 {
		t.Fatalf("failed JSON mutation left records: %v", names)
	}
}

func TestReplaceAndRemoveCoverageCompleteRepairLoop(t *testing.T) {
	root, repo := coveredSaga(t)
	if output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "broad", root); err != nil {
		t.Fatalf("seed broad coverage: %v\n%s", err, output)
	}
	batch := strings.Join([]string{
		`{"path":"internal/service/handler.go","side":"new","lines":"1-2","name":"declaration","note":"package boundary"}`,
		`{"path":"internal/service/handler.go","side":"new","lines":"3-5","name":"constants","note":"three behavior constants"}`,
		`{"path":"internal/service/handler.go","file":true,"name":"file-add","note":"introduces the service file"}`,
	}, "\n")
	var output bytes.Buffer
	if err := replaceCoverage(context.Background(), []string{"--against", "main", "--record", "___code/broad.json", "--repo", repo, "--batch", "-", "--json", root}, &output, strings.NewReader(batch)); err != nil {
		t.Fatalf("replace coverage: %v\n%s", err, output.String())
	}
	var replaced coverageRepairOutput
	if err := json.Unmarshal(output.Bytes(), &replaced); err != nil || replaced.Records != 3 || replaced.References != 3 {
		t.Fatalf("replacement output = %#v, err=%v\n%s", replaced, err, output.String())
	}
	if _, err := os.Stat(filepath.Join(root, saga.CodeDirName, "broad.json")); !os.IsNotExist(err) {
		t.Fatalf("replaced record still exists: %v", err)
	}
	report, err := buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil || !report.Complete {
		t.Fatalf("split replacement should preserve complete coverage: %#v, %v", report.Summary, err)
	}

	// Removing the constants record reopens exactly its three lines; the
	// whole-file record accounts only for the add event. Removing that record
	// then reopens the event as well.
	output.Reset()
	if err := RemoveCoverage(context.Background(), []string{"--record", "___code/constants.json", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	report, err = buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil || report.Complete || report.Summary.Uncovered != 3 {
		t.Fatalf("removing the constants should reopen exactly their lines: %#v, %v", report.Summary, err)
	}
	output.Reset()
	if err := RemoveCoverage(context.Background(), []string{"--record", "___code/file-add.json", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	report, err = buildReport(context.Background(), root, repo, gitdiff.Range{Against: "main"})
	if err != nil || report.Complete || report.Summary.Uncovered != 4 {
		t.Fatalf("removing focused coverage should reopen exact gaps: %#v, %v", report.Summary, err)
	}
}

func TestReplaceCoverageFailurePreservesOriginalRecord(t *testing.T) {
	root, repo := coveredSaga(t)
	if _, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "broad", root); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	badBatch := `{"path":"internal/service/handler.go","side":"sideways","lines":"1","name":"bad"}`
	err := replaceCoverage(context.Background(), []string{"--against", "main", "--record", "___code/broad.json", "--repo", repo, "--batch", "-", root}, &output, strings.NewReader(badBatch))
	if err == nil {
		t.Fatal("invalid replacement succeeded")
	}
	if _, statErr := os.Stat(filepath.Join(root, saga.CodeDirName, "broad.json")); statErr != nil {
		t.Fatalf("failed replacement removed original: %v", statErr)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 1 {
		t.Fatalf("failed replacement left partial files: %v", names)
	}
}

func TestReplaceCoverageCanAtomicallyReuseTheRecordName(t *testing.T) {
	root, repo := coveredSaga(t)
	if _, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "broad", root); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := replaceCoverage(context.Background(), []string{"--against", "main",
		"--record", "___code/broad.json", "--repo", repo, "--path", "internal/service/handler.go",
		"--side", "new", "--lines", "1", "--name", "broad", "--note", "package declaration", root,
	}, &output, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, saga.CodeDirName, "broad.json")); err != nil {
		t.Fatalf("same-name replacement removed its destination: %v; records=%v; output=%s", err, diffRecords(t, filepath.Join(root, saga.CodeDirName)), output.String())
	}
	references := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "broad.json"))
	if len(references) != 1 || references[0].Note != "package declaration" {
		t.Fatalf("same-name replacement = %#v", references)
	}
}

// A batch is all-or-nothing. A record that cannot be resolved must leave the
// saga exactly as it was, because a half-applied batch silently under-covers
// while looking like it succeeded.
func TestCoverBatchWritesNothingWhenAnyRecordFails(t *testing.T) {
	root, repo := coveredSaga(t)
	for _, test := range []struct {
		name  string
		batch string
		want  string
	}{
		{"bad side", `{"path":"internal/service/handler.go","side":"new","lines":"1","name":"good"}
{"path":"internal/service/handler.go","side":"sideways","lines":"3","name":"bad"}`, "--side must be old or new"},
		{"missing target", `{"path":"internal/service/handler.go","side":"new","lines":"1","name":"good"}
{"target":"nowhere.chapter","path":"internal/service/handler.go","side":"new","lines":"3"}`, "is not a valid"},
		{"duplicate name", `{"path":"internal/service/handler.go","side":"new","lines":"1","name":"same"}
{"path":"internal/service/handler.go","side":"new","lines":"3","name":"same"}`, "collides with record 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := runCover(t, test.batch, "--repo", repo, "--batch", "-", root)
			if err == nil {
				t.Fatalf("expected the batch to fail:\n%s", output)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not explain the failure %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "batch record 2 of 2") {
				t.Fatalf("error %q does not identify the failing record", err)
			}
			if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 0 {
				t.Fatalf("a failed batch left records behind: %v", names)
			}
			if strings.Contains(output, "Added ") {
				t.Fatalf("a failed batch reported a write:\n%s", output)
			}
			assertValid(t, root)
		})
	}
}

// A misspelled field would otherwise be dropped, producing a record that maps
// nothing while reporting success.
func TestCoverBatchRejectsUnknownFields(t *testing.T) {
	root, repo := coveredSaga(t)
	_, err := runCover(t, `{"path":"internal/service/handler.go","side":"new","line":"1"}`, "--repo", repo, "--batch", "-", root)
	if err == nil || !strings.Contains(err.Error(), `unknown field "line"`) {
		t.Fatalf("unknown batch fields must be rejected, got %v", err)
	}
}

func TestCoverBatchRejectsPerRecordFlags(t *testing.T) {
	root, repo := coveredSaga(t)
	_, err := runCover(t, `{"path":"internal/service/handler.go","side":"new","lines":"1"}`,
		"--repo", repo, "--batch", "-", "--path", "internal/service/handler.go", root)
	if err == nil || !strings.Contains(err.Error(), "--path cannot be combined with --batch") {
		t.Fatalf("a per-record flag alongside --batch must be rejected, got %v", err)
	}
}

func TestCoverBatchReadsFromAFile(t *testing.T) {
	root, repo := coveredSaga(t)
	path := filepath.Join(t.TempDir(), "records.jsonl")
	writeFile(t, path, `{"path":"internal/service/handler.go","side":"new","lines":"1","name":"from-file"}`+"\n")
	if output, err := runCover(t, "", "--repo", repo, "--batch", path, root); err != nil {
		t.Fatalf("file batch: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, saga.CodeDirName, "from-file.json")); err != nil {
		t.Fatalf("the file batch was not written: %v", err)
	}
}

// --target and --note are batch-wide defaults so a batch aimed at one narrative
// target does not have to repeat itself on every line.
func TestCoverBatchAppliesTargetAndNoteDefaults(t *testing.T) {
	root, repo := coveredSaga(t)
	var output bytes.Buffer
	if err := AddChapter(context.Background(), []string{"--feature", testFeature, "--title", "Service", root, "service"}, &output); err != nil {
		t.Fatal(err)
	}
	batch := `{"path":"internal/service/handler.go","side":"new","lines":"1","name":"defaulted"}
{"path":"internal/service/handler.go","side":"new","lines":"3","name":"explicit","note":"its own note"}`
	if out, err := runCover(t, batch, "--repo", repo, "--batch", "-", "--target", testFeatureRel+"/service.chapter", "--note", "shared note", root); err != nil {
		t.Fatalf("batch with defaults: %v\n%s", err, out)
	}
	defaulted := readCodeFile(t, filepath.Join(testFeatureDir(root), "service.chapter", saga.CodeDirName, "defaulted.json"))
	if defaulted[0].Note != "shared note" {
		t.Fatalf("the batch-wide note was not applied: %#v", defaulted)
	}
	explicit := readCodeFile(t, filepath.Join(testFeatureDir(root), "service.chapter", saga.CodeDirName, "explicit.json"))
	if explicit[0].Note != "its own note" {
		t.Fatalf("a record note must win over the default: %#v", explicit)
	}
}

// readCodeFile decodes one persisted evidence record strictly, so a renamed or
// leftover field fails the test instead of silently reading as empty.
func readCodeFile(t *testing.T, path string) []coderef.Reference {
	t.Helper()
	var file saga.CodeFile
	if err := readStrictJSONFile(path, &file); err != nil {
		t.Fatalf("read evidence record %s: %v", path, err)
	}
	if file.Version != saga.CurrentVersion {
		t.Fatalf("evidence record %s has version %d", path, file.Version)
	}
	return file.References
}

func TestCoverDryRunResolvesWithoutWriting(t *testing.T) {
	root, repo := coveredSaga(t)
	output, err := runCover(t, "", "--repo", repo, "--path", "internal/service/handler.go", "--side", "new", "--lines", "1,3-4", "--dry-run", root)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Would add") || !strings.Contains(output, "nothing written") {
		t.Fatalf("dry run did not report its plan:\n%s", output)
	}
	if strings.Count(output, ":internal/service/handler.go#L") != 2 || !strings.Contains(output, "#L3-L4") {
		t.Fatalf("dry run did not show the exact selectors it would write:\n%s", output)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 0 {
		t.Fatalf("a dry run wrote %v", names)
	}
	// A dry run must still fail loudly on an unresolvable target.
	if _, err := runCover(t, "", "--repo", repo, "--target", "nowhere", "--path", "internal/service/handler.go", "--side", "new", "--lines", "1", "--dry-run", root); err == nil {
		t.Fatal("a dry run must still resolve its target")
	}
}

func TestCoverExplicitNameCollisionIsReportedClearly(t *testing.T) {
	root, repo := coveredSaga(t)
	if _, err := runCover(t, "", "--repo", repo, "--name", "handler", "--path", "internal/service/handler.go", "--side", "new", "--lines", "1", root); err != nil {
		t.Fatal(err)
	}
	_, err := runCover(t, "", "--repo", repo, "--name", "handler", "--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil {
		t.Fatal("reusing an explicit name must fail rather than overwrite or crash")
	}
	for _, want := range []string{"handler.json", "already exists", "choose a different name"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q is missing %q", err, want)
		}
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 1 {
		t.Fatalf("the failed write changed the record set: %v", names)
	}
}

// Two long names slug to the same 60-character file. The author has to be told
// which stored name they actually collided on, or the error is unexplainable.
func TestCoverReportsTruncatedNameCollisions(t *testing.T) {
	root, repo := coveredSaga(t)
	long := strings.Repeat("a", 70)
	if _, err := runCover(t, "", "--repo", repo, "--name", long+"-first", "--path", "internal/service/handler.go", "--side", "new", "--lines", "1", root); err != nil {
		t.Fatal(err)
	}
	_, err := runCover(t, "", "--repo", repo, "--name", long+"-second", "--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil {
		t.Fatal("names that truncate to the same file must collide loudly")
	}
	if !strings.Contains(err.Error(), "is stored as") {
		t.Fatalf("error %q does not reveal the truncated stored name", err)
	}
}

// Generated names embed a timestamp and random suffix for uniqueness. Slugging
// the joined string truncated it to 60 characters, which for any realistic path
// discarded the uniquifier and made repeated coverage of one file collide.
func TestCoverGeneratedNamesSurviveLongPaths(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	git(t, repo, "checkout", "-b", "feature")
	deep := filepath.Join("internal", "platform", "subsystem", "controller", "reconciliation", "handler_implementation.go")
	writeFile(t, filepath.Join(repo, deep), "package handler\n\nconst A = 1\nconst B = 2\nconst C = 3\nconst D = 4\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "feature")

	root := filepath.Join(t.TempDir(), "deep.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	slashed := filepath.ToSlash(deep)
	for line := 1; line <= 4; line++ {
		if _, err := runCover(t, "", "--repo", repo, "--path", slashed, "--side", "new", "--lines", fmt.Sprint(line), root); err != nil {
			t.Fatalf("cover line %d: %v", line, err)
		}
	}
	names := diffRecords(t, filepath.Join(root, saga.CodeDirName))
	if len(names) != 4 {
		t.Fatalf("generated names collided for a long path: %v", names)
	}
	unique := map[string]bool{}
	for _, name := range names {
		if unique[name] {
			t.Fatalf("duplicate generated name %q", name)
		}
		unique[name] = true
	}
	assertValid(t, root)
}

// A generated path is the logical selector identity. Reusing it must be an
// explicit repair, otherwise two different explanations for the same code
// silently survive a Git merge as an overlap neither author chose.
func TestCoverGeneratedNamesExposeSameSelectorDisagreement(t *testing.T) {
	root, repo := coveredSaga(t)
	args := []string{"--repo", repo, "--path", "internal/service/handler.go", "--side", "new", "--lines", "1", "--note", "first explanation", root}
	if _, err := runCover(t, "", args...); err != nil {
		t.Fatal(err)
	}
	args[9] = "different explanation"
	_, err := runCover(t, "", args...)
	if err == nil || !strings.Contains(err.Error(), "same selector identity") {
		t.Fatalf("same selector should require explicit reconciliation, got %v", err)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 1 {
		t.Fatalf("selector disagreement created duplicate evidence: %v", names)
	}
}

func TestCoverWithoutSelectorsExplainsBatch(t *testing.T) {
	root, repo := coveredSaga(t)
	_, err := runCover(t, "", "--repo", repo, root)
	if err == nil || !strings.Contains(err.Error(), "--batch") {
		t.Fatalf("the empty-invocation error should mention --batch, got %v", err)
	}
}
