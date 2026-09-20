package nextaction

import (
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

func growthOf(actions []Action) []Action {
	result := []Action{}
	for _, action := range actions {
		if action.Category == CategoryGrowth {
			result = append(result, action)
		}
	}
	return result
}

// Documenting existing code starts with the overview: observing a fresh app,
// its pitch, description, and first term are growth suggestions, first in
// line, each with its practice and the one command that acts on it.
func TestObservingAFreshAppSuggestsTheOverviewAndTermsFirst(t *testing.T) {
	status := appStatus()
	status.Overview = livingapp.OverviewStatus{Name: "Checkout", Gaps: []string{"pitch", "description", "terms"}}
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeApp},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true}},
	})
	growth := growthOf(Derive(status, saga, Context{Coverage: report}))
	want := []struct{ id, command string }{
		{"growth:overview:pitch", "overview set-pitch"},
		{"growth:overview:description", "overview set-description"},
		{"growth:terms:first", "term add"},
	}
	if len(growth) < len(want) {
		t.Fatalf("growth = %#v", growth)
	}
	for index, expected := range want {
		action := growth[index]
		if action.ID != expected.id || action.Kind != KindCommand || action.Command == nil || action.Command.Command != expected.command || action.Practice == "" {
			t.Fatalf("growth %d = %#v, want %s with %s", index, action, expected.id, expected.command)
		}
		assertGrammarShape(t, *action.Command)
	}

	// Once written, the parts are no longer suggested.
	status.Overview = livingapp.OverviewStatus{Name: "Checkout", Pitch: &livingapp.OverviewPart{}, Description: &livingapp.OverviewPart{}, Terms: 1}
	for _, action := range Derive(status, saga, Context{Coverage: report}) {
		if strings.HasPrefix(action.ID, "growth:overview:") || action.ID == "growth:terms:first" {
			t.Fatalf("a written overview part is still suggested: %s", action.ID)
		}
	}
}

// When comparing, growth about the change comes before the overview.
func TestComparingPutsTheChangeBeforeTheOverview(t *testing.T) {
	status := appStatus()
	status.Overview = livingapp.OverviewStatus{Name: "Checkout"}
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeChange},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true}},
		InScope: map[string]bool{appStory: true},
	})
	growth := growthOf(Derive(status, saga, Context{Coverage: report}))
	design, pitch := -1, -1
	for index, action := range growth {
		switch action.ID {
		case "growth:design:" + appStory:
			design = index
		case "growth:overview:pitch":
			pitch = index
		}
	}
	if design < 0 || pitch < 0 || design > pitch {
		t.Fatalf("design for a story in the change comes before the overview: design %d, pitch %d in %v", design, pitch, growth)
	}
}

// Quality growth is one digestible suggestion per story, relating a test
// case to each untested criterion, not one per criterion.
func TestQualityGrowthIsOneSuggestionPerStory(t *testing.T) {
	status := appStatus()
	criteria := []areas.Criterion{}
	for _, id := range []string{"a", "b", "c"} {
		urn := appStory + ":criterion:" + id
		status.Stories[0].Criteria = append(status.Stories[0].Criteria, livingapp.CriterionStatus{Criterion: urn, ID: id, Statement: "Criterion " + id})
		criteria = append(criteria, areas.Criterion{URN: urn, Statement: "Criterion " + id})
	}
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeApp},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true, Criteria: criteria}},
	})
	quality := []Action{}
	for _, action := range Derive(status, saga, Context{Coverage: report}) {
		if strings.HasPrefix(action.ID, "growth:quality:") {
			quality = append(quality, action)
		}
	}
	if len(quality) != 1 || quality[0].ID != "growth:quality:"+appStory || quality[0].Resource != appStory || quality[0].Feature != appFeature {
		t.Fatalf("three untested criteria of one story are one suggestion: %#v", quality)
	}
	if !strings.Contains(quality[0].Reason, "any of the 3 acceptance criteria") {
		t.Fatalf("reason = %q", quality[0].Reason)
	}
	for _, option := range quality[0].Question.Options[:2] {
		related := map[string]bool{}
		for _, command := range option.Commands {
			assertGrammarShape(t, command)
			if command.Command == "relation add" {
				for _, argument := range command.Arguments {
					if argument.Flag == "to" {
						related[argument.Value] = true
					}
				}
			}
		}
		if len(related) != 3 {
			t.Fatalf("option %q relates every untested criterion: %#v", option.Answer, option.Commands)
		}
	}
}

