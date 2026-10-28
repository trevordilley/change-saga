package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagaref"
	"github.com/twentyideas/changesaga/internal/store"
)

// move preserves the bytes of a record and its sidecars. Every rename is
// reversible until the complete edit has passed Saga validation.
func (edit *contentEdit) move(from, to string) error {
	if from == to {
		return nil
	}
	if !pathWithin(edit.root, from) || !pathWithin(edit.root, to) {
		return fmt.Errorf("move must stay inside the Saga")
	}
	if pathWithin(from, to) {
		return fmt.Errorf("cannot move a record inside itself")
	}
	if _, err := os.Lstat(to); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf("destination already exists: %s", edit.relative(to))
	}
	// Reject symlinks in either path, including ancestors, before any writes.
	for _, path := range []string{from, filepath.Dir(to)} {
		for current := path; current != edit.root; current = filepath.Dir(current) {
			info, err := os.Lstat(current)
			if errors.Is(err, fs.ErrNotExist) && path != from {
				continue
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("move path must not contain symlinks: %s", edit.relative(current))
			}
			if current != from && !info.IsDir() {
				return fmt.Errorf("move parent must be a directory: %s", edit.relative(current))
			}
		}
	}
	edit.paths = append(edit.paths, edit.relative(from), edit.relative(to))
	if edit.dryRun {
		return nil
	}
	// Track missing parents so rollback also restores directory structure.
	var missing []string
	for dir := filepath.Dir(to); dir != edit.root; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(dir); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		missing = append(missing, dir)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		dir := missing[i]
		if err := os.Mkdir(dir, 0o755); err != nil {
			return err
		}
		edit.undo = append(edit.undo, func() error { return os.Remove(dir) })
	}
	if err := os.Rename(from, to); err != nil {
		return err
	}
	edit.undo = append(edit.undo, func() error { return os.Rename(to, from) })
	return nil
}

// MoveDeck transfers an implementation bundle between features. Its stable
// targets and all immutable slide revisions remain byte-for-byte unchanged.
func MoveDeck(_ context.Context, args []string, out io.Writer) error {
	const name = "move-deck"
	flags, dryRun, jsonOutput := contentEditFlags(name, out)
	target := flags.String("deck", "", "implementation deck path, id, or URN")
	feature := flags.String("feature", "", "destination feature id or URN")
	return contentEditCommand(name, args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *target == "" || *feature == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput, apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
			deck := findDeck(document, *target)
			if deck == nil || (deck.Role != saga.DeckRoleChange && deck.Role != saga.DeckRoleUX) {
				return fmt.Errorf("--deck must identify an implementation or UX deck; review, onboarding, and theme ownership cannot change")
			}
			owner, err := requireFeature(document.Root, *feature)
			if err != nil {
				return err
			}
			destination := filepath.Join(owner.Dir, saga.EmbeddedSlidesDir, deck.ID+saga.EmbeddedDeckSuffix)
			result.Resource = deck.Target
			if destination == deck.Directory {
				result.Replayed = true
				return nil
			}
			result.Changed = []string{"parent"}
			return edit.move(deck.Directory, destination)
		}}, out)
	})
}

// MoveSlide moves an ordinary slide and every Item/evidence file together.
// Transaction slides require an immutable revision API and are refused here.
func MoveSlide(_ context.Context, args []string, out io.Writer) error {
	const name = "move-slide"
	flags, dryRun, jsonOutput := contentEditFlags(name, out)
	target := flags.String("slide", "", "slide path, id, or URN")
	parent := flags.String("deck", "", "destination implementation deck")
	return contentEditCommand(name, args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *target == "" || *parent == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput, apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
			source, slide, owner, err := findAuthoredSlide(document, "", *target)
			if err != nil {
				return err
			}
			destination := findDeck(document, *parent)
			if owner != "" || source == nil || destination == nil || source.Role != destination.Role || (source.Role != saga.DeckRoleChange && source.Role != saga.DeckRoleUX) {
				return fmt.Errorf("move-slide requires two implementation decks or two UX decks; review, onboarding, and theme ownership cannot change")
			}
			result.Resource = slide.Target
			if source.Target == destination.Target {
				result.Replayed = true
				return nil
			}
			if transactionManagedSlide(slide) {
				return fmt.Errorf("cannot move %s: immutable transaction history fixes every revision to deck %s; move-deck can transfer the complete deck between features without changing that history", slide.Target, source.ID)
			}
			if err := guardDeckReferences(source, slide.ID, ""); err != nil {
				return err
			}
			filename, err := saga.FlatSlideFilename(destination.Target, slide.Target, slide.Rank)
			if err != nil {
				return err
			}
			asset, err := saga.FlatSlideAssetFilename(filename, filepath.Ext(slide.Entrypoint))
			if err != nil {
				return err
			}
			manifest := slide.SlideManifest
			manifest.DeckID = destination.ID
			manifest.Entrypoint = asset
			result.Changed = []string{"parent"}
			if err := edit.move(filepath.Join(document.Root, filepath.FromSlash(slide.Path)), filepath.Join(destination.Directory, filename)); err != nil {
				return err
			}
			if err := edit.writeJSON(filepath.Join(destination.Directory, filename), manifest); err != nil {
				return err
			}
			if err := edit.move(filepath.Join(slide.Directory, slide.Entrypoint), filepath.Join(destination.Directory, asset)); err != nil {
				return err
			}
			for _, item := range slide.Items {
				for _, path := range itemFiles(document.Root, item) {
					if err := edit.move(path, filepath.Join(destination.Directory, filepath.Base(path))); err != nil {
						return err
					}
				}
			}
			return nil
		}}, out)
	})
}

