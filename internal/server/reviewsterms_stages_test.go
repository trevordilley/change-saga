package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// TestMeasureReviewIndexStages times what /reviews does, stage by stage.
func TestMeasureReviewIndexStages(t *testing.T) {
	if os.Getenv("SAGA_MEASURE") == "" {
		t.Skip("set SAGA_MEASURE=1 to measure")
	}
	requireDogfoodSaga(t)
	application, _, cancel := servedApp(t)
	defer cancel()
	ctx := context.Background()
	for round := 0; round < 3; round++ {
		start := time.Now()
		document, _, err := saga.Load(dogfoodSaga)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("saga.Load: %s (%d reviews)", time.Since(start).Round(time.Millisecond), len(document.Reviews))
		start = time.Now()
		resolver, _ := coderesolve.New(ctx, application.sourceDir)
		t.Logf("coderesolve.New: %s", time.Since(start).Round(time.Millisecond))
		var resolve, build time.Duration
		for _, review := range document.Reviews {
			start = time.Now()
			reviewstate.ResolveRange(ctx, application.sourceDir, review)
			resolve += time.Since(start)
			start = time.Now()
			reviewstate.Build(ctx, review, reviewstate.Options{Checkout: application.sourceDir, SagaRoot: document.Root, Resolver: resolver, Repository: document.Manifest.Source.Repository})
			build += time.Since(start)
		}
		t.Logf("ResolveRange total: %s; Build total (includes a second resolve): %s", resolve.Round(time.Millisecond), build.Round(time.Millisecond))
		for _, review := range document.Reviews {
			start = time.Now()
			reviewstate.Build(ctx, review, reviewstate.Options{Checkout: application.sourceDir, SagaRoot: document.Root, Resolver: resolver, Repository: document.Manifest.Source.Repository, SkipCoverage: true})
			t.Logf("  %s build without coverage: %s", review.ID, time.Since(start).Round(time.Millisecond))
		}
		resolver.Close()
	}
}
