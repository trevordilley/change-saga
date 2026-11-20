package prototypes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCurrentHTMLPrototypePreservesAssetsWithoutRevisions(t *testing.T) {
	root := newSaga(t)
	source := filepath.Join(t.TempDir(), "index.html")
	mustWrite(t, source, "<button id=buy>Buy</button>")
	if _, err := AddHTML(root, "test", AddHTMLInput{Feature: "core", ID: "editable", Title: "Editable", State: StateReady, SourcePath: source}); err != nil {
		t.Fatal(err)
	}
	doc, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	prototype := doc.Find("editable")
	dir, err := doc.RevisionDir(*prototype, prototype.CurrentRevision.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "html", "index.html"))
	if err != nil || string(data) != "<button id=buy>Buy</button>" {
		t.Fatalf("asset: %s %v", data, err)
	}
	if _, err := Revise(root, "test", ReviseInput{Prototype: "urn:change-saga:test:prototype:editable", Title: "Edited", State: StateReady, Source: prototype.CurrentRevision.Source}); err != nil {
		t.Fatal(err)
	}
	doc, err = Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Find("editable").CurrentRevision.Title != "Edited" {
		t.Fatal("edit lost")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(prototypePath("core", "editable")), "revisions")); !os.IsNotExist(err) {
		t.Fatalf("internal prototype history retained: %v", err)
	}
}
