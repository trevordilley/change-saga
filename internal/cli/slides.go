package cli

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func relativePathForOutput(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relative)
}

func AddDeck(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("add-deck", commandUsage["add-deck"], out)
	id := flags.String("id", "", "stable deck identifier")
	title := flags.String("title", "", "deck title")
	role := flags.String("role", "change", "deck role: change for a feature's implementation deck, onboarding for the app's onboarding deck")
	qualifiedID := flags.Bool("feature-qualified-id", false, "generate <feature>--<name> when --id is omitted")
	feature := featureIDFlag(flags)
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative review order; defaults after the last deck")
	objective := flags.String("objective", "", "one concise reviewer objective")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: %s", commandUsage["add-deck"])
	}
	name := store.Slug(strings.TrimSuffix(flags.Arg(1), ".deck"))
	implicitID := *id == ""
	if *qualifiedID && !implicitID {
		return fmt.Errorf("--feature-qualified-id cannot be combined with --id")
	}
	if *title == "" {
		*title = strings.ReplaceAll(name, "-", " ")
	}
	if *objective == "" {
		return fmt.Errorf("--objective is required")
	}
	if (rank.set && rank.value < 0) || utf8.RuneCountInString(*objective) > 240 {
		return fmt.Errorf("--rank must be non-negative and --objective cannot exceed 240 characters")
	}
	var created, target string
	err := authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		var slidesRoot string
		var peers []*saga.Deck
		switch *role {
		case saga.DeckRoleChange:
			target, err := requireFeature(document.Root, *feature)
			if err != nil {
				return err
			}
			if implicitID && *qualifiedID {
				*id = applayout.FeatureQualifiedID(target.ID, name)
			} else if implicitID {
				*id = name
			}
			slidesRoot = filepath.Join(target.Dir, saga.EmbeddedSlidesDir)
			if found := document.FindFeature(target.ID); found != nil {
				peers = found.Decks
			}
		case saga.DeckRoleOnboarding:
			if *qualifiedID {
				return fmt.Errorf("--feature-qualified-id is only for feature-owned implementation decks")
			}
			if implicitID {
				*id = name
			}
			if *feature != "" {
				return fmt.Errorf("the onboarding deck belongs to the app, not a feature; omit --feature")
			}
			if len(document.Onboarding) > 0 {
				return fmt.Errorf("the app already has onboarding deck %q", document.Onboarding[0].ID)
			}
			slidesRoot = filepath.Join(document.Root, applayout.OnboardingDir)
		default:
			return fmt.Errorf("--role must be change or onboarding")
		}
		if !saga.ValidID(*id) || targetIDExists(document, *id) {
			return fmt.Errorf("deck id %q is invalid or already used", *id)
		}
		chosenRank := rank.value
		if !rank.set {
			for _, deck := range peers {
				if deck.Rank >= chosenRank {
					chosenRank = deck.Rank + 10
				}
			}
		}
		manifest := saga.DeckManifest{Version: saga.DeckRecordVersion, ID: *id, Title: *title, Role: *role, Rank: chosenRank, Objective: strings.TrimSpace(*objective)}
		target = saga.DeckTarget(document.Manifest.ID, *id)
		filename, err := saga.FlatDeckFilename(target, chosenRank)
		if err != nil {
			return err
		}
		if info, statErr := os.Lstat(slidesRoot); statErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return fmt.Errorf("%s must be a real directory", filepath.Base(slidesRoot))
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return statErr
		}
		bundle := filepath.Join(slidesRoot, *id+saga.EmbeddedDeckSuffix)
		if len(filepath.Join(bundle, strings.Repeat("x", saga.FlatMaxBasename))) > saga.FlatMaxPath {
			return fmt.Errorf("embedded deck path exceeds the portable %d-character budget; choose a shorter Saga location or deck id", saga.FlatMaxPath)
		}
		if err := os.MkdirAll(slidesRoot, 0o755); err != nil {
			return err
		}
		if err := store.CommitDir(document.Root, bundle, func(stage string) error {
			return store.WriteJSON(filepath.Join(stage, filename), manifest, true)
		}); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("embedded deck %q already exists", *id)
			}
			return err
		}
		created = relativePathForOutput(document.Root, filepath.Join(bundle, filename))
		return nil
	})
	if err != nil {
		return err
	}
	// An onboarding deck orients a newcomer; an implementation deck explains
	// the change.
	intent := "explain"
	if *role == saga.DeckRoleOnboarding {
		intent = "orient"
	}
	fmt.Fprintf(out, "Added deck %s\nTarget: %s\nNext: change-saga add-slide --deck %s --intent %s --layout diagram %s first-slide\n", filepath.ToSlash(created), target, *id, intent, flags.Arg(0))
	return nil
}

