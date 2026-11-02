package server

import (
	"fmt"
	"html"
	"html/template"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// plansIndexPath is the Plans side's own page: what the team intends to do.
const plansIndexPath = "/plans"

func planHref(id string) string { return plansIndexPath + "/" + url.PathEscape(id) }

func taskHref(planID, taskID string) string {
	return planHref(planID) + "/tasks/" + url.PathEscape(taskID)
}

// plansDirectory is the Plans page's table. A delivered Plan is set aside the
// way a merged review is: hidden until a reader searches or asks for it.
func plansDirectory(document *saga.Saga, stories planStories, query string, showDelivered bool) *directoryView {
	view := &directoryView{
		ID: "plans", Title: "Plans", Action: plansIndexPath,
		Label: "Filter plans", Noun: "plan", Nouns: "plans",
		ArchivedNoun: "delivered", ShowArchived: showDelivered,
		Columns: []directoryColumn{
			{Title: "Plan", Wide: true}, {Title: "Touches"}, {Title: "Tasks done"}, {Title: "Reviews"}, {Title: "State"},
		},
		Empty:   "No plans yet.",
		Command: "change-saga plan create",
	}
	for _, plan := range document.Plans {
		done := 0
		for _, task := range plan.Tasks {
			if task.State == saga.TaskDone {
				done++
			}
		}
		state := "open"
		if plan.Delivered() {
			state = "delivered"
		}
		reviews := gapCell("none yet")
		if len(plan.Reviews) > 0 {
			reviews = textCell(strings.Join(plan.Reviews, ", "))
		}
		view.addRow(directoryRow{Key: plan.ID, Archived: plan.Delivered(), Cells: []directoryCell{
			{Text: plan.Title, Href: planHref(plan.ID), Note: plan.ID, Target: plan.Target},
			textCell(stories.touches(plan)),
			textCell(strconv.Itoa(done) + " of " + strconv.Itoa(len(plan.Tasks))),
			reviews,
			textCell(state),
		}})
	}
	view.apply(query)
	return view
}

// planStoryInfo is what a Plan's pages show of one user story.
type planStoryInfo struct {
	Target, Title, Href, Feature, FeatureTitle, FeatureHref string
	// Open says the story is not retired, so a Plan could still cover it.
	Open bool
}

// planStories resolves story URNs to what a reader needs. A Task whose story was
// removed still reads: its URN shows, and health reports it.
type planStories map[string]planStoryInfo

func loadPlanStories(root string, document *saga.Saga) planStories {
	stories := planStories{}
	records, err := requirements.Load(root, document.Manifest.ID)
	if err != nil {
		return stories
	}
	titles := map[string]string{}
	for _, feature := range document.Features {
		titles[feature.ID] = feature.Title
	}
	for _, story := range records.Stories {
		urn := "urn:change-saga:" + document.Manifest.ID + ":story:" + story.Identity.ID
		info := planStoryInfo{Target: urn, Title: story.Identity.ID, Href: "/requirements/" + story.Identity.ID, Feature: story.Feature, FeatureTitle: titles[story.Feature], FeatureHref: featureHref(story.Feature),
			Open: story.CurrentLifecycle == nil || story.CurrentLifecycle.State != requirements.StateRetired}
		if story.CurrentRevision != nil && story.CurrentRevision.Title != "" {
			info.Title = story.CurrentRevision.Title
		}
		stories[urn] = info
	}
	return stories
}

func (stories planStories) info(urn string) planStoryInfo {
	if info, found := stories[urn]; found {
		return info
	}
	return planStoryInfo{Target: urn, Title: urn}
}

// touches says how far a Plan reaches, worked out from its Tasks.
func (stories planStories) touches(plan *saga.Plan) string {
	named := plan.PlanStories()
	features := map[string]bool{}
	for _, urn := range named {
		if feature := stories.info(urn).Feature; feature != "" {
			features[feature] = true
		}
	}
	return fmt.Sprintf("%d %s in %d %s", len(named), pluralWord(len(named), "story", "stories"), len(features), pluralWord(len(features), "feature", "features"))
}

func pluralWord(count int, one, many string) string {
	if count == 1 {
		return one
	}
	return many
}

func (a *app) planIndex(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	stories := loadPlanStories(a.root, document)
	view := planIndexView{Directory: plansDirectory(document, stories, directoryQuery(r), directoryShowArchived(r))}
	a.inShell(w, r, "plan-index", view, reviewSurfaces{deckLabel: "Plans", title: "Plans"})
}

type planIndexView struct {
	Directory *directoryView
}

// taskStatus is how a Task reads: its state, or whether it could start now.
type taskStatus struct {
	Class, Word string
}

func statusOf(task *saga.Task, ready map[string]bool) taskStatus {
	switch {
	case task.State == saga.TaskDone:
		return taskStatus{"done", "Done"}
	case task.State == saga.TaskInProgress:
		return taskStatus{"working", "In progress"}
	case ready[task.ID]:
		return taskStatus{"ready", "Can start now"}
	}
	return taskStatus{"waiting", "Waiting"}
}

type planTaskLink struct {
	ID, Title, Href string
	Status          taskStatus
}

func planReady(plan *saga.Plan) map[string]bool {
	ready := map[string]bool{}
	for _, task := range saga.TasksReady(plan.Manifests()) {
		ready[task.ID] = true
	}
	return ready
}

func taskLink(plan *saga.Plan, task *saga.Task, ready map[string]bool) planTaskLink {
	return planTaskLink{ID: task.ID, Title: task.Title, Href: taskHref(plan.ID, task.ID), Status: statusOf(task, ready)}
}

type planPhaseTasks struct {
	Title string
	Tasks []planTaskRow
}

type planTaskRow struct {
	planTaskLink
	Summary  string
	Phase    string
	Story    planStoryInfo
	HasStory bool
	Sizing   string
	Unsized  bool
	Chain    []planTaskLink
	Depends  []planTaskLink
	Unblocks []planTaskLink
}

type planReviewSlide struct {
	Review, Slide, Href string
}

// planComparisonRow is one Task beside what the Reviews did about it. It only
// reports: a person judges whether the work matches.
type planComparisonRow struct {
	Task       planTaskLink
	Delivered  []planReviewSlide
	Departures []planDeparture
}

type planDeparture struct {
	Review, Slide, Label, Body, Href string
}

type planReviewLink struct {
	ID, Title, Href string
	Merged          bool
	Missing         bool
}

type planDeckSlide struct {
	Section      string
	Slide        *saga.Slide
	DOMID        string
	Front        overviewSlideView
	VisualURL    string
	Interactive  bool
	Paper        bool
	Tasks        []planTaskRow
	Done         int
	Stories      []planStoryInfo
	Callouts     []*saga.Item
	ElementNotes []elementNoteView
}

type planPageView struct {
	Plan       *saga.Plan
	Overview   *deckOverviewView
	Saga       *saga.Saga
	Slides     []planDeckSlide
	Phases     []planPhaseTasks
	Ready      []planTaskLink
	Unsized    []planTaskLink
	Done, All  int
	Touches    string
	Reviews    []planReviewLink
	Comparison []planComparisonRow
	Unplanned  []planReviewSlide
	Prototypes []planLink
	Coverage   *planCoverageView
}

type planLink struct {
	Title, Href string
}

// planTabView is one of a Plan's views in the header. They are separate pages,
// so each is a link rather than one of the shell's client-side views.
type planTabView struct {
	Title, Href, Icon string
	Current           bool
}

func planTabs(planID, current string) []planTabView {
	base := planHref(planID)
	return []planTabView{
		{Title: "Deck", Href: base, Icon: "list", Current: current == "deck"},
		{Title: "Tasks", Href: base + "/tasks", Icon: "split", Current: current == "tasks"},
		{Title: "Coverage", Href: base + "/coverage", Icon: "check", Current: current == "coverage"},
	}
}

// planPageFor loads what every view of a Plan shows in its heading.
func (a *app) planPageFor(w http.ResponseWriter, r *http.Request) (*planPageView, planStories, map[string]bool) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return nil, nil, nil
	}
	plan := document.FindPlan(r.PathValue("id"))
	if plan == nil || plan.Deck == nil {
		http.NotFound(w, r)
		return nil, nil, nil
	}
	stories := loadPlanStories(a.root, document)
	ready := planReady(plan)
	view := &planPageView{Plan: plan, Saga: document, All: len(plan.Tasks), Touches: stories.touches(plan), Overview: planOverview(plan)}
	for _, task := range plan.Tasks {
		if task.State == saga.TaskDone {
			view.Done++
		}
		if ready[task.ID] {
			view.Ready = append(view.Ready, taskLink(plan, task, ready))
		}
		if task.Sizing == nil || !task.Sizing.WithinTwoDays {
			view.Unsized = append(view.Unsized, taskLink(plan, task, ready))
		}
	}
	return view, stories, ready
}

