package coverage

import (
	"sort"
	"time"
)

// Axis is one of the six coverage obligations a feature policy places on every
// accepted acceptance criterion. The axes are deliberately separate concerns:
// a prototype does not prove a UX flow, a UX flow does not prove a technical
// design, and none of them prove an implementation diff.
type Axis string

const (
	AxisPrototype      Axis = "prototype"
	AxisUX             Axis = "ux"
	AxisUI             Axis = "ui"
	AxisTechnical      Axis = "technical"
	AxisQuality        Axis = "quality"
	AxisImplementation Axis = "implementation"

	// AxisLegacyDesign is the pre-six-axis recorded value. Sagas written before
	// the split persisted one aggregate `design` exception, so readers must keep
	// accepting it. It expands forward to every design axis (ux, ui, technical)
	// because that is the only mapping that preserves the author's decision: the
	// aggregate exception previously excluded the criterion from all visual and
	// technical design coverage, and narrowing it to a single new axis would turn
	// a recorded exclusion into two silent gaps. Writers must never emit it.
	AxisLegacyDesign Axis = "design"
)

// AxisState classifies one criterion on one axis. The vocabulary is the design
// coverage state set from the lifecycle specification, reused unchanged for
// every axis so a reviewer reads one table instead of six.
type AxisState string

const (
	StateCoveredDirect AxisState = "covered_direct"
	StateCoveredBroad  AxisState = "covered_broad"
	StateExcluded      AxisState = "excluded"
	StateStale         AxisState = "stale"
	StateInvalid       AxisState = "invalid"
	StateConflicted    AxisState = "conflicted"
	StateGap           AxisState = "gap"
)

// Resolution collapses AxisState onto the only three outcomes the model has:
// a current link, an explicit exclusion, or a visible gap. Stale, invalid, and
// conflicted records resolve to a gap without losing their reasons, because a
// pin that no longer matches is not coverage and silently promoting it would
// be the percentage-shaped dishonesty the specification forbids.
type Resolution string

const (
	ResolutionLinked   Resolution = "linked"
	ResolutionExcluded Resolution = "excluded"
	ResolutionGap      Resolution = "gap"
)

// ExceptionState is the currency of one recorded exception record. Conflict is
// deliberately absent: competing heads are a property of a criterion/axis cell,
// not of a record, and a perfectly valid current exception can still land in a
// conflicted cell because another record competes for the same axis.
type ExceptionState string

const (
	ExceptionCurrent    ExceptionState = "current"
	ExceptionStale      ExceptionState = "stale"
	ExceptionSuperseded ExceptionState = "superseded"
	ExceptionInvalid    ExceptionState = "invalid"
)

var (
	canonicalAxes = []Axis{AxisPrototype, AxisUX, AxisUI, AxisTechnical, AxisQuality, AxisImplementation}
	designAxes    = []Axis{AxisUX, AxisUI, AxisTechnical}
)

// Axes returns the six canonical axes in their stable presentation order. The
// order matches the reviewer sidebar (Product, Design, Quality, Implementation)
// and never varies with authoring order.
func Axes() []Axis { return append([]Axis(nil), canonicalAxes...) }

// DesignAxes returns the axes that `design_ready` governs.
func DesignAxes() []Axis { return append([]Axis(nil), designAxes...) }

// Canonical reports whether the axis is one of the six; the legacy aggregate is
// deliberately not canonical.
func (axis Axis) Canonical() bool {
	for _, candidate := range canonicalAxes {
		if axis == candidate {
			return true
		}
	}
	return false
}

// RecordedAxis maps a persisted axis value onto the canonical axes it excludes.
// It is the single forward-compatibility seam for the legacy aggregate.
func RecordedAxis(value string) ([]Axis, bool) {
	axis := Axis(value)
	if axis == AxisLegacyDesign {
		return DesignAxes(), true
	}
	if axis.Canonical() {
		return []Axis{axis}, true
	}
	return nil, false
}

// AxisPolicy names the required axis set. The feature policy requires every
// axis; an axis becomes inapplicable only through an explicit exception, never
// through a missing link.
type AxisPolicy struct {
	Name     string `json:"name"`
	Required []Axis `json:"required"`
}

