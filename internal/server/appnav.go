package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer's app-level list:
//
//	Overview        name, elevator pitch, description, terms and vocabulary
//	Personas        who the app serves
//	Design system   Figma links and references
//	Onboarding      the deck that gets people up to speed
//	Feature flags   what is gated, and whether it is on
//	<current epic>  one epic, chosen from a searchable picker, over its
//	                Product, Design, Quality, and Implementation
//	Show all epics  the full list, for browsing, collapsed by default
//
// Everything above the epic is always present, because it describes the whole
// app and a reader arriving anywhere needs it. The epics are not: listing
// every one of them expanded put this repository's own sidebar at 238 rows,
// which is a wall rather than an architecture. One epic at a time keeps the
// list readable, and the picker keeps every other epic one keystroke away.
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

	navigation := []*navNodeView{
		overview,
		navPlace("Personas", "nav-personas", "", "no personas yet", personaNav(sources.requirements)),
		navPlace("Design system", "nav-designsystem", "design", "no design system yet", reportRootNav(document.DesignSystem)),
		navPlace("Onboarding", "nav-onboarding", "deck", "no onboarding deck yet", onboarding),
		navPlace("Feature flags", "nav-featureflags", "", "no feature flags yet", flagNav(sources.requirements)),
	}
	navigation = append(navigation, makeEpicNavRegion(sources, deckRows)...)
	for _, node := range navigation {
		revealActive(node)
	}
	return navigation
}

// makeEpicNavRegion is the epic end of the sidebar: the current epic over its
// four places, then the disclosure that lists every epic. An app with no epics
// keeps the row that says so, because a reader has to be able to see that the
// app has no epics rather than infer it from an absence.
func makeEpicNavRegion(sources appNavSources, deckRows map[string]*navNodeView) []*navNodeView {
	document := sources.document
	if len(document.Epics) == 0 {
		return []*navNodeView{navPlace("Epics", "nav-epics", "", "no epics yet", nil)}
	}
	current := document.Epics[0]
	for _, epic := range document.Epics {
		if epic.ID == sources.currentEpic {
			current = epic
		}
	}
	node := makeEpicNav(sources, current, deckRows)
	node.Picker = makeEpicPicker(document, current.ID)
	return []*navNodeView{node, makeAllEpicsNav(document, current.ID)}
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
