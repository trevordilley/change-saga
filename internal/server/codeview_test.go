package server

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func BenchmarkMakeCodeReviewViewLargeSaga(b *testing.B) {
	document, changes, report, selection := largeCodeViewFixture(b, 120, 20)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		view, selectionErr := makeCodeReviewView(document, changes, report, nil, selection)
		if selectionErr != nil {
			b.Fatal(selectionErr)
		}
		if view.SelectedFile == nil || len(view.Files) != 120 || len(view.NarrativeOwnership) != 120 {
			b.Fatalf("incomplete code view: %#v", view)
		}
	}
}

func TestCodeDiffURLPreservesPathAndExactCodeLocation(t *testing.T) {
	ref := testLocation(testHeadCommit, "dir/a b&c.go", 7, 9)
	href := CodeDiffURL("dir/a b&c.go", ref)
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("view") != "code" || parsed.Query().Get("file") != "dir/a b&c.go" || parsed.Query().Get("ref") != ref {
		t.Fatalf("URL did not round trip selection: %s", href)
	}
}

func TestChangedFileTreeIsNestedAndAggregatesReviewState(t *testing.T) {
	files := []*FileDiffView{
		{Path: "README.md", Deleted: 3},
		{Path: "src/api/handler.go", Added: 2, Deleted: 1, Reviewed: true, Selected: true},
		{Path: "src/ui/view.js", Added: 4},
	}
	tree := makeChangedFileTree(files)
	if tree.FileCount != 3 || tree.ReviewedCount != 1 || tree.Added != 6 || tree.Deleted != 4 {
		t.Fatalf("unexpected root aggregates: %#v", tree)
	}
	if len(tree.Nodes) != 2 || tree.Nodes[0].Name != "src" || tree.Nodes[0].Kind != "folder" {
		t.Fatalf("expected sorted folder and file roots: %#v", tree.Nodes)
	}
	src := tree.Nodes[0]
	if !src.Selected || !src.Expanded || src.FileCount != 2 || src.ReviewedCount != 1 || len(src.Children) != 2 {
		t.Fatalf("unexpected src folder state: %#v", src)
	}
	api := src.Children[0]
	if api.Name != "api" || !api.Selected || !api.Expanded || len(api.Children) != 1 || api.Children[0].Kind != "file" || !api.Children[0].Selected {
		t.Fatalf("selected path was not propagated through folders: %#v", api)
	}
}

func TestCodeReviewViewScopesReverseOwnershipAndKeepsForwardLinks(t *testing.T) {
	document, changes, report, secondRef, staleRef := codeViewFixture(t)
	view, selectionErr := makeCodeReviewView(document, changes, report, nil, codeSelection{filePath: "src/api/handler.go"})
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	if view.SelectedFile == nil || view.SelectedFile.Path != "src/api/handler.go" || !view.SelectedFile.Selected {
		t.Fatalf("unexpected selected file: %#v", view.SelectedFile)
	}
	if view.Tree.FileCount != 2 || len(view.RelatedSaga) != 2 || view.RelatedSaga[0].Title != "Overview" || view.RelatedSaga[1].Title != "Backend" {
		t.Fatalf("file ownership was not grouped in saga order: %#v", view.RelatedSaga)
	}
	flow := view.RelatedSaga[1].Fragments[0]
	if strings.Contains(strings.ToLower(flow.Excerpt), "script") || strings.Contains(flow.Excerpt, "alert") || !strings.Contains(flow.Excerpt, "Visible explanation") {
		t.Fatalf("excerpt was not safe visible text: %q", flow.Excerpt)
	}
	if flow.Href != sagaHref(flow.Target) || len(flow.Refs) != 2 {
		t.Fatalf("fragment reverse link is not exact: %#v", flow)
	}

	view, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{ref: secondRef})
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	if view.SelectedDiff == nil || view.SelectedDiff.Ref != secondRef || len(view.RelatedSaga) != 1 || view.RelatedSaga[0].Title != "Backend" {
		t.Fatalf("exact diff selection did not narrow reverse ownership: %#v", view.RelatedSaga)
	}

	var flowOwnership *FragmentOwnershipView
	for _, ownership := range view.NarrativeOwnership {
		if ownership.FragmentID == "flow" {
			flowOwnership = ownership
		}
	}
	if flowOwnership == nil || len(flowOwnership.Diffs) != 2 || !flowOwnership.Diffs[0].Available || len(flowOwnership.Diffs[0].MatchedRefs) != 2 {
		t.Fatalf("missing forward available ownership: %#v", flowOwnership)
	}
	forwardURL, err := url.Parse(flowOwnership.Diffs[0].Href)
	if err != nil {
		t.Fatal(err)
	}
	if forwardURL.Query().Get("file") != "src/api/handler.go" || forwardURL.Query().Get("ref") != flowOwnership.Diffs[0].Ref {
		t.Fatalf("forward ownership deep link lost its exact selection: %s", flowOwnership.Diffs[0].Href)
	}
	if flowOwnership.Diffs[1].Available || flowOwnership.Diffs[1].Ref != staleRef || flowOwnership.Diffs[1].Reason != staleCodeReason {
		t.Fatalf("missing fully-qualified stale ownership: %#v", flowOwnership.Diffs[1])
	}
}

