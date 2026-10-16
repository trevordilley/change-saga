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
		return out.String(), err
	}
	if _, err := run("publish", "--id", "data-model", "--revision", "b1", "--name", "Work board", "--from", path); err != nil {
		t.Fatal(err)
	}
	described, err := run("describe", "--id", "data-model")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Work board", "cloud/local", "Local-first editing"} {
		if !strings.Contains(described, want) {
			t.Fatalf("description missing %s: %s", want, described)
		}
	}
	focused, err := run("describe", "--id", "data-model", "--focus", "cloud/local/local", "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(focused, "Offline edits retain stable IDs") || !strings.Contains(focused, definition.Directory[0].Revision) {
		t.Fatalf("focused source: %s", focused)
	}
	got, err := run("get", "--id", "data-model")
	if err != nil {
		t.Fatal(err)
	}
	var fetched diagram.Document
	if err := json.Unmarshal([]byte(got), &fetched); err != nil || len(fetched.Elements) != 3 {
		t.Fatalf("get roundtrip: %v %s", err, got)
	}
	checked, err := run("check", "--id", "data-model", "--json")
	if err != nil || !strings.Contains(checked, `"valid": true`) {
		t.Fatalf("check: %v %s", err, checked)
	}
	if _, err := run("describe", "--id", "data-model", "--focus", "cloud/missing"); err == nil {
		t.Fatal("unknown focus accepted")
	}
	if _, err := run("publish", "--id", "data-model", "--revision", "b2", "--from", path); err == nil {
		t.Fatal("update without observed parent accepted")
	}
	if _, err := run("publish", "--id", "data-model", "--revision", "b2", "--parent", "b1", "--from", path); err != nil {
		t.Fatal(err)
	}
	if _, err := run("get", "--id", "data-model", "--revision", "b1", "--focus", "cloud/local/local"); err != nil {
		t.Fatal(err)
	}
	if _, err := run("publish", "--id", "deployment-topology", "--revision", "b1", "--from", path); err != nil {
		t.Fatal(err)
	}
	if other, err := run("describe", "--id", "deployment-topology", "--format", "json"); err != nil || !strings.Contains(other, `"whiteboard": "deployment-topology"`) {
		t.Fatalf("generic document identity: %v %s", err, other)
	}
	if _, err := run("publish", "--id", "deployment-topology", "--revision", "b3", "--parent", "b2", "--from", path); err == nil {
		t.Fatal("parent from a different document accepted")
	}
	assertValid(t, root)
	// Validation must not ignore a broken historical pin just because the latest
	// board is valid and no reader has focused the historical frame.
	historical := filepath.Join(root, requirements.WhiteboardDir, "data-model", "revisions", "b1.json")
	data, _ := os.ReadFile(historical)
	data = bytes.Replace(data, []byte(pins["postgres"].Revision), []byte(pins["postgres"].Target+":revision:missing"), 1)
	if err := os.WriteFile(historical, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run("check", "--id", "deployment-topology"); err != nil {
		t.Fatalf("selected document check leaked sibling history: %v", err)
	}
	if _, err := run("check", "--id", "data-model"); err == nil {
		t.Fatal("selected board check ignored broken historical pin")
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
		{"missing id", []string{"get", "missing.saga"}, "requires --id"},
		{"removed kind", []string{"get", "--kind=erd", "missing.saga"}, "flag provided but not defined"},
		{"path traversal", []string{"get", "--id=../outside", "missing.saga"}, "requires --id"},
		{"unsupported edit", []string{"edit", "--id", "data-model", "missing.saga"}, "whiteboards support publish, get, describe, check"},
		{"unknown operation", []string{"delete", "--id", "data-model", "missing.saga"}, "usage: change-saga whiteboard"},
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

func TestDiagramDoesNotExposeUnreleasedWhiteboardAlias(t *testing.T) {
	for _, args := range [][]string{
		{"publish", "--whiteboard", "erd", "missing.saga"},
		{"describe", "--whiteboard", "erd", "missing.saga"},
		{"get", "--whiteboard", "erd", "missing.saga"},
		{"check", "--whiteboard", "erd", "missing.saga"},
	} {
		var out bytes.Buffer
		if err := Diagram(context.Background(), args, &out); err == nil {
			t.Fatalf("removed alias accepted: %v", args)
		}
	}
}

func TestWhiteboardMetadataCLIDryRunDiscoveryAndLifecycle(t *testing.T) {
	ctx := context.Background()
	repo, _ := sourceRepo(t, map[string]string{"README.md": "app\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "atomic.saga")
	var out bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &out); err != nil {
		t.Fatal(err)
	}
	doc := diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "item", Kind: "node", Shape: "rect", Label: "Source", X: 10, Y: 10, Width: 200, Height: 100, Style: "normal"}}}
	for _, id := range []string{"data-model", "existing-model"} {
		if _, err := requirements.PublishWhiteboard(ctx, root, "atomic", id, "r1", "", "Original", doc); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) string {
		t.Helper()
		out.Reset()
		if err := Whiteboard(ctx, append(args, root), &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	before := run("get", "--id", "existing-model")
	plan := run("assign-role", "--id", "existing-model", "--role", "erd", "--expect", "data-model", "--dry-run", "--json")
	if !strings.Contains(plan, `"dry_run": true`) {
		t.Fatal(plan)
	}
	if _, err := os.Stat(filepath.Join(root, requirements.WhiteboardDir, requirements.WhiteboardMetadataName)); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote metadata")
	}
	run("assign-role", "--id", "existing-model", "--role", "erd", "--expect", "data-model", "--json")
	run("rename", "--id", "existing-model", "--name", "Renamed board", "--json")
	listed := run("list", "--json")
	for _, want := range []string{`"erd": "existing-model"`, `"role": "erd"`, `"name": "Renamed board"`, `"snapshot":`, `urn:change-saga:atomic:whiteboard:existing-model`} {
		if !strings.Contains(listed, want) {
			t.Fatalf("list missing %s: %s", want, listed)
		}
	}
	if got := run("describe", "--id", "existing-model", "--format", "json"); !strings.Contains(got, `"name": "Renamed board"`) {
		t.Fatal(got)
	}
	if got := run("describe", "--id", "existing-model", "--revision", "r1", "--format", "json"); !strings.Contains(got, `"name": "Original"`) {
		t.Fatal("rename changed historical title", got)
	}
	if err := Whiteboard(ctx, []string{"archive", "--id", "existing-model", root}, &out); err == nil {
		t.Fatal("archived assigned board")
	}
	run("unassign-role", "--id", "existing-model", "--role", "erd", "--json")
	run("archive", "--id", "existing-model", "--json")
	if listed := run("list", "--json"); !strings.Contains(listed, `"archived": true`) {
		t.Fatal(listed)
	}
	if after := run("get", "--id", "existing-model"); after != before {
		t.Fatal("metadata rewrote source")
	}
	run("restore", "--id", "existing-model")
	out.Reset()
	if err := Whiteboard(ctx, []string{"assign-role", "--id", "existing-model", "--role", "erd", "--expect", "wrong", "--json", root}, &out); err == nil || !strings.Contains(out.String(), `"ok": false`) {
		t.Fatalf("missing structured failure: %v %s", err, out.String())
	}
	out.Reset()
	if err := Query(ctx, []string{"whiteboards", "--saga", root, "--limit", "1"}, &out); err != nil {
		t.Fatal(err)
	}
	var first struct {
		OK   bool              `json:"ok"`
		Data whiteboardList    `json:"data"`
		Page queryPageEnvelope `json:"page"`
	}
	if err := json.Unmarshal(out.Bytes(), &first); err != nil || !first.OK || first.Page.Total != 2 || first.Page.NextCursor == nil || len(first.Data.Whiteboards) != 1 {
		t.Fatalf("first query page: %s %v", out.String(), err)
	}
	cursor := *first.Page.NextCursor
	out.Reset()
	if err := Query(ctx, []string{"whiteboards", "--saga", root, "--limit", "1", "--cursor", cursor}, &out); err != nil {
		t.Fatal(err)
	}
	var second struct {
		Data whiteboardList    `json:"data"`
		Page queryPageEnvelope `json:"page"`
	}
	if err := json.Unmarshal(out.Bytes(), &second); err != nil || second.Page.HasMore || len(second.Data.Whiteboards) != 1 || second.Data.Whiteboards[0].ID == first.Data.Whiteboards[0].ID {
		t.Fatalf("second query page: %s %v", out.String(), err)
	}
	run("rename", "--id", "existing-model", "--name", "Another title")
	out.Reset()
	if err := Query(ctx, []string{"whiteboards", "--saga", root, "--cursor", cursor}, &out); err == nil || !strings.Contains(out.String(), `"stale_snapshot"`) {
		t.Fatalf("stale query cursor accepted: %v %s", err, out.String())
	}
	run("check", "--id", "existing-model", "--json")
	assertValid(t, root)
}

func TestQueryWhiteboardsBoundsCursorBeforeLoadingSaga(t *testing.T) {
	var out bytes.Buffer
	err := Query(context.Background(), []string{"whiteboards", "--saga", "missing.saga", "--cursor", strings.Repeat("x", 4097)}, &out)
	if err == nil || !strings.Contains(out.String(), `"invalid_argument"`) || !strings.Contains(out.String(), "4096") {
		t.Fatalf("cursor bound: %v %s", err, out.String())
	}
}
