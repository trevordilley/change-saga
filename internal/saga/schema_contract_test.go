package saga

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
)

// The published JSON Schemas are the normative contract, and this package is
// the reference implementation of it. Every rule below is checked twice: once
// against the schema text as shipped, and once against the Go predicate the
// runtime actually uses. A rule that only one side enforces is exactly the
// class of bug where a saga validates in one tool and is rejected by another.

const schemaDir = "../../schema/v2"

func loadSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(schemaDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return value
}

func TestSchemaFilesAreWellFormed(t *testing.T) {
	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		count++
		schema := loadSchema(t, entry.Name())
		for _, key := range []string{"$schema", "$id", "title"} {
			if _, ok := schema[key]; !ok {
				t.Errorf("%s: missing %s", entry.Name(), key)
			}
		}
		if schema["additionalProperties"] != false {
			t.Errorf("%s: v2 rejects unknown fields, so additionalProperties must be false", entry.Name())
		}
		if id, _ := schema["$id"].(string); id != "https://changesaga.dev/schema/v2/"+entry.Name() {
			t.Errorf("%s: $id %q does not match its path", entry.Name(), id)
		}
	}
	if count < 8 {
		t.Fatalf("expected the full v2 schema set, found %d files", count)
	}
}

// dig walks a decoded schema by key path so a moved or renamed keyword fails
// loudly instead of silently skipping the comparison.
func dig(t *testing.T, value any, path ...string) any {
	t.Helper()
	for i, key := range path {
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("schema path %v is not an object at %q", path[:i], key)
		}
		value, ok = object[key]
		if !ok {
			t.Fatalf("schema path %v has no %q", path[:i], key)
		}
	}
	return value
}

func schemaPattern(t *testing.T, value any, path ...string) *regexp.Regexp {
	t.Helper()
	text, ok := dig(t, value, path...).(string)
	if !ok {
		t.Fatalf("schema path %v is not a pattern string", path)
	}
	pattern, err := regexp.Compile(text)
	if err != nil {
		t.Fatalf("schema pattern %q does not compile: %v", text, err)
	}
	return pattern
}

func schemaEnum(t *testing.T, value any, path ...string) []string {
	t.Helper()
	raw, ok := dig(t, value, path...).([]any)
	if !ok {
		t.Fatalf("schema path %v is not an enum array", path)
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("schema path %v contains a non-string enum value", path)
		}
		result = append(result, text)
	}
	sort.Strings(result)
	return result
}

// identityProbes exercise the boundaries of the stable identifier grammar from
// both directions: valid shapes that must survive, and every way an attacker or
// a careless author could try to smuggle a separator, a traversal segment, or
// an oversized value into a filename or URN.
var identityProbes = []string{
	"a", "A", "0", "saga", "saga-id", "saga_id", "saga.id", "a.b-c_d9",
	"20260819T120000.000000000Z-0a1b2c3d",
	"", " ", "  a", "a ", "-a", "_a", ".a", "..", ".", "../a", "a/b", "a\\b",
	"a:b", "a b", "a\tb", "a\nb", "a\x00b", "é", "a é", "🙂", "a?b", "a*b",
	"urn:change-saga:a:saga", "%2e%2e", "CON", "nul",
}

func init() {
	long := ""
	for len(long) < 200 {
		long += "abcdefghij"
	}
	identityProbes = append(identityProbes, long[:127], long[:128], long[:129], long[:200])
}

func TestStableIDGrammarMatchesEverySchema(t *testing.T) {
	files := map[string][]string{
		"chapter.schema.json":      {"properties", "id", "pattern"},
		"section.schema.json":      {"properties", "id", "pattern"},
		"fragment.schema.json":     {"properties", "id", "pattern"},
		"claim.schema.json":        {"properties", "id", "pattern"},
		"verification.schema.json": {"properties", "id", "pattern"},
	}
	for name, path := range files {
		schema := loadSchema(t, name)
		pattern := schemaPattern(t, schema, path...)
		for _, probe := range identityProbes {
			if got, want := pattern.MatchString(probe), ValidID(probe); got != want {
				t.Errorf("%s id %q: schema=%v runtime=%v", name, probe, got, want)
			}
		}
	}
}

