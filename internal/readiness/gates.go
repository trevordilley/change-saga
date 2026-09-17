package readiness

import (
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coverage"
)

// GateName is one readiness gate from the lifecycle gate table. The gates are
// independent projections, evaluated in this order; ready_for_review depends on
// the configured gates before it.
type GateName string

const (
	GateRequirementsReady        GateName = "requirements_ready"
	GateProductReady             GateName = "product_ready"
	GateDesignReady              GateName = "design_ready"
	GateImplementationTraceReady GateName = "implementation_trace_ready"
	GateQualityReady             GateName = "quality_ready"
	GateReadyForReview           GateName = "ready_for_review"
	GateReviewComplete           GateName = "review_complete"
)

// GateStatus is the reported outcome. It is deliberately a small enumeration
// and never a number: readiness is not a score, a percentage, or a progress bar.
type GateStatus string

const (
	StatusReady         GateStatus = "ready"
	StatusBlocked       GateStatus = "blocked"
	StatusNotApplicable GateStatus = "not_applicable"
)

const (
	// PolicyFeature is the v5 feature-Saga policy: every gate is configured and
	// quality is required before review.
	PolicyFeature = "feature"
	// PolicyCompatibility keeps the existing peer_review_ready behavior for a
	// v2/v3 Saga, or a v5 Saga whose quality capability is not adopted, unless
	// the caller explicitly selects the feature policy. The new gates are still
	// computed and reported as guidance; they simply do not block.
	PolicyCompatibility = "compatibility"
)

// Fact is one concrete, checkable observation a gate required. A gate reports
// facts, relations, paths, stale pins, exclusions, and gaps; it never reduces
// them to one opaque number.
type Fact struct {
	Code      string        `json:"code"`
	Satisfied bool          `json:"satisfied"`
	Resource  string        `json:"resource,omitempty"`
	Axis      coverage.Axis `json:"axis,omitempty"`
	Detail    string        `json:"detail,omitempty"`
	Paths     [][]string    `json:"paths,omitempty"`
}

// Gate is one row of the gate table with everything a reviewer needs to act.
type Gate struct {
	Name        GateName                `json:"name"`
	Status      GateStatus              `json:"status"`
	Satisfied   bool                    `json:"satisfied"`
	Configured  bool                    `json:"configured"`
	Facts       []Fact                  `json:"facts"`
	Blockers    []Blocker               `json:"blockers"`
	Axes        []coverage.AxisSummary  `json:"axes"`
	Exclusions  []coverage.ExclusionRow `json:"exclusions"`
	Gaps        []coverage.AxisGap      `json:"gaps"`
	StalePins   []string                `json:"stale_pins"`
	NotInferred []string                `json:"not_inferred"`
}

// GateProjection is the whole gate table for one snapshot. It has no aggregate
// readiness field on purpose: a caller that wants one number has to decide
// which facts it is willing to ignore, and that decision is not ours to hide.
type GateProjection struct {
	Policy     string      `json:"policy"`
	Gates      []Gate      `json:"gates"`
	PeerReview *Projection `json:"peer_review,omitempty"`
}

// Gate returns one gate by name.
func (projection GateProjection) Gate(name GateName) (Gate, bool) {
	for _, gate := range projection.Gates {
		if gate.Name == name {
			return gate, true
		}
	}
	return Gate{}, false
}

// Story is the requirement-identity input for requirements_ready.
type Story struct {
	URN             string
	State           string
	RevisionHeads   []string
	LifecycleHeads  []string
	CurrentRevision string
	Criteria        []string
	IdentityIssues  []string
}

// Accepted reports the recorded lifecycle head state only. Stakeholder
// agreement beyond that record is never inferred.
func (story Story) Accepted() bool { return story.State == "accepted" }

// Prototype is the product-discovery input for product_ready. A prototype may
// be temporarily unlinked during exploration; it simply cannot contribute to
// readiness until a current annotation connects it to a story or criterion.
type Prototype struct {
	URN          string
	Retained     bool
	CurrentLinks []string
	StaleReasons []string
}

