package diagram

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
)

//go:embed assets/lucide/icons.json assets/lucide/LICENSE assets/GO-FONT-LICENSE
var bundled embed.FS

// LucideRevision pins the upstream commit the bundled icons were copied from.
// Regenerate the bundle with internal/diagram/lucidebundle.
const LucideRevision = "66d8f9fc394b8530377e5f6112f0b8908ba01280"

// FontPath is where the reviewer serves the measurement font. Generated SVGs
// reference it and fall back to a system sans-serif elsewhere.
const FontPath = "/_diagram/fonts/go-regular.ttf"

// FontFamily is the CSS family name generated SVGs declare.
const FontFamily = "Change Saga Diagram"

const MonoFontPath = "/_diagram/fonts/go-mono.ttf"
const MonoFontFamily = "Change Saga Code"

// iconBundle is the complete Lucide set: every upstream icon shares one <svg>
// root, stored once, and keeps its drawing markup and search tags.
type iconBundle struct {
	Upstream string `json:"upstream"`
	Revision string `json:"revision"`
	Root     string `json:"root"`
	Icons    map[string]struct {
		SVG  string   `json:"svg"`
		Tags []string `json:"tags"`
	} `json:"icons"`
}

const iconPrefix = "lucide:"

var loadIcons = sync.OnceValues(func() (iconBundle, error) {
	var bundle iconBundle
	data, err := bundled.ReadFile("assets/lucide/icons.json")
	if err != nil {
		return bundle, err
	}
	if err := json.Unmarshal(data, &bundle); err != nil {
		return bundle, err
	}
	if bundle.Revision != LucideRevision {
		return bundle, fmt.Errorf("bundled Lucide revision %s does not match %s", bundle.Revision, LucideRevision)
	}
	return bundle, nil
})

// IconExists reports whether name is a bundled icon, such as lucide:database.
func IconExists(name string) bool {
	bundle, err := loadIcons()
	if err != nil || !strings.HasPrefix(name, iconPrefix) {
		return false
	}
	_, ok := bundle.Icons[strings.TrimPrefix(name, iconPrefix)]
	return ok
}

// Icon returns a bundled icon as a standalone SVG document.
func Icon(name string) ([]byte, error) {
	bundle, err := loadIcons()
	if err != nil {
		return nil, err
	}
	entry, ok := bundle.Icons[strings.TrimPrefix(name, iconPrefix)]
	if !ok || !strings.HasPrefix(name, iconPrefix) {
		return nil, fmt.Errorf("unknown icon %q", name)
	}
	return []byte(bundle.Root + entry.SVG + "</svg>"), nil
}

// Icons lists bundled icon names whose name or a search tag contains query,
// sorted, so a query can name what an icon depicts rather than its exact name.
func Icons(query string) []string {
	bundle, err := loadIcons()
	if err != nil {
		return []string{}
	}
	query = strings.ToLower(query)
	names := []string{}
	for name, entry := range bundle.Icons {
		matched := strings.Contains(iconPrefix+name, query)
		for _, tag := range entry.Tags {
			matched = matched || strings.Contains(strings.ToLower(tag), query)
		}
		if matched {
			names = append(names, iconPrefix+name)
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

// MonoFont is the bundled measurement and display font for code examples.
func MonoFont() ([]byte, string) {
	license, _ := bundled.ReadFile("assets/GO-FONT-LICENSE")
	return gomono.TTF, string(license)
}