func AddSlide(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("add-slide", commandUsage["add-slide"], out)
	deckTarget := flags.String("deck", "", "containing deck path, id, or URN")
	id := flags.String("id", "", "stable slide identifier")
	title := flags.String("title", "", "slide title")
	section := flags.String("section", "", "optional section label shown as a subtle divider inside the deck")
	intent := flags.String("intent", "", "reviewer job: orient, explain, compare, trace, prove, risk, or conclude")
	layout := flags.String("layout", "", "canvas arrangement, not diagram meaning: hero, diagram, before-after, sequence, evidence, risk, or custom")
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative review order; defaults after the last slide")
	takeaway := flags.String("takeaway", "", "single reviewer takeaway (maximum 180 characters)")
	rationale := flags.String("exception-rationale", "", "required reason for a custom layout")
	source := flags.String("source", "", "SVG, image, or self-contained HTML source")
	mediaType := flags.String("media-type", "image/svg+xml", "visual media type")
	entrypoint := flags.String("entrypoint", "slide.svg", "simple filename whose extension selects the compact slide asset name")
	feature := featureIDFlag(flags)
	reviewID := flags.String("review", "", "add the slide to this pull request review's deck instead of --deck")
	qualifiedID := flags.Bool("feature-qualified-id", false, "generate <feature>--<name> when --id is omitted")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 2 || (*deckTarget == "") == (*reviewID == "") || *intent == "" || *layout == "" {
		return fmt.Errorf("usage: %s", commandUsage["add-slide"])
	}
	name := store.Slug(strings.TrimSuffix(flags.Arg(1), ".slide"))
	implicitID := *id == ""
	if *qualifiedID && !implicitID {
		return fmt.Errorf("--feature-qualified-id cannot be combined with --id")
	}
	if *title == "" {
		*title = strings.ReplaceAll(name, "-", " ")
	}
	if *takeaway == "" {
		*takeaway = *title
	}
	validIntent := map[string]bool{"orient": true, "explain": true, "compare": true, "trace": true, "prove": true, "risk": true, "conclude": true}
	validLayout := map[string]bool{"hero": true, "diagram": true, "before-after": true, "sequence": true, "evidence": true, "risk": true, "custom": true}
	validMedia := map[string]bool{"image/svg+xml": true, "image/png": true, "image/jpeg": true, "image/webp": true, "text/html": true}
	if (rank.set && rank.value < 0) || !validIntent[*intent] || !validLayout[*layout] || !validMedia[*mediaType] {
		return fmt.Errorf("unsupported intent, layout, media type, or negative rank")
	}
	if utf8.RuneCountInString(*title) > 100 || utf8.RuneCountInString(*section) > 80 || utf8.RuneCountInString(*takeaway) > 180 {
		return fmt.Errorf("slide title/section/takeaway exceed the 100/80/180 character density limits")
	}
	if *layout == "custom" && strings.TrimSpace(*rationale) == "" {
		return fmt.Errorf("--exception-rationale is required for --layout custom")
	}
	if reason := saga.EntrypointError(*entrypoint); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	if strings.Contains(*entrypoint, "/") {
		return fmt.Errorf("--entrypoint is a simple filename used only to select the slide asset extension; nested paths are refused")
	}
	var data []byte
	var err error
	if *source != "" {
		info, statErr := os.Stat(*source)
		if statErr != nil {
			return fmt.Errorf("read slide source: %w", statErr)
		}
		if info.IsDir() {
			return fmt.Errorf("slides require one self-contained SVG, image, or HTML file; source directories are not portable")
		}
		data, err = os.ReadFile(*source)
		if err != nil {
			return fmt.Errorf("read slide source: %w", err)
		}
	} else if *mediaType == "image/svg+xml" {
		data = []byte(defaultSlideSVG(*title, *takeaway))
	} else {
		return fmt.Errorf("--source is required unless --media-type is image/svg+xml")
	}
	var created, target string
	err = authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		var deck *saga.Deck
		target = saga.SlideTarget(document.Manifest.ID, *id)
		if *reviewID != "" {
			if *qualifiedID {
				return fmt.Errorf("--feature-qualified-id is only for feature-owned implementation slides")
			}
			if implicitID {
				*id = name
			}
			review, err := findReviewDeck(document, *reviewID)
			if err != nil {
				return err
			}
			if *feature != "" {
				return fmt.Errorf("a review deck belongs to its review, not a feature; omit --feature")
			}
			deck = review.Deck
			if !saga.ValidID(*id) || review.Slide(*id) != nil {
				return fmt.Errorf("slide id %q is invalid or already used in review %s", *id, review.ID)
			}
			target = saga.ReviewSlideTarget(document.Manifest.ID, review.ID, *id)
		} else {
			deck = findDeck(document, *deckTarget)
			if deck == nil {
				return fmt.Errorf("--deck must identify an existing deck")
			}
			if err := assertDeckFeature(document, *feature, deck.Path, deck.Target); err != nil {
				return err
			}
			if implicitID && *qualifiedID {
				holding := saga.FeatureOf(deck.Path)
				if holding == "" {
					return fmt.Errorf("--feature-qualified-id is only for feature-owned implementation slides")
				}
				*id = applayout.FeatureQualifiedID(holding, name)
			} else if implicitID {
				*id = name
			}
			target = saga.SlideTarget(document.Manifest.ID, *id)
			if !saga.ValidID(*id) || targetIDExists(document, *id) {
				return fmt.Errorf("slide id %q is invalid or already used", *id)
			}
		}
		chosenRank := rank.value
		if !rank.set {
			for _, slide := range deck.Slides {
				if slide.Rank >= chosenRank {
					chosenRank = slide.Rank + 10
				}
			}
		}
		filename, err := saga.FlatSlideFilename(deck.Target, target, chosenRank)
		if err != nil {
			return err
		}
		extension := filepath.Ext(*entrypoint)
		assetName, err := saga.FlatSlideAssetFilename(filename, extension)
		if err != nil {
			return err
		}
		manifest := saga.SlideManifest{Version: saga.DeckRecordVersion, ID: *id, DeckID: deck.ID, Title: *title, Rank: chosenRank, Section: strings.TrimSpace(*section), Intent: *intent, Layout: *layout, MediaType: *mediaType, Entrypoint: assetName, Takeaway: *takeaway, ReadingOrder: []string{}, ExceptionRationale: *rationale}
		assetPath := filepath.Join(deck.Directory, assetName)
		manifestPath := filepath.Join(deck.Directory, filename)
		if err := store.WriteFile(assetPath, data, 0o644, true); err != nil {
			return err
		}
		if err := store.WriteJSON(manifestPath, manifest, true); err != nil {
			_ = os.Remove(assetPath)
			return err
		}
		created = relativePathForOutput(document.Root, manifestPath)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added slide %s\nTarget: %s\n", filepath.ToSlash(created), target)
	review, slideTarget := "", target
	if *reviewID != "" {
		review, slideTarget = "--review "+*reviewID+" ", *id
	}
	// A slide written from --source already has its content; what it lacks
	// is the Items that name its meaningful elements.
	if *source != "" {
		fmt.Fprintf(out, "Next: change-saga add-item %s--slide %s --kind KIND %s --label TEXT --description TEXT %s\n", review, slideTarget, selectorHint(*mediaType), flags.Arg(0))
		return nil
	}
	fmt.Fprintf(out, "Next: change-saga set-slide-content %s--target %s --source FILE %s\n", review, slideTarget, flags.Arg(0))
	return nil
}

