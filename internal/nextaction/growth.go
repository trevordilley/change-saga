package nextaction

import (
	"sort"
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/grammar"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
)

// The coverage areas an action advances.
const (
	AreaImplementation = string(areas.Implementation)
	AreaStories        = string(areas.Stories)
	AreaPersonas       = string(areas.Personas)
	AreaDesign         = string(areas.Design)
	AreaQuality        = string(areas.Quality)
	AreaHealth         = string(areas.Health)
	// AreaOverview and AreaTerms name growth that no coverage area counts:
	// the overview's pitch and description, and the project's vocabulary.
	AreaOverview = "overview"
	AreaTerms    = "terms"
)

// areaRank orders growth of equal value: the overview and the vocabulary
// first, since they are what a newcomer reads before anything else, then
// along the chain, a story first since every later link hangs from one.
func areaRank(area string) int {
	for index, name := range []string{AreaOverview, AreaTerms, AreaStories, AreaPersonas, AreaDesign, AreaQuality} {
		if name == area {
			return index
		}
	}
	return 6
}

// The practice each kind of growth teaches, and why it pays off.
const (
	practiceStories   = "Capture the story: a story says who a change serves and what done means. Written while the change is fresh, it lets the next change here say what it refines instead of rediscovering it, and links the code to the reason it exists."
	practicePersonas  = "Name the persona: a persona is a person who gets value from the app, the \"As a ...\" of a user story, never a tool or agent that operates it. Stories that name one show which people each change affects, and a persona no story serves shows what the app is missing."
	practiceDesign    = "Record the design: design explains how a story is met before the code does. It is what a reviewer compares the code against, and what the next person to change this reads first."
	practiceQuality   = "Verify the criterion: a test case turns an acceptance criterion into something checkable, so a later change that breaks it is caught rather than discovered."
	practiceCriteria  = "Write acceptance criteria: a criterion is an observable statement of done. It is what design addresses and a test verifies, so without one neither can be traced."
	practicePrototype = "Annotate the prototype: pinning part of a prototype to the story it clarifies keeps the experience and the requirement saying the same thing as both evolve."
	practiceTerms     = "Define the term: the words a team says every day are the least documented and fastest to rot. A term references the code that defines it, so renaming that code flags the term."
	practicePitch     = "Write the elevator pitch: two or three sentences on what the app is and who it is for. It is the first thing a newcomer reads, and every epic and story is read in its light."
	practiceAbout     = "Write the description: a short essay on what the app does, how it is organized, and the ideas a newcomer needs before the epics make sense. It is the orientation no single story gives."
)

// Context is what growth suggestions read beyond the living status: the
// coverage report and where each documentation target sits.
type Context struct {
	Coverage areas.Report
	// Places maps a documentation target to the slide (or other place) a
	// story link attaches to, with its title and epic.
	Places map[string]Place
	// DesignEpics holds each epic that has design content, so a design
	// suggestion in an epic with none starts by creating it.
	DesignEpics map[string]bool
}

// Place is where a story link attaches: usually the slide holding an Item.
type Place struct {
	Target string
	Title  string
	Epic   string
	// Slide is true for a slide or deck, whose link reaches its Items only
	// with scope descendants.
	Slide bool
}

// valueOf orders a growth suggestion: 0 when it concerns the current scope
// (a story or criterion the change reaches, or anything when observing the
// app), 1 otherwise.
func (b *builder) valueOf(resource string) int {
	scope := b.context.Coverage.Scope
	if scope.Kind == "" {
		return 1
	}
	epic := b.epicOf[resource]
	if scope.Epic != "" && epic != "" && epic != scope.Epic {
		return 1
	}
	if scope.Kind == areas.ScopeApp {
		return 0
	}
	coverage := b.context.Coverage.Areas
	for _, list := range [][]areas.Entry{coverage.Design.CoveredEntries, coverage.Design.UncoveredEntries, coverage.Quality.CoveredEntries, coverage.Quality.UncoveredEntries} {
		for _, entry := range list {
			if entry.Resource == resource || strings.HasPrefix(entry.Resource, resource+":") || strings.HasPrefix(resource, entry.Resource+":") {
				return 0
			}
		}
	}
	return 1
}

// growth offers the next link of the chain for what the coverage report
// found uncovered: a story for code no story explains, a persona for stories
// that name none, design for a story with none, and a test case for an
// acceptance criterion with none. Each is a question, because each needs the
// author's judgment, and "not now" is always a fine answer.
func (b *builder) growth() {
	if b.context.Coverage.Scope.Kind == "" {
		return
	}
	b.overviewGrowth()
	b.storyGrowth()
	b.personaGrowth()
	b.designGrowth()
	b.qualityGrowth()
}

