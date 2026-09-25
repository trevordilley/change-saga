package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

const lifecycleStore = "package store\n\n// Save persists one record.\nfunc Save(r Record) error {\n\treturn db.Put(r)\n}\n"

type inventoryHarness struct {
	t      *testing.T
	ctx    context.Context
	repo   string
	root   string
	source string
	out    bytes.Buffer
}

func newInventoryHarness(t *testing.T) *inventoryHarness {
	t.Helper()
	repo, _ := sourceRepo(t, map[string]string{"store.go": lifecycleStore, "view.go": "package view\n\nfunc Render() {}\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	return &inventoryHarness{t: t, ctx: context.Background(), repo: repo, root: newTermSaga(t, repo), source: filepath.Join(t.TempDir(), "definition.json")}
}

// run executes one public inventory command with a JSON definition.
func (h *inventoryHarness) run(kind, op, id string, def any, extra ...string) error {
	h.t.Helper()
	args := []string{op, "--id", id, "--repo", h.repo, "--json"}
	if def != nil {
		data, _ := json.Marshal(def)
		if err := os.WriteFile(h.source, data, 0o600); err != nil {
			h.t.Fatal(err)
		}
		args = append(args, "--from", h.source)
	}
	args = append(append(args, extra...), h.root)
	h.out.Reset()
	return Technical(h.ctx, kind, args, &h.out)
}

func (h *inventoryHarness) commit(files map[string]string, message string) string {
	h.t.Helper()
	for path, body := range files {
		writeFile(h.t, filepath.Join(h.repo, path), body)
	}
	git(h.t, h.repo, "add", ".")
	git(h.t, h.repo, "commit", "-m", message)
	return strings.TrimSpace(git(h.t, h.repo, "rev-parse", "HEAD"))
}

func (h *inventoryHarness) inventory() *requirements.Inventory {
	h.t.Helper()
	d, err := requirements.LoadInventory(h.root, "atomic")
	if err != nil {
		h.t.Fatal(err)
	}
	return &d
}

func (h *inventoryHarness) snapshot() string {
	h.t.Helper()
	var files []string
	_ = filepath.Walk(filepath.Join(h.root, requirements.InventoryDir), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			files = append(files, strings.TrimPrefix(path, h.root))
		}
		return nil
	})
	return strings.Join(files, "\n")
}

func evidence(id, commit string, start, end int, note string) map[string]any {
	return map[string]any{"id": id, "commit": commit, "path": "store.go", "start": start, "end": end, "note": note}
}

