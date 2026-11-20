package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestStoryOverviewSnapshotAuthoringAndQuery(t *testing.T) {
	root, repo := coveredSaga(t)
	ctx := context.Background()
	urn := "urn:change-saga:test:story:read"
	// Use the fixture's actual saga identity rather than assuming its title.
	sagaID, err := requirementSagaID(root)
	if err != nil {
		t.Fatal(err)
	}
	urn = "urn:change-saga:" + sagaID + ":story:read"
	prose := "## Purpose\n\nUnderstand **why** before implementing.\n\n## Boundaries\n\n- Preserve history.\n"
	source := filepath.Join(t.TempDir(), "overview.md")
	if err := os.WriteFile(source, []byte(prose), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		if err := Story(ctx, append(args, root), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	current := func() requirements.Revision {
		t.Helper()
		doc, err := requirements.Load(root, sagaID)
		if err != nil {
			t.Fatal(err)
		}
		for _, story := range doc.Stories {
			if story.Identity.ID == "read" {
				return *story.CurrentRevision
			}
		}
		t.Fatal("story absent")
		return requirements.Revision{}
	}
	run("add", "--feature", "core", "--id", "read", "--title", "Understand a change", "--statement", "As a reviewer I can understand a change", "--overview-source", source, "--criterion", "clear=The full explanation remains readable.", "--request-id", "add-read")
	if got := current(); got.Overview != prose || got.Title != "Understand a change" {
		t.Fatalf("authored revision: %+v", got)
	}
	currentPath := filepath.ToSlash(filepath.Join("___features", "core.feature", "___requirements", "stories", "read.story", "current.json"))
	old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(currentPath)))
	if err != nil {
		t.Fatal(err)
	}
	committed := commitCurrentTestDocumentation(t, root, "Document original authored prose")
	run("revise", "--story", urn, "--title", "Understand the change")
	if got := current().Overview; got != prose {
		t.Fatalf("title revise lost overview: %q", got)
	}
	if err := Criterion(ctx, []string{"add", root, "--story", urn, "--id", "safe", "--statement", "Unsafe HTML never executes."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := current().Overview; got != prose {
		t.Fatalf("criterion add lost overview: %q", got)
	}
	t.Setenv("EDITOR", "true")
	run("revise", "--story", urn, "--edit")
	if current().Overview != prose {
		t.Fatal("editor lost overview")
	}
	var output bytes.Buffer
	if err := Query(ctx, []string{"requirements", "--saga", root, "--repo", repo, "--requirement", urn}, &output); err != nil || !strings.Contains(output.String(), `"overview"`) || !strings.Contains(output.String(), "Understand **why**") {
		t.Fatalf("query: %v %s", err, &output)
	}
	run("revise", "--story", urn, "--overview", "", "--edit")
	if current().Overview != "" {
		t.Fatal("explicit clear in editor was ignored")
	}
	if err := story(ctx, []string{"revise", root, "--story", urn, "--overview-source", "-"}, &bytes.Buffer{}, strings.NewReader(prose)); err != nil {
		t.Fatal(err)
	}
	if current().Overview != prose {
		t.Fatal("stdin prose changed")
	}
	// Structured revisions are complete: omission deliberately means no overview.
	complete := current()
	complete.ID = "r7"
	complete.Parents = []string{urn + ":revision:r6"}
	complete.Overview = ""
	complete.RequestID = ""
	body, _ := json.Marshal(complete)
	if err := story(ctx, []string{"revise", root, "--from", "-"}, &bytes.Buffer{}, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	if current().Overview != "" {
		t.Fatal("complete snapshot unexpectedly inherited")
	}
	if saved := committedCurrentTestFile(t, root, committed, currentPath); strings.TrimSpace(saved) != strings.TrimSpace(string(old)) {
		t.Fatal("committed authored prose changed")
	}

	// Mutually exclusive input sources are rejected before authoring.
	for _, args := range [][]string{{"--overview", "", "--overview-source", source}, {"--from", "-", "--overview-source", "-"}} {
		if err := story(ctx, append([]string{"revise", root}, args...), &bytes.Buffer{}, strings.NewReader("{}")); err == nil {
			t.Fatal("ambiguous prose sources accepted")
		}
	}
}
