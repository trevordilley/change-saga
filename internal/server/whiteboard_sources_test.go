package server

import (
	"bytes"
	"context"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/testfixture"
)

func TestWhiteboardSourcePresentationUsesRealAuthoredBindings(t *testing.T) {
	t.Parallel()
	root, repo := termSaga(t)
	pins, err := testfixture.WriteERDWhiteboardFixture(context.Background(), root, repo)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := requirements.LoadResolvedWhiteboard(root, "test", "erd", "")
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), resolved.SVG...)
	visual, err := inlineWhiteboardSVG(original)
	if err != nil {
		t.Fatal(err)
	}
	got, capabilities := (&app{root: root}).enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
	board, err := readPresentationSVG(string(got))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, resolved.SVG) {
		t.Fatal("presentation mutated the resolved renderer bytes")
	}
	rerendered, err := resolved.Composition.Render(diagram.Options{Title: resolved.Revision.Name})
	if err != nil || !bytes.Equal(original, rerendered) {
		t.Fatal("presentation changed the deterministic composition")
	}
	if capabilities["cross-store"].Mode != "authored" || capabilities["cross-store"].Bindings != len(inventory.Pinned(pins["cross-store"]).Bindings) {
		t.Fatalf("authored capability does not describe real bindings: %+v", capabilities)
	}
	if capabilities["cross-store"].FieldDetail || capabilities["cross-store"].Reveal || capabilities["cross-store"].Landmarks {
		t.Fatal("authored source claims unsupported capabilities")
	}
	fields, bindings := 0, 0
	board.walk(func(n *presentationSVG) {
		if n.name == "image" {
			t.Error("supported authored source is still an opaque image")
		}
		if n.attr("data-field") != "" {
			fields++
			if n.attr("data-diagram-detail") != "field" || n.attr("display") != "none" || n.attr("aria-hidden") != "true" {
				t.Error("structured field was not hidden by default")
			}
		}
		if n.attr("data-binding") != "" {
			bindings++
			if n.attr("id") == "" || (n.name == "g" && len(n.children) == 0) || n.attr("data-documentation-revision") == "" {
				t.Errorf("binding is a metadata-only proxy: %+v", n)
			}
			if n.attr("data-instance-path") == "cross-store/cloud-record" && n.attr("data-documentation-revision") != pins["cloud-record"].Revision {
				t.Error("binding lost its exact pinned revision")
			}
		}
	})
	if fields != 4 || bindings != capabilities["cross-store"].Bindings || !capabilities["local"].FieldDetail || !capabilities["cloud/postgres"].FieldDetail {
		t.Fatalf("incorrect field/binding presentation: %d fields, %d bindings; %+v", fields, bindings, capabilities)
	}
}

