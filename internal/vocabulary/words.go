package vocabulary

import (
	"strings"
	"unicode"
)

// Words splits an identifier into its lower-case words: camelCase,
// PascalCase, snake_case, kebab-case, and UPPER_SNAKE alike, keeping an
// acronym such as HTTP together.
func Words(identifier string) []string {
	var words []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			words = append(words, strings.ToLower(string(current)))
			current = nil
		}
	}
	runes := []rune(identifier)
	for index, r := range runes {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
			continue
		case unicode.IsUpper(r) && index > 0:
			previous := runes[index-1]
			nextLower := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || unicode.IsUpper(previous) && nextLower {
				flush()
			}
		}
		current = append(current, r)
	}
	flush()
	return words
}

// Humanize suggests the domain word an identifier spells: its words in lower
// case, without the leading words it shares with its container, since Go and
// many enum styles repeat the type in each member. ResolutionExcluded in
// Resolution is "excluded"; StateCoveredDirect in AxisState is "covered
// direct". At least one word always remains.
func Humanize(name, container string) string {
	words := Words(name)
	shared := map[string]bool{}
	for _, word := range Words(container) {
		shared[word] = true
	}
	for len(words) > 1 && shared[words[0]] {
		words = words[1:]
	}
	return strings.Join(words, " ")
}
