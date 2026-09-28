package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAcceptsEveryKindAndReserializes(t *testing.T) {
	file, problems := Parse([]byte(`/* A theme: comments are dropped. */
:root {
  --bg: WHITE;
  --ink: #123;
  --muted: rgb(10, 20, 30);
  --accent: rgba(0 128 255 / 50%);
  --green: hsl(120deg 100% 25%);
  --red: hsla(0, 100%, 50%, 0.5);
  --ui: "Inter Display", -apple-system, Segoe UI, sans-serif;
  --radius: 0.5rem;
  --top: 0;
  --shadow: 0 6px 24px #1f232814, inset 0 -1px 3px 1px rgba(0,0,0,.2)
}
:root[data-theme="dark"] { --bg: #0b0b0b; --diagram-yellow-sticky: transparent; }
`))
	if len(problems) > 0 {
		t.Fatalf("a valid theme was refused: %v", problems)
	}
	light := map[string]string{}
	for _, override := range file.Light {
		light[override.Name] = override.Value
	}
	for name, want := range map[string]string{
		"bg": "#ffffff", "ink": "#112233", "muted": "#0a141e", "accent": "#0080ff80", "green": "#008000", "red": "#ff000080",
		"ui": `"Inter Display",-apple-system,Segoe UI,sans-serif`, "radius": "0.5rem", "top": "0",
		"shadow": "0 6px 24px #1f232814,inset 0 -1px 3px 1px #00000033",
	} {
		if light[name] != want {
			t.Errorf("--%s = %q, want %q", name, light[name], want)
		}
	}
	if len(file.Dark) != 2 || file.Dark[0].Value != "#0b0b0b" || file.Dark[1].Value != "#00000000" || file.Dark[0].Line != 14 {
		t.Errorf("dark overrides = %+v", file.Dark)
	}
	css := file.OverrideCSS()
	for _, want := range []string{":root{--bg:#ffffff;", "@media (prefers-color-scheme:dark){:root:not([data-theme=light]){--bg:#0b0b0b;", ":root[data-theme=dark]{--bg:#0b0b0b;"} {
		if !strings.Contains(css, want) {
			t.Errorf("override CSS lacks %q:\n%s", want, css)
		}
	}
}

func TestParseRefusesWithTheLine(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		line            int
	}{
		{"url", ":root {\n  --bg: url(https://example.com/x.png);\n}", "url() is not allowed", 2},
		{"import", "@import 'x.css';\n:root { --bg: #fff; }", "@-rules", 1},
		{"font-face", ":root { --bg: #fff; }\n\n@font-face { font-family: x; }", "@-rules", 3},
		{"unknown token", ":root {\n  --bg: #fff;\n  --nope: #fff;\n}", "unknown token --nope", 3},
		{"plain property", ":root {\n  color: red;\n}", `property "color" is not allowed`, 2},
		{"selector in a value", ":root {\n  --bg: #fff } body { display: none;\n}", `selector "body" is not allowed`, 2},
		{"brace in a value", ":root {\n  --bg: #fff { display: none };\n}", "unexpected {", 2},
		{"other selector", ":root { --bg: #fff; }\n.topbar { --bg: #000; }", `selector ".topbar" is not allowed`, 2},
		{"descendant selector", ":root [data-theme=dark] { --bg: #000; }", "is not allowed", 1},
		{"backslash", ":root {\n  --ui: \"\\41 rial\";\n}", "backslash escapes", 2},
		{"important", ":root { --bg: #fff !important; }", "!important", 1},
		{"angle bracket", ":root { --ui: </style><script>; }", `"<" is not allowed`, 1},
		{"bad colour", ":root {\n\n  --bg: var(--ink);\n}", "not a colour", 3},
		{"bad hex", ":root { --bg: #12345; }", "hex needs", 1},
		{"negative radius", ":root { --radius: -2px; }", "not a length", 1},
		{"percent length", ":root { --radius: 5%; }", "not a length", 1},
		{"font keyword", ":root { --ui: inherit; }", "CSS-wide", 1},
		{"font function", ":root { --ui: attr(x); }", "not a font family", 1},
		{"shadow without colour", ":root { --shadow: 0 1px 2px; }", "must be a colour", 1},
		{"twice", ":root { --bg: #fff; --bg: #000; }", "set twice", 1},
		{"second block", ":root { --bg: #fff; }\n:root { --ink: #000; }", "a second :root block", 2},
		{"unclosed block", ":root {\n  --bg: #fff;", "not closed", 1},
		{"unclosed comment", ":root { --bg: #fff; }\n/* trailing", "unterminated comment", 2},
		{"stray text", "--bg: #fff;", "outside a block", 1},
		{"empty value", ":root { --bg: ; }", "empty", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, problems := Parse([]byte(tc.src))
			for _, problem := range problems {
				if strings.Contains(problem.Message, tc.want) && problem.Line == tc.line {
					return
				}
			}
			t.Fatalf("problems %v lack %q at line %d", problems, tc.want, tc.line)
		})
	}
}

func TestLoadReadsTheSagaTheme(t *testing.T) {
	root := t.TempDir()
	if file, err := Load(root); file != nil || err != nil {
		t.Fatalf("a Saga without a theme loaded %v, %v", file, err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), []byte(":root { --bg: url(x) }"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "theme.css:line 1: url() is not allowed") {
		t.Fatalf("an invalid theme loaded: %v", err)
	}
}

func TestResolvedFollowsTheCascade(t *testing.T) {
	file, problems := Parse([]byte(":root { --bg: #111111; --radius: 2px; } :root[data-theme=dark] { --ink: #eeeeee; }"))
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	light, dark := file.Resolved("light"), file.Resolved("dark")
	if light["bg"] != "#111111" || light["ink"] != "#1f2328" {
		t.Errorf("light resolved bg %s ink %s", light["bg"], light["ink"])
	}
	// A light override of a token with a default dark value does not reach
	// dark mode; one of a token that never changes does.
	if dark["bg"] != "#0d1117" || dark["radius"] != "2px" || dark["ink"] != "#eeeeee" {
		t.Errorf("dark resolved bg %s radius %s ink %s", dark["bg"], dark["radius"], dark["ink"])
	}
}

func TestContrastNamesFailingPairs(t *testing.T) {
	var none *File
	for _, result := range none.Contrast() {
		if !result.Passes {
			t.Errorf("the default tokens fail: %s", result)
		}
	}
	file, problems := Parse([]byte(":root { --muted: #bbbbbb; } :root[data-theme=dark] { --diagram-pink-ink: #5c1a37; }"))
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	var failing []string
	for _, result := range file.Contrast() {
		if !result.Passes {
			failing = append(failing, result.String())
		}
	}
	if len(failing) != 2 || !strings.HasPrefix(failing[0], "light: --muted #bbbbbb on --bg #ffffff is 1.") || !strings.HasPrefix(failing[1], "dark: --diagram-pink-ink #5c1a37 on --diagram-pink-sticky #5c1a37 is 1.00:1") {
		t.Fatalf("failing pairs = %q", failing)
	}
}
