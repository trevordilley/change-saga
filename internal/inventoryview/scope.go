package inventoryview

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// ScopePath is one declared route establishing that a pin belongs to a
// feature: the implementation Item's own documentation pin, then each declared
// member pin at the saved revision of its predecessor.
type ScopePath struct {
	Feature string `json:"feature"`
	Deck    string `json:"deck"`
	Slide   string `json:"slide"`
	Item    string `json:"item"`
	Path    []Pin  `json:"path"`
}

type ScopeEntry struct {
	Target string      `json:"target"`
	Pins   []Pin       `json:"pins"`
	Paths  []ScopePath `json:"paths"`
}

// Scope is a feature's declared technical scope. Truncated says the path
// bound stopped descent; CycleCut that a declared cycle was cut.
type Scope struct {
	Feature   string       `json:"feature"`
	Entries   []ScopeEntry `json:"entries"`
	Truncated bool         `json:"truncated"`
	CycleCut  bool         `json:"cycle_cut"`
}

// FeatureScope follows only declared links from the feature's implementation
// deck Items, at each saved revision. It never substitutes a newer revision:
// a missing pinned revision ends its path and remains an entry.
func (ix *Index) FeatureScope(feature *saga.Feature) Scope {
	scope := Scope{Feature: feature.ID, Entries: []ScopeEntry{}}
	byTarget := map[string]*ScopeEntry{}
	order := []string{}
	visits := 0
	var descend func(base ScopePath, onPath map[string]bool)
	descend = func(base ScopePath, onPath map[string]bool) {
		visits++
		if visits > MaxTraversals {
			scope.Truncated = true
			return
		}
		last := base.Path[len(base.Path)-1]
		entry := byTarget[last.Target]
		if entry == nil {
			entry = &ScopeEntry{Target: last.Target, Pins: []Pin{}, Paths: []ScopePath{}}
			byTarget[last.Target] = entry
			order = append(order, last.Target)
		}
		seen := false
		for _, p := range entry.Pins {
			seen = seen || p == last
		}
		if !seen {
			entry.Pins = append(entry.Pins, last)
		}
		entry.Paths = append(entry.Paths, base)
		rev := ix.revision(last)
		if rev == nil {
			return
		}
		for _, member := range ix.declaredOutgoing(rev) {
			if onPath[member.Target] {
				scope.CycleCut = true
				continue
			}
			if len(base.Path) >= saga.MaxSelectionPath {
				scope.Truncated = true
				continue
			}
			next := base
			next.Path = append(append([]Pin{}, base.Path...), member)
			onPath[member.Target] = true
			descend(next, onPath)
			delete(onPath, member.Target)
		}
	}
	for _, deck := range feature.Decks {
		walkDeck(deck, func(slide *saga.Slide, item *saga.Item) {
			start := ScopePath{Feature: feature.ID, Deck: deck.Target, Slide: slide.Target, Item: item.Target, Path: []Pin{*item.Documentation}}
			descend(start, map[string]bool{item.Documentation.Target: true})
		})
	}
	sort.Strings(order)
	for _, target := range order {
		scope.Entries = append(scope.Entries, *byTarget[target])
	}
	return scope
}

func (ix *Index) revision(p Pin) *requirements.TechnicalRevision {
	if r := ix.records[p.Target]; r != nil {
		return r.Revision(p.Revision)
	}
	return nil
}

// declaredOutgoing lists the pins a revision declares and a selection path may
// follow: System members, data-entity holding Components and relationship
// destinations. Nothing else is inferred.
func (ix *Index) declaredOutgoing(rev *requirements.TechnicalRevision) []Pin {
	pins := append([]Pin{}, rev.Components...)
	for _, holder := range rev.Holders {
		pins = append(pins, holder.Component)
	}
	for _, edge := range rev.Relationships {
		pins = append(pins, edge.Destination)
	}
	return pins
}

// SelectedPin is one revision a query answers about, with its explicit intent
// and its global link status.
type SelectedPin struct {
	Pin    Pin    `json:"pin"`
	Status string `json:"status"`
	Intent Intent `json:"intent"`
}

// Unresolved reason codes. A record with any reason is reported on the
// unresolved page and is never removed by intent/newness filters.
const (
	UnresolvedMissingRecord   = "missing_record"
	UnresolvedMissingRevision = "missing_revision"
	UnresolvedRevisionHeads   = "competing_revision_heads"
	UnresolvedLifecycleHeads  = "competing_lifecycle_heads"
)

// Select returns the revisions a read answers about. Without scope pins it is
// the unique current revision; with scope pins it is exactly those pins.
func (ix *Index) Select(target string, scoped []Pin) ([]SelectedPin, []string) {
	r := ix.records[target]
	reasons := []string{}
	selected := []SelectedPin{}
	if r == nil {
		return selected, append(reasons, UnresolvedMissingRecord)
	}
	if len(scoped) == 0 {
		if r.CurrentRevision == nil {
			reasons = append(reasons, UnresolvedRevisionHeads)
		}
		if r.CurrentLifecycle == nil {
			reasons = append(reasons, UnresolvedLifecycleHeads)
		}
		if len(reasons) > 0 {
			return selected, reasons
		}
		p := Pin{Target: target, Revision: target + ":revision:" + r.CurrentRevision.ID}
		return append(selected, SelectedPin{Pin: p, Status: ix.Status(p), Intent: RevisionIntent(r.CurrentRevision)}), reasons
	}
	for _, p := range scoped {
		rev := r.Revision(p.Revision)
		if rev == nil {
			reasons = append(reasons, UnresolvedMissingRevision)
			continue
		}
		selected = append(selected, SelectedPin{Pin: p, Status: ix.Status(p), Intent: RevisionIntent(rev)})
	}
	if r.CurrentRevision == nil {
		reasons = append(reasons, UnresolvedRevisionHeads)
	}
	if r.CurrentLifecycle == nil {
		reasons = append(reasons, UnresolvedLifecycleHeads)
	}
	return selected, reasons
}
