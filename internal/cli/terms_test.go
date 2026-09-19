package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
)

const kindsGo = "package assessment\n\ntype Kind string\n\nconst (\n\tKindTesttaker Kind = \"testtaker\"\n\tKindProctor   Kind = \"proctor\"\n)\n"

// newTermSaga initializes a Saga documenting repo, with a story to name.
func newTermSaga(t *testing.T, repo string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "atomic.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	if err := Story(context.Background(), []string{"add", "--epic", testEpic, "--id", "sit-assessment", "--revision", "r1", "--event", "proposed",
		"--title", "Sit an assessment", "--statement", "As a candidate, I sit an assessment", "--priority", "high", root}, &output); err != nil {
		t.Fatalf("story add: %v\n%s", err, output.String())
	}
	return root
}

func TestATermPinsItsCodeAndGoesStaleWhenTheCodeIsRenamed(t *testing.T) {
	repo, commit := sourceRepo(t, map[string]string{"kinds.go": kindsGo})
	root := newTermSaga(t, repo)
	var output bytes.Buffer
	if err := Term(context.Background(), []string{"add", "--id", "testtaker", "--name", "Testtaker", "--definition", "One sitting of an assessment.",
		"--alias", "test taker", "--story", "sit-assessment", "--ref", "HEAD:kinds.go#L6", "--repo", repo, "--json", root}, &output); err != nil {
		t.Fatalf("term add: %v\n%s", err, output.String())
	}
	assertValid(t, root)
	document, err := requirements.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	term := document.FindTerm("testtaker")
	if term == nil || len(term.CurrentRevision.Code) != 1 || term.CurrentRevision.Code[0].Commit != commit ||
		term.CurrentRevision.Stories[0] != "urn:change-saga:atomic:story:sit-assessment" {
		t.Fatalf("term = %#v", term.CurrentRevision)
	}

	health := func() referencesOutput {
		t.Helper()
		var output bytes.Buffer
		if err := References(context.Background(), []string{"--repo", repo, "--allow-repository-mismatch", "--json", root}, &output); err != nil {
			t.Fatalf("references: %v\n%s", err, output.String())
		}
		var result referencesOutput
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := health(); result.Total != 1 || result.Current != 1 || result.References[0].Kind != "term" {
		t.Fatalf("a new term reference is current: %#v", result)
	}
	// A rename changes the defining line, so the reference goes stale and its
	// owner names exactly the term to update.
	writeFile(t, filepath.Join(repo, "kinds.go"), strings.Replace(kindsGo, "KindTesttaker", "KindCandidate", 1))
	git(t, repo, "commit", "-am", "rename testtaker")
	result := health()
	if result.Stale != 1 || result.References[0].Owner != "urn:change-saga:atomic:term:testtaker" {
		t.Fatalf("a renamed constant must stale the term that names it: %#v", result)
	}
	// Revising the term at the renamed line makes it current again.
	output.Reset()
	if err := Term(context.Background(), []string{"revise", "--term", "urn:change-saga:atomic:term:testtaker", "--revision", "r2",
		"--parent", "urn:change-saga:atomic:term:testtaker:revision:r1", "--name", "Testtaker", "--definition", "One sitting of an assessment.",
		"--ref", "HEAD:kinds.go#L6", "--repo", repo, root}, &output); err != nil {
		t.Fatalf("term revise: %v\n%s", err, output.String())
	}
	if result := health(); result.Stale != 0 || result.Current != 1 {
		t.Fatalf("a revised term is current: %#v", result)
	}
	if err := Term(context.Background(), []string{"add", "--id", "bad", "--name", "Bad", "--definition", "x", "--ref", "nope:kinds.go#L1", "--repo", repo, root}, &bytes.Buffer{}); err == nil {
		t.Fatal("an unknown revision must be refused")
	}
}
