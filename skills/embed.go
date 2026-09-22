// Package skills embeds the agent skills shipped with Change Saga, so the
// files in this directory are the one source of what "change-saga
// install-skill" prints.
package skills

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed change-saga/SKILL.md change-saga/agents/*.yaml change-saga/references/*.md
var bundled embed.FS

// File is one file of a skill, named by its path relative to the skill's
// directory.
type File struct {
	Path    string
	Content string
}

// skillFiles returns one embedded skill with SKILL.md first, then its
// remaining files in path order.
func skillFiles(name string) []File {
	skill, err := fs.Sub(bundled, name)
	if err != nil {
		panic(err)
	}
	var files []File
	err = fs.WalkDir(skill, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(skill, path)
		if err != nil {
			return err
		}
		files = append(files, File{Path: path, Content: string(content)})
		return nil
	})
	if err != nil {
		panic(err)
	}
	sort.SliceStable(files, func(i, j int) bool {
		if (files[i].Path == "SKILL.md") != (files[j].Path == "SKILL.md") {
			return files[i].Path == "SKILL.md"
		}
		return files[i].Path < files[j].Path
	})
	return files
}

// ChangeSaga returns the general-purpose authoring skill.
func ChangeSaga() []File { return skillFiles("change-saga") }
