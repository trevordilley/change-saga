package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// initRepository is a checkout in a folder deliberately named unlike the
// repository, the way a local clone often is, with origin set when given.
func initRepository(t *testing.T, folder, origin string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), folder)
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "init", "-b", "main")
	if origin != "" {
		git(t, repo, "remote", "add", "origin", origin)
	}
	return repo
}

func loadManifest(t *testing.T, root string) saga.Manifest {
	t.Helper()
	assertValid(t, root)
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return document.Manifest
}

// With no path, init creates change.saga in the current directory and names
// it after the repository its origin identifies, not the checkout's folder and
// never "change".
func TestInitWithoutAPathCreatesChangeSagaNamedAfterOrigin(t *testing.T) {
	repo := initRepository(t, "review-saga", "https://github.com/trevordilley/change-saga.git")
	t.Chdir(repo)
	var output bytes.Buffer
	if err := Init(context.Background(), nil, &output); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repo, DefaultSagaName)
	manifest := loadManifest(t, root)
	if manifest.ID != "change-saga" || manifest.Title != "change-saga" {
		t.Fatalf("default change.saga = id %q title %q, want the repository's name", manifest.ID, manifest.Title)
	}
	if !strings.HasPrefix(output.String(), "Created change.saga\n") || strings.Contains(output.String(), "Note:") {
		t.Fatalf("init output = %q", output.String())
	}
}

func TestInitNamesChangeSagaAfterTheCheckoutWithoutOrigin(t *testing.T) {
	t.Parallel()
	repo := initRepository(t, "my-product", "")
	root := filepath.Join(repo, DefaultSagaName)
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--allow-local-repository", root}, &output); err != nil {
		t.Fatal(err)
	}
	if manifest := loadManifest(t, root); manifest.ID != "my-product" || manifest.Title != "my-product" {
		t.Fatalf("change.saga without origin = id %q title %q, want the top-level directory's name", manifest.ID, manifest.Title)
	}
}

func TestInitKeepsExplicitNamesAndFlags(t *testing.T) {
	t.Parallel()
	repo := initRepository(t, "checkout", "git@github.com:acme/shop.git")
	named := filepath.Join(repo, "orders.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, named}, &output); err != nil {
		t.Fatal(err)
	}
	if manifest := loadManifest(t, named); manifest.ID != "orders" || manifest.Title != "orders" {
		t.Fatalf("explicitly named saga = id %q title %q, want its directory name", manifest.ID, manifest.Title)
	}
	flagged := filepath.Join(t.TempDir(), DefaultSagaName)
	if err := Init(context.Background(), []string{"--repo", repo, "--id", "store", "--title", "The Store", flagged}, &output); err != nil {
		t.Fatal(err)
	}
	if manifest := loadManifest(t, flagged); manifest.ID != "store" || manifest.Title != "The Store" {
		t.Fatalf("--id and --title were not kept: id %q title %q", manifest.ID, manifest.Title)
	}
}

func TestRepositoryNameStripsGitFromEveryOriginShape(t *testing.T) {
	t.Parallel()
	top := filepath.Join(t.TempDir(), "review-saga")
	for _, origin := range []string{
		"https://github.com/trevordilley/change-saga.git",
		"https://github.com/trevordilley/change-saga",
		"https://github.com/trevordilley/change-saga.git/",
		"git@github.com:trevordilley/change-saga.git",
		"ssh://git@github.com/trevordilley/change-saga.git",
		filepath.Join(t.TempDir(), "change-saga.git"),
		filepath.Join(t.TempDir(), "change-saga"),
	} {
		canonical, err := normalizeRepositoryURI(origin, top)
		if err != nil {
			t.Fatalf("%s: %v", origin, err)
		}
		if got := repositoryName(canonical, top); got != "change-saga" {
			t.Errorf("repositoryName(%q) = %q, want change-saga", canonical, got)
		}
	}
	if got := repositoryName("https://example.test/", top); got != "review-saga" {
		t.Errorf("an origin without a path = %q, want the top-level directory", got)
	}
}

// One Saga per repository is the recommended idiom, never a rule: a second
// Saga is still created, and init names the existing one so the author can
// reconsider.
func TestInitNotesAnExistingSagaWithoutRefusing(t *testing.T) {
	t.Parallel()
	repo := initRepository(t, "checkout", "https://example.test/acme/shop.git")
	if err := os.MkdirAll(filepath.Join(repo, "docs", "app.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(repo, DefaultSagaName)
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, root}, &output); err != nil {
		t.Fatalf("an existing Saga must not block init: %v", err)
	}
	loadManifest(t, root)
	text := output.String()
	for _, want := range []string{"already contains a Saga", "  - docs/app.saga\n", "one Saga per repository", "still created"} {
		if !strings.Contains(text, want) {
			t.Fatalf("init note omitted %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "  - "+DefaultSagaName) {
		t.Fatalf("init named the Saga it just created as pre-existing:\n%s", text)
	}
}

// Without a path, init creates change.saga in the directory --repo names, so
// pointing it at another checkout never leaves a Saga in the current
// directory.
func TestInitWithoutAPathCreatesChangeSagaInTheRepoDirectory(t *testing.T) {
	repo := initRepository(t, "checkout", "https://example.test/acme/shop.git")
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo}, &output); err != nil {
		t.Fatal(err)
	}
	if manifest := loadManifest(t, filepath.Join(repo, DefaultSagaName)); manifest.ID != "shop" {
		t.Fatalf("change.saga in --repo has id %q", manifest.ID)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, DefaultSagaName)); !os.IsNotExist(err) {
		t.Fatalf("init also wrote change.saga in the current directory: %v", err)
	}
}

// A name with nothing to slug is refused with a reason that names the
// derivation, not a --id flag that was never passed.
func TestInitRefusesAnIDItCannotDerive(t *testing.T) {
	t.Parallel()
	repo := initRepository(t, "___", "")
	root := filepath.Join(repo, DefaultSagaName)
	var output bytes.Buffer
	err := Init(context.Background(), []string{"--repo", repo, "--allow-local-repository", root}, &output)
	if err == nil || !strings.Contains(err.Error(), `cannot derive a Saga id from "___"; pass --id`) {
		t.Fatalf("init with an underivable id = %v", err)
	}
	if _, statErr := os.Stat(root); !os.IsNotExist(statErr) {
		t.Fatal("a refused init left a Saga behind")
	}
	if err := Init(context.Background(), []string{"--repo", repo, "--allow-local-repository", "--id", "shop", root}, &output); err != nil {
		t.Fatalf("an explicit --id should resolve it: %v", err)
	}
}
