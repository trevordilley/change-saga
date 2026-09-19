package server

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// The app-level pages: a persona, an epic, and a test case. Each is
// documentation, read-only in both modes, and links only along declared
// relations; longer paths are inferred by following those links, never
// authored. Like status, the pages report what is and is not linked and never
// pass a verdict: a missing link is stated as growth, not as a failure.

var errAppPageNotFound = errors.New("app page not found")

func personaHref(id string) string  { return "/personas/" + url.PathEscape(id) }
func epicHref(id string) string     { return "/epics/" + url.PathEscape(id) }
func testCaseHref(id string) string { return "/tests/" + url.PathEscape(id) }

// appGraph is everything the app-level pages link through, loaded once per
// request.
type appGraph struct {
	document     *saga.Saga
	requirements requirements.Document
	quality      quality.Document
	locations    map[string]manifestTargetLocation
	// targetEpic names the epic whose design or report holds a target, so a
	// link to an epic's chapter opens that epic's page.
	targetEpic map[string]string
	// inbound and outbound are the active relations by endpoint.
	inbound  map[string][]requirements.Relation
	outbound map[string][]requirements.Relation
}

func newAppGraph(document *saga.Saga, records requirements.Document, tests quality.Document) *appGraph {
	graph := &appGraph{
		document: document, requirements: records, quality: tests,
		locations:  indexManifestTargets(document),
		targetEpic: map[string]string{},
		inbound:    map[string][]requirements.Relation{},
		outbound:   map[string][]requirements.Relation{},
	}
	for _, epic := range document.Epics {
		for _, root := range []*saga.Section{epic.Report, epic.Design} {
			if root != nil {
				graph.markEpic(root, epic.ID)
			}
		}
	}
	for _, relation := range records.Relations {
		if relation.State != requirements.RelationActive {
			continue
		}
		graph.inbound[relation.To] = append(graph.inbound[relation.To], relation)
		graph.outbound[relation.From] = append(graph.outbound[relation.From], relation)
	}
	return graph
}

func (graph *appGraph) markEpic(section *saga.Section, epic string) {
	if section.Target != "" {
		graph.targetEpic[section.Target] = epic
	}
	for _, fragment := range section.Fragments {
		graph.targetEpic[fragment.Target] = epic
		for _, landmark := range fragment.Landmarks {
			graph.targetEpic[landmark.Target] = epic
		}
	}
	for _, child := range section.Children {
		graph.markEpic(child, epic)
	}
}

// epicChapter reports the epic holding a chapter of the app-wide tree.
func (graph *appGraph) epicChapter(section *saga.Section) (string, bool) {
	epic, ok := graph.targetEpic[section.Target]
	return epic, ok
}

// traceLink is one linked record: what it is, where it opens, and, when the
// link is a relation, the relation's own rationale.
type traceLink struct {
	Kind      string
	Title     string
	Href      string
	Target    string
	Rationale string
	// Relation is the relation type that makes the link, e.g. "verifies".
	Relation string
	// Note is a fact about the linked record, such as a test's last run.
	Note string
}

