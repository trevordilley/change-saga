package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
// story. The app has an onboarding deck, three personas, and three flags.
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
		decks: []*navNodeView{billingRow, onboardingRow},
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

// The app-level list replaces the four-row sidebar: the app's own places come
// first and every epic keeps Product, Design, Quality, and Implementation.
func TestAppNavigationListsAppPlacesThenEveryEpicWithItsFourPlaces(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	if got, want := topTitles(nodes), "Overview|Personas|Design system|Onboarding|Feature flags|Epics"; got != want {
		t.Fatalf("app-level list = %s, want %s", got, want)
	}
	wantIDs := []string{"nav-overview", "nav-personas", "nav-designsystem", "nav-onboarding", "nav-featureflags", "nav-epics"}
	for index, node := range nodes {
		if node.NodeID != wantIDs[index] {
			t.Fatalf("app place %q has node ID %q, want %q", node.Title, node.NodeID, wantIDs[index])
		}
	}
	epics := findNav(t, nodes, "Epics")
	if !epics.Expanded || epics.Gap || topTitles(epics.Children) != "Billing|Catalog" {
		t.Fatalf("Epics must open to every epic in order: %#v %v", epics, navTitles(epics.Children, 0))
	}
	for _, epic := range epics.Children {
		prefix := epicNavID(strings.ToLower(epic.Title))
		if epic.NodeID != prefix || !epic.Group || !epic.Expanded {
			t.Fatalf("epic group %q = %#v", epic.Title, epic)
		}
		// The epic's own report outline comes first, then its four places.
		if got, want := topTitles(epic.Children), epic.Title+" overview|"+epic.Title+" notes|Product|Design|Quality|Implementation"; got != want {
			t.Fatalf("epic %s = %s, want %s", epic.Title, got, want)
		}
		places := epic.Children[2:]
		for index, suffix := range []string{"-product", "-design", "-quality", "-implementation"} {
			if places[index].NodeID != prefix+suffix {
				t.Fatalf("epic %s place %q node ID = %q, want %q", epic.Title, places[index].Title, places[index].NodeID, prefix+suffix)
			}
		}
		for _, place := range places[:3] {
			if place.Expanded {
				t.Fatalf("epic %s: %s must stay collapsed on arrival", epic.Title, place.Title)
			}
		}
		if !places[3].Expanded {
			t.Fatalf("epic %s: Implementation must open on arrival", epic.Title)
		}
	}
	// Billing's one deck is its Implementation: the slides sit directly beneath.
	billing := findNav(t, nodes, "Epics", "Billing", "Implementation")
	if got := topTitles(billing.Children); got != "charge|refund" {
		t.Fatalf("billing Implementation must list its deck's slides directly: %v", navTitles(billing.Children, 0))
	}
	// Catalog has no deck and says so beneath a peer header.
	catalog := findNav(t, nodes, "Epics", "Catalog", "Implementation")
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
	if got, want := topTitles(nodes), "Overview|Personas|Design system|Onboarding|Feature flags|Epics"; got != want {
		t.Fatalf("empty app-level list = %s, want %s", got, want)
	}
	for _, node := range nodes {
		if !node.Gap || node.Note == "" || len(node.Children) != 0 {
			t.Fatalf("empty %s must be a stated gap: %#v", node.Title, node)
		}
	}
	for title, note := range map[string]string{
		"Overview": "no overview yet", "Personas": "no personas yet", "Design system": "no design system yet",
		"Onboarding": "no onboarding deck yet", "Feature flags": "no feature flags yet", "Epics": "no epics yet",
	} {
		if got := findNav(t, nodes, title).Note; got != note {
			t.Fatalf("%s gap note = %q, want %q", title, got, note)
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
	for _, note := range []string{"no personas yet", "no design system yet", "no onboarding deck yet", "no feature flags yet", "no epics yet"} {
		if !strings.Contains(rendered.String(), `<span class="doc-note">`+note+`</span>`) {
			t.Fatalf("rendered app-level list is missing the gap %q: %s", note, rendered.String())
		}
	}
}

// A persona is served only by an accepted story. A proposed story is not
// enough, and a retired persona needs no story at all.
func TestPersonaWithoutAnAcceptedStoryShowsItsGap(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	personas := findNav(t, nodes, "Personas")
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
	flags := findNav(t, nodes, "Feature flags")
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
	onboarding := findNav(t, nodes, "Onboarding")
	if onboarding.Gap || topTitles(onboarding.Children) != "who-it-serves" {
		t.Fatalf("Onboarding must list its deck's slides directly: %v", navTitles(onboarding.Children, 0))
	}
	slideID := "nav-" + domID(saga.SlideTarget(appNavSaga, "who-it-serves"))
	for _, epic := range findNav(t, nodes, "Epics").Children {
		if findNavByID(epic.Children, slideID) != nil {
			t.Fatalf("the onboarding slide appeared under epic %s: %v", epic.Title, navTitles(epic.Children, 0))
		}
	}
	deckID := "nav-" + domID(saga.DeckTarget(appNavSaga, "welcome"))
	if findNavByID(findNav(t, nodes, "Epics").Children, deckID) != nil {
		t.Fatal("the onboarding deck appeared under an epic")
	}
	// And an epic's implementation slides stay out of Onboarding.
	if findNavByID(onboarding.Children, "nav-"+domID(saga.SlideTarget(appNavSaga, "charge"))) != nil {
		t.Fatal("an implementation slide appeared under Onboarding")
	}
}

// Story numbering is app-wide, but each story is listed only under the epic
// whose directory holds it.
func TestEpicStoriesAppearOnlyUnderThatEpicsRequirements(t *testing.T) {
	nodes := makeAppNavTree(appNavFixture(t))
	billing := findNav(t, nodes, "Epics", "Billing", "Product", "Requirements")
	catalog := findNav(t, nodes, "Epics", "Catalog", "Product", "Requirements")
	if billing.NodeID != epicNavID("billing")+"-requirements" || catalog.NodeID != epicNavID("catalog")+"-requirements" {
		t.Fatalf("requirements node IDs = %q, %q", billing.NodeID, catalog.NodeID)
	}
	if got := topTitles(billing.Children); got != "Story 01 · pay" {
		t.Fatalf("billing requirements = %s", got)
	}
	if got := topTitles(catalog.Children); got != "Story 02 · list-item" {
		t.Fatalf("catalog requirements = %s", got)
	}
	if billing.Gap || catalog.Gap {
		t.Fatal("an epic with a story must not show its Requirements as a gap")
	}
	// An epic with no stories keeps its Requirements row as a gap.
	sources := appNavFixture(t)
	sources.document.Epics = append(sources.document.Epics, appNavEpic("search", "Search"))
	search := findNav(t, makeAppNavTree(sources), "Epics", "Search", "Product", "Requirements")
	if !search.Gap || search.Note == "" || len(search.Children) != 0 {
		t.Fatalf("an epic with no stories must state its Requirements gap: %#v", search)
	}
}

// The page handler builds the app-level list from a real app Saga on disk:
// two epics with their own report content, one implementation deck, and the
// onboarding deck at the app root.
func TestPageRendersTheAppLevelListFromAnAppSaga(t *testing.T) {
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

	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load app saga: err=%v issues=%#v", err, validation.Issues)
	}
	if len(document.Epics) != 2 || len(document.Onboarding) != 1 || len(document.Decks) != 1 {
		t.Fatalf("app saga = %d epics, %d onboarding decks, %d implementation decks", len(document.Epics), len(document.Onboarding), len(document.Decks))
	}

	recorder := httptest.NewRecorder()
	(&app{root: root, sourceDir: root, template: serverTemplate(t)}).page(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("page status = %d: %s", recorder.Code, recorder.Body.String())
	}
	html := recorder.Body.String()
	for _, id := range []string{
		"nav-overview", "nav-onboarding", "nav-epics",
		epicNavID("billing"), epicNavID("billing") + "-product", epicNavID("billing") + "-implementation",
		epicNavID("catalog"), epicNavID("catalog") + "-design", epicNavID("catalog") + "-implementation",
	} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Fatalf("the sidebar is missing %q", id)
		}
	}
	for _, id := range []string{epicNavID("billing") + "-implementation", epicNavID("catalog") + "-implementation"} {
		if strings.Contains(html, `id="`+id+`" hidden`) {
			t.Fatalf("%s must open on arrival", id)
		}
	}
	if !strings.Contains(html, `id="`+epicNavID("billing")+`-product" hidden`) {
		t.Fatal("an epic's Product must stay collapsed on arrival")
	}
	for _, note := range []string{"no personas yet", "no design system yet", "no feature flags yet", "No implementation decks yet"} {
		if !strings.Contains(html, note) {
			t.Fatalf("the sidebar does not state the gap %q", note)
		}
	}
	// The onboarding slide renders under Onboarding, before the Feature flags
	// row and the epics; the billing slide renders inside the epics.
	sidebarSlide := func(slide string) int {
		marker := `class="slide-thumbnail-hit" data-slide-thumbnail data-slide-target="` + saga.SlideTarget("shop", slide) + `"`
		if count := strings.Count(html, marker); count != 1 {
			t.Fatalf("sidebar slide %s rendered %d times", slide, count)
		}
		return strings.Index(html, marker)
	}
	onboarding, flags, epics := strings.Index(html, `id="nav-onboarding"`), strings.Index(html, `title="Feature flags"`), strings.Index(html, `id="nav-epics"`)
	if welcome := sidebarSlide("who-it-serves"); welcome < onboarding || welcome > flags {
		t.Fatalf("the onboarding slide is not under Onboarding: onboarding=%d slide=%d flags=%d", onboarding, welcome, flags)
	}
	if charge := sidebarSlide("charge"); charge < strings.Index(html, `id="`+epicNavID("billing")+`-implementation"`) || charge < epics {
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
