package server

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

const appNavSaga = "shop"

// appNavDeck is one loaded deck and the sidebar row makeDeckNavTree projects
// for it, with one slide row per slide ID.
func appNavDeck(id, role string, slides ...string) (*saga.Deck, *navNodeView) {
	target := saga.DeckTarget(appNavSaga, id)
	deck := &saga.Deck{DeckManifest: saga.DeckManifest{ID: id, Title: id, Role: role}, Target: target}
	row := &navNodeView{Title: id, NodeID: "nav-" + domID(target), Icon: "deck", Deck: true}
	for _, slide := range slides {
		slideTarget := saga.SlideTarget(appNavSaga, slide)
		row.Children = append(row.Children, &navNodeView{
			Title: slide, Href: "?view=slides#" + domID(slideTarget), NodeID: "nav-" + domID(slideTarget),
			Slide: &SlideReferenceView{Target: slideTarget, Title: slide},
		})
	}
	return deck, row
}

func appNavEpic(id, title string, decks ...*saga.Deck) *saga.Epic {
	report := &saga.Section{ID: id, Title: title, Target: applayout.EpicURN(appNavSaga, id),
		Fragments: []*saga.Fragment{{ID: id + "-overview", Title: title + " overview", Target: saga.FragmentTarget(appNavSaga, id+"-overview")}},
		Children:  []*saga.Section{{Kind: "chapter", ID: id + "-notes", Title: title + " notes", Target: saga.ChapterTarget(appNavSaga, id+"-notes")}},
	}
	return &saga.Epic{ID: id, Title: title, Path: applayout.EpicRel(id), Target: applayout.EpicURN(appNavSaga, id), Report: report, Decks: decks}
}

func appNavPersonaURN(t *testing.T, id string) string {
	t.Helper()
	urn, err := requirements.PersonaURN(appNavSaga, id)
	if err != nil {
		t.Fatal(err)
	}
	return urn
}

func appNavPersona(id, name string, state requirements.PersonaState) requirements.Persona {
	return requirements.Persona{
		Identity:         requirements.RecordIdentity{ID: id},
		CurrentRevision:  &requirements.PersonaRevision{ID: "r1", Name: name},
		CurrentLifecycle: &requirements.PersonaEvent{ID: "e1", State: state},
	}
}

func appNavStory(epic, id string, created int64, state requirements.LifecycleState, personas ...string) requirements.Story {
	return requirements.Story{
		Epic:             epic,
		Identity:         requirements.StoryIdentity{ID: id, CreatedAt: time.Unix(created, 0)},
		CurrentRevision:  &requirements.Revision{ID: "r1", Title: id, Statement: "As a buyer, I can " + id + ".", Priority: "must", Personas: personas},
		CurrentLifecycle: &requirements.LifecycleEvent{ID: "e1", State: state},
	}
}

// appNavFixture is an app with two epics: billing, which has one
// implementation deck and one story, and catalog, which has no deck and one
// story. The app has an onboarding deck, three personas, and three flags. The
// reader is on billing, so billing is the epic the sidebar opens.
func appNavFixture(t *testing.T) appNavSources {
	t.Helper()
	billingDeck, billingRow := appNavDeck("billing-flow", saga.DeckRoleChange, "charge", "refund")
	onboardingDeck, onboardingRow := appNavDeck("welcome", saga.DeckRoleOnboarding, "who-it-serves")
	billing := appNavEpic("billing", "Billing", billingDeck)
	catalog := appNavEpic("catalog", "Catalog")
	document := &saga.Saga{
		Manifest:   saga.Manifest{ID: appNavSaga},
		Section:    &saga.Section{ID: appNavSaga, Target: saga.SagaTarget(appNavSaga)},
		Decks:      []*saga.Deck{billingDeck},
		Onboarding: []*saga.Deck{onboardingDeck},
		Epics:      []*saga.Epic{billing, catalog},
	}
	buyer, seller := appNavPersonaURN(t, "buyer"), appNavPersonaURN(t, "seller")
	records := requirements.Document{
		SagaID: appNavSaga,
		Personas: []requirements.Persona{
			appNavPersona("buyer", "Buyer", requirements.PersonaActive),
			appNavPersona("seller", "Seller", requirements.PersonaActive),
			appNavPersona("auditor", "Auditor", requirements.PersonaRetired),
		},
		Flags: []requirements.Flag{
			{Identity: requirements.RecordIdentity{ID: "new-checkout"}, CurrentLifecycle: &requirements.FlagEvent{ID: "e1", State: requirements.FlagOn}},
			{Identity: requirements.RecordIdentity{ID: "bulk-listing"}, CurrentLifecycle: &requirements.FlagEvent{ID: "e1", State: requirements.FlagOff}},
			{Identity: requirements.RecordIdentity{ID: "split-heads"}},
		},
		Stories: []requirements.Story{
			appNavStory("billing", "pay", 1, requirements.StateAccepted, buyer),
			// The seller's only story is proposed, so nothing accepted serves them.
			appNavStory("catalog", "list-item", 2, requirements.StateProposed, seller),
		},
	}
	page, _, err := makeRequirementsSurface(records, requirementRoute{})
	if err != nil {
		t.Fatal(err)
	}
	return appNavSources{
		document: document, requirements: records, page: page,
		decks: []*navNodeView{billingRow, onboardingRow}, pageEpic: "billing",
	}
}

