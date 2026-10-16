package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func discoveryRecord(id, scope, target string) requirements.Deprecation {
	rev := requirements.DeprecationRevision{ID: "r1", Scope: scope, Reason: "Use the replacement for new work.", Target: requirements.DeprecationTarget{Target: target}}
	head, _ := requirements.DeprecationRevisionURN("test", id, "r1")
	return requirements.Deprecation{Identity: requirements.RecordIdentity{ID: id}, Revisions: []requirements.DeprecationRevision{rev}, RevisionHeads: []string{head}, CurrentRevision: &rev, CurrentLifecycle: &requirements.DeprecationEvent{State: requirements.DeprecationActive}}
}

func TestDeprecationDiscoveryFiltersPaginatesAndPreservesConflicts(t *testing.T) {
	doc := requirements.Document{SagaID: "test"}
	for i := 0; i < 53; i++ {
		doc.Deprecations = append(doc.Deprecations, discoveryRecord(fmt.Sprintf("old-%02d", i), "content", "urn:change-saga:test:whiteboard:systems"))
	}
	withdrawn := discoveryRecord("withdrawn", "code", "urn:change-saga:test:whiteboard:other")
	withdrawn.CurrentLifecycle.State = requirements.DeprecationWithdrawn
	doc.Deprecations = append(doc.Deprecations, withdrawn)
	conflict := discoveryRecord("conflict", "content", "urn:change-saga:test:whiteboard:systems")
	conflict.CurrentRevision = nil
	second := requirements.DeprecationRevision{ID: "r2", Scope: "code", Reason: "Competing reason.", Target: requirements.DeprecationTarget{Target: "urn:change-saga:test:whiteboard:other"}}
	conflict.Revisions = append(conflict.Revisions, second)
	conflict.RevisionHeads = append(conflict.RevisionHeads, "urn:change-saga:test:deprecation:conflict:revision:r2")
	doc.Deprecations = append(doc.Deprecations, conflict)
	metadata := requirements.WhiteboardMetadata{}
	view := makeDeprecationDirectory(doc, nil, metadata, url.Values{})
	if view.Total != 55 || len(view.Rows) != 50 || view.Next == "" {
		t.Fatalf("first page: %+v", view)
	}
	view = makeDeprecationDirectory(doc, nil, metadata, url.Values{"offset": {"50"}})
	if len(view.Rows) != 5 || view.Previous == "" || view.Next != "" {
		t.Fatalf("last page: %+v", view)
	}
	view = makeDeprecationDirectory(doc, nil, metadata, url.Values{"scope": {"code"}, "q": {"Competing"}})
	if len(view.Rows) != 1 || view.Rows[0].State != "conflicted" || len(view.Rows[0].Targets) != 2 {
		t.Fatalf("lost conflict: %+v", view)
	}
	view = makeDeprecationDirectory(doc, nil, metadata, url.Values{"state": {"withdrawn"}})
	if view.Total != 1 || view.Rows[0].ID != "withdrawn" {
		t.Fatal(view)
	}
	view = makeDeprecationDirectory(doc, nil, metadata, url.Values{"page": {"/technical/whiteboards/systems"}})
	if view.Total != 54 {
		t.Fatalf("page scope: %d", view.Total)
	}
	rows := deprecationSummaries(doc, map[string]bool{"urn:change-saga:test:whiteboard:systems": true})
	if len(rows) != 54 || len(rows[len(rows)-1].Targets) != 2 {
		t.Fatal("selection lost competing target")
	}
}

func TestDeprecationCompactSummaryDefersProvenanceAndKeepsHistoricalContext(t *testing.T) {
	root, repo := termSaga(t)
	drawing := diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "old", Kind: "node", Shape: "rect", Label: "Old", Width: 100, Height: 100, Style: "normal"}}}
	if _, err := requirements.PublishWhiteboard(context.Background(), root, "test", "systems", "r1", "", "Systems", drawing); err != nil {
		t.Fatal(err)
	}
	target := "urn:change-saga:test:whiteboard:systems"
	resolver := requirements.DeprecationTargets{Root: root, SagaID: "test"}
	pin, err := resolver.Pin(target, "old")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = requirements.AddDeprecation(root, "test", requirements.AddDeprecationInput{ID: "old", RevisionID: "r1", EventID: "active", DeprecationDefinition: requirements.DeprecationDefinition{Reason: "Move away from <script>unsafe</script>.", Scope: "content", Target: pin}}); err != nil {
		t.Fatal(err)
	}
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	mux := newMux(application)
	get := func(path string) string {
		t.Helper()
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.String()
	}
	body := get("/api/deprecations?compact=1&target=" + url.QueryEscape(target) + "&revision=r1")
	if !strings.Contains(body, "data-deprecation-summary") || strings.Contains(body, "data-deprecations ") || strings.Contains(body, "Authored assertion") || strings.Contains(body, "<script>unsafe") {
		t.Fatal(body)
	}
	if application.files.current.deprecationsView != nil {
		t.Fatal("compact discovery resolved target health")
	}
	if !strings.Contains(body, "revision=r1") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("lost context or escaping")
	}
	body = get("/api/deprecations?deprecation=old&revision=r1")
	if !strings.Contains(body, "Reading saved revision r1") || !strings.Contains(body, "Authored assertion and target pin") {
		t.Fatal(body)
	}
	body = get("/deprecations")
	if !strings.Contains(body, "data-deprecation-result=\"old\"") || !strings.Contains(body, "focus=old") {
		t.Fatal("missing index / appearance link")
	}
}

func TestDeprecationTechnicalLinksUseRevisionIDsAndEscapedPresentation(t *testing.T) {
	place := deprecationPlaceLink(deprecationPlace{Target: "urn:change-saga:test:erd:model", Revision: "urn:change-saga:test:erd:model:revision:r1"}, nil, requirements.WhiteboardMetadata{})
	if place.Href != "/technical/erd/model?revision=r1" {
		t.Fatal(place.Href)
	}
	var out bytes.Buffer
	view := &deprecationDirectory{Rows: []deprecationSummary{{Preview: "<script>bad</script>", DetailHref: "/api/deprecations?deprecation=old"}}}
	if err := serverTemplate(t).ExecuteTemplate(&out, "deprecation-directory", view); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "<script>bad") {
		t.Fatal("unsafe reason rendered")
	}
}
