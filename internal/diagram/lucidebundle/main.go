// Command lucidebundle regenerates internal/diagram/assets/lucide from a
// Lucide checkout at a pinned revision:
//
//	git -C lucide checkout <revision>
//	go run ./internal/diagram/lucidebundle -src lucide -revision <revision>
//
// Every upstream icon shares one <svg> root, so the bundle stores that root
// once and each icon's drawing markup on its own line, beside the search tags
// and deprecated aliases from the icon's metadata. One line per icon keeps an
// upgrade's diff readable and the bundle a single file to review.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var rootPattern = regexp.MustCompile(`(?s)^(<svg[^>]*>)\n(.*)</svg>\n?$`)

type metadata struct {
	Tags    []string `json:"tags"`
	Aliases []struct {
		Name string `json:"name"`
	} `json:"aliases"`
}

type icon struct {
	SVG  string   `json:"svg"`
	Tags []string `json:"tags,omitempty"`
}

func main() {
	src := flag.String("src", "", "Lucide checkout")
	revision := flag.String("revision", "", "the commit the checkout is at")
	out := flag.String("out", "internal/diagram/assets/lucide", "bundle directory")
	flag.Parse()
	if err := run(*src, *revision, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(src, revision, out string) error {
	if src == "" || revision == "" {
		return fmt.Errorf("usage: lucidebundle -src LUCIDE_CHECKOUT -revision COMMIT")
	}
	files, err := filepath.Glob(filepath.Join(src, "icons", "*.svg"))
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no icons under %s/icons", src)
	}
	sort.Strings(files)
	root := ""
	names := []string{}
	icons := map[string]icon{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		match := rootPattern.FindSubmatch(data)
		if match == nil {
			return fmt.Errorf("%s: unexpected SVG layout", file)
		}
		if root == "" {
			root = string(match[1])
		} else if string(match[1]) != root {
			return fmt.Errorf("%s: its <svg> root differs from the shared one", file)
		}
		var body strings.Builder
		for _, line := range strings.Split(string(match[2]), "\n") {
			body.WriteString(strings.TrimSpace(line))
		}
		name := strings.TrimSuffix(filepath.Base(file), ".svg")
		var meta metadata
		if data, err := os.ReadFile(strings.TrimSuffix(file, ".svg") + ".json"); err == nil {
			if err := json.Unmarshal(data, &meta); err != nil {
				return fmt.Errorf("%s: %w", file, err)
			}
		}
		tags := append([]string{}, meta.Tags...)
		for _, alias := range meta.Aliases {
			tags = append(tags, alias.Name)
		}
		names = append(names, name)
		icons[name] = icon{SVG: body.String(), Tags: tags}
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	for _, field := range [][2]string{{"upstream", "https://github.com/lucide-icons/lucide"}, {"revision", revision}, {"root", root}} {
		fmt.Fprintf(&b, "%s: %s,\n", marshal(field[0]), marshal(field[1]))
	}
	b.WriteString("\"icons\": {\n")
	for index, name := range names {
		separator := ","
		if index == len(names)-1 {
			separator = ""
		}
		fmt.Fprintf(&b, "%s: %s%s\n", marshal(name), marshal(icons[name]), separator)
	}
	b.WriteString("}\n}\n")
	if err := os.WriteFile(filepath.Join(out, "icons.json"), b.Bytes(), 0o644); err != nil {
		return err
	}
	license, err := os.ReadFile(filepath.Join(src, "LICENSE"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "LICENSE"), license, 0o644)
}

// marshal encodes v on one line without escaping markup characters.
func marshal(v any) string {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}
