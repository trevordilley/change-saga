package server

import (
	"regexp"
	"strings"
	"testing"
)

// technicalSidebar is the sidebar of a rendered page, so a link in the page
// body is never mistaken for a navigation row.
func technicalSidebar(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `<aside class="sidebar"`)
	end := strings.Index(body[start:], `</aside>`)
	if start < 0 || end < 0 {
		t.Fatalf("no sidebar: %s", body)
	}
	return body[start : start+end]
}

// navChildren reports whether the children of a navigation row are shown.
func navChildren(t *testing.T, sidebar, id string) bool {
	t.Helper()
	match := regexp.MustCompile(`<div class="doc-children" id="` + regexp.QuoteMeta(id) + `"( hidden)?>`).FindStringSubmatch(sidebar)
	if match == nil {
		t.Fatalf("no navigation row %s: %s", id, sidebar)
	}
	return match[1] == ""
}

// currentNav is every sidebar link marked as the current page.
func currentNav(sidebar string) []string {
	var current []string
	for _, match := range regexp.MustCompile(`<a class="doc-link" href="([^"]*)"[^>]*aria-current="page"`).FindAllStringSubmatch(sidebar, -1) {
		current = append(current, match[1])
	}
	return current
}

func TestTechnicalDesignSidebarNestsItsAreas(t *testing.T) {
	root, repo, _ := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for _, path := range []string{"/technical", "/technical/erd", "/technical/whiteboards/infrastructure", "/technical/whiteboards/data-flows"} {
		status, body := technicalGet(t, mux, path)
		if status != 200 {
			t.Fatalf("%s: %d", path, status)
		}
		sidebar := technicalSidebar(t, body)
		if current := currentNav(sidebar); len(current) != 1 || current[0] != path {
			t.Fatalf("%s current %v", path, current)
		}
		if !navChildren(t, sidebar, "nav-technical") {
			t.Fatal("technical design is collapsed")
		}
		erd, infra, flows := strings.Index(sidebar, `href="/technical/erd"`), strings.Index(sidebar, `href="/technical/whiteboards/infrastructure"`), strings.Index(sidebar, `href="/technical/whiteboards/data-flows"`)
		if erd < 0 || !(erd < infra && infra < flows) {
			t.Fatal("wrong board order")
		}
		for _, unwanted := range []string{`href="/technical/systems"`, `href="/technical/components"`, `href="/technical/data-entity/`} {
			if strings.Contains(sidebar, unwanted) {
				t.Errorf("old directory %s", unwanted)
			}
		}
	}
}

// Slides, drawers and reviews link to definition pages, and readers
// bookmarked the single page's sections, so both keep working.
func TestTechnicalDesignKeepsItsOldAddresses(t *testing.T) {
	t.Parallel()
	root, repo, _ := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for path, crumb := range map[string]string{
		"/technical/component/record-store?revision=r1": `<a href="/technical/components">Components</a>`,
		"/technical/system/pdf-pipeline":                `<a href="/technical/systems">Systems</a>`,
		"/technical/data-entity/pdf-job?revision=r1":    `<a href="/technical/erd">Data model</a>`,
		"/technical/erd/application":                    `<a href="/technical/erd">Data model</a>`,
		"/technical/erd-overlay/pdf-jobs":               `<a href="/technical/erd">Data model</a>`,
	} {
		status, body := technicalGet(t, mux, path)
		if status != 200 || !strings.Contains(body, `<a href="/technical">Technical design</a><span>/</span>`+crumb) {
			t.Fatalf("%s: %d, breadcrumb lost %s", path, status, crumb)
		}
	}
	for _, path := range []string{"/technical/entities", "/technical/system", "/technical/data-entity"} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Fatalf("%s: %d", path, status)
		}
	}
}
