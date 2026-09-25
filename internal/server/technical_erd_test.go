package server

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// TEMPORARY fixture writer: it writes inventory format 2 records exactly as
// the records owner's own format tests do (internal/requirements
// rawTechnical), using the canonical Go types. Replace it with the records
// owner's WriteInventoryFixture once that lands; no shape here is invented.
func rawServerTechnical(t *testing.T, root, kind, id string, revisions ...requirements.TechnicalDefinition) string {
	t.Helper()
	urn, err := requirements.TechnicalURN("test", kind, id)
	if err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	dir := filepath.Join(root, filepath.FromSlash(requirements.TechnicalPath(kind, id)))
	for _, sub := range []string{"revisions", "events"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteJSON(filepath.Join(dir, kind+".json"), requirements.RecordIdentity{Schema: requirements.TechnicalSchema(kind, ""), Version: 5, ID: id, CreatedAt: created}, true); err != nil {
		t.Fatal(err)
	}
	parents := []string{}
	for i, definition := range revisions {
		rid := "r" + string(rune('1'+i))
		v := requirements.TechnicalRevision{Schema: requirements.TechnicalSchema(kind, "-revision"), Version: 5, ID: rid, Record: urn, Parents: parents, TechnicalDefinition: definition, CreatedAt: created}
		if err := store.WriteJSON(filepath.Join(dir, "revisions", rid+".json"), v, true); err != nil {
			t.Fatal(err)
		}
		parents = []string{urn + ":revision:" + rid}
	}
	event := requirements.TechnicalEvent{Schema: requirements.TechnicalSchema(kind, "-event"), Version: 5, ID: "active", Record: urn, Parents: []string{}, State: "active", CreatedAt: created}
	if err := store.WriteJSON(filepath.Join(dir, "events", "active.json"), event, true); err != nil {
		t.Fatal(err)
	}
	return urn
}

func pinOf(urn, revision string) saga.DocumentationLink {
	return saga.DocumentationLink{Target: urn, Revision: urn + ":revision:" + revision}
}

// erdSVG is an authored drawing: a job, a report, and the production edge
// between them. The account entity is deliberately not drawn.
const erdSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 200"><defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto"><path d="M0 0L10 5L0 10z"/></marker></defs><g id="job"><rect x="10" y="40" width="180" height="100" fill="white" stroke="black"/><text x="20" y="70">PDFGenerationJob</text></g><g id="report"><rect x="400" y="40" width="180" height="100" fill="white" stroke="black"/><text x="410" y="70">PDFReport</text></g><path id="produces" d="M190 90H400" stroke="black" stroke-dasharray="6 4" marker-end="url(#arrow)"/></svg>`

// dataModelFixture writes a format 2 inventory: holding Components, an
// implemented account, a proposed report owning a proposed association to
// it, an implemented job owning a proposed production edge to the report,
// the application ERD drawing two of the three, and an overlay proposing a
// successor account.
func dataModelFixture(t *testing.T) (root, repo string, urns map[string]string) {
	t.Helper()
	root, repo = technicalFixture(t)
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	evidence := func(id string) []requirements.Evidence {
		return []requirements.Evidence{{ID: id, Reference: coderef.Reference{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest, Note: "Exact declaration provenance."}}}
	}
	if err := os.MkdirAll(filepath.Join(root, requirements.InventoryDir), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := requirements.InventoryFormat{Schema: requirements.TechnicalSchema("inventory-format", ""), Format: requirements.CurrentInventoryFormat, CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	if err := store.WriteJSON(filepath.Join(root, requirements.InventoryDir, requirements.InventoryFormatName), marker, false); err != nil {
		t.Fatal(err)
	}
	delivery := &requirements.Delivery{Repository: "https://example.test/a.git", Commit: commit}
	urns = map[string]string{}
	urns["account"] = rawServerTechnical(t, root, requirements.KindDataEntity, "account",
		requirements.TechnicalDefinition{Name: "Account", Explanation: "A customer account.", Intent: requirements.IntentImplemented, Delivery: delivery, Code: evidence("account-type"),
			Fields: []requirements.Field{{ID: "id", Name: "accountId", Type: "uuid", Keys: []string{"primary"}}}},
		requirements.TechnicalDefinition{Name: "Account", Explanation: "A customer account with a billing plan.", Intent: requirements.IntentProposed, Baseline: "urn:change-saga:test:data-entity:account:revision:r1",
			Fields: []requirements.Field{{ID: "id", Name: "accountId", Type: "uuid", Keys: []string{"primary"}}, {ID: "plan", Name: "plan", Note: "Proposed billing plan."}}})
	storeComponent := "urn:change-saga:test:component:store"
	urns["report"] = rawServerTechnical(t, root, requirements.KindDataEntity, "report",
		requirements.TechnicalDefinition{Name: "PDFReport", Explanation: "A persisted report produced from a job.", Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone,
			Fields:  []requirements.Field{{ID: "id", Name: "reportId", Keys: []string{"primary"}}, {ID: "account", Name: "accountId", Keys: []string{"foreign"}, Note: "Owning account."}, {ID: "status", Name: "status"}},
			Holders: []requirements.Holder{{Component: pinOf(storeComponent, "r1"), Role: "Reporting database table"}},
			Relationships: []requirements.Relationship{{ID: "owned-by", Meaning: "association", Destination: pinOf(urns["account"], "r1"), Explanation: "Each report belongs to one account.",
				Cardinality: &requirements.Cardinality{Owner: "0..many", Destination: "1"}, Intent: requirements.IntentProposed}}})
	urns["job"] = rawServerTechnical(t, root, requirements.KindDataEntity, "job",
		requirements.TechnicalDefinition{Name: "PDFGenerationJob", Explanation: "A logical JSON payload carried by the queue.", Intent: requirements.IntentImplemented, Delivery: delivery, Code: evidence("job-type"),
			Fields:  []requirements.Field{{ID: "id", Name: "jobId", Keys: []string{"primary"}}, {ID: "input", Name: "inputLocation"}},
			Holders: []requirements.Holder{{Component: pinOf("urn:change-saga:test:component:reader", "r1"), Role: "Queue message"}},
			Relationships: []requirements.Relationship{{ID: "produces", Meaning: "production", Destination: pinOf(urns["report"], "r1"), Label: "produces", Flow: "owner_to_destination",
				Explanation: "Processing a job produces its report.", Intent: requirements.IntentProposed}}})
	sum := sha256.Sum256([]byte(erdSVG))
	hexSum := hex.EncodeToString(sum[:])
	urns["erd"] = rawServerTechnical(t, root, requirements.KindERD, "application",
		requirements.TechnicalDefinition{Name: "Application data model", Explanation: "The data story of report generation.",
			Visual:    &requirements.Visual{Path: "assets/" + hexSum + ".svg", MediaType: "image/svg+xml", Digest: "sha256:" + hexSum},
			Directory: []saga.DocumentationLink{pinOf(urns["job"], "r1"), pinOf(urns["report"], "r1"), pinOf(urns["account"], "r1")},
			Bindings: []requirements.Binding{
				{ID: "job", Element: "job", Entity: &saga.DocumentationLink{Target: urns["job"], Revision: urns["job"] + ":revision:r1"}},
				{ID: "report", Element: "report", Entity: &saga.DocumentationLink{Target: urns["report"], Revision: urns["report"] + ":revision:r1"}},
				{ID: "produces", Element: "produces", Relationship: &requirements.RelationshipRef{Owner: pinOf(urns["job"], "r1"), ID: "produces"}},
			}})
	writeServerFile(t, filepath.Join(root, filepath.FromSlash(requirements.TechnicalPath(requirements.KindERD, "application")), "assets", hexSum+".svg"), erdSVG)
	urns["overlay"] = rawServerTechnical(t, root, requirements.KindERDOverlay, "billing",
		requirements.TechnicalDefinition{Name: "Billing design", Explanation: "Proposed account changes for billing.",
			ERD: &saga.DocumentationLink{Target: urns["erd"], Revision: urns["erd"] + ":revision:r1"}, Feature: "urn:change-saga:test:feature:core",
			Pins: []saga.DocumentationLink{pinOf(urns["account"], "r2")}})
	if _, err := requirements.LoadInventory(root, "test"); err != nil {
		t.Fatalf("fixture does not load: %v", err)
	}
	return root, repo, urns
}

func TestTechnicalDataModelRendersAuthoredERDAndDirectory(t *testing.T) {
	root, repo, urns := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical")
	if status != 200 {
		t.Fatalf("technical page: %d %s", status, body)
	}
	for _, want := range []string{
		// The authored drawing is inlined as authored, with its bindings.
		`<g id="job">`, `data-erd-element="job" data-erd-target="` + urns["job"] + `" data-erd-pin="` + urns["job"] + `:revision:r1"`,
		`data-erd-element="produces"`, `data-erd-relationship="produces"`,
		// Two of three directory entities are drawn; the third is stated.
		"2 of 3 directory entities are drawn; 1 is in the directory but not drawn",
		`data-erd-directory-row="` + urns["account"] + `"`, `<span class="directory-gap">not drawn</span>`,
		// Relationship meaning and cardinality come from the records.
		`data-relationship-meaning="production"`, "production, not a foreign key", "PDFGenerationJob</a> → <em>produces</em> → <a",
		`data-relationship-meaning="association"`, `<span class="cardinality" data-cardinality-owner>0..many</span>`, `<span class="cardinality" data-cardinality-destination>1</span>`,
		// Intent is per entity and per relationship, never inferred.
		`<span class="technical-intent intent-implemented">implemented</span>`, `<span class="technical-intent intent-proposed">proposed</span>`,
		// Holding resources are distinct from the data.
		"Store (Reporting database table)",
		`href="/technical/erd-overlay/billing"`, "3 data entities",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("data model lost %q", want)
		}
	}
	// The directory row's drawer control and the drawn element open the same pin.
	if !strings.Contains(body, `data-documentation-target="`+urns["report"]+`" data-documentation-revision="`+urns["report"]+`:revision:r1"`) {
		t.Fatal("directory row does not open the drawn pin")
	}
}

func TestTechnicalDataEntityPageShowsStructureResourceAndRelationships(t *testing.T) {
	root, repo, urns := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/data-entity/report")
	for _, want := range []string{
		"<h1>PDFReport</h1>", `data-documentation-intent="proposed"`, "A wholly new proposal: no implemented baseline.",
		"<code>accountId</code>", "foreign", "Owning account.", "Author-selected fields; this is not an exhaustive schema.",
		"Held by", `href="/technical/component/store?revision=r1"`, "Reporting database table",
		// Outgoing association, and the job's production edge derived as incoming.
		`data-relationship="owned-by"`, `data-relationship="produces"`,
		"No code yet. A proposal remains visible and usable before implementation.",
		`href="/technical/erd/application"`,
	} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("entity page lost %q: %d", want, status)
		}
	}
	status, body = technicalGet(t, mux, "/technical/data-entity/account?revision=r2")
	for _, want := range []string{`data-documentation-intent="proposed"`, `Proposed successor of the implemented baseline <a href="/technical/data-entity/account?revision=r1"><code>r1</code></a>`, `data-technical-revision="r2"`} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("proposed successor lost %q: %d %s", want, status, body)
		}
	}
	status, body = technicalGet(t, mux, "/technical/data-entity/account?revision=r1")
	if status != 200 || !strings.Contains(body, `data-documentation-intent="implemented"`) || !strings.Contains(body, "Asserted at delivery commit") || !strings.Contains(body, `data-technical-pin-status="stale"`) {
		t.Fatalf("implemented baseline: %d %s", status, body)
	}
	// The drawer reads the same definition for an Item pin.
	status, body = technicalGet(t, mux, "/api/documentation?target="+urns["job"]+"&revision="+urns["job"]+":revision:r1")
	if status != 200 || !strings.Contains(body, `data-documentation-intent="implemented"`) || !strings.Contains(body, "Queue message") || !strings.Contains(body, `href="/technical/data-entity/job?revision=r1"`) {
		t.Fatalf("drawer: %d %s", status, body)
	}
}

func TestTechnicalOverlayComposesWithoutRewritingBaseline(t *testing.T) {
	root, repo, urns := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/erd-overlay/billing")
	for _, want := range []string{
		"The baseline ERD is not rewritten", `href="/technical/erd/application?revision=r1"`, `href="/features/core"`,
		"This overlay has no drawing of its own.", `data-overlay-change="replaces"`, "proposed revision replacing r1",
		`data-erd-row-pin="` + urns["account"] + `:revision:r2"`,
	} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("overlay lost %q: %d %s", want, status, body)
		}
	}
	status, body = technicalGet(t, mux, "/technical/erd/application")
	if status != 200 || !strings.Contains(body, `data-erd-row-pin="`+urns["account"]+`:revision:r1"`) || strings.Contains(body, `data-erd-row-pin="`+urns["account"]+`:revision:r2"`) {
		t.Fatalf("baseline ERD was rewritten by its overlay: %d", status)
	}
}

func TestTechnicalERDRefusesATamperedDrawing(t *testing.T) {
	root, repo, _ := dataModelFixture(t)
	sum := sha256.Sum256([]byte(erdSVG))
	asset := filepath.Join(root, filepath.FromSlash(requirements.TechnicalPath(requirements.KindERD, "application")), "assets", hex.EncodeToString(sum[:])+".svg")
	// A drawing replaced after loading is refused at render time, not inlined.
	app := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, asset, strings.Replace(erdSVG, "<g id=\"job\">", "<g id=\"job\" onclick=\"alert(1)\">", 1))
	record := inventory.Find("urn:change-saga:test:erd:application")
	view := app.makeERDView(inventory, record, record.CurrentRevision)
	if view.Visual != "" || !strings.Contains(view.VisualNote, "unavailable") || len(view.Directory) != 3 {
		t.Fatalf("tampered drawing: %#v", view)
	}
}

// TestWriteTechnicalPreviewFixture copies the data-model fixture repository to
// CHANGE_SAGA_TECHNICAL_PREVIEW for manual browser inspection. It is skipped
// in every ordinary run.
func TestWriteTechnicalPreviewFixture(t *testing.T) {
	out := os.Getenv("CHANGE_SAGA_TECHNICAL_PREVIEW")
	if out == "" {
		t.Skip("set CHANGE_SAGA_TECHNICAL_PREVIEW to export the fixture")
	}
	_, repo, _ := dataModelFixture(t)
	if err := os.CopyFS(out, os.DirFS(repo)); err != nil {
		t.Fatal(err)
	}
}
