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
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

type inventoryQueryFixture struct {
	repo, root            string
	absent, broken, base  string
	head                  string
	store, reader, system string
	later                 string
}

// newInventoryQueryFixture commits the Saga inside its source repository so
// comparisons can read the inventory at each base: absent (no Saga), broken
// (unreadable Saga), base (two Components) and head (a System, a new
// Component, a feature Item and a conflicted Component).
func newInventoryQueryFixture(t *testing.T) inventoryQueryFixture {
	t.Helper()
	ctx := context.Background()
	f := inventoryQueryFixture{}
	f.repo, f.absent = sourceRepo(t, map[string]string{"flags.go": "package flags\n\nvar enabled = true\n\nfunc Read() bool { return enabled }\n\nfunc Write(v bool) { enabled = v }\n"})
	git(t, f.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	f.root = filepath.Join(f.repo, "atomic.saga")
	writeFile(t, filepath.Join(f.root, "saga.json"), "{")
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-m", "broken saga")
	f.broken = strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))
	if err := os.RemoveAll(f.root); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Init(ctx, []string{"--repo", f.repo, "--repository", "https://example.test/acme/app.git", f.root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, f.root)
	ref := func(start, end int) coderef.Reference {
		content, _ := os.ReadFile(filepath.Join(f.repo, "flags.go"))
		digest, err := coderef.DigestRange(content, start, end)
		if err != nil {
			t.Fatal(err)
		}
		return coderef.Reference{Commit: f.absent, Path: "flags.go", Start: start, End: end, Digest: digest, Note: "Exact flag code."}
	}
	def := func(name string, code ...coderef.Reference) requirements.TechnicalDefinition {
		return requirements.TechnicalDefinition{Name: name, Explanation: "Explains " + name + ".", Code: code}
	}
	urn := "urn:change-saga:atomic:"
	f.store, f.reader, f.system, f.later = urn+"component:store", urn+"component:reader", urn+"system:flags", urn+"component:later"
	for id, code := range map[string]coderef.Reference{"store": ref(3, 3), "reader": ref(5, 5)} {
		if _, err := requirements.WriteTechnical(f.root, "atomic", "component", id, "r1", nil, def(id, code), true); err != nil {
			t.Fatal(err)
		}
	}
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-m", "base inventory")
	f.base = strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))

	system := def("Flags", ref(3, 5))
	system.Components = []saga.DocumentationLink{{Target: f.store, Revision: f.store + ":revision:r1"}, {Target: f.reader, Revision: f.reader + ":revision:r1"}}
	system.Interactions = []requirements.Interaction{{ID: "read", From: f.reader, To: f.store, Description: "Reads the flag.", Code: []coderef.Reference{ref(5, 5)}}}
	if _, err := requirements.WriteTechnical(f.root, "atomic", "system", "flags", "r1", nil, system, true); err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.WriteTechnical(f.root, "atomic", "component", "later", "r1", nil, def("later", ref(7, 7)), true); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--objective", "Flag flow", f.root, "flags"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "flags", "--intent", "explain", "--layout", "diagram", f.root, "decision"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(ctx, []string{"--slide", "decision", "--kind", "node", "--element-id", "slide-title", "--description", "Reuses the flag System.", "--documentation", f.system, "--documentation-revision", f.system + ":revision:r1", f.root}, &output); err != nil {
		t.Fatal(err)
	}
	// Simulate a merge of two concurrent reader revisions: both heads remain.
	if _, err := requirements.WriteTechnical(f.root, "atomic", "component", "reader", "r2", []string{f.reader + ":revision:r1"}, def("reader v2", ref(5, 5)), false); err != nil {
		t.Fatal(err)
	}
	revisions := filepath.Join(f.root, filepath.FromSlash(requirements.TechnicalPath("component", "reader")), "revisions")
	data, err := os.ReadFile(filepath.Join(revisions, "r2.json"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(revisions, "fork.json"), strings.Replace(string(data), `"id": "r2"`, `"id": "fork"`, 1))
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-m", "head inventory")
	f.head = strings.TrimSpace(git(t, f.repo, "rev-parse", "HEAD"))
	return f
}

