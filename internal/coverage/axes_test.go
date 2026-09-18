package coverage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const exceptionSchemaPath = "../../schema/v5/coverage-exception.schema.json"

func criterion(urn string, links ...AxisLink) CriterionInput {
	return CriterionInput{
		URN: urn, Story: "urn:change-saga:s:story:refund",
		CurrentStoryRevision: "urn:change-saga:s:story:refund:revision:r2",
		RevisionHeads:        []string{"urn:change-saga:s:story:refund:revision:r2"},
		Links:                links,
	}
}

func exception(id string, axis Axis) Exception {
	return Exception{
		URN: "urn:change-saga:s:coverage-exception:" + id, Axis: axis,
		Criterion:     "urn:change-saga:s:story:refund:criterion:deadline",
		StoryRevision: "urn:change-saga:s:story:refund:revision:r2",
		Rationale:     "A repository-schema migration does not require this axis.",
		Citations:     []string{"urn:change-saga:s:citation:migration-policy"},
	}
}

func cell(t *testing.T, projection AxisProjection, urn string, axis Axis) AxisCoverage {
	t.Helper()
	row, ok := projection.Criterion(urn)
	if !ok {
		t.Fatalf("criterion %s missing from projection", urn)
	}
	value, ok := row.Axis(axis)
	if !ok {
		t.Fatalf("axis %s missing for %s", axis, urn)
	}
	return value
}