func epicNavID(id string) string { return "nav-epic-" + domID(id) }

func findNavByID(nodes []*navNodeView, id string) *navNodeView {
	for _, node := range nodes {
		if node.NodeID == id {
			return node
		}
		if found := findNavByID(node.Children, id); found != nil {
			return found
		}
	}
	return nil
}

func navIDs(nodes []*navNodeView) map[string]bool {
	ids := map[string]bool{}
	var walk func([]*navNodeView)
	walk = func(nodes []*navNodeView) {
		for _, node := range nodes {
			ids[node.NodeID] = true
			walk(node.Children)
		}
	}
	walk(nodes)
	return ids
}

func topTitles(nodes []*navNodeView) string {
	var titles []string
	for _, node := range nodes {
		titles = append(titles, node.Title)
	}
	return strings.Join(titles, "|")
}

// The app-level list is the app's own places, then every epic as a row, with
// only the epic the reader is inside opened over its four places. Listing
// every epic expanded put this repository's own sidebar at 238 rows.
func TestAppNavigationListsAppPlacesThenEveryEpicAsARow(t *testing.T) {
	sources := appNavFixture(t)
	sources.pageEpic = "billing"
	nodes := makeAppNavTree(sources)
	if got, want := topTitles(nodes), "Overview|Epics|Reviews"; got != want {
		t.Fatalf("app-level list = %s, want %s", got, want)
	}
	wantIDs := []string{"nav-overview", "nav-epics", "nav-reviews"}
	for index, node := range nodes {
		if node.NodeID != wantIDs[index] {
			t.Fatalf("app place %q has node ID %q, want %q", node.Title, node.NodeID, wantIDs[index])
		}
	}
	// What describes the whole app hangs off the overview, in order, beneath
	// the overview's own prose.
	if got, want := topTitles(findNav(t, nodes, "Overview").Children),
		"Name|Elevator pitch|Description|Terms and vocabulary|Personas|Design system|Onboarding|Feature flags"; got != want {
		t.Fatalf("overview parts = %s, want %s", got, want)
	}
	// Every epic is a row, in the order they were introduced.
	if got, want := topTitles(findNav(t, nodes, "Epics").Children), "Billing|Catalog"; got != want {
		t.Fatalf("epics section = %s, want %s", got, want)
	}
	// Only the epic being read opens; the other one is the row alone, linking
	// to its page, with nothing of its own beneath it.
	catalogRow := findNav(t, nodes, "Epics", "Catalog")
	if catalogRow.Href != epicHref("catalog") || len(catalogRow.Children) != 0 || catalogRow.Group || catalogRow.Expanded {
		t.Fatalf("an epic the reader is not in must be one row: %#v", catalogRow)
	}
	assertEpicSubtree(t, findNav(t, nodes, "Epics").Children, "Billing", "billing")
	// Billing's one deck is its Implementation: the slides sit directly beneath.
	billing := findNav(t, nodes, "Epics", "Billing", "Implementation")
	if got := topTitles(billing.Children); got != "charge|refund" {
		t.Fatalf("billing Implementation must list its deck's slides directly: %v", navTitles(billing.Children, 0))
	}
	// Reading the other epic moves the same four places onto it, and leaves
	// Billing the single row. Catalog has no deck and says so beneath a peer
	// header.
	other := appNavFixture(t)
	other.pageEpic = "catalog"
	chosen := makeAppNavTree(other)
	assertEpicSubtree(t, findNav(t, chosen, "Epics").Children, "Catalog", "catalog")
	if billingRow := findNav(t, chosen, "Epics", "Billing"); len(billingRow.Children) != 0 {
		t.Fatalf("both epics opened at once: %v", navTitles(chosen, 0))
	}
	catalog := findNav(t, chosen, "Epics", "Catalog", "Implementation")
	if catalog.Gap || len(catalog.Children) != 1 || !catalog.Children[0].Gap ||
		catalog.Children[0].NodeID != epicNavID("catalog")+"-implementation-empty" {
		t.Fatalf("an empty epic Implementation must state its gap beneath the header: %v", navTitles(catalog.Children, 0))
	}
	// The old unprefixed places are gone: every one belongs to an epic.
	ids := navIDs(nodes)
	for _, old := range []string{"nav-product", "nav-design", "nav-quality", "nav-implementation", "nav-requirements"} {
		if ids[old] {
			t.Fatalf("the sidebar still carries the single-change place %q", old)
		}
	}
}

