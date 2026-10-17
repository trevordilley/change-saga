package server

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestTechnicalDesignEssayAndFourWhiteboards(t *testing.T) {
	root, repo := technicalFixture(t)
	for _, page := range []struct{ pkg, id, title, body string }{
		{"technical-design", "test-technical-design", "Technical design", "Our approach uses [Infrastructure](/technical/whiteboards/infrastructure) and [Reliability](/technical/pages/reliability)."},
		{"technical-design-reliability", "test-technical-reliability", "Reliability", "Retry work through the queue. [Back to the approach](/technical)."},
	} {
		dir := filepath.Join(root, "___overview", page.pkg+".fragment")
		writeServerFile(t, filepath.Join(dir, "fragment.json"), `{"version":2,"id":"`+page.id+`","title":"`+page.title+`","media_type":"text/markdown","entrypoint":"content.md"}`)
		writeServerFile(t, filepath.Join(dir, "content.md"), page.body)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical")
	if status != 200 {
		t.Fatalf("landing: %d %s", status, body)
	}
	for _, want := range []string{"Our approach uses", `href="/technical/pages/reliability"`, `href="/technical/erd">ERD`, `href="/technical/whiteboards/infrastructure">Infrastructure`, `href="/technical/whiteboards/data-flows">Data flows`, "crow", "Not authored yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	for _, unwanted := range []string{`href="/technical/systems"`, `href="/technical/components"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("old navigation %s", unwanted)
		}
	}
	sidebar := technicalSidebar(t, body)
	if strings.Contains(sidebar, `href="/technical/pages/reliability"`) {
		t.Fatal("supporting essay became an extra technical navigation entry")
	}
	for _, title := range []string{"ERD", "Infrastructure", "Data flows", "Architecture"} {
		if !strings.Contains(sidebar, ">"+title+"</a>") {
			t.Fatalf("missing technical role %s", title)
		}
	}
	status, body = technicalGet(t, mux, "/")
	if status != 200 || strings.Contains(body, "Our approach uses") || strings.Contains(body, "Retry work through the queue") {
		t.Fatal("technical essays leaked into product overview")
	}
	status, body = technicalGet(t, mux, "/technical/pages/reliability")
	if status != 200 || !strings.Contains(body, "Retry work through the queue") || !strings.Contains(body, "<title>Reliability") {
		t.Fatalf("supporting page: %d %s", status, body)
	}
	if got := currentNav(technicalSidebar(t, body)); len(got) != 1 || got[0] != "/technical" {
		t.Fatal("page navigation", got)
	}
	for _, id := range []string{"infrastructure", "data-flows", "architecture"} {
		status, body = technicalGet(t, mux, whiteboardHref(id, ""))
		if status != 200 || !strings.Contains(body, "has not been authored yet") {
			t.Fatalf("empty %s: %d", id, status)
		}
		doc := diagram.Document{Version: 1, Width: 800, Height: 500, Elements: []diagram.Element{{ID: "service", Kind: "node", Shape: "rect", Label: "API", Width: 200, Height: 100, Style: "normal"}}}
		if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", id, "r1", "", id, doc); err != nil {
			t.Fatal(err)
		}
		status, body = technicalGet(t, mux, whiteboardHref(id, "r1"))
		if status != 200 || !strings.Contains(body, `data-whiteboard-id="`+id+`"`) || !strings.Contains(body, `data-whiteboard-revision="r1"`) {
			t.Fatalf("published %s: %d", id, status)
		}
	}
	for _, path := range []string{"/technical/pages/missing", "/technical/whiteboards/infrastructure?revision=missing"} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Errorf("%s: %d", path, status)
		}
	}
}
