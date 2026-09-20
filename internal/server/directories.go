package server

import (
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The directories themselves: one per section a reader can open. Each one is
// the whole of what its section holds, so a reader never has to open records
// one at a time to see the shape of them.

// summarise shortens a definition or a description to the one line a table
// row has room for. The whole of it is on the record's own page, which the
// row's first cell links.
func summarise(text string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= limit {
		return text
	}
	cut := text[:limit]
	if space := strings.LastIndex(cut, " "); space > limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

// designSystemPath is the design system's own page. The design system used to
// render as anchors inside the overview, which left its header a row that
// only expanded; it is a section like any other and opens like one.
const designSystemPath = "/design-system"

// onboardingHref opens the onboarding deck at its first slide. A header that
// only expands into a list of slides asks a reader to choose where a deck
// starts, which is the one thing a deck already knows.
func onboardingHref(document *saga.Saga) string {
	for _, deck := range document.Onboarding {
		for _, slide := range deck.Slides {
			return "/?view=slides#" + domID(slide.Target)
		}
	}
	return ""
}

// ----- Personas -----

// personasDirectory is every persona the app names, whether it is still
// served, and how many accepted stories serve it. An unserved persona is the
// gap the persona coverage rule exists to show, so it is stated rather than
// scored.
func personasDirectory(document requirements.Document, query string) *directoryView {
	view := &directoryView{
		ID: "personas", Title: "Personas", Action: "/personas",
		Lede:  "Who the app is for. A persona is someone who gets value from the app, the \"As a ...\" of a user story, never a tool or agent that operates it. A persona is served when an accepted story names it.",
		Label: "Filter personas", Noun: "persona", Nouns: "personas",
		Columns: []directoryColumn{
			{Title: "Persona"}, {Title: "Description", Wide: true}, {Title: "State"},
			{Title: "Stories served", Numeric: true},
		},
		Empty:   "No personas yet.",
		Command: "change-saga persona add",
	}
	served := map[string]int{}
	for _, story := range document.Stories {
		if story.CurrentRevision == nil || story.CurrentLifecycle == nil || story.CurrentLifecycle.State != requirements.StateAccepted {
			continue
		}
		for _, persona := range story.CurrentRevision.Personas {
			served[persona]++
		}
	}
	for _, persona := range document.Personas {
		urn, _ := requirements.PersonaURN(document.SagaID, persona.Identity.ID)
		name, description := persona.Identity.ID, ""
		if persona.CurrentRevision != nil {
			if strings.TrimSpace(persona.CurrentRevision.Name) != "" {
				name = persona.CurrentRevision.Name
			}
			description = persona.CurrentRevision.Description
		}
		state := "retired"
		if persona.Active() {
			state = "active"
		}
		count := countCell(served[urn])
		if served[urn] == 0 {
			count = gapCell("none accepted yet")
		}
		view.addRow(directoryRow{Key: persona.Identity.ID, Cells: []directoryCell{
			{Text: name, Href: personaHref(persona.Identity.ID), Note: persona.Identity.ID, Target: urn},
			textCell(summarise(description, 120)),
			textCell(state),
			count,
		}})
	}
	view.apply(query)
	return view
}

// ----- Feature flags -----

// flagsDirectory is every flag, whether it is on, and what it gates. A flag
// gating nothing yet says so; a target the reviewer has a page for is linked
// from the record it gates.
func flagsDirectory(graph *appGraph, query string) *directoryView {
	document := graph.requirements
	view := &directoryView{
		ID: "flags", Title: "Feature flags", Action: "/flags",
		Lede:  "What is gated, whether it is on, and the stories and features each flag gates.",
		Label: "Filter feature flags", Noun: "feature flag", Nouns: "feature flags",
		Columns: []directoryColumn{
			{Title: "Flag"}, {Title: "State"}, {Title: "Gates"}, {Title: "What it is for", Wide: true},
		},
		Empty:   "No feature flags yet.",
		Command: "change-saga flag add",
	}
	for _, flag := range document.Flags {
		urn, _ := requirements.FlagURN(document.SagaID, flag.Identity.ID)
		state := "conflicted"
		if flag.CurrentLifecycle != nil {
			state = string(flag.CurrentLifecycle.State)
		}
		description := ""
		var gates []string
		if flag.CurrentRevision != nil {
			description = flag.CurrentRevision.Description
			for _, target := range flag.CurrentRevision.Targets {
				gates = append(gates, graph.link(target).Title)
			}
		}
		view.addRow(directoryRow{Key: flag.Identity.ID, Cells: []directoryCell{
			{Text: flag.Identity.ID, Target: urn},
			textCell(state),
			listCell(gates, "gates nothing yet"),
			textCell(summarise(description, 120)),
		}})
	}
	view.apply(query)
	return view
}

// ----- Terms -----

// termsDirectory is the whole vocabulary at once: every term, its aliases,
// what it means here, and where in the code it is defined. A reader asking
// what a word means in this project should not have to open thirty pages to
// find out, which is what the list of links used to ask of them.
func termsDirectory(document requirements.Document, places map[string][]termPlace, query string) *directoryView {
	view := &directoryView{
		ID: "terms", Title: "Terms and vocabulary", Action: "/terms",
		Lede:  "The words this project uses in its own way, what each one means here, and the code that defines it.",
		Label: "Filter terms", Noun: "term", Nouns: "terms",
		Columns: []directoryColumn{
			{Title: "Term"}, {Title: "Also"}, {Title: "Definition", Wide: true},
			{Title: "Defined in code"}, {Title: "Reference"},
		},
		Empty:   "No terms yet.",
		Command: "change-saga term add",
	}
	for _, term := range document.Terms {
		item := makeTermView(document, term, nil)
		name := directoryCell{Text: item.Name, Href: item.Href, Target: item.Target}
		if item.Retired {
			name.Note = "retired"
		}
		var locations []string
		stale := false
		for _, place := range places[item.ID] {
			locations = append(locations, place.Where)
			stale = stale || place.Stale
		}
		reference := gapCell("no code yet")
		switch {
		case len(locations) == 0:
		case stale:
			reference = gapCell("stale: the code changed since")
		default:
			reference = textCell("current")
		}
		view.addRow(directoryRow{Key: item.ID, Cells: []directoryCell{
			name,
			listCell(item.Aliases, "no aliases"),
			textCell(summarise(item.Definition, 160)),
			listCell(locations, "not linked to code yet"),
			reference,
		}})
	}
	view.apply(query)
	return view
}

// ----- Features -----

// featuresDirectory is every feature with what it holds. The counts are facts and
// never a score: a feature with nothing in it is listed like any other, and the
// feature the reader is already inside is marked rather than ranked.
func featuresDirectory(document *saga.Saga, graph *appGraph, current, query string) *directoryView {
	view := &directoryView{
		ID: "features", Title: "Features", Action: "/features",
		Lede:  "Every durable area of the product, in the order they were introduced. Each one opens a page holding its stories, its design, its quality, and its implementation.",
		Label: "Filter features", Noun: "feature", Nouns: "features",
		Columns: []directoryColumn{
			{Title: "Feature"}, {Title: "Description", Wide: true},
			{Title: "Stories", Numeric: true}, {Title: "Accepted", Numeric: true},
			{Title: "With design", Numeric: true}, {Title: "Test cases", Numeric: true},
			{Title: "Slides", Numeric: true},
		},
		Empty:   "No features yet.",
		Command: "change-saga feature add",
	}
	for _, row := range featureRows(document, graph, current) {
		view.addRow(directoryRow{Key: row.ID, Current: row.Current, Cells: []directoryCell{
			{Text: row.Title, Href: row.Href, Note: row.ID},
			textCell(summarise(row.Description, 120)),
			countCell(row.Stories), countCell(row.Accepted), countCell(row.WithDesign),
			countCell(row.TestCases), countCell(row.Slides),
		}})
	}
	view.apply(query)
	return view
}

// featureIndexRow is one feature's counts, as the directory states them.
type featureIndexRow struct {
	ID          string
	Title       string
	Href        string
	Description string
	Current     bool
	Stories     int
	Accepted    int
	WithDesign  int
	TestCases   int
	Slides      int
}

func featureRows(document *saga.Saga, graph *appGraph, current string) []featureIndexRow {
	rows := make([]featureIndexRow, 0, len(document.Features))
	descriptions := map[string]string{}
	for _, manifest := range graph.requirements.Features {
		descriptions[manifest.ID] = manifest.Description
	}
	for _, choice := range featureLinks(document, current) {
		row := featureIndexRow{
			ID: choice.ID, Title: choice.Title, Href: choice.Href,
			Description: descriptions[choice.ID], Current: choice.Current,
		}
		for _, story := range graph.requirements.Stories {
			if story.Feature != choice.ID {
				continue
			}
			row.Stories++
			if story.CurrentLifecycle != nil && story.CurrentLifecycle.State == requirements.StateAccepted {
				row.Accepted++
			}
			if storyHasDesign(graph, story) {
				row.WithDesign++
			}
		}
		for _, testCase := range graph.quality.TestCases {
			if testCase.Feature == choice.ID {
				row.TestCases++
			}
		}
		for _, feature := range document.Features {
			if feature.ID != choice.ID {
				continue
			}
			for _, deck := range feature.Decks {
				row.Slides += len(deck.Slides)
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// storyHasDesign reports whether anything addresses the story or one of its
// criteria, which is the same link the feature page counts.
func storyHasDesign(graph *appGraph, story requirements.Story) bool {
	urn, err := livingid.Story(graph.requirements.SagaID, story.Identity.ID)
	if err != nil {
		return false
	}
	if len(graph.traceTo(urn).Design) > 0 {
		return true
	}
	if story.CurrentRevision == nil {
		return false
	}
	for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
		criterionURN, err := livingid.Criterion(graph.requirements.SagaID, story.Identity.ID, criterion.ID)
		if err != nil {
			continue
		}
		if len(graph.traceTo(criterionURN).Design) > 0 {
			return true
		}
	}
	return false
}

// ----- Reviews -----

// reviewsDirectory is every pull request's review: what it compares, how much
// of it has been decided, and how much of that has gone out of date. It
// counts decisions; it never adds them up into a verdict, because whether a
// review is done is the team's rule and not this tool's.
func reviewsDirectory(reviews []reviewSummaryView, query string) *directoryView {
	view := &directoryView{
		ID: "reviews", Title: "Reviews", Action: "/reviews",
		Label: "Filter reviews", Noun: "review", Nouns: "reviews",
		Columns: []directoryColumn{
			{Title: "Review", Wide: true}, {Title: "Pull request"}, {Title: "Range"},
			{Title: "Slides", Numeric: true}, {Title: "Decisions", Numeric: true},
			{Title: "Out of date", Numeric: true}, {Title: "State"},
		},
		Empty:   "No reviews yet.",
		Command: "change-saga review create",
	}
	for _, review := range reviews {
		report := review.Report
		pull := gapCell("not linked")
		if report.PullRequest != nil {
			label := report.PullRequest.URL
			if report.PullRequest.Number != 0 {
				label = "#" + strconv.Itoa(report.PullRequest.Number)
			}
			pull = directoryCell{Text: label, Href: report.PullRequest.URL}
		}
		rng := gapCell("range unavailable")
		if report.Range != nil {
			rng = textCell(shortCommit(report.Range.BaseOID) + ".." + shortCommit(report.Range.HeadOID))
		}
		decisions, outOfDate := 0, 0
		for _, slide := range report.Slides {
			for _, decision := range slide.Decisions {
				decisions++
				if decision.Currency == reviewstate.OutOfDate {
					outOfDate++
				}
			}
		}
		state := "open"
		if report.Merged != nil {
			state = "merged"
		}
		title := directoryCell{Text: report.Title, Href: review.Href, Note: report.ID, Target: report.Target}
		view.addRow(directoryRow{Key: report.ID, Current: review.Matches, Cells: []directoryCell{
			title, pull, rng,
			countCell(len(report.Slides)), countCell(decisions), countCell(outOfDate),
			textCell(state),
		}})
	}
	view.apply(query)
	return view
}

// ----- The overview's own directory -----

// overviewPartView is one part of the overview: where it opens and how much
// it holds. It is a directory in prose rather than a table, because the
// overview has five parts and a reader is reading a page, not scanning a list.
type overviewPartView struct {
	Title string
	Href  string
	Count string
	Note  string
	Gap   bool
}

// overviewDirectory names the overview's parts with what each one holds. An
// empty part keeps its row and states the command that fills it.
func overviewDirectory(document *saga.Saga, records requirements.Document, onboarding string) []overviewPartView {
	parts := []overviewPartView{
		{Title: "Personas", Href: "/personas", Count: plural(len(records.Personas), "persona", "personas"),
			Note: "Who the app is for.", Gap: len(records.Personas) == 0},
		{Title: "Terms and vocabulary", Href: "/terms", Count: plural(len(records.Terms), "term", "terms"),
			Note: "The words this project uses in its own way.", Gap: len(records.Terms) == 0},
		{Title: "Design system", Href: designSystemPath, Count: plural(designSystemParts(document), "part", "parts"),
			Note: "The references and chapters the interface is built from.", Gap: designSystemParts(document) == 0},
		{Title: "Onboarding", Href: onboarding, Count: plural(onboardingSlides(document), "slide", "slides"),
			Note: "The deck that gets someone new up to speed.", Gap: onboardingSlides(document) == 0},
		{Title: "Feature flags", Href: "/flags", Count: plural(len(records.Flags), "feature flag", "feature flags"),
			Note: "What is gated, and whether it is on.", Gap: len(records.Flags) == 0},
	}
	for index := range parts {
		if !parts[index].Gap {
			continue
		}
		parts[index].Note = overviewGrowth[parts[index].Title]
	}
	return parts
}

// overviewGrowth states each empty part as growth and names the command that
// acts on it, the way the sidebar's gap rows do.
var overviewGrowth = map[string]string{
	"Personas":             "No personas yet. Run change-saga persona add to name who the app is for.",
	"Terms and vocabulary": "No terms yet. Run change-saga term add to record a word this project uses in its own way.",
	"Design system":        "No design system yet. Run change-saga design to record what the interface is built from.",
	"Onboarding":           "No onboarding deck yet. Run change-saga add-deck --role onboarding to start one.",
	"Feature flags":        "No feature flags yet. Run change-saga flag add to record what is gated.",
}

func designSystemParts(document *saga.Saga) int {
	if document.DesignSystem == nil {
		return 0
	}
	return len(document.DesignSystem.Fragments) + len(document.DesignSystem.Children)
}

func onboardingSlides(document *saga.Saga) int {
	count := 0
	for _, deck := range document.Onboarding {
		count += len(deck.Slides)
	}
	return count
}

func plural(count int, one, many string) string {
	word := many
	if count == 1 {
		word = one
	}
	return strconv.Itoa(count) + " " + word
}
