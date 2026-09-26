package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// measureHandler builds a fresh reviewer over the repository's app Saga,
// with every per-app cache empty.
func measureHandler(tb testing.TB) http.Handler {
	tb.Helper()
	tmpl, err := newPageTemplateFor(gitdiff.Range{})
	if err != nil {
		tb.Fatal(err)
	}
	return newMux(&app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl})
}

func measureGet(tb testing.TB, handler http.Handler, path string) (time.Duration, int, string) {
	tb.Helper()
	recorder := httptest.NewRecorder()
	start := time.Now()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return time.Since(start), recorder.Code, recorder.Body.String()
}

func firstLink(body, prefix string) string {
	pattern := regexp.MustCompile(`href="(` + regexp.QuoteMeta(prefix) + `[^"#?/]+)"`)
	if match := pattern.FindStringSubmatch(body); match != nil {
		return match[1]
	}
	return ""
}

func measureFeatureIDs(tb testing.TB) []string {
	tb.Helper()
	entries, err := os.ReadDir(filepath.Join(dogfoodSaga, "___features"))
	if err != nil {
		tb.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".feature") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".feature"))
		}
	}
	return ids
}

func TestMeasurePageRender(t *testing.T) {
	if os.Getenv("SAGA_MEASURE") == "" {
		t.Skip("set SAGA_MEASURE=1 to measure page render timings")
	}
	requireDogfoodSaga(t)
	discovery := measureHandler(t)
	var paths []string
	for _, id := range measureFeatureIDs(t) {
		paths = append(paths, "/features/"+id)
	}
	paths = append(paths, "/", "/features", "/personas")
	_, _, personas := measureGet(t, discovery, "/personas")
	if link := firstLink(personas, "/personas/"); link != "" {
		paths = append(paths, link)
	} else {
		t.Log("no persona link found in /personas")
	}
	_, _, stories := measureGet(t, discovery, "/requirements")
	if link := firstLink(stories, "/requirements/"); link != "" {
		paths = append(paths, link)
	} else {
		t.Log("no story link found in /requirements")
	}
	paths = append(paths, "/technical")

	t.Logf("%-45s %6s %10s %10s %9s", "path", "status", "cold ms", "warm ms", "body KB")
	for _, path := range paths {
		handler := measureHandler(t)
		cold, code, body := measureGet(t, handler, path)
		warm := make([]time.Duration, 10)
		for i := range warm {
			warm[i], _, _ = measureGet(t, handler, path)
		}
		if dir := os.Getenv("SAGA_MEASURE_DUMP"); dir != "" {
			name := strings.Trim(strings.ReplaceAll(path, "/", "_"), "_")
			if name == "" {
				name = "root"
			}
			if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		sort.Slice(warm, func(i, j int) bool { return warm[i] < warm[j] })
		median := (warm[4] + warm[5]) / 2
		t.Logf("%-45s %6d %10.1f %10.1f %9.1f", path, code,
			float64(cold.Microseconds())/1000, float64(median.Microseconds())/1000, float64(len(body))/1024)
	}
}

const benchmarkFeaturePath = "/features/visual-review"

// warmFeatureHandler is shared across the benchmark's b.N rounds so the one
// cold request that fills its caches is paid once per process.
var warmFeatureHandler http.Handler

func BenchmarkFeaturePageWarm(b *testing.B) {
	if _, err := os.Stat(filepath.Join(dogfoodSaga, "saga.json")); err != nil {
		b.Skip("app Saga absent")
	}
	if warmFeatureHandler == nil {
		warmFeatureHandler = measureHandler(b)
		if _, code, _ := measureGet(b, warmFeatureHandler, benchmarkFeaturePath); code != http.StatusOK {
			b.Fatalf("%s returned %d", benchmarkFeaturePath, code)
		}
	}
	handler := warmFeatureHandler
	b.ReportAllocs()
	b.ResetTimer()
	// The phase label lets pprof -tagfocus=phase=warm drop the cold fill.
	pprof.Do(context.Background(), pprof.Labels("phase", "warm"), func(context.Context) {
		for i := 0; i < b.N; i++ {
			measureGet(b, handler, benchmarkFeaturePath)
		}
	})
}

func BenchmarkFeaturePageCold(b *testing.B) {
	if _, err := os.Stat(filepath.Join(dogfoodSaga, "saga.json")); err != nil {
		b.Skip("app Saga absent")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		handler := measureHandler(b)
		b.StartTimer()
		pprof.Do(context.Background(), pprof.Labels("phase", "cold"), func(context.Context) {
			if _, code, _ := measureGet(b, handler, benchmarkFeaturePath); code != http.StatusOK {
				b.Fatalf("%s returned %d", benchmarkFeaturePath, code)
			}
		})
	}
}

// TestMeasureFeaturePhases times, by wall clock, the phases a cold feature
// page pays before rendering, then the per-request freshness checks a warm
// page repeats.
func TestMeasureFeaturePhases(t *testing.T) {
	if os.Getenv("SAGA_MEASURE") == "" {
		t.Skip("set SAGA_MEASURE=1 to measure page render phases")
	}
	requireDogfoodSaga(t)
	ctx := context.Background()
	tmpl, err := newPageTemplateFor(gitdiff.Range{})
	if err != nil {
		t.Fatal(err)
	}
	application := &app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl}
	lap := func(name string, run func()) {
		start := time.Now()
		run()
		t.Logf("%-48s %9.1f ms", name, float64(time.Since(start).Microseconds())/1000)
	}
	var document = application.outlineDocument(ctx)
	lap("cold outlineDocument (2nd call, warm)", func() { application.outlineDocument(ctx) })
	application = &app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl}
	handler := newMux(application)
	lap("cold outlineDocument", func() { document = application.outlineDocument(ctx) })
	files := application.sagaFiles(context.Background())
	lap("cold sagaFiles().narrative()", func() { document = files.narrative() })
	lap("cold sagaFiles().records()", func() { files.records(document.Manifest.ID) })
	lap("cold sagaFiles().tests()", func() { files.tests() })
	lap("cold sagaFiles().inventory()", func() { files.inventory(document.Manifest.ID) })
	lap("cold relatedReviews", func() { application.relatedReviews(ctx) })
	lap("warm relatedReviews", func() { application.relatedReviews(ctx) })
	lap("relatedReviews rebuild, reviews' diffs kept", func() {
		application.related.index = nil
		application.relatedReviews(ctx)
	})
	lap("first page after the above phases", func() { measureGet(t, handler, benchmarkFeaturePath) })
	lap("warm page", func() { measureGet(t, handler, benchmarkFeaturePath) })
	for i := 0; i < 3; i++ {
		lap("sagaFingerprints walk", func() { sagaFingerprints(dogfoodSaga, true) })
		lap("freshness check (walk + both heads)", func() { application.checkSaga(ctx, time.Now(), true) })
		lap("git rev-parse HEAD", func() { gitOutput(ctx, dogfoodSaga, "rev-parse", "HEAD") })
	}
}