func TestInventoryProposedToImplementedLifecycle(t *testing.T) {
	h := newInventoryHarness(t)
	urn := "urn:change-saga:atomic:component:store"
	proposed := map[string]any{"name": "Record store", "explanation": "Persists records for later reads.", "intent": "proposed", "baseline": "none"}

	// Format 2 is opt-in: nothing new is written before deliberate adoption.
	if err := h.run("component", "add", "store", proposed); err == nil || !strings.Contains(h.out.String()+err.Error(), "adopt-format") {
		t.Fatalf("proposal accepted before adoption: %v %s", err, h.out.String())
	}
	if _, err := os.Stat(filepath.Join(h.root, requirements.InventoryDir, "components", "store.component")); !os.IsNotExist(err) {
		t.Fatal("refused write left files")
	}
	legacy := map[string]any{"name": "View", "explanation": "Renders records.", "code": []map[string]any{{"commit": "HEAD", "path": "view.go", "start": 3, "end": 3, "note": "Render entry."}}}
	if err := h.run("component", "add", "view", legacy); err != nil {
		t.Fatalf("legacy authoring before adoption: %v %s", err, h.out.String())
	}
	var out bytes.Buffer
	for range 2 {
		out.Reset()
		if err := Inventory(h.ctx, []string{"adopt-format", "--format", "2", "--json", h.root}, &out); err != nil {
			t.Fatalf("adopt: %v %s", err, out.String())
		}
	}
	if !strings.Contains(out.String(), `"replayed": true`) {
		t.Fatalf("adoption retry not idempotent: %s", out.String())
	}
	if intent := h.inventory().Find("urn:change-saga:atomic:component:view").CurrentRevision.EffectiveIntent(); intent != requirements.IntentUnspecified {
		t.Fatalf("legacy record reinterpreted as %s", intent)
	}
	legacy["name"] = "View renderer"
	if err := h.run("component", "revise", "view", legacy, "--revision", "r2", "--parent", "urn:change-saga:atomic:component:view:revision:r1"); err == nil {
		t.Fatal("new revision without explicit intent accepted after adoption")
	}

	// A code-free proposal is a real, queryable record.
	if err := h.run("component", "add", "store", proposed); err != nil {
		t.Fatalf("proposal: %v %s", err, h.out.String())
	}
	records := queryData(t, "inventory", "--saga", h.root, "--repo", h.repo, "--target", urn)["records"].([]any)
	if records[0].(map[string]any)["current_revision"].(map[string]any)["intent"] != "proposed" {
		t.Fatalf("query does not expose explicit intent: %v", records[0])
	}

	implemented := map[string]any{"name": "Record store", "explanation": "Persists records through the database client.", "intent": "implemented",
		"code": []map[string]any{evidence("save", "HEAD", 4, 6, "Save writes one record.")}}
	parent := "--parent"
	if err := h.run("component", "revise", "store", implemented, "--revision", "r2", parent, urn+":revision:r1"); err == nil {
		t.Fatal("implemented revision accepted without a delivery commit")
	}
	if err := h.run("component", "revise", "store", implemented, "--revision", "r2", parent, urn+":revision:r0", "--delivery", "HEAD"); err == nil {
		t.Fatal("parents that omit the current head accepted")
	}
	head := strings.TrimSpace(git(t, h.repo, "rev-parse", "HEAD"))
	if err := h.run("component", "revise", "store", implemented, "--revision", "r2", parent, urn+":revision:r1", "--delivery", head); err != nil {
		t.Fatalf("implementation transition: %v %s", err, h.out.String())
	}
	if err := h.run("component", "revise", "store", implemented, "--revision", "r2", parent, urn+":revision:r1", "--delivery", head); err != nil || !strings.Contains(h.out.String(), `"replayed": true`) {
		t.Fatalf("identical retry: %v %s", err, h.out.String())
	}
	record := h.inventory().Find(urn)
	r2 := record.CurrentRevision
	if r2.ID != "r2" || r2.Delivery.Commit != head || r2.Delivery.Repository != "https://example.test/acme/app.git" || r2.Code[0].Commit != head || r2.Code[0].ID != "save" {
		t.Fatalf("implemented revision not pinned: %+v", r2)
	}
	if record.Revision(urn+":revision:r1").EffectiveIntent() != requirements.IntentProposed {
		t.Fatal("the original proposal was not preserved")
	}

	// A same-identity successor proposal keeps its explicit implemented baseline.
	successor := map[string]any{"name": "Record store", "explanation": "Also batches writes.", "intent": "proposed", "baseline": urn + ":revision:r1",
		"code": []map[string]any{evidence("save", head, 4, 6, "Save is the extension point.")}}
	if err := h.run("component", "revise", "store", successor, "--revision", "r3", parent, urn+":revision:r2"); err == nil {
		t.Fatal("proposed revision accepted as an implemented baseline")
	}
	successor["baseline"] = urn + ":revision:r2"
	if err := h.run("component", "revise", "store", successor, "--revision", "r3", parent, urn+":revision:r2"); err != nil {
		t.Fatalf("successor proposal: %v %s", err, h.out.String())
	}

	// Pin from a slide: only the current revision is admitted by default.
	if err := AddDeck(h.ctx, []string{"--feature", testFeature, "--objective", "Store design", h.root, "store"}, &out); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(h.ctx, []string{"--deck", "store", "--intent", "explain", "--layout", "diagram", h.root, "writes"}, &out); err != nil {
		t.Fatal(err)
	}
	addItem := func(id, revision string) error {
		out.Reset()
		return AddItem(h.ctx, []string{"--slide", "writes", "--id", id, "--kind", "node", "--element-id", "slide-title", "--description", "Uses the shared store.", "--documentation", urn, "--documentation-revision", urn + ":revision:" + revision, h.root}, &out)
	}
	if err := addItem("baseline", "r2"); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale implemented baseline pinned without a saved view: %v", err)
	}
	if err := addItem("proposal", "r3"); err != nil {
		t.Fatalf("pin current proposal: %v %s", err, out.String())
	}
	slide := queryData(t, "slide", "--saga", h.root, "--repo", h.repo, "--target", "urn:change-saga:atomic:slide:writes")
	pin := slide["items"].([]any)[0].(map[string]any)["documentation"].(map[string]any)
	if pin["revision"] != urn+":revision:r3" {
		t.Fatalf("slide pin: %v", pin)
	}
	if _, validation, err := saga.Load(h.root); err != nil || !validation.Valid {
		t.Fatalf("saga invalid after lifecycle: %v %v", err, validation.Issues)
	}
}