// QualityDomain reports the adoption state of the quality capability. An absent
// root is not_adopted, an existing root with no test cases is adopted_empty,
// and neither is quality-ready.
type QualityDomain struct{ Adoption string }

// QualityFact is one required test kind for one criterion with its current run
// and evidence. A past pass, a claim, or an authored progress state is never
// substituted for a current passing run.
type QualityFact struct {
	Criterion        string
	Kind             string
	Required         bool
	TestCase         string
	RunResult        string
	RunHeads         []string
	EvidenceResolved bool
	StaleReasons     []string
}

// ChangedSourceAccounting is the global omission invariant, kept separate from
// every per-criterion axis on purpose. There is no exception from exact
// changed-source accounting: a coverage exception can declare an axis
// inapplicable, but documentation-only work still ends at its documentation
// diff, so no exception is consulted here.
type ChangedSourceAccounting struct {
	Complete  bool
	Uncovered []string
	Orphans   []string
}

// ReviewDecision is one required review target under the review policy.
type ReviewDecision struct {
	Target       string
	Kind         string
	Required     bool
	Decided      bool
	Current      bool
	StaleReasons []string
}

// GateInputs is the already validated graph projection the gates read. The
// package performs no filesystem or transport access; every field here is a
// fact a loader established.
type GateInputs struct {
	Policy        string
	Stories       []Story
	Prototypes    []Prototype
	Coverage      coverage.AxisProjection
	Quality       QualityDomain
	QualityFacts  []QualityFact
	ChangedSource ChangedSourceAccounting
	Reviews       []ReviewDecision
	PeerReview    []Criterion
}

// notInferred is the gate table's "Not inferred" column, kept next to the gate
// logic so a reader sees the limits at the same time as the checks.
var notInferred = map[GateName][]string{
	GateRequirementsReady:        {"stakeholder agreement beyond the recorded lifecycle"},
	GateProductReady:             {"that prototype behavior is desirable or feasible"},
	GateDesignReady:              {"design quality or correctness"},
	GateImplementationTraceReady: {"that the selected diff implements the criterion correctly"},
	GateQualityReady:             {"test sufficiency or the absence of undiscovered defects"},
	GateReadyForReview:           {"reviewer approval"},
	GateReviewComplete:           {"merge authorization outside Change Saga"},
}

// EvaluateGates computes the gate table. Every gate returns the concrete
// relations, paths, stale pins, exclusions, and gaps behind its verdict, and no
// gate is ever summarized as a percentage.
func EvaluateGates(inputs GateInputs) GateProjection {
	policy := inputs.Policy
	if policy == "" {
		policy = PolicyCompatibility
	}
	feature := policy == PolicyFeature
	projection := GateProjection{Policy: policy, Gates: []Gate{}}
	if !feature {
		legacy := Evaluate(inputs.PeerReview)
		projection.PeerReview = &legacy
	}

	gates := []Gate{
		requirementsGate(inputs, feature),
		productGate(inputs, feature),
		designGate(inputs, feature),
		implementationTraceGate(inputs, feature),
		qualityGate(inputs, feature),
	}
	gates = append(gates, reviewGate(inputs, gates, feature, projection.PeerReview))
	gates = append(gates, reviewCompleteGate(inputs, gates[len(gates)-1]))
	for index := range gates {
		finishGate(&gates[index])
	}
	projection.Gates = gates
	return projection
}

