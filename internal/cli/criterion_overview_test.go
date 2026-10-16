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

func TestCriterionProseAuthoringPreservesSnapshots(t *testing.T) {
	root, repo := coveredSaga(t)
	ctx := context.Background()
	sagaID, err := requirementSagaID(root)
	if err != nil {
		t.Fatal(err)
	}
	urn := "urn:change-saga:" + sagaID + ":story:reading"
	if err := Story(ctx, []string{"add", root, "--feature", "core", "--id", "reading", "--revision", "r1", "--event", "proposed", "--title", "Read a forecast", "--statement", "As a planner I can understand a forecast", "--subtext", "Make forecasts understandable.", "--overview", "Story explanation", "--criterion", "source=The forecast names its source."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		if err := Criterion(ctx, append(args, root), &bytes.Buffer{}); err != nil {
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
			if story.Identity.ID == "reading" {
				return *story.CurrentRevision
			}
		}
		t.Fatal("missing story")
		return requirements.Revision{}
	}
	prose := "## Why\n\nShow **when** the forecast was observed.\n"
	source := filepath.Join(t.TempDir(), "overview.md")
	if err := os.WriteFile(source, []byte(prose), 0600); err != nil {
		t.Fatal(err)
	}
	add := []string{"add", "--story", urn, "--parent", urn + ":revision:r1", "--revision", "r2", "--id", "fresh", "--statement", "The forecast shows its observation time.", "--title", "Know how fresh the forecast is", "--subtext", "Show the age of each observation.", "--overview-source", source, "--request-id", "fresh-add"}
	run(add...)
	run(add...) // Exact retry is idempotent, including authored prose.
	previous := current()
	fresh := previous.AcceptanceCriteria[1]
	if fresh.Subtext != "Show the age of each observation." || previous.Subtext != "Make forecasts understandable." || fresh.Title != "Know how fresh the forecast is" || fresh.Overview != prose || previous.Overview != "Story explanation" {
		t.Fatalf("snapshot: %+v", previous)
	}
	old, _ := json.Marshal(previous)
	run("revise", "--story", urn, "--criterion", urn+":criterion:fresh", "--parent", urn+":revision:r2", "--revision", "r3", "--statement", "The forecast shows its observation date and time.")
	if got := current().AcceptanceCriteria[1]; got.Subtext != fresh.Subtext || got.Title != fresh.Title || got.Overview != prose {
		t.Fatalf("statement edit lost prose: %+v", got)
	}
	// Structured patches preserve omitted fields; explicit empty clears optional prose.
	request := `{"story":"` + urn + `","criterion":"` + urn + `:criterion:fresh","parent":"` + urn + `:revision:r3","revision":"r4","title":"Forecast freshness"}`
	if err := criterion(ctx, []string{"revise", root, "--from", "-"}, &bytes.Buffer{}, strings.NewReader(request)); err != nil {
		t.Fatal(err)
	}
	if got := current().AcceptanceCriteria[1]; got.Title != "Forecast freshness" || got.Overview != prose {
		t.Fatalf("structured patch: %+v", got)
	}
	// The editor accepts the selected criterion's prose and preserves siblings.
	t.Setenv("EDITOR", "true")
	run("revise", "--story", urn, "--criterion", urn+":criterion:fresh", "--parent", urn+":revision:r4", "--revision", "r5", "--overview", "", "--title", "", "--subtext", "", "--edit")
	got := current()
	if got.Subtext != previous.Subtext || got.AcceptanceCriteria[1].Subtext != "" || got.AcceptanceCriteria[1].Title != "" || got.AcceptanceCriteria[1].Overview != "" || got.AcceptanceCriteria[0] != previous.AcceptanceCriteria[0] || got.Overview != previous.Overview {
		t.Fatalf("clear changed other content: %+v", got)
	}
	if err := criterion(ctx, []string{"revise", root, "--story", urn, "--criterion", urn + ":criterion:fresh", "--parent", urn + ":revision:r5", "--revision", "r6", "--overview-source", "-"}, &bytes.Buffer{}, strings.NewReader(prose)); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"requirements", "requirement-history"} {
		var out bytes.Buffer
		if err := Query(ctx, []string{operation, "--saga", root, "--repo", repo, "--requirement", urn}, &out); err != nil || !strings.Contains(out.String(), "Show **when**") {
			t.Fatalf("%s: %v %s", operation, err, &out)
		}
	}
	doc, err := requirements.Load(root, sagaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, story := range doc.Stories {
		if story.Identity.ID == "reading" {
			for _, rev := range story.Revisions {
				if rev.ID == "r2" {
					data, _ := json.Marshal(rev)
					if !bytes.Equal(data, old) {
						t.Fatal("historical revision changed")
					}
				}
			}
		}
	}
	for _, invalid := range [][]string{{"--overview", "text", "--overview-source", source}, {"--from", "-", "--overview-source", "-"}, {"--statement", ""}, {"--statement", " "}} {
		args := append([]string{"revise", root, "--story", urn, "--criterion", urn + ":criterion:fresh", "--parent", urn + ":revision:r6", "--revision", "r7"}, invalid...)
		if err := criterion(ctx, args, &bytes.Buffer{}, strings.NewReader("{}")); err == nil {
			t.Fatalf("accepted invalid inputs %v", invalid)
		}
	}
}
