package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Technical design is the overview's shared technical vocabulary: the Systems,
// Components, and data model that implementation and review decks reuse by
// identity. It is read as a small tree of pages, the same tree the sidebar
// draws:
//
//	/technical              a landing page naming the three areas
//	/technical/erd          the application ERD drawn, its other views, and
//	                        every data entity
//	/technical/systems      the directory of Systems
//	/technical/components   the directory of Components
//
// and one canonical page per definition, /technical/{kind}/{id}, which
// slides, drawers and reviews link to and which never moved. A definition
// sits beneath the area of its kind: data entities, ERDs and overlays beneath
// the ERD, Systems and Components beneath theirs. Every page is read-only
// documentation.
//
// A definition page renders an exact revision. With no revision named it shows
// the unique current revision; with competing heads it names every head and
// picks none. A saved pin opens that revision even when it is stale, retired,
// or conflicted, and says so, so following a slide's link never silently
// becomes a read of the latest definition.
//
// Proposal intent, lifecycle, and code currency are three separate facts. A
// definition authored before revisions recorded intent is unspecified, never
// inferred from its code or its age. Nothing here is a newness claim: "new"
// needs a named comparison, which this page does not have.

const technicalPath = "/technical"

// technicalArea is one of Technical design's three areas: its path segment,
// its title, and the definition kinds that sit beneath it.
type technicalArea struct {
	Segment, Title, Lede string
	Kinds                []string
}

var technicalAreas = []technicalArea{
	{Segment: "erd", Title: "ERD", Lede: "The application's data model: the authored ERD, its other views, and every data entity with its fields, holders and relationships.",
		Kinds: []string{requirements.KindDataEntity, requirements.KindERD, requirements.KindERDOverlay}},
	{Segment: "systems", Title: "Systems", Lede: "How Components interact and how data flows through them.", Kinds: []string{"system"}},
	{Segment: "components", Title: "Components", Lede: "Identifiable units of logic or transformation, reused by identity across decks.", Kinds: []string{"component"}},
}

// technicalAreaOf finds an area by its path segment, or by a definition kind
// beneath it.
func technicalAreaOf(segment, kind string) (technicalArea, bool) {
	for _, area := range technicalAreas {
		if area.Segment == segment && segment != "" {
			return area, true
		}
		for _, candidate := range area.Kinds {
			if candidate == kind && kind != "" {
				return area, true
			}
		}
	}
	return technicalArea{}, false
}

func (area technicalArea) Href() string { return technicalPath + "/" + area.Segment }

var errTechnicalNotFound = errors.New("technical definition not found")

// technicalHref is a definition's canonical page. An empty revision opens the
// current definition.
func technicalHref(kind, id, revision string) string {
	href := technicalPath + "/" + url.PathEscape(kind) + "/" + url.PathEscape(id)
	if revision != "" {
		href += "?" + url.Values{"revision": {revision}}.Encode()
	}
	return href
}

// technicalPinHref opens the page of an exact pin.
func technicalPinHref(pin saga.DocumentationLink) string {
	kind, id, ok := technicalTargetParts(pin.Target)
	if !ok {
		return ""
	}
	revision := strings.TrimPrefix(pin.Revision, pin.Target+":revision:")
	return technicalHref(kind, id, revision)
}

func technicalTargetParts(target string) (string, string, bool) {
	parts := strings.Split(target, ":")
	if len(parts) != 5 || parts[0] != "urn" || parts[1] != "change-saga" {
		return "", "", false
	}
	return parts[3], parts[4], true
}

// technicalIntent is a revision's explicit proposal intent. Revisions that
// predate intent report unspecified; the page never infers it.
func technicalIntent(revision *requirements.TechnicalRevision) string {
	return revision.EffectiveIntent()
}

// technicalLifecycle is the record's lifecycle, or conflicted when its heads
// compete.
func technicalLifecycle(record *requirements.TechnicalRecord) string {
	if record.CurrentLifecycle == nil {
		return "conflicted"
	}
	return record.CurrentLifecycle.State
}

// ----- Reverse usages -----

// Usages come from the shared reverse index of declared links, the same
// projection the CLI's queries read. An Item that pins a definition uses it
// directly; a System, holder, relationship or ERD that pins it is an owner;
// an Item that pins such an owner uses the definition through it, and says
// so. Counts are exact unless the index stopped early, which is stated.

// maxTechnicalUsages bounds the usages listed for one definition. The count is
// always exact when the walk completed; only the listing is cut.
const maxTechnicalUsages = 200

