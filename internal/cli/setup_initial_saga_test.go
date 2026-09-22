package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupInitialSagaPrintsOneTimeWorkflowWhenNoSagaExists(t *testing.T) {
	repo := t.TempDir()
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", repo}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{
		"no .saga directory was found",
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
	if strings.Contains(text, "# Establish the initial app Saga") {
		t.Fatalf("guarded setup emitted the full workflow:\n%s", text)
	}
}

func TestSetupInitialSagaOverhaulUpdatesInsteadOfDuplicating(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "app.saga"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", repo, "--overhaul"}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"explicitly requested a documentation overhaul", "app.saga", "do not create a duplicate", "# Establish the initial app Saga"} {
		if !strings.Contains(text, expected) {
			t.Errorf("overhaul workflow omitted %q:\n%s", expected, text)
		}
	}
}

func TestSetupInitialSagaRejectsInvalidRepository(t *testing.T) {
	var output bytes.Buffer
	if err := SetupInitialSaga([]string{"--repo", filepath.Join(t.TempDir(), "missing")}, &output); err == nil {
		t.Fatal("setup accepted a missing repository")
	}
}

func TestSetupInitialSagaHelpExplainsTheGuard(t *testing.T) {
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
