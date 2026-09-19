package saga

import (
	"strings"
	"testing"
)

func TestDeckRecordNamesAreCompactDeterministicAndCategorized(t *testing.T) {
	deck := DeckTarget("visual", "a-deck-title-that-never-enters-the-filename")
	slide := SlideTarget("visual", "a-slide-title-that-never-enters-the-filename")
	item := ItemTarget("visual", "a-slide-title-that-never-enters-the-filename", "a-meaningful-callout")
	deckName, err := FlatDeckFilename(deck, 10)
	if err != nil {
		t.Fatal(err)
	}
	slideName, err := FlatSlideFilename(deck, slide, 20)
	if err != nil {
		t.Fatal(err)
	}
	itemName, err := FlatItemFilename(slide, item, 30)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{
		deckName,
		slideName,
		itemName,
		FlatEvidenceFilename(item, strings.Repeat("source-path-that-does-not-leak-", 10)),
	}
	prefixes := []string{"10-d-", "20-s-", "30-i-", "40-e-"}
	for i, name := range names {
		if len(name) > FlatMaxBasename {
			t.Fatalf("%s is %d characters", name, len(name))
		}
		if !strings.HasPrefix(name, prefixes[i]) {
			t.Fatalf("%s does not use category %s", name, prefixes[i])
		}
	}
	if again, _ := FlatItemFilename(slide, item, 30); again != itemName {
		t.Fatalf("flat name is not deterministic: %s != %s", again, itemName)
	}
}

func TestDeckRecordRankBudgetIsExplicit(t *testing.T) {
	if _, err := FlatDeckFilename("target", 10000); err == nil {
		t.Fatal("rank wider than the fixed sort field was accepted")
	}
}