// technicalUsage is one declared use: where it is, the exact revision it
// pinned, and whether that pin is still current.
type technicalUsage struct {
	Target  string
	Role    string
	Pin     saga.DocumentationLink
	PinHref string
	Status  string
	// Context is "Implementation" or "Review" for an Item; Feature and Review
	// name the place that holds the deck.
	Context     string
	Feature     string
	FeatureHref string
	Review      string
	ReviewHref  string
	Deck        string
	Slide       string
	Item        string
	Description string
	// Href opens the slide at the Item.
	Href string
	// Owner is the definition whose revision declares the link, for owner
	// uses; Edge is its holder role or relationship.
	Owner, OwnerHref, Edge string
	OwnerCurrent           bool
	// Via names the owners an Item reaches this definition through.
	Via []traceLink
}

// technicalUsageIndex adapts the shared reverse index to the reviewer's
// pages: titles and hrefs come from the loaded document.
type technicalUsageIndex struct {
	index     *inventoryview.Index
	inventory requirements.Inventory
	document  *saga.Saga
	locations map[string]manifestTargetLocation
	items     map[string]*saga.Item
	// titles names every deck and slide by URN.
	titles map[string]string
}

func indexTechnicalUsages(document *saga.Saga, inventory requirements.Inventory) technicalUsageIndex {
	usages := technicalUsageIndex{index: inventoryview.Build(document, &inventory), inventory: inventory, document: document, locations: indexManifestTargets(document), items: map[string]*saga.Item{}, titles: map[string]string{}}
	var decks []*saga.Deck
	decks = append(decks, document.Decks...)
	for _, review := range document.Reviews {
		if review.Deck != nil {
			decks = append(decks, review.Deck)
		}
	}
	for _, deck := range decks {
		usages.titles[deck.Target] = deck.Title
		for _, slide := range deck.Slides {
			usages.titles[slide.Target] = slide.Title
			for _, item := range slide.Items {
				usages.items[item.Target] = item
			}
		}
	}
	return usages
}

// technicalUsagesView is one definition's usages, bounded for display.
type technicalUsagesView struct {
	Target string
	// Total counts every declared use found; Items those that are slide
	// Items, and Current the Items whose pin is current.
	Total, Items, Current int
	Usages                []technicalUsage
	Truncated             int
	// Incomplete says the walk stopped early, so the counts are lower bounds.
	Incomplete bool
	// DepthCut says owners exist beyond the one hop listed here.
	DepthCut bool
}

func (usages technicalUsageIndex) view(target string) technicalUsagesView {
	page := usages.index.Uses(target, inventoryview.UseOptions{Depth: 1, Limit: inventoryview.MaxLimit})
	view := technicalUsagesView{Target: target, Total: page.Total, Incomplete: page.Truncated || page.CycleCut, DepthCut: page.DepthCut}
	for _, use := range page.Uses {
		usage := usages.usage(use)
		if usage.Context != "" {
			view.Items++
			if use.Direct() && use.Status == "current" {
				view.Current++
			}
		}
		if len(view.Usages) < maxTechnicalUsages {
			view.Usages = append(view.Usages, usage)
		}
	}
	view.Truncated = page.Total - len(view.Usages)
	return view
}

// itemCount is the directory's "used by Items" count: direct Item pins.
func (usages technicalUsageIndex) itemCount(target string) (int, int) {
	page := usages.index.Uses(target, inventoryview.UseOptions{Limit: inventoryview.MaxLimit, Roles: []string{inventoryview.RoleImplementationItem, inventoryview.RoleReviewItem}})
	current := 0
	for _, use := range page.Uses {
		if use.Status == "current" {
			current++
		}
	}
	return page.Total, current
}

func (usages technicalUsageIndex) usage(use inventoryview.Use) technicalUsage {
	usage := technicalUsage{Target: use.Item, Role: use.Role, Pin: use.Pin, PinHref: technicalPinHref(use.Pin), Status: use.Status, Edge: use.Edge}
	for _, hop := range use.Path[1:] {
		usage.Via = append(usage.Via, traceLink{Title: entityName(usages.inventory, hop), Href: technicalPinHref(hop), Target: hop.Target})
	}
	switch use.Role {
	case inventoryview.RoleImplementationItem, inventoryview.RoleReviewItem:
		usage.Item = use.Label
		if usage.Item == "" {
			usage.Item = use.ItemID
		}
		if item := usages.items[use.Item]; item != nil {
			usage.Description = item.Description
		}
		usage.Deck, usage.Slide = usages.titles[use.Deck], usages.titles[use.Slide]
		if use.Role == inventoryview.RoleImplementationItem {
			usage.Context = "Implementation"
			if location, ok := usages.locations[use.Item]; ok {
				usage.Href = "/?view=slides" + location.Href
			}
			if id := lastSegment(use.Feature); id != "" {
				usage.Feature, usage.FeatureHref = id, featureHref(id)
				for _, feature := range usages.document.Features {
					if feature.ID == id {
						usage.Feature = featureTitle(feature)
					}
				}
			}
		} else {
			usage.Context = "Review"
			id := lastSegment(use.Review)
			usage.Review, usage.ReviewHref, usage.Href = id, reviewHref(id), reviewHref(id)+"#"+domID(use.Item)
			for _, review := range usages.document.Reviews {
				if review.ID == id {
					usage.Review = reviewNavTitle(review)
				}
			}
		}
	default:
		owner := saga.DocumentationLink{Target: use.Owner, Revision: use.OwnerRevision}
		usage.Owner, usage.OwnerHref, usage.OwnerCurrent = entityName(usages.inventory, owner), technicalPinHref(owner), use.OwnerCurrent
	}
	return usage
}

