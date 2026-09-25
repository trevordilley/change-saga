package cli

import (
	"context"
	"sort"
	"strconv"

	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/technicalpolicy"
)

// reconciliationInventory summarizes technical-inventory impact. Counts are
// omission prompts: an affected definition needs reassessment, not an edit.
type reconciliationInventory struct {
	Records           int                     `json:"records"`
	References        int                     `json:"references"`
	Current           int                     `json:"current"`
	Stale             int                     `json:"stale"`
	PreExisting       int                     `json:"pre_existing"`
	Regressions       int                     `json:"regressions"`
	Introduced        int                     `json:"introduced"`
	Unknown           int                     `json:"baseline_unknown"`
	Affected          int                     `json:"affected_by_code_change"`
	PinProblems       int                     `json:"item_pin_problems"`
	Unresolved        int                     `json:"unresolved_records"`
	Unreferenced      int                     `json:"unreferenced_needing_choice"`
	ProposedSkipped   int                     `json:"proposed_references_not_assessed"`
	Exempt            []inventoryUnreferenced `json:"unreferenced_exempt"`
	BaselineAvailable bool                    `json:"baseline_available"`
	Limits            []string                `json:"limits"`
}

type inventoryUnreferenced struct {
	Target string `json:"target"`
	Reason string `json:"reason"` // proposed_intent | new_in_comparison
}

type inventoryReconcileInput struct {
	root, repo, head, base string
	mismatch               bool
	changes                gitdiff.ChangeSet
	document               *saga.Saga
	inventory              *requirements.Inventory
	// baseInventory is nil when the baseline is unknown; baseAbsent means the
	// Saga did not exist at the base.
	baseInventory *requirements.Inventory
	baseKnown     bool
	baseAbsent    bool
	resolver      *coderesolve.Resolver
}

type inventoryRefKey struct {
	owner string
	key   string
}