// FeatureAxisPolicy is the v5 feature-Saga policy.
func FeatureAxisPolicy() AxisPolicy { return AxisPolicy{Name: "feature", Required: Axes()} }

// Exception is one immutable coverage-exception record as loaded. Axis is the
// recorded value and may be the legacy aggregate; UnresolvedCitations carries
// the runtime citation-resolution result the schema cannot express.
type Exception struct {
	URN                 string    `json:"urn"`
	Axis                Axis      `json:"axis"`
	Criterion           string    `json:"criterion"`
	StoryRevision       string    `json:"story_revision"`
	Rationale           string    `json:"rationale"`
	Citations           []string  `json:"citations"`
	Supersedes          []string  `json:"supersedes"`
	CreatedAt           time.Time `json:"created_at"`
	UnresolvedCitations []string  `json:"unresolved_citations,omitempty"`
}

// ExceptionCoverage is the projected currency of one exception record.
// ConflictedAxes names the axes on which this record competes with another
// unsuperseded head; CompetingHeads names every competitor across them.
type ExceptionCoverage struct {
	Exception      Exception      `json:"exception"`
	Axes           []Axis         `json:"axes"`
	State          ExceptionState `json:"state"`
	Reasons        []string       `json:"reasons"`
	SupersededBy   []string       `json:"superseded_by"`
	ConflictedAxes []Axis         `json:"conflicted_axes"`
	CompetingHeads []string       `json:"competing_heads"`
}

// AxisLink is one persisted relation, containment path, or evidence chain that
// an author asserts covers a criterion on one axis. Every field is an already
// validated fact: nothing here is inferred from proximity, slide order, matching
// words, code paths, Git blame, or a model.
type AxisLink struct {
	Axis            Axis       `json:"axis"`
	Relation        string     `json:"relation,omitempty"`
	Source          string     `json:"source"`
	Broad           bool       `json:"broad"`
	PinnedRevision  string     `json:"pinned_revision,omitempty"`
	Paths           [][]string `json:"paths,omitempty"`
	Diffs           []string   `json:"diffs,omitempty"`
	StaleReasons    []string   `json:"stale_reasons,omitempty"`
	InvalidReasons  []string   `json:"invalid_reasons,omitempty"`
	ConflictReasons []string   `json:"conflict_reasons,omitempty"`
	// Unsatisfied names why a link whose pins are all current still does not
	// meet the axis, such as a test whose current run failed or an Item path that
	// owns no exact diff. It is neither stale, invalid, nor conflicted, so it
	// resolves the cell to a gap that keeps its reason.
	Unsatisfied []string `json:"unsatisfied_reasons,omitempty"`
}

// Current reports whether the link carries no stale, invalid, conflicting, or
// unsatisfied fact and therefore counts as coverage.
func (link AxisLink) Current() bool {
	return len(link.StaleReasons) == 0 && len(link.InvalidReasons) == 0 && len(link.ConflictReasons) == 0 && len(link.Unsatisfied) == 0
}

// LinkCoverage is one link plus its projected currency.
type LinkCoverage struct {
	Link    AxisLink `json:"link"`
	Current bool     `json:"current"`
}

// CriterionInput is one accepted, active criterion and everything persisted
// about it. RequiredAxes narrows the policy for this criterion only; an empty
// value means the policy's full required set.
type CriterionInput struct {
	URN                  string
	Story                string
	CurrentStoryRevision string
	RevisionHeads        []string
	RequiredAxes         []Axis
	Links                []AxisLink
	// Unsatisfied names axis-level obligations no single link can carry, such as
	// a required test kind no current passing test covers. While any reason is
	// present the axis cannot be covered; an explicit exclusion still applies.
	Unsatisfied map[Axis][]string
}

// ExclusionRow is one criterion/axis exclusion. Exclusions are reported as a
// separate row and a separate count: they can satisfy a gate, but they never
// increment covered and never disappear into a denominator.
type ExclusionRow struct {
	Criterion string `json:"criterion"`
	Axis      Axis   `json:"axis"`
	Exception string `json:"exception"`
	Rationale string `json:"rationale"`
}

