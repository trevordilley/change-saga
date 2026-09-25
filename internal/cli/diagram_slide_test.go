package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

func testDiagram() diagram.Document {
	d := diagram.New()
	d.Elements = []diagram.Element{
		{ID: "title", Kind: "text", Label: "Atomic flow", X: 60, Y: 28, Width: 600, Height: 48, Style: "title", Decorative: true},
		{ID: "worker", Kind: "node", Shape: "service", Label: "Worker", Icon: "lucide:server", X: 80, Y: 120, Width: 260, Height: 100, Style: "primary"},
		{ID: "store", Kind: "node", Shape: "datastore", Label: "Store", X: 520, Y: 120, Width: 240, Height: 110, Style: "normal"},
		{ID: "write", Kind: "edge", From: "worker", To: "store", Points: []diagram.Point{{X: 340, Y: 170}, {X: 520, Y: 170}}, Head: "arrow", Label: "write", LabelBox: &diagram.Box{X: 380, Y: 132, Width: 100, Height: 26}, Style: "secondary"},
	}
	return d
}

// diagramSlideRequest converts the shared fixture request to a diagram-sourced
// slide whose single Item selects the worker node.
func diagramSlideRequest(t *testing.T, repo, base, commit, sagaID, requestID, operation, expected string, d diagram.Document) SlideTransactionRequest {
	t.Helper()
	request := slideTransactionRequest(t, repo, base, commit, sagaID, requestID, operation, expected, "worker")
	request.Asset = SlideTransactionAsset{}
	request.Slide.MediaType = ""
	request.Diagram = &d
	return request
}

