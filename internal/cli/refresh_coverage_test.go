package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func runRefresh(t *testing.T, args ...string) refreshOutput {
	t.Helper()
	var output bytes.Buffer
	if err := Review(context.Background(), append([]string{"refresh-coverage", "--json"}, args...), &output); err != nil {
		t.Fatalf("refresh-coverage %v: %v\n%s", args, err, output.String())
	}
	var result refreshOutput
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, output.String())
	}
	return result
}

// A push moves one Item's lines, edits inside another's, adds lines to a
// file one Item covers, and adds a file none covers. Refresh re-pins the
// move and leaves the edit for judgment with its proposal. New lines are
// never given to an Item on their own: they are reported uncovered with the
// one Item covering their file as the proposed owner, and only an explicit
// --accept-proposed gives them to it, under a neutral note.
func TestRefreshCoverageRepinsMovesAddsNewLinesAndLeavesEditsForJudgment(t *testing.T) {
	t.Parallel()
	fixture := newReviewFixture(t)
	repo, root := fixture.repo, fixture.root
	writeFile(t, filepath.Join(repo, "queue.go"), "package queue\n\n// Enqueue names the queue.\nfunc Enqueue() string { return \"postgres\" }\n\nfunc Dequeue() {}\n")
	writeFile(t, filepath.Join(repo, "store.go"), "package store\n\nfunc Table() string { return \"jobs_v2\" }\n")
	writeFile(t, filepath.Join(repo, "worker.go"), "package worker\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "Iterate")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	queue := saga.ReviewItemTarget("app", "pr-7", "queue", "node")
	table := saga.ReviewItemTarget("app", "pr-7", "table", "node")

	dry := runRefresh(t, "--review", "pr-7", "--dry-run", root)
	if len(dry.Moved) != 1 || dry.Moved[0].Item != queue || dry.Moved[0].To.String() != head+":queue.go#L4" {
		t.Fatalf("moved = %#v", dry.Moved)
	}
	if len(dry.NeedsJudgment) != 1 || dry.NeedsJudgment[0].Owner != table || dry.NeedsJudgment[0].Proposal.Location == nil || dry.NeedsJudgment[0].Proposal.Location.String() != head+":store.go#L3" || dry.NeedsJudgment[0].Accept == nil {
		t.Fatalf("needs judgment = %#v", dry.NeedsJudgment)
	}
	if len(dry.Added) != 0 {
		t.Fatalf("new lines must not be given to an Item without --accept-proposed: %#v", dry.Added)
	}
	gaps := map[string]refreshGap{}
	for _, gap := range dry.Uncovered {
		gaps[gap.Path] = gap
	}
	if gap := gaps["queue.go"]; gap.ProposedOwner != queue || gap.Accept == nil || !strings.Contains(strings.Join(gap.Accept.Argv, " "), "review refresh-coverage --review pr-7 --accept-proposed --path queue.go") || !strings.HasSuffix(strings.Join(gap.Locations, " "), head+":queue.go#L3 "+head+":queue.go#L5-L6") {
		t.Fatalf("queue.go gap = %#v", gap)
	}
	if gap := gaps["store.go"]; gap.ProposedOwner != table || gap.Accept == nil {
		t.Fatalf("store.go gap = %#v", gap)
	}
	if gap := gaps["worker.go"]; len(gaps) != 3 || gap.ProposedOwner != "" || gap.Accept != nil || len(gap.Candidates) != 0 {
		t.Fatalf("uncovered = %#v", dry.Uncovered)
	}
	if before := reviewReport(t, fixture).Coverage; before.Summary.Stale != 1 {
		t.Fatalf("a dry run must not write: %#v", before.Summary)
	}

	runRefresh(t, "--review", "pr-7", root)
	covered := reviewReport(t, fixture).Coverage
	if covered.Summary.Stale != 1 || len(covered.UncoveredFiles) != 3 {
		t.Fatalf("after refresh the edit waits for judgment and every new line is uncovered: %#v %#v", covered.Summary, covered.UncoveredFiles)
	}
	accepted := runRefresh(t, "--review", "pr-7", "--accept-proposed", "--path", "queue.go", root)
	added := map[string]string{}
	for _, addition := range accepted.Added {
		if addition.Note != "added in "+shortOID(head) {
			t.Fatalf("an accepted line must carry a neutral note, not another reference's: %#v", addition)
		}
		added[addition.Location.Path+":"+strings.SplitN(addition.Location.String(), "#", 2)[1]] = addition.Item
	}
	if len(accepted.Added) != 3 || added["queue.go:L3"] != queue || added["queue.go:L5-L6"] != queue {
		t.Fatalf("--accept-proposed --path queue.go added = %#v", accepted.Added)
	}
	accepted = runRefresh(t, "--review", "pr-7", "--accept-proposed", "--note", "Table names the jobs table", root)
	if len(accepted.Added) != 1 || accepted.Added[0].Item != table || accepted.Added[0].Location.Path != "store.go" || accepted.Added[0].Note != "Table names the jobs table" {
		t.Fatalf("--accept-proposed added = %#v", accepted.Added)
	}
	runAccept(t, "--review", "pr-7", root)
	covered = reviewReport(t, fixture).Coverage
	if covered.Summary.Stale != 0 || len(covered.UncoveredFiles) != 1 || covered.UncoveredFiles[0].Path != "worker.go" {
		t.Fatalf("after accepting, only the new file is left: %#v %#v", covered.Summary, covered.UncoveredFiles)
	}

	// An added file's whole-file reference survives an edit that keeps it
	// added: refresh carries the add event to the new head.
	run(t, Cover, "--target", queue, "--path", "worker.go", "--changed-lines", "--repo", repo, "--allow-repository-mismatch", root)
	writeFile(t, filepath.Join(repo, "worker.go"), "package worker\n\nfunc Work() {}\n")
	git(t, repo, "commit", "-qam", "Add Work")
	head = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	refreshed := runRefresh(t, "--review", "pr-7", root)
	carried := false
	for _, move := range refreshed.Moved {
		carried = carried || move.To.String() == head+":worker.go" && strings.Contains(move.Why, "file event")
	}
	if !carried || len(refreshed.NeedsJudgment) != 0 || len(refreshed.Added) != 0 || len(refreshed.Uncovered) != 1 || refreshed.Uncovered[0].ProposedOwner != queue || refreshed.Uncovered[0].Locations[0] != head+":worker.go#L2-L3" {
		t.Fatalf("whole-file add reference was not carried: %#v", refreshed)
	}
	runRefresh(t, "--review", "pr-7", "--accept-proposed", root)
	if covered = reviewReport(t, fixture).Coverage; covered.Summary.Uncovered != 0 {
		t.Fatalf("after accepting every change of worker.go is covered: %#v %#v", covered.Summary, covered.UncoveredFiles)
	}
	assertValid(t, root)
}

// A review Item pins an unchanged Helper on the new side, below a function
// the pull request added; a later push edits Helper. Its lines still exist at
// the merge-base, but the reference documents the head: refresh, repin
// --accept-proposed, and review list all treat it as stale at the head with a
// proposal there, and none re-pins it to the merge-base.
func TestNewSideEvidenceStaleAtTheHeadKeepsItsSide(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Dev")
	git(t, repo, "config", "user.email", "dev@example.test")
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n\nfunc Helper() int {\n\treturn 1\n}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "base")
	root := filepath.Join(repo, "app.saga")
	run(t, Init, "--repo", repo, "--repository", "https://example.test/acme/app.git", root)
	addTestApp(t, root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "saga")
	git(t, repo, "checkout", "-q", "-b", "feature/new")
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n\nfunc New() {}\n\nfunc Helper() int {\n\treturn 1\n}\n")
	git(t, repo, "commit", "-qam", "Add New above Helper")
	pinned := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	run(t, Review, "create", "--id", "pr-9", "--base", "main", "--head", "feature/new", "--pr", "9", "--url", "https://github.com/acme/app/pull/9", "--title", "Add New", root)
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, reviewSlideSVG)
	run(t, AddSlide, "--review", "pr-9", "--intent", "explain", "--layout", "diagram", "--source", visual, root, "helper")
	run(t, AddItem, "--review", "pr-9", "--slide", "helper", "--kind", "node", "--element-id", "node", "--description", "Helper", root)
	item := saga.ReviewItemTarget("app", "pr-9", "helper", "node")
	run(t, Cover, "--target", item, "--ref", pinned+":a.go#L5-L7", "--repo", repo, root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "Review deck")
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n\nfunc New() {}\n\nfunc Helper() int {\n\treturn 2\n}\n")
	git(t, repo, "commit", "-qam", "Helper returns two")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	refreshed := runRefresh(t, "--review", "pr-9", "--dry-run", root)
	if len(refreshed.Moved) != 0 {
		t.Fatalf("new-side evidence stale at the head must not be re-pinned to the merge-base: %#v", refreshed.Moved)
	}
	if len(refreshed.NeedsJudgment) != 1 || refreshed.NeedsJudgment[0].Proposal.Location == nil || refreshed.NeedsJudgment[0].Proposal.Location.String() != head+":a.go#L5-L7" {
		t.Fatalf("needs judgment = %#v", refreshed.NeedsJudgment)
	}

	var listed bytes.Buffer
	if err := Review(context.Background(), []string{"list", "--review", "pr-9", "--json", root}, &listed); err != nil {
		t.Fatal(err)
	}
	var list reviewListOutput
	if err := json.Unmarshal(listed.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Repair) != 1 || len(list.Repair[0].Items) != 1 || list.Repair[0].Items[0].Accept == nil {
		t.Fatalf("review list must list the stale Item reference: %s", listed.String())
	}

	// Another open review whose branch is gone cannot be read; --all skips
	// and reports it rather than failing everyone's repair.
	git(t, repo, "branch", "feature/gone")
	run(t, Review, "create", "--id", "pr-10", "--base", "main", "--head", "feature/gone", "--pr", "10", "--url", "https://github.com/acme/app/pull/10", "--title", "Gone", root)
	run(t, AddSlide, "--review", "pr-10", "--intent", "explain", "--layout", "diagram", "--source", visual, root, "gone")
	run(t, AddItem, "--review", "pr-10", "--slide", "gone", "--kind", "node", "--element-id", "node", "--description", "Gone", root)
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-10", "gone", "node"), "--ref", pinned+":a.go#L3", "--repo", repo, root)
	git(t, repo, "branch", "-D", "feature/gone")
	everything := runAccept(t, "--all", "--dry-run", root)
	if len(everything.Skipped) != 1 || everything.Skipped[0].Review != "pr-10" || len(everything.Accepted) != 1 {
		t.Fatalf("--all must skip and report an unreadable review: %#v", everything)
	}

	accepted := runAccept(t, "--review", "pr-9", root)
	if accepted.Current != 0 || len(accepted.Accepted) != 1 || accepted.Accepted[0].To.String() != head+":a.go#L5-L7" {
		t.Fatalf("a head-stale new-side reference is not already current: %#v", accepted)
	}
	assertValid(t, root)
}
