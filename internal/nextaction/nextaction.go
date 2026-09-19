// Package nextaction derives the ordered next actions an authoring agent takes
// from a status projection. A deterministic action carries its command shape;
// an action that needs product judgment, external access, or an explicit
// exclusion carries one focused question instead, with the command shape each
// answer leads to. Every shape is built by the grammar package, the same source
// `change-saga spec --json` publishes, so a suggestion can never name a command
// or flag the spec does not declare.
//
// Actions come in three kinds of work. Health keeps what already exists
// current (conflicts, invalid and stale records, failed runs). Implementation
// accounts for every changed line. Growth offers, and never demands, the
// next link of the chain (a story, a persona, design, a test, a term), each
// with the practice it teaches, ordered by value to the current change.
//
// The loop is: inspect status, ask or mutate, validate, re-evaluate. The fixed
// point is a list of growth suggestions only. Reaching it never implies the
// change is correct.
package nextaction

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

// Kind is whether an action can be taken as stated or needs the author.
type Kind string

const (
	KindCommand  Kind = "command"
	KindQuestion Kind = "question"
)

// Need names why an action is a question rather than a command.
type Need string

const (
	NeedProductJudgment   Need = "product_judgment"
	NeedExternalAccess    Need = "external_access"
	NeedExplicitExclusion Need = "explicit_exclusion"
)

// Category orders actions. Lower categories come first: a conflict or invalid
// record makes every later fact unreliable, stale pins and failed runs are
// what already existed breaking, and changed-source accounting is the one
// thing asked of every change. Growth comes last and is always optional.
type Category string

const (
	CategoryInvalidSaga Category = "invalid_saga"
	CategoryConflict    Category = "conflict"
	CategoryInvalid     Category = "invalid"
	CategoryStale       Category = "stale"
	// CategoryFailed is a current required test run that failed or was
	// blocked: an existing check that broke.
	CategoryFailed Category = "failed_run"
	CategorySource Category = "changed_source"
	// CategoryReview is a pull request review whose deck does not yet explain
	// every changed line of its range. It is reported, never required.
	CategoryReview Category = "review"
	// CategoryGrowth is an optional suggestion that grows the Saga one link
	// along the chain. It never blocks, and comes after everything else.
	CategoryGrowth Category = "growth"
)

var categoryRank = map[Category]int{
	CategoryInvalidSaga: 0, CategoryConflict: 1, CategoryInvalid: 2, CategoryStale: 3, CategoryFailed: 4,
	CategorySource: 5, CategoryReview: 6, CategoryGrowth: 7,
}

// Categories returns every category in order.
func Categories() []Category {
	return []Category{CategoryInvalidSaga, CategoryConflict, CategoryInvalid, CategoryStale, CategoryFailed, CategorySource, CategoryReview, CategoryGrowth}
}

// Action is one ordered next step.
type Action struct {
	ID       string   `json:"id"`
	Kind     Kind     `json:"kind"`
	Category Category `json:"category"`
	// Area is the coverage area the action advances: health or
	// implementation for required work, and the chain area a growth
	// suggestion grows. A term suggestion grows no area.
	Area     string `json:"area,omitempty"`
	Resource string `json:"resource,omitempty"`
	// Epic is the epic the action concerns, or empty when it concerns the app
	// as a whole: a persona, a flag, or the app's overall scope.
	Epic   string `json:"epic,omitempty"`
	Axis   string `json:"axis,omitempty"`
	Reason string `json:"reason"`
	// Practice, on a growth suggestion, is the practice it teaches and why
	// it pays off.
	Practice string              `json:"practice,omitempty"`
	Command  *grammar.Invocation `json:"command,omitempty"`
	Question *Question           `json:"question,omitempty"`
	// value orders growth, lowest first: a story for the most changed lines,
	// then other suggestions about the current change, then the rest.
	value int
}

// Question is one focused question for the author with what each answer does.
type Question struct {
	Text    string   `json:"text"`
	Needs   Need     `json:"needs"`
	Options []Option `json:"options"`
}

// Option is one answer and the command shapes that record it. An answer with
// no commands records nothing; the reason says what follows from it.
type Option struct {
	Answer   string               `json:"answer"`
	Effect   string               `json:"effect"`
	Commands []grammar.Invocation `json:"commands"`
}

