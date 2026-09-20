package nextaction

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/grammar"
)

// inFeature names the feature an action concerns and supplies it to every command
// shape the action suggests, so a shape that writes feature content is already
// aimed at the right feature.
func (b *builder) inFeature(action Action) Action {
	if action.Feature == "" {
		action.Feature = b.featureOf[action.Resource]
	}
	if action.Feature == "" {
		for resource, feature := range b.featureOf {
			if feature != "" && action.Resource != "" && strings.HasPrefix(action.Resource, resource+":") {
				action.Feature = feature
				break
			}
		}
	}
	if action.Feature == "" {
		return action
	}
	if action.Command != nil {
		action.Command = ptr(action.Command.With("feature", action.Feature))
	}
	if action.Question != nil {
		copied := *action.Question
		copied.Options = make([]Option, len(action.Question.Options))
		for index, value := range action.Question.Options {
			commands := make([]grammar.Invocation, len(value.Commands))
			for commandIndex, command := range value.Commands {
				commands[commandIndex] = command.With("feature", action.Feature)
			}
			value.Commands = commands
			copied.Options[index] = value
		}
		action.Question = &copied
	}
	return action
}

// personas asks about the persona -> story link: an active persona no
// accepted story serves is a gap, and the stories left serving only retired
// personas are one question.
func (b *builder) personas() {
	for _, persona := range b.status.Personas {
		if !persona.Gap {
			continue
		}
		name := firstNonEmptyString(persona.ID, persona.Persona)
		if persona.Name != "" {
			name = "\"" + persona.Name + "\" (" + name + ")"
		}
		options := []Option{
			option("a new story serves them", "a story that names the persona; accept it once it is in scope",
				b.invoke("story add", grammar.V("persona", persona.Persona))),
		}
		for _, story := range persona.Stories {
			row := b.stories[story]
			if row.State == "proposed" && len(row.LifecycleHeads) == 1 {
				options = append(options, option("accept "+story, "the proposed story already serves the persona",
					b.invoke("story set-state", grammar.V("story", story), grammar.V("parent", row.LifecycleHeads[0]), grammar.V("state", "accepted")).With("feature", row.Feature)))
			}
		}
		if persona.LifecycleHead != "" {
			options = append(options, option("the app no longer serves them", "retire the persona; it stops needing a story",
				b.invoke("persona set-state", grammar.V("persona", persona.Persona), grammar.V("parent", persona.LifecycleHead), grammar.V("state", "retired"))))
		}
		b.add(Action{
			ID: "growth:persona:" + persona.Persona, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaPersonas, Resource: persona.Persona,
			Reason:   "no accepted story serves the persona " + name + ", so the persona -> story link is a gap",
			Practice: practicePersonas, value: 1,
			Question: question("Which accepted story serves "+name+", or does the app no longer serve them?", NeedProductJudgment, options...),
		})
	}
	for _, group := range b.status.PersonaOrphans {
		retire := []grammar.Invocation{}
		reassign := []grammar.Invocation{}
		for _, story := range group.Stories {
			row := b.stories[story]
			if len(row.LifecycleHeads) == 1 {
				retire = append(retire, b.invoke("story set-state", grammar.V("story", story), grammar.V("parent", row.LifecycleHeads[0]), grammar.V("state", "retired")).With("feature", row.Feature))
			}
			reassign = append(reassign, b.reviseStory(row, []string{""}))
		}
		personas := strings.Join(group.Personas, ", ")
		b.add(Action{
			ID: "growth:retired-personas:" + personas, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaPersonas, Resource: personas,
			Practice: practicePersonas, value: 1,
			Reason: itoa(len(group.Stories)) + " stories serve only retired personas " + personas + ": " + strings.Join(group.Stories, ", "),
			Question: question("The stories that served only "+personas+" have no active persona. Retire them, or reassign them to the personas they serve now?", NeedProductJudgment,
				option("retire them", "each story's lifecycle ends; it stays as history", retire...),
				option("reassign them", "a complete revision of each story names the personas it serves now", reassign...)),
		})
	}
}