// reconcileInventory adds technical-inventory entries to the queue:
// definition evidence that is stale at head (classified against the base),
// definitions whose referenced code changed in the comparison, Items whose
// documentation pin is not current, and active definitions without any
// implementation-deck use that are neither explicitly proposed nor new.
func reconcileInventory(ctx context.Context, in inventoryReconcileInput) (reconciliationInventory, []reconciliationTask) {
	summary := reconciliationInventory{Exempt: []inventoryUnreferenced{}, BaselineAvailable: in.baseKnown,
		Limits: []string{"Only declared Item pins, System membership and exact definition references establish inventory impact; undeclared dependencies are a visibility gap.", "Unreferenced definitions need the user's choice; reconciliation never deletes, retires, reclassifies or attaches them to a feature."}}
	tasks := []reconciliationTask{}
	if in.inventory == nil {
		return summary, tasks
	}
	ix := inventoryview.Build(in.document, in.inventory)
	baseline := inventoryview.Baseline{Known: in.baseKnown, Absent: in.baseAbsent, Inventory: in.baseInventory, Commit: in.base}
	baseCurrent := map[inventoryRefKey]bool{}
	baseSeen := map[inventoryRefKey]bool{}
	if in.baseInventory != nil {
		for _, r := range in.baseInventory.Records {
			if r.CurrentRevision == nil {
				continue
			}
			for _, owned := range inventoryview.Evidence(r.CurrentRevision) {
				key := inventoryRefKey{r.Target + owned.Suffix(), owned.Evidence.Key()}
				baseSeen[key] = true
				baseCurrent[key] = in.resolver.Resolve(ctx, owned.Evidence.Reference, in.base).Current()
			}
		}
	}
	// Changed lines per side, for code-change impact on current references.
	changed := map[string]map[int]bool{}
	for _, atom := range in.changes.Atoms {
		if atom.Line == 0 {
			continue
		}
		key := atom.Side + "\x00" + atom.Path
		if changed[key] == nil {
			changed[key] = map[int]bool{}
		}
		changed[key][atom.Line] = true
	}
	countChanged := func(side string, location coderef.Location) int {
		lines := changed[side+"\x00"+location.Path]
		n := 0
		for line := location.Start; line <= location.End; line++ {
			if lines[line] {
				n++
			}
		}
		return n
	}
	usesOf := func(target string) []changeview.Cause {
		causes := []changeview.Cause{}
		page := ix.Uses(target, inventoryview.UseOptions{Depth: inventoryview.MaxDepth, Roles: []string{inventoryview.RoleImplementationItem}, Limit: 20})
		for _, u := range page.Uses {
			causes = append(causes, changeview.Cause{Kind: "declared_use", Via: u.Item, Detail: "implementation Item in feature " + u.Feature + " pins " + u.Pin.Revision + " (" + u.Status + ") through " + strconv.Itoa(len(u.Path)) + " declared hop(s)"})
		}
		if page.Total > len(page.Uses) || !page.Complete {
			causes = append(causes, changeview.Cause{Kind: "declared_use", Detail: strconv.Itoa(page.Total) + " implementation uses in total; page query inventory-uses for all of them"})
		}
		return causes
	}
	for _, r := range in.inventory.Records {
		summary.Records++
		if r.CurrentRevision == nil || r.CurrentLifecycle == nil {
			summary.Unresolved++
			task := inventoryTask(in, r.Target, "inventory_definition", "unresolved")
			task.Because = []changeview.Cause{{Kind: "competing_heads", Detail: "the definition has competing revision or lifecycle heads; reconcile them explicitly with all parents"}}
			tasks = append(tasks, task)
			continue
		}
		if r.CurrentLifecycle.State == "retired" {
			continue
		}
		staleCauses := []changeview.Cause{}
		changeCauses := []changeview.Cause{}
		debt := ""
		for _, owned := range inventoryview.Evidence(r.CurrentRevision) {
			if owned.Intent == technicalpolicy.Proposed {
				// Proposed evidence asserts no implementation currency.
				summary.ProposedSkipped++
				continue
			}
			summary.References++
			owner := r.Target + owned.Suffix()
			ref := owned.Evidence
			resolution := in.resolver.Resolve(ctx, ref.Reference, in.head)
			if resolution.Current() {
				summary.Current++
				if n := countChanged("new", resolution.Location); n > 0 {
					changeCauses = append(changeCauses, changeview.Cause{Kind: "code_change", Via: resolution.Location.String(), Detail: strconv.Itoa(n) + " lines changed in this comparison inside code referenced by " + owner})
				}
				continue
			}
			summary.Stale++
			key := inventoryRefKey{owner, ref.Key()}
			refDebt := ""
			switch {
			case !in.baseKnown:
				refDebt = "baseline_unknown"
				summary.Unknown++
			case !baseSeen[key]:
				refDebt = "introduced"
				summary.Introduced++
			case baseCurrent[key]:
				refDebt = "regression"
				summary.Regressions++
			default:
				refDebt = "pre_existing"
				summary.PreExisting++
			}
			debt = worseDebt(debt, refDebt)
			staleCauses = append(staleCauses, changeview.Cause{Kind: "head_currency", Via: ref.Location().String(), Detail: owner + ": " + resolution.Reason})
		}
		if len(staleCauses) > 0 {
			task := inventoryTask(in, r.Target, "inventory_evidence", debt)
			task.Because = append(staleCauses, usesOf(r.Target)...)
			task.Guidance = "Read the definition and the current code. If the explanation still holds, revise the definition with focused current references; if the component changed meaning, revise its explanation too. Byte currency alone never proves the explanation; reassess the Items that use it."
			tasks = append(tasks, task)
		} else if len(changeCauses) > 0 {
			summary.Affected++
			task := inventoryTask(in, r.Target, "inventory_definition", "reassess")
			task.Because = append(changeCauses, usesOf(r.Target)...)
			task.Guidance = "Code this definition references changed in this comparison. Reassess whether the definition and the Items that use it still explain it; change only what no longer holds."
			tasks = append(tasks, task)
		}
		// Unreferenced definitions ask the user, unless explicitly proposed
		// or new against this named comparison.
		uses := ix.ImplementationUses(r.Target)
		if uses.Total == 0 && uses.Complete {
			switch {
			case inventoryview.RevisionIntent(r.CurrentRevision) == technicalpolicy.Proposed:
				summary.Exempt = append(summary.Exempt, inventoryUnreferenced{r.Target, "proposed_intent"})
			case inventoryview.Newness(r.Target, baseline) == inventoryview.NewnessNew:
				summary.Exempt = append(summary.Exempt, inventoryUnreferenced{r.Target, "new_in_comparison"})
			default:
				summary.Unreferenced++
				task := inventoryTask(in, r.Target, "inventory_unreferenced", "needs_user_choice")
				detail := "no implementation-deck Item pins this definition or a definition that declares it"
				if !in.baseKnown {
					detail += "; the baseline is unknown, so whether it is new cannot be established"
				}
				task.Because = []changeview.Cause{{Kind: "unreferenced", Detail: detail}}
				task.Guidance = "Ask the user how to reconcile " + r.CurrentRevision.Name + ": reference it from the implementation Item that explains it, record an explicit proposal, or retire it with a reason. Do not delete it, infer newness, or attach it to an arbitrary feature."
				tasks = append(tasks, task)
			}
		}
	}
	// Items whose documentation pin is not current.
	for _, feature := range in.document.Features {
		for _, deck := range feature.Decks {
			for _, slide := range deck.Slides {
				for _, item := range slide.Items {
					if item.Documentation == nil {
						continue
					}
					status := in.inventory.LinkStatus(*item.Documentation)
					if status == "current" {
						continue
					}
					summary.PinProblems++
					debt := "reassess"
					if status == "missing" || status == "conflicted" {
						debt = "unresolved"
					}
					task := reconciliationRoute(in.root, in.repo, in.head, in.mismatch, in.document, nil, item.Target, "item")
					task.Debt = debt
					task.Because = []changeview.Cause{{Kind: "documentation_pin", Via: item.Documentation.Revision, Detail: "the Item's pinned definition is " + status + "; read both revisions and repin only if the Item's explanation holds for the newer one"}}
					tasks = append(tasks, task)
				}
			}
		}
	}
	sort.SliceStable(summary.Exempt, func(i, j int) bool { return summary.Exempt[i].Target < summary.Exempt[j].Target })
	return summary, tasks
}