// selectorHint names the selector a visual of this media type is addressed by.
func selectorHint(mediaType string) string {
	if mediaType == "text/html" || mediaType == "image/svg+xml" {
		return "--element-id ID"
	}
	return "--region X,Y,W,H"
}

func AddItem(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	flags := commandFlags("add-item", commandUsage["add-item"], out)
	slideTarget := flags.String("slide", "", "containing slide path, id, or URN")
	id := flags.String("id", "", "stable lowercase item identifier")
	kind := flags.String("kind", "", "node, edge, region, transition, statement, risk, metric, example, or callout")
	label := flags.String("label", "", "concise reviewer-facing label")
	description := flags.String("description", "", "non-visual semantic description")
	elementID := flags.String("element-id", "", "id of an SVG or HTML element")
	region := flags.String("region", "", "normalized image region x,y,width,height")
	hotspot := flags.String("hotspot", "", "optional normalized on-canvas hit area")
	about := flags.String("about", "", "for callouts, another item id on this slide")
	body := flags.String("body", "", "required concise callout body")
	placement := flags.String("placement", "", "top, right, bottom, left, or overlay")
	leader := flags.String("leader", "", "none, line, or arrow")
	documentation := flags.String("documentation", "", "canonical Component or System URN")
	documentationRevision := flags.String("documentation-revision", "", "exact revision URN for --documentation")
	documentationView := flags.String("documentation-view", "", "saved Saga commit that admits a non-current --documentation-revision (inventory format 2)")
	selectionsFile := flags.String("selections", "", "JSON array of explicit code selections through the documented entity (inventory format 2)")
	repo := flags.String("repo", "", "source checkout for saved views and selections")
	record := flags.String("record", "", "the record the item points at: required for onboarding items (a persona, feature, or story URN); optional for review items (a story, feature slide, or other record to open beside the change)")
	feature := featureIDFlag(flags)
	reviewID := flags.String("review", "", "the pull request review whose slide receives the item")
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative item order; defaults after the last item")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *slideTarget == "" || *kind == "" {
		return fmt.Errorf("usage: %s", commandUsage["add-item"])
	}
	if (*elementID == "") == (*region == "") {
		return fmt.Errorf("provide exactly one selector: --element-id or --region")
	}
	var selector saga.LandmarkSelector
	if *elementID != "" {
		selector = saga.LandmarkSelector{Type: "element", ElementID: *elementID}
		if *id == "" {
			*id = *elementID
		}
	} else {
		parsed, err := parseLandmarkRegion(*region)
		if err != nil {
			return fmt.Errorf("invalid --region: %w", err)
		}
		selector = saga.LandmarkSelector{Type: "region", X: parsed.X, Y: parsed.Y, Width: parsed.Width, Height: parsed.Height}
		if *id == "" {
			*id = "region"
		}
	}
	if !saga.ValidMarkdownAnchor(*id) {
		return fmt.Errorf("--id must begin with a lowercase letter and contain only lowercase letters, digits, and hyphens")
	}
	if *label == "" {
		*label = strings.ReplaceAll(*id, "-", " ")
	}
	validKind := map[string]bool{"node": true, "edge": true, "region": true, "transition": true, "statement": true, "risk": true, "metric": true, "example": true, "callout": true}
	if !validKind[*kind] {
		return fmt.Errorf("unsupported --kind %q", *kind)
	}
	if utf8.RuneCountInString(*label) > 100 || utf8.RuneCountInString(*description) > 240 {
		return fmt.Errorf("item label/description exceed the 100/240 character density limits")
	}
	if *kind == "callout" {
		if strings.TrimSpace(*body) == "" || utf8.RuneCountInString(*body) > 240 {
			return fmt.Errorf("callout --body must contain 1 to 240 characters")
		}
	} else if *about != "" || *body != "" || *placement != "" || *leader != "" {
		return fmt.Errorf("--about, --body, --placement, and --leader require --kind callout")
	}
	var hotspotRegion *saga.LandmarkRegion
	if *hotspot != "" {
		parsed, err := parseLandmarkRegion(*hotspot)
		if err != nil {
			return fmt.Errorf("invalid --hotspot: %w", err)
		}
		hotspotRegion = &parsed
	}
	var created, target string
	err := authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		var slide *saga.Slide
		target = ""
		if *reviewID != "" {
			review, err := findReviewDeck(document, *reviewID)
			if err != nil {
				return err
			}
			if slide = findReviewSlide(review, *slideTarget); slide == nil {
				return fmt.Errorf("review %s has no slide %q", review.ID, *slideTarget)
			}
			if *feature != "" {
				return fmt.Errorf("a review deck belongs to its review, not a feature; omit --feature")
			}
			if *record != "" {
				if err := requireReviewRecord(document, *record); err != nil {
					return err
				}
			}
			target = saga.ReviewItemTarget(document.Manifest.ID, review.ID, slide.ID, *id)
		} else {
			if slide = findSlide(document, *slideTarget); slide == nil {
				return fmt.Errorf("--slide must identify an existing slide")
			}
			if err := assertDeckFeature(document, *feature, slide.Path, slide.Target); err != nil {
				return err
			}
			if err := checkItemRecord(document, slide, *record); err != nil {
				return err
			}
			if err := guardCompleteSlideMutation("add-item", slide); err != nil {
				return err
			}
			target = saga.ItemTarget(document.Manifest.ID, slide.ID, *id)
		}
		var documentationPin *saga.DocumentationLink
		var viewOID string
		var selections []saga.ItemSelection
		if *documentation != "" || *documentationRevision != "" || *documentationView != "" || *selectionsFile != "" {
			if *documentation != "" || *documentationRevision != "" {
				documentationPin = &saga.DocumentationLink{Target: *documentation, Revision: *documentationRevision}
			}
			requested, err := readItemSelections(*selectionsFile)
			if err != nil {
				return err
			}
			viewOID, selections, err = prepareItemInventoryLinks(ctx, document.Root, document.Manifest, itemInventoryLinks{Documentation: documentationPin, View: *documentationView, Selections: requested, Checkout: *repo})
			if err != nil {
				return err
			}
			if slide.DeckID != "" {
				for _, deck := range document.Onboarding {
					if deck.ID == slide.DeckID {
						return fmt.Errorf("onboarding Items cannot carry documentation")
					}
				}
			}
		}
		if len(slide.Items) >= 7 && slide.Layout != "custom" {
			return fmt.Errorf("standard layouts allow at most 7 semantic Items; split the slide")
		}
		aboutFound := *about == ""
		for _, existing := range slide.Items {
			if existing.ID == *id {
				return fmt.Errorf("item id %q already exists on %s", *id, slide.ID)
			}
			aboutFound = aboutFound || existing.ID == *about
		}
		if !aboutFound || *about == *id {
			return fmt.Errorf("--about must identify a different existing Item on the same slide")
		}
		fragment := &saga.Fragment{Directory: slide.Directory, MediaType: slide.MediaType, Entrypoint: slide.Entrypoint}
		if err := validateLandmarkSelector(fragment, selector, hotspotRegion); err != nil {
			return err
		}
		if strings.TrimSpace(*description) == "" {
			return fmt.Errorf("--description is required so the visual item has a non-visual equivalent")
		}
		chosenRank := rank.value
		if !rank.set {
			for _, item := range slide.Items {
				if item.Rank >= chosenRank {
					chosenRank = item.Rank + 10
				}
			}
		}
		filename, err := saga.FlatItemFilename(slide.Target, target, chosenRank)
		if err != nil {
			return err
		}
		path := filepath.Join(slide.Directory, filename)
		manifest := saga.ItemManifest{Version: saga.DeckRecordVersion, ID: *id, SlideID: slide.ID, Rank: chosenRank, Kind: *kind, Label: *label, Description: strings.TrimSpace(*description), Selector: selector, Hotspot: hotspotRegion, About: *about, Body: *body, Placement: *placement, Leader: *leader, Record: *record, Documentation: documentationPin, DocumentationView: viewOID, Selections: selections}
		if err := store.WriteJSON(path, manifest, true); err != nil {
			return err
		}
		slide.ReadingOrder = append(slide.ReadingOrder, *id)
		if err := store.WriteJSON(filepath.Join(slide.Directory, filepath.Base(filepath.FromSlash(slide.Path))), slide.SlideManifest, false); err != nil {
			_ = os.Remove(path)
			return err
		}
		created = relativePathForOutput(document.Root, path)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Added item %s\nTarget: %s\n", filepath.ToSlash(created), target)
	if *record != "" {
		fmt.Fprintf(out, "Record: %s\n", *record)
		if *reviewID == "" {
			return nil
		}
	}
	fmt.Fprintf(out, "Next: change-saga cover --target %s ... %s\n", target, flags.Arg(0))
	return nil
}

