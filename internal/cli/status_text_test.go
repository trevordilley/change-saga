package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// A compared status passes no verdict and states one count: the header
// names the comparison without a mapped/unmapped framing, the lines nothing
// references are listed one file range per line, and the implementation line
// says what referenced the rest (test-case evidence included), so its
// numbers add up to the listing's.
func TestComparedStatusTextStatesOneCountAndListsRanges(t *testing.T) {
	implementation := areas.Area{
		Area: areas.Implementation, Unit: areas.UnitChangedLine, Total: 20, Covered: 8, Uncovered: 12,
		CoveredEntries: []areas.Entry{
			{Resource: "handler.go", Lines: "1-5", Count: 5, Via: []string{"urn:change-saga:shop:item:handler"}},
			{Resource: "handler_test.go", Lines: "1-3", Count: 3, Via: []string{"urn:change-saga:shop:test-case:charge"}},
		},
		UncoveredEntries: []areas.Entry{
			{Resource: "CHANGELOG.md", Side: "new", Lines: "13-22", Count: 10},
			{Resource: "old.go", Side: "old", Lines: "4,7", Count: 2},
		},
	}
	status := statusDocument{Opening: opening{Mode: gitdiff.ModeCompare, Against: "main", Head: "HEAD", BaseOID: "base", HeadOID: "head"}}
	status.Coverage = areas.Report{Scope: areas.Scope{Kind: areas.ScopeChange, Against: "main", Head: "HEAD"}}
	status.Coverage.Areas.Implementation = implementation
	var output bytes.Buffer
	printReport(&output, status, 100)
	printAreaLine(&output, implementation)
	text := output.String()
	for _, absent := range []string{"MAPPING GAPS", "ALL ATOMS MAPPED", "product changes mapped", "Uncovered:"} {
		if strings.Contains(text, absent) {
			t.Fatalf("status keeps the legacy framing %q:\n%s", absent, text)
		}
	}
	for _, want := range []string{
		"COMPARING main..HEAD",
		"Changed lines nothing references (12 lines in 2 file ranges):\n  CHANGELOG.md:13-22  [10 lines]\n  old.go:4,7 (deleted)  [2 lines]\n",
		"implementation  8/20 changed lines referenced by the Saga (5 by the implementation deck or narrative, 3 by test-case evidence)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text lacks %q:\n%s", want, text)
		}
	}
}
