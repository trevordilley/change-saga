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
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
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
	if err := Feature(ctx, []string{"add", "--id", "core", "--title", "Core", root}, &out); err != nil {
		t.Fatal(err)
	}
	story := "urn:change-saga:board:story:value"
	if err := Story(ctx, []string{"add", "--feature", "core", "--id", "value", "--revision", "s1", "--event", "proposed", "--title", "Read the value", "--statement", "The reader can understand the value.", "--criterion", "exact=The value is explicit.", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := Story(ctx, []string{"set-state", "--story", story, "--event", "accepted", "--parent", story + ":event:proposed", "--state", "accepted", "--reason", "The fixture obligation is accepted.", root}, &out); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "board.json")
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "value", Kind: "node", Shape: "rect", Label: "Value", Style: "normal", Width: 240, Height: 100, Note: "[implementation](https://example.test/acme/app/blob/main/app.go#L2)", Record: story, CriterionLinks: []diagram.CriterionLink{{ID: "value-exact", Criterion: story + ":criterion:exact", StoryRevision: story + ":revision:s1", Rationale: "The exact element explains the value."}}, Evidence: []coderef.Reference{{Commit: "HEAD", Path: "app.go", Start: 2, End: 2, Note: "Value returned by the app."}}}}}
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
	out.Reset()
	if err := Whiteboard(ctx, []string{"check", "--id", "architecture", "--json", root}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"valid": true`) || !strings.Contains(out.String(), `"repository_link_in_note"`) {
		t.Fatalf("repository notes should warn without blocking: %s", out.String())
	}
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

	// Criterion links join the ordinary relation/implementation graph, and
	// meaningful wording changes require reassessment independently of code.
	status, err = buildStatus(ctx, root, repo, gitdiff.Range{}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Stale) != 0 {
		t.Fatalf("current criterion link treated as stale: %+v", status.Stale)
	}
	if err := Criterion(ctx, []string{"revise", "--story", story, "--criterion", story + ":criterion:exact", "--parent", story + ":revision:s1", "--revision", "s2", "--statement", "The value includes its unit.", root}, &out); err != nil {
		t.Fatal(err)
	}
	status, err = buildStatus(ctx, root, repo, gitdiff.Range{}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Stale) == 0 {
		t.Fatal("changed criterion did not make its element link stale")
	}
	doc.Elements[0].CriterionLinks[0].StoryRevision = story + ":revision:s2"
	publish("r3", "r2")
	status, err = buildStatus(ctx, root, repo, gitdiff.Range{}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Stale) != 0 {
		t.Fatalf("reassessed criterion link still stale: %+v", status.Stale)
	}
	assertValid(t, root)
}

func TestWhiteboardSelectionsReuseInventoryEligibilityAndStayImmutable(t *testing.T) {
	ctx := context.Background()
	f := newReviewFixture(t)
	git(t, f.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	var out bytes.Buffer
	if err := Inventory(ctx, []string{"adopt-format", "--format", "2", f.root}, &out); err != nil {
		t.Fatal(err)
	}
	h := &inventoryHarness{t: t, ctx: ctx, repo: f.repo, root: f.root, source: filepath.Join(t.TempDir(), "component.json")}
	if err := h.run("component", "add", "store", map[string]any{"name": "Store", "explanation": "The store's exact implementation.", "intent": "implemented", "code": []map[string]any{{"id": "table", "commit": "HEAD", "path": "store.go", "start": 2, "end": 3, "note": "Declares the table name."}}}, "--delivery", "HEAD"); err != nil {
		t.Fatal(err)
	}
	pin := saga.DocumentationLink{Target: "urn:change-saga:app:component:store", Revision: "urn:change-saga:app:component:store:revision:r1"}
	head := strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "store", Kind: "node", Shape: "rect", Label: "Store", Style: "normal", Width: 240, Height: 100, Documentation: &pin, Selections: []diagram.ItemSelection{{ID: "table-name", Path: []diagram.DocumentationLink{pin}, Evidence: "table", Code: coderef.Reference{Commit: head, Path: "store.go", Start: 3, End: 3, Note: "The table name."}}}}}}
	source := filepath.Join(t.TempDir(), "board.json")
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Whiteboard(ctx, []string{"publish", "--id", "architecture", "--revision", "r1", "--from", source, "--repo", f.repo, f.root}, &out); err != nil {
		t.Fatal(err)
	}
	document, _, err := saga.Load(f.root)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := coderesolve.New(ctx, f.repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	inherited := inheritedSelections(ctx, document, head, resolver)
	inventory, err := requirements.LoadInventory(f.root, "app")
	if err != nil {
		t.Fatal(err)
	}
	uses := inventoryview.Build(document, &inventory)
	if counts := uses.Counts(pin.Target); counts.Whiteboards != 1 {
		t.Fatalf("whiteboard documentation use missing: %+v", counts)
	}
	if len(uses.ImplementationUses(pin.Target).Uses) != 1 {
		t.Fatal("whiteboard use counted as unreferenced")
	}

	if len(inherited) != 1 || inherited[0].Item != requirements.WhiteboardElementTarget("app", "architecture", "store") {
		t.Fatalf("inherited selection ownership: %+v", inherited)
	}
	refs, err := sagaReferences(document)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range refs {
		if r.Kind == "whiteboard" && r.Code.Start == 3 {
			found = true
		}
	}
	if !found {
		t.Fatal("selection absent from shared references")
	}
	old, _ := requirements.LoadWhiteboard(f.root, "app", "architecture", "r1")
	// Changing bytes outside the subset still requires semantic reassessment.
	content, err := os.ReadFile(filepath.Join(f.repo, "store.go"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	lines[1] = "// Updated store implementation."
	if err := os.WriteFile(filepath.Join(f.repo, "store.go"), []byte(strings.Join(lines, "\n")), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, f.repo, "add", "store.go")
	git(t, f.repo, "commit", "-m", "Change containing evidence")
	status, err := buildStatus(ctx, f.root, f.repo, gitdiff.Range{}, false, "")
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, s := range status.Stale {
		if s.Record == inherited[0].Item {
			found = true
		}
	}
	if status.Coverage.Areas.Health.Uncovered == 0 {
		t.Fatal("selection drift did not count against health")
	}
	if !found {
		t.Fatalf("outside-subset change hidden: %+v", status.Stale)
	}
	oldAgain, _ := requirements.LoadWhiteboard(f.root, "app", "architecture", "r1")
	if oldAgain.Document.Elements[0].Selections[0].Code != old.Document.Elements[0].Selections[0].Code {
		t.Fatal("saved subset was rewritten")
	}
	// An undeclared evidence target cannot be published.
	doc.Elements[0].Selections[0].Evidence = "missing"
	data, _ = json.Marshal(doc)
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Whiteboard(ctx, []string{"publish", "--id", "architecture", "--revision", "r2", "--parent", "r1", "--from", source, "--repo", f.repo, f.root}, &out); err == nil {
		t.Fatal("missing evidence selection published")
	}
}
