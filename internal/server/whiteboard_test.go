package server

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
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
	for _, want := range []string{`data-whiteboard`, `data-frame-path="local"`, `data-whiteboard-frame="cloud/postgres"`, `data-from="local/local-edit"`, `data-to="cloud/postgres/cloud-record"`, `data-documentation-target="urn:change-saga:test:data-entity:local-edit"`, `href="/technical/erd/sqlite?revision=r2"`, `Select bound entities`, `Source provenance`} {
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
		`<svg><g color="url(https://example.test/x)"/></svg>`,
		`<svg><g data-open-history="https://example.test"/></svg>`,
	} {
		if _, err := inlineDiagramSVG([]byte(markup), "safe-"); err == nil {
			t.Errorf("accepted %s", markup)
		}
	}
}

func TestInlineSharedDiagramIconLicense(t *testing.T) {
	doc := diagram.Document{Version: 1, Width: 400, Height: 200, Elements: []diagram.Element{{ID: "database", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 100, Style: "normal", Label: "Database", Icon: "lucide:database"}}}
	standalone, err := diagram.Render(doc, diagram.Options{})
	if err != nil {
		t.Fatal(err)
	}
	composition, err := diagram.ResolveComposition(doc, nil)
	if err != nil {
		t.Fatal(err)
	}
	board, err := composition.Render(diagram.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		data   []byte
		inline func([]byte) (string, error)
		prefix string
	}{
		{"standalone ERD", standalone, func(data []byte) (string, error) { return inlineDiagramSVG(data, "erd-") }, "erd-"},
		{"whiteboard", board, inlineWhiteboardSVG, "whiteboard-"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.inline(test.data)
			if err != nil {
				t.Fatal(err)
			}
			var visual struct {
				Metadata []struct {
					ID   string `xml:"id,attr"`
					Text string `xml:",chardata"`
				} `xml:"metadata"`
			}
			if err := xml.Unmarshal([]byte(got), &visual); err != nil {
				t.Fatal(err)
			}
			if len(visual.Metadata) != 1 || visual.Metadata[0].ID != test.prefix+"diagram-notices" || !strings.Contains(visual.Metadata[0].Text, diagram.IconLicense()) || !strings.Contains(visual.Metadata[0].Text, diagram.LucideRevision) {
				t.Fatal("icon license or namespaced metadata ID lost")
			}
			if !strings.Contains(got, `id="`+test.prefix+`database"`) || strings.Contains(got, "<style") {
				t.Fatal("diagram content or stylesheet isolation changed")
			}
		})
	}
}

func TestInlineDiagramMetadataTextOnly(t *testing.T) {
	got, err := inlineDiagramSVG([]byte(`<svg><metadata id="license">A &amp; B &lt;script&gt;literal&lt;/script&gt;</metadata></svg>`), "safe-")
	if err != nil || !strings.Contains(got, `<metadata id="safe-license">A &amp; B &lt;script&gt;literal&lt;/script&gt;</metadata>`) {
		t.Fatalf("escaped license text: %s, %v", got, err)
	}
	for _, markup := range []string{
		`<svg><metadata><script>alert(1)</script></metadata></svg>`,
		`<svg><metadata><rect width="10"/></metadata></svg>`,
		`<svg><metadata><style>body{display:none}</style></metadata></svg>`,
		`<svg><metadata><metadata>Nested</metadata></metadata></svg>`,
		`<svg><metadata onload="alert(1)">License</metadata></svg>`,
		`<svg><metadata href="javascript:alert(1)">License</metadata></svg>`,
		`<svg><metadata xmlns="http://www.w3.org/1999/xhtml">Foreign</metadata></svg>`,
		`<svg><metadata>Unclosed</svg>`,
		`<svg><metadata>Safe</metadata><script>alert(1)</script></svg>`,
	} {
		if _, err := inlineDiagramSVG([]byte(markup), "safe-"); err == nil {
			t.Errorf("accepted unsafe metadata: %s", markup)
		}
	}
	// This exception belongs only to the shared renderer's inliner. Authored
	// arbitrary SVG still passes through its existing, stricter element policy.
	if _, refused := sanitizeSVG([]byte(`<svg><metadata>License</metadata></svg>`), "safe-"); len(refused) == 0 {
		t.Fatal("arbitrary SVG metadata policy was widened")
	}
}
