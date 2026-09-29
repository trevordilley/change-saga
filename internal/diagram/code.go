package diagram

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/image/font/sfnt"
)

const (
	MaxCodeRunes = 16000
	MaxCodeLines = 100
	codePadding  = 16.0
)

var codeFont = sync.OnceValues(func() (*sfnt.Font, error) {
	data, _ := MonoFont()
	return sfnt.Parse(data)
})

func validateCode(e Element, fail func(string, ...any)) {
	if e.Kind != "code" {
		if e.Code != "" || e.Language != "" || e.LineNumbers || len(e.HighlightLines) != 0 {
			fail("code, language, line_numbers, and highlight_lines are only valid on code elements")
		}
		return
	}
	if strings.TrimSpace(e.Code) == "" || utf8.RuneCountInString(e.Code) > MaxCodeRunes {
		fail("code needs nonblank source of at most %d characters", MaxCodeRunes)
	}
	if e.Width <= 0 || e.Height <= 0 {
		fail("code needs an explicit positive width and height")
	}
	if e.Shape != "" || e.Detail != "" || e.Wrap || (e.Align != "" && e.Align != "start") {
		fail("code uses unwrapped, left-aligned source; shape, detail, wrap, and other alignments are not supported")
	}
	if utf8.RuneCountInString(e.Language) > 64 || strings.ContainsAny(e.Language, "\r\n\t") {
		fail("language must be a single-line label of at most 64 characters")
	}
	for _, char := range e.Code + e.Language {
		if unicode.IsControl(char) && char != '\n' && char != '\r' && char != '\t' {
			fail("code and language cannot contain control characters")
			break
		}
	}
	lines := codeLines(e.Code)
	if len(lines) > MaxCodeLines {
		fail("code may contain at most %d lines", MaxCodeLines)
	}
	seen := map[int]bool{}
	for _, line := range e.HighlightLines {
		if line < 1 || line > len(lines) || seen[line] {
			fail("highlight_lines must contain unique 1-based line numbers within the source")
			break
		}
		seen[line] = true
	}
}

// Source stays untouched in the record. Display uses four-column tab stops and
// normalizes platform line endings, while retaining indentation and blank lines.
func codeLines(source string) []string {
	source = strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		var out strings.Builder
		column := 0
		for _, char := range line {
			if char == '\t' {
				spaces := 4 - column%4
				out.WriteString(strings.Repeat(" ", spaces))
				column += spaces
			} else {
				out.WriteRune(char)
				column++
			}
		}
		lines[index] = out.String()
	}
	return lines
}

func (r *renderer) codeExample(e Element, style Style, g *node) error {
	face, err := codeFont()
	if err != nil {
		return fmt.Errorf("code font: %w", err)
	}
	mono := *r
	mono.face = face
	lines := codeLines(e.Code)
	title := e.Label
	if e.Language != "" {
		if title != "" {
			title += " · "
		}
		title += e.Language
		g.set("data-language", e.Language)
	}
	top := codePadding
	if title != "" {
		top += style.FontSize*lineHeight + codePadding
	}
	need := top + float64(len(lines))*style.FontSize*lineHeight + codePadding
	if need > e.Height+.01 {
		return fmt.Errorf("code overflow: %d lines and header need height %.1f, box has %.1f", len(lines), need, e.Height)
	}
	left := codePadding
	if e.LineNumbers {
		width, err := mono.measure(strconv.Itoa(len(lines)), style.FontSize)
		if err != nil {
			return err
		}
		left += width*widthSlack + codePadding
	}
	available := e.Width - left - codePadding
	for index, line := range lines {
		width, err := mono.measure(line, style.FontSize)
		if err != nil {
			return err
		}
		if available <= 0 || width*widthSlack > available+.01 {
			return fmt.Errorf("code overflow: line %d needs width %.1f, box has %.1f after padding and line numbers; widen the element or shorten the source", index+1, width*widthSlack, available)
		}
	}
	paint(g.add("rect", "width", num(e.Width), "height", num(e.Height), "rx", "8"), style)
	if title != "" {
		if err := r.text(g, title, Box{X: codePadding, Y: codePadding, Width: e.Width - 2*codePadding, Height: style.FontSize * lineHeight}, style.FontSize, style.Ink, false, "start"); err != nil {
			return err
		}
		g.add("path", "d", fmt.Sprintf("M0 %sH%s", num(top-codePadding/2), num(e.Width)), "stroke", style.Stroke, "stroke-width", num(style.StrokeWidth))
	}
	for _, line := range e.HighlightLines {
		g.add("rect", "data-code-highlight", strconv.Itoa(line), "x", "1", "y", num(top+float64(line-1)*style.FontSize*lineHeight), "width", num(e.Width-2), "height", num(style.FontSize*lineHeight), "fill", "@diagram-primary-fill", "aria-hidden", "true")
	}
	code := g.add("text", "class", "diagram-code", "font-size", num(style.FontSize), "fill", style.Ink, "xml:space", "preserve")
	for index, line := range lines {
		y := top + style.FontSize + float64(index)*style.FontSize*lineHeight
		code.add("tspan", "data-code-line", strconv.Itoa(index+1), "x", num(left), "y", num(y)).text = line
		if e.LineNumbers {
			g.add("text", "class", "diagram-code", "x", num(left-codePadding), "y", num(y), "text-anchor", "end", "font-size", num(style.FontSize), "fill", style.Ink, "fill-opacity", ".55", "aria-hidden", "true").text = strconv.Itoa(index + 1)
		}
	}
	return nil
}

func codeContract() map[string]any {
	return map[string]any{
		"kind": "code", "source": "code", "language": "optional display label, not an executable language selector",
		"line_numbers": "optional, start at 1", "highlight_lines": "optional unique 1-based line numbers",
		"font": MonoFontPath, "default_style": "code", "max_runes": MaxCodeRunes, "max_lines": MaxCodeLines,
		"layout":   "explicit box; bundled monospace, four-column tabs, preserved blank lines; overflow is rejected, never wrapped or shrunk",
		"evidence": "select the code element with an Item of kind example; source text is an illustration, not automatic code evidence",
	}
}