// planPage is a Plan's essay and deck, in the viewer a review's deck uses.
func (a *app) planPage(w http.ResponseWriter, r *http.Request) {
	view, stories, ready := a.planPageFor(w, r)
	if view == nil {
		return
	}
	view.Slides = planSlides(view.Plan, stories, ready)
	for _, prototype := range view.Plan.Prototypes {
		id := prototype[strings.LastIndex(prototype, ":")+1:]
		view.Prototypes = append(view.Prototypes, planLink{Title: id, Href: "/prototypes/" + url.PathEscape(id)})
	}
	view.Reviews, _, _ = a.planComparison(r, view.Saga, view.Plan, ready)
	a.inShell(w, r, "plan-page", view, reviewSurfaces{deckLabel: "Plan", title: view.Plan.Title, reviewDeck: len(view.Slides) > 0, planTabs: planTabs(view.Plan.ID, "deck")})
}

// planTasksPage lists a Plan's Tasks by phase, and what its Reviews did of them.
func (a *app) planTasksPage(w http.ResponseWriter, r *http.Request) {
	view, stories, ready := a.planPageFor(w, r)
	if view == nil {
		return
	}
	view.Phases = planPhases(view.Plan, planTaskRows(view.Plan, stories, ready))
	view.Reviews, view.Comparison, view.Unplanned = a.planComparison(r, view.Saga, view.Plan, ready)
	a.inShell(w, r, "plan-tasks", view, reviewSurfaces{deckLabel: "Plan", title: view.Plan.Title + " tasks", planTabs: planTabs(view.Plan.ID, "tasks")})
}

// planCoveragePage is the whiteboard of a Plan's Tasks ending at its stories.
func (a *app) planCoveragePage(w http.ResponseWriter, r *http.Request) {
	view, stories, ready := a.planPageFor(w, r)
	if view == nil {
		return
	}
	view.Coverage = planCoverage(view.Plan, stories, ready)
	a.inShell(w, r, "plan-coverage", view, reviewSurfaces{deckLabel: "Plan", title: view.Plan.Title + " coverage", planTabs: planTabs(view.Plan.ID, "coverage")})
}

func planOverview(plan *saga.Plan) *deckOverviewView {
	overview := makeDeckOverview(plan.Deck)
	if overview == nil {
		return nil
	}
	for _, slide := range plan.Deck.Slides {
		if strings.HasPrefix(slide.MediaType, "image/") {
			asset := planHref(plan.ID) + "/visual/" + url.PathEscape(slide.ID)
			overview.Visuals[slide.ID], overview.Visuals[slide.Target] = asset, asset
		}
	}
	return overview
}

func planSlides(plan *saga.Plan, stories planStories, ready map[string]bool) []planDeckSlide {
	var sections deckSections
	rows := planTaskRows(plan, stories, ready)
	slides := make([]planDeckSlide, 0, len(plan.Deck.Slides))
	for _, slide := range plan.Deck.Slides {
		row := planDeckSlide{
			Section: sections.next(&slide.SlideManifest), Slide: slide, DOMID: domID(slide.Target),
			Front:       overviewSlideView{Title: slide.Title, Bullets: slide.FrontBullets()},
			VisualURL:   planHref(plan.ID) + "/visual/" + url.PathEscape(slide.ID),
			Interactive: slide.MediaType == "text/html" || slide.MediaType == "image/svg+xml",
			Paper:       visualPaper(slide.MediaType, slide.Directory, slide.Entrypoint),
		}
		if slide.MediaType == "image/svg+xml" {
			if data, err := os.ReadFile(filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))); err == nil {
				if aspect := svgAspectRatio(string(data)); aspect != "" {
					row.VisualURL += "?saga_aspect=" + aspect
				}
			}
		}
		row.ElementNotes = loadSlideNotes(slide.Directory, slide.Diagram, row.DOMID).unbound(map[string]bool{})
		for _, item := range slide.Items {
			if item.Kind == "callout" {
				row.Callouts = append(row.Callouts, item)
			}
			if item.Record == "" {
				continue
			}
			if task := plan.FindTask(item.Record); task != nil {
				row.Tasks = append(row.Tasks, rows[task.ID])
				if task.State == saga.TaskDone {
					row.Done++
				}
			} else if saga.PlanIDOf(item.Record) == "" {
				row.Stories = append(row.Stories, stories.info(item.Record))
			}
		}
		slides = append(slides, row)
	}
	return slides
}

// planTaskRows is everything a reader needs about each Task: where it stands,
// what it needs, what it unblocks, and how it reaches a story.
func planTaskRows(plan *saga.Plan, stories planStories, ready map[string]bool) map[string]planTaskRow {
	unblocks := saga.TaskUnblocks(plan.Manifests())
	titles := map[string]string{}
	for _, phase := range plan.Phases {
		titles[phase.ID] = phase.Title
	}
	rows := make(map[string]planTaskRow, len(plan.Tasks))
	for _, task := range plan.Tasks {
		value := planTaskRow{planTaskLink: taskLink(plan, task, ready), Summary: task.Summary, Phase: titles[task.Phase]}
		chain := taskStoryChain(plan, task)
		if last := chain[len(chain)-1]; last.Story != "" {
			value.Story, value.HasStory = stories.info(last.Story), true
		}
		for _, step := range chain[1:] {
			value.Chain = append(value.Chain, taskLink(plan, step, ready))
		}
		for _, id := range task.DependsOn {
			if dependency := plan.FindTask(id); dependency != nil {
				value.Depends = append(value.Depends, taskLink(plan, dependency, ready))
			}
		}
		for _, id := range unblocks[task.ID] {
			if dependent := plan.FindTask(id); dependent != nil {
				value.Unblocks = append(value.Unblocks, taskLink(plan, dependent, ready))
			}
		}
		switch {
		case task.Sizing == nil:
			value.Sizing, value.Unsized = "Not sized yet", true
		case task.Sizing.WithinTwoDays:
			value.Sizing = "2 days or less"
		default:
			value.Sizing, value.Unsized = "Judged larger", true
		}
		rows[task.ID] = value
	}
	return rows
}

// planPhases orders rows by phase. A Task with no phase is listed last.
func planPhases(plan *saga.Plan, rows map[string]planTaskRow) []planPhaseTasks {
	phases := []planPhaseTasks{}
	placed := map[string]bool{}
	for _, phase := range plan.Phases {
		group := planPhaseTasks{Title: phase.Title}
		for _, task := range plan.Tasks {
			if row, found := rows[task.ID]; found && task.Phase == phase.ID {
				group.Tasks = append(group.Tasks, row)
				placed[task.ID] = true
			}
		}
		phases = append(phases, group)
	}
	rest := planPhaseTasks{Title: "No phase"}
	for _, task := range plan.Tasks {
		if row, found := rows[task.ID]; found && !placed[task.ID] {
			rest.Tasks = append(rest.Tasks, row)
		}
	}
	if len(rest.Tasks) > 0 {
		phases = append(phases, rest)
	}
	return phases
}

