package saga

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

const appTestManifest = `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"shop","title":"Shop","source":{"repository":"https://example.test/acme/shop.git"}}`

func writeFragment(t *testing.T, dir, id string) {
	t.Helper()
	writeTestFile(t, filepath.Join(dir, "fragment.json"), fmt.Sprintf(`{"version":2,"id":%q,"media_type":"text/markdown","entrypoint":"content.md"}`, id))
	writeTestFile(t, filepath.Join(dir, "content.md"), "Content.\n")
}

func writeOnboardingDeck(t *testing.T, root, record string) {
	t.Helper()
	bundle := filepath.Join(root, "___onboarding", "welcome"+EmbeddedDeckSuffix)
	deckTarget := DeckTarget("shop", "welcome")
	deckName, _ := FlatDeckFilename(deckTarget, 0)
	writeTestFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"welcome","title":"Welcome","role":"onboarding","rank":0,"objective":"Get a new teammate up to speed."}`)
	slideTarget := SlideTarget("shop", "who")
	slideName, _ := FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := FlatSlideAssetFilename(slideName, ".svg")
	writeTestFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"who","deck":"welcome","title":"Who it serves","rank":0,"intent":"orient","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"Buyers and sellers.","reading_order":["buyer"]}`, assetName))
	writeTestFile(t, filepath.Join(bundle, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><g id="buyer"><rect width="200" height="100"/></g></svg>`)
	itemTarget := ItemTarget("shop", "who", "buyer")
	itemName, _ := FlatItemFilename(slideTarget, itemTarget, 0)
	recordField := ""
	if record != "" {
		recordField = fmt.Sprintf(`,"record":%q`, record)
	}
	writeTestFile(t, filepath.Join(bundle, itemName), fmt.Sprintf(`{"version":4,"id":"buyer","slide":"who","rank":0,"kind":"node","label":"Buyer","description":"The buyer persona.","selector":{"type":"element","element_id":"buyer"}%s}`, recordField))
}

func TestAppRootsAndFeaturesLoadIntoOneTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	writeTestFile(t, filepath.Join(root, "___features", "billing.feature", "feature.json"), `{"$schema":"https://changesaga.dev/schema/v5/feature.schema.json","version":5,"id":"billing","title":"Billing","created_at":"2026-08-21T12:00:00Z"}`)
	writeFragment(t, filepath.Join(root, "___overview", "pitch.fragment"), "pitch")
	writeFragment(t, filepath.Join(root, "___designsystem", "figma.fragment"), "figma")
	writeFragment(t, filepath.Join(root, testFeatureDir, "overview.fragment"), "core-overview")
	writeFragment(t, filepath.Join(root, "___features", "billing.feature", "overview.fragment"), "billing-overview")
	writeFragment(t, filepath.Join(root, "___features", "billing.feature", "___design", "ledger.fragment"), "ledger")
	writeOnboardingDeck(t, root, "urn:change-saga:shop:persona:buyer")

	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load app: err=%v issues=%#v", err, validation.Issues)
	}
	if document.Overview == nil || len(document.Overview.Fragments) != 1 || document.DesignSystem == nil || len(document.DesignSystem.Fragments) != 1 {
		t.Fatalf("app roots = %#v %#v", document.Overview, document.DesignSystem)
	}
	if len(document.Features) != 2 || document.Features[0].ID != "billing" || document.Features[0].Title != "Billing" || document.Features[1].ID != "core" {
		t.Fatalf("features = %#v", document.Features)
	}
	billing := document.FindFeature("billing")
	if len(billing.Report.Fragments) != 1 || billing.Design == nil || len(billing.Design.Fragments) != 1 || !IsDesignPath(billing.Design.Fragments[0].Path) {
		t.Fatalf("billing feature = %#v", billing)
	}
	if len(document.Onboarding) != 1 || len(document.Decks) != 0 {
		t.Fatalf("onboarding = %d, implementation decks = %d", len(document.Onboarding), len(document.Decks))
	}
	index := MutationIndexFromDocument(document)
	for _, target := range []string{FragmentTarget("shop", "pitch"), FragmentTarget("shop", "figma"), FragmentTarget("shop", "ledger"), SlideTarget("shop", "who")} {
		if index.Targets[target] == "" {
			t.Errorf("target %s is not addressable", target)
		}
	}
}

func TestReportIDsAreUniqueAcrossFeatures(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	writeTestFile(t, filepath.Join(root, "___features", "billing.feature", "feature.json"), `{"$schema":"https://changesaga.dev/schema/v5/feature.schema.json","version":5,"id":"billing","title":"Billing","created_at":"2026-08-21T12:00:00Z"}`)
	writeFragment(t, filepath.Join(root, testFeatureDir, "overview.fragment"), "overview")
	writeFragment(t, filepath.Join(root, "___features", "billing.feature", "overview.fragment"), "overview")
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("two features reused one fragment URN")
	}
	if _, validation, _ := LoadMutationIndex(root); validation.Valid {
		t.Fatal("mutation index accepted two features reusing one fragment URN")
	}
}

func TestOnboardingItemsReferenceRecordsNotCode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	writeOnboardingDeck(t, root, "")
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, issue := range validation.Issues {
		messages = append(messages, issue.Message)
	}
	if validation.Valid || !strings.Contains(strings.Join(messages, "\n"), "onboarding item record must be") {
		t.Fatalf("onboarding item without a record = %v", messages)
	}
}

func TestReportContentAtTheAppRootIsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeTestFile(t, filepath.Join(root, ManifestName), appTestManifest)
	writeFragment(t, filepath.Join(root, "overview.fragment"), "overview")
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("report content at the app root was accepted")
	}
}
