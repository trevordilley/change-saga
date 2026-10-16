package server

import (
	"bytes"
	"html/template"
	"regexp"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestEssayMarkdownDialectMatchesDeckBodies(t *testing.T) {
	source := "## Heading {#stable}\n\n## Repeated\n\n## Repeated\n\nA claim.[^proof]\n\n[^proof]: Evidence.\n\n<script>unsafe()</script>\n\n[bad](javascript:alert(1))\n\n`[literal](annotation:example)`\n"
	ordinary := string(markdownWithAnchors(source, "owner"))
	deck := string(overviewMarkdown(source, &deckOverviewView{DOMID: "owner"}))
	if ordinary != deck {
		t.Fatalf("deck and ordinary Markdown differ:\n%s\n%s", ordinary, deck)
	}
	for _, want := range []string{`id="owner--stable"`, `id="owner--repeated"`, `id="owner--repeated-1"`, `owner--fn:1`, "&lt;script&gt;", "annotation:example"} {
		if !strings.Contains(deck, want) {
			t.Errorf("missing %q: %s", want, deck)
		}
	}
	for _, unsafe := range []string{"<script>", `href="javascript:`, "{#stable}"} {
		if strings.Contains(deck, unsafe) {
			t.Errorf("unexpected %q: %s", unsafe, deck)
		}
	}
}

func TestEssayAbstractsScopeAnchorsToOwnerAndSlot(t *testing.T) {
	const prose = "## Summary {#summary}\n\nA claim.[^proof]\n\n[^proof]: Evidence.\n\n<script>unsafe()</script>"
	for _, tmpl := range []*template.Template{serverTemplate(t), reviewTemplates} {
		t.Run(tmpl.Name(), func(t *testing.T) { testDeckEssayNamespaces(t, tmpl, prose) })
	}
	tmpl := serverTemplate(t)
	for _, tc := range []struct {
		template string
		view     any
		owner    string
	}{
		{"feature-page", &featurePageView{Target: "urn:change-saga:demo:feature:next", Abstract: prose}, "urn:change-saga:demo:feature:next"},
		{"feature-theme-page", &featureThemePageView{Target: "urn:change-saga:demo:feature-theme:growth", Abstract: prose}, "urn:change-saga:demo:feature-theme:growth"},
		{"technical-page", &technicalPageView{Overview: &fragmentView{Fragment: &saga.Fragment{Target: "urn:change-saga:demo:fragment:technical", Abstract: prose}}}, "urn:change-saga:demo:fragment:technical"},
	} {
		t.Run(tc.template, func(t *testing.T) {
			var page bytes.Buffer
			if err := tmpl.ExecuteTemplate(&page, tc.template, tc.view); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(page.String(), `id="`+domID(tc.owner)+`--abstract--summary"`) {
				t.Fatalf("abstract owner missing: %s", page.String())
			}
		})
	}
	if got := essayAbstract("", "owner"); got != "" {
		t.Fatalf("empty abstract renders %q", got)
	}
}

func testDeckEssayNamespaces(t *testing.T, tmpl *template.Template, prose string) {
	t.Helper()
	var output bytes.Buffer
	for _, id := range []string{"first", "second"} {
		deck := overviewFixture()
		deck.Target = saga.DeckTarget("demo", id)
		deck.Overview.Abstract, deck.Overview.Body = prose, prose
		if err := tmpl.ExecuteTemplate(&output, "deck-overview", makeDeckOverview(deck)); err != nil {
			t.Fatal(err)
		}
	}
	html := output.String()
	ids := map[string]bool{}
	for _, match := range regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		if ids[match[1]] {
			t.Errorf("duplicate ID %q", match[1])
		}
		ids[match[1]] = true
	}
	for _, id := range []string{"first", "second"} {
		owner := domID(saga.DeckTarget("demo", id))
		for _, namespace := range []string{owner + "--abstract", "overview-" + owner} {
			for _, suffix := range []string{"--summary", "--fn:1", "--fnref:1"} {
				if !ids[namespace+suffix] {
					t.Errorf("missing ID %s", namespace+suffix)
				}
			}
		}
	}
	for _, match := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(html, -1) {
		if strings.Contains(match[1], "--fn") && !ids[match[1]] {
			t.Errorf("unresolved footnote link %s", match[1])
		}
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("unsafe abstract or body")
	}
}
