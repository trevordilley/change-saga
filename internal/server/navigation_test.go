package server

import (
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

func TestProductNavigationHidesEveryEmptySection(t *testing.T) {
	t.Parallel()
	nodes := makeProductNavTree(productNavSources{})
	if len(nodes) != 0 {
		t.Fatalf("empty architecture rendered %v", navTitles(nodes, 0))
	}
}

// The order is an information architecture, not a phase gate: authoring
// Quality before any Design must not move Quality up.
func TestProductNavigationDoesNotReorderAsWorkProgresses(t *testing.T) {
	t.Parallel()
	sources := productNavSources{
		testCases: []*navNodeView{{Title: "Retry preserves the cart"}},
	}
	nodes := makeProductNavTree(sources)
	if len(nodes) != 1 || nodes[0].Title != "Quality" {
		t.Fatalf("authoring order changed the architecture: %v", navTitles(nodes, 0))
	}
	if findNav(t, nodes, "Quality", "Test Cases").Gap {
		t.Fatal("an authored test case must fill its place rather than stay a gap")
	}
}

// Prototypes precede the stories, which sit directly under Stories while
// keeping their canonical destinations and nested acceptance criteria.
func TestProductListsStoriesDirectlyAfterPrototypes(t *testing.T) {
	t.Parallel()
	requirements := makeRequirementsNav(&requirementsPageView{Stories: []*requirementStoryView{{
		ID: "checkout", Label: "Story 01", Title: "Complete checkout", Href: "/requirements/checkout",
		Criteria: []*requirementCriterionView{{Label: "AC 01", Statement: "The cart survives a retry.", Href: "/requirements/checkout/criteria/cart"}},
	}}})
	nodes := makeProductNavTree(productNavSources{
		requirements: requirements,
		prototypes:   []*navNodeView{{Title: "Checkout walkthrough"}},
	})
	product := findNav(t, nodes, "Stories")
	if len(product.Children) != 2 || product.Children[0].Title != "Prototypes" || product.Children[1] != requirements.Children[0] {
		t.Fatalf("product order = %v", navTitles(product.Children, 0))
	}
	story := product.Children[1]
	if story.Href != "/requirements/checkout" || len(story.Children) != 1 || story.Children[0].Href != "/requirements/checkout/criteria/cart" {
		t.Fatalf("story and criterion destinations changed: %#v", story)
	}
}

func TestRequirementsWithoutStoriesIsHidden(t *testing.T) {
	t.Parallel()
	nodes := makeProductNavTree(productNavSources{requirements: makeRequirementsNav(&requirementsPageView{})})
	if len(nodes) != 0 {
		t.Fatalf("empty requirements rendered %v", navTitles(nodes, 0))
	}
}

// Decks used to occupy their own top-level sidebar path. They now fold into
// the architecture by role: Technical is where a deck belongs unless it
// says otherwise, and "ux" is the one role that moves it out.
func TestDeckNavigationFoldsIntoDesignAndTechnicalByRole(t *testing.T) {
	t.Parallel()
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
		// artifact a Change Saga exists to review. It belongs in Technical
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
		if node.Title == "Technical" && node.Gap {
			t.Fatal("implementation decks must fill Technical")
		}
	}

	// The reviewer applies the same split inside each feature.
	sources := appNavFixture(t)
	uxDeck, uxRow := appNavDeck("billing-ux", "ux", "happy-path")
	billing := sources.document.Features[0]
	billing.Decks = append(billing.Decks, uxDeck)
	sources.decks = append(sources.decks, uxRow)
	app := makeAppNavTree(sources)
	if ux := findNav(t, app, "Features", "Billing", "Design", "UX"); ux.Gap || len(ux.Children) != 1 || ux.Children[0] != uxRow {
		t.Fatalf("a feature's ux deck must fill its Design > UX: %v", navTitles(ux.Children, 0))
	}
	if got := topTitles(findNav(t, app, "Features", "Billing", "Technical").Children); got != "charge|refund" {
		t.Fatalf("a ux deck must leave the feature's Technical to the change deck: %s", got)
	}
	// Catalog only spends rows on its four places when the reader is in it.
	sources.pageFeature = "catalog"
	if findNavByID(makeAppNavTree(sources), featureNavID("catalog")+"-design") != nil {
		t.Fatal("an empty Design section must not render for another feature")
	}
}

