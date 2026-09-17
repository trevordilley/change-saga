package saga

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxDesignDigestTargets        = 10_000
	MaxDesignDigestFilesPerTarget = 1_024
	MaxDesignDigestBytesPerTarget = 16 << 20
)

type designFileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type designFragmentDigest struct {
	Version    int                `json:"version"`
	ID         string             `json:"id"`
	Title      string             `json:"title,omitempty"`
	MediaType  string             `json:"media_type"`
	Entrypoint string             `json:"entrypoint"`
	Order      int                `json:"order,omitempty"`
	Files      []designFileDigest `json:"files"`
}

type designLandmarkDigest struct {
	Version     int              `json:"version"`
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Description string           `json:"description,omitempty"`
	Selector    LandmarkSelector `json:"selector"`
	Hotspot     *LandmarkRegion  `json:"hotspot,omitempty"`
	Fragment    string           `json:"fragment"`
}

type designSectionDigest struct {
	Kind     string            `json:"kind"`
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	Order    int               `json:"order,omitempty"`
	Contents map[string]string `json:"contents"`
}

// visualDigestList is marshaled as a JSON array so child order is an explicit
// part of the canonical contract. The v4 loader orders Slides and Items by
// rank, then by their compact manifest path as the deterministic tie-break.
type visualDigestList []string

