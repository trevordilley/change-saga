package coverage

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

type referencePreparer interface {
	PrepareReferences([]coderef.Reference, ...string)
}

// preparedWalk captures a walk once so even a streaming caller is not asked
// to traverse twice. Only resolvers that can restrict patch generation need
// the small list of target/file slices retained here.
func preparedWalk(ctx context.Context, walk func(func(string, []saga.CodeFile)), inherited []InheritedReference, changes gitdiff.ChangeSet, resolver Resolver) func(func(string, []saga.CodeFile)) {
	preparer, ok := resolver.(referencePreparer)
	if !ok {
		return walk
	}
	type targetFiles struct {
		target string
		files  []saga.CodeFile
	}
	var targets []targetFiles
	walk(func(target string, files []saga.CodeFile) {
		files = append([]saga.CodeFile(nil), files...)
		for index := range files {
			files[index].References = append([]coderef.Reference(nil), files[index].References...)
			preparer.PrepareReferences(files[index].References, changes.BaseOID, changes.HeadOID)
		}
		targets = append(targets, targetFiles{target: target, files: files})
	})
	for _, reference := range inherited {
		preparer.PrepareReferences([]coderef.Reference{reference.Reference}, changes.BaseOID, changes.HeadOID)
	}
	if warmer, ok := resolver.(interface{ WarmReferences(context.Context) }); ok {
		warmer.WarmReferences(ctx)
	}
	return func(visit func(string, []saga.CodeFile)) {
		for _, target := range targets {
			visit(target.target, target.files)
		}
	}
}
