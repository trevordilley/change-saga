// Package currentrecord stores editable documentation. Git owns earlier values.
package currentrecord

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/store"
)

const Name = "current.json"
const Schema = "https://changesaga.dev/schema/v5/current-record.schema.json"
const MaxBytes = 8 << 20

// Record is the single publication point for authored content and its state.
// Content and State contain domain fields, never revision identities or parents.
// Identity remains in the package's original identity file.
type Record struct {
	Schema  string          `json:"$schema"`
	Version int             `json:"version"`
	Content json.RawMessage `json:"content"`
	State   json.RawMessage `json:"state,omitempty"`
}

var metadata = []string{"$schema", "version", "id", "parents", "created_at", "request_id"}

// Content removes only top-level storage metadata; nested element IDs and
// code commit/digest pins are authored content and are preserved.
func Content(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("current content must be an object")
	}
	for _, key := range metadata {
		delete(fields, key)
	}
	return json.Marshal(fields)
}

func Digest(value any) (string, error) {
	content, err := Content(value)
	if err != nil {
		return "", err
	}
	// RawMessage preserves a nested struct's declaration order. Decode before
	// hashing so every object is sorted, including nested source/evidence pins.
	var semantic any
	if err := json.Unmarshal(content, &semantic); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(semantic)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// Read is read-only. A successfully published current record takes precedence
// over obsolete legacy files left behind by an interrupted cleanup.
func Read(dir string) (*Record, error) {
	path := filepath.Join(dir, Name)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxBytes {
		return nil, fmt.Errorf("%s must be a regular current record of at most %d bytes", path, MaxBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record Record
	if err := Decode(data, &record); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if record.Schema != Schema || record.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported current record schema/version", path)
	}
	for _, raw := range []json.RawMessage{record.Content, record.State} {
		if len(raw) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if err := Decode(raw, &fields); err != nil {
			return nil, err
		}
		if fields == nil {
			return nil, fmt.Errorf("current content/state must be an object")
		}
		for _, key := range metadata {
			if _, ok := fields[key]; ok {
				return nil, fmt.Errorf("current content/state cannot contain %s", key)
			}
		}
	}
	if len(record.Content) == 0 {
		return nil, fmt.Errorf("current record requires content")
	}
	return &record, nil
}

// Project overlays authored fields on a caller-supplied domain value. The
// caller supplies identity metadata only for compatibility with domain types;
// those fields are never persisted as content history.
func Project(raw json.RawMessage, value any) error {
	if len(raw) == 0 {
		return nil
	}
	return Decode(raw, value)
}

// Write atomically replaces both content and state. The caller holds its Saga
// writer lock and has converted reference pins before invoking Cleanup.
func Write(dir string, content, state any) error {
	data, err := Content(content)
	if err != nil {
		return err
	}
	record := Record{Schema: Schema, Version: 1, Content: data}
	if state != nil {
		record.State, err = Content(state)
		if err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if len(encoded)+1 > MaxBytes {
		return fmt.Errorf("current record exceeds %d bytes", MaxBytes)
	}
	return store.WriteFile(filepath.Join(dir, Name), append(encoded, '\n'), 0644, false)
}

// Cleanup is deliberately separate from publication: references must be safe
// before deletion. It is idempotent and never follows a legacy symlink.
func Cleanup(dir string, names ...string) error {
	if record, err := Read(dir); err != nil {
		return err
	} else if record == nil {
		return fmt.Errorf("cannot discard legacy files before current publication")
	}
	for _, name := range names {
		if name != "revisions" && name != "events" {
			return fmt.Errorf("invalid legacy member %q", name)
		}
		path := filepath.Join(dir, name)
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s must be a real legacy directory", path)
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return store.SyncDir(dir)
}

// SelectLegacy identifies the unique leaf of a legacy parent graph. It never
// selects by wall-clock time and never changes storage on ambiguity.
func SelectLegacy(values map[string][]string) (string, error) {
	parents := map[string]bool{}
	state := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("legacy parent graph contains a cycle at %s", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, parent := range values[id] {
			// Legacy domain graphs use either local IDs or canonical member URNs.
			if strings.HasPrefix(parent, "urn:change-saga:") {
				parent = parent[strings.LastIndex(parent, ":")+1:]
			}
			if _, ok := values[parent]; !ok {
				return fmt.Errorf("legacy %s names missing parent %s", id, parent)
			}
			parents[parent] = true
			if err := visit(parent); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return "", err
		}
	}
	heads := []string{}
	for _, id := range ids {
		if !parents[id] {
			heads = append(heads, id)
		}
	}
	if len(heads) != 1 {
		return "", fmt.Errorf("legacy content has competing heads %v; preserve these files in Git, combine the authored content explicitly, and retry normalization; no files were discarded", heads)
	}
	return heads[0], nil
}

// Decode rejects unknown fields, duplicate object keys and trailing JSON.
func Decode(data []byte, value any) error {
	if len(data) > MaxBytes {
		return fmt.Errorf("record exceeds %d bytes", MaxBytes)
	}
	tokens := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := tokens.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for tokens.More() {
				key, err := tokens.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate or invalid JSON key %q", key)
				}
				seen[name] = true
				if err := walk(); err != nil {
					return err
				}
			}
		case '[':
			for tokens.More() {
				if err := walk(); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter")
		}
		_, err = tokens.Token()
		return err
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("record must contain exactly one JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

// ValidDigest accepts exactly the canonical SHA-256 content pin syntax.
func ValidDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && value == strings.ToLower(value)
}

// Equal compares authored values while ignoring compatibility storage metadata.
func Equal(left, right any) bool {
	a, err := Content(left)
	if err != nil {
		return false
	}
	b, err := Content(right)
	return err == nil && bytes.Equal(a, b)
}

// StableReference and MemberPath keep compatibility authoring adapters from
// returning identities or paths for content graph members that no longer exist.
func StableReference(value string) string {
	for _, member := range []string{":revision:", ":event:"} {
		if at := strings.Index(value, member); at >= 0 {
			return value[:at]
		}
	}
	return value
}
func MemberPath(value string) string {
	for _, member := range []string{"/revisions/", "/events/"} {
		if at := strings.Index(value, member); at >= 0 {
			return value[:at] + "/" + Name
		}
	}
	return value
}
