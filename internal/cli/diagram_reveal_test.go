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

func TestDiagramRevealRoundTripsThroughApplySlideAndEdit(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	d := testDiagram()
	d.Elements[3].Step = 1
	invalid := diagramSlideRequest(t, repo, base, commit, sagaID, "reveal-create", "create", "absent", d)
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, invalid, false); err == nil || !strings.Contains(err.Error(), "write: step orders the reveal, but the diagram does not set one") {
		t.Fatalf("apply-slide must refuse a step without a reveal: %v", err)
	}
	d.Reveal = "fade"
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, diagramSlideRequest(t, repo, base, commit, sagaID, "reveal-create", "create", "absent", d), false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	source, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if stored, err := diagram.Decode(source); err != nil || stored.Reveal != "fade" || stored.Elements[3].Step != 1 {
		t.Fatalf("stored source lost the reveal: %v", err)
	}
	asset, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	for _, want := range []string{`<g id="diagram-reveal"/>`, `id="worker" data-diagram-kind="node" transform="translate(80 120)" data-reveal-step="1"`, `id="store" data-diagram-kind="node" transform="translate(520 120)" data-reveal-step="2"`} {
		if !strings.Contains(string(asset), want) {
			t.Errorf("SVG lacks %s", want)
		}
	}

	text, err := runDiagram(t, "", "describe", "--slide", "flow", root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\nReveal: fade in 2 steps;", "  store \"Store\" shape=datastore step=2\n", "  write: worker -> store \"write\" step=1\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q:\n%s", want, text)
		}
	}

	// The edit moves the write edge to enter with the store, then clears the
	// reveal together with its step.
	output, err := runDiagram(t, `[{"op":"update","id":"write","set":{"step":2}}]`, "edit", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "restep", "--from", "OPS", "--repo", repo, "--json", root)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, output)
	}
	var result DiagramEditResult
	if err := json.Unmarshal([]byte(output), &result); err != nil || strings.Join(result.ChangedElements, ",") != "write" {
		t.Fatalf("edit result = %s err=%v", output, err)
	}
	if text, _ := runDiagram(t, "", "describe", "--slide", "flow", root); !strings.Contains(text, "  write: worker -> store \"write\" step=2\n") || !strings.Contains(text, "Reveal: fade in 2 steps;") {
		t.Fatalf("describe after the edit:\n%s", text)
	}
	if _, err := runDiagram(t, `[{"op":"canvas","set":{"reveal":""}}]`, "edit", "--slide", "flow", "--expected", result.Snapshot, "--request-id", "half-off", "--from", "OPS", "--repo", repo, root); err == nil || !strings.Contains(err.Error(), "write: step orders the reveal") {
		t.Fatalf("clearing only the reveal must name the leftover step: %v", err)
	}
	output, err = runDiagram(t, `[{"op":"canvas","set":{"reveal":""}},{"op":"update","id":"write","set":{"step":null}}]`, "edit", "--slide", "flow", "--expected", result.Snapshot, "--request-id", "off", "--from", "OPS", "--repo", repo, "--json", root)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, output)
	}
	document, _, _ = saga.Load(root)
	slide = document.Decks[0].Slides[0]
	asset, _ = os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	static, err := diagram.Render(testDiagram(), diagram.Options{Title: slide.Title, Description: strings.TrimSpace(slide.Takeaway)})
	if err != nil || string(asset) != string(static) {
		t.Fatalf("a cleared reveal must render the static drawing byte for byte: %v", err)
	}
}
