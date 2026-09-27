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
// move, gives the new lines to the Item covering their file, leaves the edit
// for judgment with its proposal, and reports the new file uncovered.
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
	added := map[string]string{}
	for _, addition := range dry.Added {
		added[addition.Location.Path+":"+strings.SplitN(addition.Location.String(), "#", 2)[1]] = addition.Item
	}
	if added["queue.go:L3"] != queue || added["queue.go:L5-L6"] != queue || added["store.go:L3"] != table {
		t.Fatalf("added = %#v", dry.Added)
	}
	if len(dry.Uncovered) != 1 || dry.Uncovered[0].Path != "worker.go" || len(dry.Uncovered[0].Candidates) != 0 {
		t.Fatalf("uncovered = %#v", dry.Uncovered)
	}
	if before := reviewReport(t, fixture).Coverage; before.Summary.Stale != 1 {
		t.Fatalf("a dry run must not write: %#v", before.Summary)
	}

	runRefresh(t, "--review", "pr-7", root)
	covered := reviewReport(t, fixture).Coverage
	if covered.Summary.Stale != 1 || len(covered.UncoveredFiles) != 2 {
		t.Fatalf("after refresh the edit waits for judgment and worker.go is uncovered: %#v %#v", covered.Summary, covered.UncoveredFiles)
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
	if !carried || len(refreshed.NeedsJudgment) != 0 || len(refreshed.Added) != 1 || refreshed.Added[0].Location.String() != head+":worker.go#L2-L3" {
		t.Fatalf("whole-file add reference was not carried: %#v", refreshed)
	}
	if covered = reviewReport(t, fixture).Coverage; covered.Summary.Uncovered != 0 {
		t.Fatalf("after refresh every change of worker.go is covered: %#v %#v", covered.Summary, covered.UncoveredFiles)
	}
	assertValid(t, root)
}
