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
	t.Parallel()
	root, _ := coveredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Explain the change.", root, "implementation"}, &output); err != nil {
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
	if err := AddFragment(context.Background(), []string{"--feature", testFeature, "--type", "svg", "--source", svg, "--title", "Flow", root}, &output); err != nil {
		t.Fatal(err)
	}
	if hint := output.String(); strings.Contains(hint, "set-fragment-content") || !strings.Contains(hint, "Next: change-saga add-landmark --target ") {
		t.Fatalf("add-fragment --source hint:\n%s", hint)
	}
}

// The onboarding deck orients a newcomer, so its first slide is suggested
// with the orient intent rather than explain.
func TestOnboardingDeckSuggestsOrientIntent(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	root, repo := coveredSaga(t)
	_, err := runCover(t, "", "--repo", repo, "--target", "nowhere.chapter", "--path", "internal/service/handler.go", "--side", "new", "--lines", "3", root)
	if err == nil || !strings.Contains(err.Error(), "chapter, section, fragment, landmark, or Item") {
		t.Fatalf("bad cover target error = %v", err)
	}
}

// A citation's URN names no feature, so a story in any feature may cite it; --feature
// only chooses where it is stored, and the help says so.
func TestCitationIsCitableFromAnyFeature(t *testing.T) {
	t.Parallel()
	root, _ := coveredSaga(t)
	var output bytes.Buffer
	if err := Feature(context.Background(), []string{"add", "--id", "billing", "--title", "Billing", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := Citation(context.Background(), []string{"add", "--feature", testFeature, "--id", "rfc", "--kind", "url", "--title", "RFC", "--reference", "https://example.test/rfc", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := Story(context.Background(), []string{"add", "--feature", "billing", "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay", "--statement", "As a buyer I want to pay so that I get the goods", "--priority", "must", "--citation", "urn:change-saga:batch:citation:rfc", root}, &output); err != nil {
		t.Fatalf("a story in another feature could not cite the citation: %v\n%s", err, output.String())
	}
	assertValid(t, root)
	output.Reset()
	_ = Citation(context.Background(), []string{"add", "--help"}, &output)
	if !strings.Contains(output.String(), "a\nstory in any feature may cite it") {
		t.Fatalf("citation add help does not say any feature may cite it:\n%s", output.String())
	}
}
