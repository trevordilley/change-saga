package coderesolve

import (
	"context"
	"sync"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// PrepareReferences declares the references an evaluation will resolve.
// This performs no I/O. It lets the resolver generate patches only for the
// requested paths while still detecting renames across the whole repository.
// References not declared here retain the ordinary complete-comparison path.
func (resolver *Resolver) PrepareReferences(references []coderef.Reference, views ...string) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	for _, reference := range references {
		for _, view := range views {
			if view == "" || view == reference.Commit {
				continue
			}
			key := [2]string{reference.Commit, view}
			if resolver.planned[key] == nil {
				resolver.planned[key] = map[string]bool{}
				resolver.samples[key] = reference
			}
			if !resolver.planned[key][reference.Path] {
				resolver.planned[key][reference.Path] = true
				delete(resolver.selected, key)
			}
		}
	}
}

// WarmReferences overlaps independent prepared comparisons with at most four
// workers. These comparisons generate only selected patches, so unrelated
// large files do not multiply resident patch memory across workers. A failed
// or unavailable pin is still handled by Resolve, including digest fallback.
func (resolver *Resolver) WarmReferences(ctx context.Context) {
	type job struct {
		key       [2]string
		reference coderef.Reference
	}
	resolver.mu.Lock()
	var pending []job
	for key, plan := range resolver.planned {
		if _, full := resolver.changes[key]; full {
			continue
		}
		if cached, ok := resolver.selected[key]; ok && cached.paths == len(plan) {
			continue
		}
		pending = append(pending, job{key, resolver.samples[key]})
	}
	resolver.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	jobs := make(chan job)
	var workers sync.WaitGroup
	for range min(4, len(pending)) {
		workers.Go(func() {
			for item := range jobs {
				if ctx.Err() != nil {
					continue
				}
				exists, err := resolver.CommitExists(ctx, item.reference.Commit)
				if err != nil || !exists || resolver.verify(ctx, item.reference).reason != "" {
					continue
				}
				_, _ = resolver.resolutionChanges(ctx, item.key[0], item.key[1], item.reference.Path)
			}
		})
	}
	for _, item := range pending {
		select {
		case jobs <- item:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
}

func (resolver *Resolver) resolutionChanges(ctx context.Context, from, to, path string) (map[string]gitdiff.FileChange, error) {
	key := [2]string{from, to}
	resolver.mu.Lock()
	if full, ok := resolver.changes[key]; ok {
		resolver.mu.Unlock()
		return full.byOldPath, full.err
	}
	plan := resolver.planned[key]
	if !plan[path] {
		resolver.mu.Unlock()
		return resolver.treeChanges(ctx, from, to)
	}
	if selected, ok := resolver.selected[key]; ok && selected.paths == len(plan) {
		resolver.mu.Unlock()
		return selected.byOldPath, selected.err
	}
	paths := make([]string, 0, len(plan))
	for path := range plan {
		paths = append(paths, path)
	}
	resolver.mu.Unlock()
	changes, err := gitdiff.TreeChangesForPaths(ctx, resolver.repo, from, to, paths)
	result := changeSet{byOldPath: map[string]gitdiff.FileChange{}, err: err}
	for _, change := range changes {
		if change.OldPath != "" {
			result.byOldPath[change.OldPath] = change
		}
	}
	resolver.mu.Lock()
	// A concurrent caller may have added another path while Git was reading.
	// Never publish the narrower result as an answer to that expanded plan.
	if len(resolver.planned[key]) == len(paths) {
		resolver.selected[key] = preparedChanges{paths: len(paths), changeSet: result}
	}
	resolver.mu.Unlock()
	return result.byOldPath, result.err
}
