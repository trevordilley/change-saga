package theme

import (
	"regexp"
	"strings"
	"testing"
)

var (
	tokenName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	hexColor  = regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{6}|[0-9a-f]{8})$`)
)

func TestTokensAreUniqueAndWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, token := range Tokens() {
		if !tokenName.MatchString(token.Name) || seen[token.Name] {
			t.Errorf("token %q is malformed or repeated", token.Name)
		}
		seen[token.Name] = true
		if token.Group == "" || token.Kind == "" || token.Light == "" {
			t.Errorf("token %s lacks a group, kind, or light value", token.Name)
		}
		if token.Kind == "color" {
			for _, value := range []string{token.Light, token.Dark} {
				if value != "" && !hexColor.MatchString(value) {
					t.Errorf("colour token %s has a non-hex value %q", token.Name, value)
				}
			}
			if token.Dark == "" {
				t.Errorf("colour token %s has no dark value", token.Name)
			}
		}
	}
}

func TestDeclarationsCoverEachScheme(t *testing.T) {
	light, dark := Declarations("light"), Declarations("dark")
	for _, want := range []string{"--bg:#ffffff;", "--ink:#1f2328;", "--ui:", "--diagram-canvas:#fafaf8;", "--diagram-yellow-sticky:#fff3a3;"} {
		if !strings.Contains(light, want) {
			t.Errorf("light declarations lack %s", want)
		}
	}
	for _, want := range []string{"--bg:#0d1117;", "--diagram-canvas:#0d1117;"} {
		if !strings.Contains(dark, want) {
			t.Errorf("dark declarations lack %s", want)
		}
	}
	if strings.Contains(dark, "--ui:") || strings.Contains(dark, "--radius:") {
		t.Error("dark mode redeclares tokens that do not change")
	}
	if _, ok := Lookup("diagram-primary-stroke"); !ok {
		t.Error("Lookup misses a diagram token")
	}
}
