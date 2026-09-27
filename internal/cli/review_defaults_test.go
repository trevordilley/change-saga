package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitexec"
)

func TestReviewCreateWorksOutItsDefaults(t *testing.T) {
	t.Setenv("CHANGE_SAGA_NO_GH", "1")
	fixture := newReviewOnlyFixture(t, false)
	out := run(t, Review, "create", fixture.root)
	if !strings.Contains(out, "Created urn:change-saga:app:review:feature-pg") || !strings.Contains(out, "Using base main (the repository's main branch), id feature-pg") {
		t.Fatalf("review create without flags:\n%s", out)
	}
	out = run(t, Review, "create", "--pr", "12", fixture.root)
	if !strings.Contains(out, "review:pr-12") {
		t.Fatalf("review create --pr:\n%s", out)
	}
	// Explicit flags always win.
	out = run(t, Review, "create", "--id", "custom", "--base", "feature/pg", fixture.root)
	if !strings.Contains(out, "review:custom") || strings.Contains(out, "Using") {
		t.Fatalf("review create with explicit flags:\n%s", out)
	}
	document := statusJSON(t, fixture.root, "--repo", fixture.repo, "--against", "main")
	if len(document.ChangeReviews) != 3 {
		t.Fatalf("reviews of the change = %v", document.ChangeReviews)
	}
}

func TestReviewDefaultsFollowOriginsDefaultBranch(t *testing.T) {
	t.Setenv("CHANGE_SAGA_NO_GH", "1")
	repo := t.TempDir()
	git(t, repo, "init", "-b", "trunk")
	git(t, repo, "config", "user.name", "Dev")
	git(t, repo, "config", "user.email", "dev@example.test")
	writeFile(t, filepath.Join(repo, "a.go"), "package a\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	git(t, repo, "update-ref", "refs/remotes/origin/trunk", "HEAD")
	git(t, repo, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/trunk")
	git(t, repo, "checkout", "-q", "-b", "fix/login", "origin/trunk")
	git(t, repo, "branch", "-D", "trunk")
	fill := func() (reviewCreateInputs, error) {
		ctx, end := gitexec.Begin(context.Background())
		defer end()
		return fillReviewDefaults(ctx, repo, reviewCreateInputs{})
	}
	filled, err := fill()
	if err != nil || filled.base != "origin/trunk" || filled.id != "fix-login" {
		t.Fatalf("defaults = %+v, %v", filled, err)
	}
	git(t, repo, "checkout", "-q", "--detach")
	if _, err := fill(); err == nil || !strings.Contains(err.Error(), "pass --id") {
		t.Fatalf("a detached checkout named a review: %v", err)
	}
}
