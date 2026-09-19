package gitdiff

import "context"

// Mode is how a Saga is opened. Observe views the app at one commit and has
// no changed lines; compare views what head changes since its merge-base with
// the commit it is compared against, the way a pull request does.
const (
	ModeObserve = "observe"
	ModeCompare = "compare"
)

// Range is how a Saga is opened. It is never stored: an empty Against
// observes Head, and a set Against compares merge-base(Against, Head)..Head.
// An empty Head is HEAD.
type Range struct {
	Against string `json:"against,omitempty"`
	Head    string `json:"head"`
}

// Observe reports whether the range views one commit.
func (r Range) Observe() bool { return r.Against == "" }

// Mode is observe or compare.
func (r Range) Mode() string {
	if r.Observe() {
		return ModeObserve
	}
	return ModeCompare
}

// HeadRevision is the head revision, HEAD when unset.
func (r Range) HeadRevision() string {
	if r.Head == "" {
		return "HEAD"
	}
	return r.Head
}

// baseRevision is the revision the merge-base is taken with. Observing
// compares head with itself, so the comparison is empty and every reference
// is judged against the viewed commit.
func (r Range) baseRevision() string {
	if r.Observe() {
		return r.HeadRevision()
	}
	return r.Against
}

// ReadRange reads the comparison a Saga is opened with.
func ReadRange(ctx context.Context, fromDir, repositoryURI string, r Range, options ReadOptions) (ChangeSet, error) {
	changes, err := ReadWithOptions(ctx, fromDir, repositoryURI, r.baseRevision(), r.HeadRevision(), options)
	if err != nil {
		return ChangeSet{}, err
	}
	changes.Mode, changes.Base = r.Mode(), r.Against
	return changes, nil
}

// ReadCatalogRange reads the changed-file catalog of the range.
func ReadCatalogRange(ctx context.Context, fromDir, repositoryURI string, r Range, options ReadOptions) (Catalog, error) {
	catalog, err := ReadCatalogWithOptions(ctx, fromDir, repositoryURI, r.baseRevision(), r.HeadRevision(), options)
	if err != nil {
		return Catalog{}, err
	}
	catalog.Mode, catalog.Base = r.Mode(), r.Against
	return catalog, nil
}