// narrativeParent accepts explicit existing physical report roots as well as
// stable chapter/section identifiers. It never invents a container or a target.
func narrativeParent(document *saga.Saga, value string) (dir string, root bool, err error) {
	for _, feature := range document.Features {
		featureDir := filepath.Join(document.Root, filepath.FromSlash(feature.Path))
		if value == feature.ID || value == feature.Target || value == feature.Path || filepath.Clean(value) == featureDir {
			return featureDir, true, nil
		}
		design := filepath.Join(featureDir, applayout.DesignDir)
		if value == filepath.ToSlash(filepath.Join(feature.Path, applayout.DesignDir)) || filepath.Clean(value) == design {
			return design, true, nil
		}
	}
	designSystem := filepath.Join(document.Root, applayout.DesignSystemDir)
	if value == "designsystem" || value == applayout.DesignSystemDir || filepath.Clean(value) == designSystem {
		return designSystem, true, nil
	}
	dir, target, err := resolveTarget(document, value, false)
	if err != nil {
		return "", false, err
	}
	identity, parseErr := sagaref.ParseTarget(target)
	if parseErr != nil || (identity.Kind != sagaref.TargetChapter && identity.Kind != sagaref.TargetSection) {
		return "", false, fmt.Errorf("parent must be a feature, design root, chapter, or section")
	}
	return dir, false, nil
}

func moveNarrative(kind string, args []string, out io.Writer) error {
	name := "move-" + kind
	flags, dryRun, jsonOutput := contentEditFlags(name, out)
	target := flags.String("target", "", kind+" path, id, or URN")
	parent := flags.String("parent", "", "destination feature, design root, chapter, or section")
	basename := flags.String("name", "", "new directory basename without a type suffix")
	return contentEditCommand(name, args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *target == "" || *parent == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput, apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
			dir, urn, err := resolveNarrative(document, kind, *target)
			if err != nil {
				return err
			}
			if saga.IsOverviewPath(edit.relative(dir)) {
				return fmt.Errorf("formal overview parts cannot be moved")
			}
			parentDir, isRoot, err := narrativeParent(document, *parent)
			if err != nil {
				return err
			}
			if kind == "chapter" && !isRoot {
				return fmt.Errorf("chapters must be direct children of a feature or design root")
			}
			if kind == "section" && isRoot {
				return fmt.Errorf("sections must belong to a chapter or section")
			}
			base := filepath.Base(dir)
			if *basename != "" {
				if store.Slug(*basename) != *basename {
					return fmt.Errorf("--name must be a lowercase hyphenated directory basename")
				}
				base = *basename
				if kind != "section" {
					base += "." + kind
				}
			}
			destination := filepath.Join(parentDir, base)
			result.Resource = urn
			if destination == dir {
				result.Replayed = true
				return nil
			}
			result.Changed = []string{"parent"}
			if base != filepath.Base(dir) {
				result.Changed = append(result.Changed, "name")
			}
			return edit.move(dir, destination)
		}}, out)
	})
}

func MoveChapter(_ context.Context, args []string, out io.Writer) error {
	return moveNarrative("chapter", args, out)
}
func MoveSection(_ context.Context, args []string, out io.Writer) error {
	return moveNarrative("section", args, out)
}
func MoveFragment(_ context.Context, args []string, out io.Writer) error {
	return moveNarrative("fragment", args, out)
}

// guardDeckReferences checks the same authored navigation rules before both
// real edits and dry runs. References are never silently removed or rewritten.
func guardDeckReferences(deck *saga.Deck, slideID, itemID string) error {
	if deck == nil || deck.Overview == nil {
		return nil
	}
	candidate := *deck
	candidate.Slides = nil
	for _, slide := range deck.Slides {
		if slide.ID != slideID {
			candidate.Slides = append(candidate.Slides, slide)
			continue
		}
		if itemID == "" {
			continue
		}
		revised := *slide
		revised.Items = nil
		for _, item := range slide.Items {
			if item.ID != itemID {
				revised.Items = append(revised.Items, item)
			}
		}
		candidate.Slides = append(candidate.Slides, &revised)
	}
	if err := newValidationErrors(saga.ValidateDeckOverview(deck), saga.ValidateDeckOverview(&candidate)); err != nil {
		return fmt.Errorf("deck overview references would become invalid; revise them with deck overview before moving or removing this record: %w", err)
	}
	return nil
}
