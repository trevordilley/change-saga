package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

// sourceRepo creates a repository holding files in one commit and returns the
// checkout and that commit, for authoring references outside a comparison.
func sourceRepo(t *testing.T, files map[string]string) (repo, commit string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		writeFile(t, filepath.Join(repo, filepath.FromSlash(path)), files[path])
	}
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "source")
	return repo, strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

// sha256Digest is the digest a reference records for exactly these bytes,
// computed independently of the production digest code.
func sha256Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func runReferences(t *testing.T, args ...string) referencesOutput {
	t.Helper()
	var output bytes.Buffer
	if err := References(context.Background(), append([]string{"--json"}, args...), &output); err != nil {
		t.Fatalf("references %v: %v\n%s", args, err, output.String())
	}
	var result referencesOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode references output: %v\n%s", err, output.String())
	}
	return result
}

func runRepin(t *testing.T, args ...string) repinOutput {
	t.Helper()
	var output bytes.Buffer
	if err := Repin(context.Background(), append([]string{"--json"}, args...), &output); err != nil {
		t.Fatalf("repin %v: %v\n%s", args, err, output.String())
	}
	var result repinOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode repin output: %v\n%s", err, output.String())
	}
	return result
}

// statusComplete runs status --json and returns whether changed-source
// accounting is complete, with the decoded report. Status exits 3 here because
// the Saga records no accepted story; that readiness gate is not under test.
func statusComplete(t *testing.T, root string) (bool, map[string]json.RawMessage) {
	t.Helper()
	var output bytes.Buffer
	err := Status(context.Background(), []string{"--json", root}, &output)
	var exit *StatusError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("status: %v\n%s", err, output.String())
	}
	var report map[string]json.RawMessage
	if decodeErr := json.Unmarshal(output.Bytes(), &report); decodeErr != nil {
		t.Fatalf("status --json: %v\n%s", decodeErr, output.String())
	}
	return string(report["complete"]) == "true", report
}