// lastSegment is the identity at the end of a URN.
func lastSegment(urn string) string {
	if urn == "" {
		return ""
	}
	return urn[strings.LastIndex(urn, ":")+1:]
}

// technicalRoleTitle says what kind of owner declares a link.
func technicalRoleTitle(role string) string {
	switch role {
	case inventoryview.RoleSystemMember:
		return "member of"
	case inventoryview.RoleDataHolder:
		return "holds data for"
	case inventoryview.RoleRelationship:
		return "related from"
	case inventoryview.RoleERDDirectory:
		return "listed in ERD"
	case inventoryview.RoleERDOverlay:
		return "pinned by ERD overlay"
	}
	return role
}

// ----- The Technical design pages -----

// technicalPageView is the landing page when Area is empty, and otherwise one
// area's page. Only the area being read is built.
type technicalPageView struct {
	Area technicalArea
	// Areas are the landing page's three areas, each with what it holds.
	Areas      []technicalAreaView
	Systems    *directoryView
	Components *directoryView
	// DataModel is the application's ERD, its other views, and every data
	// entity.
	DataModel *technicalDataModelView
	Query     string
	// Newness is set only when comparing: what is new relative to the base.
	Newness *technicalNewness
}

// technicalAreaView is one area on the landing page. Anchor keeps the
// fragment the area had when the whole of Technical design was one page, so
// an old link to it still lands on the area.
type technicalAreaView struct {
	overviewPartView
	Segment, Anchor string
}

func technicalDirectory(id, title, lede, noun, nouns, command, action string) *directoryView {
	return &directoryView{
		ID: id, Title: title, Action: action, Lede: lede,
		Label: "Filter " + nouns, Noun: noun, Nouns: nouns,
		Columns: []directoryColumn{
			{Title: title[:len(title)-1]}, {Title: "Explanation", Wide: true},
			{Title: "Intent"}, {Title: "Lifecycle"}, {Title: "Revision"},
			{Title: "Code references", Numeric: true}, {Title: "Used by Items", Numeric: true},
		},
		Empty:   "No " + nouns + " yet.",
		Command: command,
	}
}

// technicalLanding names the three areas with what each holds. It counts
// records only; nothing resolves.
func technicalLanding(inventory requirements.Inventory) []technicalAreaView {
	systems, components, entities := technicalCount(inventory)
	erds, overlays, erdName := 0, 0, ""
	for _, record := range inventory.Records {
		switch record.Kind {
		case requirements.KindERD:
			erds++
			if record.CurrentRevision != nil && (erdName == "" || record.Identity.ID == "application") {
				erdName = record.CurrentRevision.Name
			}
		case requirements.KindERDOverlay:
			overlays++
		}
	}
	erdCount := plural(entities, "data entity", "data entities") + " · " + plural(erds, "ERD", "ERDs")
	if overlays > 0 {
		erdCount += " · " + plural(overlays, "overlay", "overlays")
	}
	erdNote := technicalAreas[0].Lede
	if erdName != "" {
		erdNote = erdName + ", drawn as authored, and every data entity with its fields, holders and relationships."
	}
	areas := []technicalAreaView{
		{overviewPartView: overviewPartView{Title: "ERD", Count: erdCount, Note: erdNote, Gap: entities+erds+overlays == 0}, Segment: "erd", Anchor: "technical-data-model"},
		{overviewPartView: overviewPartView{Title: "Systems", Count: plural(systems, "System", "Systems"), Note: technicalAreas[1].Lede, Gap: systems == 0}, Segment: "systems", Anchor: "technical-systems-section"},
		{overviewPartView: overviewPartView{Title: "Components", Count: plural(components, "Component", "Components"), Note: technicalAreas[2].Lede, Gap: components == 0}, Segment: "components", Anchor: "technical-components-section"},
	}
	for index := range areas {
		areas[index].Href = technicalPath + "/" + areas[index].Segment
	}
	if areas[0].Gap {
		areas[0].Note = "No data model yet. Run change-saga data-entity add to record a data entity."
	}
	if areas[1].Gap {
		areas[1].Note = "No Systems yet. Run change-saga system add to record how Components interact."
	}
	if areas[2].Gap {
		areas[2].Note = "No Components yet. Run change-saga component add to record a reusable Component."
	}
	return areas
}