// link names and places any record URN the reviewer has a page or anchor for.
func (graph *appGraph) link(urn string) traceLink {
	link := traceLink{Kind: "Record", Title: urn, Target: urn}
	parts := strings.Split(urn, ":")
	if len(parts) < 5 {
		return link
	}
	kind, id := parts[3], parts[4]
	switch kind {
	case "story":
		story := graph.requirements.FindStory(id)
		link.Kind, link.Href, link.Title = "Story", requirementStoryHref(id), id
		if story != nil && story.CurrentRevision != nil {
			link.Title = story.CurrentRevision.Title
		}
		if len(parts) == 7 && parts[5] == "criterion" {
			link.Kind, link.Href = "Acceptance criterion", requirementCriterionHref(id, parts[6])
			if story != nil && story.CurrentRevision != nil {
				for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
					if criterion.ID == parts[6] {
						link.Title = criterion.Statement
					}
				}
			}
		}
		return link
	case "test-case":
		link.Kind, link.Href, link.Title = "Test case", testCaseHref(id), id
		if testCase := graph.findTestCase(id); testCase != nil {
			if testCase.CurrentRevision != nil {
				link.Title = testCase.CurrentRevision.Title
			}
			link.Note = testCaseNote(*testCase)
		}
		return link
	case "persona":
		link.Kind, link.Href, link.Title = "Persona", personaHref(id), id
		if persona := graph.requirements.FindPersona(id); persona != nil && persona.CurrentRevision != nil {
			link.Title = persona.CurrentRevision.Name
		}
		return link
	case "epic":
		link.Kind, link.Href, link.Title = "Epic", epicHref(id), id
		if epic, ok := applayout.Find(graph.requirements.Epics, id); ok {
			link.Title = epic.Title
		}
		return link
	case "term":
		link.Kind, link.Href, link.Title = "Term", termHref(id), id
		if term := graph.requirements.FindTerm(id); term != nil && term.CurrentRevision != nil {
			link.Title = term.CurrentRevision.Name
		}
		return link
	case "flag":
		link.Kind, link.Title = "Feature flag", id
		return link
	}
	location, ok := graph.locations[urn]
	if !ok {
		return link
	}
	link.Kind, link.Title = location.Kind, location.Title
	switch location.Kind {
	case "Slide", "Deck", "Item":
		if location.Kind == "Item" && location.Chapter != "" {
			link.Note = location.Chapter
		}
		link.Href = "/?view=slides" + location.Href
	default:
		if location.Kind == "Chapter" || location.Kind == "Fragment" || location.Kind == "Section" {
			link.Kind = "Design"
		}
		if epic, ok := graph.targetEpic[urn]; ok {
			link.Href = epicHref(epic) + location.Href
		} else {
			link.Href = "/" + location.Href
		}
	}
	return link
}

func (graph *appGraph) findTestCase(id string) *quality.TestCase {
	for index := range graph.quality.TestCases {
		if graph.quality.TestCases[index].Identity.ID == id {
			return &graph.quality.TestCases[index]
		}
	}
	return nil
}

// testCaseNote states a test case's lifecycle and its last run as recorded.
func testCaseNote(testCase quality.TestCase) string {
	var facts []string
	if testCase.CurrentLifecycle != nil && testCase.CurrentLifecycle.State != quality.StateActive {
		facts = append(facts, string(testCase.CurrentLifecycle.State))
	}
	if run := testCase.CurrentRun; run != nil {
		facts = append(facts, "last run "+string(run.Result))
	} else {
		facts = append(facts, "no run recorded")
	}
	return strings.Join(facts, " · ")
}

// traceGroups sorts the relations pointing at a record into what a reader
// asks of it: its design, the slides that explain it, the tests that verify
// it, and anything else related.
type traceGroups struct {
	Design  []traceLink
	Slides  []traceLink
	Tests   []traceLink
	Related []traceLink
}

func (groups traceGroups) Empty() bool {
	return len(groups.Design)+len(groups.Slides)+len(groups.Tests)+len(groups.Related) == 0
}

func (graph *appGraph) traceTo(urn string) traceGroups {
	var groups traceGroups
	for _, relation := range graph.inbound[urn] {
		link := graph.link(relation.From)
		link.Rationale, link.Relation = relation.Rationale, string(relation.Type)
		switch {
		case relation.Type == requirements.RelationVerifies:
			groups.Tests = append(groups.Tests, link)
		case relation.Type == requirements.RelationAddresses:
			groups.Design = append(groups.Design, link)
		case relation.Type == requirements.RelationExplains || relation.Type == requirements.RelationImplements:
			groups.Slides = append(groups.Slides, link)
		default:
			link.Title = link.Title + " (" + strings.ReplaceAll(string(relation.Type), "_", " ") + ")"
			groups.Related = append(groups.Related, link)
		}
	}
	return groups
}

