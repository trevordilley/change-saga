package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestWhiteboardEvidenceCurrencyAndReassessment(t *testing.T) {
	ctx := context.Background()
	repo, base := sourceRepo(t, map[string]string{"app.go": "package app\nfunc Value() int { return 1 }\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "board.saga")
	var out bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &out); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "board.json")
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "value", Kind: "node", Shape: "rect", Label: "Value", Style: "normal", Width: 240, Height: 100, Evidence: []coderef.Reference{{Commit: "HEAD", Path: "app.go", Start: 2, End: 2, Note: "Value returned by the app."}}}}}
	publish := func(rev, parent string) {
		t.Helper()
		data, _ := json.Marshal(doc)
		if err := os.WriteFile(input, data, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"publish", "--id", "architecture", "--revision", rev, "--from", input, "--repo", repo}
		if parent != "" {
			args = append(args, "--parent", parent)
		}
		out.Reset()
		if err := Whiteboard(ctx, append(args, root), &out); err != nil {
			t.Fatal(err)
		}
	}
	publish("r1", "")
	old, err := requirements.LoadWhiteboard(root, "board", "architecture", "r1")
	if err != nil {
		t.Fatal(err)
	}
	if got := old.Document.Elements[0].Evidence[0]; got.Commit != base || got.Digest == "" {
		t.Fatalf("not pinned: %+v", got)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := sagaReferences(document)
	if err != nil || len(refs) != 1 || refs[0].Kind != "whiteboard" {
		t.Fatalf("refs %v %v", refs, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "app.go"), []byte("package app\nfunc Value() int { return 2 }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", "app.go")
	git(t, repo, "commit", "-m", "Change value")
	out.Reset()
	if err := References(ctx, []string{"--json", "--against", base, root}, &out); err != nil {
		t.Fatal(err)
	}
	var refResult referencesOutput
	if err := json.Unmarshal(out.Bytes(), &refResult); err != nil {
		t.Fatal(err)
	}
	if refResult.Stale != 1 {
		t.Fatalf("base match hid HEAD staleness: %s", out.String())
	}
	status, err := buildStatus(ctx, root, repo, gitdiff.Range{}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Stale) == 0 || status.Coverage.Areas.Health.Uncovered == 0 {
		t.Fatalf("staleness missing from health: %+v", status)
	}
	report, err := buildReconciliation(ctx, root, repo, gitdiff.Range{Against: base}, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.StaleByChange.Count != 1 {
		t.Fatalf("reconcile lost reference: %+v", report.Summary)
	}
	found := false
	for _, task := range report.Queue {
		if strings.Contains(task.Resource, ":whiteboard:") && strings.Contains(task.Guidance, "whiteboard publish") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing publication route: %+v", report.Queue)
	}
	// Explicit re-assessment publishes new pins; the previous revision stays exact.
	publish("r2", "r1")
	oldAgain, _ := requirements.LoadWhiteboard(root, "board", "architecture", "r1")
	if oldAgain.Document.Elements[0].Evidence[0] != old.Document.Elements[0].Evidence[0] {
		t.Fatal("history was rewritten")
	}
	out.Reset()
	if err := References(ctx, []string{"--json", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &refResult); err != nil {
		t.Fatal(err)
	}
	if refResult.Stale != 0 || len(refResult.References) != 1 {
		t.Fatalf("reassessed head not current: %s", out.String())
	}
	assertValid(t, root)
}
