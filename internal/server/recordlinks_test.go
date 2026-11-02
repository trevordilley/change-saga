package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func TestAnEssaysRecordLinkPointsAtThisReader(t *testing.T) {
	t.Parallel()
	urn := "urn:change-saga:app:story:move-queue"
	body := string(markdownWithAnchors("See [the story]("+urn+") and [elsewhere](https://example.test/x) and [isbn](urn:isbn:1).", "essay"))
	if !strings.Contains(body, `href="/records/`+urn+`"`) {
		t.Errorf("the record link was not rewritten:\n%s", body)
	}
	if !strings.Contains(body, `href="https://example.test/x"`) || !strings.Contains(body, `href="urn:isbn:1"`) {
		t.Errorf("a link that is not a record's changed:\n%s", body)
	}
}

func TestRecordLinksOpenTheRecordsPage(t *testing.T) {
	t.Parallel()
	fixture := newServerReviewFixture(t)
	if _, err := requirements.AddStory(fixture.root, "app", requirements.AddStoryInput{Feature: serverFeature, ID: "move-queue", RevisionID: "r1", EventID: "proposed",
		Title: "Move the queue", Statement: "As an operator, I run one store", Priority: "high", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	addServerPlan(t, fixture.root, "first")
	addServerTask(t, fixture.root, "first", "work", "urn:change-saga:app:story:move-queue")
	_, handler := reviewApp(t, fixture, gitdiff.Range{})

	for urn, want := range map[string]string{
		"urn:change-saga:app:feature:" + serverFeature: featureHref(serverFeature),
		"urn:change-saga:app:story:move-queue":         "/requirements/move-queue",
		"urn:change-saga:app:plan:first":               "/plans/first",
		"urn:change-saga:app:plan:first:task:work":     "/plans/first/tasks/work",
		"urn:change-saga:app:review:pr-7":              "/reviews/pr-7",
		// None of these has a page here, so none opens a wrong one.
		"urn:change-saga:app:story:absent":               "",
		"urn:change-saga:app:plan:first:task:absent":     "/plans/first",
		"urn:change-saga:other:feature:" + serverFeature: "",
		"urn:change-saga:app:citation:anything":          "",
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/records/"+url.PathEscape(urn), nil))
		if want == "" {
			if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "no page") {
				t.Errorf("%s = %d %q, want a plain not found", urn, recorder.Code, recorder.Body.String())
			}
			continue
		}
		if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != want {
			t.Errorf("%s = %d to %q, want %s", urn, recorder.Code, recorder.Header().Get("Location"), want)
		}
	}
}
