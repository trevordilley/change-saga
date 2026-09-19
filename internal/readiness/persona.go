package readiness

import (
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/coverage"
)

// Fact is one concrete, checkable observation. Facts report relations, paths,
// and gaps; they are never reduced to one opaque number.
type Fact struct {
	Code      string        `json:"code"`
	Satisfied bool          `json:"satisfied"`
	Resource  string        `json:"resource,omitempty"`
	Axis      coverage.Axis `json:"axis,omitempty"`
	Detail    string        `json:"detail,omitempty"`
	Paths     [][]string    `json:"paths,omitempty"`
}

// PersonaCoverage reports the persona -> story link of the persona -> story ->
// design -> code chain. It is a report, never a gate: an app with no personas,
// or stories that name none, is exactly as ready as it would be without the
// persona feature. Each active persona is served by an accepted story or is an
// unsatisfied fact; the stories left serving only retired personas are one
// unsatisfied fact per group, since they are one question for the author.
func PersonaCoverage(personas []Persona, orphans []PersonaOrphans) []Fact {
	facts := []Fact{}
	personas = append([]Persona(nil), personas...)
	sort.Slice(personas, func(i, j int) bool { return personas[i].URN < personas[j].URN })
	for _, persona := range personas {
		if persona.Conflicted {
			facts = append(facts, Fact{Code: "persona_lifecycle_head", Resource: persona.URN, Detail: "persona has competing lifecycle heads"})
			continue
		}
		if !persona.Active {
			continue
		}
		facts = append(facts, Fact{
			Code: "active_persona_served", Resource: persona.URN, Satisfied: len(persona.ServedBy) > 0,
			Detail: countDetail(len(persona.ServedBy), "accepted story serves it", "accepted stories serve it"),
		})
	}
	for _, group := range orphans {
		facts = append(facts, Fact{
			Code: "retired_persona_stories_decided", Resource: strings.Join(group.Personas, ", "),
			Detail: countDetail(len(group.Stories), "story serves", "stories serve") + " only retired personas: " + strings.Join(group.Stories, ", ") + "; retire them or reassign them",
		})
	}
	return facts
}

// Persona is one persona's coverage input. An active persona must be served
// by at least one accepted story; a retired persona needs none.
type Persona struct {
	URN        string
	Active     bool
	Conflicted bool
	ServedBy   []string
}

// PersonaOrphans is the stories that serve only retired personas. They are one
// question for the author, retire them or reassign them, not one error each.
type PersonaOrphans struct {
	Personas []string
	Stories  []string
}

func countDetail(count int, singular, plural string) string {
	word := plural
	if count == 1 {
		word = singular
	}
	return strconv.Itoa(count) + " " + word
}
