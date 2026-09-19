package changeview

import "sort"

// Pair links a record that dropped out with the record that took its place.
// Basis is explicit when an active supersedes relation names the pair,
// inferred when the two are the only records of their kind that explain a
// shared story or criterion, and ambiguous when several could pair: then
// Candidates lists them and an explicit supersedes relation decides.
type Pair struct {
	// Role is replaces on the new record and replaced_by on the old one.
	Role       string   `json:"role"`
	With       string   `json:"with,omitempty"`
	Basis      string   `json:"basis"`
	Shared     []string `json:"shared,omitempty"`
	Candidates []string `json:"candidates,omitempty"`
	// Link is the command that records the pair explicitly when it is
	// ambiguous.
	Link string `json:"link,omitempty"`
}

const (
	PairReplaces   = "replaces"
	PairReplacedBy = "replaced_by"

	PairExplicit  = "explicit"
	PairInferred  = "inferred"
	PairAmbiguous = "ambiguous"
)

// pair finds replacements among the Changed records: every record that
// dropped out (retired or removed) against every record the change added.
func pair(changes []Change, base, head *Inventory) {
	var dropped, added []int
	for index, change := range changes {
		switch change.Change {
		case ChangeRetired:
			dropped = append(dropped, index)
		case ChangeAdded:
			added = append(added, index)
		}
	}
	targetsOf := func(change Change, inventory *Inventory) []string {
		if node := inventory.Nodes[change.URN]; node != nil {
			return node.Targets
		}
		return nil
	}
	// Explicit links win: an active supersedes relation from the new record
	// to the old one.
	explicit := map[int]bool{}
	for _, newIndex := range added {
		for _, old := range head.Supersedes[changes[newIndex].URN] {
			for _, oldIndex := range dropped {
				if changes[oldIndex].URN == old && changes[oldIndex].Kind == changes[newIndex].Kind {
					shared := intersect(targetsOf(changes[oldIndex], base), targetsOf(changes[newIndex], head))
					changes[newIndex].Pair = &Pair{Role: PairReplaces, With: old, Basis: PairExplicit, Shared: shared}
					changes[oldIndex].Pair = &Pair{Role: PairReplacedBy, With: changes[newIndex].URN, Basis: PairExplicit, Shared: shared}
					explicit[newIndex], explicit[oldIndex] = true, true
				}
			}
		}
	}
	candidates := map[int][]int{}
	for _, oldIndex := range dropped {
		if explicit[oldIndex] {
			continue
		}
		oldTargets := targetsOf(changes[oldIndex], base)
		for _, newIndex := range added {
			if explicit[newIndex] || changes[newIndex].Kind != changes[oldIndex].Kind {
				continue
			}
			if len(intersect(oldTargets, targetsOf(changes[newIndex], head))) > 0 {
				candidates[oldIndex] = append(candidates[oldIndex], newIndex)
				candidates[newIndex] = append(candidates[newIndex], oldIndex)
			}
		}
	}
	indexes := make([]int, 0, len(candidates))
	for index := range candidates {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		options := candidates[index]
		role := PairReplacedBy
		if changes[index].Change == ChangeAdded {
			role = PairReplaces
		}
		if len(options) == 1 && len(candidates[options[0]]) == 1 {
			other := options[0]
			oldIndex, newIndex := index, other
			if role == PairReplaces {
				oldIndex, newIndex = other, index
			}
			changes[index].Pair = &Pair{Role: role, With: changes[other].URN, Basis: PairInferred,
				Shared: intersect(targetsOf(changes[oldIndex], base), targetsOf(changes[newIndex], head))}
			continue
		}
		urns := []string{}
		for _, option := range options {
			urns = append(urns, changes[option].URN)
		}
		sort.Strings(urns)
		pairing := &Pair{Role: role, Basis: PairAmbiguous, Candidates: urns}
		if role == PairReplaces && len(urns) > 0 {
			pairing.Link = "change-saga relation add --type supersedes --from " + changes[index].URN + " --to " + urns[0] + " --epic ID --id ID --rationale TEXT <saga>"
		} else if len(urns) > 0 {
			pairing.Link = "change-saga relation add --type supersedes --from " + urns[0] + " --to " + changes[index].URN + " --epic ID --id ID --rationale TEXT <saga>"
		}
		changes[index].Pair = pairing
	}
}

func intersect(left, right []string) []string {
	set := map[string]bool{}
	for _, value := range left {
		set[value] = true
	}
	shared := []string{}
	for _, value := range right {
		if set[value] {
			shared = append(shared, value)
			set[value] = false
		}
	}
	sort.Strings(shared)
	return shared
}