// AxisGap is a visible, addressable hole. It always names the axis and why the
// cell is not covered, so a reviewer never has to read a number to find work.
type AxisGap struct {
	Criterion string   `json:"criterion"`
	Axis      Axis     `json:"axis"`
	Reasons   []string `json:"reasons"`
}

// AxisCoverage is one criterion/axis cell. Exactly one of a current link, an
// explicit exclusion, or a visible gap holds, and Resolution says which.
type AxisCoverage struct {
	Criterion  string             `json:"criterion"`
	Axis       Axis               `json:"axis"`
	State      AxisState          `json:"state"`
	Resolution Resolution         `json:"resolution"`
	Precision  string             `json:"precision"`
	Links      []LinkCoverage     `json:"links"`
	Exclusion  *ExceptionCoverage `json:"exclusion,omitempty"`
	StalePins  []string           `json:"stale_pins"`
	Conflicts  []string           `json:"conflicts"`
	Invalid    []string           `json:"invalid"`
	// Unsatisfied lists recorded, current facts that still fall short of the
	// axis. They are reported beside stale pins, never folded into them.
	Unsatisfied []string `json:"unsatisfied"`
	Gap         *AxisGap `json:"gap,omitempty"`
}

// CriterionCoverage is one criterion across every required axis, in canonical
// axis order.
type CriterionCoverage struct {
	Criterion string         `json:"criterion"`
	Story     string         `json:"story"`
	Axes      []AxisCoverage `json:"axes"`
}

// Axis returns the cell for one axis.
func (criterion CriterionCoverage) Axis(axis Axis) (AxisCoverage, bool) {
	for _, cell := range criterion.Axes {
		if cell.Axis == axis {
			return cell, true
		}
	}
	return AxisCoverage{}, false
}

// StateCounts is the per-axis roll-up. It is deliberately a set of named
// counts: there is no total, ratio, score, or percentage field, because a
// single number cannot distinguish an intentional exclusion from a hole.
type StateCounts struct {
	CoveredDirect int `json:"covered_direct"`
	CoveredBroad  int `json:"covered_broad"`
	Excluded      int `json:"excluded"`
	Stale         int `json:"stale"`
	Invalid       int `json:"invalid"`
	Conflicted    int `json:"conflicted"`
	Gap           int `json:"gap"`
}

// AxisSummary pairs one axis with its counts.
type AxisSummary struct {
	Axis   Axis        `json:"axis"`
	Counts StateCounts `json:"counts"`
}

// AxisProjection is the whole six-axis model for one Saga snapshot.
type AxisProjection struct {
	Policy     AxisPolicy          `json:"policy"`
	Criteria   []CriterionCoverage `json:"criteria"`
	Exceptions []ExceptionCoverage `json:"exceptions"`
	Axes       []AxisSummary       `json:"axes"`
	Gaps       []AxisGap           `json:"gaps"`
}

// Criterion returns one criterion's cells.
func (projection AxisProjection) Criterion(urn string) (CriterionCoverage, bool) {
	for _, criterion := range projection.Criteria {
		if criterion.Criterion == urn {
			return criterion, true
		}
	}
	return CriterionCoverage{}, false
}

// Summary returns one axis roll-up.
func (projection AxisProjection) Summary(axis Axis) (StateCounts, bool) {
	for _, summary := range projection.Axes {
		if summary.Axis == axis {
			return summary.Counts, true
		}
	}
	return StateCounts{}, false
}