func TestCodeReviewSelectionRejectsUnknownOrMismatchedValues(t *testing.T) {
	document, changes, report, _, _ := codeViewFixture(t)
	fileRef := testLocation(changes.HeadOID, "src/api/handler.go", 0, 0)
	view, selectionErr := makeCodeReviewView(document, changes, report, nil, codeSelection{ref: fileRef})
	if selectionErr != nil || view.SelectedFile.Path != "src/api/handler.go" || len(view.SelectedDiffs) != 2 || len(view.RelatedSaga) != 2 {
		t.Fatalf("qualified file selection failed: view=%#v error=%v", view, selectionErr)
	}
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{filePath: "missing.go"})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("unknown file error = %#v", selectionErr)
	}
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{ref: "not-a-diff-uri"})
	if selectionErr == nil || selectionErr.status != http.StatusBadRequest {
		t.Fatalf("malformed diff error = %#v", selectionErr)
	}
	// The changed line exists at the head commit; the same line number at a
	// commit outside the comparison is a different line.
	foreign := testLocation(testForeignCommit, "src/api/handler.go", 11, 11)
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{ref: foreign})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("foreign diff error = %#v", selectionErr)
	}
	// The added line lives at the head commit, not at the merge-base.
	wrongSide := testLocation(changes.BaseOID, "src/api/handler.go", 11, 11)
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{ref: wrongSide})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("wrong-side diff error = %#v", selectionErr)
	}
	foreignFile := testLocation(testForeignCommit, "src/api/handler.go", 0, 0)
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{ref: foreignFile})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("foreign whole-file error = %#v", selectionErr)
	}
	unchangedFile := testLocation(changes.HeadOID, "src/api/unchanged.go", 0, 0)
	_, selectionErr = makeCodeReviewView(document, changes, report, nil, codeSelection{filePath: "src/api/handler.go", ref: unchangedFile})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("unchanged file diff error = %#v", selectionErr)
	}
	emptyDocument := &saga.Saga{Manifest: saga.Manifest{ID: "empty"}, Section: &saga.Section{Target: saga.SagaTarget("empty")}}
	_, selectionErr = makeCodeReviewView(emptyDocument, gitdiff.ChangeSet{}, coverage.Report{}, nil, codeSelection{ref: fileRef})
	if selectionErr == nil || selectionErr.status != http.StatusNotFound {
		t.Fatalf("diff against empty comparison error = %#v", selectionErr)
	}
}

func TestRelatedSagaEmptyStateIsExplicit(t *testing.T) {
	document, changes, report, _, _ := codeViewFixture(t)
	view, selectionErr := makeCodeReviewView(document, changes, report, nil, codeSelection{filePath: "docs/unowned.md"})
	if selectionErr != nil {
		t.Fatal(selectionErr)
	}
	if len(view.RelatedSaga) != 0 || view.RelatedEmpty == "" {
		t.Fatalf("missing empty narrative state: %#v", view)
	}
}

