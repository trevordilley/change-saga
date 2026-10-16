package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func FeatureTheme(_ context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		return livingFamilyHelp("feature-theme", []string{"add", "rename", "remove", "set-overview", "set-features"}, out)
	}
	if args[0] == "rename" || args[0] == "remove" {
		return entityEdit("feature-theme", args, out)
	}
	err := featureThemeMutation(args, out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "feature-theme "+args[0], err)
	}
	return err
}

func featureThemeMutation(args []string, out io.Writer) error {
	op := args[0]
	if op != "add" && op != "set-overview" && op != "set-features" {
		return fmt.Errorf("usage: %s", commandUsage["feature-theme"])
	}
	name := "feature-theme " + op
	flags := commandFlags(name, commandUsage[name], out)
	var id, title, body, source string
	var members stringList
	if op == "add" {
		flags.StringVar(&id, "id", "", "stable product theme id")
		flags.StringVar(&title, "title", "", "product theme title")
	} else {
		flags.StringVar(&id, "theme", "", "product theme id or URN")
	}
	subtext := flags.String("subtext", "", "one-line essay subtext")
	abstract := flags.String("abstract", "", "Markdown abstract, separate from the essay body")
	if op != "set-features" {
		flags.StringVar(&body, "text", "", "authored Markdown overview and rationale")
		flags.StringVar(&source, "source", "", "Markdown file or - for standard input")
	}
	if op != "set-overview" {
		flags.Var(&members, "feature", "member feature ID or URN; repeat (omitted clears membership on set-features)")
	}
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args[1:])); err != nil {
		return err
	}
	if flags.NArg() != 1 || id == "" || (op == "add" && strings.TrimSpace(title) == "") {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	if source != "" && body != "" {
		return fmt.Errorf("--text and --source are mutually exclusive")
	}
	if source != "" {
		var reader io.Reader = os.Stdin
		if source != "-" {
			file, err := os.Open(source)
			if err != nil {
				return err
			}
			defer file.Close()
			reader = file
		}
		data, err := io.ReadAll(io.LimitReader(reader, 256*1024+1))
		if err != nil {
			return err
		}
		body = string(data)
	}
	if op != "set-features" && strings.TrimSpace(body) == "" {
		return fmt.Errorf("an authored Markdown overview is required (--text or --source)")
	}
	given := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
	if op == "set-features" && (given["subtext"] || given["abstract"]) {
		return fmt.Errorf("essay metadata belongs to set-overview")
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if op == "add" && !applayout.ValidID(id) {
		return fmt.Errorf("--id must be a stable identifier")
	}
	themeID, ok := applayout.FeatureThemeID(manifest.ID, id)
	if !ok {
		return fmt.Errorf("invalid theme ID or cross-Saga URN %q", id)
	}
	var replayed bool
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		themes, err := applayout.FeatureThemes(root, manifest.ID)
		if err != nil {
			return err
		}
		features, err := applayout.Features(root)
		if err != nil {
			return err
		}
		refs := []string{}
		for _, member := range members {
			fid, ok := applayout.FeatureFromURN(manifest.ID, member)
			if !ok {
				return fmt.Errorf("invalid or cross-Saga feature %q", member)
			}
			refs = append(refs, applayout.FeatureURN(manifest.ID, fid))
		}
		slices.Sort(refs)
		refs = slices.Compact(refs)
		var existing *applayout.FeatureTheme
		for i := range themes {
			if themes[i].ID == themeID {
				existing = &themes[i]
			}
		}
		if existing == nil && len(themes) >= applayout.MaxFeatures {
			return fmt.Errorf("maximum of %d feature themes reached", applayout.MaxFeatures)
		}
		value := applayout.FeatureTheme{Schema: applayout.FeatureThemeSchemaURL, Version: 5, ID: themeID, Title: strings.TrimSpace(title), Overview: body, Subtext: *subtext, Abstract: *abstract, Features: refs, CreatedAt: time.Now().UTC()}
		if op != "add" {
			if existing == nil {
				return fmt.Errorf("feature theme %q does not exist", themeID)
			}
			value = *existing
			if op == "set-overview" {
				value.Overview = body
				if given["subtext"] {
					value.Subtext = *subtext
				}
				if given["abstract"] {
					value.Abstract = *abstract
				}
			} else {
				value.Features = refs
			}
		}
		if err := applayout.ValidateFeatureTheme(value, manifest.ID, features); err != nil {
			return err
		}
		if existing != nil {
			value.CreatedAt = existing.CreatedAt
			before, _ := json.Marshal(existing)
			after, _ := json.Marshal(value)
			if string(before) == string(after) {
				replayed = true
				return nil
			}
			if op == "add" {
				return fmt.Errorf("feature theme %q already exists with different content", themeID)
			}
		}
		dir := filepath.Join(root, applayout.FeatureThemeRel(themeID))
		if existing == nil {
			return store.CommitDir(root, dir, func(stage string) error { return store.WriteJSON(filepath.Join(stage, "theme.json"), value, true) })
		}
		if _, err := store.EnsureDirWithin(root, dir); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(dir, "theme.json"), value, false)
	})
	if err != nil {
		return err
	}
	target := applayout.FeatureThemeURN(manifest.ID, themeID)
	return writeLivingMutation(out, name, target, applayout.FeatureThemeRel(themeID)+"/theme.json", []string{target}, nil, replayed, *jsonOutput)
}