// assertEpicSubtree checks the rules that hold beneath whichever epic the
// sidebar shows: its own report outline first, then the same four places, with
// only Implementation open.
func assertEpicSubtree(t *testing.T, nodes []*navNodeView, title, id string) {
	t.Helper()
	epic := findNav(t, nodes, title)
	prefix := epicNavID(id)
	if epic.NodeID != prefix || !epic.Group || !epic.Expanded || epic.Href != epicHref(id) {
		t.Fatalf("epic group %q = %#v", title, epic)
	}
	if got, want := topTitles(epic.Children), title+" overview|"+title+" notes|Product|Design|Quality|Implementation"; got != want {
		t.Fatalf("epic %s = %s, want %s", title, got, want)
	}
	places := epic.Children[2:]
	for index, suffix := range []string{"-product", "-design", "-quality", "-implementation"} {
		if places[index].NodeID != prefix+suffix {
			t.Fatalf("epic %s place %q node ID = %q, want %q", title, places[index].Title, places[index].NodeID, prefix+suffix)
		}
	}
	for _, place := range places[:3] {
		if place.Expanded {
			t.Fatalf("epic %s: %s must stay collapsed on arrival", title, place.Title)
		}
	}
	if !places[3].Expanded {
		t.Fatalf("epic %s: Implementation must open on arrival", title)
	}
}

// Every epic is a row whichever one is open, in creation order, each one
// linking to its own page. A page that belongs to no epic opens none of them,
// so the list is only ever rows.
func TestEveryEpicIsARowAndOnlyThePagesEpicOpens(t *testing.T) {
	for _, open := range []string{"", "not-an-epic", "billing", "catalog"} {
		sources := appNavFixture(t)
		sources.pageEpic = open
		nodes := makeAppNavTree(sources)
		epics := findNav(t, nodes, "Epics")
		if got, want := topTitles(epics.Children), "Billing|Catalog"; got != want {
			t.Fatalf("with %q open the epics section = %s, want %s", open, got, want)
		}
		var opened []string
		for index, row := range epics.Children {
			id := []string{"billing", "catalog"}[index]
			if row.NodeID != epicNavID(id) || row.Href != epicHref(id) || row.Icon != "product" {
				t.Fatalf("epic row = %#v", row)
			}
			if len(row.Children) > 0 {
				opened = append(opened, id)
			}
		}
		want := []string{open}
		if open != "billing" && open != "catalog" {
			want = nil
		}
		if strings.Join(opened, "|") != strings.Join(want, "|") {
			t.Fatalf("with %q open, %v opened, want %v", open, opened, want)
		}
	}
}

