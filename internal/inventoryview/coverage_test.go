package inventoryview

import (
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestInventoryCoverage(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	base := r.commit(t, map[string]string{"app/store.go": lines(60, "store"), "app/changed.go": lines(10, "changed"), "lib/other.go": lines(5, "other"), "app/logo.bin": "a\x00b"})
	res := r.resolver(t)
	store := r.ref(t, res, base, "app/store.go", 14, 40)
	overlap := r.ref(t, res, base, "app/store.go", 30, 50)
	changed := r.ref(t, res, base, "app/changed.go", 2, 3)
	outside := r.ref(t, res, base, "lib/other.go", 1, 2)
	head := r.commit(t, map[string]string{"app/changed.go": replaceLine(lines(10, "changed"), 2, "edited")})
	sysRev := revision("r1", []coderef.Reference{outside}, pin("component", "store", "r1"), pin("component", "other", "r1"))
	sysRev.Interactions = []requirements.Interaction{{ID: "read", Code: evidence("read", overlap)}}
	inv := &requirements.Inventory{Records: []requirements.TechnicalRecord{
		record("component", "store", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{store, changed})}, "active", false),
		record("system", "flags", []requirements.TechnicalRevision{sysRev}, "active", false),
		record("component", "gone", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{store})}, "retired", false),
		record("component", "split", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{store}), revision("r2", []coderef.Reference{store})}, "active", true),
	}}
	report, err := Coverage(ctx, inv, CoverageInput{Head: head, Paths: []string{"app"}, Files: []string{"app/store.go", "app/changed.go", "app/logo.bin"}}, res)
	if err != nil {
		t.Fatal(err)
	}
	s := report.Summary
	// store 14-40 plus the interaction's 30-50: 37 unique lines, 11 overlapping.
	if s.Files != 2 || s.BinaryFiles != 1 || s.Lines != 70 || s.Covered != 37 || s.Overlapping != 11 || s.Uncovered != 33 {
		t.Fatalf("summary: %+v", s)
	}
	if s.Stale != 1 || report.Stale[0].Owner != ns+"component:store" || s.OutsideScope != 1 || s.Unresolved != 1 || s.Excluded != 1 {
		t.Fatalf("health: %+v %+v", s, report.Stale)
	}
	var overlapRange *CoverageRange
	for i := range report.Ranges {
		if report.Ranges[i].Path == "app/store.go" && report.Ranges[i].Start == 30 {
			overlapRange = &report.Ranges[i]
		}
	}
	if overlapRange == nil || overlapRange.End != 40 || len(overlapRange.Owners) != 2 || overlapRange.Owners[1].Owner != ns+"system:flags#read" {
		t.Fatalf("overlap owners: %+v", report.Ranges)
	}
	if report.Excluded[0].Reasons[0] != ExcludedRetired || report.Unresolved[0].Target != ns+"component:split" {
		t.Fatalf("excluded/unresolved: %+v %+v", report.Excluded, report.Unresolved)
	}
	// The kind filter measures only the chosen owners.
	components, err := Coverage(ctx, inv, CoverageInput{Head: head, Files: []string{"app/store.go"}, Kinds: []string{"component"}}, res)
	if err != nil || components.Summary.Covered != 27 || components.Summary.Overlapping != 0 {
		t.Fatalf("kind filter: %v %+v", err, components.Summary)
	}
	if _, err := Coverage(ctx, inv, CoverageInput{Head: "HEAD"}, res); err == nil {
		t.Fatal("symbolic head accepted")
	}
}
