// Package nextaction derives the ordered next actions an authoring agent takes
// from a status projection. A deterministic action carries its command shape;
// an action that needs product judgment, external access, or an explicit
// exclusion carries one focused question instead, with the command shape each
// answer leads to. Every shape is built by the grammar package, the same source
// `change-saga spec --json` publishes, so a suggestion can never name a command
// or flag the spec does not declare.
//
// The loop is: inspect status, ask or mutate, validate, re-evaluate. The fixed
// point is an empty list. Reaching it never implies the change is correct.
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
// record makes every later fact unreliable, stale pins must be revisited before
// gaps are filled, and source accounting is a separate required fact.
type Category string

const (
	CategoryInvalidSaga  Category = "invalid_saga"
	CategoryConflict     Category = "conflict"
	CategoryInvalid      Category = "invalid"
	CategoryStale        Category = "stale"
	CategorySource       Category = "changed_source"
	CategoryRequirements Category = "requirements"
	CategoryCoverage     Category = "coverage"
	CategoryOrphan       Category = "orphan"
)

var categoryRank = map[Category]int{
	CategoryInvalidSaga: 0, CategoryConflict: 1, CategoryInvalid: 2, CategoryStale: 3, CategorySource: 4,
	CategoryRequirements: 5, CategoryCoverage: 6, CategoryOrphan: 7,
}

// Action is one ordered next step.
type Action struct {
	ID       string              `json:"id"`
	Kind     Kind                `json:"kind"`
	Category Category            `json:"category"`
	Gates    []string            `json:"gates"`
	Resource string              `json:"resource,omitempty"`
	Axis     string              `json:"axis,omitempty"`
	Reason   string              `json:"reason"`
	Command  *grammar.Invocation `json:"command,omitempty"`
	Question *Question           `json:"question,omitempty"`
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
		FixedPoint: "next_actions is empty; that means no required current gap remains, never that the change is correct",
	}
}

type builder struct {
	status  livingapp.Status
	saga    string
	actions map[string]Action
	stories map[string]livingapp.StoryStatus
	crit    map[string]criterionInfo
}

type criterionInfo struct {
	story, statement, revision string
}

// Derive returns the ordered next actions for one status projection.
func Derive(status livingapp.Status, sagaPath string) []Action {
	b := &builder{status: status, saga: sagaPath, actions: map[string]Action{}, stories: map[string]livingapp.StoryStatus{}, crit: map[string]criterionInfo{}}
	for _, story := range status.Stories {
		b.stories[story.Story] = story
		for _, criterion := range story.Criteria {
			b.crit[criterion.Criterion] = criterionInfo{story: story.Story, statement: criterion.Statement, revision: story.CurrentRevision}
		}
	}
	b.diagnostics()
	b.storyConflicts()
	b.cells()
	b.staleRecords()
	b.changedSource()
	b.requirements()
	b.prototypes()
	b.testCases()
	result := make([]Action, 0, len(b.actions))
	for _, action := range b.actions {
		sort.Strings(action.Gates)
		result = append(result, action)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i], result[j]
		if categoryRank[left.Category] != categoryRank[right.Category] {
			return categoryRank[left.Category] < categoryRank[right.Category]
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
	if action.Gates == nil {
		action.Gates = []string{}
	}
	if existing, ok := b.actions[action.ID]; ok {
		existing.Gates = unique(append(existing.Gates, action.Gates...))
		b.actions[action.ID] = existing
		return
	}
	action.Gates = unique(action.Gates)
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
			ID: "invalid-saga:" + diagnostic.Code, Kind: KindCommand, Category: CategoryInvalidSaga,
			Reason:  diagnostic.Message + "; living records could not be composed, so every later fact is unavailable",
			Command: ptr(b.invoke("validate", grammar.V("json", "true"))),
		})
	}
}