// appValue is the value of growth that concerns the app rather than any
// change: the most valuable when observing the app, and after what the
// change touched when comparing.
func (b *builder) appValue() int {
	if b.context.Coverage.Scope.Kind == areas.ScopeApp {
		return 0
	}
	return 1
}

// overviewGrowth offers the overview's missing parts, the pitch and the
// description, and a first term when the project defines none. They are
// what a newcomer reads first, so documenting existing code starts there.
func (b *builder) overviewGrowth() {
	overview := b.status.Overview
	if overview.Name == "" {
		return
	}
	value := b.appValue()
	if overview.Pitch == nil {
		b.add(Action{
			ID: "growth:overview:pitch", Kind: KindCommand, Category: CategoryGrowth, Area: AreaOverview, value: value,
			Reason:   "the overview has no elevator pitch; say in two or three sentences what " + overview.Name + " is and who it is for?",
			Practice: practicePitch,
			Command:  ptr(b.invoke("overview set-pitch", grammar.V("text", ""))),
		})
	}
	if overview.Description == nil {
		b.add(Action{
			ID: "growth:overview:description", Kind: KindCommand, Category: CategoryGrowth, Area: AreaOverview, value: value, order: 1,
			Reason:   "the overview has no description; write the short essay a newcomer reads before the epics?",
			Practice: practiceAbout,
			Command:  ptr(b.invoke("overview set-description", grammar.V("text", ""))),
		})
	}
	if overview.Terms == 0 {
		b.add(Action{
			ID: "growth:terms:first", Kind: KindCommand, Category: CategoryGrowth, Area: AreaTerms, value: value,
			Reason:   "no term is defined yet; name a word the team says every day that a newcomer would not know, and the code that defines it?",
			Practice: practiceTerms,
			Command:  ptr(b.invoke("term add", grammar.V("id", ""), grammar.V("name", ""), grammar.V("definition", ""), grammar.V("ref", ""))),
		})
	}
}

type storyPlace struct {
	place Place
	count int
	files []string
}

func (b *builder) storyGrowth() {
	area := b.context.Coverage.Areas.Stories
	places := map[string]*storyPlace{}
	for _, entry := range area.UncoveredEntries {
		for _, target := range entry.Targets {
			if strings.Contains(target, ":test-case:") {
				// Test code reaches a story through the criterion its test
				// case verifies; an orphaned test case is asked about as such.
				continue
			}
			place, ok := b.context.Places[target]
			if !ok {
				place = Place{Target: target, Epic: entry.Epic}
			}
			value, ok := places[place.Target]
			if !ok {
				value = &storyPlace{place: place}
				places[place.Target] = value
			}
			value.count += entry.Count
			value.files = append(value.files, entry.Resource)
			break
		}
	}
	change := b.context.Coverage.Scope.Kind == areas.ScopeChange
	for _, value := range places {
		place := value.place
		title := firstNonEmptyString(place.Title, place.Target)
		id := slug(title)
		story, _ := livingid.Story(b.status.SagaID, id)
		reason := "\"" + title + "\" documents " + itoa(value.count) + " code targets no story explains; capture its story?"
		if change {
			reason = "this change touched \"" + title + "\" (" + countWords(value.count, "changed line", "changed lines") + " in " + fileList(value.files) +
				") and no story says why; capture the \"" + title + "\" story?"
		}
		link := []grammar.Value{grammar.V("id", id+"-code"), grammar.V("type", "addresses"), grammar.V("from", place.Target), grammar.V("to", story), grammar.V("rationale", "")}
		if place.Slide {
			link = append(link, grammar.V("scope", "descendants"))
		}
		existing := append([]grammar.Value{}, link...)
		existing[0], existing[3] = grammar.V("id", ""), grammar.V("to", "")
		b.add(Action{
			ID: "growth:story:" + place.Target, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaStories, Resource: place.Target, Epic: place.Epic,
			Reason: reason, Practice: practiceStories,
			// The more of the change a story would explain, the more it is worth.
			value: -value.count,
			Question: question("Which story does \""+title+"\" deliver?", NeedProductJudgment,
				option("capture it", "a proposed story, and an addresses relation so the code it explains reaches it",
					b.invoke("story add", grammar.V("id", id), grammar.V("revision", "r1"), grammar.V("event", "proposed"), grammar.V("title", title),
						grammar.V("statement", "")),
					b.invoke("relation add", link...)),
				option("an existing story covers it", "an addresses relation to that story", b.invoke("relation add", existing...)),
				option("not now", "nothing is recorded; the stories area keeps reporting the gap")),
		})
	}
}

