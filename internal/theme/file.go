package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileName is the theme file's name at the root of a Saga. It is a fixed
// path rather than a saga.json field so a Saga has at most one theme, found
// without reading anything else, and saga.json's strict schema is unchanged.
const FileName = "theme.css"

// maxFileBytes bounds the theme file: every token with a long value fits in a
// small fraction of it.
const maxFileBytes = 64 << 10

// darkSelector is the one dark-mode selector, in its canonical spelling.
const darkSelector = `:root[data-theme="dark"]`

// Override is one validated token value. Value is re-serialized from its
// parsed form, never the file's raw text.
type Override struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Line  int    `json:"line"`
}

// File is a validated theme: the tokens it overrides in light and dark mode.
type File struct {
	Light []Override `json:"light"`
	Dark  []Override `json:"dark"`
}

// Problem is one reason a theme file is refused, at the line it starts on.
type Problem struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

func (problem Problem) String() string {
	return fmt.Sprintf("line %d: %s", problem.Line, problem.Message)
}

// InvalidError names every problem of a refused theme file.
type InvalidError struct {
	Path     string
	Problems []Problem
}

func (err *InvalidError) Error() string {
	lines := make([]string, len(err.Problems))
	for i, problem := range err.Problems {
		lines[i] = err.Path + ":" + problem.String()
	}
	return "invalid theme: " + strings.Join(lines, "; ")
}

// Load reads and validates the theme file of the Saga at root. A Saga with
// no theme file has none: Load returns nil and no error.
func Load(root string) (*File, error) {
	path := filepath.Join(root, FileName)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, &InvalidError{Path: FileName, Problems: []Problem{{Line: 1, Message: "the theme must be a regular file"}}}
	}
	if info.Size() > maxFileBytes {
		return nil, &InvalidError{Path: FileName, Problems: []Problem{{Line: 1, Message: fmt.Sprintf("the theme is larger than %d bytes", maxFileBytes)}}}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, problems := Parse(data)
	if len(problems) > 0 {
		return nil, &InvalidError{Path: FileName, Problems: problems}
	}
	return file, nil
}

// Parse validates a theme file strictly. It accepts only comments, one :root
// block of light overrides, and one :root[data-theme="dark"] block of dark
// overrides, each holding only custom properties the contract names, with
// values that parse as the token's kind. There are no @-rules, url(),
// escapes, !important, or other selectors, so a theme can recolour the
// reviewer but never restyle, hide, or load anything.
func Parse(data []byte) (*File, []Problem) {
	src, unterminated := stripComments(data)
	p := &parser{src: src, line: 1}
	if unterminated > 0 {
		p.fail(unterminated, "unterminated comment")
	}
	if p.refuseForbidden(); len(p.problems) > 0 {
		return nil, p.problems
	}
	file := &File{}
	seen := map[string]bool{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			break
		}
		line := p.line
		selector, stop := p.scanTo("{};")
		if stop != '{' {
			p.fail(line, fmt.Sprintf("unexpected %q outside a block; a theme holds only :root { } and %s { }", compactSpace(selector+string(stop)), darkSelector))
			break
		}
		scheme := selectorScheme(selector)
		if scheme == "" {
			p.fail(line, fmt.Sprintf("selector %q is not allowed; a theme holds only :root { } and %s { }", compactSpace(selector), darkSelector))
		} else if seen[scheme] {
			p.fail(line, fmt.Sprintf("a second %s block; declare each scheme once", compactSpace(selector)))
		}
		overrides := p.declarations(scheme != "")
		if scheme != "" && !seen[scheme] {
			seen[scheme] = true
			if scheme == "light" {
				file.Light = overrides
			} else {
				file.Dark = overrides
			}
		}
	}
	if len(p.problems) > 0 {
		sort.SliceStable(p.problems, func(i, j int) bool { return p.problems[i].Line < p.problems[j].Line })
		return nil, p.problems
	}
	return file, nil
}

type parser struct {
	src       []byte
	pos, line int
	problems  []Problem
}

func (p *parser) fail(line int, message string) {
	p.problems = append(p.problems, Problem{Line: line, Message: message})
}

func (p *parser) skipSpace() {
	for p.pos < len(p.src) && isSpace(p.src[p.pos]) {
		if p.src[p.pos] == '\n' {
			p.line++
		}
		p.pos++
	}
}

