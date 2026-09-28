// Package theme is change-saga's design-token contract. Every colour, font,
// and size the app and its generated slides paint with is a named token with a
// light and a dark value, so a theme can restyle both by overriding tokens
// alone.
package theme

import "strings"

// Token is one named design value. Dark is empty when the token does not
// change between light and dark mode.
type Token struct {
	Name  string `json:"name"`
	Group string `json:"group"`
	Kind  string `json:"kind"`
	Light string `json:"light"`
	Dark  string `json:"dark,omitempty"`
	Doc   string `json:"doc,omitempty"`
}

// Tokens returns the contract in emission order.
func Tokens() []Token { return append([]Token{}, tokens...) }

// Lookup returns the token named name.
func Lookup(name string) (Token, bool) {
	for _, token := range tokens {
		if token.Name == name {
			return token, true
		}
	}
	return Token{}, false
}

// Declarations writes the custom-property declarations of one scheme, light
// or dark. Dark declares only the tokens that change.
func Declarations(scheme string) string {
	var b strings.Builder
	for _, token := range tokens {
		value := token.Light
		if scheme == "dark" {
			if token.Dark == "" {
				continue
			}
			value = token.Dark
		}
		b.WriteString("--" + token.Name + ":" + value + ";")
	}
	return b.String()
}

// Contract is the token vocabulary change-saga spec publishes, so a theme
// author can discover every overridable name from the installed CLI.
func Contract() map[string]any {
	return map[string]any{
		"file":   FileName + " at the Saga root; change-saga theme init writes a starter and theme check validates it",
		"format": "a CSS file with :root { --token: value; } for light and :root[data-theme=\"dark\"] { ... } for dark, setting only these tokens",
		"tokens": Tokens(),
	}
}