// With no design to relate, the one command a design suggestion shows
// creates design; once its feature has some, relating it comes first.
func TestDesignGrowthCreatesDesignWhenItsFeatureHasNone(t *testing.T) {
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeApp},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true}},
	})
	first := func(context Context) string {
		action := byID(Derive(appStatus(), saga, context))["growth:design:"+appStory]
		command := action.Question.Options[0].Commands[0]
		assertGrammarShape(t, command)
		if !hasArgument(command, "feature", appFeature) {
			t.Fatalf("the command is aimed at the story's feature: %#v", command)
		}
		return command.Command
	}
	if got := first(Context{Coverage: report}); got != "design add-chapter" {
		t.Fatalf("with no design in the feature the first command is %q, want design add-chapter", got)
	}
	if got := first(Context{Coverage: report, DesignFeatures: map[string]bool{appFeature: true}}); got != "relation add" {
		t.Fatalf("with design in the feature the first command is %q, want relation add", got)
	}
}

// Stories that name no persona are offered a persona already named as well
// as a new one, and the revision carries every other field forward, since
// story revise writes a complete revision.
func TestPersonaGrowthOffersAPersonaYouHaveNamed(t *testing.T) {
	status := appStatus()
	status.Stories[0].Personas = []string{}
	status.Stories[0].Statement, status.Stories[0].Priority = "As a shopper I pay from my wallet", "must"
	status.Stories[0].Criteria = []livingapp.CriterionStatus{{Criterion: appStory + ":criterion:paid", ID: "paid", Statement: "The wallet is charged"}}
	status.Stories[0].Citations = []string{"urn:change-saga:checkout:citation:ticket"}
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeApp},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true}},
	})
	action := byID(Derive(status, saga, Context{Coverage: report}))["growth:persona:unnamed"]
	if action.Question == nil || !strings.Contains(action.Reason, "who gets value from them? Assign one you have named (shopper)") {
		t.Fatalf("the suggestion names the personas already defined: %#v", action)
	}
	assign := action.Question.Options[0]
	if assign.Answer != "a persona you have named" || len(assign.Commands) != 1 {
		t.Fatalf("assigning a named persona is offered first: %#v", action.Question.Options)
	}
	revise := assign.Commands[0]
	assertGrammarShape(t, revise)
	for _, want := range [][2]string{{"story", appStory}, {"title", appStory}, {"statement", "As a shopper I pay from my wallet"}, {"priority", "must"},
		{"criterion", "paid=The wallet is charged"}, {"citation", "urn:change-saga:checkout:citation:ticket"}, {"feature", appFeature}} {
		if !hasArgument(revise, want[0], want[1]) {
			t.Fatalf("the revision carries --%s %q forward: %#v", want[0], want[1], revise.Arguments)
		}
	}
	if define := action.Question.Options[1]; define.Commands[0].Command != "persona add" {
		t.Fatalf("defining a new persona is still offered: %#v", define)
	}
}

func TestPersonaGapNamesThePersona(t *testing.T) {
	action := byID(Derive(appStatus(), saga))["growth:persona:"+appShopper]
	if !strings.Contains(action.Reason, `"Shopper" (shopper)`) {
		t.Fatalf("the headline names the persona: %q", action.Reason)
	}
}

// New terminology is one question per declaration block, offering the
// domain word each identifier spells and keeping the identifier as the
// term's code reference.
func TestNewTerminologyIsOneQuestionPerDeclarationBlock(t *testing.T) {
	status := appStatus()
	at := func(path string, line int) coderef.Location {
		return coderef.Location{Commit: "0123456789abcdef0123456789abcdef01234567", Path: path, Start: line, End: line}
	}
	status.NewTerminology = []livingapp.TermSuggestion{
		{Name: "ResolutionLinked", Suggested: "linked", Container: "Resolution", Location: at("axes.go", 10)},
		{Name: "ResolutionExcluded", Suggested: "excluded", Container: "Resolution", Location: at("axes.go", 11)},
		{Name: "StateExcluded", Suggested: "excluded", Container: "AxisState", Location: at("axes.go", 20)},
	}
	terms := []Action{}
	for _, action := range Derive(status, saga) {
		if action.Area == AreaTerms {
			terms = append(terms, action)
		}
	}
	if len(terms) != 2 {
		t.Fatalf("two declaration blocks are two suggestions: %#v", terms)
	}
	block := byID(terms)["growth:term:axes.go#Resolution"]
	if block.Question == nil || len(block.Question.Options) != 3 || !strings.Contains(block.Reason, "2 members of Resolution (axes.go#L10-L11)") {
		t.Fatalf("the block asks one question with an answer per declaration: %#v", block)
	}
	define := block.Question.Options[1].Commands[0]
	assertGrammarShape(t, define)
	if !hasArgument(define, "name", "excluded") || !hasArgument(define, "id", "resolution-excluded") || !hasArgument(define, "ref", at("axes.go", 11).String()) {
		t.Fatalf("the term is the domain word, told apart from another block's by its type, referencing the identifier: %#v", define.Arguments)
	}
	if single := byID(terms)["growth:term:axes.go#AxisState"]; !strings.Contains(single.Reason, `AxisState.StateExcluded`) || !strings.Contains(single.Reason, `"excluded"`) {
		t.Fatalf("a single declaration names its identifier and suggested word: %q", single.Reason)
	}
}