// scanTo reads up to the first of stops outside a quoted string and consumes
// it. stop is 0 at the end of the file.
func (p *parser) scanTo(stops string) (text string, stop byte) {
	start := p.pos
	var quote byte
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case quote != 0:
			if c == quote || c == '\n' {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case strings.IndexByte(stops, c) >= 0:
			p.pos++
			return string(p.src[start : p.pos-1]), c
		}
		if c == '\n' {
			p.line++
		}
		p.pos++
	}
	return string(p.src[start:]), 0
}

// declarations reads one block's custom properties through its closing
// brace. A structural error ends the parse, since nothing after it can be
// trusted to mean what it seems to.
func (p *parser) declarations(keep bool) []Override {
	open := p.line
	overrides := []Override{}
	seen := map[string]bool{}
	for {
		p.skipSpace()
		if p.pos >= len(p.src) {
			p.fail(open, "block is not closed with }")
			return overrides
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return overrides
		}
		line := p.line
		text, stop := p.scanTo(";{}")
		if stop == '{' {
			p.fail(line, "unexpected { inside a block; a theme sets token values and cannot nest rules")
			p.pos = len(p.src)
			return overrides
		}
		if stop == '}' {
			p.pos--
		}
		if override, ok := p.declaration(line, text); ok && keep {
			if seen[override.Name] {
				p.fail(line, fmt.Sprintf("--%s is set twice in one block", override.Name))
			}
			seen[override.Name] = true
			overrides = append(overrides, override)
		}
	}
}

func (p *parser) declaration(line int, text string) (Override, bool) {
	name, value, found := strings.Cut(text, ":")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !found {
		p.fail(line, fmt.Sprintf("%q is not a declaration; write --token: value;", compactSpace(text)))
		return Override{}, false
	}
	if !strings.HasPrefix(name, "--") {
		p.fail(line, fmt.Sprintf("property %q is not allowed; a theme sets only --token custom properties", name))
		return Override{}, false
	}
	token, ok := Lookup(strings.TrimPrefix(name, "--"))
	if !ok {
		p.fail(line, fmt.Sprintf("unknown token %s; change-saga spec lists every token under theme", name))
		return Override{}, false
	}
	canonical, err := ParseValue(token.Kind, value)
	if err != nil {
		p.fail(line, fmt.Sprintf("%s: %v", name, err))
		return Override{}, false
	}
	return Override{Name: token.Name, Value: canonical, Line: line}, true
}

// refuseForbidden names each construct no theme may hold, wherever it
// appears outside a comment: they are refused outright, not parsed around.
func (p *parser) refuseForbidden() {
	line := 1
	for i := 0; i < len(p.src); i++ {
		c := p.src[i]
		switch {
		case c == '\n':
			line++
		case c == '\\':
			p.fail(line, "backslash escapes are not allowed")
		case c == '@':
			p.fail(line, "@-rules (such as @import, @font-face, and @media) are not allowed")
		case c == '<':
			p.fail(line, "\"<\" is not allowed")
		case c == '!':
			p.fail(line, "!important is not allowed")
		case c < 0x20 && c != '\t' && c != '\r':
			p.fail(line, "control characters are not allowed")
		case (c == 'u' || c == 'U') && i+4 <= len(p.src) && strings.EqualFold(string(p.src[i:i+4]), "url("):
			p.fail(line, "url() is not allowed; a theme cannot load anything")
		}
	}
}

// stripComments blanks every comment but keeps its newlines, so each line
// keeps its number. It returns the line of an unterminated comment, or 0.
func stripComments(data []byte) ([]byte, int) {
	out := append([]byte{}, data...)
	line := 1
	var quote byte
	for i := 0; i < len(out); i++ {
		c := out[i]
		if c == '\n' {
			line++
		}
		switch {
		case quote != 0:
			if c == quote || c == '\n' {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			start := line
			j := i + 2
			for ; j+1 < len(out) && !(out[j] == '*' && out[j+1] == '/'); j++ {
			}
			if j+1 >= len(out) {
				blank(out[i:], &line)
				return out, start
			}
			blank(out[i:j+2], &line)
			i = j + 1
		}
	}
	return out, 0
}

func blank(span []byte, line *int) {
	for i, c := range span {
		if c == '\n' {
			*line++
			continue
		}
		span[i] = ' '
	}
}

// selectorScheme names the scheme a block's selector sets, or "" when the
// selector is not one a theme may use.
func selectorScheme(selector string) string {
	switch strings.TrimSpace(selector) {
	case ":root":
		return "light"
	case darkSelector, `:root[data-theme='dark']`, `:root[data-theme=dark]`:
		return "dark"
	}
	return ""
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

func compactSpace(text string) string { return strings.Join(strings.Fields(text), " ") }
