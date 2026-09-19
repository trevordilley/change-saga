package readiness

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coverage"
)

const (
	storyURN     = "urn:change-saga:s:story:refund"
	revisionURN  = "urn:change-saga:s:story:refund:revision:r2"
	criterionURN = "urn:change-saga:s:story:refund:criterion:deadline"
)

func acceptedStory() Story {
	return Story{
		URN: storyURN, State: "accepted",
		RevisionHeads: []string{revisionURN}, LifecycleHeads: []string{"urn:change-saga:s:story:refund:event:accepted"},
		CurrentRevision: revisionURN, Criteria: []string{criterionURN},
	}
}

func link(axis coverage.Axis, code ...string) coverage.AxisLink {
	return coverage.AxisLink{
		Axis: axis, Relation: "urn:change-saga:s:relation:" + string(axis),
		Source: "urn:change-saga:s:slide:decision:item:" + string(axis),
		Paths:  [][]string{{criterionURN, "addresses", "item", "owns_code"}},
		Code:   code,
	}
}

// coveredInputs is a Saga that satisfies every gate. Each test
// removes exactly one fact so the failure it asserts is the only difference.
func coveredInputs() GateInputs {
	criterion := coverage.CriterionInput{
		URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
		RevisionHeads: []string{revisionURN},
		Links: []coverage.AxisLink{
			link(coverage.AxisPrototype), link(coverage.AxisUX), link(coverage.AxisUI),
			link(coverage.AxisTechnical), link(coverage.AxisQuality),
			link(coverage.AxisImplementation, "saga-diff://v1/line?path=refund.go"),
		},
	}
	return GateInputs{
		Stories:    []Story{acceptedStory()},
		Prototypes: []Prototype{{URN: "urn:change-saga:s:prototype:checkout", Retained: true, CurrentLinks: []string{criterionURN}}},
		Coverage:   coverage.ProjectAxes([]coverage.CriterionInput{criterion}, nil),
		QualityFacts: []QualityFact{{
			Criterion: criterionURN, Kind: "positive", Required: true,
			TestCase: "urn:change-saga:s:test-case:deadline", RunResult: "passed",
			RunHeads: []string{"urn:change-saga:s:test-case:deadline:run:ci"}, EvidenceResolved: true,
		}},
		ChangedSource: ChangedSourceAccounting{Complete: true},
	}
}

func gate(t *testing.T, projection GateProjection, name GateName) Gate {
	t.Helper()
	value, ok := projection.Gate(name)
	if !ok {
		t.Fatalf("gate %s missing from projection", name)
	}
	return value
}

func blockerCodes(value Gate) []string {
	codes := make([]string, 0, len(value.Blockers))
	for _, blocker := range value.Blockers {
		codes = append(codes, blocker.Code)
	}
	return codes
}

func TestGateTableReportsEveryGateInOrder(t *testing.T) {
	projection := EvaluateGates(coveredInputs())
	got := make([]GateName, 0, len(projection.Gates))
	for _, value := range projection.Gates {
		got = append(got, value.Name)
	}
	want := []GateName{
		GateRequirementsReady, GateProductReady, GateDesignReady,
		GateImplementationTraceReady, GateQualityReady, GateReadyForReview,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gates = %v, want %v", got, want)
	}
	for _, value := range projection.Gates {
		if value.Status != StatusReady {
			t.Errorf("%s = %s, blockers %v", value.Name, value.Status, blockerCodes(value))
		}
		if len(value.NotInferred) == 0 {
			t.Errorf("%s does not state what it refuses to infer", value.Name)
		}
		if len(value.Facts) == 0 {
			t.Errorf("%s reported a verdict with no facts", value.Name)
		}
	}
}

func TestRequirementsReadyNeedsSingleHeadsAndCriteria(t *testing.T) {
	inputs := coveredInputs()
	inputs.Stories[0].RevisionHeads = []string{revisionURN, "urn:change-saga:s:story:refund:revision:r2b"}
	inputs.Stories[0].Criteria = nil
	inputs.Stories[0].IdentityIssues = []string{"criterion id reused"}
	projection := EvaluateGates(inputs)

	requirements := gate(t, projection, GateRequirementsReady)
	if requirements.Status != StatusBlocked {
		t.Fatalf("requirements gate = %s", requirements.Status)
	}
	codes := strings.Join(blockerCodes(requirements), ",")
	for _, want := range []string{"story_revision_head", "accepted_story_has_criteria", "identity_graph_valid"} {
		if !strings.Contains(codes, want) {
			t.Errorf("missing blocker %s in %s", want, codes)
		}
	}
	review := gate(t, projection, GateReadyForReview)
	if review.Status != StatusBlocked {
		t.Fatal("ready_for_review stepped over a blocked requirements gate")
	}
	if !strings.Contains(strings.Join(blockerCodes(review), ","), "no_graph_conflicts") {
		t.Errorf("multi-head story was not reported as a graph conflict: %v", review.Blockers)
	}
}

