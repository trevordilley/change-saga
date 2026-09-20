package saga

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

const embeddedDeckManifest = `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"visual","title":"Visual review","source":{"repository":"https://example.test/acme/app.git"}}`

func TestLoadEmbeddedDeckItemEvidence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), embeddedDeckManifest)
	bundle := filepath.Join(root, testFeatureDir, EmbeddedSlidesDir, "implementation"+EmbeddedDeckSuffix)
	deckTarget := DeckTarget("visual", "implementation")
	deckName, _ := FlatDeckFilename(deckTarget, 0)
	writeTestFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"implementation","title":"Implementation","role":"change","rank":0,"objective":"Walk the reviewer through the change."}`)
	slideTarget := SlideTarget("visual", "change")
	slideName, _ := FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := FlatSlideAssetFilename(slideName, ".svg")
	writeTestFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"change","deck":"implementation","title":"Reject early","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"Invalid requests stop before persistence.","reading_order":["validate","why"]}`, assetName))
	writeTestFile(t, filepath.Join(bundle, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><g id="validate"><rect width="200" height="100"/></g><g id="why"><text>Reject before writes</text></g></svg>`)
	validateTarget := ItemTarget("visual", "change", "validate")
	validateName, _ := FlatItemFilename(slideTarget, validateTarget, 0)
	writeTestFile(t, filepath.Join(bundle, validateName), `{"version":4,"id":"validate","slide":"change","rank":0,"kind":"node","label":"Validate","description":"The validation boundary.","selector":{"type":"element","element_id":"validate"}}`)
	whyTarget := ItemTarget("visual", "change", "why")
	whyName, _ := FlatItemFilename(slideTarget, whyTarget, 10)
	writeTestFile(t, filepath.Join(bundle, whyName), `{"version":4,"id":"why","slide":"change","rank":10,"kind":"callout","label":"Why here","description":"Explains why validation moved.","selector":{"type":"element","element_id":"why"},"about":"validate","body":"Reject before any write.","placement":"right","leader":"arrow"}`)
	code := referenceJSON(t, testReference("handler.go", 12, 12))
	writeTestFile(t, filepath.Join(bundle, FlatEvidenceFilename(whyTarget, "handler")), fmt.Sprintf(`{"version":2,"references":%s}`, code))

	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load embedded deck: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	if len(document.Decks) != 1 || len(document.Decks[0].Slides) != 1 || len(document.Decks[0].Slides[0].Items) != 2 {
		t.Fatalf("embedded deck hierarchy not loaded: %#v", document.Decks)
	}
	item := document.Decks[0].Slides[0].Items[1]
	if item.Kind != "callout" || len(item.Code) != 1 || item.Target != ItemTarget("visual", "change", "why") {
		t.Fatalf("callout evidence not preserved: %#v", item)
	}
	projected := document.Section.Children[0].Fragments[0].Landmarks[1]
	if projected.Target != item.Target || len(projected.Code) != 1 {
		t.Fatalf("review projection lost Item identity or evidence: %#v", projected)
	}
	if !document.Section.Children[0].Fragments[0].HasCode {
		t.Fatal("slide projection did not advertise its Item evidence")
	}
	index := MutationIndexFromDocument(document)
	if index.Targets[item.Target] != item.Directory {
		t.Fatalf("item is not a stable mutation target: %#v", index)
	}
	if !index.FlatTargets[slideTarget] {
		t.Fatalf("slide is not a flat deck target: %#v", index.FlatTargets)
	}

	writeTestFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"change","deck":"wrong-deck","title":"Reject early","rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"Invalid requests stop before persistence.","reading_order":["validate","why"]}`, assetName))
	writeTestFile(t, filepath.Join(bundle, validateName), `{"version":4,"id":"validate","slide":"wrong-slide","rank":0,"kind":"node","label":"Validate","description":"The validation boundary.","selector":{"type":"element","element_id":"validate"}}`)
	_, validation, err = Load(root)
	if err != nil || validation.Valid {
		t.Fatalf("incorrect semantic parent hints were accepted: valid=%v err=%v", validation.Valid, err)
	}
	var parentIssues strings.Builder
	for _, issue := range validation.Issues {
		parentIssues.WriteString(issue.Message)
		parentIssues.WriteByte('\n')
	}
	if !strings.Contains(parentIssues.String(), "slide deck must name its semantic parent") || !strings.Contains(parentIssues.String(), "item slide must name its semantic parent") {
		t.Fatalf("semantic parent failures were not explicit:\n%s", parentIssues.String())
	}
}

func TestEmbeddedDeckRefusesNestedPackagesBroadEvidenceAndOverviewRole(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), embeddedDeckManifest)
	bundle := filepath.Join(root, testFeatureDir, EmbeddedSlidesDir, "overview"+EmbeddedDeckSuffix)
	deckName, _ := FlatDeckFilename(DeckTarget("visual", "overview"), 0)
	writeTestFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"overview","title":"Overview","role":"overview","rank":0,"objective":"Orient the reviewer."}`)
	writeTestFile(t, filepath.Join(bundle, "nested.fragment", "fragment.json"), `{"version":2,"id":"nested","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(bundle, FlatEvidenceFilename(SagaTarget("visual"), "broad")), `{"version":2,"references":[]}`)
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("an embedded deck silently accepted an overview role, a nested package, and deck-level evidence")
	}
	var report strings.Builder
	for _, issue := range validation.Issues {
		report.WriteString(issue.Message)
		report.WriteByte('\n')
	}
	for _, want := range []string{"deck role must be change", "slide storage permits only regular files inside a deck bundle", "unknown Item key"} {
		if !strings.Contains(report.String(), want) {
			t.Fatalf("refusal %q was not reported:\n%s", want, report.String())
		}
	}
}
