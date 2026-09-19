package saga

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const v5SchemaDir = "../../schema/v5"

var v5RecordSchemas = []string{
	"saga.schema.json",
	"relation.schema.json",
	"coverage-exception.schema.json",
	"test-case.schema.json",
	"test-case-revision.schema.json",
	"test-case-event.schema.json",
	"quality-evidence.schema.json",
	"test-run.schema.json",
	"quality-policy.schema.json",
}

func loadV5Schema(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(v5SchemaDir, name))
	if err != nil {
		t.Fatalf("read v5 %s: %v", name, err)
	}
	var value map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("parse v5 %s: %v", name, err)
	}
	return value
}

func TestV5RecordSchemasAreClosedDraft202012(t *testing.T) {
	for _, name := range v5RecordSchemas {
		t.Run(name, func(t *testing.T) {
			schema := loadV5Schema(t, name)
			if got := schema["$schema"]; got != "https://json-schema.org/draft/2020-12/schema" {
				t.Errorf("dialect = %v", got)
			}
			if got := schema["$id"]; got != "https://changesaga.dev/schema/v5/"+name {
				t.Errorf("$id = %v", got)
			}
			if schema["additionalProperties"] != false {
				t.Error("root object is not closed")
			}
			assertClosedObjectSchemas(t, name, schema)
		})
	}
}

func assertClosedObjectSchemas(t *testing.T, path string, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		if value["type"] == "object" && value["additionalProperties"] != false {
			t.Errorf("%s declares an object without additionalProperties:false", path)
		}
		for key, child := range value {
			assertClosedObjectSchemas(t, path+"/"+key, child)
		}
	case []any:
		for i, child := range value {
			assertClosedObjectSchemas(t, path+"/"+strconv.Itoa(i), child)
		}
	}
}

func TestSagaSchemaURLIsThePublishedV5ManifestID(t *testing.T) {
	schema := loadV5Schema(t, "saga.schema.json")
	if got := schema["$id"]; got != SagaSchemaURL {
		t.Fatalf("v5 saga schema $id = %v, want %s", got, SagaSchemaURL)
	}
	if got := dig(t, schema, "properties", "version", "const"); got != json.Number(strconv.Itoa(SagaVersion)) {
		t.Fatalf("v5 saga schema version = %v, want %d", got, SagaVersion)
	}
}

func TestV5SchemaRuntimeParity(t *testing.T) {
	id := schemaPattern(t, loadV5Schema(t, "saga.schema.json"), "$defs", "id", "pattern")
	for _, probe := range identityProbes {
		if got, want := id.MatchString(probe), ValidID(probe); got != want {
			t.Errorf("stable id %q: schema=%v runtime=%v", probe, got, want)
		}
	}

	digest := schemaPattern(t, loadV5Schema(t, "relation.schema.json"), "$defs", "digest", "pattern")
	for value, valid := range map[string]bool{
		"sha256:" + strings.Repeat("a", 64): true,
		"sha256:" + strings.Repeat("A", 64): false,
		strings.Repeat("a", 64):             false,
		"sha256:" + strings.Repeat("0", 63): false,
	} {
		if digest.MatchString(value) != valid {
			t.Errorf("digest grammar mismatch for %q", value)
		}
	}

	wantRelations := []string{"addresses", "conflicts_with", "explains", "implements", "refines", "supersedes", "verifies"}
	if got := schemaEnum(t, loadV5Schema(t, "relation.schema.json"), "properties", "type", "enum"); !reflect.DeepEqual(got, wantRelations) {
		t.Errorf("relation enum = %v, want %v", got, wantRelations)
	}
	wantKinds := []string{"edge", "negative", "positive"}
	if got := schemaEnum(t, loadV5Schema(t, "test-case-revision.schema.json"), "properties", "coverage_kinds", "items", "enum"); !reflect.DeepEqual(got, wantKinds) {
		t.Errorf("coverage kind enum = %v, want %v", got, wantKinds)
	}
	wantStates := []string{"active", "deprecated", "proposed", "retired"}
	if got := schemaEnum(t, loadV5Schema(t, "test-case-event.schema.json"), "properties", "state", "enum"); !reflect.DeepEqual(got, wantStates) {
		t.Errorf("test lifecycle enum = %v, want %v", got, wantStates)
	}
	wantResults := []string{"blocked", "failed", "passed", "skipped"}
	if got := schemaEnum(t, loadV5Schema(t, "test-run.schema.json"), "properties", "result", "enum"); !reflect.DeepEqual(got, wantResults) {
		t.Errorf("test result enum = %v, want %v", got, wantResults)
	}
}

