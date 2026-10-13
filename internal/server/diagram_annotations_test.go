package server

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestDiagramAnnotationExactPinsAndLazyCode(t *testing.T) {
	root, repo := termSaga(t)
	pins, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	// A hover metadata request must work without reading a source checkout.
	mux := newMux(&app{root: root, sourceDir: filepath.Join(t.TempDir(), "missing-checkout"), template: serverTemplate(t)})
	request := func(pin saga.DocumentationLink, relationship, panel string) (int, string) {
		return technicalGet(t, mux, "/api/diagram-annotation?"+url.Values{"target": {pin.Target}, "revision": {pin.Revision}, "relationship": {relationship}, "panel": {panel}}.Encode())
	}
	status, body := request(pins["cloud-record"], "", "")
	if status != 200 || !strings.Contains(body, `class="landmark-affordance"`) || !strings.Contains(body, `data-documentation-revision="`+pins["cloud-record"].Revision+`"`) {
		t.Fatalf("metadata %d: %s", status, body)
	}
	if strings.Contains(body, "data-file-path") || strings.Contains(body, "data-open-stories") || strings.Contains(body, "data-documentation-item") {
		t.Fatal("metadata fabricated evidence or resolved code", body)
	}
	status, body = request(pins["cloud-record"], "sync", "")
	if status != 200 || !strings.Contains(body, `data-source-relationship="sync"`) {
		t.Fatalf("relationship %d: %s", status, body)
	}
	for _, panel := range []string{"", "code"} {
		if status, _ := request(pins["cloud-record"], "missing", panel); status != 404 {
			t.Fatalf("missing relationship fell back to owner: %d", status)
		}
		absent := pins["cloud-record"]
		absent.Revision = absent.Target + ":revision:missing"
		if status, _ := request(absent, "", panel); status != 404 {
			t.Fatalf("missing pin replaced: %d", status)
		}
	}
}

func TestDiagramAnnotationStoryProvenanceUsesRealExactItems(t *testing.T) {
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeEmbeddedSlideFixture(t, root)
	document, validation, err := saga.LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("fixture: %v", err)
	}
	slide := document.Decks[0].Slides[0]
	fragment := findFragmentByTarget(document, slide.Target)
	pin := saga.DocumentationLink{Target: "urn:change-saga:visual:data-entity:record", Revision: "urn:change-saga:visual:data-entity:record:revision:r1"}
	fragment.Landmarks[0].ItemMeta.Documentation = &pin
	story := "urn:change-saga:visual:story:checkout"
	records := requirements.Document{SagaID: "visual", Stories: []requirements.Story{{Identity: requirements.StoryIdentity{ID: "checkout"}, RevisionHeads: []string{story + ":revision:r1"}, CurrentRevision: &requirements.Revision{ID: "r1", Story: story, Title: "Safe checkout"}}}, Relations: []requirements.Relation{
		{ID: "real", Type: requirements.RelationExplains, From: fragment.Landmarks[0].Target, To: story, ToRevision: story + ":revision:r1", State: requirements.RelationActive, Scope: requirements.ScopeSelf},
		{ID: "broad", Type: requirements.RelationExplains, From: slide.Target, To: story, ToRevision: story + ":revision:r1", State: requirements.RelationActive, Scope: requirements.ScopeSelf},
	}}
	got, err := diagramAnnotationStories(document, records, pin, "example", "Saved record")
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Links) != 1 || len(got.Legacy) != 0 || got.Provenance == "" || !strings.HasPrefix(got.Links[0].ItemHref, "/?view=slides#") {
		t.Fatalf("provenance: %#v", got)
	}
	other := pin
	other.Revision = other.Target + ":revision:r2"
	got, err = diagramAnnotationStories(document, records, other, "other", "Saved record")
	if err != nil || got.Count != 0 {
		t.Fatalf("different revision inherited story: %#v %v", got, err)
	}
}

func TestDiagramAnnotationCodeRemainsLazyAndPinned(t *testing.T) {
	root, repo := termSaga(t)
	commit := strings.TrimSpace(serverGit(t, repo, "rev-parse", "HEAD"))
	digest, err := coderef.DigestRange([]byte(serverKinds), 6, 6)
	if err != nil {
		t.Fatal(err)
	}
	definition := requirements.TechnicalDefinition{Name: "Original store", Explanation: "The shared typed implementation.", Code: []requirements.Evidence{{Reference: coderef.Reference{Commit: commit, Path: "kinds.go", Start: 6, End: 6, Digest: digest, Note: "Original saved evidence."}}}}
	target := "urn:change-saga:test:component:store"
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r1", nil, definition, true); err != nil {
		t.Fatal(err)
	}
	definition.Name = "New store"
	definition.Code[0].Reference.Note = "New evidence note."
	if _, err := requirements.WriteTechnical(root, "test", "component", "store", "r2", []string{target + ":revision:r1"}, definition, false); err != nil {
		t.Fatal(err)
	}
	application := &app{root: root, sourceDir: filepath.Join(t.TempDir(), "missing-checkout"), template: serverTemplate(t)}
	mux := newMux(application)
	href := "/api/diagram-annotation?" + url.Values{"target": {target}, "revision": {target + ":revision:r1"}}.Encode()
	status, body := technicalGet(t, mux, href)
	if status != 200 || !strings.Contains(body, "Original store") || !strings.Contains(body, "data-target-code-href") || strings.Contains(body, "Original saved evidence") || strings.Contains(body, "data-file-path") || strings.Contains(body, "New store") {
		t.Fatalf("metadata resolved code or substituted current revision: %d %s", status, body)
	}
	application.sourceDir = repo
	status, body = technicalGet(t, mux, href+"&panel=code")
	for _, want := range []string{"data-target-code-response", "Original saved evidence.", `data-evidence-integrity="verified"`, "KindTesttaker"} {
		if status != 200 || !strings.Contains(body, want) {
			t.Fatalf("pinned code missing %s: %d %s", want, status, body)
		}
	}
}
