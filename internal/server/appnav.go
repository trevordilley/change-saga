package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer's app-level list sits above the epics:
//
//	Overview        the app's elevator pitch
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
	threads       map[string][]*threadView
	// decks is every projected deck row, implementation and onboarding.
	decks []*navNodeView
}

func makeAppNavTree(sources appNavSources) []*navNodeView {
	document := sources.document
	overview := &navNodeView{Title: "Overview", Href: sagaHref(document.Section.Target), NodeID: "nav-overview", Active: true}
	overview.Children = reportRootNav(document.Overview, sources.threads)
	overview.Expanded = len(overview.Children) > 0
	if len(overview.Children) == 0 {
		overview.Gap, overview.Note = true, "no overview yet"
	}

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
		navPlace("Design system", "nav-designsystem", "design", "no design system yet", reportRootNav(document.DesignSystem, sources.threads)),
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
				technical = append(technical, makeChapterNav(child, sources.threads))
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
		implementation: implementation,
	})
	node := &navNodeView{Title: epic.Title, NodeID: prefix, Icon: "product", Group: true, Expanded: true}
	node.Children = append(reportRootNav(epic.Report, sources.threads), places...)
	return node
}

// reportRootNav outlines one report root: its fragments, then its chapters.
func reportRootNav(root *saga.Section, threads map[string][]*threadView) []*navNodeView {
	if root == nil {
		return nil
	}
	nodes := fragmentOutline(root)
	for _, child := range root.Children {
		if child.Kind == "chapter" && !designSection(child) {
			nodes = append(nodes, makeChapterNav(child, threads))
		}
	}
	return nodes
}

// personaNav names each persona and whether an accepted story serves it. There
// is no persona page yet, so rows are names rather than links.
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
		node := &navNodeView{Title: title, NodeID: "nav-" + domID(urn), Icon: "story"}
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
