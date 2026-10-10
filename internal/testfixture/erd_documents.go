package testfixture

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// MultiERDOverview describes an example architecture, not an assertion about
// the repository hosting the fixture. Its links navigate independent documents.
const MultiERDOverview = `Local SQLite stores offline edits. Cloud Postgres stores shared records.

[Local SQLite](/technical/erd/sqlite) and [Cloud Postgres](/technical/erd/postgres)
show each store in detail. [Cross-store data flow](/technical/erd/cross-store)
shows how synchronization produces the cloud record from the local edit.
`

// WriteMultiERDFixture authors proposed stores, entities and three independent
// ERD documents. Entity definitions are shared by exact pin across documents.
func WriteMultiERDFixture(ctx context.Context, root, checkout string) (map[string]saga.DocumentationLink, error) {
	pins := map[string]saga.DocumentationLink{}
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return pins, err
	}
	if _, err = requirements.AdoptInventoryFormat(root, manifest.ID, requirements.CurrentInventoryFormat); err != nil {
		return pins, err
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return pins, err
	}
	defer resolver.Close()
	write := func(kind, id string, def requirements.TechnicalDefinition, svg string) error {
		result, err := requirements.WriteTechnicalRevision(ctx, root, manifest.ID, requirements.TechnicalWrite{Kind: kind, ID: id, RevisionID: "r1", Create: true, Definition: def, Visual: []byte(svg), Repository: manifest.Source.Repository, Resolver: resolver})
		if err != nil {
			return fmt.Errorf("%s %s: %w", kind, id, err)
		}
		pins[id] = saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
		return nil
	}
	for _, record := range []struct{ id, name, explanation string }{{"sqlite-store", "Local SQLite", "Persists edits while the client is offline."}, {"postgres-store", "Cloud Postgres", "Persists shared records in the cloud."}} {
		if err = write("component", record.id, requirements.TechnicalDefinition{Name: record.name, Explanation: record.explanation, Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone}, ""); err != nil {
			return pins, err
		}
	}
	local := requirements.TechnicalDefinition{Name: "Local edit", Explanation: "An offline edit awaiting synchronization.", Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone, Fields: []requirements.Field{{ID: "id", Name: "id", Type: "TEXT", Keys: []string{"primary"}}, {ID: "body", Name: "body", Type: "TEXT"}}, Holders: []requirements.Holder{{Component: pins["sqlite-store"], Role: "persists offline edits"}}}
	if err = write(requirements.KindDataEntity, "local-edit", local, ""); err != nil {
		return pins, err
	}
	cloud := requirements.TechnicalDefinition{Name: "Cloud record", Explanation: "The shared result of synchronizing a local edit.", Intent: requirements.IntentProposed, Baseline: requirements.BaselineNone, Fields: []requirements.Field{{ID: "id", Name: "id", Type: "uuid", Keys: []string{"primary"}}, {ID: "body", Name: "body", Type: "text"}}, Holders: []requirements.Holder{{Component: pins["postgres-store"], Role: "persists shared records"}}, Relationships: []requirements.Relationship{{ID: "sync", Meaning: "production", Destination: pins["local-edit"], Label: "synchronized into", Explanation: "Synchronization produces a cloud record from a local edit; this is not a cross-database foreign key.", Flow: "destination_to_owner", Intent: requirements.IntentProposed}}}
	if err = write(requirements.KindDataEntity, "cloud-record", cloud, ""); err != nil {
		return pins, err
	}
	for _, doc := range []struct {
		id, name, explanation string
		entities              []string
	}{{"sqlite", "Local SQLite", "Offline client data.", []string{"local-edit"}}, {"postgres", "Cloud Postgres", "Shared cloud data.", []string{"cloud-record"}}, {"cross-store", "Cross-store data flow", "Synchronization connects the local and cloud models.", []string{"local-edit", "cloud-record"}}} {
		def := requirements.TechnicalDefinition{Name: doc.name, Explanation: doc.explanation}
		var drawing strings.Builder
		drawing.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 150">`)
		for i, id := range doc.entities {
			pin := pins[id]
			def.Directory = append(def.Directory, pin)
			def.Bindings = append(def.Bindings, requirements.Binding{ID: id, Element: id, Entity: &pin})
			fmt.Fprintf(&drawing, `<g id="%s"><rect x="%d" y="20" width="200" height="90" fill="none" stroke="black"/><text x="%d" y="65">%s</text></g>`, id, 20+i*350, 30+i*350, id)
		}
		if doc.id == "cross-store" {
			drawing.WriteString(`<path id="sync" d="M220 65 L370 65 L360 60 M370 65 L360 70" stroke="black" fill="none"/><text x="240" y="50">sync</text>`)
			def.Bindings = append(def.Bindings, requirements.Binding{ID: "sync", Element: "sync", Relationship: &requirements.RelationshipRef{Owner: pins["cloud-record"], ID: "sync"}})
		}
		drawing.WriteString(`</svg>`)
		svg := drawing.String()
		digest := coderef.DigestBytes([]byte(svg))
		def.Visual = &requirements.Visual{Path: "assets/" + strings.TrimPrefix(digest, coderef.DigestPrefix) + ".svg", MediaType: "image/svg+xml", Digest: digest}
		if err = write(requirements.KindERD, doc.id, def, svg); err != nil {
			return pins, err
		}
	}
	dir := filepath.Join(root, applayout.OverviewDir, applayout.OverviewDataModel)
	err = store.CommitDir(root, dir, func(stage string) error {
		if err := store.WriteJSON(filepath.Join(stage, "fragment.json"), saga.FragmentManifest{Version: saga.CurrentVersion, ID: manifest.ID + "-data-model", Title: "Data model overview", MediaType: "text/markdown", Entrypoint: "content.md", Order: 30}, true); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stage, "content.md"), []byte(MultiERDOverview), 0644)
	})
	return pins, err
}
