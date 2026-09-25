package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestRequirementsSurfaceProjectsStoriesAndCriteriaAsStableEntities(t *testing.T) {
	t.Parallel()
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
	if len(navigation.Children) != 1 || len(navigation.Children[0].Children) != 2 || !navigation.Children[0].Children[1].Active {
		t.Fatalf("requirements hierarchy = %#v", navigation.Children)
	}
	if got := navigation.Children[0].Children[1].Title; got != "AC 02 · A successful order has a confirmation." {
		t.Fatalf("criterion navigation title = %q", got)
	}
}

func TestRequirementsTemplatesGiveOverviewAndStoryDistinctPresentations(t *testing.T) {
	t.Parallel()
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
	overview.Groups = []requirementGroupView{{Feature: traceLink{Title: "Shop", Href: "/features/shop", Target: "urn:change-saga:test:feature:shop"}, Description: "Buying things.", Stories: overview.Stories}}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", overview); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), "Rationale") {
		t.Fatal("the requirements overview still titles the elevator pitch as its rationale")
	}
	for _, expected := range []string{"<h1>Requirements</h1>", `data-requirements-feature="urn:change-saga:test:feature:shop"`, `<a href="/features/shop">Shop</a>`, "Buying things.", "requirements-story-card", "1 criterion", "/requirements/checkout"} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("requirements overview missing %q: %s", expected, rendered.String())
		}
	}

	// A criterion has its own traceability view: its statement, what links
	// to it, what links to its story, and the story's other criteria.
	criterion, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "checkout", criterionID: "fast"})
	if err != nil {
		t.Fatal(err)
	}
	rendered.Reset()
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", criterion); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"data-criterion-page", "<h1>Checkout completes promptly.</h1>", "data-criterion-own-trace", "data-criterion-story-trace", `href="/requirements/checkout"`, storyURN + ":criterion:fast"} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("criterion view missing %q: %s", expected, rendered.String())
		}
	}

	detail, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	rendered.Reset()
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", detail); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Details", "Lifecycle", "Acceptance criteria", "data-story-context", "data-story-trace", storyURN + ":criterion:fast"} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("story detail missing %q: %s", expected, rendered.String())
		}
	}
	for _, excluded := range []string{"First-class product intent", "Immutable record", "Characterization", "requirement-story-id", "requirement-badges", "<footer>"} {
		if strings.Contains(rendered.String(), excluded) {
			t.Fatalf("initial story presentation contains extra chrome %q: %s", excluded, rendered.String())
		}
	}
	if strings.Contains(rendered.String(), `<details class="requirement-story-details" open`) {
		t.Fatalf("story details must be collapsed by default: %s", rendered.String())
	}
}

