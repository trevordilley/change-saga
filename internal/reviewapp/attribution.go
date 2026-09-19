package reviewapp

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitattribution"
	"github.com/twentyideas/changesaga/internal/gitdiff"
)

func attribution(ctx context.Context, resolver *gitattribution.Resolver, path string) Attribution {
	value := resolver.Resolve(ctx, path)
	switch value.State {
	case gitattribution.Committed:
		stamp := value.CommittedAt
		return Attribution{Status: "committed", Commit: value.CommitID, Committer: &Committer{Name: value.Name, Email: value.Email}, CommittedAt: &stamp}
	case gitattribution.Uncommitted:
		return Attribution{Status: "uncommitted"}
	default:
		return Attribution{Status: "history_unavailable"}
	}
}

// atomsWithin returns the comparison's changed atoms inside reference, viewed
// wherever it is current in the comparison.
func (s *session) atomsWithin(ctx context.Context, code coverage.Resolver, reference coderef.Reference) []gitdiff.Atom {
	current, _ := coverage.Sides(ctx, reference, s.changes, code)
	var atoms []gitdiff.Atom
	for _, atom := range s.changes.Atoms {
		location := s.changes.Location(atom)
		for _, resolution := range current {
			if resolution.Location.Contains(location) {
				atoms = append(atoms, atom)
				break
			}
		}
	}
	return atoms
}