func (a *app) technicalPage(inventory requirements.Inventory, usages technicalUsageIndex, newness *technicalNewness, segment, query string) *technicalPageView {
	view := &technicalPageView{Query: query, Newness: newness}
	if segment == "" {
		view.Areas = technicalLanding(inventory)
		return view
	}
	view.Area, _ = technicalAreaOf(segment, "")
	var directory *directoryView
	switch segment {
	case "erd":
		view.DataModel = a.technicalDataModel(inventory, usages, newness, query)
		return view
	case "systems":
		view.Systems = technicalDirectory("technical-systems", "Systems", view.Area.Lede, "System", "Systems", "change-saga system add", view.Area.Href())
		directory = view.Systems
	case "components":
		view.Components = technicalDirectory("technical-components", "Components", view.Area.Lede, "Component", "Components", "change-saga component add", view.Area.Href())
		directory = view.Components
	}
	if newness != nil {
		directory.Columns = append(directory.Columns, directoryColumn{Title: "Since " + newness.Against})
	}
	for index := range inventory.Records {
		record := &inventory.Records[index]
		if area, _ := technicalAreaOf("", record.Kind); area.Segment != segment || record.Kind == requirements.KindDataEntity {
			continue
		}
		name, explanation, codeCount := record.Identity.ID, "", 0
		intent := gapCell("unknown — competing revisions")
		revision := gapCell(strconv.Itoa(len(record.RevisionHeads)) + " competing revisions")
		if current := record.CurrentRevision; current != nil {
			name, explanation, codeCount = current.Name, current.Explanation, len(current.Code)
			intent = textCell(technicalIntent(current))
			if intent.Text == "unspecified" {
				intent = gapCell("unspecified")
			}
			revision = textCell(current.ID)
		}
		lifecycle := textCell(technicalLifecycle(record))
		if record.CurrentLifecycle == nil {
			lifecycle = gapCell("conflicted")
		}
		items, current := usages.itemCount(record.Target)
		usedCell := countCell(items)
		if items > current {
			usedCell.Note = strconv.Itoa(items-current) + " not current"
		}
		cells := []directoryCell{
			{Text: name, Href: technicalHref(record.Kind, record.Identity.ID, ""), Note: record.Identity.ID, Target: record.Target},
			textCell(summarise(explanation, 140)),
			intent, lifecycle, revision, countCell(codeCount), usedCell,
		}
		if newness != nil {
			cells = append(cells, newnessCell(newness, record))
		}
		directory.addRow(directoryRow{Key: record.Identity.ID, Cells: cells})
	}
	directory.apply(query)
	return view
}

// newnessCell states one identity's newness relative to the comparison.
func newnessCell(newness *technicalNewness, record *requirements.TechnicalRecord) directoryCell {
	if !newness.Known {
		return gapCell("unknown")
	}
	return textCell(newness.Of(record))
}

// technicalCount is the overview directory's count for Technical design.
func technicalCount(inventory requirements.Inventory) (int, int, int) {
	systems, components, entities := 0, 0, 0
	for _, record := range inventory.Records {
		switch record.Kind {
		case "system":
			systems++
		case "component":
			components++
		case requirements.KindDataEntity:
			entities++
		}
	}
	return systems, components, entities
}

// technicalOverviewPart is the overview directory's row for Technical design.
func technicalOverviewPart(inventory requirements.Inventory) overviewPartView {
	systems, components, entities := technicalCount(inventory)
	count := plural(systems, "System", "Systems") + " · " + plural(components, "Component", "Components")
	if entities > 0 {
		count += " · " + plural(entities, "data entity", "data entities")
	}
	part := overviewPartView{Title: "Technical design", Href: technicalPath, Count: count,
		Note: "The Systems, Components, and data model decks reuse by identity."}
	if systems+components+entities == 0 {
		part.Gap = true
		part.Note = "No technical definitions yet. Run change-saga component add to record a reusable Component."
	}
	return part
}

