package requirements

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewVisualStoryLinksRequireItemsButLegacyLinksRemainReadable(t *testing.T) {
	root := newV5Saga(t)
	for _, kind := range []string{"deck", "slide"} {
		for _, relationType := range []RelationType{RelationExplains, RelationAddresses} {
			input := AddRelationInput{Feature: "core", ID: kind + "-" + string(relationType), Type: relationType,
				From: "urn:change-saga:test:" + kind + ":flow", To: v5Story, ToRevision: v5StoryR1,
				FromContentDigest: "sha256:" + strings.Repeat("a", 64), Rationale: "Broad legacy link."}
			if _, err := AddRelation(root, "test", input); err == nil || !strings.Contains(err.Error(), "slide Item") {
				t.Fatalf("%s %s: %v", kind, relationType, err)
			}
		}
	}
	if document, err := Load(root, "test"); err != nil || len(document.Relations) != 0 {
		t.Fatalf("rejected broad links changed the document: %#v, %v", document.Relations, err)
	}
	// Historical v5 records keep their frozen format and can be replayed
	// idempotently, but cannot be used to author another broad link.
	legacy := Relation{Schema: V5RelationSchemaURL, Version: V5RelationVersion, ID: "legacy", Type: RelationExplains,
		From: "urn:change-saga:test:slide:flow", To: v5Story, ToRevision: v5StoryR1, Scope: ScopeSelf,
		Rationale: "Broad legacy link.", State: RelationActive, CreatedAt: testTime, RequestID: "original", Feature: "core"}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "___features", "core.feature", "___requirements", "relations")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, "test"); err != nil {
		t.Fatalf("legacy read: %v", err)
	}
	result, err := AddRelation(root, "test", AddRelationInput{Feature: "core", ID: "legacy", Type: RelationExplains,
		From: legacy.From, To: legacy.To, ToRevision: legacy.ToRevision, Rationale: legacy.Rationale, RequestID: "original"})
	if err != nil || !result.Replayed {
		t.Fatalf("legacy replay = %#v, %v", result, err)
	}
	for _, relationType := range []RelationType{RelationExplains, RelationAddresses} {
		_, err := AddRelation(root, "test", AddRelationInput{Feature: "core", ID: "item-" + string(relationType), Type: relationType,
			From: "urn:change-saga:test:slide:flow:item:node", To: v5Criterion, ToRevision: v5StoryR1,
			FromContentDigest: "sha256:" + strings.Repeat("a", 64), Rationale: "This element owns the requirement."})
		if err != nil {
			t.Fatalf("Item %s: %v", relationType, err)
		}
	}
}