func commitAll(t *testing.T, repo, message string) string {
	t.Helper()
	advanceGitClock(t)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", message)
	return strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

// advanceGitClock gives the next commit a timestamp one minute after the
// previous one. Commits made within one second tie on date, and the order Git
// lists tied commits in is not their ancestry order.
func advanceGitClock(t *testing.T) {
	t.Helper()
	gitClock = gitClock.Add(time.Minute)
	stamp := gitClock.Format(time.RFC3339)
	t.Setenv("GIT_AUTHOR_DATE", stamp)
	t.Setenv("GIT_COMMITTER_DATE", stamp)
}

var gitClock = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

func commitExists(repo, commit string) bool {
	command := exec.Command("git", "cat-file", "-e", commit+"^{commit}")
	command.Dir = repo
	return command.Run() == nil
}

func findHealth(t *testing.T, result referencesOutput, kind, path string) referenceHealth {
	t.Helper()
	for _, health := range result.References {
		if health.Kind == kind && health.Code.Path == path {
			return health
		}
	}
	t.Fatalf("references output has no %s reference to %s: %#v", kind, path, result.References)
	return referenceHealth{}
}

const (
	appBase     = "package app\n\nfunc A() {}\n"
	appFeature  = "package app\n\nfunc A() {}\n\nfunc B() int {\n\treturn 1\n}\n"
	appHeader   = "// Header one.\n// Header two.\n"
	appChanged  = "package app\n\nfunc A() {}\n\nfunc B() int {\n\treturn 2\n}\n"
	utilFeature = "package app\n\nfunc U() {}\n"
)

// The point of code references, end to end through the CLI: evidence authored
// at one commit keeps covering its lines while unrelated commits shift them
// and Saga-only commits land, goes stale with a reason and a diff only when
// the referenced lines change, and survives a squash merge whose branch
// commits are gone by being re-pinned to the landed commit.
func TestReferencesSurviveShiftsGoStaleOnEditsAndRepinAfterSquash(t *testing.T) {
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "config", "commit.gpgSign", "false")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "app.go"), appBase)
	commitAll(t, repo, "Base")
	git(t, repo, "checkout", "-q", "-b", "feature")
	writeFile(t, filepath.Join(repo, "app.go"), appFeature)
	writeFile(t, filepath.Join(repo, "util.go"), utilFeature)
	authored := commitAll(t, repo, "Add B and U")

	root := filepath.Join(repo, "change.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--base", "main", "--head", "HEAD", "--title", "Feature B", root}, &output); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "overview.fragment", "content.md"), "# Feature B {#feature-b}\n\nAdds B and U.\n")
	coverJSON(t, "--path", "app.go", "--changed-lines", "--name", "app", "--note", "B returns a value", root)
	coverJSON(t, "--path", "util.go", "--changed-lines", "--name", "util", root)
	app := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json"))
	wantApp := coderef.Reference{Commit: authored, Path: "app.go", Start: 4, End: 7, Digest: sha256Digest("\nfunc B() int {\n\treturn 1\n}\n"), Note: "B returns a value"}
	if !sameReferences(app, []coderef.Reference{wantApp}) {
		t.Fatalf("authored reference = %#v, want %#v", app, wantApp)
	}

	// A commit that only touches the Saga changes no product code: the
	// references stay current at their original lines.
	commitAll(t, repo, "Record evidence")
	if complete, report := statusComplete(t, root); !complete {
		t.Fatalf("a Saga-only commit broke coverage: %s", report["summary"])
	}
	result := runReferences(t, root)
	if result.Total != 2 || result.Current != 2 || result.Stale != 0 || result.Remapped != 0 {
		t.Fatalf("after a Saga-only commit: %#v", result)
	}

	// An unrelated commit on main shifts app.go down two lines; merging it into
	// the branch moves the referenced lines without changing them.
	git(t, repo, "checkout", "-q", "main")
	writeFile(t, filepath.Join(repo, "app.go"), appHeader+appBase)
	commitAll(t, repo, "Add a header")
	git(t, repo, "checkout", "-q", "feature")
	advanceGitClock(t)
	git(t, repo, "merge", "-q", "--no-edit", "main")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if complete, report := statusComplete(t, root); !complete {
		t.Fatalf("shifted lines broke coverage: %s uncovered=%s stale=%s", report["summary"], report["uncovered"], report["stale_references"])
	}
	result = runReferences(t, root)
	if result.Total != 2 || result.Current != 2 || result.Remapped != 1 || result.Stale != 0 || result.Head != head {
		t.Fatalf("after shifting lines: %#v", result)
	}
	moved := findHealth(t, result, "evidence", "app.go")
	if moved.State != "current" || !moved.Moved || moved.Head == nil || *moved.Head != (coderef.Location{Commit: head, Path: "app.go", Start: 6, End: 9}) || moved.Pinned != wantApp.Location() {
		t.Fatalf("the shifted reference was not remapped to lines 6-9 at the head: %#v", moved)
	}
	if util := findHealth(t, result, "evidence", "util.go"); util.State != "current" || util.Moved {
		t.Fatalf("an untouched whole-file reference moved: %#v", util)
	}

	// Editing a referenced line makes the reference stale, with the reason and
	// the patch since its pin.
	writeFile(t, filepath.Join(repo, "app.go"), appHeader+appChanged)
	commitAll(t, repo, "Return two")
	stale := runReferences(t, "--stale", "--diff", root)
	if stale.Total != 2 || stale.Stale != 1 || stale.Current != 1 || len(stale.References) != 1 {
		t.Fatalf("--stale should list exactly the edited reference: %#v", stale)
	}
	edited := stale.References[0]
	if edited.EvidenceFile != "___code/app.json" || edited.State != "stale" || !strings.Contains(edited.Reason, "lines 4-7 of app.go changed") {
		t.Fatalf("stale reference = %#v", edited)
	}
	if !strings.Contains(edited.Diff, "-\treturn 1") || !strings.Contains(edited.Diff, "+\treturn 2") {
		t.Fatalf("stale reference diff does not show the edit:\n%s", edited.Diff)
	}
	if complete, report := statusComplete(t, root); complete || string(report["stale_references"]) == "[]" {
		t.Fatalf("status did not report the stale reference: complete=%v stale=%s", complete, report["stale_references"])
	}

	// Re-author the evidence at the new head and support a claim with it.
	var replaced bytes.Buffer
	if err := replaceCoverage(context.Background(), []string{"--record", "___code/app.json", "--path", "app.go", "--changed-lines", "--name", "app", root}, &replaced, strings.NewReader("")); err != nil {
		t.Fatalf("replace-coverage: %v\n%s", err, replaced.String())
	}
	edit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	if err := AddClaim(context.Background(), []string{"--id", "b-returns-two", "--target", "overview.fragment", "--statement", "B returns two.", "--ref", edit + ":app.go#L7-L9", root}, &output); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "Re-author evidence")
	if complete, report := statusComplete(t, root); !complete {
		t.Fatalf("re-authored evidence did not restore coverage: %s", report["summary"])
	}

	// Rewrite the branch tip so the commit the evidence and claim are pinned at
	// is no longer reachable, and collect it.
	git(t, repo, "reset", "-q", "--soft", "HEAD~2")
	commitAll(t, repo, "Return two, with evidence")
	git(t, repo, "reflog", "expire", "--expire=now", "--all")
	git(t, repo, "gc", "-q", "--prune=now")
	if commitExists(repo, edit) {
		t.Skip("git gc kept the rewritten commit; the digest fallback cannot be exercised here")
	}
	if !commitExists(repo, authored) {
		t.Fatal("the branch's first commit should still be reachable")
	}

	// Squash-merge onto main, then re-pin while the branch still exists so its
	// commit messages can be recorded with the change.
	git(t, repo, "checkout", "-q", "main")
	git(t, repo, "merge", "-q", "--squash", "feature")
	landed := commitAll(t, repo, "Ship feature B")
	evidenceBefore := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json"))

	dryRun := runRepin(t, "--onto", "main", "--branch", "feature", "--dry-run", root)
	if !dryRun.DryRun || len(dryRun.Repinned) != 2 || dryRun.MergeRecord != "" {
		t.Fatalf("dry run = %#v", dryRun)
	}
	if after := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json")); !sameReferences(after, evidenceBefore) {
		t.Fatalf("a dry run rewrote evidence: %#v", after)
	}
	if _, err := os.Stat(filepath.Join(root, saga.MergesDir)); !os.IsNotExist(err) {
		t.Fatalf("a dry run wrote a merge record: %v", err)
	}

	repinned := runRepin(t, "--onto", "main", "--branch", "feature", root)
	if repinned.Onto != landed || len(repinned.Left) != 0 {
		t.Fatalf("repin = %#v", repinned)
	}
	changes := map[string]repinChange{}
	for _, change := range repinned.Repinned {
		changes[change.EvidenceFile] = change
	}
	appChange := changes["___code/app.json"]
	if appChange.From != (coderef.Location{Commit: edit, Path: "app.go", Start: 6, End: 9}) || appChange.To != (coderef.Location{Commit: landed, Path: "app.go", Start: 6, End: 9}) || !appChange.ByDigest {
		t.Fatalf("the evidence whose pin is gone was not found by digest at the landed commit: %#v", appChange)
	}
	utilChange := changes["___code/util.json"]
	if utilChange.From != (coderef.Location{Commit: authored, Path: "util.go"}) || utilChange.To != (coderef.Location{Commit: landed, Path: "util.go"}) || utilChange.ByDigest {
		t.Fatalf("the evidence whose pin exists was not remapped to the landed commit: %#v", utilChange)
	}
	// The claim is an immutable record: it keeps its pin and resolves by
	// digest instead.
	if repinned.Unchanged != 1 {
		t.Fatalf("the claim should be left pinned, counted as unchanged: %#v", repinned)
	}
	evidence := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json"))
	if !sameReferences(evidence, []coderef.Reference{{Commit: landed, Path: "app.go", Start: 6, End: 9, Digest: sha256Digest("\nfunc B() int {\n\treturn 2\n}\n")}}) {
		t.Fatalf("re-pinned evidence record = %#v", evidence)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("the re-pinned Saga is invalid: err=%v issues=%#v", err, validation.Issues)
	}
	if claim := document.Claims[0].Evidence[0]; claim.Commit != edit {
		t.Fatalf("repin rewrote the claim's pin: %#v", claim)
	}

	// The branch's commit messages are recorded oldest first with the change.
	if repinned.MergeRecord != saga.MergesDir+"/"+landed+".json" || len(document.Merges) != 1 || document.Merges[0].Commit != landed {
		t.Fatalf("merge record = %q, loaded %#v", repinned.MergeRecord, document.Merges)
	}
	var subjects []string
	for _, commit := range document.Merges[0].Commits {
		subjects = append(subjects, commit.Subject)
		if commit.Author != "Test Author <test@example.test>" || commit.Date.IsZero() {
			t.Fatalf("merged commit lacks author or date: %#v", commit)
		}
	}
	wantSubjects := []string{"Add B and U", "Record evidence", "Merge branch 'main' into feature", "Return two, with evidence"}
	if strings.Join(subjects, "|") != strings.Join(wantSubjects, "|") {
		t.Fatalf("recorded commit messages = %q, want %q", subjects, wantSubjects)
	}

	// Once the branch is deleted and collected, every reference still resolves
	// at the landed commit: the evidence by its new pin, the claim by digest.
	git(t, repo, "branch", "-q", "-D", "feature")
	git(t, repo, "reflog", "expire", "--expire=now", "--all")
	git(t, repo, "gc", "-q", "--prune=now")
	assertValid(t, root)
	final := runReferences(t, root)
	if final.Total != 3 || final.Stale != 0 || final.Current != 3 {
		t.Fatalf("after deleting the branch: %#v", final)
	}
	if claim := findHealth(t, final, "claim", "app.go"); claim.State != "current" || claim.Head == nil || *claim.Head != (coderef.Location{Commit: landed, Path: "app.go", Start: 7, End: 9}) {
		t.Fatalf("the claim did not resolve by digest at the landed commit: %#v", claim)
	}
}