func TestApplySlideRendersDiagramSourceAtomically(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	deckDir := filepath.Join(testFeatureDir(root), saga.EmbeddedSlidesDir, "implementation"+saga.EmbeddedDeckSuffix)
	create := diagramSlideRequest(t, repo, base, commit, sagaID, "diagram-create", "create", "absent", testDiagram())

	dry, err := ApplySlideTransaction(context.Background(), root, base, repo, create, true)
	if err != nil || !dry.Diff.DiagramChanged {
		t.Fatalf("dry-run = %#v err=%v", dry, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(deckDir, "24-a-*")); len(matches) != 0 {
		t.Fatalf("dry-run left sidecars %v", matches)
	}
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: err=%v issues=%#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	if slide.Diagram == nil || slide.MediaType != "image/svg+xml" || slide.Diagram.Renderer != diagram.Renderer {
		t.Fatalf("slide diagram pin = %#v media=%s", slide.Diagram, slide.MediaType)
	}
	asset, err := os.ReadFile(filepath.Join(deckDir, slide.Entrypoint))
	if err != nil {
		t.Fatal(err)
	}
	want, err := diagram.Render(testDiagram(), diagram.Options{Title: "Atomic flow", Description: "The complete visual changes together."})
	if err != nil || !bytes.Equal(asset, want) {
		t.Fatalf("published asset is not the deterministic rendering: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(deckDir, slide.Diagram.Source))
	if err != nil {
		t.Fatal(err)
	}
	if stored, err := diagram.Decode(source); err != nil || len(stored.Elements) != 4 {
		t.Fatalf("stored source = %v", err)
	}
	if retry, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false); err != nil || !retry.Replayed || retry.Snapshot != created.Snapshot {
		t.Fatalf("diagram retry = %#v err=%v", retry, err)
	}

	moved, _, err := diagram.Edit(testDiagram(), []diagram.Operation{{Op: "move", ID: "store", DX: 40}})
	if err != nil {
		t.Fatal(err)
	}
	update := diagramSlideRequest(t, repo, base, commit, sagaID, "diagram-move", "update", created.Snapshot, moved)
	updated, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err != nil || !updated.Diff.DiagramChanged || !updated.Diff.AssetChanged || len(updated.Diff.UpdatedItems) != 0 {
		t.Fatalf("diagram update = %#v err=%v", updated, err)
	}
	if _, validation, err := saga.Load(root); err != nil || !validation.Valid {
		t.Fatalf("history with two diagram revisions: err=%v issues=%#v", err, validation.Issues)
	}
}

func TestApplySlideRefusesInvalidDiagramRequests(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	cases := map[string]func(*SlideTransactionRequest){
		"either asset or diagram": func(r *SlideTransactionRequest) { r.Asset = SlideTransactionAsset{ContentBase64: "PHN2Zy8+"} },
		"renders image/svg+xml":   func(r *SlideTransactionRequest) { r.Slide.MediaType = "text/html" },
		"decorative diagram element": func(r *SlideTransactionRequest) {
			r.Items[0].Selector.ElementID = "title"
		},
		"text overflow": func(r *SlideTransactionRequest) {
			r.Diagram.Elements[1].Label = "A worker label that cannot possibly fit its box"
		},
		"semantic element inside decorative group": func(r *SlideTransactionRequest) {
			r.Diagram.Elements = append(r.Diagram.Elements, diagram.Element{ID: "frame", Kind: "group", Style: "normal", Decorative: true})
			r.Diagram.Elements[2].Parent = "frame"
		},
	}
	for want, mutate := range cases {
		request := diagramSlideRequest(t, repo, base, commit, sagaID, "diagram-invalid", "create", "absent", testDiagram())
		mutate(&request)
		if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
	deckDir := filepath.Join(testFeatureDir(root), saga.EmbeddedSlidesDir, "implementation"+saga.EmbeddedDeckSuffix)
	if matches, _ := filepath.Glob(filepath.Join(deckDir, "2[45]-*")); len(matches) != 0 {
		t.Fatalf("refused requests left files %v", matches)
	}
}

func TestLoaderRejectsTamperedDiagramSource(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "diagram-create", "create", "absent", testDiagram()), false); err != nil {
		t.Fatal(err)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	slide := document.Decks[0].Slides[0]
	path := filepath.Join(slide.Directory, slide.Diagram.Source)
	if err := os.WriteFile(path, []byte(`{"version":1,"width":1,"height":1,"elements":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, validation, _ := saga.Load(root); validation.Valid || !strings.Contains(issueText(validation), "source_digest does not match") {
		t.Fatalf("tampered source accepted: %#v", validation.Issues)
	}
}

func issueText(validation saga.Validation) string {
	var parts []string
	for _, issue := range validation.Issues {
		parts = append(parts, issue.Message)
	}
	return strings.Join(parts, "\n")
}

func runDiagram(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	if stdin != "" {
		file := filepath.Join(t.TempDir(), "ops.json")
		writeFile(t, file, stdin)
		for index, arg := range args {
			if arg == "OPS" {
				args[index] = file
			}
		}
	}
	var out bytes.Buffer
	err := Diagram(context.Background(), args, &out)
	return out.String(), err
}

func TestDiagramEditRepublishesTargetedChanges(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "diagram-create", "create", "absent", testDiagram()), false)
	if err != nil {
		t.Fatal(err)
	}
	move := `[{"op":"move","id":"store","dx":40},{"op":"update","id":"write","set":{"points":[{"x":340,"y":170},{"x":560,"y":170}]}}]`
	args := []string{"edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "move-store", "--from", "OPS", "--repo", repo, "--json", root}
	output, err := runDiagram(t, move, append([]string{}, args...)...)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, output)
	}
	var result DiagramEditResult
	if err := json.Unmarshal([]byte(output), &result); err != nil || result.PreviousSnapshot != created.Snapshot || strings.Join(result.ChangedElements, ",") != "store,write" || !result.Diff.DiagramChanged {
		t.Fatalf("edit result = %#v err=%v", result, err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	if slide.AuthoringSnapshot != result.Snapshot || len(slide.Items) != 1 || len(slide.Items[0].Code) != 1 || len(slide.Items[0].CriterionLinks) != 1 {
		t.Fatalf("items, evidence, and criterion links must carry over: %#v", slide.Items)
	}
	retry, err := runDiagram(t, move, append([]string{}, args...)...)
	if err != nil || !strings.Contains(retry, `"replayed": true`) {
		t.Fatalf("retry must replay: %v\n%s", err, retry)
	}
	if _, err := runDiagram(t, `[{"op":"move","id":"store","dx":1}]`, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "stale-move", "--from", "OPS", "--repo", repo, root); err == nil || !strings.Contains(err.Error(), "expected_snapshot mismatch") {
		t.Fatalf("stale edit error = %v", err)
	}
	if _, err := runDiagram(t, `[{"op":"remove","id":"worker","cascade":true}]`, "edit", "--slide", "flow", "--expected", result.Snapshot, "--request-id", "drop-worker", "--from", "OPS", "--repo", repo, root); err == nil || !strings.Contains(err.Error(), "worker") {
		t.Fatalf("removing an Item's element must refuse: %v", err)
	}
	rerender, err := runDiagram(t, `[]`, "edit", "--slide", "flow", "--expected", result.Snapshot, "--request-id", "rerender", "--from", "OPS", "--repo", repo, "--dry-run", "--json", root)
	if err != nil || !strings.Contains(rerender, `"changed_ids": []`) {
		t.Fatalf("an unchanged re-render is a no-op: %v\n%s", err, rerender)
	}
	check, err := runDiagram(t, "", "check", "--json", root)
	if err != nil || !strings.Contains(check, `"current": true`) || !strings.Contains(check, `"ok": true`) {
		t.Fatalf("diagram check: %v\n%s", err, check)
	}
}

func TestDiagramEditRefusesHandAuthoredSlides(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, slideTransactionRequest(t, repo, base, commit, sagaID, "svg-create", "create", "absent", "node-a"), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runDiagram(t, `[]`, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "edit", "--from", "OPS", root); err == nil || !strings.Contains(err.Error(), "has no diagram source") {
		t.Fatalf("hand-authored slide error = %v", err)
	}
}

func TestDiagramIconsLists(t *testing.T) {
	output, err := runDiagram(t, "", "icons", "--query", "data")
	if err != nil || output != "lucide:database\n" {
		t.Fatalf("icons = %q err=%v", output, err)
	}
}