// Theme discovery reads the bounded current authoring snapshot. Ordinary
// overview/children/slide queries expose its decks through the shared tree.
func queryFeatureThemes(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("query feature-themes", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("saga", "", "Saga root")
	selector := flags.String("theme", "", "product theme ID or URN")
	cursor := flags.String("cursor", "", "snapshot-bound continuation")
	limit := flags.Int("limit", 50, "page size, 1 to 100")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return writeQuerySuccess(out, "", queryHelpFor("feature-themes"), nil)
		}
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: err.Error()})
	}
	if *root == "" || flags.NArg() != 0 || *limit < 1 || *limit > 100 || len(*cursor) > 4096 {
		return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: queryUsage["feature-themes"]})
	}
	document, validation, err := saga.LoadOutline(*root)
	if err != nil {
		return writeQueryFailure(out, normalizeQueryError(err))
	}
	if !validation.Valid {
		return writeQueryFailure(out, &queryError{Code: "invalid_saga", Message: "Saga validation failed", Details: validation})
	}
	encoded, _ := json.Marshal(document.FeatureThemes)
	digest := sha256.Sum256(encoded)
	snapshot := fmt.Sprintf("sha256:%x", digest)
	themes := document.FeatureThemes
	if themes == nil {
		themes = []*saga.FeatureTheme{}
	}
	if *selector != "" {
		if _, ok := applayout.FeatureThemeID(document.Manifest.ID, *selector); !ok {
			return writeQueryFailure(out, &queryError{Code: "invalid_argument", Message: "invalid or cross-Saga theme selector"})
		}
		found := document.FindFeatureTheme(*selector)
		if found == nil {
			return writeQueryFailure(out, &queryError{Code: "not_found", Message: "feature theme does not exist"})
		}
		themes = []*saga.FeatureTheme{found}
	}
	key := "feature-themes:" + document.Manifest.ID + ":" + *selector
	offset, cursorErr := decodeTermsCursor(*cursor, key, snapshot, len(themes))
	if cursorErr != nil {
		return writeQueryFailure(out, cursorErr)
	}
	end := min(offset+*limit, len(themes))
	records := []featureThemeQueryRecord{}
	for _, theme := range themes[offset:end] {
		record := featureThemeQueryRecord{ID: theme.ID, Target: theme.Target, Title: theme.Title, Path: theme.Path, FeatureCount: len(theme.Features), Decks: []featureThemeDeckSummary{}}
		if *selector != "" {
			record.Overview = theme.Overview
			record.Subtext, record.Abstract = theme.Subtext, theme.Abstract
			record.Features = &theme.Features
		}
		for _, deck := range theme.Decks {
			record.Decks = append(record.Decks, featureThemeDeckSummary{ID: deck.ID, Target: deck.Target, Title: deck.Title, Slides: len(deck.Slides)})
		}
		records = append(records, record)
	}
	page := queryPageEnvelope{Total: len(themes), Returned: len(records), HasMore: end < len(themes)}
	if page.HasMore {
		next := encodeTermsCursor(key, snapshot, end)
		page.NextCursor = &next
	}
	return writeQuerySuccess(out, snapshot, map[string]any{"feature_themes": records}, &page)
}

// Lists carry compact discovery metadata; a selected theme includes its authored
// overview and memberships. Slides stay in the normal deck/slide query surface.
type featureThemeQueryRecord struct {
	Subtext      string                    `json:"subtext,omitempty"`
	Abstract     string                    `json:"abstract,omitempty"`
	ID           string                    `json:"id"`
	Target       string                    `json:"target"`
	Title        string                    `json:"title"`
	Path         string                    `json:"path"`
	FeatureCount int                       `json:"feature_count"`
	Overview     string                    `json:"overview,omitempty"`
	Features     *[]string                 `json:"features,omitempty"`
	Decks        []featureThemeDeckSummary `json:"decks"`
}
type featureThemeDeckSummary struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Title  string `json:"title"`
	Slides int    `json:"slide_count"`
}