// technicalNav is the sidebar's Technical design rows: its three areas, each
// a section that opens its own page and discloses one row per definition
// beneath it. The ERD lists its data entities; its ERDs and overlays are
// reached from the ERD page, which is the row current while one is read. An
// area with nothing in it is omitted, as every empty section is.
func technicalNav(inventory requirements.Inventory) []*navNodeView {
	var nodes []*navNodeView
	for _, area := range technicalAreas {
		var rows []*navNodeView
		holds := false
		for _, kind := range area.Kinds {
			for _, record := range inventory.Records {
				if record.Kind != kind {
					continue
				}
				holds = true
				if kind == requirements.KindERD || kind == requirements.KindERDOverlay {
					continue
				}
				rows = append(rows, technicalNavRow(record))
			}
		}
		if !holds {
			continue
		}
		icon := map[string]string{"erd": "split", "systems": "design", "components": "implementation"}[area.Segment]
		nodes = append(nodes, navSection(area.Title, area.Href(), "nav-technical-"+area.Segment, icon, rows))
	}
	return nodes
}

// technicalNavRow is one definition, opening its canonical page.
func technicalNavRow(record requirements.TechnicalRecord) *navNodeView {
	title := record.Identity.ID
	if record.CurrentRevision != nil {
		title = record.CurrentRevision.Name
	}
	node := &navNodeView{Title: title, Href: technicalHref(record.Kind, record.Identity.ID, ""), NodeID: "nav-" + domID(record.Target), Icon: "implementation"}
	switch record.Kind {
	case "system":
		node.Icon = "design"
	case requirements.KindDataEntity:
		node.Icon = "square"
	}
	switch {
	case record.CurrentRevision == nil || record.CurrentLifecycle == nil:
		node.Gap, node.Note = true, "conflicted"
	case record.CurrentLifecycle.State == "retired":
		node.Note = "retired"
	}
	return node
}

// technicalNavPath is the sidebar row a technical page marks current: its
// own row, or, for a definition the sidebar lists no row for (an ERD or an
// overlay), the area it sits beneath.
func technicalNavPath(route appRoute, path string) string {
	if route.kind == "technical-entity" && (route.id == requirements.KindERD || route.id == requirements.KindERDOverlay) {
		if area, ok := technicalAreaOf("", route.id); ok {
			return area.Href()
		}
	}
	return path
}

// ----- One definition -----

type technicalEntityView struct {
	Kind, KindTitle, ID, Target string
	// Area is the part of Technical design the definition sits beneath.
	Area technicalArea
	Name string
	// Revision is the exact revision shown, and Requested whether a pin
	// named it rather than the current definition.
	Revision  string
	Requested bool
	Intent    string
	Lifecycle string
	// PinStatus is the shown revision's status as a pin: current, stale,
	// retired, or conflicted.
	PinStatus string
	// Heads are every competing revision head, listed when there is no
	// unique current revision.
	Heads         []technicalRevisionLink
	CurrentHref   string
	Definition    *documentationView
	ERD           *erdView
	Usages        technicalUsagesView
	History       []technicalRevisionLink
	LifecycleNote string
	// Newness is relative to the named comparison, when comparing.
	Newness, NewnessAgainst, NewnessReason string
}

type technicalRevisionLink struct {
	ID, Href string
	Current  bool
	Shown    bool
}

func (a *app) technicalEntityPage(ctx context.Context, inventory requirements.Inventory, usages technicalUsageIndex, kind, id, revisionID string) (*technicalEntityView, error) {
	target, err := requirements.TechnicalURN(inventory.SagaID, kind, id)
	if err != nil {
		return nil, errTechnicalNotFound
	}
	record := inventory.Find(target)
	if record == nil {
		return nil, errTechnicalNotFound
	}
	view := &technicalEntityView{Kind: kind, KindTitle: technicalKindTitle(kind), ID: id, Target: target, Name: id, Lifecycle: technicalLifecycle(record), Requested: revisionID != ""}
	view.Area, _ = technicalAreaOf("", kind)
	current := ""
	if record.CurrentRevision != nil {
		current = record.CurrentRevision.ID
		view.CurrentHref = technicalHref(kind, id, "")
	}
	if revisionID == "" {
		revisionID = current
	}
	for _, head := range record.RevisionHeads {
		headID := strings.TrimPrefix(head, target+":revision:")
		view.Heads = append(view.Heads, technicalRevisionLink{ID: headID, Href: technicalHref(kind, id, headID), Shown: headID == revisionID})
	}
	for _, revision := range record.Revisions {
		view.History = append(view.History, technicalRevisionLink{ID: revision.ID, Href: technicalHref(kind, id, revision.ID), Current: revision.ID == current, Shown: revision.ID == revisionID})
	}
	view.Usages = usages.view(target)
	if record.CurrentLifecycle != nil && record.CurrentLifecycle.Reason != "" {
		view.LifecycleNote = record.CurrentLifecycle.Reason
	}
	if revisionID == "" {
		// Competing heads and no pin: name them all and choose none.
		return view, nil
	}
	revision := record.Revision(target + ":revision:" + revisionID)
	if revision == nil {
		return nil, errTechnicalNotFound
	}
	pin := saga.DocumentationLink{Target: target, Revision: target + ":revision:" + revisionID}
	view.Revision, view.Name, view.Intent = revisionID, revision.Name, technicalIntent(revision)
	view.PinStatus = inventory.LinkStatus(pin)
	if kind == requirements.KindERD || kind == requirements.KindERDOverlay {
		view.Intent = ""
		view.ERD = a.makeERDView(inventory, record, revision)
		return view, nil
	}
	definition := a.documentationView(ctx, inventory, record, revision, pin)
	definition.OnPage = true
	view.Definition = &definition
	return view, nil
}

