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
		err := Whiteboard(ctx, append(args, root), &out)
		primary := out.String()
		// Both command families must address the same records and produce the
		// same output. Retrying publication also exercises cross-alias idempotence.
		legacy := append([]string{}, args...)
		for i, arg := range legacy {
			if arg == "--kind" {
				legacy[i] = "--whiteboard"
			}
		}
		out.Reset()
		aliasErr := Diagram(ctx, append(legacy, root), &out)
		if (err == nil) != (aliasErr == nil) || err == nil && primary != out.String() {
			t.Fatalf("command aliases differ for %v: primary %v %s; alias %v %s", args, err, primary, aliasErr, out.String())
		}
		return primary, err
	}
	if _, err := run("publish", "--kind", "erd", "--revision", "b1", "--name", "Work board", "--from", path); err != nil {
		t.Fatal(err)
	}
	described, err := run("describe", "--kind", "erd")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Work board", "cloud/local", "Local-first editing"} {
		if !strings.Contains(described, want) {
			t.Fatalf("description missing %s: %s", want, described)
		}
	}
	focused, err := run("describe", "--kind", "erd", "--focus", "cloud/local/local", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(focused, "Offline edits retain stable IDs") || !strings.Contains(focused, definition.Directory[0].Revision) {
		t.Fatalf("focused source: %s", focused)
	}
	got, err := run("get", "--kind", "erd")
	if err != nil {
		t.Fatal(err)
	}
	var fetched diagram.Document
	if err := json.Unmarshal([]byte(got), &fetched); err != nil || len(fetched.Elements) != 3 {
		t.Fatalf("get roundtrip: %v %s", err, got)
	}
	checked, err := run("check", "--kind", "erd", "--json")
	if err != nil || !strings.Contains(checked, `"valid": true`) {
		t.Fatalf("check: %v %s", err, checked)
	}
	if _, err := run("describe", "--kind", "erd", "--focus", "cloud/missing"); err == nil {
		t.Fatal("unknown focus accepted")
	}
	if _, err := run("publish", "--kind", "erd", "--revision", "b2", "--from", path); err == nil {
		t.Fatal("update without observed parent accepted")
	}
	if _, err := run("publish", "--kind", "erd", "--revision", "b2", "--parent", "b1", "--from", path); err != nil {
		t.Fatal(err)
	}
	if _, err := run("get", "--kind", "erd", "--revision", "b1", "--focus", "cloud/local/local"); err != nil {
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

func TestWhiteboardCLIRejectsInvalidCommandBeforeLoadingSaga(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{"missing kind", []string{"get", "missing.saga"}, "requires --kind erd"},
		{"future kind", []string{"get", "--kind=infra", "missing.saga"}, "requires --kind erd"},
		{"unsupported edit", []string{"edit", "--kind", "erd", "missing.saga"}, "whiteboards support publish, get, describe and check"},
		{"unknown operation", []string{"delete", "--kind", "erd", "missing.saga"}, "usage: change-saga whiteboard"},
		{"old selector in new family", []string{"get", "--whiteboard", "erd", "missing.saga"}, "flag provided but not defined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			err := Whiteboard(context.Background(), test.args, &out)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
