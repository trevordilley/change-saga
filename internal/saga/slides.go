package saga

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

var slideIntents = map[string]bool{
	"orient": true, "explain": true, "compare": true, "trace": true,
	"prove": true, "risk": true, "conclude": true,
}

var slideLayouts = map[string]bool{
	"hero": true, "diagram": true, "before-after": true, "sequence": true,
	"evidence": true, "risk": true, "custom": true,
}

var itemKinds = map[string]bool{
	"node": true, "edge": true, "region": true, "transition": true,
	"statement": true, "risk": true, "metric": true, "example": true, "callout": true,
}

var slideMediaTypes = map[string]bool{
	"image/svg+xml": true, "image/png": true, "image/jpeg": true,
	"image/webp": true, "text/html": true,
}

// loadEmbeddedDecks discovers the independently mergeable flat deck bundles
// under dir: an epic's ___slides/ or the app's ___onboarding/. The Saga
// manifest remains the only identity; deck, slide, and Item URNs are derived
// from its ID, so they are unique across the app.
func loadEmbeddedDecks(root, dir, role string, manifest Manifest, options loadOptions, validation *Validation) ([]*Deck, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []*Deck
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), EmbeddedDeckSuffix) {
			addIssue(validation, "error", relativePath(root, path), "embedded slide decks must be real <id>.deck directories")
			continue
		}
		decks, loadErr := loadDeckRecords(root, path, appDeckTargets(manifest.ID), options, validation)
		if loadErr != nil {
			return nil, loadErr
		}
		if len(decks) != 1 {
			addIssue(validation, "error", relativePath(root, path), "an embedded deck bundle must contain exactly one deck record")
			continue
		}
		deck := decks[0]
		if strings.TrimSuffix(entry.Name(), EmbeddedDeckSuffix) != deck.ID {
			addIssue(validation, "error", deck.Path, "embedded deck directory must match the deck id")
		}
		validateDeckRole(deck, role, manifest.ID, validation)
		result = append(result, deck)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Rank == result[j].Rank {
			return result[i].Path < result[j].Path
		}
		return result[i].Rank < result[j].Rank
	})
	return result, nil
}

// deckTargets names the deck, slide, and Item URNs of one deck bundle. The
// app's decks are named in the app's URN space; a review's deck is named
// inside its review, so slide IDs never collide across reviews.
type deckTargets struct {
	deck  func(deckID string) string
	slide func(slideID string) string
	item  func(slideID, itemID string) string
}

func appDeckTargets(sagaID string) deckTargets {
	return deckTargets{
		deck:  func(id string) string { return DeckTarget(sagaID, id) },
		slide: func(id string) string { return SlideTarget(sagaID, id) },
		item:  func(slide, id string) string { return ItemTarget(sagaID, slide, id) },
	}
}

