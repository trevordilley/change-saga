package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
)

// entityDiagram replaces reviewDiagram's datastore with an ERD entity whose
// field an edge ends on, and draws the queue as a queue.
func entityDiagram() diagram.Document {
	d := reviewDiagram()
	d.Elements[1].Shape = "queue"
	d.Elements[2] = diagram.Element{ID: "table", Kind: "node", Shape: "entity", Label: "jobs", X: 520, Y: 120, Width: 280, Height: 128, Style: "normal",
		Fields: diagram.Fields{{Name: "id", Type: "bigint", Key: "pk"}, {Name: "payload", Type: "jsonb"}}}
	d.Elements[3].ToField = "payload"
	return d
}

func TestApplySlideRoundTripsShapesAndEntities(t *testing.T) {
	t.Parallel()
	fixture, _ := newEmptyReviewFixture(t)
	root := fixture.root
	created, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, reviewSlideRequest("entity-flow", "create", "absent", entityDiagram()), false)
	if err != nil {
		t.Fatal(err)
	}
	slide := loadReviewSlide(t, root, "flow")
	source, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Diagram.Source))
	if stored, err := diagram.Decode(source); err != nil || len(stored.Elements[2].Fields) != 2 || stored.Elements[3].ToField != "payload" {
		t.Fatalf("stored source lost entity fields: %v", err)
	}
	asset, _ := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
	for _, want := range []string{`<g data-field="payload">`, `data-to-field="payload"`} {
		if !strings.Contains(string(asset), want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	text, err := runDiagram(t, "", "describe", "--review", "pr-7", "--slide", "flow", root)
	for _, want := range []string{
		"  queue \"Enqueue\" shape=queue icon=lucide:server\n",
		"  table \"jobs\" shape=entity\n    fields: id bigint pk, payload jsonb\n",
		"  insert: queue -> table.payload \"insert\"\n",
	} {
		if err != nil || !strings.Contains(text, want) {
			t.Errorf("describe lacks %q (err=%v):\n%s", want, err, text)
		}
	}

	grow := `[{"op":"update","id":"table","set":{"height":156,"fields":[{"name":"id","type":"bigint","key":"pk"},{"name":"payload","type":"jsonb"},{"name":"queued_at","type":"timestamptz"}]}}]`
	output, err := runDiagram(t, grow, "edit", "--review", "pr-7", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "grow", "--from", "OPS", "--json", root)
	var grown DiagramEditResult
	if err != nil || json.Unmarshal([]byte(output), &grown) != nil {
		t.Fatalf("adding a field row: %v\n%s", err, output)
	}
	if got, err := runDiagram(t, "", "get", "--review", "pr-7", "--slide", "flow", "--id", "table", root); err != nil || !strings.Contains(got, `"name": "queued_at"`) {
		t.Fatalf("edit did not add the field: %s %v", got, err)
	}
	drop := `[{"op":"update","id":"table","set":{"fields":[{"name":"id","type":"bigint","key":"pk"}]}}]`
	if _, err := runDiagram(t, drop, "edit", "--review", "pr-7", "--slide", "flow", "--expected", grown.Snapshot, "--request-id", "drop", "--from", "OPS", root); err == nil || !strings.Contains(err.Error(), `insert: to_field names "payload", which entity table does not list`) {
		t.Fatalf("dropping a field an edge ends on must be refused: %v", err)
	}
}
