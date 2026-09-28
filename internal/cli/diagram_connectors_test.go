package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
)

func TestApplySlidePublishesConnectorsOnAReviewSlide(t *testing.T) {
	t.Parallel()
	fixture, _ := newEmptyReviewFixture(t)
	root, repo := fixture.root, fixture.repo
	d := reviewDiagram()
	for index := range d.Elements {
		if d.Elements[index].ID == "insert" {
			d.Elements[index].Head, d.Elements[index].Tail = "only-one", "zero-or-many"
			d.Elements[index].HeadLabel, d.Elements[index].HeadLabelBox = "1", &diagram.Box{X: 470, Y: 176, Width: 40, Height: 28}
			d.Elements[index].Line = "dashed"
		}
	}
	created, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), repo, reviewSlideRequest("erd-flow", "create", "absent", d), false)
	if err != nil {
		t.Fatal(err)
	}
	text, err := runDiagram(t, "", "describe", "--review", "pr-7", "--slide", "flow", root)
	if err != nil || !strings.Contains(text, `  insert: queue -> table "insert" tail=zero-or-many head=only-one head_label="1"`) {
		t.Fatalf("describe = %s err=%v", text, err)
	}
	if _, err := runDiagram(t, `[{"op":"update","id":"insert","set":{"head":"crow"}}]`, "edit", "--review", "pr-7", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "bad-head", "--from", "OPS", root); err == nil || !strings.Contains(err.Error(), "insert: head must be none or one of") {
		t.Fatalf("an unknown terminator must be refused: %v", err)
	}
	edited, err := runDiagram(t, `[{"op":"update","id":"insert","set":{"head":"one-or-many","line":null}}]`, "edit", "--review", "pr-7", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "many", "--from", "OPS", root)
	if err != nil {
		t.Fatalf("edit terminators: %v\n%s", err, edited)
	}
	if got, err := runDiagram(t, "", "get", "--review", "pr-7", "--slide", "flow", "--id", "insert", root); err != nil || !strings.Contains(got, `"head": "one-or-many"`) || strings.Contains(got, `"line"`) {
		t.Fatalf("get after edit = %s err=%v", got, err)
	}
}
