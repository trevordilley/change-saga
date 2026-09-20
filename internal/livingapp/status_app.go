package livingapp

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// Story availability says whether a story's behavior has shipped and whether
// users can reach it. A story gated by a current off flag can be implemented
// but not enabled.
const (
	AvailabilityNotImplemented        = "not_implemented"
	AvailabilityImplementedNotEnabled = "implemented_not_enabled"
	AvailabilityEnabled               = "implemented_enabled"
)

// FeatureStatus lists what one durable product domain holds. It is a grouping of
// the app-wide facts, never a per-feature score.
type FeatureStatus struct {
	Feature   string   `json:"feature"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Stories   []string `json:"stories"`
	TestCases []string `json:"test_cases"`
	Decks     []string `json:"decks"`
	GatedBy   []string `json:"gated_by"`
}

// PersonaStatus is one persona and the stories that serve it. An active
// persona that no accepted story serves is a visible gap.
type PersonaStatus struct {
	Persona         string   `json:"persona"`
	ID              string   `json:"id"`
	Name            string   `json:"name,omitempty"`
	State           string   `json:"state"`
	RevisionHeads   []string `json:"revision_heads"`
	LifecycleHeads  []string `json:"lifecycle_heads"`
	CurrentRevision string   `json:"current_revision,omitempty"`
	LifecycleHead   string   `json:"lifecycle_head,omitempty"`
	// ServedBy lists the accepted stories whose current revision serves it.
	ServedBy []string `json:"served_by"`
	// Stories lists every story whose current revision serves it.
	Stories []string `json:"stories"`
	Gap     bool     `json:"gap"`
}

// PersonaOrphans is the one question a persona retirement raises: the stories
// that served only retired personas must be retired or reassigned.
type PersonaOrphans struct {
	Personas []string `json:"personas"`
	Stories  []string `json:"stories"`
}

// PersonaCoverage is a report, not a gate: each fact says whether an active
// persona is served by an accepted story, or names the stories left serving
// only retired personas. Nothing here changes any readiness gate.
type PersonaCoverage struct {
	Blocking bool             `json:"blocking"`
	Facts    []readiness.Fact `json:"facts"`
}

// FlagStatus is one feature flag and the stories it currently gates.
type FlagStatus struct {
	Flag            string   `json:"flag"`
	ID              string   `json:"id"`
	State           string   `json:"state"`
	Description     string   `json:"description,omitempty"`
	Targets         []string `json:"targets"`
	Stories         []string `json:"stories"`
	RevisionHeads   []string `json:"revision_heads"`
	LifecycleHeads  []string `json:"lifecycle_heads"`
	CurrentRevision string   `json:"current_revision,omitempty"`
}

// appProjection fills the app-level sections of the status and returns the
// persona facts readiness needs.
func (a *assembler) appProjection(status *Status) ([]readiness.Persona, []readiness.PersonaOrphans) {
	sagaID := a.in.SagaID
	storyByURN := map[string]*StoryStatus{}
	for index := range status.Stories {
		storyByURN[status.Stories[index].Story] = &status.Stories[index]
	}

	// Availability: implemented when every current criterion's
	// implementation cell is covered; enabled unless a current off flag gates
	// the story or its feature.
	implemented := map[string]bool{}
	missing := map[string]bool{}
	for _, criterion := range status.Axes.Criteria {
		cell, ok := criterion.Axis(coverage.AxisImplementation)
		covered := ok && (cell.State == coverage.StateCoveredDirect || cell.State == coverage.StateCoveredBroad)
		story := criterionStory(criterion.Criterion)
		if covered && !missing[story] {
			implemented[story] = true
		} else {
			missing[story], implemented[story] = true, false
		}
	}
	for index := range status.Stories {
		row := &status.Stories[index]
		row.GatedBy = copyStrings(a.in.Gates.Off[row.ID])
		switch {
		case !implemented[row.Story]:
			row.Availability = AvailabilityNotImplemented
		case len(row.GatedBy) > 0:
			row.Availability = AvailabilityImplementedNotEnabled
		default:
			row.Availability = AvailabilityEnabled
		}
	}

	// Personas.
	personaState := map[string]string{}
	status.Personas = []PersonaStatus{}
	for _, persona := range a.in.Personas {
		urn, _ := requirements.PersonaURN(sagaID, persona.Identity.ID)
		row := PersonaStatus{
			Persona: urn, ID: persona.Identity.ID, State: "conflicted",
			RevisionHeads: copyStrings(persona.RevisionHeads), LifecycleHeads: copyStrings(persona.LifecycleHeads),
			ServedBy: []string{}, Stories: []string{},
		}
		if persona.CurrentRevision != nil {
			row.Name = persona.CurrentRevision.Name
			row.CurrentRevision, _ = requirements.PersonaRevisionURN(sagaID, persona.Identity.ID, persona.CurrentRevision.ID)
		}
		if persona.CurrentLifecycle != nil {
			row.State = string(persona.CurrentLifecycle.State)
			row.LifecycleHead, _ = requirements.PersonaEventURN(sagaID, persona.Identity.ID, persona.CurrentLifecycle.ID)
		}
		personaState[urn] = row.State
		status.Personas = append(status.Personas, row)
	}
	personaIndex := map[string]int{}
	for index, row := range status.Personas {
		personaIndex[row.Persona] = index
	}
	orphanGroups := map[string]*PersonaOrphans{}
	for _, story := range status.Stories {
		for _, persona := range story.Personas {
			if index, ok := personaIndex[persona]; ok {
				status.Personas[index].Stories = append(status.Personas[index].Stories, story.Story)
				if story.State == string(requirements.StateAccepted) {
					status.Personas[index].ServedBy = append(status.Personas[index].ServedBy, story.Story)
				}
			}
		}
		if story.State == string(requirements.StateRetired) || story.State == string(requirements.StateRejected) || len(story.Personas) == 0 {
			continue
		}
		onlyRetired := true
		for _, persona := range story.Personas {
			if personaState[persona] != string(requirements.PersonaRetired) {
				onlyRetired = false
			}
		}
		if !onlyRetired {
			continue
		}
		retired := uniqueSorted(story.Personas)
		key := strings.Join(retired, "\x00")
		if orphanGroups[key] == nil {
			orphanGroups[key] = &PersonaOrphans{Personas: retired, Stories: []string{}}
		}
		orphanGroups[key].Stories = append(orphanGroups[key].Stories, story.Story)
	}
	personas := make([]readiness.Persona, 0, len(status.Personas))
	for index := range status.Personas {
		row := &status.Personas[index]
		row.Gap = row.State == string(requirements.PersonaActive) && len(row.ServedBy) == 0
		personas = append(personas, readiness.Persona{
			URN: row.Persona, Active: row.State == string(requirements.PersonaActive), Conflicted: row.State == "conflicted",
			ServedBy: copyStrings(row.ServedBy),
		})
	}
	status.PersonaOrphans = []PersonaOrphans{}
	for _, key := range sortedKeys(orphanGroups) {
		group := orphanGroups[key]
		sort.Strings(group.Stories)
		status.PersonaOrphans = append(status.PersonaOrphans, *group)
	}
	orphans := make([]readiness.PersonaOrphans, 0, len(status.PersonaOrphans))
	for _, group := range status.PersonaOrphans {
		orphans = append(orphans, readiness.PersonaOrphans{Personas: copyStrings(group.Personas), Stories: copyStrings(group.Stories)})
	}

	// Flags.
	status.Flags = []FlagStatus{}
	for _, flag := range a.in.Flags {
		urn, _ := requirements.FlagURN(sagaID, flag.Identity.ID)
		row := FlagStatus{
			Flag: urn, ID: flag.Identity.ID, State: "conflicted", Targets: []string{}, Stories: []string{},
			RevisionHeads: copyStrings(flag.RevisionHeads), LifecycleHeads: copyStrings(flag.LifecycleHeads),
		}
		if flag.CurrentLifecycle != nil {
			row.State = string(flag.CurrentLifecycle.State)
		}
		if flag.CurrentRevision != nil {
			row.Description = flag.CurrentRevision.Description
			row.Targets = copyStrings(flag.CurrentRevision.Targets)
			row.CurrentRevision, _ = requirements.FlagRevisionURN(sagaID, flag.Identity.ID, flag.CurrentRevision.ID)
		}
		for _, story := range status.Stories {
			if contains(a.in.Gates.Off[story.ID], urn) || contains(a.in.Gates.On[story.ID], urn) {
				row.Stories = append(row.Stories, story.Story)
			}
		}
		status.Flags = append(status.Flags, row)
	}

	// Features group the app-wide facts by domain.
	status.Features = []FeatureStatus{}
	for _, feature := range applayout.InCreationOrder(a.in.Features) {
		row := FeatureStatus{
			Feature: applayout.FeatureURN(sagaID, feature.ID), ID: feature.ID, Title: feature.Title,
			Stories: []string{}, TestCases: []string{}, Decks: []string{}, GatedBy: []string{},
		}
		for _, story := range status.Stories {
			if story.Feature == feature.ID {
				row.Stories = append(row.Stories, story.Story)
			}
		}
		for _, testCase := range a.in.Quality.TestCases {
			if testCase.Feature == feature.ID {
				row.TestCases = append(row.TestCases, testCaseURN(sagaID, testCase.Identity.ID))
			}
		}
		for _, deck := range a.in.Decks {
			if applayout.FeatureOfPath(deck.Path) == feature.ID {
				row.Decks = append(row.Decks, deck.Target)
			}
		}
		// Like a story, a feature is gated by a current off (or conflicted) flag;
		// an on flag enables it and a retired flag gates nothing.
		for _, flag := range status.Flags {
			if flag.State != string(requirements.FlagRetired) && flag.State != string(requirements.FlagOn) && contains(flag.Targets, row.Feature) {
				row.GatedBy = append(row.GatedBy, flag.Flag)
			}
		}
		status.Features = append(status.Features, row)
	}
	return personas, orphans
}

// criterionStory returns the story URN of a criterion URN.
func criterionStory(criterion string) string {
	if index := strings.Index(criterion, ":criterion:"); index >= 0 {
		return criterion[:index]
	}
	return criterion
}
