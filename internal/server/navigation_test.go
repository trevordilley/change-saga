package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/saga"
)

// navTitles flattens the projection to the rows a reviewer reads, in order.
func navTitles(nodes []*navNodeView, depth int) []string {
	var titles []string
	for _, node := range nodes {
		titles = append(titles, strings.Repeat("  ", depth)+node.Title)
		titles = append(titles, navTitles(node.Children, depth+1)...)
	}
	return titles
}

func findNav(t *testing.T, nodes []*navNodeView, path ...string) *navNodeView {
	t.Helper()
	for _, node := range nodes {
		if node.Title != path[0] {
			continue
		}
		if len(path) == 1 {
			return node
		}
		return findNav(t, node.Children, path[1:]...)
	}
	t.Fatalf("no navigation row %q in %v", path[0], navTitles(nodes, 0))
	return nil
}

// An empty Saga still has to show every place work can go. The reviewer's
// question is what is missing, and a hidden section cannot answer it.
func TestProductNavigationProjectsTheStableOrderWithNothingAuthored(t *testing.T) {
	nodes := makeProductNavTree(productNavSources{})
	want := []string{
		"Product",
		"  Prototypes",
		"  Requirements",
		"Design",
		"  UX",
		"  UI",
		"  Technical",
		"    ERD",
		"    System",
		"    Data Flows",
		"Quality",
		"  Test Cases",
		"Implementation",
		"  No implementation decks yet",
	}
	if got := navTitles(nodes, 0); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("architecture =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, path := range [][]string{{"Product", "Prototypes"}, {"Design", "UX"}, {"Design", "UI"},
		{"Design", "Technical", "ERD"}, {"Design", "Technical", "Data Flows"}, {"Quality", "Test Cases"}} {
		node := findNav(t, nodes, path...)
		if !node.Gap || node.Note == "" {
			t.Fatalf("%v is not an explicit gap: %#v", path, node)
		}
	}
	// Implementation is a top-level peer: empty, it keeps the same header as
	// Product, Design, and Quality, stays open, and states its gap beneath.
	implementation := findNav(t, nodes, "Implementation")
	if implementation.Gap || implementation.Note != "" || !implementation.Expanded {
		t.Fatalf("an empty Implementation must keep a peer header: %#v", implementation)
	}
	if empty := findNav(t, nodes, "Implementation", "No implementation decks yet"); !empty.Gap {
		t.Fatalf("an empty Implementation must state its gap beneath the header: %#v", empty)
	}
	// A place in the architecture is never a destination of its own.
	for _, node := range nodes {
		if node.Href != "" || !node.Group {
			t.Fatalf("top-level place = %#v", node)
		}
	}
}

// The order is an information architecture, not a phase gate: authoring
// Quality before any Design must not move Quality up.
func TestProductNavigationDoesNotReorderAsWorkProgresses(t *testing.T) {
	sources := productNavSources{
		testCases: []*navNodeView{{Title: "Retry preserves the cart"}},
	}
	nodes := makeProductNavTree(sources)
	if len(nodes) != 4 || nodes[0].Title != "Product" || nodes[1].Title != "Design" ||
		nodes[2].Title != "Quality" || nodes[3].Title != "Implementation" {
		t.Fatalf("authoring order changed the architecture: %v", navTitles(nodes, 0))
	}
	if findNav(t, nodes, "Quality", "Test Cases").Gap {
		t.Fatal("an authored test case must fill its place rather than stay a gap")
	}
	if !findNav(t, nodes, "Design", "UX").Gap {
		t.Fatal("UX must still state its gap once quality work exists")
	}
}

