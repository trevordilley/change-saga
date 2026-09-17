package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestRequirementsSurfaceProjectsStoriesAndCriteriaAsStableEntities(t *testing.T) {
	created := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	storyURN := "urn:change-saga:test:story:checkout"
	revisionURN := storyURN + ":revision:r1"
	document := requirements.Document{SagaID: "test", Stories: []requirements.Story{{
		Identity: requirements.StoryIdentity{ID: "checkout", CreatedAt: created},
		CurrentRevision: &requirements.Revision{
			ID: "r1", Story: storyURN, Title: "Complete checkout", Statement: "As a buyer, I can complete checkout without losing my cart.", Priority: "must",
			AcceptanceCriteria: []requirements.Criterion{{ID: "preserve-cart", Statement: "The cart remains intact after a retry."}, {ID: "confirm-order", Statement: "A successful order has a confirmation."}},
		},
		CurrentLifecycle: &requirements.LifecycleEvent{ID: "proposed", Story: storyURN, State: requirements.StateProposed},
		RevisionHeads:    []string{revisionURN}, LifecycleHeads: []string{storyURN + ":event:proposed"},
	}}}

	page, navigation, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "checkout", criterionID: "confirm-order"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Overview || page.Story == nil || page.FocusedCriterion == nil {
		t.Fatalf("focused requirement page = %#v", page)
	}
	if page.Story.Target != storyURN || page.Story.RevisionTarget != revisionURN {
		t.Fatalf("story identities = target %q revision %q", page.Story.Target, page.Story.RevisionTarget)
	}
	wantCriterion := storyURN + ":criterion:confirm-order"
	if page.FocusedCriterion.Target != wantCriterion || !page.FocusedCriterion.Selected {
		t.Fatalf("criterion identity = %#v", page.FocusedCriterion)
	}
	if navigation.Title != "Requirements" || navigation.Icon != "requirements" || !navigation.Expanded {
		t.Fatalf("requirements root navigation = %#v", navigation)
	}
	if len(navigation.Children) != 2 || len(navigation.Children[1].Children) != 2 || !navigation.Children[1].Children[1].Active {
		t.Fatalf("requirements hierarchy = %#v", navigation.Children)
	}
}

func TestRequirementsTemplatesGiveOverviewAndStoryDistinctPresentations(t *testing.T) {
	storyURN := "urn:change-saga:test:story:checkout"
	document := requirements.Document{SagaID: "test", Stories: []requirements.Story{{
		Identity: requirements.StoryIdentity{ID: "checkout", CreatedAt: time.Unix(1, 0)},
		CurrentRevision: &requirements.Revision{
			ID: "r1", Story: storyURN, Title: "Complete checkout", Statement: "As a buyer, I can check out.", Priority: "must",
			AcceptanceCriteria: []requirements.Criterion{{ID: "fast", Statement: "Checkout completes promptly."}},
		},
		CurrentLifecycle: &requirements.LifecycleEvent{State: requirements.StateProposed},
	}}}
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}

	overview, _, err := makeRequirementsSurface(document, requirementRoute{active: true})
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", overview); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"First-class product intent", "requirements-story-card", "acceptance criterion", "/requirements/checkout"} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("requirements overview missing %q: %s", expected, rendered.String())
		}
	}

	detail, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "checkout", criterionID: "fast"})
	if err != nil {
		t.Fatal(err)
	}
	rendered.Reset()
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", detail); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Story details", "Characterization", "Acceptance criteria", "requirement-criterion selected", storyURN + ":criterion:fast"} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("story detail missing %q: %s", expected, rendered.String())
		}
	}
	for _, excluded := range []string{"requirement-story-id", "requirement-badges", "<footer>"} {
		if strings.Contains(rendered.String(), excluded) {
			t.Fatalf("initial story presentation contains extra chrome %q: %s", excluded, rendered.String())
		}
	}
	if strings.Contains(rendered.String(), `<details class="requirement-story-details" open`) {
		t.Fatalf("story details must be collapsed by default: %s", rendered.String())
	}
}

func TestRequirementsSurfaceRejectsUnknownStoryAndCriterionRoutes(t *testing.T) {
	document := requirements.Document{SagaID: "test"}
	if _, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "missing"}); err != errRequirementNotFound {
		t.Fatalf("unknown story error = %v", err)
	}
	storyURN := "urn:change-saga:test:story:checkout"
	document.Stories = []requirements.Story{{
		Identity:        requirements.StoryIdentity{ID: "checkout"},
		CurrentRevision: &requirements.Revision{ID: "r1", Story: storyURN, Title: "Checkout", AcceptanceCriteria: []requirements.Criterion{{ID: "known", Statement: "Known"}}},
	}}
	if _, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "checkout", criterionID: "missing"}); err != errRequirementNotFound {
		t.Fatalf("unknown criterion error = %v", err)
	}
}

func TestRequirementsPathDoesNotCaptureOrdinaryRoutesWithTheSamePrefix(t *testing.T) {
	for _, value := range []string{"/requirements", "/requirements/checkout", "/requirements/checkout/criteria/fast"} {
		if !isRequirementsPath(value) {
			t.Fatalf("requirements path %q was not recognized", value)
		}
	}
	for _, value := range []string{"/", "/requirements-old", "/requirement"} {
		if isRequirementsPath(value) {
			t.Fatalf("ordinary path %q was captured as requirements", value)
		}
	}
}
