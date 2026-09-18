package readiness

import (
	"strings"
	"testing"
)

const (
	shopperURN = "urn:change-saga:s:persona:shopper"
	clerkURN   = "urn:change-saga:s:persona:clerk"
	legacyURN  = "urn:change-saga:s:persona:legacy"
)

func factsWithCode(value Gate, code string) []Fact {
	facts := []Fact{}
	for _, fact := range value.Facts {
		if fact.Code == code {
			facts = append(facts, fact)
		}
	}
	return facts
}

func TestActivePersonaWithNoAcceptedStoryBlocksRequirements(t *testing.T) {
	inputs := coveredInputs()
	inputs.Personas = []Persona{{URN: shopperURN, Active: true}}
	requirements := gate(t, EvaluateGates(inputs), GateRequirementsReady)
	if requirements.Status != StatusBlocked {
		t.Fatalf("an unserved active persona must block requirements_ready: %s", requirements.Status)
	}
	facts := factsWithCode(requirements, "active_persona_served")
	if len(facts) != 1 || facts[0].Satisfied || facts[0].Resource != shopperURN {
		t.Fatalf("active_persona_served facts = %+v", facts)
	}
	if codes := blockerCodes(requirements); len(codes) != 1 || codes[0] != "active_persona_served" {
		t.Fatalf("the unserved persona must be the only blocker: %v", codes)
	}

	inputs.Personas[0].ServedBy = []string{storyURN}
	requirements = gate(t, EvaluateGates(inputs), GateRequirementsReady)
	if requirements.Status != StatusReady {
		t.Fatalf("a served persona satisfies requirements_ready: %s blockers %v", requirements.Status, blockerCodes(requirements))
	}
	facts = factsWithCode(requirements, "active_persona_served")
	if len(facts) != 1 || !facts[0].Satisfied {
		t.Fatalf("served persona fact = %+v", facts)
	}
}

func TestRetiredPersonaNeedsNoStory(t *testing.T) {
	inputs := coveredInputs()
	inputs.Personas = []Persona{{URN: legacyURN, Active: false}}
	requirements := gate(t, EvaluateGates(inputs), GateRequirementsReady)
	if requirements.Status != StatusReady {
		t.Fatalf("a retired persona must not block: %s blockers %v", requirements.Status, blockerCodes(requirements))
	}
	if facts := factsWithCode(requirements, "active_persona_served"); len(facts) != 0 {
		t.Fatalf("a retired persona reports no served fact: %+v", facts)
	}
}

func TestPersonaOrphansAreOneFactRegardlessOfStoryCount(t *testing.T) {
	inputs := coveredInputs()
	stories := []string{
		"urn:change-saga:s:story:a", "urn:change-saga:s:story:b", "urn:change-saga:s:story:c",
	}
	inputs.Personas = []Persona{{URN: legacyURN}, {URN: clerkURN}}
	inputs.PersonaOrphans = []PersonaOrphans{{Personas: []string{clerkURN, legacyURN}, Stories: stories}}
	requirements := gate(t, EvaluateGates(inputs), GateRequirementsReady)
	if requirements.Status != StatusBlocked {
		t.Fatalf("stories serving only retired personas must block: %s", requirements.Status)
	}
	facts := factsWithCode(requirements, "retired_persona_stories_decided")
	if len(facts) != 1 || facts[0].Satisfied {
		t.Fatalf("a persona-orphan group must be exactly one unsatisfied fact: %+v", facts)
	}
	for _, story := range stories {
		if !strings.Contains(facts[0].Detail, story) {
			t.Errorf("fact detail %q does not name %s", facts[0].Detail, story)
		}
	}
	blockers := 0
	for _, code := range blockerCodes(requirements) {
		if code == "retired_persona_stories_decided" {
			blockers++
		}
	}
	if blockers != 1 {
		t.Fatalf("persona orphans must be one blocker, got %d: %v", blockers, blockerCodes(requirements))
	}
}