// ProjectAxes classifies every criterion on every required axis from persisted
// links and exception records.
//
// The state precedence is deterministic and documented so two readers agree:
//
//  1. conflicted  - the criterion has multiple revision heads, competing
//     unsuperseded exception heads, or a link that reports a conflict;
//  2. invalid     - a link or exception record for the axis is invalid;
//  3. covered_direct - a current link targets the criterion itself;
//  4. covered_broad  - every current link reaches the criterion through its
//     story, which is retained but visibly less precise;
//  5. excluded    - a current unsuperseded exception head covers the axis;
//  6. stale       - links or an exception exist but none are current; and
//  7. gap         - nothing is recorded.
//
// Severity precedes evidence so a broken record can never be hidden by a good
// one, and coverage precedes exclusion so a real link is never reported as an
// exclusion. Nothing here is a percentage, and no state is ever inferred from
// the absence of another.
func ProjectAxes(criteria []CriterionInput, exceptions []Exception, policy AxisPolicy) AxisProjection {
	if len(policy.Required) == 0 {
		policy = FeatureAxisPolicy()
	}
	projection := AxisProjection{
		Policy:     AxisPolicy{Name: policy.Name, Required: orderedAxes(policy.Required)},
		Criteria:   []CriterionCoverage{},
		Exceptions: []ExceptionCoverage{},
		Axes:       []AxisSummary{},
		Gaps:       []AxisGap{},
	}
	known := map[string]CriterionInput{}
	for _, criterion := range criteria {
		known[criterion.URN] = criterion
	}
	projected, heads := projectExceptions(exceptions, known)
	projection.Exceptions = projected

	counts := map[Axis]*StateCounts{}
	for _, axis := range projection.Policy.Required {
		counts[axis] = &StateCounts{}
	}
	ordered := append([]CriterionInput(nil), criteria...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].URN < ordered[j].URN })
	for _, input := range ordered {
		required := orderedAxes(input.RequiredAxes)
		if len(required) == 0 {
			required = projection.Policy.Required
		}
		row := CriterionCoverage{Criterion: input.URN, Story: input.Story, Axes: []AxisCoverage{}}
		for _, axis := range required {
			cell := classify(input, axis, heads[exceptionKey{criterion: input.URN, axis: axis}])
			row.Axes = append(row.Axes, cell)
			if cell.Gap != nil {
				projection.Gaps = append(projection.Gaps, *cell.Gap)
			}
			if counts[axis] == nil {
				counts[axis] = &StateCounts{}
			}
			countState(counts[axis], cell.State)
		}
		projection.Criteria = append(projection.Criteria, row)
	}
	for _, axis := range orderedAxes(mapAxes(counts)) {
		projection.Axes = append(projection.Axes, AxisSummary{Axis: axis, Counts: *counts[axis]})
	}
	sort.SliceStable(projection.Gaps, func(i, j int) bool {
		if projection.Gaps[i].Criterion != projection.Gaps[j].Criterion {
			return projection.Gaps[i].Criterion < projection.Gaps[j].Criterion
		}
		return axisRank(projection.Gaps[i].Axis) < axisRank(projection.Gaps[j].Axis)
	})
	return projection
}

type exceptionKey struct {
	criterion string
	axis      Axis
}

// exceptionHead is the unsuperseded decision attached to one criterion/axis
// cell plus every record competing for that same cell.
type exceptionHead struct {
	coverage  *ExceptionCoverage
	competing []string
}

