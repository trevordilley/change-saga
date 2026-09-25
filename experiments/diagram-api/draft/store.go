package draft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/twentyideas/changesaga/experiments/diagram-api/assets"
	sagastore "github.com/twentyideas/changesaga/internal/store"
)

type Receipt struct {
	Payload  string `json:"payload"`
	Snapshot string `json:"snapshot"`
}
type Record struct {
	Source   Document           `json:"source"`
	Snapshot string             `json:"snapshot"`
	SVGHash  string             `json:"svg_hash"`
	Receipts map[string]Receipt `json:"receipts"`
}
type Result struct {
	Snapshot        string   `json:"snapshot"`
	SVGHash         string   `json:"svg_hash"`
	Changed         []string `json:"changed_ids"`
	Replayed        bool     `json:"replayed,omitempty"`
	AppliedSnapshot string   `json:"applied_snapshot,omitempty"`
	DryRun          bool     `json:"dry_run,omitempty"`
}
type Store struct {
	Root          string
	BeforePublish func() error
}

func (s Store) path(dir, hash string) string {
	return filepath.Join(s.Root, dir, hash[len("sha256:"):])
}
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("expected regular file %s", path)
	}
	if info.Size() > 16<<20 {
		return nil, fmt.Errorf("prototype file limit exceeded")
	}
	return os.ReadFile(path)
}
func (s Store) input(pin Asset) ([]byte, error) {
	if !digestPattern.MatchString(pin.Digest) {
		return nil, fmt.Errorf("invalid digest")
	}
	b, err := readRegular(s.path("inputs", pin.Digest))
	if err != nil {
		return nil, err
	}
	if Hash(b) != pin.Digest {
		return nil, fmt.Errorf("corrupt pinned input %s", pin.Digest)
	}
	return b, nil
}
func (s Store) Load() (Record, error) {
	var r Record
	b, err := readRegular(filepath.Join(s.Root, "current.json"))
	if err != nil {
		return r, err
	}
	if err = Decode(b, &r); err != nil {
		return r, err
	}
	if err = r.Source.Validate(); err != nil {
		return r, err
	}
	if Snapshot(r.Source) != r.Snapshot || !digestPattern.MatchString(r.SVGHash) {
		return r, fmt.Errorf("source snapshot mismatch or invalid SVG digest")
	}
	return r, nil
}
func mkdir(path string) error {
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	i, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("expected real directory %s", path)
	}
	return nil
}
func put(path string, b []byte) error {
	old, err := readRegular(path)
	if err == nil {
		if !bytes.Equal(old, b) {
			return fmt.Errorf("immutable asset diverged: %s", path)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return sagastore.WriteFile(path, b, 0644, true)
}
func (s Store) pin(d *Document) (map[string][]byte, error) {
	if d.Assets == nil {
		d.Assets = map[string]Asset{}
	}
	needed := map[string]bool{"font:go-regular": true}
	for _, e := range d.Elements {
		if e.Icon != "" {
			needed[e.Icon] = true
		}
	}
	inputs := map[string][]byte{}
	for _, name := range keys(needed) {
		pin, exists := d.Assets[name]
		if exists {
			b, err := s.input(pin)
			if err == nil {
				inputs[pin.Digest] = b
				continue
			}
			// A portable source can use bundled bytes only if its exact digest matches.
			b, _, _, bundleErr := assets.Get(name)
			if bundleErr != nil || Hash(b) != pin.Digest {
				return nil, fmt.Errorf("missing/corrupt pinned input %s: %w", name, err)
			}
			// Do not quietly repair a corrupted file during an ordinary edit.
			if !os.IsNotExist(err) {
				return nil, err
			}
			inputs[pin.Digest] = b
			continue
		}
		b, origin, license, err := assets.Get(name)
		if err != nil {
			return nil, err
		}
		pin = Asset{Hash(b), origin, license}
		d.Assets[name] = pin
		inputs[pin.Digest] = b
	}
	// Preserve and check unused pins; deleting an icon does not erase historical inputs.
	for name, pin := range d.Assets {
		if _, ok := inputs[pin.Digest]; !ok {
			b, err := s.input(pin)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			inputs[pin.Digest] = b
		}
	}
	return inputs, nil
}
func (s Store) Apply(req Request, dry bool) (Result, error) {
	if req.Version != 1 || !identifier.MatchString(req.RequestID) {
		return Result{}, fmt.Errorf("version 1 and stable request_id required")
	}
	if req.Expected != "absent" && !digestPattern.MatchString(req.Expected) {
		return Result{}, fmt.Errorf("exact expected_snapshot required")
	}
	if len(req.Operations) > 1000 {
		return Result{}, fmt.Errorf("prototype batch limit exceeded")
	}
	// This spike creates an empty store directory even for first-create dry-run.
	if err := mkdir(s.Root); err != nil {
		return Result{}, err
	}
	var result Result
	err := sagastore.WithSagaLock(s.Root, 2*time.Second, func() error {
		old, err := s.Load()
		exists := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		payload, _ := json.Marshal(req)
		rh := Hash(payload)
		if exists {
			if receipt, ok := old.Receipts[req.RequestID]; ok {
				if receipt.Payload != rh {
					return fmt.Errorf("request_id reused with different payload")
				}
				result = Result{Snapshot: old.Snapshot, SVGHash: old.SVGHash, Changed: []string{}, Replayed: true, AppliedSnapshot: receipt.Snapshot, DryRun: dry}
				return nil
			}
		}
		actual := "absent"
		if exists {
			actual = old.Snapshot
		}
		if actual != req.Expected {
			return fmt.Errorf("stale_snapshot: expected %s, current %s", req.Expected, actual)
		}
		if exists {
			if _, err := s.Visual(old); err != nil {
				return fmt.Errorf("visual divergence; check/rebuild before editing: %w", err)
			}
		}
		d := old.Source
		if req.Source != nil {
			d = clone(*req.Source)
		} else if !exists {
			return fmt.Errorf("creation requires source")
		}
		if d.Elements == nil {
			d.Elements = map[string]Element{}
		}
		d, err = Edit(d, req.Operations)
		if err != nil {
			return err
		}
		if exists {
			if d.ID != old.Source.ID {
				return fmt.Errorf("diagram identity is immutable")
			}
			for id, previous := range old.Source.Elements {
				if len(previous.Evidence) == 0 && len(previous.CriterionLinks) == 0 {
					continue
				}
				next, ok := d.Elements[id]
				a, _ := json.Marshal([]any{previous.Evidence, previous.CriterionLinks})
				b, _ := json.Marshal([]any{next.Evidence, next.CriterionLinks})
				if !ok || !bytes.Equal(a, b) {
					return fmt.Errorf("refusing source replacement that alters linked Item %s", id)
				}
			}
		}
		inputs, err := s.pin(&d)
		if err != nil {
			return err
		}
		visual, err := Render(d, func(a Asset) ([]byte, error) {
			b, ok := inputs[a.Digest]
			if !ok {
				return nil, fmt.Errorf("missing asset")
			}
			return b, nil
		})
		if err != nil {
			return err
		}
		changed := []string{}
		all := map[string]bool{}
		for id := range old.Source.Elements {
			all[id] = true
		}
		for id := range d.Elements {
			all[id] = true
		}
		for _, id := range keys(all) {
			a, _ := json.Marshal(old.Source.Elements[id])
			b, _ := json.Marshal(d.Elements[id])
			oldStyle, _ := json.Marshal(old.Source.Styles[old.Source.Elements[id].Style])
			newStyle, _ := json.Marshal(d.Styles[d.Elements[id].Style])
			if !bytes.Equal(a, b) || !bytes.Equal(oldStyle, newStyle) {
				changed = append(changed, id)
			}
		}
		result = Result{Snapshot: Snapshot(d), SVGHash: Hash(visual), Changed: changed, DryRun: dry}
		if dry {
			return nil
		}
		for _, dir := range []string{"inputs", "visuals", "recovered"} {
			if err := mkdir(filepath.Join(s.Root, dir)); err != nil {
				return err
			}
		}
		for hash, b := range inputs {
			if err := put(s.path("inputs", hash), b); err != nil {
				return err
			}
		}
		if err := put(s.path("visuals", result.SVGHash), visual); err != nil {
			return err
		}
		receipts := old.Receipts
		if receipts == nil {
			receipts = map[string]Receipt{}
		}
		if len(receipts) >= 256 {
			return fmt.Errorf("prototype receipt limit reached; refusing to discard retry history")
		}
		receipts[req.RequestID] = Receipt{rh, result.Snapshot}
		record := Record{d, result.Snapshot, result.SVGHash, receipts}
		if s.BeforePublish != nil {
			if err := s.BeforePublish(); err != nil {
				return err
			}
		}
		// One existing Saga publication primitive makes source, pins, visual digest,
		// and replay identity visible together. Staged unreferenced blobs are harmless.
		return sagastore.WriteJSON(filepath.Join(s.Root, "current.json"), record, !exists)
	})
	return result, err
}
func (s Store) Visual(r Record) ([]byte, error) {
	b, err := readRegular(s.path("visuals", r.SVGHash))
	if err != nil {
		return nil, err
	}
	if Hash(b) != r.SVGHash {
		return nil, fmt.Errorf("SVG hash mismatch")
	}
	if err := CheckSelectors(r.Source, b); err != nil {
		return nil, err
	}
	return b, nil
}
func (s Store) Check() (map[string]any, error) {
	r, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := map[string]any{"snapshot": r.Snapshot, "svg_hash": r.SVGHash}
	_, err = s.Visual(r)
	out["visual_ok"] = err == nil
	if err != nil {
		out["visual_error"] = err.Error()
	}
	rendered, re := Render(r.Source, s.input)
	out["rebuildable"] = re == nil && Hash(rendered) == r.SVGHash
	if re != nil {
		out["input_error"] = re.Error()
	}
	return out, nil
}
func (s Store) Rebuild() (map[string]any, error) {
	var result map[string]any
	err := sagastore.WithSagaLock(s.Root, 2*time.Second, func() error {
		r, err := s.Load()
		if err != nil {
			return err
		}
		b, err := Render(r.Source, s.input)
		if err != nil {
			return err
		}
		if Hash(b) != r.SVGHash {
			return fmt.Errorf("renderer output differs from expected hash; explicit renderer migration required")
		}
		path := s.path("visuals", r.SVGHash)
		previous, err := readRegular(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		backup := ""
		if err == nil && !bytes.Equal(previous, b) {
			backup = s.path("recovered", Hash(previous))
			if err := mkdir(filepath.Dir(backup)); err != nil {
				return err
			}
			if err := put(backup, previous); err != nil {
				return err
			}
		}
		if err := sagastore.WriteFile(path, b, 0644, false); err != nil {
			return err
		}
		result = map[string]any{"snapshot": r.Snapshot, "svg_hash": r.SVGHash, "recovered_path": backup}
		return nil
	})
	return result, err
}
func PublicationState(err error) map[string]any {
	var p *sagastore.PublicationError
	if errors.As(err, &p) {
		return map[string]any{"published": p.Published, "durable": p.Durable, "retry": "query current state, then retry the identical request_id and payload"}
	}
	return nil
}
