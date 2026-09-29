package saga

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// FrontBullets returns authored summary bullets, or the legacy takeaway then
// Item labels when no authored front exists. The returned slice is a copy.
func (slide *Slide) FrontBullets() []string {
	if len(slide.Front) > 0 {
		return append([]string{}, slide.Front...)
	}
	if strings.TrimSpace(slide.Takeaway) != "" {
		return []string{slide.Takeaway}
	}
	result := []string{}
	for _, item := range slide.Items {
		if strings.TrimSpace(item.Label) != "" {
			result = append(result, item.Label)
		}
	}
	return result
}

func validateFront(front []string, path string, validation *Validation) {
	for i, bullet := range front {
		if strings.TrimSpace(bullet) == "" {
			addIssue(validation, "error", path, fmt.Sprintf("slide front bullet %d must not be blank", i+1))
		}
	}
}

// ResolveOverviewAnnotation only resolves within this deck, and only resolves
// an Item within the named slide. No URI or evidence is independently loaded.
func (deck *Deck) ResolveOverviewAnnotation(annotation OverviewAnnotation) (*Slide, *Item, error) {
	for _, slide := range deck.Slides {
		if annotation.Slide != slide.ID && annotation.Slide != slide.Target {
			continue
		}
		if annotation.Item == "" {
			return slide, nil, nil
		}
		for _, item := range slide.Items {
			if annotation.Item == item.ID || annotation.Item == item.Target {
				return slide, item, nil
			}
		}
		return nil, nil, fmt.Errorf("overview annotation %q item %q must identify an existing Item in slide %q", annotation.ID, annotation.Item, annotation.Slide)
	}
	return nil, nil, fmt.Errorf("overview annotation %q slide %q must identify an existing slide in this deck", annotation.ID, annotation.Slide)
}

// ValidateDeckOverview checks navigation references and every-slide coverage.
// Missing authored content/coverage is a compatibility warning, never code
// coverage. Invalid authored references remain errors even on legacy decks.
func ValidateDeckOverview(deck *Deck) Validation {
	result := Validation{Valid: true, Issues: []Issue{}}
	covered := map[string]bool{}
	if deck.Overview == nil {
		addIssue(&result, "warning", deck.Path, "deck is missing an authored overview; viewer uses a generated slide directory")
	} else {
		if strings.TrimSpace(deck.Overview.Body) == "" {
			addIssue(&result, "error", deck.Path, "deck overview body must not be blank")
		}
		ids := map[string]bool{}
		counts := map[string]int{}
		for _, annotation := range deck.Overview.Annotations {
			counts[annotation.ID]++
		}
		citations, visuals := OverviewReferences(deck.Overview.Body)
		cited := map[string]bool{}
		for _, id := range citations {
			cited[id] = true
		}
		for _, annotation := range deck.Overview.Annotations {
			if !ValidID(annotation.ID) || ids[annotation.ID] {
				addIssue(&result, "error", deck.Path, fmt.Sprintf("overview annotation id %q must be a unique stable id", annotation.ID))
			}
			ids[annotation.ID] = true
			if strings.TrimSpace(annotation.Label) == "" {
				addIssue(&result, "error", deck.Path, fmt.Sprintf("overview annotation %q label must not be blank", annotation.ID))
			}
			slide, _, err := deck.ResolveOverviewAnnotation(annotation)
			if err != nil {
				addIssue(&result, "error", deck.Path, err.Error())
			} else if cited[annotation.ID] && counts[annotation.ID] == 1 && ValidID(annotation.ID) && strings.TrimSpace(annotation.Label) != "" {
				covered[slide.Target] = true
			}
		}
		for _, id := range citations {
			if !ids[id] {
				addIssue(&result, "error", deck.Path, fmt.Sprintf("overview citation names unknown annotation %q", id))
			}
		}
		for _, annotation := range deck.Overview.Annotations {
			if !cited[annotation.ID] {
				addIssue(&result, "warning", deck.Path, fmt.Sprintf("overview annotation %q is not cited in the body", annotation.ID))
			}
		}
		for _, target := range visuals {
			slide, _, err := deck.ResolveOverviewAnnotation(OverviewAnnotation{ID: "visual", Slide: target})
			if err != nil {
				addIssue(&result, "error", deck.Path, err.Error())
			} else if !strings.HasPrefix(slide.MediaType, "image/") {
				addIssue(&result, "error", deck.Path, fmt.Sprintf("overview visual %q must name an image slide", target))
			}
		}

	}
	for _, slide := range deck.Slides {
		if !covered[slide.Target] {
			addIssue(&result, "warning", deck.Path, fmt.Sprintf("deck overview does not cover slide %q", slide.ID))
		}
	}
	result.Valid = !hasErrors(result.Issues)
	return result
}

