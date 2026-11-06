package coverage

import (
	"context"
	"fmt"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

type planningResolver struct {
	planned map[string]bool
	missing bool
}

func (resolver *planningResolver) PrepareReferences(references []coderef.Reference, _ ...string) {
	for _, reference := range references {
		resolver.planned[reference.Key()] = true
	}
}

func (resolver *planningResolver) Resolve(ctx context.Context, reference coderef.Reference, view string) coderesolve.Resolution {
	if len(resolver.planned) != 3 || !resolver.planned[reference.Key()] {
		resolver.missing = true
	}
	return (coderesolve.Pinned{}).Resolve(ctx, reference, view)
}

func TestPreparingReferencesPreservesStreamingWalkAndIncludesInheritance(t *testing.T) {
	changes := gitdiff.ChangeSet{BaseOID: testHead, HeadOID: testHead, Atoms: []gitdiff.Atom{
		lineAtom("first.go", "new", 1), lineAtom("second.go", "new", 1), lineAtom("inherited.go", "new", 1),
	}}
	resolver := &planningResolver{planned: map[string]bool{}}
	walks := 0
	walk := func(visit func(string, []saga.CodeFile)) {
		walks++
		files := []saga.CodeFile{{Path: "___code/stream.json", References: make([]coderef.Reference, 1)}}
		for index, path := range []string{"first.go", "second.go"} {
			// A streaming walk may reuse its slice for the next target.
			files[0].References[0] = lineReference(testHead, path, 1, 1)
			visit(fmt.Sprintf("target-%d", index), files)
		}
	}
	inherited := []InheritedReference{{Inheritance: Inheritance{Item: "inherited"}, Reference: lineReference(testHead, "inherited.go", 1, 1)}}
	report := evaluateTargets(context.Background(), walk, inherited, saga.Validation{Valid: true}, changes, resolver)
	if walks != 1 || resolver.missing || report.Summary.Covered != 3 || len(report.Overlaps) != 0 || len(report.StaleReferences) != 0 || len(report.Targets) != 3 {
		t.Fatalf("walks=%d missing preparation=%v summary=%+v targets=%+v", walks, resolver.missing, report.Summary, report.Targets)
	}
}
