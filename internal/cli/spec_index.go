package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/twentyideas/changesaga/internal/grammar"
)

// specSelection is what a `spec --json` read asks for; none of them means the
// compact index.
type specSelection struct {
	command, resource, section string
	all                        bool
}

func (selection specSelection) count() int {
	count := 0
	for _, set := range []bool{selection.command != "", selection.resource != "", selection.section != "", selection.all} {
		if set {
			count++
		}
	}
	return count
}

// specIndex is the default `spec --json`: the names to ask about, with the
// scalars that identify the format. A selected read returns the rest at the
// same path it has in the whole contract.
type specIndex struct {
	Version       any               `json:"version"`
	Manifest      any               `json:"manifest"`
	TargetScheme  any               `json:"target_scheme"`
	StatusSchema  string            `json:"status_schema"`
	CheckSchema   string            `json:"check_schema"`
	QuerySchema   string            `json:"query_schema"`
	QueryDiscover string            `json:"query_discovery"`
	Expand        map[string]string `json:"expand"`
	Sections      []string          `json:"sections"`
	Resources     []string          `json:"resources"`
	Commands      []string          `json:"commands"`
}

// writeSpec prints the part of the contract the selection names.
func writeSpec(out io.Writer, contract map[string]any, selection specSelection) error {
	switch {
	case selection.all:
		return writeJSON(out, contract)
	case selection.command != "":
		for _, command := range grammar.Commands() {
			if command.Name == selection.command {
				return writeJSON(out, map[string]any{"command": command})
			}
		}
		return fmt.Errorf("spec has no command %q; spec --json lists them", selection.command)
	case selection.resource != "":
		for _, resource := range grammar.Resources() {
			if resource.Kind == selection.resource {
				return writeJSON(out, map[string]any{"resource": resource})
			}
		}
		return fmt.Errorf("spec has no resource %q; spec --json lists them", selection.resource)
	case selection.section != "":
		return writeSpecSection(out, contract, selection.section)
	}
	return writeJSON(out, specIndexOf(contract))
}

// writeSpecSection prints one section at the path it has in the whole contract.
// A section of the living grammar is named on its own, and is printed under
// "living".
func writeSpecSection(out io.Writer, contract map[string]any, name string) error {
	living, _ := contract["living"].(map[string]any)
	if value, ok := contract[name]; ok && name != "living" {
		return writeJSON(out, map[string]any{name: value})
	}
	if value, ok := living[name]; ok && !specIndexedElsewhere(name) {
		return writeJSON(out, map[string]any{"living": map[string]any{name: value}})
	}
	return fmt.Errorf("spec has no section %q; it has %s", name, strings.Join(specSections(contract), ", "))
}

// specIndexedElsewhere names the living keys read one at a time, through
// --command and --resource, rather than as a section.
func specIndexedElsewhere(name string) bool {
	return name == "commands" || name == "resources"
}

// specSections names every section of the contract that holds more than a
// scalar, in sorted order: the top-level ones and those of the living grammar.
func specSections(contract map[string]any) []string {
	sections := []string{}
	add := func(values map[string]any) {
		for name, value := range values {
			switch value.(type) {
			case string, int, bool:
				continue
			}
			if name != "living" && !specIndexedElsewhere(name) {
				sections = append(sections, name)
			}
		}
	}
	add(contract)
	if living, ok := contract["living"].(map[string]any); ok {
		add(living)
	}
	slices.Sort(sections)
	return sections
}

func specIndexOf(contract map[string]any) specIndex {
	index := specIndex{
		Version: contract["version"], Manifest: contract["manifest"], TargetScheme: contract["target_scheme"],
		StatusSchema: CompactStatusSchema, CheckSchema: CheckSchema, QuerySchema: querySchema, QueryDiscover: queryUsage["schema"],
		Expand: map[string]string{
			"command":  "spec --json --command NAME for one command's usage, flags, and what it writes",
			"resource": "spec --json --resource KIND for one resource's definition",
			"section":  "spec --json --section NAME for one section, such as implementation_deck or theme",
			"all":      "spec --json --all for the whole contract, which is hundreds of kilobytes",
		},
		Sections: specSections(contract), Resources: []string{}, Commands: []string{},
	}
	for _, resource := range grammar.Resources() {
		index.Resources = append(index.Resources, resource.Kind)
	}
	for _, command := range grammar.Commands() {
		index.Commands = append(index.Commands, command.Name)
	}
	return index
}
