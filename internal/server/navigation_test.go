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
	}
	if got := navTitles(nodes, 0); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("architecture =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	for _, path := range [][]string{{"Product", "Prototypes"}, {"Design", "UX"}, {"Design", "UI"},
		{"Design", "Technical", "ERD"}, {"Design", "Technical", "Data Flows"}, {"Quality", "Test Cases"}, {"Implementation"}} {
		node := findNav(t, nodes, path...)
		if !node.Gap || node.Note == "" {
			t.Fatalf("%v is not an explicit gap: %#v", path, node)
		}
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
// the architecture by role, and a deck whose role the format does not record
// says so instead of being assigned one.
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

// The architecture sits directly below the report overview so it occupies the
// same four rows on every Saga; narrative chapters, whose number varies, follow.
func TestProductNavigationSitsBelowTheOverviewAndAboveNarrativeChapters(t *testing.T) {
	narrative := []*navNodeView{{Title: "Overview"}, {Title: "Delivery"}, {Title: "Evidence"}}
	nodes := spliceProductNav(narrative, makeProductNavTree(productNavSources{}))
	var top []string
	for _, node := range nodes {
		top = append(top, node.Title)
	}
	want := "Overview|Product|Design|Quality|Implementation|Delivery|Evidence"
	if got := strings.Join(top, "|"); got != want {
		t.Fatalf("sidebar = %s, want %s", got, want)
	}
	if got := strings.Join(func() (titles []string) {
		for _, node := range spliceProductNav(nil, makeProductNavTree(productNavSources{})) {
			titles = append(titles, node.Title)
		}
		return titles
	}(), "|"); got != "Product|Design|Quality|Implementation" {
		t.Fatalf("architecture without a narrative overview = %s", got)
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
