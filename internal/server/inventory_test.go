package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestDocumentationReviewControl(t *testing.T) {
	pin := &saga.DocumentationLink{Target: "urn:change-saga:test:component:store", Revision: "urn:change-saga:test:component:store:revision:r1"}
	item := reviewItemView{Item: &saga.Item{ItemManifest: saga.ItemManifest{Documentation: pin}}}
	var rendered bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&rendered, "review-item-affordance", item); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `data-documentation-target="`+pin.Target+`"`) {
		t.Fatalf("review lost shared documentation control: %s", rendered.String())
	}
}

func TestDocumentationPinnedPage(t *testing.T) {
	root, repo := termSaga(t)
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	definition := requirements.TechnicalDefinition{Name: "Store", Explanation: "The shared typed implementation.", Code: []requirements.Evidence{{Reference: coderef.Reference{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest, Note: "Exact declaration provenance."}}}}
	target := "urn:change-saga:test:component:store"
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r1", nil, definition, true); err != nil {
		t.Fatal(err)
	}
	definition.Name = "NewStore"
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r2", []string{target + ":revision:r1"}, definition, false); err != nil {
		t.Fatal(err)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	get := func(pin string) (int, string) {
		t.Helper()
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/documentation?"+url.Values{"target": {target}, "revision": {pin}}.Encode(), nil))
		return rr.Code, rr.Body.String()
	}
	status, body := get(target + ":revision:r1")
	for _, want := range []string{`data-documentation-status="stale"`, `<h2>Store</h2>`, "Exact declaration provenance.", "KindTesttaker", target + ":revision:r1"} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("saved revision lost %s: %d %s", want, status, body)
		}
	}
	status, _ = get(target + ":revision:missing")
	if status != 404 {
		t.Fatalf("missing revision: %d", status)
	}
	status, _ = get("urn:change-saga:other:component:store:revision:r1")
	if status != 400 {
		t.Fatalf("cross-Saga pin: %d", status)
	}
	if _, err := requirements.SetTechnicalState(root, "test", "component", "store", "retired", "retired", "Replaced", []string{target + ":event:active"}); err != nil {
		t.Fatal(err)
	}
	status, body = get(target + ":revision:r1")
	if status != 200 || !strings.Contains(body, `data-documentation-status="retired"`) {
		t.Fatalf("retirement hidden: %d %s", status, body)
	}
}