func TestRelatedSagaLinksBackToExactLandmark(t *testing.T) {
	fragment := &saga.Fragment{ID: "flow", Title: "Flow", Target: saga.FragmentTarget("test", "flow")}
	landmarkTarget := saga.LandmarkTarget("test", "flow", "submit-action")
	location := narrativeLocation{
		fragment: fragment, target: landmarkTarget, itemID: "submit-action", title: "Submit action",
		chapterID: "backend", chapterTitle: "Backend", chapterTarget: saga.ChapterTarget("test", "backend"),
		chapterHref: sagaHref(saga.ChapterTarget("test", "backend")), fragmentHref: sagaHref(fragment.Target) + "--submit-action",
	}
	atom := &diffAtomView{Atom: gitdiff.Atom{Key: "changed", Ref: testLocation(testHeadCommit, "app.go", 1, 1)}}
	result := makeRelatedSagaViews([]narrativeLocation{location}, []*diffAtomView{atom}, map[string][]coverage.Assignment{
		"changed": {{Target: landmarkTarget}},
	})
	if len(result) != 1 || len(result[0].Fragments) != 1 || result[0].Fragments[0].Title != "Submit action" || result[0].Fragments[0].Href != location.fragmentHref || result[0].Fragments[0].Anchor != strings.TrimPrefix(location.fragmentHref, "#") {
		t.Fatalf("landmark reverse link = %#v", result)
	}
}

func TestRelatedSagaRollsItemOwnersUpToSlidesAndGroupsByDeck(t *testing.T) {
	deckTarget := saga.DeckTarget("visual", "implementation")
	firstTarget := saga.SlideTarget("visual", "flow")
	secondTarget := saga.SlideTarget("visual", "failure")
	first := &saga.Fragment{
		ID: "flow", Title: "Request flow", Target: firstTarget, MediaType: "image/svg+xml", Entrypoint: "flow.svg",
		SlideMeta: &saga.SlideManifest{ID: "flow", DeckID: "implementation"},
		Reviews:   []saga.Review{{State: "approved", CreatedAt: time.Now()}},
		Landmarks: []saga.Landmark{
			{ID: "client", Label: "Client", Target: saga.ItemTarget("visual", "flow", "client")},
			{ID: "server", Label: "Server", Target: saga.ItemTarget("visual", "flow", "server")},
		},
	}
	second := &saga.Fragment{
		ID: "failure", Title: "Failure path", Target: secondTarget, MediaType: "text/html", Entrypoint: "failure.html",
		SlideMeta: &saga.SlideManifest{ID: "failure", DeckID: "implementation"},
		Landmarks: []saga.Landmark{{ID: "timeout", Label: "Timeout", Target: saga.ItemTarget("visual", "failure", "timeout")}},
	}
	document := &saga.Saga{
		Manifest: saga.Manifest{Version: saga.SagaVersion, ID: "visual", Title: "Visual review"},
		Section: &saga.Section{Kind: "saga", ID: "visual-root", Target: saga.SagaTarget("visual"), Children: []*saga.Section{{
			Kind: "deck", ID: "implementation", Title: "System tour", Target: deckTarget, Fragments: []*saga.Fragment{first, second},
		}}},
	}
	locations := indexNarrativeFragments(document)
	atoms := []*diffAtomView{
		{Atom: gitdiff.Atom{Key: "client", Ref: testLocation(testHeadCommit, "web/client.js", 1, 1)}},
		{Atom: gitdiff.Atom{Key: "server", Ref: testLocation(testHeadCommit, "api/server.go", 1, 1)}},
		{Atom: gitdiff.Atom{Key: "timeout", Ref: testLocation(testHeadCommit, "api/server.go", 2, 2)}},
	}
	result := makeRelatedSagaViews(locations, atoms, map[string][]coverage.Assignment{
		"client":  {{Target: first.Landmarks[0].Target}},
		"server":  {{Target: first.Landmarks[1].Target}},
		"timeout": {{Target: second.Landmarks[0].Target}},
	})
	if len(result) != 1 || result[0].Title != "System tour" || !result[0].Deck || len(result[0].Fragments) != 2 {
		t.Fatalf("slide owners were not grouped by deck: %#v", result)
	}
	flow := result[0].Fragments[0].Slide
	if flow == nil || flow.Title != "Request flow" || flow.ItemCount != 2 || flow.URL != "/f/flow/flow.svg" || flow.ReviewState != "approved" || flow.Href != sagaHref(firstTarget) {
		t.Fatalf("item owners did not roll up to the visual slide reference: %#v", flow)
	}
	if failure := result[0].Fragments[1].Slide; failure == nil || failure.ItemCount != 1 || failure.MediaType != "text/html" {
		t.Fatalf("second slide reference = %#v", failure)
	}
	var rendered strings.Builder
	if err := serverTemplate(t).ExecuteTemplate(&rendered, "file-owners", fileOwnersView{Groups: result}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(rendered.String(), `class="related-slide"`); got != 2 || strings.Contains(rendered.String(), ">Client<") || strings.Count(rendered.String(), ">System tour<") != 1 {
		t.Fatalf("visual slide references were not deduplicated and grouped: %s", rendered.String())
	}
	owner := manifestOwner(first.Landmarks[0].Target, indexManifestTargets(document))
	if owner.Slide == nil || owner.Slide.Target != first.Target || owner.Title != "Client" || owner.Chapter != "System tour" {
		t.Fatalf("coverage owner lost its exact Item or parent slide: %#v", owner)
	}
	rendered.Reset()
	if err := serverTemplate(t).ExecuteTemplate(&rendered, "manifest-owner-reference", owner); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `class="manifest-slide-owner"`) || !strings.Contains(rendered.String(), "Request flow") || !strings.Contains(rendered.String(), "Client") {
		t.Fatalf("coverage did not render its slide and exact Item: %s", rendered.String())
	}
}

