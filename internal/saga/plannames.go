package saga

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// A Plan's names are a hierarchy of the abstractions it works on. A name labels
// an abstraction, and an abstraction hides details: a Plan has a few top-level
// names, and zooming in on one shows the more detailed names beneath it, down to
// the parts its Tasks work on. Names link to one another. Tasks name the names
// they work on, so each name's progress is derived from the Tasks beneath it, and
// work on no name, or a name no Task reaches, stands out.
//
// A name belongs to its Plan, beside the Tasks, and Git is its history. It may
// point at a Term, Component, or System of the Saga that it means. It holds no
// code: its code is the code of the Tasks beneath it.
//
//	___plans/<id>.plan/names/<id>.name/name.json
const (
	PlanNamesDir     = "names"
	NameSuffix       = ".name"
	NameManifestName = "name.json"
	NameSchemaURL    = "https://changesaga.dev/schema/v5/name.schema.json"

	// MaxNameLinks bounds the links one name holds.
	MaxNameLinks = 64
)

// NameRecordKinds are the kinds of Saga record a name may point at.
var NameRecordKinds = []string{"term", "component", "system"}

// NameLink is a labeled link from one name to another of the same Plan.
type NameLink struct {
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// NameManifest is name.json.
type NameManifest struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	// Parent is the name this one details. A name without one is top-level.
	Parent string     `json:"parent,omitempty"`
	Links  []NameLink `json:"links,omitempty"`
	// Record is the URN of a Term, Component, or System the name means.
	Record    string    `json:"record,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// PlanName is one loaded name of a Plan.
type PlanName struct {
	NameManifest
	// Path is the app-relative name directory.
	Path      string `json:"path"`
	Directory string `json:"-"`
	Target    string `json:"target"`
}

func NameTarget(sagaID, planID, nameID string) string {
	return PlanTarget(sagaID, planID) + ":name:" + nameID
}

// FindName returns the Plan's name with id or target.
func (plan *Plan) FindName(id string) *PlanName {
	for _, name := range plan.Names {
		if name.ID == id || name.Target == id {
			return name
		}
	}
	return nil
}

// NameManifests returns the Plan's name manifests, for checking a candidate.
func (plan *Plan) NameManifests() []NameManifest {
	manifests := make([]NameManifest, 0, len(plan.Names))
	for _, name := range plan.Names {
		manifests = append(manifests, name.NameManifest)
	}
	return manifests
}

// ValidateNameManifest checks one name.json on its own: everything except how
// it relates to the rest of its Plan, which PlanNameProblems checks.
func ValidateNameManifest(sagaID string, manifest NameManifest) error {
	switch {
	case manifest.Schema != NameSchemaURL:
		return fmt.Errorf("$schema must be %s", NameSchemaURL)
	case manifest.Version != PlanVersion:
		return fmt.Errorf("version must be %d", PlanVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	case manifest.Parent != "" && !ValidID(manifest.Parent):
		return fmt.Errorf("parent must be the id of a name of this Plan")
	case manifest.Parent == manifest.ID:
		return fmt.Errorf("a name cannot be its own parent")
	case manifest.Record != "" && !validRecordOfKinds(sagaID, manifest.Record, NameRecordKinds):
		return fmt.Errorf("record must be a canonical term, component, or system URN of this Saga")
	case len(manifest.Links) > MaxNameLinks:
		return fmt.Errorf("a name holds at most %d links", MaxNameLinks)
	}
	seen := map[string]bool{}
	for _, link := range manifest.Links {
		switch {
		case !ValidID(link.To):
			return fmt.Errorf("a link must name the id of a name of this Plan")
		case link.To == manifest.ID:
			return fmt.Errorf("a name cannot link to itself")
		case seen[link.To]:
			return fmt.Errorf("it links to %q more than once", link.To)
		case link.Label != strings.TrimSpace(link.Label):
			return fmt.Errorf("a link's label must not begin or end with space")
		}
		seen[link.To] = true
	}
	return nil
}

// NameProblem is one name or Task a Plan's names refuse, and why. Exactly one of
// Name and Task is set.
type NameProblem struct {
	Name    string `json:"name,omitempty"`
	Task    string `json:"task,omitempty"`
	Message string `json:"message"`
}

func (problem NameProblem) String() string {
	if problem.Task != "" {
		return "task " + problem.Task + ": " + problem.Message
	}
	return "name " + problem.Name + ": " + problem.Message
}

// PlanNameProblems checks a Plan's names as a hierarchy: each parent and link
// names a name of the Plan, parents form no cycle, and each Task names only names
// of the Plan, once each. It runs in time linear in the names, links, and Task
// names, so a command can afford it on every write.
func PlanNameProblems(names []NameManifest, tasks []TaskManifest) []NameProblem {
	problems := []NameProblem{}
	known := make(map[string]bool, len(names))
	for _, name := range names {
		if known[name.ID] {
			problems = append(problems, NameProblem{Name: name.ID, Message: "the name id appears more than once"})
		}
		known[name.ID] = true
	}
	parent := make(map[string]string, len(names))
	for _, name := range names {
		if name.Parent != "" {
			if known[name.Parent] {
				parent[name.ID] = name.Parent
			} else {
				problems = append(problems, NameProblem{Name: name.ID, Message: fmt.Sprintf("its parent %q is not a name of this Plan", name.Parent)})
			}
		}
		for _, link := range name.Links {
			if !known[link.To] {
				problems = append(problems, NameProblem{Name: name.ID, Message: fmt.Sprintf("it links to %q, which is not a name of this Plan", link.To)})
			}
		}
	}
	// One parent each, so following parents from any name either reaches a
	// top-level name or loops. Each name is walked once.
	const (
		visiting = 1
		finished = 2
	)
	marks := make(map[string]int, len(names))
	for _, name := range names {
		path := []string{}
		current := name.ID
		for current != "" && marks[current] == 0 {
			marks[current] = visiting
			path = append(path, current)
			current = parent[current]
		}
		if current != "" && marks[current] == visiting {
			start := 0
			for index, value := range path {
				if value == current {
					start = index
				}
			}
			cycle := append(append([]string{}, path[start:]...), current)
			problems = append(problems, NameProblem{Name: path[len(path)-1], Message: "parent cycle: " + strings.Join(cycle, " -> ")})
		}
		for _, id := range path {
			marks[id] = finished
		}
	}
	for _, task := range tasks {
		seen := map[string]bool{}
		for _, id := range task.Names {
			switch {
			case !known[id]:
				problems = append(problems, NameProblem{Task: task.ID, Message: fmt.Sprintf("it names %q, which is not a name of this Plan", id)})
			case seen[id]:
				problems = append(problems, NameProblem{Task: task.ID, Message: fmt.Sprintf("it names %q more than once", id)})
			}
			seen[id] = true
		}
	}
	return problems
}

// NameReferences says what still refers to a name: its children, the names that
// link to it, and the Tasks that name it. A name is removed only once none do.
func NameReferences(names []NameManifest, tasks []TaskManifest, id string) (children, linkedFrom, taskIDs []string) {
	for _, name := range names {
		if name.Parent == id {
			children = append(children, name.ID)
		}
		for _, link := range name.Links {
			if link.To == id {
				linkedFrom = append(linkedFrom, name.ID)
			}
		}
	}
	for _, task := range tasks {
		for _, named := range task.Names {
			if named == id {
				taskIDs = append(taskIDs, task.ID)
			}
		}
	}
	return children, linkedFrom, taskIDs
}

// The derived states of a name.
const (
	// NameDone is a name every Task beneath it is done.
	NameDone = "done"
	// NameUnderWay is a name some Task beneath it has started or finished.
	NameUnderWay = "under_way"
	// NamePlanned is a name whose Tasks have not started.
	NamePlanned = "planned"
	// NameUnreached is a name no Task reaches: maybe unneeded, or a Task is missing.
	NameUnreached = "unreached"
)

// NameProgress is what a name's Tasks say about it. A Task beneath a name names
// it or any name beneath it; it is counted once however many of those it names.
type NameProgress struct {
	ID       string   `json:"id"`
	State    string   `json:"state"`
	Done     int      `json:"done"`
	All      int      `json:"all"`
	Children []string `json:"children,omitempty"`
	// Depth is 0 for a top-level name.
	Depth int `json:"depth"`
	// Tasks are the Tasks that name this name itself, in Task order.
	Tasks []string `json:"tasks,omitempty"`
}

// NamesView is a Plan's names as a hierarchy, with each name's progress and the
// gaps between the names and the Tasks.
type NamesView struct {
	// Roots are the top-level names, in the order they were added; so are Children.
	Roots    []string                 `json:"roots"`
	Progress map[string]*NameProgress `json:"progress"`
	// Unnamed are the Tasks that name no name: unexpected work.
	Unnamed []string `json:"unnamed,omitempty"`
	// Unreached are the names no Task reaches, in hierarchy order: maybe unneeded.
	// A name is listed only when no ancestor is, so one missing branch is one entry.
	Unreached []string `json:"unreached,omitempty"`
}

// PlanNames derives a Plan's names view. It walks each name and its ancestors
// once per Task that names it, so it is linear in the Task names times the depth.
// A Plan without names returns nil: it reports no gaps.
func PlanNames(names []NameManifest, tasks []TaskManifest) *NamesView {
	if len(names) == 0 {
		return nil
	}
	byID := make(map[string]NameManifest, len(names))
	for _, name := range names {
		byID[name.ID] = name
	}
	view := &NamesView{Progress: make(map[string]*NameProgress, len(names))}
	for _, name := range names {
		view.Progress[name.ID] = &NameProgress{ID: name.ID}
	}
	// Names keep the order they were added in, which is the order the author
	// thought of them.
	inOrder := func(ids []string) {
		sort.SliceStable(ids, func(i, j int) bool {
			left, right := byID[ids[i]].CreatedAt, byID[ids[j]].CreatedAt
			if !left.Equal(right) {
				return left.Before(right)
			}
			return ids[i] < ids[j]
		})
	}
	for _, name := range names {
		if _, ok := byID[name.Parent]; ok && name.Parent != "" {
			view.Progress[name.Parent].Children = append(view.Progress[name.Parent].Children, name.ID)
		} else {
			view.Roots = append(view.Roots, name.ID)
		}
	}
	inOrder(view.Roots)
	for _, progress := range view.Progress {
		inOrder(progress.Children)
	}
	started := map[string]bool{}
	for _, task := range tasks {
		if len(task.Names) == 0 {
			view.Unnamed = append(view.Unnamed, task.ID)
			continue
		}
		counted, direct := map[string]bool{}, map[string]bool{}
		for _, id := range task.Names {
			if progress := view.Progress[id]; progress != nil && !direct[id] {
				direct[id] = true
				progress.Tasks = append(progress.Tasks, task.ID)
			}
			// Each ancestor counts the Task once, and a parent cycle stops the walk.
			for current, steps := id, 0; current != "" && steps <= len(names); current, steps = byID[current].Parent, steps+1 {
				progress := view.Progress[current]
				if progress == nil || counted[current] {
					break
				}
				counted[current] = true
				progress.All++
				if task.State == TaskDone {
					progress.Done++
				}
				if task.State != TaskPlanned {
					started[current] = true
				}
			}
		}
	}
	for id, progress := range view.Progress {
		switch {
		case progress.All == 0:
			progress.State = NameUnreached
		case progress.Done == progress.All:
			progress.State = NameDone
		case started[id]:
			progress.State = NameUnderWay
		default:
			progress.State = NamePlanned
		}
	}
	visited := map[string]bool{}
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		if visited[id] {
			return
		}
		visited[id] = true
		progress := view.Progress[id]
		progress.Depth = depth
		if progress.State == NameUnreached {
			view.Unreached = append(view.Unreached, id)
			for _, child := range progress.Children {
				markDepth(view, child, depth+1, visited)
			}
			return
		}
		for _, child := range progress.Children {
			walk(child, depth+1)
		}
	}
	for _, root := range view.Roots {
		walk(root, 0)
	}
	return view
}

// markDepth records the depth of a branch already reported as unreached.
func markDepth(view *NamesView, id string, depth int, visited map[string]bool) {
	if visited[id] {
		return
	}
	visited[id] = true
	view.Progress[id].Depth = depth
	for _, child := range view.Progress[id].Children {
		markDepth(view, child, depth+1, visited)
	}
}

// Ancestry returns the ids from a top-level name down to id.
func (view *NamesView) Ancestry(names []NameManifest, id string) []string {
	parent := make(map[string]string, len(names))
	for _, name := range names {
		parent[name.ID] = name.Parent
	}
	chain := []string{}
	for current, steps := id, 0; current != "" && steps <= len(names); current, steps = parent[current], steps+1 {
		if view.Progress[current] == nil {
			break
		}
		chain = append([]string{current}, chain...)
	}
	return chain
}

// loadNames loads a Plan's names directory.
func loadNames(root string, plan *Plan, manifest Manifest, validation *Validation) error {
	namesDir := filepath.Join(plan.Directory, PlanNamesDir)
	if !realDirectoryExists(namesDir) {
		return nil
	}
	entries, err := os.ReadDir(namesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".change-saga-") {
			continue
		}
		path := filepath.Join(namesDir, entry.Name())
		nameID := strings.TrimSuffix(entry.Name(), NameSuffix)
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), NameSuffix) || !ValidID(nameID) {
			addIssue(validation, "error", relativePath(root, path), "names must be real <id>.name directories")
			continue
		}
		if name := loadName(root, path, nameID, plan, manifest, validation); name != nil {
			plan.Names = append(plan.Names, name)
		}
	}
	sort.Slice(plan.Names, func(i, j int) bool { return plan.Names[i].ID < plan.Names[j].ID })
	for _, problem := range PlanNameProblems(plan.NameManifests(), plan.Manifests()) {
		path := filepath.Join(namesDir, problem.Name+NameSuffix)
		if problem.Task != "" {
			path = filepath.Join(plan.Directory, PlanTasksDir, problem.Task+TaskSuffix)
		}
		addIssue(validation, "error", relativePath(root, path), problem.String())
	}
	return nil
}

func loadName(root, dir, id string, plan *Plan, manifest Manifest, validation *Validation) *PlanName {
	manifestPath := filepath.Join(dir, NameManifestName)
	var value NameManifest
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		return nil
	}
	if err := ValidateNameManifest(manifest.ID, value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
	}
	if value.ID != id {
		addIssue(validation, "error", relativePath(root, manifestPath), fmt.Sprintf("name id %q must match its directory %q", value.ID, id+NameSuffix))
	}
	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if entry.Name() != NameManifestName && !strings.HasPrefix(entry.Name(), ".change-saga-") {
				addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown entry in a name; a name holds name.json")
			}
		}
	}
	return &PlanName{NameManifest: value, Path: relativePath(root, dir), Directory: dir, Target: NameTarget(manifest.ID, plan.ID, id)}
}
