package vocabulary

import "strings"

// codeOnly blanks comments and the contents of string and character literals
// in a C-family source file, keeping every newline and every other byte in
// place, so a detector can scan code without being misled by an "enum" in a
// comment or a comma in a string. singleQuoteStrings treats '...' as a string
// (TypeScript); otherwise a single quote opens a character literal only when
// it closes within a few bytes, which keeps Rust lifetimes ('a) as code.
func codeOnly(content []byte, singleQuoteStrings bool) []byte {
	out := append([]byte(nil), content...)
	blank := func(from, to int) {
		for index := from; index < to && index < len(out); index++ {
			if out[index] != '\n' {
				out[index] = ' '
			}
		}
	}
	for index := 0; index < len(out); index++ {
		switch c := out[index]; {
		case c == '/' && index+1 < len(out) && out[index+1] == '/':
			end := index
			for end < len(out) && out[end] != '\n' {
				end++
			}
			blank(index, end)
			index = end
		case c == '/' && index+1 < len(out) && out[index+1] == '*':
			end := strings.Index(string(out[index+2:]), "*/")
			if end < 0 {
				blank(index, len(out))
				return out
			}
			blank(index, index+2+end+2)
			index += 2 + end + 1
		case c == '"' || c == '`' || c == '\'' && singleQuoteStrings:
			end := closing(out, index, c, c != '`')
			blank(index+1, end)
			index = end
		case c == '\'':
			// A character literal is short; anything else is code.
			for end := index + 1; end < len(out) && end <= index+12 && out[end] != '\n'; end++ {
				if out[end] == '\\' {
					end++
					continue
				}
				if out[end] == '\'' {
					blank(index+1, end)
					index = end
					break
				}
			}
		}
	}
	return out
}

// closing returns the index of the quote closing the literal opened at open,
// or the end of the line (or file, for a raw literal) when it is unclosed.
func closing(content []byte, open int, quote byte, singleLine bool) int {
	for index := open + 1; index < len(content); index++ {
		switch content[index] {
		case '\\':
			if quote != '`' {
				index++
			}
		case '\n':
			if singleLine {
				return index
			}
		case quote:
			return index
		}
	}
	return len(content)
}

// lineOf returns the 1-based line of offset.
func lineOf(content []byte, offset int) int {
	return strings.Count(string(content[:offset]), "\n") + 1
}

// braceDetector recognizes enum members in languages that declare an enum as
// "enum Name ... { members }": TypeScript, Java, Kotlin, C#, Rust, and Swift.
// Members are the leading identifiers of the body's top-level comma-separated
// entries, after any attribute or annotation. Java and Kotlin enums end their
// constants at the first top-level semicolon; Swift members are the names
// after each top-level "case".
type braceDetector struct {
	language        string
	stopAtSemicolon bool
	swiftCases      bool
}

func (detector braceDetector) Language() string { return detector.language }

func (detector braceDetector) Declarations(content []byte) []Declaration {
	code := codeOnly(content, detector.language == "typescript")
	var result []Declaration
	for index := 0; index < len(code); index++ {
		if !keywordAt(code, index, "enum") {
			continue
		}
		cursor := skipSpace(code, index+len("enum"))
		name := leadingIdentifier(string(code[cursor:min(len(code), cursor+256)]))
		if name == "class" { // Kotlin: enum class Name
			cursor = skipSpace(code, cursor+len(name))
			name = leadingIdentifier(string(code[cursor:min(len(code), cursor+256)]))
		}
		if !isIdentifier(name) {
			continue
		}
		open := cursor + len(name)
		for open < len(code) && code[open] != '{' && code[open] != ';' && code[open] != '=' {
			open++
		}
		if open >= len(code) || code[open] != '{' {
			continue
		}
		body := detector.members(code, open+1)
		for _, member := range body {
			result = append(result, Declaration{Name: member.name, Line: lineOf(code, member.offset), Kind: EnumMember, Container: name, Language: detector.language})
		}
		index = open
	}
	return result
}

type member struct {
	name   string
	offset int
}

