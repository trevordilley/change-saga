package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/theme"
)

func themeSaga(t *testing.T) string {
	t.Helper()
	repo := initRepository(t, "themed", "https://example.test/acme/app.git")
	root := filepath.Join(repo, "change.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	return root
}

func statusCode(err error) int {
	var status *StatusError
	if errors.As(err, &status) {
		return status.Code
	}
	if err != nil {
		return -1
	}
	return 0
}

func TestThemeInitWritesTheStarterOnce(t *testing.T) {
	root := themeSaga(t)
	var output bytes.Buffer
	if err := Theme(context.Background(), []string{"init", root}, &output); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, theme.FileName)
	if !strings.Contains(output.String(), "Wrote "+path) {
		t.Errorf("init does not name the file it wrote: %s", output.String())
	}
	if data, _ := os.ReadFile(path); string(data) != theme.Starter() {
		t.Fatal("init did not write the starter")
	}
	if err := os.WriteFile(path, []byte(":root { --bg: #fafafa; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Theme(context.Background(), []string{"init", root}, &output); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("init replaced an existing theme: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != ":root { --bg: #fafafa; }\n" {
		t.Fatal("a refused init changed the theme")
	}
	if err := Theme(context.Background(), []string{"init", "--force", root}, &output); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != theme.Starter() {
		t.Fatal("init --force did not replace the theme")
	}
	// A theme alone is not living documentation: the Saga stays review first.
	if holdsLivingDocumentation(root) {
		t.Error("a theme file made a review-only Saga hold living documentation")
	}
}

func TestThemeCheckValidatesAndNamesContrastFailures(t *testing.T) {
	root := themeSaga(t)
	path := filepath.Join(root, theme.FileName)
	check := func(args ...string) (string, int) {
		var output bytes.Buffer
		err := Theme(context.Background(), append([]string{"check"}, append(args, root)...), &output)
		return output.String(), statusCode(err)
	}

	if output, code := check(); code != 0 || !strings.Contains(output, "No theme at") || !strings.Contains(output, "meet WCAG AA") {
		t.Fatalf("defaults check = %d:\n%s", code, output)
	}

	writeTheme := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTheme(":root {\n  --accent: #7c3aed;\n}\n:root[data-theme=\"dark\"] { --accent: #a78bfa; }\n")
	if output, code := check(); code != 0 || !strings.Contains(output, "Valid theme") || !strings.Contains(output, "1 light and 1 dark overrides") {
		t.Fatalf("valid check = %d:\n%s", code, output)
	}

	writeTheme(":root {\n  --muted: #cccccc;\n  --primary-bg: #eeeeee;\n}\n:root[data-theme=\"dark\"] {\n  --diagram-blue-ink: #15315e;\n}\n")
	output, code := check()
	if code != 3 {
		t.Fatalf("contrast failures exited %d:\n%s", code, output)
	}
	for _, want := range []string{
		"Contrast: 3 of", "light: --muted #cccccc on --bg #ffffff is 1.", "light: --primary-ink #fff on --primary-bg #eeeeee is 1.",
		"dark: --diagram-blue-ink #15315e on --diagram-blue-sticky #15315e is 1.00:1",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("check output lacks %q:\n%s", want, output)
		}
	}
	jsonOutput, code := check("--json")
	var result themeCheckResult
	if err := json.Unmarshal([]byte(jsonOutput), &result); err != nil || code != 3 || !result.Valid || result.Failing != 3 || len(result.Contrast) != 2*len(theme.ContrastPairs()) {
		t.Fatalf("check --json = %d %v %+v", code, err, result)
	}

	writeTheme(":root {\n  --bg: #fff;\n  --bg: url(x.png);\n}\n@import 'x.css';\n")
	output, code = check()
	if code != 1 || !strings.Contains(output, "Invalid theme") || !strings.Contains(output, "theme.css:3: url() is not allowed") || !strings.Contains(output, "theme.css:5: @-rules") {
		t.Fatalf("invalid check = %d:\n%s", code, output)
	}
}

func TestValidateReportsAnInvalidTheme(t *testing.T) {
	root := themeSaga(t)
	if err := os.WriteFile(filepath.Join(root, theme.FileName), []byte(":root {\n  --nope: red;\n}\nbody { --bg: red; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err := Validate(context.Background(), []string{root}, &output)
	if statusCode(err) != 1 || !strings.Contains(output.String(), "Invalid saga") ||
		!strings.Contains(output.String(), "error: theme.css:2: theme: unknown token --nope") ||
		!strings.Contains(output.String(), `error: theme.css:4: theme: selector "body" is not allowed`) {
		t.Fatalf("validate = %v:\n%s", err, output.String())
	}
	if err := os.WriteFile(filepath.Join(root, theme.FileName), []byte(theme.Starter()), 0o644); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Validate(context.Background(), []string{root}, &output); err != nil {
		t.Fatalf("the starter theme is invalid: %v\n%s", err, output.String())
	}
}
