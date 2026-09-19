package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

var errRequirementNotFound = errors.New("requirement not found")

// requirementsPageView is a reviewer projection over the canonical v3
// requirements records. It deliberately owns no prose or identity of its own:
// every target, revision, lifecycle state, and criterion comes from
// ___requirements.
type requirementsPageView struct {
	Active           bool
	Overview bool
	// Groups is the overview's stories by epic, in epic order.
	Groups           []requirementGroupView
	Stories          []*requirementStoryView
	Story            *requirementStoryView
	FocusedCriterion *requirementCriterionView
	StoryCount       int
	CriterionCount   int
	AcceptedCount    int
	ProposedCount    int
}

type requirementStoryView struct {
	// Epic is the epic whose directory holds the story.
	Epic               string
	Number             int
	Label              string
	ID                 string
	Target             string
	DOMID              string
	Href               string
	Title              string
	Statement          string
	Priority           string
	CreatedAt          time.Time
	Lifecycle          string
	LifecycleReason    string
	LifecycleAt        time.Time
	Revision           string
	RevisionTarget     string
	RevisionAt         time.Time
	RevisionConflict   bool
	LifecycleConflict  bool
	RevisionHeadCount  int
	LifecycleHeadCount int
	Revisions          []requirementHistoryView
	LifecycleEvents    []requirementHistoryView
	Criteria           []*requirementCriterionView
	// Terms are the project vocabulary that names this story.
	Terms []termLinkView
	// EpicLink, Personas, Citations, and Trace are the story's declared
	// links, filled for the story a page shows.
	EpicLink  traceLink
	Personas  []traceLink
	Citations []citationView
	Trace     traceGroups
}

// requirementGroupView is one epic's stories on the requirements overview.
type requirementGroupView struct {
	Epic        traceLink
	Description string
	Stories     []*requirementStoryView
}

// citationView is one source a story cites.
type citationView struct {
	Title     string
	Kind      string
	Reference string
	Href      string
}

type requirementCriterionView struct {
	Number    int
	Label     string
	ID        string
	Target    string
	DOMID     string
	Href      string
	Statement string
	Selected  bool
	// Trace is what links to this criterion: its design, the slides that
	// explain it, and the tests that verify it.
	Trace traceGroups
}

type requirementHistoryView struct {
	ID        string
	Target    string
	State     string
	Reason    string
	CreatedAt time.Time
	Current   bool
	Parents   int
}

func loadRequirementsSurface(root, sagaID string, r *http.Request) (*requirementsPageView, *navNodeView, requirements.Document, error) {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return nil, nil, requirements.Document{}, err
	}
	page, nav, err := makeRequirementsSurface(document, requirementRoute{
		active:      isRequirementsPath(r.URL.Path),
		storyID:     r.PathValue("story"),
		criterionID: r.PathValue("criterion"),
	})
	return page, nav, document, err
}

func isRequirementsPath(value string) bool {
	return value == "/requirements" || strings.HasPrefix(value, "/requirements/")
}

type requirementRoute struct {
	active      bool
	storyID     string
	criterionID string
}

func makeRequirementsSurface(document requirements.Document, route requirementRoute) (*requirementsPageView, *navNodeView, error) {
	page := &requirementsPageView{Active: route.active, Overview: route.active && route.storyID == ""}
	stories := append([]requirements.Story(nil), document.Stories...)
	sort.SliceStable(stories, func(i, j int) bool {
		if stories[i].Identity.CreatedAt.Equal(stories[j].Identity.CreatedAt) {
			return stories[i].Identity.ID < stories[j].Identity.ID
		}
		return stories[i].Identity.CreatedAt.Before(stories[j].Identity.CreatedAt)
	})

	for index := range stories {
		story := stories[index]
		view, err := makeRequirementStoryView(document.SagaID, index+1, story)
		if err != nil {
			return nil, nil, err
		}
		page.Stories = append(page.Stories, view)
		page.CriterionCount += len(view.Criteria)
		switch view.Lifecycle {
		case "accepted":
			page.AcceptedCount++
		case "proposed":
			page.ProposedCount++
		}
		if route.storyID == view.ID {
			page.Story = view
		}
	}
	page.StoryCount = len(page.Stories)
	naming := document.TermsNaming()
	for _, view := range page.Stories {
		for _, urn := range naming[view.Target] {
			view.Terms = append(view.Terms, recordLink(document, urn))
		}
	}

	if route.active && route.storyID != "" && page.Story == nil {
		return nil, nil, errRequirementNotFound
	}
	if page.Story != nil && route.criterionID != "" {
		for _, criterion := range page.Story.Criteria {
			if criterion.ID == route.criterionID {
				criterion.Selected = true
				page.FocusedCriterion = criterion
				break
			}
		}
		if page.FocusedCriterion == nil {
			return nil, nil, errRequirementNotFound
		}
	}

	navigation := makeRequirementsNav(page)
	return page, navigation, nil
}

