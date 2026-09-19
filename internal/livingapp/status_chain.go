package livingapp

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/sagaref"
)

// Chain is the persona -> story -> design -> code chain as the coverage
// report walks it. Only adjacent links are recorded, each from a current,
// active relation; every longer path is inferred by the reader. It is not part
// of the status JSON: the coverage report is.
type Chain struct {
	// TargetStories maps every documentation target (a deck, slide, Item,
	// or report target) to the stories its current addresses or explains
	// relations reach. A deck or slide related with scope descendants passes
	// its stories to the Items it contains, since the Items own the code.
	TargetStories map[string][]string
	// StoryDesign maps a story to the design sources that address it or one
	// of its criteria through a current relation.
	StoryDesign map[string][]string
	// CriterionTests maps a criterion to the active test cases that verify it
	// through a current relation.
	CriterionTests map[string][]string
	// Relations is every active relation with its currency and target.
	Relations map[string]RelationState
}

// RelationState is one active relation's currency and the record it targets.
type RelationState struct {
	Currency requirements.Currency
	To       string
}

func (a *assembler) chain() Chain {
	result := Chain{
		TargetStories: map[string][]string{}, StoryDesign: map[string][]string{},
		CriterionTests: map[string][]string{}, Relations: map[string]RelationState{},
	}
	retiredTests := map[string]bool{}
	for urn, testCase := range a.testCases {
		if testCase.CurrentLifecycle != nil && testCase.CurrentLifecycle.State == quality.StateRetired {
			retiredTests[urn] = true
		}
	}
	for _, link := range a.in.Links {
		if !link.Active {
			continue
		}
		result.Relations[link.URN] = RelationState{Currency: link.Currency, To: link.To}
		if link.Currency != requirements.CurrencyCurrent {
			continue
		}
		story := storyOf(link.To)
		if story == "" {
			continue
		}
		switch link.Type {
		case requirements.RelationAddresses, requirements.RelationExplains:
			result.TargetStories[link.From] = append(result.TargetStories[link.From], story)
			visual, isVisual := a.visual[link.From]
			if isVisual && (visual.kind == sagaref.TargetItem || link.Scope == scopeDescendants) {
				for _, item := range visual.items {
					result.TargetStories[item.Target] = append(result.TargetStories[item.Target], story)
				}
			}
			if link.Type == requirements.RelationAddresses {
				if ref, err := livingid.Parse(link.From); isVisual || err == nil && ref.Kind == livingid.KindDesign {
					result.StoryDesign[story] = append(result.StoryDesign[story], link.From)
				}
			}
		case requirements.RelationVerifies:
			if !strings.Contains(link.To, ":criterion:") || retiredTests[link.From] {
				continue
			}
			result.CriterionTests[link.To] = append(result.CriterionTests[link.To], link.From)
		}
	}
	for key, values := range result.TargetStories {
		result.TargetStories[key] = uniqueSorted(values)
	}
	for key, values := range result.StoryDesign {
		result.StoryDesign[key] = uniqueSorted(values)
	}
	for key, values := range result.CriterionTests {
		result.CriterionTests[key] = uniqueSorted(values)
	}
	return result
}

// storyOf returns the story a relation endpoint names: the story itself, or
// the story holding a criterion. Any other endpoint names no story.
func storyOf(endpoint string) string {
	ref, err := livingid.Parse(endpoint)
	if err != nil {
		return ""
	}
	switch ref.Kind {
	case livingid.KindStory:
		return endpoint
	case livingid.KindCriterion:
		return criterionStory(endpoint)
	}
	return ""
}