func (b *builder) storyConflicts() {
	for _, story := range b.status.Stories {
		if len(story.RevisionHeads) > 1 {
			b.add(Action{
				ID: "conflict:" + story.Story + ":revision", Kind: KindQuestion, Category: CategoryConflict, Resource: story.Story,
				Gates:  []string{"requirements_ready", "ready_for_review"},
				Reason: "story has competing revision heads " + strings.Join(story.RevisionHeads, ", ") + "; heads are never resolved by timestamp",
				Question: question("Which combination of these story revisions states what the story means now: "+strings.Join(story.RevisionHeads, ", ")+"?", NeedProductJudgment,
					option("write the reconciled revision", "one complete revision whose parents are every current head",
						b.invoke("story revise", append([]grammar.Value{grammar.V("story", story.Story)}, parents(story.RevisionHeads)...)...))),
			})
		}
		if len(story.LifecycleHeads) > 1 {
			b.add(Action{
				ID: "conflict:" + story.Story + ":lifecycle", Kind: KindQuestion, Category: CategoryConflict, Resource: story.Story,
				Gates:  []string{"requirements_ready", "ready_for_review"},
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

var axisGate = map[coverage.Axis]string{
	coverage.AxisPrototype: "product_ready", coverage.AxisUX: "design_ready", coverage.AxisUI: "design_ready",
	coverage.AxisTechnical: "design_ready", coverage.AxisQuality: "quality_ready", coverage.AxisImplementation: "implementation_trace_ready",
}

// cells turns every uncovered criterion/axis cell into one action. Stale cells
// are handled through the stale record that caused them.
func (b *builder) cells() {
	for _, criterion := range b.status.Axes.Criteria {
		for _, cell := range criterion.Axes {
			gates := []string{axisGate[cell.Axis], "ready_for_review"}
			id := string(cell.Axis) + ":" + criterion.Criterion
			switch cell.State {
			case coverage.StateConflicted:
				b.add(Action{
					ID: "conflict:" + id, Kind: KindQuestion, Category: CategoryConflict, Resource: criterion.Criterion, Axis: string(cell.Axis), Gates: gates,
					Reason:   strings.Join(cell.Conflicts, "; "),
					Question: question("Which of the competing records should stand for "+string(cell.Axis)+" on "+b.describe(criterion.Criterion)+"?", NeedProductJudgment, b.conflictOptions(cell)...),
				})
			case coverage.StateInvalid:
				b.add(Action{
					ID: "invalid:" + id, Kind: KindQuestion, Category: CategoryInvalid, Resource: criterion.Criterion, Axis: string(cell.Axis), Gates: gates,
					Reason:   strings.Join(cell.Invalid, "; "),
					Question: question("How should the invalid "+string(cell.Axis)+" record on "+b.describe(criterion.Criterion)+" be corrected?", NeedProductJudgment, b.invalidOptions(cell)...),
				})
			case coverage.StateGap:
				if action, ok := b.gapAction(criterion.Criterion, cell); ok {
					action.Gates = gates
					b.add(action)
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

// gapAction asks the one focused question that fills or explicitly excludes a
// gap. A missing link is never read as intent: only an authored link or a
// cited exception resolves it.
func (b *builder) gapAction(criterion string, cell coverage.AxisCoverage) (Action, bool) {
	info := b.crit[criterion]
	action := Action{
		ID: "gap:" + string(cell.Axis) + ":" + criterion, Kind: KindQuestion, Category: CategoryCoverage, Resource: criterion, Axis: string(cell.Axis),
		Reason: strings.Join(cell.Gap.Reasons, "; "),
	}
	exclude := option("it does not apply", "an explicit, cited exception resolves this axis for this criterion revision only",
		b.invoke("coverage-exception add", grammar.V("axis", string(cell.Axis)), grammar.V("criterion", criterion), grammar.V("story-revision", info.revision)))
	relate := func(relationType string, extra ...grammar.Value) grammar.Invocation {
		values := append([]grammar.Value{grammar.V("type", relationType), grammar.V("from", ""), grammar.V("to", criterion), grammar.V("to-revision", info.revision)}, extra...)
		return b.invoke("relation add", values...)
	}
	subject := b.describe(criterion)
	switch cell.Axis {
	case coverage.AxisPrototype:
		action.Question = question("Which prototype element expresses "+subject+", or does no prototype apply to it?", NeedProductJudgment,
			option("an existing prototype expresses it", "a pinned annotation connects the prototype to the criterion",
				b.invoke("prototype annotate", grammar.V("target", criterion), grammar.V("story-revision", info.revision))),
			option("a new prototype is needed", "record the prototype, then annotate it",
				b.invoke("prototype add-html"), b.invoke("prototype annotate", grammar.V("target", criterion), grammar.V("story-revision", info.revision))),
			exclude)
	case coverage.AxisUX:
		action.Question = question("Which UX flow addresses "+subject+", or does UX design not apply to it?", NeedProductJudgment,
			option("a UX deck, slide, or Item addresses it", "a digest-pinned addresses relation from the ux-role visual target",
				relate("addresses", grammar.V("from-content-digest", ""))),
			option("a new UX flow is needed", "create a ux-role deck, then relate its Item",
				b.invoke("add-deck", grammar.V("role", "ux")), relate("addresses", grammar.V("from-content-digest", ""))),
			exclude)
	case coverage.AxisUI:
		action.Question = question("Does "+subject+" need UI design? No UI reference resource is recorded yet, so the only recordable answer today is an explicit ui exception.", NeedExplicitExclusion, exclude)
	case coverage.AxisTechnical:
		action.Question = question("Which technical design addresses "+subject+", or does technical design not apply to it?", NeedProductJudgment,
			option("a design target or implementation-deck Item addresses it", "a digest-pinned addresses relation", relate("addresses", grammar.V("from-content-digest", ""))),
			exclude)
	case coverage.AxisQuality:
		return b.qualityGap(criterion, cell, action, exclude)
	case coverage.AxisImplementation:
		return b.implementationGap(criterion, cell, action)
	}
	return action, true
}

func (b *builder) qualityGap(criterion string, cell coverage.AxisCoverage, action Action, exclude Option) (Action, bool) {
	info := b.crit[criterion]
	var row livingapp.QualityCriterion
	for _, candidate := range b.status.Quality.Criteria {
		if candidate.Criterion == criterion {
			row = candidate
		}
	}
	subject := b.describe(criterion)
	missing, failing, unrun := []string{}, []string{}, []string{}
	for _, kind := range row.Kinds {
		if !kind.Required {
			continue
		}
		switch kind.State {
		case "missing_kind", "inactive", "automation_not_allowed", "invalid":
			missing = append(missing, kind.Kind)
		case "failed", "blocked", "skipped":
			failing = append(failing, kind.Kind+" ("+kind.State+" on "+strings.Join(kind.TestCases, ", ")+")")
		case "not_run":
			unrun = append(unrun, strings.Join(kind.TestCases, ", "))
		}
	}
	switch {
	case len(failing) > 0:
		action.Question = question("The current run for "+strings.Join(failing, "; ")+" on "+subject+" does not pass. Is the product or the test wrong?", NeedProductJudgment,
			option("the product is wrong", "fix the code, run the test, and record the new run",
				b.invoke("quality run record", grammar.V("result", ""), grammar.V("evidence", ""))),
			option("the test is wrong", "revise the test definition; its runs become stale until re-run",
				b.invoke("quality test-case revise")))
	case len(unrun) > 0 && len(missing) == 0:
		action.Question = question("Run "+strings.Join(unique(unrun), ", ")+" against the current test revision and source comparison, then record the result.", NeedExternalAccess,
			option("recorded", "an immutable run pinned to the current test revision and source identity",
				b.invoke("quality run record", grammar.V("result", ""), grammar.V("evidence", ""))))
	default:
		kinds := missing
		if len(kinds) == 0 {
			kinds = row.RequiredKinds
		}
		action.Question = question("Which test case verifies the "+strings.Join(unique(kinds), ", ")+" behavior of "+subject+", or does quality not apply to it?", NeedProductJudgment,
			option("an existing test case verifies it", "a pinned verifies relation from the test case",
				b.invoke("relation add", grammar.V("type", "verifies"), grammar.V("from", ""), grammar.V("from-revision", ""), grammar.V("to", criterion), grammar.V("to-revision", info.revision))),
			option("a new test case is needed", "define ordered steps and coverage kinds, activate it, and relate it",
				b.invoke("quality test-case add"),
				b.invoke("relation add", grammar.V("type", "verifies"), grammar.V("from", ""), grammar.V("from-revision", ""), grammar.V("to", criterion), grammar.V("to-revision", info.revision))),
			option("a different set of kinds is required", "a policy pinned to this story revision",
				b.invoke("quality policy set", grammar.V("criterion", criterion), grammar.V("story-revision", info.revision))),
			exclude)
	}
	return action, true
}

// implementationGap never offers an exclusion: delivery is not excusable, and
// changed-source accounting is a separate fact no exception reaches.
func (b *builder) implementationGap(criterion string, cell coverage.AxisCoverage, action Action) (Action, bool) {
	info := b.crit[criterion]
	subject := b.describe(criterion)
	for _, link := range cell.Links {
		for _, reason := range link.Link.Unsatisfied {
			if reason == livingapp.UnsatisfiedNoItemCode {
				action.Kind = KindCommand
				action.Question = nil
				action.Reason = link.Link.Source + " addresses the criterion but references no code"
				action.Command = ptr(b.invoke("cover", grammar.V("target", ""), grammar.V("path", ""), grammar.V("side", "new"), grammar.V("lines", "")))
				return action, true
			}
		}
	}
	action.Question = question("Which design Item owns the code that implements "+subject+"?", NeedProductJudgment,
		option("an Item that references the code addresses it", "a digest-pinned addresses relation from the Item",
			b.invoke("relation add", grammar.V("type", "addresses"), grammar.V("from", ""), grammar.V("from-content-digest", ""), grammar.V("to", criterion), grammar.V("to-revision", info.revision))),
		option("the implementing code has no Item yet", "reference its changed lines from an Item, then relate the Item",
			b.invoke("cover", grammar.V("target", ""), grammar.V("path", ""), grammar.V("side", "new"), grammar.V("lines", "")),
			b.invoke("relation add", grammar.V("type", "addresses"), grammar.V("from", ""), grammar.V("from-content-digest", ""), grammar.V("to", criterion), grammar.V("to-revision", info.revision))))
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
