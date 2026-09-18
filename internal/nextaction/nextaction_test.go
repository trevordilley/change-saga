package nextaction

import (
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/readiness"
)

const saga = "checkout.saga"

func criterion(id string) string { return "urn:change-saga:checkout:story:refund:criterion:" + id }

// statusFixture is a small projection with one failing quality kind, one stale
// run, one stale relation, one orphaned selector, one uncovered file, and gaps.
func statusFixture() livingapp.Status {
	cells := func(id string, states map[coverage.Axis]coverage.AxisCoverage) coverage.CriterionCoverage {
		row := coverage.CriterionCoverage{Criterion: criterion(id), Story: "urn:change-saga:checkout:story:refund", Axes: []coverage.AxisCoverage{}}
		for _, axis := range coverage.Axes() {
			value, ok := states[axis]
			if !ok {
				value = coverage.AxisCoverage{Criterion: criterion(id), Axis: axis, State: coverage.StateCoveredDirect, Resolution: coverage.ResolutionLinked}
			}
			row.Axes = append(row.Axes, value)
		}
		return row
	}
	gap := func(id string, axis coverage.Axis, reason string) coverage.AxisCoverage {
		return coverage.AxisCoverage{Criterion: criterion(id), Axis: axis, State: coverage.StateGap, Resolution: coverage.ResolutionGap,
			Gap: &coverage.AxisGap{Criterion: criterion(id), Axis: axis, Reasons: []string{reason}}}
	}
	return livingapp.Status{
		SagaID: "checkout", SagaVersion: 5,
		Capabilities: []livingapp.Capability{{Name: "requirements", State: "adopted"}, {Name: "quality", State: "adopted"}},
		Stories: []livingapp.StoryStatus{{
			Story: "urn:change-saga:checkout:story:refund", Title: "Refund", State: "accepted",
			RevisionHeads: []string{"urn:change-saga:checkout:story:refund:revision:r2"}, LifecycleHeads: []string{"urn:change-saga:checkout:story:refund:event:accepted"},
			CurrentRevision: "urn:change-saga:checkout:story:refund:revision:r2",
			Criteria:        []livingapp.CriterionStatus{{Criterion: criterion("failing"), Statement: "Rejects late refunds."}, {Criterion: criterion("done"), Statement: "Accepts on time."}},
		}},
		Axes: coverage.AxisProjection{Criteria: []coverage.CriterionCoverage{
			cells("failing", map[coverage.Axis]coverage.AxisCoverage{
				coverage.AxisUI:             gap("failing", coverage.AxisUI, "no UI reference resource"),
				coverage.AxisQuality:        gap("failing", coverage.AxisQuality, "required positive test: failed"),
				coverage.AxisImplementation: gap("failing", coverage.AxisImplementation, "no current implementation link"),
			}),
			cells("done", nil),
		}},
		Quality: livingapp.QualityStatus{Adoption: "adopted", Criteria: []livingapp.QualityCriterion{{
			Criterion: criterion("failing"), RequiredKinds: []string{"positive"},
			Kinds: []livingapp.KindStatus{{Kind: "positive", Required: true, State: "failed", TestCases: []string{"urn:change-saga:checkout:test-case:broken"}}},
		}}},
		Stale: []livingapp.StaleRecord{
			{Record: "urn:change-saga:checkout:relation:old", Kind: "relation", History: "review", Reasons: []string{"to revision changed"},
				Type: "verifies", From: "urn:change-saga:checkout:test-case:happy", To: criterion("done"),
				Pins:    []livingapp.Pin{{Field: "to_revision", Pinned: "urn:change-saga:checkout:story:refund:revision:r1", Current: "urn:change-saga:checkout:story:refund:revision:r2"}},
				Affects: []string{criterion("done")}},
			{Record: "urn:change-saga:checkout:test-case:revised:run:ci-1", Kind: "test_run", History: "review", Reasons: []string{"test revision changed"},
				Pins: []livingapp.Pin{{Field: "test_revision", Pinned: "urn:change-saga:checkout:test-case:revised:revision:r1", Current: "urn:change-saga:checkout:test-case:revised:revision:r2"}}},
		},
		ChangedSource: livingapp.ChangedSource{
			Uncovered: []livingapp.UncoveredPath{{Path: "internal/other.go", Atoms: 1}}, UncoveredAtoms: 1,
			Orphans: []livingapp.OrphanRef{{Target: "item", DiffFile: "40-e.json", Diff: 1, Affects: []string{criterion("done")}}},
		},
		Readiness: readiness.GateProjection{Policy: readiness.PolicyFeature},
	}
}

