package nextaction

import (
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/livingapp"
)

const (
	appStory     = "urn:change-saga:checkout:story:wallet"
	appOrphanA   = "urn:change-saga:checkout:story:fax-a"
	appOrphanB   = "urn:change-saga:checkout:story:fax-b"
	appShopper   = "urn:change-saga:checkout:persona:shopper"
	appFaxer     = "urn:change-saga:checkout:persona:faxer"
	appEpic      = "payments"
	appOtherEpic = "legacy"
)

// appStatus holds a proposed story in one epic, an active persona no accepted
// story serves, and two stories left serving only a retired persona.
func appStatus() livingapp.Status {
	story := func(urn, epic, state string, personas ...string) livingapp.StoryStatus {
		return livingapp.StoryStatus{
			Story: urn, Epic: epic, Title: urn, State: state, Personas: personas, GatedBy: []string{},
			RevisionHeads: []string{urn + ":revision:r1"}, LifecycleHeads: []string{urn + ":event:" + state},
			CurrentRevision: urn + ":revision:r1", Criteria: []livingapp.CriterionStatus{},
		}
	}
	return livingapp.Status{
		SagaID: "checkout", SagaVersion: 5,
		Epics: []livingapp.EpicStatus{
			{Epic: "urn:change-saga:checkout:epic:" + appEpic, ID: appEpic, Stories: []string{appStory}},
			{Epic: "urn:change-saga:checkout:epic:" + appOtherEpic, ID: appOtherEpic, Stories: []string{appOrphanA, appOrphanB}},
		},
		Personas: []livingapp.PersonaStatus{
			{Persona: appShopper, ID: "shopper", Name: "Shopper", State: "active", LifecycleHead: appShopper + ":event:active",
				ServedBy: []string{}, Stories: []string{appStory}, Gap: true},
			{Persona: appFaxer, ID: "faxer", State: "retired", ServedBy: []string{appOrphanA, appOrphanB}, Stories: []string{appOrphanA, appOrphanB}},
		},
		PersonaOrphans: []livingapp.PersonaOrphans{{Personas: []string{appFaxer}, Stories: []string{appOrphanA, appOrphanB}}},
		Stories: []livingapp.StoryStatus{
			story(appStory, appEpic, "proposed", appShopper),
			story(appOrphanA, appOtherEpic, "accepted", appFaxer),
			story(appOrphanB, appOtherEpic, "accepted", appFaxer),
		},
		Axes: coverage.AxisProjection{Criteria: []coverage.CriterionCoverage{}},
		ChangedSource: livingapp.ChangedSource{
			Complete: true, Uncovered: []livingapp.UncoveredPath{}, Orphans: []livingapp.OrphanRef{}, TestOwned: []livingapp.TestOwned{},
		},
	}
}

func TestEpicScopedStoryActionNamesItsEpicAndFillsTheCommand(t *testing.T) {
	action, ok := byID(Derive(appStatus(), saga))["requirements:accept:"+appStory]
	if !ok {
		t.Fatal("a proposed story yields an accept question")
	}
	if action.Epic != appEpic {
		t.Fatalf("the action names the story's epic: %q", action.Epic)
	}
	found := false
	for _, command := range commandsOf(action) {
		if command.Command != "story set-state" {
			continue
		}
		found = true
		assertGrammarShape(t, command)
		if !hasArgument(command, "epic", appEpic) || !hasArgument(command, "story", appStory) {
			t.Fatalf("story set-state is aimed at the story's epic: %#v", command)
		}
		if !containsArg(command.Argv, "--epic", appEpic) {
			t.Fatalf("argv carries the epic: %v", command.Argv)
		}
	}
	if !found {
		t.Fatalf("the accept question offers story set-state: %#v", action.Question)
	}
}

func TestPersonaGapIsAnAppLevelQuestion(t *testing.T) {
	action, ok := byID(Derive(appStatus(), saga))["requirements:persona:"+appShopper]
	if !ok {
		t.Fatal("an active persona no accepted story serves yields a question")
	}
	if action.Kind != KindQuestion || action.Question == nil || action.Command != nil {
		t.Fatalf("a persona gap is a question: %#v", action)
	}
	if action.Epic != "" {
		t.Fatalf("a persona concerns the app, not an epic: %q", action.Epic)
	}
	if len(action.Gates) != 0 {
		t.Fatalf("persona coverage gates nothing: %v", action.Gates)
	}
	accept := false
	for _, command := range commandsOf(action) {
		assertGrammarShape(t, command)
		if command.Command == "story set-state" && hasArgument(command, "story", appStory) {
			accept = true
			if !hasArgument(command, "epic", appEpic) {
				t.Fatalf("accepting the serving story is still aimed at its epic: %#v", command)
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
		if strings.HasPrefix(action.ID, "requirements:retired-personas:") {
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
					if !hasArgument(command, "epic", appOtherEpic) {
						t.Fatalf("%s is aimed at the story's epic: %#v", command.Command, command)
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
