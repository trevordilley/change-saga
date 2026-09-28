package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// A slide frame's URL names the scheme the reviewer pinned, and the served
// visual declares every token for it last, so it outranks the drawing's own
// declarations and media rule in every browser.
func TestSlideVisualServesTheSchemeItsURLNames(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder
	}
	plain := get("/reviews/pr-7/visual/queue")
	if follow := plain.Body.String(); !strings.Contains(follow, "<style data-change-saga-scheme>:root{--bg:#ffffff;") || !strings.Contains(follow, "@media (prefers-color-scheme:dark){:root{--bg:#0d1117;") {
		t.Fatalf("a visual without a scheme does not declare the tokens for both schemes: %s", follow)
	}
	if plain.Header().Get("ETag") == "" {
		t.Fatal("a rewritten visual has no validator")
	}
	if ignored := get("/reviews/pr-7/visual/queue?saga_scheme=sepia"); ignored.Body.String() != plain.Body.String() {
		t.Fatal("an unknown scheme was honoured")
	}
	dark := get("/reviews/pr-7/visual/queue?saga_scheme=dark")
	body := dark.Body.String()
	style, rest, found := strings.Cut(body, "<style data-change-saga-scheme>:root{color-scheme:dark;")
	if dark.Code != http.StatusOK || !found || !strings.HasPrefix(strings.TrimSpace(rest[strings.Index(rest, "</style>")+len("</style>"):]), "</svg>") {
		t.Fatalf("dark visual: status=%d body=%s", dark.Code, body)
	}
	if !strings.Contains(style, `<rect id="node"`) || !strings.Contains(rest, "--diagram-canvas:#fafaf8;") || !strings.Contains(rest, "--diagram-canvas:#0d1117;") ||
		strings.Index(rest, "--diagram-canvas:#0d1117;") < strings.Index(rest, "--diagram-canvas:#fafaf8;") {
		t.Fatal("the dark scheme does not declare its tokens after the light defaults")
	}
	if dark.Header().Get("Last-Modified") != "" {
		t.Fatal("a rewritten visual is validated by its file's modification time")
	}
	light := get("/reviews/pr-7/visual/queue?saga_scheme=light").Body.String()
	if !strings.Contains(light, ":root{color-scheme:light;") || strings.Contains(light, "--diagram-canvas:#0d1117;") {
		t.Fatalf("light visual = %s", light)
	}
	// The review page marks the hand-authored visual as paper, so dark mode
	// shows it on a light card with a light frame.
	page := getPage(t, handler, "/reviews/pr-7").Body.String()
	if !strings.Contains(page, `data-slide-visual data-slide-paper`) {
		t.Fatal("the hand-authored slide is not marked as paper")
	}
}

func TestInjectSlideStyleComesLast(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ contentType, in, want string }{
		{"image/svg+xml", `<svg><g><svg></svg></g></svg>`, `<svg><g><svg></svg></g><style data-change-saga-scheme>X</style></svg>`},
		{"text/html; charset=utf-8", `<html><HEAD><style>a{}</style></HEAD><body></body></html>`, `<html><HEAD><style>a{}</style><style data-change-saga-scheme>X</style></HEAD><body></body></html>`},
		{"text/html", `<p>bare</p>`, `<p>bare</p><style data-change-saga-scheme>X</style>`},
		{"image/png", `binary`, `binary`},
	} {
		if got := string(injectSlideStyle([]byte(test.in), test.contentType, "X")); got != test.want {
			t.Errorf("%s: got %s, want %s", test.contentType, got, test.want)
		}
	}
}

func TestVisualPaperMarksVisualsWithFixedColours(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("themed.svg", `<svg><style>@media (prefers-color-scheme:dark){}</style></svg>`)
	write("fixed.svg", `<svg><rect fill="#fff"/></svg>`)
	write("fixed.html", `<p>hi</p>`)
	write("token.html", `<p style="color:var( --ink, #000)">hi</p>`)
	write("own.html", `<p style="color:var(--brand)">hi</p>`)
	for _, test := range []struct {
		mediaType, name string
		paper           bool
	}{
		{"image/svg+xml", "themed.svg", false}, {"image/svg+xml", "fixed.svg", true}, {"text/html", "fixed.html", true},
		{"text/html", "token.html", false}, {"text/html", "own.html", true},
		{"image/png", "shot.png", true}, {"text/markdown", "notes.md", false},
	} {
		if got := visualPaper(test.mediaType, dir, test.name); got != test.paper {
			t.Errorf("%s: paper=%v, want %v", test.name, got, test.paper)
		}
	}
	// A rewritten file is read again.
	write("fixed.html", `<style>@media (prefers-color-scheme:dark){}</style>`)
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(dir, "fixed.html"), later, later); err != nil {
		t.Fatal(err)
	}
	if visualPaper("text/html", dir, "fixed.html") {
		t.Error("a file that now follows the scheme is still paper")
	}
}