// ----- Persona -----

type personaPageView struct {
	ID          string
	Target      string
	Name        string
	Description string
	Retired     bool
	// Served are accepted stories that name the persona; Other are stories
	// that name it in any other state.
	Served []traceLink
	Other  []traceLink
	Terms  []termLinkView
}

func (graph *appGraph) personaPage(id string) (*personaPageView, error) {
	persona := graph.requirements.FindPersona(id)
	if persona == nil {
		return nil, errAppPageNotFound
	}
	urn, _ := requirements.PersonaURN(graph.requirements.SagaID, id)
	view := &personaPageView{ID: id, Target: urn, Name: id, Retired: !persona.Active()}
	if persona.CurrentRevision != nil {
		view.Name, view.Description = persona.CurrentRevision.Name, persona.CurrentRevision.Description
	}
	for _, story := range sortedStories(graph.requirements.Stories) {
		if story.CurrentRevision == nil || !contains(story.CurrentRevision.Personas, urn) {
			continue
		}
		storyURN, _ := livingid.Story(graph.requirements.SagaID, story.Identity.ID)
		link := graph.link(storyURN)
		if story.CurrentLifecycle != nil && story.CurrentLifecycle.State == requirements.StateAccepted {
			view.Served = append(view.Served, link)
			continue
		}
		link.Note = "unresolved"
		if story.CurrentLifecycle != nil {
			link.Note = string(story.CurrentLifecycle.State)
		}
		view.Other = append(view.Other, link)
	}
	for _, term := range graph.requirements.TermsNaming()[urn] {
		view.Terms = append(view.Terms, recordLink(graph.requirements, term))
	}
	return view, nil
}

func sortedStories(stories []requirements.Story) []requirements.Story {
	sorted := append([]requirements.Story(nil), stories...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Identity.CreatedAt.Equal(sorted[j].Identity.CreatedAt) {
			return sorted[i].Identity.ID < sorted[j].Identity.ID
		}
		return sorted[i].Identity.CreatedAt.Before(sorted[j].Identity.CreatedAt)
	})
	return sorted
}

// ----- Epic -----

type epicPageView struct {
	ID          string
	Target      string
	Title       string
	Description string
	Summary     epicSummaryView
	Stories     []epicStoryView
	// Report is the epic's own report content; Design its technical design.
	Report *sectionView
	Design *sectionView
	Tests  []traceLink
	Decks  []epicDeckView
}

// epicSummaryView counts what is linked, as status does: covered of total,
// never a score or a verdict.
type epicSummaryView struct {
	Stories           int
	Accepted          int
	StoriesWithDesign int
	Criteria          int
	CriteriaWithTests int
	TestCases         int
	Slides            int
	Items             int
	ItemsWithCode     int
}

type epicStoryView struct {
	traceLink
	Lifecycle string
	Personas  []traceLink
	Design    int
	Criteria  int
	Tested    int
}

type epicDeckView struct {
	Title  string
	Role   string
	Slides []traceLink
}

