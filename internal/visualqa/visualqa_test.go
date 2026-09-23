package visualqa

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

func writeFixtureFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func visualFixture(t *testing.T, broken bool) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "visual.saga")
	writeFixtureFile(t, filepath.Join(root, "saga.json"), `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"visual","title":"Visual QA","source":{"repository":"https://example.test/acme/app.git"}}`)
	featureDir := filepath.Join(root, "___features", "rendering.feature")
	writeFixtureFile(t, filepath.Join(featureDir, "feature.json"), `{"$schema":"https://changesaga.dev/schema/v5/feature.schema.json","version":5,"id":"rendering","title":"Rendering","created_at":"2026-09-23T00:00:00Z"}`)
	bundle := filepath.Join(featureDir, saga.EmbeddedSlidesDir, "workflow"+saga.EmbeddedDeckSuffix)
	deckTarget := saga.DeckTarget("visual", "workflow")
	deckName, _ := saga.FlatDeckFilename(deckTarget, 0)
	writeFixtureFile(t, filepath.Join(bundle, deckName), `{"version":4,"id":"workflow","title":"Workflow","role":"change","rank":0,"objective":"Review the render workflow."}`)
	slideTarget := saga.SlideTarget("visual", "render")
	slideName, _ := saga.FlatSlideFilename(deckTarget, slideTarget, 0)
	assetName, _ := saga.FlatSlideAssetFilename(slideName, ".html")
	writeFixtureFile(t, filepath.Join(bundle, slideName), fmt.Sprintf(`{"version":4,"id":"render","deck":"workflow","title":"Render","rank":0,"intent":"explain","layout":"sequence","media_type":"text/html","entrypoint":%q,"takeaway":"The same slide is checked raw and in the reviewer.","reading_order":["input","output","runtime"]}`, assetName))
	body := `<!doctype html><style>html,body{margin:0;width:100%;height:100%;overflow:hidden}.box{position:absolute;left:100px;top:100px;width:180px;height:50px;overflow:hidden;background:#dbeafe;border:1px solid #315b84;font:20px sans-serif}#output{left:440px}</style><div id="input" class="box">Input</div><div id="output" class="box">Output</div><div id="runtime"></div>`
	if broken {
		body = `<!doctype html><style>html,body{margin:0;width:100%;height:100%;overflow:hidden}.box{position:absolute;left:100px;top:100px;width:100px;height:24px;overflow:hidden;background:#dbeafe;border:1px solid #315b84;font:20px sans-serif}#output{left:150px}</style><div id="input" class="box">Input label that cannot fit</div><div id="output" class="box">Output</div><div id="runtime"></div><script>document.getElementById('runtime').remove()</script>`
	}
	writeFixtureFile(t, filepath.Join(bundle, assetName), body)
	for rank, item := range []struct{ id, kind string }{{"input", "node"}, {"output", "node"}, {"runtime", "node"}} {
		target := saga.ItemTarget("visual", "render", item.id)
		name, _ := saga.FlatItemFilename(slideTarget, target, rank*10)
		writeFixtureFile(t, filepath.Join(bundle, name), fmt.Sprintf(`{"version":4,"id":%q,"slide":"render","rank":%d,"kind":%q,"label":%q,"description":"Addressable visual node.","selector":{"type":"element","element_id":%q}}`, item.id, rank*10, item.kind, item.id, item.id))
	}
	return root
}

