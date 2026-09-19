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
			ID: "stale:" + record.Record, Kind: KindQuestion, Category: CategoryStale, Area: AreaHealth, Resource: record.Record,
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
	if len(source.Stale) > 0 {
		affected := []string{}
		for _, stale := range source.Stale {
			affected = append(affected, stale.Affects...)
		}
		b.add(Action{
			ID: "source:stale", Kind: KindCommand, Category: CategoryStale, Area: AreaHealth,
			Reason: itoa(len(source.Stale)) + " code references are stale because their code changed since they were pinned" + affectsSummary(unique(affected)) +
				"; read what changed, then re-author each one with replace-coverage",
			Command: ptr(b.invoke("references", grammar.V("stale", "true"), grammar.V("diff", "true"), grammar.V("json", "true"))),
		})
	}
	for _, uncovered := range source.Uncovered {
		b.add(Action{
			ID: "source:uncovered:" + uncovered.Path, Kind: KindCommand, Category: CategorySource, Area: AreaImplementation, Resource: uncovered.Path,
			Reason:  itoa(uncovered.Atoms) + " changed atoms in " + uncovered.Path + " are owned by no target; choose the smallest target that explains them",
			Command: ptr(b.invoke("cover", b.compared(grammar.V("target", ""), grammar.V("path", uncovered.Path), grammar.V("changed-lines", "true"))...)),
		})
	}
	for _, implicated := range source.Implicated {
		if implicated.Kind != "test_case" {
			continue
		}
		b.add(Action{
			ID: "source:test-case:" + implicated.Resource, Kind: KindQuestion, Category: CategoryStale, Area: AreaHealth, Resource: implicated.Resource,
			Reason: "a source change touched this test case's evidence: " + strings.Join(implicated.Via, "; "),
			Question: question("Does "+implicated.Resource+" still verify its criteria after the source change? Re-run it and re-record its evidence.", NeedExternalAccess,
				option("recorded", "new evidence and a new run pinned to the current source comparison",
					b.invoke("quality evidence add", grammar.V("test", implicated.Resource)),
					b.invoke("quality run record", grammar.V("test", implicated.Resource)))),
		})
	}
}

// requirements suggests acceptance criteria for an accepted story that has
// none: without them, nothing can say the story is met.
func (b *builder) requirements() {
	for _, story := range b.status.Stories {
		if story.State != "accepted" || len(story.Criteria) > 0 || story.CurrentRevision == "" {
			continue
		}
		b.add(Action{
			ID: "growth:criteria:" + story.Story, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaQuality, Resource: story.Story,
			Reason: "\"" + story.Title + "\" has no acceptance criteria, so nothing can say when it is met", Practice: practiceCriteria,
			value: b.valueOf(story.Story),
			Question: question("What observable acceptance criteria does \""+story.Title+"\" have?", NeedProductJudgment,
				option("add a criterion", "a complete story revision with the new criterion",
					b.invoke("criterion add", grammar.V("story", story.Story), grammar.V("parent", story.CurrentRevision)))),
		})
	}
}

func (b *builder) prototypes() {
	for _, prototype := range b.status.Prototypes {
		if !prototype.Retained || len(prototype.CurrentLinks) > 0 {
			continue
		}
		b.add(Action{
			ID: "growth:prototype:" + prototype.Prototype, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaDesign, Resource: prototype.Prototype,
			Reason: "a retained prototype has no current story or criterion annotation, so no story says what it shows", Practice: practicePrototype,
			value: b.valueOf(prototype.Prototype),
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
			ID: "growth:test-case:" + testCase.TestCase, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaQuality, Resource: testCase.TestCase,
			Reason: "no active verifies relation connects this test case to a criterion, so it proves nothing about any story", Practice: practiceQuality,
			value: b.valueOf(testCase.TestCase),
			Question: question("Which criterion does "+testCase.TestCase+" verify, or should it be retired?", NeedProductJudgment,
				option("it verifies a criterion", "a pinned verifies relation",
					b.invoke("relation add", grammar.V("type", "verifies"), grammar.V("from", testCase.TestCase), grammar.V("from-revision", testCase.CurrentRevision), grammar.V("to", ""), grammar.V("to-revision", ""))),
				option("retire it", "it stays as history and leaves the queue",
					b.invoke("quality test-case set-state", grammar.V("test", testCase.TestCase), grammar.V("state", "retired")))),
		})
	}
}

// compared adds the comparison status was opened with, so a cover shape
// selects the same changed lines status reported.
func (b *builder) compared(values ...grammar.Value) []grammar.Value {
	if scope := b.context.Coverage.Scope; scope.Kind == "change" && scope.Against != "" {
		values = append(values, grammar.V("against", scope.Against))
		if scope.Head != "" && scope.Head != "HEAD" {
			values = append(values, grammar.V("head", scope.Head))
		}
	}
	return values
}

func itoa(value int) string { return strconv.Itoa(value) }