// members parses an enum body starting just after its opening brace.
func (detector braceDetector) members(code []byte, start int) []member {
	var result []member
	depth := 0
	segment := start
	done := false
	take := func(from, to int) {
		if done {
			return
		}
		if detector.swiftCases {
			result = append(result, swiftCases(code, from, to)...)
			return
		}
		if value, ok := leadingMember(code, from, to); ok {
			result = append(result, value)
		}
	}
	for index := start; index < len(code); index++ {
		switch code[index] {
		case '(', '[', '{':
			depth++
		case ')', ']':
			depth--
		case '}':
			if depth == 0 {
				take(segment, index)
				return result
			}
			depth--
		case ',':
			if depth == 0 && !detector.swiftCases {
				take(segment, index)
				segment = index + 1
			}
		case ';':
			if depth == 0 {
				if detector.stopAtSemicolon {
					take(segment, index)
					done = true
				} else if detector.swiftCases {
					take(segment, index)
					segment = index + 1
				}
			}
		case '\n':
			if depth == 0 && detector.swiftCases {
				take(segment, index)
				segment = index + 1
			}
		}
	}
	return result
}

// leadingMember returns the member an enum entry declares: its first
// identifier after any annotation (@Name(...)), Rust attribute (#[...]), or
// C# attribute ([...]).
func leadingMember(code []byte, from, to int) (member, bool) {
	cursor := skipSpace(code, from)
	for cursor < to {
		switch {
		case code[cursor] == '@':
			cursor += 1 + len(leadingIdentifier(string(code[cursor+1:to])))
			cursor = skipBalanced(code, skipSpace(code, cursor), to)
		case code[cursor] == '#' && cursor+1 < to && code[cursor+1] == '[':
			cursor = skipBalanced(code, cursor+1, to)
		case code[cursor] == '[':
			cursor = skipBalanced(code, cursor, to)
		default:
			name := leadingIdentifier(string(code[cursor:to]))
			if !isIdentifier(name) || reserved[name] {
				return member{}, false
			}
			return member{name: name, offset: cursor}, true
		}
		cursor = skipSpace(code, cursor)
	}
	return member{}, false
}

// swiftCases returns the names a Swift "case a, b(Int), c = 3" line declares.
func swiftCases(code []byte, from, to int) []member {
	cursor := skipSpace(code, from)
	for _, modifier := range []string{"indirect"} {
		if keywordAt(code, cursor, modifier) {
			cursor = skipSpace(code, cursor+len(modifier))
		}
	}
	if !keywordAt(code, cursor, "case") {
		return nil
	}
	cursor += len("case")
	var result []member
	depth := 0
	entry := cursor
	for index := cursor; index <= to; index++ {
		if index == to || code[index] == ',' && depth == 0 {
			start := skipSpace(code, entry)
			if name := leadingIdentifier(string(code[start:max(start, index)])); isIdentifier(name) && !reserved[name] {
				result = append(result, member{name: name, offset: start})
			}
			entry = index + 1
			continue
		}
		switch code[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		}
	}
	return result
}

// reserved are words that can begin an enum entry without naming a member.
var reserved = map[string]bool{"case": true, "default": true, "func": true, "fun": true, "var": true, "val": true, "let": true, "static": true,
	"public": true, "private": true, "protected": true, "internal": true, "override": true, "abstract": true, "companion": true, "init": true}

// skipBalanced returns the index just past the bracketed group starting at
// open, or open when no group starts there.
func skipBalanced(code []byte, open, to int) int {
	if open >= to || (code[open] != '(' && code[open] != '[') {
		return open
	}
	depth := 0
	for index := open; index < to; index++ {
		switch code[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return index + 1
			}
		}
	}
	return to
}

func skipSpace(code []byte, index int) int {
	for index < len(code) && (code[index] == ' ' || code[index] == '\t' || code[index] == '\n' || code[index] == '\r') {
		index++
	}
	return index
}

// keywordAt reports whether word appears at index as a whole word.
func keywordAt(code []byte, index int, word string) bool {
	if index < 0 || index+len(word) > len(code) || string(code[index:index+len(word)]) != word {
		return false
	}
	isPart := func(c byte) bool {
		return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '$'
	}
	if index > 0 && isPart(code[index-1]) {
		return false
	}
	return index+len(word) == len(code) || !isPart(code[index+len(word)])
}