func TestWhiteboardRepeatedAuthoredSourcesNamespaceMarksAndReferences(t *testing.T) {
	t.Parallel()
	root, _, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pin := pins["reporting"]
	revision := inventory.Pinned(pin)
	// Change only this temporary fixture's authored asset. The relationship
	// binding deliberately uses a different binding ID from its element ID.
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="10 20 320 120"><defs><marker id="arrow"><path d="M0 0L4 2L0 4Z"/></marker></defs><g id="report-card"><rect x="10" y="20" width="120" height="60"/><text>A &amp; B<tspan> report</tspan></text></g><path id="report-flow" d="M150 60L250 60" marker-end="url(#arrow)"/><use href="#report-card" x="150"/></svg>`
	revision.Bindings = append(revision.Bindings, requirements.Binding{ID: "flow-binding", Element: "report-flow", Relationship: &requirements.RelationshipRef{Owner: pins["report"], ID: "produced-from-job"}})
	setWhiteboardTestVisual(t, root, inventory.Find(pin.Target), revision, svg)
	resolved := composeWhiteboardTestSource(t, pin, []byte(svg), revision.Bindings, "first", "second")
	visual, err := inlineWhiteboardSVG(resolved.SVG)
	if err != nil {
		t.Fatal(err)
	}
	got, capabilities := (&app{root: root}).enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
	board, err := readPresentationSVG(string(got))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	paths := map[string]*presentationSVG{}
	board.walk(func(n *presentationSVG) {
		if id := n.attr("id"); id != "" {
			if ids[id] {
				t.Errorf("repeated DOM ID %s", id)
			}
			ids[id] = true
		}
		if path := n.attr("data-instance-path"); path != "" && n.attr("data-binding") != "" {
			if paths[path] != nil {
				t.Errorf("duplicate binding path %s", path)
			}
			paths[path] = n
		}
	})
	board.walk(func(n *presentationSVG) {
		if href := n.attr("href"); strings.HasPrefix(href, "#") && !ids[href[1:]] {
			t.Errorf("local href lost its target: %s", href)
		}
		for _, attr := range n.attrs {
			for _, match := range svgURL.FindAllStringSubmatch(attr.Value, -1) {
				if !ids[strings.TrimPrefix(match[1], "#")] {
					t.Errorf("paint reference lost its target: %s", attr.Value)
				}
			}
		}
		if n.attr("data-diagram-source") == "authored" && (n.attr("viewBox") != "10 20 320 120" || n.attr("width") != "320" || n.attr("height") != "120") {
			t.Errorf("image-to-inline replacement changed the viewport: %+v", n.attrs)
		}
	})
	for _, frame := range []string{"first", "second"} {
		if capabilities[frame].Bindings != 2 || capabilities[frame].Mode != "authored" {
			t.Errorf("repeated instance missing bindings: %+v", capabilities[frame])
		}
		for _, element := range []string{"report-card", "report-flow"} {
			n := paths[frame+"/"+element]
			if n == nil || n.attr("data-documentation-revision") != pins["report"].Revision {
				t.Fatalf("missing exact binding for %s/%s", frame, element)
			}
			if element == "report-card" && (n.attr("data-diagram-entity") != "true" || n.attr("data-diagram-label") == "" || !capabilities[frame].EntityDetail) {
				t.Fatal("actual entity group lacks summary hooks")
			}
			if element == "report-flow" && n.attr("data-diagram-entity") != "" {
				t.Fatal("relationship was misrepresented as an entity group")
			}
			if element == "report-flow" && n.attr("data-source-relationship") != "produced-from-job" {
				t.Fatal("relationship binding became a whole-entity reference")
			}
		}
	}
	if !strings.Contains(string(got), `A &amp; B<tspan> report</tspan>`) {
		t.Fatal("mixed authored text lost its content/order")
	}
}

func TestWhiteboardUnsupportedAuthoredSourceKeepsWholeImage(t *testing.T) {
	t.Parallel()
	root, _, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pin := pins["reporting"]
	record, revision := inventory.Find(pin.Target), inventory.Pinned(pin)
	for _, content := range []string{
		`<style>rect{fill:red}</style><g id="report-card"><rect width="80" height="30"/></g>`,
		`<g id="report-card" style="fill:red"><rect width="80" height="30"/></g>`,
		`<g id="report-card"><image width="80" height="30" href="data:image/png;base64,iVBORw0KGgo="/></g>`,
	} {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120">` + content + `</svg>`
		setWhiteboardTestVisual(t, root, record, revision, svg)
		resolved := composeWhiteboardTestSource(t, pin, []byte(svg), revision.Bindings, "source")
		visual, err := inlineWhiteboardSVG(resolved.SVG)
		if err != nil {
			t.Fatal(err)
		}
		standalone := (&app{root: root}).makeERDView(inventory, record, revision)
		got, capabilities := (&app{root: root}).enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
		if standalone.Visual != "" || standalone.VisualNote == "" {
			t.Fatal("standalone safety policy unexpectedly accepted test source")
		}
		if capabilities["source"].Mode != "image" || capabilities["source"].Bindings != 0 || !strings.Contains(capabilities["source"].Note, standalone.VisualNote) {
			t.Fatalf("fallback not explicit or does not use shared policy: %+v", capabilities["source"])
		}
		if !strings.Contains(string(got), `href="data:image/svg+xml;base64,`) || strings.Contains(string(got), content) || strings.Contains(string(got), `data-diagram-source="authored"`) {
			t.Fatal("unsupported source was partially inlined instead of kept isolated")
		}
	}
}

func TestWhiteboardSourcePresentationValidatesDigestBeforeInlining(t *testing.T) {
	t.Parallel()
	root, _, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pin := pins["reporting"]
	record, revision := inventory.Find(pin.Target), inventory.Pinned(pin)
	resolved := composeWhiteboardTestSource(t, pin, []byte(reportingSVG), revision.Bindings, "source")
	path := filepath.Join(root, requirements.TechnicalPath(record.Kind, record.Identity.ID), revision.Visual.Path)
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(reportingSVG, "PDF report", "tampered")), 0600); err != nil {
		t.Fatal(err)
	}
	visual, _ := inlineWhiteboardSVG(resolved.SVG)
	got, capabilities := (&app{root: root}).enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
	if capabilities["source"].Mode != "image" || strings.Contains(string(got), "tampered") || !strings.Contains(capabilities["source"].Note, "digest") {
		t.Fatalf("changed source asset escaped immutable validation: %+v", capabilities["source"])
	}
}

func setWhiteboardTestVisual(t *testing.T, root string, record *requirements.TechnicalRecord, revision *requirements.TechnicalRevision, svg string) {
	t.Helper()
	revision.Visual = &requirements.Visual{Path: "assets/test-presentation.svg", MediaType: "image/svg+xml", Digest: coderef.DigestBytes([]byte(svg))}
	path := filepath.Join(root, requirements.TechnicalPath(record.Kind, record.Identity.ID), revision.Visual.Path)
	if err := os.WriteFile(path, []byte(svg), 0600); err != nil {
		t.Fatal(err)
	}
}

