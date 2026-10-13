package testfixture

import (
	"context"
	"fmt"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// WriteERDWhiteboardFixture composes three independently readable ERDs. Two
// have structured fields and local notes; the third intentionally remains an
// SVG with semantic bindings, so clients must expose its inspection limits.
// This is a proposed example architecture, not a claim about its host app.
func WriteERDWhiteboardFixture(ctx context.Context, root, checkout string) (map[string]saga.DocumentationLink, error) {
	pins, err := WriteMultiERDFixture(ctx, root, checkout)
	if err != nil {
		return pins, err
	}
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return pins, err
	}
	ref := func(id string) *diagram.SourceReference {
		pin := pins[id]
		return &diagram.SourceReference{Target: pin.Target, Revision: pin.Revision}
	}
	inventory, err := requirements.LoadInventory(root, manifest.ID)
	if err != nil {
		return pins, err
	}
	for _, child := range []struct{ id, entity, label, note, fieldType string }{
		{"sqlite", "local-edit", "Local edit", "Edits remain available while the client is offline.", "TEXT"},
		{"postgres", "cloud-record", "Cloud record", "Synchronization produces the shared record; it is not a cross-store foreign key.", "uuid"},
	} {
		doc := diagram.Document{Version: 1, Width: 720, Height: 440, Background: "#fafaf8", Elements: []diagram.Element{
			{ID: child.entity, Kind: "node", Shape: "entity", Label: child.label, X: 40, Y: 40, Width: 360, Height: 140, Style: "normal", Source: ref(child.entity), Fields: diagram.Fields{{Name: "id", Type: child.fieldType, Key: "pk"}, {Name: "body", Type: "text"}}},
			{ID: "local-note", Kind: "annotation", Shape: "bubble", Label: child.note, X: 180, Y: 230, Width: 470, Height: 145, Style: "normal", Color: "blue", About: child.entity, Source: ref(child.entity)},
		}}
		definition := inventory.Pinned(pins[child.id]).TechnicalDefinition
		definition.Visual = nil
		definition.Diagram = &doc
		result, err := requirements.WriteTechnicalRevision(ctx, root, manifest.ID, requirements.TechnicalWrite{
			Kind: requirements.KindERD, ID: child.id, RevisionID: "r2", Parents: []string{pins[child.id].Revision}, Definition: definition,
		})
		if err != nil {
			return pins, fmt.Errorf("structured %s: %w", child.id, err)
		}
		pins[child.id] = saga.DocumentationLink{Target: result.URN, Revision: result.Created[0]}
	}
	styles := diagram.DefaultStyles()
	large := styles["normal"]
	large.FontSize = 34
	hero := styles["title"]
	hero.FontSize = 52
	connection := styles["primary"]
	connection.FontSize = 28
	// Caption space sits above each full-aspect frame. The SVG overview is
	// 640 by 150, so its 980-unit appearance retains that exact ratio.
	board := diagram.Document{Version: 1, Width: 1800, Height: 1400, Background: "#fafaf8", Styles: map[string]diagram.Style{"overview": large, "hero": hero, "primary": connection}, Elements: []diagram.Element{
		{ID: "board-title", Kind: "text", Label: "Offline edits become shared records", X: 60, Y: 30, Width: 1680, Height: 80, Style: "hero"},
		{ID: "overview", Kind: "text", Label: "Two stores, one synchronization flow. Focus a diagram to read its fields and local explanation.", X: 60, Y: 115, Width: 1680, Height: 80, Style: "overview"},
		{ID: "local", Kind: "frame", Label: "Local SQLite", Summary: "Keeps edits available offline", X: 60, Y: 340, Width: 720, Height: 440, Style: "overview", Source: ref("sqlite")},
		{ID: "cloud", Kind: "group", Label: "Shared cloud", X: 1020, Y: 400, Width: 720, Height: 440, Style: "boundary"},
		{ID: "postgres", Kind: "frame", Parent: "cloud", Label: "Cloud Postgres", Summary: "Stores synchronized shared records", Width: 720, Height: 440, Style: "overview", Source: ref("postgres")},
		{ID: "sync", Kind: "connection", From: "local/local-edit", To: "cloud/postgres/cloud-record", Label: "synchronizes into", Style: "primary", Head: "arrow", Source: &diagram.SourceReference{Target: pins["cloud-record"].Target, Revision: pins["cloud-record"].Revision, Relationship: "sync"}, LabelBox: &diagram.Box{X: 775, Y: 480, Width: 245, Height: 65}},
		{ID: "flow-note", Kind: "annotation", Shape: "bubble", Label: "Synchronization produces a cloud record. This connection is a production flow, not a foreign key.", X: 60, Y: 930, Width: 580, Height: 300, Style: "overview", Color: "blue", Target: &diagram.Point{X: 840, Y: -450}, Source: &diagram.SourceReference{Target: pins["cloud-record"].Target, Revision: pins["cloud-record"].Revision, Relationship: "sync"}},
		{ID: "cross-store", Kind: "frame", Label: "Cross-store data flow", Summary: "The same entities appear together here", X: 760, Y: 1070, Width: 980, Height: 229.6875, Style: "overview", Source: ref("cross-store")},
	}}
	_, err = requirements.PublishWhiteboard(ctx, root, manifest.ID, "erd", "r1", "", "Offline and shared data", board)
	return pins, err
}