// Loop is the inspect/mutate/validate/re-evaluate cycle, as command shapes.
type Loop struct {
	AfterEachMutation []grammar.Invocation `json:"after_each_mutation"`
	FixedPoint        string               `json:"fixed_point"`
}

// AuthoringLoop returns the loop an agent runs around next actions.
func AuthoringLoop(sagaPath string) Loop {
	return Loop{
		AfterEachMutation: []grammar.Invocation{
			grammar.MustInvoke("validate", sagaPath, grammar.V("json", "true")),
			grammar.MustInvoke("status", sagaPath, grammar.V("json", "true")),
		},
		FixedPoint: "next_actions holds only optional growth suggestions; that means every changed line is covered and nothing existing is stale or broken, never that the change is correct",
	}
}

type builder struct {
	// epicOf maps each story, criterion, test case, and prototype URN to the
	// epic that holds it.
	epicOf  map[string]string
	context Context
	status  livingapp.Status
	saga    string
	actions map[string]Action
	stories map[string]livingapp.StoryStatus
	crit    map[string]criterionInfo
}

type criterionInfo struct {
	story, statement, revision string
}

// Derive returns the ordered next actions for one status projection. The
// optional context adds the growth suggestions the coverage report implies.
func Derive(status livingapp.Status, sagaPath string, context ...Context) []Action {
	b := &builder{status: status, saga: sagaPath, actions: map[string]Action{}, stories: map[string]livingapp.StoryStatus{}, crit: map[string]criterionInfo{}, epicOf: map[string]string{}}
	if len(context) > 0 {
		b.context = context[0]
	}
	for _, story := range status.Stories {
		b.stories[story.Story] = story
		b.epicOf[story.Story] = story.Epic
		for _, criterion := range story.Criteria {
			b.crit[criterion.Criterion] = criterionInfo{story: story.Story, statement: criterion.Statement, revision: story.CurrentRevision}
			b.epicOf[criterion.Criterion] = story.Epic
		}
	}
	for _, testCase := range status.Quality.TestCases {
		b.epicOf[testCase.TestCase] = testCase.Epic
	}
	for _, prototype := range status.Prototypes {
		b.epicOf[prototype.Prototype] = prototype.Epic
	}
	b.diagnostics()
	b.storyConflicts()
	b.cells()
	b.staleRecords()
	b.changedSource()
	b.requirements()
	b.prototypes()
	b.testCases()
	b.personas()
	b.terms()
	b.growth()
	result := make([]Action, 0, len(b.actions))
	for _, action := range b.actions {
		result = append(result, b.inEpic(action))
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if categoryRank[left.Category] != categoryRank[right.Category] {
			return categoryRank[left.Category] < categoryRank[right.Category]
		}
		if left.Category == CategoryGrowth {
			if left.value != right.value {
				return left.value < right.value
			}
			if areaRank(left.Area) != areaRank(right.Area) {
				return areaRank(left.Area) < areaRank(right.Area)
			}
		}
		if axisRank(left.Axis) != axisRank(right.Axis) {
			return axisRank(left.Axis) < axisRank(right.Axis)
		}
		if left.Resource != right.Resource {
			return left.Resource < right.Resource
		}
		return left.ID < right.ID
	})
	return result
}

func axisRank(axis string) int {
	for index, candidate := range coverage.Axes() {
		if string(candidate) == axis {
			return index
		}
	}
	return -1
}

func (b *builder) add(action Action) {
	if _, ok := b.actions[action.ID]; ok {
		return
	}
	b.actions[action.ID] = action
}

func (b *builder) invoke(name string, values ...grammar.Value) grammar.Invocation {
	return grammar.MustInvoke(name, b.saga, values...)
}

func question(text string, needs Need, options ...Option) *Question {
	return &Question{Text: text, Needs: needs, Options: options}
}

func option(answer, effect string, commands ...grammar.Invocation) Option {
	if commands == nil {
		commands = []grammar.Invocation{}
	}
	return Option{Answer: answer, Effect: effect, Commands: commands}
}