func makeRequirementStoryView(sagaID string, number int, story requirements.Story) (*requirementStoryView, error) {
	target, err := livingid.Story(sagaID, story.Identity.ID)
	if err != nil {
		return nil, err
	}
	view := &requirementStoryView{
		Epic: story.Epic, Number: number, Label: fmt.Sprintf("Story %02d", number), ID: story.Identity.ID,
		Target: target, DOMID: domID(target), Href: requirementStoryHref(story.Identity.ID),
		Title: story.Identity.ID, CreatedAt: story.Identity.CreatedAt, Lifecycle: "unresolved",
		RevisionConflict: story.RevisionConflict(), LifecycleConflict: story.LifecycleConflict(),
		RevisionHeadCount: len(story.RevisionHeads), LifecycleHeadCount: len(story.LifecycleHeads),
	}
	if story.CurrentRevision != nil {
		view.Title = story.CurrentRevision.Title
		view.Statement = story.CurrentRevision.Statement
		view.Priority = story.CurrentRevision.Priority
		view.Revision = story.CurrentRevision.ID
		view.RevisionAt = story.CurrentRevision.CreatedAt
		view.RevisionTarget, _ = livingid.Revision(sagaID, story.Identity.ID, story.CurrentRevision.ID)
		for index, criterion := range story.CurrentRevision.AcceptanceCriteria {
			target, err := livingid.Criterion(sagaID, story.Identity.ID, criterion.ID)
			if err != nil {
				return nil, err
			}
			view.Criteria = append(view.Criteria, &requirementCriterionView{
				Number: index + 1, Label: fmt.Sprintf("AC %02d", index+1), ID: criterion.ID,
				Target: target, DOMID: domID(target), Href: requirementCriterionHref(story.Identity.ID, criterion.ID),
				Statement: criterion.Statement,
			})
		}
	}
	if story.CurrentLifecycle != nil {
		view.Lifecycle = string(story.CurrentLifecycle.State)
		view.LifecycleReason = story.CurrentLifecycle.Reason
		view.LifecycleAt = story.CurrentLifecycle.CreatedAt
	}
	for _, revision := range story.Revisions {
		target, _ := livingid.Revision(sagaID, story.Identity.ID, revision.ID)
		view.Revisions = append(view.Revisions, requirementHistoryView{
			ID: revision.ID, Target: target, CreatedAt: revision.CreatedAt,
			Current: target == view.RevisionTarget, Parents: len(revision.Parents),
		})
	}
	for _, event := range story.Events {
		target, _ := requirements.StoryEventURN(sagaID, story.Identity.ID, event.ID)
		current := story.CurrentLifecycle != nil && story.CurrentLifecycle.ID == event.ID
		view.LifecycleEvents = append(view.LifecycleEvents, requirementHistoryView{
			ID: event.ID, Target: target, State: string(event.State), Reason: event.Reason,
			CreatedAt: event.CreatedAt, Current: current, Parents: len(event.Parents),
		})
	}
	sort.SliceStable(view.Revisions, func(i, j int) bool { return view.Revisions[i].CreatedAt.After(view.Revisions[j].CreatedAt) })
	sort.SliceStable(view.LifecycleEvents, func(i, j int) bool { return view.LifecycleEvents[i].CreatedAt.After(view.LifecycleEvents[j].CreatedAt) })
	return view, nil
}

func makeRequirementsNav(page *requirementsPageView) *navNodeView {
	return makeEpicRequirementsNav(page, "", "nav")
}

// makeEpicRequirementsNav lists one epic's stories, or every story when epic
// is empty. A story row reads as the story's title: an ordinal such as
// "Story 03" named nothing a reader could recognize.
func makeEpicRequirementsNav(page *requirementsPageView, epic, prefix string) *navNodeView {
	root := &navNodeView{
		Title: "Requirements", Href: "/requirements", NodeID: prefix + "-requirements",
		Icon: "requirements", Requirement: true, Active: page.Overview && epic == "", Expanded: page.Active,
	}
	for _, story := range page.Stories {
		if epic != "" && story.Epic != epic {
			continue
		}
		selectedStory := page.Story != nil && page.Story.ID == story.ID
		node := &navNodeView{
			Title: story.Title, Href: story.Href, NodeID: "nav-" + story.DOMID,
			Icon: "story", Requirement: true, Active: selectedStory && page.FocusedCriterion == nil,
			Expanded: selectedStory,
		}
		for _, criterion := range story.Criteria {
			node.Children = append(node.Children, &navNodeView{
				Title: criterion.Label + " · " + shortStatement(criterion.Statement, criterionNavRunes), Href: criterion.Href,
				NodeID: "nav-" + criterion.DOMID, Icon: "criterion", Requirement: true,
				Active: criterion.Selected,
			})
		}
		root.Children = append(root.Children, node)
	}
	return root
}

func requirementStoryHref(storyID string) string {
	return "/requirements/" + url.PathEscape(storyID)
}

func requirementCriterionHref(storyID, criterionID string) string {
	return requirementStoryHref(storyID) + "/criteria/" + url.PathEscape(criterionID)
}

func clearActiveNav(nodes []*navNodeView) {
	for _, node := range nodes {
		node.Active = false
		clearActiveNav(node.Children)
	}
}

// criterionNavRunes bounds a criterion's sidebar row. Titles wrap rather than
// truncate, so an unbounded statement became a paragraph in the sidebar; the
// criterion's page carries the whole statement.
const criterionNavRunes = 64

// shortStatement cuts statement at a word boundary within limit runes.
func shortStatement(statement string, limit int) string {
	runes := []rune(strings.TrimSpace(statement))
	if len(runes) <= limit {
		return string(runes)
	}
	cut := string(runes[:limit])
	if space := strings.LastIndex(cut, " "); space > limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}
