package vocabulary

import (
	"regexp"
	"strings"
)

// pythonDetector recognizes the members of classes derived from the enum
// module's Enum, IntEnum, StrEnum, Flag, or IntFlag: the NAME = value
// assignments directly in the class body. Private names are skipped.
type pythonDetector struct{}

func (pythonDetector) Language() string { return "python" }

var (
	pythonClass  = regexp.MustCompile(`^(\s*)class\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*:`)
	pythonEnum   = regexp.MustCompile(`(^|[\s,.])(Enum|IntEnum|StrEnum|Flag|IntFlag)\s*(,|$)`)
	pythonMember = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_]*)\s*(:[^=]*)?=[^=]`)
)

func (pythonDetector) Declarations(content []byte) []Declaration {
	var result []Declaration
	source := lines(content)
	for index := 0; index < len(source); index++ {
		match := pythonClass.FindStringSubmatch(source[index])
		if match == nil || !pythonEnum.MatchString(strings.TrimSpace(match[3])) {
			continue
		}
		classIndent, name := len(match[1]), match[2]
		bodyIndent := -1
		inDocstring := ""
		for body := index + 1; body < len(source); body++ {
			line := source[body]
			trimmed := strings.TrimSpace(line)
			if inDocstring != "" {
				if strings.Contains(trimmed, inDocstring) {
					inDocstring = ""
				}
				continue
			}
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			if indent <= classIndent {
				break
			}
			if bodyIndent < 0 {
				bodyIndent = indent
			}
			if indent != bodyIndent {
				continue
			}
			for _, quote := range []string{`"""`, `'''`} {
				if strings.HasPrefix(trimmed, quote) && !strings.Contains(trimmed[3:], quote) {
					inDocstring = quote
				}
			}
			if member := pythonMember.FindStringSubmatch(trimmed); member != nil && !strings.HasPrefix(member[1], "_") {
				result = append(result, Declaration{Name: member[1], Line: body + 1, Kind: EnumMember, Container: name, Language: "python"})
			}
		}
	}
	return result
}
