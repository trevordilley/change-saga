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

func TestFlatPathWarningExplainsWindowsCompatibility(t *testing.T) {
	base := filepath.Join(t.TempDir(), "s")
	for _, filenameBytes := range []int{24, FlatMaxBasename} {
		padding := FlatMaxPath - len(filepath.Join(base, "x")) + 1 - filenameBytes
		directory := base + strings.Repeat("a", padding)
		if issue := flatPathBudgetWarning(directory, filenameBytes, "windows"); issue != "" {
			t.Fatalf("exactly 240 bytes must remain valid: %s", issue)
		}
		// A non-ASCII suffix checks that the diagnostic uses the same byte
		// measurement as the guard rather than promising character counts.
		issue := flatPathBudgetWarning(directory+"é", filenameBytes, "windows")
		for _, want := range []string{"242 bytes", "240-byte Windows portability budget (2-byte reduction required)", fmt.Sprintf("filename allowance: %d bytes", filenameBytes), "shorter checkout/worktree or Saga location", "Preserve existing record IDs", "does not invalidate the Saga or block authoring", "validate --json"} {
			if !strings.Contains(issue, want) {
				t.Errorf("diagnostic missing %q: %s", want, issue)
			}
		}
	}
}

func TestFlatPathSeverityDependsOnPlatformAndIssue(t *testing.T) {
	base := filepath.Join(t.TempDir(), "s")
	directory := base + strings.Repeat("a", FlatMaxPath-len(filepath.Join(base, "x"))+1-24)
	for _, platform := range []string{"windows", "darwin", "linux"} {
		if severity, message := flatPathIssueForOS(directory, strings.Repeat("x", 24), platform); severity != "" || message != "" {
			t.Fatalf("%s: exact budget: %s %s", platform, severity, message)
		}
		severity, message := flatPathIssueForOS(directory, strings.Repeat("x", 25), platform)
		if platform == "windows" {
			if severity != "warning" || !strings.Contains(message, "1-byte reduction required") {
				t.Fatalf("expected Windows warning: %s %s", severity, message)
			}
		} else if severity != "" || message != "" {
			t.Fatalf("unexpected %s path diagnostic: %s %s", platform, severity, message)
		}
		severity, message = flatPathIssueForOS(directory, strings.Repeat("x", 65), platform)
		if severity != "error" || !strings.Contains(message, "65 bytes") || !strings.Contains(message, "do not hand-rename") {
			t.Fatalf("%s: basename limit must remain an error: %s %s", platform, severity, message)
		}
	}
}

func TestDeckRecordRankBudgetIsExplicit(t *testing.T) {
	if _, err := FlatDeckFilename("target", 10000); err == nil {
		t.Fatal("rank wider than the fixed sort field was accepted")
	}
}