func (a *app) planComparison(r *http.Request, document *saga.Saga, plan *saga.Plan, ready map[string]bool) ([]planReviewLink, []planComparisonRow, []planReviewSlide) {
	reviews := []planReviewLink{}
	rows := make([]planComparisonRow, 0, len(plan.Tasks))
	byTask := map[string]*planComparisonRow{}
	for _, task := range plan.Tasks {
		rows = append(rows, planComparisonRow{Task: taskLink(plan, task, ready)})
	}
	for index := range rows {
		byTask[plan.FindTask(rows[index].Task.ID).Target] = &rows[index]
	}
	unplanned := []planReviewSlide{}
	merged := a.mergedReviews(r.Context(), document, a.sagaFiles(r.Context()).fingerprint)
	for _, id := range plan.Reviews {
		review := document.FindReview(id)
		if review == nil {
			reviews = append(reviews, planReviewLink{ID: id, Title: id, Missing: true})
			continue
		}
		reviews = append(reviews, planReviewLink{ID: id, Title: reviewNavTitle(review), Href: reviewHref(id), Merged: merged[id] || review.Merged != nil})
		if review.Deck == nil {
			continue
		}
		for _, slide := range review.Deck.Slides {
			named := false
			for _, item := range slide.Items {
				row := byTask[item.Record]
				if row == nil {
					continue
				}
				named = true
				href := reviewHref(id) + "#" + domID(slide.Target)
				if item.Kind == "callout" {
					row.Departures = append(row.Departures, planDeparture{Review: id, Slide: slide.Title, Label: item.Label, Body: item.Body, Href: href})
				} else {
					row.Delivered = append(row.Delivered, planReviewSlide{Review: id, Slide: slide.Title, Href: href})
				}
			}
			if !named {
				unplanned = append(unplanned, planReviewSlide{Review: id, Slide: slide.Title, Href: reviewHref(id) + "#" + domID(slide.Target)})
			}
		}
	}
	return reviews, rows, unplanned
}

// planGraph draws a Plan's Tasks, one column per step of dependencies, so a
// column's Tasks could run at the same time. It is drawn on the server, so it
// reads with no script; each Task links to its page.
// planStoryNode is a story on the Coverage whiteboard: the Tasks it takes,
// counting every Task that unblocks one naming it.
type planStoryNode struct {
	Story     planStoryInfo
	DrawerID  string
	Done, All int
	Phases    []planPhaseTasks
}

// planCoverageGap is a feature this Plan reaches, and the stories no Task reaches.
type planCoverageGap struct {
	Feature, Href string
	Covered, All  int
	Missing       []planStoryInfo
	Stories       []planStoryNode
}

type planCoverageView struct {
	Board   template.HTML
	Tasks   []planTaskRow
	Stories []planStoryNode
	Gaps    []planCoverageGap
	Href    string
}

// wrapWords breaks a title into at most lines lines of about width characters.
func wrapWords(title string, width, lines int) []string {
	out := []string{}
	current := ""
	for _, word := range strings.Fields(title) {
		switch {
		case current == "":
			current = word
		case len([]rune(current))+1+len([]rune(word)) <= width:
			current += " " + word
		default:
			out = append(out, current)
			current = word
		}
	}
	if current != "" {
		out = append(out, current)
	}
	if len(out) > lines {
		out = out[:lines]
		out[lines-1] += "…"
	}
	return out
}

func planCoverage(plan *saga.Plan, stories planStories, ready map[string]bool) *planCoverageView {
	rows := planTaskRows(plan, stories, ready)
	view := &planCoverageView{Href: planHref(plan.ID) + "/coverage"}
	for _, task := range plan.Tasks {
		view.Tasks = append(view.Tasks, rows[task.ID])
	}
	// The Tasks a story takes are those naming it, and all they depend on.
	needs := map[string]map[string]bool{}
	var collect func(story string, task *saga.Task)
	collect = func(story string, task *saga.Task) {
		if needs[story][task.ID] {
			return
		}
		needs[story][task.ID] = true
		for _, id := range task.DependsOn {
			if dependency := plan.FindTask(id); dependency != nil {
				collect(story, dependency)
			}
		}
	}
	for _, task := range plan.Tasks {
		if task.Story != "" {
			if needs[task.Story] == nil {
				needs[task.Story] = map[string]bool{}
			}
			collect(task.Story, task)
		}
	}
	for _, urn := range plan.PlanStories() {
		info := stories.info(urn)
		node := planStoryNode{Story: info, DrawerID: "plan-node-story-" + domID(urn)}
		required := map[string]planTaskRow{}
		for id := range needs[urn] {
			required[id] = rows[id]
			node.All++
			if plan.FindTask(id).State == saga.TaskDone {
				node.Done++
			}
		}
		for _, phase := range planPhases(plan, required) {
			if len(phase.Tasks) > 0 {
				node.Phases = append(node.Phases, phase)
			}
		}
		view.Stories = append(view.Stories, node)
	}
	sort.SliceStable(view.Stories, func(i, j int) bool { return view.Stories[i].Story.Title < view.Stories[j].Story.Title })

	reached := map[string]bool{}
	for _, node := range view.Stories {
		reached[node.Story.Target] = true
	}
	gaps := map[string]*planCoverageGap{}
	for _, node := range view.Stories {
		if node.Story.Feature == "" {
			continue
		}
		if gaps[node.Story.Feature] == nil {
			gaps[node.Story.Feature] = &planCoverageGap{Feature: node.Story.FeatureTitle, Href: node.Story.FeatureHref}
		}
		gaps[node.Story.Feature].Stories = append(gaps[node.Story.Feature].Stories, node)
	}
	for _, info := range stories {
		gap := gaps[info.Feature]
		if gap == nil || !info.Open {
			continue
		}
		gap.All++
		if reached[info.Target] {
			gap.Covered++
		} else {
			gap.Missing = append(gap.Missing, info)
		}
	}
	for _, gap := range gaps {
		sort.Slice(gap.Missing, func(i, j int) bool { return gap.Missing[i].Title < gap.Missing[j].Title })
		view.Gaps = append(view.Gaps, *gap)
	}
	sort.Slice(view.Gaps, func(i, j int) bool { return view.Gaps[i].Feature < view.Gaps[j].Feature })
	view.Board = planBoard(plan, view.Stories, ready)
	return view
}

