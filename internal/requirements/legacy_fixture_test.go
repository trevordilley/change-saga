package requirements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// legacyPackageFixture deliberately constructs the old format for decoder and
// ambiguity coverage. Authoring tests use current writers instead.
func legacyPackageFixture(t *testing.T, dir, ownerKey, owner, revisionSchema, eventSchema string, version int, revisionID, eventID string) {
	t.Helper()
	path := filepath.Join(dir, "current.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var current struct {
		Content map[string]any `json:"content"`
		State   map[string]any `json:"state"`
	}
	if err = json.Unmarshal(data, &current); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"revisions", "events"} {
		if err = os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(sub, id, schema string, value map[string]any) {
		if value == nil {
			t.Fatal("missing fixture content/state")
		}
		value["$schema"], value["version"], value["id"], value["parents"], value["created_at"], value[ownerKey] = schema, version, id, []string{}, testTime, owner
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
