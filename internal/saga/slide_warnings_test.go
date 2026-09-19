package saga

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// writeChangeSlide writes an implementation deck with one slide holding the
// given Items, none of which reference code.
func writeChangeSlide(t *testing.T, root string, items ...string) string {
	t.Helper()
	bundle := filepath.Join(root, testEpicDir, EmbeddedSlidesDir, "implementation"+EmbeddedDeckSuffix)
	deckTarget := DeckTarget("shop", "implementation")
	deckName, _ := FlatDeckFilename(deckTarget, 0)
	writeTestFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"implementation","title":"Implementation","role":"change","rank":0,"objective":"Explain the change."}`)
	slideTarget := SlideTarget("shop", "flow")
	slideName, _ := FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := FlatSlideAssetFilename(slideName, ".svg")
	order := jsonStrings(items)
	writeTestFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"flow","deck":"implementation","title":"Flow","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"How it flows.","reading_order":%s}`, assetName, order))
	var svg strings.Builder
	for _, id := range items {
		fmt.Fprintf(&svg, `<g id=%q><rect width="10" height="10"/></g>`, id)
	}
	writeTestFile(t, filepath.Join(bundle, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720">`+svg.String()+`</svg>`)
	for index, id := range items {
		itemName, _ := FlatItemFilename(slideTarget, ItemTarget("shop", "flow", id), index*10)
		writeTestFile(t, filepath.Join(bundle, itemName), fmt.Sprintf(`{"version":4,"id":%q,"slide":"flow","rank":%d,"kind":"node","label":"Node","description":"A node.","selector":{"type":"element","element_id":%q}}`, id, index*10, id))
	}
	return filepath.ToSlash(filepath.Join(testEpicDir, EmbeddedSlidesDir, "implementation"+EmbeddedDeckSuffix, slideName))
}

func jsonStrings(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return "[" + strings.Join(quoted, ",") + "]"
}

// A slide just added with add-slide has no Items yet. That is work in
// progress: one warning, and the Saga stays valid so queries keep working.
func TestEmptySlideIsOneWarningNotAnError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	slidePath := writeChangeSlide(t, root)
	_, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("empty slide made the Saga invalid: err=%v issues=%#v", err, validation.Issues)
	}
	var onSlide []Issue
	for _, issue := range validation.Issues {
		if issue.Path == slidePath {
			onSlide = append(onSlide, issue)
		}
	}
	if len(onSlide) != 1 || onSlide[0].Severity != "warning" || !strings.Contains(onSlide[0].Message, "no semantic Items yet") {
		t.Fatalf("empty slide issues = %#v, want one warning", onSlide)
	}
}

// Only an implementation deck's Items own code, so only its slides are asked
// to reference code. Onboarding Items reference records and never own code.
func TestOnlyImplementationSlidesAreAskedForCode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	writeOnboardingDeck(t, root, "urn:change-saga:shop:persona:buyer")
	slidePath := writeChangeSlide(t, root, "start")
	_, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: err=%v issues=%#v", err, validation.Issues)
	}
	var codeWarnings []string
	for _, issue := range validation.Issues {
		if strings.Contains(issue.Message, "no Item-linked code") {
			codeWarnings = append(codeWarnings, issue.Path)
		}
	}
	if len(codeWarnings) != 1 || codeWarnings[0] != slidePath {
		t.Fatalf("code warnings = %v, want only the implementation slide %s", codeWarnings, slidePath)
	}
}
