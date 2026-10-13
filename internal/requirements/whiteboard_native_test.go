package requirements

import (
	"bytes"
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
)

func TestWhiteboardNativeSectionsAndRelationshipsWithoutERDRecords(t *testing.T) {
	root := newSaga(t)
	adoptRaw(t, root, 2)
	job := pinOf(rawTechnical(t, root, KindDataEntity, "job", TechnicalDefinition{
		Name: "Job", Explanation: "Captured work", Intent: IntentProposed, Baseline: BaselineNone,
	}), "r1")
	report := pinOf(rawTechnical(t, root, KindDataEntity, "report", TechnicalDefinition{
		Name: "Report", Explanation: "Searchable output", Intent: IntentProposed, Baseline: BaselineNone,
		Relationships: []Relationship{{ID: "produced-from-job", Meaning: "production", Destination: job,
			Label: "produced from", Flow: "destination_to_owner", Explanation: "Worker renders a queued job.", Intent: IntentProposed}},
	}), "r1")
	ref := func(pin DocumentationLink) *diagram.SourceReference {
		return &diagram.SourceReference{Target: pin.Target, Revision: pin.Revision}
	}
	edge := ref(report)
	edge.Relationship = "produced-from-job"
	doc := diagram.Document{Version: 1, Width: 3000, Height: 1800, Elements: []diagram.Element{
		{ID: "capture", Kind: "group", Shape: "section", Label: "Capture", Color: "yellow", Style: "boundary", X: 100, Y: 100, Width: 1000, Height: 900},
		{ID: "queue", Parent: "capture", Kind: "group", Shape: "section", Label: "Queue", Color: "gray", Style: "boundary", X: 40, Y: 100, Width: 800, Height: 600},
		{ID: "job", Parent: "queue", Kind: "node", Shape: "entity", Label: "Job", Style: "normal", X: 60, Y: 80, Width: 500, Height: 220, Source: ref(job)},
		{ID: "search", Kind: "group", Shape: "section", Label: "Search", Color: "blue", Style: "boundary", X: 1600, Y: 100, Width: 1000, Height: 900},
		{ID: "report", Parent: "search", Kind: "node", Shape: "entity", Label: "Report", Style: "normal", X: 80, Y: 120, Width: 500, Height: 220, Source: ref(report)},
		{ID: "production", Kind: "connection", From: "capture/queue/job", To: "search/report", Label: "produces", LabelBox: &diagram.Box{X: 1200, Y: 280, Width: 280, Height: 70}, Style: "normal", Source: edge, Bends: []diagram.Point{{X: 1400, Y: 360}}},
	}}
	if _, err := PublishWhiteboard(context.Background(), root, "test", "erd", "r1", "", "System data model", doc); err != nil {
		t.Fatal(err)
	}
	board, err := LoadResolvedWhiteboard(root, "test", "erd", "r1")
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range inventory.Records {
		if record.Kind == KindERD {
			t.Fatal("native document unexpectedly required a child ERD record")
		}
	}
	for _, focus := range []string{"capture", "capture/queue", "capture/queue/job", "search/report", "production"} {
		description, err := board.Composition.Describe(focus)
		if err != nil {
			t.Fatalf("describe %s: %v", focus, err)
		}
		if len(description.Connections) != 1 {
			t.Fatalf("%s lost its cross-section relationship: %+v", focus, description)
		}
		connection := description.Connections[0]
		if connection.From != "capture/queue/job" || connection.To != "search/report" || connection.Source == nil || *connection.Source != *edge {
			t.Fatalf("%s changed semantic endpoints or relationship evidence: %+v", focus, connection)
		}
	}
	for _, want := range []string{`data-instance-path="capture/queue/job"`, `data-source-relationship="produced-from-job"`, report.Revision} {
		if !bytes.Contains(board.SVG, []byte(want)) {
			t.Fatalf("render lost native source metadata %s", want)
		}
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
}