// planBoard draws the Tasks by phase, left to right, ending at the stories they
// lead to. Within a column, a node sits near what it depends on, so lines cross
// less. It is drawn here because its shape comes wholly from the records; nobody
// authors it.
func planBoard(plan *saga.Plan, stories []planStoryNode, ready map[string]bool) template.HTML {
	if len(plan.Tasks) == 0 {
		return ""
	}
	const (
		nodeWidth, nodeHeight = 232, 78
		gapX, gapY            = 96, 14
		margin, top           = 24, 74
		lineHeight            = 17
	)
	type column struct {
		title string
		tasks []*saga.Task
	}
	columns := []column{}
	placed := map[string]bool{}
	for _, phase := range plan.Phases {
		group := column{title: phase.Title}
		for _, task := range plan.Tasks {
			if task.Phase == phase.ID {
				group.tasks = append(group.tasks, task)
				placed[task.ID] = true
			}
		}
		if len(group.tasks) > 0 {
			columns = append(columns, group)
		}
	}
	rest := column{title: "No phase"}
	for _, task := range plan.Tasks {
		if !placed[task.ID] {
			rest.tasks = append(rest.tasks, task)
		}
	}
	if len(rest.tasks) > 0 {
		columns = append(columns, rest)
	}
	type point struct{ x, y int }
	positions := map[string]point{}
	slot := func(index int) int { return top + index*(nodeHeight+gapY) }
	average := func(ids []string) float64 {
		total, count := 0.0, 0
		for _, id := range ids {
			if position, found := positions[id]; found {
				total += float64(position.y)
				count++
			}
		}
		if count == 0 {
			return float64(top)
		}
		return total / float64(count)
	}
	tallest := len(stories)
	for x, group := range columns {
		sort.SliceStable(group.tasks, func(i, j int) bool { return average(group.tasks[i].DependsOn) < average(group.tasks[j].DependsOn) })
		tallest = max(tallest, len(group.tasks))
		for y, task := range group.tasks {
			positions[task.ID] = point{margin + x*(nodeWidth+gapX), slot(y)}
		}
	}
	namedBy := map[string][]string{}
	for _, task := range plan.Tasks {
		if task.Story != "" {
			namedBy[task.Story] = append(namedBy[task.Story], task.ID)
		}
	}
	ordered := slices.Clone(stories)
	sort.SliceStable(ordered, func(i, j int) bool {
		return average(namedBy[ordered[i].Story.Target]) < average(namedBy[ordered[j].Story.Target])
	})
	storyX := margin + len(columns)*(nodeWidth+gapX)
	storyY := map[string]int{}
	for y, node := range ordered {
		storyY[node.Story.Target] = slot(y)
	}
	width := storyX + nodeWidth + margin
	if len(ordered) == 0 {
		width = storyX
	}
	height := slot(tallest) + margin

	var out strings.Builder
	fmt.Fprintf(&out, `<svg class="plan-board" data-plan-board-drawing role="group" aria-label="Tasks by phase, ending at the stories they lead to" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg"><defs><marker id="plan-board-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L8 4L0 8z" class="plan-board-head"/></marker></defs>`, width, height)
	heading := func(x int, title string) {
		fmt.Fprintf(&out, `<rect class="plan-board-band" x="%d" y="%d" width="%d" height="%d" rx="12"/>`, x-12, 8, nodeWidth+24, height-16)
		for index, line := range wrapWords(title, 30, 2) {
			fmt.Fprintf(&out, `<text class="plan-board-phase" x="%d" y="%d">%s</text>`, x, 32+index*lineHeight, html.EscapeString(line))
		}
	}
	for x, group := range columns {
		heading(margin+x*(nodeWidth+gapX), group.title)
	}
	if len(ordered) > 0 {
		heading(storyX, "User stories")
	}
	edge := func(from, to string, start, end point) {
		startX, startY, endX, endY := start.x+nodeWidth, start.y+nodeHeight/2, end.x-2, end.y+nodeHeight/2
		middle := (startX + endX) / 2
		fmt.Fprintf(&out, `<path class="plan-board-edge" data-plan-edge-from="%s" data-plan-edge-to="%s" d="M%d %dC%d %d %d %d %d %d" marker-end="url(#plan-board-arrow)"/>`, html.EscapeString(from), html.EscapeString(to), startX, startY, middle, startY, middle, endY, endX, endY)
	}
	for _, task := range plan.Tasks {
		for _, dependency := range task.DependsOn {
			if start, found := positions[dependency]; found {
				edge("task:"+dependency, "task:"+task.ID, start, positions[task.ID])
			}
		}
		if task.Story != "" {
			edge("task:"+task.ID, "story:"+task.Story, positions[task.ID], point{storyX, storyY[task.Story]})
		}
	}
	node := func(id, drawer, drawerTitle, class, label, title, detail string, at point) {
		fmt.Fprintf(&out, `<g class="plan-board-node %s" data-plan-node="%s" data-plan-drawer="%s" data-plan-drawer-title="%s" tabindex="0" role="button" aria-label="%s" transform="translate(%d %d)"><title>%s</title><rect width="%d" height="%d" rx="9"/>`,
			class, html.EscapeString(id), html.EscapeString(drawer), html.EscapeString(drawerTitle), html.EscapeString(label), at.x, at.y, html.EscapeString(title), nodeWidth, nodeHeight)
		for index, line := range wrapWords(title, 30, 3) {
			fmt.Fprintf(&out, `<text class="plan-board-title" x="12" y="%d">%s</text>`, 21+index*lineHeight, html.EscapeString(line))
		}
		fmt.Fprintf(&out, `<text class="plan-board-state" x="12" y="%d">%s</text></g>`, nodeHeight-8, html.EscapeString(detail))
	}
	for _, task := range plan.Tasks {
		status := statusOf(task, ready)
		node("task:"+task.ID, "plan-node-task-"+task.ID, "Task", "task "+status.Class, task.Title+": "+status.Word, task.Title, status.Word, positions[task.ID])
	}
	for _, story := range ordered {
		detail := strconv.Itoa(story.Done) + " of " + strconv.Itoa(story.All) + " tasks done"
		class := "story"
		if story.All > 0 && story.Done == story.All {
			class += " done"
		}
		node("story:"+story.Story.Target, story.DrawerID, "User story", class, story.Story.Title+": "+detail, story.Story.Title, detail, point{storyX, storyY[story.Story.Target]})
	}
	out.WriteString(`</svg>`)
	return template.HTML(out.String())
}

type taskPageView struct {
	Plan      *saga.Plan
	Task      *saga.Task
	Status    taskStatus
	Phase     string
	Essay     template.HTML
	Story     planStoryInfo
	HasStory  bool
	Chain     []planTaskLink
	Depends   []planTaskLink
	Unblocks  []planTaskLink
	Slides    []planLink
	Code      []taskCodeRow
	Sizing    string
	Unsized   bool
	Reason    string
	DoneCmd   string
	Delivered bool
}

type taskCodeRow struct {
	Location, Note, Href string
}

// taskStoryChain is the shortest way a Task reaches a story: itself if it names
// one, else through the Tasks it unblocks.
func taskStoryChain(plan *saga.Plan, task *saga.Task) []*saga.Task {
	unblocks := saga.TaskUnblocks(plan.Manifests())
	type step struct {
		task *saga.Task
		path []*saga.Task
	}
	seen := map[string]bool{task.ID: true}
	queue := []step{{task, []*saga.Task{task}}}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.task.Story != "" {
			return current.path
		}
		for _, id := range unblocks[current.task.ID] {
			next := plan.FindTask(id)
			if next == nil || seen[id] {
				continue
			}
			seen[id] = true
			queue = append(queue, step{next, append(slices.Clone(current.path), next)})
		}
	}
	return []*saga.Task{task}
}

func (a *app) taskPage(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	plan := document.FindPlan(r.PathValue("id"))
	if plan == nil {
		http.NotFound(w, r)
		return
	}
	task := plan.FindTask(r.PathValue("task"))
	if task == nil {
		http.NotFound(w, r)
		return
	}
	stories := loadPlanStories(a.root, document)
	ready := planReady(plan)
	view := taskPageView{Plan: plan, Task: task, Status: statusOf(task, ready), Delivered: plan.Delivered(),
		DoneCmd: "change-saga task done --task " + task.Target + " " + filepath.Base(a.root)}
	for _, phase := range plan.Phases {
		if phase.ID == task.Phase {
			view.Phase = phase.Title
		}
	}
	if task.Essay != "" {
		view.Essay = markdown(task.Essay)
	}
	chain := taskStoryChain(plan, task)
	if last := chain[len(chain)-1]; last.Story != "" {
		view.Story, view.HasStory = stories.info(last.Story), true
	}
	for _, step := range chain[1:] {
		view.Chain = append(view.Chain, taskLink(plan, step, ready))
	}
	for _, id := range task.DependsOn {
		if dependency := plan.FindTask(id); dependency != nil {
			view.Depends = append(view.Depends, taskLink(plan, dependency, ready))
		}
	}
	for _, id := range saga.TaskUnblocks(plan.Manifests())[task.ID] {
		if dependent := plan.FindTask(id); dependent != nil {
			view.Unblocks = append(view.Unblocks, taskLink(plan, dependent, ready))
		}
	}
	if plan.Deck != nil {
		for _, slide := range plan.Deck.Slides {
			for _, item := range slide.Items {
				if item.Record == task.Target {
					view.Slides = append(view.Slides, planLink{Title: slide.Title, Href: planHref(plan.ID) + "#" + domID(slide.Target)})
					break
				}
			}
		}
	}
	for _, reference := range task.Code {
		view.Code = append(view.Code, taskCodeRow{Location: reference.Path + ":" + strconv.Itoa(reference.Start) + "-" + strconv.Itoa(reference.End), Note: reference.Note})
	}
	switch {
	case task.Sizing == nil:
		view.Sizing, view.Unsized = "Not sized yet", true
	case task.Sizing.WithinTwoDays:
		view.Sizing, view.Reason = "2 days or less", task.Sizing.Reason
	default:
		view.Sizing, view.Unsized, view.Reason = "Judged larger than 2 days", true, task.Sizing.Reason
	}
	a.inShell(w, r, "task-page", view, reviewSurfaces{deckLabel: "Plan", title: task.Title, planTabs: planTabs(plan.ID, "tasks")})
}