// Test code reaches a story through the criterion its test case verifies, so
// story growth never offers to capture a story named after a test case.
func TestStoryGrowthSkipsTestCaseEvidence(t *testing.T) {
	report := areas.Evaluate(areas.Inputs{
		Scope:  areas.Scope{Kind: areas.ScopeChange},
		Atoms:  []gitdiff.Atom{{Kind: "line", Key: "k", Path: "refund_test.go", Side: "new", Line: 1}},
		Owners: map[string][]string{"k": {"urn:change-saga:checkout:test-case:refund"}},
	})
	for _, action := range Derive(appStatus(), saga, Context{Coverage: report}) {
		if strings.HasPrefix(action.ID, "growth:story:") {
			t.Fatalf("story growth for test-case evidence: %#v", action)
		}
	}
}

// storyGrowthContext is three headings of one fragment, each owning code no
// story explains, with the fragment's title on each: the shape dogfooding
// produced, where one command would have to answer three suggestions.
func storyGrowthContext(title string, targets ...string) (livingapp.Status, Context) {
	report := areas.Evaluate(areas.Inputs{
		Scope:       areas.Scope{Kind: areas.ScopeApp},
		CodeTargets: targets,
		TargetFeature: func() map[string]string {
			result := map[string]string{}
			for _, target := range targets {
				result[target] = appFeature
			}
			return result
		}(),
	})
	places := map[string]Place{}
	for _, target := range targets {
		places[target] = Place{Target: target, Title: title, Feature: appFeature}
	}
	return appStatus(), Context{Coverage: report, Places: places}
}

func storySuggestions(actions []Action) []Action {
	result := []Action{}
	for _, action := range actions {
		if strings.HasPrefix(action.ID, "growth:story:") {
			result = append(result, action)
		}
	}
	return result
}

// Suggestions are deduplicated by what the author would actually do. Three
// headings of one fragment ask for one story, not the same story three times:
// running the command once cannot satisfy three suggestions, and the second
// run would collide on the id it names.
func TestStoryGrowthAsksOnceForTheStoryItWouldCapture(t *testing.T) {
	const fragment = "urn:change-saga:checkout:fragment:documentation-and-review"
	status, context := storyGrowthContext("Documentation and Review",
		fragment+":landmark:documentation-side", fragment+":landmark:related-reviews", fragment+":landmark:review-side")
	growth := storySuggestions(Derive(status, saga, context))
	if len(growth) != 1 {
		reasons := []string{}
		for _, action := range growth {
			reasons = append(reasons, action.Reason)
		}
		t.Fatalf("three headings of one fragment are one suggestion, got %d:\n%s", len(growth), strings.Join(reasons, "\n"))
	}
	if !strings.Contains(growth[0].Reason, "documents 3 code targets no story explains") {
		t.Fatalf("reason = %q", growth[0].Reason)
	}
	// The one answer relates every target it named, with a distinct relation id.
	ids, related := map[string]bool{}, map[string]bool{}
	adds := 0
	for _, command := range growth[0].Question.Options[0].Commands {
		assertGrammarShape(t, command)
		switch command.Command {
		case "story add":
			adds++
		case "relation add":
			for _, argument := range command.Arguments {
				switch argument.Flag {
				case "id":
					ids[argument.Value] = true
				case "from":
					related[argument.Value] = true
				}
			}
		}
	}
	if adds != 1 || len(ids) != 3 || len(related) != 3 {
		t.Fatalf("one story and a distinct relation per target: %#v", growth[0].Question.Options[0].Commands)
	}
}

