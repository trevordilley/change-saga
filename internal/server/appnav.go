package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer's app-level list sits above the epics:
//
//	Overview        name, elevator pitch, description, terms and vocabulary
//	Personas        who the app serves
//	Design system   Figma links and references
//	Onboarding      the deck that gets people up to speed
//	Feature flags   what is gated, and whether it is on
//	Epics           each epic expands to Product, Design, Quality, Implementation
//
// It is deliberately the minimum that keeps every place reachable and every
// gap stated. Within an epic the per-epic rules are unchanged: Implementation
// is the deck and opens all the way to its slides, and the other places stay
// shut until something inside them is active.
// TODO(app-view): the real app-level view is designed in Phase 4.

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

	epics := make([]*navNodeView, 0, len(document.Epics))
	for _, epic := range document.Epics {
		epics = append(epics, makeEpicNav(sources, epic, deckRows))
	}
	epicsPlace := navPlace("Epics", "nav-epics", "", "no epics yet", epics)
	epicsPlace.Expanded = len(epics) > 0

	navigation := []*navNodeView{
		overview,
		navPlace("Personas", "nav-personas", "", "no personas yet", personaNav(sources.requirements)),
		navPlace("Design system", "nav-designsystem", "design", "no design system yet", reportRootNav(document.DesignSystem)),
		navPlace("Onboarding", "nav-onboarding", "deck", "no onboarding deck yet", onboarding),
		navPlace("Feature flags", "nav-featureflags", "", "no feature flags yet", flagNav(sources.requirements)),
		epicsPlace,
	}
	for _, node := range navigation {
		revealActive(node)
	}
	return navigation
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
