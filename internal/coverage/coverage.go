package coverage

import (
	"context"
	"fmt"
	"sort"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Resolver views a code reference at a commit. *coderesolve.Resolver is the
// production implementation.
type Resolver interface {
	Resolve(ctx context.Context, reference coderef.Reference, view string) coderesolve.Resolution
}

type Summary struct {
	Total       int `json:"total"`
	Covered     int `json:"covered"`
	Uncovered   int `json:"uncovered"`
	Overlapping int `json:"overlapping"`
	Stale       int `json:"stale"`
	Remapped    int `json:"remapped"`
	SagaChanges int `json:"saga_changes"`
}

// Assignment names one reference: the target that owns it, its evidence
// record, and its 1-based position in that record.
type Assignment struct {
	Target       string `json:"target"`
	EvidenceFile string `json:"evidence_file"`
	Reference    int    `json:"reference"`
	// Inherited is set, with no evidence file or position, when an Item
	// accounts for the atom through a saved inventory selection rather than
	// an authored evidence record.
	Inherited *Inheritance `json:"inherited,omitempty"`
}

type Overlap struct {
	Atom      gitdiff.Atom `json:"atom"`
	CoveredBy []Assignment `json:"covered_by"`
}

// StaleReference is a reference that is current at neither side of the
// comparison: its content changed, disappeared, or failed verification. It
// accounts for nothing until it is re-authored.
type StaleReference struct {
	Assignment Assignment        `json:"assignment"`
	Reference  coderef.Reference `json:"reference"`
	Reason     string            `json:"reason"`
}

type TargetSummary struct {
	Target  string `json:"target"`
	Covered int    `json:"covered"`
}

// Report is a machine-consumed contract. Every collection field is always
// present and never null, so an agent can index into it without first testing
// for a missing key: a complete saga reports "uncovered": [] rather than
// omitting the field or emitting null.
//
// Coverage is computed per comparison and never stored: every changed atom of
// BaseOID..HeadOID must lie inside a reference that is current at the side the
// atom lives on. Added lines are matched at the head commit, deleted lines at
// the merge-base, and file events by whole-file references, which account for
// no lines.
type Report struct {
	Complete        bool                    `json:"complete"`
	CoverageScope   string                  `json:"coverage_scope"`
	Summary         Summary                 `json:"summary"`
	Uncovered       []gitdiff.Atom          `json:"uncovered"`
	Overlaps        []Overlap               `json:"overlaps"`
	StaleReferences []StaleReference        `json:"stale_references"`
	Targets         []TargetSummary         `json:"targets"`
	SagaChanges     []gitdiff.Atom          `json:"saga_changes"`
	Repository      string                  `json:"repository"`
	Base            string                  `json:"base"`
	Head            string                  `json:"head"`
	BaseOID         string                  `json:"base_oid"`
	HeadOID         string                  `json:"head_oid"`
	SchemaValid     bool                    `json:"schema_valid"`
	SchemaIssues    []saga.Issue            `json:"schema_issues"`
	Ownership       map[string][]Assignment `json:"-"`
}

type summaryAssignment struct {
	owners      int
	firstTarget string
}

// atomIndex finds the atoms inside a resolved location without scanning the
// comparison: line atoms by commit and path sorted by line, and event atoms by
// their whole-file location.
type atomIndex struct {
	lines  map[string][]int
	events map[string][]int
	atoms  []gitdiff.Atom
}

func Evaluate(ctx context.Context, document *saga.Saga, validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	return EvaluateTargets(ctx, func(visit func(string, []saga.CodeFile)) { WalkDocumentCode(document, visit) }, validation, changes, resolver)
}

// EvaluateTargets computes coverage over the targets walk visits, with
// exactly Evaluate's semantics. A pull request review evaluates its own deck's
// Items this way, against its own range, apart from the documentation tree.
func EvaluateTargets(ctx context.Context, walk func(visit func(string, []saga.CodeFile)), validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	return evaluateTargets(ctx, walk, nil, validation, changes, resolver)
}

func evaluateTargets(ctx context.Context, walk func(visit func(string, []saga.CodeFile)), inherited []InheritedReference, validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	report := newReport(validation, changes)
	assignments := make([][]Assignment, len(changes.Atoms))
	index := buildIndex(changes)
	walk(func(target string, files []saga.CodeFile) {
		visitReferences(ctx, target, files, index, changes, resolver, &report, func(atom int, assignment Assignment) {
			assignments[atom] = append(assignments[atom], assignment)
		})
	})
	visitInherited(ctx, inherited, index, changes, resolver, &report, func(atom int, assignment Assignment) {
		assignments[atom] = append(assignments[atom], assignment)
	})

	targetCounts := map[string]int{}
	for i, atom := range changes.Atoms {
		owners := assignments[i]
		if len(owners) > 0 {
			// The per-atom slice is complete and never mutated again, so transfer
			// its backing array into the report instead of copying every owner.
			report.Ownership[atom.Key] = owners
		}
		switch len(owners) {
		case 0:
			report.Uncovered = append(report.Uncovered, atom)
		case 1:
			targetCounts[owners[0].Target]++
		default:
			report.Overlaps = append(report.Overlaps, Overlap{Atom: atom, CoveredBy: owners})
			seen := map[string]bool{}
			for _, owner := range owners {
				if !seen[owner.Target] {
					targetCounts[owner.Target]++
					seen[owner.Target] = true
				}
			}
		}
	}
	finishReport(&report, targetCounts, len(changes.Atoms), len(report.Uncovered), len(report.Overlaps), len(changes.SagaChanges))
	return report
}

// SelectTarget returns the changed atoms inside one narrative target's
// references. It does not construct ownership for any sibling target. Lazy
// linked-code endpoints use it after reading only the source files named by
// that target.
func SelectTarget(ctx context.Context, files []saga.CodeFile, changes gitdiff.ChangeSet, resolver Resolver) []gitdiff.Atom {
	selected := make([]bool, len(changes.Atoms))
	report := Report{StaleReferences: []StaleReference{}}
	visitReferences(ctx, "", files, buildIndex(changes), changes, resolver, &report, func(atom int, _ Assignment) {
		selected[atom] = true
	})
	matched := make([]gitdiff.Atom, 0)
	for index, ok := range selected {
		if ok {
			matched = append(matched, changes.Atoms[index])
		}
	}
	return matched
}

// EvaluateSummary computes coverage verdicts and per-target rollups without
// retaining atom-level ownership, uncovered, or overlap details. Overview-style
// queries use it because their bounded response needs counts, not the complete
// reverse indexes that gap, fragment, and atom-owner queries traverse.
func EvaluateSummary(ctx context.Context, document *saga.Saga, validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	return EvaluateSummaryInherited(ctx, document, nil, validation, changes, resolver)
}

// EvaluateSummaryInherited is EvaluateSummary plus inherited selections.
func EvaluateSummaryInherited(ctx context.Context, document *saga.Saga, inherited []InheritedReference, validation saga.Validation, changes gitdiff.ChangeSet, resolver Resolver) Report {
	report := newReport(validation, changes)
	assignments := make([]summaryAssignment, len(changes.Atoms))
	otherTargets := map[int][]string{}
	index := buildIndex(changes)
	WalkDocumentCode(document, func(target string, files []saga.CodeFile) {
		visitReferences(ctx, target, files, index, changes, resolver, &report, func(atom int, _ Assignment) {
			addSummaryAssignment(assignments, otherTargets, atom, target)
		})
	})
	visitInherited(ctx, inherited, index, changes, resolver, &report, func(atom int, assignment Assignment) {
		addSummaryAssignment(assignments, otherTargets, atom, assignment.Target)
	})

	targetCounts := map[string]int{}
	uncovered, overlapping := 0, 0
	for atomIndex, assignment := range assignments {
		if assignment.owners == 0 {
			uncovered++
			continue
		}
		if assignment.owners > 1 {
			overlapping++
		}
		targetCounts[assignment.firstTarget]++
		for _, target := range otherTargets[atomIndex] {
			targetCounts[target]++
		}
	}
	finishReport(&report, targetCounts, len(changes.Atoms), uncovered, overlapping, len(changes.SagaChanges))
	return report
}

func newReport(validation saga.Validation, changes gitdiff.ChangeSet) Report {
	return Report{
		CoverageScope: "mapping_only",
		Repository:    changes.Repository, Base: changes.Base, Head: changes.Head, BaseOID: changes.BaseOID, HeadOID: changes.HeadOID,
		SchemaValid: validation.Valid, SchemaIssues: nonNil(validation.Issues), SagaChanges: nonNil(changes.SagaChanges),
		Uncovered: []gitdiff.Atom{}, Overlaps: []Overlap{}, StaleReferences: []StaleReference{}, Targets: []TargetSummary{},
		Ownership: make(map[string][]Assignment),
	}
}

func finishReport(report *Report, targetCounts map[string]int, total, uncovered, overlapping, sagaChanges int) {
	for target, count := range targetCounts {
		report.Targets = append(report.Targets, TargetSummary{Target: target, Covered: count})
	}
	sort.Slice(report.Targets, func(i, j int) bool { return report.Targets[i].Target < report.Targets[j].Target })
	report.Summary.Total, report.Summary.Covered, report.Summary.Uncovered = total, total-uncovered, uncovered
	report.Summary.Overlapping, report.Summary.Stale, report.Summary.SagaChanges = overlapping, len(report.StaleReferences), sagaChanges
	report.Complete = report.SchemaValid && uncovered == 0 && len(report.StaleReferences) == 0
}

// nonNil keeps an empty collection encodable as [] instead of null. A nil Go
// slice and an empty one are indistinguishable in code but not in JSON, and
// consumers of the report branch on the JSON.
func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// Sides resolves reference at both sides of the comparison and returns the
// locations where it is current, deduplicated. The second result is the
// reason it is current at neither.
func Sides(ctx context.Context, reference coderef.Reference, changes gitdiff.ChangeSet, resolver Resolver) ([]coderesolve.Resolution, string) {
	head := resolver.Resolve(ctx, reference, changes.HeadOID)
	var current []coderesolve.Resolution
	if head.Current() {
		current = append(current, head)
	}
	if changes.BaseOID != changes.HeadOID {
		base := resolver.Resolve(ctx, reference, changes.BaseOID)
		if base.Current() {
			current = append(current, base)
		}
	}
	if len(current) == 0 {
		return nil, head.Reason
	}
	return current, ""
}

func visitReferences(ctx context.Context, target string, files []saga.CodeFile, index atomIndex, changes gitdiff.ChangeSet, resolver Resolver, report *Report, match func(int, Assignment)) {
	for _, file := range files {
		for i, reference := range file.References {
			assignment := Assignment{Target: target, EvidenceFile: file.Path, Reference: i + 1}
			current, reason := Sides(ctx, reference, changes, resolver)
			if len(current) == 0 {
				report.StaleReferences = append(report.StaleReferences, StaleReference{Assignment: assignment, Reference: reference, Reason: reason})
				continue
			}
			for _, resolution := range current {
				if resolution.Moved {
					report.Summary.Remapped++
					break
				}
			}
			for _, resolution := range current {
				for _, atom := range index.within(resolution.Location) {
					match(atom, assignment)
				}
			}
		}
	}
}

func addSummaryAssignment(assignments []summaryAssignment, otherTargets map[int][]string, atomIndex int, target string) {
	assignment := &assignments[atomIndex]
	assignment.owners++
	if assignment.firstTarget == "" {
		assignment.firstTarget = target
		return
	}
	if assignment.firstTarget == target {
		return
	}
	for _, existing := range otherTargets[atomIndex] {
		if existing == target {
			return
		}
	}
	otherTargets[atomIndex] = append(otherTargets[atomIndex], target)
}

func buildIndex(changes gitdiff.ChangeSet) atomIndex {
	index := atomIndex{lines: map[string][]int{}, events: map[string][]int{}, atoms: changes.Atoms}
	for atomIndex := range changes.Atoms {
		location := changes.Location(changes.Atoms[atomIndex])
		key := location.Commit + "\x00" + location.Path
		if changes.Atoms[atomIndex].Kind == "line" {
			index.lines[key] = append(index.lines[key], atomIndex)
		} else {
			index.events[key] = append(index.events[key], atomIndex)
		}
	}
	for key, values := range index.lines {
		sort.SliceStable(values, func(i, j int) bool { return changes.Atoms[values[i]].Line < changes.Atoms[values[j]].Line })
		index.lines[key] = values
	}
	return index
}

// within returns the atoms a location accounts for: the changed lines of its
// range, or, for a whole file, the file's own events (add, delete, rename,
// mode, type, and binary changes). A whole-file reference does not account
// for the file's changed lines; they need line references, so ownership stays
// as narrow as the explanation.
func (index atomIndex) within(location coderef.Location) []int {
	key := location.Commit + "\x00" + location.Path
	if location.WholeFile() {
		return index.events[key]
	}
	values := index.lines[key]
	start := sort.Search(len(values), func(i int) bool { return index.atoms[values[i]].Line >= location.Start })
	end := sort.Search(len(values), func(i int) bool { return index.atoms[values[i]].Line > location.End })
	return values[start:end]
}

// WalkDocumentCode visits every narrative target's evidence records.
func WalkDocumentCode(document *saga.Saga, visit func(string, []saga.CodeFile)) {
	visit(document.Section.Target, document.Section.Code)
	walkSections(document.Section, func(section *saga.Section) {
		if section != document.Section {
			visit(section.Target, section.Code)
		}
		for _, fragment := range section.Fragments {
			visit(fragment.Target, fragment.Code)
			for landmarkIndex := range fragment.Landmarks {
				landmark := &fragment.Landmarks[landmarkIndex]
				visit(landmark.Target, landmark.Code)
			}
		}
	})
}

func walkSections(section *saga.Section, fn func(*saga.Section)) {
	fn(section)
	for _, child := range section.Children {
		walkSections(child, fn)
	}
}

func DescribeAtom(atom gitdiff.Atom) string {
	if atom.Kind == "event" {
		if atom.Event == "rename" {
			return fmt.Sprintf("rename %s -> %s", atom.OldPath, atom.NewPath)
		}
		return fmt.Sprintf("%s %s", atom.Event, atom.Path)
	}
	return fmt.Sprintf("%s:%d (%s)", atom.Path, atom.Line, atom.Side)
}

// SelectLocations returns the changed atoms inside any of the given locations,
// which must already be resolved to the comparison's commits.
func SelectLocations(changes gitdiff.ChangeSet, locations []coderef.Location) []gitdiff.Atom {
	index := buildIndex(changes)
	selected := make([]bool, len(changes.Atoms))
	for _, location := range locations {
		for _, atom := range index.within(location) {
			selected[atom] = true
		}
	}
	matched := make([]gitdiff.Atom, 0)
	for atom, ok := range selected {
		if ok {
			matched = append(matched, changes.Atoms[atom])
		}
	}
	return matched
}

// ResolvedCode is one reference viewed in a comparison: the locations where it
// is current, or, when there are none, why it is stale.
type ResolvedCode struct {
	Current []coderef.Location `json:"current"`
	Reason  string             `json:"reason,omitempty"`
}

// Resolve views reference at both sides of the comparison.
func Resolve(ctx context.Context, reference coderef.Reference, changes gitdiff.ChangeSet, resolver Resolver) ResolvedCode {
	current, reason := Sides(ctx, reference, changes, resolver)
	result := ResolvedCode{Reason: reason}
	for _, resolution := range current {
		result.Current = append(result.Current, resolution.Location)
	}
	return result
}
