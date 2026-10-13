package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestWhiteboardDataModelSourcesAndExistingERDs(t *testing.T) {
	t.Parallel()
	root, repo := termSaga(t)
	if _, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo); err != nil {
		t.Fatal(err)
	}
	mux := newMux(&app{root: root, sourceDir: repo, template: serverTemplate(t)})
	status, body := technicalGet(t, mux, "/technical/erd")
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	if i := strings.Index(body, `data-whiteboard-note`); i >= 0 {
		t.Fatalf("board unavailable: %s", body[i:i+min(350, len(body)-i)])
	}
	for _, want := range []string{`data-whiteboard`, `data-frame-path="local"`, `data-whiteboard-frame="cloud/postgres"`, `data-from="local/local-edit"`, `data-to="cloud/postgres/cloud-record"`, `data-documentation-target="urn:change-saga:test:data-entity:local-edit"`, `href="/technical/erd/sqlite?revision=r2"`, `Select bound entities`, `Diagrams on this board`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	// Pinned sources remain independently readable at the existing route.
	status, body = technicalGet(t, mux, "/technical/erd/sqlite?revision=r1")
	if status != 200 || !strings.Contains(body, `data-erd-revision="r1"`) {
		t.Fatal("saved ERD no longer readable")
	}
	// Broken composition cannot erase source documents or the entity directory.
	if err := os.WriteFile(filepath.Join(root, "___whiteboards", "erd", "revisions", "r1.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	status, body = technicalGet(t, mux, "/technical/erd")
	if status != 200 || !strings.Contains(body, `data-whiteboard-note`) || !strings.Contains(body, `data-technical-erd-documents`) {
		t.Fatal("broken board hides source documents")
	}
}

func TestInlineWhiteboardSVGNamespacesAndKeepsThemeLocal(t *testing.T) {
	got, err := inlineWhiteboardSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" aria-labelledby="page caption"><style>:root{--bg:red}text{font-size:2px}</style><defs><marker id="arrow"/></defs><g id="page" data-instance-path="local/page" data-documentation-target="urn:test" style="fill:var(--diagram-normal-fill,#ffffff)"><title id="caption">A &amp; B</title><path marker-end="url(#arrow)"/><use href="#page"/></g></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="whiteboard-page"`, `aria-labelledby="whiteboard-page whiteboard-caption"`, `href="#whiteboard-page"`, `url(#whiteboard-arrow)`, `data-instance-path="local/page"`, `data-documentation-target="urn:test"`, `fill:var(--diagram-normal-fill,#ffffff)`, `A &amp; B`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(got, "<style") || strings.Contains(got, ":root") {
		t.Fatal("standalone stylesheet escaped into app")
	}
	if _, err := inlineWhiteboardSVG([]byte(`<svg><g></svg>`)); err == nil {
		t.Fatal("malformed composition accepted")
	}
}

func TestInlineDiagramRefusesActiveAndExternalContent(t *testing.T) {
	for _, markup := range []string{
		`<svg><script>alert(1)</script></svg>`,
		`<svg onload="alert(1)"></svg>`,
		`<svg><use href="https://example.test/x.svg"/></svg>`,
		`<svg><image href="https://example.test/x.svg"/></svg>`,
		`<svg><rect style="position:fixed;inset:0"/></svg>`,
		`<svg><rect fill="url(https://example.test/x)"/></svg>`,
		`<svg><g data-open-history="https://example.test"/></svg>`,
	} {
		if _, err := inlineDiagramSVG([]byte(markup), "safe-"); err == nil {
			t.Errorf("accepted %s", markup)
		}
	}
}