func loadDeckRecords(root, recordRoot string, targets deckTargets, options loadOptions, validation *Validation) ([]*Deck, error) {
	entries, err := os.ReadDir(recordRoot)
	if err != nil {
		return nil, err
	}
	var decks []*Deck
	decksByKey := map[string]*Deck{}
	slidesByKey := map[string]*Slide{}
	itemsByKey := map[string]*Item{}
	allowedAssets := map[string]bool{}
	regular := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(recordRoot, name)
		diagnostic := relativePath(root, path)
		if issue := flatPathIssue(recordRoot, name); issue != "" {
			addIssue(validation, "error", diagnostic, issue)
		}
		if !flatRegular(entry) {
			addIssue(validation, "error", diagnostic, "slide storage permits only regular files inside a deck bundle")
			continue
		}
		regular[name] = true
		matches := flatDeckName.FindStringSubmatch(name)
		if matches == nil {
			continue
		}
		var value DeckManifest
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", name, err.Error())
			continue
		}
		deck := &Deck{Path: diagnostic, Directory: recordRoot, DeckManifest: value, Target: targets.deck(value.ID)}
		validateDeckManifest(value, name, deck.Target, validation)
		key := FlatTargetKey(deck.Target)
		if matches[2] != key {
			addIssue(validation, "error", name, "deck filename key does not match its stable target")
		}
		if previous := decksByKey[key]; previous != nil {
			addIssue(validation, "error", name, fmt.Sprintf("deck storage key collides with %s", previous.Path))
		}
		decksByKey[key] = deck
		decks = append(decks, deck)
	}

	for _, entry := range entries {
		name := entry.Name()
		matches := flatSlideName.FindStringSubmatch(name)
		if matches == nil || !regular[name] {
			continue
		}
		path := filepath.Join(recordRoot, name)
		var value SlideManifest
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", name, err.Error())
			continue
		}
		deck := decksByKey[matches[1]]
		if deck == nil {
			addIssue(validation, "error", name, "slide filename references an unknown deck key")
			continue
		}
		slide := &Slide{Path: relativePath(root, path), Directory: recordRoot, SlideManifest: value, Target: targets.slide(value.ID)}
		validateSlideManifest(value, name, deck.ID, deck.Target, slide.Target, recordRoot, options.outline, validation)
		key := FlatTargetKey(slide.Target)
		if matches[3] != key {
			addIssue(validation, "error", name, "slide filename key does not match its stable target")
		}
		if previous := slidesByKey[key]; previous != nil {
			addIssue(validation, "error", name, fmt.Sprintf("slide storage key collides with %s", previous.Path))
		}
		slidesByKey[key] = slide
		deck.Slides = append(deck.Slides, slide)
		allowedAssets[value.Entrypoint] = true
	}

	// Items are part of a slide's structural outline, not its potentially large
	// evidence graph. Review targets must remain discoverable even in an outline
	// load so an Item comment or decision cannot make the shell invalid.
	for _, entry := range entries {
		name := entry.Name()
		matches := flatItemName.FindStringSubmatch(name)
		if matches == nil || !regular[name] {
			continue
		}
		path := filepath.Join(recordRoot, name)
		var value ItemManifest
		if err := readJSON(path, &value); err != nil {
			addIssue(validation, "error", name, err.Error())
			continue
		}
		slide := slidesByKey[matches[1]]
		if slide == nil {
			addIssue(validation, "error", name, "item filename references an unknown slide key")
			continue
		}
		item := &Item{Path: relativePath(root, path), Directory: recordRoot, ItemManifest: value, Target: targets.item(slide.ID, value.ID)}
		validateItem(item, slide, validation)
		expected, nameErr := FlatItemFilename(slide.Target, item.Target, value.Rank)
		if nameErr != nil || expected != name {
			addIssue(validation, "error", name, "item filename does not match its parent, rank, and stable target")
		}
		key := FlatTargetKey(item.Target)
		if previous := itemsByKey[key]; previous != nil {
			addIssue(validation, "error", name, fmt.Sprintf("item storage key collides with %s", previous.Path))
		}
		itemsByKey[key] = item
		slide.Items = append(slide.Items, item)
	}

	if !options.outline {
		for _, entry := range entries {
			name := entry.Name()
			matches := flatEvidenceName.FindStringSubmatch(name)
			if matches == nil || !regular[name] {
				continue
			}
			item := itemsByKey[matches[1]]
			if item == nil {
				addIssue(validation, "error", name, "evidence filename references an unknown Item key")
				continue
			}
			item.HasCode = true
			if options.skipCoverage {
				continue
			}
			var value CodeFile
			if err := readJSON(filepath.Join(recordRoot, name), &value); err != nil {
				addIssue(validation, "error", name, err.Error())
				continue
			}
			value.Path = relativePath(root, filepath.Join(recordRoot, name))
			validateCodeFile(value, validation)
			item.Code = append(item.Code, value)
		}
	}

	knownRecord := func(name string) bool {
		contentRecord := allowedAssets[name] || flatDeckName.MatchString(name) || flatSlideName.MatchString(name) ||
			flatItemName.MatchString(name) || flatEvidenceName.MatchString(name)
		return contentRecord || strings.HasPrefix(name, ".change-saga-stage-") || strings.HasPrefix(name, ".change-saga-write-")
	}
	for _, entry := range entries {
		if !entry.IsDir() && !knownRecord(entry.Name()) {
			addIssue(validation, "error", entry.Name(), "file does not match the compact deck storage contract")
		}
	}

	sort.Slice(decks, func(i, j int) bool {
		if decks[i].Rank == decks[j].Rank {
			return decks[i].Path < decks[j].Path
		}
		return decks[i].Rank < decks[j].Rank
	})
	for _, deck := range decks {
		sort.Slice(deck.Slides, func(i, j int) bool {
			if deck.Slides[i].Rank == deck.Slides[j].Rank {
				return deck.Slides[i].Path < deck.Slides[j].Path
			}
			return deck.Slides[i].Rank < deck.Slides[j].Rank
		})
		for _, slide := range deck.Slides {
			sort.Slice(slide.Items, func(i, j int) bool {
				if slide.Items[i].Rank == slide.Items[j].Rank {
					return slide.Items[i].Path < slide.Items[j].Path
				}
				return slide.Items[i].Rank < slide.Items[j].Rank
			})
			if !options.outline {
				validateSlideComposition(slide, validation)
			}
		}
	}
	return decks, nil
}

