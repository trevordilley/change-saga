package inventoryview

import (
	"context"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

var (
	store1, store2 = pin("component", "store", "r1"), pin("component", "store", "r2")
	reader1        = pin("component", "reader", "r1")
	sys1, sys2     = pin("system", "flags", "r1"), pin("system", "flags", "r2")
)

func usesInventory() *requirements.Inventory {
	return &requirements.Inventory{Records: []requirements.TechnicalRecord{
		record("component", "store", []requirements.TechnicalRevision{revision("r1", nil), revision("r2", nil)}, "active", false),
		record("component", "reader", []requirements.TechnicalRevision{revision("r1", nil)}, "active", false),
		record("component", "orphan", []requirements.TechnicalRevision{revision("r1", nil)}, "active", false),
		// r1 pinned the old store revision; the current r2 pins store:r2.
		record("system", "flags", []requirements.TechnicalRevision{revision("r1", nil, store1, reader1), revision("r2", nil, store2, reader1)}, "active", false),
	}}
}

func TestUsesDirectTransitiveAndRoles(t *testing.T) {
	docSys, docStore, docOld := sys2, store2, store1
	doc := document(map[string][]*saga.Item{
		"checkout": {item("checkout", "s", "system", &docSys), item("checkout", "s", "none", nil)},
		"billing":  {item("billing", "s", "store", &docStore), item("billing", "s", "old", &docOld)},
	}, map[string][]*saga.Item{"pr-7": {item("pr-7", "s", "store", &docStore)}})
	ix := Build(doc, usesInventory())

	direct := ix.Uses(store2.Target, UseOptions{})
	// Direct: billing Item (r2), billing old Item (r1, stale), review Item,
	// System r1 (historical owner) and System r2 (current owner).
	if direct.Total != 5 || !direct.DepthCut || direct.Complete {
		t.Fatalf("direct uses: %+v", direct)
	}
	statuses := map[string]string{}
	for _, u := range direct.Uses {
		if !u.Direct() {
			t.Fatalf("depth 0 returned a transitive use: %+v", u)
		}
		statuses[u.Role+"|"+u.Pin.Revision] = u.Status
	}
	if statuses[RoleImplementationItem+"|"+store1.Revision] != "stale" || statuses[RoleImplementationItem+"|"+store2.Revision] != "current" || statuses[RoleReviewItem+"|"+store2.Revision] != "current" {
		t.Fatalf("pin statuses: %v", statuses)
	}

	deep := ix.Uses(store2.Target, UseOptions{Depth: 2, Roles: []string{RoleImplementationItem}})
	var viaSystem *Use
	for i := range deep.Uses {
		if deep.Uses[i].Feature == "checkout" {
			viaSystem = &deep.Uses[i]
		}
	}
	if viaSystem == nil || len(viaSystem.Path) != 2 || viaSystem.Path[0] != sys2 || viaSystem.Path[1] != store2 {
		t.Fatalf("transitive path through the current System revision: %+v", deep.Uses)
	}
	// The historical System r1 still pins store:r1, but it is not traversed:
	// checkout's Item pins r2, and r1 is not the current owner.
	exact := ix.Uses(store1.Target, UseOptions{Revision: store1.Revision, Depth: 3})
	for _, u := range exact.Uses {
		if u.Pin != store1 {
			t.Fatalf("revision filter leaked %+v", u)
		}
		if u.Role == RoleImplementationItem && u.Feature == "checkout" {
			t.Fatal("superseded owner revision made a transitive use")
		}
		if u.Role == RoleSystemMember && u.OwnerCurrent {
			t.Fatal("historical owner reported current")
		}
	}
	if orphan := ix.Uses(ns+"component:orphan", UseOptions{Depth: MaxDepth}); orphan.Total != 0 || !orphan.Complete {
		t.Fatalf("orphan: %+v", orphan)
	}
	// Paging never loses totals.
	page := ix.Uses(store2.Target, UseOptions{Limit: 2, Offset: 4})
	if page.Total != 5 || len(page.Uses) != 1 {
		t.Fatalf("page: %+v", page)
	}
	if got := ix.ImplementationUses(reader1.Target); got.Total != 1 || got.Uses[0].Feature != "checkout" || len(got.Uses[0].Path) != 2 {
		t.Fatalf("reader implementation uses: %+v", got)
	}
}

func TestUsesBoundsAndCycles(t *testing.T) {
	// Two Systems naming each other are impossible today, but the traversal
	// must still terminate and say so if a future owned link forms a cycle.
	a, b, c := pin("system", "a", "r1"), pin("system", "b", "r1"), pin("component", "c", "r1")
	inv := &requirements.Inventory{Records: []requirements.TechnicalRecord{
		record("system", "a", []requirements.TechnicalRevision{revision("r1", nil, b, c)}, "active", false),
		record("system", "b", []requirements.TechnicalRevision{revision("r1", nil, a)}, "active", false),
		record("component", "c", []requirements.TechnicalRevision{revision("r1", nil)}, "active", false),
	}}
	ix := Build(nil, inv)
	page := ix.Uses(c.Target, UseOptions{Depth: MaxDepth})
	if !page.CycleCut || page.Complete {
		t.Fatalf("cycle not reported: %+v", page)
	}
	shallow := ix.Uses(c.Target, UseOptions{Depth: 0})
	if !shallow.DepthCut || shallow.Complete || shallow.Total != 1 {
		t.Fatalf("depth cut not reported: %+v", shallow)
	}
}

func TestSelectionResolution(t *testing.T) {
	ctx := context.Background()
	r := newRepo(t)
	base := r.commit(t, map[string]string{"store.go": lines(100, "store"), "other.go": lines(10, "other")})
	res := r.resolver(t)
	containing := r.ref(t, res, base, "store.go", 14, 96)
	selected := r.ref(t, res, base, "store.go", 14, 40)
	sysRef := r.ref(t, res, base, "other.go", 1, 3)
	comp := pin("component", "store", "r1")
	sys := pin("system", "flags", "r1")
	other := pin("component", "other", "r1")
	inv := &requirements.Inventory{Records: []requirements.TechnicalRecord{
		record("component", "store", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{containing})}, "active", false),
		record("component", "other", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{sysRef})}, "active", false),
		record("system", "flags", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{sysRef}, comp, other)}, "active", false),
	}}
	// Fixture evidence IDs stand in for the records contract's persisted IDs.
	ids := map[string]string{containing.Location().String(): "store-body", sysRef.Location().String(): "other-body"}
	restore := lookupEvidence
	t.Cleanup(func() { lookupEvidence = restore })
	lookupEvidence = func(rev *requirements.TechnicalRevision, id string) (*coderef.Reference, string) {
		for i := range rev.Code {
			if ids[rev.Code[i].Location().String()] == id {
				return &rev.Code[i], ""
			}
		}
		for _, edge := range rev.Interactions {
			for i := range edge.Code {
				if "edge-"+ids[edge.Code[i].Location().String()] == id {
					return &edge.Code[i], edge.ID
				}
			}
		}
		return nil, ""
	}
	sel := Selection{ID: "sel", Path: []Pin{sys, comp}, Evidence: "store-body", Selected: selected}

	ok := ResolveSelection(ctx, inv, sel, base, res)
	if ok.State != "resolved" || !ok.Eligible || !ok.PinsCurrent || len(ok.Reasons) != 0 || *ok.Containing != containing {
		t.Fatalf("valid selection: %+v", ok)
	}

	cases := map[string]func(Selection) Selection{
		ReasonUndeclaredHop:      func(s Selection) Selection { s.Path = []Pin{sys, pin("component", "orphan", "r1")}; return s },
		ReasonMissingEvidence:    func(s Selection) Selection { s.Evidence = "nope"; return s },
		ReasonInvalidSelection:   func(s Selection) Selection { s.Path = nil; return s },
		ReasonWholeFileSelection: func(s Selection) Selection { s.Selected.Start, s.Selected.End = 0, 0; return s },
		ReasonOutsideEvidence: func(s Selection) Selection {
			s.Selected = r.ref(t, res, base, "store.go", 10, 20)
			return s
		},
		ReasonDigestMismatch: func(s Selection) Selection { s.Selected.Digest = sysRef.Digest; return s },
	}
	for code, mutate := range cases {
		got := ResolveSelection(ctx, inv, mutate(sel), base, res)
		if got.State != "unresolved" || got.Eligible || !hasReason(got, code) {
			t.Fatalf("%s: %+v", code, got)
		}
	}
	// Missing pins cannot be substituted by a newer revision.
	missing := sel
	missing.Path = []Pin{sys, pin("component", "store", "r9")}
	if got := ResolveSelection(ctx, inv, missing, base, res); got.State != "unresolved" || !hasReason(got, ReasonMissingPin) {
		t.Fatalf("missing pin: %+v", got)
	}

	// Pure movement: insert lines above; both stay current and remap.
	moved := r.commit(t, map[string]string{"store.go": "new 1\nnew 2\n" + lines(100, "store")})
	got := ResolveSelection(ctx, inv, sel, moved, res)
	if !got.Eligible || !got.SelectedHealth.Moved || got.SelectedHealth.Location.Start != 16 || !got.ContainingHealth.Current() {
		t.Fatalf("movement: %+v", got)
	}
	// Edit outside the subset but inside the containing evidence: selected
	// bytes stay current while the entity needs semantic reassessment.
	content := "new 1\nnew 2\n" + lines(100, "store")
	outside := r.commit(t, map[string]string{"store.go": replaceLine(content, 2+60, "edited outside subset")})
	got = ResolveSelection(ctx, inv, sel, outside, res)
	if !got.Eligible || got.ContainingHealth.Current() || !hasReason(got, ReasonSemanticReassess) || hasReason(got, ReasonSelectedStale) {
		t.Fatalf("outside edit: %+v", got)
	}
	// Edit inside the subset: selected bytes stale; not eligible.
	inside := r.commit(t, map[string]string{"store.go": replaceLine(content, 2+20, "edited inside subset")})
	got = ResolveSelection(ctx, inv, sel, inside, res)
	if got.Eligible || !hasReason(got, ReasonSelectedStale) || !hasReason(got, ReasonContainingStale) || got.State != "resolved" {
		t.Fatalf("inside edit: %+v", got)
	}
	// Deletion: file removed at the view.
	deleted := r.commit(t, map[string]string{"store.go": ""})
	got = ResolveSelection(ctx, inv, sel, deleted, res)
	if got.Eligible || !hasReason(got, ReasonSelectedStale) {
		t.Fatalf("deletion: %+v", got)
	}

	// A stale (superseded) hop remains readable but is not current coverage;
	// a conflicted record is unresolved.
	inv.Records[2] = record("system", "flags", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{sysRef}, comp, other), revision("r2", []coderef.Reference{sysRef}, comp, other)}, "active", false)
	got = ResolveSelection(ctx, inv, sel, base, res)
	if got.State != "resolved" || got.Eligible || got.PinsCurrent || !hasReason(got, ReasonNoncurrentPin) {
		t.Fatalf("stale hop: %+v", got)
	}
	inv.Records[2] = record("system", "flags", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{sysRef}, comp, other), revision("r2", []coderef.Reference{sysRef}, comp, other)}, "active", true)
	if got = ResolveSelection(ctx, inv, sel, base, res); got.State != "unresolved" || !hasReason(got, ReasonConflictedPin) {
		t.Fatalf("conflicted hop: %+v", got)
	}
	// Evidence IDs are unique across the revision; the owning edge is derived.
	inv.Records[2] = record("system", "flags", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{sysRef}, comp, other)}, "active", false)
	inv.Records[2].Revisions[0].Interactions = []requirements.Interaction{{ID: "read", From: other.Target, To: comp.Target, Description: "reads", Code: []coderef.Reference{containing}}}
	inv.Records[2].CurrentRevision = &inv.Records[2].Revisions[0]
	edge := Selection{ID: "edge", Path: []Pin{sys}, Evidence: "edge-store-body", Selected: selected}
	if got = ResolveSelection(ctx, inv, edge, base, res); !got.Eligible || got.EvidenceOwner != "read" {
		t.Fatalf("interaction evidence: %+v", got)
	}
}

