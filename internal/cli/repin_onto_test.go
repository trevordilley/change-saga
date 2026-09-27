package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// landRepinOntoFixture publishes the transaction fixture's managed slide and a
// plain evidence file, both pinned at the fixture's base commit, then lands a
// later commit that leaves the referenced code unchanged.
func landRepinOntoFixture(t *testing.T) (root, repo, sagaID, landed string) {
	t.Helper()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "flow-create", "create", "absent", "worker")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	coverJSON(t, "--repo", repo, "--commit", commit, "--path", "service.go", "--lines", "3", "--name", "plain", root)
	writeFile(t, filepath.Join(repo, "notes.txt"), "landed\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-q", "-m", "land")
	return root, repo, sagaID, strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
}

// currentFlowEvidenceCommit is the commit the managed slide's evidence is
// pinned at, read through the request apply-slide --print-current prints.
func currentFlowEvidenceCommit(t *testing.T, root string) string {
	t.Helper()
	var printed bytes.Buffer
	if err := ApplySlide(context.Background(), []string{"--print-current", "flow", root}, &printed); err != nil {
		t.Fatal(err)
	}
	var current SlideTransactionRequest
	if err := json.Unmarshal(printed.Bytes(), &current); err != nil {
		t.Fatal(err)
	}
	return current.Items[0].Evidence[0].References[0].Commit
}

// repin --onto moves the evidence of a slide apply-slide manages through one
// complete-slide update, as accept-proposed does, instead of reading the
// slide's evidence as if it were a file of its own.
func TestRepinOntoUpdatesAManagedSlideThroughItsTransaction(t *testing.T) {
	t.Parallel()
	root, repo, _, landed := landRepinOntoFixture(t)

	dry := runRepin(t, "--repo", repo, "--onto", landed, "--dry-run", root)
	if len(dry.Repinned) != 2 || currentFlowEvidenceCommit(t, root) == landed {
		t.Fatalf("dry run = %#v", dry)
	}
	result := runRepin(t, "--repo", repo, "--onto", landed, root)
	if len(result.Repinned) != 2 {
		t.Fatalf("repin --onto = %#v", result)
	}
	if got := currentFlowEvidenceCommit(t, root); got != landed {
		t.Fatalf("managed slide evidence pinned at %s, want %s", got, landed)
	}
	if plain := readCodeFile(t, filepath.Join(root, saga.CodeDirName, "plain.json")); len(plain) != 1 || plain[0].Commit != landed {
		t.Fatalf("plain evidence = %#v", plain)
	}
}

// A repin --onto that cannot complete writes nothing: every edit, plain and
// managed, is validated before the first is written. Here the managed slide's
// criterion link went stale when its story was revised, so its complete-slide
// update is refused, and the plain evidence file must keep its old pin.
func TestRepinOntoThatFailsLeavesTheSagaUntouched(t *testing.T) {
	t.Parallel()
	root, repo, sagaID, landed := landRepinOntoFixture(t)
	story := "urn:change-saga:" + sagaID + ":story:run"
	mustRun(t, Story, "revise", "--feature", testFeature, "--story", story, "--revision", "r2", "--parent", story+":revision:r1",
		"--title", "Run safely", "--statement", "As a user I run the service again", "--criterion", "returns=Run returns without error", root)

	plainPath := filepath.Join(root, saga.CodeDirName, "plain.json")
	before, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	pinned := currentFlowEvidenceCommit(t, root)
	var output bytes.Buffer
	err = Repin(context.Background(), []string{"--repo", repo, "--onto", landed, root}, &output)
	if err == nil || !strings.Contains(err.Error(), "update slide") {
		t.Fatalf("repin --onto with a slide it cannot update = %v\n%s", err, output.String())
	}
	after, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("a failed repin --onto rewrote the plain evidence:\n%s", after)
	}
	if got := currentFlowEvidenceCommit(t, root); got != pinned {
		t.Fatalf("a failed repin --onto moved the slide's evidence to %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, saga.MergesDir)); !os.IsNotExist(err) {
		t.Fatalf("a failed repin --onto recorded a merge: %v", err)
	}
}
