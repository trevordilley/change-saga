package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// This diagnostic measures the validation cost required before sharing an
// immutable selected-file model. It deliberately does not introduce a cache.
func reviewCodeKeyExperiment(ctx context.Context, repo, target string, rng reviewstate.Range, catalog gitdiff.Catalog, file gitdiff.FileSummary) (string, error) {
	commands := [][]string{
		{"rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir", "--git-path", "objects"},
		{"config", "--null", "--list", "--includes"},
		{"check-attr", "-z", "--all", "--"},
		{"for-each-ref", "--format=%(refname) %(objectname)", "refs/replace/"},
	}
	for _, path := range []string{file.Path, file.OldPath, file.NewPath} {
		if path != "" {
			commands[2] = append(commands[2], path)
		}
	}
	var results [4]string
	var errors [4]error
	var group sync.WaitGroup
	for i, args := range commands {
		group.Add(1)
		go func() {
			defer group.Done()
			out, err := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...).Output()
			results[i], errors[i] = string(out), err
		}()
	}
	group.Wait()
	for _, err := range errors {
		if err != nil {
			return "", err
		}
	}
	var canonical []string
	for _, path := range strings.Split(strings.TrimSpace(results[0]), "\n") {
		path, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		canonical = append(canonical, path)
	}
	identity, err := json.Marshal([]any{"review-file-v1-unified20", canonical, results[1:], os.Environ(), target, rng, catalog, file})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(identity)), nil
}

func BenchmarkPRCodePages(b *testing.B) {
	root, repo, id := os.Getenv("PR_PERF_SAGA"), os.Getenv("PR_PERF_REPO"), os.Getenv("PR_PERF_REVIEW")
	if root == "" || repo == "" || id == "" {
		b.Skip("set PR_PERF_SAGA, PR_PERF_REPO and PR_PERF_REVIEW")
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		b.Fatalf("load: %v; valid: %v", err, validation.Valid)
	}
	review := document.FindReview(id)
	if review == nil {
		b.Fatal("review missing")
	}
	ctx := context.Background()
	rng, err := reviewstate.ResolveRange(ctx, repo, review)
	if err != nil {
		b.Fatal(err)
	}
	tmpl, err := newPageTemplate()
	if err != nil {
		b.Fatal(err)
	}
	application := &app{root: root, sourceDir: repo, template: tmpl}
	catalog, err := application.reviewCatalog(ctx, document, rng)
	if err != nil {
		b.Fatal(err)
	}
	path := "internal/server/reviews.go"
	file, ok := catalogFile(catalog, path)
	if !ok {
		b.Fatal("fixture must change internal/server/reviews.go")
	}
	target := saga.SagaTarget(document.Manifest.ID)
	b.Run("key_experiment", func(b *testing.B) {
		for b.Loop() {
			if _, err := reviewCodeKeyExperiment(ctx, repo, target, rng, catalog, file); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("file_model", func(b *testing.B) {
		for b.Loop() {
			changes, err := gitdiff.ReadFile(ctx, repo, catalog, file)
			if err != nil {
				b.Fatal(err)
			}
			makeFileViews(changes, target)
		}
	})
	for _, limit := range []int{50, 200} {
		b.Run(fmt.Sprintf("all_hunks_limit%d", limit), func(b *testing.B) {
			mux := newMux(application)
			for b.Loop() {
				cursor, returned, pages := "", 0, 0
				for {
					query := url.Values{"file": {path}, "limit": {fmt.Sprint(limit)}, "cursor": {cursor}}
					w := httptest.NewRecorder()
					mux.ServeHTTP(w, httptest.NewRequest("GET", reviewHref(id)+"/file-diff?"+query.Encode(), nil))
					if w.Code != 200 {
						b.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
					}
					var count int
					fmt.Sscan(w.Header().Get("X-Change-Saga-Returned"), &count)
					returned += count
					pages++
					cursor = w.Header().Get("X-Change-Saga-Next-Cursor")
					if cursor == "" {
						break
					}
				}
				b.ReportMetric(float64(returned), "rows/op")
				b.ReportMetric(float64(pages), "pages/op")
			}
		})
	}
}