func TestV5GoldenFixturesValidate(t *testing.T) {
	fixtures, err := filepath.Glob(filepath.Join(v5SchemaDir, "examples", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var records []string
	for _, fixture := range fixtures {
		if !strings.Contains(fixture, ".golden.") {
			records = append(records, fixture)
		}
	}
	if len(records) != 10 {
		t.Fatalf("record fixture count = %d, want 10", len(records))
	}
	for _, fixture := range records {
		name := strings.TrimSuffix(filepath.Base(fixture), ".json") + ".schema.json"
		schemaPath := filepath.Join(v5SchemaDir, name)
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			compiler.AssertFormat()
			schema, err := compiler.Compile(schemaPath)
			if err != nil {
				t.Fatalf("compile %s: %v", name, err)
			}
			input, err := os.Open(fixture)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			instance, err := jsonschema.UnmarshalJSON(input)
			if err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(instance); err != nil {
				t.Fatalf("%s does not validate against %s: %v", fixture, name, err)
			}
		})
	}
}

func TestV5VisualDigestGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(v5SchemaDir, "examples", "visual-digest-v1.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		ItemManifest       string `json:"item_manifest"`
		SlideManifest      string `json:"slide_manifest"`
		DeckManifest       string `json:"deck_manifest"`
		Asset              string `json:"asset"`
		ItemManifestDigest string `json:"item_manifest_digest"`
		ItemContentDigest  string `json:"item_content_digest"`
		SlideContentDigest string `json:"slide_content_digest"`
		DeckContentDigest  string `json:"deck_content_digest"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	itemManifest := visualV1Digest("change-saga-visual-item-manifest-v1", golden.ItemManifest)
	itemContent := visualV1Digest("change-saga-visual-item-v1", golden.ItemManifest, golden.Asset)
	slideContent := visualV1Digest("change-saga-visual-slide-v1", golden.SlideManifest, golden.Asset, itemManifest)
	deckContent := visualV1Digest("change-saga-visual-deck-v1", golden.DeckManifest, slideContent)
	got := []string{itemManifest, itemContent, slideContent, deckContent}
	want := []string{golden.ItemManifestDigest, golden.ItemContentDigest, golden.SlideContentDigest, golden.DeckContentDigest}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("visual-v1 digests = %v, want %v", got, want)
	}
}

func visualV1Digest(domain string, values ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write([]byte{0})
	var length [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

func TestV5SchemasRejectUnknownFieldsAndContractViolations(t *testing.T) {
	tests := []struct {
		schema  string
		fixture string
		mutate  func(map[string]any)
	}{
		{"test-case", "test-case", func(v map[string]any) { v["author"] = "not persisted" }},
		{"test-case-revision", "test-case-revision", func(v map[string]any) { v["coverage_kinds"] = []any{"edge", "edge"} }},
		{"test-case-event", "test-case-event", func(v map[string]any) { v["state"] = "passed" }},
		{"quality-evidence", "quality-evidence", func(v map[string]any) { v["diffs"] = []any{} }},
		{"test-run", "test-run", func(v map[string]any) { v["result"] = "successful" }},
		{"quality-policy", "quality-policy", func(v map[string]any) { v["required_kinds"] = []any{} }},
		{"coverage-exception", "coverage-exception", func(v map[string]any) { v["citations"] = []any{} }},
		{"relation", "relation", func(v map[string]any) { v["scope"] = "descendants" }},
		{"relation-supersedes-kind", "relation", func(v map[string]any) { v["type"] = "supersedes" }},
	}
	for _, test := range tests {
		t.Run(test.schema, func(t *testing.T) {
			compiler := jsonschema.NewCompiler()
			schemaName := strings.TrimSuffix(test.schema, "-supersedes-kind")
			schema, err := compiler.Compile(filepath.Join(v5SchemaDir, schemaName+".schema.json"))
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(v5SchemaDir, "examples", test.fixture+".json"))
			if err != nil {
				t.Fatal(err)
			}
			var instance map[string]any
			if err := json.Unmarshal(data, &instance); err != nil {
				t.Fatal(err)
			}
			test.mutate(instance)
			if err := schema.Validate(instance); err == nil {
				t.Fatal("invalid contract mutation unexpectedly validated")
			}
		})
	}
}

const v5TestManifest = `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"v5","title":"Composition","source":{"repository":"https://example.test/acme/app.git"}}`

func writeV5Composition(t *testing.T, manifest string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "v5.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), manifest)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "content.md"), "Composition.\n")
	return root
}

func loadErrors(t *testing.T, root string) []string {
	t.Helper()
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, issue := range validation.Issues {
		if issue.Severity == "error" {
			messages = append(messages, issue.Path+": "+issue.Message)
		}
	}
	if validation.Valid != (len(messages) == 0) {
		t.Fatalf("validity %v disagrees with errors %v", validation.Valid, messages)
	}
	return messages
}

func TestSagaLoadsReportComponentsAndEveryLivingRoot(t *testing.T) {
	root := writeV5Composition(t, v5TestManifest)
	for _, name := range []string{"___requirements", "___workplan", "___design", QualityRootDir, EmbeddedSlidesDir} {
		if err := os.MkdirAll(filepath.Join(root, testEpicDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if messages := loadErrors(t, root); len(messages) != 0 {
		t.Fatalf("Saga rejected: %v", messages)
	}
	if _, validation, err := LoadMutationIndex(root); err != nil || !validation.Valid {
		t.Fatalf("mutation index = %v, %#v", err, validation.Issues)
	}
}

func TestSagaValidationRejectsMisplacedRootsAndNoncanonicalRepository(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{"quality root nested", func(t *testing.T) string {
			root := writeV5Composition(t, v5TestManifest)
			if err := os.MkdirAll(filepath.Join(root, testEpicDir, "intro.chapter", QualityRootDir), 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(root, testEpicDir, "intro.chapter", "chapter.json"), `{"version":2,"id":"intro","title":"Intro"}`)
			return root
		}, "unknown reserved directory"},
		{"noncanonical repository", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, "https://example.test/acme/app.git", "https://Example.test/acme/app.git", 1))
		}, "source.repository must be canonical"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			messages := loadErrors(t, test.setup(t))
			if !strings.Contains(strings.Join(messages, "\n"), test.want) {
				t.Fatalf("errors %v do not contain %q", messages, test.want)
			}
		})
	}
}

// Only a version 5 saga.json is a Change Saga. Everything else fails to open
// instead of loading with validation issues.
func TestSagaLoadRefusesEveryOtherContainer(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) string
		want  string
	}{
		{"presentation member", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, `"source"`, `"presentation":{"mode":"slides","aspect_ratio":"16:9","overview_deck":"overview"},"source"`, 1))
		}, "presentation"},
		{"flat 00-saga.json root", func(t *testing.T) string {
			root := writeV5Composition(t, v5TestManifest)
			if err := os.Rename(filepath.Join(root, ManifestName), filepath.Join(root, testEpicDir, "00-saga.json")); err != nil {
				t.Fatal(err)
			}
			return root
		}, "has no saga.json; it is not a Change Saga"},
		{"version 2", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, `"version":5`, `"version":2`, 1))
		}, "saga.json: unsupported Saga version 2; change-saga reads only version 5"},
		{"version 3", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, `"version":5`, `"version":3`, 1))
		}, "saga.json: unsupported Saga version 3; change-saga reads only version 5"},
		{"version 4", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, `"version":5`, `"version":4`, 1))
		}, "saga.json: unsupported Saga version 4; change-saga reads only version 5"},
		{"version 6", func(t *testing.T) string {
			return writeV5Composition(t, strings.Replace(v5TestManifest, `"version":5`, `"version":6`, 1))
		}, "saga.json: unsupported Saga version 6; change-saga reads only version 5"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := test.setup(t)
			if _, _, err := Load(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load error = %v, want %q", err, test.want)
			}
			if _, err := ReadManifest(root); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ReadManifest error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestV5SchemaListIsStable(t *testing.T) {
	got := append([]string(nil), v5RecordSchemas...)
	sort.Strings(got)
	want := []string{"coverage-exception.schema.json", "quality-evidence.schema.json", "quality-policy.schema.json", "relation.schema.json", "saga.schema.json", "test-case-event.schema.json", "test-case-revision.schema.json", "test-case.schema.json", "test-run.schema.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record schema set = %v, want %v", got, want)
	}
}
