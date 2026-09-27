package diagram

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// MaxNoteRunes bounds an element's note. A note is read in a hover popover
// beside the element, so it is sized for a glance rather than a page: about
// 150 words, or a short paragraph and a three-item list, fits a popover of
// roughly 22rem without scrolling. An explanation that needs more belongs in a
// callout Item or the Saga's documentation, where a reader can navigate it.
const MaxNoteRunes = 1000

// NoteFormat names what a note may contain, for errors and discovery.
const NoteFormat = "bold, italics, inline code, lists, and http, https, or mailto links"

// noteMarkdown parses notes with the same extensions the reviewer renders
// authored Markdown with, so validation sees exactly the structure a reader
// would: anything the reviewer would render beyond the note format is refused
// here rather than silently rendered there.
var noteMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote))

func parseNote(note string) (ast.Node, []byte) {
	source := []byte(note)
	return noteMarkdown.Parser().Parse(text.NewReader(source)), source
}

// noteRefusals names every Markdown construct a note may not use.
var noteRefusals = map[ast.NodeKind]string{
	ast.KindHeading:             "headings",
	ast.KindThematicBreak:       "thematic breaks",
	ast.KindCodeBlock:           "code blocks",
	ast.KindFencedCodeBlock:     "code blocks",
	ast.KindBlockquote:          "block quotes",
	ast.KindHTMLBlock:           "raw HTML",
	ast.KindRawHTML:             "raw HTML",
	ast.KindImage:               "images",
	extast.KindTable:            "tables",
	extast.KindTaskCheckBox:     "task checkboxes",
	extast.KindStrikethrough:    "strikethrough",
	extast.KindFootnote:         "footnotes",
	extast.KindFootnoteLink:     "footnotes",
	extast.KindFootnoteList:     "footnotes",
	extast.KindFootnoteBacklink: "footnotes",
}

// validateNote reports each distinct problem with note, which the caller has
// already found non-empty.
func validateNote(note string) []string {
	problems := []string{}
	if strings.TrimSpace(note) == "" {
		return append(problems, "note is blank; omit it instead")
	}
	if count := len([]rune(note)); count > MaxNoteRunes {
		problems = append(problems, fmt.Sprintf("note is %d characters; a note may carry at most %d", count, MaxNoteRunes))
	}
	document, source := parseNote(note)
	seen := map[string]bool{}
	report := func(problem string) {
		if !seen[problem] {
			seen[problem] = true
			problems = append(problems, problem)
		}
	}
	_ = ast.Walk(document, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if refused, ok := noteRefusals[n.Kind()]; ok {
			problem := fmt.Sprintf("note uses %s; a note may use only %s", refused, NoteFormat)
			if refused == "raw HTML" {
				problem += " (write a literal < as \\< or inside `code`)"
			}
			report(problem)
			return ast.WalkSkipChildren, nil
		}
		switch link := n.(type) {
		case *ast.Link:
			if !safeNoteURL(string(link.Destination), false, false) {
				report(fmt.Sprintf("note link %s must use http, https, or mailto", strconv.Quote(string(link.Destination))))
			}
		case *ast.AutoLink:
			if !safeNoteURL(string(link.URL(source)), link.AutoLinkType == ast.AutoLinkEmail, true) {
				report(fmt.Sprintf("note link %s must use http, https, or mailto", strconv.Quote(string(link.URL(source)))))
			}
		}
		return ast.WalkContinue, nil
	})
	// A note of only link reference definitions, or of links without text,
	// renders nothing a reader could see.
	if len(problems) == 0 && NoteText(note) == "" {
		problems = append(problems, "note renders no text; omit it instead")
	}
	return problems
}

// safeNoteURL accepts http, https, and mailto links. A bare www. address is
// accepted only as a GFM autolink, which renders it as http; as an ordinary
// link destination it would render as a broken relative link.
func safeNoteURL(destination string, email, autolink bool) bool {
	if email {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(destination))
	for _, prefix := range []string{"https://", "http://", "mailto:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return autolink && strings.HasPrefix(lower, "www.")
}

// NoteText projects a note's Markdown to plain text for places that cannot
// render it, such as the SVG desc a screen reader announces: emphasis and code
// markers drop away, links keep their text, and list items keep a marker.
func NoteText(note string) string {
	document, source := parseNote(note)
	lines := noteBlocks(document, source)
	return strings.Join(lines, "\n")
}

func noteBlocks(parent ast.Node, source []byte) []string {
	lines := []string{}
	for block := parent.FirstChild(); block != nil; block = block.NextSibling() {
		list, ok := block.(*ast.List)
		if !ok {
			var b strings.Builder
			noteInline(block, source, &b)
			for _, line := range strings.Split(b.String(), "\n") {
				if line = strings.TrimSpace(line); line != "" {
					lines = append(lines, line)
				}
			}
			continue
		}
		number := list.Start
		for item := list.FirstChild(); item != nil; item = item.NextSibling() {
			marker := "- "
			if list.IsOrdered() {
				marker = strconv.Itoa(number) + ". "
				number++
			}
			for index, line := range noteBlocks(item, source) {
				if index == 0 {
					lines = append(lines, marker+line)
				} else {
					lines = append(lines, "  "+line)
				}
			}
		}
	}
	return lines
}

func noteInline(parent ast.Node, source []byte, b *strings.Builder) {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			b.Write(plainValue(n.Segment.Value(source)))
			if n.HardLineBreak() {
				b.WriteString("\n")
			} else if n.SoftLineBreak() {
				b.WriteString(" ")
			}
		case *ast.String:
			b.Write(plainValue(n.Value))
		case *ast.AutoLink:
			b.Write(n.Label(source))
		case *ast.CodeSpan:
			for segment := n.FirstChild(); segment != nil; segment = segment.NextSibling() {
				if t, ok := segment.(*ast.Text); ok {
					b.Write(t.Segment.Value(source))
				}
			}
		default:
			noteInline(child, source, b)
		}
	}
}

func plainValue(value []byte) []byte {
	return util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(bytes.Clone(value))))
}
