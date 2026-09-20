package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func TestV3DesignChapterAndFragmentEnterExistingRenderTree(t *testing.T) {
	root := filepath.Join(t.TempDir(), "render-design.saga")
	writeDesignTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"render-design","title":"Render design","source":{"repository":"https://example.test/acme/app.git"}}`)
	writeServerEpic(t, root)
	writeDesignTestFile(t, filepath.Join(serverEpicDir(root), "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeDesignTestFile(t, filepath.Join(serverEpicDir(root), "overview.fragment", "content.md"), "Narrative overview.\n")
	writeDesignTestFile(t, filepath.Join(serverEpicDir(root), "___design", "architecture.chapter", "chapter.json"), `{"version":2,"id":"architecture","title":"Technical architecture"}`)
	writeDesignTestFile(t, filepath.Join(serverEpicDir(root), "___design", "architecture.chapter", "sequence.fragment", "fragment.json"), `{"version":2,"id":"sequence","title":"Request sequence","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeDesignTestFile(t, filepath.Join(serverEpicDir(root), "___design", "architecture.chapter", "sequence.fragment", "content.md"), "# Request sequence {#request-sequence}\n\nRenderable design content.\n")

	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("LoadNarrative = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	// A ___design chapter is technical design, not narrative: it leaves the
	// chapter list and reappears under Design > Technical.
	nav := makeNavTree(document.Section)
	if len(nav) != 1 || nav[0].Title != "Overview" {
		t.Fatalf("narrative navigation still carries the design chapter: %#v", nav)
	}
	technical := makeDesignChapterNav(document.Section)
	if len(technical) != 1 || technical[0].Title != "Technical architecture" || technical[0].Href != sagaHref(saga.ChapterTarget("render-design", "architecture")) {
		t.Fatalf("design navigation = %#v", technical)
	}
	// In the app-level list the design chapter joins its epic's Technical.
	app := makeAppNavTree(appNavSources{document: document, page: &requirementsPageView{}, pageEpic: serverEpic})
	epicTechnical := findNav(t, app, "Epics", "Core", "Design", "Technical").Children
	if got := topTitles(epicTechnical); got != "ERD|System|Data Flows|Technical architecture" {
		t.Fatalf("epic technical design = %v", got)
	}
	if got := topTitles(findNav(t, app, "Epics", "Core").Children); got != "Overview|Product|Design|Quality|Implementation" {
		t.Fatalf("the design chapter left the epic's Design: %s", got)
	}
	view := makeSectionView(document.Section, viewScope{})
	if len(view.ChildViews) != 1 || len(view.ChildViews[0].FragmentViews) != 1 {
		t.Fatalf("design render tree = %#v", view.ChildViews)
	}
	fragment := view.ChildViews[0].FragmentViews[0]
	if fragment.Target != saga.FragmentTarget("render-design", "sequence") || fragment.Markdown == "" {
		t.Fatalf("rendered design fragment = %#v", fragment)
	}
}

func writeDesignTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
