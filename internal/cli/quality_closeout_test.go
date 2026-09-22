package cli

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
)

func TestFailedClaimVerificationRemainsVisibleInHistory(t *testing.T) {
	root := newAuthoredSaga(t)
	repo, commit := sourceRepo(t, map[string]string{
		"worker.go": "package worker\n\nfunc Run() error { return nil }\n",
	})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")

	var output bytes.Buffer
	if err := AddClaim(context.Background(), []string{
		"--id", "worker-runs", "--target", "___overview/description.fragment", "--kind", "behavior",
		"--statement", "The worker completes successfully.", "--repo", repo,
		"--ref", commit + ":worker.go#L3", root,
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaim(context.Background(), []string{
		"--id", "worker-failed", "--claim", "worker-runs", "--status", "failed",
		"--method", "test", "--summary", "The worker test failed.", "--command", "go test ./...", root,
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := VerifyClaim(context.Background(), []string{
		"--id", "worker-passed", "--claim", "worker-runs", "--status", "verified",
		"--method", "test", "--summary", "The worker test passed after the fix.", "--command", "go test ./...", root,
	}, &output); err != nil {
		t.Fatal(err)
	}

	data := queryData(t, "verifications", "--saga", root, "--repo", repo, "--claim", "worker-runs")
	records, ok := data["verifications"].([]any)
	if !ok || len(records) != 2 {
		t.Fatalf("verification history = %#v", data["verifications"])
	}
	statuses := map[string]bool{}
	for _, value := range records {
		record := value.(map[string]any)
		statuses[record["status"].(string)] = true
	}
	if !statuses["failed"] || !statuses["verified"] {
		t.Fatalf("verification statuses = %#v; failed result disappeared from history", statuses)
	}
}

func TestSelectedComparisonScopesStatusQueryAndReviewer(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\nfunc Enqueue(job string) error { return retry(3, func() error { return sqs.Send(job) }) }\n")
	git(t, repo, "commit", "-am", "Retry enqueue")

	statusReport, _ := statusLayers(t, root, "--against", "main")
	if statusReport.Opening.Mode != gitdiff.ModeCompare || statusReport.Opening.Against != "main" || statusReport.Opening.Head != "HEAD" {
		t.Fatalf("status opening = %#v", statusReport.Opening)
	}
	query, queryStatus, body := runRealQuery(t, []string{"overview", "--saga", root, "--against", "main"})
	if queryStatus != 0 || !query.OK {
		t.Fatalf("query status=%d body=%s", queryStatus, body)
	}
	source := query.Data.(map[string]any)["source"].(map[string]any)
	if source["mode"] != gitdiff.ModeCompare || source["base"] != "main" || source["head"] != "HEAD" ||
		source["base_oid"] != statusReport.Opening.BaseOID || source["head_oid"] != statusReport.Opening.HeadOID {
		t.Fatalf("status opening %#v and query source %#v selected different comparisons", statusReport.Opening, source)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	served := make(chan error, 1)
	go func() {
		served <- reviewserver.ListenManaged(ctx, root, repo, "127.0.0.1:0", false, io.Discard, reviewserver.ManagedOptions{
			Range: gitdiff.Range{Against: "main"},
			OnReady: func(url string) error {
				ready <- url
				return nil
			},
		})
	}()
	var serverURL string
	select {
	case serverURL = <-ready:
	case err := <-served:
		t.Fatalf("review server failed before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("review server did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		if err := <-served; err != nil {
			t.Errorf("review server shutdown: %v", err)
		}
	})

	var responseBody string
	for attempt := 0; attempt < 100; attempt++ {
		response, err := http.Get(serverURL + "/api/change")
		if err != nil {
			t.Fatal(err)
		}
		payload, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode == http.StatusOK {
			responseBody = string(payload)
			break
		}
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("review change status=%d body=%s", response.StatusCode, payload)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if responseBody == "" {
		t.Fatal("review change did not finish building")
	}
	if !strings.Contains(responseBody, "<h1>HEAD against main</h1>") || !strings.Contains(responseBody, statusReport.Opening.BaseOID[:12]) {
		t.Fatalf("review selected a different comparison: %s", responseBody)
	}
}
