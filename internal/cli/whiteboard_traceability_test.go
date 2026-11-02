package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

func TestWhiteboardTraceabilityIncludesCurrentNativeEvidence(t *testing.T) {
	ctx := context.Background()
	repo, head := sourceRepo(t, map[string]string{"app.go": "package app\nfunc Value() int { return 1 }\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "board.saga")
	run(t, Init, "--repo", repo, "--repository", "https://example.test/acme/app.git", root)
	run(t, Feature, "add", "--id", "core", "--title", "Core", root)
	story := "urn:change-saga:board:story:value"
	run(t, Story, "add", "--feature", "core", "--id", "value", "--revision", "r1", "--event", "proposed", "--title", "Read the value", "--statement", "The reader can inspect the value.", "--criterion", "exact=The value is explicit.", root)
	run(t, Story, "set-state", "--story", story, "--event", "accepted", "--parent", story+":event:proposed", "--state", "accepted", "--reason", "Confirmed fixture intent.", root)
	ref := coderef.Reference{Commit: head, Path: "app.go", Start: 2, End: 2}
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{
		{ID: "value", Kind: "node", Shape: "rect", Label: "Value", Style: "normal", Width: 240, Height: 100, Evidence: []coderef.Reference{ref}, CriterionLinks: []diagram.CriterionLink{{ID: "value-exact", Criterion: story + ":criterion:exact", StoryRevision: story + ":revision:r1", Rationale: "This native element explains the value."}}},
		{ID: "unlinked", Kind: "node", Shape: "rect", Label: "Package", Style: "normal", X: 300, Width: 240, Height: 100, Evidence: []coderef.Reference{{Commit: head, Path: "app.go", Start: 1, End: 1}}},
	}}
	input := filepath.Join(t.TempDir(), "board.json")
	publish := func(revision, parent string) {
		t.Helper()
		data, _ := json.Marshal(doc)
		writeFile(t, input, string(data))
		args := []string{"publish", "--id", "architecture", "--revision", revision, "--from", input, "--repo", repo}
		if parent != "" {
			args = append(args, "--parent", parent)
		}
		run(t, Whiteboard, append(args, root)...)
	}
	query := func(extra ...string) livingapp.TraceabilityPage {
		t.Helper()
		var out bytes.Buffer
		args := append([]string{"traceability", "--saga", root, "--repo", repo}, extra...)
		if err := Query(ctx, args, &out); err != nil {
			t.Fatalf("%v: %s", err, out.String())
		}
		var envelope struct{ Data livingapp.TraceabilityPage }
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}
	publish("r1", "")
	page := query("--requirement", story)
	owner := "urn:change-saga:board:whiteboard:architecture:element:value"
	if len(page.Criteria) != 1 || len(page.Criteria[0].CodeEvidence) != 1 || page.Criteria[0].CodeEvidence[0] != ref.Location().String() {
		t.Fatalf("native forward evidence missing: %+v", page.Criteria)
	}
	if len(page.UnlinkedCodeEvidence) != 1 || page.UnlinkedCodeEvidence[0].Item != "urn:change-saga:board:whiteboard:architecture:element:unlinked" {
		t.Fatalf("accepted native link not distinguished from unlinked code: %+v", page.UnlinkedCodeEvidence)
	}
	if len(page.Criteria[0].ReviewTargets) != 1 || page.Criteria[0].ReviewTargets[0] != owner {
		t.Fatalf("native owner replaced by slide identity: %+v", page.Criteria[0])
	}
	if reverse := query("--ref", ref.Location().String()); len(reverse.Criteria) != 1 {
		t.Fatalf("native reverse evidence missing: %+v", reverse)
	}
	doc.Elements[0].Evidence[0].Start, doc.Elements[0].Evidence[0].End = 1, 1
	publish("r2", "r1")
	if old := query("--ref", ref.Location().String()); len(old.Criteria) != 0 {
		t.Fatalf("historical revision inflated current traceability: %+v", old.Criteria)
	}
	if current := query("--requirement", story); len(current.Criteria) != 1 || len(current.Criteria[0].CodeEvidence) != 1 {
		t.Fatalf("current native evidence duplicated: %+v", current.Criteria)
	}
}
