// Package vocabulary recognizes the declarations in source code that most
// often introduce a project's own words: enum members, and constants of a
// named domain type. A comparison uses it to suggest that an added
// declaration no term references may be new terminology worth defining.
//
// It is a heuristic and only ever suggests. It never parses a language
// fully, so it is deliberately conservative: a declaration it cannot
// recognize with confidence is skipped, because a missed suggestion costs
// nothing and a false one teaches authors to ignore the rest. In particular,
// plain constants (a timeout, a buffer size, a UPPER_SNAKE setting) are never
// reported; only Go constants declared with a named, package-local type are,
// since that is how Go spells an enum.
//
// Each language is one Detector registered by file extension. Adding a
// language is adding a Detector and its tests; nothing else changes.
package vocabulary

import (
	"path"
	"sort"
	"strings"
)

// Kind is what a declaration is.
type Kind string

const (
	// EnumMember is one member of an enum type.
	EnumMember Kind = "enum_member"
	// TypedConstant is a constant of a named domain type, such as a Go
	// const in an iota block.
	TypedConstant Kind = "typed_constant"
)

// Declaration is one recognized declaration. Line is 1-based. Container is
// the enum or type it belongs to, when known.
type Declaration struct {
	Name      string `json:"name"`
	Line      int    `json:"line"`
	Kind      Kind   `json:"kind"`
	Container string `json:"container,omitempty"`
	Language  string `json:"language"`
}

// Detector recognizes declarations in one language. It must be
// conservative: return only what it recognizes with confidence.
type Detector interface {
	Language() string
	Declarations(content []byte) []Declaration
}

var detectors = map[string]Detector{}

// Register makes a detector responsible for files with the given extensions,
// each spelled with its leading dot. A later registration replaces an
// earlier one.
func Register(detector Detector, extensions ...string) {
	for _, extension := range extensions {
		detectors[strings.ToLower(extension)] = detector
	}
}

func init() {
	Register(goDetector{}, ".go")
	Register(braceDetector{language: "typescript"}, ".ts", ".tsx", ".mts", ".cts")
	Register(braceDetector{language: "java", stopAtSemicolon: true}, ".java")
	Register(braceDetector{language: "kotlin", stopAtSemicolon: true}, ".kt", ".kts")
	Register(braceDetector{language: "csharp"}, ".cs")
	Register(braceDetector{language: "rust"}, ".rs")
	Register(braceDetector{language: "swift", swiftCases: true}, ".swift")
	Register(pythonDetector{}, ".py")
}

// maxBytes bounds the content a detector reads; larger files are skipped.
const maxBytes = 2 << 20

// Detect returns the declarations recognized in one file, in line order, or
// nothing when no detector handles its extension.
func Detect(filePath string, content []byte) []Declaration {
	detector, ok := detectors[strings.ToLower(path.Ext(filePath))]
	if !ok || len(content) > maxBytes || isGenerated(filePath, content) {
		return nil
	}
	values := detector.Declarations(content)
	sort.SliceStable(values, func(i, j int) bool { return values[i].Line < values[j].Line })
	return values
}

// Supported reports whether some detector handles the file's extension.
func Supported(filePath string) bool {
	_, ok := detectors[strings.ToLower(path.Ext(filePath))]
	return ok
}

// isGenerated skips generated code and tests: their declarations are not the
// team's vocabulary.
func isGenerated(filePath string, content []byte) bool {
	base := path.Base(filePath)
	for _, suffix := range []string{"_test.go", ".pb.go", "_generated.go", ".g.cs", ".d.ts", ".test.ts", ".spec.ts"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	head := content
	if len(head) > 1024 {
		head = head[:1024]
	}
	return strings.Contains(string(head), "Code generated") && strings.Contains(string(head), "DO NOT EDIT")
}

// isIdentifier reports whether value is an ASCII identifier.
func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case index > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// leadingIdentifier returns the identifier value starts with.
func leadingIdentifier(value string) string {
	end := 0
	for end < len(value) {
		c := value[end]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || end > 0 && c >= '0' && c <= '9' {
			end++
			continue
		}
		break
	}
	return value[:end]
}

// lines splits content into lines without their terminators.
func lines(content []byte) []string {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	return strings.Split(text, "\n")
}
