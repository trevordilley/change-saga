package server

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestWhiteboardSourceDestination(t *testing.T) {
	board := &whiteboardView{Visual: template.HTML("<svg/>"), Frames: []whiteboardFrameView{
		{Path: "z/copy", Href: "/technical/erd/sqlite?revision=r2"},
		{Path: "a/original", Href: "/technical/erd/sqlite?revision=r1"},
	}}
	if got := whiteboardSourceDestination(board, "sqlite"); got != "/technical/erd?focus=a%2Foriginal" {
		t.Fatalf("deterministic destination: %s", got)
	}
	if got := whiteboardSourceDestination(board, "missing"); got != "" {
		t.Fatal(got)
	}
	board.Visual = ""
	if got := whiteboardSourceDestination(board, "sqlite"); got != "" {
		t.Fatal("broken board redirects", got)
	}
	if got := whiteboardSourceDestination(nil, "sqlite"); got != "" {
		t.Fatal(got)
	}
}

func TestWhiteboardLatestRoutesAndFallback(t *testing.T) {
	root, repo := termSaga(t)
	if _, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo); err != nil {
		t.Fatal(err)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for _, boosted := range []bool{false, true} {
		request := httptest.NewRequest("GET", "/technical/erd/postgres", nil)
		if boosted {
			request.Header.Set("HX-Boosted", "true")
		}
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		header, status := "Location", http.StatusFound
		if boosted {
			header, status = "HX-Location", http.StatusNoContent
		}
		if response.Code != status || response.Header().Get(header) != "/technical/erd?focus=cloud%2Fpostgres" {
			t.Fatalf("boosted=%v: %d %v", boosted, response.Code, response.Header())
		}
	}
	for _, path := range []string{"/technical/erd/sqlite?revision=r1", "/technical/erd/sqlite?revision=r2", "/technical/data-entity/local-edit"} {
		status, body := technicalGet(t, mux, path)
		if status != 200 || !strings.Contains(body, "data-technical-entity=") {
			t.Fatalf("historical/entity route %s: %d", path, status)
		}
	}
	if status, _ := technicalGet(t, mux, "/technical/erd/missing"); status != 404 {
		t.Fatalf("missing source: %d", status)
	}
	path := filepath.Join(root, "___whiteboards", "data-model", "revisions", "r1.json")
	if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if status, body := technicalGet(t, mux, "/technical/erd/sqlite"); status != 200 || !strings.Contains(body, `data-erd-revision="r2"`) {
		t.Fatalf("broken board fallback: %d", status)
	}
	if err := os.RemoveAll(filepath.Join(root, "___whiteboards")); err != nil {
		t.Fatal(err)
	}
	if status, body := technicalGet(t, mux, "/technical/erd/sqlite"); status != 200 || !strings.Contains(body, `data-erd-revision="r2"`) {
		t.Fatalf("no board fallback: %d", status)
	}
}

