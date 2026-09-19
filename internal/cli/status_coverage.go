package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/changeview"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/nextaction"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// codeTarget is one documentation target with its epic and title.
type codeTarget struct {
	epic, title string
	references  []string
}

// documentTargets indexes every documentation target: its epic, its title,
// and the code references it owns (as evidence-file#index record keys).
func documentTargets(document *saga.Saga) map[string]*codeTarget {
	result := map[string]*codeTarget{}
	at := func(target, path, title string) *codeTarget {
		value, ok := result[target]
		if !ok {
			value = &codeTarget{}
			result[target] = value
		}
		if epic := applayout.EpicOfPath(path); epic != "" {
			value.epic = epic
		}
		if value.title == "" {
			value.title = title
		}
		return value
	}
	for _, deck := range document.Decks {
		at(deck.Target, deck.Path, deck.Title)
		for _, slide := range deck.Slides {
			at(slide.Target, firstNonEmpty(slide.Path, deck.Path), slide.Title)
			for _, item := range slide.Items {
				at(item.Target, firstNonEmpty(item.Path, slide.Path, deck.Path), firstNonEmpty(item.Label, slide.Title))
			}
		}
	}
	var visitSection func(*saga.Section)
	visitSection = func(section *saga.Section) {
		at(section.Target, section.Path, section.Title)
		for _, fragment := range section.Fragments {
			at(fragment.Target, fragment.Path, firstNonEmpty(fragment.Title, section.Title))
			for index := range fragment.Landmarks {
				at(fragment.Landmarks[index].Target, fragment.Path, firstNonEmpty(fragment.Title, section.Title))
			}
		}
		for _, child := range section.Children {
			visitSection(child)
		}
	}
	if document.Section != nil {
		visitSection(document.Section)
	}
	coverage.WalkDocumentCode(document, func(target string, files []saga.CodeFile) {
		value := at(target, "", "")
		for _, file := range files {
			if value.epic == "" {
				value.epic = applayout.EpicOfPath(file.Path)
			}
			for index := range file.References {
				value.references = append(value.references, file.Path+"#"+strconv.Itoa(index+1))
			}
		}
	})
	return result
}

// storyPlaces maps every documentation target to where a story link
// attaches: an Item's slide, since a slide explains one part of a change and
// reaches its Items with scope descendants; any other target is its own place.
func storyPlaces(document *saga.Saga) map[string]nextaction.Place {
	result := map[string]nextaction.Place{}
	for target, value := range documentTargets(document) {
		result[target] = nextaction.Place{Target: target, Title: value.title, Epic: value.epic}
	}
	for _, deck := range document.Decks {
		epic := applayout.EpicOfPath(deck.Path)
		for _, slide := range deck.Slides {
			place := nextaction.Place{Target: slide.Target, Title: firstNonEmpty(slide.Title, deck.Title), Epic: epic, Slide: true}
			result[slide.Target] = place
			for _, item := range slide.Items {
				result[item.Target] = place
			}
		}
	}
	return result
}

