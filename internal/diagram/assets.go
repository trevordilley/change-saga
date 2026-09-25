package diagram

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font/gofont/goregular"
)

//go:embed assets/lucide/*.svg assets/lucide/LICENSE assets/lucide/manifest.json assets/GO-FONT-LICENSE
var bundled embed.FS

// LucideRevision pins the upstream commit the bundled icons were copied from.
const LucideRevision = "66d8f9fc394b8530377e5f6112f0b8908ba01280"

// FontPath is where the reviewer serves the measurement font. Generated SVGs
// reference it and fall back to a system sans-serif elsewhere.
const FontPath = "/_diagram/fonts/go-regular.ttf"

// FontFamily is the CSS family name generated SVGs declare.
const FontFamily = "Change Saga Diagram"

type iconManifest struct {
	Upstream string `json:"upstream"`
	Revision string `json:"revision"`
	Icons    map[string]struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	} `json:"icons"`
}

var loadIcons = sync.OnceValues(func() (iconManifest, error) {
	var manifest iconManifest
	data, err := bundled.ReadFile("assets/lucide/manifest.json")
	if err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	if manifest.Revision != LucideRevision {
		return manifest, fmt.Errorf("bundled Lucide manifest revision %s does not match %s", manifest.Revision, LucideRevision)
	}
	return manifest, nil
})

// IconExists reports whether name is a bundled icon, such as lucide:database.
func IconExists(name string) bool {
	manifest, err := loadIcons()
	if err != nil {
		return false
	}
	_, ok := manifest.Icons[name]
	return ok
}

// Icon returns a bundled icon's SVG bytes after checking its pinned digest.
func Icon(name string) ([]byte, error) {
	manifest, err := loadIcons()
	if err != nil {
		return nil, err
	}
	entry, ok := manifest.Icons[name]
	if !ok {
		return nil, fmt.Errorf("unknown icon %q", name)
	}
	data, err := bundled.ReadFile("assets/lucide/" + entry.File)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != entry.SHA256 {
		return nil, fmt.Errorf("bundled icon %s does not match its pinned digest", name)
	}
	return data, nil
}

// Icons lists bundled icon names containing query, sorted.
func Icons(query string) []string {
	manifest, err := loadIcons()
	if err != nil {
		return []string{}
	}
	names := []string{}
	for name := range manifest.Icons {
		if strings.Contains(name, strings.ToLower(query)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// IconLicense is the notice embedded in SVGs that include Lucide icons.
func IconLicense() string {
	data, _ := bundled.ReadFile("assets/lucide/LICENSE")
	return string(data)
}

// Font returns the measurement font bytes and their license notice.
func Font() ([]byte, string) {
	license, _ := bundled.ReadFile("assets/GO-FONT-LICENSE")
	return goregular.TTF, string(license)
}
