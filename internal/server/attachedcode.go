package server

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// attachedCodeView is the narrative-first projection of a target's evidence.
// It preserves exact diff atoms while presenting the file as the first unit a
// reviewer chooses to expand.
type attachedCodeView struct {
	Title           string
	ChangeCount     int
	LineCount       int
	Added           int
	Deleted         int
	EventCount      int
	LinkedLineCount int
	LinkedEvents    int
	Files           []*attachedCodeFileView
}

type attachedCodeFileView struct {
	Path           string
	Href           string
	Target         string
	Summary        string
	MissingSummary bool
	Added          int
	Deleted        int
	Events         int
	Changes        int
	LinkedLines    int
	LinkedEvents   int
}

func makeAttachedCodeView(title, target string, linked, full []gitdiff.Atom, evidence []saga.CodeFile) *attachedCodeView {
	return makeAttachedCodeViewFromAtoms(
		title, target,
		len(linked), func(index int) gitdiff.Atom { return linked[index] },
		len(full), func(index int) gitdiff.Atom { return full[index] },
		evidence,
	)
}

func makeAttachedCodeViewIndexed(title, target string, snapshot *reviewSnapshot, indexes []int, evidence []saga.CodeFile) *attachedCodeView {
	fullIndexes := make([]int, 0)
	for _, path := range snapshot.targetFiles[target] {
		fullIndexes = append(fullIndexes, snapshot.fileAtoms[path]...)
	}
	return makeAttachedCodeViewFromAtoms(
		title, target,
		len(indexes), func(index int) gitdiff.Atom { return snapshot.changes.Atoms[indexes[index]] },
		len(fullIndexes), func(index int) gitdiff.Atom { return snapshot.changes.Atoms[fullIndexes[index]] },
		evidence,
	)
}

func makeAttachedCodeViewFromAtoms(title, target string, linkedCount int, linkedAt func(int) gitdiff.Atom, fullCount int, fullAt func(int) gitdiff.Atom, evidence []saga.CodeFile) *attachedCodeView {
	if linkedCount == 0 {
		return nil
	}
	view := &attachedCodeView{Title: title}
	byPath := map[string]*attachedCodeFileView{}
	for index := 0; index < fullCount; index++ {
		atom := fullAt(index)
		path := effectiveAtomPath(atom)
		file := byPath[path]
		if file == nil {
			file = &attachedCodeFileView{Path: path, Href: CodeDiffURL(path, ""), Target: target}
			byPath[path] = file
			view.Files = append(view.Files, file)
		}
		file.Changes++
		switch {
		case atom.Kind == "event":
			file.Events++
		case atom.Side == "old":
			file.Deleted++
		default:
			file.Added++
		}
	}
	for index := 0; index < linkedCount; index++ {
		atom := linkedAt(index)
		file := byPath[effectiveAtomPath(atom)]
		if file == nil {
			continue
		}
		if atom.Kind == "event" {
			file.LinkedEvents++
			view.LinkedEvents++
		} else {
			file.LinkedLines++
			view.LinkedLineCount++
		}
	}
	for _, file := range view.Files {
		view.Added += file.Added
		view.Deleted += file.Deleted
		view.EventCount += file.Events
	}
	view.LineCount = view.Added + view.Deleted
	view.ChangeCount = view.LineCount
	if view.ChangeCount == 0 {
		view.ChangeCount = view.EventCount
	}

	for path, notes := range attachedFileNotesFromAtoms(linkedCount, linkedAt, evidence) {
		if file := byPath[path]; file != nil {
			file.Summary = strings.Join(notes, " ")
		}
	}
	for _, file := range view.Files {
		if file.Summary == "" {
			file.Summary = "No change summary was authored for this file."
			file.MissingSummary = true
		}
	}
	sort.SliceStable(view.Files, func(i, j int) bool { return view.Files[i].Path < view.Files[j].Path })
	return view
}

// attachedFileNotes collects the authored notes that apply to each changed
// file: a note applies to the file its reference names when that file holds a
// linked atom. The linked atoms were already selected by resolving the same
// references, so this pass only groups notes and never re-resolves them.
func attachedFileNotes(atoms []gitdiff.Atom, evidence []saga.CodeFile) map[string][]string {
	return attachedFileNotesFromAtoms(len(atoms), func(index int) gitdiff.Atom { return atoms[index] }, evidence)
}

func attachedFileNotesFromAtoms(atomCount int, atomAt func(int) gitdiff.Atom, evidence []saga.CodeFile) map[string][]string {
	notes := map[string][]string{}
	var linked map[string]string
	for _, file := range evidence {
		for _, reference := range file.References {
			note := strings.TrimSpace(reference.Note)
			if note == "" {
				continue
			}
			// Evidence without authored notes never builds the path index.
			if linked == nil {
				linked = make(map[string]string, atomCount)
				for index := 0; index < atomCount; index++ {
					atom := atomAt(index)
					path := effectiveAtomPath(atom)
					for _, candidate := range []string{atom.Path, atom.OldPath, atom.NewPath} {
						if candidate != "" {
							linked[candidate] = path
						}
					}
				}
			}
			path, ok := linked[reference.Path]
			if ok && !contains(notes[path], note) {
				notes[path] = append(notes[path], note)
			}
		}
	}
	return notes
}
