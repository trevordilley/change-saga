package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestFeatureOverviewPreservesIdentityAndHasFragmentQueryHistory(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	ctx := context.Background()
	if err := Feature(ctx, []string{"add", root, "--id", "reading", "--title", "Reading", "--description", "Understand intent."}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(root, applayout.FeatureRel("reading"), "feature.json")
	before, err := os.ReadFile(identity)
	if err != nil {
		t.Fatal(err)
	}
	git(t, filepath.Dir(root), "init", "-b", "main")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "user.email", "test@example.test")
	for i, text := range []string{"## Purpose\n\nRead **intent** first.", "## Purpose\n\nRead intent and rationale first."} {
		if err := Feature(ctx, []string{"set-overview", root, "--feature", "reading", "--text", text}, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
		doc, validation, err := saga.Load(root)
		if err != nil || !validation.Valid {
			t.Fatalf("load: %v %+v", err, validation)
		}
		fragment := doc.FindFeature("reading").Overview()
		if fragment == nil {
			t.Fatal("missing fragment")
		}
		var output bytes.Buffer
		if err := Query(ctx, []string{"fragment", "--saga", root, "--repo", repo, "--target", fragment.Target}, &output); err != nil || !strings.Contains(output.String(), "Read") {
			t.Fatalf("fragment query: %v %s", err, &output)
		}
		output.Reset()
		if err := Query(ctx, []string{"context", "--saga", root, "--repo", repo, "--feature", "reading"}, &output); err != nil || !strings.Contains(output.String(), fragment.Target) {
			t.Fatalf("context overview: %v %s", err, &output)
		}
		if i == 0 {
			git(t, root, "add", ".")
			git(t, root, "commit", "-m", "Author feature purpose")
		}
	}
	after, _ := os.ReadFile(identity)
	if !bytes.Equal(before, after) {
		t.Fatal("immutable feature identity changed")
	}
	var output bytes.Buffer
	if err := Query(ctx, []string{"history", "--saga", root, "--node", saga.FragmentTarget("batch", "reading--overview")}, &output); err != nil || !strings.Contains(output.String(), "Author feature purpose") {
		t.Fatalf("history: %v %s", err, &output)
	}
}

func TestFeatureOverviewRefusesUnsafeOrIncompatiblePackages(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"html", "package-symlink", "entrypoint-symlink", "collision"} {
		t.Run(mode, func(t *testing.T) {
			root := newAuthoredSaga(t)
			if err := Feature(context.Background(), []string{"add", root, "--id", "reading", "--title", "Reading"}, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(root, applayout.FeatureRel("reading"), applayout.FeatureOverview)
			external := t.TempDir()
			sentinel := filepath.Join(external, "content.md")
			writeFile(t, sentinel, "untouched")
			switch mode {
			case "html":
				writeFile(t, filepath.Join(dir, "fragment.json"), `{"version":2,"id":"existing","media_type":"text/html","entrypoint":"content.html"}`)
				writeFile(t, filepath.Join(dir, "content.html"), "<p>Existing</p>")
			case "package-symlink":
				if err := os.Symlink(external, dir); err != nil {
					t.Fatal(err)
				}
			case "entrypoint-symlink":
				writeFile(t, filepath.Join(dir, "fragment.json"), `{"version":2,"id":"existing","media_type":"text/markdown","entrypoint":"content.md"}`)
				if err := os.Symlink(sentinel, filepath.Join(dir, "content.md")); err != nil {
					t.Fatal(err)
				}
			case "collision":
				other := filepath.Join(root, applayout.FeatureRel("reading"), "other.fragment")
				writeFile(t, filepath.Join(other, "fragment.json"), `{"version":2,"id":"reading--overview","media_type":"text/markdown","entrypoint":"content.md"}`)
				writeFile(t, filepath.Join(other, "content.md"), "existing")
			}
			if err := Feature(context.Background(), []string{"set-overview", root, "--feature", "reading", "--text", "Replacement"}, &bytes.Buffer{}); err == nil {
				t.Fatal("unsafe replacement accepted")
			}
			data, _ := os.ReadFile(sentinel)
			if string(data) != "untouched" {
				t.Fatal("external content changed")
			}
		})
	}
}
