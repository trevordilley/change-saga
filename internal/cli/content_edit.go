package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// Content records (decks, slides, Items, chapters, sections, and fragments)
// are documentation, edited in place. A revise rewrites the fields it is
// given and keeps the record's identity; a remove deletes the record and
// everything it contains. Neither may leave the Saga invalid: the edit is
// applied under the Saga lock, the Saga is validated, and any new error puts
// every file back.

// contentEditOutput is the --json result of every revise-* and remove-*
// command.
type contentEditOutput struct {
	OK        bool   `json:"ok"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
	DryRun    bool   `json:"dry_run"`
	// Replayed is true when a revise asked for what the record already
	// holds, so nothing was written. Repeating a revise is always safe.
	Replayed bool `json:"replayed"`
	// Changed names the fields a revise changed.
	Changed []string `json:"changed"`
	// Removed lists every record a remove deleted: the target and what it
	// contained.
	Removed []string `json:"removed"`
	// Paths are the Saga-relative files written or removed.
	Paths []string `json:"paths"`
	// DanglingRelations are active relations with an endpoint the remove
	// deleted; status reports them stale until they are superseded.
	DanglingRelations []string `json:"dangling_relations"`
	// Error is set, with ok false, when the edit was refused.
	Error *contentEditError `json:"error,omitempty"`
}

type contentEditError struct {
	Message string `json:"message"`
}

// contentEdit is one edit's file operations, recorded so they can be undone.
// Removed files move to a directory beside the Saga and are deleted only
// once the edit is kept. A dry run records the paths and writes nothing.
type contentEdit struct {
	root   string
	dryRun bool
	trash  string
	undo   []func() error
	paths  []string
}

func (edit *contentEdit) relative(path string) string {
	rel, err := filepath.Rel(edit.root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (edit *contentEdit) writeJSON(path string, value any) error {
	if edit.dryRun {
		edit.paths = append(edit.paths, edit.relative(path))
		return nil
	}
	previous, readErr := os.ReadFile(path)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}
	if err := store.WriteJSON(path, value, readErr != nil); err != nil {
		return err
	}
	edit.paths = append(edit.paths, edit.relative(path))
	edit.undo = append(edit.undo, func() error {
		if readErr != nil {
			return os.Remove(path)
		}
		return store.WriteFile(path, previous, 0o644, false)
	})
	return nil
}

func (edit *contentEdit) remove(path string) error {
	if edit.dryRun {
		edit.paths = append(edit.paths, edit.relative(path))
		return nil
	}
	if edit.trash == "" {
		trash, err := os.MkdirTemp(filepath.Dir(edit.root), ".change-saga-remove-")
		if err != nil {
			return err
		}
		edit.trash = trash
	}
	parked := filepath.Join(edit.trash, fmt.Sprintf("%d", len(edit.undo)))
	if err := os.Rename(path, parked); err != nil {
		return err
	}
	edit.paths = append(edit.paths, edit.relative(path))
	edit.undo = append(edit.undo, func() error { return os.Rename(parked, path) })
	return nil
}

func (edit *contentEdit) rollback() error {
	var failed error
	for index := len(edit.undo) - 1; index >= 0; index-- {
		if err := edit.undo[index](); err != nil && failed == nil {
			failed = err
		}
	}
	edit.discard()
	return failed
}

func (edit *contentEdit) discard() {
	if edit.trash != "" {
		_ = os.RemoveAll(edit.trash)
	}
}

// contentEditRequest is what one revise-* or remove-* invocation asks for.
type contentEditRequest struct {
	operation string
	root      string
	dryRun    bool
	json      bool
	// apply performs the edit on a freshly loaded Saga through edit, which
	// writes nothing in a dry run, and fills result.
	apply func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error
}

// runContentEdit applies one edit under the Saga lock and keeps it only when
// it introduces no validation error.
func runContentEdit(request contentEditRequest, out io.Writer) error {
	result := contentEditOutput{OK: true, Operation: request.operation, DryRun: request.dryRun, Changed: []string{}, Removed: []string{}, Paths: []string{}, DanglingRelations: []string{}}
	absRoot, err := filepath.Abs(request.root)
	if err != nil {
		return err
	}
	err = store.WithSagaLock(request.root, store.DefaultLockTimeout, func() error {
		document, before, err := saga.Load(request.root)
		if err != nil {
			return err
		}
		edit := &contentEdit{root: absRoot, dryRun: request.dryRun}
		err = request.apply(document, edit, &result)
		if err == nil && !request.dryRun {
			var after saga.Validation
			if _, after, err = saga.Load(request.root); err == nil {
				err = newValidationErrors(before, after)
			}
		}
		if err != nil {
			if rollbackErr := edit.rollback(); rollbackErr != nil {
				return fmt.Errorf("%w; restoring the Saga also failed: %v", err, rollbackErr)
			}
			return err
		}
		result.Paths = append(result.Paths, edit.paths...)
		edit.discard()
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(result.Paths)
	return writeContentEdit(out, result, request.root, request.json)
}

// newValidationErrors refuses an edit that makes the Saga invalid in a way it
// was not before.
func newValidationErrors(before, after saga.Validation) error {
	existing := map[string]bool{}
	for _, issue := range before.Issues {
		existing[issue.Severity+"\x00"+issue.Path+"\x00"+issue.Message] = true
	}
	var introduced []string
	for _, issue := range after.Issues {
		if issue.Severity == "error" && !existing[issue.Severity+"\x00"+issue.Path+"\x00"+issue.Message] {
			introduced = append(introduced, issue.Path+": "+issue.Message)
		}
	}
	if len(introduced) == 0 {
		return nil
	}
	return fmt.Errorf("the edit would make the Saga invalid, so nothing was changed:\n  %s", strings.Join(introduced, "\n  "))
}

func writeContentEdit(out io.Writer, result contentEditOutput, root string, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, result)
	}
	switch {
	case strings.HasPrefix(result.Operation, "remove-"):
		verb := "Removed"
		if result.DryRun {
			verb = "Would remove"
		}
		fmt.Fprintf(out, "%s %s\n", verb, result.Resource)
		for _, removed := range result.Removed {
			if removed != result.Resource {
				fmt.Fprintf(out, "  and %s\n", removed)
			}
		}
		if len(result.DanglingRelations) > 0 {
			fmt.Fprintf(out, "These relations now point at a removed record; status reports them stale until you supersede them:\n")
			for _, relation := range result.DanglingRelations {
				fmt.Fprintf(out, "  change-saga relation supersede --relation %s %s\n", relation, root)
			}
		}
	case result.Replayed:
		fmt.Fprintf(out, "Unchanged %s: it already holds what was asked\n", result.Resource)
	default:
		verb := "Revised"
		if result.DryRun {
			verb = "Would revise"
		}
		fmt.Fprintf(out, "%s %s: %s\n", verb, result.Resource, strings.Join(result.Changed, ", "))
	}
	return nil
}

// contentEditFlags declares the flags every revise-* and remove-* shares.
func contentEditFlags(name string, out io.Writer) (*flag.FlagSet, *bool, *bool) {
	flags := commandFlags(name, commandUsage[name], out)
	dryRun := flags.Bool("dry-run", false, "report what would change without writing")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable result")
	return flags, dryRun, jsonOutput
}

// contentEditCommand runs a revise-* or remove-* command so a --json failure
// is itself one JSON value.
func contentEditCommand(name string, args []string, out io.Writer, run func() error) error {
	err := run()
	if err == nil || errors.Is(err, flag.ErrHelp) || !jsonFlagRequested(args) {
		return err
	}
	failure := contentEditOutput{Operation: name, Changed: []string{}, Removed: []string{}, Paths: []string{}, DanglingRelations: []string{}, Error: &contentEditError{Message: err.Error()}}
	if writeErr := writeJSON(out, failure); writeErr != nil {
		return writeErr
	}
	return &StatusError{Code: 1}
}

// fieldChanges applies each requested field that differs from the record,
// in the order given, and returns the names of those that changed.
type fieldChange struct {
	name    string
	set     bool
	changed bool
	apply   func()
}

func applyFieldChanges(changes []fieldChange) []string {
	changed := []string{}
	for _, change := range changes {
		if change.set && change.changed {
			change.apply()
			changed = append(changed, change.name)
		}
	}
	return changed
}

func stringField(flags *flag.FlagSet, name string, value *string, target *string) fieldChange {
	return fieldChange{name: name, set: flagWasSet(flags, name), changed: *value != *target, apply: func() { *target = *value }}
}

func intField(flags *flag.FlagSet, name string, value *optionalInt, target *int) fieldChange {
	return fieldChange{name: name, set: value.set, changed: value.value != *target, apply: func() { *target = value.value }}
}

// danglingRelations lists the active relations with an endpoint in removed.
func danglingRelations(root, sagaID string, removed []string) []string {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return []string{}
	}
	gone := map[string]bool{}
	for _, target := range removed {
		gone[target] = true
	}
	result := []string{}
	for _, relation := range document.Relations {
		if relation.State == requirements.RelationActive && (gone[relation.From] || gone[relation.To]) {
			urn := "urn:change-saga:" + sagaID + ":relation:" + relation.ID
			result = append(result, urn)
		}
	}
	sort.Strings(result)
	return result
}

// deckRecords lists a deck's slide and Item targets.
func slideRecords(slide *saga.Slide) []string {
	records := []string{slide.Target}
	for _, item := range slide.Items {
		records = append(records, item.Target)
	}
	return records
}

func ReviseDeck(_ context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("revise-deck", out)
	deckValue := flags.String("deck", "", "deck path, id, or URN")
	title := flags.String("title", "", "deck title")
	objective := flags.String("objective", "", "one concise reviewer objective")
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative deck order")
	return contentEditCommand("revise-deck", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *deckValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["revise-deck"])
		}
		return runContentEdit(contentEditRequest{operation: "revise-deck", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				deck := findDeck(document, *deckValue)
				if deck == nil {
					return fmt.Errorf("--deck must identify an existing deck")
				}
				result.Resource = deck.Target
				manifest := deck.DeckManifest
				result.Changed = applyFieldChanges([]fieldChange{
					stringField(flags, "title", title, &manifest.Title),
					stringField(flags, "objective", objective, &manifest.Objective),
					intField(flags, "rank", &rank, &manifest.Rank),
				})
				return writeRankedRecord(edit, result, filepath.Join(document.Root, filepath.FromSlash(deck.Path)), manifest, func() (string, error) {
					return saga.FlatDeckFilename(deck.Target, manifest.Rank)
				})
			}}, out)
	})
}

// writeRankedRecord rewrites a flat deck, slide, or Item record whose
// filename encodes its rank, moving it when the rank changed.
func writeRankedRecord(edit *contentEdit, result *contentEditOutput, path string, manifest any, filename func() (string, error)) error {
	if len(result.Changed) == 0 {
		result.Replayed = true
		return nil
	}
	name, err := filename()
	if err != nil {
		return err
	}
	moved := filepath.Join(filepath.Dir(path), name)
	if moved != path {
		if err := edit.remove(path); err != nil {
			return err
		}
	}
	return edit.writeJSON(moved, manifest)
}

func RemoveDeck(_ context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("remove-deck", out)
	deckValue := flags.String("deck", "", "deck path, id, or URN")
	return contentEditCommand("remove-deck", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *deckValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["remove-deck"])
		}
		return runContentEdit(contentEditRequest{operation: "remove-deck", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				deck := findDeck(document, *deckValue)
				if deck == nil {
					return fmt.Errorf("--deck must identify an existing deck; a review's deck is removed with its review")
				}
				result.Resource = deck.Target
				result.Removed = append(result.Removed, deck.Target)
				for _, slide := range deck.Slides {
					result.Removed = append(result.Removed, slideRecords(slide)...)
				}
				result.DanglingRelations = danglingRelations(document.Root, document.Manifest.ID, result.Removed)
				return edit.remove(deck.Directory)
			}}, out)
	})
}

// findEditableSlide finds a slide by path, id, or URN, or with review a slide
// of that pull request review's deck.
func findEditableSlide(document *saga.Saga, review, value string) (*saga.Slide, error) {
	if review != "" {
		found, err := findReviewDeck(document, review)
		if err != nil {
			return nil, err
		}
		if slide := findReviewSlide(found, value); slide != nil {
			return slide, nil
		}
		return nil, fmt.Errorf("review %s has no slide %q", found.ID, value)
	}
	if slide := findSlide(document, value); slide != nil {
		return slide, nil
	}
	return nil, fmt.Errorf("--slide must identify an existing slide")
}

func slideDeck(document *saga.Saga, slide *saga.Slide) *saga.Deck {
	decks := allDecks(document)
	for _, review := range document.Reviews {
		if review.Deck != nil {
			decks = append(decks, review.Deck)
		}
	}
	for _, deck := range decks {
		for _, candidate := range deck.Slides {
			if candidate == slide {
				return deck
			}
		}
	}
	return nil
}

func ReviseSlide(_ context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("revise-slide", out)
	slideValue := flags.String("slide", "", "slide path, id, or URN")
	review := flags.String("review", "", "the pull request review whose slide this is")
	title := flags.String("title", "", "slide title")
	section := flags.String("section", "", "section label shown as a subtle divider inside the deck; empty clears it")
	intent := flags.String("intent", "", "reviewer job: orient, explain, compare, trace, prove, risk, or conclude")
	layout := flags.String("layout", "", "canvas arrangement: hero, diagram, before-after, sequence, evidence, risk, or custom")
	takeaway := flags.String("takeaway", "", "single reviewer takeaway (maximum 180 characters)")
	rationale := flags.String("exception-rationale", "", "reason for a custom layout")
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative review order")
	return contentEditCommand("revise-slide", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *slideValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["revise-slide"])
		}
		return runContentEdit(contentEditRequest{operation: "revise-slide", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				slide, err := findEditableSlide(document, *review, *slideValue)
				if err != nil {
					return err
				}
				if err := guardCompleteSlideMutation("revise-slide", slide); err != nil {
					return err
				}
				deck := slideDeck(document, slide)
				result.Resource = slide.Target
				manifest := slide.SlideManifest
				result.Changed = applyFieldChanges([]fieldChange{
					stringField(flags, "title", title, &manifest.Title),
					stringField(flags, "section", section, &manifest.Section),
					stringField(flags, "intent", intent, &manifest.Intent),
					stringField(flags, "layout", layout, &manifest.Layout),
					stringField(flags, "takeaway", takeaway, &manifest.Takeaway),
					stringField(flags, "exception-rationale", rationale, &manifest.ExceptionRationale),
					intField(flags, "rank", &rank, &manifest.Rank),
				})
				path := filepath.Join(document.Root, filepath.FromSlash(slide.Path))
				if len(result.Changed) == 0 || manifest.Rank == slide.Rank {
					return writeRankedRecord(edit, result, path, manifest, func() (string, error) { return filepath.Base(path), nil })
				}
				// The asset's name follows the manifest's, so a new rank moves
				// both.
				name, err := saga.FlatSlideFilename(deck.Target, slide.Target, manifest.Rank)
				if err != nil {
					return err
				}
				asset, err := saga.FlatSlideAssetFilename(name, filepath.Ext(slide.Entrypoint))
				if err != nil {
					return err
				}
				data, err := os.ReadFile(filepath.Join(slide.Directory, slide.Entrypoint))
				if err != nil {
					return err
				}
				if err := edit.remove(filepath.Join(slide.Directory, slide.Entrypoint)); err != nil {
					return err
				}
				if err := edit.writeFile(filepath.Join(slide.Directory, asset), data); err != nil {
					return err
				}
				manifest.Entrypoint = asset
				return writeRankedRecord(edit, result, path, manifest, func() (string, error) { return name, nil })
			}}, out)
	})
}

func (edit *contentEdit) writeFile(path string, data []byte) error {
	if edit.dryRun {
		edit.paths = append(edit.paths, edit.relative(path))
		return nil
	}
	if err := store.WriteFile(path, data, 0o644, true); err != nil {
		return err
	}
	edit.paths = append(edit.paths, edit.relative(path))
	edit.undo = append(edit.undo, func() error { return os.Remove(path) })
	return nil
}

func RemoveSlide(_ context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("remove-slide", out)
	slideValue := flags.String("slide", "", "slide path, id, or URN")
	return contentEditCommand("remove-slide", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *slideValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["remove-slide"])
		}
		return runContentEdit(contentEditRequest{operation: "remove-slide", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				slide := findSlide(document, *slideValue)
				if slide == nil {
					return fmt.Errorf("--slide must identify an existing slide; a review's slides carry its approvals and comments, so they are not removed")
				}
				if err := guardCompleteSlideMutation("remove-slide", slide); err != nil {
					return err
				}
				result.Resource = slide.Target
				result.Removed = slideRecords(slide)
				result.DanglingRelations = danglingRelations(document.Root, document.Manifest.ID, result.Removed)
				files := []string{filepath.Join(document.Root, filepath.FromSlash(slide.Path)), filepath.Join(slide.Directory, slide.Entrypoint)}
				for _, item := range slide.Items {
					files = append(files, itemFiles(document.Root, item)...)
				}
				return removeFiles(edit, files)
			}}, out)
	})
}

// itemFiles are an Item's record and its code evidence files.
func itemFiles(root string, item *saga.Item) []string {
	files := []string{filepath.Join(root, filepath.FromSlash(item.Path))}
	for _, code := range item.Code {
		files = append(files, filepath.Join(root, filepath.FromSlash(code.Path)))
	}
	return files
}

func removeFiles(edit *contentEdit, files []string) error {
	for _, file := range files {
		if err := edit.remove(file); err != nil {
			return err
		}
	}
	return nil
}

// findEditableItem finds an Item by URN, or by id on the slide --slide names.
func findEditableItem(document *saga.Saga, review, slideValue, value string) (*saga.Slide, *saga.Item, error) {
	var slides []*saga.Slide
	if slideValue != "" || review != "" {
		if slideValue == "" {
			return nil, nil, fmt.Errorf("--review needs --slide to name the review slide that holds the Item")
		}
		slide, err := findEditableSlide(document, review, slideValue)
		if err != nil {
			return nil, nil, err
		}
		slides = []*saga.Slide{slide}
	} else {
		for _, deck := range allDecks(document) {
			slides = append(slides, deck.Slides...)
		}
	}
	for _, slide := range slides {
		for _, item := range slide.Items {
			if value == item.Target || value == item.ID && slideValue != "" || filepath.Clean(value) == filepath.Clean(item.Path) {
				return slide, item, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("--item must identify an existing Item by URN, or by id with --slide")
}

func ReviseItem(ctx context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("revise-item", out)
	itemValue := flags.String("item", "", "Item URN, or its id with --slide")
	slideValue := flags.String("slide", "", "slide holding the Item, when --item is an id")
	review := flags.String("review", "", "the pull request review whose slide holds the Item")
	kind := flags.String("kind", "", "node, edge, region, transition, statement, risk, metric, example, or callout")
	label := flags.String("label", "", "concise reviewer-facing label")
	description := flags.String("description", "", "non-visual semantic description")
	elementID := flags.String("element-id", "", "id of an SVG or HTML element")
	region := flags.String("region", "", "normalized image region x,y,width,height")
	hotspot := flags.String("hotspot", "", "normalized on-canvas hit area; empty clears it")
	about := flags.String("about", "", "for callouts, another item id on this slide")
	body := flags.String("body", "", "callout body")
	placement := flags.String("placement", "", "top, right, bottom, left, or overlay")
	leader := flags.String("leader", "", "none, line, or arrow")
	documentation := flags.String("documentation", "", "Component/System URN; empty clears the link with empty revision")
	documentationRevision := flags.String("documentation-revision", "", "exact canonical definition revision URN")
	documentationView := flags.String("documentation-view", "", "saved Saga commit that admits a non-current pin; empty clears it")
	selectionsFile := flags.String("selections", "", "JSON array replacing the Item's code selections; [] clears them")
	repo := flags.String("repo", "", "source checkout for saved views and selections")
	record := flags.String("record", "", "the record the item points at")
	var rank optionalInt
	flags.Var(&rank, "rank", "non-negative item order")
	return contentEditCommand("revise-item", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *itemValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["revise-item"])
		}
		if *elementID != "" && *region != "" {
			return fmt.Errorf("provide at most one selector: --element-id or --region")
		}
		return runContentEdit(contentEditRequest{operation: "revise-item", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				slide, item, err := findEditableItem(document, *review, *slideValue, *itemValue)
				if err != nil {
					return err
				}
				if err := guardCompleteSlideMutation("revise-item", slide); err != nil {
					return err
				}
				result.Resource = item.Target
				manifest := item.ItemManifest
				if flagWasSet(flags, "record") && *record != "" && *review == "" {
					if err := checkItemRecord(document, slide, *record); err != nil {
						return err
					}
				}
				changes := []fieldChange{
					stringField(flags, "kind", kind, &manifest.Kind),
					stringField(flags, "label", label, &manifest.Label),
					stringField(flags, "description", description, &manifest.Description),
					stringField(flags, "about", about, &manifest.About),
					stringField(flags, "body", body, &manifest.Body),
					stringField(flags, "placement", placement, &manifest.Placement),
					stringField(flags, "leader", leader, &manifest.Leader),
					stringField(flags, "record", record, &manifest.Record),
					intField(flags, "rank", &rank, &manifest.Rank),
				}
				if flagWasSet(flags, "documentation") || flagWasSet(flags, "documentation-revision") || flagWasSet(flags, "documentation-view") || flagWasSet(flags, "selections") {
					pin, view, selections := manifest.Documentation, manifest.DocumentationView, manifest.Selections
					if flagWasSet(flags, "documentation") || flagWasSet(flags, "documentation-revision") {
						if !flagWasSet(flags, "documentation") || !flagWasSet(flags, "documentation-revision") {
							return fmt.Errorf("provide both --documentation and --documentation-revision")
						}
						pin = nil
						if *documentation != "" || *documentationRevision != "" {
							pin = &saga.DocumentationLink{Target: *documentation, Revision: *documentationRevision}
						}
						if !reflect.DeepEqual(pin, manifest.Documentation) {
							// A new pin is admitted on its own terms; a view or
							// selections from the old pin never carry over silently.
							view, selections = "", nil
						}
					}
					if flagWasSet(flags, "documentation-view") {
						view = *documentationView
					}
					if flagWasSet(flags, "selections") {
						requested, err := readItemSelections(*selectionsFile)
						if err != nil {
							return err
						}
						selections = requested
					}
					previous := item.ItemManifest
					resolvedView, prepared, err := prepareItemInventoryLinks(ctx, document.Root, document.Manifest, itemInventoryLinks{Documentation: pin, View: view, Selections: selections, Previous: &previous, Checkout: *repo})
					if err != nil {
						return err
					}
					changed := !reflect.DeepEqual(pin, manifest.Documentation) || resolvedView != manifest.DocumentationView || !reflect.DeepEqual(prepared, manifest.Selections)
					changes = append(changes, fieldChange{name: "documentation", set: true, changed: changed, apply: func() {
						manifest.Documentation, manifest.DocumentationView, manifest.Selections = pin, resolvedView, prepared
					}})
				}
				if *elementID != "" || *region != "" {
					selector := saga.LandmarkSelector{Type: "element", ElementID: *elementID}
					if *region != "" {
						parsed, err := parseLandmarkRegion(*region)
						if err != nil {
							return fmt.Errorf("invalid --region: %w", err)
						}
						selector = saga.LandmarkSelector{Type: "region", X: parsed.X, Y: parsed.Y, Width: parsed.Width, Height: parsed.Height}
					}
					changes = append(changes, fieldChange{name: "selector", set: true, changed: selector != manifest.Selector, apply: func() { manifest.Selector = selector }})
				}
				if flagWasSet(flags, "hotspot") {
					var parsed *saga.LandmarkRegion
					if *hotspot != "" {
						value, err := parseLandmarkRegion(*hotspot)
						if err != nil {
							return fmt.Errorf("invalid --hotspot: %w", err)
						}
						parsed = &value
					}
					changed := (parsed == nil) != (manifest.Hotspot == nil) || parsed != nil && *parsed != *manifest.Hotspot
					changes = append(changes, fieldChange{name: "hotspot", set: true, changed: changed, apply: func() { manifest.Hotspot = parsed }})
				}
				result.Changed = applyFieldChanges(changes)
				return writeRankedRecord(edit, result, filepath.Join(document.Root, filepath.FromSlash(item.Path)), manifest, func() (string, error) {
					return saga.FlatItemFilename(slide.Target, item.Target, manifest.Rank)
				})
			}}, out)
	})
}

func RemoveItem(_ context.Context, args []string, out io.Writer) error {
	flags, dryRun, jsonOutput := contentEditFlags("remove-item", out)
	itemValue := flags.String("item", "", "Item URN, or its id with --slide")
	slideValue := flags.String("slide", "", "slide holding the Item, when --item is an id")
	return contentEditCommand("remove-item", args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *itemValue == "" {
			return fmt.Errorf("usage: %s", commandUsage["remove-item"])
		}
		return runContentEdit(contentEditRequest{operation: "remove-item", root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				slide, item, err := findEditableItem(document, "", *slideValue, *itemValue)
				if err != nil {
					return err
				}
				if err := guardCompleteSlideMutation("remove-item", slide); err != nil {
					return err
				}
				for _, other := range slide.Items {
					if other.Kind == "callout" && other.About == item.ID {
						return fmt.Errorf("callout %s is about %s; revise its --about or remove it first", other.Target, item.ID)
					}
				}
				result.Resource = item.Target
				result.Removed = []string{item.Target}
				result.DanglingRelations = danglingRelations(document.Root, document.Manifest.ID, result.Removed)
				if err := removeFiles(edit, itemFiles(document.Root, item)); err != nil {
					return err
				}
				// The slide lists every Item in its reading order.
				manifest := slide.SlideManifest
				manifest.ReadingOrder = []string{}
				for _, id := range slide.ReadingOrder {
					if id != item.ID {
						manifest.ReadingOrder = append(manifest.ReadingOrder, id)
					}
				}
				return edit.writeJSON(filepath.Join(document.Root, filepath.FromSlash(slide.Path)), manifest)
			}}, out)
	})
}

// narrativeKinds are the directory records of chapters, sections, and
// fragments: the manifest each holds and the URN kind that names it.
var narrativeKinds = map[string]struct{ manifest, urn string }{
	"chapter":  {"chapter.json", ":chapter:"},
	"section":  {"section.json", ":section:"},
	"fragment": {"fragment.json", ":fragment:"},
}

// resolveNarrative finds a chapter, section, or fragment directory.
func resolveNarrative(document *saga.Saga, kind, value string) (string, string, error) {
	dir, target, err := resolveTarget(document, value, kind == "fragment")
	if err != nil {
		return "", "", err
	}
	if !strings.Contains(target, narrativeKinds[kind].urn) {
		return "", "", fmt.Errorf("target %q is not a %s", value, kind)
	}
	return dir, target, nil
}

func reviseNarrative(kind string, args []string, out io.Writer) error {
	name := "revise-" + kind
	flags, dryRun, jsonOutput := contentEditFlags(name, out)
	targetValue := flags.String("target", "", kind+" path, id, or URN")
	title := flags.String("title", "", kind+" title")
	var order optionalInt
	flags.Var(&order, "order", "display order")
	return contentEditCommand(name, args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *targetValue == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				dir, target, err := resolveNarrative(document, kind, *targetValue)
				if err != nil {
					return err
				}
				result.Resource = target
				path := filepath.Join(dir, narrativeKinds[kind].manifest)
				// Chapters, sections, and fragments share id, title, and order;
				// a fragment's media type and entrypoint are carried through.
				var manifest saga.FragmentManifest
				if err := readJSONFile(path, &manifest); err != nil {
					return err
				}
				result.Changed = applyFieldChanges([]fieldChange{
					stringField(flags, "title", title, &manifest.Title),
					intField(flags, "order", &order, &manifest.Order),
				})
				if len(result.Changed) == 0 {
					result.Replayed = true
					return nil
				}
				switch kind {
				case "chapter":
					return edit.writeJSON(path, saga.ChapterManifest{Version: manifest.Version, ID: manifest.ID, Title: manifest.Title, Order: manifest.Order})
				case "section":
					return edit.writeJSON(path, saga.SectionManifest{Version: manifest.Version, ID: manifest.ID, Title: manifest.Title, Order: manifest.Order})
				}
				return edit.writeJSON(path, manifest)
			}}, out)
	})
}

func removeNarrative(kind string, args []string, out io.Writer) error {
	name := "remove-" + kind
	flags, dryRun, jsonOutput := contentEditFlags(name, out)
	targetValue := flags.String("target", "", kind+" path, id, or URN")
	return contentEditCommand(name, args, out, func() error {
		if err := flags.Parse(args); err != nil {
			return err
		}
		if flags.NArg() != 1 || *targetValue == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				dir, target, err := resolveNarrative(document, kind, *targetValue)
				if err != nil {
					return err
				}
				result.Resource = target
				removedDir, _ := filepath.Abs(dir)
				walkTargets(document.Root, document.Section, func(candidate, candidateDir string, _ bool) {
					if abs, _ := filepath.Abs(candidateDir); pathWithin(removedDir, abs) && !contains(result.Removed, candidate) {
						result.Removed = append(result.Removed, candidate)
					}
				})
				if !contains(result.Removed, target) {
					result.Removed = append([]string{target}, result.Removed...)
				}
				result.DanglingRelations = danglingRelations(document.Root, document.Manifest.ID, result.Removed)
				return edit.remove(dir)
			}}, out)
	})
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func ReviseChapter(_ context.Context, args []string, out io.Writer) error {
	return reviseNarrative("chapter", args, out)
}

func RemoveChapter(_ context.Context, args []string, out io.Writer) error {
	return removeNarrative("chapter", args, out)
}

func ReviseSection(_ context.Context, args []string, out io.Writer) error {
	return reviseNarrative("section", args, out)
}

func RemoveSection(_ context.Context, args []string, out io.Writer) error {
	return removeNarrative("section", args, out)
}

func ReviseFragment(_ context.Context, args []string, out io.Writer) error {
	return reviseNarrative("fragment", args, out)
}

func RemoveFragment(_ context.Context, args []string, out io.Writer) error {
	return removeNarrative("fragment", args, out)
}
