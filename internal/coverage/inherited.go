package coverage

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Inheritance names the saved inventory selection through which an
// implementation Item accounts for code: the Item, the selection, the declared
// path of pins and the evidence ID the selected subset lies within.
type Inheritance struct {
	Item      string                   `json:"item"`
	Selection string                   `json:"selection"`
	Path      []saga.DocumentationLink `json:"path"`
	Evidence  string                   `json:"evidence"`
}

// InheritedReference is one selected subset an Item inherits. Callers pass only
// selections they have already found eligible; there is no evidence record, so
// nothing here may be repaired through evidence-file commands.
type InheritedReference struct {
	Inheritance
	Reference coderef.Reference
}

// EvaluateInherited is Evaluate plus the given inherited selections. Each
// inherited reference is owned by its Item and labeled with its provenance;
// an inherited reference current at neither side contributes nothing and is
// not reported as a stale evidence record (selection health reports it).
func EvaluateInherited(ctx context.Context, document *saga.Saga, inherited []InheritedReference, validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	return evaluateTargets(ctx, func(visit func(string, []saga.CodeFile)) { WalkDocumentCode(document, visit) }, inherited, validation, changes, resolver)
}

func visitInherited(ctx context.Context, inherited []InheritedReference, index atomIndex, changes gitdiff.ChangeSet, resolver Resolver, report *Report, match func(int, Assignment)) {
	for i := range inherited {
		inheritance := inherited[i].Inheritance
		assignment := Assignment{Target: inheritance.Item, Inherited: &inheritance}
		current, _ := Sides(ctx, inherited[i].Reference, changes, resolver)
		for _, resolution := range current {
			for _, atom := range index.within(resolution.Location) {
				match(atom, assignment)
			}
		}
	}
}