func technicalKindTitle(kind string) string {
	switch kind {
	case requirements.KindDataEntity:
		return "Data entity"
	case requirements.KindERD:
		return "ERD"
	case requirements.KindERDOverlay:
		return "ERD overlay"
	}
	return strings.ToUpper(kind[:1]) + kind[1:]
}

// technicalShell serves the pages' routes inside the app shell.
func (a *app) technicalShell(ctx context.Context, document *saga.Saga, route appRoute, query url.Values) (*technicalPageView, *technicalEntityView, error) {
	inventory, err := requirements.LoadInventory(a.root, document.Manifest.ID)
	if err != nil {
		return nil, nil, err
	}
	if route.kind == "technical" {
		// The landing page counts records; it resolves no usage or newness.
		return a.technicalPage(inventory, technicalUsageIndex{}, nil, "", ""), nil, nil
	}
	usages := indexTechnicalUsages(document, inventory)
	newness := a.comparisonNewness(ctx, document.Manifest)
	if route.kind == "technical-area" {
		return a.technicalPage(inventory, usages, newness, route.id, query.Get("q")), nil, nil
	}
	entity, err := a.technicalEntityPage(ctx, inventory, usages, route.id, route.sub, query.Get("revision"))
	if entity != nil && newness != nil {
		entity.NewnessAgainst, entity.NewnessReason = newness.Against, newness.Reason
		entity.Newness = newness.Of(inventory.Find(entity.Target))
	}
	return nil, entity, err
}