func TestRequirementsSurfaceRejectsUnknownStoryAndCriterionRoutes(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestRetiredRequirementsShowOnlyExplicitCurrentReplacements(t *testing.T) {
	t.Parallel()
	created := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	oldURN := "urn:change-saga:test:story:old-checkout"
	newURN := "urn:change-saga:test:story:new-checkout"
	document := requirements.Document{SagaID: "test", Stories: []requirements.Story{
		{
			Identity:         requirements.StoryIdentity{ID: "old-checkout", CreatedAt: created},
			CurrentRevision:  &requirements.Revision{ID: "r1", Story: oldURN, Title: "Old checkout", Statement: "As a buyer, I can use the old checkout.", AcceptanceCriteria: []requirements.Criterion{{ID: "confirmation", Statement: "The old flow confirms the order."}}},
			CurrentLifecycle: &requirements.LifecycleEvent{ID: "retired", Story: oldURN, State: requirements.StateRetired, Reason: "The new flow replaces it.", CreatedAt: created.Add(time.Hour)},
		},
		{
			Identity:         requirements.StoryIdentity{ID: "new-checkout", CreatedAt: created.Add(2 * time.Hour)},
			CurrentRevision:  &requirements.Revision{ID: "r1", Story: newURN, Title: "Current checkout", Statement: "As a buyer, I can use the current checkout.", AcceptanceCriteria: []requirements.Criterion{{ID: "receipt", Statement: "The current flow shows a receipt."}}},
			CurrentLifecycle: &requirements.LifecycleEvent{ID: "accepted", Story: newURN, State: requirements.StateAccepted},
		},
	}, Relations: []requirements.Relation{
		{ID: "new-replaces-old", Type: requirements.RelationSupersedes, From: newURN, To: oldURN, Rationale: "The current flow replaces the retired flow.", State: requirements.RelationActive},
		{ID: "receipt-replaces-confirmation", Type: requirements.RelationSupersedes, From: newURN + ":criterion:receipt", To: oldURN + ":criterion:confirmation", Rationale: "The receipt is the current confirmation contract.", State: requirements.RelationActive},
	}}

	page, navigation, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "old-checkout", criterionID: "confirmation"})
	if err != nil {
		t.Fatal(err)
	}
	newAppGraph(&saga.Saga{Section: &saga.Section{}}, document, quality.Document{}).decorateRequirements(page)
	if page.Story.Historical == nil || page.FocusedCriterion.Historical == nil {
		t.Fatalf("retired story and criterion were not marked historical: %#v", page.Story)
	}
	if got := page.Story.Historical.Replacements; len(got) != 1 || got[0].Target != newURN || got[0].Href != "/requirements/new-checkout" || got[0].Note != "accepted" {
		t.Fatalf("story replacements = %#v", got)
	}
	if got := page.FocusedCriterion.Historical.Replacements; len(got) != 1 || got[0].Target != newURN+":criterion:receipt" || got[0].Href != "/requirements/new-checkout/criteria/receipt" {
		t.Fatalf("criterion replacements = %#v", got)
	}
	if navigation.Children[0].Note != "retired" || navigation.Children[0].Children[0].Note != "historical" {
		t.Fatalf("historical navigation is not labelled: %#v", navigation.Children[0])
	}

	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", page); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Historical", "Retired acceptance criterion", "not current product intent", "Current replacement", "Current checkout", "The receipt is the current confirmation contract."} {
		if !strings.Contains(rendered.String(), expected) {
			t.Fatalf("historical criterion page missing %q: %s", expected, rendered.String())
		}
	}
	overview, _, err := makeRequirementsSurface(document, requirementRoute{active: true})
	if err != nil {
		t.Fatal(err)
	}
	overview.Groups = []requirementGroupView{{Feature: traceLink{Title: "Checkout", Href: "/features/checkout"}, Stories: overview.Stories}}
	rendered.Reset()
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", overview); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "requirements-story-card historical") || !strings.Contains(rendered.String(), "Historical · retired") {
		t.Fatalf("requirements overview does not mark retired stories: %s", rendered.String())
	}
}

func TestRetiredRequirementDoesNotInferReplacementFromItsReason(t *testing.T) {
	t.Parallel()
	oldURN := "urn:change-saga:test:story:old-checkout"
	document := requirements.Document{SagaID: "test", Stories: []requirements.Story{{
		Identity:         requirements.StoryIdentity{ID: "old-checkout"},
		CurrentRevision:  &requirements.Revision{ID: "r1", Story: oldURN, Title: "Old checkout", Statement: "As a buyer, I can use the old checkout."},
		CurrentLifecycle: &requirements.LifecycleEvent{ID: "retired", Story: oldURN, State: requirements.StateRetired, Reason: "Use new-checkout instead."},
	}}}
	page, _, err := makeRequirementsSurface(document, requirementRoute{active: true, storyID: "old-checkout"})
	if err != nil {
		t.Fatal(err)
	}
	newAppGraph(&saga.Saga{Section: &saga.Section{}}, document, quality.Document{}).decorateRequirements(page)
	if len(page.Story.Historical.Replacements) != 0 {
		t.Fatalf("replacement was inferred from prose: %#v", page.Story.Historical.Replacements)
	}
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := tmpl.ExecuteTemplate(&rendered, "requirements-page", page); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "No current replacement is explicitly linked in the Saga.") || strings.Contains(rendered.String(), `href="/requirements/new-checkout"`) {
		t.Fatalf("no-successor state is not truthful: %s", rendered.String())
	}
}
