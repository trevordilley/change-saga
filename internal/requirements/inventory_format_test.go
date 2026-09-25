package requirements

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// rawTechnical writes records directly, bypassing writers, to exercise the
// reader's compatibility and refusal rules on persisted bytes.
func rawTechnical(t *testing.T, root, kind, id string, revisions ...TechnicalDefinition) string {
	t.Helper()
	urn, err := TechnicalURN("test", kind, id)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, filepath.FromSlash(TechnicalPath(kind, id)))
	for _, sub := range []string{"revisions", "events"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteJSON(filepath.Join(dir, kind+".json"), RecordIdentity{Schema: TechnicalSchema(kind, ""), Version: 5, ID: id, CreatedAt: testTime}, true); err != nil {
		t.Fatal(err)
	}
	parents := []string{}
	for i, def := range revisions {
		rid := "r" + string(rune('1'+i))
		v := TechnicalRevision{Schema: TechnicalSchema(kind, "-revision"), Version: 5, ID: rid, Record: urn, Parents: parents, TechnicalDefinition: def, CreatedAt: testTime}
		if err := store.WriteJSON(filepath.Join(dir, "revisions", rid+".json"), v, true); err != nil {
			t.Fatal(err)
		}
		parents = []string{urn + ":revision:" + rid}
	}
	event := TechnicalEvent{Schema: TechnicalSchema(kind, "-event"), Version: 5, ID: "active", Record: urn, Parents: []string{}, State: "active", CreatedAt: testTime}
	if err := store.WriteJSON(filepath.Join(dir, "events", "active.json"), event, true); err != nil {
		t.Fatal(err)
	}
	return urn
}

func adoptRaw(t *testing.T, root string, format int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, InventoryDir), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := InventoryFormat{Schema: TechnicalSchema("inventory-format", ""), Format: format, CreatedAt: testTime}
	if err := store.WriteJSON(filepath.Join(root, InventoryDir, InventoryFormatName), marker, false); err != nil {
		t.Fatal(err)
	}
}

func testEvidence(id string, start int) Evidence {
	return Evidence{ID: id, Reference: coderef.Reference{Commit: strings.Repeat("c", 40), Path: "store.go", Start: start, End: start + 4, Digest: "sha256:" + strings.Repeat("d", 64), Note: "Exact behavior."}}
}

func pinOf(urn, rev string) DocumentationLink {
	return DocumentationLink{Target: urn, Revision: urn + ":revision:" + rev}
}

func TestInventoryLegacyReadsUnspecifiedWithoutMarker(t *testing.T) {
	root := newSaga(t)
	legacy := testEvidence("", 1)
	urn := rawTechnical(t, root, "component", "store", TechnicalDefinition{Name: "Store", Explanation: "Legacy.", Code: []Evidence{legacy}})
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := d.Find(urn)
	if d.Format != 1 || r.CurrentRevision.EffectiveIntent() != IntentUnspecified {
		t.Fatalf("legacy intent must read unspecified: format=%d intent=%s", d.Format, r.CurrentRevision.EffectiveIntent())
	}
	if ev, _ := r.CurrentRevision.EvidenceByID(""); ev != nil {
		t.Fatal("legacy evidence without an ID must not be addressable")
	}
	// Adopting format 2 does not reinterpret legacy history.
	adoptRaw(t, root, 2)
	d, err = LoadInventory(root, "test")
	if err != nil || d.Format != 2 || d.Find(urn).CurrentRevision.EffectiveIntent() != IntentUnspecified {
		t.Fatalf("legacy history changed after adoption: %v", err)
	}
}

