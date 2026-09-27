package diagram

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// noted is the sample diagram with notes on a node, an edge, a group, and a
// text element, including Markdown and characters XML must escape.
func noted() Document {
	d := sample()
	notes := map[string]string{
		"store":   "Holds **one record** per batch.\n\n- written by `publish`\n- read by [the reviewer](https://example.com/review)",
		"publish": "Retries on *conflict*; gives up after 3 attempts & reports `<why>`.",
		"alt":     "Only taken when the snapshot matches.",
	}
	for index := range d.Elements {
		d.Elements[index].Note = notes[d.Elements[index].ID]
	}
	return d
}

func TestNoteValidation(t *testing.T) {
	valid := New()
	valid.Elements = []Element{{ID: "box", Kind: "node", Shape: "rect", Width: 100, Height: 60, Style: "normal",
		Note: "**Owns** the queue. See <https://example.com> or [mail](mailto:a@example.com), www.example.com, and a@example.com.\n\n1. first\n2. `second`\n   - nested"}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("a note in the supported format was refused: %v", err)
	}
	valid.Elements[0].Note = strings.Repeat("é", MaxNoteRunes)
	if err := valid.Validate(); err != nil {
		t.Fatalf("a note of exactly %d characters was refused: %v", MaxNoteRunes, err)
	}
	for note, want := range map[string]string{
		strings.Repeat("a", MaxNoteRunes+1): "note is 1001 characters; a note may carry at most 1000",
		"  \n ":                             "note is blank",
		"Hi <b>there</b>":                   "note uses raw HTML; a note may use only bold, italics, inline code, lists, and http, https, or mailto links (write a literal < as \\< or inside `code`)",
		"<div>\nblock\n</div>":              "note uses raw HTML",
		"# Heading":                         "note uses headings",
		"![alt](https://example.com/x.png)": "note uses images",
		"[x](javascript:alert(1))":          `note link "javascript:alert(1)" must use http, https, or mailto`,
		"[x](/relative)":                    `note link "/relative" must use http, https, or mailto`,
		"<javascript:alert(1)>":             `note link "javascript:alert(1)" must use http, https, or mailto`,
		"```\ncode\n```":                    "note uses code blocks",
		"> quoted":                          "note uses block quotes",
		"| a |\n| - |\n| b |":               "note uses tables",
		"see[^1]\n\n[^1]: cite":             "note uses footnotes",
		"~~gone~~":                          "note uses strikethrough",
		"- [x] done":                        "note uses task checkboxes",
	} {
		d := New()
		d.Elements = []Element{{ID: "box", Kind: "node", Shape: "rect", Width: 100, Height: 60, Style: "normal", Note: note}}
		err := d.Validate()
		if err == nil || !strings.Contains(err.Error(), "box: "+want) {
			t.Errorf("note %q: want %q, got %v", note, want, err)
		}
	}
	d := New()
	d.Elements = []Element{{ID: "flourish", Kind: "text", Label: "x", Width: 100, Height: 60, Style: "normal", Decorative: true, Note: "Why"}}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "flourish: a decorative element is hidden from readers") {
		t.Fatalf("a decorative element's note must be refused: %v", err)
	}
}

func TestNoteText(t *testing.T) {
	got := NoteText("Holds **one record** per batch &amp; *more*\\!\nsoft break.\n\n- written by `publish`\n- read by [the reviewer](https://example.com)\n  1. nested\n\n3. three\n4. four")
	want := "Holds one record per batch & more! soft break.\n- written by publish\n- read by the reviewer\n  1. nested\n3. three\n4. four"
	if got != want {
		t.Fatalf("plain projection:\n got %q\nwant %q", got, want)
	}
}

