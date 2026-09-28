package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
// the scheme instead: ?saga_scheme=light or dark. Every SVG or HTML slide
// visual is served with a style appended that declares every token, for the
// named scheme or else for both schemes under prefers-color-scheme. Coming
// last, it outranks the drawing's own declarations and media rule, and a
// hand-authored slide may paint with any token.

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
// scheme, "" to follow the OS, with the Saga's theme, if any, applied.
func slideVisualStyle(scheme string, sagaTheme *theme.File) string {
	switch scheme {
	case "light", "dark":
		return ":root{color-scheme:" + scheme + ";" + sagaTheme.Declare(scheme) + "}"
	}
	return sagaTheme.FrameCSS()
}

// injectSlideStyle inserts style as the last thing an SVG or HTML document
// styles: before its final </svg>, else before </head>, else at the end.
// The element must also be well-formed XML, so its attribute has a value.
func injectSlideStyle(data []byte, contentType, style string) []byte {
	element := []byte(`<style data-change-saga-scheme="">` + style + `</style>`)
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
// the requested scheme's style, with the Saga's theme applied, to an SVG or
// HTML document. The committed bytes, and every digest and approval over
// them, are unchanged; only the response carries the style.
func serveSlideVisual(w http.ResponseWriter, r *http.Request, name, contentType string, modified time.Time, file *os.File, sagaTheme *theme.File) {
	if !strings.HasPrefix(contentType, "image/svg+xml") && !strings.HasPrefix(contentType, "text/html") {
		http.ServeContent(w, r, name, modified, file)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "the slide could not be read", http.StatusInternalServerError)
		return
	}
	data = injectSlideStyle(data, contentType, slideVisualStyle(slideScheme(r), sagaTheme))
	// The appended style is not part of the file, so the file's modification
	// time cannot validate the response; a digest of the bytes served can.
	digest := sha256.Sum256(data)
	w.Header().Set("ETag", `"`+hex.EncodeToString(digest[:16])+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}