// CurrentDesignContentDigests returns the current digest for every addressable
// report-design target and embedded v4 Deck, Slide, and Item. Digests cover
// authored design content only: diffs, claims, verifications, comments,
// approvals, and other review overlays do not invalidate pinned relations. The
// fixed budgets keep a query from turning one design target into an unbounded
// filesystem read.
func CurrentDesignContentDigests(document *Saga) (map[string]string, error) {
	result := map[string]string{}
	if document == nil || document.Section == nil {
		return result, nil
	}
	reportContainer := document.Manifest.Version == CurrentSagaVersion || document.Manifest.Version > SlideSagaVersion
	if reportContainer {
		if _, err := digestDesignSection(document.Section, result, false); err != nil {
			return nil, err
		}
	}
	// A standalone v4 Saga remains a slide-only review document. Embedded v4
	// bundles in a report are visual design targets; the same condition also
	// leaves this API ready for later report-container versions without coupling
	// it to their relation schemas.
	if reportContainer {
		if err := digestEmbeddedVisualDesign(document.Decks, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// CurrentDesignContentDigest looks up one addressable design target using the
// same canonical digest contract as CurrentDesignContentDigests.
func CurrentDesignContentDigest(document *Saga, target string) (string, bool, error) {
	digests, err := CurrentDesignContentDigests(document)
	if err != nil {
		return "", false, err
	}
	digest, ok := digests[target]
	return digest, ok, nil
}

func digestDesignSection(section *Section, result map[string]string, includeSelf bool) (string, error) {
	contents := map[string]string{}
	for _, fragment := range section.Fragments {
		if !isDesignPath(fragment.Path) {
			continue
		}
		digest, err := digestDesignFragment(fragment, result)
		if err != nil {
			return "", err
		}
		contents[fragment.Target] = digest
	}
	for _, child := range section.Children {
		childIsDesign := isDesignPath(child.Path)
		digest, err := digestDesignSection(child, result, childIsDesign)
		if err != nil {
			return "", err
		}
		if childIsDesign {
			contents[child.Target] = digest
		}
	}
	if !includeSelf {
		return "", nil
	}
	digest, err := canonicalDesignDigest("section-v1", designSectionDigest{
		Kind: section.Kind, ID: section.ID, Title: section.Title, Order: section.Order, Contents: contents,
	})
	if err != nil {
		return "", err
	}
	if err := addDesignDigest(result, section.Target, digest); err != nil {
		return "", err
	}
	return digest, nil
}

func digestDesignFragment(fragment *Fragment, result map[string]string) (string, error) {
	files, err := designAuthoredFiles(fragment.Directory)
	if err != nil {
		return "", fmt.Errorf("digest design fragment %q: %w", fragment.Target, err)
	}
	base, err := canonicalDesignDigest("fragment-content-v1", designFragmentDigest{
		Version: ComponentVersion, ID: fragment.ID, Title: fragment.Title, MediaType: fragment.MediaType,
		Entrypoint: fragment.Entrypoint, Order: fragment.Order, Files: files,
	})
	if err != nil {
		return "", err
	}
	landmarks := map[string]string{}
	for _, landmark := range fragment.Landmarks {
		digest, err := canonicalDesignDigest("landmark-v1", designLandmarkDigest{
			Version: landmark.Version, ID: landmark.ID, Label: landmark.Label,
			Description: landmark.Description, Selector: landmark.Selector,
			Hotspot: landmark.Hotspot, Fragment: base,
		})
		if err != nil {
			return "", err
		}
		if err := addDesignDigest(result, landmark.Target, digest); err != nil {
			return "", err
		}
		landmarks[landmark.Target] = digest
	}
	digest, err := canonicalDesignDigest("fragment-v1", struct {
		Content   string            `json:"content"`
		Landmarks map[string]string `json:"landmarks"`
	}{Content: base, Landmarks: landmarks})
	if err != nil {
		return "", err
	}
	if err := addDesignDigest(result, fragment.Target, digest); err != nil {
		return "", err
	}
	return digest, nil
}

func digestEmbeddedVisualDesign(decks []*Deck, result map[string]string) error {
	orderedDecks := append([]*Deck(nil), decks...)
	sort.Slice(orderedDecks, func(i, j int) bool {
		if orderedDecks[i].Rank == orderedDecks[j].Rank {
			return orderedDecks[i].Path < orderedDecks[j].Path
		}
		return orderedDecks[i].Rank < orderedDecks[j].Rank
	})
	for _, deck := range orderedDecks {
		if _, err := digestVisualDeck(deck, result); err != nil {
			return err
		}
	}
	return nil
}

func digestVisualDeck(deck *Deck, result map[string]string) (string, error) {
	manifest, err := json.Marshal(deck.DeckManifest)
	if err != nil {
		return "", fmt.Errorf("digest visual deck %q: canonicalize manifest: %w", deck.Target, err)
	}
	orderedSlides := append([]*Slide(nil), deck.Slides...)
	sort.Slice(orderedSlides, func(i, j int) bool {
		if orderedSlides[i].Rank == orderedSlides[j].Rank {
			return orderedSlides[i].Path < orderedSlides[j].Path
		}
		return orderedSlides[i].Rank < orderedSlides[j].Rank
	})
	slideDigests := make(visualDigestList, 0, len(orderedSlides))
	for _, slide := range orderedSlides {
		digest, digestErr := digestVisualSlide(slide, result)
		if digestErr != nil {
			return "", digestErr
		}
		slideDigests = append(slideDigests, digest)
	}
	children, err := json.Marshal(slideDigests)
	if err != nil {
		return "", fmt.Errorf("digest visual deck %q: canonicalize slide digests: %w", deck.Target, err)
	}
	digest := canonicalDesignDigestParts("deck-v1", manifest, children)
	if err := addDesignDigest(result, deck.Target, digest); err != nil {
		return "", err
	}
	return digest, nil
}

func digestVisualSlide(slide *Slide, result map[string]string) (string, error) {
	manifest, err := json.Marshal(slide.SlideManifest)
	if err != nil {
		return "", fmt.Errorf("digest visual slide %q: canonicalize manifest: %w", slide.Target, err)
	}
	asset, err := readVisualAsset(slide)
	if err != nil {
		return "", fmt.Errorf("digest visual slide %q: %w", slide.Target, err)
	}
	orderedItems := append([]*Item(nil), slide.Items...)
	sort.Slice(orderedItems, func(i, j int) bool {
		if orderedItems[i].Rank == orderedItems[j].Rank {
			return orderedItems[i].Path < orderedItems[j].Path
		}
		return orderedItems[i].Rank < orderedItems[j].Rank
	})
	itemManifestDigests := make(visualDigestList, 0, len(orderedItems))
	for _, item := range orderedItems {
		itemManifest, marshalErr := json.Marshal(item.ItemManifest)
		if marshalErr != nil {
			return "", fmt.Errorf("digest visual item %q: canonicalize manifest: %w", item.Target, marshalErr)
		}
		manifestDigest := canonicalDesignDigestParts("item-manifest-v1", itemManifest)
		itemManifestDigests = append(itemManifestDigests, manifestDigest)
		itemDigest := canonicalDesignDigestParts("item-v1", itemManifest, asset)
		if err := addDesignDigest(result, item.Target, itemDigest); err != nil {
			return "", err
		}
	}
	children, err := json.Marshal(itemManifestDigests)
	if err != nil {
		return "", fmt.Errorf("digest visual slide %q: canonicalize item manifest digests: %w", slide.Target, err)
	}
	digest := canonicalDesignDigestParts("slide-v1", manifest, asset, children)
	if err := addDesignDigest(result, slide.Target, digest); err != nil {
		return "", err
	}
	return digest, nil
}

func readVisualAsset(slide *Slide) ([]byte, error) {
	entrypoint := filepath.FromSlash(slide.Entrypoint)
	if entrypoint == "." || filepath.IsAbs(entrypoint) || filepath.Base(entrypoint) != entrypoint {
		return nil, fmt.Errorf("entrypoint %q must name one flat slide asset", slide.Entrypoint)
	}
	path := filepath.Join(slide.Directory, entrypoint)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read entrypoint %q: %w", slide.Entrypoint, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("entrypoint %q must be a regular file", slide.Entrypoint)
	}
	if info.Size() > MaxDesignDigestBytesPerTarget {
		return nil, fmt.Errorf("entrypoint %q exceeds %d bytes", slide.Entrypoint, MaxDesignDigestBytesPerTarget)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entrypoint %q: %w", slide.Entrypoint, err)
	}
	if len(data) > MaxDesignDigestBytesPerTarget {
		return nil, fmt.Errorf("entrypoint %q exceeds %d bytes", slide.Entrypoint, MaxDesignDigestBytesPerTarget)
	}
	return data, nil
}

func designAuthoredFiles(root string) ([]designFileDigest, error) {
	files := []designFileDigest{}
	total := int64(0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if strings.HasPrefix(parts[0], "___") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || rel == "fragment.json" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("authored file %q must be a regular file", filepath.ToSlash(rel))
		}
		if len(files) >= MaxDesignDigestFilesPerTarget {
			return fmt.Errorf("authored package exceeds %d files", MaxDesignDigestFilesPerTarget)
		}
		total += info.Size()
		if total > MaxDesignDigestBytesPerTarget {
			return fmt.Errorf("authored package exceeds %d bytes", MaxDesignDigestBytesPerTarget)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, designFileDigest{Path: filepath.ToSlash(rel), SHA256: fmt.Sprintf("sha256:%x", sum)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func canonicalDesignDigest(domain string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("change-saga-design-"+domain+"\x00"), data...))
	return fmt.Sprintf("sha256:%x", sum), nil
}

// canonicalDesignDigestParts hashes exact byte inputs with an eight-byte
// big-endian length before every part. This makes boundaries unambiguous while
// keeping binary slide assets byte-for-byte significant. The versioned domain
// separates Item manifests, Items, Slides, and Decks from one another.
func canonicalDesignDigestParts(domain string, parts ...[]byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte("change-saga-design-" + domain + "\x00"))
	for _, part := range parts {
		writeDigestPart(h, part)
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func writeDigestPart(h hash.Hash, part []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(part)))
	_, _ = h.Write(size[:])
	_, _ = h.Write(part)
}

func addDesignDigest(result map[string]string, target, digest string) error {
	if len(result) >= MaxDesignDigestTargets {
		return fmt.Errorf("design exceeds %d addressable targets", MaxDesignDigestTargets)
	}
	result[target] = digest
	return nil
}

func isDesignPath(path string) bool {
	return path == "___design" || strings.HasPrefix(path, "___design/")
}
