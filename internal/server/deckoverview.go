package server

import (
	"bytes"
	"html/template"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type deckOverviewView struct {
	Target, Title, DOMID, Objective string
	Body                            template.HTML
	Generated                       bool
	References                      []overviewReferenceView
	Slides                          []overviewSlideView
	Visuals                         map[string]string
}

type overviewSlideView struct {
	Title, Anchor string
	Bullets       []string
}

type overviewReferenceView struct {
	ID, DOMID, Label, Title, Anchor, SlideTarget, ItemTarget, Problem string
	Details                                                           []string
}

// Front is prose about the existing visual, never another evidence owner.
func slideFront(meta *saga.SlideManifest, labels []string) []string {
	slide := &saga.Slide{}
	if meta != nil {
		slide.SlideManifest = *meta
	}
	for _, label := range labels {
		slide.Items = append(slide.Items, &saga.Item{ItemManifest: saga.ItemManifest{Label: label}})
	}
	return slide.FrontBullets()
}

func makeDeckOverview(deck *saga.Deck, reviewID ...string) *deckOverviewView {
	if deck == nil {
		return nil
	}
	view := &deckOverviewView{Target: deck.Target, Title: deck.Title, DOMID: "overview-" + domID(deck.Target), Objective: deck.Objective, Generated: deck.Overview == nil, Visuals: map[string]string{}}
	for _, slide := range deck.Slides {
		if strings.HasPrefix(slide.MediaType, "image/") {
			asset := "/f/" + url.PathEscape(slide.ID) + "/" + strings.Join(pathEscapeParts(slide.Entrypoint), "/")
			if len(reviewID) > 0 {
				asset = reviewHref(reviewID[0]) + "/visual/" + url.PathEscape(slide.ID)
			}
			view.Visuals[slide.ID], view.Visuals[slide.Target] = asset, asset
		}
		view.Slides = append(view.Slides, overviewSlideView{Title: slide.Title, Anchor: domID(slide.Target), Bullets: slide.FrontBullets()})
	}
	if deck.Overview == nil {
		return view
	}
	for _, annotation := range deck.Overview.Annotations {
		ref := overviewReferenceView{ID: annotation.ID, DOMID: view.DOMID + "-ref-" + annotation.ID, Label: annotation.Label}
		slide, item, err := deck.ResolveOverviewAnnotation(annotation)
		if err != nil {
			ref.Problem = "This reference does not resolve to a slide or item in this deck."
		} else {
			ref.Title, ref.Anchor, ref.SlideTarget = slide.Title, domID(slide.Target), slide.Target
			if item != nil {
				ref.Title += " · " + item.Label
				ref.Anchor, ref.ItemTarget = domID(item.Target), item.Target
				ref.Details = itemOverviewDetails(item)
			} else {
				for _, item := range slide.Items {
					ref.Details = append(ref.Details, item.Label)
					ref.Details = append(ref.Details, itemOverviewDetails(item)...)
				}
			}
		}
		if ref.Label == "" {
			ref.Label = annotation.ID
		}
		view.References = append(view.References, ref)
	}
	view.Body = overviewMarkdown(deck.Overview.Body, view)
	return view
}

func itemOverviewDetails(item *saga.Item) []string {
	var details []string
	if item.Description != "" {
		details = append(details, item.Description)
	}
	if item.Record != "" {
		details = append(details, "Affected documentation: "+recordLabel(item.Record))
	}
	if item.Documentation != nil {
		details = append(details, "Linked definition: "+item.Documentation.Target)
	}
	seen := map[string]bool{}
	for _, file := range item.Code {
		for _, ref := range file.References {
			if !seen[ref.Path] {
				seen[ref.Path] = true
				if len(seen) <= 8 {
					details = append(details, "Linked code: "+ref.Path)
				}
			}
		}
	}
	if len(seen) > 8 {
		details = append(details, "More linked files are available in the evidence drawer.")
	}
	if len(item.Code) == 0 {
		if item.HasCode {
			details = append(details, "Linked code loads when opened.")
		} else {
			details = append(details, "No linked code.")
		}
	}
	return details
}

// Resolve citations in the Markdown syntax tree. Code spans and fenced examples
// stay literal; raw HTML and unsafe URLs retain the ordinary Markdown policy.
func overviewMarkdown(body string, view *deckOverviewView) template.HTML {
	engine := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.NewFootnote(extension.WithFootnoteIDPrefix(view.DOMID+"--"))), goldmark.WithParserOptions(parser.WithAutoHeadingID()), goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(&headingRenderer{namespace: view.DOMID}, 100))))
	source := []byte(body)
	document := engine.Parser().Parse(text.NewReader(source))
	ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if visual, ok := node.(*ast.Image); ok && entering && strings.HasPrefix(string(visual.Destination), "slide:") {
			asset := view.Visuals[strings.TrimPrefix(string(visual.Destination), "slide:")]
			if asset == "" {
				node.Parent().ReplaceChild(node.Parent(), node, ast.NewString([]byte("Unavailable overview visual.")))
				return ast.WalkSkipChildren, nil
			}
			visual.Destination = []byte(asset)
		}
		link, ok := node.(*ast.Link)
		if !ok || !entering || !strings.HasPrefix(string(link.Destination), "annotation:") {
			return ast.WalkContinue, nil
		}
		id := strings.TrimPrefix(string(link.Destination), "annotation:")
		anchor := view.DOMID + "-ref-" + id
		found := false
		for _, ref := range view.References {
			if ref.ID == id {
				found = true
				break
			}
		}
		if !found {
			view.References = append(view.References, overviewReferenceView{ID: id, DOMID: anchor, Label: id, Problem: "This reference has no annotation."})
		}
		link.Destination = []byte("#" + anchor)
		return ast.WalkContinue, nil
	})
	var output bytes.Buffer
	if err := engine.Renderer().Render(&output, source, document); err != nil {
		return "Unable to render overview."
	}
	return template.HTML(output.String()) // #nosec G203 -- goldmark safe renderer.
}

