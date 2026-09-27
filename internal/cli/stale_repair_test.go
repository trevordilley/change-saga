package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

func runAccept(t *testing.T, args ...string) acceptOutput {
	t.Helper()
	var output bytes.Buffer
	if err := Repin(context.Background(), append([]string{"--json", "--accept-proposed"}, args...), &output); err != nil {
		t.Fatalf("repin --accept-proposed %v: %v\n%s", args, err, output.String())
	}
	var result acceptOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, output.String())
	}
	return result
}

// An edit inside a referenced range leaves the reference stale; the proposal
// maps it through the diff, and one explicit accept moves it there keeping
// its note and record. A reference with nothing to propose is refused.
func TestAcceptProposedMovesAStaleReferenceAndRefusesWithoutAProposal(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "app.go"), appFeature)
	writeFile(t, filepath.Join(repo, "util.go"), utilFeature)
	pin := commitAll(t, repo, "Base")
	root := filepath.Join(repo, "change.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--title", "App", root}, &output); err != nil {
		t.Fatal(err)
	}
	coverJSON(t, "--commit", pin, "--path", "app.go", "--lines", "5-7", "--name", "app", "--note", "B returns a value", root)
	coverJSON(t, "--commit", pin, "--path", "util.go", "--lines", "3", "--name", "util", root)
	commitAll(t, repo, "Record evidence")

	writeFile(t, filepath.Join(repo, "app.go"), "package app\n\nfunc A() {}\n\nfunc B() int {\n\t// one\n\treturn 1\n}\n")
	git(t, repo, "rm", "-q", "util.go")
	head := commitAll(t, repo, "Comment B and drop U")

	stale := runReferences(t, "--stale", root)
	if stale.Stale != 2 {
		t.Fatalf("want both references stale: %#v", stale)
	}
	app := findHealth(t, stale, "evidence", "app.go")
	if app.Proposal == nil || !app.Proposal.Proposed() || app.Proposal.Location.String() != head+":app.go#L5-L8" {
		t.Fatalf("app.go proposal = %#v", app.Proposal)
	}
	if util := findHealth(t, stale, "evidence", "util.go"); util.Proposal == nil || util.Proposal.Proposed() || !strings.Contains(util.Proposal.Reason, "deleted") {
		t.Fatalf("util.go proposal = %#v", util.Proposal)
	}

	var refused bytes.Buffer
	if err := Repin(context.Background(), []string{"--accept-proposed", "--record", "___code/util.json", "--reference", "1", root}, &refused); err == nil || !strings.Contains(err.Error(), "no proposal") {
		t.Fatalf("accepting a reference with no proposal = %v", err)
	}
	dry := runAccept(t, "--dry-run", "--record", "___code/app.json", root)
	if len(dry.Accepted) != 1 || readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json"))[0].Commit != pin {
		t.Fatalf("dry run = %#v", dry)
	}
	result := runAccept(t, "--all", root)
	if len(result.Accepted) != 1 || len(result.Refused) != 1 || result.Accepted[0].To.String() != head+":app.go#L5-L8" {
		t.Fatalf("accept --all = %#v", result)
	}
	got := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "app.json"))
	want := coderef.Reference{Commit: head, Path: "app.go", Start: 5, End: 8, Digest: sha256Digest("func B() int {\n\t// one\n\treturn 1\n}\n"), Note: "B returns a value"}
	if !sameReferences(got, []coderef.Reference{want}) {
		t.Fatalf("accepted reference = %#v, want %#v", got, want)
	}
	if after := runReferences(t, "--stale", root); after.Stale != 1 {
		t.Fatalf("only the deleted file's reference should stay stale: %#v", after)
	}
}

