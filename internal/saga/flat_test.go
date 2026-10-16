package saga

import (
	"fmt"
	"path/filepath"
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

func TestFlatPathBudgetPreservesLimitAndExplainsRecovery(t *testing.T) {
	base := filepath.Join(t.TempDir(), "s")
	for _, filenameBytes := range []int{24, FlatMaxBasename} {
		padding := FlatMaxPath - len(filepath.Join(base, "x")) + 1 - filenameBytes
		directory := base + strings.Repeat("a", padding)
		if issue := FlatPathBudgetIssue(directory, filenameBytes); issue != "" {
			t.Fatalf("exactly 240 bytes must remain valid: %s", issue)
		}
		// A non-ASCII suffix checks that the diagnostic uses the same byte
		// measurement as the guard rather than promising character counts.
		issue := FlatPathBudgetIssue(directory+"é", filenameBytes)
		for _, want := range []string{"242 bytes", "240-byte Windows portability budget (2-byte reduction required)", fmt.Sprintf("filename allowance: %d bytes", filenameBytes), "shorter checkout/worktree or Saga location", "Preserve existing record IDs", "guard enabled", "validate --json"} {
			if !strings.Contains(issue, want) {
				t.Errorf("diagnostic missing %q: %s", want, issue)
			}
		}
	}
}

func TestFlatPathChecksActualFilenameAndBasenameBudget(t *testing.T) {
	base := filepath.Join(t.TempDir(), "s")
	directory := base + strings.Repeat("a", FlatMaxPath-len(filepath.Join(base, "x"))+1-24)
	if issue := flatPathIssue(directory, strings.Repeat("x", 24)); issue != "" {
		t.Fatal(issue)
	}
	if issue := flatPathIssue(directory, strings.Repeat("x", 25)); !strings.Contains(issue, "1-byte reduction required") {
		t.Fatalf("expected over-budget actual file: %s", issue)
	}
	if issue := flatPathIssue(base, strings.Repeat("x", 65)); !strings.Contains(issue, "65 bytes") || !strings.Contains(issue, "do not hand-rename") {
		t.Fatalf("expected actionable basename diagnostic: %s", issue)
	}
}

func TestDeckRecordRankBudgetIsExplicit(t *testing.T) {
	if _, err := FlatDeckFilename("target", 10000); err == nil {
		t.Fatal("rank wider than the fixed sort field was accepted")
	}
}