func (b *builder) diagnostics() {
	for _, diagnostic := range b.status.Diagnostics {
		b.add(Action{
			ID: "invalid-saga:" + diagnostic.Code, Kind: KindCommand, Category: CategoryInvalidSaga, Area: AreaHealth,
			Reason:  diagnostic.Message + "; living records could not be composed, so every later fact is unavailable",
			Command: ptr(b.invoke("validate", grammar.V("json", "true"))),
		})
	}
}

func (b *builder) storyConflicts() {
	for _, story := range b.status.Stories {
		if len(story.RevisionHeads) > 1 {
			b.add(Action{
				ID: "conflict:" + story.Story + ":revision", Kind: KindQuestion, Category: CategoryConflict, Area: AreaHealth, Resource: story.Story,
				Reason: "story has competing revision heads " + strings.Join(story.RevisionHeads, ", ") + "; heads are never resolved by timestamp",
				Question: question("Which combination of these story revisions states what the story means now: "+strings.Join(story.RevisionHeads, ", ")+"?", NeedProductJudgment,
					option("write the reconciled revision", "one complete revision whose parents are every current head",
						b.invoke("story revise", append([]grammar.Value{grammar.V("story", story.Story)}, parents(story.RevisionHeads)...)...))),
			})
		}
		if len(story.LifecycleHeads) > 1 {
			b.add(Action{
				ID: "conflict:" + story.Story + ":lifecycle", Kind: KindQuestion, Category: CategoryConflict, Area: AreaHealth, Resource: story.Story,
				Reason: "story has competing lifecycle heads " + strings.Join(story.LifecycleHeads, ", "),
				Question: question("What is the story's lifecycle state after these competing decisions: "+strings.Join(story.LifecycleHeads, ", ")+"?", NeedProductJudgment,
					option("record the reconciled state", "one lifecycle event whose parents are every current head",
						b.invoke("story set-state", append([]grammar.Value{grammar.V("story", story.Story)}, parents(story.LifecycleHeads)...)...))),
			})
		}
	}
}

func parents(heads []string) []grammar.Value {
	values := []grammar.Value{}
	for _, head := range heads {
		values = append(values, grammar.V("parent", head))
	}
	return values
}

// cells reports the criterion/axis cells that concern health: a conflicted
// or invalid record, and a required test whose current run failed. A cell
// that is merely a gap is growth, which the coverage report suggests story
// by story and criterion by criterion; a quality cell whose test was never
// run is growth too. Stale cells are handled through the stale record that
// caused them.
func (b *builder) cells() {
	for _, criterion := range b.status.Axes.Criteria {
		for _, cell := range criterion.Axes {
			id := string(cell.Axis) + ":" + criterion.Criterion
			switch cell.State {
			case coverage.StateConflicted:
				b.add(Action{
					ID: "conflict:" + id, Kind: KindQuestion, Category: CategoryConflict, Area: AreaHealth, Resource: criterion.Criterion, Axis: string(cell.Axis),
					Reason:   strings.Join(cell.Conflicts, "; "),
					Question: question("Which of the competing records should stand for "+string(cell.Axis)+" on "+b.describe(criterion.Criterion)+"?", NeedProductJudgment, b.conflictOptions(cell)...),
				})
			case coverage.StateInvalid:
				b.add(Action{
					ID: "invalid:" + id, Kind: KindQuestion, Category: CategoryInvalid, Area: AreaHealth, Resource: criterion.Criterion, Axis: string(cell.Axis),
					Reason:   strings.Join(cell.Invalid, "; "),
					Question: question("How should the invalid "+string(cell.Axis)+" record on "+b.describe(criterion.Criterion)+" be corrected?", NeedProductJudgment, b.invalidOptions(cell)...),
				})
			case coverage.StateGap:
				if cell.Axis == coverage.AxisQuality {
					if action, ok := b.qualityRun(criterion.Criterion, cell); ok {
						b.add(action)
					}
				}
			}
		}
	}
}

func (b *builder) describe(criterion string) string {
	info, ok := b.crit[criterion]
	if !ok || info.statement == "" {
		return criterion
	}
	return criterion + " (\"" + info.statement + "\")"
}

