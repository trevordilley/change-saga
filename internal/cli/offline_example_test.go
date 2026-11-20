package cli

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/releasearchive"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
)

func TestOfflineExampleArchiveNeedsNoCheckout(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "example.zip")
	source := filepath.Join("..", "..", "examples", "offline", "change.saga")
	if err := releasearchive.WriteTreeZip(archive, 1_700_000_000, source, "change.saga"); err != nil {
		t.Fatal(err)
	}
	unpack := t.TempDir()
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if !strings.HasPrefix(entry.Name, "change.saga/") || strings.Contains(entry.Name, "/revisions/") || strings.Contains(entry.Name, "/.git/") {
			t.Fatalf("unexpected example history/path: %s", entry.Name)
		}
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(unpack, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command("git", "-C", unpack, "rev-parse", "--show-toplevel").CombinedOutput(); err == nil {
		t.Fatalf("archive test unexpectedly has a Git checkout: %s", output)
	}
	root := filepath.Join(unpack, "change.saga")
	before := snapshotFiles(t, root)
	var output bytes.Buffer
	if err := Doctor(context.Background(), []string{"--json", root}, &output); err != nil {
		t.Fatalf("offline validation: %v %s", err, &output)
	}
	var report doctorReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil || report.SagaCheck == nil || !report.SagaCheck.Valid || len(report.SagaCheck.Issues) != 0 {
		t.Fatalf("example is not clean: %v %s", err, &output)
	}
	output.Reset()
	if err := Query(context.Background(), []string{"plan", "--saga", root, "--plan", "arrival-window"}, &output); err != nil {
		t.Fatalf("offline Plan query: %v %s", err, &output)
	}
	if !strings.Contains(output.String(), `"checkpoint"`) || strings.Contains(output.String(), `"passed"`) {
		t.Fatal("the example lost or passed its review checkpoint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, finished := make(chan string, 1), make(chan error, 1)
	go func() {
		finished <- reviewserver.ListenManaged(ctx, root, root, "127.0.0.1:0", false, io.Discard, reviewserver.ManagedOptions{OnReady: func(url string) error { ready <- url; return nil }})
	}()
	var baseURL string
	select {
	case baseURL = <-ready:
	case err := <-finished:
		t.Fatalf("offline reader startup: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("offline reader did not start")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-finished:
			if err != nil {
				t.Errorf("stop offline reader: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("offline reader did not stop")
		}
	})
	client := &http.Client{Timeout: 15 * time.Second}
	for path, text := range map[string]string{"/": "Harbor Watch", "/plans/arrival-window/tasks": "Review the assumptions", "/technical/whiteboards/arrival": "Read the tide bulletin"} {
		response, err := client.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(text)) {
			t.Fatalf("offline page %s: status %d, error %v, body %s", path, response.StatusCode, err, body)
		}
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("offline reads changed example data")
	}
}
