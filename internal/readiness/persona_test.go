package readiness

import (
	"strings"
	"testing"
)

const (
	shopperURN = "urn:change-saga:s:persona:shopper"
	clerkURN   = "urn:change-saga:s:persona:clerk"
	legacyURN  = "urn:change-saga:s:persona:legacy"
	storyURN   = "urn:change-saga:s:story:checkout"
)

func factsWithCode(facts []Fact, code string) []Fact {
	result := []Fact{}
	for _, fact := range facts {
		if fact.Code == code {
			result = append(result, fact)
		}
	}
	return result
}

func TestActivePersonaWithNoAcceptedStoryIsAnUnsatisfiedFact(t *testing.T) {
	facts := factsWithCode(PersonaCoverage([]Persona{{URN: shopperURN, Active: true}}, nil), "active_persona_served")
	if len(facts) != 1 || facts[0].Satisfied || facts[0].Resource != shopperURN {
		t.Fatalf("active_persona_served facts = %+v", facts)
	}
	facts = factsWithCode(PersonaCoverage([]Persona{{URN: shopperURN, Active: true, ServedBy: []string{storyURN}}}, nil), "active_persona_served")
	if len(facts) != 1 || !facts[0].Satisfied {
		t.Fatalf("served persona fact = %+v", facts)
	}
}

func TestRetiredPersonaNeedsNoStory(t *testing.T) {
	if facts := PersonaCoverage([]Persona{{URN: legacyURN, Active: false}}, nil); len(facts) != 0 {
		t.Fatalf("a retired persona reports no served fact: %+v", facts)
	}
}

func TestPersonaOrphansAreOneFactRegardlessOfStoryCount(t *testing.T) {
	stories := []string{"urn:change-saga:s:story:a", "urn:change-saga:s:story:b", "urn:change-saga:s:story:c"}
	facts := factsWithCode(PersonaCoverage([]Persona{{URN: legacyURN}, {URN: clerkURN}},
		[]PersonaOrphans{{Personas: []string{clerkURN, legacyURN}, Stories: stories}}), "retired_persona_stories_decided")
	if len(facts) != 1 || facts[0].Satisfied {
		t.Fatalf("a persona-orphan group must be exactly one unsatisfied fact: %+v", facts)
	}
	for _, story := range stories {
		if !strings.Contains(facts[0].Detail, story) {
			t.Errorf("fact detail %q does not name %s", facts[0].Detail, story)
		}
	}
}