func validateDeckManifest(value DeckManifest, path, target string, validation *Validation) {
	if value.Version != DeckRecordVersion || !stableID.MatchString(value.ID) || strings.TrimSpace(value.Title) == "" {
		addIssue(validation, "error", path, "deck requires version 4, a stable id, and a title")
	}
	expected, err := FlatDeckFilename(target, value.Rank)
	if err != nil || expected != path {
		addIssue(validation, "error", path, "deck filename does not match its rank and stable target")
	}
	if value.Role != DeckRoleChange && value.Role != DeckRoleOnboarding && value.Role != DeckRoleReview {
		addIssue(validation, "error", path, "deck role must be change, onboarding, or review")
	}
	if validFlatRank(value.Rank) != nil || strings.TrimSpace(value.Objective) == "" || utf8.RuneCountInString(value.Objective) > 240 {
		addIssue(validation, "error", path, "deck rank must fit the portable range and objective must contain 1 to 240 characters")
	}
}

func validateSlideManifest(value SlideManifest, path, deckID, deckTarget, target, dir string, outline bool, validation *Validation) {
	if value.Version != DeckRecordVersion || !stableID.MatchString(value.ID) || strings.TrimSpace(value.Title) == "" {
		addIssue(validation, "error", path, "slide requires version 4, a stable id, and a title")
	}
	if value.DeckID != deckID {
		addIssue(validation, "error", path, "slide deck must name its semantic parent")
	}
	expected, err := FlatSlideFilename(deckTarget, target, value.Rank)
	if err != nil || expected != path {
		addIssue(validation, "error", path, "slide filename does not match its parent, rank, and stable target")
	}
	if validFlatRank(value.Rank) != nil || !slideIntents[value.Intent] || !slideLayouts[value.Layout] || !slideMediaTypes[value.MediaType] {
		addIssue(validation, "error", path, "slide requires a portable rank and supported intent, layout, and visual media_type")
	}
	if value.Section != strings.TrimSpace(value.Section) || utf8.RuneCountInString(value.Section) > 80 {
		addIssue(validation, "error", path, "slide section must be trimmed and at most 80 characters")
	}
	if strings.TrimSpace(value.Takeaway) == "" || utf8.RuneCountInString(value.Takeaway) > 180 {
		addIssue(validation, "error", path, "slide takeaway must contain 1 to 180 characters")
	}
	if value.Layout == "custom" && strings.TrimSpace(value.ExceptionRationale) == "" {
		addIssue(validation, "error", path, "custom layout requires exception_rationale")
	}
	extension := filepath.Ext(value.Entrypoint)
	expectedAsset, assetErr := FlatSlideAssetFilename(path, extension)
	if assetErr != nil || expectedAsset != value.Entrypoint {
		addIssue(validation, "error", path, "slide entrypoint must be the compact asset paired with its manifest")
	}
	component := FragmentManifest{Version: CurrentVersion, ID: value.ID, Title: value.Title, MediaType: value.MediaType, Entrypoint: value.Entrypoint, Order: value.Rank}
	validateFragmentManifestMode(component, path, dir, !outline, validation)
}

