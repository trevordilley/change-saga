package deprecationview

import (
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
