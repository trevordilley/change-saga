package requirements

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/store"
)

func whiteboardFixture(t *testing.T) (string, DocumentationLink, DocumentationLink) {
	t.Helper()
	root := newSaga(t)
	adoptRaw(t, root, 2)
	entity := pinOf(rawTechnical(t, root, KindDataEntity, "job", TechnicalDefinition{Name: "Job", Explanation: "Queued work", Intent: IntentProposed, Baseline: BaselineNone}), "r1")
	child := diagram.Document{Version: 1, Width: 400, Height: 200, Elements: []diagram.Element{{ID: "job", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 180, Height: 100, Style: "normal", Label: "Job", Note: "Original detail"}}}
	def := TechnicalDefinition{Name: "Jobs", Explanation: "Job directory", Directory: []DocumentationLink{entity}, Bindings: []Binding{{ID: "job", Element: "job", Entity: &entity}}, Diagram: &child}
	if _, err := WriteTechnicalRevision(context.Background(), root, "test", TechnicalWrite{Kind: KindERD, ID: "jobs", RevisionID: "r1", Definition: def, Create: true}); err != nil {
		t.Fatal(err)
	}
	target, _ := TechnicalURN("test", KindERD, "jobs")
	return root, pinOf(target, "r1"), entity
}

func boardDocument(pin DocumentationLink) diagram.Document {
	return diagram.Document{Version: 1, Width: 1200, Height: 600, Elements: []diagram.Element{
		{ID: "cloud", Kind: "group", X: 40, Y: 40, Width: 1100, Height: 500, Style: "boundary", Label: "Cloud"},
		{ID: "jobs", Parent: "cloud", Kind: "frame", X: 30, Y: 50, Width: 500, Height: 300, Style: "normal", Label: "Jobs", Summary: "Queued work lives here", Source: &diagram.SourceReference{Target: pin.Target, Revision: pin.Revision}},
	}}
}

func TestWhiteboardPinnedHistoryAndStructuredERDCompatibility(t *testing.T) {
	root, pin, entity := whiteboardFixture(t)
	inventory, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	before := inventory.Pinned(pin)
	beforeJSON, _ := json.Marshal(before)
	oldSVG, err := os.ReadFile(filepath.Join(root, TechnicalPath(KindERD, "jobs"), before.Visual.Path))
	if err != nil {
		t.Fatal(err)
	}
	board := boardDocument(pin)
	published, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b1", "", "Work", board)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b1", "", "Work", board)
	if err != nil || !replay.CreatedAt.Equal(published.CreatedAt) {
		t.Fatalf("retry: %v", err)
	}
	loaded, err := LoadResolvedWhiteboard(root, "test", "erd", "")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := loaded.Composition.Describe("cloud/jobs/job")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(detail)
	if !bytes.Contains(encoded, []byte(entity.Revision)) || !bytes.Contains(encoded, []byte("Original detail")) {
		t.Fatalf("missing bound detail: %s", encoded)
	}
	def := before.TechnicalDefinition
	doc := *def.Diagram
	doc.Elements = append([]diagram.Element(nil), doc.Elements...)
	doc.Elements[0].Note = "New detail"
	def.Diagram = &doc
	if _, err := WriteTechnicalRevision(context.Background(), root, "test", TechnicalWrite{Kind: KindERD, ID: "jobs", RevisionID: "r2", Parents: []string{pin.Revision}, Definition: def}); err != nil {
		t.Fatal(err)
	}
	unchanged, err := LoadResolvedWhiteboard(root, "test", "erd", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded.SVG, unchanged.SVG) {
		t.Fatal("board silently followed newer child")
	}
	next := boardDocument(pinOf(pin.Target, "r2"))
	if _, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b2", "b1", "Work", next); err != nil {
		t.Fatal(err)
	}
	historical, err := LoadResolvedWhiteboard(root, "test", "erd", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(historical.SVG, loaded.SVG) {
		t.Fatal("historical board changed")
	}
	inventory, err = LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, _ := json.Marshal(inventory.Pinned(pin))
	afterSVG, _ := os.ReadFile(filepath.Join(root, TechnicalPath(KindERD, "jobs"), before.Visual.Path))
	if !bytes.Equal(beforeJSON, afterJSON) || !bytes.Equal(oldSVG, afterSVG) {
		t.Fatal("publishing rewrote ERD history")
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		id, parent string
		doc        diagram.Document
	}{{"b3", "b1", next}, {"b2", "b1", board}, {"../bad", "b2", board}} {
		if _, err := PublishWhiteboard(context.Background(), root, "test", "erd", test.id, test.parent, "Work", test.doc); err == nil {
			t.Fatalf("accepted invalid update %s", test.id)
		}
	}
}

