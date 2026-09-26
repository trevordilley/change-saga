package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/testfixture"
)

// TestInventoryFormatTwoReadsCoverageAndReconcile drives the read, coverage
// and reconciliation consumers over the records owner's format-2 fixture,
// published through the real writers, plus Items whose selections were
// authored through add-item.
func TestInventoryFormatTwoReadsCoverageAndReconcile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, before := sourceRepo(t, map[string]string{"README.md": "app\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "atomic.saga")
	var output bytes.Buffer
	if err := Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	fixture, err := testfixture.WriteInventoryFixture(ctx, root, repo)
	if err != nil {
		t.Fatal(err)
	}
	p := fixture.Pins
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--objective", "PDF flow", root, "pdf"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(ctx, []string{"--deck", "pdf", "--intent", "explain", "--layout", "diagram", root, "pipeline"}, &output); err != nil {
		t.Fatal(err)
	}
	item := func(id string, doc string, selections string) {
		t.Helper()
		file := filepath.Join(t.TempDir(), id+".json")
		if err := os.WriteFile(file, []byte(selections), 0o600); err != nil {
			t.Fatal(err)
		}
		output.Reset()
		if err := AddItem(ctx, []string{"--slide", "pipeline", "--id", id, "--kind", "node", "--element-id", "slide-title", "--description", "Explains " + id + ".",
			"--documentation", p[doc].Target, "--documentation-revision", p[doc].Revision, "--selections", file, "--repo", repo, root}, &output); err != nil {
			t.Fatalf("add-item %s: %v %s", id, err, output.String())
		}
	}
	sel := func(id string, path []string, evidence string, start, end int) map[string]any {
		pins := []any{}
		for _, name := range path {
			pins = append(pins, p[name])
		}
		return map[string]any{"id": id, "path": pins, "evidence": evidence, "code": map[string]any{"path": "fixture/store.go", "start": start, "end": end, "note": "The exact subset this Item explains."}}
	}
	encode := func(v ...map[string]any) string { data, _ := json.Marshal(v); return string(data) }
	// Lines 9-10 of the 8-11 "get" evidence, reached System -> Component.
	item("pipeline", "pipeline", encode(sel("store-get", []string{"pipeline", "store-r3"}, "get", 9, 10)))
	// Lines 4-5 of the 3-6 "put" evidence, reached data entity -> holder.
	item("report", "report", encode(sel("row-put", []string{"report", "store-r3"}, "put", 4, 5)))
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "documented inventory")
	head := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	q := func(args ...string) map[string]any {
		t.Helper()
		data, page, _ := inventoryEnvelope(t, append(args, "--saga", root, "--repo", repo)...)
		if data == nil {
			t.Fatalf("query %v failed: %v", args, page)
		}
		return data
	}
	has := func(list []string, want ...string) bool {
		joined := "," + strings.Join(list, ",") + ","
		for _, w := range want {
			if !strings.Contains(joined, ","+w+",") {
				return false
			}
		}
		return true
	}

	// Explicit intent filters: the current proposed revisions, not the
	// implemented Component whose history contains a proposal.
	proposed := targets(q("inventory", "--intent", "proposed")["records"].([]any))
	if !has(proposed, p["queue"].Target, p["job"].Target) || has(proposed, p["store-r3"].Target) {
		t.Fatalf("proposed: %v", proposed)
	}
	implemented := targets(q("inventory", "--intent", "implemented", "--kind", "component")["records"].([]any))
	if !has(implemented, p["store-r3"].Target, p["worker"].Target) || has(implemented, p["queue"].Target) {
		t.Fatalf("implemented: %v", implemented)
	}
	// Newness is independent: against a base without a Saga everything is new,
	// implemented or not; against head nothing is.
	if n := len(q("inventory", "--new", "--against", before)["records"].([]any)); n != 8 {
		t.Fatalf("new against absent base: %d", n)
	}
	if n := len(q("inventory", "--new", "--against", head)["records"].([]any)); n != 0 {
		t.Fatalf("new against head: %d", n)
	}
	// Feature scope follows the declared System members, data-entity holders
	// and relationship destinations from the two Items.
	scoped := targets(q("inventory", "--feature", testFeature)["records"].([]any))
	if !has(scoped, p["pipeline"].Target, p["store-r3"].Target, p["worker"].Target, p["queue"].Target, p["report"].Target, p["job"].Target) || has(scoped, p["erd"].Target) {
		t.Fatalf("scope: %v", scoped)
	}
	roles := map[string]bool{}
	for _, u := range q("inventory-uses", "--target", p["job"].Target)["uses"].([]any) {
		roles[u.(map[string]any)["role"].(string)] = true
	}
	if !roles["relationship_destination"] || !roles["erd_overlay"] {
		t.Fatalf("job uses: %v", roles)
	}

	// Both selections resolve, are current and are attached.
	selections := q("inventory-selections")["selections"].([]any)
	if len(selections) != 2 {
		t.Fatalf("selections: %v", selections)
	}
	for _, s := range selections {
		row := s.(map[string]any)
		if row["attached"] != true || row["resolution"].(map[string]any)["eligible"] != true {
			t.Fatalf("selection not eligible: %v", row)
		}
	}
	// Implementation-deck coverage counts exactly the selected lines.
	uncovered := map[string]bool{}
	for _, g := range q("gaps", "--kind", "uncovered", "--against", before, "--limit", "200")["gaps"].([]any) {
		gap := g.(map[string]any)
		data, _ := json.Marshal(gap)
		if strings.Contains(string(data), "fixture/store.go") {
			for _, line := range []string{"3", "4", "5", "6", "8", "9", "10", "11"} {
				if strings.Contains(string(data), `"line":`+line+`,`) {
					uncovered[line] = true
				}
			}
		}
	}
	if uncovered["4"] || uncovered["5"] || uncovered["9"] || uncovered["10"] || !uncovered["3"] || !uncovered["8"] || !uncovered["11"] {
		t.Fatalf("inherited coverage must be exactly the selected subsets: uncovered %v", uncovered)
	}

	// Edit inside the evidence but outside the subset: the selected bytes stay
	// current, the containing evidence is stale and needs reassessment.
	store := filepath.Join(repo, "fixture", "store.go")
	content, _ := os.ReadFile(store)
	writeFile(t, store, strings.Replace(string(content), "// Get reads one record.", "// Get reads one record by ID.", 1))
	git(t, repo, "commit", "-am", "edit outside the subset")
	selection := q("inventory-selections", "--item", selections[0].(map[string]any)["item"].(string))["selections"].([]any)[0].(map[string]any)["resolution"].(map[string]any)
	if selection["eligible"] != true || !strings.Contains(mustJSON(selection["reasons"]), "outside_subset_changed") {
		t.Fatalf("outside edit: %v", selection)
	}
	var out bytes.Buffer
	if err := Reconcile(ctx, []string{"--against", head, "--repo", repo, "--json", root}, &out); err != nil {
		t.Fatalf("reconcile: %v %s", err, out.String())
	}
	var report reconciliationReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Inventory.SelectionsReassess != 1 || report.Inventory.Regressions != 1 || report.Inventory.ProposedSkipped != 0 {
		t.Fatalf("reconcile inventory: %+v", report.Inventory)
	}
	// Edit inside the subset: selected bytes are stale and inherit nothing.
	content, _ = os.ReadFile(store)
	writeFile(t, store, strings.Replace(string(content), "return db.Find(id)", "return db.Lookup(id)", 1))
	git(t, repo, "commit", "-am", "edit the subset")
	selection = q("inventory-selections", "--item", selections[0].(map[string]any)["item"].(string))["selections"].([]any)[0].(map[string]any)["resolution"].(map[string]any)
	if selection["eligible"] != false || !strings.Contains(mustJSON(selection["reasons"]), "selected_bytes_stale") {
		t.Fatalf("inside edit: %v", selection)
	}
	// Regression: a drifted selection is repaired through its Item, never
	// through evidence-file commands naming a record that does not exist.
	out.Reset()
	if err := Reconcile(ctx, []string{"--against", head, "--repo", repo, "--json", root}, &out); err != nil {
		t.Fatalf("reconcile: %v %s", err, out.String())
	}
	report = reconciliationReport{}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	selectionTask := false
	for _, task := range report.Queue {
		for _, repair := range task.Repair {
			if (repair.Command == "replace-coverage" || repair.Command == "remove-coverage" || repair.Command == "cover") && strings.Contains(mustJSON(repair), "selections") {
				t.Fatalf("evidence repair names a selection: %+v", repair)
			}
		}
		for _, cause := range task.Because {
			selectionTask = selectionTask || (cause.Kind == "selection_selected_bytes_stale" && task.Debt == "selected_bytes_stale")
		}
	}
	if !selectionTask || report.Inventory.SelectionsStale != 1 {
		t.Fatalf("selection task missing: %+v", report.Inventory)
	}
	for _, args := range [][]string{{"mappings"}, {"gaps", "--kind", "stale"}, {"gaps", "--kind", "overlap", "--against", before}} {
		data, _, _ := inventoryEnvelope(t, append(append([]string{}, args...), "--saga", root, "--repo", repo, "--limit", "200")...)
		if strings.Contains(mustJSON(data), "#selections") {
			t.Fatalf("query %v presents a selection as an evidence record: %s", args, mustJSON(data))
		}
	}
}

func mustJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
