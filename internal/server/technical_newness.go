package server

import (
	"context"
	"sync"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/savedview"
)

// Newness is relative to a named comparison, never a property of a proposal.
// Only when the reviewer compares a head against a base does Technical design
// say what is new: an identity absent from the inventory at the comparison's
// merge-base is new; one present there whose revisions have since grown is
// revised; the rest are unchanged. A base whose Saga cannot be read is
// unknown with its reason, never an empty inventory and never "all new".

type technicalNewness struct {
	// Against names the comparison, for example "main".
	Against string
	Known   bool
	Reason  string
	base    map[string]map[string]bool
}

// Of classifies one identity against the base inventory.
func (n *technicalNewness) Of(record *requirements.TechnicalRecord) string {
	if n == nil || !n.Known {
		return "unknown"
	}
	revisions, ok := n.base[record.Target]
	if !ok {
		return "new"
	}
	for _, revision := range record.Revisions {
		if !revisions[revision.ID] {
			return "revised"
		}
	}
	return "unchanged"
}

var technicalBaseCache struct {
	sync.Mutex
	key   string
	value *technicalNewness
}

// comparisonNewness is nil when observing, since there is nothing to be new
// relative to.
func (a *app) comparisonNewness(ctx context.Context, manifest saga.Manifest) *technicalNewness {
	if a.rng.Observe() {
		return nil
	}
	head := a.rng.HeadRevision()
	newness := &technicalNewness{Against: a.rng.Against}
	base, err := gitOutput(ctx, a.sourceDir, "merge-base", "--", a.rng.Against, head)
	if err != nil || base == "" {
		newness.Reason = "the comparison's merge-base could not be resolved"
		return newness
	}
	key := a.root + "\x00" + base
	technicalBaseCache.Lock()
	defer technicalBaseCache.Unlock()
	if technicalBaseCache.key == key {
		return technicalBaseCache.value
	}
	view, err := savedview.Load(ctx, a.sourceDir, a.root, manifest, base)
	if err != nil {
		newness.Reason = err.Error()
		if ctx.Err() == nil {
			technicalBaseCache.key, technicalBaseCache.value = key, newness
		}
		return newness
	}
	newness.Known, newness.base = true, map[string]map[string]bool{}
	for _, record := range view.Inventory.Records {
		revisions := map[string]bool{}
		for _, revision := range record.Revisions {
			revisions[revision.ID] = true
		}
		newness.base[record.Target] = revisions
	}
	technicalBaseCache.key, technicalBaseCache.value = key, newness
	return newness
}
