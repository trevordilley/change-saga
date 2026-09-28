package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
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

func contentETag(body []byte) string {
	digest := sha256.Sum256(body)
	return `"` + hex.EncodeToString(digest[:])[:20] + `"`
}
