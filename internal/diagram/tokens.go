package diagram

import (
	"regexp"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/theme"
)

// Generated slides paint with the design-token contract, so they follow the
// app's light and dark mode and a theme's overrides. The renderer, and an
// author's style, name a colour token as "@name"; resolveTokens writes each
// one twice: the token's light value as the presentation attribute, and an
// inline style that reads the token with that light value as its fallback,
//
//	fill="#ffffff" style="fill:var(--diagram-normal-fill,#ffffff)"
//
// An inline style is used rather than var() in the presentation attribute,
// which SVG does not reliably accept, or classes, which would need a class
// vocabulary per token and property. Inline CSS outranks the presentation
// attribute wherever CSS custom properties work, and a consumer without them
// (a rasterizer, a code reader) drops the declaration and still sees the
// light drawing. The SVG's own style declares every token it uses, light by
// default and dark under prefers-color-scheme, so a standalone SVG themes
// itself and a slide frame follows its color-scheme.

// tokenReference is an author's or the renderer's reference to a colour token.
var tokenReference = regexp.MustCompile(`^@[a-z][a-z0-9-]{0,63}$`)

// canvasBackground is the token a document's default background paints with.
// New documents store the default's light value, so every stored diagram
// drawn on the default canvas themes without editing its source.
const canvasBackground = "@diagram-canvas"

// colorToken returns the colour token value names, if value is a reference.
func colorToken(value string) (theme.Token, bool) {
	if !tokenReference.MatchString(value) {
		return theme.Token{}, false
	}
	token, ok := theme.Lookup(value[1:])
	return token, ok && token.Kind == "color"
}

// validColor accepts #rrggbb, none, or a reference to a colour token.
func validColor(value string) bool {
	if colorPattern.MatchString(value) {
		return true
	}
	_, ok := colorToken(value)
	return ok
}

// colorTokenNames lists the tokens a style colour may reference.
func colorTokenNames() []string {
	names := []string{}
	for _, token := range theme.Tokens() {
		if token.Kind == "color" {
			names = append(names, "@"+token.Name)
		}
	}
	return names
}

// colorProperties are the presentation attributes a token may paint.
var colorProperties = map[string]bool{"fill": true, "stroke": true, "color": true, "flood-color": true}

// resolveTokens rewrites every token reference under n into its light value
// plus an inline style that reads the token, and returns the tokens used.
// Author fragments and icons cannot carry references: their colour grammar
// has no "@".
func resolveTokens(n *node, used map[string]theme.Token) {
	var declarations []string
	for index, attr := range n.attrs {
		if !colorProperties[attr[0]] {
			continue
		}
		token, ok := colorToken(attr[1])
		if !ok {
			continue
		}
		used[token.Name] = token
		n.attrs[index][1] = token.Light
		declarations = append(declarations, attr[0]+":var(--"+token.Name+","+token.Light+")")
	}
	if len(declarations) > 0 {
		n.set("style", strings.Join(declarations, ";"))
	}
	for _, child := range n.children {
		resolveTokens(child, used)
	}
}

// tokenStyle declares the used tokens' light values, and their dark values
// under a dark color scheme. Declaring color-scheme lets a slide frame in a
// dark page draw the dark canvas instead of an opaque light backdrop.
func tokenStyle(used map[string]theme.Token) string {
	if len(used) == 0 {
		return ""
	}
	names := make([]string, 0, len(used))
	for name := range used {
		names = append(names, name)
	}
	sort.Strings(names)
	var light, dark strings.Builder
	for _, name := range names {
		token := used[name]
		light.WriteString("--" + name + ":" + token.Light + ";")
		if token.Dark != "" {
			dark.WriteString("--" + name + ":" + token.Dark + ";")
		}
	}
	return ":root{color-scheme:light dark;" + light.String() + "}@media (prefers-color-scheme:dark){:root{" + dark.String() + "}}"
}
