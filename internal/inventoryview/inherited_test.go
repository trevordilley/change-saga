package inventoryview

import (
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// A System-level Item selects lines 14-40 of its member's 14-96 evidence.
// Implementation-deck coverage counts exactly those lines, through the Item.
func TestInheritedSelectionCoverage(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	empty := r.commit(t, map[string]string{"README": "x\n"})
	head := r.commit(t, map[string]string{"store.go": lines(100, "store")})
	res := r.resolver(t)
	containing := r.ref(t, res, head, "store.go", 14, 96)
	selected := r.ref(t, res, head, "store.go", 14, 40)
	comp, sys := pin("component", "store", "r1"), pin("system", "flags", "r1")
	other := pin("component", "other", "r1")
	inv := &requirements.Inventory{Records: []requirements.TechnicalRecord{
		record("component", "store", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{containing})}, "active", false),
		record("component", "other", []requirements.TechnicalRevision{revision("r1", nil)}, "active", false),
		record("system", "flags", []requirements.TechnicalRevision{revision("r1", nil, comp, other)}, "active", false),
	}}
	good := item("checkout", "s", "flags", &sys)
	good.Path = "features/checkout/items/flags"
	good.Selections = []saga.ItemSelection{
		{ID: "store-read", Path: []Pin{sys, comp}, Evidence: "e1", Code: selected},
		// Not declared by the System: contributes nothing and stays visible.
		{ID: "stray", Path: []Pin{sys, pin("component", "missing", "r1")}, Evidence: "e1", Code: selected},
	}
	doc := document(map[string][]*saga.Item{"checkout": {good}}, nil)
	doc.Section = &saga.Section{Target: ns + "saga", Fragments: []*saga.Fragment{{Target: ns + "fragment:slide", Landmarks: []saga.Landmark{{Target: good.Target}}}}}

	run(t, r.dir, "remote", "add", "origin", "https://example.test/acme/app.git")
	changes, err := gitdiff.Read(ctx, r.dir, "https://example.test/acme/app.git", empty, head)
	if err != nil {
		t.Fatal(err)
	}
	inherited, selections := InheritedReferences(ctx, doc, inv, head, res)
	if len(selections) != 2 || !selections[0].Attached || selections[1].Attached || selections[1].Result.State != "unresolved" || len(inherited) != 1 {
		t.Fatalf("eligibility: %+v", selections)
	}
	if len(good.Code) != 0 || len(doc.Section.Fragments[0].Landmarks[0].Code) != 0 {
		t.Fatal("inherited code must never be written into the document as authored evidence")
	}
	report := coverage.EvaluateInherited(ctx, doc, inherited, saga.Validation{Valid: true}, changes, res)
	// 100 added lines plus the file-addition event; 73 unselected lines and
	// the event stay uncovered.
	if report.Summary.Total != 101 || report.Summary.Covered != 27 || report.Summary.Uncovered != 74 {
		t.Fatalf("only the selected subset may count: %+v", report.Summary)
	}
	for _, owners := range report.Ownership {
		for _, owner := range owners {
			if owner.Target != good.Target || owner.Inherited == nil || owner.EvidenceFile != "" || owner.Inherited.Selection != "store-read" || len(owner.Inherited.Path) != 2 {
				t.Fatalf("inherited ownership must be labeled, not an evidence record: %+v", owner)
			}
		}
	}
	// Selected bytes that drift contribute nothing and are not reported as a
	// stale evidence record; selection health reports them instead.
	drifted := r.commit(t, map[string]string{"store.go": replaceLine(lines(100, "store"), 20, "edited")})
	changes, err = gitdiff.Read(ctx, r.dir, "https://example.test/acme/app.git", empty, drifted)
	if err != nil {
		t.Fatal(err)
	}
	inherited, selections = InheritedReferences(ctx, doc, inv, drifted, res)
	report = coverage.EvaluateInherited(ctx, doc, inherited, saga.Validation{Valid: true}, changes, res)
	if len(inherited) != 0 || selections[0].Attached || report.Summary.Covered != 0 || len(report.StaleReferences) != 0 || !hasReason(selections[0].Result, ReasonSelectedStale) {
		t.Fatalf("drifted selection: %+v %+v", selections[0], report.Summary)
	}

	// A superseded System pin stays readable but inherits nothing.
	inv.Records[2] = record("system", "flags", []requirements.TechnicalRevision{revision("r1", nil, comp, other), revision("r2", nil, comp, other)}, "active", false)
	if inherited, got := InheritedReferences(ctx, doc, inv, head, res); len(inherited) != 0 || got[0].Attached || !hasReason(got[0].Result, ReasonNoncurrentPin) {
		t.Fatalf("noncurrent path inherited: %+v", got[0])
	}
}
