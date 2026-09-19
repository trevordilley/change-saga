package areas

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

func line(path string, number int) gitdiff.Atom {
	return gitdiff.Atom{Key: path + "#" + string(rune('a'+number)), Kind: "line", Path: path, Side: "new", Line: number}
}

// A first change with only its implementation covered: implementation is
// complete, the chain areas report every line as a gap, and the areas with
// nothing in scope are complete because absence is not failure.
func TestFirstChangeCoversOnlyImplementation(t *testing.T) {
	atoms := []gitdiff.Atom{line("a.go", 1), line("a.go", 2), line("a.go", 3), line("a.go", 7), line("b.go", 1)}
	owners := map[string][]string{}
	for _, atom := range atoms {
		owners[atom.Key] = []string{"item:one"}
	}
	report := Evaluate(Inputs{Scope: Scope{Kind: ScopeChange, Against: "main", Head: "HEAD"}, Atoms: atoms, Owners: owners, TargetEpic: map[string]string{"item:one": "first"}})
	implementation := report.Areas.Implementation
	if implementation.Total != 5 || implementation.Covered != 5 || !implementation.Complete || implementation.Unit != UnitChangedLine {
		t.Fatalf("implementation = %+v", implementation)
	}
	if got := implementation.CoveredEntries[0]; got.Resource != "a.go" || got.Lines != "1-3,7" || got.Count != 4 || got.Epic != "first" {
		t.Fatalf("covered entry = %+v", got)
	}
	for _, area := range []Area{report.Areas.Stories, report.Areas.Personas} {
		if area.Complete || area.Uncovered != 5 || area.Covered != 0 || len(area.UncoveredEntries) != 2 {
			t.Fatalf("%s = %+v", area.Area, area)
		}
	}
	for _, area := range []Area{report.Areas.Design, report.Areas.Quality, report.Areas.Health} {
		if !area.Complete || area.Total != 0 {
			t.Fatalf("%s = %+v", area.Area, area)
		}
	}
}

// The chain is followed one link at a time: a line reaches a story through
// its Item, a persona through the story, and a story in scope brings its
// design and its criteria's tests into the report.
func TestChainFollowsEachLink(t *testing.T) {
	atoms := []gitdiff.Atom{line("a.go", 1), line("b.go", 1), line("c.go", 1)}
	report := Evaluate(Inputs{
		Scope: Scope{Kind: ScopeChange}, Atoms: atoms,
		Owners:         map[string][]string{atoms[0].Key: {"item:served"}, atoms[1].Key: {"item:storied"}},
		TargetStories:  map[string][]string{"item:served": {"story:checkout"}, "item:storied": {"story:refund"}},
		ActivePersonas: map[string]bool{"persona:buyer": true},
		Stories: []Story{
			{URN: "story:checkout", Active: true, Personas: []string{"persona:buyer"}, Criteria: []Criterion{{URN: "story:checkout:criterion:pay"}, {URN: "story:checkout:criterion:receipt"}}},
			{URN: "story:refund", Active: true, Criteria: []Criterion{{URN: "story:refund:criterion:refund"}}},
			{URN: "story:elsewhere", Active: true},
		},
		StoryDesign:    map[string][]string{"story:checkout": {"design:flow"}},
		CriterionTests: map[string][]string{"story:checkout:criterion:pay": {"test:pay"}},
	})
	check := func(area Area, covered, total int) {
		t.Helper()
		if area.Covered != covered || area.Total != total {
			t.Fatalf("%s = %d/%d, want %d/%d: %+v", area.Area, area.Covered, area.Total, covered, total, area)
		}
	}
	check(report.Areas.Implementation, 2, 3)
	check(report.Areas.Stories, 2, 3)
	check(report.Areas.Personas, 1, 3)
	// story:elsewhere is neither reached nor changed, so it is out of scope.
	check(report.Areas.Design, 1, 2)
	check(report.Areas.Quality, 1, 3)
}

// --epic keeps the lines the epic's records own plus the lines no record
// owns, since an unowned line could belong to any epic.
func TestEpicNarrowsScope(t *testing.T) {
	atoms := []gitdiff.Atom{line("a.go", 1), line("b.go", 1), line("c.go", 1)}
	report := Evaluate(Inputs{
		Scope: Scope{Kind: ScopeChange, Epic: "one"}, Atoms: atoms,
		Owners:     map[string][]string{atoms[0].Key: {"item:one"}, atoms[1].Key: {"item:two"}},
		TargetEpic: map[string]string{"item:one": "one", "item:two": "two"},
		Problems:   []Problem{{Resource: "rel:two", Epic: "two", Reason: "stale"}, {Resource: "rel:one", Epic: "one", Reason: "stale"}},
	})
	if area := report.Areas.Implementation; area.Covered != 1 || area.Total != 2 {
		t.Fatalf("implementation = %+v", area)
	}
	if area := report.Areas.Health; area.Uncovered != 1 || area.UncoveredEntries[0].Resource != "rel:one" {
		t.Fatalf("health = %+v", area)
	}
}

// Observing has no change: implementation does not apply, and the chain
// areas count the documentation targets that reference code.
func TestObservingCountsCodeTargets(t *testing.T) {
	report := Evaluate(Inputs{
		Scope: Scope{Kind: ScopeApp}, CodeTargets: []string{"item:a", "item:b"},
		TargetStories: map[string][]string{"item:a": {"story:s"}},
		Stories:       []Story{{URN: "story:s", Active: true}},
	})
	if area := report.Areas.Implementation; !area.Complete || area.Total != 0 || area.Note == "" {
		t.Fatalf("implementation = %+v", area)
	}
	if area := report.Areas.Stories; area.Unit != UnitCodeTarget || area.Covered != 1 || area.Total != 2 {
		t.Fatalf("stories = %+v", area)
	}
	if area := report.Areas.Design; area.Total != 1 || area.Complete {
		t.Fatalf("design = %+v", area)
	}
}

// The report is counts and lists, never one blended number, and every list
// is present even when empty.
func TestReportShapeIsStable(t *testing.T) {
	encoded, err := json.Marshal(Evaluate(Inputs{Scope: Scope{Kind: ScopeApp}}))
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"percent", "score", "ratio", "overall", "null"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("report contains %q: %s", forbidden, text)
		}
	}
	for _, name := range Names() {
		if !strings.Contains(text, `"`+string(name)+`":{"area":"`+string(name)+`"`) {
			t.Fatalf("report lacks area %s: %s", name, text)
		}
	}
}

func TestParse(t *testing.T) {
	names, err := Parse("implementation, stories,implementation")
	if err != nil || len(names) != 2 || names[1] != Stories {
		t.Fatalf("Parse = %v, %v", names, err)
	}
	if _, err := Parse("implementation,tests"); err == nil || !strings.Contains(err.Error(), "areas are implementation, stories") {
		t.Fatalf("unknown area error = %v", err)
	}
	if _, err := Parse(" , "); err == nil {
		t.Fatal("empty list accepted")
	}
}