// coverageInputs gathers the facts the coverage report reads from one status
// document. Scope is the change when comparing, the app when observing, and
// epic narrows either.
func coverageInputs(document *saga.Saga, changes gitdiff.ChangeSet, report coverage.Report, living livingapp.Status, layers *changeview.Layers, epic string) areas.Inputs {
	in := areas.Inputs{
		Scope:  areas.Scope{Kind: areas.ScopeApp, Head: changes.Head, Epic: epic},
		Owners: map[string][]string{}, TargetEpic: map[string]string{}, TargetTitle: map[string]string{},
		TargetStories: living.Chain.TargetStories, StoryDesign: living.Chain.StoryDesign, CriterionTests: living.Chain.CriterionTests,
		ActivePersonas: map[string]bool{}, DesignExcluded: map[string]bool{}, QualityExcluded: map[string]bool{},
	}
	targets := documentTargets(document)
	for target, value := range targets {
		in.TargetEpic[target], in.TargetTitle[target] = value.epic, value.title
		if len(value.references) > 0 {
			in.CodeTargets = append(in.CodeTargets, target)
		}
	}
	if changes.Mode == gitdiff.ModeCompare {
		in.Scope.Kind, in.Scope.Against = areas.ScopeChange, changes.Base
		in.Atoms = changes.Atoms
		for key, owners := range report.Ownership {
			for _, owner := range owners {
				in.Owners[key] = append(in.Owners[key], owner.Target)
			}
		}
		// Current test-code evidence accounts for the test lines it selects.
		keyOf := map[string]string{}
		for _, atom := range changes.Atoms {
			keyOf[atom.Ref] = atom.Key
		}
		for _, owned := range living.ChangedSource.TestOwned {
			in.Owners[keyOf[owned.Atom]] = append(in.Owners[keyOf[owned.Atom]], owned.TestCase)
		}
		in.InScope = map[string]bool{}
		if layers != nil {
			for _, change := range layers.Changed {
				if change.Kind == changeview.KindStory {
					in.InScope[change.URN] = true
				}
			}
			for _, affected := range layers.Affected {
				if affected.Kind == changeview.KindStory {
					in.InScope[affected.URN] = true
				}
			}
		}
	}
	storyEpic := map[string]string{}
	for _, story := range living.Stories {
		storyEpic[story.Story] = story.Epic
		value := areas.Story{
			URN: story.Story, Title: story.Title, Epic: story.Epic, Personas: story.Personas,
			Active: story.State != string(requirements.StateRetired) && story.State != string(requirements.StateDeferred),
		}
		for _, criterion := range story.Criteria {
			value.Criteria = append(value.Criteria, areas.Criterion{URN: criterion.Criterion, Statement: criterion.Statement})
		}
		in.Stories = append(in.Stories, value)
	}
	for _, persona := range living.Personas {
		if persona.State == "active" {
			in.ActivePersonas[persona.Persona] = true
		}
	}
	for _, criterion := range living.Axes.Criteria {
		for _, cell := range criterion.Axes {
			if cell.Resolution != coverage.ResolutionExcluded {
				continue
			}
			if cell.Axis == coverage.AxisQuality {
				in.QualityExcluded[criterion.Criterion] = true
			}
			for _, axis := range coverage.DesignAxes() {
				if cell.Axis == axis {
					in.DesignExcluded[criterion.Criterion] = true
				}
			}
		}
	}
	in.Examined, in.Problems = healthRecords(targets, living, storyEpic)
	return in
}

// healthRecords lists the existing records health examines and those that
// went stale or broke: code references, relations, stories, test cases, and
// terms. A record's epic is the epic of the story it concerns.
func healthRecords(targets map[string]*codeTarget, living livingapp.Status, storyEpic map[string]string) ([]areas.Record, []areas.Problem) {
	examined := []areas.Record{}
	problems := []areas.Problem{}
	epicOf := func(resources ...string) string {
		for _, resource := range resources {
			story := resource
			if index := strings.Index(resource, ":criterion:"); index >= 0 {
				story = resource[:index]
			}
			if epic := storyEpic[story]; epic != "" {
				return epic
			}
		}
		return ""
	}
	for _, value := range targets {
		for _, reference := range value.references {
			examined = append(examined, areas.Record{Resource: reference, Kind: "code_reference", Epic: value.epic})
		}
	}
	for relation, state := range living.Chain.Relations {
		examined = append(examined, areas.Record{Resource: relation, Kind: "relation", Epic: epicOf(state.To)})
		switch state.Currency {
		case requirements.CurrencyConflicted, requirements.CurrencyInvalid:
			problems = append(problems, areas.Problem{Resource: relation, Kind: "relation", Epic: epicOf(state.To), Reason: "relation is " + string(state.Currency)})
		}
	}
	for _, story := range living.Stories {
		examined = append(examined, areas.Record{Resource: story.Story, Kind: "story", Epic: story.Epic})
		if len(story.RevisionHeads) > 1 || len(story.LifecycleHeads) > 1 {
			problems = append(problems, areas.Problem{Resource: story.Story, Kind: "story", Epic: story.Epic, Reason: "story has competing heads; reconcile them"})
		}
	}
	for _, testCase := range living.Quality.TestCases {
		examined = append(examined, areas.Record{Resource: testCase.TestCase, Kind: "test_case", Epic: testCase.Epic})
	}
	for _, fact := range living.Quality.Facts {
		if fact.Required && fact.TestCase != "" && (fact.RunResult == "failed" || fact.RunResult == "blocked") {
			problems = append(problems, areas.Problem{Resource: fact.TestCase, Kind: "test_case", Epic: epicOf(fact.Criterion), Reason: "its current run " + fact.RunResult + " for " + fact.Criterion})
		}
	}
	for _, term := range living.Terms {
		if len(term.Code) == 0 {
			continue
		}
		examined = append(examined, areas.Record{Resource: term.Term, Kind: "term"})
		for _, code := range term.Code {
			if code.State == coderesolve.Stale {
				problems = append(problems, areas.Problem{Resource: term.Term, Kind: "term", Reason: "the code that defines it changed: " + code.Reason})
			}
		}
	}
	for _, stale := range living.Stale {
		problems = append(problems, areas.Problem{Resource: stale.Record, Kind: stale.Kind, Epic: epicOf(stale.Affects...), Reason: stale.Kind + " is stale: " + strings.Join(stale.Reasons, "; ")})
	}
	return examined, problems
}