func TestWhiteboardRejectsUnresolvedSourcesWithoutPublishing(t *testing.T) {
	root, pin, entity := whiteboardFixture(t)
	for _, test := range []struct {
		name   string
		source diagram.SourceReference
		kind   string
	}{
		{"missing revision", diagram.SourceReference{Target: pin.Target, Revision: pin.Target + ":revision:missing"}, "frame"},
		{"unpinned", diagram.SourceReference{Target: pin.Target}, "frame"},
		{"entity frame", diagram.SourceReference{Target: entity.Target, Revision: entity.Revision}, "frame"},
		{"missing relationship", diagram.SourceReference{Target: entity.Target, Revision: entity.Revision, Relationship: "absent"}, "text"},
		{"missing element", diagram.SourceReference{Target: pin.Target, Revision: pin.Revision, Element: "absent"}, "text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := boardDocument(pin)
			doc.Elements[1].Source = &test.source
			doc.Elements[1].Kind = test.kind
			if _, err := PublishWhiteboard(context.Background(), root, "test", "erd", "bad", "", "Work", doc); err == nil {
				t.Fatal("accepted invalid source")
			}
			if _, err := os.Stat(filepath.Join(root, WhiteboardDir)); !os.IsNotExist(err) {
				t.Fatalf("invalid publication wrote storage: %v", err)
			}
		})
	}
	if _, err := LoadResolvedWhiteboard(root, "test", "erd", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWhiteboard(root, "test", "erd", "absent"); err == nil {
		t.Fatal("missing explicit revision accepted")
	}
}

func TestWhiteboardOpaqueBindingsAndNestedERD(t *testing.T) {
	root, _, entity := whiteboardFixture(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 400 200"><g id="Job_1"><rect width="100" height="100"/><text x="20" y="20">Job</text></g></svg>`)
	sum := sha256.Sum256(svg)
	digest := hex.EncodeToString(sum[:])
	def := TechnicalDefinition{Name: "Opaque", Explanation: "Imported ERD", Directory: []DocumentationLink{entity}, Bindings: []Binding{{ID: "job", Element: "Job_1", Entity: &entity}}, Visual: &Visual{Path: "assets/" + digest + ".svg", Digest: "sha256:" + digest, MediaType: "image/svg+xml"}}
	if _, err := WriteTechnicalRevision(context.Background(), root, "test", TechnicalWrite{Kind: KindERD, ID: "opaque", RevisionID: "r1", Definition: def, Visual: svg, Create: true}); err != nil {
		t.Fatal(err)
	}
	target, _ := TechnicalURN("test", KindERD, "opaque")
	doc := boardDocument(pinOf(target, "r1"))
	if _, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b1", "", "Opaque", doc); err != nil {
		t.Fatal(err)
	}
	board, err := LoadResolvedWhiteboard(root, "test", "erd", "")
	if err != nil {
		t.Fatal(err)
	}
	focused, err := board.Composition.Describe("cloud/jobs/Job_1")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(focused)
	if !bytes.Contains(data, []byte(entity.Revision)) {
		t.Fatalf("missing opaque binding: %s", data)
	}
	// A structured ERD can itself embed an existing opaque ERD and remain
	// independently readable as a saved SVG with immutable source pins.
	def = TechnicalDefinition{Name: "Nested", Explanation: "Nested ERD", Directory: []DocumentationLink{entity}, Diagram: &doc}
	if _, err := WriteTechnicalRevision(context.Background(), root, "test", TechnicalWrite{Kind: KindERD, ID: "nested", RevisionID: "r1", Definition: def, Create: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInventory(root, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardRejectsMalformedHistoryAndSymlink(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	board := boardDocument(pin)
	v, err := PublishWhiteboard(context.Background(), root, "test", "erd", "b1", "", "Work", board)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, WhiteboardDir, "erd", "revisions", "b1.json")
	original, _ := os.ReadFile(path)
	for _, bad := range []string{strings.Replace(string(original), `"version": 1`, `"version": 1, "version": 1`, 1), strings.Replace(string(original), `"parents": []`, `"parents": ["missing"]`, 1)} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateWhiteboards(root, "test"); err == nil {
			t.Fatal("accepted malformed history")
		}
	}
	if err := store.WriteJSON(path, v, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "b1.json")
	if err := os.WriteFile(outside, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboards(root, "test"); err == nil {
		t.Fatal("accepted symlink revision")
	}
}

func TestVisualEmbeddedSVGIsOfflineAndRecursivelySafe(t *testing.T) {
	embed := func(svg string) []byte {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg"><image href="data:image/svg+xml;base64,` + base64.StdEncoding.EncodeToString([]byte(svg)) + `"/></svg>`)
	}
	if err := ValidateVisual(embed(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`), "", nil); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`<svg xmlns="http://www.w3.org/2000/svg"><script>evil()</script></svg>`, `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.com/a.svg"/></svg>`, `<svg xmlns="http://www.w3.org/2000/svg"><g onclick="evil()"/></svg>`} {
		if err := ValidateVisual(embed(bad), "", nil); err == nil {
			t.Fatal("accepted unsafe embedded SVG")
		}
	}
}
