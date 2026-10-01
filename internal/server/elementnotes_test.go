package server

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// A source on disk is not re-validated before a page renders it, so a
// tampered note must still reach the page only as sanitized Markdown.
func TestElementNotesRenderSanitizedAndEscaped(t *testing.T) {
	dir := t.TempDir()
	source := `{"version":1,"width":100,"height":100,"elements":[` +
		`{"id":"a","kind":"node","shape":"rect","x":0,"y":0,"width":10,"height":10,"style":"normal","label":"A <i>","note":"**bold** [x](javascript:alert(1)) <img src=x onerror=alert(1)>\n\n<script>alert(1)</script>"},` +
		`{"id":"b","kind":"node","shape":"rect","x":0,"y":0,"width":10,"height":10,"style":"normal","label":"B","note":"Plain"},` +
		`{"id":"c","kind":"node","shape":"rect","x":0,"y":0,"width":10,"height":10,"style":"normal","decorative":true,"note":"Hidden"}]}`
	if err := os.WriteFile(filepath.Join(dir, "24-a-source.json"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	notes := loadSlideNotes(dir, &saga.DiagramSource{Source: "24-a-source.json"}, "slide")
	if len(notes.order) != 2 {
		t.Fatalf("decorative elements carry no readable note: %v", notes.order)
	}
	var out bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&out, "element-note-targets", notes.unbound(map[string]bool{"b": true})); err != nil {
		t.Fatal(err)
	}
	page := out.String()
	for _, want := range []string{`data-element-id="a" data-element-name="A &lt;i&gt;" hidden>`, "<strong>bold</strong>", "&lt;script&gt;"} {
		if !strings.Contains(page, want) {
			t.Errorf("note targets lack %s:\n%s", want, page)
		}
	}
	for _, refused := range []string{"<script", "<img", `href="javascript`, `data-element-id="b"`} {
		if strings.Contains(page, refused) {
			t.Errorf("note targets carry %s:\n%s", refused, page)
		}
	}
	out.Reset()
	popover := notePopoverView{Label: "Label <b>", Description: "Why & how", Body: "Surprise <i>", Note: notes.html["b"]}
	pages, err := newPageTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if err := pages.ExecuteTemplate(&out, "element-note-template", popover); err != nil {
		t.Fatal(err)
	}
	if want := `<template data-landmark-note-template><div class="element-note"><p class="element-note-label">Label &lt;b&gt;</p><p class="element-note-description">Why &amp; how</p><p class="element-note-body">Surprise &lt;i&gt;</p><div class="element-note-markdown"><p>Plain</p>`; !strings.Contains(out.String(), want) {
		t.Fatalf("popover template = %s", out.String())
	}
	if empty := loadSlideNotes(dir, nil, "slide"); len(empty.order) != 0 {
		t.Fatal("a hand-authored slide has no notes")
	}
}

func TestItemNotesRequireExplicitDetail(t *testing.T) {
	item := &saga.Item{ItemManifest: saga.ItemManifest{Kind: "node", Label: "Cache", Description: "Cache"}}
	view := &reviewItemView{Item: item}
	var out bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&out, "element-note-template", view.Popover()); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("ordinary Item created a repetitive note: %s", out.String())
	}
	view.Note = markdownWithAnchors("Entries expire after **five minutes**.", "note")
	if err := reviewTemplates.ExecuteTemplate(&out, "element-note-template", view.Popover()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "five minutes") || strings.Contains(out.String(), "element-note-description") {
		t.Fatalf("expected explicit detail only: %s", out.String())
	}
	out.Reset()
	view.Note = ""
	item.Kind, item.Body = "callout", "An expired entry is served while refresh runs."
	if err := reviewTemplates.ExecuteTemplate(&out, "element-note-template", view.Popover()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), item.Body) {
		t.Fatalf("surprise lost its explanation: %s", out.String())
	}
}