func TestRenderWithNotesIsDeterministicAndMatchesGolden(t *testing.T) {
	options := Options{Title: "Publishing one batch", Description: "One record publishes source and SVG together."}
	first, err := Render(noted(), options)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := Render(noted(), options)
	if !bytes.Equal(first, second) {
		t.Fatal("rendering with notes is not deterministic")
	}
	golden := filepath.Join("testdata", "notes.svg")
	if *update {
		if err := os.WriteFile(golden, first, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/diagram -update)", err)
	}
	if !bytes.Equal(first, want) {
		t.Fatal("renderer output with notes changed; if intended, run with -update")
	}
	svg := string(first)
	for _, want := range []string{
		"<desc>Holds one record per batch.\n- written by publish\n- read by the reviewer</desc>",
		"<desc>Retries on conflict; gives up after 3 attempts &amp; reports &lt;why&gt;.</desc>",
		"<desc>Only taken when the snapshot matches.</desc>",
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("SVG lacks %s", want)
		}
	}
	if strings.Contains(svg, "**") || strings.Contains(svg, "<why>") {
		t.Fatal("the SVG must carry the note as escaped plain text")
	}
	// A note adds only its desc: removing every note restores the exact bytes
	// of a diagram that never had one.
	plain, err := Render(sample(), options)
	if err != nil {
		t.Fatal(err)
	}
	stripped := regexpDesc.ReplaceAll(first, nil)
	if !bytes.Equal(stripped, plain) {
		t.Fatal("a note changed more of the rendering than its desc")
	}
}

var regexpDesc = regexp.MustCompile(`\n *<desc>[^<]*</desc>`)

func TestNoteDescribeAndEditRoundTrip(t *testing.T) {
	var b strings.Builder
	Describe(noted(), 0, 30).WriteText(&b)
	text := b.String()
	for _, want := range []string{
		"  alt \"alt  [snapshot matches]\" shape=boundary\n    note: Only taken when the snapshot matches.\n",
		"    note: \"Holds **one record** per batch.\\n\\n- written by `publish`\\n- read by [the reviewer](https://example.com/review)\"\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("describe lacks %q in:\n%s", want, text)
		}
	}
	summary := Describe(noted(), 0, 30)
	data, _ := json.Marshal(summary)
	if !strings.Contains(string(data), `"note":"Only taken when the snapshot matches."`) {
		t.Fatalf("describe JSON lacks the note: %s", data)
	}

	set, changed, err := Edit(sample(), []Operation{{Op: "update", ID: "author", Set: json.RawMessage(`{"note":"Signs **every** batch."}`)}})
	if author, _ := set.Element("author"); err != nil || author.Note != "Signs **every** batch." || strings.Join(changed, ",") != "author" {
		t.Fatalf("set note: %+v %v %v", author, changed, err)
	}
	if err := set.Validate(); err != nil {
		t.Fatal(err)
	}
	cleared, _, err := Edit(set, []Operation{{Op: "update", ID: "author", Set: json.RawMessage(`{"note":null}`)}})
	if author, _ := cleared.Element("author"); err != nil || author.Note != "" {
		t.Fatalf("clear note: %+v %v", author, err)
	}
	encoded, _ := Encode(set)
	decoded, err := Decode(encoded)
	if author, _ := decoded.Element("author"); err != nil || author.Note != "Signs **every** batch." {
		t.Fatalf("stored source must round-trip the note: %v", err)
	}
	if encoded, _ := Encode(cleared); strings.Contains(string(encoded), `"note"`) {
		t.Fatal("a cleared note must leave no field in the stored source")
	}
}

func TestSchemaBoundsNotes(t *testing.T) {
	schema := compileSchema(t)
	data, _ := Encode(noted())
	value, _ := jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if err := schema.Validate(value); err != nil {
		t.Fatalf("a noted source does not satisfy its schema: %v", err)
	}
	long := noted()
	long.Elements[1].Note = strings.Repeat("a", MaxNoteRunes+1)
	data, _ = Encode(long)
	value, _ = jsonschema.UnmarshalJSON(strings.NewReader(string(data)))
	if schema.Validate(value) == nil {
		t.Fatal("schema accepted an overlong note")
	}
}
