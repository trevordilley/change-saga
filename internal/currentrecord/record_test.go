package currentrecord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func legacy(t *testing.T, root, id string, parents []string, title string) {
	t.Helper()
	put(t, filepath.Join(root, "a.story", "revisions", id+".json"), map[string]any{"$schema": "https://changesaga.dev/schema/v3/story-revision.schema.json", "version": 3, "id": id, "story": "urn:change-saga:test:story:a", "parents": parents, "title": title, "created_at": "2000-01-01T00:00:00Z", "acceptance_criteria": []any{map[string]any{"id": "stable", "statement": "Keep me"}}})
}
func state(t *testing.T, root string) {
	put(t, filepath.Join(root, "a.story", "story.json"), map[string]any{"id": "a"})
	t.Helper()
	put(t, filepath.Join(root, "a.story", "events", "accepted.json"), map[string]any{"id": "accepted", "parents": []string{}, "story": "urn:change-saga:test:story:a", "state": "accepted", "reason": "Agreed requirement"})
}
func TestNormalizePreservesContentAndStaleEvidence(t *testing.T) {
	root := t.TempDir()
	legacy(t, root, "r1", nil, "Old")
	legacy(t, root, "r2", []string{"urn:change-saga:test:story:a:revision:r1"}, "Current")
	state(t, root)
	put(t, filepath.Join(root, "relation.json"), map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation.schema.json", "to": "urn:change-saga:test:story:a", "to_revision": "urn:change-saga:test:story:a:revision:r1"})
	put(t, filepath.Join(root, "evidence", "asset.json"), map[string]any{"revision": "ordinary external asset data"})
	index, err := AuditPins(root)
	if err != nil {
		t.Fatal(err)
	}
	oldDigest, err := index.Resolve("urn:change-saga:test:story:a:revision:r1")
	if err != nil {
		t.Fatal(err)
	}
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	record, err := Read(filepath.Join(root, "a.story"))
	if err != nil {
		t.Fatal(err)
	}
	currentDigest, _ := Digest(record.Content)
	if oldDigest == currentDigest {
		t.Fatal("stale evidence was made current")
	}
	var content map[string]any
	if err = json.Unmarshal(record.Content, &content); err != nil {
		t.Fatal(err)
	}
	if content["title"] != "Current" {
		t.Fatalf("wrong selected content: %s", record.Content)
	}
	data, _ := os.ReadFile(filepath.Join(root, "relation.json"))
	if !strings.Contains(string(data), oldDigest) {
		t.Fatalf("pin lost: %s", data)
	}
	if _, err := os.Stat(filepath.Join(root, "a.story", "revisions")); !os.IsNotExist(err) {
		t.Fatalf("legacy history retained: %v", err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "a.story", Name))
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "a.story", Name))
	if string(before) != string(after) {
		t.Fatal("normalization is not idempotent")
	}
}
func TestAmbiguousLegacyHeadsAreNonDestructive(t *testing.T) {
	root := t.TempDir()
	legacy(t, root, "base", nil, "Base")
	legacy(t, root, "left", []string{"base"}, "Left")
	legacy(t, root, "right", []string{"base"}, "Right")
	state(t, root)
	if err := Normalize(root); err == nil || !strings.Contains(err.Error(), "left right") {
		t.Fatalf("missing explicit conflict: %v", err)
	}
	for _, id := range []string{"base", "left", "right"} {
		if _, err := os.Stat(filepath.Join(root, "a.story", "revisions", id+".json")); err != nil {
			t.Fatal(err)
		}
	}
	if record, err := Read(filepath.Join(root, "a.story")); err != nil || record != nil {
		t.Fatal("conflicting content was published")
	}
}
func TestInterruptedCleanupResumesFromCurrentPublication(t *testing.T) {
	root := t.TempDir()
	legacy(t, root, "r1", nil, "Old")
	state(t, root)
	if err := Write(filepath.Join(root, "a.story"), map[string]any{"story": "urn:change-saga:test:story:a", "title": "Published"}, map[string]any{"state": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	record, err := Read(filepath.Join(root, "a.story"))
	if err != nil || !strings.Contains(string(record.Content), "Published") {
		t.Fatal("retry replaced published content")
	}
	if _, err := os.Stat(filepath.Join(root, "a.story", "events")); !os.IsNotExist(err) {
		t.Fatal("retry did not finish cleanup")
	}
}
func TestReadNeverPersistsNormalization(t *testing.T) {
	root := t.TempDir()
	legacy(t, root, "r1", nil, "Current")
	state(t, root)
	if record, err := Read(filepath.Join(root, "a.story")); record != nil || err != nil {
		t.Fatalf("legacy read: %v %v", record, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.story", "revisions", "r1.json")); err != nil {
		t.Fatal(err)
	}
}
func TestContentPreservesNestedIdentityAndSourcePins(t *testing.T) {
	value := map[string]any{"id": "obsolete-revision", "parents": []string{"old"}, "elements": []any{map[string]any{"id": "stable", "parent": "group", "commit": strings.Repeat("a", 40), "digest": "sha256:" + strings.Repeat("b", 64)}}}
	data, err := Content(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "obsolete-revision") || !strings.Contains(string(data), `"id":"stable"`) || !strings.Contains(string(data), `"parent":"group"`) {
		t.Fatalf("identities changed: %s", data)
	}
}
func TestCurrentRecordRejectsAmbiguousJSONAndMetadata(t *testing.T) {
	for _, data := range []string{`{"a":1,"a":2}`, `{} {}`} {
		var value any
		if err := Decode([]byte(data), &value); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	root := t.TempDir()
	put(t, filepath.Join(root, Name), Record{Schema: Schema, Version: 1, Content: json.RawMessage(`{"parents":[]}`)})
	if _, err := Read(root); err == nil {
		t.Fatal("current graph metadata accepted")
	}
}

func TestNormalizeCopiesPinnedAssetsAndUpdatesTheirDigest(t *testing.T) {
	root := t.TempDir()
	legacy(t, root, "r1", nil, "Original")
	legacy(t, root, "r2", []string{"r1"}, "Current")
	state(t, root)
	original := []byte(`{"version":1,"elements":[{"id":"stable-element","criterion_links":[{"story_revision":"urn:change-saga:test:story:a:revision:r1"}]}]}`)
	oldName := "24-a-" + strings.Repeat("a", 52) + ".json"
	if err := os.WriteFile(filepath.Join(root, oldName), original, 0644); err != nil {
		t.Fatal(err)
	}
	snapshot := "sha256:" + strings.Repeat("b", 64)
	put(t, filepath.Join(root, "25-t-example.json"), map[string]any{"version": 1, "deck": "deck", "slide": "slide", "current": snapshot, "revisions": []any{map[string]any{"snapshot": snapshot, "parent_snapshots": []string{}, "request_id": "old", "created_at": "2000-01-01T00:00:00Z", "asset": "visual.svg", "asset_digest": "sha256:" + strings.Repeat("c", 64), "slide": map[string]any{"id": "slide"}, "items": []any{}, "diagram": map[string]any{"source": oldName, "source_digest": "sha256:" + strings.Repeat("d", 64), "renderer": "renderer"}}}})
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	retained, err := os.ReadFile(filepath.Join(root, oldName))
	if err != nil || string(retained) != string(original) {
		t.Fatal("migration changed original asset bytes")
	}
	data, err := os.ReadFile(filepath.Join(root, "25-t-example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	diagram := value["content"].(map[string]any)["diagram"].(map[string]any)
	name := diagram["source"].(string)
	if name == oldName {
		t.Fatal("current source still points to legacy asset")
	}
	converted, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(converted), ":revision:") || !strings.Contains(string(converted), "stable-element") {
		t.Fatalf("source conversion lost identity or pin: %s", converted)
	}
	sum := sha256.Sum256(converted)
	if diagram["source_digest"] != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatal("source digest does not pin the new asset bytes")
	}
	if err := Normalize(root); err != nil {
		t.Fatalf("cleanup retry tried to rewrite historical orphan assets: %v", err)
	}
}

func TestNormalizeRelationPinsWithoutASecondHistoryStore(t *testing.T) {
	root := t.TempDir()
	relations := filepath.Join(root, "___features", "core.feature", "___requirements", "relations")
	path := filepath.Join(relations, "explains.json")
	dir := filepath.Join(filepath.Dir(relations), "relation-repins", "explains")
	digest := "sha256:" + strings.Repeat("a", 64)
	put(t, path, map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation.schema.json", "id": "explains", "from": "urn:change-saga:test:fragment:design", "rationale": "Authored explanation", "from_content_digest": "sha256:" + strings.Repeat("b", 64)})
	put(t, filepath.Join(dir, "confirmation.json"), map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation-repin.schema.json", "id": "confirmation", "version": 5, "relation": "urn:change-saga:test:relation:explains", "created_at": "2026-09-01T00:00:00Z", "from_content_digest": digest, "rationale": "Re-read the pinned explanation."})
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), digest) || !strings.Contains(string(data), RelationSchema) || !strings.Contains(string(data), "Authored explanation") {
		t.Fatalf("current relation: %s %v", data, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("relation pin history was retained")
	}
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if string(again) != string(data) {
		t.Fatal("relation normalization is not idempotent")
	}
}

func TestNormalizeLegacyRelationFoldPreservesPartialPinsAndIDTieBreak(t *testing.T) {
	root := t.TempDir()
	relations := filepath.Join(root, "___features", "core.feature", "___requirements", "relations")
	path := filepath.Join(relations, "explains.json")
	dir := filepath.Join(filepath.Dir(relations), "relation-repins", "explains")
	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	put(t, path, map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation.schema.json", "id": "explains", "from": "urn:change-saga:test:fragment:design", "rationale": "Original authored rationale", "from_content_digest": digest("a"), "to_revision": digest("b")})
	// Lexical file order deliberately differs from chronology. Equal-time IDs
	// follow the exact tie break used by the legacy reader.
	for _, value := range []struct{ id, at, key, pin string }{
		{"z-first", "2026-09-01T00:00:00Z", "from_content_digest", digest("c")},
		{"a-second", "2026-09-02T00:00:00Z", "to_revision", digest("d")},
		{"b-second", "2026-09-02T00:00:00Z", "to_revision", digest("e")},
		{"c-partial", "2026-09-03T00:00:00Z", "from_content_digest", digest("f")},
	} {
		put(t, filepath.Join(dir, value.id+".json"), map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation-repin.schema.json", "version": 5, "id": value.id, "relation": "urn:change-saga:test:relation:explains", "rationale": "Reassessed existing pin", "created_at": value.at, value.key: value.pin})
	}
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var relation map[string]any
	if err := json.Unmarshal(data, &relation); err != nil {
		t.Fatal(err)
	}
	if relation["from_content_digest"] != digest("f") || relation["to_revision"] != digest("e") || relation["rationale"] != "Original authored rationale" {
		t.Fatalf("changed the effective legacy relation: %s", data)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("obsolete confirmations retained")
	}
	if err := Normalize(root); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(path)
	if string(data) != string(again) {
		t.Fatal("fold normalization is not idempotent")
	}
}

func TestInvalidLegacyRelationConfirmationDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	relations := filepath.Join(root, "___features", "core.feature", "___requirements", "relations")
	path := filepath.Join(relations, "explains.json")
	put(t, path, map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation.schema.json", "id": "explains", "from": "urn:change-saga:test:fragment:design", "rationale": "Original"})
	dir := filepath.Join(filepath.Dir(relations), "relation-repins", "explains")
	put(t, filepath.Join(dir, "invalid.json"), map[string]any{"$schema": "https://changesaga.dev/schema/v5/relation-repin.schema.json", "id": "invalid", "to_revision": "sha256:" + strings.Repeat("a", 64)})
	before, _ := os.ReadFile(path)
	if err := Normalize(root); err == nil || !strings.Contains(err.Error(), "invalid legacy relation confirmation") {
		t.Fatalf("invalid confirmation: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid confirmation changed the publication")
	}
	if _, err := os.Stat(filepath.Join(dir, "invalid.json")); err != nil {
		t.Fatal("invalid legacy bytes were discarded")
	}
}

func TestContentDigestCanonicalizesNestedObjects(t *testing.T) {
	typed := struct {
		Source struct {
			Z string `json:"z"`
			A string `json:"a"`
		} `json:"source"`
	}{}
	typed.Source.Z, typed.Source.A = "last", "first"
	data, err := json.Marshal(typed)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	fromTyped, _ := Digest(typed)
	fromMap, _ := Digest(decoded)
	fromRaw, _ := Digest(json.RawMessage(data))
	if fromTyped != fromMap || fromMap != fromRaw {
		t.Fatalf("digest depends on Go representation: %s %s %s", fromTyped, fromMap, fromRaw)
	}
}

func TestNormalizeChecksAllHeadsBeforeCopyingPrototypeAssets(t *testing.T) {
	root := t.TempDir()
	prototype := filepath.Join(root, "0.prototype")
	put(t, filepath.Join(prototype, "prototype.json"), map[string]any{"id": "0"})
	put(t, filepath.Join(prototype, "revisions", "r1.revision", "revision.json"), map[string]any{"$schema": "https://changesaga.dev/schema/v3/prototype-revision.schema.json", "version": 3, "id": "r1", "prototype": "urn:change-saga:test:prototype:0", "parents": []string{}, "source": map[string]any{"kind": "html", "content_digest": "sha256:" + strings.Repeat("a", 64)}})
	html := filepath.Join(prototype, "revisions", "r1.revision", "html", "index.html")
	if err := os.MkdirAll(filepath.Dir(html), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(html, []byte("<h1>Authored prototype</h1>"), 0644); err != nil {
		t.Fatal(err)
	}
	legacy(t, root, "base", nil, "Base")
	legacy(t, root, "left", []string{"base"}, "Left")
	legacy(t, root, "right", []string{"base"}, "Right")
	state(t, root)
	if err := Normalize(root); err == nil || !strings.Contains(err.Error(), "competing") {
		t.Fatalf("competing heads: %v", err)
	}
	if _, err := os.Stat(filepath.Join(prototype, "assets")); !os.IsNotExist(err) {
		t.Fatal("head preflight copied assets before rejecting ambiguity")
	}
	if data, err := os.ReadFile(html); err != nil || string(data) != "<h1>Authored prototype</h1>" {
		t.Fatal("head preflight changed legacy authored assets")
	}
}
