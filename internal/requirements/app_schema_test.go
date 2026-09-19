package requirements

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/twentyideas/changesaga/internal/coderef"
)

// Every app-level record and every story revision the writers produce must
// validate against the schema its $schema names.
func TestAppRecordsValidateAgainstTheirPublishedSchemas(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	repository := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	root := newSaga(t)
	if _, err := AddStory(root, "test", storyInput("checkout", "r1", "proposed", []Criterion{{ID: "fast", Statement: "Fast"}})); err != nil {
		t.Fatal(err)
	}
	if _, err := AddFlag(root, "test", AddFlagInput{ID: "new-checkout", RevisionID: "r1", EventID: "off", Description: "New checkout",
		Targets: []string{"urn:change-saga:test:story:checkout", "urn:change-saga:test:epic:core"}, CreatedAt: testTime}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddTerm(root, "test", testtakerInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := ReviseTerm(root, "test", ReviseTermInput{Term: testTermURN, ID: "r2", Parents: []string{testTermURN + ":revision:r1"}, CreatedAt: testTime,
		TermDefinition: TermDefinition{Name: "Testtaker", Definition: "One sitting.", Code: []coderef.Reference{{Commit: strings.Repeat("c", 40), Path: "a.go", Digest: "sha256:" + strings.Repeat("d", 64), Note: "the whole file"}}}}); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	checked := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".json" || entry.Name() == "saga.json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		url, _ := value["$schema"].(string)
		name := strings.TrimPrefix(url, "https://changesaga.dev/schema/")
		schema, err := compiler.Compile(filepath.Join(repository, "schema", filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s names an unpublished schema %q: %v", path, url, err)
		}
		instance, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		if err := schema.Validate(instance); err != nil {
			t.Errorf("%s does not validate against %s: %v", path, url, err)
		}
		checked[name] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"v5/epic.schema.json", "v5/persona.schema.json", "v5/persona-revision.schema.json", "v5/persona-event.schema.json",
		"v5/flag.schema.json", "v5/flag-revision.schema.json", "v5/flag-event.schema.json", "v3/story-revision.schema.json",
		"v5/term.schema.json", "v5/term-revision.schema.json", "v5/term-event.schema.json"} {
		if !checked[name] {
			t.Errorf("no written record exercised %s", name)
		}
	}
}
