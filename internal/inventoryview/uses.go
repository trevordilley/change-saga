// Package inventoryview derives read-only projections over the technical
// inventory: reverse uses, declared membership paths, exact selection
// resolution, intent/newness classification and inventory code coverage.
//
// Nothing here is persisted or authoritative. Every projection is rebuilt from
// one loaded Saga document and inventory, follows only declared links, and
// reports truncation, cycles and unresolved facts instead of guessing. A
// documentation pin never transfers code ownership; only an explicit exact
// selection can contribute inherited evidence.
package inventoryview

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Use roles. Review Items are pull-request explanations; they are reported as
// usages but never count as implementation-deck usages.
const (
	RoleImplementationItem = "implementation_item"
	RoleReviewItem         = "review_item"
	RoleSystemMember       = "system_member"
)

const (
	MaxDepth      = 8
	DefaultLimit  = 50
	MaxLimit      = 500
	MaxTraversals = 10000
)

type Pin = saga.DocumentationLink

// Use is one explicit reference to a technical identity. Path lists the
// declared pins from the referencing owner's own pin to the requested target;
// a single-element Path is a direct use.
type Use struct {
	Role    string `json:"role"`
	Feature string `json:"feature,omitempty"`
	Review  string `json:"review,omitempty"`
	Deck    string `json:"deck,omitempty"`
	Slide   string `json:"slide,omitempty"`
	Item    string `json:"item,omitempty"`
	ItemID  string `json:"item_id,omitempty"`
	Label   string `json:"label,omitempty"`
	// Owner and OwnerRevision identify a technical record whose revision
	// declares the link; OwnerCurrent says whether that revision is current.
	Owner         string `json:"owner,omitempty"`
	OwnerRevision string `json:"owner_revision,omitempty"`
	OwnerCurrent  bool   `json:"owner_current,omitempty"`
	Pin           Pin    `json:"pin"`
	Status        string `json:"status"`
	Path          []Pin  `json:"path"`
}

func (u Use) Direct() bool { return len(u.Path) == 1 }

type UseOptions struct {
	// Revision restricts matches to uses pinning exactly this revision URN.
	Revision string
	// Depth is the number of additional declared hops traversed upward from
	// the target; 0 returns direct uses only.
	Depth  int
	Offset int
	Limit  int
	// Roles, when non-empty, keeps only these roles.
	Roles []string
}

// UsePage is complete only when the whole transitive answer was traversed:
// DepthCut says declared owners exist beyond the requested depth, CycleCut
// that a cycle was cut and Truncated that MaxTraversals stopped the walk. An
// empty complete page means no declared use was found, not that none exists
// in undeclared prose or code.
type UsePage struct {
	Uses      []Use `json:"uses"`
	Total     int   `json:"total"`
	Truncated bool  `json:"truncated"`
	DepthCut  bool  `json:"depth_cut"`
	CycleCut  bool  `json:"cycle_cut"`
	Complete  bool  `json:"complete"`
}

type incoming struct {
	use Use // Path holds only the edge's own pin
}

// Index is a disposable, per-snapshot reverse index of declared links.
type Index struct {
	Inventory *requirements.Inventory
	byTarget  map[string][]incoming
	records   map[string]*requirements.TechnicalRecord
}

