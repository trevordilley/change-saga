package semanticcheck

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestCheckReportsRisksWithProvenanceAndDoesNotTouchGit(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Semantic Test")
	git("config", "user.email", "semantic@example.test")
	root := filepath.Join(repo, "app.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := saga.Manifest{Schema: saga.SagaSchemaURL, Version: saga.SagaVersion, ID: "semantic", Title: "Semantic", Source: saga.Source{Repository: "https://example.test/semantic.git"}}
	if err := store.WriteJSON(filepath.Join(root, saga.ManifestName), manifest, true); err != nil {
		t.Fatal(err)
	}
	if _, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: "core", Title: "Core", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	common, err := requirements.AddStory(root, "semantic", requirements.AddStoryInput{Feature: "core", ID: "common", RevisionID: "r1", EventID: "proposed", Title: "Common", Statement: "As a buyer I complete checkout", AcceptanceCriteria: []requirements.Criterion{{ID: "works", Statement: "Checkout works"}}})
	if err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base saga")
	base := git("rev-parse", "HEAD")

	makeBranch := func(branch, revision, collisionTitle, proposalID string) {
		t.Helper()
		git("checkout", "-b", branch, base)
		_, err := requirements.ReviseStory(root, "semantic", requirements.ReviseStoryInput{Story: common.URN, ID: revision, Parents: []string{common.URN + ":revision:r1"}, Title: "Common " + branch, Statement: "As a buyer I complete checkout", AcceptanceCriteria: []requirements.Criterion{{ID: "works", Statement: "Checkout works"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := requirements.AddStory(root, "semantic", requirements.AddStoryInput{Feature: "core", ID: "collision", RevisionID: "r1", EventID: "proposed", Title: collisionTitle, Statement: "Independent identity", AcceptanceCriteria: []requirements.Criterion{{ID: "works", Statement: "It works"}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := requirements.AddStory(root, "semantic", requirements.AddStoryInput{Feature: "core", ID: proposalID, RevisionID: "r1", EventID: "proposed", Title: "Fast checkout", Statement: "As a buyer I complete checkout quickly", AcceptanceCriteria: []requirements.Criterion{{ID: "fast", Statement: "Checkout is quick"}}}); err != nil {
			t.Fatal(err)
		}
		git("add", ".")
		git("commit", "-m", branch)
	}
	makeBranch("left", "left-r2", "Left collision", "proposal-left")
	git("checkout", "main")
	makeBranch("right", "right-r2", "Right collision", "proposal-right")

	headBefore := git("rev-parse", "HEAD")
	statusBefore := git("status", "--porcelain=v1")
	report, err := Check(context.Background(), Options{Repository: repo, SagaPath: "app.saga", Refs: []string{"left", "right"}})
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReadOnly || len(report.Refs) != 2 || len(report.Collisions) != 1 || report.Collisions[0].ID != "collision" {
		t.Fatalf("collision report = %#v", report)
	}
	if len(report.CompetingHeads) == 0 || report.CompetingHeads[0].Story != "common" {
		t.Fatalf("competing heads = %#v", report.CompetingHeads)
	}
	foundOverlap := false
	for _, candidate := range report.IntentCandidates {
		foundOverlap = foundOverlap || candidate.Left == "proposal-left" && candidate.Right == "proposal-right"
		if candidate.Provenance.Commit == "" || candidate.Other.Commit == "" || !strings.Contains(candidate.Explanation, "must decide") {
			t.Fatalf("candidate provenance/explanation = %#v", candidate)
		}
	}
	if !foundOverlap {
		t.Fatalf("overlap candidates = %#v", report.IntentCandidates)
	}
	if got := git("rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("HEAD changed: %s -> %s", headBefore, got)
	}
	if got := git("status", "--porcelain=v1"); got != statusBefore {
		t.Fatalf("working tree changed: %q -> %q", statusBefore, got)
	}
}

func TestCheckRequiresExplicitRefs(t *testing.T) {
	_, err := Check(context.Background(), Options{Repository: t.TempDir(), SagaPath: "app.saga", Refs: []string{"HEAD"}})
	if err == nil || !strings.Contains(err.Error(), "at least two explicit") {
		t.Fatalf("explicit refs error = %v", err)
	}
}
