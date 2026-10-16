package requirements

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestWhiteboardMetadataRoleLifecyclePreservesSourceHistory(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	ctx := context.Background()
	for _, id := range []string{"data-model", "existing", "infrastructure"} {
		for _, rev := range []string{"r1", "r2"} {
			parent := ""
			if rev == "r2" {
				parent = "r1"
			}
			if _, err := PublishWhiteboard(ctx, root, "test", id, rev, parent, id, boardDocument(pin)); err != nil {
				t.Fatal(err)
			}
		}
	}
	target, _ := WhiteboardURN("test", "existing")
	targets := DeprecationTargets{Root: root, SagaID: "test"}
	reference, err := targets.Pin(target, "cloud/jobs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddDeprecation(root, "test", AddDeprecationInput{ID: "preserved-reference", RevisionID: "r1", EventID: "active", DeprecationDefinition: DeprecationDefinition{Scope: "content", Reason: "Preserve this board reference", Target: reference}}); err != nil {
		t.Fatal(err)
	}
	saved := map[string][]byte{}
	filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			saved[path], _ = os.ReadFile(path)
		}
		return err
	})
	mutate := func(w WhiteboardMetadataWrite) WhiteboardMetadataChange {
		t.Helper()
		v, err := UpdateWhiteboardMetadata(ctx, root, "test", w)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	dry := mutate(WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "erd", Expected: "data-model", DryRun: true})
	if !dry.Changed || dry.Before.Roles["erd"] != "data-model" || dry.After.Roles["erd"] != "existing" {
		t.Fatalf("bad plan: %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(root, WhiteboardDir, WhiteboardMetadataName)); !os.IsNotExist(err) {
		t.Fatal("dry run wrote metadata")
	}
	assigned := mutate(WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "erd", Expected: "data-model"})
	for _, w := range []WhiteboardMetadataWrite{
		{Operation: "assign-role", ID: "data-model", Role: "erd", Expected: "data-model"},
		{Operation: "assign-role", ID: "existing", Role: "data-flows", Expected: "none"},
		{Operation: "assign-role", ID: "existing", Role: "data-flows", Expected: "none", Move: true},
		{Operation: "archive", ID: "existing"},
		{Operation: "unassign-role", ID: "data-model", Role: "erd"},
		{Operation: "rename", ID: "existing", Name: "New", ExpectedSnapshot: dry.Before.Snapshot()},
	} {
		if _, err := UpdateWhiteboardMetadata(ctx, root, "test", w); err == nil {
			t.Fatalf("unsafe mutation accepted: %+v", w)
		}
	}
	moved := mutate(WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "data-flows", Expected: "none", Move: true, ExpectedSnapshot: assigned.After.Snapshot()})
	if moved.After.Roles["erd"] != "" || moved.After.Roles["data-flows"] != "existing" {
		t.Fatal("role move did not account for both roles")
	}
	mutate(WhiteboardMetadataWrite{Operation: "unassign-role", ID: "existing", Role: "data-flows"})
	mutate(WhiteboardMetadataWrite{Operation: "rename", ID: "existing", Name: "Renamed presentation"})
	mutate(WhiteboardMetadataWrite{Operation: "archive", ID: "existing"})
	if _, err := UpdateWhiteboardMetadata(ctx, root, "test", WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "erd", Expected: "none"}); err == nil {
		t.Fatal("assigned archived board")
	}
	if _, err := LoadResolvedWhiteboard(root, "test", "existing", "r1"); err != nil {
		t.Fatal("archive broke historical references", err)
	}
	if health := targets.Observe(reference); health.Status != "current" {
		t.Fatalf("archive broke inbound reference: %+v", health)
	}
	mutate(WhiteboardMetadataWrite{Operation: "restore", ID: "existing"})
	metadata, err := LoadWhiteboardMetadata(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Roles["erd"] != "" || metadata.Role("existing") != "" || metadata.Boards["existing"].Name != "Renamed presentation" {
		t.Fatalf("explicit unassignment resurrected defaults: %+v", metadata)
	}
	for path, before := range saved {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("changed source/history/reference %s: %v", path, err)
		}
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardMetadataRejectsContainmentOwnershipAndMalformedState(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	if _, err := PublishWhiteboard(context.Background(), root, "test", "existing", "r1", "", "Existing", boardDocument(pin)); err != nil {
		t.Fatal(err)
	}
	write := WhiteboardMetadataWrite{Operation: "rename", ID: "existing", Name: "New"}
	if _, err := UpdateWhiteboardMetadata(context.Background(), root, "other", write); err == nil {
		t.Fatal("accepted foreign Saga ownership")
	}
	for _, id := range []string{"../existing", "missing"} {
		w := write
		w.ID = id
		if _, err := UpdateWhiteboardMetadata(context.Background(), root, "test", w); err == nil {
			t.Fatal("accepted", id)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	os.WriteFile(outside, []byte("unchanged"), 0600)
	path := filepath.Join(root, WhiteboardDir, WhiteboardMetadataName)
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateWhiteboardMetadata(context.Background(), root, "test", write); err == nil {
		t.Fatal("accepted symlink metadata")
	}
	if data, _ := os.ReadFile(outside); string(data) != "unchanged" {
		t.Fatal("modified outside target")
	}
	os.Remove(path)
	if _, err := UpdateWhiteboardMetadata(context.Background(), root, "test", write); err != nil {
		t.Fatal(err)
	}
	metadata, _ := LoadWhiteboardMetadata(root, "test")
	invalid := metadata
	invalid.Roles = map[string]string{"erd": "existing", "infrastructure": "existing", "data-flows": ""}
	data, _ := json.Marshal(invalid)
	os.WriteFile(path, data, 0600)
	if _, err := LoadWhiteboardMetadata(root, "test"); err == nil {
		t.Fatal("accepted duplicate role ownership")
	}
	for _, invalid := range []string{`{}`, `{"version":1,"saga_id":"test","roles":{"erd":"","infrastructure":"","data-flows":""},"boards":{}}`} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadWhiteboardMetadata(root, "test"); err == nil {
			t.Fatal("accepted missing required metadata fields")
		}
	}

}

func TestWhiteboardMetadataPublishedSchema(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	if _, err := PublishWhiteboard(context.Background(), root, "test", "existing", "r1", "", "Existing", boardDocument(pin)); err != nil {
		t.Fatal(err)
	}
	result, err := UpdateWhiteboardMetadata(context.Background(), root, "test", WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "erd", Expected: "none"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "schema", "v5", "whiteboard-metadata.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	resource, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(WhiteboardMetadataSchemaURL, resource); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(WhiteboardMetadataSchemaURL)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(result.After)
	instance, _ := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err := schema.Validate(instance); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWhiteboardMetadata(root, "test")
	if err != nil || !reflect.DeepEqual(got.Roles, result.After.Roles) {
		t.Fatal(got, err)
	}
}

func TestWhiteboardRoleExpectedURNDisambiguatesNoneID(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	for _, id := range []string{"none", "existing"} {
		if _, err := PublishWhiteboard(context.Background(), root, "test", id, "r1", "", id, boardDocument(pin)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := UpdateWhiteboardMetadata(context.Background(), root, "test", WhiteboardMetadataWrite{Operation: "assign-role", ID: "none", Role: "erd", Expected: "none"}); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateWhiteboardMetadata(context.Background(), root, "test", WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing", Role: "erd", Expected: "urn:change-saga:test:whiteboard:none"}); err != nil {
		t.Fatal(err)
	}
}
