package theme

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// ParseValue checks value against the grammar of a token kind and returns
// its canonical re-serialization: colours as hex, lengths as a number and a
// unit, fonts as a family list, shadows rebuilt from their parts.
func ParseValue(kind, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("the value is empty")
	}
	switch kind {
	case "color":
		color, err := ParseColor(value)
		if err != nil {
			return "", err
		}
		return color.Hex(), nil
	case "length":
		return parseLength(value, false)
	case "font":
		return parseFont(value)
	case "shadow":
		return parseShadow(value)
	}
	return "", fmt.Errorf("unknown token kind %q", kind)
}

// Color is a parsed sRGB colour with alpha, each channel in [0, 1].
type Color struct{ R, G, B, A float64 }

// Hex is the colour as #rrggbb, or #rrggbbaa when it is translucent.
func (color Color) Hex() string {
	channel := func(v float64) int { return int(math.Round(clamp01(v) * 255)) }
	hex := fmt.Sprintf("#%02x%02x%02x", channel(color.R), channel(color.G), channel(color.B))
	if alpha := channel(color.A); alpha != 255 {
		hex += fmt.Sprintf("%02x", alpha)
	}
	return hex
}

// ParseColor reads a hex colour, rgb()/rgba(), hsl()/hsla(), or a CSS named
// colour. Nothing else, not even currentcolor or a var(), is a colour here.
func ParseColor(value string) (Color, error) {
	lower := strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(lower, "#") {
		return parseHex(lower)
	}
	if open := strings.IndexByte(lower, '('); open > 0 {
		if !strings.HasSuffix(lower, ")") {
			return Color{}, fmt.Errorf("%q is not a colour: an unclosed function", value)
		}
		name, args := lower[:open], lower[open+1:len(lower)-1]
		switch name {
		case "rgb", "rgba":
			return parseRGB(value, args)
		case "hsl", "hsla":
			return parseHSL(value, args)
		}
		return Color{}, fmt.Errorf("%q is not a colour: only rgb(), rgba(), hsl(), and hsla() are allowed", value)
	}
	if lower == "transparent" {
		return Color{}, nil
	}
	if rgb, ok := namedColors[lower]; ok {
		return Color{R: float64(rgb>>16&0xff) / 255, G: float64(rgb>>8&0xff) / 255, B: float64(rgb&0xff) / 255, A: 1}, nil
	}
	return Color{}, fmt.Errorf("%q is not a colour: use hex, rgb(), rgba(), hsl(), hsla(), or a named colour", value)
}

func parseHex(value string) (Color, error) {
	digits := value[1:]
	switch len(digits) {
	case 3, 4:
		expanded := make([]byte, 0, 8)
		for i := 0; i < len(digits); i++ {
			expanded = append(expanded, digits[i], digits[i])
		}
		digits = string(expanded)
	case 6, 8:
	default:
		return Color{}, fmt.Errorf("%q is not a colour: hex needs 3, 4, 6, or 8 digits", value)
	}
	parsed, err := strconv.ParseUint(digits, 16, 64)
	if err != nil {
		return Color{}, fmt.Errorf("%q is not a colour: bad hex digits", value)
	}
	if len(digits) == 6 {
		parsed = parsed<<8 | 0xff
	}
	return Color{R: float64(parsed>>24&0xff) / 255, G: float64(parsed>>16&0xff) / 255, B: float64(parsed>>8&0xff) / 255, A: float64(parsed&0xff) / 255}, nil
}

// colorArgs splits a colour function's arguments in either syntax, legacy
// commas or spaces with a slash before alpha, into three channels and an
// optional alpha.
func colorArgs(value, args string) ([]string, string, error) {
	var channels []string
	alpha := ""
	if strings.Contains(args, ",") {
		channels = strings.Split(args, ",")
		for i := range channels {
			channels[i] = strings.TrimSpace(channels[i])
		}
		if len(channels) == 4 {
			alpha, channels = channels[3], channels[:3]
		}
	} else {
		main, rest, slash := strings.Cut(args, "/")
		channels = strings.Fields(main)
		if slash {
			alpha = strings.TrimSpace(rest)
			if alpha == "" {
				return nil, "", fmt.Errorf("%q is not a colour: nothing after /", value)
			}
		}
	}
	if len(channels) != 3 {
		return nil, "", fmt.Errorf("%q is not a colour: it needs three channels and an optional alpha", value)
	}
	return channels, alpha, nil
}

