package saga

// AnnotationContentDigest uses the existing authored-content digest contract
// for a fragment, landmark, deck, slide or Item, including report fragments
// outside design roots. Evidence and review overlays never change the pin.
func AnnotationContentDigest(document *Saga, target string) (digest string, found, conflicted bool, err error) {
	result := map[string]string{}
	decks := append([]*Deck{}, document.Decks...)
	decks = append(decks, document.Onboarding...)
	for _, theme := range document.FeatureThemes {
		decks = append(decks, theme.Decks...)
	}
	for _, review := range document.Reviews {
		if review.Deck != nil {
			decks = append(decks, review.Deck)
		}
	}
	for _, deck := range decks {
		if deck.Target == target {
			for _, slide := range deck.Slides {
				conflicted = conflicted || slide.AuthoringConflict
			}
			digest, err = digestVisualDeck(deck, result)
			return digest, true, conflicted, err
		}
		for _, slide := range deck.Slides {
			match := slide.Target == target
			for _, item := range slide.Items {
				match = match || item.Target == target
			}
			if match {
				_, err = digestVisualSlide(slide, result)
				return result[target], true, slide.AuthoringConflict, err
			}
		}
	}
	var walk func(*Section) bool
	walk = func(section *Section) bool {
		if section == nil {
			return false
		}
		for _, fragment := range section.Fragments {
			if fragment.SlideMeta != nil {
				continue
			}
			match := fragment.Target == target
			for _, landmark := range fragment.Landmarks {
				match = match || landmark.Target == target
			}
			if match {
				_, err = digestDesignFragment(fragment, result)
				digest = result[target]
				return true
			}
		}
		for _, child := range section.Children {
			if walk(child) {
				return true
			}
		}
		return false
	}
	found = walk(document.Section)
	return
}