func (graph *appGraph) epicPage(id string) (*epicPageView, error) {
	var epic *saga.Epic
	for _, candidate := range graph.document.Epics {
		if candidate.ID == id {
			epic = candidate
		}
	}
	if epic == nil {
		return nil, errAppPageNotFound
	}
	view := &epicPageView{ID: epic.ID, Target: epic.Target, Title: epic.Title}
	if manifest, ok := applayout.Find(graph.requirements.Epics, id); ok {
		view.Description = manifest.Description
	}
	scope := viewScope{}.shell()
	if epic.Report != nil && (len(epic.Report.Fragments) > 0 || len(epic.Report.Children) > 0) {
		view.Report = makeSectionView(epic.Report, scope)
	}
	if epic.Design != nil && len(epic.Design.Children)+len(epic.Design.Fragments) > 0 {
		view.Design = makeSectionView(epic.Design, scope)
	}
	sagaID := graph.requirements.SagaID
	for _, story := range sortedStories(graph.requirements.Stories) {
		if story.Epic != id {
			continue
		}
		urn, _ := livingid.Story(sagaID, story.Identity.ID)
		row := epicStoryView{traceLink: graph.link(urn), Lifecycle: "unresolved"}
		if story.CurrentLifecycle != nil {
			row.Lifecycle = string(story.CurrentLifecycle.State)
		}
		view.Summary.Stories++
		if row.Lifecycle == string(requirements.StateAccepted) {
			view.Summary.Accepted++
		}
		design := len(graph.traceTo(urn).Design)
		if revision := story.CurrentRevision; revision != nil {
			for _, persona := range revision.Personas {
				row.Personas = append(row.Personas, graph.link(persona))
			}
			for _, criterion := range revision.AcceptanceCriteria {
				criterionURN, _ := livingid.Criterion(sagaID, story.Identity.ID, criterion.ID)
				trace := graph.traceTo(criterionURN)
				design += len(trace.Design)
				row.Criteria++
				if len(trace.Tests) > 0 {
					row.Tested++
				}
			}
		}
		row.Design = design
		if design > 0 {
			view.Summary.StoriesWithDesign++
		}
		view.Summary.Criteria += row.Criteria
		view.Summary.CriteriaWithTests += row.Tested
		view.Stories = append(view.Stories, row)
	}
	for _, testCase := range graph.quality.TestCases {
		if testCase.Epic != id {
			continue
		}
		urn, _ := qualityid.TestCase(sagaID, testCase.Identity.ID)
		view.Tests = append(view.Tests, graph.link(urn))
	}
	view.Summary.TestCases = len(view.Tests)
	for _, deck := range epic.Decks {
		deckView := epicDeckView{Title: deck.Title, Role: deck.Role}
		for _, slide := range deck.Slides {
			deckView.Slides = append(deckView.Slides, graph.link(slide.Target))
			view.Summary.Slides++
			for _, item := range slide.Items {
				view.Summary.Items++
				// The narrative load marks code without reading it.
				if item.HasCode || len(item.Code) > 0 {
					view.Summary.ItemsWithCode++
				}
			}
		}
		view.Decks = append(view.Decks, deckView)
	}
	return view, nil
}

// ----- Test case -----

type testCasePageView struct {
	ID             string
	Target         string
	Title          string
	Epic           traceLink
	Lifecycle      string
	Automation     string
	CoverageKinds  []string
	Preconditions  []string
	Steps          []quality.Step
	ExpectedResult string
	Conflict       bool
	Verifies       []traceLink
	Evidence       []testEvidenceView
	Runs           []testRunView
}

type testEvidenceView struct {
	Role string
	Code []*termCodeView
}

type testRunView struct {
	ID         string
	Result     string
	Summary    string
	Command    string
	Commit     string
	ExecutedAt time.Time
	Current    bool
}