func (a *app) planVisual(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	plan := document.FindPlan(r.PathValue("id"))
	if plan == nil || plan.Deck == nil {
		http.NotFound(w, r)
		return
	}
	slide := plan.Slide(r.PathValue("slide"))
	if slide == nil {
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(slide.Directory, filepath.FromSlash(slide.Entrypoint))
	if filepath.Dir(path) != filepath.Clean(slide.Directory) {
		http.Error(w, "invalid slide visual", http.StatusBadRequest)
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Security-Policy", authoredContentPolicy)
	w.Header().Set("Cache-Control", "no-store")
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	serveSlideVisual(w, r, filepath.Base(path), contentType, info.ModTime(), file, a.sagaTheme())
}

// plansTouching lists the open and delivered Plans with a Task naming one of
// stories. Nobody authors this: it comes from the Tasks' own links.
func plansTouching(document *saga.Saga, stories map[string]bool) []featureLinkView {
	links := []featureLinkView{}
	for _, plan := range document.Plans {
		for _, urn := range plan.PlanStories() {
			if stories[urn] {
				links = append(links, featureLinkView{ID: plan.ID, Title: plan.Title, Href: planHref(plan.ID)})
				break
			}
		}
	}
	return links
}

// makePlansNav lists every open Plan, as the Reviews side lists open reviews.
// Delivered Plans are set aside, with one row that counts and opens them.
func makePlansNav(document *saga.Saga) *navNodeView {
	section := navSection("Plans", plansIndexPath, "nav-plans", "list", nil)
	setAside := 0
	for _, plan := range document.Plans {
		if plan.Delivered() {
			setAside++
			continue
		}
		section.Children = append(section.Children, &navNodeView{Title: plan.Title, Href: planHref(plan.ID), NodeID: "nav-plan-" + domID(plan.ID), Icon: "list"})
	}
	if setAside > 0 {
		section.Children = append(section.Children, &navNodeView{Title: strconv.Itoa(setAside) + " delivered", Href: plansIndexPath + "?archived=show", NodeID: "nav-plans-delivered", Icon: "list"})
	}
	if len(section.Children) > 0 {
		section.Expanded = true
	}
	return section
}

const planTemplateSource = `{{define "plan-index"}}<div class="review-surface plan-surface" data-plan-index><header class="review-top"><h1>Plans</h1><p>The work we intend to do. A Plan is one body of work, and each of its Tasks leads to a user story.</p></header>{{template "directory" .Directory}}</div>{{end}}
{{define "plan-head"}}<header class="plan-head"><p class="plan-eyebrow">Plan, not a review. Describes intended work</p><h1>{{.Plan.Title}}</h1><p class="plan-summary">{{if .Plan.Delivered}}<span class="plan-state done" data-plan-delivered>Delivered</span> {{end}}{{.Done}} of {{.All}} tasks done. Touches {{.Touches}}.</p></header>{{end}}
{{define "plan-task-state"}}<span class="plan-state {{.Class}}">{{.Word}}</span>{{end}}
{{define "plan-task-names"}}{{range $i,$t := .}}{{if $i}}, {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}}{{end}}
{{define "plan-task-card"}}<article class="plan-task-card" data-plan-task-row="{{.ID}}"><header><a class="plan-task-title" href="{{.Href}}">{{.Title}}</a>{{template "plan-task-state" .Status}}{{if .Unsized}}<span class="plan-state warn">{{.Sizing}}</span>{{end}}</header>{{if .Summary}}<p class="plan-note">{{.Summary}}</p>{{end}}<dl class="plan-task-chain" data-plan-chain>{{if .Depends}}<div><dt>Needs</dt><dd>{{template "plan-task-names" .Depends}}</dd></div>{{end}}{{if .Unblocks}}<div><dt>Unblocks</dt><dd>{{template "plan-task-names" .Unblocks}}</dd></div>{{end}}{{if .HasStory}}<div><dt>{{if .Chain}}Leads to{{else}}Story{{end}}</dt><dd><a href="{{.Story.Href}}">{{.Story.Title}}</a>{{if .Chain}} <span class="plan-note">through {{range $i,$t := .Chain}}{{if $i}} → {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}}</span>{{end}}</dd></div>{{end}}{{if .Phase}}<div data-plan-task-phase><dt>Phase</dt><dd>{{.Phase}}</dd></div>{{end}}</dl></article>{{end}}
{{define "plan-page"}}{{$page := .}}{{if not .Slides}}<div class="review-surface plan-surface" data-plan="{{.Plan.ID}}">{{template "plan-head" .}}<p class="app-empty">This Plan has no slides yet. Publish the first with <code>change-saga apply-slide --plan {{.Plan.ID}} --from SLIDE.json</code>.</p><section class="deck-viewer review-empty-viewer" data-deck-viewer aria-label="{{.Plan.Title}}"><div class="deck-viewer-canvas"><div class="deck-viewer-stage">{{template "deck-overview" .Overview}}</div></div><footer class="deck-viewer-header">{{template "deck-face-controls" .}}</footer></section></div>{{else}}<div class="review-deck-page" data-plan="{{.Plan.ID}}" data-plan-deck><nav class="review-deck-rail slide-side" aria-label="Plan slides"><div class="review-sidebar-header"><a class="sidebar-title" href="/">{{.Saga.Manifest.Title}}</a>{{template "side-links" "plans"}}</div><a class="review-deck-title" href="/plans">{{.Plan.Title}}</a><p class="review-deck-pr plan-rail-note">Plan, not a review. {{if .Plan.Delivered}}<span data-plan-delivered>Delivered.</span> {{end}}{{.Done}} of {{.All}} tasks done. Touches {{.Touches}}.</p>{{range .Reviews}}<p class="review-deck-pr" data-plan-review>Implemented by {{if .Href}}<a href="{{.Href}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}</p>{{end}}{{range .Prototypes}}<p class="review-deck-pr" data-plan-prototype>Prototype: <a href="{{.Href}}">{{.Title}}</a></p>{{end}}<section class="slide-rail-deck"><h2><a class="review-overview-link" href="#{{.Overview.DOMID}}">Plan deck</a></h2><ol class="review-thumbnail-list slide-thumbnail-list">{{range $index,$slide := .Slides}}<li>{{template "slide-section-divider" .Section}}<div class="slide-thumbnail-card{{if eq $index 0}} active{{end}}" data-slide-thumbnail-card><span class="slide-thumbnail-caption"><span class="slide-thumbnail-title">{{.Slide.Title}}</span></span><button type="button" class="slide-thumbnail-hit" data-slide-thumbnail data-slide-target="{{.Slide.Target}}" aria-current="{{if eq $index 0}}true{{else}}false{{end}}" aria-label="Show slide: {{.Slide.Title}}"></button></div></li>{{end}}</ol></section></nav><section class="deck-viewer review-deck-viewer" data-deck-viewer aria-label="{{.Plan.Title}}"><div class="deck-viewer-canvas"><div class="deck-viewer-stage">{{template "deck-overview" .Overview}}{{range $index,$slide := .Slides}}<section class="deck-viewer-slide review-deck-slide{{if eq $index 0}} active{{end}}" data-deck-slide data-deck-target="{{$page.Plan.Deck.Target}}" data-deck-role="plan" data-deck-title="{{$page.Plan.Title}}" data-slide-title="{{.Slide.Title}}" data-slide-target="{{.Slide.Target}}"{{if ne $index 0}} hidden{{end}}>{{template "slide-front" .Front}}<article class="fragment review-fragment" id="{{.DOMID}}" data-target="{{.Slide.Target}}" data-fragment-title="{{.Slide.Title}}" tabindex="0"><div class="fragment-head"><div class="fragment-actions">{{if .Tasks}}<button type="button" class="plan-slide-tasks-open" data-plan-drawer="plan-tasks-{{.DOMID}}" data-plan-drawer-title="Tasks on this slide" aria-label="Show this slide's {{len .Tasks}} tasks: {{.Done}} done" title="Tasks on this slide"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-list"></use></svg>{{.Done}} of {{len .Tasks}} tasks done</button>{{end}}<a class="permalink" href="#{{.DOMID}}" aria-label="Link to {{.Slide.Title}}" title="Copy link"><svg class="i" aria-hidden="true" focusable="false"><use href="#i-link"></use></svg></a></div></div>{{if .Tasks}}<template id="plan-tasks-{{.DOMID}}"><section class="plan-drawer" data-plan-slide-tasks><h2>{{.Slide.Title}}</h2>{{range .Tasks}}{{template "plan-task-card" .}}{{end}}{{if .Stories}}<h3>Stories</h3><ul>{{range .Stories}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{end}}</section></template>{{end}}{{template "element-note-targets" .ElementNotes}}<div class="fragment-stage">{{if .Interactive}}<iframe class="fragment-frame" data-fragment-frame sandbox="allow-scripts" src="{{.VisualURL}}" title="{{.Slide.Title}}" data-slide-visual{{if .Paper}} data-slide-paper{{end}}></iframe>{{else}}<img class="fragment-image" src="{{.VisualURL}}" alt="{{.Slide.Title}}" data-slide-visual{{if .Paper}} data-slide-paper{{end}}>{{end}}</div>{{if .Callouts}}<ul class="plan-callouts">{{range .Callouts}}<li><strong>{{.Label}}</strong> {{.Body}}</li>{{end}}</ul>{{end}}</article></section>{{end}}<nav class="deck-viewer-controls" aria-label="Slide navigation"><button type="button" class="slide-step" data-slide-previous aria-label="Previous slide" title="Previous slide">‹</button><button type="button" class="slide-step" data-slide-next aria-label="Next slide" title="Next slide">›</button></nav><button type="button" class="slide-exit-presentation" data-slide-exit-presentation hidden>Exit presentation</button></div></div><footer class="deck-viewer-header" aria-live="polite"><div><strong data-current-slide-title>{{(index .Slides 0).Slide.Title}}</strong></div><span data-slide-position></span>{{template "deck-face-controls" .}}</footer></section></div>{{end}}{{end}}
{{define "plan-tasks"}}<div class="review-surface plan-surface" data-plan="{{.Plan.ID}}" data-plan-tasks>{{template "plan-head" .}}<p class="plan-note">Tasks in a phase can run in parallel. The next phase needs what came before it.</p>{{if .Ready}}<p class="plan-callout" data-plan-ready><strong>{{len .Ready}} {{if eq (len .Ready) 1}}task{{else}}tasks{{end}} can start now:</strong> {{template "plan-task-names" .Ready}}.</p>{{end}}{{if .Unsized}}<p class="plan-callout" data-plan-unsized><strong>{{len .Unsized}} of {{.All}} tasks are not confidently two days or less:</strong> {{template "plan-task-names" .Unsized}}.</p>{{end}}{{range .Phases}}<section class="plan-phase" data-plan-phase><h2>{{.Title}}</h2>{{range .Tasks}}{{template "plan-task-card" .}}{{else}}<p class="plan-note">No tasks yet.</p>{{end}}</section>{{else}}<p class="app-empty">This Plan has no tasks yet. Add the first with <code>change-saga task add --plan {{.Plan.ID}}</code>.</p>{{end}}<section data-plan-comparison><h2>Plan vs Reviews</h2>{{if .Reviews}}<p class="plan-note">Implemented by {{range $i,$r := .Reviews}}{{if $i}}, {{end}}{{if .Href}}<a href="{{.Href}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}{{if .Merged}} (merged){{end}}{{if .Missing}} (missing){{end}}{{end}}. This reports differences. It never blocks a merge, and a person still judges whether the work matches.</p><table class="plan-table"><thead><tr><th>What we said</th><th>What we did</th></tr></thead><tbody>{{range .Comparison}}<tr data-plan-comparison-row="{{.Task.ID}}"><td><a href="{{.Task.Href}}">{{.Task.Title}}</a></td><td>{{range .Delivered}}<div><a href="{{.Href}}">{{.Slide}}</a> <span class="plan-note">{{.Review}}</span></div>{{end}}{{range .Departures}}<div class="plan-departure"><span class="plan-state working">Departed</span> <a href="{{.Href}}">{{.Label}}</a> <span class="plan-note">{{.Body}}</span></div>{{end}}{{if and (not .Delivered) (not .Departures)}}<span class="plan-state waiting">Not delivered</span>{{end}}</td></tr>{{end}}</tbody></table>{{if .Unplanned}}<h3>Not in the Plan</h3><ul class="plan-unplanned" data-plan-unplanned>{{range .Unplanned}}<li><a href="{{.Href}}">{{.Slide}}</a> <span class="plan-note">{{.Review}}</span></li>{{end}}</ul>{{end}}{{else}}<p class="plan-note">No Review implements this Plan yet. Link one with <code>change-saga plan link-review --plan {{.Plan.ID}} --review ID</code>.</p>{{end}}</section></div>{{end}}
{{define "plan-coverage"}}<div class="plan-coverage" data-plan="{{.Plan.ID}}" data-plan-coverage><header class="plan-coverage-head"><h1>{{.Plan.Title}}</h1><p class="plan-note">{{len .Coverage.Stories}} {{if eq (len .Coverage.Stories) 1}}story{{else}}stories{{end}} reached by {{.All}} {{if eq .All 1}}task{{else}}tasks{{end}}, {{.Done}} done.{{range .Coverage.Gaps}} <a href="{{.Href}}">{{.Feature}}</a>: {{.Covered}} of {{.All}} stories have tasks{{if .Missing}} <span class="gap" data-plan-uncovered>(none yet for {{range $i,$s := .Missing}}{{if $i}}, {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}})</span>{{end}}.{{end}} Select a task or story to see the path to it.</p><details class="plan-coverage-list" data-plan-stories><summary>Stories by feature</summary>{{range .Coverage.Gaps}}<h2><a href="{{.Href}}">{{.Feature}}</a></h2><ul>{{range .Stories}}<li><a href="{{.Story.Href}}">{{.Story.Title}}</a> <span class="plan-note">{{.Done}} of {{.All}} tasks done</span></li>{{end}}</ul>{{end}}</details></header>{{if .Coverage.Board}}<section class="whiteboard plan-whiteboard" data-whiteboard-view data-plan-board data-whiteboard-id="plan-{{.Plan.ID}}" data-whiteboard-url="{{.Coverage.Href}}" data-whiteboard-revision="" aria-label="{{.Plan.Title}} coverage"><div hidden data-whiteboard-metadata="[]" data-whiteboard-opening=""></div><div class="whiteboard-viewport" data-whiteboard-viewport tabindex="0" role="region" aria-label="Whiteboard. Scroll or drag to pan, Control or Command plus scroll to zoom. Plus and minus zoom, zero fits the board, arrow keys pan.">{{.Coverage.Board}}</div><header class="whiteboard-heading whiteboard-overlay plan-board-help"><details class="whiteboard-description"><summary>How to read this</summary><div class="whiteboard-description-body"><p>Each column is a phase: its tasks can run in parallel. Arrows point at what a task unblocks, and every path ends at a user story.</p><p>Select a node for its detail. The path to it is marked, and a dashed line shows where it leads.</p><p class="whiteboard-help">Scroll or drag to pan · Ctrl/Cmd + scroll or + / − to zoom · 0 to fit</p></div></details></header><div class="whiteboard-toolbar whiteboard-overlay" role="toolbar" aria-label="Whiteboard navigation" hidden data-whiteboard-toolbar><button type="button" data-whiteboard-zoom="-1" aria-label="Zoom out whiteboard" title="Zoom out (-)">−</button><output data-whiteboard-level aria-live="polite"></output><button type="button" data-whiteboard-zoom="1" aria-label="Zoom in whiteboard" title="Zoom in (+)">+</button><button type="button" data-whiteboard-replay hidden>Replay</button><button type="button" data-whiteboard-fields hidden aria-pressed="false">Details</button><button type="button" data-whiteboard-fit title="Fit board (0)">Fit board</button><label hidden><select data-whiteboard-focus><option value="">Whole board</option></select></label><button type="button" data-whiteboard-evidence hidden>Evidence</button><span data-whiteboard-mode hidden aria-live="polite">Full board</span></div></section>{{range .Coverage.Tasks}}<template id="plan-node-task-{{.ID}}"><section class="plan-drawer" data-plan-node-detail="{{.ID}}">{{template "plan-task-card" .}}<p><a href="{{.Href}}">Open this task</a></p></section></template>{{end}}{{range .Coverage.Stories}}<template id="{{.DrawerID}}"><section class="plan-drawer" data-plan-story-detail="{{.Story.Target}}"><h2><a href="{{.Story.Href}}">{{.Story.Title}}</a></h2><p class="plan-note">{{if .Story.FeatureTitle}}In <a href="{{.Story.FeatureHref}}">{{.Story.FeatureTitle}}</a>. {{end}}It takes these {{.All}} {{if eq .All 1}}task{{else}}tasks{{end}}, {{.Done}} done.</p>{{range .Phases}}<h3>{{.Title}}</h3>{{range .Tasks}}{{template "plan-task-card" .}}{{end}}{{end}}</section></template>{{end}}{{else}}<p class="app-empty">This Plan has no tasks yet. Add the first with <code>change-saga task add --plan {{.Plan.ID}}</code>.</p>{{end}}</div>{{end}}
{{define "task-page"}}<div class="review-surface plan-surface" data-task="{{.Task.ID}}"><header class="plan-head"><p class="plan-eyebrow">Task in <a href="{{planHref .Plan.ID}}">{{.Plan.Title}}</a>{{if .Phase}}, {{.Phase}}{{end}}</p><h1>{{.Task.Title}}</h1><p>{{template "plan-task-state" .Status}} <span class="plan-state {{if .Unsized}}warn{{else}}done{{end}}">{{.Sizing}}</span></p>{{if .Reason}}<p class="plan-note">{{.Reason}}</p>{{end}}</header><div class="plan-columns"><div>{{if .Essay}}<section data-task-essay><h2>Essay</h2><div class="markdown">{{.Essay}}</div></section>{{end}}{{if .Task.Summary}}<section><h2>What was done</h2><p>{{.Task.Summary}}</p></section>{{end}}{{if .Code}}<section data-task-code><h2>Code that matters</h2><ul class="plan-code">{{range .Code}}<li><code>{{.Location}}</code>{{if .Note}}<br><span class="plan-note">{{.Note}}</span>{{end}}</li>{{end}}</ul></section>{{end}}{{if not .Delivered}}{{if ne .Task.State "done"}}<section><h2>Mark it done</h2><code class="plan-command">{{.DoneCmd}}</code></section>{{end}}{{end}}</div><aside class="plan-side"><h3>User story</h3>{{if .HasStory}}<p data-task-story><a href="{{.Story.Href}}">{{.Story.Title}}</a>{{if .Story.FeatureTitle}} <span class="plan-note">in <a href="{{.Story.FeatureHref}}">{{.Story.FeatureTitle}}</a></span>{{end}}</p>{{if .Chain}}<p class="plan-note">No story of its own. It leads there by unblocking:</p><ul data-task-chain>{{range .Chain}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{end}}{{else}}<p class="plan-note">None.</p>{{end}}<h3>Depends on</h3>{{if .Depends}}<ul data-task-depends>{{range .Depends}}<li><a href="{{.Href}}">{{.Title}}</a> {{template "plan-task-state" .Status}}</li>{{end}}</ul>{{else}}<p class="plan-note">Nothing: it could start any time.</p>{{end}}<h3>Unblocks</h3>{{if .Unblocks}}<ul data-task-unblocks>{{range .Unblocks}}<li><a href="{{.Href}}">{{.Title}}</a> {{template "plan-task-state" .Status}}</li>{{end}}</ul>{{else}}<p class="plan-note">Nothing is waiting on it.</p>{{end}}{{if .Slides}}<h3>Slides that show it</h3><ul>{{range .Slides}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{end}}</aside></div></div>{{end}}
`

const planStyles = `
/* Plans: intended work, read beside Documentation and Reviews. */
.plan-surface{max-width:1120px;margin:0 auto;padding:24px}
.plan-head h1{margin:2px 0 6px}
.plan-eyebrow{margin:0;color:var(--muted);font:600 11px var(--ui);letter-spacing:.06em;text-transform:uppercase}
.plan-summary{color:var(--muted);margin:0 0 12px}
.plan-state{display:inline-block;padding:1px 8px;border-radius:10px;font-size:12px;font-weight:600;white-space:nowrap}
.plan-state.done{color:var(--review-approved-ink);background:var(--review-approved-bg)}
.plan-state.ready{color:var(--accent);background:var(--accent-soft)}
.plan-state.working{color:var(--review-stale-ink);background:var(--review-stale-bg)}
.plan-state.waiting{color:var(--review-none-ink);background:var(--review-none-bg)}
.plan-state.warn{color:var(--review-changes-ink);background:var(--review-changes-bg)}
.plan-note{color:var(--muted);font-size:13px}
.plan-callout{background:var(--accent-soft);border-radius:8px;padding:9px 13px;font-size:14px}
.plan-table{border-collapse:collapse;width:100%;margin-bottom:16px}
.plan-table th{text-align:left;font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:var(--muted);padding:6px 10px;border-bottom:1px solid var(--line)}
.plan-table td{padding:9px 10px;border-bottom:1px solid var(--line);vertical-align:top}
.view-tabs a.view-tab{text-decoration:none}
.plan-rail-note{margin-top:2px}
.plan-slide-tasks-open{display:inline-flex;align-items:center;gap:5px;background:var(--bg);color:var(--ink);border:1px solid var(--line);border-radius:6px;padding:3px 9px;font:600 12px var(--ui);cursor:pointer}
.plan-slide-tasks-open:hover{border-color:var(--accent)}
.plan-callouts{margin:12px 0 0;padding-left:18px;font-size:14px}
.plan-phase{margin:22px 0}
.plan-phase>h2{margin:0 0 8px;font-size:17px}
.plan-phase [data-plan-task-phase]{display:none}
[data-deck-role="plan"] .fragment-head{top:6px}
.review-deck-slide[data-deck-role="plan"] .fragment-head::before{content:'Plan slide'}
[data-plan-deck] [data-slide-viewed-controls]{display:none}
.shell:has(.plan-coverage)>.content{width:100%;max-width:none;padding:0}
.plan-board-help{top:12px;left:12px;padding:4px 12px}
.plan-whiteboard [data-whiteboard-toolbar] label{display:none}
[data-plan-story-detail] [data-plan-task-phase]{display:none}
.plan-coverage-list{margin-top:4px;font-size:13px}
.plan-coverage-list summary{cursor:pointer;color:var(--muted)}
.plan-coverage-list h2{margin:8px 0 2px;font-size:13px}
.plan-coverage-list ul{margin:0;padding-left:16px;max-height:30vh;overflow:auto}
.diff-drawer.plan-node-drawer{width:min(440px,92vw)}
body:has(.diff-drawer.open.plan-node-drawer) .drawer-backdrop{display:none}
.plan-task-card{border:1px solid var(--line);border-radius:9px;padding:10px 13px;margin-bottom:8px;background:var(--bg)}
.plan-task-card header{display:flex;flex-wrap:wrap;align-items:center;gap:8px}
.plan-task-title{font-weight:600;margin-right:auto}
.plan-task-card .plan-note{margin:5px 0 0}
.plan-task-chain{margin:6px 0 0;font-size:13px}
.plan-task-chain>div{display:flex;gap:8px;padding:1px 0}
.plan-task-chain dt{flex:0 0 66px;color:var(--muted)}
.plan-task-chain dd{margin:0;min-width:0}
.plan-drawer{padding:14px 16px;font:14px/1.5 var(--ui)}
.plan-drawer h2{margin:0 0 8px;font-size:16px}
.plan-drawer h3{margin:14px 0 6px;font-size:12px;letter-spacing:.04em;color:var(--muted)}
.plan-coverage{display:flex;flex-direction:column;height:calc(100dvh - var(--top,44px));min-height:420px}
.plan-coverage-head{padding:14px 22px 10px}
.plan-coverage-head h1{margin:0 0 2px;font-size:19px}
.plan-coverage-head .plan-note{margin:0}
.plan-whiteboard{position:relative;flex:1;min-height:0;isolation:isolate;overflow:hidden;border-top:1px solid var(--line)}
.plan-board{font:13px var(--ui)}
.plan-board-band{fill:var(--bg-subtle);stroke:var(--line-soft)}
.plan-board-phase{fill:var(--muted);font-size:13px;font-weight:600}
.plan-board-edge{fill:none;stroke:var(--faint);stroke-width:1.5}
.plan-board-head{fill:var(--faint)}
.plan-board-node{cursor:pointer}
.plan-board-node rect{fill:var(--review-none-bg);stroke:var(--line);stroke-width:1}
.plan-board-node.done rect{fill:var(--review-approved-bg);stroke:var(--review-approved-ink)}
.plan-board-node.ready rect{fill:var(--accent-soft);stroke:var(--accent);stroke-width:2.5}
.plan-board-node.working rect{fill:var(--review-stale-bg);stroke:var(--review-stale-ink)}
.plan-board-node.story rect{stroke-width:2;rx:18px}
.plan-board-node:focus-visible{outline:none}
.plan-board-node:focus-visible rect{stroke:var(--accent);stroke-width:3}
.plan-board-title{fill:var(--ink);font-weight:600}
.plan-board-state{fill:var(--muted);font-size:12px;font-weight:600}
.plan-board-node.done .plan-board-state{fill:var(--review-approved-ink)}
.plan-board-node.ready .plan-board-state{fill:var(--accent)}
.plan-board-node.working .plan-board-state{fill:var(--review-stale-ink)}
.plan-selecting .plan-board-node,.plan-selecting .plan-board-edge{opacity:.22}
.plan-selecting .plan-board-node.plan-path,.plan-selecting .plan-board-edge.plan-path,.plan-selecting .plan-board-node.plan-onward,.plan-selecting .plan-board-edge.plan-onward{opacity:1}
.plan-selecting .plan-board-edge.plan-path{stroke:var(--accent);stroke-width:2.5}
.plan-selecting .plan-board-edge.plan-onward{stroke:var(--muted);stroke-dasharray:5 4}
.plan-selecting .plan-board-node.plan-selected rect{stroke:var(--accent);stroke-width:3.5}
.plan-selecting .plan-board-node.plan-path:not(.plan-selected) rect{stroke:var(--accent);stroke-width:2}
.plan-columns{display:grid;grid-template-columns:minmax(0,1.5fr) minmax(0,1fr);gap:28px}
.plan-side{border-left:1px solid var(--line);padding-left:22px}
.plan-side h3{margin:14px 0 4px}
.plan-side ul{list-style:none;margin:0;padding:0}
.plan-side li{padding:3px 0}
.plan-code{list-style:none;padding:0}
.plan-code li{padding:6px 0;border-bottom:1px solid var(--line)}
.plan-command{display:block;background:var(--bg-inset);border-radius:6px;padding:8px 11px;overflow:auto;white-space:pre}
@media(max-width:820px){.plan-columns{grid-template-columns:1fr}.plan-side{border-left:0;padding-left:0}}
`

// planJavaScript opens a slide's Tasks or a whiteboard node's detail in the
// shared drawer, and marks a node's path on the Coverage whiteboard: everything
// that must come before it, and more faintly where it leads.
const planJavaScript = `
  function openPlanDrawer(opener) {
    const template = document.getElementById(opener.dataset.planDrawer);
    if (!template) return;
    restoreDrawerContent();
    q('.drawer-body')?.append(template.content.cloneNode(true));
    configureDrawer('reference', opener.dataset.planDrawerTitle || 'Plan');
    // A node's detail sits beside the board, so its path stays in view and the
    // next node can be selected without closing it first.
    q('.diff-drawer')?.classList.toggle('plan-node-drawer', opener.matches('[data-plan-node]'));
    showDrawer(opener);
  }
  function openPlanEssay(root) {
    const viewer = q('[data-plan-deck] [data-deck-viewer]', root);
    if (viewer && !location.hash) setDeckFace('overview', viewer);
  }
  let planPressed = null;
  document.addEventListener('pointerdown', event => { planPressed = {x: event.clientX, y: event.clientY}; }, true);
  function clearPlanPath(drawing) {
    drawing.classList.remove('plan-selecting');
    qa('.plan-path,.plan-onward,.plan-selected', drawing).forEach(element => element.classList.remove('plan-path', 'plan-onward', 'plan-selected'));
  }
  function markPlanPath(node) {
    const drawing = node.closest('[data-plan-board-drawing]');
    if (!drawing) return;
    clearPlanPath(drawing);
    const edges = qa('[data-plan-edge-from]', drawing);
    const walk = (start, here, there, mark) => {
      const seen = new Set([start]), queue = [start];
      while (queue.length) {
        const current = queue.shift();
        for (const edge of edges) {
          if (edge.dataset[here] !== current) continue;
          edge.classList.add(mark);
          const next = edge.dataset[there];
          if (!seen.has(next)) { seen.add(next); queue.push(next); }
        }
      }
      for (const id of seen) qa('[data-plan-node]', drawing).find(row => row.dataset.planNode === id)?.classList.add(mark);
    };
    walk(node.dataset.planNode, 'planEdgeFrom', 'planEdgeTo', 'plan-onward');
    walk(node.dataset.planNode, 'planEdgeTo', 'planEdgeFrom', 'plan-path');
    node.classList.remove('plan-onward');
    node.classList.add('plan-selected');
    drawing.classList.add('plan-selecting');
  }
  document.addEventListener('click', event => {
    const opener = event.target.closest?.('[data-plan-drawer]');
    if (opener) {
      event.preventDefault();
      if (opener.matches('[data-plan-node]')) markPlanPath(opener);
      openPlanDrawer(opener);
      return;
    }
    // Dragging the board to pan is not a click on its background.
    const drawing = event.target.closest?.('[data-plan-board-drawing]');
    if (drawing && planPressed && Math.hypot(event.clientX - planPressed.x, event.clientY - planPressed.y) < 5) {
      clearPlanPath(drawing);
      if (q('.diff-drawer.open.plan-node-drawer')) closeDrawer();
    }
  });
  document.addEventListener('keydown', event => {
    if (event.key !== 'Enter' && event.key !== ' ') return;
    const node = event.target.closest?.('[data-plan-node]');
    if (!node) return;
    event.preventDefault();
    markPlanPath(node);
    openPlanDrawer(node);
  });
`
