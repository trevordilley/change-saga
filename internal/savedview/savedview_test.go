package savedview

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

const repository = "https://example.test/acme/app.git"

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=T", "-c", "user.email=t@example.test"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeManifest(t *testing.T, root, id, source string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"$schema": saga.SagaSchemaURL, "version": saga.SagaVersion, "id": id, "title": "App", "source": map[string]any{"repository": source}}
	if err := store.WriteJSON(filepath.Join(root, saga.ManifestName), manifest, false); err != nil {
		t.Fatal(err)
	}
}

func commitAll(t *testing.T, repo, message string) string {
	t.Helper()
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-q", "--allow-empty", "-m", message)
	return runGit(t, repo, "rev-parse", "HEAD")
}

func expectReason(t *testing.T, err error, reason Reason) {
	t.Helper()
	var viewErr *Error
	if !errors.As(err, &viewErr) || viewErr.Reason != reason {
		t.Fatalf("got %v, want %s", err, reason)
	}
}

func TestLoadRefusesWithSpecificReasons(t *testing.T) {
	ctx := context.Background()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n")
	beforeSaga := commitAll(t, repo, "source")

	root := filepath.Join(repo, "app.saga")
	writeManifest(t, root, "app", "https://example.test/other.git")
	otherSource := commitAll(t, repo, "saga with another source")

	writeManifest(t, root, "app", repository)
	if err := os.MkdirAll(filepath.Join(root, "___inventory", "diagrams"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "___inventory", "diagrams", "x.json"), "{}\n")
	badInventory := commitAll(t, repo, "unreadable inventory")

	if err := os.RemoveAll(filepath.Join(root, "___inventory")); err != nil {
		t.Fatal(err)
	}
	good := commitAll(t, repo, "valid saga")
	current, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Load(ctx, repo, root, current, beforeSaga)
	expectReason(t, err, SagaMissing)
	_, err = Load(ctx, repo, root, current, otherSource)
	expectReason(t, err, SourceMismatch)
	_, err = Load(ctx, repo, root, current, badInventory)
	expectReason(t, err, InventoryInvalid)
	_, err = Load(ctx, repo, root, current, "no-such-revision")
	expectReason(t, err, CommitUnavailable)

	view, err := Load(ctx, repo, root, current, good[:10])
	if err != nil || view.Commit != good || view.SourceCommit != good || view.Artifact != "git:"+good+":app.saga" || len(view.Inventory.Records) != 0 {
		t.Fatalf("valid view: %+v %v", view, err)
	}
	if facts := view.Facts("urn:change-saga:app:component:missing"); facts.Definition != "missing" || !facts.Resolved {
		t.Fatalf("absent record facts: %+v", facts)
	}

	// A Saga kept outside its source checkout (companion layout) has no view.
	companion := filepath.Join(t.TempDir(), "app.saga")
	writeManifest(t, companion, "app", repository)
	_, err = Load(ctx, repo, companion, current, good)
	expectReason(t, err, SagaMissing)

	// A shallow clone that lacks the view commit refuses without fallback.
	shallow := filepath.Join(t.TempDir(), "shallow")
	runGit(t, repo, "clone", "-q", "--depth", "1", "file://"+repo, shallow)
	_, err = Load(ctx, shallow, filepath.Join(shallow, "app.saga"), current, otherSource)
	expectReason(t, err, CommitUnavailable)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