func TestReferencesAndRepinRejectBadArguments(t *testing.T) {
	root, repo := coveredSaga(t)
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	for _, test := range []struct {
		name string
		run  func(args []string, out *bytes.Buffer) error
		args []string
		want string
	}{
		{"references without a saga", referencesCommand, []string{"--repo", repo}, "usage: change-saga references"},
		{"references with two sagas", referencesCommand, []string{"--repo", repo, root, root}, "usage: change-saga references"},
		{"references with an unknown flag", referencesCommand, []string{"--orphans", root}, "flag provided but not defined"},
		{"references with a missing saga", referencesCommand, []string{"--repo", repo, filepath.Join(t.TempDir(), "missing.saga")}, "missing.saga"},
		{"references outside a repository", referencesCommand, []string{"--repo", t.TempDir(), root}, "read source comparison"},
		{"repin without --onto", repinCommand, []string{"--repo", repo, root}, "usage: change-saga repin"},
		{"repin with a blank --onto", repinCommand, []string{"--repo", repo, "--onto", " ", root}, "usage: change-saga repin"},
		{"repin without a saga", repinCommand, []string{"--repo", repo, "--onto", head}, "usage: change-saga repin"},
		{"repin onto an unknown revision", repinCommand, []string{"--repo", repo, "--onto", "no-such-branch", root}, "no-such-branch"},
		{"repin with an unknown branch", repinCommand, []string{"--repo", repo, "--onto", head, "--branch", "no-such-branch", root}, "no-such-branch"},
		{"repin with an unknown flag", repinCommand, []string{"--repo", repo, "--onto", head, "--squash", root}, "flag provided but not defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := test.run(test.args, &output)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q\n%s", err, test.want, output.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(root, saga.MergesDir)); !os.IsNotExist(err) {
		t.Fatalf("a refused repin wrote a merge record: %v", err)
	}
	if names := diffRecords(t, filepath.Join(root, saga.CodeDirName)); len(names) != 0 {
		t.Fatalf("a refused command wrote evidence: %v", names)
	}
}

func referencesCommand(args []string, out *bytes.Buffer) error {
	return References(context.Background(), args, out)
}

func repinCommand(args []string, out *bytes.Buffer) error {
	return Repin(context.Background(), args, out)
}

// Repinning a Saga whose evidence is already at the landed commit changes
// nothing and records no commits.
func TestRepinOntoTheCurrentPinChangesNothing(t *testing.T) {
	root, repo := coveredSaga(t)
	coverJSON(t, "--repo", repo, "--path", "internal/service/handler.go", "--changed-lines", "--name", "handler", root)
	before := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "handler.json"))
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	result := runRepin(t, "--repo", repo, "--onto", "HEAD", root)
	if result.Onto != head || result.Unchanged != 1 || len(result.Repinned) != 0 || len(result.Commits) != 0 || result.MergeRecord != "" {
		t.Fatalf("repin onto the existing pin = %#v", result)
	}
	if after := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "handler.json")); !sameReferences(after, before) {
		t.Fatalf("a no-op repin rewrote evidence: %#v", after)
	}
	var text bytes.Buffer
	if err := References(context.Background(), []string{"--repo", repo, root}, &text); err != nil || !strings.Contains(text.String(), "1 references: 1 current (0 remapped), 0 stale") {
		t.Fatalf("references text output = %v\n%s", err, text.String())
	}
}