// projectExceptions resolves currency, supersession, and the one-head-per
// criterion/axis rule. Competing heads are returned as a conflict with every
// competitor named; they are never resolved by timestamp.
func projectExceptions(exceptions []Exception, criteria map[string]CriterionInput) ([]ExceptionCoverage, map[exceptionKey]exceptionHead) {
	ordered := append([]Exception(nil), exceptions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].URN < ordered[j].URN })
	supersededBy := map[string][]string{}
	for _, exception := range ordered {
		for _, target := range exception.Supersedes {
			supersededBy[target] = append(supersededBy[target], exception.URN)
		}
	}
	projected := make([]ExceptionCoverage, 0, len(ordered))
	candidates := map[exceptionKey][]int{}
	rejected := map[exceptionKey][]int{}
	for _, exception := range ordered {
		item := ExceptionCoverage{
			Exception: exception, Axes: []Axis{}, State: ExceptionCurrent, Reasons: []string{},
			SupersededBy: uniqueSorted(supersededBy[exception.URN]), ConflictedAxes: []Axis{}, CompetingHeads: []string{},
		}
		axes, ok := RecordedAxis(string(exception.Axis))
		if ok {
			item.Axes = axes
		} else {
			item.State = ExceptionInvalid
			item.Reasons = append(item.Reasons, "axis is not one of the six coverage axes")
		}
		if exception.Rationale == "" {
			item.State = ExceptionInvalid
			item.Reasons = append(item.Reasons, "rationale is blank")
		}
		if len(exception.Citations) == 0 {
			item.State = ExceptionInvalid
			item.Reasons = append(item.Reasons, "at least one citation is required")
		}
		for _, citation := range uniqueSorted(exception.UnresolvedCitations) {
			item.State = ExceptionInvalid
			item.Reasons = append(item.Reasons, "citation does not resolve: "+citation)
		}
		criterion, resolved := criteria[exception.Criterion]
		if !resolved {
			item.State = ExceptionInvalid
			item.Reasons = append(item.Reasons, "criterion endpoint is unresolved")
		}
		if item.State != ExceptionInvalid {
			switch {
			case len(item.SupersededBy) > 0:
				item.State = ExceptionSuperseded
			case len(criterion.RevisionHeads) > 1:
				item.State = ExceptionStale
				item.Reasons = append(item.Reasons, "story has multiple revision heads")
			case exception.StoryRevision == "" || criterion.CurrentStoryRevision == "":
				item.State = ExceptionStale
				item.Reasons = append(item.Reasons, "story revision pin is unavailable")
			case exception.StoryRevision != criterion.CurrentStoryRevision:
				item.State = ExceptionStale
				item.Reasons = append(item.Reasons, "story revision changed to "+criterion.CurrentStoryRevision)
			}
		}
		index := len(projected)
		projected = append(projected, item)
		// A superseded record is explicitly replaced and competes with nothing. An
		// invalid record is not a usable head either, but it is still attached to
		// its cell below so the author sees why their decision did not take.
		if item.State == ExceptionSuperseded {
			continue
		}
		for _, axis := range item.Axes {
			key := exceptionKey{criterion: exception.Criterion, axis: axis}
			if item.State == ExceptionInvalid {
				rejected[key] = append(rejected[key], index)
				continue
			}
			candidates[key] = append(candidates[key], index)
		}
	}
	heads := map[exceptionKey]exceptionHead{}
	for key, indexes := range candidates {
		// The head is attached whatever its currency: a reviewer must see a stale
		// decision, not an empty exclusion column.
		head := exceptionHead{coverage: &projected[indexes[0]], competing: []string{}}
		if len(indexes) > 1 {
			for _, index := range indexes {
				head.competing = append(head.competing, projected[index].Exception.URN)
			}
			sort.Strings(head.competing)
			for _, index := range indexes {
				projected[index].ConflictedAxes = orderedAxes(append(projected[index].ConflictedAxes, key.axis))
				projected[index].CompetingHeads = uniqueSorted(append(projected[index].CompetingHeads, head.competing...))
			}
		}
		heads[key] = head
	}
	for key, indexes := range rejected {
		if _, ok := heads[key]; !ok {
			heads[key] = exceptionHead{coverage: &projected[indexes[0]], competing: []string{}}
		}
	}
	return projected, heads
}

