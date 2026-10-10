package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestMultipleERDDocumentsOverviewQueriesAndImmutableHistory(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := sourceRepo(t, map[string]string{"README.md": "app\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "atomic.saga")
	var output bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	pins, err := testfixture.WriteMultiERDFixture(ctx, root, repo)
	if err != nil {
		t.Fatal(err)
	}
	data, page, _ := inventoryEnvelope(t, "inventory", "--saga", root, "--kind", "erd")
	if data == nil || page["total"] != float64(3) {
		t.Fatalf("three docs: %v %v", data, page)
	}
	overview := data["data_model_overview"].(map[string]any)
	target := overview["target"].(string)
	if target != saga.FragmentTarget("atomic", "atomic-data-model") {
		t.Fatal(overview)
	}
	fragment, _, _ := inventoryEnvelope(t, "fragment", "--saga", root, "--target", target)
	if fragment == nil {
		t.Fatal("query fragment did not find shared prose")
	}
	serialized, _ := json.Marshal(fragment)
	if !strings.Contains(string(serialized), "Local SQLite stores offline edits") {
		t.Fatalf("fragment: %s", serialized)
	}
	inventory, err := requirements.LoadInventory(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	old := inventory.Pinned(pins["sqlite"])
	dir := filepath.Join(root, requirements.TechnicalPath("erd", "sqlite"))
	revisionFile := filepath.Join(dir, "revisions", "r1.json")
	oldRevision, err := os.ReadFile(revisionFile)
	if err != nil {
		t.Fatal(err)
	}
	oldSVG, err := os.ReadFile(filepath.Join(dir, old.Visual.Path))
	if err != nil {
		t.Fatal(err)
	}
	if err := Overview(ctx, []string{"set-data-model", "--text", "Updated shared context.", root}, &output); err != nil {
		t.Fatal(err)
	}
	definition := old.TechnicalDefinition
	definition.Name = "Local SQLite revised"
	path := filepath.Join(t.TempDir(), "sqlite.json")
	body, _ := json.Marshal(definition)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Technical(ctx, "erd", []string{"revise", "--id", "sqlite", "--revision", "r2", "--parent", pins["sqlite"].Revision, "--from", path, "--visual", filepath.Join(dir, old.Visual.Path), "--repo", repo, root}, &output); err != nil {
		t.Fatal(err)
	}
	data, page, _ = inventoryEnvelope(t, "inventory", "--saga", root, "--target", pins["sqlite"].Target, "--history")
	if data == nil || page["total"] != float64(1) {
		t.Fatalf("history: %v %v", data, page)
	}
	serialized, _ = json.Marshal(data)
	if !strings.Contains(string(serialized), "Local SQLite revised") || !strings.Contains(string(serialized), `"id":"r1"`) {
		t.Fatalf("history: %s", serialized)
	}
	afterRevision, _ := os.ReadFile(revisionFile)
	afterSVG, _ := os.ReadFile(filepath.Join(dir, old.Visual.Path))
	if !bytes.Equal(oldRevision, afterRevision) || !bytes.Equal(oldSVG, afterSVG) {
		t.Fatal("overview or revision rewrote immutable ERD history")
	}
	inventory, err = requirements.LoadInventory(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	cross := inventory.Pinned(pins["cross-store"])
	if len(cross.Directory) != 2 || cross.Directory[0] != pins["local-edit"] || cross.Directory[1] != pins["cloud-record"] {
		t.Fatal("cross-store shared pins changed")
	}
	// Existing ERDs remain fully readable without adopting the optional prose.
	if err := os.RemoveAll(filepath.Join(root, applayout.OverviewDir, applayout.OverviewDataModel)); err != nil {
		t.Fatal(err)
	}
	data, page, _ = inventoryEnvelope(t, "inventory", "--saga", root, "--kind", "erd")
	if data == nil || data["data_model_overview"] != nil || page["total"] != float64(3) {
		t.Fatalf("legacy read: %v %v", data, page)
	}
	assertValid(t, root)
}
