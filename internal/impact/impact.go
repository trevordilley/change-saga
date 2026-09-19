// Package impact projects an incoming Git comparison onto the code references
// of an existing Change Saga. It never compares authored content.
package impact

import (
	"context"
	"sort"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

const Schema = "change-saga.impact/v1"

type SourceIdentity struct {
	Repository string `json:"repository"`
	Base       string `json:"base"`
	Head       string `json:"head"`
	BaseOID    string `json:"base_oid"`
	HeadOID    string `json:"head_oid"`
}

type BaselineStatus struct {
	SagaID      string         `json:"saga_id"`
	Title       string         `json:"title"`
	Source      SourceIdentity `json:"source"`
	Complete    bool           `json:"complete"`
	Covered     int            `json:"covered"`
	Total       int            `json:"total"`
	Uncovered   int            `json:"uncovered"`
	Stale       int            `json:"stale"`
	Overlapping int            `json:"overlapping"`
}

type IncomingStatus struct {
	SagaID string         `json:"saga_id,omitempty"`
	Title  string         `json:"title,omitempty"`
	Source SourceIdentity `json:"source"`
}

type Summary struct {
	IncomingAtoms         int `json:"incoming_atoms"`
	DirectIntersections   int `json:"direct_intersections"`
	ContextualAdditions   int `json:"contextual_additions"`
	NewContentRequired    int `json:"new_content_required"`
	TargetsMustUpdate     int `json:"targets_must_update"`
	TargetsConsiderUpdate int `json:"targets_consider_update"`
	RequirementsImpacted  int `json:"requirements_implicated"`
	TestCasesImpacted     int `json:"test_cases_implicated"`
}

type Change struct {
	Relationship string       `json:"relationship"`
	Atom         gitdiff.Atom `json:"atom"`
}

type TargetImpact struct {
	Target        string   `json:"target"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title"`
	Location      string   `json:"location,omitempty"`
	ContentPath   string   `json:"content_path,omitempty"`
	Action        string   `json:"action"`
	EvidenceFiles []string `json:"evidence_files"`
	Changes       []Change `json:"changes"`
}

type UnownedChange struct {
	Atom   gitdiff.Atom `json:"atom"`
	Reason string       `json:"reason"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Result struct {
	Schema          string          `json:"schema"`
	Mode            string          `json:"mode"`
	Basis           string          `json:"basis"`
	ContentCompared bool            `json:"content_compared"`
	Baseline        BaselineStatus  `json:"baseline"`
	Incoming        IncomingStatus  `json:"incoming"`
	Summary         Summary         `json:"summary"`
	Targets         []TargetImpact  `json:"targets"`
	NewContent      []UnownedChange `json:"new_content"`
	// Requirements and TestCases carry the source change back to the review
	// repository: every story or criterion reached from an affected target
	// through persisted relations, and every test case whose recorded evidence
	// selects a changed atom. They are projections of recorded edges, never
	// inferred from paths, names, or proximity.
	Requirements []RequirementImpact `json:"requirements"`
	TestCases    []TestCaseImpact    `json:"test_cases"`
	Diagnostics  []Diagnostic        `json:"diagnostics"`
}

// Graph is the review-repository side of the projection, supplied by the
// living composition layer so this package stays free of domain loaders.
type Graph struct {
	// Requirements maps an evidence-owning target (a report target or an Item)
	// to the stories and criteria reached from it through persisted relations.
	Requirements map[string][]Reach
	// TestCases lists the current evidence selectors of every test case.
	TestCases []TestCaseEvidence
}

// Reach is one recorded path from an evidence owner to a requirement.
type Reach struct {
	Requirement string   `json:"requirement"`
	Relation    string   `json:"relation"`
	Path        []string `json:"path"`
}

// TestCaseEvidence is one current quality-evidence record's code references.
type TestCaseEvidence struct {
	TestCase string
	Evidence string
	Role     string
	Code     []coderef.Reference
	Criteria []string
}

// RequirementImpact is one story or criterion whose recorded evidence path
// intersects the incoming change.
type RequirementImpact struct {
	Requirement string        `json:"requirement"`
	Action      string        `json:"action"`
	Via         []ImpactRoute `json:"via"`
}

// ImpactRoute names the affected owner and the recorded path to the requirement.
type ImpactRoute struct {
	Target   string   `json:"target"`
	Relation string   `json:"relation,omitempty"`
	Path     []string `json:"path"`
}

// TestCaseImpact is one test case whose evidence selects changed source. Its
// runs pinned to the old comparison stop being current once the change lands.
type TestCaseImpact struct {
	TestCase string   `json:"test_case"`
	Action   string   `json:"action"`
	Evidence []string `json:"evidence"`
	Roles    []string `json:"roles"`
	Criteria []string `json:"criteria"`
	Changes  []Change `json:"changes"`
}

type targetLocation struct {
	Kind        string
	Title       string
	Location    string
	ContentPath string
}

type targetAccumulator struct {
	location targetLocation
	action   string
	evidence map[string]bool
	changes  map[string]Change
}

type ownerSet map[string][]coverage.Assignment

// ownedRange is one reference resolved at the incoming comparison's base.
// Start and End are zero for a whole file.
type ownedRange struct {
	start, end int
	assignment coverage.Assignment
}

// ownership answers which references hold a line or file at one commit.
type ownership map[string][]ownedRange

func (owned ownership) line(path string, line int) ownerSet {
	owners := ownerSet{}
	for _, value := range owned[path] {
		if value.start == 0 || line >= value.start && line <= value.end {
			mergeAssignments(owners, []coverage.Assignment{value.assignment})
		}
	}
	return owners
}

func (owned ownership) file(path string) ownerSet {
	owners := ownerSet{}
	for _, value := range owned[path] {
		mergeAssignments(owners, []coverage.Assignment{value.assignment})
	}
	return owners
}

// Analyze projects incoming changed atoms onto the baseline Saga's references.
// Each reference is resolved at incoming.BaseOID, so evidence authored at any
// earlier commit applies wherever its lines now are.
func Analyze(ctx context.Context, document *saga.Saga, baseline gitdiff.ChangeSet, report coverage.Report, incoming gitdiff.ChangeSet, mode string, incomingSaga *saga.Saga, resolver coverage.Resolver) Result {
	return AnalyzeGraph(ctx, document, baseline, report, incoming, mode, incomingSaga, Graph{}, resolver)
}

// AnalyzeGraph is Analyze plus the review-repository projection: test-case
// evidence joins target ownership, and every affected owner is followed along
// recorded relations to the stories and criteria it serves.
func AnalyzeGraph(ctx context.Context, document *saga.Saga, baseline gitdiff.ChangeSet, report coverage.Report, incoming gitdiff.ChangeSet, mode string, incomingSaga *saga.Saga, graph Graph, resolver coverage.Resolver) Result {
	result := Result{
		Schema: Schema, Mode: mode, Basis: "code_references", ContentCompared: false,
		Baseline: BaselineStatus{
			SagaID: document.Manifest.ID, Title: document.Manifest.Title, Source: sourceIdentity(baseline),
			Complete: report.Complete, Covered: report.Summary.Covered, Total: report.Summary.Total,
			Uncovered: report.Summary.Uncovered, Stale: report.Summary.Stale, Overlapping: report.Summary.Overlapping,
		},
		Incoming: IncomingStatus{Source: sourceIdentity(incoming)},
		Targets:  []TargetImpact{}, NewContent: []UnownedChange{}, Diagnostics: []Diagnostic{},
		Requirements: []RequirementImpact{}, TestCases: []TestCaseImpact{},
	}
	if incomingSaga != nil {
		result.Incoming.SagaID = incomingSaga.Manifest.ID
		result.Incoming.Title = incomingSaga.Manifest.Title
	}
	if !report.Complete {
		result.Diagnostics = append(result.Diagnostics, Diagnostic{
			Code:    "baseline_incomplete",
			Message: "the maintained Saga does not completely account for its own comparison; impact results may omit affected content",
		})
	}

	locations := indexTargets(document)
	owned := ownership{}
	resolve := func(target, evidenceFile string, index int, reference coderef.Reference) {
		resolution := resolver.Resolve(ctx, reference, incoming.BaseOID)
		if !resolution.Current() {
			return
		}
		location := resolution.Location
		owned[location.Path] = append(owned[location.Path], ownedRange{start: location.Start, end: location.End, assignment: coverage.Assignment{Target: target, EvidenceFile: evidenceFile, Reference: index + 1}})
	}
	coverage.WalkDocumentCode(document, func(target string, files []saga.CodeFile) {
		for _, file := range files {
			for index, reference := range file.References {
				resolve(target, file.Path, index, reference)
			}
		}
	})
	for _, evidence := range graph.TestCases {
		locations[evidence.TestCase] = targetLocation{Kind: "test-case", Title: evidence.TestCase, Location: evidence.TestCase}
		for index, reference := range evidence.Code {
			resolve(evidence.TestCase, evidence.Evidence, index, reference)
		}
	}

	directOwners := map[string]ownerSet{}
	for _, atom := range incoming.Atoms {
		owners := ownerSet{}
		switch {
		case atom.Kind == "line" && atom.Side == "old":
			owners = owned.line(atom.Path, atom.Line)
		case atom.Kind == "event" && atom.Event != "add":
			path := atom.Path
			if atom.Event == "rename" && atom.OldPath != "" {
				path = atom.OldPath
			}
			owners = owned.file(path)
		}
		if len(owners) > 0 {
			directOwners[atom.Key] = owners
		}
	}

	incomingByKey := map[string]gitdiff.Atom{}
	linePosition := map[string]int{}
	for _, atom := range incoming.Atoms {
		incomingByKey[atom.Key] = atom
	}
	for index, line := range incoming.DisplayLines {
		if line.AtomKey != "" {
			linePosition[line.AtomKey] = index
		}
	}

	contextualOwners := map[string]ownerSet{}
	for _, atom := range incoming.Atoms {
		if atom.Kind != "line" || atom.Side != "new" {
			continue
		}
		owners := ownerSet{}
		if position, ok := linePosition[atom.Key]; ok {
			mergeOwnerSets(owners, replacementOwners(position, incoming.DisplayLines, incomingByKey, directOwners))
			if len(owners) == 0 {
				mergeOwnerSets(owners, adjacentOwners(position, incoming.DisplayLines, owned))
			}
		}
		if len(owners) > 0 {
			contextualOwners[atom.Key] = owners
		}
	}

	accumulators := map[string]*targetAccumulator{}
	directAtoms := map[string]bool{}
	contextualAtoms := map[string]bool{}
	ownedAtoms := map[string]bool{}
	for _, atom := range incoming.Atoms {
		if owners := directOwners[atom.Key]; len(owners) > 0 {
			directAtoms[atom.Key], ownedAtoms[atom.Key] = true, true
			addTargetChanges(accumulators, locations, owners, atom, "conflicting_intersection", "must_update")
		}
		if owners := contextualOwners[atom.Key]; len(owners) > 0 {
			contextualAtoms[atom.Key], ownedAtoms[atom.Key] = true, true
			addTargetChanges(accumulators, locations, owners, atom, "additive_near_owned_code", "consider_update")
		}
		if !ownedAtoms[atom.Key] {
			result.NewContent = append(result.NewContent, UnownedChange{Atom: atom, Reason: unownedReason(atom)})
		}
	}

	for target, accumulator := range accumulators {
		impact := TargetImpact{
			Target: target, Kind: accumulator.location.Kind, Title: accumulator.location.Title,
			Location: accumulator.location.Location, ContentPath: accumulator.location.ContentPath,
			Action: accumulator.action, EvidenceFiles: keys(accumulator.evidence),
		}
		for _, change := range accumulator.changes {
			impact.Changes = append(impact.Changes, change)
		}
		sortChanges(impact.Changes)
		result.Targets = append(result.Targets, impact)
	}
	sort.Slice(result.Targets, func(i, j int) bool {
		if result.Targets[i].Action != result.Targets[j].Action {
			return result.Targets[i].Action == "must_update"
		}
		return result.Targets[i].Target < result.Targets[j].Target
	})
	sort.Slice(result.NewContent, func(i, j int) bool { return atomLess(result.NewContent[i].Atom, result.NewContent[j].Atom) })

	result.Summary.IncomingAtoms = len(incoming.Atoms)
	result.Summary.DirectIntersections = len(directAtoms)
	result.Summary.ContextualAdditions = len(contextualAtoms)
	result.Summary.NewContentRequired = len(result.NewContent)
	for _, target := range result.Targets {
		if target.Action == "must_update" {
			result.Summary.TargetsMustUpdate++
		} else {
			result.Summary.TargetsConsiderUpdate++
		}
	}
	projectReviewRepository(&result, graph)
	result.Summary.RequirementsImpacted = len(result.Requirements)
	result.Summary.TestCasesImpacted = len(result.TestCases)
	return result
}

// projectReviewRepository follows every affected owner to the requirements and
// test cases the review repository records for it.
func projectReviewRepository(result *Result, graph Graph) {
	testEvidence := map[string][]TestCaseEvidence{}
	for _, evidence := range graph.TestCases {
		testEvidence[evidence.TestCase] = append(testEvidence[evidence.TestCase], evidence)
	}
	requirements := map[string]*RequirementImpact{}
	addRequirement := func(requirement, action string, route ImpactRoute) {
		value := requirements[requirement]
		if value == nil {
			value = &RequirementImpact{Requirement: requirement, Action: action, Via: []ImpactRoute{}}
			requirements[requirement] = value
		}
		if action == "must_update" {
			value.Action = action
		}
		value.Via = append(value.Via, route)
	}
	for _, target := range result.Targets {
		for _, reach := range graph.Requirements[target.Target] {
			addRequirement(reach.Requirement, target.Action, ImpactRoute{Target: target.Target, Relation: reach.Relation, Path: append([]string{}, reach.Path...)})
		}
		evidence := testEvidence[target.Target]
		if len(evidence) == 0 {
			continue
		}
		impact := TestCaseImpact{TestCase: target.Target, Action: target.Action, Evidence: []string{}, Roles: []string{}, Criteria: []string{}, Changes: target.Changes}
		touched := map[string]bool{}
		for _, file := range target.EvidenceFiles {
			touched[file] = true
		}
		for _, value := range evidence {
			if !touched[value.Evidence] {
				continue
			}
			impact.Evidence = append(impact.Evidence, value.Evidence)
			impact.Roles = append(impact.Roles, value.Role)
			impact.Criteria = append(impact.Criteria, value.Criteria...)
		}
		impact.Evidence, impact.Roles, impact.Criteria = sortedUnique(impact.Evidence), sortedUnique(impact.Roles), sortedUnique(impact.Criteria)
		for _, criterion := range impact.Criteria {
			addRequirement(criterion, target.Action, ImpactRoute{Target: target.Target, Path: []string{target.Target, criterion}})
		}
		result.TestCases = append(result.TestCases, impact)
	}
	for _, value := range requirements {
		sort.Slice(value.Via, func(i, j int) bool {
			if value.Via[i].Target != value.Via[j].Target {
				return value.Via[i].Target < value.Via[j].Target
			}
			return value.Via[i].Relation < value.Via[j].Relation
		})
		result.Requirements = append(result.Requirements, *value)
	}
	sort.Slice(result.Requirements, func(i, j int) bool {
		if result.Requirements[i].Action != result.Requirements[j].Action {
			return result.Requirements[i].Action == "must_update"
		}
		return result.Requirements[i].Requirement < result.Requirements[j].Requirement
	})
	sort.Slice(result.TestCases, func(i, j int) bool {
		if result.TestCases[i].Action != result.TestCases[j].Action {
			return result.TestCases[i].Action == "must_update"
		}
		return result.TestCases[i].TestCase < result.TestCases[j].TestCase
	})
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func sourceIdentity(changes gitdiff.ChangeSet) SourceIdentity {
	return SourceIdentity{Repository: changes.Repository, Base: changes.Base, Head: changes.Head, BaseOID: changes.BaseOID, HeadOID: changes.HeadOID}
}

func mergeAssignments(destination ownerSet, assignments []coverage.Assignment) {
	for _, assignment := range assignments {
		duplicate := false
		for _, existing := range destination[assignment.Target] {
			if existing.EvidenceFile == assignment.EvidenceFile && existing.Reference == assignment.Reference {
				duplicate = true
				break
			}
		}
		if !duplicate {
			destination[assignment.Target] = append(destination[assignment.Target], assignment)
		}
	}
}

func mergeOwnerSets(destination, source ownerSet) {
	for _, assignments := range source {
		mergeAssignments(destination, assignments)
	}
}

func replacementOwners(position int, lines []gitdiff.DisplayLine, atoms map[string]gitdiff.Atom, direct map[string]ownerSet) ownerSet {
	owners := ownerSet{}
	for index := position - 1; index >= 0; index-- {
		line := lines[index]
		if line.Kind == "context" || line.Kind == "event" || line.Path != lines[position].Path {
			break
		}
		if atom := atoms[line.AtomKey]; atom.Side == "old" {
			mergeOwnerSets(owners, direct[atom.Key])
		}
	}
	for index := position + 1; index < len(lines); index++ {
		line := lines[index]
		if line.Kind == "context" || line.Kind == "event" || line.Path != lines[position].Path {
			break
		}
		if atom := atoms[line.AtomKey]; atom.Side == "old" {
			mergeOwnerSets(owners, direct[atom.Key])
		}
	}
	return owners
}

// adjacentOwners returns the references holding the unchanged line nearest an
// addition on either side.
func adjacentOwners(position int, lines []gitdiff.DisplayLine, owned ownership) ownerSet {
	owners := ownerSet{}
	path := lines[position].Path
	for index := position - 1; index >= 0; index-- {
		line := lines[index]
		if line.Path != path || line.Kind == "event" {
			break
		}
		if line.Kind == "context" {
			mergeOwnerSets(owners, owned.line(path, line.OldLine))
			break
		}
	}
	for index := position + 1; index < len(lines); index++ {
		line := lines[index]
		if line.Path != path || line.Kind == "event" {
			break
		}
		if line.Kind == "context" {
			mergeOwnerSets(owners, owned.line(path, line.OldLine))
			break
		}
	}
	return owners
}

func addTargetChanges(accumulators map[string]*targetAccumulator, locations map[string]targetLocation, owners ownerSet, atom gitdiff.Atom, relationship, action string) {
	for target, assignments := range owners {
		accumulator := accumulators[target]
		if accumulator == nil {
			accumulator = &targetAccumulator{location: locations[target], action: action, evidence: map[string]bool{}, changes: map[string]Change{}}
			accumulators[target] = accumulator
		}
		if action == "must_update" {
			accumulator.action = action
		}
		for _, assignment := range assignments {
			if assignment.EvidenceFile != "" {
				accumulator.evidence[assignment.EvidenceFile] = true
			}
		}
		key := atom.Key
		if existing, ok := accumulator.changes[key]; !ok || existing.Relationship != "conflicting_intersection" {
			accumulator.changes[key] = Change{Relationship: relationship, Atom: atom}
		}
	}
}

func indexTargets(document *saga.Saga) map[string]targetLocation {
	result := map[string]targetLocation{
		document.Section.Target: {Kind: "saga", Title: document.Manifest.Title, Location: "."},
	}
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section != document.Section {
			result[section.Target] = targetLocation{Kind: section.Kind, Title: section.Title, Location: section.Path}
		}
		for _, fragment := range section.Fragments {
			contentPath := fragment.Path + "/" + fragment.Entrypoint
			result[fragment.Target] = targetLocation{Kind: "fragment", Title: first(fragment.Title, fragment.ID), Location: fragment.Path, ContentPath: contentPath}
			for index := range fragment.Landmarks {
				landmark := &fragment.Landmarks[index]
				result[landmark.Target] = targetLocation{Kind: "landmark", Title: landmark.Label, Location: landmark.Path, ContentPath: contentPath}
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	return result
}

func unownedReason(atom gitdiff.Atom) string {
	if atom.Kind == "event" && atom.Event == "add" {
		return "new file has no existing Saga owner"
	}
	if atom.Kind == "line" && atom.Side == "new" {
		return "addition is not adjacent to source evidence owned by the existing Saga"
	}
	return "changed source does not intersect evidence owned by the existing Saga"
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool { return atomLess(changes[i].Atom, changes[j].Atom) })
}

func atomLess(left, right gitdiff.Atom) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	if left.Side != right.Side {
		return left.Side < right.Side
	}
	return left.Ref < right.Ref
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
