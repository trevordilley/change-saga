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
		info := planStoryInfo{Target: urn, Title: story.Identity.ID, Href: "/requirements/" + story.Identity.ID, Feature: story.Feature, FeatureTitle: titles[story.Feature], FeatureHref: featureHref(story.Feature)}
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
	Story     planStoryInfo
	HasStory  bool
	Sizing    string
	Unsized   bool
	Dependent int
}

type planSlideView struct {
	Section  string
	Slide    *saga.Slide
	DOMID    string
	Visual   string
	Tasks    []planTaskLink
	Stories  []planStoryInfo
	Callouts []*saga.Item
}

type planStoryGroup struct {
	Feature string
	Href    string
	Stories []planStoryRow
}

type planStoryRow struct {
	Story     planStoryInfo
	Done, All int
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

type planPageView struct {
	Plan       *saga.Plan
	Tab        string
	Overview   *deckOverviewView
	Slides     []planSlideView
	Phases     []planPhaseTasks
	Ready      []planTaskLink
	Graph      template.HTML
	Stories    []planStoryGroup
	Unsized    []planTaskLink
	Done, All  int
	Touches    string
	Reviews    []planReviewLink
	Comparison []planComparisonRow
	Unplanned  []planReviewSlide
	Prototypes []planLink
}

type planLink struct {
	Title, Href string
}

// planTabs are the Plan page's views, in order.
var planTabs = []struct{ ID, Title string }{
	{"deck", "Deck"}, {"dependencies", "Dependencies"}, {"tasks", "Tasks"}, {"stories", "Stories"}, {"reviews", "Plan vs Reviews"}, {"prototypes", "Prototypes"},
}

func (view planPageView) Tabs() []planLink {
	tabs := make([]planLink, 0, len(planTabs))
	for _, tab := range planTabs {
		href := planHref(view.Plan.ID)
		if tab.ID != "deck" {
			href += "?tab=" + tab.ID
		}
		tabs = append(tabs, planLink{Title: tab.Title, Href: href})
	}
	return tabs
}

// TabActive reports whether the titled tab is the one showing.
func (view planPageView) TabActive(title string) bool {
	for _, tab := range planTabs {
		if tab.Title == title {
			return tab.ID == view.Tab
		}
	}
	return false
}

func (a *app) planPage(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	plan := document.FindPlan(r.PathValue("id"))
	if plan == nil || plan.Deck == nil {
		http.NotFound(w, r)
		return
	}
	stories := loadPlanStories(a.root, document)
	ready := planReady(plan)
	view := planPageView{Plan: plan, Tab: "deck", All: len(plan.Tasks), Touches: stories.touches(plan), Overview: planOverview(plan)}
	if requested := r.URL.Query().Get("tab"); requested != "" {
		for _, tab := range planTabs {
			if tab.ID == requested {
				view.Tab = requested
			}
		}
	}
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
	view.Slides = planSlides(plan, stories, ready)
	view.Phases = planPhases(plan, stories, ready)
	view.Graph = planGraph(plan, ready)
	view.Stories = planStoryGroups(plan, stories)
	view.Reviews, view.Comparison, view.Unplanned = a.planComparison(r, document, plan, ready)
	for _, prototype := range plan.Prototypes {
		id := prototype[strings.LastIndex(prototype, ":")+1:]
		view.Prototypes = append(view.Prototypes, planLink{Title: id, Href: "/prototypes/" + url.PathEscape(id)})
	}
	a.inShell(w, r, "plan-page", view, reviewSurfaces{deckLabel: "Plan", title: plan.Title})
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

func planSlides(plan *saga.Plan, stories planStories, ready map[string]bool) []planSlideView {
	var sections deckSections
	slides := make([]planSlideView, 0, len(plan.Deck.Slides))
	for _, slide := range plan.Deck.Slides {
		row := planSlideView{Section: sections.next(&slide.SlideManifest), Slide: slide, DOMID: domID(slide.Target), Visual: planHref(plan.ID) + "/visual/" + url.PathEscape(slide.ID)}
		for _, item := range slide.Items {
			if item.Kind == "callout" {
				row.Callouts = append(row.Callouts, item)
			}
			if item.Record == "" {
				continue
			}
			if task := plan.FindTask(item.Record); task != nil {
				row.Tasks = append(row.Tasks, taskLink(plan, task, ready))
			} else if saga.PlanIDOf(item.Record) == "" {
				row.Stories = append(row.Stories, stories.info(item.Record))
			}
		}
		slides = append(slides, row)
	}
	return slides
}

func planPhases(plan *saga.Plan, stories planStories, ready map[string]bool) []planPhaseTasks {
	unblocks := saga.TaskUnblocks(plan.Manifests())
	row := func(task *saga.Task) planTaskRow {
		value := planTaskRow{planTaskLink: taskLink(plan, task, ready), Dependent: len(unblocks[task.ID])}
		if task.Story != "" {
			value.Story, value.HasStory = stories.info(task.Story), true
		}
		switch {
		case task.Sizing == nil:
			value.Sizing, value.Unsized = "Not sized yet", true
		case task.Sizing.WithinTwoDays:
			value.Sizing = "2 days or less"
		default:
			value.Sizing, value.Unsized = "Judged larger", true
		}
		return value
	}
	phases := []planPhaseTasks{}
	placed := map[string]bool{}
	for _, phase := range plan.Phases {
		group := planPhaseTasks{Title: phase.Title}
		for _, task := range plan.Tasks {
			if task.Phase == phase.ID {
				group.Tasks = append(group.Tasks, row(task))
				placed[task.ID] = true
			}
		}
		phases = append(phases, group)
	}
	rest := planPhaseTasks{Title: "No phase"}
	for _, task := range plan.Tasks {
		if !placed[task.ID] {
			rest.Tasks = append(rest.Tasks, row(task))
		}
	}
	if len(rest.Tasks) > 0 {
		phases = append(phases, rest)
	}
	return phases
}

func planStoryGroups(plan *saga.Plan, stories planStories) []planStoryGroup {
	byFeature := map[string]*planStoryGroup{}
	order := []string{}
	for _, urn := range plan.PlanStories() {
		info := stories.info(urn)
		key := info.Feature
		group := byFeature[key]
		if group == nil {
			group = &planStoryGroup{Feature: info.FeatureTitle, Href: info.FeatureHref}
			byFeature[key] = group
			order = append(order, key)
		}
		row := planStoryRow{Story: info}
		for _, task := range plan.Tasks {
			if task.Story == urn {
				row.All++
				if task.State == saga.TaskDone {
					row.Done++
				}
			}
		}
		group.Stories = append(group.Stories, row)
	}
	sort.Strings(order)
	groups := make([]planStoryGroup, 0, len(order))
	for _, key := range order {
		groups = append(groups, *byFeature[key])
	}
	return groups
}

// planComparison reads a Plan beside its Reviews. A review Item that names a Task
// says it delivers it, and a callout that names one says why the work departed.
// Review slides that name no Task of the Plan are work the Plan did not include.
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
func planGraph(plan *saga.Plan, ready map[string]bool) template.HTML {
	if len(plan.Tasks) == 0 {
		return ""
	}
	depth := map[string]int{}
	var depthOf func(task *saga.Task) int
	depthOf = func(task *saga.Task) int {
		if value, seen := depth[task.ID]; seen {
			return value
		}
		value := 0
		for _, dependency := range task.DependsOn {
			if prerequisite := plan.FindTask(dependency); prerequisite != nil {
				if deeper := depthOf(prerequisite) + 1; deeper > value {
					value = deeper
				}
			}
		}
		depth[task.ID] = value
		return value
	}
	columns := [][]*saga.Task{}
	for _, task := range plan.Tasks {
		column := depthOf(task)
		for len(columns) <= column {
			columns = append(columns, nil)
		}
		columns[column] = append(columns[column], task)
	}
	const (
		width, height = 176, 54
		gapX, gapY    = 46, 10
		margin, top   = 16, 34
	)
	type point struct{ x, y int }
	positions := map[string]point{}
	tallest := 0
	for x, column := range columns {
		tallest = max(tallest, len(column))
		for y, task := range column {
			positions[task.ID] = point{margin + x*(width+gapX), top + y*(height+gapY)}
		}
	}
	canvasWidth := margin*2 + len(columns)*width + (len(columns)-1)*gapX
	canvasHeight := top + tallest*(height+gapY) + 6
	var out strings.Builder
	fmt.Fprintf(&out, `<svg class="plan-graph" role="img" aria-label="Task dependency graph" viewBox="0 0 %d %d" width="%d" height="%d"><defs><marker id="plan-graph-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto"><path d="M0 0L8 4L0 8z" class="plan-graph-head"/></marker></defs>`, canvasWidth, canvasHeight, canvasWidth, canvasHeight)
	for x := range columns {
		label := "No dependencies"
		if x > 0 {
			label = strconv.Itoa(x) + " " + pluralWord(x, "step", "steps") + " in"
		}
		fmt.Fprintf(&out, `<text x="%d" y="20" class="plan-graph-column">%s</text>`, margin+x*(width+gapX), html.EscapeString(label))
	}
	for _, task := range plan.Tasks {
		for _, dependency := range task.DependsOn {
			from, to := positions[dependency], positions[task.ID]
			startX, startY, endX, endY := from.x+width, from.y+height/2, to.x-2, to.y+height/2
			middle := (startX + endX) / 2
			fmt.Fprintf(&out, `<path class="plan-graph-edge" d="M%d %dC%d %d %d %d %d %d" marker-end="url(#plan-graph-arrow)"/>`, startX, startY, middle, startY, middle, endY, endX, endY)
		}
	}
	for _, task := range plan.Tasks {
		position := positions[task.ID]
		status := statusOf(task, ready)
		title := task.Title
		if runes := []rune(title); len(runes) > 21 {
			title = string(runes[:20]) + "…"
		}
		fmt.Fprintf(&out, `<a href="%s" aria-label="%s: %s" data-plan-graph-task="%s"><title>%s</title><g transform="translate(%d %d)"><rect class="plan-graph-node %s" width="%d" height="%d" rx="8"/><text x="10" y="22" class="plan-graph-title">%s</text><text x="10" y="41" class="plan-graph-state %s">%s</text></g></a>`,
			html.EscapeString(taskHref(plan.ID, task.ID)), html.EscapeString(task.Title), status.Word, html.EscapeString(task.ID), html.EscapeString(task.Title),
			position.x, position.y, status.Class, width, height, html.EscapeString(title), status.Class, status.Word)
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
	a.inShell(w, r, "task-page", view, reviewSurfaces{deckLabel: "Plan", title: task.Title})
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
{{define "plan-essay"}}{{with .}}{{if not .Generated}}<article class="plan-essay" data-plan-essay><header><h2>{{.Title}}</h2>{{template "essay-subtext" .Subtext}}</header>{{template "essay-abstract" (essayAbstract .Abstract .Target)}}<div class="fragment-markdown overview-report essay-body" data-essay-body>{{.Body}}</div>{{if .References}}<section class="overview-references" aria-label="Essay references"><h3>References</h3><ul>{{range .References}}<li id="{{.DOMID}}">{{if .Problem}}<span>{{.Label}}</span>{{else}}<a href="#{{.Anchor}}">{{.Label}}</a>{{end}}</li>{{end}}</ul></section>{{end}}</article>{{end}}{{end}}{{end}}
{{define "plan-page"}}{{$page := .}}<div class="review-surface plan-surface" data-plan="{{.Plan.ID}}">{{template "plan-head" .}}<nav class="plan-tabs" aria-label="Plan views">{{range .Tabs}}<a href="{{.Href}}"{{if $page.TabActive .Title}} class="current" aria-current="page"{{end}}>{{.Title}}</a>{{end}}</nav>
{{if eq .Tab "deck"}}<div class="plan-deck" data-plan-deck>{{template "plan-essay" .Overview}}{{range .Slides}}{{template "slide-section-divider" .Section}}<article class="plan-slide" id="{{.DOMID}}" data-plan-slide="{{.Slide.ID}}"><h2>{{.Slide.Title}}</h2>{{if .Slide.Takeaway}}<p class="plan-takeaway">{{.Slide.Takeaway}}</p>{{end}}<img class="plan-slide-visual" src="{{.Visual}}" alt="{{.Slide.Title}}" loading="lazy">{{if .Callouts}}<ul class="plan-callouts">{{range .Callouts}}<li><strong>{{.Label}}</strong> {{.Body}}</li>{{end}}</ul>{{end}}{{if .Tasks}}<div class="plan-slide-tasks" data-plan-slide-tasks><h3>Tasks on this slide</h3><ul>{{range .Tasks}}<li><a href="{{.Href}}">{{.Title}}</a> {{template "plan-task-state" .Status}}</li>{{end}}</ul></div>{{end}}{{if .Stories}}<p class="plan-slide-stories">Stories: {{range $i,$s := .Stories}}{{if $i}}, {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}}</p>{{end}}</article>{{else}}<p class="app-empty">This Plan has no slides yet. Publish the first with <code>change-saga apply-slide --plan {{.Plan.ID}} --from SLIDE.json</code>.</p>{{end}}</div>{{end}}
{{if eq .Tab "dependencies"}}<div data-plan-dependencies>{{if .Ready}}<p class="plan-callout"><strong>{{len .Ready}} {{if eq (len .Ready) 1}}task{{else}}tasks{{end}} can start now.</strong> Everything {{if eq (len .Ready) 1}}it depends{{else}}they depend{{end}} on is done, so {{if eq (len .Ready) 1}}it can begin{{else}}they can run in parallel{{end}}: {{range $i,$t := .Ready}}{{if $i}}, {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}}.</p>{{end}}{{if .Graph}}<div class="plan-graph-wrap" tabindex="0" role="region" aria-label="Task dependency graph">{{.Graph}}</div><p class="plan-note">Columns come from dependencies, not phases. A column's tasks could run at the same time.</p>{{else}}<p class="app-empty">This Plan has no tasks yet. Add the first with <code>change-saga task add --plan {{.Plan.ID}}</code>.</p>{{end}}</div>{{end}}
{{if eq .Tab "tasks"}}<div data-plan-tasks>{{if .Unsized}}<p class="plan-callout" data-plan-unsized><strong>{{len .Unsized}} of {{.All}} tasks are not confidently two days or less:</strong> {{range $i,$t := .Unsized}}{{if $i}}, {{end}}<a href="{{.Href}}">{{.Title}}</a>{{end}}.</p>{{end}}{{range .Phases}}<h3>{{.Title}}</h3><table class="plan-table"><tbody>{{range .Tasks}}<tr data-plan-task-row="{{.ID}}"><td><a href="{{.Href}}">{{.Title}}</a></td><td>{{template "plan-task-state" .Status}}</td><td><span class="plan-state {{if .Unsized}}warn{{else}}done{{end}}">{{.Sizing}}</span></td><td class="plan-note">{{if .HasStory}}<a href="{{.Story.Href}}">{{.Story.Title}}</a>{{else}}Unblocks {{.Dependent}}{{end}}</td></tr>{{end}}</tbody></table>{{else}}<p class="app-empty">This Plan has no tasks yet.</p>{{end}}</div>{{end}}
{{if eq .Tab "stories"}}<div data-plan-stories><p class="plan-note">These come from the Plan's tasks. Nobody keeps this list.</p>{{range .Stories}}<h3>{{if .Href}}<a href="{{.Href}}">{{.Feature}}</a>{{else}}{{.Feature}}{{end}}</h3><table class="plan-table"><tbody>{{range .Stories}}<tr><td><a href="{{.Story.Href}}">{{.Story.Title}}</a></td><td>{{.Done}} of {{.All}} tasks done</td></tr>{{end}}</tbody></table>{{else}}<p class="app-empty">No tasks name a story yet.</p>{{end}}</div>{{end}}
{{if eq .Tab "reviews"}}<div data-plan-comparison>{{if .Reviews}}<p class="plan-note">Implemented by {{range $i,$r := .Reviews}}{{if $i}}, {{end}}{{if .Href}}<a href="{{.Href}}">{{.Title}}</a>{{else}}{{.Title}}{{end}}{{if .Merged}} (merged){{end}}{{if .Missing}} (missing){{end}}{{end}}. This reports differences. It never blocks a merge, and a person still judges whether the work matches.</p><table class="plan-table"><thead><tr><th>What we said</th><th>What we did</th></tr></thead><tbody>{{range .Comparison}}<tr data-plan-comparison-row="{{.Task.ID}}"><td><a href="{{.Task.Href}}">{{.Task.Title}}</a></td><td>{{range .Delivered}}<div><a href="{{.Href}}">{{.Slide}}</a> <span class="plan-note">{{.Review}}</span></div>{{end}}{{range .Departures}}<div class="plan-departure"><span class="plan-state working">Departed</span> <a href="{{.Href}}">{{.Label}}</a> <span class="plan-note">{{.Body}}</span></div>{{end}}{{if and (not .Delivered) (not .Departures)}}<span class="plan-state waiting">Not delivered</span>{{end}}</td></tr>{{end}}</tbody></table>{{if .Unplanned}}<h3>Not in the Plan</h3><ul class="plan-unplanned" data-plan-unplanned>{{range .Unplanned}}<li><a href="{{.Href}}">{{.Slide}}</a> <span class="plan-note">{{.Review}}</span></li>{{end}}</ul>{{end}}{{else}}<p class="app-empty">No Review implements this Plan yet. Link one with <code>change-saga plan link-review --plan {{.Plan.ID}} --review ID</code>.</p>{{end}}</div>{{end}}
{{if eq .Tab "prototypes"}}<div data-plan-prototypes>{{if .Prototypes}}<ul class="trace-links">{{range .Prototypes}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{else}}<p class="app-empty">This Plan has no prototypes yet. Attach one with <code>change-saga plan add-prototype --plan {{.Plan.ID}} --prototype URN</code>.</p>{{end}}</div>{{end}}
</div>{{end}}
{{define "task-page"}}<div class="review-surface plan-surface" data-task="{{.Task.ID}}"><header class="plan-head"><p class="plan-eyebrow">Task in <a href="{{planHref .Plan.ID}}">{{.Plan.Title}}</a>{{if .Phase}}, {{.Phase}}{{end}}</p><h1>{{.Task.Title}}</h1><p>{{template "plan-task-state" .Status}} <span class="plan-state {{if .Unsized}}warn{{else}}done{{end}}">{{.Sizing}}</span></p>{{if .Reason}}<p class="plan-note">{{.Reason}}</p>{{end}}</header><div class="plan-columns"><div>{{if .Essay}}<section data-task-essay><h2>Essay</h2><div class="markdown">{{.Essay}}</div></section>{{end}}{{if .Task.Summary}}<section><h2>What was done</h2><p>{{.Task.Summary}}</p></section>{{end}}{{if .Code}}<section data-task-code><h2>Code that matters</h2><ul class="plan-code">{{range .Code}}<li><code>{{.Location}}</code>{{if .Note}}<br><span class="plan-note">{{.Note}}</span>{{end}}</li>{{end}}</ul></section>{{end}}{{if not .Delivered}}{{if ne .Task.State "done"}}<section><h2>Mark it done</h2><code class="plan-command">{{.DoneCmd}}</code></section>{{end}}{{end}}</div><aside class="plan-side"><h3>User story</h3>{{if .HasStory}}<p data-task-story><a href="{{.Story.Href}}">{{.Story.Title}}</a>{{if .Story.FeatureTitle}} <span class="plan-note">in <a href="{{.Story.FeatureHref}}">{{.Story.FeatureTitle}}</a></span>{{end}}</p>{{if .Chain}}<p class="plan-note">No story of its own. It leads there by unblocking:</p><ul data-task-chain>{{range .Chain}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{end}}{{else}}<p class="plan-note">None.</p>{{end}}<h3>Depends on</h3>{{if .Depends}}<ul data-task-depends>{{range .Depends}}<li><a href="{{.Href}}">{{.Title}}</a> {{template "plan-task-state" .Status}}</li>{{end}}</ul>{{else}}<p class="plan-note">Nothing: it could start any time.</p>{{end}}<h3>Unblocks</h3>{{if .Unblocks}}<ul data-task-unblocks>{{range .Unblocks}}<li><a href="{{.Href}}">{{.Title}}</a> {{template "plan-task-state" .Status}}</li>{{end}}</ul>{{else}}<p class="plan-note">Nothing is waiting on it.</p>{{end}}{{if .Slides}}<h3>Slides that show it</h3><ul>{{range .Slides}}<li><a href="{{.Href}}">{{.Title}}</a></li>{{end}}</ul>{{end}}</aside></div></div>{{end}}
`

const planStyles = `
/* Plans: intended work, read beside Documentation and Reviews. */
.plan-surface{max-width:1120px;margin:0 auto;padding:24px}
.plan-head h1{margin:2px 0 6px}
.plan-eyebrow{margin:0;color:var(--muted);font:600 11px var(--ui);letter-spacing:.06em;text-transform:uppercase}
.plan-summary{color:var(--muted);margin:0 0 12px}
.plan-tabs{display:flex;flex-wrap:wrap;gap:2px;border-bottom:1px solid var(--line);margin:8px 0 20px}
.plan-tabs a{padding:8px 12px;color:var(--muted);text-decoration:none;border-bottom:2px solid transparent;font-size:14px}
.plan-tabs a.current{color:var(--ink);border-color:var(--accent);font-weight:600}
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
.plan-slide{border:1px solid var(--line);border-radius:10px;padding:20px;margin-bottom:20px}
.plan-essay{margin:0 0 28px}
.plan-essay h2{margin:0 0 8px}
.plan-slide h2{margin:0 0 4px}
.plan-takeaway{color:var(--muted);margin:0 0 12px}
.plan-slide-visual{display:block;width:100%;height:auto;border-radius:6px}
.plan-slide-tasks{border-top:1px solid var(--line);margin-top:14px;padding-top:8px}
.plan-slide-tasks h3{margin:0 0 4px;font-size:12px;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
.plan-slide-tasks ul{list-style:none;margin:0;padding:0}
.plan-slide-tasks li{display:flex;justify-content:space-between;gap:12px;padding:4px 0}
.plan-callouts{margin:12px 0 0;padding-left:18px;font-size:14px}
.plan-graph-wrap{overflow:auto;border:1px solid var(--line);border-radius:10px;background:var(--bg-subtle)}
.plan-graph{display:block;font:13px var(--ui)}
.plan-graph-column{fill:var(--muted);font-size:12px;font-weight:600}
.plan-graph-edge{fill:none;stroke:var(--faint);stroke-width:1.5}
.plan-graph-head{fill:var(--faint)}
.plan-graph-node{stroke-width:1;fill:var(--review-none-bg);stroke:var(--line)}
.plan-graph-node.done{fill:var(--review-approved-bg);stroke:var(--review-approved-ink)}
.plan-graph-node.ready{fill:var(--accent-soft);stroke:var(--accent);stroke-width:2.5}
.plan-graph-node.working{fill:var(--review-stale-bg);stroke:var(--review-stale-ink)}
.plan-graph-title{fill:var(--ink);font-weight:600}
.plan-graph-state{fill:var(--muted);font-size:12px;font-weight:600}
.plan-graph-state.done{fill:var(--review-approved-ink)}
.plan-graph-state.ready{fill:var(--accent)}
.plan-graph-state.working{fill:var(--review-stale-ink)}
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