func (b *builder) conflictOptions(cell coverage.AxisCoverage) []Option {
	options := []Option{}
	if cell.Exclusion != nil && len(cell.Exclusion.CompetingHeads) > 1 {
		for _, head := range cell.Exclusion.CompetingHeads {
			commands := []grammar.Invocation{}
			for _, other := range cell.Exclusion.CompetingHeads {
				if other != head {
					commands = append(commands, b.invoke("coverage-exception supersede", grammar.V("exception", other), grammar.V("with", head)))
				}
			}
			options = append(options, option("keep "+head, "the other exception heads are superseded by it", commands...))
		}
	}
	if len(options) == 0 {
		options = append(options, option("reconcile the competing heads", "record one head whose parents are every competing head; then re-evaluate"))
	}
	return options
}

func (b *builder) invalidOptions(cell coverage.AxisCoverage) []Option {
	options := []Option{}
	for _, link := range cell.Links {
		if len(link.Link.InvalidReasons) > 0 && link.Link.Relation != "" {
			options = append(options, option("retire "+link.Link.Relation, "the invalid relation no longer participates",
				b.invoke("relation supersede", grammar.V("relation", link.Link.Relation))))
		}
	}
	if cell.Exclusion != nil && cell.Exclusion.State == coverage.ExceptionInvalid {
		info := b.crit[cell.Criterion]
		options = append(options, option("record a corrected exception", "a complete, cited exception replaces the invalid one",
			b.invoke("coverage-exception add", grammar.V("axis", string(cell.Axis)), grammar.V("criterion", cell.Criterion), grammar.V("story-revision", info.revision)),
			b.invoke("coverage-exception supersede", grammar.V("exception", cell.Exclusion.Exception.URN), grammar.V("with", ""))))
	}
	if len(options) == 0 {
		options = append(options, option("correct the record", "fix the named record, then re-evaluate"))
	}
	return options
}

// qualityRun turns a required test kind whose current run fails into a
// health question, and one that was never run into a growth suggestion. A
// criterion with no test at all is growth the coverage report suggests.
func (b *builder) qualityRun(criterion string, cell coverage.AxisCoverage) (Action, bool) {
	var row livingapp.QualityCriterion
	for _, candidate := range b.status.Quality.Criteria {
		if candidate.Criterion == criterion {
			row = candidate
		}
	}
	subject := b.describe(criterion)
	failing, unrun := []string{}, []string{}
	for _, kind := range row.Kinds {
		if !kind.Required {
			continue
		}
		switch kind.State {
		case "failed", "blocked", "skipped":
			failing = append(failing, kind.Kind+" ("+kind.State+" on "+strings.Join(kind.TestCases, ", ")+")")
		case "not_run":
			unrun = append(unrun, kind.TestCases...)
		}
	}
	action := Action{Resource: criterion, Axis: string(cell.Axis), Reason: strings.Join(cell.Gap.Reasons, "; ")}
	switch {
	case len(failing) > 0:
		action.ID, action.Kind, action.Category, action.Area = "failed:quality:"+criterion, KindQuestion, CategoryFailed, AreaHealth
		action.Question = question("The current run for "+strings.Join(failing, "; ")+" on "+subject+" does not pass. Is the product or the test wrong?", NeedProductJudgment,
			option("the product is wrong", "fix the code, run the test, and record the new run",
				b.invoke("quality run record", grammar.V("result", ""), grammar.V("evidence", ""))),
			option("the test is wrong", "revise the test definition; its runs become stale until re-run",
				b.invoke("quality test-case revise")))
	case len(unrun) > 0:
		action.ID, action.Kind, action.Category, action.Area = "growth:run:"+criterion, KindQuestion, CategoryGrowth, AreaQuality
		action.Practice = practiceQuality
		action.value = b.valueOf(criterion)
		action.Question = question("Run "+strings.Join(unique(unrun), ", ")+" against the current test revision and source comparison, then record the result?", NeedExternalAccess,
			option("recorded", "an immutable run pinned to the current test revision and source identity",
				b.invoke("quality run record", grammar.V("result", ""), grammar.V("evidence", ""))))
	default:
		return Action{}, false
	}
	return action, true
}

func ptr[T any](value T) *T { return &value }

func unique(values []string) []string {
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