// Prototypes precedes Requirements because prototype-first is the common
// discovery path, and Requirements is its own overview: it never gains a
// redundant "Overview" child.
func TestProductPutsPrototypesBeforeRequirementsAndKeepsRequirementsItsOwnOverview(t *testing.T) {
	requirements := makeRequirementsNav(&requirementsPageView{Stories: []*requirementStoryView{{
		ID: "checkout", Label: "Story 01", Title: "Complete checkout", Href: "/requirements/checkout",
		Criteria: []*requirementCriterionView{{Label: "AC 01", Statement: "The cart survives a retry.", Href: "/requirements/checkout/criteria/cart"}},
	}}})
	nodes := makeProductNavTree(productNavSources{
		requirements: requirements,
		prototypes:   []*navNodeView{{Title: "Checkout walkthrough"}},
	})
	product := findNav(t, nodes, "Product")
	if len(product.Children) != 2 || product.Children[0].Title != "Prototypes" || product.Children[1].Title != "Requirements" {
		t.Fatalf("product order = %v", navTitles(product.Children, 0))
	}
	stories := product.Children[1]
	if stories.Href != "/requirements" || stories.Gap {
		t.Fatalf("Requirements must stay its own overview destination: %#v", stories)
	}
	for _, child := range stories.Children {
		if strings.EqualFold(child.Title, "Overview") {
			t.Fatalf("Requirements gained a redundant Overview child: %v", navTitles(stories.Children, 0))
		}
	}
	if len(stories.Children) != 1 || len(stories.Children[0].Children) != 1 {
		t.Fatalf("story and criterion hierarchy = %v", navTitles(stories.Children, 0))
	}
}

func TestRequirementsWithoutStoriesStaysVisibleAsAGap(t *testing.T) {
	nodes := makeProductNavTree(productNavSources{requirements: makeRequirementsNav(&requirementsPageView{})})
	stories := findNav(t, nodes, "Product", "Requirements")
	if !stories.Gap || stories.Note == "" || stories.Href != "/requirements" {
		t.Fatalf("empty requirements = %#v", stories)
	}
}

// Decks used to occupy their own top-level sidebar path. They now fold into
// the architecture by role: Implementation is where a deck belongs unless it
// says otherwise, and "ux" is the one role that moves it out.
func TestDeckNavigationFoldsIntoDesignAndImplementationByRole(t *testing.T) {
	root := &saga.Section{ID: "root", Target: saga.SagaTarget("test"), Children: []*saga.Section{
		{Kind: "deck", ID: "flows", Title: "Checkout flows", Target: saga.DeckTarget("test", "flows")},
		{Kind: "deck", ID: "build", Title: "Build plan", Target: saga.DeckTarget("test", "build")},
		{Kind: "deck", ID: "intro", Title: "How the refund changed", Target: saga.DeckTarget("test", "intro")},
	}}
	decks := []*saga.Deck{
		{DeckManifest: saga.DeckManifest{ID: "flows", Role: "ux"}, Target: saga.DeckTarget("test", "flows")},
		{DeckManifest: saga.DeckManifest{ID: "build", Role: "implementation"}, Target: saga.DeckTarget("test", "build")},
		// "change" is the only role an embedded report deck is allowed to
		// carry, and it is the slide deck that explains the change: the core
		// artifact a Change Saga exists to review. It belongs in Implementation
		// on its own terms, not as a deck whose role went unrecorded.
		{DeckManifest: saga.DeckManifest{ID: "intro", Role: "change"}, Target: saga.DeckTarget("test", "intro")},
	}

	ux, implementation := splitDeckNavByRole(makeDeckNavTree(root), decks)
	if len(ux) != 1 || ux[0].Title != "Checkout flows" || ux[0].Note != "" {
		t.Fatalf("ux decks = %#v", ux)
	}
	if len(implementation) != 2 || implementation[0].Title != "Build plan" || implementation[0].Note != "" {
		t.Fatalf("implementation decks = %#v", implementation)
	}
	if implementation[1].Title != "How the refund changed" || implementation[1].Note != "" {
		t.Fatalf("the change deck is the implementation deck and must carry no caveat: %#v", implementation[1])
	}

	nodes := makeProductNavTree(productNavSources{uxDecks: ux, implementation: implementation})
	if findNav(t, nodes, "Design", "UX").Gap {
		t.Fatal("a ux deck must fill Design > UX")
	}
	for _, node := range nodes {
		if node.Title == "Implementation" && node.Gap {
			t.Fatal("implementation decks must fill Implementation")
		}
	}

	// The reviewer applies the same split inside each epic.
	sources := appNavFixture(t)
	uxDeck, uxRow := appNavDeck("billing-ux", "ux", "happy-path")
	billing := sources.document.Epics[0]
	billing.Decks = append(billing.Decks, uxDeck)
	sources.decks = append(sources.decks, uxRow)
	app := makeAppNavTree(sources)
	if ux := findNav(t, app, "Epics", "Billing", "Design", "UX"); ux.Gap || len(ux.Children) != 1 || ux.Children[0] != uxRow {
		t.Fatalf("an epic's ux deck must fill its Design > UX: %v", navTitles(ux.Children, 0))
	}
	if got := topTitles(findNav(t, app, "Epics", "Billing", "Implementation").Children); got != "charge|refund" {
		t.Fatalf("a ux deck must leave the epic's Implementation to the change deck: %s", got)
	}
	if !findNav(t, app, "Epics", "Catalog", "Design", "UX").Gap {
		t.Fatal("another epic's ux deck must not fill this epic's Design > UX")
	}
}