func worseDebt(a, b string) string {
	rank := map[string]int{"": 0, "pre_existing": 1, "baseline_unknown": 2, "introduced": 3, "regression": 4}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func inventoryTask(in inventoryReconcileInput, target, kind, debt string) reconciliationTask {
	commandKind := inventoryKindOf(target)
	id := lastSegment(target)
	task := reconciliationTask{Resource: target, Kind: kind, Debt: debt,
		Inspect: []grammar.Invocation{
			reconciliationQuery(in.root, in.repo, in.head, "query inventory", grammar.V("target", target), grammar.V("history", "true")),
			inventoryUsesInvocation(in.root, target),
		},
		Repair:   []grammar.Invocation{},
		Guidance: "Read the definition, its history and its uses; change it only if its meaning or evidence no longer holds.",
	}
	if commandKind == "component" || commandKind == "system" {
		values := []grammar.Value{grammar.V("id", id), grammar.V("from", ""), grammar.V("revision", "")}
		if r := in.inventory.Find(target); r != nil {
			for _, h := range r.RevisionHeads {
				values = append(values, grammar.V("parent", h))
			}
		}
		task.Repair = append(task.Repair, reconciliationInvoke(commandKind+" revise", in.root, in.repo, in.mismatch, values...))
	}
	return task
}

func lastSegment(urn string) string {
	for i := len(urn) - 1; i >= 0; i-- {
		if urn[i] == ':' {
			return urn[i+1:]
		}
	}
	return urn
}

func inventoryUsesInvocation(root, target string) grammar.Invocation {
	values := []grammar.Value{grammar.V("saga", root), grammar.V("target", target), grammar.V("depth", strconv.Itoa(inventoryview.MaxDepth))}
	invocation := grammar.Invocation{Command: "query inventory-uses", Status: grammar.StatusImplemented, Usage: queryUsage["inventory-uses"], Arguments: []grammar.Argument{}, Inputs: []grammar.Argument{}, Argv: []string{"change-saga", "query", "inventory-uses"}}
	for _, value := range values {
		invocation.Arguments = append(invocation.Arguments, grammar.Argument{Flag: value.Flag, Value: value.Value})
		invocation.Argv = append(invocation.Argv, "--"+value.Flag, value.Value)
	}
	return invocation
}