// technicalUsagesPage is the drawer's lazily loaded usage list for one
// definition: every Item that pins it, at whichever revision it pinned.
func (a *app) technicalUsagesPage(w http.ResponseWriter, r *http.Request) {
	document := a.narrativeDocument(r.Context())
	if document == nil {
		http.Error(w, "The saga could not be loaded. Run change-saga validate for details.", http.StatusInternalServerError)
		return
	}
	target := r.URL.Query().Get("target")
	if _, _, ok := technicalTargetParts(target); !ok || !strings.HasPrefix(target, "urn:change-saga:"+document.Manifest.ID+":") {
		http.Error(w, "Invalid definition reference", http.StatusBadRequest)
		return
	}
	inventory, err := requirements.LoadInventory(a.root, document.Manifest.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if inventory.Find(target) == nil {
		http.Error(w, "The definition is missing.", http.StatusNotFound)
		return
	}
	writeIncrementalHeaders(w, "text/html; charset=utf-8")
	renderHTML(w, a.template, "technical-usages", indexTechnicalUsages(document, inventory).view(target), "The usages could not be rendered.")
}

const technicalTemplates = `
{{define "technical-usages"}}<div class="technical-usages" data-technical-usages="{{.Target}}">{{if .Usages}}<p class="technical-usage-count">{{if .Incomplete}}At least {{end}}{{.Items}} {{if eq .Items 1}}slide Item uses{{else}}slide Items use{{end}} this definition{{if lt .Current .Items}}; {{.Current}} pin its current revision directly{{end}}. {{.Total}} declared {{if eq .Total 1}}use{{else}}uses{{end}} in all{{if .Incomplete}}; the reverse index stopped early, so these are lower bounds{{end}}.</p><ul class="trace-links technical-usage-list">{{range .Usages}}<li data-technical-usage="{{if .Target}}{{.Target}}{{else}}{{.Owner}}{{end}}" data-usage-role="{{.Role}}" data-usage-status="{{.Status}}">{{if .Context}}{{if .Href}}<a href="{{.Href}}" data-usage-item="{{.Target}}">{{.Item}}</a>{{else}}<span>{{.Item}}</span>{{end}} <small class="trace-kind">{{.Context}} Item</small><small class="trace-note">{{if .Feature}}<a href="{{.FeatureHref}}">{{.Feature}}</a> · {{end}}{{if .Review}}<a href="{{.ReviewHref}}">{{.Review}}</a> · {{end}}{{.Deck}} · {{.Slide}}</small>{{else}}<small class="trace-kind">{{roleTitle .Role}}</small> <a href="{{.OwnerHref}}">{{.Owner}}</a>{{if .Edge}} <small class="trace-note">{{.Edge}}</small>{{end}}{{if not .OwnerCurrent}} <small class="gap">an earlier revision of it</small>{{end}}{{end}}<small class="technical-pin">pins <a href="{{.PinHref}}"><code>{{.Pin.Revision}}</code></a>{{if ne .Status "current"}} · <span class="gap">{{.Status}}</span>{{end}}</small>{{if .Via}}<small class="technical-via">through {{range $i, $v := .Via}}{{if $i}} → {{end}}<a href="{{$v.Href}}">{{$v.Title}}</a>{{end}}</small>{{end}}{{if .Description}}<p class="trace-rationale">{{.Description}}</p>{{end}}</li>{{end}}</ul>{{if .Truncated}}<p class="gap" role="status">{{.Truncated}} more uses are not listed here.</p>{{end}}{{if .DepthCut}}<p class="term-empty">Owners further up are not listed here; open an owner to follow it.</p>{{end}}{{else}}<p class="term-empty">{{if .Incomplete}}The reverse index stopped before finding a use; this is unknown, not unused.{{else}}No declared use: no slide Item, System, holder, relationship or ERD pins this definition.{{end}}</p>{{end}}</div>{{end}}

{{define "technical-newness"}}{{with .}}<p class="technical-newness" data-technical-newness="{{.Known}}">Compared against <code>{{.Against}}</code>: “new” means the identity did not exist at the comparison's base{{if not .Known}}. The base inventory is unknown: {{.Reason}}{{end}}.</p>{{end}}{{end}}

{{define "technical-page"}}{{if .Area.Segment}}{{template "technical-area-page" .}}{{else}}<section class="app-page technical-page" data-technical-page="landing"><nav class="requirements-breadcrumbs" aria-label="Technical design breadcrumb"><a href="/">Overview</a><span>/</span><strong>Technical design</strong></nav><header class="page-heading"><h1>Technical design</h1><p class="app-lede">The application's shared technical vocabulary: Systems, Components, and the data model that implementation and review decks reuse by identity. Intent, lifecycle, and code currency are separate facts; none of them is a review approval.</p></header>
<ul class="overview-parts technical-areas" aria-label="Technical design areas">{{range .Areas}}<li class="overview-part{{if .Gap}} gap{{end}}" id="{{.Anchor}}" data-technical-area="{{.Segment}}"><a class="overview-part-link" href="{{.Href}}">{{.Title}}</a><span class="overview-part-count">{{.Count}}</span><p class="overview-part-note">{{.Note}}</p></li>{{end}}</ul>
</section>{{end}}{{end}}

{{define "technical-area-page"}}<section class="app-page technical-page technical-area-page{{if .DataModel}} technical-erd-page{{end}}" data-technical-page="{{.Area.Segment}}"><nav class="requirements-breadcrumbs" aria-label="{{.Area.Title}} breadcrumb"><a href="/">Overview</a><span>/</span><a href="/technical">Technical design</a><span>/</span><strong>{{.Area.Title}}</strong></nav><header class="page-heading"><p class="app-page-kind">Technical design</p><h1>{{.Area.Title}}</h1>{{template "technical-newness" .Newness}}<p class="app-lede">{{.Area.Lede}}</p></header>
{{with .Systems}}<div data-technical-kind="system">{{template "directory" .}}</div>{{end}}{{with .Components}}<div data-technical-kind="component">{{template "directory" .}}</div>{{end}}
{{with .DataModel}}<div data-technical-data-model>{{with .Primary}}<section class="app-page-section" data-technical-primary-erd><h2>{{.Name}}</h2><p class="app-lede">{{.Explanation}}</p><p class="documentation-page-link"><a href="/technical/erd/{{.ID}}">Revision {{.Revision}}, its history and where it is used</a></p>{{template "erd-view" .}}</section>{{else}}<p class="app-empty">No ERD is authored yet. The data entities below remain readable without one.</p>{{end}}{{if .Views}}<section class="app-page-section" data-technical-erd-views><h2>{{if .Primary}}Other ERD views{{else}}ERD views{{end}}</h2><p class="term-empty">Other authored ERDs, and overlays proposing changes over an exact baseline revision.</p>{{template "trace-links" .Views}}</section>{{end}}<section class="app-page-section" data-technical-entities><h2>All data entities</h2>{{template "directory" .Entities}}</section></div>{{end}}
</section>{{end}}

{{define "technical-entity-page"}}<section class="app-page technical-entity-page{{if .ERD}} technical-erd-page{{end}}" data-technical-entity="{{.Target}}"{{if .Revision}} data-technical-revision="{{.Revision}}"{{end}}><nav class="requirements-breadcrumbs" aria-label="Definition breadcrumb"><a href="/">Overview</a><span>/</span><a href="/technical">Technical design</a><span>/</span>{{with .Area.Segment}}<a href="/technical/{{.}}">{{$.Area.Title}}</a><span>/</span>{{end}}<strong>{{.Name}}</strong></nav>
<header class="requirement-story-hero"><div><p class="app-page-kind">{{.KindTitle}}</p><h1>{{.Name}}</h1><p class="technical-facts"><span data-technical-fact="revision">Revision <code>{{if .Revision}}{{.Revision}}{{else}}none chosen{{end}}</code></span>{{if not .ERD}}<span data-technical-fact="intent">Intent: {{if eq .Intent "unspecified"}}<span class="gap">unspecified</span>{{else if .Intent}}{{.Intent}}{{else}}<span class="gap">unknown</span>{{end}}</span>{{end}}{{if .NewnessAgainst}}<span data-technical-fact="newness" data-newness="{{.Newness}}">Since {{.NewnessAgainst}}: {{if eq .Newness "unknown"}}<span class="gap" title="{{.NewnessReason}}">unknown</span>{{else}}{{.Newness}}{{end}}</span>{{end}}<span data-technical-fact="lifecycle">Lifecycle: {{if eq .Lifecycle "conflicted"}}<span class="gap">conflicted</span>{{else}}{{.Lifecycle}}{{end}}</span></p>
{{if and .Revision (ne .PinStatus "current")}}<p role="status" class="gap" data-technical-pin-status="{{.PinStatus}}">{{if .Requested}}You are reading saved revision <code>{{.Revision}}</code>. It is {{.PinStatus}}; nothing has been repinned.{{else}}This definition is {{.PinStatus}}.{{end}}{{if and .CurrentHref .Requested}} <a href="{{.CurrentHref}}">Read the current definition</a>{{end}}</p>{{end}}
{{if gt (len .Heads) 1}}<div role="status" class="gap" data-technical-conflict><p>This definition has {{len .Heads}} competing revisions. None is chosen as current; reconciling them names every head.</p><ul>{{range .Heads}}<li><a href="{{.Href}}"{{if .Shown}} aria-current="page"{{end}}><code>{{.ID}}</code></a></li>{{end}}</ul></div>{{end}}
{{if .LifecycleNote}}<p class="term-state">{{.LifecycleNote}}</p>{{end}}</div></header>
{{with .Definition}}{{template "documentation" .}}{{end}}{{with .ERD}}<p class="documentation-prose">{{.Explanation}}</p>{{template "erd-view" .}}{{end}}
<section class="app-page-section" data-technical-used-by><h2>Used by</h2>{{template "technical-usages" .Usages}}</section>
<section class="app-page-section" data-technical-history><h2>Revision history</h2><ul class="technical-history">{{range .History}}<li><a href="{{.Href}}"{{if .Shown}} aria-current="page"{{end}}><code>{{.ID}}</code></a>{{if .Current}} <small>current</small>{{end}}{{if .Shown}} <small>shown</small>{{end}}</li>{{end}}</ul><p class="term-empty">Opening another revision reads it only; it changes no slide's pin.</p></section>
</section>{{end}}
`

const technicalStyles = `
.technical-entity-page .documentation-explanation{max-width:100%;overflow-x:auto}
.technical-page .directory-table{min-width:640px}
.technical-page .directory-filter{flex-wrap:wrap}
.technical-areas{margin-top:8px}
.technical-areas .overview-part:target{outline:2px solid var(--accent);outline-offset:2px}
.technical-facts{display:flex;flex-wrap:wrap;gap:4px 18px;margin:6px 0 0;color:var(--muted);font:13px var(--ui)}
.technical-facts code{color:var(--ink)}
.technical-usage-list small{display:inline-block;margin-right:8px}
.technical-pin code{font-size:11px}
.technical-history{list-style:none;padding:0;display:flex;flex-wrap:wrap;gap:6px 16px}
.technical-entity-page [data-technical-conflict] ul{margin:4px 0 0}
.technical-entity-page .documentation-members{list-style:none}
.documentation-page-link{margin:4px 0 12px;font:500 13px var(--ui)}
`