// ___design is the only recorded signal that a chapter is technical design.
// Which of ERD, System, or Data Flows it satisfies is not recorded, so the
// chapter keeps its authored title and those three stay explicit gaps.
func TestDesignChaptersJoinTechnicalWithoutClaimingAFixedRole(t *testing.T) {
	root := &saga.Section{ID: "root", Target: saga.SagaTarget("test"), Children: []*saga.Section{
		{Kind: "chapter", ID: "delivery", Title: "Delivery", Path: "delivery.chapter", Target: saga.ChapterTarget("test", "delivery")},
		{Kind: "chapter", ID: "architecture", Title: "Technical architecture", Path: "___design/architecture.chapter", Target: saga.ChapterTarget("test", "architecture")},
	}}
	narrative := makeNavTree(root, nil)
	if len(narrative) != 2 || narrative[1].Title != "Delivery" {
		t.Fatalf("narrative chapters = %v", navTitles(narrative, 0))
	}
	technical := makeDesignChapterNav(root, nil)
	if len(technical) != 1 || technical[0].Href != sagaHref(saga.ChapterTarget("test", "architecture")) {
		t.Fatalf("technical chapters = %#v", technical)
	}
	nodes := makeProductNavTree(productNavSources{technical: technical})
	children := findNav(t, nodes, "Design", "Technical").Children
	if got := navTitles(children, 0); strings.Join(got, "|") != "ERD|System|Data Flows|Technical architecture" {
		t.Fatalf("technical order = %v", got)
	}
	for _, title := range []string{"ERD", "System", "Data Flows"} {
		if !findNav(t, nodes, "Design", "Technical", title).Gap {
			t.Fatalf("%s must stay an explicit gap until it is recorded", title)
		}
	}
}

func TestPrototypeNavigationNamesPrototypesFromTheirCurrentRevision(t *testing.T) {
	document := prototypes.Document{SagaID: "test", Prototypes: []prototypes.Prototype{
		{Identity: prototypes.Identity{ID: "checkout", CreatedAt: time.Unix(1, 0)},
			CurrentRevision: &prototypes.Revision{ID: "r1", Title: "Checkout walkthrough"}},
		{Identity: prototypes.Identity{ID: "unrevised", CreatedAt: time.Unix(2, 0)}},
	}}
	nodes := makePrototypeNav(document)
	if len(nodes) != 2 || nodes[0].Title != "Checkout walkthrough" || nodes[1].Title != "unrevised" {
		t.Fatalf("prototype navigation = %v", navTitles(nodes, 0))
	}
	if nodes[0].NodeID == nodes[1].NodeID || nodes[0].Icon != "prototype" {
		t.Fatalf("prototype rows must be distinct and recognisable: %#v", nodes)
	}
}

// The app-level list has the same six rows on every Saga, and each epic lists
// its own report outline and then the same four places, so the architecture
// sits at a stable place inside every epic whatever the app-level content is.
func TestProductNavigationSitsInsideEveryEpicBelowItsReportOutline(t *testing.T) {
	sources := appNavFixture(t)
	billing := sources.document.Epics[0]
	billing.Report.Children = []*saga.Section{
		{Kind: "chapter", ID: "delivery", Title: "Delivery", Target: saga.ChapterTarget(appNavSaga, "delivery")},
		{Kind: "chapter", ID: "evidence", Title: "Evidence", Target: saga.ChapterTarget(appNavSaga, "evidence")},
	}
	nodes := makeAppNavTree(sources)
	if got, want := topTitles(nodes), "Overview|Personas|Design system|Onboarding|Feature flags|Epics"; got != want {
		t.Fatalf("sidebar = %s, want %s", got, want)
	}
	if got, want := topTitles(findNav(t, nodes, "Epics", "Billing").Children), "Billing overview|Delivery|Evidence|Product|Design|Quality|Implementation"; got != want {
		t.Fatalf("billing epic = %s, want %s", got, want)
	}
	billing.Report = nil
	if got, want := topTitles(findNav(t, makeAppNavTree(sources), "Epics", "Billing").Children), "Product|Design|Quality|Implementation"; got != want {
		t.Fatalf("epic without report content = %s, want %s", got, want)
	}
}