func composeWhiteboardTestSource(t *testing.T, pin saga.DocumentationLink, svg []byte, bindings []requirements.Binding, frames ...string) *requirements.ResolvedWhiteboard {
	t.Helper()
	doc := diagram.Document{Version: 1, Width: 1000, Height: 300}
	for i, id := range frames {
		doc.Elements = append(doc.Elements, diagram.Element{ID: id, Kind: "frame", X: float64(i) * 400, Y: 50, Width: 350, Height: 150, Style: "normal", Source: &diagram.SourceReference{Target: pin.Target, Revision: pin.Revision}})
	}
	composition, err := diagram.ResolveComposition(doc, func(source diagram.SourceReference) (diagram.ResolvedSource, error) {
		resolved := diagram.ResolvedSource{SVG: svg}
		for _, binding := range bindings {
			ref := &diagram.SourceReference{}
			if binding.Entity != nil {
				ref.Target, ref.Revision = binding.Entity.Target, binding.Entity.Revision
			} else if binding.Relationship != nil {
				ref.Target, ref.Revision, ref.Relationship = binding.Relationship.Owner.Target, binding.Relationship.Owner.Revision, binding.Relationship.ID
			}
			resolved.Bindings = append(resolved.Bindings, diagram.Binding{ID: binding.Element, Source: ref})
		}
		return resolved, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := composition.Render(diagram.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return &requirements.ResolvedWhiteboard{Composition: composition, SVG: rendered}
}

func TestWhiteboardStructuredSourceRetainsSharedNoteCodeAndRevealMetadata(t *testing.T) {
	t.Parallel()
	child := diagram.Document{Version: 1, Width: 800, Height: 400, Reveal: "fade", Elements: []diagram.Element{
		{ID: "code", Kind: "code", Label: "Read account", Description: "Exact authored example", Note: "Check the **saved revision** before applying this example.", Language: "go", Code: "// <script> is text\nread(account)", X: 20, Y: 20, Width: 740, Height: 280, Style: "code", LineNumbers: true, HighlightLines: []int{2}},
	}}
	pin := &diagram.SourceReference{Target: "urn:test:erd:example", Revision: "urn:test:erd:example:revision:r1"}
	doc := diagram.Document{Version: 1, Width: 900, Height: 500, Elements: []diagram.Element{{ID: "example", Kind: "frame", Width: 800, Height: 400, Style: "normal", Source: pin}}}
	composition, err := diagram.ResolveComposition(doc, func(diagram.SourceReference) (diagram.ResolvedSource, error) {
		return diagram.ResolvedSource{Document: &child}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	svg, err := composition.Render(diagram.Options{})
	if err != nil {
		t.Fatal(err)
	}
	visual, err := inlineWhiteboardSVG(svg)
	if err != nil {
		t.Fatal(err)
	}
	got, capabilities := (&app{}).enhanceWhiteboardSources(requirements.Inventory{}, &requirements.ResolvedWhiteboard{Composition: composition, SVG: svg}, template.HTML(visual))
	capability := capabilities["example"]
	if !capability.Code || !capability.Reveal || capability.FieldDetail || capability.Landmarks {
		t.Fatalf("shared diagram metadata described incorrectly: %+v", capability)
	}
	for _, want := range []string{`data-language="go"`, `data-code-highlight="2"`, `data-code-line="2"`, `data-reveal-step="1"`, `xml:space="preserve"`, `<title>Exact authored example</title>`, `saved revision`, `&lt;script&gt; is text`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("lost existing diagram capability: %s", want)
		}
	}
	if strings.Contains(string(got), "<script>") || strings.Contains(string(got), "<style>") {
		t.Fatal("active markup or standalone stylesheet entered the app")
	}
}

func TestWhiteboardSourceRejectsActiveOrExternalMarkupBeforeInlining(t *testing.T) {
	t.Parallel()
	root, _, pins := dataModelFixture(t)
	inventory, err := requirements.LoadInventory(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	pin := pins["reporting"]
	record, revision := inventory.Find(pin.Target), inventory.Pinned(pin)
	resolved := composeWhiteboardTestSource(t, pin, []byte(reportingSVG), revision.Bindings, "source")
	visual, _ := inlineWhiteboardSVG(resolved.SVG)
	for _, content := range []string{
		`<script>alert(1)</script>`,
		`<rect onmouseover="alert(1)"/>`,
		`<foreignObject><div xmlns="http://www.w3.org/1999/xhtml">active</div></foreignObject>`,
		`<use href="https://example.test/remote.svg#x"/>`,
		`<rect fill="url(https://example.test/remote.svg#x)"/>`,
		`<g data-documentation-target="urn:forged"/>`,
	} {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 320 120"><g id="report-card"><rect width="100" height="100"/></g>` + content + `</svg>`
		setWhiteboardTestVisual(t, root, record, revision, svg)
		got, capabilities := (&app{root: root}).enhanceWhiteboardSources(inventory, resolved, template.HTML(visual))
		if capabilities["source"].Mode != "image" || capabilities["source"].Bindings != 0 || strings.Contains(string(got), content) {
			t.Errorf("unsafe source inlined: %s", content)
		}
	}
}