// ___design is the only recorded signal that a chapter is technical design.
// Which technical category it satisfies is not recorded, so the chapter keeps
// its authored title without inventing empty categories around it.
func TestDesignChaptersJoinArchitectureWithoutClaimingAFixedRole(t *testing.T) {
	t.Parallel()
	root := &saga.Section{ID: "root", Target: saga.SagaTarget("test"), Children: []*saga.Section{
		{Kind: "chapter", ID: "delivery", Title: "Delivery", Path: "delivery.chapter", Target: saga.ChapterTarget("test", "delivery")},
		{Kind: "chapter", ID: "architecture", Title: "Technical architecture", Path: "___design/architecture.chapter", Target: saga.ChapterTarget("test", "architecture")},
	}}
	narrative := makeNavTree(root)
	if len(narrative) != 2 || narrative[1].Title != "Delivery" {
		t.Fatalf("narrative chapters = %v", navTitles(narrative, 0))
	}
	technical := makeDesignChapterNav(root)
	if len(technical) != 1 || technical[0].Href != sagaHref(saga.ChapterTarget("test", "architecture")) {
		t.Fatalf("technical chapters = %#v", technical)
	}
	nodes := makeProductNavTree(productNavSources{technical: technical})
	architecture := findNav(t, nodes, "Technical", "Architecture")
	if architecture.NodeID != "nav-technical" {
		t.Fatalf("Architecture rename changed stable navigation identity: %q", architecture.NodeID)
	}
	if got := topTitles(nodes); got != "Technical" {
		t.Fatalf("architecture without a deck must still live under Technical: %s", got)
	}
	technical[0].Active = true
	if !findNav(t, makeProductNavTree(productNavSources{technical: technical}), "Technical", "Architecture").Expanded {
		t.Fatal("Architecture must reveal its active chapter")
	}
	children := architecture.Children
	if got := navTitles(children, 0); strings.Join(got, "|") != "Technical architecture" {
		t.Fatalf("technical order = %v", got)
	}
}