// The gap rows have to survive rendering: a place that is only a struct field
// tells a reviewer nothing.
func TestGapRowsRenderAsVisibleNonNavigableText(t *testing.T) {
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "doc-tree", makeProductNavTree(productNavSources{})); err != nil {
		t.Fatal(err)
	}
	html := rendered.String()
	for _, expected := range []string{"Prototypes", "Requirements", "Data Flows", "Test Cases",
		"Implementation", `class="doc-note"`, "no test cases yet", `class="doc-link doc-static"`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("rendered architecture missing %q: %s", expected, html)
		}
	}
	if strings.Contains(html, `href=""`) {
		t.Fatalf("a place in the architecture was rendered as an empty link: %s", html)
	}
	if !strings.Contains(html, "data-doc-toggle") {
		t.Fatalf("a filled place must disclose its children: %s", html)
	}
}

// Implementation opens on arrival and the rest stays shut. A sidebar that
// opened every filled place would bury the deck a reviewer came to read under
// rows they did not ask for.
func TestOnlyImplementationOpensOnArrival(t *testing.T) {
	deck := &navNodeView{Title: "Implementation review", NodeID: "nav-deck", Deck: true,
		Children: []*navNodeView{{Title: "Architecture and storage", NodeID: "nav-slide"}}}
	nodes := makeProductNavTree(productNavSources{
		requirements:   &navNodeView{Title: "Requirements", Children: []*navNodeView{{Title: "Story 01 · Refund window"}}},
		prototypes:     []*navNodeView{{Title: "Checkout flow"}},
		technical:      []*navNodeView{{Title: "Storage model"}},
		implementation: []*navNodeView{deck},
	})

	implementation := findNav(t, nodes, "Implementation")
	if !implementation.Expanded {
		t.Fatal("Implementation must open on arrival")
	}
	// Implementation is the deck: its slides sit directly beneath the section,
	// with no deck row restating it in between.
	if len(implementation.Children) != 1 || implementation.Children[0].Title != "Architecture and storage" {
		t.Fatalf("implementation must list the deck's slides directly: %v", navTitles(implementation.Children, 0))
	}
	for _, title := range []string{"Product", "Design", "Quality"} {
		if findNav(t, nodes, title).Expanded {
			t.Fatalf("%s must stay collapsed on arrival", title)
		}
	}
}

// With several implementation decks the rows stay, so a reader can tell which
// deck a slide belongs to; each still opens to its slides.
func TestSeveralImplementationDecksKeepTheirRows(t *testing.T) {
	first := &navNodeView{Title: "Storage", NodeID: "nav-a", Deck: true, Children: []*navNodeView{{Title: "Schema"}}}
	second := &navNodeView{Title: "Checkout", NodeID: "nav-b", Deck: true, Children: []*navNodeView{{Title: "Retry"}}}
	nodes := makeProductNavTree(productNavSources{implementation: []*navNodeView{first, second}})

	implementation := findNav(t, nodes, "Implementation")
	if len(implementation.Children) != 2 || !implementation.Children[0].Deck || !implementation.Children[1].Deck {
		t.Fatalf("several decks must keep their rows: %v", navTitles(implementation.Children, 0))
	}
	if !first.Expanded || !second.Expanded {
		t.Fatal("each implementation deck must open to its slides")
	}
}

// A collapsed place hides its children outright, so the places containing the
// current page have to open or the reader loses the row they are standing on.
func TestActivePageOpensThePlacesThatContainIt(t *testing.T) {
	story := &navNodeView{Title: "Story 01 · Refund window", Active: true}
	nodes := makeProductNavTree(productNavSources{
		requirements: &navNodeView{Title: "Requirements", Children: []*navNodeView{
			story, {Title: "Story 02 · Refund limits"},
		}},
	})

	if product := findNav(t, nodes, "Product"); !product.Expanded {
		t.Fatal("Product must open around the active story")
	}
	if requirements := findNav(t, nodes, "Product", "Requirements"); !requirements.Expanded {
		t.Fatal("Requirements must open around the active story")
	}
	if findNav(t, nodes, "Design").Expanded || findNav(t, nodes, "Quality").Expanded {
		t.Fatal("a place that does not contain the current page must stay collapsed")
	}
}