func (b *builder) personaGrowth() {
	active := map[string]bool{}
	named := []string{}
	for _, persona := range b.status.Personas {
		if persona.State == "active" {
			active[persona.Persona] = true
			named = append(named, persona.ID)
		}
	}
	unnamed := []string{}
	for _, entry := range append(append([]areas.Entry{}, b.context.Coverage.Areas.Design.CoveredEntries...), b.context.Coverage.Areas.Design.UncoveredEntries...) {
		serves := false
		for _, persona := range b.stories[entry.Resource].Personas {
			serves = serves || active[persona]
		}
		if !serves {
			unnamed = append(unnamed, entry.Resource)
		}
	}
	sort.Strings(unnamed)
	if len(unnamed) > 0 {
		titles := []string{}
		assign := []grammar.Invocation{}
		define := []grammar.Invocation{b.invoke("persona add", grammar.V("id", ""), grammar.V("name", ""), grammar.V("description", ""))}
		for _, story := range unnamed {
			row := b.stories[story]
			titles = append(titles, "\""+firstNonEmptyString(row.Title, story)+"\"")
			revise := b.reviseStory(row, append(append([]string{}, row.Personas...), ""))
			assign = append(assign, revise)
			define = append(define, revise)
		}
		later := option("not now", "nothing is recorded; the personas area keeps reporting the gap")
		defineOption := option("define the persona they serve", "a persona, and a revision of each story that names it; every other field carries forward", define...)
		reason := countWords(len(unnamed), "story serves", "stories serve") + " someone you haven't named (" + someOf(titles, 3) + "); define the person who gets value from it?"
		options := []Option{defineOption, later}
		if len(named) > 0 {
			reason = countWords(len(unnamed), "story names", "stories name") + " no persona (" + someOf(titles, 3) + "); who gets value from them? Assign one you have named (" +
				someOf(named, 4) + "), or define that person?"
			options = []Option{
				option("a persona you have named", "a revision of each story that names it; every other field carries forward", assign...),
				defineOption, later,
			}
		}
		b.add(Action{
			ID: "growth:persona:unnamed", Kind: KindQuestion, Category: CategoryGrowth, Area: AreaPersonas,
			Reason: reason, Practice: practicePersonas,
			Question: question("Who gets value from "+strings.Join(titles, ", ")+"?", NeedProductJudgment, options...),
		})
		return
	}
	if len(b.status.Personas) == 0 && !b.context.Coverage.Areas.Personas.Complete {
		b.add(Action{
			ID: "growth:persona:first", Kind: KindCommand, Category: CategoryGrowth, Area: AreaPersonas,
			Reason:   "no persona is named yet, so no change can say whom it serves; which person gets value from this change?",
			Practice: practicePersonas,
			Command:  ptr(b.invoke("persona add", grammar.V("id", ""), grammar.V("name", ""), grammar.V("description", ""))),
		})
	}
}

// reviseStory is a story revision that sets the story's personas and carries
// every other field of the current revision forward: story revise writes a
// complete revision, so a field left out would be dropped. An empty persona
// is an input the author supplies.
func (b *builder) reviseStory(row livingapp.StoryStatus, personas []string) grammar.Invocation {
	values := []grammar.Value{grammar.V("story", row.Story), grammar.V("revision", "")}
	values = append(values, parents(row.RevisionHeads)...)
	values = append(values, grammar.V("title", row.Title), grammar.V("statement", row.Statement))
	if row.Priority != "" {
		values = append(values, grammar.V("priority", row.Priority))
	}
	for _, persona := range personas {
		values = append(values, grammar.V("persona", persona))
	}
	for _, criterion := range row.Criteria {
		values = append(values, grammar.V("criterion", criterion.ID+"="+criterion.Statement))
	}
	for _, citation := range row.Citations {
		values = append(values, grammar.V("citation", citation))
	}
	return b.invoke("story revise", values...).With("epic", row.Epic)
}

func (b *builder) designGrowth() {
	for _, entry := range b.context.Coverage.Areas.Design.UncoveredEntries {
		title := firstNonEmptyString(entry.Title, entry.Resource)
		relate := []grammar.Value{grammar.V("id", ""), grammar.V("type", "addresses"), grammar.V("from", ""), grammar.V("to", entry.Resource), grammar.V("rationale", "")}
		existing := option("an existing design, deck, slide, or Item explains it", "an addresses relation from it", b.invoke("relation add", relate...))
		write := option("write it", "a design chapter (add fragments to it with design add-fragment), then an addresses relation from the chapter or a fragment in it",
			b.invoke("design add-chapter", grammar.V("title", title)), b.invoke("relation add", relate...))
		later := option("not now", "nothing is recorded; the design area keeps reporting the gap")
		reason := "\"" + title + "\" has no design; add the design that explains how it is met?"
		options := []Option{existing, write, later}
		if !b.context.DesignEpics[entry.Epic] {
			// With no design to relate, the one command creates it.
			reason = "\"" + title + "\" has no design, and its epic has none yet; write the design that explains how it is met?"
			options = []Option{write, existing, later}
		}
		b.add(Action{
			ID: "growth:design:" + entry.Resource, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaDesign, Resource: entry.Resource, Epic: entry.Epic,
			Reason: reason, Practice: practiceDesign,
			value:    b.valueOf(entry.Resource),
			Question: question("What design explains how \""+title+"\" is met?", NeedProductJudgment, options...),
		})
	}
}

