package saga

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOutlineDoesNotOpenCoverageOrContentTrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "outline.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"outline","title":"Outline","source":{"repository":"https://example.test/acme/app.git"}}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "content.md"), strings.Repeat("large narrative body\n", 1024))
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", CodeDirName, "broken.json"), `{this is deliberately not JSON`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "___landmarks", "broken.landmark", "landmark.json"), `{this is deliberately not JSON`)

	document, validation, err := LoadOutline(root)
	if err != nil || !validation.Valid {
		t.Fatalf("outline load = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	fragment := document.Section.Fragments[0]
	if len(fragment.Code) != 0 || len(fragment.Landmarks) != 0 {
		t.Fatalf("outline materialized deferred metadata: diffs=%d landmarks=%d", len(fragment.Code), len(fragment.Landmarks))
	}
	if _, full, err := Load(root); err != nil || full.Valid {
		t.Fatalf("full load did not observe malformed deferred metadata: valid=%v err=%v", full.Valid, err)
	}
}

func TestLoadNarrativeAdvertisesTargetEvidenceWithoutMaterializingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "narrative.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"narrative","title":"Narrative","source":{"repository":"https://example.test/acme/app.git"}}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "story.fragment", "fragment.json"), `{"version":2,"id":"story","title":"Story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "story.fragment", "content.md"), "# Story\n")
	reference := testReference("app.go", 1, 2)
	reference.Note = "Implements the story."
	writeTestFile(t, filepath.Join(root, testEpicDir, "story.fragment", CodeDirName, "app.json"), fmt.Sprintf(`{"version":2,"references":%s}`, referenceJSON(t, reference)))

	document, validation, err := LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("narrative load = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	fragment := document.Section.Fragments[0]
	if !fragment.HasCode || len(fragment.Code) != 0 {
		t.Fatalf("narrative evidence state = has %v, materialized %d", fragment.HasCode, len(fragment.Code))
	}
	diffs, targetValidation, err := LoadTargetCode(MutationIndexFromDocument(document), fragment.Target)
	if err != nil || !targetValidation.Valid || len(diffs) != 1 || len(diffs[0].References) != 1 || diffs[0].References[0] != reference {
		t.Fatalf("target evidence = %#v, valid %v, err %v, issues %#v", diffs, targetValidation.Valid, err, targetValidation.Issues)
	}
}

func TestLoadRejectsNestedChapter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/a.git"}}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "outer.chapter", "chapter.json"), `{"version":2,"id":"outer","title":"Outer"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "outer.chapter", "inner.chapter", "chapter.json"), `{"version":2,"id":"inner","title":"Inner"}`)
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("nested chapters should be invalid; recurse with sections instead")
	}
}

func TestLoadRejectsUnknownJSONFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","surprise":true,"source":{"repository":"https://example.test/a.git"}}`)
	if _, _, err := Load(root); err == nil {
		t.Fatal("expected unknown manifest field to fail")
	}
}

func TestLoadDesignReusesAddressableHierarchyAndMutationIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "design.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"design","title":"Living design","source":{"repository":"https://example.test/acme/app.git"}}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "fragment.json"), `{"version":2,"id":"narrative-overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "overview.fragment", "content.md"), "Root narrative remains readable.\n")
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "chapter.json"), `{"version":2,"id":"architecture","title":"Architecture","order":2}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "overview.fragment", "fragment.json"), `{"version":2,"id":"architecture-overview","title":"Architecture overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "overview.fragment", "content.md"), "Design overview.\n")
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "request-flow", "section.json"), `{"version":2,"id":"request-flow","title":"Request flow"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "fragment.json"), `{"version":2,"id":"sequence","title":"Sequence","media_type":"image/svg+xml","entrypoint":"sequence.svg"}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "sequence.svg"), `<svg viewBox="0 0 10 10"><path id="retry-edge" d="M0 0 L10 10"/></svg>`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "___landmarks", "retry-edge.landmark", "landmark.json"), `{"version":2,"id":"retry-edge","label":"Retry edge","description":"Retries failed requests.","selector":{"type":"element","element_id":"retry-edge"}}`)

	document, validation, err := Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("Load(design) = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	if len(document.Section.Fragments) != 1 || document.Section.Fragments[0].ID != "narrative-overview" {
		t.Fatalf("root narrative changed: %#v", document.Section.Fragments)
	}
	if len(document.Section.Children) != 1 {
		t.Fatalf("design chapter was not joined to the authored hierarchy: %#v", document.Section.Children)
	}
	chapter := document.Section.Children[0]
	if chapter.Path != "___epics/core.epic/___design/architecture.chapter" || chapter.Target != ChapterTarget("design", "architecture") || len(chapter.Children) != 1 {
		t.Fatalf("design chapter = %#v", chapter)
	}
	fragment := chapter.Children[0].Fragments[0]
	if fragment.Path != "___epics/core.epic/___design/architecture.chapter/request-flow/sequence.fragment" || fragment.Target != FragmentTarget("design", "sequence") || len(fragment.Landmarks) != 1 || fragment.Landmarks[0].Target != LandmarkTarget("design", "sequence", "retry-edge") {
		t.Fatalf("design fragment = %#v", fragment)
	}

	index, indexValidation, err := LoadMutationIndex(root)
	if err != nil || !indexValidation.Valid {
		t.Fatalf("LoadMutationIndex(design) = valid %v, err %v, issues %#v", indexValidation.Valid, err, indexValidation.Issues)
	}
	for target, wantDir := range map[string]string{
		chapter.Target:               filepath.Join(root, testEpicDir, "___design", "architecture.chapter"),
		fragment.Target:              fragment.Directory,
		fragment.Landmarks[0].Target: fragment.Landmarks[0].Directory,
	} {
		if got := index.Targets[target]; got != wantDir {
			t.Errorf("index target %s = %q, want %q", target, got, wantDir)
		}
	}
}

