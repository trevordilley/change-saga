// Package areas computes the coverage report: for each area of the
// persona -> story -> design -> code chain, how many things in scope are
// covered, with the lists of what is and is not. It is a report, never a
// verdict: a gap is a finding, and deciding whether a gap matters is the
// team's rule, written over this report (or asked with `change-saga check`).
// Nothing here is ever reduced to one blended number.
//
// The package performs no I/O. Every input is a fact a loader established.
package areas

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitdiff"
)

// Name is one coverage area. The areas follow the chain, so each is one more
// link: implementation (changed code -> the deck), stories (-> a story),
// personas (-> a persona), design (story -> design), quality (criterion ->
// test), and health (the links that already exist still hold).
type Name string

const (
	Implementation Name = "implementation"
	Stories        Name = "stories"
	Personas       Name = "personas"
	Design         Name = "design"
	Quality        Name = "quality"
	Health         Name = "health"
)

// Names returns every area in chain order.
func Names() []Name {
	return []Name{Implementation, Stories, Personas, Design, Quality, Health}
}

// Parse reads a comma-separated list of area names, in the order given and
// without repeats. An unknown name is an error that lists the known ones.
func Parse(value string) ([]Name, error) {
	result := []Name{}
	seen := map[Name]bool{}
	for _, part := range strings.Split(value, ",") {
		name := Name(strings.TrimSpace(part))
		if name == "" {
			continue
		}
		if !known(name) {
			return nil, fmt.Errorf("unknown coverage area %q; areas are %s", name, strings.Join(nameStrings(), ", "))
		}
		if !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("name at least one coverage area: %s", strings.Join(nameStrings(), ", "))
	}
	return result, nil
}

func known(name Name) bool {
	for _, candidate := range Names() {
		if candidate == name {
			return true
		}
	}
	return false
}

func nameStrings() []string {
	result := []string{}
	for _, name := range Names() {
		result = append(result, string(name))
	}
	return result
}

// Unit is what an area counts.
type Unit string

const (
	// UnitChangedLine is one changed line of the comparison, or one file
	// event (a rename, mode, or binary change) counted as one.
	UnitChangedLine Unit = "changed_line"
	// UnitCodeTarget is one documentation target (usually an Item) that
	// references code: the unit of the line areas when observing, since
	// there is no change to count.
	UnitCodeTarget Unit = "code_target"
	UnitStory      Unit = "story"
	UnitCriterion  Unit = "criterion"
	UnitRecord     Unit = "record"
)

// Scope names what the report covers. Kind is "change" with --against (what
// the change changed and what it affected) or "app" without; Epic narrows
// either.
type Scope struct {
	Kind    string `json:"kind"`
	Against string `json:"against,omitempty"`
	Head    string `json:"head"`
	Epic    string `json:"epic,omitempty"`
}

const (
	ScopeChange = "change"
	ScopeApp    = "app"
)