func inventoryEnvelope(t *testing.T, args ...string) (map[string]any, map[string]any, string) {
	t.Helper()
	var output bytes.Buffer
	_ = Query(context.Background(), args, &output)
	var envelope struct {
		OK       bool           `json:"ok"`
		Snapshot string         `json:"snapshot"`
		Data     map[string]any `json:"data"`
		Page     map[string]any `json:"page"`
		Error    map[string]any `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatalf("%v: %s", err, output.String())
	}
	if !envelope.OK {
		return nil, envelope.Error, ""
	}
	return envelope.Data, envelope.Page, envelope.Snapshot
}

func targets(records []any) []string {
	out := []string{}
	for _, r := range records {
		out = append(out, r.(map[string]any)["target"].(string))
	}
	return out
}

func TestInventoryQueryIntentNewnessScopeAndUnresolved(t *testing.T) {
	f := newInventoryQueryFixture(t)
	q := func(args ...string) (map[string]any, map[string]any) {
		data, page, _ := inventoryEnvelope(t, append([]string{"inventory", "--saga", f.root, "--repo", f.repo}, args...)...)
		return data, page
	}

	all, page := q()
	if got := targets(all["records"].([]any)); strings.Join(got, ",") != f.later+","+f.store+","+f.system || page["total"].(float64) != 3 {
		t.Fatalf("resolvable records: %v", got)
	}
	unresolved := all["unresolved"].([]any)
	if len(unresolved) != 1 || unresolved[0].(map[string]any)["target"] != f.reader || unresolved[0].(map[string]any)["reasons"].([]any)[0] != "competing_revision_heads" {
		t.Fatalf("conflicted reader must stay visible: %v", unresolved)
	}
	for _, r := range all["records"].([]any) {
		selected := r.(map[string]any)["selected"].([]any)[0].(map[string]any)
		if selected["intent"] != "unspecified" {
			t.Fatalf("legacy intent inferred: %v", selected)
		}
	}

	// Filters never hide unresolved candidates.
	proposed, page := q("--intent", "proposed")
	if page["total"].(float64) != 0 || len(proposed["unresolved"].([]any)) != 1 {
		t.Fatalf("proposed filter: %v %v", page, proposed["unresolved"])
	}

	// Newness is identity introduction relative to a named, readable base.
	compared, _ := q("--against", f.base)
	newness := map[string]string{}
	for _, r := range compared["records"].([]any) {
		newness[r.(map[string]any)["target"].(string)] = r.(map[string]any)["newness"].(string)
	}
	if newness[f.store] != "existing" || newness[f.system] != "new" || newness[f.later] != "new" || compared["comparison"].(map[string]any)["baseline"] != "known" {
		t.Fatalf("newness: %v %v", newness, compared["comparison"])
	}
	onlyNew, _ := q("--new", "--against", f.base)
	if got := targets(onlyNew["records"].([]any)); strings.Join(got, ",") != f.later+","+f.system {
		t.Fatalf("--new: %v", got)
	}
	if len(onlyNew["unresolved"].([]any)) != 1 {
		t.Fatal("--new hid the conflicted record")
	}
	absent, _ := q("--new", "--against", f.absent)
	if absent["comparison"].(map[string]any)["baseline"] != "absent" || len(absent["records"].([]any)) != 3 {
		t.Fatalf("absent baseline: %v", absent["comparison"])
	}
	_, failure := q("--new", "--against", f.broken)
	if failure["code"] != "baseline_unknown" {
		t.Fatalf("unreadable baseline must refuse --new: %v", failure)
	}
	unknown, _ := q("--against", f.broken)
	for _, r := range unknown["records"].([]any) {
		if r.(map[string]any)["newness"] != "unknown" {
			t.Fatalf("unreadable baseline invented newness: %v", r)
		}
	}
	_, failure = q("--new")
	if failure["code"] != "invalid_argument" {
		t.Fatalf("--new without baseline: %v", failure)
	}

	// Feature scope follows declared pins at the saved revisions only.
	scoped, _ := q("--feature", testFeature)
	if got := targets(scoped["records"].([]any)); strings.Join(got, ",") != f.store+","+f.system {
		t.Fatalf("feature scope: %v", got)
	}
	for _, r := range scoped["records"].([]any) {
		record := r.(map[string]any)
		paths := record["scope_paths"].([]any)
		path := paths[0].(map[string]any)["path"].([]any)
		if record["target"] == f.store && (len(path) != 2 || path[0].(map[string]any)["target"] != f.system) {
			t.Fatalf("membership path not explained: %v", paths)
		}
	}
	scopedUnresolved := scoped["unresolved"].([]any)
	if len(scopedUnresolved) != 1 || scopedUnresolved[0].(map[string]any)["scope_paths"] == nil {
		t.Fatalf("scoped conflicted member hidden: %v", scopedUnresolved)
	}

	// The unresolved page has its own snapshot-bound cursor.
	paged, _, _ := inventoryEnvelope(t, "inventory", "--saga", f.root, "--repo", f.repo, "--intent", "unspecified", "--conflict-limit", "1")
	up := paged["completeness"].(map[string]any)["unresolved_page"].(map[string]any)
	if up["total"].(float64) != 1 || up["has_more"].(bool) {
		t.Fatalf("unresolved page: %v", up)
	}
}

func TestInventoryUsesQuery(t *testing.T) {
	f := newInventoryQueryFixture(t)
	q := func(args ...string) (map[string]any, map[string]any, string) {
		return inventoryEnvelope(t, append([]string{"inventory-uses", "--saga", f.root}, args...)...)
	}
	direct, page, _ := q("--target", f.store)
	uses := direct["uses"].([]any)
	if page["total"].(float64) != 1 || uses[0].(map[string]any)["role"] != "system_member" || direct["completeness"].(map[string]any)["depth_cut"] != true {
		t.Fatalf("direct store uses: %v %v", uses, direct["completeness"])
	}
	deep, page, snapshot := q("--target", f.store, "--depth", "1", "--limit", "1")
	deepCursor, _ := page["next_cursor"].(string)
	if page["total"].(float64) != 2 || !page["has_more"].(bool) || deep["completeness"].(map[string]any)["complete"] != true {
		t.Fatalf("transitive store uses: %v %v", page, deep["completeness"])
	}
	next, _, snapshot2 := q("--target", f.store, "--depth", "1", "--limit", "1", "--cursor", page["next_cursor"].(string))
	item := next["uses"].([]any)[0].(map[string]any)
	if snapshot != snapshot2 || item["role"] != "implementation_item" || item["feature"] != testFeature || len(item["path"].([]any)) != 2 {
		t.Fatalf("second page: %v", item)
	}
	onlyItems, page, _ := q("--target", f.store, "--depth", "1", "--role", "implementation_item")
	if page["total"].(float64) != 1 || onlyItems["uses"].([]any)[0].(map[string]any)["slide"] == "" {
		t.Fatalf("role filter: %v", onlyItems)
	}
	none, page, _ := q("--target", f.later)
	if page["total"].(float64) != 0 || none["completeness"].(map[string]any)["complete"] != true || none["subject"].(map[string]any)["exists"] != true {
		t.Fatalf("unused later: %v", none)
	}
	_, failure, _ := q("--target", f.store, "--depth", "99")
	if failure["code"] != "invalid_argument" {
		t.Fatalf("unbounded depth accepted: %v", failure)
	}
	// A changed Saga invalidates the cursor.
	if _, err := requirements.WriteTechnical(f.root, "atomic", "component", "extra", "r1", nil, requirements.TechnicalDefinition{Name: "extra", Explanation: "x", Code: []coderef.Reference{{Commit: f.absent, Path: "flags.go", Start: 1, End: 1, Digest: coderef.DigestBytes([]byte("package flags\n")), Note: "x"}}}, true); err != nil {
		t.Fatal(err)
	}
	_, failure, _ = q("--target", f.store, "--depth", "1", "--limit", "1", "--cursor", deepCursor)
	if failure["code"] != "stale_snapshot" {
		t.Fatalf("cursor survived a Saga change: %v", failure)
	}
}