func TestProductReadyNeedsLinkedPrototypesAndPrototypeCoverage(t *testing.T) {
	inputs := coveredInputs()
	inputs.Prototypes[0].CurrentLinks = nil
	inputs.Coverage = coverage.ProjectAxes([]coverage.CriterionInput{{
		URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
		RevisionHeads: []string{revisionURN},
		Links:         []coverage.AxisLink{link(coverage.AxisUX)},
	}}, nil)
	product := gate(t, EvaluateGates(inputs), GateProductReady)

	if product.Status != StatusBlocked {
		t.Fatalf("product gate = %s", product.Status)
	}
	codes := strings.Join(blockerCodes(product), ",")
	if !strings.Contains(codes, "retained_prototype_linked") || !strings.Contains(codes, "axis_gap") {
		t.Fatalf("product blockers = %v", codes)
	}
	if len(product.Gaps) != 1 || product.Gaps[0].Axis != coverage.AxisPrototype {
		t.Fatalf("prototype gap was not surfaced: %+v", product.Gaps)
	}
}

// design_ready is per-axis: a UX flow does not cover UI, and a current
// exception on one axis does not excuse the others.
func TestDesignReadyIsPerAxisAndHonorsExceptions(t *testing.T) {
	inputs := coveredInputs()
	inputs.Coverage = coverage.ProjectAxes(
		[]coverage.CriterionInput{{
			URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
			RevisionHeads: []string{revisionURN},
			Links:         []coverage.AxisLink{link(coverage.AxisUX)},
		}},
		[]coverage.Exception{{
			URN: "urn:change-saga:s:coverage-exception:schema-migration", Axis: coverage.AxisUI,
			Criterion: criterionURN, StoryRevision: revisionURN,
			Rationale: "A repository-schema migration does not require UI design.",
			Citations: []string{"urn:change-saga:s:citation:migration-policy"},
		}},
	)
	design := gate(t, EvaluateGates(inputs), GateDesignReady)

	if design.Status != StatusBlocked {
		t.Fatalf("design gate = %s", design.Status)
	}
	if len(design.Gaps) != 1 || design.Gaps[0].Axis != coverage.AxisTechnical {
		t.Fatalf("only the technical axis should be a gap: %+v", design.Gaps)
	}
	if len(design.Exclusions) != 1 || design.Exclusions[0].Axis != coverage.AxisUI {
		t.Fatalf("ui exclusion was not reported as its own row: %+v", design.Exclusions)
	}
	if design.Exclusions[0].Rationale == "" {
		t.Error("an exclusion was reported without its rationale")
	}
	axes := map[coverage.Axis]coverage.StateCounts{}
	for _, summary := range design.Axes {
		axes[summary.Axis] = summary.Counts
	}
	if len(axes) != 3 {
		t.Fatalf("design gate axes = %v, want ux, ui and technical", axes)
	}
	if axes[coverage.AxisUX].CoveredDirect != 1 || axes[coverage.AxisUI].Excluded != 1 || axes[coverage.AxisTechnical].Gap != 1 {
		t.Fatalf("per-axis counts = %+v", axes)
	}
}

// An exception can declare an axis inapplicable. It can never excuse the exact
// changed-source accounting: documentation-only work still ends at its diff.
func TestNoExceptionExcusesChangedSourceAccounting(t *testing.T) {
	inputs := coveredInputs()
	inputs.Coverage = coverage.ProjectAxes(
		[]coverage.CriterionInput{{
			URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
			RevisionHeads: []string{revisionURN},
		}},
		[]coverage.Exception{{
			URN: "urn:change-saga:s:coverage-exception:docs-only", Axis: coverage.AxisImplementation,
			Criterion: criterionURN, StoryRevision: revisionURN,
			Rationale: "Documentation-only obligation.",
			Citations: []string{"urn:change-saga:s:citation:support-policy"},
		}},
	)
	inputs.ChangedSource = ChangedSourceAccounting{Uncovered: []string{"docs/playbook.md:12"}, Stale: []string{"___code/gone.json#1"}}
	trace := gate(t, EvaluateGates(inputs), GateImplementationTraceReady)

	if trace.Status != StatusBlocked {
		t.Fatalf("an implementation exception excused the global diff invariant: %+v", trace)
	}
	if !strings.Contains(strings.Join(blockerCodes(trace), ","), "changed_source_accounting_complete") {
		t.Fatalf("trace blockers = %v", trace.Blockers)
	}
	excluded := false
	for _, fact := range trace.Facts {
		if fact.Code == "axis_excluded" && fact.Satisfied {
			excluded = true
		}
	}
	if !excluded {
		t.Error("the recorded implementation exception was not honored on its own axis")
	}
}

