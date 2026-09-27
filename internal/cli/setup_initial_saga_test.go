package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupInitialSagaPrintsOneTimeWorkflowWhenNoSagaExists(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", repo}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"no .saga directory was found",
		"one Saga per repository", "change.saga", "monorepo",
		"Use this workflow only once",
		"Stories added solely at the user's request are fully valid requirements",
		"marking a confirmed story accepted, give it at least one criterion",
		"do not invent broader behavior merely to make",
		"Interview the user in short rounds",
		"Offer a feature-led parallel code deep dive",
		"one lane per feature",
		"evidence-backed candidate stories and criteria",
		"silently promote code-derived candidates into requirements",
		"Review before quality expansion",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("initial setup omitted %q:\n%s", expected, text)
		}
	}
}

func TestSetupInitialSagaGuardsExistingSaga(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs", "app.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "node_modules", "dependency.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", repo}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"normally runs once", "docs/app.saga", "ordinary documentation work", "--overhaul"} {
		if !strings.Contains(text, expected) {
			t.Errorf("existing-Saga guard omitted %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "dependency.saga") {
		t.Fatalf("setup searched ignored dependency directory:\n%s", text)
	}
	if strings.Contains(text, "# Establish the repository's initial Saga") {
		t.Fatalf("guarded setup emitted the full workflow:\n%s", text)
	}
}

func TestSetupInitialSagaOverhaulUpdatesInsteadOfDuplicating(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "app.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", repo, "--overhaul"}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"explicitly requested a documentation overhaul", "app.saga", "do not create a duplicate", "# Establish the repository's initial Saga"} {
		if !strings.Contains(text, expected) {
			t.Errorf("overhaul workflow omitted %q:\n%s", expected, text)
		}
	}
}

func TestSetupInitialSagaRejectsInvalidRepository(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", filepath.Join(t.TempDir(), "missing")}, &output); err == nil {
		t.Fatal("setup accepted a missing repository")
	}
}

func TestSetupInitialSagaHelpExplainsTheGuard(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"-h"}, &output); err == nil {
		t.Fatal("-h must report flag.ErrHelp")
	}
	text := output.String()
	for _, expected := range []string{"change-saga setup-initial-saga", "one-time agent workflow", "does not modify", "--overhaul", "--repo"} {
		if !strings.Contains(text, expected) {
			t.Errorf("setup help omitted %q:\n%s", expected, text)
		}
	}
}

func TestSetupInitialSagaGrowsASagaOfOnlyReviews(t *testing.T) {
	t.Setenv("CHANGE_SAGA_NO_GH", "1")
	fixture := newReviewOnlyFixture(t, true)
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", fixture.repo}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"app.saga holds 1 review and no living documentation yet", "Grow it in place", "# Establish the repository's initial Saga", "read them first"} {
		if !strings.Contains(text, expected) {
			t.Errorf("growing setup omitted %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "normally runs once") {
		t.Fatalf("setup guarded a Saga that holds only reviews:\n%s", text)
	}
}

func TestSetupInitialSagaComesAfterReviews(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", t.TempDir()}, &output); err != nil {
		t.Fatal(err)
	}
	if text := output.String(); !strings.Contains(text, "Most teams start with a review") || !strings.Contains(text, "many never need more") {
		t.Fatalf("setup with no Saga does not suggest a review first:\n%s", text)
	}
}