func parseRGB(value, args string) (Color, error) {
	channels, alpha, err := colorArgs(value, args)
	if err != nil {
		return Color{}, err
	}
	var rgb [3]float64
	for i, channel := range channels {
		number, percent, err := number(channel, true)
		if err != nil {
			return Color{}, fmt.Errorf("%q is not a colour: %v", value, err)
		}
		if percent {
			rgb[i] = number / 100
		} else {
			rgb[i] = number / 255
		}
	}
	a, err := parseAlpha(value, alpha)
	return Color{R: clamp01(rgb[0]), G: clamp01(rgb[1]), B: clamp01(rgb[2]), A: a}, err
}

func parseHSL(value, args string) (Color, error) {
	channels, alpha, err := colorArgs(value, args)
	if err != nil {
		return Color{}, err
	}
	hue := strings.TrimSuffix(channels[0], "deg")
	h, percent, err := number(hue, false)
	if err != nil || percent {
		return Color{}, fmt.Errorf("%q is not a colour: the hue must be a number or degrees", value)
	}
	var sl [2]float64
	for i, channel := range channels[1:] {
		n, percent, err := number(channel, true)
		if err != nil || !percent {
			return Color{}, fmt.Errorf("%q is not a colour: saturation and lightness must be percentages", value)
		}
		sl[i] = clamp01(n / 100)
	}
	a, err := parseAlpha(value, alpha)
	r, g, b := hslToRGB(math.Mod(math.Mod(h, 360)+360, 360)/360, sl[0], sl[1])
	return Color{R: r, G: g, B: b, A: a}, err
}

func parseAlpha(value, alpha string) (float64, error) {
	if alpha == "" {
		return 1, nil
	}
	n, percent, err := number(alpha, true)
	if err != nil {
		return 0, fmt.Errorf("%q is not a colour: alpha %v", value, err)
	}
	if percent {
		n /= 100
	}
	return clamp01(n), nil
}

// number reads a plain decimal, optionally a percentage. It refuses
// exponents, signs other than a leading minus, and anything else CSS allows.
func number(text string, allowPercent bool) (float64, bool, error) {
	percent := strings.HasSuffix(text, "%")
	if percent {
		if !allowPercent {
			return 0, false, fmt.Errorf("%q may not be a percentage", text)
		}
		text = strings.TrimSuffix(text, "%")
	}
	digits := strings.TrimPrefix(text, "-")
	if digits == "" || strings.Trim(digits, "0123456789.") != "" || strings.Count(digits, ".") > 1 || digits == "." {
		return 0, false, fmt.Errorf("%q is not a number", text)
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false, fmt.Errorf("%q is not a number", text)
	}
	return n, percent, nil
}