func SetSlideContent(_ context.Context, args []string, out io.Writer) error {
	flags := commandFlags("set-slide-content", commandUsage["set-slide-content"], out)
	targetValue := flags.String("target", "", "slide path, id, or target URN")
	source := flags.String("source", "", "content file, or - for standard input")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	quiet := flags.Bool("quiet", false, "suppress successful output")
	feature := featureIDFlag(flags)
	reviewID := flags.String("review", "", "the pull request review whose slide is replaced")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *targetValue == "" || *source == "" {
		return fmt.Errorf("usage: %s", commandUsage["set-slide-content"])
	}
	if *jsonOutput && *quiet {
		return fmt.Errorf("--json and --quiet cannot be combined")
	}
	var data []byte
	var err error
	if *source == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(*source)
	}
	if err != nil {
		return fmt.Errorf("read slide content: %w", err)
	}
	var target string
	err = authorMutation(flags.Arg(0), func(document *saga.Saga) error {
		var slide *saga.Slide
		if *reviewID != "" {
			review, err := findReviewDeck(document, *reviewID)
			if err != nil {
				return err
			}
			if slide = findReviewSlide(review, *targetValue); slide == nil {
				return fmt.Errorf("review %s has no slide %q", review.ID, *targetValue)
			}
		} else {
			if slide = findSlide(document, *targetValue); slide == nil {
				return fmt.Errorf("--target must identify a slide")
			}
			if err := assertDeckFeature(document, *feature, slide.Path, slide.Target); err != nil {
				return err
			}
			if err := guardCompleteSlideMutation("set-slide-content", slide); err != nil {
				return err
			}
		}
		entrypoint := filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))
		if err := store.WriteFile(entrypoint, data, 0o644, false); err != nil {
			return err
		}
		target = slide.Target
		return nil
	})
	if err != nil {
		return err
	}
	if *quiet {
		return nil
	}
	if *jsonOutput {
		return writeJSON(out, map[string]any{"ok": true, "target": target, "bytes": len(data)})
	}
	fmt.Fprintf(out, "Updated %s (%d bytes)\n", target, len(data))
	return nil
}

