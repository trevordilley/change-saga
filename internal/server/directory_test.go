package server

import (
	"net/http"
	"strings"
	"testing"
)

// Every section header opens a page. The rule this file holds is the one the
// old sidebar broke: a header that only expands makes a reader click twice to
// reach a page that already exists, and in the case of the vocabulary it made
// them click thirty-five times to read a table.

func TestEverySectionHeaderIsADestination(t *testing.T) {
	sources := appNavFixture(t)
	nodes := makeAppNavTree(sources)
	for _, want := range []struct {
		path []string
		href string
	}{
		{[]string{"Overview"}, sagaHref(sources.document.Section.Target)},
		{[]string{"Overview", "Terms and vocabulary"}, "/terms"},
		{[]string{"Overview", "Personas"}, "/personas"},
		{[]string{"Overview", "Design system"}, designSystemPath},
		{[]string{"Features"}, "/features"},
		{[]string{"Features", "Billing", "Product"}, featureHref("billing") + "#feature-product"},
		{[]string{"Features", "Billing", "Design"}, featureHref("billing") + "#feature-design"},
		{[]string{"Features", "Billing", "Quality"}, featureHref("billing") + "#feature-quality"},
	} {
		if got := findNav(t, nodes, want.path...).Href; got != want.href {
			t.Fatalf("%v opens %q, want %q", want.path, got, want.href)
		}
	}
	// A deck's header opens the deck, at the slide it starts on.
	onboarding := findNav(t, nodes, "Overview", "Onboarding")
	if len(onboarding.Children) == 0 || onboarding.Href != onboarding.Children[0].Href {
		t.Fatalf("Onboarding opens %q, want its first slide %#v", onboarding.Href, onboarding.Children)
	}
	implementation := findNav(t, nodes, "Features", "Billing", "Implementation")
	if len(implementation.Children) == 0 || implementation.Href != implementation.Children[0].Href {
		t.Fatalf("Implementation opens %q, want its first slide", implementation.Href)
	}
	// A feature with no deck has no first slide to open, and says so instead.
	empty := appNavFixture(t)
	empty.pageFeature = "catalog"
	catalog := findNav(t, makeAppNavTree(empty), "Features", "Catalog", "Implementation")
	if catalog.Href != "" {
		t.Fatalf("an empty Implementation opens %q, want nowhere", catalog.Href)
	}
}

// A directory is a real table a reader can filter, and the same filter runs
// on the server for a browser that cannot run the enhanced one.
func TestADirectoryFiltersOnTheTextItShows(t *testing.T) {
	_, records, _ := dogfoodRecords(t)
	all := personasDirectory(records, "")
	if all.Total < 2 || all.Matches() != all.Total {
		t.Fatalf("unfiltered personas = %d of %d", all.Matches(), all.Total)
	}
	if got := all.Caption(); got != plural(all.Total, "persona", "personas") {
		t.Fatalf("caption = %q", got)
	}
	// The filter matches the text a row shows, and hides the rest rather than
	// dropping them, so the reader can widen it again.
	filtered := personasDirectory(records, strings.ToUpper(all.Rows[0].Cells[0].Text))
	if filtered.Total != all.Total || len(filtered.Rows) != len(all.Rows) {
		t.Fatalf("filtering dropped rows: %d of %d", len(filtered.Rows), filtered.Total)
	}
	if filtered.Matches() == 0 || filtered.Matches() == filtered.Total {
		t.Fatalf("filter matched %d of %d rows", filtered.Matches(), filtered.Total)
	}
	if !strings.Contains(filtered.Caption(), "of") {
		t.Fatalf("a filtered caption must say how much is in view: %q", filtered.Caption())
	}
	none := personasDirectory(records, "nothing names this")
	if none.Matches() != 0 {
		t.Fatalf("an impossible filter matched %d rows", none.Matches())
	}
}

// Each directory is a table with headers, carries its counts and no verdict,
// and states a missing link as growth with the command that acts on it.
func TestEachDirectoryRendersAsATableOfCounts(t *testing.T) {
	for _, page := range []struct{ path, id, column string }{
		{"/terms", "terms", `<th scope="col">Defined in code</th>`},
		{"/personas", "personas", `<th scope="col" class="numeric">Stories served</th>`},
		{"/features", "features", `<th scope="col" class="numeric">Accepted</th>`},
	} {
		body := dogfoodOK(t, page.path)
		for _, want := range []string{
			`<table class="directory-table" id="` + page.id + `-table"`,
			`<caption data-directory-caption>`,
			page.column,
			`data-directory-filter`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("GET %s is not a filterable table: missing %q", page.path, want)
			}
		}
		for _, verdict := range []string{"score", "passing", "failing", "complete "} {
			if strings.Contains(strings.ToLower(body), "coverage "+verdict) {
				t.Fatalf("GET %s passes a verdict: %q", page.path, verdict)
			}
		}
	}
	// This repository records no feature flags and no reviews yet, so those
	// directories state the growth and name the command rather than rendering
	// an empty table.
	for _, want := range []struct{ path, growth, command string }{
		{"/flags", "No feature flags yet.", "change-saga flag add"},
		{"/reviews", "No reviews yet.", "change-saga review create"},
	} {
		body := dogfoodOK(t, want.path)
		if !strings.Contains(body, want.growth) || !strings.Contains(body, "<code>"+want.command+"</code>") {
			t.Fatalf("GET %s must state the growth and the command that acts on it", want.path)
		}
	}
}

// The server-side filter is the whole of the no-JavaScript path: ?q= comes
// back with the rows that do not match already hidden.
func TestTheServerAppliesTheFilterItWasGiven(t *testing.T) {
	body := dogfoodOK(t, "/terms?q=persona")
	if !strings.Contains(body, `<tr hidden data-directory-row=`) {
		t.Fatal("?q= must hide the rows it rules out")
	}
	if !strings.Contains(body, `value="persona"`) {
		t.Fatal("?q= must come back in the filter field")
	}
	if !strings.Contains(body, `class="directory-filter-clear" href="/terms"`) {
		t.Fatal("a filtered directory must offer a way back to all of it")
	}
}

// /personas used to answer 404 while /personas/{id} answered: the section had
// records but no directory. Every app-level section has one now.
func TestEveryAppSectionAnswersAtItsOwnPath(t *testing.T) {
	for _, path := range []string{"/", "/terms", "/personas", "/flags", "/features", designSystemPath} {
		if code, body := dogfoodPage(t, path); code != http.StatusOK {
			t.Fatalf("GET %s = %d\n%s", path, code, body)
		}
	}
}

// The overview is the one section that is prose. It still says what the app
// is made of, and how much of each part there is.
func TestTheOverviewNamesItsPartsWithTheirCounts(t *testing.T) {
	body := dogfoodOK(t, "/")
	for _, part := range []string{"Personas", "Terms and vocabulary", "Design system", "Onboarding", "Feature flags"} {
		if !strings.Contains(body, `data-overview-part="`+part+`"`) {
			t.Fatalf("the overview does not name %s", part)
		}
	}
	_, records, _ := dogfoodRecords(t)
	for _, count := range []string{plural(len(records.Terms), "term", "terms"), plural(len(records.Personas), "persona", "personas")} {
		if !strings.Contains(body, `<span class="overview-part-count">`+count+`</span>`) {
			t.Fatalf("the overview does not count %q", count)
		}
	}
	// An empty part states its growth and the command, never a failure.
	if !strings.Contains(body, "No feature flags yet. Run change-saga flag add") {
		t.Fatal("an empty overview part must state the command that fills it")
	}
}