// A slide apply-slide manages takes an accepted proposal through one
// complete-slide update that changes only the evidence, and --print-current
// prints the request that republishes it.
func TestAcceptProposedUpdatesAManagedSlideThroughItsTransaction(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "flow-create", "create", "absent", "worker")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "service.go"), "package service\n\n// Run runs.\nfunc Run() error {\n\treturn nil\n}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "expand Run")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	slide := findSlide(document, "flow")
	record := slide.Items[0].Code[0].Path
	if !strings.Contains(record, "#items/worker/evidence/0") {
		t.Fatalf("managed evidence file = %q", record)
	}
	result := runAccept(t, "--repo", repo, "--record", record, root)
	if len(result.Accepted) != 1 || len(result.Slides) != 1 || result.Accepted[0].To.String() != head+":service.go#L3-L6" {
		t.Fatalf("accept = %#v", result)
	}
	if diff := result.Slides[0].Diff; diff.AssetChanged || len(diff.UpdatedItems) != 1 || diff.UpdatedItems[0] != "worker" {
		t.Fatalf("slide diff = %#v", diff)
	}
	var printed bytes.Buffer
	if err := ApplySlide(context.Background(), []string{"--print-current", "flow", root}, &printed); err != nil {
		t.Fatal(err)
	}
	var current SlideTransactionRequest
	if err := json.Unmarshal(printed.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	reference := current.Items[0].Evidence[0].References[0]
	if current.ExpectedSnapshot != result.Slides[0].Snapshot || reference.Location().String() != head+":service.go#L3-L6" || reference.Note != "Run is the exact implementation entrypoint." || current.RequestID == "" {
		t.Fatalf("printed request = %#v", current)
	}
	again, err := ApplySlideTransaction(context.Background(), root, base, repo, current, true)
	if err != nil || len(again.ChangedIDs) != 0 {
		t.Fatalf("the printed request must republish the slide unchanged: %#v %v", again, err)
	}
}

// review create's next steps and review list say what the review's change
// made stale in the living documentation, with proposals.
func TestReviewCreateAndListSayWhatTheChangeMadeStale(t *testing.T) {
	t.Parallel()
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(3, func() error { return sqs.Send(job) })\n}\n")
	git(t, repo, "commit", "-am", "Retry enqueue")
	var created bytes.Buffer
	if err := Review(context.Background(), []string{"create", "--id", "retry", "--base", "main", "--head", "retry", root}, &created); err != nil {
		t.Fatalf("%v\n%s", err, created.String())
	}
	if !strings.Contains(created.String(), "your change made 1 living documentation reference stale (1 with a proposed range)") {
		t.Fatalf("review create next steps:\n%s", created.String())
	}
	var listed bytes.Buffer
	if err := Review(context.Background(), []string{"list", "--json", root}, &listed); err != nil {
		t.Fatal(err)
	}
	var output reviewListOutput
	if err := json.Unmarshal(listed.Bytes(), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Repair) != 1 || output.Repair[0].StaleByChange == nil || output.Repair[0].StaleByChange.Count != 1 || output.Repair[0].StaleByChange.References[0].Accept == nil {
		t.Fatalf("review list repair = %s", listed.String())
	}
	listed.Reset()
	if err := Review(context.Background(), []string{"list", root}, &listed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed.String(), "living documentation: your change made 1 reference stale (1 with a proposed range") {
		t.Fatalf("review list text:\n%s", listed.String())
	}
}

// Deleted-side evidence (pinned at the merge-base to lines the change
// removed) is current at the base by design; it is counted apart, never as a
// reference the change made stale.
func TestDeletedSideEvidenceIsNotMadeStaleByTheChange(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "app.go"), appFeature)
	commitAll(t, repo, "Base")
	root := filepath.Join(repo, "change.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--title", "App", root}, &output); err != nil {
		t.Fatal(err)
	}
	commitAll(t, repo, "Saga")
	git(t, repo, "checkout", "-q", "-b", "two")
	writeFile(t, filepath.Join(repo, "app.go"), appChanged)
	commitAll(t, repo, "Return two")
	coverJSON(t, "--against", "main", "--path", "app.go", "--changed-lines", "--name", "app", root)
	_, report := statusComplete(t, root)
	var staleness changeStaleness
	if err := json.Unmarshal(report["stale_by_change"], &staleness); err != nil {
		t.Fatalf("stale_by_change: %v %s", err, report["stale_by_change"])
	}
	if staleness.Count != 0 || staleness.DeletedSide != 1 {
		t.Fatalf("stale_by_change = %+v", staleness)
	}
}
