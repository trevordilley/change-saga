package requirements

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
)

func TestStructuredERDIconPublicationPreservesLicense(t *testing.T) {
	root := newSaga(t)
	adoptRaw(t, root, 2)
	entity := pinOf(rawTechnical(t, root, KindDataEntity, "job", TechnicalDefinition{Name: "Job", Explanation: "Queued work", Intent: IntentProposed, Baseline: BaselineNone}), "r1")
	doc := diagram.Document{Version: 1, Width: 400, Height: 200, Elements: []diagram.Element{{ID: "job", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 180, Height: 100, Style: "normal", Label: "Job", Icon: "lucide:database"}}}
	def := TechnicalDefinition{Name: "Jobs", Explanation: "Job directory", Directory: []DocumentationLink{entity}, Bindings: []Binding{{ID: "job", Element: "job", Entity: &entity}}, Diagram: &doc}
	if _, err := WriteTechnicalRevision(context.Background(), root, "test", TechnicalWrite{Kind: KindERD, ID: "jobs", RevisionID: "r1", Definition: def, Create: true}); err != nil {
		t.Fatal(err)
	}
	inventory, err := LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	revision := inventory.Pinned(pinOf("urn:change-saga:test:erd:jobs", "r1"))
	if revision == nil || revision.Visual == nil || revision.Diagram == nil || revision.Diagram.Elements[0].Icon != "lucide:database" {
		t.Fatal("published ERD lost its structured icon source or visual")
	}
	data, err := os.ReadFile(filepath.Join(root, TechnicalPath(KindERD, "jobs"), revision.Visual.Path))
	if err != nil {
		t.Fatal(err)
	}
	var visual struct {
		Metadata []struct {
			ID   string `xml:"id,attr"`
			Text string `xml:",chardata"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(data, &visual); err != nil {
		t.Fatal(err)
	}
	if len(visual.Metadata) != 1 || visual.Metadata[0].ID != "diagram-notices" || !strings.Contains(visual.Metadata[0].Text, diagram.IconLicense()) || !strings.Contains(visual.Metadata[0].Text, diagram.LucideRevision) {
		t.Fatal("published visual did not preserve the shared renderer's icon license")
	}
	if err := ValidateVisual(data, revision.Visual.Digest, revision.Bindings); err != nil {
		t.Fatal(err)
	}
}

func TestVisualMetadataAllowsOnlyInertText(t *testing.T) {
	wrap := func(content string) []byte {
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg">` + content + `</svg>`)
	}
	for _, content := range []string{
		`<metadata id="diagram-notices">License &amp; attribution</metadata>`,
		`<metadata>&lt;script&gt;literal text&lt;/script&gt;</metadata><rect id="mark" width="10" height="10"/>`,
		`<metadata>First</metadata><metadata>Second</metadata>`,
	} {
		if err := ValidateVisual(wrap(content), "", nil); err != nil {
			t.Errorf("inert metadata rejected: %v", err)
		}
	}
	for _, content := range []string{
		`<metadata><script>alert(1)</script></metadata>`,
		`<metadata><rect width="10" height="10"/></metadata>`,
		`<metadata><metadata>Nested</metadata></metadata>`,
		`<metadata><foreignObject><div>HTML</div></foreignObject></metadata>`,
		`<metadata xmlns="http://www.w3.org/1999/xhtml">Foreign namespace</metadata>`,
		`<metadata onload="alert(1)">License</metadata>`,
		`<metadata href="javascript:alert(1)">License</metadata>`,
		`<metadata><![CDATA[<script>alert(1)</script>]]></metadata>`,
		`<metadata>Unclosed`,
		`<metadata>Safe</metadata><script>alert(1)</script>`,
		`<metadata>Safe</metadata><style>@import "https://example.com/a.css";</style>`,
		`<metadata id="duplicate">License</metadata><rect id="duplicate"/>`,
	} {
		if err := ValidateVisual(wrap(content), "", nil); err == nil {
			t.Errorf("unsafe or malformed metadata accepted: %s", content)
		}
	}
}
