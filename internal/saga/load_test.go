package saga

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diffuri"
)

func TestLoadRecursiveFragmentsAndReviewOverlay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "content.md"), "# The whole story\n")
	writeTestFile(t, filepath.Join(root, "backend.chapter", "chapter.json"), `{"version":2,"id":"backend","title":"Backend"}`)
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "section.json"), `{"version":2,"id":"request-flow","title":"Request flow"}`)
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "flow.fragment", "fragment.json"), `{"version":2,"id":"flow","title":"Flow","media_type":"text/html","entrypoint":"index.html"}`)
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "flow.fragment", "index.html"), `<button id="try-flow" onclick="this.textContent='ok'">Try it</button>`)
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "flow.fragment", "___landmarks", "try-flow.landmark", "landmark.json"), `{"version":2,"id":"try-flow","label":"Try the flow","selector":{"type":"element","element_id":"try-flow"}}`)

	diff, err := diffuri.Build(diffuri.Reference{Repository: "https://example.test/acme/app.git", Base: "aaa", Head: "bbb", Kind: "line", Path: "api.go", Side: "new", Start: 2, End: 4})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "flow.fragment", CodeDirName, "api.json"), fmt.Sprintf(`{"version":2,"diffs":[{"uri":%q}]}`, diff))
	writeTestFile(t, filepath.Join(root, "backend.chapter", "request-flow", "flow.fragment", "___landmarks", "try-flow.landmark", CodeDirName, "api.json"), fmt.Sprintf(`{"version":2,"diffs":[{"uri":%q}]}`, diff))
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "thread.json"), `{"version":2,"id":"thread-1","target":"urn:change-saga:test:fragment:flow","anchor":{"type":"region","coordinate_space":"normalized","shapes":[{"type":"rect","x":0.1,"y":0.2,"width":0.3,"height":0.4}]},"created_by":"Ada","created_at":"2026-08-19T12:00:00Z"}`)
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "messages", "message-1.message", "message.json"), `{"version":2,"id":"message-1","author":"Ada","created_at":"2026-08-19T12:00:00Z"}`)
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "messages", "message-1.message", "body.fragment", "fragment.json"), `{"version":2,"id":"message-body","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "messages", "message-1.message", "body.fragment", "content.md"), "Please explain this transition.\n")
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "events", "withdrawn.json"), `{"version":2,"id":"withdrawn","state":"withdrawn","created_at":"2026-08-19T12:01:00Z"}`)
	writeTestFile(t, filepath.Join(root, "___review", "threads", "thread-1.thread", "events", "moved.json"), `{"version":2,"id":"moved","anchor":{"type":"region","coordinate_space":"normalized","shapes":[{"type":"rect","x":0.2,"y":0.3,"width":0.3,"height":0.4}]},"created_at":"2026-08-19T12:02:00Z"}`)

	document, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("saga should be valid: %#v", validation)
	}
	if len(document.Section.Fragments) != 1 || len(document.Section.Children) != 1 {
		t.Fatalf("unexpected root: %#v", document.Section)
	}
	chapter := document.Section.Children[0]
	if chapter.Kind != "chapter" || chapter.Target != ChapterTarget("test", "backend") || len(chapter.Children) != 1 {
		t.Fatalf("chapter was not loaded as a review boundary: %#v", chapter)
	}
	flow := chapter.Children[0].Fragments[0]
	if flow.MediaType != "text/html" || len(flow.Code) != 1 || len(flow.Landmarks) != 1 || flow.Landmarks[0].Selector.ElementID != "try-flow" || flow.Landmarks[0].Target != LandmarkTarget("test", "flow", "try-flow") || len(flow.Landmarks[0].Code) != 1 {
		t.Fatalf("interactive fragment was not loaded: %#v", flow)
	}
	if len(document.Threads) != 1 || len(document.Threads[0].Messages) != 1 || document.Threads[0].Target != flow.Target || document.Threads[0].State != "withdrawn" || document.Threads[0].Anchor.Shapes[0].X != .2 {
		t.Fatalf("review overlay was not loaded: %#v", document.Threads)
	}
}

func TestLoadOutlineDoesNotOpenCoverageOrContentTrees(t *testing.T) {
	root := filepath.Join(t.TempDir(), "outline.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"outline","title":"Outline","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "fragment.json"), `{"version":2,"id":"overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "content.md"), strings.Repeat("large narrative body\n", 1024))
	writeTestFile(t, filepath.Join(root, "overview.fragment", CodeDirName, "broken.json"), `{this is deliberately not JSON`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "___landmarks", "broken.landmark", "landmark.json"), `{this is deliberately not JSON`)

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
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"narrative","title":"Narrative","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "story.fragment", "fragment.json"), `{"version":2,"id":"story","title":"Story","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "story.fragment", "content.md"), "# Story\n")
	uri, err := diffuri.Build(diffuri.Reference{Repository: "https://example.test/acme/app.git", Base: "aaa", Head: "bbb", Kind: "line", Path: "app.go", Side: "new", Start: 1, End: 2})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "story.fragment", CodeDirName, "app.json"), fmt.Sprintf(`{"version":2,"diffs":[{"uri":%q,"note":"Implements the story."}]}`, uri))

	document, validation, err := LoadNarrative(root)
	if err != nil || !validation.Valid {
		t.Fatalf("narrative load = valid %v, err %v, issues %#v", validation.Valid, err, validation.Issues)
	}
	fragment := document.Section.Fragments[0]
	if !fragment.HasCode || len(fragment.Code) != 0 {
		t.Fatalf("narrative evidence state = has %v, materialized %d", fragment.HasCode, len(fragment.Code))
	}
	diffs, targetValidation, err := LoadTargetCode(MutationIndexFromDocument(document), fragment.Target)
	if err != nil || !targetValidation.Valid || len(diffs) != 1 || len(diffs[0].References) != 1 || diffs[0].References[0].URI != uri {
		t.Fatalf("target evidence = %#v, valid %v, err %v, issues %#v", diffs, targetValidation.Valid, err, targetValidation.Issues)
	}
}

