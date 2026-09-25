package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

// reportingSVG is a second authored ERD: it draws the report only, while its
// directory also lists the queued job.
const reportingSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120"><g id="report-card"><rect x="10" y="10" width="120" height="60"/><text x="20" y="45">PDF report</text></g></svg>`

// dataModelFixture publishes the shared realistic inventory through the real
// writers, then adds, through the same writers, a proposed successor of the
// job that declares an association with an explicitly unknown endpoint and a
// second ERD that lists an entity it does not draw.
func dataModelFixture(t *testing.T) (root, repo string, pins map[string]saga.DocumentationLink) {
	t.Helper()
	root, repo = termSaga(t)
	ctx := context.Background()
	fixture, err := testfixture.WriteInventoryFixture(ctx, root, repo)
	if err != nil {
		t.Fatal(err)
	}
	pins = fixture.Pins
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	write := func(w requirements.TechnicalWrite) saga.DocumentationLink {
		t.Helper()
		w.Repository, w.Resolver = manifest.Source.Repository, resolver
		result, err := requirements.WriteTechnicalRevision(ctx, root, manifest.ID, w)
		if err != nil {
			t.Fatalf("%s %s: %v", w.Kind, w.ID, err)
		}
		return saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
	}
	job, _ := requirements.LoadInventory(root, manifest.ID)
	previous := job.Pinned(pins["job"])
	successor := previous.TechnicalDefinition
	successor.Explanation = "Queued JSON payload requesting a PDF for one account."
	successor.Relationships = []requirements.Relationship{{ID: "for-report", Meaning: "association", Destination: pins["report"], Explanation: "A job names the report row it will fill.",
		Cardinality: &requirements.Cardinality{Owner: "0..many", Destination: "unknown"}, Intent: requirements.IntentProposed}}
	pins["job-r2"] = write(requirements.TechnicalWrite{Kind: requirements.KindDataEntity, ID: "pdf-job", RevisionID: "r2", Parents: []string{pins["job"].Revision}, Definition: successor})
	report := pins["report"]
	digest := coderef.DigestBytes([]byte(reportingSVG))
	pins["reporting"] = write(requirements.TechnicalWrite{Kind: requirements.KindERD, ID: "reporting", RevisionID: "r1", Create: true, Visual: []byte(reportingSVG),
		Definition: requirements.TechnicalDefinition{Name: "Reporting data", Explanation: "Reports and the jobs that request them.",
			Visual:    &requirements.Visual{Path: "assets/" + strings.TrimPrefix(digest, coderef.DigestPrefix) + ".svg", MediaType: "image/svg+xml", Digest: digest},
			Directory: []saga.DocumentationLink{report, pins["job-r2"]},
			Bindings:  []requirements.Binding{{ID: "report", Element: "report-card", Entity: &report}}}})
	return root, repo, pins
}

func TestTechnicalDataModelRendersAuthoredERDAndDirectory(t *testing.T) {
	root, repo, pins := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical")
	if status != 200 {
		t.Fatalf("technical page: %d %s", status, body)
	}
	report := pins["report"]
	for _, want := range []string{
		// The application ERD is the primary view, inlined as authored.
		`<g id="report">`, `data-erd-element="report" data-erd-target="` + report.Target + `" data-erd-pin="` + report.Revision + `"`,
		"1 of 1 directory entity is drawn.",
		`<span class="technical-intent intent-implemented">implemented</span>`,
		// Productions read in their declared flow and are not foreign keys.
		`data-relationship-meaning="production"`, "production, not a foreign key",
		"PDF job</a> → <em>rendered from</em> → <a",
		// Holding resources are distinct from the data.
		"Record store (persists report rows)",
		`href="/technical/erd/reporting"`, `href="/technical/erd-overlay/pdf-jobs"`, "2 data entities",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("data model lost %q", want)
		}
	}
	status, body = technicalGet(t, mux, "/technical/erd/reporting")
	for _, want := range []string{
		`<g id="report-card">`, "1 of 2 directory entities are drawn; 1 is in the directory but not drawn.",
		`data-erd-row-pin="` + pins["job-r2"].Revision + `"`, `<span class="directory-gap">not drawn</span>`,
		`data-relationship-meaning="association"`, `<span class="cardinality" data-cardinality-owner>0..many</span>`,
		`<span class="cardinality" data-cardinality-destination>unknown</span>`, "Cardinality is declared unknown, not guessed.",
		// The directory row opens the same pin as the drawn element.
		`data-documentation-target="` + report.Target + `" data-documentation-revision="` + report.Revision + `"`,
	} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("reporting ERD lost %q: %d", want, status)
		}
	}
}

