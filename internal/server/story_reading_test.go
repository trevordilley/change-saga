package server

import (
	"bytes"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestStoryReadingPreservesAuthoredContentAndSafelyRendersOverview(t *testing.T) {
	title := "As a legacy author I need this title preserved"
	statement := "Old authored wording with no inferred persona.\nA second line."
	criterion := strings.Repeat("Full obligation with no truncation. ", 12)
	doc := requirements.Document{SagaID: "reading", Stories: []requirements.Story{{Identity: requirements.StoryIdentity{ID: "read"}, CurrentRevision: &requirements.Revision{ID: "r1", Title: title, Statement: statement, Overview: "## Purpose\n\n**Understand** the rationale.\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))", AcceptanceCriteria: []requirements.Criterion{{ID: "complete", Statement: criterion}}}}}}
	page, nav, err := makeRequirementsSurface(doc, requirementRoute{active: true, storyID: "read"})
	if err != nil {
		t.Fatal(err)
	}
	if nav.Children[0].Title != title || page.Story.Statement != statement {
		t.Fatal("authored need or title rewritten")
	}
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "requirements-page", page); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"<strong>Understand</strong>", statement, criterion, `<ul class="requirement-criteria-list">`, `<summary><span class="criterion-statement">`, `id="` + page.Story.Criteria[0].DOMID + `"`, `href="/requirements/read/criteria/complete"`} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, unsafe := range []string{"<script>", `href="javascript:`} {
		if strings.Contains(html, unsafe) {
			t.Fatalf("unsafe Markdown: %s", html)
		}
	}
	if strings.Index(html, "data-story-overview") > strings.Index(html, "requirement-criteria-list") || strings.Index(html, "requirement-criteria-list") > strings.Index(html, "requirement-story-details") {
		t.Fatal("narrative and criteria must precede secondary metadata")
	}
	if strings.Contains(html, `data-requirement-target="`+page.Story.Criteria[0].Target+`" open`) {
		t.Fatal("criteria should start collapsed")
	}
}

func TestCriterionReadingShowsAuthoredTitleAndSafeOverview(t *testing.T) {
	statement := "The forecast shows its observation date and time."
	doc := requirements.Document{SagaID: "reading", Stories: []requirements.Story{{Identity: requirements.StoryIdentity{ID: "read"}, CurrentRevision: &requirements.Revision{ID: "r1", Title: "Read forecasts", Statement: "I can read forecasts", AcceptanceCriteria: []requirements.Criterion{{ID: "fresh", Title: "Forecast freshness", Statement: statement, Overview: "## Why\n\nCheck **observation time**.\n\n<script>alert(1)</script>\n\n[bad](javascript:alert(1))"}}}}}}
	tmpl, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	for _, criterionID := range []string{"", "fresh"} {
		page, nav, err := makeRequirementsSurface(doc, requirementRoute{active: true, storyID: "read", criterionID: criterionID})
		if err != nil {
			t.Fatal(err)
		}
		if nav.Children[0].Children[0].Title != "AC 01 · Forecast freshness" {
			t.Fatalf("navigation: %+v", nav.Children[0].Children[0])
		}
		var output bytes.Buffer
		if err := tmpl.ExecuteTemplate(&output, "requirements-page", page); err != nil {
			t.Fatal(err)
		}
		html := output.String()
		for _, want := range []string{"Forecast freshness", statement, "<strong>observation time</strong>", "data-criterion-overview"} {
			if !strings.Contains(html, want) {
				t.Fatalf("missing %q on %q", want, criterionID)
			}
		}
		for _, unsafe := range []string{"<script>", `href="javascript:`} {
			if strings.Contains(html, unsafe) {
				t.Fatalf("unsafe criterion prose: %s", html)
			}
		}
	}
}
