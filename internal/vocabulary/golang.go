package vocabulary

import "strings"

// goDetector recognizes Go's enums: constants declared with a named,
// package-local type, as in an iota block. An untyped constant, or one of a
// builtin or imported type (a size, a time.Duration), is configuration rather
// than vocabulary and is never reported.
type goDetector struct{}

func (goDetector) Language() string { return "go" }

// goBuiltinTypes are predeclared types; a constant of one is not an enum.
var goBuiltinTypes = map[string]bool{
	"bool": true, "string": true, "byte": true, "rune": true, "error": true, "any": true, "uintptr": true,
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "complex64": true, "complex128": true,
}

func (goDetector) Declarations(content []byte) []Declaration {
	var result []Declaration
	inBlock := false
	// lastType is the type the most recent spec with an expression declared;
	// a bare name in a block repeats it, as iota continuations do.
	lastType := ""
	for index, line := range lines(codeOnly(content, false)) {
		trimmed := strings.TrimSpace(line)
		switch {
		case !inBlock && strings.HasPrefix(trimmed, "const") && strings.TrimSpace(strings.TrimPrefix(trimmed, "const")) == "(":
			inBlock, lastType = true, ""
			continue
		case !inBlock && strings.HasPrefix(trimmed, "const "):
			names, typ, _ := goSpec(strings.TrimPrefix(trimmed, "const "))
			result = append(result, goDeclarations(names, typ, index+1)...)
			continue
		case !inBlock:
			continue
		case strings.HasPrefix(trimmed, ")"):
			inBlock = false
			continue
		case trimmed == "":
			continue
		}
		names, typ, hasValue := goSpec(trimmed)
		switch {
		case hasValue:
			lastType = typ
		case typ == "":
			typ = lastType
		default:
			continue
		}
		result = append(result, goDeclarations(names, typ, index+1)...)
	}
	return result
}

// goSpec splits one constant spec into its names, its declared type (or ""),
// and whether it has a value.
func goSpec(spec string) (names []string, typ string, hasValue bool) {
	left, _, hasValue := strings.Cut(spec, "=")
	fields := strings.Fields(strings.ReplaceAll(left, ",", " , "))
	expectName := true
	for index, field := range fields {
		switch {
		case field == ",":
			expectName = true
		case expectName && isIdentifier(field):
			names = append(names, field)
			expectName = false
		case !expectName && index == len(fields)-1:
			typ = field
		default:
			return nil, "", false
		}
	}
	return names, typ, hasValue
}

func goDeclarations(names []string, typ string, line int) []Declaration {
	if !isIdentifier(typ) || goBuiltinTypes[typ] {
		return nil
	}
	var result []Declaration
	for _, name := range names {
		if name != "_" {
			result = append(result, Declaration{Name: name, Line: line, Kind: TypedConstant, Container: typ, Language: "go"})
		}
	}
	return result
}
