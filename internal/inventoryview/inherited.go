package inventoryview

import (
	"context"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// ItemSelection is one saved selection of one implementation Item.
type ItemSelection struct {
	Feature string          `json:"feature"`
	Deck    string          `json:"deck"`
	Slide   string          `json:"slide"`
	Item    string          `json:"item"`
	Pin     Pin             `json:"documentation"`
	Result  SelectionResult `json:"resolution"`
	// Attached is true when the selection contributes inherited
	// implementation-deck coverage (see InheritedReferences).
	Attached bool `json:"attached"`
}

// ItemSelections resolves every saved selection of every implementation deck
// Item, in deck order. Review deck Items never inherit coverage.
func ItemSelections(ctx context.Context, document *saga.Saga, inventory *requirements.Inventory, view string, resolver SelectionResolver) []ItemSelection {
	out := []ItemSelection{}
	if document == nil || inventory == nil {
		return out
	}
	for _, feature := range document.Features {
		for _, deck := range feature.Decks {
			for _, slide := range deck.Slides {
				for _, item := range slide.Items {
					if item.Documentation == nil {
						continue
					}
					for _, sel := range item.Selections {
						result := ResolveSelection(ctx, inventory, *item.Documentation, sel, view, resolver)
						result.Item = item.Target
						out = append(out, ItemSelection{Feature: feature.ID, Deck: deck.Target, Slide: slide.Target, Item: item.Target, Pin: *item.Documentation, Result: result})
					}
				}
			}
		}
	}
	return out
}

// InheritedReferences returns the selections that may contribute inherited
// implementation-deck coverage: structurally resolved, every pin current, and
// selected bytes current at view. It also returns every selection's
// resolution so ineligible ones stay visible. The document is not modified:
// inherited code is never presented as an authored evidence record.
func InheritedReferences(ctx context.Context, document *saga.Saga, inventory *requirements.Inventory, view string, resolver SelectionResolver) ([]coverage.InheritedReference, []ItemSelection) {
	selections := ItemSelections(ctx, document, inventory, view, resolver)
	inherited := []coverage.InheritedReference{}
	for i := range selections {
		s := &selections[i]
		if !s.Result.Eligible {
			continue
		}
		s.Attached = true
		inherited = append(inherited, coverage.InheritedReference{
			Inheritance: coverage.Inheritance{Item: s.Item, Selection: s.Result.Selection.ID, Path: append([]Pin{}, s.Result.Selection.Path...), Evidence: s.Result.Selection.Evidence},
			Reference:   s.Result.Selection.Code,
		})
	}
	return inherited, selections
}
