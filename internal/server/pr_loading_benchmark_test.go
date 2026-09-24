package server

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// BenchmarkPRLoading profiles the actual review route and its major stages.
// Supply a disposable, CLI-authored Saga copy and its separate source checkout.
// No records are written. Setup is excluded; fresh_app resets application caches,
// not OS filesystem/Git caches. Run stages separately for interpretable profiles.
func BenchmarkPRLoading(b *testing.B) {
	root, repo, id := os.Getenv("PR_PERF_SAGA"), os.Getenv("PR_PERF_REPO"), os.Getenv("PR_PERF_REVIEW")
	if root == "" || repo == "" || id == "" {
		b.Skip("set PR_PERF_SAGA, PR_PERF_REPO and PR_PERF_REVIEW")
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		b.Fatalf("load: %v; validation: %+v", err, validation)
	}
	review := doc.FindReview(id)
	if review == nil || review.Deck == nil {
		b.Fatal("review with slide deck required")
	}
	tmpl, err := newPageTemplate()
	if err != nil {
		b.Fatal(err)
	}
	newApp := func() *app { return &app{root: root, sourceDir: repo, template: tmpl} }
	application := newApp()
	ctx := context.Background()
	report := application.reviewReports(ctx, doc, []*saga.Review{review})[0]
	if report.Range == nil {
		b.Fatalf("range unavailable: %v", report.Diagnostics)
	}
	request := func(b *testing.B, a *app) {
		b.Helper()
		w := httptest.NewRecorder()
		newMux(a).ServeHTTP(w, httptest.NewRequest("GET", reviewHref(id), nil))
		if w.Code != 200 {
			b.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
		}
		b.ReportMetric(float64(w.Body.Len()), "response-B")
	}
	b.Run("fresh_app", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			request(b, newApp())
		}
	})
	b.Run("warm_page", func(b *testing.B) {
		request(b, application)
		b.ReportAllocs()
		for b.Loop() {
			request(b, application)
		}
	})
	b.Run("load_validate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, v, err := saga.Load(root)
			if err != nil || !v.Valid {
				b.Fatalf("load: %v; valid: %v", err, v.Valid)
			}
		}
	})
	b.Run("report_coverage_currency", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			application.reviewReports(ctx, doc, []*saga.Review{review})
		}
	})
	b.Run("item_reference_diffs", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			resolver, err := coderesolve.New(ctx, repo)
			if err != nil {
				b.Fatal(err)
			}
			for _, slide := range review.Deck.Slides {
				for _, item := range slide.Items {
					for _, file := range item.Code {
						for _, ref := range file.References {
							application.referenceDiff(ctx, resolver, ref, *report.Range)
						}
					}
				}
			}
			resolver.Close()
		}
	})
	b.Run("warm_shell", func(b *testing.B) {
		r := httptest.NewRequest("GET", reviewHref(id), nil)
		if _, err := application.shell(r); err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			if _, err := application.shell(r); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("shell_template", func(b *testing.B) {
		data, err := application.shell(httptest.NewRequest("GET", reviewHref(id), nil))
		if err != nil {
			b.Fatal(err)
		}
		b.ReportAllocs()
		for b.Loop() {
			if err := tmpl.ExecuteTemplate(io.Discard, "page", data); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkPRReferenceMemoExperiment is an isolated proposal, not a production
// cache. Each operation owns its resolver and patch map; no state survives the
// request. It checks every result against the current implementation first.
func BenchmarkPRReferenceMemoExperiment(b *testing.B) {
	root, repo, id := os.Getenv("PR_PERF_SAGA"), os.Getenv("PR_PERF_REPO"), os.Getenv("PR_PERF_REVIEW")
	if root == "" || repo == "" || id == "" {
		b.Skip("set PR_PERF_SAGA, PR_PERF_REPO and PR_PERF_REVIEW")
	}
	doc, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		b.Fatalf("load: %v; valid: %v", err, validation.Valid)
	}
	review := doc.FindReview(id)
	if review == nil || review.Deck == nil {
		b.Fatal("review with slide deck required")
	}
	ctx := context.Background()
	rng, err := reviewstate.ResolveRange(ctx, repo, review)
	if err != nil {
		b.Fatal(err)
	}
	var refs []coderef.Reference
	for _, slide := range review.Deck.Slides {
		for _, item := range slide.Items {
			for _, file := range item.Code {
				refs = append(refs, file.References...)
			}
		}
	}
	if len(refs) == 0 {
		b.Fatal("fixture must have exact references")
	}
	application := &app{root: root, sourceDir: repo}
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		b.Fatal(err)
	}
	want := make([]*reviewDiffView, len(refs))
	for i, ref := range refs {
		want[i] = application.referenceDiff(ctx, resolver, ref, rng)
	}
	resolver.Close()
	run := func() []*reviewDiffView {
		resolver, err := coderesolve.New(ctx, repo)
		if err != nil {
			b.Fatal(err)
		}
		defer resolver.Close()
		top, err := gitOutput(ctx, repo, "rev-parse", "--show-toplevel")
		if err != nil {
			b.Fatal(err)
		}
		patches := map[string]string{}
		result := make([]*reviewDiffView, 0, len(refs))
		for _, ref := range refs {
			view := &reviewDiffView{Path: ref.Path, Location: ref.Location().String()}
			path, start, end := ref.Path, ref.Start, ref.End
			if resolution := resolver.Resolve(ctx, ref, rng.HeadOID); resolution.Current() {
				path, start, end = resolution.Location.Path, resolution.Location.Start, resolution.Location.End
			} else if !ref.WholeFile() {
				view.Note = "The referenced lines changed after the reference was written; showing every change to the file."
				start, end = 0, 0
			}
			patch, ok := patches[path]
			if !ok {
				patch, err = gitdiff.FileDiff(ctx, top, rng.BaseOID, rng.HeadOID, path)
				if err != nil {
					b.Fatal(err)
				} // Failure semantics need a production regression test.
				patches[path] = patch
			}
			view.Path = path
			view.Lines = diffLinesTouching(patch, start, end)
			if len(view.Lines) == 0 {
				if start > 0 {
					view.Note = fmt.Sprintf("Lines %d-%d are unchanged between the base and the head.", start, end)
				} else if view.Note == "" {
					view.Note = "The file is unchanged between the base and the head."
				}
			}
			result = append(result, view)
		}
		return result
	}
	if got := run(); !reflect.DeepEqual(got, want) {
		b.Fatal("memo proposal changed exact diff views")
	}
	b.ReportAllocs()
	for b.Loop() {
		run()
	}
	b.ReportMetric(float64(len(refs)), "references/op")
}
