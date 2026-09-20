package nextaction

import (
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

const (
	appStory        = "urn:change-saga:checkout:story:wallet"
	appOrphanA      = "urn:change-saga:checkout:story:fax-a"
	appOrphanB      = "urn:change-saga:checkout:story:fax-b"
	appShopper      = "urn:change-saga:checkout:persona:shopper"
	appFaxer        = "urn:change-saga:checkout:persona:faxer"
	appFeature      = "payments"
	appOtherFeature = "legacy"
)

// appStatus holds a proposed story in one feature, an active persona no accepted
// story serves, and two stories left serving only a retired persona.
func appStatus() livingapp.Status {
	story := func(urn, feature, state string, personas ...string) livingapp.StoryStatus {
		return livingapp.StoryStatus{
			Story: urn, Feature: feature, Title: urn, State: state, Personas: personas, GatedBy: []string{},
			RevisionHeads: []string{urn + ":revision:r1"}, LifecycleHeads: []string{urn + ":event:" + state},
			CurrentRevision: urn + ":revision:r1", Criteria: []livingapp.CriterionStatus{},
		}
	}
	return livingapp.Status{
		SagaID: "checkout", SagaVersion: 5,
		Features: []livingapp.FeatureStatus{
			{Feature: "urn:change-saga:checkout:feature:" + appFeature, ID: appFeature, Stories: []string{appStory}},
			{Feature: "urn:change-saga:checkout:feature:" + appOtherFeature, ID: appOtherFeature, Stories: []string{appOrphanA, appOrphanB}},
		},
		Personas: []livingapp.PersonaStatus{
			{Persona: appShopper, ID: "shopper", Name: "Shopper", State: "active", LifecycleHead: appShopper + ":event:active",
				ServedBy: []string{}, Stories: []string{appStory}, Gap: true},
			{Persona: appFaxer, ID: "faxer", State: "retired", ServedBy: []string{appOrphanA, appOrphanB}, Stories: []string{appOrphanA, appOrphanB}},
		},
		PersonaOrphans: []livingapp.PersonaOrphans{{Personas: []string{appFaxer}, Stories: []string{appOrphanA, appOrphanB}}},
		Stories: []livingapp.StoryStatus{
			story(appStory, appFeature, "proposed", appShopper),
			story(appOrphanA, appOtherFeature, "accepted", appFaxer),
			story(appOrphanB, appOtherFeature, "accepted", appFaxer),
		},
		Axes: coverage.AxisProjection{Criteria: []coverage.CriterionCoverage{}},
		ChangedSource: livingapp.ChangedSource{
			Complete: true, Uncovered: []livingapp.UncoveredPath{}, Stale: []livingapp.StaleReference{}, TestOwned: []livingapp.TestOwned{},
		},
	}
}

func TestFeatureScopedGrowthNamesItsFeatureAndFillsTheCommand(t *testing.T) {
	report := areas.Evaluate(areas.Inputs{
		Scope:   areas.Scope{Kind: areas.ScopeApp},
		Stories: []areas.Story{{URN: appStory, Title: "Wallet", Feature: appFeature, Active: true}},
	})
	action, ok := byID(Derive(appStatus(), saga, Context{Coverage: report}))["growth:design:"+appStory]
	if !ok {
		t.Fatal("a story with no design yields a growth suggestion")
	}
	if action.Feature != appFeature || action.Category != CategoryGrowth || action.Area != AreaDesign || action.Practice == "" {
		t.Fatalf("the suggestion names the story's feature, its area, and its practice: %#v", action)
	}
	found := false
	for _, command := range commandsOf(action) {
		if command.Command != "relation add" {
			continue
		}
		found = true
		assertGrammarShape(t, command)
		if !hasArgument(command, "feature", appFeature) || !hasArgument(command, "to", appStory) {
			t.Fatalf("relation add is aimed at the story's feature: %#v", command)
		}
		if !containsArg(command.Argv, "--feature", appFeature) {
			t.Fatalf("argv carries the feature: %v", command.Argv)
		}
	}
	if !found {
		t.Fatalf("the design suggestion offers relation add: %#v", action.Question)
	}
}

func TestPersonaGapIsAnAppLevelQuestion(t *testing.T) {
	action, ok := byID(Derive(appStatus(), saga))["growth:persona:"+appShopper]
	if !ok {
		t.Fatal("an active persona no accepted story serves yields a question")
	}
	if action.Kind != KindQuestion || action.Question == nil || action.Command != nil {
		t.Fatalf("a persona gap is a question: %#v", action)
	}
	if action.Feature != "" {
		t.Fatalf("a persona concerns the app, not a feature: %q", action.Feature)
	}
	if action.Category != CategoryGrowth || action.Area != AreaPersonas {
		t.Fatalf("persona coverage is growth, never required: %#v", action)
	}
	accept := false
	for _, command := range commandsOf(action) {
		assertGrammarShape(t, command)
		if command.Command == "story set-state" && hasArgument(command, "story", appStory) {
			accept = true
			if !hasArgument(command, "feature", appFeature) {
				t.Fatalf("accepting the serving story is still aimed at its feature: %#v", command)
			}
		}
	}
	if !accept {
		t.Fatalf("the proposed story serving the persona is offered for acceptance: %#v", action.Question)
	}
}

func TestPersonaOrphanGroupIsExactlyOneAction(t *testing.T) {
	actions := Derive(appStatus(), saga)
	matches := []Action{}
	for _, action := range actions {
		if strings.HasPrefix(action.ID, "growth:retired-personas:") {
			matches = append(matches, action)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("one persona-orphan group is one action, got %d: %#v", len(matches), matches)
	}
	action := matches[0]
	if action.Question == nil || len(action.Question.Options) != 2 {
		t.Fatalf("the group asks one retire-or-reassign question: %#v", action)
	}
	for _, option := range action.Question.Options {
		stories := map[string]bool{}
		for _, command := range option.Commands {
			assertGrammarShape(t, command)
			for _, story := range []string{appOrphanA, appOrphanB} {
				if hasArgument(command, "story", story) {
					stories[story] = true
					if !hasArgument(command, "feature", appOtherFeature) {
						t.Fatalf("%s is aimed at the story's feature: %#v", command.Command, command)
					}
				}
			}
		}
		if len(stories) != 2 {
			t.Fatalf("option %q covers both stories: %#v", option.Answer, option.Commands)
		}
	}
}

func containsArg(argv []string, flag, value string) bool {
	for index := 0; index+1 < len(argv); index++ {
		if argv[index] == flag && argv[index+1] == value {
			return true
		}
	}
	for _, arg := range argv {
		if arg == flag+"="+value {
			return true
		}
	}
	return false
}
