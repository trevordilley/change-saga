package server

import (
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The reviewer's app-level list has two sections, and every one of them is
// a page. The first is the same on both sides of the header; the second is
// what that side is about:
//
//	Overview        name, elevator pitch, description, and the parts that
//	                describe the whole app: terms and vocabulary, personas,
//	                the design system, onboarding, and feature flags
//	Features        (Documentation) the directory of every feature, over
//	                every feature as a row
//	Reviews         (Review) every pull request's review, one row each
//
// The overview is on both sides because it is what the application is, and a
// reader reviewing a change needs the same vocabulary and personas as one
// learning the app. What differs is the second section, which is the whole
// distinction the header draws: what the app does, or what is being changed
// about it. Personas, the design system, onboarding, and feature flags all
// describe the whole app rather than any one part of it, so they belong to
// the overview and not beside the features they cut across.
//
// No header in this list is a row that only expands. A section header opens
// the section: Overview opens its prose and its directory, Terms and
// vocabulary opens the table of terms, Onboarding opens the deck at its first
// slide, and Features and Reviews open their tables. The disclosure beside a
// header is how a reader reaches one row inside the section without leaving
// where they are; it is not the only way in.
//
// Every feature is listed, one row each, in the order an author introduced them.
// The row links to the feature's page, which is the directory of that feature's
// stories, design, quality, and implementation, so a reader reaches any feature
// in one click and reads the whole of it on a page.
//
// One of those rows opens: the feature whose content the reader is looking at,
// whether that is the feature's own page or a story, criterion, test case, slide,
// or chapter inside it. Every other feature stays shut, a single row. Listing
// every feature expanded put this repository's own sidebar at 238 rows, which is
// a wall rather than an architecture; listing them shut costs one row each, and
// the one feature the reader is already in is the only one that spends more.
//
// The sidebar is loaded once and kept while the reader moves between pages, so
// it holds every feature's places whichever page it was loaded with; a page
// only says which rows are current and which are open. A shut feature's places
// are there to disclose, like any other section's.
//
// Within the open feature, Implementation is the deck and opens all the way to
// its slides. Other authored places stay shut until something inside is active;
// empty places are omitted.
//
// Nothing about which feature is open is stored. It is a fact about the page
// being read, not a preference about the reader.

// appNavSources is everything the app-level list reads, already loaded.
type appNavSources struct {
	document      *saga.Saga
	requirements  requirements.Document
	page          *requirementsPageView
	prototypes    prototypes.Document
	prototypeNote string
	// quality holds the test cases each feature's Quality lists.
	quality quality.Document
	// decks is every projected deck row, implementation and onboarding.
	decks []*navNodeView
	// overviewActive says which overview row the page shows; see overviewNav.
	overviewActive string
	// pageFeature is the feature the page being read belongs to, and so the one
	// feature that opens over its four places. Empty on a page that belongs to
	// no feature, where every feature stays a row.
	pageFeature string
	// technical is the Technical design rows, one per definition.
	technical []*navNodeView
	// reviewSide says the page is on the Review side of the header, which
	// lists the reviews where Documentation lists the features.
	reviewSide bool
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

	// Everything that describes the whole app hangs off the overview, when it
	// actually exists.
	if personas := personaNav(sources.requirements); len(personas) > 0 {
		overview.Children = append(overview.Children, navSection("Personas", "/personas", "nav-personas", "story", personas))
	}
	if document.DesignSystem != nil {
		overview.Children = append(overview.Children, navSection("Design system", designSystemPath, "nav-designsystem", "design",
			onPage(designSystemPath, reportRootNav(document.DesignSystem))))
	}
	if len(onboarding) > 0 {
		overview.Children = append(overview.Children, navDeck("Onboarding", "nav-onboarding", "deck", onboarding))
	}
	if len(sources.technical) > 0 {
		overview.Children = append(overview.Children, navSection("Technical design", technicalPath, "nav-technical", "design", sources.technical))
	}
	if flags := flagNav(sources.requirements); len(flags) > 0 {
		overview.Children = append(overview.Children, navSection("Feature flags", "/flags", "nav-featureflags", "flag", flags))
	}

	// Each side lists what it is about, and the overview is on both because
	// it is what the application is: a reader reviewing a change needs the
	// same pitch, vocabulary, and personas a reader learning the app does.
	// Only the second section differs, which is the whole distinction the
	// header draws — Documentation lists the features, Review the reviews.
	// Both are in the sidebar, which is loaded once and kept as the reader
	// moves between pages; the other side's is hidden.
	features, reviews := makeFeaturesNav(sources, deckRows), makeReviewsNav(document)
	other := reviews
	if sources.reviewSide {
		other = features
		// The overview is here for reference, not to be read down: a reader
		// on the Review side came for the reviews, so those are what is open.
		// No page on this side is the overview or anything inside it, so it
		// is neither the current page nor holding one, and stays shut.
		overview.Expanded, overview.Active = false, false
	}
	other.Hidden, other.dormant = true, true
	clearActiveNav(other.Children)
	navigation := []*navNodeView{overview}
	for _, section := range []*navNodeView{features, reviews} {
		if len(section.Children) > 0 {
			navigation = append(navigation, section)
		}
	}
	for _, node := range navigation {
		revealActive(node)
	}
	alignIcons(navigation)
	return navigation
}

// alignIcons gives every ordinary navigation row a real icon. Slide rows carry
// thumbnails instead, so they intentionally remain iconless.
func alignIcons(nodes []*navNodeView) {
	for _, node := range nodes {
		if node.Icon == "" && node.Slide == nil {
			node.Icon = "list"
		}
		alignIcons(node.Children)
	}
}

// navSection is a section header that is also a destination. The row opens
// the section's own page and the twisty beside it discloses what the section
// holds; a header that only expanded made a reader click twice to reach a
// page that already existed. Callers omit sections that have no content.
func navSection(title, href, id, icon string, children []*navNodeView) *navNodeView {
	node := navPlace(title, id, icon, children)
	node.Href = href
	return node
}

// navDeck is a section whose page is a deck: the header opens it at its first
// slide, and the slides stay beneath it so any one of them is still one click
// away. A deck already knows where it starts, so a header that only expanded
// into a list of slides asked the reader a question they had no way to answer.
func navDeck(title, id, icon string, slides []*navNodeView) *navNodeView {
	node := navPlace(title, id, icon, slides)
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

// makeFeaturesNav is the Features section: the header opens the table of every
// feature, and beneath it every feature is a row of its own, in creation order. The
// feature the reader is inside opens over its authored places; the rest stay
// shut over theirs. An app with no features omits the section.
func makeFeaturesNav(sources appNavSources, deckRows map[string]*navNodeView) *navNodeView {
	document := sources.document
	section := navSection("Features", featuresIndexHref, "nav-features", "product", nil)
	if len(document.Features) == 0 {
		return section
	}
	for _, feature := range document.Features {
		node := makeFeatureNav(sources, feature, deckRows)
		if feature.ID != sources.pageFeature {
			// Every feature carries its places, so the sidebar serves every
			// page; only the page's own feature is open, and nothing inside
			// another one is the page being read.
			node.Expanded, node.dormant = false, true
			clearActiveNav(node.Children)
		}
		section.Children = append(section.Children, node)
	}
	section.Expanded = true
	return section
}

// makeReviewsNav is the Review side's second section: every review, one row
// each, in the order they were created. It reads the reviews the Saga already
// holds and opens none of them, so the sidebar costs no range resolution: a
// review's slides, decisions, and coverage all belong to its own page.
func makeReviewsNav(document *saga.Saga) *navNodeView {
	section := navSection("Reviews", reviewsIndexPath, "nav-reviews", "diff", nil)
	for _, review := range document.Reviews {
		section.Children = append(section.Children, &navNodeView{
			Title: reviewNavTitle(review), Href: reviewHref(review.ID),
			NodeID: "nav-review-" + domID(review.ID), Icon: "diff",
		})
	}
	if len(section.Children) == 0 {
		return section
	}
	section.Expanded = true
	return section
}

// reviewNavTitle names a review by its title, with the pull request it is the
// review of. A review is a pull request, so the number is how a reader
// recognizes it; a review missing both falls back to its ID rather than
// rendering an empty row.
func reviewNavTitle(review *saga.Review) string {
	title := strings.TrimSpace(review.Title)
	if title == "" {
		title = review.ID
	}
	if review.PullRequest != nil && review.PullRequest.Number != 0 {
		title += " #" + strconv.Itoa(review.PullRequest.Number)
	}
	return title
}

// makeFeatureNav is one feature: its own report content first, then today's four
// places filled from this feature's records only.
func makeFeatureNav(sources appNavSources, feature *saga.Feature, deckRows map[string]*navNodeView) *navNodeView {
	prefix := "nav-feature-" + domID(feature.ID)
	var prototypeRows []*navNodeView
	for _, row := range makePrototypeNav(sources.prototypes) {
		for _, prototype := range sources.prototypes.Prototypes {
			if prototype.Feature == feature.ID {
				if target, err := prototypes.PrototypeURN(sources.prototypes.SagaID, prototype.Identity.ID); err == nil && row.NodeID == "nav-"+domID(target) {
					prototypeRows = append(prototypeRows, row)
				}
			}
		}
	}
	var uxDecks, implementation []*navNodeView
	for _, deck := range feature.Decks {
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
	if feature.Design != nil {
		for _, child := range feature.Design.Children {
			if child.Kind == "chapter" {
				technical = append(technical, onFeaturePage(makeChapterNav(child), feature.ID))
			}
		}
	}
	places := makeProductNavTree(productNavSources{
		prefix:         prefix,
		feature:        feature.ID,
		requirements:   makeFeatureRequirementsNav(sources.page, feature.ID, prefix),
		prototypes:     prototypeRows,
		prototypeNote:  sources.prototypeNote,
		uxDecks:        uxDecks,
		technical:      technical,
		testCases:      testCaseNav(sources.quality, feature.ID),
		implementation: implementation,
	})
	// The feature row opens the feature's page; its places disclose beneath it.
	node := &navNodeView{Title: featureTitle(feature), Href: featureHref(feature.ID), NodeID: prefix, Icon: "product", Group: true, Expanded: true}
	var report []*navNodeView
	for _, row := range reportRootNav(feature.Report) {
		report = append(report, onFeaturePage(row, feature.ID))
	}
	node.Children = append(report, places...)
	return node
}

// onFeaturePage points a row's in-page anchors, and its outline's, at the feature's
// page, where the feature's own chapters are rendered.
func onFeaturePage(node *navNodeView, feature string) *navNodeView {
	if strings.HasPrefix(node.Href, "#") {
		node.Href = featureHref(feature) + node.Href
	}
	for _, child := range node.Children {
		onFeaturePage(child, feature)
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
		nodes = append(nodes, &navNodeView{Title: flag.Identity.ID, NodeID: "nav-" + domID(urn), Icon: "flag", Note: state})
	}
	return nodes
}

// ----- Which feature, and every feature -----

// featuresIndexHref is the browsable table of every feature, which the Features header
// opens and every feature row sits beneath.
const featuresIndexHref = "/features"

// featureTitle is what a feature's row says: the title an author gave it, or its ID
// while it has none.
func featureTitle(feature *saga.Feature) string {
	if title := strings.TrimSpace(feature.Title); title != "" {
		return title
	}
	return feature.ID
}

// featureLinkView is one feature as a title and a link to its page.
type featureLinkView struct {
	ID      string
	Title   string
	Href    string
	Current bool
}

// featureLinks names every feature in creation order, marking the one whose content
// is being read.
func featureLinks(document *saga.Saga, current string) []featureLinkView {
	links := make([]featureLinkView, 0, len(document.Features))
	for _, feature := range document.Features {
		links = append(links, featureLinkView{
			ID: feature.ID, Title: featureTitle(feature), Href: featureHref(feature.ID),
			Current: feature.ID == current,
		})
	}
	return links
}

// pageFeature names the feature of the page being read: a feature's own page, a story
// or criterion of one, or a test case of one. A chapter redirects to its
// feature's page before it reaches here, and a slide is read on that page too, so
// both arrive as "feature". A page that belongs to no feature names none, and then
// no feature opens.
func pageFeature(route appRoute, page *requirementsPageView, tests quality.Document) string {
	switch route.kind {
	case "feature":
		return route.id
	case "requirements":
		if page != nil && page.Story != nil {
			return page.Story.Feature
		}
	case "test":
		for _, testCase := range tests.TestCases {
			if testCase.Identity.ID == route.id {
				return testCase.Feature
			}
		}
	}
	return ""
}
