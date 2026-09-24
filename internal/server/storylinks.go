package server

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/semanticgraph"
)

// Story links belong to Items. A slide is only a summary of those exact
// owners; legacy broad links remain visible without being inherited by Items.
type storyLinksView struct {
	DOMID, Title, Tooltip string
	Count                 int
	Links, Legacy         []storyLinkView
}

type storyLinkView struct {
	Story, StoryHref, StoryTarget string
	Criterion, CriterionHref      string
	Item, ItemHref, Rationale     string
	Status                        requirements.Currency
}

const storyLinksTemplate = `
{{define "story-control"}}<button type="button" class="icon-button stories-button" data-open-stories="stories-{{.DOMID}}" aria-label="Open {{.Count}} linked stories for {{.Title}}" title="{{.Tooltip}}">{{template "icon" "story"}}<span>{{.Count}}</span>{{if .Legacy}}<span aria-label="Legacy links need element assignment">!</span>{{end}}</button><template id="stories-{{.DOMID}}">{{template "story-links" .}}</template>{{end}}
{{define "story-links"}}<section class="story-links"><h2>{{.Title}}</h2><p>{{.Count}} linked {{if eq .Count 1}}story{{else}}stories{{end}} · linked through specific diagram elements</p>{{if .Links}}{{template "story-link-rows" .Links}}{{else}}<p data-story-links-empty>No element-level story links yet.</p>{{end}}{{if .Legacy}}<section class="story-links-legacy" data-story-links-legacy><h3>Needs element assignment</h3><p>These older links apply to a whole slide or deck. They are not attributed to individual elements or included in the summary count. Replace them with links from the relevant elements.</p>{{template "story-link-rows" .Legacy}}</section>{{end}}</section>{{end}}
{{define "story-link-rows"}}<ul>{{range .}}<li data-story-link="{{.StoryTarget}}"><strong>{{if .StoryHref}}<a href="{{.StoryHref}}">{{.Story}}</a>{{else}}{{.Story}}{{end}}</strong><span class="story-link-status" data-status="{{.Status}}">{{.Status}}</span>{{if .Criterion}}<p>{{if .CriterionHref}}<a href="{{.CriterionHref}}">{{.Criterion}}</a>{{else}}{{.Criterion}}{{end}}</p>{{end}}<p>Element: {{if .ItemHref}}<a href="{{.ItemHref}}">{{.Item}}</a>{{else}}{{.Item}}{{end}}</p>{{if .Rationale}}<p>{{.Rationale}}</p>{{end}}</li>{{end}}</ul>{{end}}
`

func (view *storyLinksView) summarize() {
	stories := map[string]string{}
	for _, link := range view.Links {
		stories[link.StoryTarget] = link.Story
	}
	titles := make([]string, 0, len(stories))
	for _, title := range stories {
		titles = append(titles, title)
	}
	sort.Strings(titles)
	view.Count = len(titles)
	view.Tooltip = "Linked stories"
	if len(titles) > 0 {
		view.Tooltip += ": " + strings.Join(titles, "; ")
	}
}

func decorateFragmentStories(document *saga.Saga, records requirements.Document, view *fragmentView) error {
	if err := semanticgraph.ProjectSlideCriterionLinks(document, &records); err != nil {
		return err
	}
	return (&storyLinkDecorator{document: document, records: records}).decorate(view)
}

type storyLinkDecorator struct {
	document *saga.Saga
	records  requirements.Document
	digests  map[*saga.Deck]map[string]string
}

func (decorator *storyLinkDecorator) decorate(view *fragmentView) error {
	document, records := decorator.document, decorator.records
	var deck *saga.Deck
	for _, candidate := range document.Decks {
		for _, slide := range candidate.Slides {
			if slide.Target == view.Target {
				deck = candidate
				break
			}
		}
	}
	if deck == nil {
		return nil // Narrative landmarks are not slide Items.
	}
	view.Stories = &storyLinksView{DOMID: view.DOMID, Title: view.Title}
	if view.Stories.Title == "" {
		view.Stories.Title = view.ID
	}
	owners := map[string]*landmarkView{}
	for _, item := range view.LandmarkViews {
		item.Stories = &storyLinksView{DOMID: item.DOMID, Title: item.Title}
		owners[item.Target] = item
	}
	// Compute only this deck's visual pins. No source/code, work plan, quality
	// evidence, or unrelated design content is loaded by a story affordance.
	if decorator.digests == nil {
		decorator.digests = map[*saga.Deck]map[string]string{}
	}
	digests, ok := decorator.digests[deck]
	if !ok {
		var err error
		digests, err = saga.CurrentDesignContentDigests(&saga.Saga{Section: &saga.Section{}, Decks: []*saga.Deck{deck}})
		if err != nil {
			return err
		}
		decorator.digests[deck] = digests
	}
	inputs := requirements.StaleInputs{CurrentContentDigests: digests, Missing: map[string]bool{}}
	stories := map[string]requirements.Story{}
	for _, story := range records.Stories {
		stories[story.Identity.ID] = story
	}
	for _, relation := range records.Relations {
		if relation.State != requirements.RelationActive || (relation.Type != requirements.RelationExplains && relation.Type != requirements.RelationAddresses) {
			continue
		}
		owner := owners[relation.From]
		broad := relation.From == view.Target || relation.From == deck.Target
		if owner == nil && !broad {
			continue
		}
		ref, err := livingid.Parse(relation.To)
		if err != nil || (ref.Kind != livingid.KindStory && ref.Kind != livingid.KindCriterion) {
			continue
		}
		storyID := ref.ID
		if ref.Kind == livingid.KindCriterion {
			storyID = ref.ParentID
		}
		storyURN, _ := livingid.Story(records.SagaID, storyID)
		link := storyLinkView{Story: storyID, StoryTarget: storyURN, Rationale: relation.Rationale}
		if story, ok := stories[storyID]; ok {
			link.StoryHref = requirementStoryHref(storyID)
			if story.CurrentRevision != nil {
				link.Story = story.CurrentRevision.Title
				for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
					if ref.Kind == livingid.KindCriterion && criterion.ID == ref.ID {
						link.Criterion = criterion.Statement
						link.CriterionHref = requirementCriterionHref(storyID, criterion.ID)
					}
				}
			}
		} else {
			inputs.Missing[relation.To] = true
		}
		if ref.Kind == livingid.KindCriterion && link.Criterion == "" {
			link.Criterion = "Criterion " + ref.ID + " (not current)"
		}
		link.Status = requirements.EvaluateRelation(records, relation, inputs).Status
		if owner != nil {
			link.Item, link.ItemHref = owner.Title, "#"+owner.DOMID
			owner.Stories.Links = append(owner.Stories.Links, link)
			view.Stories.Links = append(view.Stories.Links, link)
		} else {
			link.Item = "Whole slide"
			if relation.From == deck.Target {
				link.Item = "Whole deck"
			}
			view.Stories.Legacy = append(view.Stories.Legacy, link)
		}
	}
	for _, owner := range owners {
		owner.Stories.summarize()
	}
	view.Stories.summarize()
	return nil
}