func TestInventoryFormatTwoRequiresExplicitAdoption(t *testing.T) {
	cases := map[string]func(t *testing.T, root string){
		"intent": func(t *testing.T, root string) {
			rawTechnical(t, root, "component", "store", TechnicalDefinition{Name: "Store", Explanation: "Proposed.", Intent: IntentProposed, Baseline: BaselineNone})
		},
		"entity": func(t *testing.T, root string) {
			rawTechnical(t, root, KindDataEntity, "job", TechnicalDefinition{Name: "Job", Explanation: "Queued payload.", Intent: IntentProposed, Baseline: BaselineNone})
		},
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			root := newSaga(t)
			write(t, root)
			if _, err := LoadInventory(root, "test"); err == nil || !strings.Contains(err.Error(), "adopt-format") {
				t.Fatalf("format-2 content accepted without the marker: %v", err)
			}
			adoptRaw(t, root, 2)
			if _, err := LoadInventory(root, "test"); err != nil {
				t.Fatal(err)
			}
		})
	}
	root := newSaga(t)
	adoptRaw(t, root, 3)
	if _, err := LoadInventory(root, "test"); err == nil || !strings.Contains(err.Error(), "unsupported inventory format 3") {
		t.Fatalf("future format accepted: %v", err)
	}
	root = newSaga(t)
	if err := os.MkdirAll(filepath.Join(root, InventoryDir, "diagrams"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadInventory(root, "test"); err == nil {
		t.Fatal("unknown inventory directory accepted")
	}
}

func TestInventoryIntentSuccessionAndBaseline(t *testing.T) {
	root := newSaga(t)
	adoptRaw(t, root, 2)
	delivery := &Delivery{Repository: "https://example.com/repo.git", Commit: strings.Repeat("a", 40)}
	urn := rawTechnical(t, root, "component", "store",
		TechnicalDefinition{Name: "Store", Explanation: "Implemented.", Intent: IntentImplemented, Delivery: delivery, Code: []Evidence{testEvidence("write", 1)}},
		TechnicalDefinition{Name: "Store", Explanation: "Proposed successor.", Intent: IntentProposed, Baseline: "urn:change-saga:test:component:store:revision:r1"},
		TechnicalDefinition{Name: "Store", Explanation: "Delivered successor.", Intent: IntentImplemented, Delivery: delivery, Code: []Evidence{testEvidence("write", 1), testEvidence("read", 20)}},
	)
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	r := d.Find(urn)
	if len(r.Revisions) != 3 || r.CurrentRevision.ID != "r3" || r.Revision(urn+":revision:r2").Baseline != urn+":revision:r1" {
		t.Fatal("succession history not retained")
	}
	if ev, owner := r.CurrentRevision.EvidenceByID("read"); ev == nil || owner != "" || ev.Start != 20 {
		t.Fatal("stable evidence ID not resolvable")
	}
	validateInventorySchemas(t, d)
	// A baseline must be an implemented ancestor, never a guess.
	root = newSaga(t)
	adoptRaw(t, root, 2)
	rawTechnical(t, root, "component", "store",
		TechnicalDefinition{Name: "Store", Explanation: "Proposed.", Intent: IntentProposed, Baseline: BaselineNone},
		TechnicalDefinition{Name: "Store", Explanation: "Proposed again.", Intent: IntentProposed, Baseline: "urn:change-saga:test:component:store:revision:r1"},
	)
	if _, err := LoadInventory(root, "test"); err == nil || !strings.Contains(err.Error(), "implemented revision") {
		t.Fatalf("proposed revision accepted as baseline: %v", err)
	}
}

func TestInventoryRevisionRules(t *testing.T) {
	store := pinOf("urn:change-saga:test:component:store", "r1")
	job := pinOf("urn:change-saga:test:data-entity:job", "r1")
	delivery := &Delivery{Repository: "https://example.com/repo.git", Commit: strings.Repeat("a", 40)}
	association := &Cardinality{Owner: "1", Destination: "unknown"}
	valid := map[string]struct {
		kind string
		def  TechnicalDefinition
	}{
		"code-free proposal": {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone}},
		"entity with mixed edges": {KindDataEntity, TechnicalDefinition{Name: "Report", Explanation: "Persisted report.", Intent: IntentImplemented, Delivery: delivery, Code: []Evidence{testEvidence("type", 1)},
			Fields:        []Field{{ID: "id", Name: "id", Keys: []string{"primary"}}, {ID: "job", Name: "job_id", Keys: []string{"foreign"}}},
			Holders:       []Holder{{Component: store, Role: "persists report rows"}},
			Relationships: []Relationship{{ID: "produced-from-job", Meaning: "production", Destination: job, Label: "produced from", Flow: "destination_to_owner", Explanation: "Worker renders a queued job.", Intent: IntentProposed}, {ID: "job", Meaning: "association", Destination: job, Cardinality: association, Explanation: "Report remembers its job.", Intent: IntentImplemented, Code: []Evidence{testEvidence("fk", 10)}}}}},
	}
	for name, c := range valid {
		urn, _ := TechnicalURN("test", c.kind, "x")
		v := TechnicalRevision{Schema: TechnicalSchema(c.kind, "-revision"), Version: 5, ID: "r1", Record: urn, Parents: []string{}, TechnicalDefinition: c.def, CreatedAt: testTime}
		if err := validateTechnicalRevision(v, c.kind, urn, "r1"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	invalid := map[string]struct {
		kind string
		def  TechnicalDefinition
	}{
		"legacy evidence id":           {"component", TechnicalDefinition{Name: "C", Explanation: "E", Code: []Evidence{testEvidence("id", 1)}}},
		"missing evidence id":          {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone, Code: []Evidence{testEvidence("", 1)}}},
		"duplicate evidence id":        {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentImplemented, Delivery: delivery, Code: []Evidence{testEvidence("a", 1), testEvidence("a", 9)}}},
		"proposal without baseline":    {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed}},
		"initial proposal baseline":    {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: "urn:change-saga:test:component:x:revision:r0"}},
		"implemented without delivery": {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentImplemented, Code: []Evidence{testEvidence("a", 1)}}},
		"implemented without code":     {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentImplemented, Delivery: delivery}},
		"proposal with delivery":       {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone, Delivery: delivery}},
		"unknown intent":               {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: "done"}},
		"entity without intent":        {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E"}},
		"proposed owner implemented edge": {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone,
			Relationships: []Relationship{{ID: "j", Meaning: "association", Destination: job, Cardinality: association, Explanation: "x", Intent: IntentImplemented, Code: []Evidence{testEvidence("a", 1)}}}}},
		"association without cardinality": {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone,
			Relationships: []Relationship{{ID: "j", Meaning: "association", Destination: job, Explanation: "x", Intent: IntentProposed}}}},
		"production with cardinality": {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone,
			Relationships: []Relationship{{ID: "j", Meaning: "production", Destination: job, Label: "l", Flow: "owner_to_destination", Cardinality: association, Explanation: "x", Intent: IntentProposed}}}},
		"holder is not a component": {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone, Holders: []Holder{{Component: job, Role: "r"}}}},
		"unknown key role":          {KindDataEntity, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone, Fields: []Field{{ID: "f", Name: "f", Keys: []string{"fk"}}}}},
		"component with fields":     {"component", TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed, Baseline: BaselineNone, Fields: []Field{{ID: "f", Name: "f"}}}},
		"erd with intent":           {KindERD, TechnicalDefinition{Name: "C", Explanation: "E", Intent: IntentProposed}},
		"erd binding outside directory": {KindERD, TechnicalDefinition{Name: "C", Explanation: "E", Visual: &Visual{Path: "assets/" + strings.Repeat("0", 64) + ".svg", MediaType: "image/svg+xml", Digest: "sha256:" + strings.Repeat("0", 64)},
			Directory: []DocumentationLink{job}, Bindings: []Binding{{ID: "b", Element: "e", Entity: &DocumentationLink{Target: "urn:change-saga:test:data-entity:report", Revision: "urn:change-saga:test:data-entity:report:revision:r1"}}}}},
		"overlay without baseline": {KindERDOverlay, TechnicalDefinition{Name: "C", Explanation: "E", Pins: []DocumentationLink{job}}},
	}
	for name, c := range invalid {
		urn, _ := TechnicalURN("test", c.kind, "x")
		v := TechnicalRevision{Schema: TechnicalSchema(c.kind, "-revision"), Version: 5, ID: "r1", Record: urn, Parents: []string{}, TechnicalDefinition: c.def, CreatedAt: testTime}
		if err := validateTechnicalRevision(v, c.kind, urn, "r1"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestInventoryVisualIsOfflineAndBound(t *testing.T) {
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"/></defs><g id="report" fill="url(#g)"><rect/></g><use href="#report"/><path id="produced"/></svg>`)
	sum := sha256.Sum256(svg)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	bindings := []Binding{{ID: "b", Element: "report"}}
	if err := ValidateVisual(svg, digest, bindings); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]byte{
		"external image": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.com/x.png"/><g id="report"/></svg>`),
		"script":         []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><g id="report"/></svg>`),
		"handler":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="report" onclick="x()"/></svg>`),
		"missing":        []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="other"/></svg>`),
		"not svg":        []byte(`<html><g id="report"/></html>`),
		"xlink":          []byte(`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="other.svg#a"/><g id="report"/></svg>`),
		"import":         []byte(`<svg xmlns="http://www.w3.org/2000/svg"><style>@import "x.css";</style><g id="report"/></svg>`),
		"style url":      []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="report" style="fill:url(https://x/y)"/></svg>`),
		"doctype":        []byte(`<!DOCTYPE svg [<!ENTITY x "y">]><svg xmlns="http://www.w3.org/2000/svg"><g id="report"/></svg>`),
		"duplicate id":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="report"/><g id="report"/></svg>`),
	} {
		if err := ValidateVisual(bad, "", bindings); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := ValidateVisual(svg, "sha256:"+strings.Repeat("0", 64), nil); err == nil {
		t.Fatal("digest mismatch accepted")
	}
}

func TestInventoryResolveSelectionIsStructuralAndExact(t *testing.T) {
	root := newSaga(t)
	adoptRaw(t, root, 2)
	delivery := &Delivery{Repository: "https://example.com/repo.git", Commit: strings.Repeat("a", 40)}
	implemented := func(name string, ev ...Evidence) TechnicalDefinition {
		return TechnicalDefinition{Name: name, Explanation: "Implemented.", Intent: IntentImplemented, Delivery: delivery, Code: ev}
	}
	storeURN := rawTechnical(t, root, "component", "store", implemented("Store", testEvidence("record-store", 14)), implemented("Store", testEvidence("record-store", 30)))
	clientURN := rawTechnical(t, root, "component", "client", implemented("Client", testEvidence("call", 1)))
	storeR1, client := pinOf(storeURN, "r1"), pinOf(clientURN, "r1")
	system := implemented("Docs", testEvidence("wiring", 50))
	system.Components = []DocumentationLink{storeR1, client}
	system.Interactions = []Interaction{{ID: "read", From: clientURN, To: storeURN, Description: "Reads.", Intent: IntentProposed}}
	systemURN := rawTechnical(t, root, "system", "docs", system)
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	validateInventorySchemas(t, d)
	doc := pinOf(systemURN, "r1")
	subset := testEvidence("", 14).Reference
	subset.Start, subset.End = 15, 17
	selection := saga.ItemSelection{ID: "s", Path: []DocumentationLink{doc, storeR1}, Evidence: "record-store", Code: subset}
	resolved, err := d.ResolveSelection(doc, selection)
	if err != nil || len(resolved.Hops) != 2 || resolved.Hops[1].ID != "r1" || resolved.Evidence.Start != 14 {
		t.Fatalf("selection did not resolve through the saved member pin: %v", err)
	}
	expect := func(name string, sel saga.ItemSelection, code SelectionCode) {
		t.Helper()
		_, err := d.ResolveSelection(doc, sel)
		var selErr *SelectionError
		if !errors.As(err, &selErr) || selErr.Code != code {
			t.Errorf("%s: got %v, want %s", name, err, code)
		}
	}
	// The System pins store:r1; a newer store:r2 is never substituted.
	newer := selection
	newer.Path = []DocumentationLink{doc, pinOf(storeURN, "r2")}
	expect("newer revision", newer, SelectionUndeclared)
	wide := selection
	wide.Code.End = 40
	expect("wider range", wide, SelectionOutside)
	other := selection
	other.Code.Commit = strings.Repeat("b", 40)
	expect("other commit", other, SelectionOutside)
	missing := selection
	missing.Evidence = "call"
	expect("evidence of another hop", missing, SelectionNoEvidence)
	gone := selection
	gone.Path = []DocumentationLink{doc, pinOf(storeURN, "r9")}
	expect("missing revision", gone, SelectionMissingPin)
	detached := selection
	detached.Path = []DocumentationLink{storeR1}
	expect("path not rooted at documentation", detached, SelectionInvalid)
}

func TestInventoryDataModelRecordsAndOverlay(t *testing.T) {
	root := newSaga(t)
	adoptRaw(t, root, 2)
	delivery := &Delivery{Repository: "https://example.com/repo.git", Commit: strings.Repeat("a", 40)}
	queue := rawTechnical(t, root, "component", "queue", TechnicalDefinition{Name: "Queue", Explanation: "Holds jobs.", Intent: IntentProposed, Baseline: BaselineNone})
	jobURN := rawTechnical(t, root, KindDataEntity, "job", TechnicalDefinition{Name: "PDF job", Explanation: "Queued JSON payload.", Intent: IntentProposed, Baseline: BaselineNone,
		Fields: []Field{{ID: "template", Name: "template", Type: "string"}}, Holders: []Holder{{Component: pinOf(queue, "r1"), Role: "carries the payload"}}})
	reportURN := rawTechnical(t, root, KindDataEntity, "report",
		TechnicalDefinition{Name: "PDF report", Explanation: "Persisted report.", Intent: IntentImplemented, Delivery: delivery, Code: []Evidence{testEvidence("row", 1)},
			Fields: []Field{{ID: "id", Name: "id", Keys: []string{"primary"}}}},
		TechnicalDefinition{Name: "PDF report", Explanation: "Report produced from a job.", Intent: IntentProposed, Baseline: "urn:change-saga:test:data-entity:report:revision:r1",
			Fields:        []Field{{ID: "id", Name: "id", Keys: []string{"primary"}}},
			Relationships: []Relationship{{ID: "produced-from-job", Meaning: "production", Destination: pinOf(jobURN, "r1"), Label: "rendered from", Flow: "destination_to_owner", Explanation: "A worker renders the queued job.", Intent: IntentProposed}}},
	)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><g id="report"/></svg>`)
	sum := sha256.Sum256(svg)
	hexSum := hex.EncodeToString(sum[:])
	erdURN := rawTechnical(t, root, KindERD, "application", TechnicalDefinition{Name: "Application data", Explanation: "Authored overview.",
		Visual:    &Visual{Path: "assets/" + hexSum + ".svg", MediaType: "image/svg+xml", Digest: "sha256:" + hexSum},
		Directory: []DocumentationLink{pinOf(reportURN, "r1")}, Bindings: []Binding{{ID: "report", Element: "report", Entity: &DocumentationLink{Target: reportURN, Revision: reportURN + ":revision:r1"}}}})
	if _, err := LoadInventory(root, "test"); err == nil || !strings.Contains(err.Error(), "visual") {
		t.Fatalf("ERD without its pinned asset accepted: %v", err)
	}
	asset := filepath.Join(root, filepath.FromSlash(TechnicalPath(KindERD, "application")), "assets", hexSum+".svg")
	if err := os.MkdirAll(filepath.Dir(asset), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, svg, 0o644); err != nil {
		t.Fatal(err)
	}
	overlayURN := rawTechnical(t, root, KindERDOverlay, "pdf-jobs", TechnicalDefinition{Name: "PDF jobs design", Explanation: "Proposed data changes.",
		ERD: &DocumentationLink{Target: erdURN, Revision: erdURN + ":revision:r1"}, Feature: "urn:change-saga:test:feature:core",
		Pins: []DocumentationLink{pinOf(reportURN, "r2"), pinOf(jobURN, "r1")}})
	removal := rawTechnical(t, root, KindERDOverlay, "drop-report", TechnicalDefinition{Name: "Drop reports", Explanation: "Proposes removing reports.",
		ERD: &DocumentationLink{Target: erdURN, Revision: erdURN + ":revision:r1"}, Removals: []Removal{{Target: reportURN, Explanation: "Reports move to another service."}}})
	d, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	validateInventorySchemas(t, d)
	composed, err := d.ComposeOverlay(d.Find(overlayURN).CurrentRevision)
	if err != nil || len(composed) != 2 || composed[0] != pinOf(reportURN, "r2") || composed[1] != pinOf(jobURN, "r1") {
		t.Fatalf("overlay composition: %v %v", composed, err)
	}
	if composed, err := d.ComposeOverlay(d.Find(removal).CurrentRevision); err != nil || len(composed) != 0 {
		t.Fatalf("overlay removal: %v %v", composed, err)
	}
	if d.Find(erdURN).CurrentRevision.Directory[0] != pinOf(reportURN, "r1") {
		t.Fatal("overlay rewrote the canonical ERD")
	}
	report := d.Find(reportURN)
	if report.CurrentRevision.EffectiveIntent() != IntentProposed || report.Revision(reportURN+":revision:r1").EffectiveIntent() != IntentImplemented {
		t.Fatal("proposed successor must retain the implemented baseline")
	}
}