func TestWhiteboardSectionsReplaceInventorySidebarOnlyWhenValid(t *testing.T) {
	root, repo := termSaga(t)
	if _, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo); err != nil {
		t.Fatal(err)
	}
	resolved, err := requirements.LoadResolvedWhiteboard(root, "test", "data-model", "")
	if err != nil {
		t.Fatal(err)
	}
	document := resolved.Revision.Document
	document.Elements = append(document.Elements,
		diagram.Element{ID: "area", Kind: "group", Shape: "section", Label: "Delivery", X: 100, Y: 200, Width: 600, Height: 400, Style: "normal"},
		diagram.Element{ID: "detail", Parent: "area", Kind: "group", Shape: "section", Label: "Saved records", X: 30, Y: 60, Width: 400, Height: 250, Style: "normal"})
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "data-model", "r2", "r1", "One model", document); err != nil {
		t.Fatal(err)
	}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	a := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	board := a.makeWhiteboardView(inventory)
	if board == nil || board.Visual == "" || len(board.Sections) != 2 || len(board.Frames) != 3 {
		t.Fatalf("section/frame view: %+v", board)
	}
	if board.Sections[1].Path != "area/detail" || board.Sections[1].Title != "Saved records" {
		t.Fatalf("nested section: %+v", board.Sections)
	}
	var geometry []whiteboardGeometry
	if err := json.Unmarshal([]byte(board.Metadata), &geometry); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range geometry {
		if item.Path == "area/detail" {
			found = item.Bounds.X == 130 && item.Bounds.Y == 260
		}
	}
	if !found {
		t.Fatal("nested section must use composed world bounds")
	}
	nav := technicalNav(inventory, true, board)
	if len(nav[0].Children) != 2 || nav[0].Children[1].Href != "/technical/erd?focus=area%2Fdetail" {
		t.Fatalf("section outline: %+v", nav[0])
	}
	for _, invalid := range []*whiteboardView{nil, {Note: "invalid"}} {
		fallback := technicalNav(inventory, true, invalid)
		if len(fallback) != 4 || len(fallback[0].Children) != 0 {
			t.Fatal("missing boards must retain the four design views")
		}
	}
	for _, route := range []string{"/technical/erd", "/technical"} {
		status, body := technicalGet(t, newMux(a), route)
		if status != 200 {
			t.Fatalf("%s: %d", route, status)
		}
		if route == "/technical/erd" && !strings.Contains(body, `<optgroup label="Imported sources" hidden>`) {
			t.Fatal("source catalog remains in the primary section menu")
		}
		sidebar := technicalSidebar(t, body)
		if !strings.Contains(sidebar, `href="/technical/erd?focus=area%2Fdetail"`) || strings.Contains(sidebar, `href="/technical/erd/sqlite"`) || strings.Contains(sidebar, `href="/technical/data-entity/`) {
			t.Fatalf("sidebar does not describe one model: %s", sidebar)
		}
	}
}

func TestWhiteboardSidebarCacheTracksBoardAndSourceFiles(t *testing.T) {
	root, repo := termSaga(t)
	pins, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	mux := newMux(application)
	open := func() string {
		status, body := technicalGet(t, mux, "/technical")
		if status != 200 {
			t.Fatalf("status %d", status)
		}
		return body
	}
	if body := open(); !strings.Contains(body, `data-technical-area="data-model"`) || !strings.Contains(body, `overview-part-count">Whiteboard</span>`) {
		t.Fatal("landing page still presents a source catalog")
	}
	first := application.files.current.whiteboardView
	open()
	if first == nil || first.Visual == "" || application.files.current.whiteboardView != first {
		t.Fatal("unchanged sidebar rebuilt its board")
	}
	resolved, err := requirements.LoadResolvedWhiteboard(root, "test", "data-model", "")
	if err != nil {
		t.Fatal(err)
	}
	document := resolved.Revision.Document
	document.Elements = append(document.Elements, diagram.Element{ID: "added-section", Kind: "group", Shape: "section", Label: "Added section", Width: 600, Height: 400, Style: "normal"})
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "data-model", "r2", "r1", "Updated model", document); err != nil {
		t.Fatal(err)
	}
	if body := open(); !strings.Contains(technicalSidebar(t, body), "Added section") {
		t.Fatal("board edit served a stale outline")
	}
	second := application.files.current.whiteboardView
	if second == first {
		t.Fatal("board edit reused presentation")
	}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	source := inventory.Pinned(pins["sqlite"])
	definition := source.TechnicalDefinition
	definition.Name = "Updated source"
	if _, err := requirements.WriteTechnicalRevision(context.Background(), root, "test", requirements.TechnicalWrite{Kind: requirements.KindERD, ID: "sqlite", RevisionID: "r3", Parents: []string{pins["sqlite"].Revision}, Definition: definition}); err != nil {
		t.Fatal(err)
	}
	open()
	third := application.files.current.whiteboardView
	if third == second || third.Visual == "" {
		t.Fatal("changed source reused its old presentation")
	}
	found := false
	for _, frame := range third.Frames {
		if frame.Path == "local" {
			found = frame.Status == "stale" && frame.Revision == pins["sqlite"].Revision
		}
	}
	if !found {
		t.Fatal("source edit failed to refresh status while preserving the exact board pin")
	}
}