func TestMarkdownAnchorGrammarMatchesLandmarkSchema(t *testing.T) {
	schema := loadSchema(t, "landmark.schema.json")
	selectors, ok := dig(t, schema, "properties", "selector", "oneOf").([]any)
	if !ok {
		t.Fatal("landmark selector is not a oneOf list")
	}
	patterns := []*regexp.Regexp{schemaPattern(t, schema, "properties", "id", "pattern")}
	for _, selector := range selectors {
		for _, field := range []string{"heading_id", "element_id"} {
			properties, _ := dig(t, selector, "properties").(map[string]any)
			if _, ok := properties[field]; ok {
				patterns = append(patterns, schemaPattern(t, selector, "properties", field, "pattern"))
			}
		}
	}
	if len(patterns) != 3 {
		t.Fatalf("expected the landmark id, heading_id, and element_id grammars, found %d", len(patterns))
	}
	probes := append([]string{"a", "a-b", "a1", "request-validation", "A", "1a", "-a", "a_b", "a.b", "a--b", "z"}, identityProbes...)
	long := ""
	for len(long) < 80 {
		long += "abcdefghij"
	}
	probes = append(probes, long[:63], long[:64], long[:65])
	for _, pattern := range patterns {
		for _, probe := range probes {
			if got, want := pattern.MatchString(probe), ValidMarkdownAnchor(probe); got != want {
				t.Errorf("anchor grammar %q for %q: schema=%v runtime=%v", pattern, probe, got, want)
			}
		}
	}
}

func TestMediaTypeGrammarMatchesFragmentSchema(t *testing.T) {
	pattern := schemaPattern(t, loadSchema(t, "fragment.schema.json"), "properties", "media_type", "pattern")
	probes := []string{
		"text/markdown", "text/html", "text/plain", "image/svg+xml", "image/png",
		"image/jpeg", "image/webp", "image/x-icon", "image/", "image", "image/a b",
		"image/../../etc/passwd", "image/png\r\nX-Injected: 1", "IMAGE/PNG",
		"text/markdown; charset=utf-8", "text/csv", "application/json", "",
		" text/html", "text/html ", "text/html\n",
	}
	for _, probe := range probes {
		if got, want := pattern.MatchString(probe), ValidMediaType(probe); got != want {
			t.Errorf("media_type %q: schema=%v runtime=%v", probe, got, want)
		}
	}
}

func TestEntrypointGrammarMatchesFragmentSchema(t *testing.T) {
	schema := loadSchema(t, "fragment.schema.json")
	entrypoint := dig(t, schema, "properties", "entrypoint")
	allow := schemaPattern(t, entrypoint, "pattern")
	denials, ok := dig(t, entrypoint, "not", "anyOf").([]any)
	if !ok {
		t.Fatal("entrypoint denials are not an anyOf list")
	}
	var deny []*regexp.Regexp
	for _, item := range denials {
		deny = append(deny, schemaPattern(t, item, "pattern"))
	}
	schemaAccepts := func(value string) bool {
		if value == "" || !allow.MatchString(value) {
			return false
		}
		for _, pattern := range deny {
			if pattern.MatchString(value) {
				return false
			}
		}
		return true
	}
	for _, probe := range entrypointProbes {
		if got, want := schemaAccepts(probe), EntrypointError(probe) == ""; got != want {
			t.Errorf("entrypoint %q: schema=%v runtime=%v (%s)", probe, got, want, EntrypointError(probe))
		}
	}
}

// TestCodeReferenceSchemasAgreeWithTheValidator keeps every published copy of
// the code reference identical and its patterns equal to what the loader
// accepts, so a record the schema admits is a record the runtime admits.
func TestCodeReferenceSchemasAgreeWithTheValidator(t *testing.T) {
	canonical := dig(t, loadSchema(t, "code.schema.json"), "$defs", "code_reference").(map[string]any)
	delete(canonical["properties"].(map[string]any), "note")
	for _, name := range []string{"claim.schema.json"} {
		if got := dig(t, loadSchema(t, name), "$defs", "code_reference"); !reflect.DeepEqual(got, canonical) {
			t.Errorf("%s publishes a different code reference: %v", name, got)
		}
	}
	quality := loadSchema(t, filepath.Join("..", "v5", "quality-evidence.schema.json"))
	if got := dig(t, quality, "$defs", "code_reference"); !reflect.DeepEqual(got, canonical) {
		t.Errorf("quality evidence publishes a different code reference: %v", got)
	}
	commit := schemaPattern(t, canonical, "properties", "commit", "pattern")
	digest := schemaPattern(t, canonical, "properties", "digest", "pattern")
	for value, want := range map[string]bool{
		strings.Repeat("a", 40): true, strings.Repeat("b", 64): true, strings.Repeat("A", 40): false,
		strings.Repeat("a", 39): false, "HEAD": false, "": false,
	} {
		if commit.MatchString(value) != want || coderef.ValidCommit(value) != want {
			t.Errorf("commit %q: schema %v, validator %v, want %v", value, commit.MatchString(value), coderef.ValidCommit(value), want)
		}
	}
	for value, want := range map[string]bool{
		"sha256:" + strings.Repeat("0", 64): true, "sha256:" + strings.Repeat("0", 63): false, strings.Repeat("0", 64): false,
	} {
		reference := coderef.Reference{Commit: strings.Repeat("a", 40), Path: "app.go", Digest: value}
		if digest.MatchString(value) != want || (coderef.Validate(reference) == nil) != want {
			t.Errorf("digest %q: schema %v, want %v", value, digest.MatchString(value), want)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func equal(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
