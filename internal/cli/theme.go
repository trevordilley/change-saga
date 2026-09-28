package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
	reviewserver "github.com/twentyideas/changesaga/internal/server"
	"github.com/twentyideas/changesaga/internal/theme"
)

func init() {
	for name, usage := range map[string]string{
		"theme":         "change-saga theme <init|check|preview> [flags] <saga>",
		"theme init":    "change-saga theme init [--force] <saga>",
		"theme check":   "change-saga theme check [--json] <saga>",
		"theme preview": "change-saga theme preview [--repo PATH] [--no-open] <saga>",
	} {
		commandUsage[name] = usage
	}
	commandDescription["theme"] = "Recolour the reviewer and its slides with a theme: theme.css at the Saga root, holding only\ndesign-token overrides, a :root { } block for light mode and a :root[data-theme=\"dark\"] { }\nblock for dark. No selectors, url(), @-rules, or font files, so a theme cannot restyle or hide\nthe review controls or load anything. change-saga spec lists every token under theme."
	commandDescription["theme init"] = "Write a starter theme.css listing every token with its light and dark defaults, grouped and\ncommented out; uncomment and change the ones to override. It refuses to replace an existing\ntheme unless --force."
	commandDescription["theme check"] = "Validate theme.css strictly, naming each problem with its line, then measure WCAG AA contrast\n(4.5:1) for the key text and background pairs in light and dark mode: ink and muted on the\nbackground, the primary button, each diagram style's text on its fill, and each palette ink on\nits sticky. It exits 0 when all pass, 3 naming each failing pair, and 1 when the theme is invalid.\nWithout a theme it checks the defaults."
	commandDescription["theme preview"] = "Open the reviewer's /theme page: every token as a swatch, the reviewer's chrome, and a diagram in\nevery style and palette colour, in light and dark side by side. It starts the Saga's background\nreviewer when none is running; --no-open prints the URL instead of opening a browser."
}

// Theme runs the theme family: init, check, and preview.
func Theme(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("theme", []string{"init", "check", "preview"}, out)
	}
	switch args[0] {
	case "init":
		return themeInit(args[1:], out)
	case "check":
		return themeCheck(args[1:], out)
	case "preview":
		return themePreview(ctx, args[1:], out)
	}
	return fmt.Errorf("usage: %s", commandUsage["theme"])
}

func themeInit(args []string, out io.Writer) error {
	flags := commandFlags("theme init", commandUsage["theme init"], out)
	force := flags.Bool("force", false, "replace an existing theme.css")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["theme init"])
	}
	root := flags.Arg(0)
	if _, err := os.Stat(filepath.Join(root, saga.ManifestName)); err != nil {
		return fmt.Errorf("%s is not a Saga: %w", root, err)
	}
	path := filepath.Join(root, theme.FileName)
	if _, err := os.Lstat(path); err == nil && !*force {
		return fmt.Errorf("%s already exists; pass --force to replace it with the starter", path)
	}
	if err := os.WriteFile(path, []byte(theme.Starter()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s\nEvery token is listed with its defaults, commented out: uncomment and change the ones to override,\nthen run change-saga theme check.\n", path)
	return nil
}

// themeCheckResult is theme check's machine-readable answer.
type themeCheckResult struct {
	Path     string                 `json:"path"`
	Present  bool                   `json:"present"`
	Valid    bool                   `json:"valid"`
	Problems []theme.Problem        `json:"problems"`
	Light    []theme.Override       `json:"light"`
	Dark     []theme.Override       `json:"dark"`
	Contrast []theme.ContrastResult `json:"contrast"`
	Failing  int                    `json:"failing"`
}

func themeCheck(args []string, out io.Writer) error {
	flags := commandFlags("theme check", commandUsage["theme check"], out)
	jsonOutput := flags.Bool("json", false, "emit machine-readable JSON")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["theme check"])
	}
	root := flags.Arg(0)
	result := themeCheckResult{Path: filepath.Join(root, theme.FileName), Problems: []theme.Problem{}, Light: []theme.Override{}, Dark: []theme.Override{}, Contrast: []theme.ContrastResult{}}
	_, statErr := os.Lstat(result.Path)
	result.Present = statErr == nil
	file, err := theme.Load(root)
	var invalid *theme.InvalidError
	if errors.As(err, &invalid) {
		result.Problems = invalid.Problems
	} else if err != nil {
		return err
	} else {
		result.Valid = true
		if file != nil {
			result.Light, result.Dark = file.Light, file.Dark
		}
		result.Contrast = file.Contrast()
		for _, pair := range result.Contrast {
			if !pair.Passes {
				result.Failing++
			}
		}
	}
	if *jsonOutput {
		if err := writeJSON(out, result); err != nil {
			return err
		}
	} else {
		writeThemeCheck(out, result)
	}
	switch {
	case !result.Valid:
		return &StatusError{Code: 1}
	case result.Failing > 0:
		return &StatusError{Code: 3}
	}
	return nil
}

