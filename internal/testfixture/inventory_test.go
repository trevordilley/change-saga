package testfixture

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestWriteInventoryFixture(t *testing.T) {
	ctx := context.Background()
	checkout := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://example.test/acme/app.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", checkout}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	root := filepath.Join(checkout, "app.saga")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"$schema": "https://changesaga.dev/schema/v5/saga.schema.json", "version": 5, "id": "fixture", "title": "Fixture", "source": map[string]any{"repository": "https://example.test/acme/app.git"}}
	if err := store.WriteJSON(filepath.Join(root, "saga.json"), manifest, true); err != nil {
		t.Fatal(err)
	}
	fixture, err := WriteInventoryFixture(ctx, root, checkout)
	if err != nil {
		t.Fatal(err)
	}
	d, err := requirements.LoadInventory(root, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if d.Format != 2 || len(d.Records) != 8 || len(fixture.Delivery) != 40 {
		t.Fatalf("fixture shape: format=%d records=%d", d.Format, len(d.Records))
	}
	store3 := d.Pinned(fixture.Pins["store-r3"])
	if store3 == nil || store3.EffectiveIntent() != requirements.IntentImplemented || d.Pinned(fixture.Pins["store-r2"]).Baseline != fixture.Pins["store-r1"].Revision {
		t.Fatal("component succession not recorded")
	}
	composed, err := d.ComposeOverlay(d.Pinned(fixture.Pins["overlay"]))
	if err != nil || len(composed) != 2 {
		t.Fatalf("overlay: %v %v", composed, err)
	}
}
