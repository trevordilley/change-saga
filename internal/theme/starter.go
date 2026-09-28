package theme

import "strings"

// Starter is the file `change-saga theme init` writes: every token with its
// light and dark defaults, grouped and commented out, so the file changes
// nothing until an author uncomments a line.
func Starter() string {
	var b strings.Builder
	b.WriteString(`/* change-saga theme: design-token overrides for the reviewer and its slides.

   Only two blocks are allowed: :root { } for light mode and
   :root[data-theme="dark"] { } for dark mode. Each holds only the tokens
   below, as --token: value; with a value of the token's kind: colours as hex,
   rgb(), rgba(), hsl(), hsla(), or a named colour; lengths as px, rem, or
   em; fonts as a family list; shadows as [inset] x y [blur [spread]] colour.
   No other selectors, url(), @-rules, escapes, or !important.

   Every token is listed with its default, commented out. Uncomment and
   change the ones to override, then run change-saga theme check to validate
   the file and measure contrast, and change-saga theme preview to see it. */
`)
	var groups []string
	byGroup := map[string][]Token{}
	for _, token := range tokens {
		if byGroup[token.Group] == nil {
			groups = append(groups, token.Group)
		}
		byGroup[token.Group] = append(byGroup[token.Group], token)
	}
	block := func(selector, scheme string) {
		b.WriteString("\n" + selector + " {\n")
		for _, group := range groups {
			header := false
			for _, token := range byGroup[group] {
				value := token.Light
				if scheme == "dark" {
					if token.Dark == "" {
						continue
					}
					value = token.Dark
				}
				if !header {
					header = true
					b.WriteString("\n  /* " + group + " */\n")
				}
				line := "  /* --" + token.Name + ": " + value + ";"
				if token.Doc != "" {
					line += "  " + token.Doc
				}
				b.WriteString(line + " */\n")
			}
		}
		b.WriteString("}\n")
	}
	block(":root", "light")
	b.WriteString("\n/* Tokens missing below (fonts, sizes, and radius) are the same in both modes:\n   a :root override applies to dark mode too. */")
	block(darkSelector, "dark")
	return b.String()
}
