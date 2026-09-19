package nextaction

import (
	"sort"
	"strings"
	"unicode"

	"github.com/twentyideas/changesaga/internal/areas"
	"github.com/twentyideas/changesaga/internal/grammar"
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
)

// areaRank orders growth along the chain: a story first, since every later
// link hangs from one, and terms last.
func areaRank(area string) int {
	for index, name := range []string{AreaStories, AreaPersonas, AreaDesign, AreaQuality} {
		if name == area {
			return index
		}
	}
	return 4
}

// The practice each kind of growth teaches, and why it pays off.
const (
	practiceStories   = "Capture the story: a story says who a change serves and what done means. Written while the change is fresh, it lets the next change here say what it refines instead of rediscovering it, and links the code to the reason it exists."
	practicePersonas  = "Name the persona: a persona is who the app serves. Stories that name one show which people each change affects, and a persona no story serves shows what the app is missing."
	practiceDesign    = "Record the design: design explains how a story is met before the code does. It is what a reviewer compares the code against, and what the next person to change this reads first."
	practiceQuality   = "Verify the criterion: a test case turns an acceptance criterion into something checkable, so a later change that breaks it is caught rather than discovered."
	practiceCriteria  = "Write acceptance criteria: a criterion is an observable statement of done. It is what design addresses and a test verifies, so without one neither can be traced."
	practicePrototype = "Annotate the prototype: pinning part of a prototype to the story it clarifies keeps the experience and the requirement saying the same thing as both evolve."
	practiceTerms     = "Define the term: the words a team says every day are the least documented and fastest to rot. A term references the code that defines it, so renaming that code flags the term."
)

// Context is what growth suggestions read beyond the living status: the
// coverage report and where each documentation target sits.
type Context struct {
	Coverage areas.Report
	// Places maps a documentation target to the slide (or other place) a
	// story link attaches to, with its title and epic.
	Places map[string]Place
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
	b.storyGrowth()
	b.personaGrowth()
	b.designGrowth()
	b.qualityGrowth()
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
	for _, persona := range b.status.Personas {
		if persona.State == "active" {
			active[persona.Persona] = true
		}
	}
	unnamed := []string{}
	for _, entry := range append(append([]areas.Entry{}, b.context.Coverage.Areas.Design.CoveredEntries...), b.context.Coverage.Areas.Design.UncoveredEntries...) {
		named := false
		for _, persona := range b.stories[entry.Resource].Personas {
			named = named || active[persona]
		}
		if !named {
			unnamed = append(unnamed, entry.Resource)
		}
	}
	sort.Strings(unnamed)
	if len(unnamed) > 0 {
		titles := []string{}
		revise := []grammar.Invocation{b.invoke("persona add", grammar.V("id", ""), grammar.V("name", ""), grammar.V("description", ""))}
		for _, story := range unnamed {
			row := b.stories[story]
			titles = append(titles, "\""+firstNonEmptyString(row.Title, story)+"\"")
			values := []grammar.Value{grammar.V("story", story), grammar.V("persona", "")}
			values = append(values, parents(row.RevisionHeads)...)
			revise = append(revise, b.invoke("story revise", values...).With("epic", row.Epic))
		}
		b.add(Action{
			ID: "growth:persona:unnamed", Kind: KindQuestion, Category: CategoryGrowth, Area: AreaPersonas,
			Reason:   countWords(len(unnamed), "story serves", "stories serve") + " someone you haven't named (" + strings.Join(titles, ", ") + "); define that persona?",
			Practice: practicePersonas,
			Question: question("Who do "+strings.Join(titles, ", ")+" serve?", NeedProductJudgment,
				option("name them", "a persona, and a revision of each story that names it", revise...),
				option("not now", "nothing is recorded; the personas area keeps reporting the gap")),
		})
		return
	}
	if len(b.status.Personas) == 0 && !b.context.Coverage.Areas.Personas.Complete {
		b.add(Action{
			ID: "growth:persona:first", Kind: KindCommand, Category: CategoryGrowth, Area: AreaPersonas,
			Reason:   "no persona is named yet, so no change can say whom it serves; who does this change serve?",
			Practice: practicePersonas,
			Command:  ptr(b.invoke("persona add", grammar.V("id", ""), grammar.V("name", ""), grammar.V("description", ""))),
		})
	}
}

func (b *builder) designGrowth() {
	for _, entry := range b.context.Coverage.Areas.Design.UncoveredEntries {
		title := firstNonEmptyString(entry.Title, entry.Resource)
		relate := []grammar.Value{grammar.V("id", ""), grammar.V("type", "addresses"), grammar.V("from", ""), grammar.V("to", entry.Resource), grammar.V("rationale", "")}
		b.add(Action{
			ID: "growth:design:" + entry.Resource, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaDesign, Resource: entry.Resource, Epic: entry.Epic,
			Reason: "\"" + title + "\" has no design; add the design that explains how it is met?", Practice: practiceDesign,
			value: b.valueOf(entry.Resource),
			Question: question("What design explains how \""+title+"\" is met?", NeedProductJudgment,
				option("an existing design, deck, slide, or Item explains it", "an addresses relation from it", b.invoke("relation add", relate...)),
				option("write it", "write a design chapter (change-saga design add-chapter --epic ID NAME), then relate it with an addresses relation", b.invoke("relation add", relate...)),
				option("not now", "nothing is recorded; the design area keeps reporting the gap")),
		})
	}
}

func (b *builder) qualityGrowth() {
	for _, entry := range b.context.Coverage.Areas.Quality.UncoveredEntries {
		relate := []grammar.Value{grammar.V("id", ""), grammar.V("type", "verifies"), grammar.V("from", ""), grammar.V("to", entry.Resource), grammar.V("rationale", "")}
		subject := b.describe(entry.Resource)
		if entry.Title != "" {
			subject = "\"" + entry.Title + "\""
		}
		b.add(Action{
			ID: "growth:quality:" + entry.Resource, Kind: KindQuestion, Category: CategoryGrowth, Area: AreaQuality, Resource: entry.Resource, Epic: entry.Epic,
			Reason: "no test case verifies " + subject + "; add one?", Practice: practiceQuality,
			value:    b.valueOf(entry.Resource),
			Question: question("Which test case verifies "+subject+"?", NeedProductJudgment, b.testOptions(relate)...),
		})
	}
}

// testOptions offers an existing test case first when the app has any, and a
// new one first when it has none yet.
func (b *builder) testOptions(relate []grammar.Value) []Option {
	existing := option("an existing test case verifies it", "a verifies relation from the test case", b.invoke("relation add", relate...))
	fresh := option("a new test case is needed", "define its ordered steps and kinds, then relate it", b.invoke("quality test-case add"), b.invoke("relation add", relate...))
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
