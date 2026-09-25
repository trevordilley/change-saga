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

	selections := AttachInherited(doc, inv)
	if len(selections) != 2 || !selections[0].Attached || selections[1].Attached || selections[1].Result.State != "unresolved" {
		t.Fatalf("attachment: %+v", selections)
	}
	if len(good.Code) != 1 || !IsInherited(good.Code[0].Path) || len(doc.Section.Fragments[0].Landmarks[0].Code) != 1 {
		t.Fatalf("inherited evidence not on the Item and its landmark: %+v", good.Code)
	}
	run(t, r.dir, "remote", "add", "origin", "https://example.test/acme/app.git")
	changes, err := gitdiff.Read(ctx, r.dir, "https://example.test/acme/app.git", empty, head)
	if err != nil {
		t.Fatal(err)
	}
	report := coverage.Evaluate(ctx, doc, saga.Validation{Valid: true}, changes, res)
	// 100 added lines plus the file-addition event; 73 unselected lines and
	// the event stay uncovered.
	if report.Summary.Total != 101 || report.Summary.Covered != 27 || report.Summary.Uncovered != 74 {
		t.Fatalf("only the selected subset may count: %+v", report.Summary)
	}
	for _, target := range report.Targets {
		if target.Target == good.Target && target.Covered != 27 {
			t.Fatalf("Item owner: %+v", target)
		}
	}

	// A superseded System pin stays readable but inherits nothing.
	inv.Records[2] = record("system", "flags", []requirements.TechnicalRevision{revision("r1", nil, comp, other), revision("r2", nil, comp, other)}, "active", false)
	good.Code = nil
	doc.Section.Fragments[0].Landmarks[0].Code = nil
	if got := AttachInherited(doc, inv); got[0].Attached || !hasReason(got[0].Result, ReasonNoncurrentPin) || len(good.Code) != 0 {
		t.Fatalf("noncurrent path attached: %+v", got[0])
	}
}
