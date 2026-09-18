package nextaction

import (
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/quality"
)

// staleRecords asks, for every record whose pin moved, whether the claim it
// relied on still holds against the current head. Nothing silently follows a
// new revision: the author re-pins or retires the record.
func (b *builder) staleRecords() {
	for _, record := range b.status.Stale {
		if record.Kind == "diff_selector" {
			continue // handled once by changed-source accounting
		}
		action := Action{
			ID: "stale:" + record.Record, Kind: KindQuestion, Category: CategoryStale, Resource: record.Record,
			Gates:  gatesForStale(record.Kind),
			Reason: record.Kind + " is stale (" + strings.Join(record.Reasons, "; ") + ")" + pinSummary(record.Pins) + affectsSummary(record.Affects),
		}
		switch record.Kind {
		case "relation":
			action.Question = b.relationQuestion(record)
		case "prototype_annotation":
			action.Question = question("Does the annotated prototype element still express "+b.subjects(record.Affects)+" as currently worded?", NeedProductJudgment,
				option("yes", "a new annotation pinned to the current story and prototype revisions replaces it",
					b.invoke("prototype annotate", grammar.V("target", firstOr(record.Affects, "")), grammar.V("story-revision", current(record.Pins, "story_revision")))),
				option("no", "the stale annotation stays as history and no longer counts; the prototype axis shows the gap"))
		case "coverage_exception":
			criterion := firstOr(record.Affects, "")
			action.Question = question("Does the recorded exclusion still hold for "+b.describe(criterion)+" as currently worded?", NeedExplicitExclusion,
				option("yes", "a new exception pinned to the current story revision supersedes the stale one",
					b.invoke("coverage-exception add", grammar.V("axis", record.Axis), grammar.V("criterion", criterion), grammar.V("story-revision", current(record.Pins, "story_revision"))),
					b.invoke("coverage-exception supersede", grammar.V("exception", record.Record), grammar.V("with", ""))),
				option("no", "the axis becomes a gap that needs coverage"))
		case "quality_policy":
			criterion := firstOr(record.Affects, "")
			action.Question = question("Which test kinds does "+b.describe(criterion)+" require as currently worded?", NeedProductJudgment,
				option("record the kinds", "a policy pinned to the current story revision supersedes the stale one",
					b.invoke("quality policy set", grammar.V("criterion", criterion), grammar.V("story-revision", current(record.Pins, "story_revision")), grammar.V("supersedes", record.Record))))
		case "test_run":
			testCase := testCaseOf(record.Record)
			action.Question = question("Run "+testCase+" against its current revision and the current source comparison, then record the result.", NeedExternalAccess,
				option("recorded", "an immutable run pinned to the current test revision and source identity; the stale run stays as history",
					b.invoke("quality run record", grammar.V("test", testCase), grammar.V("test-revision", current(record.Pins, "test_revision")),
						grammar.V("parent", record.Record), grammar.V("result", ""), grammar.V("evidence", ""))))
		case "quality_evidence":
			testCase := testCaseOf(record.Record)
			action.Question = question("Which exact lines now implement the evidence "+record.Record+" described?", NeedProductJudgment,
				option("re-record the evidence", "new evidence for the current test revision supersedes the stale record",
					b.invoke("quality evidence add", grammar.V("test", testCase), grammar.V("test-revision", current(record.Pins, "test_revision")),
						grammar.V("role", ""), grammar.V("diff", ""), grammar.V("supersedes", record.Record))))
		default:
			action.Question = question("Does "+record.Record+" still hold against the current heads?", NeedProductJudgment,
				option("revisit the record", "re-pin or retire it, then re-evaluate"))
		}
		b.add(action)
	}
}

func (b *builder) relationQuestion(record livingapp.StaleRecord) *Question {
	subject := b.subjects(record.Affects)
	values := []grammar.Value{grammar.V("type", record.Type), grammar.V("from", record.From), grammar.V("to", record.To)}
	if record.Scope != "" {
		values = append(values, grammar.V("scope", record.Scope))
	}
	for _, pin := range record.Pins {
		if pin.Current == "" {
			continue
		}
		flag := strings.ReplaceAll(pin.Field, "_", "-")
		values = append(values, grammar.V(flag, pin.Current))
	}
	return question("Does the relation still hold for "+subject+" against the current revision and content?", NeedProductJudgment,
		option("yes", "retire the stale relation and record one pinned to the current heads",
			b.invoke("relation supersede", grammar.V("relation", record.Record)),
			b.invoke("relation add", values...)),
		option("no", "retire the stale relation; the axis it covered becomes a visible gap",
			b.invoke("relation supersede", grammar.V("relation", record.Record))))
}

func (b *builder) subjects(criteria []string) string {
	if len(criteria) == 0 {
		return "its target"
	}
	if len(criteria) == 1 {
		return b.describe(criteria[0])
	}
	return strings.Join(criteria, ", ")
}

