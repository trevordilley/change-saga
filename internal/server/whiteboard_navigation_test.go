package server

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	path := filepath.Join(root, "___whiteboards", "erd", "revisions", "r1.json")
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
