package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// disconnectedWriter is a browser that went away: every body write fails.
// It counts status writes, which net/http reports as superfluous after the
// first.
type disconnectedWriter struct {
	header  http.Header
	headers int
}

func (w *disconnectedWriter) Header() http.Header { return w.header }
func (w *disconnectedWriter) WriteHeader(int)     { w.headers++ }

// Write sends the implicit 200 first, as net/http does.
func (w *disconnectedWriter) Write([]byte) (int, error) {
	if w.headers == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return 0, errors.New("broken pipe")
}

// A page whose client disconnects mid-response writes its status once. The
// page used to report the failed write with http.Error, a second WriteHeader
// that the server logged as superfluous.
func TestPageWritesItsStatusOnceWhenTheClientGoesAway(t *testing.T) {
	root := validServerSaga(t)
	writer := &disconnectedWriter{header: http.Header{}}
	(&app{root: root, sourceDir: root, template: serverTemplate(t)}).page(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	if writer.headers > 1 {
		t.Fatalf("page wrote its status %d times", writer.headers)
	}
}
