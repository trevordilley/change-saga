package deprecationview

import (
	"context"
	"github.com/twentyideas/changesaga/internal/requirements"
	"sync"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
)

func TestMatchDoesNotPromoteProvisionalOrStaleEvidence(t *testing.T) {
	reference := coderef.Reference{Commit: "pin", Path: "old.go", Start: 3, End: 5}
	selected := coderef.Location{Commit: "view", Path: "new.go", Start: 4, End: 4}
	moved := coderesolve.Resolution{State: coderesolve.Current, Location: coderef.Location{Commit: "view", Path: "new.go", Start: 3, End: 5}, Moved: true}
	if got := Match(reference, moved, selected); got != "exact" {
		t.Fatalf("verified move: %s", got)
	}
	moved.Provisional = true
	if got := Match(reference, moved, selected); got != "uncertain_path" {
		t.Fatalf("missing pinned commit must stay uncertain: %s", got)
	}
	selected.Path = "old.go"
	selected.Start, selected.End = 100, 100
	stale := coderesolve.Resolution{State: coderesolve.Stale, Location: reference.Location()}
	if got := Match(reference, stale, selected); got != "uncertain_path" {
		t.Fatalf("changed bytes remain visible without claiming overlap: %s", got)
	}
	stale.State = coderesolve.Current
	if got := Match(reference, stale, selected); got != "" {
		t.Fatalf("current disjoint lines are not deprecated: %s", got)
	}
	selected.Path = "unrelated.go"
	if got := Match(reference, moved, selected); got != "" {
		t.Fatalf("unrelated file matched: %s", got)
	}
}

func TestContentSnapshotSelectsBeforeProjectionAndKeepsConflictingHeads(t *testing.T) {
	const target = "urn:change-saga:test:fragment:chosen"
	const other = "urn:change-saga:test:fragment:other"
	doc := requirements.Document{Root: t.TempDir(), SagaID: "test"}
	dep := requirements.Deprecation{Identity: requirements.RecordIdentity{ID: "old"}}
	for i, name := range []string{"a", "b"} {
		urn, _ := requirements.DeprecationRevisionURN("test", "old", name)
		dep.RevisionHeads = append(dep.RevisionHeads, urn)
		dep.Revisions = append(dep.Revisions, requirements.DeprecationRevision{ID: name, Scope: "content", Target: requirements.DeprecationTarget{Target: []string{target, other}[i]}})
	}
	doc.Deprecations = append(doc.Deprecations, dep)
	snapshot := NewContentSnapshot(doc)
	if rows := snapshot.Records(context.Background(), map[string]bool{"unrelated": true}); len(rows) != 0 {
		t.Fatal(rows)
	}
	built := 0
	snapshot.records.Range(func(_, _ any) bool { built++; return true })
	if built != 0 {
		t.Fatal("unrelated target projected an annotation")
	}
	var wait sync.WaitGroup
	for range 12 {
		wait.Go(func() {
			rows := snapshot.Records(context.Background(), map[string]bool{target: true})
			if len(rows) != 1 || rows[0].State != "conflicted" || len(rows[0].Assertions) != 2 {
				t.Error(rows)
			}
		})
	}
	wait.Wait()
	first := snapshot.Records(context.Background(), map[string]bool{target: true})
	second := snapshot.Records(context.Background(), map[string]bool{other: true})
	if &first[0].Assertions[0] != &second[0].Assertions[0] {
		t.Fatal("competing targets did not reuse the immutable projection")
	}
}

type countingResolver struct{ calls int }

func (r *countingResolver) Resolve(_ context.Context, ref coderef.Reference, _ string) coderesolve.Resolution {
	r.calls++
	return coderesolve.Resolution{State: coderesolve.Current, Location: ref.Location()}
}

func TestTargetFilterSkipsUnrelatedEvidenceResolution(t *testing.T) {
	dep := requirements.Deprecation{Identity: requirements.RecordIdentity{ID: "old"}, RevisionHeads: []string{"urn:change-saga:test:deprecation:old:revision:a"}, Revisions: []requirements.DeprecationRevision{{ID: "a", Scope: "code", Target: requirements.DeprecationTarget{Target: "urn:change-saga:test:fragment:other"}, Code: []coderef.Reference{{Path: "old.go"}}}}}
	doc := requirements.Document{Root: t.TempDir(), SagaID: "test", Deprecations: []requirements.Deprecation{dep}}
	resolver := &countingResolver{}
	if rows := Records(context.Background(), doc, "", resolver, Filter{Target: "urn:change-saga:test:fragment:chosen"}); len(rows) != 0 || resolver.calls != 0 {
		t.Fatalf("rows=%v resolutions=%d", rows, resolver.calls)
	}
}
