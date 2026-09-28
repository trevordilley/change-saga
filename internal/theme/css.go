package theme

import "strings"

// declarationsOf writes one scheme's overrides as custom-property
// declarations. Every value is the validated re-serialization.
func declarationsOf(overrides []Override) string {
	var b strings.Builder
	for _, override := range overrides {
		b.WriteString("--" + override.Name + ":" + override.Value + ";")
	}
	return b.String()
}

// OverrideCSS is the theme's overrides as the reviewer emits them after its
// default tokens: light on :root, dark both when the OS prefers it (unless
// the reviewer pinned light) and when the reviewer pinned dark. It is empty
// for no theme.
func (file *File) OverrideCSS() string {
	if file == nil {
		return ""
	}
	var b strings.Builder
	if light := declarationsOf(file.Light); light != "" {
		b.WriteString(":root{" + light + "}\n")
	}
	if dark := declarationsOf(file.Dark); dark != "" {
		b.WriteString("@media (prefers-color-scheme:dark){:root:not([data-theme=light]){" + dark + "}}\n")
		b.WriteString(":root[data-theme=dark]{" + dark + "}\n")
	}
	return b.String()
}

// Resolved is every token's value in one scheme once the theme applies, as
// the cascade resolves it: a dark override, else the default dark value,
// else the light override, else the default light value.
func (file *File) Resolved(scheme string) map[string]string {
	values := map[string]string{}
	for _, token := range tokens {
		values[token.Name] = token.Light
		if scheme == "dark" && token.Dark != "" {
			values[token.Name] = token.Dark
		}
	}
	if file == nil {
		return values
	}
	for _, override := range file.Light {
		if token, _ := Lookup(override.Name); scheme != "dark" || token.Dark == "" {
			values[override.Name] = override.Value
		}
	}
	if scheme == "dark" {
		for _, override := range file.Dark {
			values[override.Name] = override.Value
		}
	}
	return values
}

// Declare declares every token with its value in scheme, the theme applied.
func (file *File) Declare(scheme string) string {
	values := file.Resolved(scheme)
	var b strings.Builder
	for _, token := range tokens {
		b.WriteString("--" + token.Name + ":" + values[token.Name] + ";")
	}
	return b.String()
}

// FrameCSS declares every token for a slide served into a frame that follows
// the OS: the light values with the theme applied, and the dark ones when
// the frame prefers dark. It sets no color-scheme: a slide that does not
// support dark mode keeps the light scheme the reviewer gives its frame.
func (file *File) FrameCSS() string {
	return ":root{" + file.Declare("light") + "}@media (prefers-color-scheme:dark){:root{" + file.Declare("dark") + "}}"
}