// EffectiveOverview returns the authored overview or an explicitly generated,
// deterministic directory. The boolean identifies the generated fallback;
// callers must expose that distinction. Nothing is written back to storage.
func (deck *Deck) EffectiveOverview() (*DeckOverview, bool) {
	if deck.Overview != nil {
		return deck.Overview, false
	}
	result := &DeckOverview{Annotations: []OverviewAnnotation{}}
	var body strings.Builder
	body.WriteString("## Generated slide directory\n\nThis deck has no authored overview.\n")
	for i, slide := range deck.Slides {
		id := fmt.Sprintf("slide-%d", i+1)
		result.Annotations = append(result.Annotations, OverviewAnnotation{ID: id, Label: slide.Title, Slide: slide.Target})
		// Labels are literal text, not authored Markdown.
		label := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "\n", " ", "\r", " ", "*", "\\*", "_", "\\_", "`", "\\`").Replace(slide.Title)
		fmt.Fprintf(&body, "\n- [%s](annotation:%s)", label, id)
	}
	body.WriteByte('\n')
	result.Body = body.String()
	return result, true
}

// OverviewReferences returns actual Markdown citation and slide-image destinations
// in source order. Code spans/blocks and raw HTML are not citations. An image
// inside a link is still a visual; its enclosing link may be a citation.
func OverviewReferences(body string) (citationIDs, slideVisuals []string) {
	markdown := goldmark.New(goldmark.WithExtensions(extension.GFM))
	document := markdown.Parser().Parse(text.NewReader([]byte(body)))
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Link:
			if id, ok := strings.CutPrefix(string(node.Destination), "annotation:"); ok {
				citationIDs = append(citationIDs, id)
			}
		case *ast.Image:
			if target, ok := strings.CutPrefix(string(node.Destination), "slide:"); ok {
				slideVisuals = append(slideVisuals, target)
			}
		}
		return ast.WalkContinue, nil
	})
	return
}

// DeckOverviewReport exposes the effective report and reference resolution without
// copying evidence. Generated references are navigation only and earn no coverage.
type DeckOverviewReport struct {
	Overview        *DeckOverview       `json:"overview"`
	Generated       bool                `json:"generated"`
	References      []OverviewReference `json:"references"`
	CoveredSlides   []string            `json:"covered_slides"`
	UncoveredSlides []string            `json:"uncovered_slides"`
	Complete        bool                `json:"complete"`
	Validation      Validation          `json:"validation"`
}

type OverviewReference struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Slide string `json:"slide,omitempty"`
	Item  string `json:"item,omitempty"`
	Cited bool   `json:"cited"`
	Error string `json:"error,omitempty"`
}

func (deck *Deck) OverviewReport() DeckOverviewReport {
	overview, generated := deck.EffectiveOverview()
	report := DeckOverviewReport{Overview: overview, Generated: generated, References: []OverviewReference{}, CoveredSlides: []string{}, UncoveredSlides: []string{}, Validation: ValidateDeckOverview(deck)}
	citations, _ := OverviewReferences(overview.Body)
	cited, covered := map[string]bool{}, map[string]bool{}
	counts := map[string]int{}
	for _, a := range overview.Annotations {
		counts[a.ID]++
	}
	for _, id := range citations {
		cited[id] = true
	}
	for _, a := range overview.Annotations {
		ref := OverviewReference{ID: a.ID, Label: a.Label, Cited: cited[a.ID]}
		slide, item, err := deck.ResolveOverviewAnnotation(a)
		if err != nil {
			ref.Error = err.Error()
		} else {
			ref.Slide = slide.Target
			if item != nil {
				ref.Item = item.Target
			}
			if !generated && cited[a.ID] && counts[a.ID] == 1 && ValidID(a.ID) && strings.TrimSpace(a.Label) != "" {
				covered[slide.Target] = true
			}
		}
		report.References = append(report.References, ref)
	}
	for _, slide := range deck.Slides {
		if covered[slide.Target] {
			report.CoveredSlides = append(report.CoveredSlides, slide.Target)
		} else {
			report.UncoveredSlides = append(report.UncoveredSlides, slide.Target)
		}
	}
	report.Complete = !generated && report.Validation.Valid && len(report.UncoveredSlides) == 0
	return report
}
