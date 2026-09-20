package server

import (
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer sidebar projects one stable information architecture:
//
//	Product          Prototypes, then Requirements
//	Design           UX, UI, Technical (ERD, System, Data Flows)
//	Quality          Test Cases
//	Implementation   the slide deck that explains the change
//
// The order is fixed. It is an information architecture, not a phase gate: it
// never reorders as authoring progresses, so it never implies waterfall.
// Within Product, Prototypes precedes Requirements because prototype-first is
// the common discovery path, not because prototypes come due first.
//
// A place nothing has been authored into keeps its row and says so. Hiding an
// empty section would leave a reviewer unable to see what is missing, which is
// the one question this architecture exists to answer.
//
// The four headers are destinations as well as disclosures. Product, Design,
// and Quality open the feature's page at the section that lists what they hold,
// and Implementation opens its deck at the first slide.

// productNavSources is everything the architecture can be filled from. Every
// field is optional: an empty field becomes a visible gap, never a hidden
// section. Fields with no producer yet are the seams the remaining domains
// will be joined through, and their TODOs name what is still missing.
type productNavSources struct {
	// prefix namespaces the architecture's node IDs, since every feature has
	// its own four places. An empty prefix is "nav".
	prefix string
	// feature is the feature these four places belong to, so each place's header
	// can open that feature's page at the section it names. An empty feature leaves
	// the headers as disclosures, which is what they were before a feature had
	// a page of its own.
	feature string
	// requirements is makeRequirementsNav's tree. Requirements is its own
	// overview and never gains a redundant "Overview" child.
	requirements *navNodeView
	prototypes   []*navNodeView
	// prototypeNote replaces the default empty note when the prototype
	// packages exist but could not be read.
	prototypeNote string
	uxDecks       []*navNodeView
	// uiDesign is UI references and embeds.
	// TODO: no UI design resource is recorded yet; nothing fills this.
	uiDesign []*navNodeView
	// technical is authored technical-design chapters stored under ___design.
	technical []*navNodeView
	// dataFlows is individual flow diagrams.
	// TODO: no diagram resource is recorded yet; nothing fills this.
	dataFlows []*navNodeView
	// testCases is the feature's test cases, each opening its page.
	testCases      []*navNodeView
	implementation []*navNodeView
}

// makeProductNavTree projects the stable order. It reads only what has already
// been loaded plus the prototype packages, so building the whole architecture
// opens no narrative content.
func makeProductNavTree(sources productNavSources) []*navNodeView {
	prefix := sources.prefix
	if prefix == "" {
		prefix = "nav"
	}
	requirements := sources.requirements
	if requirements == nil {
		requirements = &navNodeView{
			Title: "Requirements", Href: "/requirements", NodeID: prefix + "-requirements",
			Icon: "requirements", Requirement: true,
		}
	}
	if len(requirements.Children) == 0 {
		requirements.Gap, requirements.Note = true, "no stories yet"
	}
	prototypeNote := sources.prototypeNote
	if prototypeNote == "" {
		prototypeNote = "no prototypes yet"
	}

	// Product, Design, and Quality open the feature's page at the section that
	// lists what they hold: its stories, its design, and its test cases. The
	// feature's page is already the directory of all three, so a second table
	// per place would say the same thing twice and go out of step the first
	// time one of them changed.
	place := func(title, id, icon, anchor, emptyNote string, children []*navNodeView) *navNodeView {
		node := navPlace(title, id, icon, emptyNote, children)
		if sources.feature != "" {
			node.Href = featureHref(sources.feature) + anchor
		}
		return node
	}
	product := place("Product", prefix+"-product", "product", "#feature-product", "", []*navNodeView{
		navPlace("Prototypes", prefix+"-prototypes", "prototype", prototypeNote, sources.prototypes),
		requirements,
	})
	technical := navPlace("Technical", prefix+"-technical", "", "", append([]*navNodeView{
		navPlace("ERD", prefix+"-technical-erd", "", "not authored yet", nil),
		navPlace("System", prefix+"-technical-system", "", "not authored yet", nil),
		navPlace("Data Flows", prefix+"-technical-data-flows", "", "no flow diagrams yet", sources.dataFlows),
	}, sources.technical...))
	design := place("Design", prefix+"-design", "design", "#feature-design", "", []*navNodeView{
		navPlace("UX", prefix+"-design-ux", "", "no flow decks yet", sources.uxDecks),
		navPlace("UI", prefix+"-design-ui", "", "no references yet", sources.uiDesign),
		technical,
	})
	quality := place("Quality", prefix+"-quality", "quality", "#feature-quality", "", []*navNodeView{
		navPlace("Test Cases", prefix+"-test-cases", "", "no test cases yet", sources.testCases),
	})
	// Implementation is the one place that opens on arrival, and it opens all
	// the way to the slides. The deck that explains the change is what a
	// reviewer came for, so the sidebar shows what is actually there instead of
	// a row to click first. Everything else stays shut: four short rows read as
	// one architecture, where four open ones read as a wall.
	implementation := navPlace("Implementation", prefix+"-implementation", "implementation", "", sources.implementation)
	if len(implementation.Children) == 0 {
		// A top-level place is a peer of the others and has to read as one. As
		// a gap row it lost its disclosure, its weight, and most of its title
		// to the note, so an empty Implementation looked like a leftover rather
		// than a quarter of the architecture. It keeps the header every
		// section has and states the gap beneath it instead.
		implementation.Gap, implementation.Note = false, ""
		implementation.Children = []*navNodeView{{
			Title: "No implementation decks yet", NodeID: prefix + "-implementation-empty", Gap: true,
		}}
	}
	// Implementation is the deck. With the one deck a Saga normally has, its
	// slides sit directly beneath the section instead of under a deck row that
	// only restates the section and costs a click. Several decks keep their
	// rows, since the reader then needs to know which deck a slide belongs to.
	if len(implementation.Children) == 1 && implementation.Children[0].Deck && len(implementation.Children[0].Children) > 0 {
		implementation.Children = implementation.Children[0].Children
	}
	implementation.Expanded = true
	for _, deck := range implementation.Children {
		deck.Expanded = len(deck.Children) > 0
	}
	// The header opens the deck at its first slide. A deck knows where it
	// starts; a header that only expanded made the reader pick a slide before
	// they had read one.
	if href := firstSlideHref(implementation.Children); href != "" {
		implementation.Href = href
	}

	navigation := []*navNodeView{product, design, quality, implementation}
	for _, node := range navigation {
		revealActive(node)
	}
	return navigation
}

// navPlace is one row of the architecture: a disclosure when something fills
// it and an explicit gap when nothing does. It carries no destination of its
// own. navSection is the variant that does, and every section header in the
// sidebar is one of those; navPlace is what is left for the rows inside a
// section that name a place rather than a page.
func navPlace(title, id, icon, emptyNote string, children []*navNodeView) *navNodeView {
	node := &navNodeView{
		Title: title, NodeID: id, Icon: icon, Group: true,
		Children: children,
	}
	if len(children) == 0 {
		node.Gap, node.Note = true, emptyNote
	}
	return node
}

// revealActive opens the places containing the current page. A collapsed place
// hides its children outright, so without this, opening a story would collapse
// the sidebar around the very row the reader is on.
func revealActive(node *navNodeView) bool {
	revealed := node.Active
	for _, child := range node.Children {
		if revealActive(child) {
			revealed = true
		}
	}
	if revealed {
		node.Expanded = true
	}
	return revealed
}

// makePrototypeNav names the prototypes that already exist. The server has no
// prototype surface yet, so each row is a name rather than a link: a prototype
// a reviewer cannot even see listed is harder to ask about than one that is
// listed and not yet openable.
// TODO: give these rows an href once a prototype review route exists.
func makePrototypeNav(document prototypes.Document) []*navNodeView {
	var nodes []*navNodeView
	for _, prototype := range document.Prototypes {
		title := prototype.Identity.ID
		if prototype.CurrentRevision != nil && strings.TrimSpace(prototype.CurrentRevision.Title) != "" {
			title = strings.TrimSpace(prototype.CurrentRevision.Title)
		}
		target, err := prototypes.PrototypeURN(document.SagaID, prototype.Identity.ID)
		if err != nil {
			target = prototype.Identity.ID
		}
		nodes = append(nodes, &navNodeView{Title: title, NodeID: "nav-" + domID(target), Icon: "prototype"})
	}
	return nodes
}

// designSection reports whether a section was loaded from ___design. The saga
// loader joins design packages into the report hierarchy, and their retained
// on-disk path is the only recorded signal that a chapter is technical design
// rather than narrative.
func designSection(section *saga.Section) bool {
	return saga.IsDesignPath(section.Path)
}

// makeDesignChapterNav projects the ___design chapters into Technical. Which
// of ERD, System, or Data Flows a chapter satisfies is not recorded, so the
// chapter keeps its authored title and claims none of them.
func makeDesignChapterNav(root *saga.Section) []*navNodeView {
	if root == nil {
		return nil
	}
	var nodes []*navNodeView
	for _, child := range root.Children {
		if child.Kind != "chapter" || !designSection(child) {
			continue
		}
		nodes = append(nodes, makeChapterNav(child))
	}
	return nodes
}

// splitDeckNavByRole folds the decks that used to occupy their own top-level
// sidebar path into Design > UX and Implementation.
//
// Implementation is where a deck belongs unless it says otherwise. The slide
// deck that explains the change is the core artifact of a Change Saga, and an
// embedded report deck must carry role "change": internal/saga validation
// rejects any other role, because the report itself is the overview. So a deck
// arriving here without a design role is not one whose role went unrecorded —
// it is the implementation deck, named by the only role it is allowed to have.
//
// "ux" is the one role that moves a deck out of Implementation, for the UX
// flows the authoring grammar will add as `add-deck --role ux`. It is accepted
// ahead of that grammar so the seam is already correct when the role lands.
func splitDeckNavByRole(nodes []*navNodeView, decks []*saga.Deck) (ux, implementation []*navNodeView) {
	roles := make(map[string]string, len(decks))
	for _, deck := range decks {
		roles["nav-"+domID(deck.Target)] = deck.Role
	}
	for _, node := range nodes {
		if roles[node.NodeID] == "ux" {
			ux = append(ux, node)
			continue
		}
		implementation = append(implementation, node)
	}
	return ux, implementation
}

// spliceProductNav puts the architecture immediately below the report
// overview, which keeps it at the same four rows on every Saga. Narrative
// chapters follow it: their number varies with what was written, so anything
// placed after them would move.
func spliceProductNav(narrative, product []*navNodeView) []*navNodeView {
	if len(narrative) == 0 {
		return product
	}
	navigation := make([]*navNodeView, 0, len(narrative)+len(product))
	navigation = append(navigation, narrative[0])
	navigation = append(navigation, product...)
	return append(navigation, narrative[1:]...)
}

// prototypeNav reads the prototype packages for the sidebar. The packages are
// a small bounded directory scan that opens no prototype content, and an
// unreadable one degrades to a stated gap: a broken package is a thing the
// reviewer should be told about, not a reason the report fails to render.
func (a *app) prototypeNav(sagaID string) ([]*navNodeView, string) {
	document, note := a.prototypeDocument(sagaID)
	return makePrototypeNav(document), note
}

// prototypeDocument reads the prototype packages, or reports why it could not.
func (a *app) prototypeDocument(sagaID string) (prototypes.Document, string) {
	document, err := prototypes.Load(a.root, sagaID)
	if err != nil {
		return prototypes.Document{SagaID: sagaID}, "could not be read; run change-saga validate"
	}
	return document, ""
}
