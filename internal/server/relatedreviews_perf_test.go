package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/reviewstore"

	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Related reviews are a cross product of records and reviews, so their cost is
// measured on this repository's own app Saga rather than asserted. A
// documentation page asks for the index once and holds it until the Saga or
// the source head changes, so the first build is the whole cost.
func TestRelatedReviewsCostOnThisRepository(t *testing.T) {
	t.Parallel()
	requireDogfoodSaga(t)
	document, _, err := saga.Load(dogfoodSaga)
	if err != nil {
		t.Fatal(err)
	}
	records, err := requirements.Load(dogfoodSaga, document.Manifest.ID)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := quality.Load(dogfoodSaga)
	if err != nil {
		tests = quality.Document{SagaID: document.Manifest.ID}
	}
	application := &app{root: dogfoodSaga, sourceDir: ".."}
	started := time.Now()
	if _, err := application.relatedFingerprint(requestContext(t)); err != nil {
		t.Fatal(err)
	}
	fingerprint := time.Since(started)
	started = time.Now()
	if _, err := application.outlineFingerprint(requestContext(t)); err != nil {
		t.Fatal(err)
	}
	t.Logf("freshness check: related %s, the shell's own outline %s", fingerprint, time.Since(started))
	started = time.Now()
	index := application.relatedReviews(requestContext(t), document, records)
	cold := time.Since(started)
	started = time.Now()
	application.relatedReviews(requestContext(t), document, records)
	warm := time.Since(started)
	t.Logf("app.saga: %d features, %d stories, %d test cases, %d reviews; %d code references; cold %s (intersection %s), warm %s, builds %d",
		len(document.Features), len(records.Stories), len(tests.TestCases), len(document.Reviews),
		documentedReferenceCount(document), cold, index.Elapsed, warm, application.related.builds)
	if len(document.Reviews) == 0 && cold > 100*time.Millisecond {
		t.Fatalf("a Saga with no reviews paid %s for related reviews", cold)
	}

	// A review over the range that last changed the file app.saga documents,
	// so the measurement includes the work of an intersection that hits.
	touching := copyDogfoodSagaWithReview(t, "pr-resolve", lastCommitTouching(t, "internal/coderesolve/resolve.go"))
	touched, _, err := saga.Load(touching)
	if err != nil {
		t.Fatal(err)
	}
	hitting := &app{root: touching, sourceDir: ".."}
	started = time.Now()
	hit := hitting.relatedReviews(requestContext(t), touched, records)
	t.Logf("app.saga + 1 review of the commit that changed the documented file: cold %s, %d records list it", time.Since(started), len(hit.byRecord))
	if len(hit.byRecord) == 0 {
		t.Fatal("a review of the very commit that changed the documented code was related to nothing")
	}

	// app.saga has no reviews of its own yet, so the cross product is measured
	// over a copy of it given real pull-request-sized ranges of this repository.
	for _, reviews := range []int{1, 5, 20} {
		root := copyDogfoodSagaWithReviews(t, reviews)
		copied, _, err := saga.Load(root)
		if err != nil {
			t.Fatal(err)
		}
		measured := &app{root: root, sourceDir: ".."}
		started := time.Now()
		built := measured.relatedReviews(requestContext(t), copied, records)
		elapsed := time.Since(started)
		started = time.Now()
		measured.relatedReviews(requestContext(t), copied, records)
		t.Logf("app.saga + %d reviews: cold %s (intersection %s, %d records with a review), warm %s",
			reviews, elapsed, built.Elapsed, len(built.byRecord), time.Since(started))
	}
}

// lastCommitTouching is the most recent commit that changed path.
func lastCommitTouching(t *testing.T, path string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", filepath.Join("..", ".."), "log", "--format=%H", "-1", "--", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(output))
	if commit == "" {
		t.Fatalf("no commit has touched %s", path)
	}
	return commit
}

// copyDogfoodSagaWithReview is app.saga with one review of exactly commit.
func copyDogfoodSagaWithReview(t *testing.T, id, commit string) string {
	t.Helper()
	root := copySaga(t)
	if err := reviewstore.Create(root, saga.ReviewManifest{
		ID: id, Title: "Review of " + id, Base: commit + "^", Head: commit,
		PullRequest: &saga.PullRequest{Number: 99},
	}, saga.DeckManifest{ID: id, Title: "Measured " + id, Objective: "Measure the intersection."}); err != nil {
		t.Fatal(err)
	}
	return root
}

// copySaga is a writable copy of this repository's own app Saga. It lives
// in a short temporary directory rather than t.TempDir, which is named after
// the test: under macOS's long temporary root that name alone pushes the
// copy's deck files past the portable path limit the Saga enforces, and the
// copy would not validate.
func copySaga(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "saga")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	root := filepath.Join(dir, "app.saga")
	if err := os.CopyFS(root, os.DirFS(dogfoodSaga)); err != nil {
		t.Fatal(err)
	}
	return root
}

// copyDogfoodSagaWithReviews is app.saga with count reviews, each over a real
// range of this repository. Only the range matters to the intersection: the
// deck is what a review explains, not what it changed.
func copyDogfoodSagaWithReviews(t *testing.T, count int) string {
	t.Helper()
	root := copySaga(t)
	for index := 0; index < count; index++ {
		id := fmt.Sprintf("pr-%d", index+1)
		base := fmt.Sprintf("HEAD~%d", index+2)
		if err := reviewstore.Create(root, saga.ReviewManifest{
			ID: id, Title: "Measured review " + id, Base: base, Head: "HEAD",
			PullRequest: &saga.PullRequest{Number: index + 1},
		}, saga.DeckManifest{ID: id, Title: "Measured " + id, Objective: "Measure the intersection."}); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func documentedReferenceCount(document *saga.Saga) int {
	count := 0
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		for _, file := range section.Code {
			count += len(file.References)
		}
		for _, fragment := range section.Fragments {
			for _, file := range fragment.Code {
				count += len(file.References)
			}
			for _, landmark := range fragment.Landmarks {
				for _, file := range landmark.Code {
					count += len(file.References)
				}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	if document.Section != nil {
		walk(document.Section)
	}
	return count
}

// requestContext is the context a request handler gets: its own Git session,
// ended when the test does. The cost measured is the cost a reviewer pays.
func requestContext(t *testing.T) context.Context {
	ctx, end := gitexec.BeginIsolated(context.Background())
	t.Cleanup(end)
	return ctx
}
