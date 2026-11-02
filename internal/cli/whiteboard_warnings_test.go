package cli

import (
	"github.com/twentyideas/changesaga/internal/diagram"
	"testing"
)

func TestWhiteboardNoteRepositoryWarningsDoNotAssumeLinkShape(t *testing.T) {
	for _, note := range []string{
		"[implementation](https://github.com/acme/app/blob/main/app.go#L2)",
		"<https://github.com/acme/app/tree/main/src>",
		"The details are at https://github.com/acme/app/commit/123.",
		"[implementation][source]\n\n[source]: https://github.com/acme/app/blob/123/app.go",
		"[source](https://code.acme.test/acme/app/src/main/app.go)",
	} {
		doc := diagram.Document{Elements: []diagram.Element{{ID: "owner", Note: note}}}
		warnings := whiteboardNoteWarnings(doc, "https://code.acme.test/acme/app.git")
		if len(warnings) != 1 || warnings[0].Element != "owner" {
			t.Errorf("note %q: %+v", note, warnings)
		}
		if doc.Elements[0].Note != note {
			t.Fatal("warning rewrote note")
		}
	}
	if got := whiteboardNoteWarnings(diagram.Document{Elements: []diagram.Element{{Note: "[specification](https://example.org/docs)"}}}, "https://github.com/acme/app.git"); len(got) != 0 {
		t.Fatalf("unrelated documentation warned: %+v", got)
	}
}
