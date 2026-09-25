package inventoryview

import (
	"context"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// InheritedMarker separates an Item's path from the selection it inherits
// evidence through, in the pseudo evidence-file path coverage reports.
const InheritedMarker = "#selections/"

// ItemSelection is one saved selection of one implementation Item.
type ItemSelection struct {
	Feature string          `json:"feature"`
	Deck    string          `json:"deck"`
	Slide   string          `json:"slide"`
	Item    string          `json:"item"`
	Pin     Pin             `json:"documentation"`
	Result  SelectionResult `json:"resolution"`
	// Attached is true when the selected code joined the Item's evidence for
	// implementation-deck coverage (structurally resolved, every pin current).
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

// AttachInherited adds each structurally resolved selection whose pins are all
// current to its Item's evidence as a pseudo evidence file
// "<item path>#selections/<id>", so implementation-deck coverage counts the
// selected lines once, through the Item, without a duplicate mapping. Coverage
// then judges the selected bytes itself: stale selected bytes account for
// nothing. Unresolved, conflicted, retired or noncurrent paths contribute
// nothing and stay visible through ItemSelections. Only the in-memory
// document changes.
func AttachInherited(document *saga.Saga, inventory *requirements.Inventory) []ItemSelection {
	selections := ItemSelections(context.Background(), document, inventory, "", nil)
	if len(selections) == 0 {
		return selections
	}
	landmarks := map[string]*saga.Landmark{}
	walkLandmarks(document.Section, func(l *saga.Landmark) { landmarks[l.Target] = l })
	items := map[string]*saga.Item{}
	for _, feature := range document.Features {
		for _, deck := range feature.Decks {
			for _, slide := range deck.Slides {
				for _, item := range slide.Items {
					items[item.Target] = item
				}
			}
		}
	}
	for i := range selections {
		s := &selections[i]
		if s.Result.State != "resolved" || !s.Result.PinsCurrent {
			continue
		}
		item := items[s.Item]
		file := saga.CodeFile{Path: item.Path + InheritedMarker + s.Result.Selection.ID, Version: saga.CurrentVersion, References: []coderef.Reference{s.Result.Selection.Code}}
		item.Code = append(item.Code, file)
		item.HasCode = true
		if l := landmarks[s.Item]; l != nil {
			l.Code = append(l.Code, file)
			l.HasCode = true
		}
		s.Attached = true
	}
	return selections
}

// IsInherited reports whether an evidence-file path names an inherited
// selection rather than an authored evidence record.
func IsInherited(evidenceFile string) bool { return strings.Contains(evidenceFile, InheritedMarker) }

func walkLandmarks(section *saga.Section, visit func(*saga.Landmark)) {
	if section == nil {
		return
	}
	for _, fragment := range section.Fragments {
		for i := range fragment.Landmarks {
			visit(&fragment.Landmarks[i])
		}
	}
	for _, child := range section.Children {
		walkLandmarks(child, visit)
	}
}