func TestActionsAreOrderedByCategoryThenAxisThenResource(t *testing.T) {
	actions := Derive(statusFixture(), saga)
	if len(actions) == 0 {
		t.Fatal("a projection with gaps must produce next actions")
	}
	for index := 1; index < len(actions); index++ {
		left, right := actions[index-1], actions[index]
		if categoryRank[left.Category] > categoryRank[right.Category] {
			t.Fatalf("category order broken at %d: %s before %s", index, left.Category, right.Category)
		}
	}
	if actions[0].Category != CategoryStale {
		t.Fatalf("stale pins are revisited before gaps are filled: first is %s", actions[0].Category)
	}
	if !reflect.DeepEqual(actions, Derive(statusFixture(), saga)) {
		t.Fatal("next actions must be deterministic")
	}
}

func TestDeterministicActionsCarryGrammarShapesAndQuestionsCarryOneQuestion(t *testing.T) {
	for _, action := range Derive(statusFixture(), saga) {
		switch action.Kind {
		case KindCommand:
			if action.Command == nil || action.Question != nil {
				t.Fatalf("a command action carries exactly one command shape: %#v", action)
			}
			assertGrammarShape(t, *action.Command)
		case KindQuestion:
			if action.Question == nil || action.Command != nil || strings.TrimSpace(action.Question.Text) == "" || len(action.Question.Options) == 0 {
				t.Fatalf("a question action carries one focused question and its answers: %#v", action)
			}
			for _, option := range action.Question.Options {
				for _, command := range option.Commands {
					assertGrammarShape(t, command)
				}
			}
		default:
			t.Fatalf("unknown action kind %q", action.Kind)
		}
	}
}

// assertGrammarShape proves an emitted shape came from the published grammar:
// the command exists and every argument is a flag it declares.
func assertGrammarShape(t *testing.T, invocation grammar.Invocation) {
	t.Helper()
	command, ok := grammar.Lookup(invocation.Command)
	if !ok || command.Usage != invocation.Usage || command.Status != invocation.Status {
		t.Fatalf("shape %q does not match the grammar: %#v", invocation.Command, invocation)
	}
	for _, argument := range append(append([]grammar.Argument{}, invocation.Arguments...), invocation.Inputs...) {
		if _, ok := command.Flag(argument.Flag); !ok {
			t.Fatalf("shape %q uses undeclared --%s", invocation.Command, argument.Flag)
		}
	}
}

func TestFailingRunAsksForJudgmentAndStaleRunNeedsExternalAccess(t *testing.T) {
	actions := byID(Derive(statusFixture(), saga))
	failing := actions["gap:quality:"+criterion("failing")]
	if failing.Question == nil || failing.Question.Needs != NeedProductJudgment || !strings.Contains(failing.Question.Text, "product or the test") {
		t.Fatalf("a failing run asks whether the product or the test is wrong: %#v", failing)
	}
	run := actions["stale:urn:change-saga:checkout:test-case:revised:run:ci-1"]
	if run.Question == nil || run.Question.Needs != NeedExternalAccess {
		t.Fatalf("re-running a test needs external access: %#v", run)
	}
	shape := run.Question.Options[0].Commands[0]
	if shape.Command != "quality run record" || shape.Status != grammar.StatusPlanned || !hasArgument(shape, "test-revision", "urn:change-saga:checkout:test-case:revised:revision:r2") {
		t.Fatalf("the re-run shape pins the current test revision: %#v", shape)
	}
}

func TestStaleRelationRestatesTheSameClaimAgainstCurrentPins(t *testing.T) {
	action := byID(Derive(statusFixture(), saga))["stale:urn:change-saga:checkout:relation:old"]
	if action.Question == nil || len(action.Question.Options) != 2 {
		t.Fatalf("a stale relation asks whether the claim still holds: %#v", action)
	}
	yes := action.Question.Options[0].Commands
	if len(yes) != 2 || yes[0].Command != "relation supersede" || yes[1].Command != "relation add" {
		t.Fatalf("yes supersedes then re-records: %#v", yes)
	}
	add := yes[1]
	if !hasArgument(add, "type", "verifies") || !hasArgument(add, "to-revision", "urn:change-saga:checkout:story:refund:revision:r2") || !hasArgument(add, "to", criterion("done")) {
		t.Fatalf("the refreshed relation restates type, endpoints, and the current pin: %#v", add)
	}
}