// allDecks is every feature's implementation decks plus the onboarding deck.
func allDecks(document *saga.Saga) []*saga.Deck {
	return append(append([]*saga.Deck{}, document.Decks...), document.Onboarding...)
}

// assertDeckFeature checks an optional --feature against the feature holding a deck
// record. The onboarding deck belongs to the app, so it takes no --feature.
func assertDeckFeature(document *saga.Saga, feature, path, target string) error {
	if strings.TrimSpace(feature) == "" {
		return nil
	}
	holding := saga.FeatureOf(path)
	if holding == "" {
		return fmt.Errorf("%s belongs to the app's onboarding deck, not a feature; omit --feature", target)
	}
	return assertFeature(document.Root, feature, target, holding)
}

// checkItemRecord enforces what an Item may point at: onboarding Items
// explain a persona, feature, or story record; implementation Items explain code.
func checkItemRecord(document *saga.Saga, slide *saga.Slide, record string) error {
	onboarding := saga.FeatureOf(slide.Path) == ""
	if !onboarding {
		if record != "" {
			return fmt.Errorf("--record is for onboarding items; implementation items explain code")
		}
		return nil
	}
	if record == "" {
		return fmt.Errorf("--record is required: an onboarding item explains a persona, feature, or story URN")
	}
	return requireAppRecord(document.Root, document.Manifest.ID, record)
}

