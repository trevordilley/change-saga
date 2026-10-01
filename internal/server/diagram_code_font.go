package server

import (
	"net/http"

	"github.com/twentyideas/changesaga/internal/diagram"
)

func (a *app) diagramMonoFont(w http.ResponseWriter, _ *http.Request) {
	data, _ := diagram.MonoFont()
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
