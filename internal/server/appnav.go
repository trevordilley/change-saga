package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer's app-level list has three sections, and every one of them is
// a page:
//
//	Overview        name, elevator pitch, description, and the parts that
//	                describe the whole app: terms and vocabulary, personas,
//	                the design system, onboarding, and feature flags
//	Epics           the directory of every epic, over the one epic the
//	                sidebar is currently showing and its four places
//	Reviews         every pull request's review
//
// Three sections, because those are the three things a reviewer arrives
// looking for: what the app is, what it does, and what is being changed about
// it. Personas, the design system, onboarding, and feature flags all describe
// the whole app rather than any one part of it, so they belong to the
// overview and not beside the epics they cut across.
//
// No header in this list is a row that only expands. A section header opens
// the section: Overview opens its prose and its directory, Terms and
// vocabulary opens the table of terms, Onboarding opens the deck at its first
// slide, and Epics and Reviews open their tables. The disclosure beside a
// header is how a reader reaches one row inside the section without leaving
// where they are; it is not the only way in.
//
// The epics are not all listed. Listing every one of them expanded put this
// repository's own sidebar at 238 rows, which is a wall rather than an
// architecture. One epic at a time keeps the list readable, the picker keeps
// every other epic one keystroke away, and the Epics header opens the table
// of all of them.
//
// Within the epic the per-epic rules are unchanged: Implementation is the deck
// and opens all the way to its slides, the other places stay shut until
// something inside them is active, and an empty place states its gap.

// appNavSources is everything the app-level list reads, already loaded.
type appNavSources struct {
	document      *saga.Saga
	requirements  requirements.Document
	page          *requirementsPageView
	prototypes    prototypes.Document
	prototypeNote string
	// quality holds the test cases each epic's Quality lists.
	quality quality.Document
	// decks is every projected deck row, implementation and onboarding.
	decks []*navNodeView
	// overviewActive says which overview row the page shows; see overviewNav.
	overviewActive string
	// currentEpic is the one epic the sidebar shows, already resolved by
	// resolveCurrentEpic. An unknown or empty ID falls back to the first epic.
	currentEpic string
	// hasReviews says whether any pull request has a review yet, so the
	// Reviews section can state the gap without loading one.
	hasReviews bool
}

func makeAppNavTree(sources appNavSources) []*navNodeView {
	document := sources.document
	overview := overviewNav(document, sources.requirements, sources.overviewActive)

	deckRows := map[string]*navNodeView{}
	for _, row := range sources.decks {
		deckRows[row.NodeID] = row
	}
	var onboarding []*navNodeView
	for _, deck := range document.Onboarding {
		if row := deckRows["nav-"+domID(deck.Target)]; row != nil {
			onboarding = append(onboarding, row)
		}
	}
	// The onboarding deck is one deck; its slides sit directly beneath the
	// place, as Implementation's do.
	if len(onboarding) == 1 && len(onboarding[0].Children) > 0 {
		onboarding = onboarding[0].Children
	}

	// Everything that describes the whole app hangs off the overview.
	overview.Children = append(overview.Children,
		navSection("Personas", "/personas", "nav-personas", "", "no personas yet", personaNav(sources.requirements)),
		navSection("Design system", designSystemPath, "nav-designsystem", "design", "no design system yet",
			onPage(designSystemPath, reportRootNav(document.DesignSystem))),
		navDeck("Onboarding", "nav-onboarding", "deck", "no onboarding deck yet", onboarding),
		navSection("Feature flags", "/flags", "nav-featureflags", "", "no feature flags yet", flagNav(sources.requirements)),
	)

	reviews := navSection("Reviews", "/reviews", "nav-reviews", "diff", "no reviews yet", nil)
	if sources.hasReviews {
		// Every review is a page of its own, so the section holds no rows: its
		// header opens the table of them. The gap is stated only while there
		// are none to open.
		reviews.Gap, reviews.Note = false, ""
	}
	navigation := []*navNodeView{overview, makeEpicsNav(sources, deckRows), reviews}
	for _, node := range navigation {
		revealActive(node)
	}
	return navigation
}

