package testfixture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// InventoryFixture names the records WriteInventoryFixture published.
type InventoryFixture struct {
	Delivery string // full commit OID of the fixture source
	// Pins by short name: store-r1 (implemented), store-r2 (proposed successor),
	// store-r3 (implemented, current), queue (proposed, code-free), worker,
	// pipeline (System), job (proposed data entity), report (implemented
	// entity with a proposed production relationship), erd, overlay.
	Pins map[string]saga.DocumentationLink
}

// InventoryFixtureSVG is the ERD visual; ids report, job and produced bind.
const InventoryFixtureSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120"><g id="report"><rect x="10" y="10" width="120" height="60"/><text x="20" y="45">PDF report</text></g></svg>`

// InventoryOverlaySVG is the overlay visual showing the proposed job and flow.
const InventoryOverlaySVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120"><g id="job"><rect x="190" y="10" width="120" height="60" stroke-dasharray="4"/><text x="200" y="45">PDF job</text></g><g id="report"><rect x="10" y="10" width="120" height="60"/></g><path id="produced" d="M190 40 L130 40"/></svg>`

var inventoryFixtureSource = map[string]string{
	"fixture/store.go":  "package fixture\n\n// Put persists one record.\nfunc Put(r Record) error {\n\treturn db.Insert(r)\n}\n\n// Get reads one record.\nfunc Get(id string) (Record, error) {\n\treturn db.Find(id)\n}\n",
	"fixture/worker.go": "package fixture\n\n// Render turns a job into a report.\nfunc Render(j Job) error {\n\treport := Report{JobID: j.ID}\n\treturn Put(report)\n}\n",
	"fixture/report.go": "package fixture\n\n// Report is a persisted PDF report row.\ntype Report struct {\n\tID    string\n\tJobID string\n}\n",
}

