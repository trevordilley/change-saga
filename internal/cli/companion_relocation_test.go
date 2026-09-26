package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A code link does not depend on the directory that holds its Saga. A
// companion Saga can move beside the source, drop the explicit --repo, and
// still resolve the same Item to the same code reference.
func TestCompanionSagaLinksSurviveMoveIntoSourceRepository(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "app.go"), "package app\n\nfunc Ready() bool { return true }\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Add app")

	root := filepath.Join(shortTempDir(t), "app.saga")
	var output bytes.Buffer
	mustRun(t, Init, "--repo", repo, "--id", "portable", root)
	mustRun(t, Feature, "add", "--id", "app", "--title", "App", root)
	mustRun(t, AddDeck, "--feature", "app", "--objective", "Explain readiness.", root, "implementation")
	mustRun(t, AddSlide, "--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "ready")
	mustRun(t, AddItem, "--slide", "ready", "--kind", "statement", "--id", "ready-check", "--element-id", "slide-title", "--description", "The readiness check.", root)
	target := "urn:change-saga:portable:slide:ready:item:ready-check"
	mustRun(t, Cover, "--repo", repo, "--target", target, "--ref", "HEAD:app.go#L3", "--note", "The readiness Item owns the exact readiness implementation.", root)
	mustRun(t, Sync, "--repo", repo, root)

	query := func(sagaRoot string, explicitRepo bool) json.RawMessage {
		t.Helper()
		args := []string{"slide-diffs", "--saga", sagaRoot, "--target", target}
		if explicitRepo {
			args = append(args, "--repo", repo)
		}
		output.Reset()
		if err := Query(context.Background(), args, &output); err != nil {
			t.Fatalf("query relocated evidence: %v\n%s", err, output.String())
		}
		var envelope struct {
			OK   bool            `json:"ok"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(output.Bytes(), &envelope); err != nil || !envelope.OK {
			t.Fatalf("decode relocated evidence: %v\n%s", err, output.String())
		}
		return envelope.Data
	}

	before := query(root, true)
	moved := filepath.Join(repo, "app.saga")
	if err := os.Rename(root, moved); err != nil {
		t.Fatalf("move companion Saga into source repository: %v", err)
	}
	after := query(moved, false)
	if !bytes.Equal(before, after) {
		t.Fatalf("moving the Saga changed its resolved link:\nbefore: %s\nafter:  %s", before, after)
	}
	if !strings.Contains(string(after), `"status":"current"`) || !strings.Contains(string(after), `"path":"app.go"`) {
		t.Fatalf("relocated evidence did not remain current: %s", after)
	}

	output.Reset()
	if err := Sync(context.Background(), []string{"--repo", repo, moved}, &output); err == nil || !strings.Contains(err.Error(), "no sync cursor") {
		t.Fatalf("moved Saga was not recognized inside its source repository: %v\n%s", err, output.String())
	}
}