// requireReviewRecord checks that a review Item's record names an existing
// documentation record: a persona, feature, or story, or a deck, slide, chapter,
// section, or fragment of the living Saga.
func requireReviewRecord(document *saga.Saga, record string) error {
	prefix := "urn:change-saga:" + document.Manifest.ID + ":"
	for _, kind := range []string{"deck", "slide", "chapter", "section", "fragment"} {
		if strings.HasPrefix(record, prefix+kind+":") {
			found := false
			walkTargets(document.Root, document.Section, func(target, _ string, _ bool) {
				found = found || target == record
			})
			if !found {
				return fmt.Errorf("%s %q does not exist in the Saga", kind, record)
			}
			return nil
		}
	}
	if id, ok := strings.CutPrefix(record, prefix+"test-case:"); ok && applayout.ValidID(id) {
		return nil
	}
	return requireAppRecord(document.Root, document.Manifest.ID, record)
}

// requireAppRecord checks that a persona, feature, or story URN names an
// existing record.
func requireAppRecord(root, sagaID, record string) error {
	if id, ok := strings.CutPrefix(record, "urn:change-saga:"+sagaID+":feature:"); ok {
		features, err := applayout.Features(root)
		if err != nil {
			return err
		}
		if _, found := applayout.Find(features, id); !found {
			return fmt.Errorf("feature %q does not exist", record)
		}
		return nil
	}
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return err
	}
	if id, err := requirements.ParsePersonaURN(sagaID, record); err == nil {
		if document.FindPersona(id) == nil {
			return fmt.Errorf("persona %q does not exist", record)
		}
		return nil
	}
	if id, ok := strings.CutPrefix(record, "urn:change-saga:"+sagaID+":story:"); ok && applayout.ValidID(id) {
		if document.FindStory(id) == nil {
			return fmt.Errorf("story %q does not exist", record)
		}
		return nil
	}
	return fmt.Errorf("record %q must be a canonical persona, feature, or story URN of this Saga", record)
}