// An app nothing has been authored into still shows every app-level place, and
// each one says what is missing.
func TestEmptyAppPlacesStateTheirGap(t *testing.T) {
	page, _, err := makeRequirementsSurface(requirements.Document{SagaID: appNavSaga}, requirementRoute{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := makeAppNavTree(appNavSources{
		document:     &saga.Saga{Manifest: saga.Manifest{ID: appNavSaga}, Section: &saga.Section{ID: appNavSaga, Target: saga.SagaTarget(appNavSaga)}},
		requirements: requirements.Document{SagaID: appNavSaga},
		page:         page,
	})
	if got, want := topTitles(nodes), "Overview|Epics|Reviews"; got != want {
		t.Fatalf("empty app-level list = %s, want %s", got, want)
	}
	// With no epics there is nothing to list, so the row that says so stands
	// alone.
	if epics := findNav(t, nodes, "Epics"); len(epics.Children) != 0 {
		t.Fatalf("an app with no epics still listed some: %v", navTitles(epics.Children, 0))
	}
	for _, node := range append(findNav(t, nodes, "Overview").Children[4:], nodes[1:]...) {
		if !node.Gap || node.Note == "" || len(node.Children) != 0 {
			t.Fatalf("empty %s must be a stated gap: %#v", node.Title, node)
		}
	}
	// The overview always has its name; each other part is a stated gap.
	overview := findNav(t, nodes, "Overview")
	if overview.Gap || topTitles(overview.Children) != "Name|Elevator pitch|Description|Terms and vocabulary|Personas|Design system|Onboarding|Feature flags" {
		t.Fatalf("overview = %#v %s", overview, topTitles(overview.Children))
	}
	for _, part := range overview.Children[1:] {
		if !part.Gap || part.Note == "" {
			t.Fatalf("an absent overview part is a stated gap: %#v", part)
		}
	}
	for _, want := range []struct {
		path []string
		note string
	}{
		{[]string{"Overview", "Personas"}, "no personas yet"},
		{[]string{"Overview", "Design system"}, "no design system yet"},
		{[]string{"Overview", "Onboarding"}, "no onboarding deck yet"},
		{[]string{"Overview", "Feature flags"}, "no feature flags yet"},
		{[]string{"Epics"}, "no epics yet"},
		{[]string{"Reviews"}, "no reviews yet"},
	} {
		if got := findNav(t, nodes, want.path...).Note; got != want.note {
			t.Fatalf("%v gap note = %q, want %q", want.path, got, want.note)
		}
	}

	// The gaps survive rendering as visible, non-navigable text.
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered strings.Builder
	if err := tmpl.ExecuteTemplate(&rendered, "doc-tree", nodes); err != nil {
		t.Fatal(err)
	}
	for _, note := range []string{"not written yet", "no terms yet", "no personas yet", "no design system yet", "no onboarding deck yet", "no feature flags yet", "no epics yet", "no reviews yet"} {
		if !strings.Contains(rendered.String(), `<span class="doc-note">`+note+`</span>`) {
			t.Fatalf("rendered app-level list is missing the gap %q: %s", note, rendered.String())
		}
	}
}

// A persona is served only by an accepted story. A proposed story is not
// enough, and a retired persona needs no story at all.
func TestPersonaWithoutAnAcceptedStoryShowsItsGap(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	personas := findNav(t, nodes, "Overview", "Personas")
	if personas.Gap || topTitles(personas.Children) != "Buyer|Seller|Auditor" {
		t.Fatalf("personas = %v", navTitles(personas.Children, 0))
	}
	buyer, seller, auditor := personas.Children[0], personas.Children[1], personas.Children[2]
	if buyer.Gap || buyer.Note != "" || buyer.NodeID != "nav-"+domID(appNavPersonaURN(t, "buyer")) {
		t.Fatalf("a persona an accepted story serves must carry no gap: %#v", buyer)
	}
	if !seller.Gap || seller.Note != "no accepted story serves it" {
		t.Fatalf("a persona only a proposed story serves must state its gap: %#v", seller)
	}
	if auditor.Gap || auditor.Note != "retired" {
		t.Fatalf("a retired persona is not a gap: %#v", auditor)
	}
}

func TestFeatureFlagRowsShowTheirState(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	flags := findNav(t, nodes, "Overview", "Feature flags")
	if flags.Gap || len(flags.Children) != 3 {
		t.Fatalf("feature flags = %v", navTitles(flags.Children, 0))
	}
	for _, want := range []struct{ id, state string }{{"new-checkout", "on"}, {"bulk-listing", "off"}, {"split-heads", "conflicted"}} {
		row := findNav(t, flags.Children, want.id)
		urn, err := requirements.FlagURN(appNavSaga, want.id)
		if err != nil {
			t.Fatal(err)
		}
		if row.Note != want.state || row.NodeID != "nav-"+domID(urn) {
			t.Fatalf("flag %s = %#v, want state %q", want.id, row, want.state)
		}
	}
}

// The onboarding deck is the app's, not an epic's: its slides sit directly
// beneath Onboarding and never under any epic's Implementation.
func TestOnboardingSlidesSitUnderOnboardingAndNotUnderAnyEpic(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	onboarding := findNav(t, nodes, "Overview", "Onboarding")
	if onboarding.Gap || topTitles(onboarding.Children) != "who-it-serves" {
		t.Fatalf("Onboarding must list its deck's slides directly: %v", navTitles(onboarding.Children, 0))
	}
	slideID := "nav-" + domID(saga.SlideTarget(appNavSaga, "who-it-serves"))
	for _, epic := range []*navNodeView{findNav(t, nodes, "Epics", "Billing")} {
		if findNavByID(epic.Children, slideID) != nil {
			t.Fatalf("the onboarding slide appeared under epic %s: %v", epic.Title, navTitles(epic.Children, 0))
		}
	}
	deckID := "nav-" + domID(saga.DeckTarget(appNavSaga, "welcome"))
	if findNavByID([]*navNodeView{findNav(t, nodes, "Epics", "Billing")}, deckID) != nil {
		t.Fatal("the onboarding deck appeared under an epic")
	}
	// And an epic's implementation slides stay out of Onboarding.
	if findNavByID(onboarding.Children, "nav-"+domID(saga.SlideTarget(appNavSaga, "charge"))) != nil {
		t.Fatal("an implementation slide appeared under Onboarding")
	}
}

// Each story is listed, by its title, only under the epic whose directory
// holds it.
func TestEpicStoriesAppearOnlyUnderThatEpicsRequirements(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	billing := findNav(t, nodes, "Epics", "Billing", "Product", "Requirements")
	catalogSources := appNavFixture(t)
	catalogSources.pageEpic = "catalog"
	catalog := findNav(t, makeAppNavTree(catalogSources), "Epics", "Catalog", "Product", "Requirements")
	if billing.NodeID != epicNavID("billing")+"-requirements" || catalog.NodeID != epicNavID("catalog")+"-requirements" {
		t.Fatalf("requirements node IDs = %q, %q", billing.NodeID, catalog.NodeID)
	}
	if got := topTitles(billing.Children); got != "pay" {
		t.Fatalf("billing requirements = %s", got)
	}
	if got := topTitles(catalog.Children); got != "list-item" {
		t.Fatalf("catalog requirements = %s", got)
	}
	if billing.Gap || catalog.Gap {
		t.Fatal("an epic with a story must not show its Requirements as a gap")
	}
	// An epic with no stories keeps its Requirements row as a gap.
	sources := appNavFixture(t)
	sources.document.Epics = append(sources.document.Epics, appNavEpic("search", "Search"))
	sources.pageEpic = "search"
	search := findNav(t, makeAppNavTree(sources), "Epics", "Search", "Product", "Requirements")
	if !search.Gap || search.Note == "" || len(search.Children) != 0 {
		t.Fatalf("an epic with no stories must state its Requirements gap: %#v", search)
	}
}

// writeAppNavSaga is a two-epic app Saga on disk: Billing with its own
// implementation deck and Catalog without one, plus the app's overview and its
// onboarding deck. Both epics share a creation instant, so Billing is first by
// ID and is the epic a reader arrives on.
func writeAppNavSaga(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "shop.saga")
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"shop","title":"Shop","source":{"repository":"https://example.test/acme/shop.git"}}`)
	for _, epic := range []struct{ id, title string }{{"billing", "Billing"}, {"catalog", "Catalog"}} {
		dir := applayout.EpicDir(root, epic.id)
		writeServerFile(t, filepath.Join(dir, applayout.EpicManifestName), fmt.Sprintf(`{"$schema":%q,"version":5,"id":%q,"title":%q,"created_at":"2026-08-21T12:00:00Z"}`, applayout.EpicSchemaURL, epic.id, epic.title))
		writeServerFile(t, filepath.Join(dir, "overview.fragment", "fragment.json"), fmt.Sprintf(`{"version":2,"id":"%s-overview","title":"%s overview","media_type":"text/markdown","entrypoint":"content.md"}`, epic.id, epic.title))
		writeServerFile(t, filepath.Join(dir, "overview.fragment", "content.md"), "# "+epic.title+"\n")
	}
	writeServerFile(t, filepath.Join(root, applayout.OverviewDir, "pitch.fragment", "fragment.json"), `{"version":2,"id":"pitch","title":"Elevator pitch","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(root, applayout.OverviewDir, "pitch.fragment", "content.md"), "# Shop\n")
	writeAppNavDeck(t, filepath.Join(applayout.EpicDir(root, "billing"), saga.EmbeddedSlidesDir), "billing-flow", saga.DeckRoleChange, "charge", "")
	writeAppNavDeck(t, filepath.Join(root, applayout.OnboardingDir), "welcome", saga.DeckRoleOnboarding, "who-it-serves", "urn:change-saga:shop:epic:billing")
	return root
}

// treeDigest is every file beneath root with its bytes, so a test can say
// that reading the reviewer wrote nothing.
func treeDigest(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = append(lines, fmt.Sprintf("%s %x", path, sha256.Sum256(body)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// The sidebar opens the epic of the page being read and nothing else. No
// preference is stored: nothing is written to the Saga, and nothing is written
// to the reader's browser either.
func TestTheSidebarOpensThePagesEpicAndStoresNothing(t *testing.T) {
	root := writeAppNavSaga(t)
	before := treeDigest(t, root)
	application := &app{root: root, sourceDir: root, template: serverTemplate(t)}
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		newMux(application).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, recorder.Code)
		}
		return recorder
	}
	// Every epic is a row wherever the reader is, and only the page's epic is
	// opened. The app's own pages belong to no epic, so they open none.
	opens := func(body, epic string) bool {
		return strings.Contains(body, `<div class="doc-children" id="`+epicNavID(epic)+`"`)
	}
	for _, want := range []struct {
		path string
		epic string
	}{
		{"/", ""},
		{"/terms", ""},
		{"/epics", ""},
		{epicHref("billing"), "billing"},
		{epicHref("catalog"), "catalog"},
	} {
		recorder := get(want.path)
		body := recorder.Body.String()
		for _, epic := range []string{"billing", "catalog"} {
			if !strings.Contains(body, `href="`+epicHref(epic)+`"`) {
				t.Fatalf("%s does not list the epic %s", want.path, epic)
			}
			if got := opens(body, epic); got != (epic == want.epic) {
				t.Fatalf("%s opens %s = %v, want %v", want.path, epic, got, epic == want.epic)
			}
		}
		// Nothing about the reading is remembered, so nothing varies by it.
		if cookies := recorder.Result().Cookies(); len(cookies) != 0 {
			t.Fatalf("%s wrote %#v", want.path, cookies)
		}
		if vary := recorder.Result().Header.Values("Vary"); len(vary) != 0 {
			t.Fatalf("%s varies by %v", want.path, vary)
		}
	}
	if after := treeDigest(t, root); after != before {
		t.Fatal("reading the reviewer wrote to the Saga")
	}
}

// The page handler builds the app-level list from a real app Saga on disk:
// two epics with their own report content, one implementation deck, and the
// onboarding deck at the app root.
func TestPageRendersTheAppLevelListFromAnAppSaga(t *testing.T) {
	root := writeAppNavSaga(t)
	writeServerFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"shop","title":"Shop","source":{"repository":"https://example.test/acme/shop.git"}}`)
	for _, epic := range []struct{ id, title string }{{"billing", "Billing"}, {"catalog", "Catalog"}} {
		dir := applayout.EpicDir(root, epic.id)
		writeServerFile(t, filepath.Join(dir, applayout.EpicManifestName), fmt.Sprintf(`{"$schema":%q,"version":5,"id":%q,"title":%q,"created_at":"2026-08-21T12:00:00Z"}`, applayout.EpicSchemaURL, epic.id, epic.title))
		writeServerFile(t, filepath.Join(dir, "overview.fragment", "fragment.json"), fmt.Sprintf(`{"version":2,"id":"%s-overview","title":"%s overview","media_type":"text/markdown","entrypoint":"content.md"}`, epic.id, epic.title))
		writeServerFile(t, filepath.Join(dir, "overview.fragment", "content.md"), "# "+epic.title+"\n")
	}
	writeServerFile(t, filepath.Join(root, applayout.OverviewDir, "pitch.fragment", "fragment.json"), `{"version":2,"id":"pitch","title":"Elevator pitch","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeServerFile(t, filepath.Join(root, applayout.OverviewDir, "pitch.fragment", "content.md"), "# Shop\n")
	writeAppNavDeck(t, filepath.Join(applayout.EpicDir(root, "billing"), saga.EmbeddedSlidesDir), "billing-flow", saga.DeckRoleChange, "charge", "")
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load app saga: err=%v issues=%#v", err, validation.Issues)
	}
	if len(document.Epics) != 2 || len(document.Onboarding) != 1 || len(document.Decks) != 1 {
		t.Fatalf("app saga = %d epics, %d onboarding decks, %d implementation decks", len(document.Epics), len(document.Onboarding), len(document.Decks))
	}

	render := func(path string) string {
		t.Helper()
		recorder := httptest.NewRecorder()
		newMux(&app{root: root, sourceDir: root, template: serverTemplate(t)}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
		}
		return recorder.Body.String()
	}
	html := render("/")
	// The app's own places, then every epic as a row. On a page that belongs
	// to no epic, none of them is opened over its places.
	for _, id := range []string{"nav-overview", "nav-onboarding", "nav-epics"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Fatalf("the sidebar is missing %q", id)
		}
	}
	for _, epic := range []string{"billing", "catalog"} {
		if !strings.Contains(html, `href="`+epicHref(epic)+`"`) {
			t.Fatalf("the sidebar does not list the epic %q", epic)
		}
		if strings.Contains(html, `id="`+epicNavID(epic)+`-product"`) {
			t.Fatalf("the sidebar spends rows on %q the reader is not reading", epic)
		}
	}
	if !strings.Contains(html, `href="/epics"`) {
		t.Fatal("the Epics header does not open the epics table")
	}
	// Opening an epic's page opens that epic, and only that one, over its
	// four places, with Implementation open to its slides.
	billing := render(epicHref("billing"))
	for _, id := range []string{epicNavID("billing"), epicNavID("billing") + "-product", epicNavID("billing") + "-implementation"} {
		if !strings.Contains(billing, `id="`+id+`"`) {
			t.Fatalf("the epic's page is missing %q", id)
		}
	}
	if strings.Contains(billing, `id="`+epicNavID("catalog")+`-product"`) {
		t.Fatal("reading one epic opened another")
	}
	if strings.Contains(billing, `id="`+epicNavID("billing")+`-implementation" hidden`) {
		t.Fatal("the epic's Implementation must open on arrival")
	}
	if !strings.Contains(billing, `id="`+epicNavID("billing")+`-product" hidden`) {
		t.Fatal("an epic's Product must stay collapsed on arrival")
	}
	// Opening the other epic's page moves the whole subtree onto it.
	catalog := render(epicHref("catalog"))
	if !strings.Contains(catalog, `id="`+epicNavID("catalog")+`-implementation"`) || strings.Contains(catalog, `id="`+epicNavID("billing")+`-product"`) {
		t.Fatal("opening an epic must be the only epic the sidebar opens")
	}
	// The epics index lists both, and is reachable as a page of its own.
	index := render("/epics")
	for _, want := range []string{`data-directory-page="epics"`, `href="/epics/billing"`, `href="/epics/catalog"`} {
		if !strings.Contains(index, want) {
			t.Fatalf("the epics index lacks %q", want)
		}
	}
	for _, note := range []string{"not written yet", "no terms yet", "no personas yet", "no design system yet", "no feature flags yet"} {
		if !strings.Contains(html, note) {
			t.Fatalf("the sidebar does not state the gap %q", note)
		}
	}
	// Catalog has no deck; its gap is stated on the page that shows it.
	if !strings.Contains(render(epicHref("catalog")), "No implementation decks yet") {
		t.Fatal("an epic with no deck does not state the gap")
	}
	// The onboarding slide renders under Onboarding, before the Feature flags
	// row and the epics. The billing slide renders inside Billing, on the
	// page that opens it, and nowhere else.
	sidebarSlide := func(body, slide string) int {
		marker := `class="slide-thumbnail-hit" data-slide-thumbnail data-slide-target="` + saga.SlideTarget("shop", slide) + `"`
		if count := strings.Count(body, marker); count != 1 {
			t.Fatalf("sidebar slide %s rendered %d times", slide, count)
		}
		return strings.Index(body, marker)
	}
	onboarding, flags := strings.Index(html, `id="nav-onboarding"`), strings.Index(html, `title="Feature flags"`)
	if welcome := sidebarSlide(html, "who-it-serves"); welcome < onboarding || welcome > flags {
		t.Fatalf("the onboarding slide is not under Onboarding: onboarding=%d slide=%d flags=%d", onboarding, welcome, flags)
	}
	chargeMarker := `class="slide-thumbnail-hit" data-slide-thumbnail data-slide-target="` + saga.SlideTarget("shop", "charge") + `"`
	if strings.Contains(html, chargeMarker) {
		t.Fatal("an epic's slide is in the sidebar of a page outside that epic")
	}
	if charge := sidebarSlide(billing, "charge"); charge < strings.Index(billing, `id="`+epicNavID("billing")+`-implementation"`) ||
		charge < strings.Index(billing, `id="nav-epics"`) {
		t.Fatal("the billing slide is not under Billing's Implementation")
	}
}

