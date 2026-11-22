package currentrecord

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/store"
)

// Normalize updates legacy record packages when a writer encounters a Saga.
// Call under the Saga writer lock. Reads never call this function. All heads
// and reference conversions are checked before writing anything. Sources stay
// available until every current record and external pin has been published.
func Normalize(root string) error {
	pins, err := AuditPins(root)
	if err != nil {
		return err
	}
	if err := pins.prepare(); err != nil {
		return err
	}
	type pending struct {
		dir            string
		content, state any
	}
	records := []pending{}
	type assetCopy struct{ source, destination, packageDir string }
	assets := []assetCopy{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") && path != root {
			return filepath.SkipDir
		}
		kind := strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
		if filepath.Base(filepath.Dir(path)) == "___whiteboards" {
			kind = "whiteboard"
		}
		switch kind {
		case "whiteboard", "prototype", "test", "story", "persona", "flag", "term", "deprecation", "component", "system", "data-entity", "erd", "erd-overlay":
		default:
			return nil
		}
		if kind != "whiteboard" {
			identity := kind + ".json"
			if kind == "test" {
				identity = "test-case.json"
			}
			if _, err := os.Lstat(filepath.Join(path, identity)); os.IsNotExist(err) {
				return filepath.SkipDir
			} else if err != nil {
				return err
			}
		}
		current, err := Read(path)
		if err != nil {
			return err
		}
		if current != nil {
			records = append(records, pending{dir: path})
			return filepath.SkipDir
		}
		content, err := selectLegacyJSON(filepath.Join(path, "revisions"))
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var state json.RawMessage
		if kind != "prototype" && kind != "whiteboard" {
			state, err = selectLegacyJSON(filepath.Join(path, "events"))
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		if kind == "prototype" {
			var prototype struct {
				ID     string `json:"id"`
				Source struct {
					Kind          string `json:"kind"`
					ContentDigest string `json:"content_digest"`
				} `json:"source"`
			}
			if err := json.Unmarshal(content, &prototype); err != nil {
				return err
			}
			if prototype.Source.Kind == "html" {
				source := filepath.Join(path, "revisions", prototype.ID+".revision", "html")
				dest := filepath.Join(path, "assets", strings.TrimPrefix(prototype.Source.ContentDigest, "sha256:"), "html")
				assets = append(assets, assetCopy{source, dest, path})
			}
		}
		content, err = pins.Normalize(content)
		if err != nil {
			return err
		}
		var c, s any
		if err = json.Unmarshal(content, &c); err != nil {
			return err
		}
		if len(state) > 0 {
			if err = json.Unmarshal(state, &s); err != nil {
				return err
			}
		}
		records = append(records, pending{dir: path, content: c, state: s})
		return filepath.SkipDir
	})
	if err != nil {
		return err
	}
	for _, asset := range assets {
		if err := copyAssets(asset.source, asset.destination, asset.packageDir); err != nil {
			return err
		}
	}
	if err = pins.PersistReferences(); err != nil {
		return err
	}
	for _, record := range records {
		if record.content != nil {
			if err = Write(record.dir, record.content, record.state); err != nil {
				return err
			}
		}
	}
	for _, record := range records {
		if err = Cleanup(record.dir, "revisions", "events"); err != nil {
			return err
		}
	}
	return pins.cleanupRelations()
}

func selectLegacyJSON(dir string) (json.RawMessage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(entries) > 10000 {
		return nil, fmt.Errorf("too many legacy records in %s", dir)
	}
	graph := map[string][]string{}
	data := map[string]json.RawMessage{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".change-saga-write-") {
			continue
		}
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".revision") {
			value, err := os.ReadFile(filepath.Join(dir, entry.Name(), "revision.json"))
			if err != nil {
				return nil, err
			}
			var fields struct {
				ID      string   `json:"id"`
				Parents []string `json:"parents"`
			}
			if err := json.Unmarshal(value, &fields); err != nil {
				return nil, err
			}
			if fields.ID != strings.TrimSuffix(entry.Name(), ".revision") {
				return nil, fmt.Errorf("legacy prototype ID does not match package")
			}
			graph[fields.ID] = fields.Parents
			data[fields.ID] = value
			continue
		}
		if !entry.Type().IsRegular() || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("%s must contain regular legacy JSON files", dir)
		}
		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() > MaxBytes {
			return nil, fmt.Errorf("legacy record too large: %s", path)
		}
		value, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var fields struct {
			ID      string   `json:"id"`
			Parents []string `json:"parents"`
		}
		if err = json.Unmarshal(value, &fields); err != nil {
			return nil, err
		}
		if fields.ID != strings.TrimSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("legacy ID does not match %s", path)
		}
		graph[fields.ID] = fields.Parents
		data[fields.ID] = value
	}
	head, err := SelectLegacy(graph)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return data[head], nil
}

// WriteMember replaces content or state at the package containing a legacy
// member path. Domain writers use it after validation, under the Saga lock.
func WriteMember(path string, value any) error {
	dir := filepath.Dir(filepath.Dir(path))
	current, err := Read(dir)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("%s requires normalization before editing", dir)
	}
	content, state := any(current.Content), any(current.State)
	if filepath.Base(filepath.Dir(path)) == "events" {
		state = value
	} else {
		content = value
	}
	return Write(dir, content, state)
}

func copyAssets(source, destination, packageRoot string) error {
	directories := []string{}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("asset symlink %s is unsafe", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			directories = append(directories, target)
			return os.MkdirAll(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("asset must be regular: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if old, err := os.ReadFile(target); err == nil {
			if string(old) != string(data) {
				return fmt.Errorf("asset collision at %s", target)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		return store.WriteFile(target, data, 0644, true)
	})
	if err != nil {
		return err
	}
	// File publication syncs each leaf. Sync directory entries from the deepest
	// asset directories through the package before current.json can name them.
	for i := len(directories) - 1; i >= 0; i-- {
		if err := store.SyncDir(directories[i]); err != nil {
			return err
		}
	}
	for dir := filepath.Dir(destination); ; dir = filepath.Dir(dir) {
		if err := store.SyncDir(dir); err != nil {
			return err
		}
		if dir == packageRoot {
			break
		}
		if dir == filepath.Dir(dir) {
			return fmt.Errorf("asset path escapes package")
		}
	}
	return nil
}