func TestLoadRejectsNestedChapter(t *testing.T) {
	root := filepath.Join(t.TempDir(), "test.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/a.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "outer.chapter", "chapter.json"), `{"version":2,"id":"outer","title":"Outer"}`)
	writeTestFile(t, filepath.Join(root, "outer.chapter", "inner.chapter", "chapter.json"), `{"version":2,"id":"inner","title":"Inner"}`)
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
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","surprise":true,"source":{"repository":"https://example.test/a.git","base":"main","head":"HEAD"}}`)
	if _, _, err := Load(root); err == nil {
		t.Fatal("expected unknown manifest field to fail")
	}
}

func TestLoadDesignReusesAddressableHierarchyAndMutationIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "design.saga")
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"design","title":"Living design","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "fragment.json"), `{"version":2,"id":"narrative-overview","title":"Overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "overview.fragment", "content.md"), "Root narrative remains readable.\n")
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "chapter.json"), `{"version":2,"id":"architecture","title":"Architecture","order":2}`)
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "overview.fragment", "fragment.json"), `{"version":2,"id":"architecture-overview","title":"Architecture overview","media_type":"text/markdown","entrypoint":"content.md"}`)
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "overview.fragment", "content.md"), "Design overview.\n")
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "request-flow", "section.json"), `{"version":2,"id":"request-flow","title":"Request flow"}`)
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "fragment.json"), `{"version":2,"id":"sequence","title":"Sequence","media_type":"image/svg+xml","entrypoint":"sequence.svg"}`)
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "sequence.svg"), `<svg viewBox="0 0 10 10"><path id="retry-edge" d="M0 0 L10 10"/></svg>`)
	writeTestFile(t, filepath.Join(root, "___design", "architecture.chapter", "request-flow", "sequence.fragment", "___landmarks", "retry-edge.landmark", "landmark.json"), `{"version":2,"id":"retry-edge","label":"Retry edge","description":"Retries failed requests.","selector":{"type":"element","element_id":"retry-edge"}}`)

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
	if chapter.Path != "___design/architecture.chapter" || chapter.Target != ChapterTarget("design", "architecture") || len(chapter.Children) != 1 {
		t.Fatalf("design chapter = %#v", chapter)
	}
	fragment := chapter.Children[0].Fragments[0]
	if fragment.Path != "___design/architecture.chapter/request-flow/sequence.fragment" || fragment.Target != FragmentTarget("design", "sequence") || len(fragment.Landmarks) != 1 || fragment.Landmarks[0].Target != LandmarkTarget("design", "sequence", "retry-edge") {
		t.Fatalf("design fragment = %#v", fragment)
	}

	index, indexValidation, err := LoadMutationIndex(root)
	if err != nil || !indexValidation.Valid {
		t.Fatalf("LoadMutationIndex(design) = valid %v, err %v, issues %#v", indexValidation.Valid, err, indexValidation.Issues)
	}
	for target, wantDir := range map[string]string{
		chapter.Target:               filepath.Join(root, "___design", "architecture.chapter"),
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
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/acme/app.git","base":"main","head":"HEAD"}}`)
	for _, base := range []string{"overview.fragment", filepath.Join("___design", "overview.fragment")} {
		writeTestFile(t, filepath.Join(root, base, "fragment.json"), `{"version":2,"id":"shared","media_type":"text/markdown","entrypoint":"content.md"}`)
		writeTestFile(t, filepath.Join(root, base, "content.md"), "Content.\n")
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
			if err := os.MkdirAll(filepath.Join(present, name), 0o755); err != nil {
				t.Fatal(err)
			}
			validation, report := loadIssues(t, present)
			if !validation.Valid {
				t.Fatalf("rejected %s:\n%s", name, report)
			}

			fileRoot := buildSaga(t, map[string]string{name: "not a directory\n"})
			validation, report = loadIssues(t, fileRoot)
			if validation.Valid || !strings.Contains(report, "real directory") {
				t.Fatalf("accepted non-directory %s:\n%s", name, report)
			}

			symlinkRoot := buildSaga(t, nil)
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(symlinkRoot, name)); err != nil {
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
	writeTestFile(t, filepath.Join(root, "saga.json"), `{"version":5,"id":"test","title":"A saga","source":{"repository":"https://example.test/a.git","base":"main","head":"HEAD"}}`)
	writeTestFile(t, filepath.Join(root, "broken.fragment", "fragment.json"), `{"version":2,"id":"broken","media_type":"text/html","entrypoint":"index.html"}`)
	_, validation, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatal("missing entrypoint should invalidate saga")
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
