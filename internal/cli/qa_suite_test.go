package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func e2eSuiteCommand(ctx context.Context, args []string, out *bytes.Buffer) error {
	return E2ESuite(ctx, args, out)
}

func e2eSuiteReportOf(t *testing.T, root string) e2eSuiteReport {
	t.Helper()
	var output bytes.Buffer
	if err := E2ESuite(context.Background(), []string{"show", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	var report e2eSuiteReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("%v in %s", err, output.String())
	}
	return report
}

func TestE2ESuiteIsOptional(t *testing.T) {
	fixture, _ := newEmptyReviewFixture(t)
	if report := e2eSuiteReportOf(t, fixture.root); report.Declared || len(report.Paths) != 0 {
		t.Fatalf("report = %+v", report)
	}
	var output bytes.Buffer
	if err := E2ESuite(context.Background(), []string{"show", fixture.root}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "which is fine") {
		t.Fatalf("a Saga with none should not read as a problem: %s", output.String())
	}
}

func TestE2ESuiteSetsChangesAndClears(t *testing.T) {
	fixture, _ := newEmptyReviewFixture(t)
	root := fixture.root
	run(t, E2ESuite, "set", "--path", "e2e/tests", "--path", "more/e2e", "--command", "npx playwright test {locations}", root)
	report := e2eSuiteReportOf(t, root)
	if !report.Declared || strings.Join(report.Paths, ",") != "e2e/tests,more/e2e" || report.RunCommand != "npx playwright test {locations}" {
		t.Fatalf("report = %+v", report)
	}
	// Neither of these stops the Saga being valid; the tests may not exist yet.
	if strings.Join(report.MissingPaths, ",") != "e2e/tests,more/e2e" {
		t.Fatalf("missing = %v", report.MissingPaths)
	}
	if _, validation, err := saga.Load(root); err != nil || !validation.Valid {
		t.Fatalf("load: %v, %#v", err, validation.Issues)
	}
	run(t, E2ESuite, "set", "--path", "e2e/tests", root)
	if report := e2eSuiteReportOf(t, root); strings.Join(report.Paths, ",") != "e2e/tests" || report.RunCommand == "" {
		t.Fatalf("setting paths must keep the command: %+v", report)
	}
	run(t, E2ESuite, "set", "--command", "", root)
	if report := e2eSuiteReportOf(t, root); report.RunCommand != "" || len(report.Paths) != 1 {
		t.Fatalf("an empty command must remove only it: %+v", report)
	}
	run(t, E2ESuite, "clear", root)
	if report := e2eSuiteReportOf(t, root); report.Declared {
		t.Fatalf("report = %+v", report)
	}
	if _, err := os.Stat(filepath.Join(root, saga.QADir)); !os.IsNotExist(err) {
		t.Fatalf("clear left %s behind: %v", saga.QADir, err)
	}
	run(t, E2ESuite, "clear", root) // Clearing nothing is fine.
}

func TestE2ESuiteRefusals(t *testing.T) {
	fixture, _ := newEmptyReviewFixture(t)
	root := fixture.root
	cases := map[string][]string{
		"inside the repository":  {"set", "--path", "../outside", root},
		"inside the repository ": {"set", "--path", "/absolute", root},
		"more than once":         {"set", "--path", "e2e", "--path", "e2e", root},
		"{locations}":            {"set", "--path", "e2e", "--command", "npx playwright test", root},
		"at least one path":      {"set", "--command", "npx playwright test {locations}", root},
		"usage":                  {"set", root},
	}
	for want, args := range cases {
		if message := refused(t, e2eSuiteCommand, args...); !strings.Contains(message, strings.TrimSpace(want)) {
			t.Errorf("%v: want %q in %q", args, want, message)
		}
	}
	if _, err := os.Stat(filepath.Join(root, saga.QADir)); !os.IsNotExist(err) {
		t.Fatalf("a refusal wrote %s: %v", saga.QADir, err)
	}
}

func TestE2ESuiteRejectsStrayEntries(t *testing.T) {
	fixture, _ := newEmptyReviewFixture(t)
	dir := filepath.Join(fixture.root, saga.QADir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, validation, err := saga.Load(fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("a stray entry in the QA directory was accepted")
	}
}