func requirementsGate(inputs GateInputs, feature bool) Gate {
	gate := newGate(GateRequirementsReady, feature)
	accepted := 0
	stories := append([]Story(nil), inputs.Stories...)
	sort.Slice(stories, func(i, j int) bool { return stories[i].URN < stories[j].URN })
	for _, story := range stories {
		gate.fact(Fact{
			Code: "story_revision_head", Resource: story.URN,
			Satisfied: len(story.RevisionHeads) == 1,
			Detail:    headDetail("revision", story.RevisionHeads),
		})
		gate.fact(Fact{
			Code: "story_lifecycle_head", Resource: story.URN,
			Satisfied: len(story.LifecycleHeads) == 1,
			Detail:    headDetail("lifecycle", story.LifecycleHeads),
		})
		gate.fact(Fact{
			Code: "identity_graph_valid", Resource: story.URN,
			Satisfied: len(story.IdentityIssues) == 0,
			Detail:    strings.Join(story.IdentityIssues, "; "),
		})
		if story.Accepted() {
			accepted++
			gate.fact(Fact{
				Code: "accepted_story_has_criteria", Resource: story.URN,
				Satisfied: len(story.Criteria) > 0,
				Detail:    countDetail(len(story.Criteria), "active criterion", "active criteria"),
			})
		}
	}
	gate.fact(Fact{
		Code: "accepted_story_present", Satisfied: accepted > 0,
		Detail: countDetail(accepted, "accepted story", "accepted stories"),
	})
	return gate
}

func productGate(inputs GateInputs, feature bool) Gate {
	gate := newGate(GateProductReady, feature)
	prototypes := append([]Prototype(nil), inputs.Prototypes...)
	sort.Slice(prototypes, func(i, j int) bool { return prototypes[i].URN < prototypes[j].URN })
	for _, prototype := range prototypes {
		if !prototype.Retained {
			continue
		}
		linked := len(prototype.CurrentLinks) > 0 && len(prototype.StaleReasons) == 0
		detail := countDetail(len(prototype.CurrentLinks), "current story or criterion link", "current story or criterion links")
		if len(prototype.StaleReasons) > 0 {
			detail = strings.Join(uniqueSorted(prototype.StaleReasons), "; ")
			gate.StalePins = append(gate.StalePins, prototype.URN+": "+detail)
		}
		gate.fact(Fact{Code: "retained_prototype_linked", Resource: prototype.URN, Satisfied: linked, Detail: detail})
	}
	gate.axis(inputs.Coverage, coverage.AxisPrototype)
	return gate
}

func designGate(inputs GateInputs, feature bool) Gate {
	gate := newGate(GateDesignReady, feature)
	for _, axis := range coverage.DesignAxes() {
		gate.axis(inputs.Coverage, axis)
	}
	return gate
}

func implementationTraceGate(inputs GateInputs, feature bool) Gate {
	gate := newGate(GateImplementationTraceReady, feature)
	gate.axis(inputs.Coverage, coverage.AxisImplementation)
	for _, criterion := range inputs.Coverage.Criteria {
		cell, ok := criterion.Axis(coverage.AxisImplementation)
		if !ok || cell.Resolution != coverage.ResolutionLinked {
			continue
		}
		paths, diffs := currentPaths(cell)
		gate.fact(Fact{
			Code: "implementation_path_ends_at_diff", Resource: criterion.Criterion,
			Axis: coverage.AxisImplementation, Satisfied: diffs > 0, Paths: paths,
			Detail: countDetail(diffs, "exact diff", "exact diffs"),
		})
	}
	// The global omission invariant is a separate required fact. An exception can
	// declare an axis inapplicable; it can never excuse an unaccounted changed
	// atom or an orphaned diff reference.
	gate.fact(Fact{
		Code:      "changed_source_accounting_complete",
		Satisfied: inputs.ChangedSource.Complete && len(inputs.ChangedSource.Uncovered) == 0 && len(inputs.ChangedSource.Orphans) == 0,
		Detail: countDetail(len(inputs.ChangedSource.Uncovered), "unaccounted changed atom", "unaccounted changed atoms") +
			", " + countDetail(len(inputs.ChangedSource.Orphans), "orphaned diff reference", "orphaned diff references"),
	})
	return gate
}