func TestInventoryImplementationEvidencePolicy(t *testing.T) {
	h := newInventoryHarness(t)
	var out bytes.Buffer
	if err := Inventory(h.ctx, []string{"adopt-format", "--format", "2", h.root}, &out); err != nil {
		t.Fatal(err)
	}
	original := strings.TrimSpace(git(t, h.repo, "rev-parse", "HEAD"))
	urn := "urn:change-saga:atomic:component:"
	def := func(ev map[string]any) map[string]any {
		return map[string]any{"name": "Record store", "explanation": "Persists records.", "intent": "implemented", "code": []map[string]any{ev}}
	}

	// Pure movement before delivery keeps the original reference unchanged.
	moved := h.commit(map[string]string{"store.go": "// Package store.\n" + lifecycleStore}, "move")
	if err := h.run("component", "add", "moved", def(evidence("save", original, 4, 6, "Save.")), "--delivery", moved); err != nil {
		t.Fatalf("pure movement refused: %v %s", err, h.out.String())
	}
	if ref := h.inventory().Find(urn + "moved").CurrentRevision.Code[0]; ref.Commit != original || ref.Start != 4 {
		t.Fatalf("evidence silently repinned: %+v", ref)
	}

	// Edited selected bytes at delivery refuse the implementation assertion.
	edited := h.commit(map[string]string{"store.go": "// Package store.\n" + strings.Replace(lifecycleStore, "db.Put(r)", "db.PutBatch(r)", 1)}, "edit")
	before := h.snapshot()
	err := h.run("component", "add", "drifted", def(evidence("save", original, 4, 6, "Save.")), "--delivery", edited)
	if err == nil || !strings.Contains(h.out.String(), "stale_delivery_evidence") {
		t.Fatalf("drifted evidence accepted: %v %s", err, h.out.String())
	}
	if h.snapshot() != before {
		t.Fatal("refused implementation left partial writes")
	}

	// A supplied digest that does not match the original bytes is refused
	// even though identical bytes exist elsewhere.
	wrong := evidence("save", original, 4, 6, "Save.")
	wrong["digest"] = "sha256:" + strings.Repeat("0", 64)
	if err := h.run("component", "add", "forged", def(wrong), "--delivery", moved); err == nil {
		t.Fatal("invalid original digest accepted")
	}

	// A foreign repository cannot supply delivery evidence.
	foreign := def(evidence("save", original, 4, 6, "Save."))
	foreign["delivery"] = map[string]any{"repository": "https://example.test/other.git", "commit": moved}
	if err := h.run("component", "add", "foreign", foreign); err == nil || !strings.Contains(h.out.String()+err.Error(), "canonical source") {
		t.Fatalf("foreign delivery repository accepted: %v", err)
	}

	// Mixed intent: an implemented System may keep a proposed interaction, but
	// an implemented interaction to a proposed member is refused.
	if err := h.run("component", "add", "queue", map[string]any{"name": "Queue", "explanation": "Proposed queue.", "intent": "proposed", "baseline": "none"}); err != nil {
		t.Fatal(err)
	}
	members := []saga.DocumentationLink{{Target: urn + "moved", Revision: urn + "moved:revision:r1"}, {Target: urn + "queue", Revision: urn + "queue:revision:r1"}}
	system := map[string]any{"name": "Pipeline", "explanation": "Moves records.", "intent": "implemented", "components": members,
		"code":         []map[string]any{evidence("wiring", original, 1, 1, "Package.")},
		"interactions": []map[string]any{{"id": "enqueue", "from": urn + "moved", "to": urn + "queue", "description": "Enqueues saved records.", "intent": "implemented", "code": []map[string]any{evidence("put", original, 5, 5, "Put.")}}}}
	if err := h.run("system", "add", "pipeline", system, "--delivery", moved); err == nil || !strings.Contains(h.out.String(), "endpoint_not_implemented") {
		t.Fatalf("implemented edge to proposed member accepted: %v %s", err, h.out.String())
	}
	system["interactions"] = []map[string]any{{"id": "enqueue", "from": urn + "moved", "to": urn + "queue", "description": "Will enqueue saved records.", "intent": "proposed"}}
	if err := h.run("system", "add", "pipeline", system, "--delivery", moved); err != nil {
		t.Fatalf("implemented System with proposed edge: %v %s", err, h.out.String())
	}
}

