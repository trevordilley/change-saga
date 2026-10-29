package livingapp

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// danglingReferences finds the current records that still point at a removed
// story, criterion, persona, feature, flag, or term. Removing a record deletes
// the links that exist only to point at it; slides, test cases, terms, flags,
// and record Items keep their own content, so the change shows up here as
// coverage they no longer give, with the command that adjusts each one.
func danglingReferences(in StatusInputs) []DanglingReference {
	exists := map[string]bool{}
	for _, story := range in.Stories {
		urn := "urn:change-saga:" + in.SagaID + ":story:" + story.Identity.ID
		exists[urn] = true
		if story.CurrentRevision != nil {
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				exists[urn+":criterion:"+criterion.ID] = true
			}
		}
	}
	for _, citation := range in.Citations {
		urn, _ := livingid.Citation(in.SagaID, citation.ID)
		exists[urn] = true
	}
	for _, persona := range in.Personas {
		urn, _ := requirements.PersonaURN(in.SagaID, persona.Identity.ID)
		exists[urn] = true
	}
	for _, feature := range in.Features {
		exists[applayout.FeatureURN(in.SagaID, feature.ID)] = true
	}
	for _, flag := range in.Flags {
		urn, _ := requirements.FlagURN(in.SagaID, flag.Identity.ID)
		exists[urn] = true
	}
	for _, term := range in.Terms {
		urn, _ := requirements.TermURN(in.SagaID, term.Identity.ID)
		exists[urn] = true
	}
	missing := func(values ...string) []string {
		var result []string
		for _, value := range values {
			if value != "" && !exists[value] {
				result = append(result, value)
			}
		}
		return uniqueSorted(result)
	}

	result := []DanglingReference{}
	add := func(value DanglingReference) {
		if len(value.Missing) > 0 {
			result = append(result, value)
		}
	}
	for _, story := range in.Stories {
		if story.CurrentRevision == nil {
			continue
		}
		urn, _ := livingid.Story(in.SagaID, story.Identity.ID)
		add(DanglingReference{Record: urn, Kind: "story", Missing: missing(append(append([]string{}, story.CurrentRevision.Personas...), story.CurrentRevision.Citations...)...),
			Fix: "change-saga story revise --story " + urn + " without the removed personas or citations"})
	}
	for _, term := range in.Terms {
		if term.CurrentRevision == nil {
			continue
		}
		urn, _ := requirements.TermURN(in.SagaID, term.Identity.ID)
		add(DanglingReference{Record: urn, Kind: "term", Missing: missing(append(append([]string{}, term.CurrentRevision.Stories...), term.CurrentRevision.Records...)...),
			Fix: "change-saga term revise --term " + urn + " without the removed links"})
	}
	for _, flag := range in.Flags {
		if flag.CurrentRevision == nil {
			continue
		}
		urn, _ := requirements.FlagURN(in.SagaID, flag.Identity.ID)
		add(DanglingReference{Record: urn, Kind: "flag", Missing: missing(flag.CurrentRevision.Targets...),
			Fix: "change-saga flag revise --flag " + urn + " to gate what remains, or flag set-state to retire it"})
	}
	for _, testCase := range in.Quality.TestCases {
		if testCase.CurrentRevision == nil {
			continue
		}
		var criteria []string
		for _, step := range testCase.CurrentRevision.Steps {
			criteria = append(criteria, step.Criteria...)
		}
		urn, _ := qualityid.TestCase(in.SagaID, testCase.Identity.ID)
		add(DanglingReference{Record: urn, Kind: "test_case", Missing: missing(criteria...),
			Fix: "change-saga quality test-case revise to verify a current criterion, or set-state retired"})
	}
	for _, feature := range in.Features {
		if feature.Plan.Augments == "" {
			continue
		}
		add(DanglingReference{Record: applayout.FeatureURN(in.SagaID, feature.ID), Kind: "feature", Missing: missing(feature.Plan.Augments),
			Fix: "change-saga feature set-plan --feature " + feature.ID + " --augments EXISTING_FEATURE, or --augments '' to clear it"})
	}
	for _, deck := range in.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				var criteria []string
				for _, link := range item.CriterionLinks {
					criteria = append(criteria, link.Criterion)
				}
				add(DanglingReference{Record: item.Target, Kind: "slide_item", Path: item.Path, Missing: missing(criteria...),
					Fix: "change-saga apply-slide --print-current " + slide.Target + ", then relink the Item to a current criterion or remove it"})
			}
		}
	}
	for _, deck := range in.RecordDecks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				if !recordReference(in.SagaID, item.Record) {
					continue
				}
				add(DanglingReference{Record: item.Target, Kind: deck.Role + "_item", Path: item.Path, Missing: missing(item.Record),
					Fix: "change-saga revise-item --item " + item.Target + " --record CURRENT_URN, or remove-item"})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Record < result[j].Record })
	return result
}

// recordReference reports a record URN this check can resolve: a story,
// persona, or feature.
func recordReference(sagaID, value string) bool {
	prefix := "urn:change-saga:" + sagaID + ":"
	for _, kind := range []string{"story:", "persona:", "feature:"} {
		if strings.HasPrefix(value, prefix+kind) {
			return true
		}
	}
	return false
}

// DanglingReferences lists the current records that point at something that
// no longer exists, as if the named records were already gone. A removal
// previews with them named, and reports with none.
func DanglingReferences(in StatusInputs, removed ...string) []DanglingReference {
	if len(removed) == 0 {
		return danglingReferences(in)
	}
	gone := map[string]bool{}
	for _, urn := range removed {
		gone[urn] = true
	}
	urn := func(kind, id string) string { return "urn:change-saga:" + in.SagaID + ":" + kind + ":" + id }
	in.Stories = keep(in.Stories, func(value requirements.Story) bool { return !gone[urn("story", value.Identity.ID)] })
	in.Personas = keep(in.Personas, func(value requirements.Persona) bool { return !gone[urn("persona", value.Identity.ID)] })
	in.Flags = keep(in.Flags, func(value requirements.Flag) bool { return !gone[urn("flag", value.Identity.ID)] })
	in.Terms = keep(in.Terms, func(value requirements.Term) bool { return !gone[urn("term", value.Identity.ID)] })
	in.Citations = keep(in.Citations, func(value requirements.Citation) bool { return !gone[urn("citation", value.ID)] })
	in.Features = keep(in.Features, func(value applayout.Feature) bool { return !gone[urn("feature", value.ID)] })
	return danglingReferences(in)
}

func keep[T any](values []T, wanted func(T) bool) []T {
	result := make([]T, 0, len(values))
	for _, value := range values {
		if wanted(value) {
			result = append(result, value)
		}
	}
	return result
}