// areaSentence says what an area counts, in the words of the report.
var areaSentence = map[areas.Name]string{
	areas.Implementation: "referenced by the implementation deck",
	areas.Stories:        "reach a story",
	areas.Personas:       "reach a persona",
	areas.Design:         "have design",
	areas.Quality:        "have a test case",
	areas.Health:         "still healthy",
}

var unitWords = map[areas.Unit][2]string{
	areas.UnitChangedLine: {"changed line", "changed lines"},
	areas.UnitCodeTarget:  {"documented code target", "documented code targets"},
	areas.UnitStory:       {"story in scope", "stories in scope"},
	areas.UnitCriterion:   {"acceptance criterion in scope", "acceptance criteria in scope"},
	areas.UnitRecord:      {"existing record", "existing records"},
}

// describeScope names the report's scope in words.
func describeScope(scope areas.Scope) string {
	text := "the whole app at " + scope.Head
	if scope.Kind == areas.ScopeChange {
		text = "the change " + scope.Against + ".." + scope.Head + " (what it changed and what it affected)"
	}
	if scope.Epic != "" {
		text += ", epic " + scope.Epic
	}
	return text
}

// printAreaLine prints one area's counts.
func printAreaLine(out io.Writer, area areas.Area) {
	words := unitWords[area.Unit]
	unit := words[1]
	if area.Total == 1 {
		unit = words[0]
	}
	fmt.Fprintf(out, "  %-15s %d/%d %s %s", area.Area, area.Covered, area.Total, unit, areaSentence[area.Area])
	if area.Note != "" {
		fmt.Fprintf(out, " (%s)", area.Note)
	}
	fmt.Fprintln(out)
}

// printCoverage prints the coverage report: every area's counts. Gaps in the
// growth areas are opportunities, never failures.
func printCoverage(out io.Writer, report areas.Report) {
	fmt.Fprintf(out, "\nCoverage of %s:\n", describeScope(report.Scope))
	for _, area := range report.All() {
		printAreaLine(out, area)
	}
	fmt.Fprintln(out, "A gap is a finding, not a failure; ask about specific areas with change-saga check --covers AREA,...")
}

// printGaps prints one area's uncovered entries.
func printGaps(out io.Writer, area areas.Area, maxItems int) {
	limit := len(area.UncoveredEntries)
	if maxItems > 0 && maxItems < limit {
		limit = maxItems
	}
	for _, entry := range area.UncoveredEntries[:limit] {
		fmt.Fprintf(out, "    %s\n", describeEntry(entry))
	}
	if limit < len(area.UncoveredEntries) {
		fmt.Fprintf(out, "    … and %d more (use --max 0 or --json)\n", len(area.UncoveredEntries)-limit)
	}
}

func describeEntry(entry areas.Entry) string {
	text := entry.Resource
	switch {
	case entry.Event != "":
		text += " (" + entry.Event + ")"
	case entry.Lines != "":
		text += ":" + entry.Lines
		if entry.Side == "old" {
			text += " (deleted)"
		}
	}
	if entry.Title != "" {
		text += " \"" + entry.Title + "\""
	}
	if entry.Epic != "" {
		text += " [epic " + entry.Epic + "]"
	}
	if entry.Reason != "" {
		text += " — " + entry.Reason
	}
	return text
}
