package changeview

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/impact"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Schema names the layers contract.
const Schema = "change-saga.layers/v1"

// Change kinds.
const (
	ChangeAdded   = "added"
	ChangeRevised = "revised"
	ChangeRetired = "retired"
)

// Cause kinds: why a record the change did not edit is affected.
const (
	// CauseCode: code the record references changed, or code was added
	// beside it.
	CauseCode = "code"
	// CausePin: the record, or a relation it holds, pins a revision this
	// change revised or retired.
	CausePin = "pin"
	// CauseChain: the record is reached from an affected record through the
	// persona, story, design, and code chain.
	CauseChain = "chain"
)

// Layers is a Saga opened to compare one change.
type Layers struct {
	Schema      string       `json:"schema"`
	Against     string       `json:"against"`
	Head        string       `json:"head"`
	BaseOID     string       `json:"base_oid"`
	HeadOID     string       `json:"head_oid"`
	Saga        SagaSides    `json:"saga"`
	Summary     Summary      `json:"summary"`
	Changed     []Change     `json:"changed"`
	Affected    []Affected   `json:"affected"`
	Code        CodeLayer    `json:"code"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// SagaSides says which Saga snapshots the Changed layer compares.
type SagaSides struct {
	Base SagaSide `json:"base"`
	Head SagaSide `json:"head"`
}

// SagaSide is one Saga snapshot: the Saga commit it was read at, or the
// working tree, or absent when the Saga did not exist yet.
type SagaSide struct {
	// Commit is the Saga repository commit, empty for the working tree.
	Commit string `json:"commit,omitempty"`
	// Source is git, working_tree, or absent.
	Source string `json:"source"`
}

// Saga side sources.
const (
	SideGit     = "git"
	SideWorking = "working_tree"
	SideAbsent  = "absent"
)

type Summary struct {
	Changed      int `json:"changed"`
	Affected     int `json:"affected"`
	CodeGroups   int `json:"code_groups"`
	ChangedLines int `json:"changed_lines"`
	Unreferenced int `json:"unreferenced_lines"`
}

// NodeRef names a record.
type NodeRef struct {
	URN   string `json:"urn"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Epic  string `json:"epic,omitempty"`
}

// Change is one record the change added, revised, or retired, with its
// before and after. A record that no longer exists at head is retired and
// Removed.
type Change struct {
	NodeRef
	Change  string    `json:"change"`
	Removed bool      `json:"removed,omitempty"`
	Before  *Snapshot `json:"before,omitempty"`
	After   *Snapshot `json:"after,omitempty"`
}

// Snapshot is a record at one side of the comparison.
type Snapshot struct {
	Revision string   `json:"revision"`
	Title    string   `json:"title"`
	State    string   `json:"state,omitempty"`
	Text     string   `json:"text,omitempty"`
	Files    []string `json:"files"`
}

// Affected is a record the change did not edit but invalidated.
type Affected struct {
	NodeRef
	Revision string  `json:"revision"`
	Because  []Cause `json:"because"`
}

// Cause is one reason a record is affected. Via names the record it came
// through: the changed relation endpoint, or the affected record one step
// down the chain.
type Cause struct {
	Kind   string `json:"kind"`
	Via    string `json:"via,omitempty"`
	Detail string `json:"detail"`
}

// CodeLayer is the changed code: hunks grouped under the records that
// reference them, and every changed line no record references.
type CodeLayer struct {
	Groups       []CodeGroup `json:"groups"`
	Unreferenced []Hunk      `json:"unreferenced"`
}

// CodeGroup is the changed code one record references. Layer is changed or
// affected when the record is in that layer.
type CodeGroup struct {
	NodeRef
	Layer string `json:"layer,omitempty"`
	Hunks []Hunk `json:"hunks"`
}

// Hunk is one contiguous run of changed lines in one file, in diff order, or
// one file event.
type Hunk struct {
	Path  string     `json:"path"`
	Event string     `json:"event,omitempty"`
	Lines []HunkLine `json:"lines,omitempty"`
}

