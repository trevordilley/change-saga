package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime/pprof"
	"testing"
)

// benchmarkServedPath times one page of a running server over the app Saga,
// whole or as the partial a link swaps in, labelled for pprof -tagfocus.
func benchmarkServedPath(b *testing.B, path string, partial bool) {
	if _, err := os.Stat(filepath.Join(dogfoodSaga, "saga.json")); err != nil {
		b.Skip("app Saga absent")
	}
	_, handler, cancel := servedApp(b)
	defer cancel()
	get := func() {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		if partial {
			request.Header.Set("HX-Request", "true")
			request.Header.Set("HX-Target", "page")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			b.Fatalf("GET %s = %d", path, recorder.Code)
		}
	}
	get()
	phase := path + " full"
	if partial {
		phase = path + " partial"
	}
	b.ReportAllocs()
	b.ResetTimer()
	pprof.Do(context.Background(), pprof.Labels("phase", phase), func(context.Context) {
		for i := 0; i < b.N; i++ {
			get()
		}
	})
}

func BenchmarkReviewsServed(b *testing.B)        { benchmarkServedPath(b, "/reviews", false) }
func BenchmarkReviewsServedPartial(b *testing.B) { benchmarkServedPath(b, "/reviews", true) }
func BenchmarkTermsServed(b *testing.B)          { benchmarkServedPath(b, "/terms", false) }
func BenchmarkTermsServedPartial(b *testing.B)   { benchmarkServedPath(b, "/terms", true) }
