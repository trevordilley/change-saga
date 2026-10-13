package testfixture

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestERDWhiteboardFixturePreservesSourceAndLocalExplanation(t *testing.T) {
	checkout := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://example.test/acme/app.git"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-q", "--allow-empty", "-m", "Fixture"}} {
		if output, err := exec.Command("git", append([]string{"-C", checkout}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	root := filepath.Join(checkout, "app.saga")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"$schema": "https://changesaga.dev/schema/v5/saga.schema.json", "version": 5, "id": "fixture", "title": "Fixture", "source": map[string]any{"repository": "https://example.test/acme/app.git"}}
	if err := store.WriteJSON(filepath.Join(root, "saga.json"), manifest, true); err != nil {
		t.Fatal(err)
	}
	pins, err := WriteERDWhiteboardFixture(context.Background(), root, checkout)
	if err != nil {
		t.Fatal(err)
	}
	_, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("board storage must remain a valid Saga: %v %+v", err, validation)
	}
	if err := requirements.ValidateWhiteboards(root, "fixture"); err != nil {
		t.Fatal(err)
	}
	board, err := requirements.LoadResolvedWhiteboard(root, "fixture", "erd", "r1")
	if err != nil {
		t.Fatal(err)
	}
	focus, err := board.Composition.Describe("local/local-edit")
	if err != nil {
		t.Fatal(err)
	}
	entity, note := false, false
	for _, instance := range focus.Elements {
		if instance.Path == "local/local-edit" {
			entity = len(instance.Element.Fields) == 2 && instance.Source != nil && instance.Source.Revision == pins["local-edit"].Revision
		}
	}
	for _, instance := range append(focus.Elements, focus.Context...) {
		note = note || instance.Path == "local/local-note"
	}
	if !entity || !note || len(focus.Connections) != 1 {
		t.Fatalf("focus lost fields, local explanation or semantic connection: %+v", focus)
	}
	opaque, err := board.Composition.Describe("cross-store/local-edit")
	if err != nil || len(opaque.Elements) == 0 || opaque.Elements[0].Source == nil || opaque.Elements[0].Source.Revision != pins["local-edit"].Revision {
		t.Fatalf("repeated SVG binding lost source: %+v %v", opaque, err)
	}
	if opaque.Elements[0].Bounds != nil {
		t.Fatal("SVG bindings must not invent geometry")
	}
}
