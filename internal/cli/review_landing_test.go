package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/reviewstate"
)

// reviewListJSON is every review review list reports, merged ones included.
func reviewListJSON(t *testing.T, root string) []reviewstate.Report {
	t.Helper()
	var result reviewListOutput
	if err := json.Unmarshal([]byte(run(t, Review, "list", "--json", root)), &result); err != nil {
		t.Fatal(err)
	}
	return result.Reviews
}

// A review whose change merged without repin --onto is detected from Git:
// review list says merged and sets it aside unless asked, and status no
// longer counts it open. Nothing is written, and nothing is suggested that
// would write: a repin --onto chosen after the fact can re-pin the whole
// Saga to an old commit.
func TestReviewListDetectsAMergedReviewWithoutWritingIt(t *testing.T) {
	t.Parallel()
	fixture := newReviewFixture(t)
	if reports := reviewListJSON(t, fixture.root); len(reports) != 1 || reports[0].State != reviewstate.StateOpen || reports[0].StateSource != reviewstate.StateDetected {
		t.Fatalf("open review = %#v", reports)
	}
	if output := run(t, Review, "list", fixture.root); !strings.Contains(output, "Review pr-7") || strings.Contains(output, "merged") {
		t.Fatalf("open review list:\n%s", output)
	}

	git(t, fixture.repo, "checkout", "main")
	git(t, fixture.repo, "merge", "--no-ff", "-m", "Merge pull request #7", "feature/pg")
	git(t, fixture.repo, "branch", "-D", "feature/pg")

	reports := reviewListJSON(t, fixture.root)
	if len(reports) != 1 || reports[0].State != reviewstate.StateMerged || reports[0].StateSource != reviewstate.StateDetected || reports[0].LandedIn != "main" || reports[0].Merged != nil {
		t.Fatalf("merged review = %#v", reports)
	}
	output := run(t, Review, "list", fixture.root)
	if strings.Contains(output, "Review pr-7") || !strings.Contains(output, "No open reviews.") || !strings.Contains(output, "1 merged review hidden; --all shows them.") {
		t.Fatalf("default review list:\n%s", output)
	}
	for _, args := range [][]string{{"list", "--all", fixture.root}, {"list", "--review", "pr-7", fixture.root}} {
		output := run(t, Review, args...)
		if !strings.Contains(output, "merged: detected, its change is in main") || strings.Contains(output, "repin") {
			t.Fatalf("review %v:\n%s", args, output)
		}
	}

	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	var status bytes.Buffer
	err := Status(context.Background(), []string{"--json", fixture.root}, &status)
	var document struct {
		Reviews []reviewstate.Report `json:"reviews"`
	}
	if jsonErr := json.Unmarshal(status.Bytes(), &document); jsonErr != nil || len(document.Reviews) != 0 {
		t.Fatalf("status counted a merged review open: %v %v\n%s", err, jsonErr, status.String())
	}
	if dirty := git(t, fixture.repo, "status", "--porcelain"); dirty != "" {
		t.Fatalf("detection wrote to the checkout:\n%s", dirty)
	}
}