func TestPrototypeNavigationNamesPrototypesFromTheirCurrentRevision(t *testing.T) {
	t.Parallel()
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

// The app-level list has the same five app rows on every Saga, then the one
// feature the reader is on and the way to every other. That feature lists its own
// report outline and then the same four places, so the architecture sits at a
// stable place inside it whatever the app-level content is.
func TestProductNavigationSitsInsideEveryFeatureBelowItsReportOutline(t *testing.T) {
	t.Parallel()
	sources := appNavFixture(t)
	billing := sources.document.Features[0]
	billing.Report.Children = []*saga.Section{
		{Kind: "chapter", ID: "delivery", Title: "Delivery", Target: saga.ChapterTarget(appNavSaga, "delivery")},
		{Kind: "chapter", ID: "evidence", Title: "Evidence", Target: saga.ChapterTarget(appNavSaga, "evidence")},
	}
	nodes := makeAppNavTree(sources)
	if got, want := topTitles(nodes), "Overview|Features"; got != want {
		t.Fatalf("sidebar = %s, want %s", got, want)
	}
	if got, want := topTitles(findNav(t, nodes, "Features", "Billing").Children), "Billing overview|Delivery|Evidence|Stories|Technical"; got != want {
		t.Fatalf("billing feature = %s, want %s", got, want)
	}
	billing.Report = nil
	if got, want := topTitles(findNav(t, makeAppNavTree(sources), "Features", "Billing").Children), "Stories|Technical"; got != want {
		t.Fatalf("feature without report content = %s, want %s", got, want)
	}
}

func TestEmptySectionsDoNotRender(t *testing.T) {
	t.Parallel()
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	if err := tmpl.ExecuteTemplate(&rendered, "doc-tree", makeProductNavTree(productNavSources{})); err != nil {
		t.Fatal(err)
	}
	if html := rendered.String(); strings.TrimSpace(html) != "" {
		t.Fatalf("empty architecture rendered markup: %s", html)
	}
}

// Technical opens on arrival and the rest stays shut. A sidebar that
// opened every filled place would bury the deck a reviewer came to read under
// rows they did not ask for.
func TestOnlyTechnicalOpensOnArrival(t *testing.T) {
	t.Parallel()
	deck := &navNodeView{Title: "Technical review", NodeID: "nav-deck", Deck: true,
		Children: []*navNodeView{
			{Title: "Overview", NodeID: "overview-deck", Href: "/features/core?view=slides#overview-deck"},
			{Title: "Architecture and storage", NodeID: "nav-slide", Href: "/features/core?view=slides#slide-one", Slide: &SlideReferenceView{}},
		}}
	nodes := makeProductNavTree(productNavSources{
		requirements:   &navNodeView{Title: "Requirements", Children: []*navNodeView{{Title: "Story 01 · Refund window"}}},
		prototypes:     []*navNodeView{{Title: "Checkout flow"}},
		uxDecks:        []*navNodeView{{Title: "Checkout UX"}},
		technical:      []*navNodeView{{Title: "Storage model"}},
		implementation: []*navNodeView{deck},
	})

	implementation := findNav(t, nodes, "Technical")
	if !implementation.Expanded {
		t.Fatal("Technical must open on arrival")
	}
	// Technical is the deck: its slides sit directly beneath the section,
	// with no deck row restating it in between.
	if got := topTitles(implementation.Children); got != "Overview|Architecture|Architecture and storage" {
		t.Fatalf("implementation must list the deck's slides directly: %v", navTitles(implementation.Children, 0))
	}
	if implementation.Href != deck.Children[1].Href {
		t.Fatal("Technical must keep opening the first slide")
	}
	if findNav(t, nodes, "Technical", "Architecture").Expanded {
		t.Fatal("Architecture must stay collapsed until opened or active")
	}
	for _, title := range []string{"Stories", "Design"} {
		if findNav(t, nodes, title).Expanded {
			t.Fatalf("%s must stay collapsed on arrival", title)
		}
	}
}

// With several implementation decks the rows stay, so a reader can tell which
// deck a slide belongs to; each still opens to its slides.
func TestSeveralTechnicalDecksKeepTheirRows(t *testing.T) {
	t.Parallel()
	first := &navNodeView{Title: "Storage", NodeID: "nav-a", Deck: true, Children: []*navNodeView{{Title: "Schema"}}}
	second := &navNodeView{Title: "Checkout", NodeID: "nav-b", Deck: true, Children: []*navNodeView{{Title: "Retry"}}}
	nodes := makeProductNavTree(productNavSources{implementation: []*navNodeView{first, second}})

	implementation := findNav(t, nodes, "Technical")
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
	t.Parallel()
	story := &navNodeView{Title: "Story 01 · Refund window", Active: true}
	nodes := makeProductNavTree(productNavSources{
		requirements: &navNodeView{Title: "Requirements", Children: []*navNodeView{
			story, {Title: "Story 02 · Refund limits"},
		}},
	})

	if product := findNav(t, nodes, "Stories"); !product.Expanded {
		t.Fatal("Stories must open around the active story")
	}
	if current := findNav(t, nodes, "Stories", story.Title); current != story {
		t.Fatal("the active story must sit directly under Stories")
	}
	if findNavByID(nodes, "nav-design") != nil || findNavByID(nodes, "nav-quality") != nil {
		t.Fatal("empty places must not render")
	}
}

func TestTechnicalKeepsOverviewDestinationForEmptyDeck(t *testing.T) {
	t.Parallel()
	overview := &navNodeView{Title: "Overview", NodeID: "overview-deck-empty", Href: "/features/billing?view=slides#overview-deck-empty"}
	deck := &navNodeView{Title: "Empty deck", Deck: true, NodeID: "nav-empty", Children: []*navNodeView{overview}}
	nodes := makeProductNavTree(productNavSources{feature: "billing", implementation: []*navNodeView{deck}})
	technical := findNav(t, nodes, "Technical")
	if technical.NodeID != "nav-implementation" || technical.Href != overview.Href || len(technical.Children) != 1 || technical.Children[0] != overview {
		t.Fatalf("empty deck lost its stable navigation identity or overview: %#v", technical)
	}
	slide := &navNodeView{Href: "/features/billing?view=slides#slide-one", Slide: &SlideReferenceView{}}
	if got := deckEntryHref([]*navNodeView{overview, slide}); got != slide.Href {
		t.Fatalf("populated deck must still open first slide, got %q", got)
	}
	if got := navDeck("Onboarding", "nav-onboarding", "deck", []*navNodeView{overview}).Href; got != overview.Href {
		t.Fatalf("empty onboarding deck lost overview: %q", got)
	}
}