func Build(document *saga.Saga, inventory *requirements.Inventory) *Index {
	ix := &Index{Inventory: inventory, byTarget: map[string][]incoming{}, records: map[string]*requirements.TechnicalRecord{}}
	if inventory == nil {
		inventory = &requirements.Inventory{}
		ix.Inventory = inventory
	}
	for i := range inventory.Records {
		ix.records[inventory.Records[i].Target] = &inventory.Records[i]
	}
	add := func(u Use) {
		u.Status = inventory.LinkStatus(u.Pin)
		ix.byTarget[u.Pin.Target] = append(ix.byTarget[u.Pin.Target], incoming{u})
	}
	if document != nil {
		for _, feature := range document.Features {
			for _, deck := range feature.Decks {
				walkDeck(deck, func(slide *saga.Slide, item *saga.Item) {
					add(Use{Role: RoleImplementationItem, Feature: feature.ID, Deck: deck.Target, Slide: slide.Target, Item: item.Target, ItemID: item.ID, Label: item.Label, Pin: *item.Documentation})
				})
			}
		}
		for _, review := range document.Reviews {
			walkDeck(review.Deck, func(slide *saga.Slide, item *saga.Item) {
				add(Use{Role: RoleReviewItem, Review: review.ID, Deck: review.Deck.Target, Slide: slide.Target, Item: item.Target, ItemID: item.ID, Label: item.Label, Pin: *item.Documentation})
			})
		}
	}
	for _, r := range inventory.Records {
		for _, rev := range r.Revisions {
			pin := r.Target + ":revision:" + rev.ID
			current := r.CurrentRevision != nil && r.CurrentRevision.ID == rev.ID
			for _, member := range rev.Components {
				add(Use{Role: RoleSystemMember, Owner: r.Target, OwnerRevision: pin, OwnerCurrent: current, Pin: member})
			}
		}
	}
	for target := range ix.byTarget {
		list := ix.byTarget[target]
		sort.SliceStable(list, func(i, j int) bool { return useKey(list[i].use) < useKey(list[j].use) })
	}
	return ix
}

func walkDeck(deck *saga.Deck, visit func(*saga.Slide, *saga.Item)) {
	if deck == nil {
		return
	}
	for _, slide := range deck.Slides {
		for _, item := range slide.Items {
			if item.Documentation != nil {
				visit(slide, item)
			}
		}
	}
}

func useKey(u Use) string {
	return u.Role + "\x00" + u.Feature + "\x00" + u.Review + "\x00" + u.Item + "\x00" + u.OwnerRevision + "\x00" + u.Pin.Revision
}

// Record returns the loaded record for target, or nil.
func (ix *Index) Record(target string) *requirements.TechnicalRecord { return ix.records[target] }

// Uses returns declared uses of target, walking upward through technical
// owners (System membership today) up to Depth extra hops. Owner revisions
// that are not current are still reported, marked OwnerCurrent=false, but are
// not traversed further: a superseded System revision does not make its old
// Item users users of the member.
func (ix *Index) Uses(target string, o UseOptions) UsePage {
	page := UsePage{Uses: []Use{}}
	depth := min(max(o.Depth, 0), MaxDepth)
	limit := o.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	roles := map[string]bool{}
	for _, r := range o.Roles {
		roles[r] = true
	}
	all := []Use{}
	visits := 0
	var walk func(pinTarget, pinRevision string, suffix []Pin, level int, onPath map[string]bool)
	walk = func(pinTarget, pinRevision string, suffix []Pin, level int, onPath map[string]bool) {
		for _, in := range ix.byTarget[pinTarget] {
			if pinRevision != "" && in.use.Pin.Revision != pinRevision {
				continue
			}
			visits++
			if visits > MaxTraversals {
				page.Truncated = true
				return
			}
			u := in.use
			u.Path = append([]Pin{u.Pin}, suffix...)
			all = append(all, u)
			if u.Role != RoleSystemMember || !u.OwnerCurrent {
				continue
			}
			if onPath[u.Owner] {
				page.CycleCut = true
				continue
			}
			if level >= depth {
				if len(ix.byTarget[u.Owner]) > 0 {
					page.DepthCut = true
				}
				continue
			}
			onPath[u.Owner] = true
			walk(u.Owner, u.OwnerRevision, u.Path, level+1, onPath)
			delete(onPath, u.Owner)
		}
	}
	walk(target, o.Revision, nil, 0, map[string]bool{target: true})
	for _, u := range all {
		if len(roles) == 0 || roles[u.Role] {
			page.Uses = append(page.Uses, u)
		}
	}
	page.Total = len(page.Uses)
	start := min(max(o.Offset, 0), page.Total)
	end := min(start+limit, page.Total)
	page.Uses = page.Uses[start:end]
	page.Complete = !page.Truncated && !page.DepthCut && !page.CycleCut
	return page
}

// ImplementationUses reports direct and transitive implementation-deck uses of
// any revision of target, following current owner revisions to MaxDepth. The
// caller must treat an incomplete page as unknown, never as zero uses.
func (ix *Index) ImplementationUses(target string) UsePage {
	return ix.Uses(target, UseOptions{Depth: MaxDepth, Limit: MaxLimit, Roles: []string{RoleImplementationItem}})
}