func TestImplementationTraceNeedsAPathThatEndsAtCode(t *testing.T) {
	inputs := coveredInputs()
	inputs.Coverage = coverage.ProjectAxes([]coverage.CriterionInput{{
		URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
		RevisionHeads: []string{revisionURN},
		Links:         []coverage.AxisLink{link(coverage.AxisImplementation)},
	}}, nil)
	trace := gate(t, EvaluateGates(inputs), GateImplementationTraceReady)

	if trace.Status != StatusBlocked {
		t.Fatalf("a link with no code reference satisfied the trace gate: %+v", trace)
	}
	found := false
	for _, blocker := range trace.Blockers {
		if blocker.Code == "implementation_path_ends_at_code" {
			found = true
			if len(blocker.Path) == 0 {
				t.Error("the blocker did not carry the candidate path")
			}
		}
	}
	if !found {
		t.Fatalf("trace blockers = %v", blockerCodes(trace))
	}
}

func TestQualityReadyNeedsACurrentPassingRun(t *testing.T) {
	inputs := coveredInputs()
	inputs.QualityFacts[0].RunResult = "failed"
	projection := EvaluateGates(inputs)

	quality := gate(t, projection, GateQualityReady)
	codes := strings.Join(blockerCodes(quality), ",")
	if !strings.Contains(codes, "required_kind_passing_run") {
		t.Fatalf("quality blockers = %v", codes)
	}
	review := gate(t, projection, GateReadyForReview)
	if !strings.Contains(strings.Join(blockerCodes(review), ","), "no_failed_required_run") {
		t.Fatalf("a failed current required run did not block review: %v", review.Blockers)
	}
}

func TestStaleQualityRunIsReportedAsAStalePinNotAPass(t *testing.T) {
	inputs := coveredInputs()
	inputs.QualityFacts[0].StaleReasons = []string{"run pins an older test revision"}
	quality := gate(t, EvaluateGates(inputs), GateQualityReady)

	if quality.Status != StatusBlocked {
		t.Fatal("a stale passing run satisfied quality readiness")
	}
	if len(quality.StalePins) != 1 || !strings.Contains(quality.StalePins[0], "older test revision") {
		t.Fatalf("stale pins = %v", quality.StalePins)
	}
}

// Quality is always required before review. A Saga with no quality records is
// not exempt: its quality axis is a visible gap that blocks quality_ready, and
// through it ready_for_review.
func TestQualityIsRequiredBeforeReviewEvenWithNoQualityRecords(t *testing.T) {
	inputs := coveredInputs()
	inputs.QualityFacts = nil
	inputs.Coverage = coverage.ProjectAxes([]coverage.CriterionInput{{
		URN: criterionURN, Story: storyURN, CurrentStoryRevision: revisionURN,
		RevisionHeads: []string{revisionURN},
		Links: []coverage.AxisLink{
			link(coverage.AxisPrototype), link(coverage.AxisUX), link(coverage.AxisUI), link(coverage.AxisTechnical),
			link(coverage.AxisImplementation, "saga-diff://v1/line?path=refund.go"),
		},
	}}, nil)
	projection := EvaluateGates(inputs)

	quality := gate(t, projection, GateQualityReady)
	if quality.Status != StatusBlocked || len(quality.Gaps) != 1 || quality.Gaps[0].Axis != coverage.AxisQuality {
		t.Fatalf("a Saga with no quality records was not a quality gap: %+v", quality)
	}
	review := gate(t, projection, GateReadyForReview)
	if review.Status != StatusBlocked {
		t.Fatal("a Saga with no quality records passed ready_for_review")
	}
	for _, blocker := range review.Blockers {
		if blocker.Resource == string(GateQualityReady) {
			return
		}
	}
	t.Fatalf("quality_ready was not named as the blocker: %v", review.Blockers)
}

// Every gate always applies: no gate reports itself as inapplicable, and an
// empty Saga is blocked, never vacuously ready.
func TestEveryGateAppliesAndAnEmptySagaIsBlocked(t *testing.T) {
	projection := EvaluateGates(GateInputs{})
	for _, value := range projection.Gates {
		if value.Status != StatusReady && value.Status != StatusBlocked {
			t.Errorf("%s = %s", value.Name, value.Status)
		}
	}
	if gate(t, projection, GateReadyForReview).Status != StatusBlocked {
		t.Fatal("a Saga with no accepted story is ready for review")
	}
}

func TestGateProjectionExposesNoAggregateScore(t *testing.T) {
	data, err := json.Marshal(EvaluateGates(coveredInputs()))
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	assertNoReducingKey(t, decoded, "")
}

var reducingKeys = map[string]bool{
	"percent": true, "percentage": true, "score": true, "ratio": true,
	"fraction": true, "readiness_score": true, "coverage_percent": true, "overall": true,
}

func assertNoReducingKey(t *testing.T, value any, path string) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if reducingKeys[key] {
				t.Errorf("%s/%s reduces readiness to one opaque number", path, key)
			}
			assertNoReducingKey(t, child, path+"/"+key)
		}
	case []any:
		for _, child := range value {
			assertNoReducingKey(t, child, path+"/[]")
		}
	}
}

func TestEvaluateGatesIsDeterministic(t *testing.T) {
	first := EvaluateGates(coveredInputs())
	second := EvaluateGates(coveredInputs())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("gate projection is not deterministic")
	}
}