func TestSelectionRejectsEmptyAndUnsupportedContent(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "empty.saga")
	writeFixtureFile(t, filepath.Join(empty, "saga.json"), `{"$schema":"https://changesaga.dev/schema/v5/saga.schema.json","version":5,"id":"empty","title":"Empty","source":{"repository":"https://example.test/acme/app.git"}}`)
	if _, err := Run(context.Background(), Options{SagaRoot: empty, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil || !strings.Contains(err.Error(), "no slides matched") {
		t.Fatalf("empty saga error = %v", err)
	}

	unsupported := visualFixture(t, false)
	entries, err := filepath.Glob(filepath.Join(unsupported, "___features", "rendering.feature", saga.EmbeddedSlidesDir, "workflow.deck", "20-s-*.json"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("find slide manifest: %v %v", entries, err)
	}
	bytes, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, entries[0], strings.Replace(string(bytes), `"text/html"`, `"application/pdf"`, 1))
	if _, err := Run(context.Background(), Options{SagaRoot: unsupported, OutputDir: filepath.Join(t.TempDir(), "out")}); err == nil || !strings.Contains(err.Error(), "structurally invalid") {
		t.Fatalf("unsupported content error = %v", err)
	}
}

func TestOutputPathSafety(t *testing.T) {
	root := visualFixture(t, false)
	if _, err := safeOutputPath(root, "visual", filepath.Join(root, "qa")); err == nil || !strings.Contains(err.Error(), "outside the Saga") {
		t.Fatalf("inside-saga output error = %v", err)
	}
	unmanaged := filepath.Join(t.TempDir(), "existing")
	writeFixtureFile(t, filepath.Join(unmanaged, "keep.txt"), "user data")
	if _, err := safeOutputPath(root, "visual", unmanaged); err == nil || !strings.Contains(err.Error(), "not created by visual-qa") {
		t.Fatalf("unmanaged output error = %v", err)
	}
	symlinkParent := filepath.Join(t.TempDir(), "saga-link")
	if err := os.Symlink(root, symlinkParent); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	nested := filepath.Join(symlinkParent, "missing", "nested", "qa")
	if _, err := safeOutputPath(root, "visual", nested); err == nil || !strings.Contains(err.Error(), "outside the Saga") {
		t.Fatalf("nested path beneath Saga symlink error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(symlinkParent, "missing")); !os.IsNotExist(err) {
		t.Fatalf("path safety check mutated nonexistent output ancestors: %v", err)
	}
}

func TestInstallOutputRestoresPriorReportWhenPublishFails(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "report")
	stage := filepath.Join(parent, "stage")
	writeFixtureFile(t, filepath.Join(output, ".change-saga-visual-qa"), "managed output\n")
	writeFixtureFile(t, filepath.Join(output, "old.txt"), "old report")
	writeFixtureFile(t, filepath.Join(stage, ".change-saga-visual-qa"), "managed output\n")
	writeFixtureFile(t, filepath.Join(stage, "new.txt"), "new report")

	calls := 0
	err := installOutputWithRename(stage, output, func(from, to string) error {
		calls++
		if calls == 2 {
			return errors.New("injected publish failure")
		}
		return os.Rename(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "prior output restored") {
		t.Fatalf("publish failure = %v", err)
	}
	if value, readErr := os.ReadFile(filepath.Join(output, "old.txt")); readErr != nil || string(value) != "old report" {
		t.Fatalf("prior report was not restored: value=%q err=%v", value, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(stage, "new.txt")); statErr != nil {
		t.Fatalf("failed stage should remain recoverable: %v", statErr)
	}
}

func TestInstallOutputReportsRecoverableBackupWhenRestoreFails(t *testing.T) {
	parent := t.TempDir()
	output := filepath.Join(parent, "report")
	stage := filepath.Join(parent, "stage")
	writeFixtureFile(t, filepath.Join(output, ".change-saga-visual-qa"), "managed output\n")
	writeFixtureFile(t, filepath.Join(output, "old.txt"), "old report")
	writeFixtureFile(t, filepath.Join(stage, ".change-saga-visual-qa"), "managed output\n")

	calls := 0
	err := installOutputWithRename(stage, output, func(from, to string) error {
		calls++
		if calls >= 2 {
			return fmt.Errorf("injected rename failure %d", calls)
		}
		return os.Rename(from, to)
	})
	if err == nil || !strings.Contains(err.Error(), "restore prior output from") || !strings.Contains(err.Error(), "injected rename failure 3") {
		t.Fatalf("restore failure = %v", err)
	}
	backups, globErr := filepath.Glob(filepath.Join(parent, ".change-saga-visual-qa-backup-*"))
	if globErr != nil || len(backups) != 1 {
		t.Fatalf("recoverable backup = %v err=%v", backups, globErr)
	}
	if value, readErr := os.ReadFile(filepath.Join(backups[0], "old.txt")); readErr != nil || string(value) != "old report" {
		t.Fatalf("backup did not preserve prior report: value=%q err=%v", value, readErr)
	}
}

func TestRealRenderProducesSurfacesContactSheetAndBrokenFindings(t *testing.T) {
	playwright, err := findPlaywright("")
	if err != nil {
		t.Skip(err)
	}
	root := visualFixture(t, true)
	output := filepath.Join(t.TempDir(), "visual-output")
	report, err := Run(context.Background(), Options{SagaRoot: root, OutputDir: output, PlaywrightDir: playwright, Feature: "rendering", Deck: "workflow", Slide: "render"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Passed || report.SemanticArrows != "not_evaluated" || len(report.Slides) != 1 || len(report.Slides[0].Artifacts) != 4 {
		t.Fatalf("report = %#v", report)
	}
	codes := map[string]bool{}
	for _, value := range report.Findings {
		codes[value.Code] = true
	}
	for _, code := range []string{"missing_selector", "text_overflow", "item_overlap"} {
		if !codes[code] {
			t.Errorf("broken fixture did not report %s: %#v", code, report.Findings)
		}
	}
	for _, name := range []string{"visual-qa.json", "contact-sheet.png", "slides/workflow/render/raw-1280x720.png", "slides/workflow/render/raw-1024x576.png", "slides/workflow/render/reviewer-1280x720.png", "slides/workflow/render/reviewer-1024x576.png"} {
		value, readErr := os.ReadFile(filepath.Join(output, filepath.FromSlash(name)))
		if readErr != nil || len(value) < 100 {
			t.Errorf("generated %s: bytes=%d err=%v", name, len(value), readErr)
		}
		if strings.HasSuffix(name, ".png") && (len(value) < 8 || string(value[:8]) != "\x89PNG\r\n\x1a\n") {
			t.Errorf("generated %s is not a PNG", name)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "runner.mjs")); !os.IsNotExist(err) {
		t.Errorf("bundled helper leaked into output: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "input.json")); !os.IsNotExist(err) {
		t.Errorf("runner input leaked into output: %v", err)
	}
}
