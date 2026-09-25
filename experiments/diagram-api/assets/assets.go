// Package assets contains pinned local drawing inputs. No runtime downloads.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"golang.org/x/image/font/gofont/goregular"
)

//go:embed lucide/*.svg lucide/LICENSE lucide/manifest.json GO-FONT-LICENSE
var files embed.FS

const LucideRevision = "66d8f9fc394b8530377e5f6112f0b8908ba01280"

func Get(name string) ([]byte, string, string, error) {
	if name == "font:go-regular" {
		l, _ := files.ReadFile("GO-FONT-LICENSE")
		return goregular.TTF, "golang.org/x/image@v0.44.0/font/gofont/goregular", string(l), nil
	}
	var m struct {
		Icons map[string]struct {
			File string `json:"file"`
			Hash string `json:"sha256"`
		} `json:"icons"`
	}
	b, _ := files.ReadFile("lucide/manifest.json")
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, "", "", err
	}
	v, ok := m.Icons[name]
	if !ok {
		return nil, "", "", fmt.Errorf("unknown bundled asset %q; use assets search", name)
	}
	b, err := files.ReadFile("lucide/" + v.File)
	if err != nil {
		return nil, "", "", err
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != v.Hash {
		return nil, "", "", fmt.Errorf("bundled asset digest mismatch: %s", name)
	}
	l, _ := files.ReadFile("lucide/LICENSE")
	return b, "lucide@" + LucideRevision + "/" + v.File, string(l), err
}
func Names(query string) []string {
	es, _ := files.ReadDir("lucide")
	names := []string{}
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".svg") {
			name := "lucide:" + strings.TrimSuffix(e.Name(), ".svg")
			if strings.Contains(name, query) {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}
