package currentrecord

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/store"
)

// Pins is an ephemeral index for converting legacy evidence pins. It is never
// saved as a second history store. A digest describes the content that was
// actually pinned, including pins within that content.
type assetReplacement struct {
	Path, Digest string
	Data         []byte
}

type Pins struct {
	prepared  bool
	relations map[string]json.RawMessage
	repinDirs []string
	assets    map[string]assetReplacement
	source    func(string) (json.RawMessage, error)
	root      string
	sources   map[string]json.RawMessage
	digests   map[string]string
	visiting  map[string]bool
	files     map[string]json.RawMessage
}

// AuditPins reads the Saga without writing. Missing pins and cyclic evidence
// dependencies fail before publication, so historical evidence cannot silently
// become an assertion about the current document.
func AuditPins(root string) (*Pins, error) {
	p := &Pins{root: root, sources: map[string]json.RawMessage{}, digests: map[string]string{}, visiting: map[string]bool{}, files: map[string]json.RawMessage{}}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("normalization does not follow symlink %s", path)
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		count++
		if count > 100000 {
			return fmt.Errorf("too many JSON records to normalize")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxBytes {
			return fmt.Errorf("%s is not a bounded regular JSON record", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			return nil
		}
		var schema string
		_ = json.Unmarshal(object["$schema"], &schema)
		base := filepath.Base(path)
		typed := strings.HasPrefix(schema, "https://changesaga.dev/schema/") || (len(base) > 3 && base[0] >= '0' && base[0] <= '9' && base[1] >= '0' && base[1] <= '9' && base[2] == '-')
		if !typed {
			return nil
		}
		if err := Decode(data, &object); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		p.files[path] = data
		if !strings.Contains(filepath.ToSlash(path), "/revisions/") {
			return nil
		}
		var id string
		_ = json.Unmarshal(object["id"], &id)
		if id == "" {
			return nil
		}
		var owner string
		for _, key := range []string{"story", "persona", "term", "flag", "deprecation", "record", "prototype", "test_case"} {
			if len(object[key]) != 0 {
				_ = json.Unmarshal(object[key], &owner)
				if owner != "" {
					break
				}
			}
		}
		if owner == "" {
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if len(parts) == 4 && parts[0] == "___whiteboards" {
				manifest := map[string]json.RawMessage{}
				data, e := os.ReadFile(filepath.Join(root, "saga.json"))
				if e != nil {
					return e
				}
				if e = json.Unmarshal(data, &manifest); e != nil {
					return e
				}
				var sagaID string
				_ = json.Unmarshal(manifest["id"], &sagaID)
				owner = "urn:change-saga:" + sagaID + ":whiteboard:" + parts[1]
			}
		}
		if owner != "" {
			pin := owner + ":revision:" + id
			if _, exists := p.sources[pin]; exists {
				return fmt.Errorf("duplicate legacy revision %s", pin)
			}
			p.sources[pin] = data
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Pins) prepare() error {
	if p.prepared {
		return nil
	}
	if err := p.prepareRelations(); err != nil {
		return err
	}
	if err := p.prepareAssets(); err != nil {
		return err
	}
	p.prepared = true
	return nil
}

func (p *Pins) Resolve(pin string) (string, error) {
	if !strings.Contains(pin, ":revision:") {
		return pin, nil
	}
	if digest := p.digests[pin]; digest != "" {
		return digest, nil
	}
	data, ok := p.sources[pin]
	if !ok && p.source != nil {
		var err error
		data, err = p.source(pin)
		if err != nil {
			return "", err
		}
		p.sources[pin] = data
		ok = true
	}
	if !ok {
		return "", fmt.Errorf("legacy evidence pin %s has no source; restore its file from Git or replace it with an explicit verified Git/content pin before normalization", pin)
	}
	if p.visiting[pin] {
		return "", fmt.Errorf("cyclic legacy evidence pins at %s; retain the files and resolve their source pins explicitly", pin)
	}
	p.visiting[pin] = true
	normalized, err := p.Normalize(data)
	if err != nil {
		return "", err
	}
	var value any
	if err = json.Unmarshal(normalized, &value); err != nil {
		return "", err
	}
	digest, err := Digest(value)
	if err != nil {
		return "", err
	}
	p.digests[pin] = digest
	delete(p.visiting, pin)
	return digest, nil
}

var pinFields = map[string]bool{"revision": true, "story_revision": true, "prototype_revision": true, "test_revision": true, "from_revision": true, "to_revision": true, "baseline": true}

// Normalize converts only typed pin fields. Ordinary prose and stable element
// identities are untouched; parent graph edges are discarded by publication,
// not converted into evidence.
func (p *Pins) Normalize(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	var walk func(any) error
	walk = func(value any) error {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "parents" || key == "parent_snapshots" {
					continue
				}
				if text, ok := child.(string); ok && isPinField(key) && strings.Contains(text, ":revision:") {
					digest, err := p.Resolve(text)
					if err != nil {
						return err
					}
					value[key] = digest
				} else if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value); err != nil {
		return nil, err
	}
	return json.MarshalIndent(value, "", "  ")
}

// PersistReferences converts external pins before any source can be discarded.
// Each file is replaced atomically. Interrupted runs can retry while legacy
// sources remain; converted digest pins never need their old revision files.
func (p *Pins) PersistReferences() error {
	if err := p.prepare(); err != nil {
		return err
	}
	paths := make([]string, 0, len(p.files))
	for path := range p.files {
		paths = append(paths, path)
	}
	// Publication order is deterministic, independent of map iteration.
	slices.Sort(paths)
	pending := []store.WriteBatchEntry{}
	for _, path := range paths {
		if strings.Contains(filepath.ToSlash(path), "/revisions/") {
			continue
		}
		if strings.Contains(filepath.ToSlash(path), "/relation-repins/") {
			continue
		}
		if strings.HasPrefix(filepath.Base(path), "24-a-") {
			continue
		}
		old := p.files[path]
		value := old
		if relation := p.relations[path]; len(relation) > 0 {
			value = relation
		}
		if strings.HasPrefix(filepath.Base(path), "25-t-") {
			var err error
			value, err = normalizeSlide(value)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		normalized, err := p.normalizeSourceFiles(value, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var before, after any
		_ = json.Unmarshal(old, &before)
		_ = json.Unmarshal(normalized, &after)
		canonicalBefore, _ := json.Marshal(before)
		canonicalAfter, _ := json.Marshal(after)
		if string(canonicalBefore) == string(canonicalAfter) {
			continue
		}
		pending = append(pending, store.WriteBatchEntry{Path: path, Data: append(normalized, '\n')})
	}
	assetPaths := make([]string, 0, len(p.assets))
	for original := range p.assets {
		assetPaths = append(assetPaths, original)
	}
	slices.Sort(assetPaths)
	for _, original := range assetPaths {
		asset := p.assets[original]
		if existing, err := os.ReadFile(asset.Path); err == nil {
			if !bytes.Equal(existing, asset.Data) {
				return fmt.Errorf("content-addressed asset collision at %s", asset.Path)
			}
		} else if !os.IsNotExist(err) {
			return err
		} else if err := store.WriteFile(asset.Path, asset.Data, 0644, true); err != nil {
			return err
		}
	}
	for _, entry := range pending {
		if err := store.WriteFile(entry.Path, entry.Data, 0644, false); err != nil {
			return err
		}
	}
	return p.cleanupRelations()
}

// normalizeSlide discards an unambiguous legacy content graph at its existing
// atomic publication point. It is not used for actual review decision files.
func normalizeSlide(data []byte) ([]byte, error) {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if len(value["content"]) > 0 {
		return data, nil
	}
	var revisions []map[string]json.RawMessage
	if err := json.Unmarshal(value["revisions"], &revisions); err != nil {
		return nil, err
	}
	graph := map[string][]string{}
	byID := map[string]map[string]json.RawMessage{}
	previous := ""
	for _, revision := range revisions {
		var id string
		var parents []string
		if err := json.Unmarshal(revision["snapshot"], &id); err != nil {
			return nil, err
		}
		if raw, exists := revision["parent_snapshots"]; exists {
			if err := json.Unmarshal(raw, &parents); err != nil {
				return nil, err
			}
		} else if previous != "" {
			parents = []string{previous}
		}
		if _, exists := graph[id]; exists {
			return nil, fmt.Errorf("duplicate legacy slide snapshot %s", id)
		}
		graph[id] = parents
		byID[id] = revision
		previous = id
	}
	selected, err := SelectLegacy(graph)
	if err != nil {
		return nil, err
	}
	var current string
	_ = json.Unmarshal(value["current"], &current)
	if current != selected {
		return nil, fmt.Errorf("legacy slide current pointer does not name its unique head")
	}
	content := byID[selected]
	for _, key := range []string{"snapshot", "parent_snapshots", "request_id", "created_at"} {
		delete(content, key)
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil, err
	}
	result := map[string]json.RawMessage{"$schema": json.RawMessage(`"https://changesaga.dev/schema/v5/slide-current.schema.json"`), "version": json.RawMessage(`2`), "deck": value["deck"], "slide": value["slide"], "content": encoded}
	return json.Marshal(result)
}

func (p *Pins) prepareAssets() error {
	p.assets = map[string]assetReplacement{}
	referenced := map[string]bool{}
	for file, data := range p.files {
		if strings.HasPrefix(filepath.Base(file), "24-a-") || strings.Contains(filepath.ToSlash(file), "/revisions/") {
			continue
		}
		if strings.HasPrefix(filepath.Base(file), "25-t-") {
			var err error
			data, err = normalizeSlide(data)
			if err != nil {
				return err
			}
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		var walk func(any)
		walk = func(value any) {
			switch value := value.(type) {
			case map[string]any:
				if name, ok := value["source"].(string); ok {
					if _, pinned := value["source_digest"]; pinned {
						referenced[filepath.Join(filepath.Dir(file), name)] = true
					}
				}
				for _, child := range value {
					walk(child)
				}
			case []any:
				for _, child := range value {
					walk(child)
				}
			}
		}
		walk(value)
	}
	paths := []string{}
	for path := range p.files {
		if referenced[path] && strings.HasPrefix(filepath.Base(path), "24-a-") && filepath.Ext(path) == ".json" {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		old := p.files[path]
		normalized, err := p.Normalize(old)
		if err != nil {
			return err
		}
		var before, after any
		if err := json.Unmarshal(old, &before); err != nil {
			return err
		}
		if err := json.Unmarshal(normalized, &after); err != nil {
			return err
		}
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(after)
		if bytes.Equal(a, b) {
			continue
		}
		data := append(normalized, '\n')
		sum := sha256.Sum256(data)
		name := "24-a-" + hex.EncodeToString(sum[:26]) + ".json"
		p.assets[path] = assetReplacement{Path: filepath.Join(filepath.Dir(path), name), Digest: "sha256:" + hex.EncodeToString(sum[:]), Data: data}
	}
	return nil
}
func (p *Pins) normalizeSourceFiles(data []byte, recordPath string) ([]byte, error) {
	normalized, err := p.Normalize(data)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(normalized, &value); err != nil {
		return nil, err
	}
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if name, ok := value["source"].(string); ok {
				if _, pinned := value["source_digest"]; pinned {
					if asset, changed := p.assets[filepath.Join(filepath.Dir(recordPath), name)]; changed {
						value["source"] = filepath.Base(asset.Path)
						value["source_digest"] = asset.Digest
					}
				}
			}
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(value)
	return json.MarshalIndent(value, "", "  ")
}

// NormalizeValue converts incoming typed authoring pins before legacy sources
// are removed. This is a read-only preflight, not a history sidecar.
func NormalizeValue(root string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Contains(data, []byte(":revision:")) {
		return nil
	}
	pins, err := AuditPins(root)
	if err != nil {
		return err
	}
	data, err = pins.Normalize(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func (p *Pins) ContentAtPin(pin string) (json.RawMessage, error) {
	if _, err := p.Resolve(pin); err != nil {
		return nil, err
	}
	data, exists := p.sources[pin]
	if !exists {
		return nil, fmt.Errorf("legacy source %s is unavailable", pin)
	}
	normalized, err := p.Normalize(data)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(normalized, &value); err != nil {
		return nil, err
	}
	return Content(value)
}

func isPinField(key string) bool {
	if pinFields[key] {
		return true
	}
	switch strings.ToLower(strings.ReplaceAll(key, "_", "")) {
	case "revision", "storyrevision", "prototyperevision", "testrevision", "fromrevision", "torevision", "baseline":
		return true
	}
	return false
}