func TestLoadDesignRejectsIDsDuplicatedByRootNarrative(t *testing.T) {
	root := filepath.Join(t.TempDir(), "duplicate-design.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/acme/app.git"}}`)
	for _, base := range []string{"overview.fragment", filepath.Join("___design", "overview.fragment")} {
		writeTestFile(t, filepath.Join(root, testEpicDir, base, "fragment.json"), `{"version":2,"id":"shared","media_type":"text/markdown","entrypoint":"content.md"}`)
		writeTestFile(t, filepath.Join(root, testEpicDir, base, "content.md"), "Content.\n")
	}
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("root narrative and technical design reused one globally addressed ID")
	}
}

func TestLivingRootsAreReservedAndMustBeRealDirectories(t *testing.T) {
	for _, name := range []string{"___requirements", "___design", "___workplan", QualityRootDir, EmbeddedSlidesDir} {
		t.Run(name, func(t *testing.T) {
			present := buildSaga(t, nil)
			if err := os.MkdirAll(filepath.Join(present, testEpicDir, name), 0o755); err != nil {
				t.Fatal(err)
			}
			validation, report := loadIssues(t, present)
			if !validation.Valid {
				t.Fatalf("rejected %s:\n%s", name, report)
			}

			// Epic content never sits at the app root.
			atRoot := buildSaga(t, nil)
			if err := os.MkdirAll(filepath.Join(atRoot, name), 0o755); err != nil {
				t.Fatal(err)
			}
			validation, report = loadIssues(t, atRoot)
			if validation.Valid || !strings.Contains(report, "unknown reserved directory") {
				t.Fatalf("accepted %s at the app root:\n%s", name, report)
			}

			fileRoot := buildSaga(t, map[string]string{name: "not a directory\n"})
			validation, report = loadIssues(t, fileRoot)
			if validation.Valid || !strings.Contains(report, "real directory") {
				t.Fatalf("accepted non-directory %s:\n%s", name, report)
			}

			symlinkRoot := buildSaga(t, nil)
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(symlinkRoot, testEpicDir, name)); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			validation, report = loadIssues(t, symlinkRoot)
			if validation.Valid || !strings.Contains(report, "real directory") {
				t.Fatalf("accepted symlinked %s:\n%s", name, report)
			}
		})
	}
}

func TestLoadRejectsMissingFragmentEntrypoint(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/a.git"}}`)
	writeTestFile(t, filepath.Join(root, testEpicDir, "broken.fragment", "fragment.json"), `{"version":2,"id":"broken","media_type":"text/html","entrypoint":"index.html"}`)
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("missing entrypoint should invalidate saga")
	}
}

// testEpicDir is the one epic the package fixtures author report content,
// design, and decks into.
var testEpicDir = filepath.Join("___epics", "core.epic")

const testEpicManifest = `{"$schema":"https://changesaga.dev/schema/v5/epic.schema.json","version":5,"id":"core","title":"Core","created_at":"2026-08-21T12:00:00Z"}`

// inEpic places an app-relative fixture path inside the test epic unless it
// names something that lives at the app root.
func inEpic(rel string) string {
	switch strings.SplitN(rel, "/", 2)[0] {
	case "saga.json", "___review", "___claims", "___verifications", "___approvals", "___code", "___merges", "___overview", "___personas", "___designsystem", "___onboarding", "___featureflags", "___epics", "README.md":
		return rel
	}
	return filepath.ToSlash(filepath.Join(testEpicDir, rel))
}

// writeTestFile writes one fixture file. Writing saga.json also creates the
// test epic, since every fixture authors its content into that epic.
func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if filepath.Base(path) == "saga.json" {
		defer writeTestFile(t, filepath.Join(filepath.Dir(path), testEpicDir, "epic.json"), testEpicManifest)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