func (a *app) testCasePage(ctx context.Context, graph *appGraph, id string) (*testCasePageView, error) {
	testCase := graph.findTestCase(id)
	if testCase == nil {
		return nil, errAppPageNotFound
	}
	urn, _ := qualityid.TestCase(graph.requirements.SagaID, id)
	view := &testCasePageView{ID: id, Target: urn, Title: id, Lifecycle: "unresolved",
		Conflict: testCase.RevisionConflict() || testCase.LifecycleConflict()}
	if epicURN := applayout.EpicURN(graph.requirements.SagaID, testCase.Epic); testCase.Epic != "" {
		view.Epic = graph.link(epicURN)
	}
	if testCase.CurrentLifecycle != nil {
		view.Lifecycle = string(testCase.CurrentLifecycle.State)
	}
	if revision := testCase.CurrentRevision; revision != nil {
		view.Title, view.Automation = revision.Title, string(revision.Automation)
		for _, kind := range revision.CoverageKinds {
			view.CoverageKinds = append(view.CoverageKinds, string(kind))
		}
		view.Preconditions, view.Steps, view.ExpectedResult = revision.Preconditions, revision.Steps, revision.ExpectedResult
	}
	for _, relation := range graph.outbound[urn] {
		link := graph.link(relation.To)
		link.Rationale, link.Relation = relation.Rationale, string(relation.Type)
		view.Verifies = append(view.Verifies, link)
	}
	for _, evidence := range testCase.Evidence {
		// Superseded evidence is history; the heads are what the test
		// case points at now.
		evidenceURN, _ := qualityid.Evidence(graph.requirements.SagaID, id, evidence.ID)
		if len(testCase.EvidenceHeads) > 0 && !contains(testCase.EvidenceHeads, evidenceURN) {
			continue
		}
		view.Evidence = append(view.Evidence, testEvidenceView{
			Role: strings.ReplaceAll(string(evidence.Role), "_", " "),
			Code: a.referenceCode(ctx, evidence.Code, "test case"),
		})
	}
	for _, run := range testCase.Runs {
		view.Runs = append(view.Runs, testRunView{
			ID: run.ID, Result: string(run.Result), Summary: run.Summary, Command: run.Command,
			Commit: run.Source.Commit, ExecutedAt: run.ExecutedAt,
			Current: testCase.CurrentRun != nil && testCase.CurrentRun.ID == run.ID,
		})
	}
	sort.SliceStable(view.Runs, func(i, j int) bool { return view.Runs[i].ExecutedAt.After(view.Runs[j].ExecutedAt) })
	return view, nil
}

// testCaseNav lists one epic's test cases for Quality > Test Cases.
func testCaseNav(document quality.Document, epic string) []*navNodeView {
	var nodes []*navNodeView
	for _, testCase := range document.TestCases {
		if testCase.Epic != epic {
			continue
		}
		urn, _ := qualityid.TestCase(document.SagaID, testCase.Identity.ID)
		title := testCase.Identity.ID
		if testCase.CurrentRevision != nil && strings.TrimSpace(testCase.CurrentRevision.Title) != "" {
			title = testCase.CurrentRevision.Title
		}
		node := &navNodeView{Title: title, Href: testCaseHref(testCase.Identity.ID), NodeID: "nav-" + domID(urn), Icon: "quality"}
		if testCase.CurrentLifecycle != nil && testCase.CurrentLifecycle.State != quality.StateActive {
			node.Note = string(testCase.CurrentLifecycle.State)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

// decorateRequirements adds the links a story page shows: the story's epic,
// personas, and citations, and what links to it and to each criterion.
func (graph *appGraph) decorateRequirements(page *requirementsPageView) {
	for _, view := range page.Stories {
		if view.Epic != "" {
			view.EpicLink = graph.link(applayout.EpicURN(graph.requirements.SagaID, view.Epic))
		}
	}
	view := page.Story
	if view == nil {
		return
	}
	if story := graph.requirements.FindStory(view.ID); story != nil && story.CurrentRevision != nil {
		for _, persona := range story.CurrentRevision.Personas {
			view.Personas = append(view.Personas, graph.link(persona))
		}
		for _, cited := range story.CurrentRevision.Citations {
			view.Citations = append(view.Citations, graph.citation(cited))
		}
	}
	view.Trace = graph.traceTo(view.Target)
	for _, criterion := range view.Criteria {
		criterion.Trace = graph.traceTo(criterion.Target)
	}
}

func (graph *appGraph) citation(urn string) citationView {
	view := citationView{Title: urn, Reference: urn}
	for _, citation := range graph.requirements.Citations {
		target, _ := livingid.Citation(graph.requirements.SagaID, citation.ID)
		if target != urn {
			continue
		}
		view = citationView{Title: citation.Title, Kind: strings.ReplaceAll(string(citation.Kind), "_", " "), Reference: citation.Reference}
		if strings.HasPrefix(citation.Reference, "https://") || strings.HasPrefix(citation.Reference, "http://") {
			view.Href = citation.Reference
		}
	}
	return view
}