// qualityGrowth offers a test case for each story whose acceptance criteria
// no test verifies: one suggestion per story, naming its untested criteria,
// rather than one per criterion.
func (b *builder) qualityGrowth() {
	order := []string{}
	byStory := map[string][]areas.Entry{}
	for _, entry := range b.context.Coverage.Areas.Quality.UncoveredEntries {
		story := entry.Resource
		if index := strings.Index(story, ":criterion:"); index >= 0 {
			story = story[:index]
		}
		if _, ok := byStory[story]; !ok {
			order = append(order, story)
		}
		byStory[story] = append(byStory[story], entry)
	}
	for _, story := range order {
		entries := byStory[story]
		row := b.stories[story]
		title := "\"" + firstNonEmptyString(row.Title, story) + "\""
		relates := []grammar.Invocation{}
		statements := []string{}
		value := 1
		for _, entry := range entries {
			relates = append(relates, b.invoke("relation add", grammar.V("id", ""), grammar.V("type", "verifies"), grammar.V("from", ""), grammar.V("to", entry.Resource), grammar.V("rationale", "")))
			statements = append(statements, "\""+firstNonEmptyString(entry.Title, b.crit[entry.Resource].statement, entry.Resource)+"\"")
			value = min(value, b.valueOf(entry.Resource))
		}
		reason := "no test case verifies " + statements[0] + " of " + title + "; add one?"
		switch total := max(len(row.Criteria), len(entries)); {
		case len(entries) == total && total > 1:
			reason = "no test case verifies any of the " + itoa(total) + " acceptance criteria of " + title + "; add one?"
		case len(entries) > 1:
			reason = "no test case verifies " + itoa(len(entries)) + " of the " + itoa(total) + " acceptance criteria of " + title + "; add one?"
		}
		b.add(Action{
			ID: "growth:quality:" + story, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaQuality, Resource: story, Epic: firstNonEmptyString(row.Epic, entries[0].Epic),
			Reason: reason, Practice: practiceQuality, value: value,
			Question: question("Which test cases verify these acceptance criteria of "+title+": "+strings.Join(statements, "; ")+"?", NeedProductJudgment, b.testOptions(relates)...),
		})
	}
}

// testOptions offers an existing test case first when the app has any, and a
// new one first when it has none yet. Each relates the test case to every
// criterion the suggestion names.
func (b *builder) testOptions(relates []grammar.Invocation) []Option {
	existing := option("an existing test case verifies them", "a verifies relation from the test case to each criterion it verifies", relates...)
	fresh := option("a new test case is needed", "define its ordered steps and kinds, then relate it to each criterion it verifies", append([]grammar.Invocation{b.invoke("quality test-case add")}, relates...)...)
	later := option("not now", "nothing is recorded; the quality area keeps reporting the gap")
	if len(b.status.Quality.TestCases) == 0 {
		return []Option{fresh, existing, later}
	}
	return []Option{existing, fresh, later}
}

// slug turns a title into a stable lowercase identifier.
func slug(title string) string {
	var builder strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if unicode.IsLetter(r) && r < unicode.MaxASCII || unicode.IsDigit(r) {
			builder.WriteRune(r)
			dash = false
		} else if !dash && builder.Len() > 0 {
			builder.WriteByte('-')
			dash = true
		}
	}
	value := strings.Trim(builder.String(), "-")
	if len(value) > 48 {
		value = strings.Trim(value[:48], "-")
	}
	if value == "" {
		return "story"
	}
	return value
}

// someOf lists up to limit values and says how many more there are.
func someOf(values []string, limit int) string {
	if len(values) <= limit {
		return strings.Join(values, ", ")
	}
	return strings.Join(values[:limit], ", ") + " and " + itoa(len(values)-limit) + " more"
}

func fileList(files []string) string {
	files = unique(files)
	if len(files) <= 3 {
		return strings.Join(files, ", ")
	}
	return strings.Join(files[:3], ", ") + " and " + countWords(len(files)-3, "more file", "more files")
}

func countWords(count int, singular, plural string) string {
	if count == 1 {
		return "1 " + singular
	}
	return itoa(count) + " " + plural
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