func findDeck(document *saga.Saga, value string) *saga.Deck {
	for _, deck := range allDecks(document) {
		if value == deck.ID || value == deck.Target || filepath.Clean(value) == filepath.Clean(deck.Path) || filepath.Clean(value) == filepath.Clean(deck.Directory) {
			return deck
		}
	}
	return nil
}

func findSlide(document *saga.Saga, value string) *saga.Slide {
	for _, deck := range allDecks(document) {
		for _, slide := range deck.Slides {
			if value == slide.ID || value == slide.Target || filepath.Clean(value) == filepath.Clean(slide.Path) {
				return slide
			}
		}
	}
	return nil
}

func defaultSlideSVG(title, takeaway string) string {
	title = html.EscapeString(title)
	takeaway = html.EscapeString(takeaway)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720" role="img" aria-labelledby="slide-title slide-desc">
  <title id="slide-title">%s</title>
  <desc id="slide-desc">%s</desc>
  <rect width="1280" height="720" fill="#f7f7f4"/>
  <text x="80" y="112" font-family="system-ui, sans-serif" font-size="48" font-weight="700" fill="#171717">%s</text>
  <text x="80" y="650" font-family="system-ui, sans-serif" font-size="28" fill="#444">%s</text>
</svg>
`, title, takeaway, title, takeaway)
}
