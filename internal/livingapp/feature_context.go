package livingapp

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// FeatureContextPage is a bounded, feature-owned index. The default compact
// projection deliberately carries identities and link structure rather than
// repeating full prose. Expand names one story when prose and exact evidence
// are needed.
type FeatureContextPage struct {
	Feature      FeatureContextFeature      `json:"feature"`
	Projection   string                     `json:"projection"`
	Completeness FeatureContextCompleteness `json:"completeness"`
	Stories      []FeatureContextStory      `json:"stories"`
	Neighbors    []FeatureContextNeighbor   `json:"neighbors"`
	Terms        []FeatureContextTerm       `json:"terms"`
	Visuals      []FeatureContextSlide      `json:"visuals"`
	Gaps         []FeatureContextGap        `json:"gaps"`
	Conflicts    []FeatureContextConflict   `json:"conflicts"`
	Expanded     *FeatureContextExpansion   `json:"expanded,omitempty"`
}

type FeatureContextFeature struct {
	URN         string `json:"urn"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type FeatureContextCompleteness struct {
	Bounded        bool     `json:"bounded"`
	RelationHops   int      `json:"relation_hops"`
	AncillaryScope string   `json:"ancillary_scope"`
	Includes       []string `json:"includes"`
	Excludes       []string `json:"excludes"`
	ExpandUsage    string   `json:"expand_usage"`
}

type FeatureContextStory struct {
	Requirement       string                    `json:"requirement"`
	ID                string                    `json:"id"`
	Title             string                    `json:"title,omitempty"`
	State             string                    `json:"state"`
	Criteria          []FeatureContextCriterion `json:"criteria"`
	RevisionHeads     []string                  `json:"revision_heads"`
	LifecycleHeads    []string                  `json:"lifecycle_heads"`
	RevisionConflict  bool                      `json:"revision_conflict"`
	LifecycleConflict bool                      `json:"lifecycle_conflict"`
}

type FeatureContextCriterion struct {
	Criterion string `json:"criterion"`
	ID        string `json:"id"`
}

type FeatureContextNeighbor struct {
	Feature     string                       `json:"feature"`
	Requirement string                       `json:"requirement"`
	Title       string                       `json:"title,omitempty"`
	Relations   []FeatureContextRelationLink `json:"relations"`
}

type FeatureContextRelationLink struct {
	Relation string `json:"relation"`
	Type     string `json:"type"`
	From     string `json:"from"`
	To       string `json:"to"`
}

type FeatureContextTerm struct {
	Term                   string                              `json:"term"`
	Name                   string                              `json:"name,omitempty"`
	State                  string                              `json:"state"`
	DefinitionMaturity     requirements.DefinitionMaturity     `json:"definition_maturity"`
	ImplementationEvidence requirements.ImplementationEvidence `json:"implementation_evidence"`
	RevisionHeads          []string                            `json:"revision_heads"`
	LifecycleHeads         []string                            `json:"lifecycle_heads"`
	RevisionConflict       bool                                `json:"revision_conflict"`
	LifecycleConflict      bool                                `json:"lifecycle_conflict"`
	Aliases                []string                            `json:"aliases,omitempty"`
	Definition             string                              `json:"definition,omitempty"`
	Stories                []string                            `json:"stories,omitempty"`
	Code                   []coderef.Reference                 `json:"code,omitempty"`
}

type FeatureContextSlide struct {
	Deck           string               `json:"deck"`
	Slide          string               `json:"slide"`
	Title          string               `json:"title"`
	Intent         string               `json:"intent"`
	Layout         string               `json:"layout,omitempty"`
	Section        string               `json:"section,omitempty"`
	Takeaway       string               `json:"takeaway,omitempty"`
	ReadingOrder   []string             `json:"reading_order,omitempty"`
	StoryLinks     []string             `json:"story_links"`
	CriterionLinks []string             `json:"criterion_links"`
	Relations      []string             `json:"relations"`
	Items          []FeatureContextItem `json:"items"`
}

type FeatureContextItem struct {
	Item               string                 `json:"item"`
	Label              string                 `json:"label"`
	Kind               string                 `json:"kind"`
	Description        string                 `json:"description,omitempty"`
	About              string                 `json:"about,omitempty"`
	Selector           *saga.LandmarkSelector `json:"selector,omitempty"`
	StoryLinks         []string               `json:"story_links"`
	CriterionLinks     []string               `json:"criterion_links"`
	Relations          []string               `json:"relations"`
	CodeReferenceCount int                    `json:"code_reference_count"`
	Code               []coderef.Reference    `json:"code,omitempty"`
}

type FeatureContextGap struct {
	Code     string `json:"code"`
	Resource string `json:"resource"`
	Detail   string `json:"detail,omitempty"`
}

type FeatureContextConflict struct {
	Kind      string   `json:"kind"`
	Resource  string   `json:"resource"`
	Heads     []string `json:"heads,omitempty"`
	Relations []string `json:"relations,omitempty"`
}

type FeatureContextExpansion struct {
	Requirement  Requirement    `json:"requirement"`
	Citations    []Citation     `json:"citations"`
	Relations    []Relation     `json:"relations"`
	Traceability []Traceability `json:"traceability"`
}

type contextLinkSet struct {
	stories   []string
	criteria  []string
	relations []string
}

func (s *session) featureContextResult(query Query) (Result, error) {
	if query.Filters.Expand != "" && query.Cursor != "" {
		return Result{}, appError(CodeInvalidArgument, "context expansion and cursor are mutually exclusive", false, nil, nil)
	}
	feature, err := s.requireFeature(query.Filters.Feature)
	if err != nil {
		return Result{}, err
	}
	ownedStories := s.featureStories(feature.ID)
	selected := ownedStories
	projection := "compact"
	if query.Filters.Expand != "" {
		projection = "expanded"
		selected = nil
		for _, story := range ownedStories {
			if resourceMatches(query.Filters.Expand, story.ID, story.Requirement) {
				selected = append(selected, story)
			}
		}
		if len(selected) == 0 {
			return Result{}, appError(CodeNotFound, "expanded requirement was not found in the feature", false, map[string]any{"kind": "requirement", "feature": feature.ID}, nil)
		}
	}

	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, len(selected))
	if err != nil {
		return Result{}, err
	}
	pageStories := append([]FeatureContextStory{}, selected[start:end]...)
	pageEndpoints := endpointSet(pageStories)
	relations := s.contextRelations(pageEndpoints)
	featureSummary := FeatureContextFeature{URN: applayout.FeatureURN(s.requirements.SagaID, feature.ID), ID: feature.ID, Title: feature.Title}
	if projection == "expanded" {
		featureSummary.Description = feature.Description
	}
	data := FeatureContextPage{
		Feature:    featureSummary,
		Projection: projection,
		Completeness: FeatureContextCompleteness{
			Bounded: true, RelationHops: 1, AncillaryScope: "stories returned on this page, plus terms that name the feature itself",
			Includes: []string{
				"feature-owned stories and their unique current criteria",
				"one-hop relations, cross-feature requirement neighbors, linked terms, and linked visual Items",
				"known requirement, relation, readiness, and work-plan gaps or conflicts",
			},
			Excludes: []string{
				"revision and lifecycle history", "unlinked global prose and app metadata", "relations beyond one hop",
			},
			ExpandUsage: "repeat with --expand STORY-ID|URN for current prose, citations, exact relation records, traceability, and code references",
		},
		Stories:   pageStories,
		Neighbors: s.contextNeighbors(feature.ID, pageEndpoints, relations),
		Terms:     s.contextTerms(feature.ID, pageEndpoints, projection == "expanded"),
		Visuals:   s.contextVisuals(pageEndpoints, relations, projection == "expanded"),
	}
	data.Gaps, data.Conflicts = s.contextProblems(feature.ID, pageEndpoints, relations, pageStories)
	if projection == "expanded" {
		detail, detailErr := s.contextExpansion(pageStories[0], relations)
		if detailErr != nil {
			return Result{}, detailErr
		}
		data.Expanded = &detail
	}
	return Result{Data: data, Page: page}, nil
}

func (s *session) requireFeature(value string) (applayout.Feature, error) {
	if strings.TrimSpace(value) == "" {
		return applayout.Feature{}, appError(CodeInvalidArgument, "feature is required", false, nil, nil)
	}
	id, ok := applayout.FeatureFromURN(s.requirements.SagaID, value)
	if !ok {
		return applayout.Feature{}, appError(CodeInvalidArgument, "feature must be an exact feature ID or canonical URN", false, nil, nil)
	}
	for _, feature := range s.requirements.Features {
		if feature.ID == id {
			return feature, nil
		}
	}
	known := make([]string, 0, len(s.requirements.Features))
	for _, feature := range s.requirements.Features {
		known = append(known, feature.ID)
	}
	return applayout.Feature{}, appError(CodeNotFound, "feature was not found", false, map[string]any{"feature": id, "known": known}, nil)
}

func (s *session) featureStories(feature string) []FeatureContextStory {
	rows := []FeatureContextStory{}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
		if story.Feature != feature {
			continue
		}
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		row := FeatureContextStory{
			Requirement: storyURN, ID: story.Identity.ID, State: "conflicted",
			Criteria: []FeatureContextCriterion{}, RevisionHeads: copyStrings(story.RevisionHeads), LifecycleHeads: copyStrings(story.LifecycleHeads),
			RevisionConflict: story.RevisionConflict(), LifecycleConflict: story.LifecycleConflict(),
		}
		if story.CurrentLifecycle != nil {
			row.State = string(story.CurrentLifecycle.State)
		}
		if story.CurrentRevision != nil {
			row.Title = story.CurrentRevision.Title
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				criterionURN, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
				row.Criteria = append(row.Criteria, FeatureContextCriterion{Criterion: criterionURN, ID: criterion.ID})
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Requirement < rows[j].Requirement })
	return rows
}

func endpointSet(stories []FeatureContextStory) map[string]bool {
	result := map[string]bool{}
	for _, story := range stories {
		result[story.Requirement] = true
		for _, criterion := range story.Criteria {
			result[criterion.Criterion] = true
		}
	}
	return result
}

func (s *session) contextRelations(endpoints map[string]bool) []Relation {
	rows := []Relation{}
	for _, relation := range s.requirements.Relations {
		if !endpoints[relation.From] && !endpoints[relation.To] {
			continue
		}
		urn, _ := livingid.Relation(s.requirements.SagaID, relation.ID)
		rows = append(rows, Relation{URN: urn, Stale: relation.Stale, StaleReasons: copyStrings(relation.StaleReasons), Relation: relation})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].URN < rows[j].URN })
	return rows
}

func (s *session) contextNeighbors(feature string, endpoints map[string]bool, relations []Relation) []FeatureContextNeighbor {
	type neighbor struct {
		row       FeatureContextNeighbor
		relations []FeatureContextRelationLink
	}
	byRequirement := map[string]*neighbor{}
	for _, relation := range relations {
		if relation.State != requirements.RelationActive || relation.Stale {
			continue
		}
		other := ""
		if endpoints[relation.From] {
			other = relation.To
		} else if endpoints[relation.To] {
			other = relation.From
		}
		ref, err := livingid.Parse(other)
		if err != nil || ref.Kind != livingid.KindStory && ref.Kind != livingid.KindCriterion {
			continue
		}
		storyID := ref.ID
		if ref.Kind == livingid.KindCriterion {
			storyID = ref.ParentID
		}
		story := s.storyByID(storyID)
		if story == nil || story.Feature == feature {
			continue
		}
		storyURN, _ := livingid.Story(s.requirements.SagaID, storyID)
		value := byRequirement[storyURN]
		if value == nil {
			title := ""
			if story.CurrentRevision != nil {
				title = story.CurrentRevision.Title
			}
			value = &neighbor{row: FeatureContextNeighbor{Feature: story.Feature, Requirement: storyURN, Title: title}}
			byRequirement[storyURN] = value
		}
		value.relations = append(value.relations, FeatureContextRelationLink{Relation: relation.URN, Type: string(relation.Type), From: relation.From, To: relation.To})
	}
	rows := make([]FeatureContextNeighbor, 0, len(byRequirement))
	for _, requirement := range sortedKeys(byRequirement) {
		value := byRequirement[requirement]
		sort.Slice(value.relations, func(i, j int) bool { return value.relations[i].Relation < value.relations[j].Relation })
		value.row.Relations = value.relations
		rows = append(rows, value.row)
	}
	return rows
}

func (s *session) contextTerms(feature string, endpoints map[string]bool, expanded bool) []FeatureContextTerm {
	featureURN := applayout.FeatureURN(s.requirements.SagaID, feature)
	rows := []FeatureContextTerm{}
	for _, term := range s.requirements.Terms {
		if !contextTermMatches(s.requirements.SagaID, term, featureURN, endpoints) {
			continue
		}
		revision := term.CurrentRevision
		termURN, _ := requirements.TermURN(s.requirements.SagaID, term.Identity.ID)
		state := "conflicted"
		if term.CurrentLifecycle != nil {
			state = string(term.CurrentLifecycle.State)
		}
		row := FeatureContextTerm{
			Term: termURN, State: state,
			DefinitionMaturity: requirements.DefinitionMaturityUnknown, ImplementationEvidence: requirements.ImplementationEvidenceUnknown,
			RevisionHeads: copyStrings(term.RevisionHeads), LifecycleHeads: copyStrings(term.LifecycleHeads),
			RevisionConflict: len(term.RevisionHeads) > 1, LifecycleConflict: len(term.LifecycleHeads) > 1,
		}
		if revision != nil {
			row.Name = revision.Name
			row.DefinitionMaturity = revision.EffectiveDefinitionMaturity()
			row.ImplementationEvidence = revision.EffectiveImplementationEvidence()
			if expanded {
				row.Aliases = copyStrings(revision.Aliases)
				row.Definition = revision.Definition
				row.Stories = copyStrings(revision.Stories)
				row.Code = append([]coderef.Reference{}, revision.Code...)
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Term < rows[j].Term })
	return rows
}

func contextTermMatches(sagaID string, term requirements.Term, featureURN string, endpoints map[string]bool) bool {
	matches := func(revision requirements.TermRevision) bool {
		if contains(revision.Records, featureURN) {
			return true
		}
		for _, story := range revision.Stories {
			if endpoints[story] {
				return true
			}
		}
		return false
	}
	if term.CurrentRevision != nil {
		return matches(*term.CurrentRevision)
	}
	heads := map[string]bool{}
	for _, head := range term.RevisionHeads {
		heads[head] = true
	}
	for _, revision := range term.Revisions {
		urn, _ := requirements.TermRevisionURN(sagaID, term.Identity.ID, revision.ID)
		if heads[urn] && matches(revision) {
			return true
		}
	}
	return false
}

func (s *session) contextVisuals(endpoints map[string]bool, relations []Relation, expanded bool) []FeatureContextSlide {
	links := map[string]*contextLinkSet{}
	descendantLinks := map[string]*contextLinkSet{}
	for _, relation := range relations {
		if !endpoints[relation.To] || relation.State != requirements.RelationActive || relation.Stale {
			continue
		}
		value := &contextLinkSet{relations: []string{relation.URN}}
		if ref, err := livingid.Parse(relation.To); err == nil && ref.Kind == livingid.KindCriterion {
			value.criteria = append(value.criteria, relation.To)
		} else {
			value.stories = append(value.stories, relation.To)
		}
		links[relation.From] = mergeContextLinks(links[relation.From], value)
		if relation.Scope == requirements.ScopeDescendants {
			descendantLinks[relation.From] = mergeContextLinks(descendantLinks[relation.From], value)
		}
	}
	rows := []FeatureContextSlide{}
	for _, deck := range s.saga.Decks {
		for _, slide := range deck.Slides {
			slideLinks := mergeContextLinks(nil, links[slide.Target], descendantLinks[deck.Target])
			row := FeatureContextSlide{Deck: deck.Target, Slide: slide.Target, Title: slide.Title, Intent: slide.Intent, Items: []FeatureContextItem{}}
			for _, item := range slide.Items {
				itemLinks := mergeContextLinks(nil, links[item.Target], descendantLinks[deck.Target], descendantLinks[slide.Target])
				if itemLinks == nil {
					continue
				}
				itemRow := FeatureContextItem{Item: item.Target, Label: item.Label, Kind: item.Kind, StoryLinks: uniqueSorted(itemLinks.stories), CriterionLinks: uniqueSorted(itemLinks.criteria), Relations: uniqueSorted(itemLinks.relations)}
				if expanded {
					selector := item.Selector
					itemRow.Description, itemRow.About, itemRow.Selector = item.Description, item.About, &selector
				}
				for _, file := range item.Code {
					itemRow.CodeReferenceCount += len(file.References)
					if expanded {
						itemRow.Code = append(itemRow.Code, file.References...)
					}
				}
				row.Items = append(row.Items, itemRow)
				slideLinks = mergeContextLinks(slideLinks, itemLinks)
			}
			if slideLinks == nil {
				continue
			}
			row.StoryLinks = uniqueSorted(slideLinks.stories)
			row.CriterionLinks = uniqueSorted(slideLinks.criteria)
			row.Relations = uniqueSorted(slideLinks.relations)
			if expanded {
				row.Layout, row.Section, row.Takeaway = slide.Layout, slide.Section, slide.Takeaway
				row.ReadingOrder = copyStrings(slide.ReadingOrder)
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Slide < rows[j].Slide })
	return rows
}

func mergeContextLinks(base *contextLinkSet, values ...*contextLinkSet) *contextLinkSet {
	for _, value := range values {
		if value == nil {
			continue
		}
		if base == nil {
			base = &contextLinkSet{}
		}
		base.stories = append(base.stories, value.stories...)
		base.criteria = append(base.criteria, value.criteria...)
		base.relations = append(base.relations, value.relations...)
	}
	return base
}

func (s *session) contextProblems(feature string, endpoints map[string]bool, relations []Relation, stories []FeatureContextStory) ([]FeatureContextGap, []FeatureContextConflict) {
	gaps := []FeatureContextGap{}
	conflicts := []FeatureContextConflict{}
	for _, story := range stories {
		if story.RevisionConflict {
			conflicts = append(conflicts, FeatureContextConflict{Kind: "requirement_revision", Resource: story.Requirement, Heads: copyStrings(story.RevisionHeads)})
		}
		if story.LifecycleConflict {
			conflicts = append(conflicts, FeatureContextConflict{Kind: "requirement_lifecycle", Resource: story.Requirement, Heads: copyStrings(story.LifecycleHeads)})
		}
	}
	for _, relation := range relations {
		if relation.Stale {
			gaps = append(gaps, FeatureContextGap{Code: "stale_relation", Resource: relation.URN, Detail: strings.Join(relation.StaleReasons, "; ")})
		}
		if relation.State == requirements.RelationActive && !relation.Stale && relation.Type == requirements.RelationConflictsWith {
			resource := relation.From
			if !endpoints[resource] {
				resource = relation.To
			}
			conflicts = append(conflicts, FeatureContextConflict{Kind: "intent", Resource: resource, Relations: []string{relation.URN}})
		}
	}
	traces, _ := s.traceRows(Filters{Feature: feature})
	for _, trace := range traces {
		if !endpoints[trace.Story] && !endpoints[trace.Criterion] {
			continue
		}
		for _, blocker := range trace.Blockers {
			gaps = append(gaps, FeatureContextGap{Code: blocker.Code, Resource: blocker.Resource, Detail: blocker.Detail})
			if blocker.Code == "work_conflict" {
				conflicts = append(conflicts, FeatureContextConflict{Kind: "work_plan", Resource: blocker.Resource})
			}
		}
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Resource == gaps[j].Resource {
			return gaps[i].Code < gaps[j].Code
		}
		return gaps[i].Resource < gaps[j].Resource
	})
	sort.Slice(conflicts, func(i, j int) bool {
		if conflicts[i].Resource == conflicts[j].Resource {
			return conflicts[i].Kind < conflicts[j].Kind
		}
		return conflicts[i].Resource < conflicts[j].Resource
	})
	return gaps, conflicts
}

func (s *session) contextExpansion(story FeatureContextStory, relations []Relation) (FeatureContextExpansion, error) {
	value := s.storyByID(story.ID)
	if value == nil {
		return FeatureContextExpansion{}, appError(CodeNotFound, "requirement was not found", false, nil, nil)
	}
	requirement := s.projectRequirement(value)
	citations := []Citation{}
	if value.CurrentRevision != nil {
		wanted := map[string]bool{}
		for _, citation := range value.CurrentRevision.Citations {
			wanted[citation] = true
		}
		for _, citation := range s.citationRows(Filters{}) {
			if wanted[citation.URN] {
				citations = append(citations, citation)
			}
		}
	}
	traces, _ := s.traceRows(Filters{Requirement: story.Requirement})
	return FeatureContextExpansion{Requirement: requirement, Citations: citations, Relations: relations, Traceability: traces}, nil
}

func (s *session) storyByID(id string) *requirements.Story {
	for index := range s.requirements.Stories {
		if s.requirements.Stories[index].Identity.ID == id {
			return &s.requirements.Stories[index]
		}
	}
	return nil
}

func (s *session) projectRequirement(story *requirements.Story) Requirement {
	urn, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
	return Requirement{
		Requirement: urn, ID: story.Identity.ID, Feature: story.Feature, CreatedAt: story.Identity.CreatedAt,
		RevisionHeads: copyStrings(story.RevisionHeads), LifecycleHeads: copyStrings(story.LifecycleHeads),
		CurrentRevision: story.CurrentRevision, CurrentLifecycle: story.CurrentLifecycle,
		RevisionConflict: story.RevisionConflict(), LifecycleConflict: story.LifecycleConflict(),
	}
}

func (s *session) validateFeatureFilter(filters *Filters) error {
	if filters.Feature == "" {
		return nil
	}
	feature, err := s.requireFeature(filters.Feature)
	if err != nil {
		return err
	}
	filters.Feature = feature.ID
	return nil
}
