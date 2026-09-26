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
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestInventoryItemSelectionsAndSavedViews(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newReviewFixture(t)
	git(t, f.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	h := &inventoryHarness{t: t, ctx: ctx, repo: f.repo, root: f.root, source: filepath.Join(t.TempDir(), "definition.json")}
	var out bytes.Buffer
	if err := Inventory(ctx, []string{"adopt-format", "--format", "2", f.root}, &out); err != nil {
		t.Fatal(err)
	}
	urn := "urn:change-saga:app:"
	pin := func(kind, id, rev string) saga.DocumentationLink {
		return saga.DocumentationLink{Target: urn + kind + ":" + id, Revision: urn + kind + ":" + id + ":revision:" + rev}
	}
	code := func(id, path string, start, end int) []map[string]any {
		return []map[string]any{{"id": id, "commit": "HEAD", "path": path, "start": start, "end": end, "note": "Exact behavior."}}
	}
	for _, c := range []struct{ id, ev, path string }{{"store", "table", "store.go"}, {"queue", "enqueue", "queue.go"}} {
		if err := h.run("component", "add", c.id, map[string]any{"name": c.id, "explanation": "Implemented " + c.id + ".", "intent": "implemented", "code": code(c.ev, c.path, 2, 3)}, "--delivery", "HEAD"); err != nil {
			t.Fatalf("%s: %v %s", c.id, err, h.out.String())
		}
	}
	system := map[string]any{"name": "Storage", "explanation": "Queue writes into the store.", "intent": "implemented", "code": code("wiring", "queue.go", 3, 3),
		"components":   []saga.DocumentationLink{pin("component", "store", "r1"), pin("component", "queue", "r1")},
		"interactions": []map[string]any{{"id": "write", "from": urn + "component:queue", "to": urn + "component:store", "description": "Will write jobs.", "intent": "proposed"}}}
	if err := h.run("system", "add", "storage", system, "--delivery", "HEAD"); err != nil {
		t.Fatalf("system: %v %s", err, h.out.String())
	}
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-m", "inventory r1")
	viewOne := strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))
	evidenceCommit := strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD^"))

	selectionsFile := filepath.Join(t.TempDir(), "selections.json")
	writeSelections := func(path []saga.DocumentationLink, evidence string, start, end int, digest string) {
		t.Helper()
		selection := map[string]any{"id": "table-fn", "path": path, "evidence": evidence, "code": map[string]any{"path": "store.go", "start": start, "end": end, "note": "Only the table name matters here."}}
		if digest != "" {
			selection["code"].(map[string]any)["digest"] = digest
		}
		data, _ := json.Marshal([]any{selection})
		if err := os.WriteFile(selectionsFile, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sysPath := []saga.DocumentationLink{pin("system", "storage", "r1"), pin("component", "store", "r1")}
	addReviewItem := func(id string, doc saga.DocumentationLink, extra ...string) error {
		out.Reset()
		args := []string{"--review", "pr-7", "--slide", "queue", "--id", id, "--kind", "node", "--element-id", "node", "--description", "Uses shared storage.", "--documentation", doc.Target, "--documentation-revision", doc.Revision, "--repo", f.repo}
		return AddItem(ctx, append(append(args, extra...), f.root), &out)
	}
	refusals := map[string]struct {
		path     []saga.DocumentationLink
		evidence string
		start    int
		end      int
		digest   string
		want     string
	}{
		"outside":    {sysPath, "table", 1, 3, "", "outside_evidence"},
		"evidence":   {sysPath, "enqueue", 3, 3, "", "missing_evidence"},
		"missing":    {[]saga.DocumentationLink{pin("system", "storage", "r1"), pin("component", "store", "r9")}, "table", 3, 3, "", "missing_pin"},
		"undeclared": {[]saga.DocumentationLink{pin("system", "storage", "r1"), pin("system", "storage", "r1")}, "wiring", 3, 3, "", "undeclared_hop"},
		"digest":     {sysPath, "table", 3, 3, "sha256:" + strings.Repeat("0", 64), "selected_digest_mismatch"},
	}
	for name, c := range refusals {
		writeSelections(c.path, c.evidence, c.start, c.end, c.digest)
		if err := addReviewItem("refused-"+name, pin("system", "storage", "r1"), "--selections", selectionsFile); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %s", name, err, c.want)
		}
	}
	writeSelections(sysPath, "table", 3, 3, "")
	if err := addReviewItem("storage-use", pin("system", "storage", "r1"), "--selections", selectionsFile); err != nil {
		t.Fatalf("review selection: %v %s", err, out.String())
	}
	item := findTestItem(t, f.root, "storage-use")
	if len(item.Selections) != 1 || item.Selections[0].Code.Commit != evidenceCommit || !strings.HasPrefix(item.Selections[0].Code.Digest, "sha256:") || item.Selections[0].Code.Start != 3 {
		t.Fatalf("selection not authored exactly: %+v", item.Selections)
	}

	// A definition successor never rewrites the saved pin or selection.
	if err := h.run("component", "revise", "store", map[string]any{"name": "store", "explanation": "Implemented store with jobs.", "intent": "implemented", "code": code("table", "store.go", 2, 3)},
		"--revision", "r2", "--parent", urn+"component:store:revision:r1", "--delivery", "HEAD"); err != nil {
		t.Fatalf("revise store: %v %s", err, h.out.String())
	}
	out.Reset()
	if err := ReviseItem(ctx, []string{"--review", "pr-7", "--slide", "queue", "--item", "storage-use", "--label", "Storage use", f.root}, &out); err != nil {
		t.Fatalf("relabel: %v %s", err, out.String())
	}
	if after := findTestItem(t, f.root, "storage-use"); after.Documentation.Revision != pin("system", "storage", "r1").Revision || len(after.Selections) != 1 {
		t.Fatalf("old pins were not preserved: %+v", after)
	}
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-m", "inventory r2")

	// Historical pins need an explicit saved view whose binding is exact.
	old := pin("component", "store", "r1")
	if err := addReviewItem("baseline", old); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale pin admitted without a view: %v", err)
	}
	if err := addReviewItem("baseline", old, "--documentation-view", "HEAD"); err == nil || !strings.Contains(err.Error(), "mismatched_view_binding") {
		t.Fatalf("view with a different binding admitted: %v", err)
	}
	if err := addReviewItem("baseline", old, "--documentation-view", "no-such-commit"); err == nil || !strings.Contains(err.Error(), "view_commit_unavailable") {
		t.Fatalf("missing view: %v", err)
	}
	if err := addReviewItem("baseline", old, "--documentation-view", viewOne[:12]); err != nil {
		t.Fatalf("saved view admission: %v %s", err, out.String())
	}
	if item := findTestItem(t, f.root, "baseline"); item.DocumentationView != viewOne {
		t.Fatalf("view not recorded as a full commit: %q", item.DocumentationView)
	}

	// Implementation decks use the same exact authoring.
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--objective", "Storage design", f.root, "storage"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "storage", "--intent", "explain", "--layout", "diagram", f.root, "storage-flow"}, &out); err != nil {
		t.Fatal(err)
	}
	system["explanation"] = "Queue writes into the store (r2)."
	system["components"] = []saga.DocumentationLink{pin("component", "store", "r2"), pin("component", "queue", "r1")}
	if err := h.run("system", "revise", "storage", system, "--revision", "r2", "--parent", urn+"system:storage:revision:r1", "--delivery", "HEAD"); err != nil {
		t.Fatalf("system r2: %v %s", err, h.out.String())
	}
	writeSelections([]saga.DocumentationLink{pin("system", "storage", "r2"), pin("component", "store", "r2")}, "table", 2, 3, "")
	out.Reset()
	if err := AddItem(ctx, []string{"--slide", "storage-flow", "--id", "store", "--kind", "node", "--element-id", "slide-title", "--description", "Stores jobs.", "--documentation", urn + "system:storage", "--documentation-revision", urn + "system:storage:revision:r2", "--selections", selectionsFile, "--repo", f.repo, f.root}, &out); err != nil {
		t.Fatalf("implementation selection: %v %s", err, out.String())
	}
	assertValid(t, f.root)
}