func TestInventoryDataEntitiesAndERDAuthoring(t *testing.T) {
	h := newInventoryHarness(t)
	var out bytes.Buffer
	if err := Inventory(h.ctx, []string{"adopt-format", "--format", "2", h.root}, &out); err != nil {
		t.Fatal(err)
	}
	urn := "urn:change-saga:atomic:"
	pin := func(kind, id, rev string) saga.DocumentationLink {
		return saga.DocumentationLink{Target: urn + kind + ":" + id, Revision: urn + kind + ":" + id + ":revision:" + rev}
	}
	head := strings.TrimSpace(git(t, h.repo, "rev-parse", "HEAD"))
	if err := h.run("component", "add", "store", map[string]any{"name": "Store", "explanation": "Stores rows.", "intent": "implemented", "code": []map[string]any{evidence("save", "HEAD", 4, 6, "Save.")}}, "--delivery", head); err != nil {
		t.Fatal(err)
	}
	job := map[string]any{"name": "PDF job", "explanation": "Queued payload.", "intent": "proposed", "baseline": "none", "fields": []map[string]any{{"id": "template", "name": "template"}}}
	if err := h.run("data-entity", "add", "job", job); err != nil {
		t.Fatalf("job: %v %s", err, h.out.String())
	}
	report := map[string]any{"name": "PDF report", "explanation": "Persisted report.", "intent": "implemented",
		"code":    []map[string]any{evidence("row", "HEAD", 4, 6, "Report persistence.")},
		"fields":  []map[string]any{{"id": "id", "name": "id", "keys": []string{"primary"}}},
		"holders": []map[string]any{{"component": pin("component", "store", "r1"), "role": "persists report rows"}},
		"relationships": []map[string]any{{"id": "produced-from-job", "meaning": "production", "destination": pin("data-entity", "job", "r1"), "label": "rendered from", "flow": "destination_to_owner",
			"explanation": "A worker renders a queued job.", "intent": "implemented", "code": []map[string]any{evidence("render", "HEAD", 5, 5, "Render call.")}}}}
	if err := h.run("data-entity", "add", "report", report, "--delivery", head); err == nil || !strings.Contains(h.out.String(), "endpoint_not_implemented") {
		t.Fatalf("implemented relationship to a proposed entity accepted: %v %s", err, h.out.String())
	}
	report["relationships"].([]map[string]any)[0]["intent"] = "proposed"
	delete(report["relationships"].([]map[string]any)[0], "code")
	if err := h.run("data-entity", "add", "report", report, "--delivery", head); err != nil {
		t.Fatalf("implemented entity with proposed relationship: %v %s", err, h.out.String())
	}
	stale := map[string]any{"name": "Audit", "explanation": "Audit rows.", "intent": "proposed", "baseline": "none",
		"holders": []map[string]any{{"component": pin("component", "store", "r9"), "role": "missing"}}}
	if err := h.run("data-entity", "add", "audit", stale); err == nil {
		t.Fatal("holder pin to a missing revision accepted")
	}

	svg := filepath.Join(t.TempDir(), "erd.svg")
	writeFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg"><g id="report"/><g id="job"/></svg>`)
	erd := map[string]any{"name": "Application data", "explanation": "Authored overview.", "directory": []saga.DocumentationLink{pin("data-entity", "report", "r1")},
		"bindings": []map[string]any{{"id": "job", "element": "job", "entity": pin("data-entity", "job", "r1")}}}
	if err := h.run("erd", "add", "application", erd, "--visual", svg); err == nil {
		t.Fatal("binding outside the directory accepted")
	}
	erd["bindings"] = []map[string]any{{"id": "report", "element": "report", "entity": pin("data-entity", "report", "r1")}}
	if err := h.run("erd", "add", "application", erd, "--visual", svg); err != nil {
		t.Fatalf("erd: %v %s", err, h.out.String())
	}
	unsafe := filepath.Join(t.TempDir(), "unsafe.svg")
	writeFile(t, unsafe, `<svg xmlns="http://www.w3.org/2000/svg"><g id="report" onclick="steal()"/></svg>`)
	if err := h.run("erd", "revise", "application", erd, "--visual", unsafe, "--revision", "r2", "--parent", urn+"erd:application:revision:r1"); err == nil {
		t.Fatal("unsafe SVG accepted")
	}
	overlay := map[string]any{"name": "PDF jobs", "explanation": "Adds jobs.", "erd": pin("erd", "application", "r1"), "pins": []saga.DocumentationLink{pin("data-entity", "job", "r1")},
		"bindings": []map[string]any{{"id": "flow", "element": "job", "relationship": map[string]any{"owner": pin("data-entity", "report", "r1"), "id": "produced-from-job"}}}}
	if err := h.run("erd-overlay", "add", "pdf-jobs", overlay, "--visual", svg); err != nil {
		t.Fatalf("overlay: %v %s", err, h.out.String())
	}
	d := h.inventory()
	if len(d.Find(urn+"erd:application").CurrentRevision.Directory) != 1 {
		t.Fatal("overlay changed the canonical ERD")
	}
	asset := filepath.Join(h.root, filepath.FromSlash(requirements.TechnicalPath("erd", "application")), filepath.FromSlash(d.Find(urn+"erd:application").CurrentRevision.Visual.Path))
	if _, err := os.Stat(asset); err != nil {
		t.Fatalf("visual asset not stored offline: %v", err)
	}
}
