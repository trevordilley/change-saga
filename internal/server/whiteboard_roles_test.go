package server

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestWhiteboardRolesUseExistingIdentityAndExactlyFourNavigationRows(t *testing.T) {
	root, repo := termSaga(t)
	doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "area", Kind: "group", Shape: "section", Label: "Existing section", Width: 700, Height: 400, Style: "normal"}}}
	for _, id := range []string{"data-model", "existing-model", "infrastructure"} {
		for _, rev := range []string{"r1", "r2"} {
			parent := ""
			if rev == "r2" {
				parent = "r1"
			}
			if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", id, rev, parent, id, doc); err != nil {
				t.Fatal(err)
			}
		}
	}
	mutate := func(w requirements.WhiteboardMetadataWrite) {
		t.Helper()
		if _, err := requirements.UpdateWhiteboardMetadata(context.Background(), root, "test", w); err != nil {
			t.Fatal(err)
		}
	}
	mutate(requirements.WhiteboardMetadataWrite{Operation: "assign-role", ID: "existing-model", Role: "erd", Expected: "data-model"})
	mutate(requirements.WhiteboardMetadataWrite{Operation: "rename", ID: "existing-model", Name: "Existing model renamed"})
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	mux := newMux(application)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	board := application.makeWhiteboardView(inventory)
	if board.ID != "existing-model" || board.Revision != "r2" {
		t.Fatalf("wrong role document: %+v", board)
	}
	rows := technicalNav(inventory, false, board)
	documents, err := application.whiteboardDocuments("test")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := requirements.LoadWhiteboardMetadata(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	applyWhiteboardRoles(nil, rows, documents, metadata)
	if len(rows) != 4 || rows[0].Title != "ERD" || rows[1].Title != "Infrastructure" || rows[2].Title != "Data flows" || rows[3].Title != "Architecture" || rows[0].Href != "/technical/whiteboards/existing-model" {
		t.Fatalf("roles: %+v", rows)
	}
	for _, path := range []string{"/technical/erd", "/technical/whiteboards/existing-model", "/technical/whiteboards/existing-model?revision=r1", "/technical/whiteboards/data-model", "/technical/erd?revision=r1"} {
		status, body := technicalGet(t, mux, path)
		if status != 200 {
			t.Fatalf("%s: %d", path, status)
		}
		want := "existing-model"
		if path == "/technical/whiteboards/data-model" || path == "/technical/erd?revision=r1" {
			want = "data-model"
		}
		if !strings.Contains(body, `data-whiteboard-id="`+want+`"`) {
			t.Fatalf("%s lost identity %s", path, want)
		}
		if strings.Contains(path, "revision=r1") && !strings.Contains(body, `data-whiteboard-revision="r1"`) {
			t.Fatal("lost exact history")
		}
		sidebar := technicalSidebar(t, body)
		if path == "/technical/erd?revision=r1" && strings.Contains(sidebar, `href="/technical/erd?focus=area&amp;revision=r1"`) {
			t.Fatal("ERD role kept displaced historical section links")
		}
		if strings.Contains(sidebar, `nav-whiteboard-document-existing-model`) || strings.Contains(sidebar, `nav-whiteboard-document-data-model`) {
			t.Fatal("role board duplicated as fourth navigation entry")
		}
	}
	_, body := technicalGet(t, mux, "/technical")
	if !strings.Contains(body, `href="/technical/whiteboards/data-model"`) {
		t.Fatal("displaced board missing from overview")
	}
	mutate(requirements.WhiteboardMetadataWrite{Operation: "unassign-role", ID: "existing-model", Role: "erd"})
	if status, body := technicalGet(t, mux, "/technical/erd"); status != 200 || strings.Contains(body, `data-whiteboard-view`) {
		t.Fatal("unassignment resurrected legacy default")
	}
	mutate(requirements.WhiteboardMetadataWrite{Operation: "archive", ID: "existing-model"})
	if status, body := technicalGet(t, mux, "/technical/whiteboards/existing-model?revision=r1"); status != 200 || !strings.Contains(body, `data-whiteboard-revision="r1"`) {
		t.Fatal("archive broke direct history")
	}
}