func TestFileViewsAttachRendererContextWithoutChangingAtoms(t *testing.T) {
	old := gitdiff.Atom{Key: "line:app.go:old:2", Kind: "line", Path: "app.go", Side: "old", Line: 2, Content: "old", Ref: "old-uri"}
	added := gitdiff.Atom{Key: "line:app.go:new:2", Kind: "line", Path: "app.go", Side: "new", Line: 2, Content: "new", Ref: "new-uri"}
	changes := gitdiff.ChangeSet{
		Repository: "https://example.test/a.git", BaseOID: "aaa", HeadOID: "bbb", Atoms: []gitdiff.Atom{old, added},
		DisplayLines: []gitdiff.DisplayLine{
			{Kind: "context", Path: "app.go", OldLine: 1, NewLine: 1, Content: "package app"},
			{Kind: "old", Path: "app.go", OldLine: 2, Content: "old", AtomKey: old.Key},
			{Kind: "new", Path: "app.go", NewLine: 2, Content: "new", AtomKey: added.Key},
		},
	}
	files := makeFileViews(changes, "urn:change-saga:test:saga", nil, nil)
	if len(files) != 1 || len(files[0].Atoms) != 2 || len(files[0].Lines) != 3 {
		if len(files) == 0 {
			t.Fatal("focused file was not built")
		}
		t.Fatalf("unexpected focused file: atoms=%d lines=%d lines=%#v", len(files[0].Atoms), len(files[0].Lines), files[0].Lines)
	}
	if files[0].Lines[0].Atom != nil || files[0].Lines[1].Atom == nil || files[0].Lines[1].Atom.Ref != "old-uri" || files[0].Lines[2].Atom.Ref != "new-uri" {
		t.Fatalf("display lines did not preserve atom actions: %#v", files[0].Lines)
	}
}

