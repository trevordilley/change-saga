package reviewapp

import (
	"strconv"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

const scaleTarget = "urn:change-saga:scale:saga"

// linkedSession runs the selector-resolution half of build against a synthetic
// saga, so selector identity can be exercised without a repository or the
// review attribution pass that build performs afterwards.
func linkedSession(document *saga.Saga, changes gitdiff.ChangeSet, report coverage.Report) *session {
	service := &session{
		document: document, changes: changes, report: report,
		targets: map[string]*targetEntry{}, selectors: map[string][]selectorEntry{},
		selectorsByAtom: make(map[string][]DiffOwner, len(report.Ownership)),
		fragments:       map[string]fragmentValue{},
	}
	service.indexSection(document.Section, "")
	service.linkOwnership()
	service.resolveStaleSelectors()
	service.sortAtomOwners()
	return service
}

func sagaWithDiffs(target string, files ...saga.CodeFile) *saga.Saga {
	return &saga.Saga{Section: &saga.Section{Kind: "saga", ID: "scale", Target: target, Code: files}}
}

const scaleHead = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func atomFor(index int) gitdiff.Atom {
	suffix := strconv.Itoa(index)
	location := coderef.Location{Commit: scaleHead, Path: "app.go", Start: index + 1, End: index + 1}
	return gitdiff.Atom{Key: "key-" + suffix, Ref: location.String(), Kind: "line", Path: "app.go", Side: "new", Line: index + 1, Content: "line " + suffix}
}

// referenceFor references atomFor(index)'s line, or a line no atom holds when
// index is negative.
func referenceFor(index int, note string) coderef.Reference {
	line := index + 1
	if index < 0 {
		line = 100000 - index
	}
	return coderef.Reference{Commit: scaleHead, Path: "app.go", Start: line, End: line, Digest: coderef.DigestBytes(nil), Note: note}
}

func TestSelectorIdentityResolvesEveryAssignment(t *testing.T) {
	first := saga.CodeFile{Version: 2, Path: "___code/root.json", References: []coderef.Reference{
		referenceFor(0, "first"), referenceFor(1, "second"), referenceFor(2, "third"),
	}}
	second := saga.CodeFile{Version: 2, Path: "___code/extra.json", References: []coderef.Reference{referenceFor(1, "extra")}}
	atoms := []gitdiff.Atom{atomFor(0), atomFor(1), atomFor(2)}
	report := coverage.Report{Ownership: map[string][]coverage.Assignment{
		"key-0": {{Target: scaleTarget, EvidenceFile: "___code/root.json", Reference: 1}},
		// The unnormalized path must still resolve to the stored evidence file.
		"key-1": {{Target: scaleTarget, EvidenceFile: "___code/../___code/root.json", Reference: 2}, {Target: scaleTarget, EvidenceFile: "___code/extra.json", Reference: 1}},
		"key-2": {{Target: scaleTarget, EvidenceFile: "___code/root.json", Reference: 3}},
	}}
	service := linkedSession(sagaWithDiffs(scaleTarget, first, second), gitdiff.ChangeSet{Atoms: atoms}, report)

	selectors := service.selectors[scaleTarget]
	if len(selectors) != 4 {
		t.Fatalf("selectors = %d, want 4", len(selectors))
	}
	for index, want := range []string{atomFor(0).Ref, atomFor(1).Ref, atomFor(2).Ref, atomFor(1).Ref} {
		entry := selectors[index]
		if len(entry.selector.Atoms) != 1 || entry.selector.Status != "current" {
			t.Fatalf("selector %d (%s) = %#v, want exactly one current atom", index, want, entry.selector)
		}
		if entry.selector.Atoms[0].Ref != want {
			t.Fatalf("selector %d owns %q, want %q", index, entry.selector.Atoms[0].Ref, want)
		}
	}
	owners := service.selectorsByAtom["key-1"]
	if len(owners) != 2 || owners[0].EvidenceFile != "___code/extra.json" || owners[0].Note != "extra" {
		t.Fatalf("owners of uri-1 = %#v, want the extra evidence file sorted first", owners)
	}
	if owners[1].EvidenceFile != "___code/root.json" || owners[1].Note != "second" {
		t.Fatalf("second owner of uri-1 = %#v, want the root evidence file note", owners[1])
	}
	if owners[0].Reference != atomFor(1).Ref {
		t.Fatalf("owner reference = %q, want the location %q", owners[0].Reference, atomFor(1).Ref)
	}
}

func TestSelectorIdentityKeepsFirstEntryForDuplicateEvidencePaths(t *testing.T) {
	// Absolute evidence paths are redacted to the empty string, so two files
	// under one target can share a selector identity. The scan the index
	// replaced stopped at the first match, and that must not change.
	first := saga.CodeFile{Version: 2, Path: "/private/one.json", References: []coderef.Reference{referenceFor(0, "first file")}}
	second := saga.CodeFile{Version: 2, Path: "/private/two.json", References: []coderef.Reference{referenceFor(1, "second file")}}
	report := coverage.Report{Ownership: map[string][]coverage.Assignment{
		"key-0": {{Target: scaleTarget, EvidenceFile: "/private/two.json", Reference: 1}},
	}}
	service := linkedSession(sagaWithDiffs(scaleTarget, first, second), gitdiff.ChangeSet{Atoms: []gitdiff.Atom{atomFor(0)}}, report)

	selectors := service.selectors[scaleTarget]
	if len(selectors[0].selector.Atoms) != 1 || selectors[0].selector.Status != "current" {
		t.Fatalf("first selector = %#v, want the atom attributed to it", selectors[0].selector)
	}
	// Holding no changed atom is not staleness: only coverage decides that.
	if len(selectors[1].selector.Atoms) != 0 || selectors[1].selector.Status != "current" {
		t.Fatalf("second selector = %#v, want no atoms", selectors[1].selector)
	}
	if owners := service.selectorsByAtom["key-0"]; len(owners) != 1 || owners[0].Note != "first file" {
		t.Fatalf("owners = %#v, want the first duplicate entry", owners)
	}
}

func TestCleanDiagnosticPathRedactsPortableAbsolutePaths(t *testing.T) {
	for _, value := range []string{
		"/private/evidence.json",
		`C:\private\evidence.json`,
		"C:/private/evidence.json",
		`\\server\share\evidence.json`,
	} {
		if got := cleanDiagnosticPath(value); got != "" {
			t.Errorf("cleanDiagnosticPath(%q) = %q, want redacted", value, got)
		}
	}
}

func TestStaleSelectorsReuseCoverageStaleReasons(t *testing.T) {
	file := saga.CodeFile{Version: 2, Path: "___code/root.json", References: []coderef.Reference{
		referenceFor(0, "matched"), referenceFor(-1, "changed since its pin"), referenceFor(-2, "explains unchanged code"),
	}}
	report := coverage.Report{
		Ownership: map[string][]coverage.Assignment{"key-0": {{Target: scaleTarget, EvidenceFile: "___code/root.json", Reference: 1}}},
		StaleReferences: []coverage.StaleReference{
			{Assignment: coverage.Assignment{Target: "urn:change-saga:scale:other", EvidenceFile: "___code/root.json", Reference: 2}, Reason: "wrong target"},
			{Assignment: coverage.Assignment{Target: scaleTarget, EvidenceFile: "___code/root.json", Reference: 2}, Reason: "lines changed since the pin"},
		},
	}
	service := linkedSession(sagaWithDiffs(scaleTarget, file), gitdiff.ChangeSet{Atoms: []gitdiff.Atom{atomFor(0)}}, report)

	selectors := service.selectors[scaleTarget]
	if selectors[0].stale != nil || selectors[0].selector.Status != "current" {
		t.Fatalf("matched selector = %#v, want current", selectors[0])
	}
	if selectors[1].stale == nil || selectors[1].stale.Reason != "lines changed since the pin" {
		t.Fatalf("stale reason = %#v, want the reason coverage recorded", selectors[1].stale)
	}
	if selectors[1].stale.Target != scaleTarget || selectors[1].stale.Reference != referenceFor(-1, "changed since its pin") || selectors[1].stale.EvidenceFile != "___code/root.json" {
		t.Fatalf("stale projection = %#v", selectors[1].stale)
	}
	if selectors[2].stale != nil || selectors[2].selector.Status != "current" {
		t.Fatalf("a reference coverage did not report stale = %#v, want current", selectors[2])
	}
}

// scaleFixture holds one synthetic saga whose selectors and atoms grow
// together, which is the shape that made the former per-assignment scan
// quadratic. Only the session is rebuilt per measurement; the fixture data is
// read-only, so it is shared across iterations.
type scaleFixture struct {
	document *saga.Saga
	changes  gitdiff.ChangeSet
	report   coverage.Report
}

func newScaleFixture(size int) scaleFixture {
	file := saga.CodeFile{Version: 2, Path: "___code/root.json", References: make([]coderef.Reference, 0, 2*size)}
	atoms := make([]gitdiff.Atom, 0, size)
	ownership := make(map[string][]coverage.Assignment, size)
	stale := make([]coverage.StaleReference, 0, size)
	for index := 0; index < size; index++ {
		atom := atomFor(index)
		file.References = append(file.References, referenceFor(index, "note "+strconv.Itoa(index)))
		atoms = append(atoms, atom)
		ownership[atom.Key] = []coverage.Assignment{{Target: scaleTarget, EvidenceFile: file.Path, Reference: index + 1}}
	}
	for index := 0; index < size; index++ {
		suffix := strconv.Itoa(index)
		file.References = append(file.References, referenceFor(-index-1, "stale "+suffix))
		stale = append(stale, coverage.StaleReference{
			Assignment: coverage.Assignment{Target: scaleTarget, EvidenceFile: file.Path, Reference: size + index + 1},
			Reason:     "lines changed since the pin",
		})
	}
	return scaleFixture{
		document: sagaWithDiffs(scaleTarget, file),
		changes:  gitdiff.ChangeSet{Atoms: atoms},
		report:   coverage.Report{Ownership: ownership, StaleReferences: stale},
	}
}

func (f scaleFixture) link() *session { return linkedSession(f.document, f.changes, f.report) }

func TestScaleFixtureResolvesEverySelector(t *testing.T) {
	const size = 64
	service := newScaleFixture(size).link()
	current, stale := 0, 0
	for _, entry := range service.selectors[scaleTarget] {
		if entry.stale != nil {
			stale++
			continue
		}
		if len(entry.selector.Atoms) != 1 || entry.selector.Atoms[0].Ref != entry.selector.Reference.Location().String() {
			t.Fatalf("selector %s owns %#v, want its own atom", entry.selector.Reference.Location(), entry.selector.Atoms)
		}
		current++
	}
	if current != size || stale != size {
		t.Fatalf("current = %d, stale = %d, want %d of each", current, stale, size)
	}
}

// TestSelectorLinkingScalesLinearly is the regression for the quadratic scan.
// An eightfold saga costs about eight times as much to link when selector
// identity is a map lookup, and about sixty-four times as much when it is a
// scan, so the allowance below sits well clear of linear noise and well below
// quadratic growth.
func TestSelectorLinkingScalesLinearly(t *testing.T) {
	if testing.Short() {
		t.Skip("scale regression measures work per saga size")
	}
	const (
		base      = 256
		growth    = 8
		allowance = 24.0
	)
	small, large := newScaleFixture(base), newScaleFixture(base*growth)

	// Allocation growth guards the hoisted path normalization; the timing
	// comparison below is what actually catches a reintroduced scan.
	smallAllocs := testing.AllocsPerRun(3, func() { small.link() })
	largeAllocs := testing.AllocsPerRun(3, func() { large.link() })
	if ratio := largeAllocs / smallAllocs; ratio > allowance {
		t.Fatalf("allocations grew %.1fx for %dx the saga (%.0f -> %.0f), want at most %.0fx", ratio, growth, smallAllocs, largeAllocs, allowance)
	}

	// A loaded machine can distort a single measurement, so the best of a few
	// rounds is taken. A reintroduced scan stays near the quadratic ratio in
	// every round, so retrying cannot hide one.
	best := 0.0
	for attempt := 0; attempt < 3; attempt++ {
		smallTime := float64(testing.Benchmark(func(b *testing.B) {
			for b.Loop() {
				small.link()
			}
		}).NsPerOp())
		largeTime := float64(testing.Benchmark(func(b *testing.B) {
			for b.Loop() {
				large.link()
			}
		}).NsPerOp())
		if smallTime <= 0 {
			t.Skip("timing resolution too coarse to compare")
		}
		ratio := largeTime / smallTime
		if best == 0 || ratio < best {
			best = ratio
		}
		if best <= allowance {
			return
		}
	}
	t.Fatalf("linking took %.1fx longer for %dx the saga, want at most %.0fx", best, growth, allowance)
}

func BenchmarkSelectorLinking(b *testing.B) {
	for _, size := range []int{256, 1024, 4096} {
		fixture := newScaleFixture(size)
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				fixture.link()
			}
		})
	}
}
