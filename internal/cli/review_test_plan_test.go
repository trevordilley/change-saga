package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestReviewTestPlanCommandUsesReviewRangeAndNeverExecutes(t *testing.T) {
	f := newReviewFixture(t)
	git(t, f.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	ctx := context.Background()
	story := "urn:change-saga:app:story:queue"
	test := "urn:change-saga:app:test-case:queue"
	run(t, Story, "add", "--feature", testFeature, "--id", "queue", "--revision", "r1", "--event", "proposed", "--persona", personaURNFor("app"),
		"--title", "Durable jobs", "--statement", "As a user I can submit durable jobs", "--priority", "must", "--criterion", "durable=Jobs survive restart", f.root)
	for _, entry := range []struct{ slide, path string }{{"queue", "queue.go"}, {"table", "store.go"}} {
		item := saga.ReviewItemTarget("app", "pr-7", entry.slide, "node")
		run(t, ReviseItem, "--review", "pr-7", "--slide", entry.slide, "--item", item, "--record", story, f.root)
		run(t, Cover, "--target", item, "--path", entry.path, "--changed-lines", "--repo", f.repo, f.root)
	}
	writeFile(t, filepath.Join(f.repo, "queue_test.go"), "package queue\n// automated durability test\n")
	git(t, f.repo, "add", "queue_test.go")
	git(t, f.repo, "commit", "-m", "Add test")
	head := strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))
	runQuality(t, "", "test-case", "add", f.root, "--feature", testFeature, "--id", "queue", "--title", "Queue durability", "--kind", "positive", "--automation", "automated",
		"--step", `{"id":"run","action":"Restart worker","expected_result":"Job remains"}`, "--expected-result", "Job remains")
	run(t, Relation, "add", f.root, "--feature", testFeature, "--id", "queue-verifies", "--type", "verifies", "--from", test, "--to", story+":criterion:durable", "--rationale", "Exercises durability")
	evidence := runQuality(t, "", "evidence", "add", f.root, "--test", test, "--role", "test_implementation", "--repo", f.repo, "--code", head+":queue_test.go#L1-L2")
	// This is deliberately not a runnable command. Planning must only return it.
	runQuality(t, "", "run", "record", f.root, "--test", test, "--id", "previous", "--commit", head, "--result", "skipped", "--summary", "Recorded invocation", "--evidence", evidence.Resource, "--command", "do-not-execute-this-test-command")
	// Move the checkout away from the review branch; selection must still use it.
	git(t, f.repo, "checkout", "-b", "unrelated-checkout")
	writeFile(t, filepath.Join(f.repo, "unrelated.go"), "package unrelated\n")
	git(t, f.repo, "add", "unrelated.go")
	git(t, f.repo, "commit", "-m", "Unrelated change")
	before := git(t, f.repo, "status", "--porcelain")
	var out bytes.Buffer
	if err := Review(ctx, []string{"test-plan", "--review", "pr-7", "--repo", f.repo, "--json", f.root}, &out); err != nil {
		t.Fatalf("plan: %v\n%s", err, out.String())
	}
	var plan livingapp.ReviewTestPlan
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.Complete || plan.Range.HeadOID != head || len(plan.Tests) != 1 || len(plan.Commands) != 1 || plan.Commands[0] != "do-not-execute-this-test-command" {
		t.Fatalf("plan: %+v", plan)
	}
	if after := git(t, f.repo, "status", "--porcelain"); after != before {
		t.Fatalf("planning changed files: before %s after %s", before, after)
	}
	out.Reset()
	if err := Review(ctx, []string{"test-plan", "--review", "pr-7", "--repo", f.repo, f.root}, &out); err != nil || !strings.Contains(out.String(), "Queue durability") {
		t.Fatalf("text: %v %s", err, out.String())
	}
}

func TestReviewTestPlanCommandGapsAndErrorsAreSingleJSONDocuments(t *testing.T) {
	f, _ := newEmptyReviewFixture(t)
	for _, tt := range []struct {
		id    string
		code  int
		extra []string
	}{{"pr-7", 3, nil}, {"missing", 1, nil}, {"pr-7", 1, []string{"--unknown"}}} {
		var out bytes.Buffer
		args := append([]string{"test-plan", "--review", tt.id, "--json", f.root}, tt.extra...)
		err := Review(context.Background(), args, &out)
		var status *StatusError
		if !errors.As(err, &status) || status.Code != tt.code {
			t.Fatalf("status: %v %s", err, out.String())
		}
		decoder := json.NewDecoder(&out)
		var value map[string]any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if err := decoder.Decode(&value); err != io.EOF {
			t.Fatalf("more than one response: %v", err)
		}
		if tt.code == 3 && value["complete"] != false {
			t.Fatal(value)
		}
	}
}