// WriteInventoryFixture commits deterministic source into checkout (which
// must be the Saga's canonical source checkout), adopts inventory format 2 and
// publishes a realistic technical inventory through the real writers:
// baseline -> proposed -> implemented Component history, a code-free
// proposal, a System with mixed edge intent, two data entities, an ERD and an
// overlay. It never fabricates review or approval records.
func WriteInventoryFixture(ctx context.Context, root, checkout string) (InventoryFixture, error) {
	fixture := InventoryFixture{Pins: map[string]saga.DocumentationLink{}}
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return fixture, err
	}
	for path, body := range inventoryFixtureSource {
		full := filepath.Join(checkout, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fixture, err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return fixture, err
		}
	}
	git := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", checkout}, args...)...).CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := git("add", "fixture"); err != nil {
		return fixture, err
	}
	if _, err := git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-m", "inventory fixture source"); err != nil {
		return fixture, err
	}
	if fixture.Delivery, err = git("rev-parse", "HEAD"); err != nil {
		return fixture, err
	}
	if _, err := requirements.AdoptInventoryFormat(root, manifest.ID, requirements.CurrentInventoryFormat); err != nil {
		return fixture, err
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return fixture, err
	}
	defer resolver.Close()
	ev := func(id, path string, start, end int, note string) requirements.Evidence {
		ref, authorErr := resolver.Author(ctx, coderef.Location{Commit: fixture.Delivery, Path: path, Start: start, End: end}, note)
		if authorErr != nil && err == nil {
			err = authorErr
		}
		return requirements.Evidence{ID: id, Reference: ref}
	}
	delivery := &requirements.Delivery{Repository: manifest.Source.Repository, Commit: fixture.Delivery}
	write := func(name, kind, id, revision string, parents []string, def requirements.TechnicalDefinition, visual string) error {
		if err != nil {
			return err
		}
		result, writeErr := requirements.WriteTechnicalRevision(ctx, root, manifest.ID, requirements.TechnicalWrite{
			Kind: kind, ID: id, RevisionID: revision, Parents: parents, Definition: def, Create: len(parents) == 0,
			Repository: manifest.Source.Repository, Resolver: resolver, Visual: []byte(visual),
		})
		if writeErr != nil {
			return fmt.Errorf("%s %s:%s: %w", kind, id, revision, writeErr)
		}
		fixture.Pins[name] = saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
		return nil
	}
	put := ev("put", "fixture/store.go", 3, 6, "Put writes one record.")
	get := ev("get", "fixture/store.go", 8, 11, "Get reads one record.")
	render := ev("render", "fixture/worker.go", 3, 7, "Render persists a report for a job.")
	persist := ev("persist", "fixture/worker.go", 6, 6, "The worker hands the report to the store.")
	row := ev("row", "fixture/report.go", 3, 7, "The persisted report row type.")
	if err != nil {
		return fixture, err
	}
	steps := []func() error{
		func() error {
			return write("store-r1", "component", "record-store", "r1", nil, requirements.TechnicalDefinition{Name: "Record store", Explanation: "Persists and reads records.", Intent: requirements.IntentImplemented, Delivery: delivery, Code: []requirements.Evidence{put}}, "")
		},
		func() error {
			return write("store-r2", "component", "record-store", "r2", []string{fixture.Pins["store-r1"].Revision}, requirements.TechnicalDefinition{Name: "Record store", Explanation: "Proposes adding reads by ID.", Intent: requirements.IntentProposed, Baseline: fixture.Pins["store-r1"].Revision, Code: []requirements.Evidence{put}}, "")
		},
		func() error {
			return write("store-r3", "component", "record-store", "r3", []string{fixture.Pins["store-r2"].Revision}, requirements.TechnicalDefinition{Name: "Record store", Explanation: "Persists and reads records by ID.", Intent: requirements.IntentImplemented, Delivery: delivery, Code: []requirements.Evidence{put, get}}, "")
		},
		func() error {
			return write("queue", "component", "job-queue", "r1", nil, requirements.TechnicalDefinition{Name: "Job queue", Explanation: "Proposed queue carrying PDF jobs.", Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone}, "")
		},
		func() error {
			return write("worker", "component", "pdf-worker", "r1", nil, requirements.TechnicalDefinition{Name: "PDF worker", Explanation: "Renders jobs into reports.", Intent: requirements.IntentImplemented, Delivery: delivery, Code: []requirements.Evidence{render}}, "")
		},
		func() error {
			p := fixture.Pins
			return write("pipeline", "system", "pdf-pipeline", "r1", nil, requirements.TechnicalDefinition{Name: "PDF pipeline", Explanation: "Turns queued jobs into stored reports.", Intent: requirements.IntentImplemented, Delivery: delivery,
				Code:       []requirements.Evidence{ev("pipeline", "fixture/worker.go", 3, 4, "The pipeline entry point.")},
				Components: []saga.DocumentationLink{p["worker"], p["store-r3"], p["queue"]},
				Interactions: []requirements.Interaction{
					{ID: "persist", From: p["worker"].Target, To: p["store-r3"].Target, Description: "Stores the rendered report.", Intent: requirements.IntentImplemented, Code: []requirements.Evidence{persist}},
					{ID: "consume", From: p["queue"].Target, To: p["worker"].Target, Description: "Will deliver queued jobs to the worker.", Intent: requirements.IntentProposed},
				}}, "")
		},
		func() error {
			return write("job", requirements.KindDataEntity, "pdf-job", "r1", nil, requirements.TechnicalDefinition{Name: "PDF job", Explanation: "Queued JSON payload requesting a PDF.", Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone,
				Fields:  []requirements.Field{{ID: "id", Name: "id", Type: "string", Keys: []string{"primary"}}, {ID: "template", Name: "template", Type: "string"}},
				Holders: []requirements.Holder{{Component: fixture.Pins["queue"], Role: "carries the job until a worker consumes it"}}}, "")
		},
		func() error {
			return write("report", requirements.KindDataEntity, "pdf-report", "r1", nil, requirements.TechnicalDefinition{Name: "PDF report", Explanation: "Persisted record of a rendered PDF.", Intent: requirements.IntentImplemented, Delivery: delivery,
				Code:    []requirements.Evidence{row},
				Fields:  []requirements.Field{{ID: "id", Name: "ID", Keys: []string{"primary"}}, {ID: "job", Name: "JobID", Keys: []string{"foreign"}, Note: "Not an enforced constraint."}},
				Holders: []requirements.Holder{{Component: fixture.Pins["store-r3"], Role: "persists report rows"}},
				Relationships: []requirements.Relationship{{ID: "produced-from-job", Meaning: "production", Destination: fixture.Pins["job"], Label: "rendered from", Flow: "destination_to_owner",
					Explanation: "A worker renders each queued job into one report.", Intent: requirements.IntentProposed}}}, "")
		},
		func() error {
			report := fixture.Pins["report"]
			return write("erd", requirements.KindERD, "application", "r1", nil, requirements.TechnicalDefinition{Name: "Application data model", Explanation: "Authored overview of persisted data.",
				Directory: []saga.DocumentationLink{report}, Bindings: []requirements.Binding{{ID: "report", Element: "report", Entity: &report}},
				Visual: fixtureVisual(InventoryFixtureSVG)}, InventoryFixtureSVG)
		},
		func() error {
			report, job := fixture.Pins["report"], fixture.Pins["job"]
			erd := fixture.Pins["erd"]
			return write("overlay", requirements.KindERDOverlay, "pdf-jobs", "r1", nil, requirements.TechnicalDefinition{Name: "PDF jobs design", Explanation: "Adds the queued job and its production flow.",
				ERD: &erd, Pins: []saga.DocumentationLink{job}, Visual: fixtureVisual(InventoryOverlaySVG),
				Bindings: []requirements.Binding{{ID: "job", Element: "job", Entity: &job}, {ID: "report", Element: "report", Entity: &report},
					{ID: "produced", Element: "produced", Relationship: &requirements.RelationshipRef{Owner: report, ID: "produced-from-job"}}}}, InventoryOverlaySVG)
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return fixture, err
		}
	}
	return fixture, err
}

func fixtureVisual(svg string) *requirements.Visual {
	digest := coderef.DigestBytes([]byte(svg))
	return &requirements.Visual{Path: "assets/" + strings.TrimPrefix(digest, coderef.DigestPrefix) + ".svg", MediaType: "image/svg+xml", Digest: digest}
}
