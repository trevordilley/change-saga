package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/theme"
)

// sagaTheme is the Saga's validated theme, or nil when it has none. An
// invalid theme is not applied at all: change-saga validate names its
// problems, and the reviewer keeps its defaults meanwhile.
func (a *app) sagaTheme() *theme.File {
	file, err := theme.Load(a.root)
	if err != nil {
		return nil
	}
	return file
}

// themeStylesheet serves the theme's overrides, linked right after app.css
// so they follow the default tokens. They are re-serialized from the
// validated file, never its raw text. The page's markup does not change
// with the theme, so a theme edit reaches the next full page load without
// touching any page's cache.
func (a *app) themeStylesheet(w http.ResponseWriter, r *http.Request) {
	body := []byte(a.sagaTheme().OverrideCSS())
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", contentETag(body))
	http.ServeContent(w, r, theme.FileName, time.Time{}, bytes.NewReader(body))
}

// frameThemeMarker marks the token style injected into a served visual.
const frameThemeMarker = "data-change-saga-theme"

// serveVisual serves a slide or fragment file. An SVG or HTML document gets
// every token declared, with the theme applied, in one style element at its
// end, so hand-authored visuals that use var(--token) and generated ones
// that declare their own defaults both follow the theme: same specificity,
// later rule. The committed bytes, and every digest and approval over them,
// are unchanged; only the response carries the style.
func serveVisual(w http.ResponseWriter, r *http.Request, path string, file *os.File, info os.FileInfo, frameCSS string) {
	kind := visualDocumentKind(path)
	if kind == "" {
		http.ServeContent(w, r, filepath.Base(path), info.ModTime(), file)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "the visual could not be read", http.StatusInternalServerError)
		return
	}
	body := injectFrameTheme(data, kind, frameCSS)
	w.Header().Set("ETag", contentETag(body))
	http.ServeContent(w, r, filepath.Base(path), time.Time{}, bytes.NewReader(body))
}

func visualDocumentKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg":
		return "svg"
	case ".html", ".htm":
		return "html"
	}
	return ""
}

// injectFrameTheme places the token style just before an SVG's closing tag,
// or an HTML document's </head> (else </body>, else its end). A malformed
// SVG with no closing tag is served as it is.
func injectFrameTheme(data []byte, kind, css string) []byte {
	style := []byte("<style " + frameThemeMarker + "=\"\">" + css + "</style>")
	lower := bytes.ToLower(data)
	at := -1
	if kind == "svg" {
		at = bytes.LastIndex(lower, []byte("</svg"))
	} else if at = bytes.Index(lower, []byte("</head")); at < 0 {
		if at = bytes.LastIndex(lower, []byte("</body")); at < 0 {
			at = len(data)
		}
	}
	if at < 0 {
		return data
	}
	out := make([]byte, 0, len(data)+len(style))
	out = append(out, data[:at]...)
	out = append(out, style...)
	return append(out, data[at:]...)
}

func contentETag(body []byte) string {
	digest := sha256.Sum256(body)
	return `"` + hex.EncodeToString(digest[:])[:20] + `"`
}