func qualityGate(inputs GateInputs, feature bool) Gate {
	gate := newGate(GateQualityReady, feature)
	gate.fact(Fact{
		Code: "quality_adopted", Satisfied: inputs.Quality.Adoption == "adopted",
		Detail: "quality capability is " + orUnknown(inputs.Quality.Adoption),
	})
	gate.axis(inputs.Coverage, coverage.AxisQuality)
	facts := append([]QualityFact(nil), inputs.QualityFacts...)
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Criterion != facts[j].Criterion {
			return facts[i].Criterion < facts[j].Criterion
		}
		return facts[i].Kind < facts[j].Kind
	})
	for _, fact := range facts {
		if !fact.Required {
			gate.fact(Fact{
				Code: "observed_kind", Resource: fact.Criterion, Axis: coverage.AxisQuality, Satisfied: true,
				Detail: fact.Kind + " observed on " + orUnknown(fact.TestCase) + " but not required by policy",
			})
			continue
		}
		satisfied := fact.RunResult == "passed" && len(fact.RunHeads) <= 1 && fact.EvidenceResolved && len(fact.StaleReasons) == 0
		detail := fact.Kind + ": run " + orUnknown(fact.RunResult)
		if len(fact.RunHeads) > 1 {
			detail += "; multiple current run heads: " + strings.Join(uniqueSorted(fact.RunHeads), ", ")
		}
		if !fact.EvidenceResolved {
			detail += "; evidence does not resolve on the current source comparison"
		}
		if len(fact.StaleReasons) > 0 {
			stale := strings.Join(uniqueSorted(fact.StaleReasons), "; ")
			detail += "; " + stale
			gate.StalePins = append(gate.StalePins, fact.Criterion+" "+fact.Kind+": "+stale)
		}
		gate.fact(Fact{Code: "required_kind_passing_run", Resource: fact.Criterion, Axis: coverage.AxisQuality, Satisfied: satisfied, Detail: detail})
	}
	return gate
}

func reviewGate(inputs GateInputs, preceding []Gate, feature bool, peerReview *Projection) Gate {
	gate := newGate(GateReadyForReview, true)
	if feature {
		for _, earlier := range preceding {
			satisfied, _ := gateVerdict(earlier)
			gate.fact(Fact{
				Code: "preceding_gate_satisfied", Resource: string(earlier.Name),
				Satisfied: !earlier.Configured || satisfied,
				Detail:    gateDetail(earlier, satisfied),
			})
			gate.Gaps = append(gate.Gaps, earlier.Gaps...)
			gate.StalePins = append(gate.StalePins, earlier.StalePins...)
		}
	} else if peerReview != nil {
		gate.fact(Fact{
			Code: "peer_review_ready", Satisfied: peerReview.PeerReviewReady,
			Detail: countDetail(peerReview.DeliveryCoverage.Missing, "criterion without a complete delivery path", "criteria without a complete delivery path"),
		})
	}

	conflicts := conflictResources(inputs)
	gate.fact(Fact{
		Code: "no_graph_conflicts", Satisfied: len(conflicts) == 0,
		Detail: strings.Join(conflicts, "; "),
	})

	orphans := len(inputs.ChangedSource.Orphans)
	unresolved := []string{}
	failed := []string{}
	for _, fact := range inputs.QualityFacts {
		if !fact.Required {
			continue
		}
		if !fact.EvidenceResolved {
			unresolved = append(unresolved, fact.Criterion+" "+fact.Kind)
		}
		if fact.RunResult == "failed" {
			failed = append(failed, fact.Criterion+" "+fact.Kind+" on "+orUnknown(fact.TestCase))
		}
	}
	gate.fact(Fact{
		Code: "immutable_current_evidence", Satisfied: orphans == 0 && len(unresolved) == 0,
		Detail: countDetail(orphans, "orphaned diff reference", "orphaned diff references") +
			", unresolved quality evidence: " + orNone(uniqueSorted(unresolved)),
	})
	gate.fact(Fact{
		Code: "no_failed_required_run", Satisfied: len(failed) == 0,
		Detail: orNone(uniqueSorted(failed)),
	})
	return gate
}

