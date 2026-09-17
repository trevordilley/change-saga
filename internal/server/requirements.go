package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

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
	Overview         bool
	Stories          []*requirementStoryView
	Story            *requirementStoryView
	FocusedCriterion *requirementCriterionView
	StoryCount       int
	CriterionCount   int
	AcceptedCount    int
	ProposedCount    int
}

type requirementStoryView struct {
	Number            int
	Label             string
	ID                string
	Target            string
	DOMID             string
	Href              string
	Title             string
	Statement         string
	Priority          string
	Lifecycle         string
	Revision          string
	RevisionTarget    string
	RevisionConflict  bool
	LifecycleConflict bool
	Criteria          []*requirementCriterionView
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
}

func loadRequirementsSurface(root, sagaID string, r *http.Request) (*requirementsPageView, *navNodeView, error) {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return nil, nil, err
	}
	return makeRequirementsSurface(document, requirementRoute{
		active:      isRequirementsPath(r.URL.Path),
		storyID:     r.PathValue("story"),
		criterionID: r.PathValue("criterion"),
	})
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
		Number: number, Label: fmt.Sprintf("Story %02d", number), ID: story.Identity.ID,
		Target: target, DOMID: domID(target), Href: requirementStoryHref(story.Identity.ID),
		Title: story.Identity.ID, Lifecycle: "unresolved",
		RevisionConflict: story.RevisionConflict(), LifecycleConflict: story.LifecycleConflict(),
	}
	if story.CurrentRevision != nil {
		view.Title = story.CurrentRevision.Title
		view.Statement = story.CurrentRevision.Statement
		view.Priority = story.CurrentRevision.Priority
		view.Revision = story.CurrentRevision.ID
		view.RevisionTarget = "urn:change-saga:" + sagaID + ":story:" + story.Identity.ID + ":revision:" + story.CurrentRevision.ID
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
	}
	return view, nil
}

func makeRequirementsNav(page *requirementsPageView) *navNodeView {
	root := &navNodeView{
		Title: "Requirements", Href: "/requirements", NodeID: "nav-requirements",
		Icon: "requirements", Requirement: true, Expanded: page.Active,
	}
	root.Children = append(root.Children, &navNodeView{
		Title: "Overview", Href: "/requirements", NodeID: "nav-requirements-overview",
		Icon: "requirements-overview", Active: page.Overview,
	})
	for _, story := range page.Stories {
		selectedStory := page.Story != nil && page.Story.ID == story.ID
		node := &navNodeView{
			Title: story.Label + " · " + story.Title, Href: story.Href, NodeID: "nav-" + story.DOMID,
			Icon: "story", Requirement: true, Active: selectedStory && page.FocusedCriterion == nil,
			Expanded: selectedStory,
		}
		for _, criterion := range story.Criteria {
			node.Children = append(node.Children, &navNodeView{
				Title: criterion.Label + " · " + criterion.ID, Href: criterion.Href,
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
