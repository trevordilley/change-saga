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
	t.Parallel()
	root, repo, _ := dataModelFixture(t)
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	for _, tc := range []struct {
		path, current string
		open, shut    []string
	}{
		// The landing page opens Technical design over its three areas and
		// none of them.
		{"/technical", "/technical", []string{"nav-technical"}, []string{"nav-technical-erd", "nav-technical-systems", "nav-technical-components"}},
		{"/technical/erd", "/technical/erd", []string{"nav-technical", "nav-technical-erd"}, []string{"nav-technical-systems", "nav-technical-components"}},
		{"/technical/systems", "/technical/systems", []string{"nav-technical", "nav-technical-systems"}, []string{"nav-technical-erd", "nav-technical-components"}},
		{"/technical/components", "/technical/components", []string{"nav-technical", "nav-technical-components"}, []string{"nav-technical-erd", "nav-technical-systems"}},
		// A definition is current beneath its area, which opens around it.
		{"/technical/data-entity/pdf-report", "/technical/data-entity/pdf-report", []string{"nav-technical", "nav-technical-erd"}, []string{"nav-technical-systems"}},
		{"/technical/system/pdf-pipeline?revision=r1", "/technical/system/pdf-pipeline", []string{"nav-technical", "nav-technical-systems"}, []string{"nav-technical-erd"}},
		{"/technical/component/record-store", "/technical/component/record-store", []string{"nav-technical", "nav-technical-components"}, []string{"nav-technical-systems"}},
		// ERDs and overlays have no row of their own; the ERD area is current.
		{"/technical/erd/reporting", "/technical/erd", []string{"nav-technical-erd"}, nil},
		{"/technical/erd-overlay/pdf-jobs", "/technical/erd", []string{"nav-technical-erd"}, nil},
	} {
		status, body := technicalGet(t, mux, tc.path)
		if status != 200 {
			t.Fatalf("%s: %d", tc.path, status)
		}
		sidebar := technicalSidebar(t, body)
		if current := currentNav(sidebar); len(current) != 1 || current[0] != tc.current {
			t.Fatalf("%s: current rows %v, want %s", tc.path, current, tc.current)
		}
		for _, id := range tc.open {
			if !navChildren(t, sidebar, id) {
				t.Fatalf("%s: %s is shut", tc.path, id)
			}
			if !strings.Contains(sidebar, `aria-expanded="true" aria-controls="`+id+`"`) {
				t.Fatalf("%s: %s's disclosure does not say it is open", tc.path, id)
			}
		}
		for _, id := range tc.shut {
			if navChildren(t, sidebar, id) {
				t.Fatalf("%s: %s is open", tc.path, id)
			}
		}
	}

	_, body := technicalGet(t, mux, "/technical/erd")
	sidebar := technicalSidebar(t, body)
	// Areas in reading order, each a link that is also a disclosure.
	erd, systems, components := strings.Index(sidebar, `href="/technical/erd"`), strings.Index(sidebar, `href="/technical/systems"`), strings.Index(sidebar, `href="/technical/components"`)
	if erd < 0 || !(erd < systems && systems < components) {
		t.Fatalf("areas out of order: %d %d %d", erd, systems, components)
	}
	// Data entities beneath the ERD; its ERDs and overlays are reached from
	// the page, not listed as entities.
	entities := sidebar[strings.Index(sidebar, `id="nav-technical-erd"`):]
	entities = entities[:strings.Index(entities, `aria-controls="nav-technical-systems"`)]
	for _, want := range []string{`href="/technical/data-entity/pdf-report"`, `href="/technical/data-entity/pdf-job"`} {
		if !strings.Contains(entities, want) {
			t.Fatalf("ERD rows lost %s: %s", want, entities)
		}
	}
	for _, unwanted := range []string{`href="/technical/erd/`, `href="/technical/erd-overlay/`, `href="/technical/component/`} {
		if strings.Contains(entities, unwanted) {
			t.Fatalf("ERD rows list %s", unwanted)
		}
	}
	// The ERD page reaches its other views and overlays.
	for _, want := range []string{`data-technical-erd-views`, `href="/technical/erd/reporting"`, `href="/technical/erd-overlay/pdf-jobs"`, `href="/technical/erd/application">Revision`} {
		if !strings.Contains(body, want) {
			t.Fatalf("ERD page lost %s", want)
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
		"/technical/data-entity/pdf-job?revision=r1":    `<a href="/technical/erd">ERD</a>`,
		"/technical/erd/application":                    `<a href="/technical/erd">ERD</a>`,
		"/technical/erd-overlay/pdf-jobs":               `<a href="/technical/erd">ERD</a>`,
	} {
		status, body := technicalGet(t, mux, path)
		if status != 200 || !strings.Contains(body, `<a href="/technical">Technical design</a><span>/</span>`+crumb) {
			t.Fatalf("%s: %d, breadcrumb lost %s", path, status, crumb)
		}
	}
	// The single page's section fragments land on the matching area.
	_, body := technicalGet(t, mux, "/technical")
	for anchor, area := range map[string]string{"technical-systems-section": "systems", "technical-components-section": "components", "technical-data-model": "erd"} {
		if !strings.Contains(body, `id="`+anchor+`" data-technical-area="`+area+`"`) {
			t.Fatalf("old fragment #%s does not land on %s", anchor, area)
		}
	}
	for _, path := range []string{"/technical/entities", "/technical/system", "/technical/data-entity"} {
		if status, _ := technicalGet(t, mux, path); status != 404 {
			t.Fatalf("%s: %d", path, status)
		}
	}
}