// HunkLine is one changed line: side old (deleted, numbered at the
// merge-base) or new (added, numbered at head).
type HunkLine struct {
	Side    string `json:"side"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	Key     string `json:"key"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Options is one opened comparison.
type Options struct {
	// SagaRoot and Document are the opened Saga, which the reviewer shows and
	// Report evaluated.
	SagaRoot string
	Document *saga.Saga
	Changes  gitdiff.ChangeSet
	Report   coverage.Report
	Resolver coverage.Resolver
	// Location is where the Saga lives in its own repository.
	Location Location
	// Base and Head are the Saga repository commits the Changed layer reads
	// the Saga at. An empty Head is the working tree; an empty Base means the
	// Saga did not exist at the base.
	Base, Head string
}

// Snapshot pairs a loaded Saga snapshot with its inventory.
type snapshot struct {
	root      string
	document  *saga.Saga
	inputs    livingapp.StatusInputs
	inventory *Inventory
}

// Compute opens the comparison and derives its three layers.
func Compute(ctx context.Context, options Options) (Layers, *Inventory, error) {
	layers := Layers{
		Schema: Schema, Against: options.Changes.Base, Head: options.Changes.Head,
		BaseOID: options.Changes.BaseOID, HeadOID: options.Changes.HeadOID,
		Changed: []Change{}, Affected: []Affected{}, Code: CodeLayer{Groups: []CodeGroup{}, Unreferenced: []Hunk{}}, Diagnostics: []Diagnostic{},
	}
	temp, err := os.MkdirTemp("", "change-saga-compare-")
	if err != nil {
		return Layers{}, nil, err
	}
	defer os.RemoveAll(temp)

	head, err := loadSnapshot(ctx, options.SagaRoot, options.Document)
	if err != nil {
		return Layers{}, nil, fmt.Errorf("load the opened Saga: %w", err)
	}
	opened := head
	layers.Saga.Head = SagaSide{Source: SideWorking}
	if options.Head != "" {
		root, readErr := extract(ctx, options.Location, options.Head, temp+"/head")
		if readErr == nil {
			if at, loadErr := loadSnapshot(ctx, root, nil); loadErr == nil {
				head, layers.Saga.Head = at, SagaSide{Commit: options.Head, Source: SideGit}
			} else {
				readErr = loadErr
			}
		}
		if readErr != nil {
			layers.Diagnostics = append(layers.Diagnostics, Diagnostic{Code: "saga_at_head_unavailable",
				Message: "the Saga could not be read at " + options.Head + " (" + readErr.Error() + "); the opened Saga stands in for head"})
		}
	}
	base := &snapshot{inventory: &Inventory{Nodes: map[string]*Node{}}}
	layers.Saga.Base = SagaSide{Source: SideAbsent}
	if options.Base != "" {
		root, readErr := extract(ctx, options.Location, options.Base, temp+"/base")
		if readErr == nil {
			at, loadErr := loadSnapshot(ctx, root, nil)
			if loadErr == nil {
				base, layers.Saga.Base = at, SagaSide{Commit: options.Base, Source: SideGit}
			} else {
				readErr = loadErr
			}
		}
		if readErr != nil && !errors.Is(readErr, errAbsent) {
			layers.Diagnostics = append(layers.Diagnostics, Diagnostic{Code: "saga_at_base_unreadable",
				Message: "the Saga at " + options.Base + " could not be read (" + readErr.Error() + "); every record is treated as added"})
		}
	}

	changed := diff(base.inventory, head.inventory)
	layers.Changed = changed
	changedByURN := map[string]*Change{}
	for index := range layers.Changed {
		changedByURN[layers.Changed[index].URN] = &layers.Changed[index]
	}

	// Code impact is read from the head Saga's references. When head is the
	// opened Saga, that is what coverage already evaluated; a Saga read at a
	// head commit is evaluated on its own references.
	report, document := options.Report, options.Document
	if head != opened {
		document = head.document
		report = coverage.Evaluate(ctx, head.document, saga.Validation{Valid: true}, options.Changes, options.Resolver)
	}
	graph := livingapp.ImpactGraph(head.inputs)
	projection := impact.AnalyzeGraph(ctx, document, options.Changes, report, options.Changes, "saga_to_diff", nil, graph, options.Resolver)
	affected := newAffectedSet(head.inventory, changedByURN)
	seeds := []string{}
	for _, target := range projection.Targets {
		node := head.inventory.Resolve(target.Target)
		if node == nil {
			continue
		}
		detail := strconv.Itoa(len(target.Changes)) + " changed lines in code it references"
		if target.Action != "must_update" {
			detail = strconv.Itoa(len(target.Changes)) + " lines added beside code it references"
		}
		affected.add(node.URN, Cause{Kind: CauseCode, Detail: detail})
		seeds = append(seeds, node.URN)
	}
	pinned(head.inventory, changedByURN, affected)
	for urn := range affected.causes {
		seeds = append(seeds, urn)
	}
	chain(head.inventory, uniqueSorted(seeds), affected)
	layers.Affected = affected.list()

	layers.Code = codeLayer(options.Changes, report, projection, head.inventory, changedByURN, affected)
	layers.Summary = Summary{Changed: len(layers.Changed), Affected: len(layers.Affected), CodeGroups: len(layers.Code.Groups), ChangedLines: len(options.Changes.Atoms)}
	for _, hunk := range layers.Code.Unreferenced {
		layers.Summary.Unreferenced += max(len(hunk.Lines), 1)
	}
	return layers, head.inventory, nil
}

func loadSnapshot(ctx context.Context, root string, document *saga.Saga) (*snapshot, error) {
	if document == nil {
		loaded, validation, err := saga.Load(root)
		if err != nil {
			return nil, err
		}
		if !validation.Valid {
			return nil, fmt.Errorf("the Saga is not valid in this format")
		}
		document = loaded
	}
	inputs, err := livingapp.LoadStatusInputs(ctx, livingapp.StatusOptions{SagaRoot: root, Document: document})
	if err != nil {
		return nil, err
	}
	return &snapshot{root: root, document: document, inputs: inputs, inventory: Build(root, document, inputs)}, nil
}

// diff compares the Saga at the merge-base with the Saga at head.
func diff(base, head *Inventory) []Change {
	changes := []Change{}
	for _, node := range head.Sorted() {
		before := base.Nodes[node.URN]
		switch {
		case before == nil:
			changes = append(changes, Change{NodeRef: refOf(node), Change: ChangeAdded, After: snapshotOf(node)})
		case before.Revision != node.Revision:
			kind := ChangeRevised
			if node.Retired() && !before.Retired() {
				kind = ChangeRetired
			}
			changes = append(changes, Change{NodeRef: refOf(node), Change: kind, Before: snapshotOf(before), After: snapshotOf(node)})
		}
	}
	for _, node := range base.Sorted() {
		if head.Nodes[node.URN] == nil {
			changes = append(changes, Change{NodeRef: refOf(node), Change: ChangeRetired, Removed: true, Before: snapshotOf(node)})
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].URN < changes[j].URN })
	return changes
}