func validateItem(item *Item, slide *Slide, validation *Validation) {
	if item.Version != DeckRecordVersion || !ValidMarkdownAnchor(item.ID) || !itemKinds[item.Kind] || strings.TrimSpace(item.Label) == "" {
		addIssue(validation, "error", item.Path, "item requires version 4, a lowercase stable id, a supported kind, and a label")
	}
	if item.SlideID != slide.ID {
		addIssue(validation, "error", item.Path, "item slide must name its semantic parent")
	}
	if strings.TrimSpace(item.Description) == "" {
		addIssue(validation, "error", item.Path, "item description is required as its non-visual equivalent")
	}
	if validFlatRank(item.Rank) != nil {
		addIssue(validation, "error", item.Path, "item rank must fit the portable range")
	}
	if utf8.RuneCountInString(item.Label) > 100 || utf8.RuneCountInString(item.Description) > 240 {
		addIssue(validation, "error", item.Path, "item label and description exceed the 100/240 character density limits")
	}
	if item.Kind == "callout" {
		if strings.TrimSpace(item.Body) == "" || utf8.RuneCountInString(item.Body) > 240 {
			addIssue(validation, "error", item.Path, "callout item body must contain 1 to 240 characters")
		}
		if item.Placement != "" && item.Placement != "top" && item.Placement != "right" && item.Placement != "bottom" && item.Placement != "left" && item.Placement != "overlay" {
			addIssue(validation, "error", item.Path, "callout placement is not supported")
		}
		if item.Leader != "" && item.Leader != "none" && item.Leader != "line" && item.Leader != "arrow" {
			addIssue(validation, "error", item.Path, "callout leader is not supported")
		}
	} else if item.About != "" || item.Body != "" || item.Placement != "" || item.Leader != "" {
		addIssue(validation, "error", item.Path, "about, body, placement, and leader are callout-only fields")
	}
	legacy := Landmark{Path: item.Path, Directory: item.Directory, Version: CurrentVersion, ID: item.ID, Label: item.Label, Description: item.Description, Selector: item.Selector, Hotspot: item.Hotspot}
	fragment := &Fragment{Directory: slide.Directory, ID: slide.ID, MediaType: slide.MediaType, Entrypoint: slide.Entrypoint}
	firstSelectorIssue := len(validation.Issues)
	validateLandmark(legacy, fragment, validation)
	for index := firstSelectorIssue; index < len(validation.Issues); index++ {
		validation.Issues[index].Message = strings.NewReplacer("landmark", "item", "fragment", "slide").Replace(validation.Issues[index].Message)
	}
}

func validateSlideComposition(slide *Slide, validation *Validation) {
	if len(slide.Items) == 0 {
		// A new slide is work in progress until its Items are added, so an
		// empty slide is reported once and never makes the Saga invalid.
		addIssue(validation, "warning", slide.Path, "slide has no semantic Items yet; add an Item for every meaningful node, edge, region, transition, and callout")
	}
	if len(slide.Items) > 7 {
		severity := "error"
		if slide.Layout == "custom" && strings.TrimSpace(slide.ExceptionRationale) != "" {
			severity = "warning"
		}
		addIssue(validation, severity, slide.Path, "standard slide layouts allow at most 7 semantic items; split the slide or use a justified custom layout")
	}
	items := map[string]*Item{}
	for _, item := range slide.Items {
		items[item.ID] = item
	}
	seen := map[string]bool{}
	for _, id := range slide.ReadingOrder {
		if seen[id] {
			addIssue(validation, "error", slide.Path, fmt.Sprintf("reading_order repeats item %q", id))
		} else if items[id] == nil {
			addIssue(validation, "error", slide.Path, fmt.Sprintf("reading_order references unknown item %q", id))
		}
		seen[id] = true
	}
	for _, item := range slide.Items {
		if !seen[item.ID] {
			addIssue(validation, "error", item.Path, "every semantic item must appear in slide reading_order")
		}
		if item.Kind == "callout" && item.About != "" {
			if item.About == item.ID || items[item.About] == nil {
				addIssue(validation, "error", item.Path, "callout about must name a different item on the same slide")
			}
		}
	}
}