// Entry is one covered or uncovered thing. Resource is a URN, or a
// repository path for changed lines, which then carry Side and Lines (for
// example "4-9,12"), or Event for a file event. Count is how many units the
// entry stands for, so the counts are always the sums of the entries. Via
// names what covers a covered entry, and Reason why an uncovered one is not.
// Targets, on the stories and personas lines, are the documentation targets
// that reference those lines: where a link to a story would attach.
type Entry struct {
	Resource string   `json:"resource"`
	Title    string   `json:"title,omitempty"`
	Epic     string   `json:"epic,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Side     string   `json:"side,omitempty"`
	Lines    string   `json:"lines,omitempty"`
	Event    string   `json:"event,omitempty"`
	Count    int      `json:"count"`
	Via      []string `json:"via"`
	Targets  []string `json:"targets,omitempty"`
	Reason   string   `json:"reason,omitempty"`
}

// Area is one row of the report. Complete means nothing in scope is
// uncovered; an area with nothing in scope is complete, because absence is
// not failure. Note explains an area that does not apply to this scope.
type Area struct {
	Area             Name    `json:"area"`
	Unit             Unit    `json:"unit"`
	Total            int     `json:"total"`
	Covered          int     `json:"covered"`
	Uncovered        int     `json:"uncovered"`
	Complete         bool    `json:"complete"`
	Note             string  `json:"note,omitempty"`
	CoveredEntries   []Entry `json:"covered_entries"`
	UncoveredEntries []Entry `json:"uncovered_entries"`
}

// Areas is every area, keyed by name so a rule can read one directly:
// .coverage.areas.stories.complete.
type Areas struct {
	Implementation Area `json:"implementation"`
	Stories        Area `json:"stories"`
	Personas       Area `json:"personas"`
	Design         Area `json:"design"`
	Quality        Area `json:"quality"`
	Health         Area `json:"health"`
}

// Report is the coverage report: its scope and its areas.
type Report struct {
	Scope Scope `json:"scope"`
	Areas Areas `json:"areas"`
}

// Get returns one area by name.
func (report Report) Get(name Name) Area {
	switch name {
	case Implementation:
		return report.Areas.Implementation
	case Stories:
		return report.Areas.Stories
	case Personas:
		return report.Areas.Personas
	case Design:
		return report.Areas.Design
	case Quality:
		return report.Areas.Quality
	default:
		return report.Areas.Health
	}
}

// All returns every area in chain order.
func (report Report) All() []Area {
	result := []Area{}
	for _, name := range Names() {
		result = append(result, report.Get(name))
	}
	return result
}

// Story is one story as the report reads it. Active is false for a retired
// or deferred story, which is out of every scope.
type Story struct {
	URN      string
	Title    string
	Epic     string
	Active   bool
	Personas []string
	Criteria []Criterion
}

// Criterion is one acceptance criterion of a story's current revision.
type Criterion struct {
	URN       string
	Statement string
}

// Problem is one existing record that went stale or broke.
type Problem struct {
	Resource string
	Kind     string
	Epic     string
	Reason   string
}

// Record is one existing record the health area examined.
type Record struct {
	Resource string
	Kind     string
	Epic     string
}

// Inputs is every fact the report reads.
type Inputs struct {
	Scope Scope
	// Atoms are the comparison's changed atoms; empty when observing.
	Atoms []gitdiff.Atom
	// Owners maps an atom key to the documentation targets whose code
	// references own it (or the test case whose evidence owns it).
	Owners map[string][]string
	// CodeTargets are the documentation targets that reference code, the
	// unit of the line areas when observing.
	CodeTargets []string
	// TargetEpic is the epic holding each target; TargetTitle its title.
	TargetEpic  map[string]string
	TargetTitle map[string]string
	// TargetStories maps a target to the stories it reaches.
	TargetStories map[string][]string
	Stories       []Story
	// ActivePersonas are the personas the app serves now.
	ActivePersonas map[string]bool
	// InScope names the stories a comparison changed or affected; nil when
	// observing, where every active story is in scope.
	InScope map[string]bool
	// StoryDesign and CriterionTests are the story -> design and criterion ->
	// test links; Excluded names the criteria an exception resolves.
	StoryDesign     map[string][]string
	CriterionTests  map[string][]string
	DesignExcluded  map[string]bool
	QualityExcluded map[string]bool
	// Examined are the existing records health checked; Problems the ones
	// that went stale or broke.
	Examined []Record
	Problems []Problem
}

// Evaluate computes the report.
func Evaluate(in Inputs) Report {
	stories := map[string]Story{}
	for _, story := range in.Stories {
		stories[story.URN] = story
	}
	e := evaluator{in: in, stories: stories}
	report := Report{Scope: in.Scope}
	if in.Scope.Kind == ScopeChange {
		report.Areas.Implementation, report.Areas.Stories, report.Areas.Personas = e.lineAreas()
	} else {
		report.Areas.Implementation, report.Areas.Stories, report.Areas.Personas = e.targetAreas()
	}
	scoped := e.scopedStories(report.Areas.Stories)
	report.Areas.Design = e.design(scoped)
	report.Areas.Quality = e.quality(scoped)
	report.Areas.Health = e.health()
	return report
}

type evaluator struct {
	in      Inputs
	stories map[string]Story
}

// inEpic reports whether a record in epic belongs to the scope. A changed
// line no record owns belongs to no epic yet, so it counts in every epic's
// scope: it could belong to any of them.
func (e evaluator) inEpic(epic string, unowned bool) bool {
	return e.in.Scope.Epic == "" || epic == e.in.Scope.Epic || unowned
}

// storiesOf returns the active stories a set of targets reaches.
func (e evaluator) storiesOf(targets []string) []string {
	result := []string{}
	for _, target := range targets {
		for _, story := range e.in.TargetStories[target] {
			if e.stories[story].Active {
				result = append(result, story)
			}
		}
	}
	return uniqueSorted(result)
}

// personasOf returns the active personas a set of stories serves.
func (e evaluator) personasOf(stories []string) []string {
	result := []string{}
	for _, story := range stories {
		for _, persona := range e.stories[story].Personas {
			if e.in.ActivePersonas[persona] {
				result = append(result, persona)
			}
		}
	}
	return uniqueSorted(result)
}

// lineAreas counts changed lines for implementation, stories, and personas.
func (e evaluator) lineAreas() (Area, Area, Area) {
	implementation := lines{area: Implementation}
	stories := lines{area: Stories}
	personas := lines{area: Personas}
	for _, atom := range e.in.Atoms {
		owners := e.in.Owners[atom.Key]
		epic := e.epicOf(owners)
		if !e.inEpic(epic, len(owners) == 0) {
			continue
		}
		reached := e.storiesOf(owners)
		served := e.personasOf(reached)
		implementation.add(atom, epic, owners, nil, "no deck Item references this changed line")
		stories.add(atom, epic, reached, owners, "reaches no story: its Item addresses no story")
		personas.add(atom, epic, served, owners, "reaches no persona: the stories it reaches name none")
	}
	return implementation.finish(), stories.finish(), personas.finish()
}

func (e evaluator) epicOf(targets []string) string {
	for _, target := range targets {
		if epic := e.in.TargetEpic[target]; epic != "" {
			return epic
		}
	}
	return ""
}

// targetAreas counts code-referencing targets when observing: there is no
// change, so implementation does not apply, and the chain is asked of the
// code the Saga already documents.
func (e evaluator) targetAreas() (Area, Area, Area) {
	implementation := newArea(Implementation, UnitChangedLine)
	implementation.Note = "no change to account for when observing; compare with --against REV"
	stories := newArea(Stories, UnitCodeTarget)
	personas := newArea(Personas, UnitCodeTarget)
	for _, target := range uniqueSorted(e.in.CodeTargets) {
		epic := e.in.TargetEpic[target]
		if !e.inEpic(epic, false) {
			continue
		}
		reached := e.storiesOf([]string{target})
		entry := Entry{Resource: target, Title: e.in.TargetTitle[target], Epic: epic, Count: 1, Targets: []string{target}}
		stories.record(entry, reached, "addresses no story")
		personas.record(entry, e.personasOf(reached), "reaches no persona: the stories it reaches name none")
	}
	return implementation.finish(), stories.finish(), personas.finish()
}

// scopedStories returns the active stories in scope, sorted: when observing,
// every active story (in the epic); when comparing, the stories the change
// changed or affected plus the stories its changed lines reach.
func (e evaluator) scopedStories(storyArea Area) []Story {
	reached := map[string]bool{}
	if e.in.Scope.Kind == ScopeChange {
		for _, entry := range storyArea.CoveredEntries {
			for _, story := range entry.Via {
				reached[story] = true
			}
		}
	}
	result := []Story{}
	for _, story := range e.in.Stories {
		if !story.Active || !e.inEpic(story.Epic, false) {
			continue
		}
		if e.in.Scope.Kind == ScopeChange && !e.in.InScope[story.URN] && !reached[story.URN] {
			continue
		}
		result = append(result, story)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URN < result[j].URN })
	return result
}

func (e evaluator) design(stories []Story) Area {
	area := newArea(Design, UnitStory)
	for _, story := range stories {
		entry := Entry{Resource: story.URN, Title: story.Title, Epic: story.Epic, Count: 1}
		via := append([]string{}, e.in.StoryDesign[story.URN]...)
		if len(via) == 0 && len(story.Criteria) > 0 {
			excluded := true
			for _, criterion := range story.Criteria {
				excluded = excluded && e.in.DesignExcluded[criterion.URN]
			}
			if excluded {
				via = []string{"coverage exception"}
			}
		}
		area.record(entry, via, "no design addresses this story")
	}
	return area.finish()
}

func (e evaluator) quality(stories []Story) Area {
	area := newArea(Quality, UnitCriterion)
	for _, story := range stories {
		for _, criterion := range story.Criteria {
			entry := Entry{Resource: criterion.URN, Title: criterion.Statement, Epic: story.Epic, Count: 1}
			via := append([]string{}, e.in.CriterionTests[criterion.URN]...)
			if len(via) == 0 && e.in.QualityExcluded[criterion.URN] {
				via = []string{"coverage exception"}
			}
			area.record(entry, via, "no test case verifies this acceptance criterion")
		}
	}
	return area.finish()
}

func (e evaluator) health() Area {
	area := newArea(Health, UnitRecord)
	problems := map[string][]Problem{}
	for _, problem := range e.in.Problems {
		problems[problem.Resource] = append(problems[problem.Resource], problem)
	}
	seen := map[string]bool{}
	records := append([]Record{}, e.in.Examined...)
	for _, problem := range e.in.Problems {
		records = append(records, Record{Resource: problem.Resource, Kind: problem.Kind, Epic: problem.Epic})
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Resource < records[j].Resource })
	for _, record := range records {
		if seen[record.Resource] || !e.inEpic(record.Epic, false) {
			continue
		}
		seen[record.Resource] = true
		entry := Entry{Resource: record.Resource, Kind: record.Kind, Epic: record.Epic, Count: 1, Via: []string{}}
		found := problems[record.Resource]
		if len(found) == 0 {
			area.Covered++
			area.CoveredEntries = append(area.CoveredEntries, entry)
			continue
		}
		reasons := []string{}
		for _, problem := range found {
			reasons = append(reasons, problem.Reason)
		}
		entry.Kind = found[0].Kind
		if entry.Epic == "" {
			entry.Epic = found[0].Epic
		}
		entry.Reason = strings.Join(uniqueSorted(reasons), "; ")
		area.Uncovered++
		area.UncoveredEntries = append(area.UncoveredEntries, entry)
	}
	return area.finish()
}

func newArea(name Name, unit Unit) Area {
	return Area{Area: name, Unit: unit, CoveredEntries: []Entry{}, UncoveredEntries: []Entry{}}
}

// record adds one entry: covered when via names anything.
func (area *Area) record(entry Entry, via []string, reason string) {
	entry.Via = uniqueSorted(via)
	if len(entry.Via) > 0 {
		area.Covered += entry.Count
		area.CoveredEntries = append(area.CoveredEntries, entry)
		return
	}
	entry.Reason = reason
	area.Uncovered += entry.Count
	area.UncoveredEntries = append(area.UncoveredEntries, entry)
}

func (area Area) finish() Area {
	area.Total = area.Covered + area.Uncovered
	area.Complete = area.Uncovered == 0
	return area
}

// lines groups changed lines into entries: one per path, side, epic, and
// covering set, with the line numbers compressed into ranges.
type lines struct {
	area   Name
	groups map[string]*lineGroup
	order  []string
}

type lineGroup struct {
	entry   Entry
	covered bool
	numbers []int
}

func (l *lines) add(atom gitdiff.Atom, epic string, via, targets []string, reason string) {
	if l.groups == nil {
		l.groups = map[string]*lineGroup{}
	}
	via, targets = uniqueSorted(via), uniqueSorted(targets)
	if len(targets) == 0 {
		targets = nil
	}
	path := firstNonEmpty(atom.Path, atom.NewPath, atom.OldPath)
	key := strings.Join([]string{path, atom.Side, atom.Event, epic, strings.Join(via, "\x00"), strings.Join(targets, "\x00")}, "\x01")
	if atom.Kind == "event" {
		key += "\x01" + atom.Key
	}
	group, ok := l.groups[key]
	if !ok {
		group = &lineGroup{entry: Entry{Resource: path, Epic: epic, Side: atom.Side, Event: atom.Event, Via: via, Targets: targets}, covered: len(via) > 0}
		if !group.covered {
			group.entry.Reason = reason
		}
		l.groups[key] = group
		l.order = append(l.order, key)
	}
	group.entry.Count++
	if atom.Kind != "event" && atom.Line > 0 {
		group.numbers = append(group.numbers, atom.Line)
	}
}

func (l *lines) finish() Area {
	area := newArea(l.area, UnitChangedLine)
	keys := append([]string{}, l.order...)
	sort.Strings(keys)
	for _, key := range keys {
		group := l.groups[key]
		group.entry.Lines = ranges(group.numbers)
		if group.covered {
			area.Covered += group.entry.Count
			area.CoveredEntries = append(area.CoveredEntries, group.entry)
		} else {
			area.Uncovered += group.entry.Count
			area.UncoveredEntries = append(area.UncoveredEntries, group.entry)
		}
	}
	return area.finish()
}

// ranges compresses line numbers into "4-9,12".
func ranges(numbers []int) string {
	if len(numbers) == 0 {
		return ""
	}
	sorted := append([]int{}, numbers...)
	sort.Ints(sorted)
	parts := []string{}
	start, end := sorted[0], sorted[0]
	flush := func() {
		if start == end {
			parts = append(parts, strconv.Itoa(start))
		} else {
			parts = append(parts, strconv.Itoa(start)+"-"+strconv.Itoa(end))
		}
	}
	for _, number := range sorted[1:] {
		switch {
		case number == end || number == end+1:
			end = number
		default:
			flush()
			start, end = number, number
		}
	}
	flush()
	return strings.Join(parts, ",")
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
