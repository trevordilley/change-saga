package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// A Next hint names the step that is actually left. A slide or fragment
// written from --source already has its content, so the hint moves on to
// naming its elements instead of asking for the content again.
func TestNextHintsAfterSourceSkipSettingContent(t *testing.T) {
	root, _ := coveredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--epic", testEpic, "--objective", "Explain the change.", root, "implementation"}, &output); err != nil {
		t.Fatal(err)
	}
	svg := filepath.Join(t.TempDir(), "flow.svg")
	writeFile(t, svg, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><g id="a"><rect width="1" height="1"/></g></svg>`)
	output.Reset()
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", "--source", svg, root, "flow"}, &output); err != nil {
		t.Fatal(err)
	}
	if hint := output.String(); strings.Contains(hint, "set-slide-content") || !strings.Contains(hint, "Next: change-saga add-item --slide urn:change-saga:") || !strings.Contains(hint, "--element-id ID") {
		t.Fatalf("add-slide --source hint:\n%s", hint)
	}
	output.Reset()
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "blank"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Next: change-saga set-slide-content") {
		t.Fatalf("add-slide without --source should ask for content:\n%s", output.String())
	}
	output.Reset()
	if err := AddFragment(context.Background(), []string{"--epic", testEpic, "--type", "svg", "--source", svg, "--title", "Flow", root}, &output); err != nil {
		t.Fatal(err)
	}
	if hint := output.String(); strings.Contains(hint, "set-fragment-content") || !strings.Contains(hint, "Next: change-saga add-landmark --target ") {
		t.Fatalf("add-fragment --source hint:\n%s", hint)
	}
}

// The onboarding deck orients a newcomer, so its first slide is suggested
// with the orient intent rather than explain.
func TestOnboardingDeckSuggestsOrientIntent(t *testing.T) {
	root, _ := coveredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--role", "onboarding", "--objective", "Get a newcomer up to speed.", root, "onboarding"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--intent orient") {
		t.Fatalf("onboarding add-deck hint:\n%s", output.String())
	}
}

// Items are valid cover targets, so the refusal of a bad target names them.
func TestBadCoverTargetNamesItems(t *testing.T) {
	root, repo := coveredSaga(t)
	_, err := runCover(t, "", "--repo", repo, "--target", "nowhere.chapter", "--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil || !strings.Contains(err.Error(), "chapter, section, fragment, landmark, or Item") {
		t.Fatalf("bad cover target error = %v", err)
	}
}
