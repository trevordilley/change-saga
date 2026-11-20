package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func currentServerPin(t *testing.T, root, target string) string {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := requirements.LoadInventory(root, manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	record := inventory.Find(target)
	if record == nil {
		t.Fatalf("fixture record missing: %s", target)
	}
	digest, err := record.CurrentContentDigest()
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func commitServerDocumentation(t *testing.T, root string) string {
	t.Helper()
	serverGit(t, root, "add", ".")
	serverGit(t, root, "commit", "--allow-empty", "-m", "Preserve fixture documentation in Git")
	return strings.TrimSpace(serverGit(t, root, "rev-parse", "HEAD"))
}

func currentServerBoardPin(t *testing.T, root, id string) string {
	t.Helper()
	board, err := requirements.LoadWhiteboard(root, "test", id, "")
	if err != nil {
		t.Fatal(err)
	}
	if board == nil {
		t.Fatal("missing fixture board", id)
	}
	digest, err := requirements.WhiteboardContentDigest(*board)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func serverBoardCurrentPath(root, id string) string {
	return filepath.Join(root, requirements.WhiteboardDir, id, "current.json")
}

func legacyServerTestPackage(t *testing.T, root, relative, ownerKey, owner, revisionSchema, eventSchema string, version int, revisionID, eventID string) {
	t.Helper()
	dir := filepath.Join(root, relative)
	path := filepath.Join(dir, "current.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Content map[string]any `json:"content"`
		State   map[string]any `json:"state"`
	}
	if err = json.Unmarshal(raw, &current); err != nil {
		t.Fatal(err)
	}
	stamp := "2026-08-31T19:00:00Z"
	for _, sub := range []string{"revisions", "events"} {
		if err = os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(sub, id, schema string, value map[string]any) {
		if value == nil {
			t.Fatal("missing legacy fixture value")
		}
		value["$schema"], value["version"], value["id"], value["parents"], value["created_at"], value[ownerKey] = schema, version, id, []string{}, stamp, owner
		b, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, sub, id+".json"), b, 0644); e != nil {
			t.Fatal(e)
		}
	}
	write("revisions", revisionID, revisionSchema, current.Content)
	write("events", eventID, eventSchema, current.State)
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyTechnicalLinksUseExactDigestWithoutWriting(t *testing.T) {
	root, repo := technicalFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	target := "urn:change-saga:test:component:store"
	system := inventory.Find("urn:change-saga:test:system:flags")
	oldPin := system.CurrentRevision.Components[1].Revision
	old := inventory.Find(target).Revision(oldPin)
	if old == nil {
		t.Fatal("missing committed original definition")
	}
	legacyServerTestPackage(t, root, requirements.TechnicalPath("component", "store"), "record", target, requirements.TechnicalSchema("component", "-revision"), requirements.TechnicalSchema("component", "-event"), 5, "r2", "active")
	directory := filepath.Join(root, requirements.TechnicalPath("component", "store"), "revisions")
	data, err := os.ReadFile(filepath.Join(directory, "r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var next map[string]any
	json.Unmarshal(data, &next)
	next["parents"] = []string{target + ":revision:r1"}
	data, _ = json.Marshal(next)
	if err = os.WriteFile(filepath.Join(directory, "r2.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	type legacy requirements.TechnicalRevision
	original := *old
	original.ID = "r1"
	original.Parents = []string{}
	data, _ = json.Marshal(legacy(original))
	if err = os.WriteFile(filepath.Join(directory, "r1.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	owner := "urn:change-saga:test:system:flags"
	relative := requirements.TechnicalPath("system", "flags")
	legacyServerTestPackage(t, root, relative, "record", owner, requirements.TechnicalSchema("system", "-revision"), requirements.TechnicalSchema("system", "-event"), 5, "r1", "active")
	source := filepath.Join(root, relative, "revisions/r1.json")
	data, err = os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	var definition map[string]any
	json.Unmarshal(data, &definition)
	definition["components"].([]any)[1].(map[string]any)["revision"] = target + ":revision:r1"
	data, _ = json.Marshal(definition)
	if err = os.WriteFile(source, data, 0644); err != nil {
		t.Fatal(err)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/system/flags")
	href := technicalHref("component", "store", oldPin)
	if status != 200 || !strings.Contains(body, `href="`+href+`"`) || strings.Contains(body, "content_digest=r1") {
		t.Fatalf("legacy link did not preserve exact content digest: %d", status)
	}
	status, body = technicalGet(t, mux, href)
	if status != 200 || !strings.Contains(body, "<h1>Store</h1>") || strings.Contains(body, "<h1>FlagStore</h1>") {
		t.Fatalf("legacy link followed current content: %d", status)
	}
	if after, err := os.ReadFile(source); err != nil || string(after) != string(data) {
		t.Fatal("ordinary read persisted normalization", err)
	}
}
