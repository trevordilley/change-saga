package server

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/theme"
)

// Slides follow the app's light and dark mode. A generated diagram declares
// its tokens for both schemes and switches on prefers-color-scheme, which a
// standalone SVG takes from the OS. In a slide frame the reviewer's manual
// toggle must win too, but a sandboxed frame's prefers-color-scheme follows
// its element's color-scheme only in some browsers, so the frame's URL names
// the scheme instead: ?saga_scheme=light or dark. A slide visual served with
// one gets a style appended that declares every token for that scheme, which
// outranks the drawing's own declarations and its media rule by coming last.

const slideSchemeParam = "saga_scheme"

// slideScheme is the scheme a slide visual request forces, or "" to follow
// the OS.
func slideScheme(r *http.Request) string {
	switch scheme := r.URL.Query().Get(slideSchemeParam); scheme {
	case "light", "dark":
		return scheme
	}
	return ""
}

// slideVisualStyle is the style text appended to a slide visual served for
// scheme, or "" when nothing is appended.
func slideVisualStyle(scheme string) string {
	if scheme == "" {
		return ""
	}
	declarations := theme.Declarations("light")
	if scheme == "dark" {
		declarations += theme.Declarations("dark")
	}
	return ":root{color-scheme:" + scheme + ";" + declarations + "}"
}

// injectSlideStyle inserts style as the last thing an SVG or HTML document
// styles: before its final </svg>, else before </head>, else at the end.
func injectSlideStyle(data []byte, contentType, style string) []byte {
	element := []byte(`<style data-change-saga-scheme>` + style + `</style>`)
	lower := bytes.ToLower(data)
	at := len(data)
	switch {
	case strings.HasPrefix(contentType, "image/svg+xml"):
		if index := bytes.LastIndex(lower, []byte("</svg>")); index >= 0 {
			at = index
		}
	case strings.HasPrefix(contentType, "text/html"):
		if index := bytes.Index(lower, []byte("</head>")); index >= 0 {
			at = index
		}
	default:
		return data
	}
	out := make([]byte, 0, len(data)+len(element))
	out = append(out, data[:at]...)
	out = append(out, element...)
	return append(out, data[at:]...)
}

// serveSlideVisual answers a slide visual or fragment file request, appending
// the requested scheme's style to an SVG or HTML document.
func serveSlideVisual(w http.ResponseWriter, r *http.Request, name, contentType string, modified time.Time, file *os.File) {
	style := slideVisualStyle(slideScheme(r))
	if style == "" || !(strings.HasPrefix(contentType, "image/svg+xml") || strings.HasPrefix(contentType, "text/html")) {
		http.ServeContent(w, r, name, modified, file)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "the slide could not be read", http.StatusInternalServerError)
		return
	}
	// The appended style is not part of the file, so the file's modification
	// time cannot validate the response.
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(injectSlideStyle(data, contentType, style)))
}
