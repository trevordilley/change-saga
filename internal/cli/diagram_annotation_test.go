package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
)

// boardDiagram places the shared fixture's store in a section and annotates
// it with a sticky, the worker with a numbered pin, and the write edge with a
// bubble pointing at an explicit point on it.
func boardDiagram() diagram.Document {
	d := testDiagram()
	store := d.Elements[d.Index("store")]
	store.Parent, store.X, store.Y = "backend", 40, 50
	d.Elements[d.Index("store")] = store
	d.Elements = append(d.Elements[:2], append([]diagram.Element{
		{ID: "backend", Kind: "group", Shape: "section", Label: "Backend", Color: "blue", X: 480, Y: 70, Width: 320, Height: 200, Style: "normal"},
	}, d.Elements[2:]...)...)
	d.Elements = append(d.Elements,
		diagram.Element{ID: "why", Kind: "sticky", Label: "One row per job.", About: "store", X: 860, Y: 120, Width: 180, Height: 180, Style: "normal"},
		diagram.Element{ID: "first", Kind: "annotation", Shape: "pin", Label: "1", About: "worker", Color: "purple", X: 324, Y: 104, Width: 32, Style: "normal"},
		diagram.Element{ID: "retries", Kind: "annotation", Shape: "bubble", Label: "Retried twice", About: "write", Target: &diagram.Point{X: 20, Y: -60}, X: 400, Y: 230, Width: 180, Height: 64, Style: "caption"},
	)
	return d
}

func TestBoardDiagramRoundTripsThroughApplySlide(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	invalid := boardDiagram()
	invalid.Elements[invalid.Index("why")].About = "cache"
	invalid.Elements[invalid.Index("first")].Color = "orange"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "board", "create", "absent", invalid), false); err == nil ||
		!strings.Contains(err.Error(), `why: about names unknown element "cache"`) || !strings.Contains(err.Error(), `first: color "orange" is not in the palette`) {
		t.Fatalf("apply-slide must report every board problem: %v", err)
	}
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "board", "create", "absent", boardDiagram()), false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	source, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if stored, err := diagram.Decode(source); err != nil {
		t.Fatal(err)
	} else if bubble, _ := stored.Element("retries"); bubble.Target == nil || bubble.About != "write" {
		t.Fatalf("stored source lost the bubble's pointer: %+v", bubble)
	}
	asset, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	for _, want := range []string{`<filter id="diagram-shadow"`, `id="backend" data-diagram-kind="group"`, `data-annotation="pin"`} {
		if !strings.Contains(string(asset), want) {
			t.Errorf("published SVG lacks %s", want)
		}
	}

	text, err := runDiagram(t, "", "describe", "--slide", "flow", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Sections:\n  backend \"Backend\"\n    members: store\n",
		"  store \"Store\" shape=datastore in=backend\n",
		"Notes:\n  why: sticky about store \"One row per job.\"\n  first: pin about worker \"1\"\n  retries: bubble about write \"Retried twice\"\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q:\n%s", want, text)
		}
	}

	if _, err := runDiagram(t, `[{"op":"remove","id":"store"}]`, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "drop-store", "--from", "OPS", "--repo", repo, root); err == nil || !strings.Contains(err.Error(), "depends on store") {
		t.Fatalf("removing an annotated element must name what depends on it: %v", err)
	}
	edit := `[{"op":"update","id":"why","set":{"color":"green","label":"One row per job, kept 30 days."}},{"op":"move","id":"backend","dx":10}]`
	output, err := runDiagram(t, edit, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "recolor", "--from", "OPS", "--repo", repo, "--json", root)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, output)
	}
	var result DiagramEditResult
	if err := json.Unmarshal([]byte(output), &result); err != nil || strings.Join(result.ChangedElements, ",") != "backend,why" || !result.Diff.DiagramChanged {
		t.Fatalf("edit result = %s err=%v", output, err)
	}
	if got, err := runDiagram(t, "", "get", "--slide", "flow", "--id", "why", root); err != nil || !strings.Contains(got, `"color": "green"`) {
		t.Fatalf("edit did not recolour the sticky: %s %v", got, err)
	}
}