func findTestItem(t *testing.T, root, id string) saga.ItemManifest {
	t.Helper()
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, review := range document.Reviews {
		if review.Deck == nil {
			continue
		}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				if item.ID == id {
					return item.ItemManifest
				}
			}
		}
	}
	t.Fatalf("item %s not found", id)
	return saga.ItemManifest{}
}

func TestInventoryTransactionSelections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root, repo, base, commit, id := newSlideTransactionFixture(t)
	var out bytes.Buffer
	if err := Inventory(ctx, []string{"adopt-format", "--format", "2", root}, &out); err != nil {
		t.Fatal(err)
	}
	h := &inventoryHarness{t: t, ctx: ctx, repo: repo, root: root, source: filepath.Join(t.TempDir(), "definition.json")}
	if err := h.run("component", "add", "worker", map[string]any{"name": "Worker", "explanation": "Runs the service.", "intent": "implemented",
		"code": []map[string]any{{"id": "run", "commit": commit, "path": "service.go", "start": 1, "end": 3, "note": "The service entry point."}}}, "--delivery", commit); err != nil {
		t.Fatalf("worker: %v %s", err, h.out.String())
	}
	target := "urn:change-saga:" + id + ":component:worker"
	pin := saga.DocumentationLink{Target: target, Revision: target + ":revision:r1"}
	request := slideTransactionRequest(t, repo, base, commit, id, "with-selection", "create", "absent", "node")
	request.Items[0].Documentation = &pin
	request.Items[0].Selections = []saga.ItemSelection{{ID: "run-fn", Path: []saga.DocumentationLink{pin}, Evidence: "run", Code: coderefAt("", "service.go", 3, 3, "Run is what this Item explains.")}}
	created, err := ApplySlideTransaction(ctx, root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := ApplySlideTransaction(ctx, root, base, repo, request, false)
	if err != nil || !retry.Replayed || retry.Snapshot != created.Snapshot {
		t.Fatalf("selection retry not idempotent: %v %#v", err, retry)
	}
	bad := request
	bad.RequestID = "bad-selection"
	bad.Items = append([]SlideTransactionItemRequest(nil), request.Items...)
	bad.Items[0].Selections = []saga.ItemSelection{{ID: "run-fn", Path: []saga.DocumentationLink{pin}, Evidence: "nope", Code: coderefAt("", "service.go", 3, 3, "x")}}
	if _, err := ApplySlideTransaction(ctx, root, base, repo, bad, false); err == nil || !strings.Contains(err.Error(), "missing_evidence") {
		t.Fatalf("bad selection: %v", err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %v", err, validation.Issues)
	}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if item.Documentation != nil && (len(item.Selections) != 1 || item.Selections[0].Code.Commit != commit || item.Selections[0].Code.Digest == "") {
					t.Fatalf("transaction selection not persisted exactly: %+v", item.Selections)
				}
			}
		}
	}
}

func coderefAt(commit, path string, start, end int, note string) coderef.Reference {
	return coderef.Reference{Commit: commit, Path: path, Start: start, End: end, Note: note}
}