// navSection is a section header that is also a destination. The row opens
// the section's own page and the twisty beside it discloses what the section
// holds; a header that only expanded made a reader click twice to reach a
// page that already existed. A section nothing fills keeps both the link and
// the stated gap, because the page is where that gap is explained.
func navSection(title, href, id, icon, emptyNote string, children []*navNodeView) *navNodeView {
	node := navPlace(title, id, icon, emptyNote, children)
	node.Href = href
	return node
}

// navDeck is a section whose page is a deck: the header opens it at its first
// slide, and the slides stay beneath it so any one of them is still one click
// away. A deck already knows where it starts, so a header that only expanded
// into a list of slides asked the reader a question they had no way to answer.
func navDeck(title, id, icon, emptyNote string, slides []*navNodeView) *navNodeView {
	node := navPlace(title, id, icon, emptyNote, slides)
	node.Href = firstSlideHref(slides)
	return node
}

// firstSlideHref is where a deck opens: its first slide, wherever that slide
// sits among the rows the deck was given.
func firstSlideHref(nodes []*navNodeView) string {
	for _, node := range nodes {
		if node.Slide != nil {
			return node.Href
		}
		if href := firstSlideHref(node.Children); href != "" {
			return href
		}
	}
	return ""
}

// onPage moves a row's in-page anchors, and its outline's, onto the page that
// renders them.
func onPage(path string, nodes []*navNodeView) []*navNodeView {
	for _, node := range nodes {
		if strings.HasPrefix(node.Href, "#") {
			node.Href = path + node.Href
		}
		onPage(path, node.Children)
	}
	return nodes
}

// makeEpicsNav is the Epics section: the header opens the table of every
// epic, the one epic the sidebar is showing sits beneath it over its four
// places, and the full list is one disclosure away. An app with no epics keeps
// the section, because a reader has to be able to see that the app has no
// epics rather than infer it from an absence.
func makeEpicsNav(sources appNavSources, deckRows map[string]*navNodeView) *navNodeView {
	document := sources.document
	section := navSection("Epics", epicsIndexHref, "nav-epics", "product", "no epics yet", nil)
	if len(document.Epics) == 0 {
		return section
	}
	current := document.Epics[0]
	for _, epic := range document.Epics {
		if epic.ID == sources.currentEpic {
			current = epic
		}
	}
	node := makeEpicNav(sources, current, deckRows)
	node.Picker = makeEpicPicker(document, current.ID)
	section.Gap, section.Note = false, ""
	section.Children = []*navNodeView{node, makeAllEpicsNav(document, current.ID)}
	section.Expanded = true
	return section
}

// makeAllEpicsNav is the browsing list: every epic as one row, shut until a
// reader opens it. It is a disclosure rather than a tree, so it costs the
// sidebar one row until it is asked for and never repeats an epic's places.
func makeAllEpicsNav(document *saga.Saga, current string) *navNodeView {
	return &navNodeView{
		Title: "Show all epics", NodeID: "nav-all-epics",
		Epics: epicChoices(document, current), IndexHref: epicsIndexHref,
	}
}

