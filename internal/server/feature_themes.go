package server

import (
	"html/template"
	"net/http"
	"net/url"
	"sort"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func featureThemeHref(id string) string { return "/feature-themes/" + url.PathEscape(id) }

func validFeatureGrouping(value string) bool {
	return value == "all" || value == "theme" || value == "persona" || value == "label" || value == "milestone"
}

type featureGroupingView struct {
	Path      string
	Mode      string
	HasThemes bool
}

func (v featureGroupingView) Action() string {
	if v.Path == "" {
		return "/features"
	}
	return v.Path
}

func featureGrouping(r *http.Request, document *saga.Saga) string {
	return featureCollectionGrouping(r, document, false)
}
func featureGroupingCookie(planned bool) string {
	if planned {
		return "saga-roadmap-group"
	}
	return "saga-feature-group"
}
func featureCollectionGrouping(r *http.Request, document *saga.Saga, planned bool) string {
	available := func(value string) bool {
		return validFeatureGrouping(value) && (value != "theme" || len(document.FeatureThemes) > 0)
	}
	path := "/features"
	if planned {
		path = "/roadmap"
	}
	if r.URL.Path == path {
		if value := r.URL.Query().Get("group"); available(value) {
			return value
		}
	}
	if cookie, err := r.Cookie(featureGroupingCookie(planned)); err == nil && available(cookie.Value) {
		return cookie.Value
	}
	if len(document.FeatureThemes) > 0 {
		return "theme"
	}
	return "all"
}

func featurePresentationPlan(document *saga.Saga, plan applayout.FeaturePlan) applayout.FeaturePlan {
	plan.Milestone = document.Classifications.Title(document.Manifest.ID, "milestone", plan.Milestone)
	labels := make([]string, 0, len(plan.Labels))
	for _, ref := range plan.Labels {
		labels = append(labels, document.Classifications.Title(document.Manifest.ID, "label", ref))
	}
	plan.Labels = labels
	return plan
}

type featureGroup struct {
	ID, Title, Href string
	Features        []*saga.Feature
}

// Each group owns fresh presentation rows; the canonical model is read-only.
// Membership comes from current story revisions, regardless of acceptance.
func featureGroups(document *saga.Saga, records requirements.Document, mode string) []featureGroup {
	groups := []featureGroup{}
	seen := map[string]bool{}
	if mode == "theme" {
		for _, theme := range document.FeatureThemes {
			group := featureGroup{ID: "theme-" + theme.ID, Title: theme.Title, Href: featureThemeHref(theme.ID)}
			members := map[string]bool{}
			for _, ref := range theme.Features {
				members[ref] = true
			}
			for _, feature := range document.Features {
				if members[feature.Target] {
					group.Features = append(group.Features, feature)
					seen[feature.ID] = true
				}
			}
			if len(group.Features) > 0 {
				groups = append(groups, group)
			}
		}
	} else if mode == "persona" {
		for _, persona := range records.Personas {
			ref, _ := requirements.PersonaURN(records.SagaID, persona.Identity.ID)
			title := persona.Identity.ID
			if persona.CurrentRevision != nil {
				title = persona.CurrentRevision.Name
			}
			members := map[string]bool{}
			for _, story := range records.Stories {
				if story.CurrentRevision == nil {
					continue
				}
				for _, member := range story.CurrentRevision.Personas {
					if member == ref {
						members[story.Feature] = true
					}
				}
			}
			group := featureGroup{ID: "persona-" + persona.Identity.ID, Title: title, Href: personaHref(persona.Identity.ID)}
			for _, feature := range document.Features {
				if members[feature.ID] {
					group.Features = append(group.Features, feature)
					seen[feature.ID] = true
				}
			}
			if len(group.Features) > 0 {
				groups = append(groups, group)
			}
		}
	} else if mode == "label" || mode == "milestone" {
		byRef := map[string]*featureGroup{}
		for _, feature := range document.Features {
			refs := feature.Plan.Labels
			if mode == "milestone" {
				refs = nil
				if feature.Plan.Milestone != "" {
					refs = []string{feature.Plan.Milestone}
				}
			}
			for _, ref := range refs {
				group := byRef[ref]
				if group == nil {
					group = &featureGroup{ID: mode + "-" + ref, Title: document.Classifications.Title(document.Manifest.ID, mode, ref)}
					byRef[ref] = group
				}
				group.Features = append(group.Features, feature)
				seen[feature.ID] = true
			}
		}
		for _, group := range byRef {
			groups = append(groups, *group)
		}
		sort.Slice(groups, func(i, j int) bool {
			if groups[i].Title == groups[j].Title {
				return groups[i].ID < groups[j].ID
			}
			return groups[i].Title < groups[j].Title
		})
	}

	remaining := featureGroup{ID: "ungrouped", Title: "Ungrouped"}
	if mode == "label" {
		remaining.Title = "Unlabeled"
	}
	if mode == "milestone" {
		remaining.Title = "No milestone"
	}
	for _, feature := range document.Features {
		if !seen[feature.ID] {
			remaining.Features = append(remaining.Features, feature)
		}
	}
	if len(remaining.Features) > 0 {
		groups = append(groups, remaining)
	}
	return groups
}

// cloneGroupNav isolates all mutable navigation fields and prefixes only
// presentation identities; canonical destinations and slide targets stay fixed.
func cloneGroupNav(node *navNodeView, prefix string) *navNodeView {
	result := *node
	result.NodeID = prefix + node.NodeID
	result.Children = nil
	for _, child := range node.Children {
		result.Children = append(result.Children, cloneGroupNav(child, prefix))
	}
	return &result
}

func groupedFeaturesDirectory(document *saga.Saga, graph *appGraph, current, query, mode string, planned ...bool) *directoryView {
	view := featuresDirectory(document, graph, current, "", planned...)
	if mode == "all" {
		view.apply(query)
		return view
	}
	rows := map[string]directoryRow{}
	for _, row := range view.Rows {
		rows[row.Key] = row
	}
	view.Columns = append([]directoryColumn{{Title: map[string]string{"theme": "Theme", "persona": "Persona", "label": "Label", "milestone": "Milestone"}[mode]}}, view.Columns...)
	view.Rows, view.Total = nil, 0
	view.Noun, view.Nouns = "membership", "memberships"
	view.Lede += " Grouped by " + mode + ". Features without an assignment remain visible."
	for _, group := range featureGroups(document, graph.requirements, mode) {
		for _, feature := range group.Features {
			row := rows[feature.ID]
			row.Key = group.ID + "--" + feature.ID
			row.Cells = append([]directoryCell{linkCell(group.Title, group.Href)}, row.Cells...)
			view.addRow(row)
		}
	}
	view.apply(query)
	return view
}

type featureThemePageView struct {
	Subtext, Abstract string
	ID, Target, Title string
	Body              template.HTML
	Features          []featureLinkView
	Decks             []featureDeckView
}

func featureThemePage(document *saga.Saga, id string) (*featureThemePageView, error) {
	theme := document.FindFeatureTheme(id)
	if theme == nil {
		return nil, errAppPageNotFound
	}
	view := &featureThemePageView{ID: theme.ID, Title: theme.Title, Target: theme.Target}
	view.Subtext, view.Abstract = theme.Subtext, theme.Abstract
	view.Body = overviewMarkdown(theme.Overview, &deckOverviewView{DOMID: "theme-overview-" + domID(theme.Target)})
	for _, feature := range document.Features {
		if feature.Plan.Archived {
			continue
		}
		for _, ref := range theme.Features {
			if ref == feature.Target {
				view.Features = append(view.Features, featureLinkView{Plan: featurePresentationPlan(document, feature.Plan), ID: feature.ID, Title: feature.Title, Href: featureHref(feature.ID)})
				break
			}
		}
	}
	for _, deck := range theme.Decks {
		view.Decks = append(view.Decks, featureDeckView{Title: deck.Title, Role: deck.Role, Href: "/?view=slides#overview-" + domID(deck.Target)})
	}
	return view, nil
}

func featureThemesDirectory(document *saga.Saga, query string) *directoryView {
	view := &directoryView{ID: "feature-themes", Title: "Product themes", Action: "/feature-themes", Lede: "Optional categories explaining why related features belong together.", Label: "Filter product themes", Noun: "theme", Nouns: "themes", Columns: []directoryColumn{{Title: "Theme"}, {Title: "Features", Numeric: true}, {Title: "Rationale deck"}}, Empty: "No product themes yet.", Command: "change-saga feature-theme add"}
	for _, theme := range document.FeatureThemes {
		deck := gapCell("Not authored yet")
		if len(theme.Decks) > 0 {
			deck = linkCell(theme.Decks[0].Title, "/?view=slides#overview-"+domID(theme.Decks[0].Target))
		}
		view.addRow(directoryRow{Key: theme.ID, Cells: []directoryCell{{Text: theme.Title, Href: featureThemeHref(theme.ID), Target: applayout.FeatureThemeURN(document.Manifest.ID, theme.ID)}, countCell(len(theme.Features)), deck}})
	}
	view.apply(query)
	return view
}