// The schema enum and the runtime vocabulary must not drift: a Saga written by
// one and read by the other would silently lose an exception. The six axes are
// the whole vocabulary; no aggregate value is accepted by either.
func TestExceptionSchemaAxisEnumMatchesRuntimeVocabulary(t *testing.T) {
	data, err := os.ReadFile(exceptionSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Axis struct {
				Enum []string `json:"enum"`
			} `json:"axis"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), schema.Properties.Axis.Enum...)
	sort.Strings(got)
	want := []string{}
	for _, axis := range Axes() {
		want = append(want, string(axis))
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema axis enum = %v, want %v", got, want)
	}
	for _, value := range []Axis{"design", "delivery"} {
		if value.Canonical() {
			t.Errorf("runtime accepted axis %q that the schema does not", value)
		}
	}
}

// The committed example validates, and an exception naming anything but one of
// the six axes is rejected by the schema and is an invalid record at runtime.
func TestExceptionSchemaRejectsNonCanonicalAxis(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile(exceptionSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("../../schema/v5/examples", "coverage-exception.json"))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("example exception does not validate: %v", err)
	}
	instance.(map[string]any)["axis"] = "design"
	if err := schema.Validate(instance); err == nil {
		t.Fatal("schema accepted the non-canonical axis design")
	}

	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	projection := ProjectAxes([]CriterionInput{criterion(urn)}, []Exception{exception("aggregate", "design")})
	if projection.Exceptions[0].State != ExceptionInvalid {
		t.Fatalf("non-canonical exception state = %s", projection.Exceptions[0].State)
	}
	for _, axis := range Axes() {
		if state := cell(t, projection, urn, axis).State; state != StateGap {
			t.Errorf("non-canonical exception excluded %s as %s", axis, state)
		}
	}
}

// Every criterion/axis cell is a current link, an explicit exclusion, or a
// visible gap. Those three states are the whole model.
func TestEveryRequiredAxisCellIsLinkedExcludedOrGap(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	projection := ProjectAxes(
		[]CriterionInput{criterion(urn,
			AxisLink{Axis: AxisUX, Relation: "urn:change-saga:s:relation:flow", Source: "urn:change-saga:s:deck:flows"},
			AxisLink{Axis: AxisImplementation, Relation: "urn:change-saga:s:relation:guard", Source: "urn:change-saga:s:slide:d:item:guard", StaleReasons: []string{"source content digest changed"}},
		)},
		[]Exception{exception("no-ui", AxisUI)},
	)
	row, _ := projection.Criterion(urn)
	if len(row.Axes) != len(Axes()) {
		t.Fatalf("required axis count = %d, want %d", len(row.Axes), len(Axes()))
	}
	seen := map[Resolution]int{}
	for _, value := range row.Axes {
		switch value.Resolution {
		case ResolutionLinked:
			if len(value.Links) == 0 {
				t.Errorf("%s resolved linked without a link", value.Axis)
			}
			if value.Gap != nil || value.Exclusion != nil {
				t.Errorf("%s resolved linked but also reported an exclusion or gap", value.Axis)
			}
		case ResolutionExcluded:
			if value.Exclusion == nil || value.Gap != nil {
				t.Errorf("%s resolved excluded without a current exception", value.Axis)
			}
		case ResolutionGap:
			if value.Gap == nil || len(value.Gap.Reasons) == 0 {
				t.Errorf("%s resolved gap without a stated reason", value.Axis)
			}
		default:
			t.Errorf("%s resolved to %q, which is outside the three-state model", value.Axis, value.Resolution)
		}
		seen[value.Resolution]++
	}
	if seen[ResolutionLinked] != 1 || seen[ResolutionExcluded] != 1 || seen[ResolutionGap] != 4 {
		t.Fatalf("resolutions = %v", seen)
	}
	// The stale implementation pin is a gap, but its reason stays visible.
	stale := cell(t, projection, urn, AxisImplementation)
	if stale.State != StateStale || len(stale.StalePins) != 1 {
		t.Fatalf("stale implementation link = %+v", stale)
	}
	if !strings.Contains(stale.Gap.Reasons[0], "source content digest changed") {
		t.Fatalf("stale reason was lost: %v", stale.Gap.Reasons)
	}
}

func TestBroadStoryLinkIsRetainedButNeverPresentedAsDirect(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	projection := ProjectAxes(
		[]CriterionInput{criterion(urn, AxisLink{Axis: AxisTechnical, Relation: "urn:change-saga:s:relation:erd", Source: "urn:change-saga:s:deck:erd", Broad: true})},
		nil,
	)
	value := cell(t, projection, urn, AxisTechnical)
	if value.State != StateCoveredBroad || value.Precision != "broad" {
		t.Fatalf("broad link = %+v", value)
	}
	counts, _ := projection.Summary(AxisTechnical)
	if counts.CoveredBroad != 1 || counts.CoveredDirect != 0 {
		t.Fatalf("technical counts = %+v", counts)
	}
}

func TestExceptionGoesStaleWhenTheStoryRevisionChanges(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	input := criterion(urn)
	input.CurrentStoryRevision = "urn:change-saga:s:story:refund:revision:r3"
	input.RevisionHeads = []string{"urn:change-saga:s:story:refund:revision:r3"}
	projection := ProjectAxes([]CriterionInput{input}, []Exception{exception("no-ui", AxisUI)})
	if got := projection.Exceptions[0].State; got != ExceptionStale {
		t.Fatalf("exception state = %s, want stale", got)
	}
	value := cell(t, projection, urn, AxisUI)
	if value.State != StateStale || value.Resolution != ResolutionGap {
		t.Fatalf("stale exception cell = %+v", value)
	}
	if !strings.Contains(value.Gap.Reasons[0], "revision:r3") {
		t.Fatalf("stale pin does not name the current revision: %v", value.Gap.Reasons)
	}
}

func TestCompetingExceptionHeadsConflictAndNameEveryCompetitor(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	// Two technical exceptions are two heads on the technical axis; they are
	// never resolved by timestamp.
	projection := ProjectAxes(
		[]CriterionInput{criterion(urn)},
		[]Exception{exception("broad", AxisTechnical), exception("narrow", AxisTechnical), exception("no-ui", AxisUI)},
	)
	value := cell(t, projection, urn, AxisTechnical)
	if value.State != StateConflicted || value.Resolution != ResolutionGap {
		t.Fatalf("technical cell = %+v", value)
	}
	if !strings.Contains(value.Conflicts[0], "broad") || !strings.Contains(value.Conflicts[0], "narrow") {
		t.Fatalf("competing heads were not both named: %v", value.Conflicts)
	}
	if state := cell(t, projection, urn, AxisUI).State; state != StateExcluded {
		t.Fatalf("uncontested ui axis = %s, want excluded", state)
	}
}

func TestSupersededAndInvalidExceptionsDoNotExclude(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	replacement := exception("replacement", AxisQuality)
	replacement.Supersedes = []string{"urn:change-saga:s:coverage-exception:original"}
	replacement.Citations = nil
	original := exception("original", AxisQuality)
	projection := ProjectAxes([]CriterionInput{criterion(urn)}, []Exception{original, replacement})

	states := map[string]ExceptionState{}
	for _, item := range projection.Exceptions {
		states[item.Exception.URN] = item.State
	}
	if states["urn:change-saga:s:coverage-exception:original"] != ExceptionSuperseded {
		t.Fatalf("original exception state = %s", states["urn:change-saga:s:coverage-exception:original"])
	}
	if states["urn:change-saga:s:coverage-exception:replacement"] != ExceptionInvalid {
		t.Fatalf("citation-less replacement state = %s", states["urn:change-saga:s:coverage-exception:replacement"])
	}
	value := cell(t, projection, urn, AxisQuality)
	if value.State != StateInvalid || value.Resolution != ResolutionGap {
		t.Fatalf("quality cell = %+v", value)
	}
}

func TestExclusionsAreCountedSeparatelyFromCoverage(t *testing.T) {
	urn := "urn:change-saga:s:story:refund:criterion:deadline"
	projection := ProjectAxes(
		[]CriterionInput{criterion(urn, AxisLink{Axis: AxisUX, Relation: "urn:change-saga:s:relation:flow", Source: "urn:change-saga:s:deck:flows"})},
		[]Exception{exception("no-ui", AxisUI)},
	)
	ux, _ := projection.Summary(AxisUX)
	ui, _ := projection.Summary(AxisUI)
	if ux.CoveredDirect != 1 || ux.Excluded != 0 {
		t.Fatalf("ux counts = %+v", ux)
	}
	if ui.Excluded != 1 || ui.CoveredDirect != 0 || ui.CoveredBroad != 0 || ui.Gap != 0 {
		t.Fatalf("ui counts = %+v", ui)
	}
}

// The projection must never offer a single number a UI could show instead of
// the facts.
func TestProjectionExposesNoScoreOrPercentage(t *testing.T) {
	projection := ProjectAxes(
		[]CriterionInput{criterion("urn:change-saga:s:story:refund:criterion:deadline")},
		[]Exception{exception("no-ui", AxisUI)},
	)
	data, err := json.Marshal(projection)
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
	"fraction": true, "progress": true, "readiness_score": true, "coverage_percent": true,
}

func assertNoReducingKey(t *testing.T, value any, path string) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if reducingKeys[key] {
				t.Errorf("%s/%s reduces coverage to one opaque number", path, key)
			}
			assertNoReducingKey(t, child, path+"/"+key)
		}
	case []any:
		for _, child := range value {
			assertNoReducingKey(t, child, path+"/[]")
		}
	}
}

func TestProjectAxesIsDeterministic(t *testing.T) {
	inputs := []CriterionInput{
		criterion("urn:change-saga:s:story:refund:criterion:zeta"),
		criterion("urn:change-saga:s:story:refund:criterion:alpha"),
	}
	first := ProjectAxes(inputs, []Exception{exception("no-ui", AxisUI)})
	second := ProjectAxes(inputs, []Exception{exception("no-ui", AxisUI)})
	if !reflect.DeepEqual(first, second) {
		t.Fatal("projection is not deterministic")
	}
	if first.Criteria[0].Criterion >= first.Criteria[1].Criterion {
		t.Fatalf("criteria are not ordered: %v", first.Criteria)
	}
	for index, axis := range Axes() {
		if first.Axes[index].Axis != axis || first.Criteria[0].Axes[index].Axis != axis {
			t.Fatalf("axes are not in canonical order %v: %v", Axes(), first.Axes)
		}
	}
}

// A current link that still falls short of the axis (a failing run, a path that
// owns no diff) and an axis-level unmet obligation (a required test kind nobody
// covers) are neither stale, invalid, nor conflicted. They resolve to a visible
// gap that keeps the reason, and an explicit exclusion still applies.
func TestUnsatisfiedFactsResolveToAGapWithTheirReason(t *testing.T) {
	const urn = "urn:change-saga:s:story:refund:criterion:deadline"
	failing := AxisLink{Axis: AxisQuality, Relation: "urn:change-saga:s:relation:verifies", Source: "urn:change-saga:s:test-case:t", Unsatisfied: []string{"current run failed"}}
	projection := ProjectAxes([]CriterionInput{criterion(urn, failing)}, nil)
	got := cell(t, projection, urn, AxisQuality)
	if got.State != StateGap || len(got.Unsatisfied) != 1 || !strings.Contains(got.Gap.Reasons[0], "current run failed") || got.Links[0].Current {
		t.Fatalf("an unsatisfied link is a gap with its reason: %#v", got)
	}

	passing := AxisLink{Axis: AxisQuality, Relation: "urn:change-saga:s:relation:verifies", Source: "urn:change-saga:s:test-case:t"}
	input := criterion(urn, passing)
	input.Unsatisfied = map[Axis][]string{AxisQuality: {"required negative test: missing_kind"}}
	projection = ProjectAxes([]CriterionInput{input}, nil)
	if got := cell(t, projection, urn, AxisQuality); got.State != StateGap || !strings.Contains(strings.Join(got.Gap.Reasons, ";"), "missing_kind") {
		t.Fatalf("an axis-level obligation blocks coverage even with a current link: %#v", got)
	}

	projection = ProjectAxes([]CriterionInput{input}, []Exception{exception("no-quality", AxisQuality)})
	if got := cell(t, projection, urn, AxisQuality); got.State != StateExcluded {
		t.Fatalf("an explicit current exception still resolves the axis: %#v", got)
	}

	stale := AxisLink{Axis: AxisQuality, Source: "x", StaleReasons: []string{"test revision changed"}, Unsatisfied: []string{"current run failed"}}
	projection = ProjectAxes([]CriterionInput{criterion(urn, stale)}, nil)
	if got := cell(t, projection, urn, AxisQuality); got.State != StateStale || len(got.Unsatisfied) != 0 {
		t.Fatalf("a stale link explains itself; its unsatisfied facts are not double-reported: %#v", got)
	}
}
