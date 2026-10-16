package server

import (
	"bytes"
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestDeprecationInspectionShowsScopeEvidenceAndWithdrawal(t *testing.T) {
	root, repo := termSaga(t)
	serverGit(t, repo, "remote", "add", "origin", "https://example.test/a.git")
	ctx := context.Background()
	doc := diagram.Document{Version: 1, Width: 600, Height: 400, Elements: []diagram.Element{{ID: "legacy", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 120, Style: "normal", Label: "Legacy"}}}
	if _, err := requirements.PublishWhiteboard(ctx, root, "test", "systems", "b1", "", "Systems", doc); err != nil {
		t.Fatal(err)
	}
	target := "urn:change-saga:test:whiteboard:systems"
	targets := requirements.DeprecationTargets{Root: root, SagaID: "test"}
	pin, err := targets.Pin(target, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	records, err := requirements.Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	refs := records.Terms[0].CurrentRevision.Code
	for _, scope := range []string{"content", "code"} {
		if _, err := requirements.AddDeprecation(root, "test", requirements.AddDeprecationInput{ID: scope, RevisionID: "r1", EventID: "active", DeprecationDefinition: requirements.DeprecationDefinition{Reason: "Migrate <script>unsafe</script> safely.", Scope: scope, Target: pin, Code: refs}}); err != nil {
			t.Fatal(err)
		}
	}
	application := &app{root: root, sourceDir: repo, template: serverTemplate(t)}
	mux := newMux(application)
	get := func(path string) string {
		t.Helper()
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != 200 {
			t.Fatalf("%s: %d %s", path, out.Code, out.Body.String())
		}
		return out.Body.String()
	}
	body := get("/api/deprecations?ref=" + url.QueryEscape(refs[0].Location().String()))
	for _, want := range []string{"Explicit code scope", "Content scope", "Related code only", "Authored deprecated code", refs[0].Digest, "exact", "&lt;script&gt;", "Evidence provenance"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "<script>unsafe") {
		t.Fatal("authored reason executed as HTML")
	}
	noRecord := refs[0].Location()
	noRecord.Start = 7
	noRecord.End = 7
	if body := get("/api/deprecations?ref=" + url.QueryEscape(noRecord.String())); !strings.Contains(body, "No recorded deprecation") || !strings.Contains(body, "not proof") {
		t.Fatal(body)
	}
	urn := "urn:change-saga:test:deprecation:code"
	if _, err := requirements.SetDeprecationState(root, "test", requirements.SetDeprecationStateInput{Deprecation: urn, ID: "withdraw", Parents: []string{urn + ":event:active"}, State: requirements.DeprecationWithdrawn, Reason: "Wrong scope"}); err != nil {
		t.Fatal(err)
	}
	if body := get("/api/deprecations?target=" + url.QueryEscape(target)); !strings.Contains(body, "Deprecation withdrawn") || !strings.Contains(body, "Wrong scope") {
		t.Fatal(body)
	}
	page := get("/technical/whiteboards/systems")
	if !strings.Contains(page, "data-deprecation-load") || !strings.Contains(page, "revision=b1") {
		t.Fatalf("board does not load its annotations: %s", page)
	}
	for _, bad := range []string{"/api/deprecations", "/api/deprecations?target=../../../private", "/api/deprecations?ref=HEAD:kinds.go", "/api/deprecations?file=../private"} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest("GET", bad, nil))
		if response.Code != 400 {
			t.Fatalf("%s: %d", bad, response.Code)
		}
	}
}

func TestDeprecationFragmentAndItemVisibility(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("fixture: %v %v", err, validation)
	}
	slide := document.Decks[0].Slides[0]
	item := slide.Items[0]
	targets := requirements.DeprecationTargets{Root: root, SagaID: document.Manifest.ID}
	pin, err := targets.Pin(item.Target, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.AddDeprecation(root, document.Manifest.ID, requirements.AddDeprecationInput{ID: "old-item", RevisionID: "r1", EventID: "active", DeprecationDefinition: requirements.DeprecationDefinition{Reason: "This Item describes a retired path.", Scope: "content", Target: pin}}); err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	if err := serverTemplate(t).ExecuteTemplate(&rendered, "fragment", makeFragmentView(findFragmentByTarget(document, slide.Target), viewScope{})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "data-deprecation-load") {
		t.Fatal("fragment does not discover annotations")
	}
	mux := newMux(&app{root: root, template: serverTemplate(t)})
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest("GET", "/api/deprecations?target="+url.QueryEscape(slide.Target)+"&children=1", nil))
	if out.Code != 200 || !strings.Contains(out.Body.String(), "This Item describes a retired path.") || !strings.Contains(out.Body.String(), item.Target) {
		t.Fatalf("Item missing from slide: %d %s", out.Code, out.Body.String())
	}
}
