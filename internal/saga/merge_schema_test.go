package saga

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// A merge record is written at runtime from Merge but published as a closed
// schema, and merge records name no $schema, so nothing else validates them. A
// field the runtime writes but the schema forbids would make every landed
// change's record invalid against its own contract without any test noticing.
func TestMergeRecordsMatchTheirPublishedSchema(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile(filepath.Join("..", "..", "schema", "v2", "merge.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	validate := func(t *testing.T, label string, data []byte) {
		t.Helper()
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(instance); err != nil {
			t.Fatalf("%s does not validate against merge.schema.json: %v", label, err)
		}
	}

	commit := strings.Repeat("a", 40)
	written := Merge{
		Version: ComponentVersion, Commit: commit, Base: strings.Repeat("b", 40), Review: "pr-7",
		Commits: []MergedCommit{{
			Commit: strings.Repeat("c", 40), Author: "Dev <dev@example.com>",
			Date: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC), Subject: "Move the queue to a table", Body: "Transactional enqueue.",
		}},
		PinnedAt: time.Date(2026, 9, 18, 12, 5, 0, 0, time.UTC),
	}
	data, err := json.Marshal(written)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, "a fully populated merge record", data)

	committed, err := filepath.Glob(filepath.Join("..", "..", "*.saga", "___merges", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob(filepath.Join("..", "..", "docs", "sagas", "*", "*.saga", "___merges", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(committed, more...) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		validate(t, path, data)
	}
}