func TestIntentAndNewness(t *testing.T) {
	inv := usesInventory()
	if RevisionIntent(inv.Records[0].CurrentRevision) != technicalpolicy.Unspecified {
		t.Fatal("legacy intent must read unspecified")
	}
	target := inv.Records[0].Target
	if Newness(target, Baseline{}) != NewnessUnknown {
		t.Fatal("unreadable baseline must be unknown, not new")
	}
	if Newness(target, Baseline{Known: true, Absent: true}) != NewnessNew {
		t.Fatal("absent baseline Saga introduces every identity")
	}
	if Newness(target, Baseline{Known: true, Inventory: inv}) != NewnessExisting {
		t.Fatal("revised existing identity reported new")
	}
	if Newness(ns+"component:later", Baseline{Known: true, Inventory: inv}) != NewnessNew {
		t.Fatal("new identity not reported")
	}
}

func hasReason(r SelectionResult, code string) bool {
	for _, reason := range r.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func replaceLine(content string, line int, text string) string {
	parts := splitKeep(content)
	parts[line-1] = text + "\n"
	out := ""
	for _, p := range parts {
		out += p
	}
	return out
}

func splitKeep(s string) []string {
	var out []string
	for len(s) > 0 {
		i := 0
		for i < len(s) && s[i] != '\n' {
			i++
		}
		if i < len(s) {
			i++
		}
		out = append(out, s[:i])
		s = s[i:]
	}
	return out
}

func TestLegacyEvidenceIsNotSelectable(t *testing.T) {
	ref := coderef.Reference{Commit: strings40("a"), Path: "a.go", Start: 1, End: 2, Digest: "sha256:" + strings40("b") + strings40("b")[:24], Note: "x"}
	comp := pin("component", "store", "r1")
	inv := &requirements.Inventory{Records: []requirements.TechnicalRecord{record("component", "store", []requirements.TechnicalRevision{revision("r1", []coderef.Reference{ref})}, "active", false)}}
	sel := Selection{ID: "s", Path: []Pin{comp}, Evidence: ref.Location().String(), Selected: ref}
	if got := ResolveSelection(context.Background(), inv, sel, "", nil); got.State != "unresolved" || !hasReason(got, ReasonMissingEvidence) {
		t.Fatalf("a legacy location became a selectable identity: %+v", got)
	}
}

func strings40(c string) string {
	out := ""
	for len(out) < 40 {
		out += c
	}
	return out
}