func TestTechnicalDataEntityPageShowsStructureResourceAndRelationships(t *testing.T) {
	root, repo, pins := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/data-entity/pdf-report")
	for _, want := range []string{
		"<h1>PDF report</h1>", `data-documentation-intent="implemented"`, "Asserted at delivery commit",
		"<code>JobID</code>", "foreign", "Not an enforced constraint.", "Author-selected fields; this is not an exhaustive schema.",
		"Held by", `href="/technical/component/record-store?revision=r3"`, "persists report rows",
		// Its own production edge, and the job's association derived as incoming.
		`data-relationship="produced-from-job"`, `data-relationship="for-report"`,
		`href="/technical/erd/application"`, "The persisted report row type.",
	} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("entity page lost %q: %d", want, status)
		}
	}
	status, body = technicalGet(t, mux, "/technical/data-entity/pdf-job")
	for _, want := range []string{`data-documentation-intent="proposed"`, "A wholly new proposal: no implemented baseline.", "No code yet. A proposal remains visible and usable before implementation.", `data-technical-revision="r2"`} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("code-free proposal lost %q: %d", want, status)
		}
	}
	// The proposed successor of an implemented Component names its baseline.
	status, body = technicalGet(t, mux, "/technical/component/record-store?revision=r2")
	for _, want := range []string{`data-documentation-intent="proposed"`, `Proposed successor of the implemented baseline <a href="/technical/component/record-store?revision=r1"><code>r1</code></a>`, `data-technical-pin-status="stale"`} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("proposed successor lost %q: %d", want, status)
		}
	}
	// The drawer reads the same definition for an Item pin.
	status, body = technicalGet(t, mux, "/api/documentation?target="+pins["job"].Target+"&revision="+pins["job"].Revision)
	if status != 200 || !strings.Contains(body, `data-documentation-intent="proposed"`) || !strings.Contains(body, "carries the job until a worker consumes it") || !strings.Contains(body, `href="/technical/data-entity/pdf-job?revision=r1"`) || !strings.Contains(body, `data-documentation-status="stale"`) {
		t.Fatalf("drawer: %d %s", status, body)
	}
}

func TestTechnicalSystemShowsEdgeIntentIndependently(t *testing.T) {
	root, repo, _ := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/system/pdf-pipeline")
	for _, want := range []string{`data-documentation-intent="implemented"`, `data-interaction="persist" data-interaction-intent="implemented"`, `data-interaction="consume" data-interaction-intent="proposed"`} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("system lost %q: %d %s", want, status, body)
		}
	}
}

func TestTechnicalOverlayComposesWithoutRewritingBaseline(t *testing.T) {
	root, repo, pins := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/erd-overlay/pdf-jobs")
	for _, want := range []string{
		"The baseline ERD is not rewritten", `href="/technical/erd/application?revision=r1"`,
		`<g id="job">`, `data-overlay-change="adds"`, "added by this overlay",
		`data-erd-row-pin="` + pins["job"].Revision + `"`, `data-erd-relationship="produced-from-job"`,
		"2 of 2 directory entities are drawn.",
	} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("overlay lost %q: %d %s", want, status, body)
		}
	}
	status, body = technicalGet(t, mux, "/technical/erd/application")
	if status != 200 || strings.Contains(body, `data-erd-row-pin="`+pins["job"].Revision+`"`) {
		t.Fatalf("baseline ERD was rewritten by its overlay: %d", status)
	}
}

func TestTechnicalERDRefusesATamperedDrawing(t *testing.T) {
	root, repo, _ := dataModelFixture(t)
	app := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	record := inventory.Find("urn:change-saga:test:erd:application")
	asset := filepath.Join(root, filepath.FromSlash(requirements.TechnicalPath(requirements.KindERD, "application")), filepath.FromSlash(record.CurrentRevision.Visual.Path))
	// A drawing replaced after loading is refused at render time, not inlined.
	writeServerFile(t, asset, strings.Replace(testfixture.InventoryFixtureSVG, `<g id="report">`, `<g id="report" onclick="alert(1)">`, 1))
	view := app.makeERDView(inventory, record, record.CurrentRevision)
	if view.Visual != "" || !strings.Contains(view.VisualNote, "unavailable") || len(view.Directory) != 1 {
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
