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
	run("add", "--feature", "core", "--id", "read", "--revision", "r1", "--event", "proposed", "--title", "Understand a change", "--statement", "As a reviewer I can understand a change", "--overview-source", source, "--criterion", "clear=The full explanation remains readable.", "--request-id", "add-read")
	if got := current(); got.Overview != prose || got.Title != "Understand a change" {
		t.Fatalf("authored revision: %+v", got)
	}
	old, _ := json.Marshal(current())
	run("revise", "--story", urn, "--revision", "r2", "--parent", urn+":revision:r1", "--title", "Understand the change")
	if got := current().Overview; got != prose {
		t.Fatalf("title revise lost overview: %q", got)
	}
	if err := Criterion(ctx, []string{"add", root, "--story", urn, "--parent", urn + ":revision:r2", "--revision", "r3", "--id", "safe", "--statement", "Unsafe HTML never executes."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := current().Overview; got != prose {
		t.Fatalf("criterion add lost overview: %q", got)
	}
	t.Setenv("EDITOR", "true")
	run("revise", "--story", urn, "--revision", "r4", "--parent", urn+":revision:r3", "--edit")
	if current().Overview != prose {
		t.Fatal("editor lost overview")
	}
	var output bytes.Buffer
	if err := Query(ctx, []string{"requirements", "--saga", root, "--repo", repo, "--requirement", urn}, &output); err != nil || !strings.Contains(output.String(), `"overview"`) || !strings.Contains(output.String(), "Understand **why**") {
		t.Fatalf("query: %v %s", err, &output)
	}
	run("revise", "--story", urn, "--revision", "r5", "--parent", urn+":revision:r4", "--overview", "", "--edit")
	if current().Overview != "" {
		t.Fatal("explicit clear in editor was ignored")
	}
	if err := story(ctx, []string{"revise", root, "--story", urn, "--revision", "r6", "--parent", urn + ":revision:r5", "--overview-source", "-"}, &bytes.Buffer{}, strings.NewReader(prose)); err != nil {
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
	doc, err := requirements.Load(root, sagaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range doc.Stories {
		if story.Identity.ID == "read" {
			for _, rev := range story.Revisions {
				if rev.ID == "r1" {
					got, _ := json.Marshal(rev)
					if !bytes.Equal(got, old) {
						t.Fatal("old snapshot mutated")
					}
				}
			}
		}
	}
	// Mutually exclusive input sources are rejected before authoring.
	for _, args := range [][]string{{"--overview", "", "--overview-source", source}, {"--from", "-", "--overview-source", "-"}} {
		if err := story(ctx, append([]string{"revise", root}, args...), &bytes.Buffer{}, strings.NewReader("{}")); err == nil {
			t.Fatal("ambiguous prose sources accepted")
		}
	}
}
