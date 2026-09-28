package server

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/theme"
)

const testTheme = `/* Brand colours. */
:root { --bg: rgb(16, 32, 48); --diagram-canvas: #fefefe; }
:root[data-theme="dark"] { --bg: #000; }
`

// The reviewer links the theme right after its own stylesheet and serves
// only the validated, re-serialized overrides, for light and both ways of
// choosing dark.
func TestThemeStylesheetFollowsTheDefaultTokens(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	page := getPage(t, handler, "/reviews/pr-7").Body.String()
	app, linked := strings.Index(page, `href="`+assetPath("app.css")+`"`), strings.Index(page, `<link rel="stylesheet" href="/theme.css">`)
	if app < 0 || linked < app {
		t.Fatalf("the theme is not linked after app.css:\n%s", page[:min(len(page), 800)])
	}

	get := func(header http.Header) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/theme.css", nil)
		for name, values := range header {
			request.Header[name] = values
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	if empty := get(nil); empty.Code != http.StatusOK || empty.Body.Len() != 0 {
		t.Fatalf("no theme served %d %q", empty.Code, empty.Body.String())
	}

	writeServerFile(t, filepath.Join(fixture.root, theme.FileName), testTheme)
	served := get(nil)
	body := served.Body.String()
	for _, want := range []string{
		":root{--bg:#102030;--diagram-canvas:#fefefe;}",
		"@media (prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#000000;}}",
		":root[data-theme=dark]{--bg:#000000;}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("theme.css lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Brand") || strings.Contains(body, "rgb(") || served.Header().Get("Content-Type") != "text/css; charset=utf-8" {
		t.Errorf("theme.css is not re-serialized: %q %v", body, served.Header())
	}
	if again := get(http.Header{"If-None-Match": {served.Header().Get("ETag")}}); again.Code != http.StatusNotModified {
		t.Errorf("an unchanged theme revalidated with %d", again.Code)
	}

	writeServerFile(t, filepath.Join(fixture.root, theme.FileName), ":root { --bg: url(https://example.test/x.png); }")
	if refused := get(nil); refused.Body.Len() != 0 {
		t.Errorf("an invalid theme was applied: %q", refused.Body.String())
	}
}

// A served slide declares every token with the theme applied, after its own
// styles, while its committed bytes and the visual CSP stay as they were.
func TestServedVisualsCarryTheThemeTokens(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	visual := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		securityHeaders(handler).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/reviews/pr-7/visual/queue", nil))
		return recorder
	}

	defaults := visual().Body.String()
	if !strings.Contains(defaults, `<rect id="node" width="5" height="5"/><style data-change-saga-theme="">:root{--bg:#ffffff;`) || !strings.HasSuffix(defaults, "}}</style></svg>") {
		t.Fatalf("a slide without a theme lacks the default tokens:\n%s", defaults)
	}

	writeServerFile(t, filepath.Join(fixture.root, theme.FileName), testTheme)
	recorder := visual()
	body := recorder.Body.String()
	for _, want := range []string{"--bg:#102030;", "--diagram-canvas:#fefefe;", "@media (prefers-color-scheme:dark){:root{--bg:#000000;", "--diagram-canvas:#0d1117;"} {
		if !strings.Contains(body, want) {
			t.Errorf("the served slide lacks %q", want)
		}
	}
	if strings.Contains(body, ";color-scheme:") || strings.Contains(body, "data-theme") {
		t.Error("the frame style sets a color-scheme or uses selectors that never match in a frame")
	}
	assertAuthoredContentPolicy(t, recorder.Header())
	// The injected SVG is still well-formed XML, or a browser shows an error
	// page instead of the slide.
	for decoder := xml.NewDecoder(strings.NewReader(body)); ; {
		if _, err := decoder.Token(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("the served slide is not well-formed XML: %v", err)
		}
	}
	assets, _ := filepath.Glob(filepath.Join(fixture.root, "___reviews", "pr-7.review", "deck", "*.svg"))
	for _, asset := range assets {
		if data, _ := os.ReadFile(asset); strings.Contains(string(data), "data-change-saga-theme") {
			t.Fatalf("the committed slide %s was rewritten", asset)
		}
	}
}

func TestEmbeddedSlideAssetCarriesTheThemeTokens(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "visual.saga")
	assetName := writeEmbeddedSlideFixture(t, root)
	writeServerFile(t, filepath.Join(root, theme.FileName), testTheme)
	recorder := httptest.NewRecorder()
	newMux(&app{root: root}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/f/change/"+assetName, nil))
	if body := recorder.Body.String(); recorder.Code != http.StatusOK || !strings.Contains(body, "--bg:#102030;") || !strings.Contains(body, "Complex flow") {
		t.Fatalf("embedded slide asset lacks the theme: status=%d body=%s", recorder.Code, body)
	}
}

func TestInjectFrameThemePlacesTheStyleLast(t *testing.T) {
	for _, tc := range []struct{ kind, in, want string }{
		{"svg", `<svg><style>:root{--bg:#fff}</style><rect/></SVG>`, `<svg><style>:root{--bg:#fff}</style><rect/><style data-change-saga-theme="">X</style></SVG>`},
		{"html", `<html><head><style>a{}</style></head><body>b</body></html>`, `<html><head><style>a{}</style><style data-change-saga-theme="">X</style></head><body>b</body></html>`},
		{"html", `<p>b</p></body>`, `<p>b</p><style data-change-saga-theme="">X</style></body>`},
		{"html", `<p>b</p>`, `<p>b</p><style data-change-saga-theme="">X</style>`},
		{"svg", `<svg><rect/>`, `<svg><rect/>`},
	} {
		if got := string(injectFrameTheme([]byte(tc.in), tc.kind, "X")); got != tc.want {
			t.Errorf("inject %s %q = %q, want %q", tc.kind, tc.in, got, tc.want)
		}
	}
}

// The preview shows light and dark side by side, each pane declaring its
// scheme's resolved tokens, with a generated diagram served like a slide.
func TestThemePreviewShowsBothSchemes(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	handler := newMux(&app{root: root})
	get := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		securityHeaders(handler).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	if page := get("/theme").Body.String(); !strings.Contains(page, "this Saga has no <code>theme.css</code>") {
		t.Errorf("the preview does not say the defaults are shown")
	}

	writeServerFile(t, filepath.Join(root, theme.FileName), testTheme+":root[data-theme=dark] { --muted: #222; }\n")
	if page := get("/theme").Body.String(); !strings.Contains(page, "theme.css:4: a second :root[data-theme=dark] block") {
		t.Errorf("the preview does not name the theme's problem:\n%s", page)
	}

	writeServerFile(t, filepath.Join(root, theme.FileName), strings.Replace(testTheme, "--bg: #000;", "--bg: #000; --muted: #222;", 1))
	page := get("/theme").Body.String()
	for _, want := range []string{
		`data-scheme="light" style="--bg:#102030;`, `data-scheme="dark" style="--bg:#000000;`, "color-scheme:dark",
		`<img class="theme-diagram" src="/theme/diagram.svg"`, `class="btn btn-primary"`, "--diagram-purple-sticky",
		"dark: --muted #222222 on --bg #000000 is 1.", `<link rel="stylesheet" href="/theme.css">`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the preview lacks %q", want)
		}
	}
	diagramResponse := get("/theme/diagram.svg")
	if body := diagramResponse.Body.String(); !strings.Contains(body, `id="sticky-purple"`) || !strings.Contains(body, "--bg:#102030;") {
		t.Errorf("the preview diagram lacks its palette or the theme")
	}
	assertAuthoredContentPolicy(t, diagramResponse.Header())
}
