package grammar

// themeCommands are the shapes that write, check, and preview a Saga's
// theme file.
var themeCommands = []Command{
	{
		Name: "theme check", Status: StatusImplemented,
		Usage:       "change-saga theme check [--json] <saga>",
		Summary:     "validate theme.css strictly, naming each problem's line, then measure WCAG AA contrast of the key text and background pairs in light and dark mode; exits 1 when invalid and 3 naming each failing pair",
		Flags:       []Flag{jsonFlag},
		Positionals: sagaOnly,
	},
	{
		Name: "theme init", Status: StatusImplemented, Mutates: true, Writes: []string{"theme"},
		Usage:       "change-saga theme init [--force] <saga>",
		Summary:     "write a starter theme.css listing every design token with its light and dark defaults, grouped and commented out",
		Flags:       []Flag{{Name: "force", Description: "replace an existing theme.css"}},
		Positionals: sagaOnly,
	},
	{
		Name: "theme preview", Status: StatusImplemented,
		Usage:   "change-saga theme preview [--repo PATH] [--no-open] <saga>",
		Summary: "open the reviewer's /theme page: every token, the reviewer's chrome, and a diagram in every style and palette colour, light and dark side by side",
		Flags: []Flag{
			optional("repo", "PATH", "source repository checkout; required when separate"),
			{Name: "no-open", Description: "print the preview URL without opening a browser"},
		},
		Positionals: sagaOnly,
	},
}

func init() { commands = append(commands, themeCommands...) }