func gatesForStale(kind string) []string {
	switch kind {
	case "prototype_annotation":
		return []string{"product_ready", "ready_for_review"}
	case "quality_policy", "test_run", "quality_evidence":
		return []string{"quality_ready", "ready_for_review"}
	default:
		return []string{"design_ready", "implementation_trace_ready", "quality_ready", "ready_for_review"}
	}
}

func pinSummary(pins []livingapp.Pin) string {
	parts := []string{}
	for _, pin := range pins {
		if pin.Current != "" && pin.Current != pin.Pinned {
			parts = append(parts, pin.Field+" "+pin.Pinned+" -> "+pin.Current)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "; " + strings.Join(parts, ", ")
}

func affectsSummary(affects []string) string {
	if len(affects) == 0 {
		return ""
	}
	return "; affects " + strings.Join(affects, ", ")
}

func current(pins []livingapp.Pin, field string) string {
	for _, pin := range pins {
		if pin.Field == field {
			return pin.Current
		}
	}
	return ""
}

func firstOr(values []string, fallback string) string {
	if len(values) > 0 {
		return values[0]
	}
	return fallback
}

// testCaseOf returns the test-case URN owning a nested quality URN.
func testCaseOf(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) >= 5 {
		return strings.Join(parts[:5], ":")
	}
	return urn
}

// changedSource keeps the global omission invariant as its own actions. No
// exception is ever offered here.
func (b *builder) changedSource() {
	source := b.status.ChangedSource
	gates := []string{"implementation_trace_ready", "ready_for_review"}
	if len(source.Orphans) > 0 {
		affected := []string{}
		for _, orphan := range source.Orphans {
			affected = append(affected, orphan.Affects...)
		}
		b.add(Action{
			ID: "source:orphans", Kind: KindCommand, Category: CategorySource, Gates: gates,
			Reason: itoa(len(source.Orphans)) + " diff selectors no longer match the current source comparison" + affectsSummary(unique(affected)) +
				"; prove equivalence first with --dry-run, and re-author any selector it cannot carry",
			Command: ptr(b.invoke("rebase-evidence", grammar.V("dry-run", "true"), grammar.V("json", "true"))),
		})
	}
	for _, uncovered := range source.Uncovered {
		b.add(Action{
			ID: "source:uncovered:" + uncovered.Path, Kind: KindCommand, Category: CategorySource, Resource: uncovered.Path, Gates: gates,
			Reason:  itoa(uncovered.Atoms) + " changed atoms in " + uncovered.Path + " are owned by no target; choose the smallest target that explains them",
			Command: ptr(b.invoke("cover", grammar.V("target", ""), grammar.V("path", uncovered.Path), grammar.V("changed-lines", "true"))),
		})
	}
	for _, implicated := range source.Implicated {
		if implicated.Kind != "test_case" {
			continue
		}
		b.add(Action{
			ID: "source:test-case:" + implicated.Resource, Kind: KindQuestion, Category: CategorySource, Resource: implicated.Resource,
			Gates:  []string{"quality_ready", "ready_for_review"},
			Reason: "a source change touched this test case's evidence: " + strings.Join(implicated.Via, "; "),
			Question: question("Does "+implicated.Resource+" still verify its criteria after the source change? Re-run it and re-record its evidence.", NeedExternalAccess,
				option("recorded", "new evidence and a new run pinned to the current source comparison",
					b.invoke("quality evidence add", grammar.V("test", implicated.Resource)),
					b.invoke("quality run record", grammar.V("test", implicated.Resource)))),
		})
	}
}

// requirements asks for the story decisions readiness needs before any axis
// can be evaluated.
func (b *builder) requirements() {
	gates := []string{"requirements_ready", "ready_for_review"}
	requirementsAdopted := false
	for _, capability := range b.status.Capabilities {
		if capability.Name == "requirements" && capability.State == string(quality.Adopted) {
			requirementsAdopted = true
		}
	}
	accepted := 0
	for _, story := range b.status.Stories {
		if story.State == "accepted" {
			accepted++
			if len(story.Criteria) == 0 && story.CurrentRevision != "" {
				b.add(Action{
					ID: "requirements:criteria:" + story.Story, Kind: KindQuestion, Category: CategoryRequirements, Resource: story.Story, Gates: gates,
					Reason: "an accepted story needs at least one acceptance criterion",
					Question: question("What observable acceptance criteria does \""+story.Title+"\" have?", NeedProductJudgment,
						option("add a criterion", "a complete story revision with the new criterion",
							b.invoke("criterion add", grammar.V("story", story.Story), grammar.V("parent", story.CurrentRevision)))),
				})
			}
			continue
		}
		if story.State != "proposed" || len(story.LifecycleHeads) != 1 {
			continue
		}
		b.add(Action{
			ID: "requirements:accept:" + story.Story, Kind: KindQuestion, Category: CategoryRequirements, Resource: story.Story, Gates: gates,
			Reason: "a proposed story contributes no criteria to readiness",
			Question: question("Is \""+story.Title+"\" accepted into this change's scope?", NeedProductJudgment,
				option("accepted", "its current criteria become the traceability backbone",
					b.invoke("story set-state", grammar.V("story", story.Story), grammar.V("parent", story.LifecycleHeads[0]), grammar.V("state", "accepted"))),
				option("deferred", "it stays recorded and out of scope",
					b.invoke("story set-state", grammar.V("story", story.Story), grammar.V("parent", story.LifecycleHeads[0]), grammar.V("state", "deferred")))),
		})
	}
	if accepted > 0 {
		return
	}
	options := []Option{option("record a story", "a story with criteria; accept it once it is in scope", b.invoke("story add"))}
	if !requirementsAdopted && b.status.SagaVersion < 3 {
		options = []Option{option("record stories", "adopt the living container first, then add stories",
			b.invoke("upgrade", grammar.V("to", "3")), b.invoke("story add"))}
	}
	b.add(Action{
		ID: "requirements:accepted-story", Kind: KindQuestion, Category: CategoryRequirements, Gates: gates,
		Reason:   "no accepted story exists, so no criterion anchors the transitive code -> design -> criterion trace",
		Question: question("Which user stories, with acceptance criteria, does this change deliver?", NeedProductJudgment, options...),
	})
}

// capabilities surfaces a capability that is not adopted as a decision, not an
// error and not a silent pass.
func (b *builder) capabilities() {
	if b.status.Quality.Adoption != string(quality.NotAdopted) || len(b.status.Axes.Criteria) == 0 {
		return
	}
	options := []Option{option("no", "quality_ready stays blocked as not_adopted; under the compatibility policy it does not gate peer review")}
	if b.status.SagaVersion != quality.Version {
		options = append([]Option{option("yes", "adopt the v5 report container, then add test cases, policies, evidence, and runs",
			b.invoke("upgrade", grammar.V("to", "5")), b.invoke("quality test-case add"))}, options...)
	} else {
		options = append([]Option{option("yes", "add the first test case; the ___quality root becomes adopted", b.invoke("quality test-case add"))}, options...)
	}
	b.add(Action{
		ID: "capability:quality", Kind: KindQuestion, Category: CategoryCapability, Resource: "quality",
		Gates:    []string{"quality_ready"},
		Reason:   "quality is not_adopted: " + b.status.Quality.Reason + "; the quality axis of every accepted criterion is a gap until it is adopted or excluded",
		Question: question("Should this Saga record test cases so the quality axis can be satisfied?", NeedProductJudgment, options...),
	})
}

func (b *builder) prototypes() {
	for _, prototype := range b.status.Prototypes {
		if !prototype.Retained || len(prototype.CurrentLinks) > 0 {
			continue
		}
		b.add(Action{
			ID: "orphan:prototype:" + prototype.Prototype, Kind: KindQuestion, Category: CategoryOrphan, Resource: prototype.Prototype,
			Gates:  []string{"product_ready", "ready_for_review"},
			Reason: "a retained prototype has no current story or criterion annotation, so it cannot contribute to readiness",
			Question: question("Which story or criterion does "+prototype.Prototype+" express, or should it be retired?", NeedProductJudgment,
				option("it expresses a requirement", "a pinned annotation", b.invoke("prototype annotate", grammar.V("prototype", prototype.Prototype)))),
		})
	}
}

func (b *builder) testCases() {
	for _, testCase := range b.status.Quality.TestCases {
		if !testCase.Orphaned || testCase.Lifecycle == string(quality.StateRetired) {
			continue
		}
		b.add(Action{
			ID: "orphan:test-case:" + testCase.TestCase, Kind: KindQuestion, Category: CategoryOrphan, Resource: testCase.TestCase,
			Gates:  []string{"quality_ready"},
			Reason: "no active verifies relation connects this test case to a criterion, so it proves nothing about any story",
			Question: question("Which criterion does "+testCase.TestCase+" verify, or should it be retired?", NeedProductJudgment,
				option("it verifies a criterion", "a pinned verifies relation",
					b.invoke("relation add", grammar.V("type", "verifies"), grammar.V("from", testCase.TestCase), grammar.V("from-revision", testCase.CurrentRevision), grammar.V("to", ""), grammar.V("to-revision", ""))),
				option("retire it", "it stays as history and leaves the queue",
					b.invoke("quality test-case set-state", grammar.V("test", testCase.TestCase), grammar.V("state", "retired")))),
		})
	}
}

func itoa(value int) string { return strconv.Itoa(value) }