// servedApp is the app Saga as a running server holds it: watched, and read
// in the background before any request.
func servedApp(tb testing.TB) (*app, http.Handler, context.CancelFunc) {
	tb.Helper()
	tmpl, err := newPageTemplateFor(gitdiff.Range{})
	if err != nil {
		tb.Fatal(err)
	}
	application := &app{root: dogfoodSaga, sourceDir: filepath.Join("..", ".."), template: tmpl}
	ctx, cancel := context.WithCancel(context.Background())
	go application.watchSaga(ctx)
	deadline := time.Now().Add(time.Minute)
	for {
		application.related.mutex.Lock()
		built := application.related.builds
		application.related.mutex.Unlock()
		if built > 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			tb.Fatal("the served app never read the Saga in the background")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return application, newMux(application), cancel
}

// BenchmarkFeaturePageServed is a feature page from a running server, which
// checks the Saga in the background rather than per request.
func BenchmarkFeaturePageServed(b *testing.B) {
	if _, err := os.Stat(filepath.Join(dogfoodSaga, "saga.json")); err != nil {
		b.Skip("app Saga absent")
	}
	_, handler, cancel := servedApp(b)
	defer cancel()
	measureGet(b, handler, benchmarkFeaturePath)
	b.ReportAllocs()
	b.ResetTimer()
	pprof.Do(context.Background(), pprof.Labels("phase", "served"), func(context.Context) {
		for i := 0; i < b.N; i++ {
			measureGet(b, handler, benchmarkFeaturePath)
		}
	})
}

// TestMeasureServedEdit times a running server's feature page after an edit
// to a feature's story, once the watcher has seen it, beside the first page
// of a fresh server.
func TestMeasureServedEdit(t *testing.T) {
	if os.Getenv("SAGA_MEASURE") == "" {
		t.Skip("set SAGA_MEASURE=1 to measure")
	}
	requireDogfoodSaga(t)
	started := time.Now()
	application, handler, cancel := servedApp(t)
	defer cancel()
	t.Logf("background read of the Saga at startup: %s", time.Since(started).Round(time.Millisecond))
	elapsed, _, _ := measureGet(t, handler, benchmarkFeaturePath)
	t.Logf("first feature page after the background read: %s", elapsed.Round(time.Millisecond))
	stories, err := filepath.Glob(filepath.Join(dogfoodSaga, "___features", "*", "___requirements", "stories", "*", "story.json"))
	if err != nil || len(stories) == 0 {
		t.Fatalf("no story to edit: %v", err)
	}
	edited := stories[0]
	info, err := os.Stat(edited)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chtimes(edited, info.ModTime(), info.ModTime())
	for round := 1; round <= 3; round++ {
		touched := info.ModTime().Add(time.Duration(round) * time.Second)
		if err := os.Chtimes(edited, touched, touched); err != nil {
			t.Fatal(err)
		}
		before := application.related.builds
		saved := time.Now()
		for application.related.builds == before && time.Since(saved) < 30*time.Second {
			time.Sleep(5 * time.Millisecond)
		}
		caughtUp := time.Since(saved)
		// The warming page goes on to build the decks after the related
		// reviews; wait for it to finish so the page measured is the next
		// reviewer's.
		for application.files.current == nil || application.files.current.slidesView == nil {
			time.Sleep(5 * time.Millisecond)
		}
		caughtUp = time.Since(saved)
		elapsed, _, _ := measureGet(t, handler, benchmarkFeaturePath)
		t.Logf("edit %d: watcher rebuilt within %s of the save; the next feature page took %s", round, caughtUp.Round(time.Millisecond), elapsed.Round(time.Millisecond))
	}
}