func TestExclusionsAreNeverOfferedForSourceOrImplementation(t *testing.T) {
	for _, action := range Derive(statusFixture(), saga) {
		if action.Category != CategorySource && action.Axis != string(coverage.AxisImplementation) {
			continue
		}
		for _, command := range commandsOf(action) {
			if strings.HasPrefix(command.Command, "coverage-exception") {
				t.Fatalf("%s offers an exception; changed-source accounting and delivery are never excusable", action.ID)
			}
		}
	}
	actions := byID(Derive(statusFixture(), saga))
	if actions["source:orphans"].Command == nil || actions["source:orphans"].Command.Command != "rebase-evidence" {
		t.Fatalf("orphaned selectors get the deterministic rebase shape: %#v", actions["source:orphans"])
	}
	if cover := actions["source:uncovered:internal/other.go"]; cover.Command == nil || !hasArgument(*cover.Command, "path", "internal/other.go") {
		t.Fatalf("an uncovered file gets a cover shape for its path: %#v", cover)
	}
	ui := actions["gap:ui:"+criterion("failing")]
	if ui.Question == nil || ui.Question.Needs != NeedExplicitExclusion {
		t.Fatalf("a ui gap can only be resolved by an explicit exclusion today: %#v", ui)
	}
}

func TestCoveredCriterionProducesNoActionAndNotAdoptedQualityIsOneDecision(t *testing.T) {
	status := statusFixture()
	for _, action := range Derive(status, saga) {
		if action.Resource == criterion("done") && action.Category == CategoryCoverage {
			t.Fatalf("a fully covered criterion has no coverage action: %#v", action)
		}
	}
	status.Quality = livingapp.QualityStatus{Adoption: string(quality.NotAdopted), Reason: "v3 Saga"}
	status.SagaVersion = 3
	actions := Derive(status, saga)
	quality := 0
	for _, action := range actions {
		if action.Axis == string(coverage.AxisQuality) {
			t.Fatalf("an unadopted capability is one decision, not a question per criterion: %#v", action)
		}
		if action.ID == "capability:quality" {
			quality++
			if !strings.Contains(strings.Join(action.Question.Options[0].Commands[0].Argv, " "), "upgrade --to 5") {
				t.Fatalf("adopting quality on v3 starts with the v5 upgrade shape: %#v", action.Question.Options[0])
			}
		}
	}
	if quality != 1 {
		t.Fatalf("expected one quality adoption decision, found %d", quality)
	}
}

func TestEmptyActionsAreTheFixedPoint(t *testing.T) {
	status := livingapp.Status{
		Stories: []livingapp.StoryStatus{{Story: "urn:change-saga:checkout:story:refund", State: "accepted", CurrentRevision: "r", Criteria: []livingapp.CriterionStatus{{Criterion: criterion("done")}}}},
		Axes:    coverage.AxisProjection{Criteria: []coverage.CriterionCoverage{}},
		Quality: livingapp.QualityStatus{Adoption: "adopted"},
		ChangedSource: livingapp.ChangedSource{
			Complete: true, Uncovered: []livingapp.UncoveredPath{}, Orphans: []livingapp.OrphanRef{}, TestOwned: []livingapp.TestOwned{},
		},
	}
	if actions := Derive(status, saga); len(actions) != 0 {
		t.Fatalf("nothing left to do yields no actions: %#v", actions)
	}
	loop := AuthoringLoop(saga)
	if len(loop.AfterEachMutation) != 2 || loop.AfterEachMutation[0].Command != "validate" || !strings.Contains(loop.FixedPoint, "never that the change is correct") {
		t.Fatalf("the loop validates and re-evaluates, and never claims correctness: %#v", loop)
	}
}

func byID(actions []Action) map[string]Action {
	result := map[string]Action{}
	for _, action := range actions {
		result[action.ID] = action
	}
	return result
}

func commandsOf(action Action) []grammar.Invocation {
	result := []grammar.Invocation{}
	if action.Command != nil {
		result = append(result, *action.Command)
	}
	if action.Question != nil {
		for _, option := range action.Question.Options {
			result = append(result, option.Commands...)
		}
	}
	return result
}

func hasArgument(invocation grammar.Invocation, flag, value string) bool {
	for _, argument := range invocation.Arguments {
		if argument.Flag == flag && argument.Value == value {
			return true
		}
	}
	return false
}