// makeEpicNav is one epic: its own report content first, then today's four
// places filled from this epic's records only.
func makeEpicNav(sources appNavSources, epic *saga.Epic, deckRows map[string]*navNodeView) *navNodeView {
	prefix := "nav-epic-" + domID(epic.ID)
	var prototypeRows []*navNodeView
	for _, row := range makePrototypeNav(sources.prototypes) {
		for _, prototype := range sources.prototypes.Prototypes {
			if prototype.Epic == epic.ID {
				if target, err := prototypes.PrototypeURN(sources.prototypes.SagaID, prototype.Identity.ID); err == nil && row.NodeID == "nav-"+domID(target) {
					prototypeRows = append(prototypeRows, row)
				}
			}
		}
	}
	var uxDecks, implementation []*navNodeView
	for _, deck := range epic.Decks {
		row := deckRows["nav-"+domID(deck.Target)]
		if row == nil {
			continue
		}
		if deck.Role == "ux" {
			uxDecks = append(uxDecks, row)
		} else {
			implementation = append(implementation, row)
		}
	}
	var technical []*navNodeView
	if epic.Design != nil {
		for _, child := range epic.Design.Children {
			if child.Kind == "chapter" {
				technical = append(technical, onEpicPage(makeChapterNav(child), epic.ID))
			}
		}
	}
	places := makeProductNavTree(productNavSources{
		prefix:         prefix,
		epic:           epic.ID,
		requirements:   makeEpicRequirementsNav(sources.page, epic.ID, prefix),
		prototypes:     prototypeRows,
		prototypeNote:  sources.prototypeNote,
		uxDecks:        uxDecks,
		technical:      technical,
		testCases:      testCaseNav(sources.quality, epic.ID),
		implementation: implementation,
	})
	// The epic row opens the epic's page; its places disclose beneath it.
	node := &navNodeView{Title: epic.Title, Href: epicHref(epic.ID), NodeID: prefix, Icon: "product", Group: true, Expanded: true}
	var report []*navNodeView
	for _, row := range reportRootNav(epic.Report) {
		report = append(report, onEpicPage(row, epic.ID))
	}
	node.Children = append(report, places...)
	return node
}

// onEpicPage points a row's in-page anchors, and its outline's, at the epic's
// page, where the epic's own chapters are rendered.
func onEpicPage(node *navNodeView, epic string) *navNodeView {
	if strings.HasPrefix(node.Href, "#") {
		node.Href = epicHref(epic) + node.Href
	}
	for _, child := range node.Children {
		onEpicPage(child, epic)
	}
	return node
}

// reportRootNav outlines one report root: its fragments, then its chapters.
func reportRootNav(root *saga.Section) []*navNodeView {
	if root == nil {
		return nil
	}
	nodes := fragmentOutline(root)
	for _, child := range root.Children {
		if child.Kind == "chapter" && !designSection(child) {
			nodes = append(nodes, makeChapterNav(child))
		}
	}
	return nodes
}

// personaNav names each persona, links its page, and says when no accepted
// story serves it yet.
func personaNav(document requirements.Document) []*navNodeView {
	served := map[string]bool{}
	for _, story := range document.Stories {
		if story.CurrentRevision == nil || story.CurrentLifecycle == nil || story.CurrentLifecycle.State != requirements.StateAccepted {
			continue
		}
		for _, persona := range story.CurrentRevision.Personas {
			served[persona] = true
		}
	}
	var nodes []*navNodeView
	for _, persona := range document.Personas {
		urn, _ := requirements.PersonaURN(document.SagaID, persona.Identity.ID)
		title := persona.Identity.ID
		if persona.CurrentRevision != nil && strings.TrimSpace(persona.CurrentRevision.Name) != "" {
			title = persona.CurrentRevision.Name
		}
		node := &navNodeView{Title: title, Href: personaHref(persona.Identity.ID), NodeID: "nav-" + domID(urn), Icon: "story"}
		switch {
		case !persona.Active():
			node.Note = "retired"
		case !served[urn]:
			node.Gap, node.Note = true, "no accepted story serves it"
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// flagNav names each flag with its state.
func flagNav(document requirements.Document) []*navNodeView {
	var nodes []*navNodeView
	for _, flag := range document.Flags {
		urn, _ := requirements.FlagURN(document.SagaID, flag.Identity.ID)
		state := "conflicted"
		if flag.CurrentLifecycle != nil {
			state = string(flag.CurrentLifecycle.State)
		}
		nodes = append(nodes, &navNodeView{Title: flag.Identity.ID, NodeID: "nav-" + domID(urn), Note: state})
	}
	return nodes
}
