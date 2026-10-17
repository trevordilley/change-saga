package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestDeckSectionBoundariesInNavigationAndReading(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		labels, want []string
	}{
		{"unsectioned", []string{"", "", ""}, []string{"", "", ""}},
		{"named-first", []string{"Request", "Errors", "", "Request"}, []string{"Request", "Errors", "", "Request"}},
		{"runs", []string{"", " Request ", "Request", "Errors", "", "Errors", "Request", " ", "Request"}, []string{"", "Request", "", "Errors", "", "Errors", "Request", "", "Request"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			deck := &saga.Section{Kind: "deck", Target: saga.DeckTarget("test", "flow")}
			for i, label := range test.labels {
				id := fmt.Sprintf("slide-%d", i)
				deck.Fragments = append(deck.Fragments, &saga.Fragment{ID: id, Title: id, Target: saga.SlideTarget("test", id), SlideMeta: &saga.SlideManifest{Section: label}})
			}
			// Legacy fragments without metadata also end a named run.
			deck.Fragments[len(deck.Fragments)-2].SlideMeta = nil
			root := &saga.Section{Children: []*saga.Section{deck, deck}}
			nodes := makeDeckNavTree(root)
			reading := makeSectionView(root, viewScope{deferContent: true, backgroundFrames: true})
			for d, node := range nodes {
				var navigationLabels, readingLabels []string
				for i, child := range node.Children {
					navigationLabels = append(navigationLabels, child.Slide.Section)
					fragment := reading.ChildViews[d].FragmentViews[i]
					readingLabels = append(readingLabels, fragment.SectionTitle)
					if child.Slide.Target != deck.Fragments[i].Target || fragment.Fragment != deck.Fragments[i] || !fragment.Deferred || !fragment.BackgroundFrame {
						t.Fatal("section grouping changed identity, order, or deferred loading")
					}
				}
				if !reflect.DeepEqual(navigationLabels, test.want) || !reflect.DeepEqual(readingLabels, test.want) {
					t.Fatalf("navigation=%q reading=%q want=%q", navigationLabels, readingLabels, test.want)
				}
			}
		})
	}
}

func TestReviewSectionsUseSharedDividerWithoutAddingSlides(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	document, _, err := saga.Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("pr-7")
	original := review.Deck.Slides[0]
	labels := []string{"", "Request", "Request", "Errors", "", "Errors", "<Queue & owner>"}
	for i, label := range labels[1:] {
		slide := original.SlideManifest
		slide.ID, slide.Title, slide.Rank, slide.Section = fmt.Sprintf("extra-%d", i), fmt.Sprintf("Extra %d", i), i+1, label
		slide.ReadingOrder = nil
		name, _ := saga.FlatSlideFilename(review.Deck.Target, saga.ReviewSlideTarget("app", "pr-7", slide.ID), slide.Rank)
		slide.Entrypoint, _ = saga.FlatSlideAssetFilename(name, ".svg")
		writeServerFile(t, filepath.Join(review.Deck.Directory, slide.Entrypoint), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="5" height="5"/></svg>`)
		writeServerJSON(t, filepath.Join(review.Deck.Directory, name), slide)
	}
	if _, validation, err := saga.Load(fixture.root); err != nil || !validation.Valid {
		t.Fatalf("section fixture: %v %#v", err, validation.Issues)
	}
	_, handler := reviewApp(t, fixture, gitdiff.Range{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/reviews/pr-7", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("review: %d %s", response.Code, response.Body.String())
	}
	html := response.Body.String()
	// The same template, including HTML escaping, renders both deck contexts.
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var dividers bytes.Buffer
	for _, label := range []string{"Request", "Errors", "<Queue & owner>"} {
		dividers.Reset()
		if err := tmpl.ExecuteTemplate(&dividers, "slide-section-divider", label); err != nil {
			t.Fatal(err)
		}
		want := 1
		if label == "Errors" {
			want = 2
		}
		if got := strings.Count(html, dividers.String()); got != want {
			t.Fatalf("divider %q count=%d want=%d", label, got, want)
		}
	}
	for _, marker := range []string{"data-slide-thumbnail-card", "data-deck-slide data-deck-target="} {
		if got := strings.Count(html, marker); got != len(labels) {
			t.Fatalf("%s count=%d want=%d", marker, got, len(labels))
		}
	}
	for _, marker := range []string{`id="` + domID(original.Target) + `"`, `id="` + domID(original.Items[0].Target) + `"`, `data-review-approve`, `data-review-annotation-toggle`, `data-review-comment-form`, `data-deck-overview`} {
		if !strings.Contains(html, marker) {
			t.Fatalf("sectioned review lost %s", marker)
		}
	}
}