// projectDecks lets the established review/evidence plumbing address deck,
// slide, and Item targets through the section model.
func projectDecks(manifest Manifest, decks []*Deck) *Section {
	root := &Section{Kind: "saga", ID: manifest.ID + "-root", Title: manifest.Title, Target: SagaTarget(manifest.ID)}
	for _, deck := range decks {
		section := &Section{Path: deck.Path, Kind: "deck", ID: deck.ID, Title: deck.Title, Order: deck.Rank, Target: deck.Target}
		for _, slide := range deck.Slides {
			meta := slide.SlideManifest
			fragment := &Fragment{Path: slide.Path, Directory: slide.Directory, ID: slide.ID, Title: slide.Title, MediaType: slide.MediaType, Entrypoint: slide.Entrypoint, Order: slide.Rank, Target: slide.Target, SlideMeta: &meta, DeckRole: deck.Role}
			for _, item := range slide.Items {
				meta := item.ItemManifest
				fragment.Landmarks = append(fragment.Landmarks, Landmark{Path: item.Path, Directory: item.Directory, Version: item.Version, ID: item.ID, Label: item.Label, Description: item.Description, Selector: item.Selector, Hotspot: item.Hotspot, Target: item.Target, Code: item.Code, HasCode: item.HasCode, ItemMeta: &meta})
				fragment.HasCode = fragment.HasCode || item.HasCode
			}
			section.Fragments = append(section.Fragments, fragment)
		}
		root.Children = append(root.Children, section)
	}
	return root
}

// validateDeckRole enforces what each deck location may hold. An epic's
// implementation deck explains code, so its Items own code evidence and never
// reference records. The onboarding deck explains the app, so every Item
// references a persona, epic, or story record and owns no code evidence.
func validateDeckRole(deck *Deck, role, sagaID string, validation *Validation) {
	if deck.Role != role {
		addIssue(validation, "error", deck.Path, fmt.Sprintf("a deck in this location must use role %s", role))
	}
	for _, slide := range deck.Slides {
		for _, item := range slide.Items {
			switch role {
			case DeckRoleReview:
				// A review Item may reference the code the change touched,
				// a Saga record to open beside it, both, or neither.
				if item.Record != "" && !validReviewRecordReference(sagaID, item.Record) {
					addIssue(validation, "error", item.Path, "review item record must be a canonical record URN of this Saga")
				}
			case DeckRoleOnboarding:
				if !validRecordReference(sagaID, item.Record) {
					addIssue(validation, "error", item.Path, "onboarding item record must be a canonical persona, epic, or story URN of this Saga")
				}
				if item.HasCode {
					addIssue(validation, "error", item.Path, "onboarding items reference records, not code evidence")
				}
			default:
				if item.Record != "" {
					addIssue(validation, "error", item.Path, "record is an onboarding-only field; implementation items reference code")
				}
			}
		}
	}
}

// RecordReferenceKinds are the record kinds an onboarding Item may reference.
var RecordReferenceKinds = []string{"persona", "epic", "story"}

// ReviewRecordReferenceKinds are the documentation records a review Item may
// reference so a reviewer can open them beside the change.
var ReviewRecordReferenceKinds = []string{"persona", "epic", "story", "test-case", "deck", "slide", "chapter", "section", "fragment"}

func validReviewRecordReference(sagaID, value string) bool {
	return validRecordOfKinds(sagaID, value, ReviewRecordReferenceKinds)
}

func validRecordReference(sagaID, value string) bool {
	return validRecordOfKinds(sagaID, value, RecordReferenceKinds)
}

func validRecordOfKinds(sagaID, value string, kinds []string) bool {
	for _, kind := range kinds {
		prefix := "urn:change-saga:" + sagaID + ":" + kind + ":"
		if id := strings.TrimPrefix(value, prefix); id != value && stableID.MatchString(id) {
			return true
		}
	}
	return false
}