func hslToRGB(h, s, l float64) (float64, float64, float64) {
	if s == 0 {
		return l, l, l
	}
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	hue := func(t float64) float64 {
		t = math.Mod(t+1, 1)
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	return hue(h + 1.0/3), hue(h), hue(h - 1.0/3)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// parseLength reads 0 or a non-negative number of px, rem, or em; a shadow
// offset may be negative.
func parseLength(value string, signed bool) (string, error) {
	if value == "0" {
		return "0", nil
	}
	for _, unit := range []string{"rem", "px", "em"} {
		if !strings.HasSuffix(value, unit) {
			continue
		}
		n, percent, err := number(strings.TrimSuffix(value, unit), false)
		if err != nil || percent || (n < 0 && !signed) {
			break
		}
		return strconv.FormatFloat(n, 'f', -1, 64) + unit, nil
	}
	if signed {
		return "", fmt.Errorf("%q is not a length: use 0 or a number of px, rem, or em", value)
	}
	return "", fmt.Errorf("%q is not a length: use 0 or a non-negative number of px, rem, or em", value)
}

// parseFont reads a comma-separated list of font families: each a quoted
// name or a run of plain words, such as system-ui or Segoe UI. It names
// installed families only; a theme cannot load a font.
func parseFont(value string) (string, error) {
	families := strings.Split(value, ",")
	out := make([]string, 0, len(families))
	for _, family := range families {
		family = strings.TrimSpace(family)
		if family == "" {
			return "", fmt.Errorf("%q is not a font list: an empty family", value)
		}
		if quote := family[0]; quote == '"' || quote == '\'' {
			if len(family) < 2 || family[len(family)-1] != quote {
				return "", fmt.Errorf("%q is not a font list: an unclosed quote", value)
			}
			name := family[1 : len(family)-1]
			if strings.TrimSpace(name) == "" || !fontNameOK(name, true) {
				return "", fmt.Errorf("%q is not a font family: quoted names hold letters, digits, spaces, dots, hyphens, and underscores", family)
			}
			out = append(out, `"`+compactSpace(name)+`"`)
			continue
		}
		words := strings.Fields(family)
		if len(words) == 1 && cssWideKeywords[strings.ToLower(words[0])] {
			return "", fmt.Errorf("%q is not a font family: CSS-wide keywords are not allowed", family)
		}
		for _, word := range words {
			if !fontNameOK(word, false) || !(unicode.IsLetter(rune(word[0])) || word[0] == '-' && len(word) > 1) {
				return "", fmt.Errorf("%q is not a font family: quote a name that is not plain words", family)
			}
		}
		out = append(out, strings.Join(words, " "))
	}
	return strings.Join(out, ","), nil
}

// cssWideKeywords would make a token inherit or reset instead of naming a
// family.
var cssWideKeywords = map[string]bool{"inherit": true, "initial": true, "unset": true, "revert": true, "revert-layer": true, "default": true}

func fontNameOK(name string, quoted bool) bool {
	for _, r := range name {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
		case quoted && (r == ' ' || r == '.'):
		default:
			return false
		}
	}
	return true
}

// parseShadow reads none or a comma-separated list of shadows, each
// [inset] <x> <y> [<blur> [<spread>]] <colour>.
func parseShadow(value string) (string, error) {
	if strings.EqualFold(value, "none") {
		return "none", nil
	}
	layers := splitTopLevel(value, ',')
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		parts := splitTopLevel(strings.TrimSpace(layer), ' ')
		var built []string
		if len(parts) > 0 && strings.EqualFold(parts[0], "inset") {
			built, parts = append(built, "inset"), parts[1:]
		}
		if len(parts) < 3 || len(parts) > 5 {
			return "", fmt.Errorf("%q is not a shadow: write [inset] x y [blur [spread]] colour", strings.TrimSpace(layer))
		}
		for i, part := range parts[:len(parts)-1] {
			length, err := parseLength(part, i < 2 || i == 3)
			if err != nil {
				return "", fmt.Errorf("%q is not a shadow: %v", strings.TrimSpace(layer), err)
			}
			built = append(built, length)
		}
		color, err := ParseColor(parts[len(parts)-1])
		if err != nil {
			return "", fmt.Errorf("%q is not a shadow: the last part must be a colour: %v", strings.TrimSpace(layer), err)
		}
		out = append(out, strings.Join(append(built, color.Hex()), " "))
	}
	return strings.Join(out, ","), nil
}

// splitTopLevel splits text at sep outside parentheses, dropping empty parts
// when sep is a space.
func splitTopLevel(text string, sep rune) []string {
	var parts []string
	depth, start := 0, 0
	for i, r := range text {
		switch {
		case r == '(':
			depth++
		case r == ')':
			depth--
		case depth == 0 && (r == sep || sep == ' ' && unicode.IsSpace(r)):
			parts = append(parts, text[start:i])
			start = i + 1
		}
	}
	parts = append(parts, text[start:])
	if sep != ' ' {
		return parts
	}
	kept := parts[:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return kept
}
