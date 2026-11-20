package cli

import (
	"encoding/json"
	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Historical fixture bytes belong to an actual Git commit. A companion Saga
// receives its own temporary repository; no test fabricates a revision archive.
func commitCurrentTestDocumentation(t *testing.T, root, message string) string {
	t.Helper()
	if exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Run() != nil {
		git(t, filepath.Dir(root), "init", "-b", "main")
	}
	git(t, root, "add", ".")
	git(t, root, "-c", "user.name=Test Author", "-c", "user.email=test@example.test", "commit", "--allow-empty", "-m", message)
	return strings.TrimSpace(git(t, root, "rev-parse", "HEAD"))
}

func committedCurrentTestFile(t *testing.T, root, commit, relative string) string {
	t.Helper()
	prefix := strings.TrimSpace(git(t, root, "rev-parse", "--show-prefix"))
	return git(t, root, "show", commit+":"+path.Join(prefix, relative))
}

// Capture the exact current definition when authoring fixture evidence. Tests
// needing older content must capture this before editing and commit the Saga.
func currentTestPin(t *testing.T, root, target string) string {
	t.Helper()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(target, ":")
	if len(parts) < 5 {
		t.Fatalf("bad fixture target %s", target)
	}
	switch parts[3] {
	case "component", "system", "data-entity", "erd", "erd-overlay":
		inventory, err := requirements.LoadInventory(root, manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		record := inventory.Find(target)
		if record == nil {
			t.Fatalf("missing fixture record %s", target)
		}
		digest, err := record.CurrentContentDigest()
		if err != nil {
			t.Fatal(err)
		}
		return digest
	case "story":
		document, err := requirements.Load(root, manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		record := document.FindStory(parts[4])
		if record == nil || record.CurrentRevision == nil {
			t.Fatalf("missing fixture story %s", target)
		}
		return requirements.ContentPin(target, record.CurrentRevision.ID)
	case "test-case":
		document, err := quality.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range document.TestCases {
			if record.Identity.ID == parts[4] && record.CurrentRevision != nil {
				digest, err := currentrecord.Digest(*record.CurrentRevision)
				if err != nil {
					t.Fatal(err)
				}
				return digest
			}
		}
		t.Fatalf("missing fixture test case %s", target)
	case "prototype":
		document, err := prototypes.Load(root, manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		record := document.Find(parts[4])
		if record == nil || record.CurrentRevision == nil {
			t.Fatalf("missing fixture prototype %s", target)
		}
		digest, err := prototypes.RevisionURN(manifest.ID, parts[4], record.CurrentRevision.ID)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	t.Fatalf("unsupported fixture pin %s", target)
	return ""
}

func currentPrototypeTestHTML(t *testing.T, root, id string) string {
	t.Helper()
	document, err := prototypes.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	record := document.Find(id)
	if record == nil || record.CurrentRevision == nil {
		t.Fatal("missing prototype", id)
	}
	dir, err := document.RevisionDir(*record, record.CurrentRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "html", "index.html")
}

// Explicit old-format bytes retain reader coverage for competing legacy heads.
// Current writers never create a branch graph.
func competingTechnicalTestFixture(t *testing.T, root, target string) {
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
	if record == nil || record.CurrentRevision == nil {
		t.Fatal("missing technical fixture", target)
	}
	dir := filepath.Join(root, requirements.TechnicalPath(record.Kind, record.Identity.ID))
	raw, err := os.ReadFile(filepath.Join(dir, "current.json"))
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
	for _, sub := range []string{"revisions", "events"} {
		if err = os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string, value any) {
		data, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, data, 0644); e != nil {
			t.Fatal(e)
		}
	}
	for _, id := range []string{"r1", "r2", "fork"} {
		value := current.Content
		value["$schema"], value["version"], value["id"], value["parents"], value["created_at"] = requirements.TechnicalSchema(record.Kind, "-revision"), 5, id, []string{}, record.Identity.CreatedAt
		if id != "r1" {
			value["parents"] = []string{target + ":revision:r1"}
		}
		value["name"] = id + " reader"
		write(filepath.Join(dir, "revisions", id+".json"), value)
	}
	state := current.State
	state["$schema"], state["version"], state["id"], state["parents"], state["created_at"] = requirements.TechnicalSchema(record.Kind, "-event"), 5, "active", []string{}, record.Identity.CreatedAt
	write(filepath.Join(dir, "events/active.json"), state)
	if err = os.Remove(filepath.Join(dir, "current.json")); err != nil {
		t.Fatal(err)
	}
}

func legacyCurrentTestPackage(t *testing.T, root, relative, ownerKey, owner, revisionSchema, eventSchema string, version int, revisionID, eventID string) {
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