func TestFragmentExcerptIsConciseAndCannotFollowEscapingSymlink(t *testing.T) {
	directory := t.TempDir()
	writeServerFile(t, filepath.Join(directory, "content.md"), "# Heading\n"+strings.Repeat("word ", 100))
	fragment := &saga.Fragment{ID: "story", Title: "Story", Directory: directory, MediaType: "text/markdown", Entrypoint: "content.md"}
	excerpt := fragmentExcerpt(fragment)
	if utf8.RuneCountInString(excerpt) > 181 || !strings.HasSuffix(excerpt, "…") {
		t.Fatalf("excerpt is not bounded: %d runes, %q", utf8.RuneCountInString(excerpt), excerpt)
	}

	outside := filepath.Join(filepath.Dir(directory), "outside.txt")
	writeServerFile(t, outside, "private outside content")
	if err := os.Symlink(outside, filepath.Join(directory, "escape.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fragment.Entrypoint = "escape.md"
	if excerpt := fragmentExcerpt(fragment); excerpt != "Story" {
		t.Fatalf("escaping symlink content was exposed: %q", excerpt)
	}
}

// The excerpt sits beside the fragment title in the explanations panel, so it
// should read as prose: no heading label running into the first sentence and no
// leftover Markdown delimiters.
func TestFragmentExcerptReadsAsProseNotMarkdownSource(t *testing.T) {
	directory := t.TempDir()
	writeServerFile(t, filepath.Join(directory, "content.md"), "## CLI surface {#cli-surface}\n\nThe CLI runs `validate` and **checks** the tree.\n")
	fragment := &saga.Fragment{ID: "cli", Title: "CLI and AI workflow", Directory: directory, MediaType: "text/markdown", Entrypoint: "content.md"}
	excerpt := fragmentExcerpt(fragment)
	if want := "The CLI runs validate and checks the tree."; excerpt != want {
		t.Fatalf("excerpt = %q, want %q", excerpt, want)
	}

	// Headings are only dropped when prose survives them. A fragment made of
	// nothing but headings keeps them rather than showing an empty excerpt.
	writeServerFile(t, filepath.Join(directory, "content.md"), "# Only a heading\n")
	if excerpt := fragmentExcerpt(fragment); excerpt != "Only a heading" {
		t.Fatalf("heading-only fragment lost its last readable text: %q", excerpt)
	}
}

func codeViewFixture(t *testing.T) (*saga.Saga, gitdiff.ChangeSet, coverage.Report, string, string) {
	t.Helper()
	root := t.TempDir()
	overviewDir := filepath.Join(root, "overview.fragment")
	flowDir := filepath.Join(root, "backend.chapter", "flow.fragment")
	writeServerFile(t, filepath.Join(overviewDir, "content.md"), "# Overview\nA concise overview.\n")
	writeServerFile(t, filepath.Join(flowDir, "index.html"), `<script>alert("not excerpted")</script><p>Visible explanation of the request flow.</p>`)

	firstRef := testLocation(testHeadCommit, "src/api/handler.go", 10, 10)
	secondRef := testLocation(testHeadCommit, "src/api/handler.go", 11, 11)
	rangeReference := testReference(testHeadCommit, "src/api/handler.go", 10, 11, "request handling")
	staleReference := testReference(testForeignCommit, "src/api/handler.go", 99, 99, "")
	staleRef := staleReference.Location().String()
	first := gitdiff.Atom{Kind: "line", Path: "src/api/handler.go", Side: "new", Line: 10, Content: "first", Ref: firstRef}
	first.Key = gitdiff.Key(first)
	second := gitdiff.Atom{Kind: "line", Path: "src/api/handler.go", Side: "new", Line: 11, Content: "second", Ref: secondRef}
	second.Key = gitdiff.Key(second)
	unownedRef := testLocation(testHeadCommit, "docs/unowned.md", 1, 1)
	unowned := gitdiff.Atom{Kind: "line", Path: "docs/unowned.md", Side: "new", Line: 1, Content: "docs", Ref: unownedRef}
	unowned.Key = gitdiff.Key(unowned)

	overview := &saga.Fragment{ID: "overview", Title: "Overview", Target: saga.FragmentTarget("test", "overview"), Directory: overviewDir, MediaType: "text/markdown", Entrypoint: "content.md"}
	flowDiffPath := filepath.Join(flowDir, saga.CodeDirName, "flow.json")
	flow := &saga.Fragment{
		ID: "flow", Title: "Request flow", Target: saga.FragmentTarget("test", "flow"), Directory: flowDir, MediaType: "text/html", Entrypoint: "index.html",
		Code: []saga.CodeFile{{Path: flowDiffPath, References: []coderef.Reference{rangeReference, staleReference}}},
	}
	chapter := &saga.Section{Kind: "chapter", ID: "backend", Title: "Backend", Target: saga.ChapterTarget("test", "backend"), Fragments: []*saga.Fragment{flow}}
	section := &saga.Section{Kind: "saga", ID: "test", Title: "Test", Target: saga.SagaTarget("test"), Fragments: []*saga.Fragment{overview}, Children: []*saga.Section{chapter}}
	document := &saga.Saga{Root: root, Manifest: saga.Manifest{ID: "test"}, Section: section}
	changes := gitdiff.ChangeSet{Repository: "https://example.test/repo.git", BaseOID: testBaseCommit, HeadOID: testHeadCommit, Atoms: []gitdiff.Atom{first, second, unowned}}
	report := coverage.Report{
		Ownership: map[string][]coverage.Assignment{
			first.Key:  {{Target: overview.Target}, {Target: flow.Target, EvidenceFile: flowDiffPath, Reference: 1}},
			second.Key: {{Target: flow.Target, EvidenceFile: flowDiffPath, Reference: 1}},
		},
		StaleReferences: []coverage.StaleReference{{
			Assignment: coverage.Assignment{Target: flow.Target, EvidenceFile: flowDiffPath, Reference: 2},
			Reference:  staleReference, Reason: staleCodeReason,
		}},
	}
	document.FileReviews = []saga.FileReview{{
		Code: testReference(changes.HeadOID, "src/api/handler.go", 0, 0, ""), Author: "Ada", State: "reviewed", CreatedAt: time.Now(),
	}}
	return document, changes, report, secondRef, staleRef
}

func largeCodeViewFixture(tb testing.TB, fileCount, linesPerFile int) (*saga.Saga, gitdiff.ChangeSet, coverage.Report, codeSelection) {
	tb.Helper()
	const (
		repository = "https://example.test/org/large.git"
	)
	base, head := testBaseCommit, testHeadCommit
	document := &saga.Saga{
		Manifest: saga.Manifest{ID: "large"},
		Section:  &saga.Section{Kind: "saga", ID: "large", Title: "Large", Target: saga.SagaTarget("large")},
	}
	chapter := &saga.Section{Kind: "chapter", ID: "implementation", Title: "Implementation", Target: saga.ChapterTarget("large", "implementation")}
	document.Section.Children = []*saga.Section{chapter}
	changes := gitdiff.ChangeSet{Repository: repository, BaseOID: base, HeadOID: head}
	report := coverage.Report{Ownership: make(map[string][]coverage.Assignment, fileCount*linesPerFile)}

	for fileIndex := range fileCount {
		filePath := fmt.Sprintf("internal/component%03d/handler.go", fileIndex)
		fragmentID := fmt.Sprintf("component-%03d", fileIndex)
		fragment := &saga.Fragment{
			ID: fragmentID, Title: fmt.Sprintf("Component %03d", fileIndex),
			Target: saga.FragmentTarget("large", fragmentID), MediaType: "application/octet-stream",
		}
		diffPath := fmt.Sprintf("implementation.chapter/%s.fragment/___code/implementation.json", fragmentID)
		fragment.Code = []saga.CodeFile{{Path: diffPath, References: []coderef.Reference{testReference(head, filePath, 1, linesPerFile, "Implements the component.")}}}
		chapter.Fragments = append(chapter.Fragments, fragment)
		for line := 1; line <= linesPerFile; line++ {
			atom := gitdiff.Atom{Kind: "line", Path: filePath, Side: "new", Line: line, Content: "changed line"}
			atom.Key = gitdiff.Key(atom)
			atom.Ref = changes.Location(atom).String()
			changes.Atoms = append(changes.Atoms, atom)
			report.Ownership[atom.Key] = []coverage.Assignment{{Target: fragment.Target, EvidenceFile: diffPath, Reference: 1}}
		}
	}
	selectedPath := fmt.Sprintf("internal/component%03d/handler.go", fileCount-1)
	return document, changes, report, codeSelection{filePath: selectedPath}
}

// Hand-built comparisons name full commits so their code locations parse.
var (
	testBaseCommit    = strings.Repeat("a", 40)
	testHeadCommit    = strings.Repeat("b", 40)
	testForeignCommit = strings.Repeat("c", 40)
)

// staleCodeReason is what the pinned resolver reports for a reference whose
// commit is neither side of the hand-built comparison.
var staleCodeReason = "pinned at cccccccccccc, viewed at bbbbbbbbbbbb"

func testLocation(commit, path string, start, end int) string {
	return coderef.Location{Commit: commit, Path: path, Start: start, End: end}.String()
}

// testReference builds a well-formed reference for a hand-built comparison.
// No repository holds its bytes, so the digest is a stand-in derived from the
// location; the pinned resolver does not read content.
func testReference(commit, path string, start, end int, note string) coderef.Reference {
	location := coderef.Location{Commit: commit, Path: path, Start: start, End: end}
	return coderef.Reference{Commit: commit, Path: path, Start: start, End: end, Digest: coderef.DigestBytes([]byte(location.String())), Note: note}
}