func decorateOverviewStories(view *deckOverviewView, slides []*fragmentView) {
	if view == nil {
		return
	}
	for index := range view.References {
		ref := &view.References[index]
		for _, slide := range slides {
			if slide.Target != ref.SlideTarget {
				continue
			}
			stories := slide.Stories
			if ref.ItemTarget != "" {
				for _, item := range slide.LandmarkViews {
					if item.Target == ref.ItemTarget {
						stories = item.Stories
						ref.Anchor = item.DOMID
						break
					}
				}
			}
			if stories != nil {
				for _, story := range stories.Links {
					ref.Details = append(ref.Details, "Story: "+story.Story)
				}
				if stories.Count == 0 {
					ref.Details = append(ref.Details, "No item-level story links.")
				}
			}
		}
	}
}

const deckOverviewTemplates = `
{{define "deck-face-controls"}}<nav class="deck-face-controls" aria-label="Deck view"><button type="button" data-deck-face-button="overview" aria-pressed="false">Overview</button><button type="button" data-deck-face-button="front" aria-pressed="false">Front</button><button type="button" data-deck-face-button="back" aria-pressed="true">Back</button></nav><div data-slide-viewed-host></div>{{end}}
{{define "slide-front"}}<section class="slide-front" data-slide-front hidden aria-label="Slide summary"><h2>{{.Title}}</h2>{{if .Bullets}}<ul>{{range .Bullets}}<li>{{.}}</li>{{end}}</ul>{{else}}<p>No summary has been authored for this slide.</p>{{end}}</section>{{end}}
{{define "deck-overview"}}{{with .}}<article class="deck-overview" data-deck-overview="{{.Target}}" id="{{.DOMID}}" hidden tabindex="-1" aria-label="{{.Title}} overview"><header><p class="eyebrow">Overview</p><h2>{{.Title}}</h2>{{if .Objective}}<p>{{.Objective}}</p>{{end}}</header>{{if .Generated}}<p class="overview-generated" data-overview-generated>Generated slide directory — this deck has no authored overview.</p><ol class="overview-directory">{{range .Slides}}<li><a href="#{{.Anchor}}" data-overview-slide>{{.Title}}</a>{{if .Bullets}}<ul>{{range .Bullets}}<li>{{.}}</li>{{end}}</ul>{{end}}</li>{{else}}<li>No slides yet.</li>{{end}}</ol>{{else}}<div class="fragment-markdown overview-report">{{.Body}}</div>{{end}}{{if .References}}<section class="overview-references" aria-label="Overview references"><h3>References</h3><ul>{{range .References}}<li id="{{.DOMID}}" data-overview-reference data-overview-slide-target="{{.SlideTarget}}" data-overview-item-target="{{.ItemTarget}}" data-overview-anchor="{{.Anchor}}"{{if .Problem}} data-overview-invalid{{end}}>{{if .Problem}}<span>{{.Label}}</span><p class="overview-reference-problem">{{.Problem}}</p>{{else}}<a href="#{{.Anchor}}" data-overview-open>{{.Label}}</a><div class="overview-reference-detail"><strong>{{.Title}}</strong>{{range .Details}}<p>{{.}}</p>{{end}}<span class="overview-evidence-controls" data-overview-evidence-controls></span></div>{{end}}</li>{{end}}</ul></section>{{end}}<p class="overview-evidence-note">References open the linked slide or item’s existing evidence.</p><div class="overview-preview" data-overview-preview role="region" aria-label="Evidence preview" hidden></div></article>{{end}}{{end}}
`
