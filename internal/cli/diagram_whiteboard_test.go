package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestWhiteboardCLIAndStructuredERDAuthoring(t *testing.T) {
	ctx := context.Background()
	repo, _ := sourceRepo(t, map[string]string{"README.md": "app\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "atomic.saga")
	var out bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &out); err != nil {
		t.Fatal(err)
	}
	pins, err := testfixture.WriteMultiERDFixture(ctx, root, repo)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := requirements.LoadInventory(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	definition := inventory.Pinned(pins["sqlite"]).TechnicalDefinition
	definition.Visual = nil
	definition.Bindings = []requirements.Binding{{ID: "local", Element: "local", Entity: &definition.Directory[0]}}
	definition.Diagram = &diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "local", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 150, Style: "normal", Label: "Local edits", Note: "Offline edits retain stable IDs."}}}
	write := func(name string, value any) string {
		t.Helper()
		data, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		path := filepath.Join(t.TempDir(), name)
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
		return path
	}
	definitionPath := write("definition.json", definition)
	if err := Technical(ctx, "erd", []string{"revise", "--id", "sqlite", "--revision", "r2", "--parent", pins["sqlite"].Revision, "--from", definitionPath, "--repo", repo, root}, &out); err != nil {
		t.Fatal(err)
	}
	doc := diagram.Document{Version: 1, Width: 1600, Height: 900, Elements: []diagram.Element{
		{ID: "cloud", Kind: "group", X: 30, Y: 30, Width: 1500, Height: 800, Style: "boundary", Label: "Cloud"},
		{ID: "local", Parent: "cloud", Kind: "frame", X: 30, Y: 60, Width: 600, Height: 450, Style: "normal", Label: "Offline", Summary: "Local-first editing", Source: &diagram.SourceReference{Target: pins["sqlite"].Target, Revision: pins["sqlite"].Target + ":revision:r2"}},
		{ID: "remote", Parent: "cloud", Kind: "frame", X: 750, Y: 60, Width: 600, Height: 450, Style: "normal", Label: "Server", Source: &diagram.SourceReference{Target: pins["postgres"].Target, Revision: pins["postgres"].Revision}},
	}}
	path := write("board.json", doc)
	run := func(args ...string) (string, error) {
		out.Reset()
		err := Diagram(ctx, append(args, root), &out)
		return out.String(), err
	}
	if _, err := run("publish", "--whiteboard", "erd", "--revision", "b1", "--name", "Work board", "--from", path); err != nil {
		t.Fatal(err)
	}
	described, err := run("describe", "--whiteboard", "erd")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Work board", "cloud/local", "Local-first editing"} {
		if !strings.Contains(described, want) {
			t.Fatalf("description missing %s: %s", want, described)
		}
	}
	focused, err := run("describe", "--whiteboard", "erd", "--focus", "cloud/local/local", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(focused, "Offline edits retain stable IDs") || !strings.Contains(focused, definition.Directory[0].Revision) {
		t.Fatalf("focused source: %s", focused)
	}
	got, err := run("get", "--whiteboard", "erd")
	if err != nil {
		t.Fatal(err)
	}
	var fetched diagram.Document
	if err := json.Unmarshal([]byte(got), &fetched); err != nil || len(fetched.Elements) != 3 {
		t.Fatalf("get roundtrip: %v %s", err, got)
	}
	checked, err := run("check", "--whiteboard", "erd", "--json")
	if err != nil || !strings.Contains(checked, `"valid": true`) {
		t.Fatalf("check: %v %s", err, checked)
	}
	if _, err := run("describe", "--whiteboard", "erd", "--focus", "cloud/missing"); err == nil {
		t.Fatal("unknown focus accepted")
	}
	if _, err := run("publish", "--whiteboard", "erd", "--revision", "b2", "--from", path); err == nil {
		t.Fatal("update without observed parent accepted")
	}
	if _, err := run("publish", "--whiteboard", "erd", "--revision", "b2", "--parent", "b1", "--from", path); err != nil {
		t.Fatal(err)
	}
	if _, err := run("get", "--whiteboard", "erd", "--revision", "b1", "--focus", "cloud/local/local"); err != nil {
		t.Fatal(err)
	}
	assertValid(t, root)
	// Validation must not ignore a broken historical pin just because the latest
	// board is valid and no reader has focused the historical frame.
	historical := filepath.Join(root, requirements.WhiteboardDir, "erd", "revisions", "b1.json")
	data, _ := os.ReadFile(historical)
	data = bytes.Replace(data, []byte(pins["postgres"].Revision), []byte(pins["postgres"].Target+":revision:missing"), 1)
	if err := os.WriteFile(historical, data, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Validate(ctx, []string{"--json", root}, &out); err == nil || !strings.Contains(out.String(), requirements.WhiteboardDir) {
		t.Fatalf("general validation ignored broken history: %v %s", err, out.String())
	}
}
