package reviewstate

import (
	"context"
	"sort"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Coverage is how completely a review's deck accounts for its own range: every
// changed line of the range covered by a review Item reference current at the
// side it lives on (added lines at the head, deleted lines at the merge-base),
// and every file event by a whole-file reference. It is the coverage engine's
// semantics applied to the review deck alone, computed per report and never
// stored. It is reported, never a verdict: the team decides what it requires.
//
// Review decks stay out of the documentation's coverage, readiness, and
// comparison layers; this is coverage of the review itself.
type Coverage struct {
	BaseOID string           `json:"base_oid"`
	HeadOID string           `json:"head_oid"`
	Summary coverage.Summary `json:"summary"`
	// Uncovered lists every changed line and file event no review Item
	// accounts for. Each atom's ref is a ready-to-use location for cover --ref.
	Uncovered []gitdiff.Atom `json:"uncovered"`
	// UncoveredFiles groups the uncovered atoms by file, with their locations
	// coalesced into ranges.
	UncoveredFiles  []UncoveredFile           `json:"uncovered_files"`
	Overlaps        []coverage.Overlap        `json:"overlaps"`
	StaleReferences []coverage.StaleReference `json:"stale_references"`
	// Items is how many changed atoms each review Item accounts for.
	Items []coverage.TargetSummary `json:"items"`
}

// UncoveredFile is one file's uncovered atoms.
type UncoveredFile struct {
	Path  string `json:"path"`
	Atoms int    `json:"atoms"`
	// Locations are the uncovered atoms as the fewest code locations: dense
	// line ranges per side, and the whole file for a file event.
	Locations []string `json:"locations"`
}

// ReadCoverage reads the review's range and evaluates its deck's Items over
// it. repository is the Saga's declared source repository; like the rest of a
// review report, the range is read from checkout without verifying its origin.
func ReadCoverage(ctx context.Context, review *saga.Review, rng Range, checkout, repository string, resolver *coderesolve.Resolver) (*Coverage, error) {
	changes, err := gitdiff.ReadWithOptions(ctx, checkout, repository, rng.BaseOID, rng.HeadOID, gitdiff.ReadOptions{AllowRepositoryMismatch: true})
	if err != nil {
		return nil, err
	}
	return Evaluate(ctx, review, changes, resolver), nil
}

// Evaluate computes the review deck's coverage of changes.
func Evaluate(ctx context.Context, review *saga.Review, changes gitdiff.ChangeSet, resolver coverage.Resolver) *Coverage {
	walk := func(visit func(string, []saga.CodeFile)) {
		if review.Deck == nil {
			return
		}
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				visit(item.Target, item.Code)
			}
		}
	}
	report := coverage.EvaluateTargets(ctx, walk, saga.Validation{Valid: true}, changes, resolver)
	return &Coverage{
		BaseOID: changes.BaseOID, HeadOID: changes.HeadOID, Summary: report.Summary,
		Uncovered: report.Uncovered, UncoveredFiles: uncoveredFiles(changes, report.Uncovered),
		Overlaps: report.Overlaps, StaleReferences: report.StaleReferences, Items: report.Targets,
	}
}

// uncoveredFiles groups atoms by the file they live in, in path order.
func uncoveredFiles(changes gitdiff.ChangeSet, atoms []gitdiff.Atom) []UncoveredFile {
	type side struct {
		commit, path string
		whole        bool
		lines        []int
	}
	byPath := map[string]*UncoveredFile{}
	sides := map[string][]*side{}
	for _, atom := range atoms {
		location := changes.Location(atom)
		file := byPath[location.Path]
		if file == nil {
			file = &UncoveredFile{Path: location.Path, Locations: []string{}}
			byPath[location.Path] = file
		}
		file.Atoms++
		var current *side
		for _, candidate := range sides[location.Path] {
			if candidate.commit == location.Commit {
				current = candidate
			}
		}
		if current == nil {
			current = &side{commit: location.Commit, path: location.Path}
			sides[location.Path] = append(sides[location.Path], current)
		}
		if location.WholeFile() {
			current.whole = true
		} else {
			current.lines = append(current.lines, location.Start)
		}
	}
	result := make([]UncoveredFile, 0, len(byPath))
	for path, file := range byPath {
		for _, value := range sides[path] {
			if value.whole {
				file.Locations = append(file.Locations, coderef.Location{Commit: value.commit, Path: value.path}.String())
			}
			sort.Ints(value.lines)
			for index := 0; index < len(value.lines); {
				end := index
				for end+1 < len(value.lines) && value.lines[end+1] <= value.lines[end]+1 {
					end++
				}
				file.Locations = append(file.Locations, coderef.Location{Commit: value.commit, Path: value.path, Start: value.lines[index], End: value.lines[end]}.String())
				index = end + 1
			}
		}
		result = append(result, *file)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}