func refOf(node *Node) NodeRef {
	return NodeRef{URN: node.URN, Kind: node.Kind, Title: node.Title, Epic: node.Epic}
}

func snapshotOf(node *Node) *Snapshot {
	return &Snapshot{Revision: node.Revision, Title: node.Title, State: node.State, Text: node.Text, Files: append([]string{}, node.Files...)}
}

type affectedSet struct {
	inventory *Inventory
	changed   map[string]*Change
	causes    map[string][]Cause
}

func newAffectedSet(inventory *Inventory, changed map[string]*Change) *affectedSet {
	return &affectedSet{inventory: inventory, changed: changed, causes: map[string][]Cause{}}
}

// add records a cause unless the record is itself in the Changed layer: a
// record the change edited is reviewed as changed.
func (set *affectedSet) add(urn string, cause Cause) bool {
	if set.changed[urn] != nil || set.inventory.Nodes[urn] == nil {
		return false
	}
	for _, existing := range set.causes[urn] {
		if existing == cause {
			return false
		}
	}
	set.causes[urn] = append(set.causes[urn], cause)
	return true
}

func (set *affectedSet) list() []Affected {
	result := []Affected{}
	for urn, causes := range set.causes {
		node := set.inventory.Nodes[urn]
		sort.SliceStable(causes, func(i, j int) bool { return causeRank(causes[i].Kind) < causeRank(causes[j].Kind) })
		result = append(result, Affected{NodeRef: refOf(node), Revision: node.Revision, Because: causes})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URN < result[j].URN })
	return result
}

func causeRank(kind string) int {
	switch kind {
	case CauseCode:
		return 0
	case CausePin:
		return 1
	}
	return 2
}

// pinned marks every active relation pinned to an endpoint this change
// revised or retired, and the relation's other endpoint, whose link now
// points at a revision that is no longer current.
func pinned(inventory *Inventory, changed map[string]*Change, affected *affectedSet) {
	for _, relation := range inventory.Sorted() {
		if relation.Kind != KindRelation || relation.Retired() || changed[relation.URN] != nil {
			continue
		}
		for side, endpoint := range relation.Explains {
			target := inventory.Resolve(endpoint)
			if target == nil {
				continue
			}
			change := changed[target.URN]
			if change == nil || change.Change == ChangeAdded {
				continue
			}
			affected.add(relation.URN, Cause{Kind: CausePin, Via: target.URN, Detail: "pins " + target.Kind + " " + target.Title + ", which this change " + change.Change})
			if other := inventory.Resolve(relation.Explains[1-side]); other != nil {
				affected.add(other.URN, Cause{Kind: CausePin, Via: relation.URN, Detail: "its " + relation.relationType + " link to " + target.Title + " pins a revision this change " + change.Change})
			}
		}
	}
}

