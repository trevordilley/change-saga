package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

// entityEdit changes display titles or removes unused current-snapshot records.
// Immutable story/persona/inventory records keep their revision and lifecycle APIs.
func entityEdit(kind string, args []string, out io.Writer) error {
	name := kind + " " + args[0]
	return contentEditCommand(name, args[1:], out, func() error {
		flags, dryRun, jsonOutput := contentEditFlags(name, out)
		selectorName := kind
		if kind == "feature-theme" {
			selectorName = "theme"
		}
		selector := flags.String(selectorName, "", "existing record ID or URN")
		var title string
		if args[0] == "rename" {
			flags.StringVar(&title, "title", "", "display title; stable identity is preserved")
		}
		if kind == "feature" && args[0] == "remove" {
			// Feature dispatches --with-content to featureRemoveWithContent
			// before reaching here; it is declared so help lists it.
			flags.Bool("with-content", false, "delete the feature with everything in it")
		}
		if err := flags.Parse(normalizeLivingArgs(args[1:])); err != nil {
			return err
		}
		if err := requireLivingArgs(flags, *selector); err != nil {
			return err
		}
		if args[0] == "rename" {
			title = strings.TrimSpace(title)
			if title == "" || len(title) > 1000 || strings.ContainsAny(title, "\r\n\t") {
				return fmt.Errorf("--title must be a single-line name of at most 1000 bytes")
			}
		}
		return runContentEdit(contentEditRequest{operation: name, root: flags.Arg(0), dryRun: *dryRun, json: *jsonOutput,
			apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
				path, target, value, oldTitle, err := editableEntity(document.Root, document.Manifest.ID, kind, *selector, strings.TrimSpace(title))
				if err != nil {
					return err
				}
				result.Resource = target
				if args[0] == "rename" {
					if oldTitle == strings.TrimSpace(title) {
						result.Replayed = true
						return nil
					}
					result.Changed = []string{"title"}
					return edit.writeJSON(path, value)
				}
				removePath := path
				if kind == "feature" || kind == "feature-theme" {
					removePath = filepath.Dir(path)
					allowed := []string{filepath.Base(path)}
					if kind == "feature" {
						allowed = append(allowed, applayout.FeaturePlanName)
					}
					if err := requireMetadataOnly(removePath, allowed); err != nil {
						if kind == "feature" {
							return fmt.Errorf("%w; or pass --with-content to delete the feature with everything in it (preview with --dry-run)", err)
						}
						return err
					}
				}
				refs, err := entityReferences(document.Root, removePath, target)
				if err != nil {
					return err
				}
				if len(refs) > 0 {
					return fmt.Errorf("cannot remove %s: referenced by %s; update references explicitly first", target, strings.Join(refs, ", "))
				}
				result.Removed = []string{target}
				// Record precisely which metadata files the directory removal accounts for.
				if removePath != path {
					err := filepath.WalkDir(removePath, func(p string, d fs.DirEntry, err error) error {
						if err != nil {
							return err
						}
						if !d.IsDir() {
							result.Paths = append(result.Paths, edit.relative(p))
						}
						return nil
					})
					if err != nil {
						return err
					}
				}
				return edit.remove(removePath)
			},
		}, out)
	})
}

// editableEntity resolves only owner-checked IDs, reads strict regular records,
// and copies the full snapshot so a title edit preserves all other metadata.
func editableEntity(root, sagaID, kind, selector, title string) (path, target string, value any, oldTitle string, err error) {
	switch kind {
	case "feature":
		var feature applayout.Feature
		feature, err = applayout.Require(root, sagaID, selector)
		if err != nil {
			return
		}
		path, target = filepath.Join(feature.Dir, applayout.FeatureManifestName), applayout.FeatureURN(sagaID, feature.ID)
		record := feature.FeatureManifest
		oldTitle = record.Title
		if title != "" {
			record.Title = title
		}
		err = applayout.ValidateFeatureManifest(record)
		value = record
	case "feature-theme":
		id, ok := applayout.FeatureThemeID(sagaID, selector)
		if !ok {
			err = fmt.Errorf("invalid theme ID or cross-Saga URN %q", selector)
			return
		}
		var themes []applayout.FeatureTheme
		themes, err = applayout.FeatureThemes(root, sagaID)
		if err != nil {
			return
		}
		for _, record := range themes {
			if record.ID != id {
				continue
			}
			path, target = filepath.Join(root, applayout.FeatureThemeRel(id), "theme.json"), applayout.FeatureThemeURN(sagaID, id)
			oldTitle = record.Title
			if title != "" {
				record.Title = title
			}
			var features []applayout.Feature
			features, err = applayout.Features(root)
			if err == nil {
				err = applayout.ValidateFeatureTheme(record, sagaID, features)
			}
			value = record
			return
		}
		err = fmt.Errorf("feature theme %q does not exist", id)
	case "label", "milestone":
		var catalog applayout.Classifications
		catalog, err = applayout.ReadClassifications(root)
		if err != nil {
			return
		}
		var record applayout.Classification
		record, err = catalog.Resolve(sagaID, kind, selector)
		if err != nil {
			return
		}
		path, target = filepath.Join(root, applayout.ClassificationDir(kind), record.ID+".json"), applayout.ClassificationURN(sagaID, kind, record.ID)
		value, oldTitle = record, record.Title
	default:
		err = fmt.Errorf("unsupported entity kind %q", kind)
	}
	return
}

func requireMetadataOnly(dir string, allowed []string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || filepath.Dir(path) != dir || !slices.Contains(allowed, entry.Name()) {
			return fmt.Errorf("cannot remove a record containing authored content or history: %s; move or remove that content explicitly first", path)
		}
		return nil
	})
}

// Check all JSON snapshots (including immutable revisions and review records)
// and Markdown links. Decode JSON first so escaped URNs cannot evade the check.
// Fail closed on unreadable records or symlinks; never follow them out of root.
func entityReferences(root, removed, target string) ([]string, error) {
	refs := []string{}
	pattern := regexp.MustCompile(regexp.QuoteMeta(target) + `($|[^A-Za-z0-9._-])`)
	var references func(any) bool
	references = func(value any) bool {
		switch v := value.(type) {
		case string:
			return pattern.MatchString(v)
		case []any:
			for _, child := range v {
				if references(child) {
					return true
				}
			}
		case map[string]any:
			for key, child := range v {
				if pattern.MatchString(key) || references(child) {
					return true
				}
			}
		}
		return false
	}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == removed {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cannot check dependencies through symlink %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".md" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return fmt.Errorf("cannot check dependencies in non-regular or oversized record %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		found := false
		if ext == ".json" {
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				return fmt.Errorf("cannot check dependencies in %s: %w", path, err)
			}
			found = references(value)
		} else {
			found = pattern.Match(data)
		}
		if found {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			refs = append(refs, filepath.ToSlash(rel))
		}
		return nil
	})
	return refs, err
}