// writeAppNavDeck writes a one-slide, one-Item deck bundle into dir. An
// onboarding Item carries a record URN instead of code evidence.
func writeAppNavDeck(t *testing.T, dir, id, role, slide, record string) {
	t.Helper()
	bundle := filepath.Join(dir, id+saga.EmbeddedDeckSuffix)
	deckTarget := saga.DeckTarget("shop", id)
	deckName, _ := saga.FlatDeckFilename(deckTarget, 0)
	writeServerFile(t, filepath.Join(bundle, deckName), fmt.Sprintf(`{"version":4,"id":%q,"title":%q,"role":%q,"rank":0,"objective":"Explain %s."}`, id, id, role, id))
	slideTarget := saga.SlideTarget("shop", slide)
	slideName, _ := saga.FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeServerFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":%q,"deck":%q,"title":%q,"rank":0,"intent":"explain","layout":"diagram","media_type":"image/svg+xml","entrypoint":%q,"takeaway":"It is explicit.","reading_order":["%s-point"]}`, slide, id, slide, assetName, slide))
	writeServerFile(t, filepath.Join(bundle, assetName), fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1280 720"><text id="%s-point">%s</text></svg>`, slide, slide))
	itemTarget := saga.ItemTarget("shop", slide, slide+"-point")
	itemName, _ := saga.FlatItemFilename(slideTarget, itemTarget, 0)
	recordField := ""
	if record != "" {
		recordField = fmt.Sprintf(`,"record":%q`, record)
	}
	writeServerFile(t, filepath.Join(bundle, itemName), fmt.Sprintf(`{"version":4,"id":"%s-point","slide":%q,"rank":0,"kind":"callout","label":"Point","description":"The point.","selector":{"type":"element","element_id":"%s-point"},"body":"The point of %s."%s}`, slide, slide, slide, slide, recordField))
}