func writeThemeCheck(out io.Writer, result themeCheckResult) {
	switch {
	case !result.Valid:
		fmt.Fprintf(out, "Invalid theme %s: it is not applied until fixed\n", result.Path)
		for _, problem := range result.Problems {
			fmt.Fprintf(out, "  %s:%d: %s\n", theme.FileName, problem.Line, problem.Message)
		}
		return
	case result.Present:
		fmt.Fprintf(out, "Valid theme %s: %d light and %d dark overrides\n", result.Path, len(result.Light), len(result.Dark))
	default:
		fmt.Fprintf(out, "No theme at %s; checking the default tokens (change-saga theme init writes a starter)\n", result.Path)
	}
	if result.Failing == 0 {
		fmt.Fprintf(out, "Contrast: all %d text and background pairs meet WCAG AA (%.1f:1) in light and dark mode\n", len(result.Contrast), theme.MinimumContrast)
		return
	}
	fmt.Fprintf(out, "Contrast: %d of %d pairs are below WCAG AA (%.1f:1):\n", result.Failing, len(result.Contrast), theme.MinimumContrast)
	for _, pair := range result.Contrast {
		if !pair.Passes {
			fmt.Fprintf(out, "  %s\n", pair)
		}
	}
}

func themePreview(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("theme preview", commandUsage["theme preview"], out)
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	noOpen := flags.Bool("no-open", false, "print the preview URL without opening a browser")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage["theme preview"])
	}
	absRoot, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	statePath, err := detachedStatePath(absRoot)
	if err != nil {
		return err
	}
	// The theme does not depend on how the reviewer was opened, so a
	// running reviewer of either opening serves the preview.
	state, err := readDetachedState(statePath)
	if err != nil || !detachedServerActive(ctx, state) {
		if err := startDetachedServer(ctx, absRoot, *repoDir, gitdiff.Range{}, "127.0.0.1:0", false, io.Discard); err != nil {
			return err
		}
		if state, err = readDetachedState(statePath); err != nil {
			return err
		}
	}
	url := strings.TrimSuffix(state.URL, "/") + "/theme"
	if !*noOpen {
		_ = reviewserver.OpenBrowser(url)
	}
	fmt.Fprintf(out, "Theme preview: %s\n", url)
	return nil
}

// appendThemeIssues reports an invalid theme through validate, one issue per
// problem, each naming its line.
func appendThemeIssues(root string, validation *saga.Validation) {
	_, err := theme.Load(root)
	if err == nil {
		return
	}
	var invalid *theme.InvalidError
	if !errors.As(err, &invalid) {
		validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: theme.FileName, Message: err.Error()})
		validation.Valid = false
		return
	}
	for _, problem := range invalid.Problems {
		validation.Issues = append(validation.Issues, saga.Issue{Severity: "error", Path: fmt.Sprintf("%s:%d", theme.FileName, problem.Line), Message: "theme: " + problem.Message})
	}
	validation.Valid = false
}