func classify(input CriterionInput, axis Axis, head exceptionHead) AxisCoverage {
	cell := AxisCoverage{
		Criterion: input.URN, Axis: axis, Precision: "none",
		Links: []LinkCoverage{}, StalePins: []string{}, Conflicts: []string{}, Invalid: []string{}, Unsatisfied: []string{},
	}
	if len(input.RevisionHeads) > 1 {
		cell.Conflicts = append(cell.Conflicts, "criterion story has multiple revision heads: "+joinSorted(input.RevisionHeads))
	}
	direct, broad, stalePresent := false, false, false
	for _, link := range input.Links {
		if link.Axis != axis {
			continue
		}
		current := link.Current()
		cell.Links = append(cell.Links, LinkCoverage{Link: link, Current: current})
		for _, reason := range link.StaleReasons {
			cell.StalePins = append(cell.StalePins, describe(link, reason))
			stalePresent = true
		}
		for _, reason := range link.InvalidReasons {
			cell.Invalid = append(cell.Invalid, describe(link, reason))
		}
		for _, reason := range link.ConflictReasons {
			cell.Conflicts = append(cell.Conflicts, describe(link, reason))
		}
		// Unsatisfied facts matter only while nothing else is wrong with the link;
		// a stale or invalid link already explains itself.
		if len(link.StaleReasons) == 0 && len(link.InvalidReasons) == 0 && len(link.ConflictReasons) == 0 {
			for _, reason := range link.Unsatisfied {
				cell.Unsatisfied = append(cell.Unsatisfied, describe(link, reason))
			}
		}
		if current && link.Broad {
			broad = true
		}
		if current && !link.Broad {
			direct = true
		}
	}
	sort.SliceStable(cell.Links, func(i, j int) bool {
		left, right := cell.Links[i].Link, cell.Links[j].Link
		if left.Relation != right.Relation {
			return left.Relation < right.Relation
		}
		return left.Source < right.Source
	})
	if blockers := input.Unsatisfied[axis]; len(blockers) > 0 {
		cell.Unsatisfied = append(cell.Unsatisfied, blockers...)
		direct, broad = false, false
	}
	excluded := false
	if head.coverage != nil {
		copied := *head.coverage
		cell.Exclusion = &copied
		if len(head.competing) > 1 {
			cell.Conflicts = append(cell.Conflicts, "competing exception heads: "+joinSorted(head.competing))
		}
		switch copied.State {
		case ExceptionInvalid:
			cell.Invalid = append(cell.Invalid, "exception "+copied.Exception.URN+" is invalid: "+joinSorted(copied.Reasons))
		case ExceptionStale:
			cell.StalePins = append(cell.StalePins, "exception "+copied.Exception.URN+" is stale: "+joinSorted(copied.Reasons))
			stalePresent = true
		case ExceptionCurrent:
			excluded = len(head.competing) <= 1
		}
	}
	switch {
	case len(cell.Conflicts) > 0:
		cell.State = StateConflicted
	case len(cell.Invalid) > 0:
		cell.State = StateInvalid
	case direct:
		cell.State, cell.Precision = StateCoveredDirect, "direct"
	case broad:
		cell.State, cell.Precision = StateCoveredBroad, "broad"
	case excluded:
		cell.State = StateExcluded
	case stalePresent:
		cell.State = StateStale
	default:
		cell.State = StateGap
	}
	cell.Resolution = cell.State.Resolution()
	if cell.Resolution == ResolutionGap {
		cell.Gap = &AxisGap{Criterion: input.URN, Axis: axis, Reasons: gapReasons(cell)}
	}
	return cell
}

// Resolution maps a state onto the three-state model.
func (state AxisState) Resolution() Resolution {
	switch state {
	case StateCoveredDirect, StateCoveredBroad:
		return ResolutionLinked
	case StateExcluded:
		return ResolutionExcluded
	default:
		return ResolutionGap
	}
}

func gapReasons(cell AxisCoverage) []string {
	reasons := []string{}
	reasons = append(reasons, cell.Conflicts...)
	reasons = append(reasons, cell.Invalid...)
	reasons = append(reasons, cell.StalePins...)
	reasons = append(reasons, cell.Unsatisfied...)
	if len(reasons) == 0 {
		reasons = append(reasons, "no current "+string(cell.Axis)+" link and no current exception")
	}
	return reasons
}

func describe(link AxisLink, reason string) string {
	source := link.Relation
	if source == "" {
		source = link.Source
	}
	if source == "" {
		return reason
	}
	return source + ": " + reason
}

func countState(counts *StateCounts, state AxisState) {
	switch state {
	case StateCoveredDirect:
		counts.CoveredDirect++
	case StateCoveredBroad:
		counts.CoveredBroad++
	case StateExcluded:
		counts.Excluded++
	case StateStale:
		counts.Stale++
	case StateInvalid:
		counts.Invalid++
	case StateConflicted:
		counts.Conflicted++
	default:
		counts.Gap++
	}
}

func axisRank(axis Axis) int {
	for index, candidate := range canonicalAxes {
		if axis == candidate {
			return index
		}
	}
	return len(canonicalAxes)
}

func orderedAxes(values []Axis) []Axis {
	seen := map[Axis]bool{}
	result := make([]Axis, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return axisRank(result[i]) < axisRank(result[j]) })
	return result
}

func mapAxes(counts map[Axis]*StateCounts) []Axis {
	result := make([]Axis, 0, len(counts))
	for axis := range counts {
		result = append(result, axis)
	}
	return result
}

func joinSorted(values []string) string {
	ordered := uniqueSorted(values)
	result := ""
	for index, value := range ordered {
		if index > 0 {
			result += ", "
		}
		result += value
	}
	return result
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
