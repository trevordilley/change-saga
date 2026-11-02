package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// recordLinkPrefix is where an essay's link to a record goes. Markdown renders
// without the document, and a link should still say what it names, so the
// record's page is worked out when the reader follows it.
const (
	recordURNPrefix  = "urn:change-saga:"
	recordLinkPrefix = "/records/"
)

// recordLinkTransformer turns [text](urn:change-saga:...) into a link that opens
// the record. The browser could do nothing with the URN itself.
type recordLinkTransformer struct{}

func (recordLinkTransformer) Transform(document *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if link, ok := node.(*ast.Link); ok && entering && strings.HasPrefix(string(link.Destination), recordURNPrefix) {
			link.Destination = []byte(recordLinkPrefix + url.PathEscape(string(link.Destination)))
		}
		return ast.WalkContinue, nil
	})
}

func recordLinkTransformerOption() parser.Option {
	return parser.WithASTTransformers(util.Prioritized(recordLinkTransformer{}, 100))
}

// followRecordLink sends the reader to a record's page. A record with no page
// here, including one in another Saga, says so rather than opening a wrong one.
func (a *app) followRecordLink(w http.ResponseWriter, r *http.Request) {
	document := a.loadReviewDocument(w)
	if document == nil {
		return
	}
	urn := r.PathValue("urn")
	href := a.recordPage(r.Context(), document, urn)
	if href == "" {
		http.Error(w, "This Saga has no page for "+urn+".", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, href, http.StatusFound)
}

// recordPage is the page a record opens on, or empty if it has none.
func (a *app) recordPage(ctx context.Context, document *saga.Saga, urn string) string {
	rest, ok := strings.CutPrefix(urn, recordURNPrefix+document.Manifest.ID+":")
	if !ok {
		return ""
	}
	parts := strings.Split(rest, ":")
	if len(parts)%2 != 0 {
		return ""
	}
	kind, id := parts[0], parts[1]
	nested := len(parts) > 2
	switch kind {
	case "story", "persona", "term":
		records, err := a.sagaFiles(ctx).records(document.Manifest.ID)
		if err != nil {
			return ""
		}
		switch kind {
		case "story":
			story := records.FindStory(id)
			if story == nil {
				return ""
			}
			if nested {
				if parts[2] != "criterion" || len(parts) != 4 || story.CurrentRevision == nil {
					return ""
				}
				for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
					if criterion.ID == parts[3] {
						return "/requirements/" + url.PathEscape(id) + "/criteria/" + url.PathEscape(parts[3])
					}
				}
				return ""
			}
			return recordHref(document, urn)
		case "persona":
			if !nested && records.FindPersona(id) != nil {
				return "/personas/" + url.PathEscape(id)
			}
		case "term":
			if !nested && records.FindTerm(id) != nil {
				return termHref(id)
			}
		}
		return ""
	case "feature":
		if !nested && document.FindFeature(id) != nil {
			return featureHref(id)
		}
	case "feature-theme":
		if !nested && document.FindFeatureTheme(id) != nil {
			return featureThemeHref(id)
		}
	case "plan":
		plan := document.FindPlan(id)
		if plan == nil {
			return ""
		}
		if !nested {
			return planHref(id)
		}
		if len(parts) == 4 && parts[2] == "task" && plan.FindTask(parts[3]) != nil {
			return taskHref(id, parts[3])
		}
		return planHref(id)
	case "review":
		if document.FindReview(id) != nil {
			return reviewHref(id)
		}
	case "prototype":
		if !nested {
			if records, err := prototypes.Load(document.Root, document.Manifest.ID); err == nil && records.Find(id) != nil {
				return prototypeHref(id)
			}
		}
	case "test-case":
		if !nested {
			if tests, err := a.sagaFiles(ctx).tests(); err == nil {
				for _, test := range tests.TestCases {
					if test.Identity.ID == id {
						return testCaseHref(id)
					}
				}
			}
		}
	case "deck", "slide":
		for _, deck := range document.Decks {
			if deck.Target == urn {
				return recordHref(document, urn)
			}
			for _, slide := range deck.Slides {
				if slide.Target == urn {
					return recordHref(document, urn)
				}
			}
		}
	}
	return ""
}