func reviewCompleteGate(inputs GateInputs, review Gate) Gate {
	gate := newGate(GateReviewComplete, true)
	satisfied, _ := gateVerdict(review)
	gate.fact(Fact{Code: "ready_for_review", Resource: string(GateReadyForReview), Satisfied: satisfied, Detail: gateDetail(review, satisfied)})
	decisions := append([]ReviewDecision(nil), inputs.Reviews...)
	sort.Slice(decisions, func(i, j int) bool {
		if decisions[i].Kind != decisions[j].Kind {
			return decisions[i].Kind < decisions[j].Kind
		}
		return decisions[i].Target < decisions[j].Target
	})
	for _, decision := range decisions {
		if !decision.Required {
			continue
		}
		detail := decision.Kind + " decision"
		if !decision.Decided {
			detail += " is missing"
		} else if !decision.Current {
			detail += " is not current"
		}
		if len(decision.StaleReasons) > 0 {
			stale := strings.Join(uniqueSorted(decision.StaleReasons), "; ")
			detail += "; " + stale
			gate.StalePins = append(gate.StalePins, decision.Target+": "+stale)
		}
		gate.fact(Fact{
			Code: "required_review_decision", Resource: decision.Target,
			Satisfied: decision.Decided && decision.Current && len(decision.StaleReasons) == 0,
			Detail:    detail,
		})
	}
	return gate
}

// conflictResources names every multi-head or conflicted record ready_for_review
// must not step over. Ambiguity is returned with every competing head and is
// never resolved by timestamp.
func conflictResources(inputs GateInputs) []string {
	result := []string{}
	for _, story := range inputs.Stories {
		if len(story.RevisionHeads) > 1 {
			result = append(result, story.URN+": multiple revision heads")
		}
		if len(story.LifecycleHeads) > 1 {
			result = append(result, story.URN+": multiple lifecycle heads")
		}
	}
	for _, criterion := range inputs.Coverage.Criteria {
		for _, cell := range criterion.Axes {
			if cell.State == coverage.StateConflicted {
				result = append(result, criterion.Criterion+" ("+string(cell.Axis)+"): "+strings.Join(cell.Conflicts, "; "))
			}
		}
	}
	for _, fact := range inputs.QualityFacts {
		if fact.Required && len(fact.RunHeads) > 1 {
			result = append(result, fact.Criterion+" "+fact.Kind+": multiple current run heads")
		}
	}
	return uniqueSorted(result)
}

func newGate(name GateName, configured bool) Gate {
	return Gate{
		Name: name, Configured: configured, Facts: []Fact{}, Blockers: []Blocker{},
		Axes: []coverage.AxisSummary{}, Exclusions: []coverage.ExclusionRow{}, Gaps: []coverage.AxisGap{},
		StalePins: []string{}, NotInferred: notInferred[name],
	}
}

func (gate *Gate) fact(value Fact) { gate.Facts = append(gate.Facts, value) }

// axis records one whole axis column: its counts, every current exclusion, every
// stale pin, and one fact per criterion asserting the three-state invariant that
// each cell holds a current link, an explicit exclusion, or a visible gap.
func (gate *Gate) axis(projection coverage.AxisProjection, axis coverage.Axis) {
	if counts, ok := projection.Summary(axis); ok {
		gate.Axes = append(gate.Axes, coverage.AxisSummary{Axis: axis, Counts: counts})
	}
	examined := 0
	for _, criterion := range projection.Criteria {
		if _, ok := criterion.Axis(axis); ok {
			examined++
		}
	}
	// Scope is a fact too. Without it an axis with nothing to examine would
	// report a verdict backed by no observation at all.
	gate.fact(Fact{
		Code: "axis_examined", Axis: axis, Satisfied: true,
		Detail: countDetail(examined, "criterion", "criteria") + " examined on the " + string(axis) + " axis",
	})
	for _, criterion := range projection.Criteria {
		cell, ok := criterion.Axis(axis)
		if !ok {
			continue
		}
		paths, _ := currentPaths(cell)
		gate.fact(Fact{
			Code: "axis_" + string(cell.Resolution), Resource: criterion.Criterion, Axis: axis,
			Satisfied: cell.Resolution != coverage.ResolutionGap,
			Detail:    axisDetail(cell), Paths: paths,
		})
		if cell.Gap != nil {
			gate.Gaps = append(gate.Gaps, *cell.Gap)
		}
		if cell.Resolution == coverage.ResolutionExcluded && cell.Exclusion != nil {
			gate.Exclusions = append(gate.Exclusions, coverage.ExclusionRow{
				Criterion: criterion.Criterion, Axis: axis,
				Exception: cell.Exclusion.Exception.URN, Rationale: cell.Exclusion.Exception.Rationale,
			})
		}
		for _, pin := range cell.StalePins {
			gate.StalePins = append(gate.StalePins, criterion.Criterion+" ("+string(axis)+"): "+pin)
		}
	}
}

