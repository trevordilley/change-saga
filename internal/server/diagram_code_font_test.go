package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
)

func TestCodeExampleFontServedToSandboxedSlides(t *testing.T) {
	recorder := httptest.NewRecorder()
	newMux(&app{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, diagram.MonoFontPath, nil))
	want, _ := diagram.MonoFont()
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "font/ttf" || !bytes.Equal(recorder.Body.Bytes(), want) || recorder.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("font unavailable to sandboxed frame: status=%d headers=%v", recorder.Code, recorder.Header())
	}
}
