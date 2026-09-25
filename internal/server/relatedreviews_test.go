package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

// documentTheFixture gives the review fixture two documented stories: one
// whose design references the line the review changes, and one whose design
// references a line of the same file that it does not. Both reach code the
// same way —
// design addresses a criterion, the criterion belongs to a story, the story
// to a feature — so the only difference between them is the code itself.
func documentTheFixture(t *testing.T, fixture serverReviewFixture) {
	t.Helper()
	featureDir := serverFeatureDir(fixture.root)
	head := strings.TrimSpace(serverGit(t, fixture.repo, "rev-parse", "HEAD"))
	design := func(chapter, fragment, title, anchor, path string, start, end int) string {
		chapterDir := filepath.Join(featureDir, applayout.DesignDir, chapter+".chapter")
		writeServerFile(t, filepath.Join(chapterDir, "chapter.json"), fmt.Sprintf(`{"version":2,"id":%q,"title":%q}`, chapter, title))
		fragmentDir := filepath.Join(chapterDir, fragment+".fragment")
		writeServerFile(t, filepath.Join(fragmentDir, "fragment.json"), fmt.Sprintf(`{"version":2,"id":%q,"title":%q,"media_type":"text/markdown","entrypoint":"content.md"}`, fragment, title))
		writeServerFile(t, filepath.Join(fragmentDir, "content.md"), "# "+title+" {#"+anchor+"}\n\n"+title+" is designed here.\n")
		resolver, err := coderesolve.New(context.Background(), fixture.repo)
		if err != nil {
			t.Fatal(err)
		}
		defer resolver.Close()
		reference, err := resolver.Author(context.Background(), coderef.Location{Commit: head, Path: path, Start: start, End: end}, "")
		if err != nil {
			t.Fatal(err)
		}
		writeServerJSON(t, filepath.Join(fragmentDir, saga.CodeDirName, fragment+".json"), saga.CodeFile{Version: saga.CurrentVersion, References: []coderef.Reference{reference}})
		return saga.FragmentTarget("app", fragment)
	}
	story := func(id, criterion, statement string) {
		storyDir := filepath.Join(featureDir, applayout.RequirementsDir, "stories", id+".story")
		writeServerFile(t, filepath.Join(storyDir, "story.json"), fmt.Sprintf(`{"$schema":"https://changesaga.dev/schema/v3/story.schema.json","version":3,"id":%q,"created_at":"2026-08-21T12:00:00Z"}`, id))
		writeServerFile(t, filepath.Join(storyDir, "revisions", "r1.json"), fmt.Sprintf(
			`{"$schema":"https://changesaga.dev/schema/v3/story-revision.schema.json","version":3,"id":"r1","story":"urn:change-saga:app:story:%s","parents":[],"title":%q,"statement":%q,"priority":"must","personas":[],"citations":[],"acceptance_criteria":[{"id":%q,"statement":%q}],"created_at":"2026-08-21T12:00:00Z"}`,
			id, id, statement, criterion, statement))
		writeServerFile(t, filepath.Join(storyDir, "events", "e1.json"), fmt.Sprintf(
			`{"$schema":"https://changesaga.dev/schema/v3/story-event.schema.json","version":3,"id":"e1","story":"urn:change-saga:app:story:%s","parents":[],"state":"proposed","created_at":"2026-08-21T12:00:00Z"}`, id))
	}
	changed := design("queue-design", "enqueue-design", "Enqueue", "enqueue", "queue.go", 3, 3)
	untouched := design("package-design", "package-clause", "The package", "the-package", "queue.go", 1, 1)
	story("place-an-order", "queued", "The order is queued durably.")
	story("name-the-package", "named", "The queue lives in its own package.")

	// The relation's design pin is the fragment's current content digest, the
	// same one the authoring command defaults to.
	document, validation, err := saga.Load(fixture.root)
	if err != nil || !validation.Valid {
		t.Fatalf("documented fixture: %v %#v", err, validation.Issues)
	}
	digests, err := saga.CurrentDesignContentDigests(document)
	if err != nil {
		t.Fatal(err)
	}
	relate := func(id, from, story, criterion string) {
		writeServerFile(t, filepath.Join(featureDir, applayout.RequirementsDir, "relations", id+".json"), fmt.Sprintf(
			`{"$schema":"https://changesaga.dev/schema/v5/relation.schema.json","version":5,"id":%q,"type":"addresses","from":%q,"to":"urn:change-saga:app:story:%s:criterion:%s","scope":"self","rationale":"The design addresses the criterion.","to_revision":"urn:change-saga:app:story:%s:revision:r1","from_content_digest":%q,"state":"active","created_at":"2026-08-21T12:00:00Z"}`,
			id, from, story, criterion, story, digests[from]))
	}
	relate("r-queue", changed, "place-an-order", "queued")
	relate("r-package", untouched, "name-the-package", "named")
	if _, validation, err := saga.Load(fixture.root); err != nil || !validation.Valid {
		t.Fatalf("documented fixture: %v %#v", err, validation.Issues)
	}
	serverGit(t, fixture.repo, "add", ".")
	serverGit(t, fixture.repo, "commit", "-m", "Document the queue and the report")
}

// A feature, a story, and an acceptance criterion each list the reviews that
// touched the code they explain, and a record whose code the review never
// touched lists none. Nothing declares the link: it is the intersection of
// the review's changed lines with the code the record's own chain reaches.
func TestRelatedReviewsAreDerivedFromTheChangedLines(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	documentTheFixture(t, fixture)
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	touched := []string{featureHref(serverFeature), requirementStoryHref("place-an-order"), requirementCriterionHref("place-an-order", "queued")}
	for _, path := range touched {
		body := documentationPage(t, handler, path)
		if !strings.Contains(body, `data-related-review="pr-7"`) {
			t.Fatalf("%s does not list the review that changed its code:\n%s", path, relatedReviewSection(body))
		}
		if !strings.Contains(body, "Move the queue to Postgres</a>") {
			t.Fatalf("%s names the review by something other than its title", path)
		}
	}
	for _, path := range []string{requirementStoryHref("name-the-package"), requirementCriterionHref("name-the-package", "named")} {
		if body := documentationPage(t, handler, path); strings.Contains(body, "data-related-review=") {
			t.Fatalf("%s lists a review that never touched its code:\n%s", path, relatedReviewSection(body))
		}
	}
	// It is a footnote: it appears after the record's own content, and it is
	// counted in no table and no heading.
	page := documentationPage(t, handler, requirementStoryHref("place-an-order"))
	if strings.Index(page, "data-requirement-target=") > strings.Index(page, "data-related-reviews=") {
		t.Fatal("the related-reviews footnote was rendered above the record's own content")
	}
	if strings.Contains(documentationPage(t, handler, "/features"), "data-related-review") {
		t.Fatal("the features table gained a related-reviews column")
	}
}

// documentationPage is a documentation page's HTML. Documentation is served
// by the ordinary page handler, which carries no incremental-surface headers.
func documentationPage(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func relatedReviewSection(body string) string {
	index := strings.Index(body, "related-reviews")
	if index < 0 {
		return "no related-reviews section"
	}
	return body[index:min(index+400, len(body))]
}