func axisDetail(cell coverage.AxisCoverage) string {
	parts := []string{string(cell.State)}
	if cell.Precision != "" && cell.Precision != "none" {
		parts = append(parts, "precision "+cell.Precision)
	}
	if cell.Exclusion != nil {
		parts = append(parts, "exception "+cell.Exclusion.Exception.URN+" ("+string(cell.Exclusion.State)+")")
	}
	if cell.Gap != nil {
		parts = append(parts, cell.Gap.Reasons...)
	}
	return strings.Join(parts, "; ")
}

func currentPaths(cell coverage.AxisCoverage) ([][]string, int) {
	paths := [][]string{}
	diffs := 0
	for _, link := range cell.Links {
		if !link.Current {
			continue
		}
		paths = append(paths, link.Link.Paths...)
		diffs += len(link.Link.Diffs)
	}
	return paths, diffs
}

// finishGate derives the verdict and the blocker list from the facts. A gate is
// satisfied only when every fact it required holds; an unconfigured gate is
// reported as not_applicable but keeps its facts so the guidance is not lost.
func finishGate(gate *Gate) {
	satisfied, blockers := gateVerdict(*gate)
	gate.Satisfied = satisfied
	gate.Blockers = blockers
	switch {
	case !gate.Configured:
		gate.Status = StatusNotApplicable
	case satisfied:
		gate.Status = StatusReady
	default:
		gate.Status = StatusBlocked
	}
	gate.StalePins = uniqueSorted(gate.StalePins)
	sort.SliceStable(gate.Gaps, func(i, j int) bool {
		if gate.Gaps[i].Criterion != gate.Gaps[j].Criterion {
			return gate.Gaps[i].Criterion < gate.Gaps[j].Criterion
		}
		return gate.Gaps[i].Axis < gate.Gaps[j].Axis
	})
}

func gateVerdict(gate Gate) (bool, []Blocker) {
	satisfied := true
	blockers := []Blocker{}
	for _, fact := range gate.Facts {
		if fact.Satisfied {
			continue
		}
		satisfied = false
		blocker := Blocker{Code: fact.Code, Resource: fact.Resource, Detail: fact.Detail}
		if len(fact.Paths) > 0 {
			blocker.Path = fact.Paths[0]
		}
		blockers = append(blockers, blocker)
	}
	return satisfied, blockers
}

func gateDetail(gate Gate, satisfied bool) string {
	if !gate.Configured {
		return string(gate.Name) + " is not configured by this policy"
	}
	if satisfied {
		return string(gate.Name) + " is ready"
	}
	_, blockers := gateVerdict(gate)
	return countDetail(len(blockers), "blocking fact", "blocking facts")
}

func headDetail(kind string, heads []string) string {
	switch len(heads) {
	case 1:
		return "one " + kind + " head"
	case 0:
		return "no " + kind + " head"
	default:
		return "competing " + kind + " heads: " + strings.Join(uniqueSorted(heads), ", ")
	}
}

func countDetail(count int, singular, plural string) string {
	word := plural
	if count == 1 {
		word = singular
	}
	return strconv.Itoa(count) + " " + word
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func orNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, "; ")
}