// chain follows the persona, story, design, and code chain upward from every
// seed: an Item to its slide and deck, a landmark to its fragment, a record to
// what it addresses, explains, verifies, or implements, and a story to the
// personas it serves.
func chain(inventory *Inventory, seeds []string, affected *affectedSet) {
	up := map[string][]string{}
	for _, node := range inventory.Sorted() {
		if node.Parent != "" && node.contained {
			up[node.URN] = append(up[node.URN], node.Parent)
		}
		if node.Kind == KindStory {
			up[node.URN] = append(up[node.URN], node.Links...)
		}
		if node.Kind == KindRelation && !node.Retired() {
			switch node.relationType {
			case "addresses", "explains", "verifies", "implements":
				from, to := inventory.Resolve(node.Explains[0]), inventory.Resolve(node.Explains[1])
				if from != nil && to != nil {
					up[from.URN] = append(up[from.URN], to.URN)
				}
			}
		}
	}
	queue := append([]string{}, seeds...)
	visited := map[string]bool{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if visited[current] {
			continue
		}
		visited[current] = true
		from := inventory.Nodes[current]
		for _, next := range up[current] {
			target := inventory.Resolve(next)
			if target == nil || target.URN == current {
				continue
			}
			affected.add(target.URN, Cause{Kind: CauseChain, Via: current, Detail: "reached from " + from.Kind + " " + from.Title})
			queue = append(queue, target.URN)
		}
	}
}

// codeLayer groups every changed line under the records that reference it:
// references current where the line lives (coverage ownership), and the
// references the change touched at the merge-base (impact projection).
func codeLayer(changes gitdiff.ChangeSet, report coverage.Report, projection impact.Result, inventory *Inventory, changed map[string]*Change, affected *affectedSet) CodeLayer {
	byNode := map[string]map[string]bool{}
	attach := func(target, key string) {
		node := inventory.Resolve(target)
		urn := target
		if node != nil {
			urn = node.URN
		}
		if byNode[urn] == nil {
			byNode[urn] = map[string]bool{}
		}
		byNode[urn][key] = true
	}
	for key, owners := range report.Ownership {
		for _, owner := range owners {
			attach(owner.Target, key)
		}
	}
	for _, target := range projection.Targets {
		for _, change := range target.Changes {
			attach(target.Target, change.Atom.Key)
		}
	}
	layer := CodeLayer{Groups: []CodeGroup{}, Unreferenced: hunks(changes, keySet(report.Uncovered))}
	urns := make([]string, 0, len(byNode))
	for urn := range byNode {
		urns = append(urns, urn)
	}
	sort.Strings(urns)
	for _, urn := range urns {
		group := CodeGroup{NodeRef: NodeRef{URN: urn, Kind: "unknown", Title: urn}, Hunks: hunks(changes, byNode[urn])}
		if node := inventory.Nodes[urn]; node != nil {
			group.NodeRef = refOf(node)
		}
		switch {
		case changed[urn] != nil:
			group.Layer = "changed"
		case len(affected.causes[urn]) > 0:
			group.Layer = "affected"
		}
		layer.Groups = append(layer.Groups, group)
	}
	return layer
}

func keySet(atoms []gitdiff.Atom) map[string]bool {
	keys := map[string]bool{}
	for _, atom := range atoms {
		keys[atom.Key] = true
	}
	return keys
}

// hunks renders the atoms named by keys as contiguous runs in diff order.
// File events become hunks of their own.
func hunks(changes gitdiff.ChangeSet, keys map[string]bool) []Hunk {
	result := []Hunk{}
	if len(keys) == 0 {
		return result
	}
	atoms := map[string]gitdiff.Atom{}
	for _, atom := range changes.Atoms {
		if keys[atom.Key] {
			atoms[atom.Key] = atom
		}
	}
	seen := map[string]bool{}
	var current *Hunk
	last := -2
	for index, line := range changes.DisplayLines {
		atom, ok := atoms[line.AtomKey]
		if !ok || atom.Kind != "line" || seen[atom.Key] {
			continue
		}
		seen[atom.Key] = true
		if current == nil || current.Path != atom.Path || index != last+1 {
			result = append(result, Hunk{Path: atom.Path})
			current = &result[len(result)-1]
		}
		current.Lines = append(current.Lines, HunkLine{Side: atom.Side, Line: atom.Line, Content: atom.Content, Key: atom.Key})
		last = index
	}
	// Lines without display context and file events are listed in atom order.
	for _, atom := range changes.Atoms {
		if !keys[atom.Key] || seen[atom.Key] {
			continue
		}
		seen[atom.Key] = true
		if atom.Kind == "event" {
			result = append(result, Hunk{Path: firstNonEmpty(atom.NewPath, atom.Path), Event: atom.Event})
			continue
		}
		result = append(result, Hunk{Path: atom.Path, Lines: []HunkLine{{Side: atom.Side, Line: atom.Line, Content: atom.Content, Key: atom.Key}}})
	}
	return result
}