// One code target is one code target, not "1 code targets".
func TestStoryGrowthCountsOneCodeTargetInTheSingular(t *testing.T) {
	status, context := storyGrowthContext("Documentation and Review", "urn:change-saga:checkout:fragment:doc:landmark:only")
	growth := storySuggestions(Derive(status, saga, context))
	if len(growth) != 1 || !strings.Contains(growth[0].Reason, "documents 1 code target no story explains") {
		t.Fatalf("reason = %#v", growth)
	}
}

// A suggestion never offers to create a record that exists. When the story the
// title names is already written, the answer is to relate the code to it.
func TestStoryGrowthNeverOffersToCaptureAStoryThatExists(t *testing.T) {
	status, context := storyGrowthContext("wallet", "urn:change-saga:checkout:fragment:doc:landmark:only")
	growth := storySuggestions(Derive(status, saga, context))
	if len(growth) != 1 {
		t.Fatalf("growth = %#v", growth)
	}
	action := growth[0]
	if strings.Contains(action.Reason, "capture its story") {
		t.Fatalf("the story exists, so the suggestion must not name capturing it: %q", action.Reason)
	}
	for _, option := range action.Question.Options {
		for _, command := range option.Commands {
			if command.Command == "story add" {
				t.Fatalf("the suggestion offers to add a story that exists: %#v", command)
			}
		}
	}
	first := action.Question.Options[0]
	if len(first.Commands) != 1 || first.Commands[0].Command != "relation add" || !hasArgument(first.Commands[0], "to", appStory) {
		t.Fatalf("the first answer relates the code to the story that exists: %#v", first)
	}
}

// Two suggestions that would run the same commands are one suggestion: the
// author can only do that work once. This holds across every kind of growth in
// one report, not just the story gaps that first showed the duplication.
func TestNoTwoGrowthSuggestionsRunTheSameCommands(t *testing.T) {
	const fragment = "urn:change-saga:checkout:fragment:documentation-and-review"
	status := appStatus()
	criteria := []areas.Criterion{}
	for _, id := range []string{"a", "b"} {
		urn := appStory + ":criterion:" + id
		status.Stories[0].Criteria = append(status.Stories[0].Criteria, livingapp.CriterionStatus{Criterion: urn, ID: id, Statement: "Criterion " + id})
		criteria = append(criteria, areas.Criterion{URN: urn, Statement: "Criterion " + id})
	}
	targets := []string{fragment + ":landmark:documentation-side", fragment + ":landmark:related-reviews", fragment + ":landmark:review-side",
		"urn:change-saga:checkout:slide:reference:item:one", "urn:change-saga:checkout:slide:reference:item:two"}
	places := map[string]Place{}
	feature := map[string]string{}
	title := map[string]string{}
	for _, target := range targets[:3] {
		places[target] = Place{Target: target, Title: "Documentation and Review", Feature: appFeature}
		feature[target], title[target] = appFeature, "Documentation and Review"
	}
	for _, target := range targets[3:] {
		places[target] = Place{Target: target, Title: "Reference resolution", Feature: appOtherFeature, Slide: true}
		feature[target], title[target] = appOtherFeature, "Reference resolution"
	}
	report := areas.Evaluate(areas.Inputs{
		Scope: areas.Scope{Kind: areas.ScopeApp}, CodeTargets: targets, TargetFeature: feature, TargetTitle: title,
		Stories: []areas.Story{
			{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true, Criteria: criteria},
			{URN: appOrphanA, Title: "Fax A", Feature: appOtherFeature, Active: true},
		},
	})
	seen := map[string]string{}
	for _, action := range growthOf(Derive(status, saga, Context{Coverage: report, Places: places})) {
		// The headline command is the one status prints and the author runs.
		shape := ""
		switch {
		case action.Command != nil:
			shape = strings.Join(action.Command.Argv, " ")
		case action.Question != nil && len(action.Question.Options) > 0 && len(action.Question.Options[0].Commands) > 0:
			shape = strings.Join(action.Question.Options[0].Commands[0].Argv, " ")
		default:
			continue
		}
		if other, ok := seen[shape]; ok {
			t.Fatalf("%s and %s both ask the author to run:\n$ %s", other, action.ID, shape)
		}
		seen[shape] = action.ID
	}
}
