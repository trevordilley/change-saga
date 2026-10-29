package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/prototypes"
)

func newPrototypeSource(t *testing.T, body string) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "prototype")
	writeFile(t, filepath.Join(source, "index.html"), body)
	return source
}

func runPrototype(t *testing.T, args ...string) livingMutationOutput {
	t.Helper()
	var output bytes.Buffer
	if err := Prototype(context.Background(), append(args, "--json"), &output); err != nil {
		t.Fatalf("prototype %v: %v\n%s", args, err, output.String())
	}
	return decodeLivingOutput(t, &output)
}

func TestPrototypeFamilyHelpListsTheAuthoringGrammar(t *testing.T) {
	t.Parallel()
	var first, second bytes.Buffer
	if err := Prototype(context.Background(), []string{"-h"}, &first); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("first help error = %v", err)
	}
	if err := Prototype(context.Background(), []string{"--help"}, &second); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("second help error = %v", err)
	}
	if first.String() != second.String() {
		t.Fatalf("help is not deterministic:\n%s\n---\n%s", first.String(), second.String())
	}
	if !strings.Contains(first.String(), "add-html\n  add-repo\n  add-external\n  revise\n  refresh\n  annotate") {
		t.Fatalf("help omitted the prototype operations:\n%s", first.String())
	}
	for _, operation := range prototypeOperations {
		var help bytes.Buffer
		if err := Prototype(context.Background(), []string{operation, "-h"}, &help); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%s help error = %v", operation, err)
		}
		if !strings.Contains(help.String(), commandUsage["prototype "+operation]) {
			t.Fatalf("%s help omitted its usage line:\n%s", operation, help.String())
		}
	}
}

func TestPrototypeAddHTMLCopiesAnImmutableRevisionAndReplays(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	source := newPrototypeSource(t, "<!doctype html><button id=\"buy\">Buy</button>")
	args := []string{
		"add-html", root, "--feature", testFeature, "--id", "checkout", "--revision", "r1", "--title", "Checkout",
		"--source", source, "--state", "ready", "--request-id", "checkout-request",
	}
	created := runPrototype(t, args...)
	if !created.OK || created.Replayed || created.Resource != "urn:change-saga:atomic:prototype:checkout" {
		t.Fatalf("add-html result = %#v", created)
	}
	if created.Path != testFeatureRel+"/___requirements/prototypes/checkout.prototype" {
		t.Fatalf("add-html path = %q", created.Path)
	}
	if replay := runPrototype(t, args...); !replay.Replayed {
		t.Fatalf("identical request-id did not replay: %#v", replay)
	}

	// The authored directory is not a live dependency of the revision.
	writeFile(t, filepath.Join(source, "index.html"), "edited outside the saga")
	document, err := prototypes.Load(root, "atomic")
	if err != nil {
		t.Fatalf("load prototypes: %v", err)
	}
	if len(document.Prototypes) != 1 || document.Prototypes[0].CurrentRevision == nil {
		t.Fatalf("loaded document = %#v", document)
	}
	revision := document.Prototypes[0].CurrentRevision
	if revision.Source.Kind != prototypes.SourceHTML || revision.State != prototypes.StateReady {
		t.Fatalf("revision = %#v", revision)
	}
	packaged, err := os.ReadFile(filepath.Join(testFeatureDir(root), "___requirements", "prototypes", "checkout.prototype", "revisions", "r1.revision", "html", "index.html"))
	if err != nil || !strings.Contains(string(packaged), "id=\"buy\"") {
		t.Fatalf("packaged html = %q, err = %v", packaged, err)
	}
	assertValid(t, root)
}

func TestPrototypeAuthoringDoesNotRequireAnnotationOrLinkage(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	source := newPrototypeSource(t, "<!doctype html><main id=\"explore\">Exploring</main>")
	created := runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "explore", "--revision", "r1",
		"--title", "Exploration", "--source", source)
	if !created.OK {
		t.Fatalf("unlinked prototype was rejected: %#v", created)
	}
	document, err := prototypes.Load(root, "atomic")
	if err != nil {
		t.Fatalf("load prototypes: %v", err)
	}
	if len(document.Annotations) != 0 {
		t.Fatalf("authoring invented an annotation: %#v", document.Annotations)
	}
	// An unlinked prototype is legal at authoring time; it only fails to
	// contribute to readiness, which validation does not decide.
	assertValid(t, root)

	// Story authoring must keep working beside the sibling capability root.
	var output bytes.Buffer
	if err := Story(context.Background(), []string{
		"add", root, "--feature", testFeature, "--persona", testPersonaURN, "--id", "buyer", "--revision", "s1", "--event", "proposed", "--title", "Buyer",
		"--statement", "As a buyer I can explore", "--priority", "must", "--json",
	}, &output); err != nil {
		t.Fatalf("story authoring broke beside prototypes: %v\n%s", err, output.String())
	}
}

func TestPrototypeAddExternalRequiresExplicitEmbedAllowlisting(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	reference := runPrototype(t, "add-external", root, "--feature", testFeature, "--id", "figma-link", "--revision", "r1",
		"--title", "Figma", "--url", "https://www.figma.com/file/abc")
	if !reference.OK || reference.Resource != "urn:change-saga:atomic:prototype:figma-link" {
		t.Fatalf("external reference result = %#v", reference)
	}

	embed := runPrototype(t, "add-external", root, "--feature", testFeature, "--id", "figma-embed", "--revision", "r1",
		"--title", "Embedded Figma", "--url", "https://www.figma.com/file/abc",
		"--embed-url", "https://embed.figma.com/proto/abc", "--provider", "figma",
		"--embed-origin", "https://embed.figma.com", "--sandbox", "allow-scripts",
		"--permission", "fullscreen")
	if !embed.OK {
		t.Fatalf("allowlisted embed result = %#v", embed)
	}
	document, err := prototypes.Load(root, "atomic")
	if err != nil {
		t.Fatalf("load prototypes: %v", err)
	}
	var embedded *prototypes.Revision
	for index := range document.Prototypes {
		if document.Prototypes[index].Identity.ID == "figma-embed" {
			embedded = document.Prototypes[index].CurrentRevision
		}
	}
	if embedded == nil || embedded.Source.Kind != prototypes.SourceEmbed || embedded.Source.FallbackURL != "https://www.figma.com/file/abc" {
		t.Fatalf("embed source = %#v", embedded)
	}
	if embedded.Source.Allowlist == nil || embedded.Source.Allowlist.Provider != "figma" {
		t.Fatalf("embed allowlist = %#v", embedded.Source.Allowlist)
	}

	var output bytes.Buffer
	err = Prototype(context.Background(), []string{
		"add-external", root, "--feature", testFeature, "--id", "implicit", "--revision", "r1", "--title", "Implicit",
		"--url", "https://www.figma.com/file/abc", "--embed-url", "https://embed.figma.com/proto/abc",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "explicit allowlist") {
		t.Fatalf("an unallowlisted embed must be refused, error = %v", err)
	}
	output.Reset()
	err = Prototype(context.Background(), []string{
		"add-external", root, "--feature", testFeature, "--id", "stray", "--revision", "r1", "--title", "Stray",
		"--url", "https://www.figma.com/file/abc", "--provider", "figma",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "require --embed-url") {
		t.Fatalf("allowlist flags without an embed must be refused, error = %v", err)
	}
	assertValid(t, root)
}

func TestPrototypeReviseIsAppendOnlyAndReconcilesHeads(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	source := newPrototypeSource(t, "<!doctype html><button id=\"buy\">Buy</button>")
	runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "checkout", "--revision", "r1", "--title", "Checkout", "--source", source)

	prototypeURN := "urn:change-saga:atomic:prototype:checkout"
	revised := newPrototypeSource(t, "<!doctype html><button id=\"buy\">Buy now</button>")
	result := runPrototype(t, "revise", root, "--prototype", prototypeURN, "--revision", "r2",
		"--parent", prototypeURN+":revision:r1", "--title", "Checkout", "--state", "ready", "--source", revised)
	if !result.OK || result.Resource != prototypeURN+":revision:r2" {
		t.Fatalf("revise result = %#v", result)
	}

	document, err := prototypes.Load(root, "atomic")
	if err != nil {
		t.Fatalf("load prototypes: %v", err)
	}
	if len(document.Prototypes[0].Revisions) != 2 || document.Prototypes[0].CurrentRevision.ID != "r2" {
		t.Fatalf("revision graph = %#v", document.Prototypes[0])
	}
	original, err := os.ReadFile(filepath.Join(testFeatureDir(root), "___requirements", "prototypes", "checkout.prototype", "revisions", "r1.revision", "html", "index.html"))
	if err != nil || strings.Contains(string(original), "Buy now") {
		t.Fatalf("the earlier revision was not immutable: %q, err = %v", original, err)
	}

	var output bytes.Buffer
	err = Prototype(context.Background(), []string{
		"revise", root, "--prototype", prototypeURN, "--revision", "r3", "--title", "Checkout",
		"--parent", prototypeURN + ":revision:r1", "--source", revised,
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "every current head") {
		t.Fatalf("a stale parent must be refused, error = %v", err)
	}
	output.Reset()
	err = Prototype(context.Background(), []string{
		"revise", root, "--prototype", prototypeURN, "--revision", "r3", "--title", "Checkout",
		"--parent", prototypeURN + ":revision:r2", "--source", revised, "--url", "https://example.test/p",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "exactly one revised source") {
		t.Fatalf("two revised sources must be refused, error = %v", err)
	}
	assertValid(t, root)
}

func TestPrototypeAnnotatePinsSelectorsToStoriesAndCriteria(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	source := newPrototypeSource(t, "<!doctype html><button id=\"buy\">Buy</button>")
	runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "checkout", "--revision", "r1", "--title", "Checkout", "--source", source)

	var output bytes.Buffer
	if err := Story(context.Background(), []string{
		"add", root, "--feature", testFeature, "--persona", testPersonaURN, "--id", "buyer", "--revision", "s1", "--event", "proposed", "--title", "Buyer",
		"--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--json",
	}, &output); err != nil {
		t.Fatalf("story fixture: %v\n%s", err, output.String())
	}

	prototypeURN := "urn:change-saga:atomic:prototype:checkout"
	storyURN := "urn:change-saga:atomic:story:buyer"
	storyRevision := storyURN + ":revision:s1"
	element := runPrototype(t, "annotate", root, "--prototype", prototypeURN, "--id", "buy-button",
		"--target", storyURN, "--rationale", "The buy button is the primary action.",
		"--prototype-revision", prototypeURN+":revision:r1", "--story-revision", storyRevision,
		"--element-id", "buy")
	if !element.OK || element.Resource != prototypeURN+":annotation:buy-button" {
		t.Fatalf("element annotation result = %#v", element)
	}
	region := runPrototype(t, "annotate", root, "--prototype", prototypeURN, "--id", "summary-area",
		"--target", storyURN+":criterion:fast", "--rationale", "The order summary shows the promptness.",
		"--prototype-revision", prototypeURN+":revision:r1", "--story-revision", storyRevision,
		"--region", "0.1,0.2,0.3,0.4")
	if !region.OK {
		t.Fatalf("region annotation result = %#v", region)
	}

	document, err := prototypes.Load(root, "atomic")
	if err != nil {
		t.Fatalf("load prototypes: %v", err)
	}
	if len(document.Annotations) != 2 {
		t.Fatalf("annotations = %#v", document.Annotations)
	}
	kinds := map[prototypes.SelectorKind]bool{}
	for _, annotation := range document.Annotations {
		kinds[annotation.Selector.Kind] = true
	}
	if !kinds[prototypes.SelectorElement] || !kinds[prototypes.SelectorRegion] {
		t.Fatalf("selector kinds = %#v", kinds)
	}

	output.Reset()
	err = Prototype(context.Background(), []string{
		"annotate", root, "--prototype", prototypeURN, "--id", "ambiguous", "--target", storyURN,
		"--rationale", "Ambiguous.", "--prototype-revision", prototypeURN + ":revision:r1",
		"--story-revision", storyRevision, "--element-id", "buy", "--text", "Buy",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "exactly one selector") {
		t.Fatalf("two selectors must be refused, error = %v", err)
	}
	output.Reset()
	err = Prototype(context.Background(), []string{
		"annotate", root, "--prototype", prototypeURN, "--id", "unpinned", "--target", storyURN,
		"--rationale", "Unpinned.", "--story-revision", storyRevision, "--element-id", "buy",
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "exactly one prototype pin") {
		t.Fatalf("a missing prototype pin must be refused, error = %v", err)
	}
	assertValid(t, root)
}

func TestPrototypeMutationFailureReportsJSONAndExitStatus(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	var output bytes.Buffer
	err := Prototype(context.Background(), []string{
		"add-html", root, "--feature", testFeature, "--id", "checkout", "--revision", "r1", "--title", "Checkout",
		"--source", filepath.Join(t.TempDir(), "missing"), "--json",
	}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("failure status = %v", err)
	}
	var failure livingMutationOutput
	if decodeErr := json.Unmarshal(output.Bytes(), &failure); decodeErr != nil {
		t.Fatalf("decode failure output %q: %v", output.String(), decodeErr)
	}
	if failure.OK || failure.Operation != "prototype add-html" || failure.Error == nil {
		t.Fatalf("failure output = %#v", failure)
	}
}

func TestValidateCoversPrototypeRecords(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	source := newPrototypeSource(t, "<!doctype html><button id=\"buy\">Buy</button>")
	runPrototype(t, "add-html", root, "--feature", testFeature, "--id", "checkout", "--revision", "r1", "--title", "Checkout", "--source", source)

	var output bytes.Buffer
	if err := Validate(context.Background(), []string{"--json", root}, &output); err != nil {
		t.Fatalf("validate a sound prototype: %v\n%s", err, output.String())
	}

	packaged := filepath.Join(testFeatureDir(root), "___requirements", "prototypes", "checkout.prototype", "revisions", "r1.revision", "html", "index.html")
	writeFile(t, packaged, "tampered after the revision was sealed")
	output.Reset()
	err := Validate(context.Background(), []string{"--json", root}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("tampered prototype validate error = %v", err)
	}
	var result validationOutput
	if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
		t.Fatalf("decode validation %q: %v", output.String(), decodeErr)
	}
	if result.Valid {
		t.Fatalf("a tampered prototype revision stayed valid: %#v", result)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Severity == "error" && issue.Path == "___requirements/prototypes" && strings.Contains(issue.Message, "digest mismatch") {
			found = true
		}
	}
	if !found {
		t.Fatalf("validation did not report the prototype record: %#v", result.Issues)
	}
}

// A repository prototype stays in the repository: the Saga pins its
// entrypoint and the directory beside it at a commit, refuses uncommitted
// files, and records a later change only through refresh.
func TestRepoPrototypePinsCommittedFilesAndRefreshesAfterTheyChange(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	writeFile(t, filepath.Join(repo, "prototypes", "sidebar", "index.html"), `<link rel="stylesheet" href="app.css"><h1 id="go">Go</h1>`)
	writeFile(t, filepath.Join(repo, "prototypes", "sidebar", "app.css"), `h1{color:red}`)
	add := []string{"add-repo", root, "--feature", testFeature, "--id", "sidebar", "--revision", "r1", "--title", "Sidebar", "--path", "prototypes/sidebar/index.html", "--repo", repo}
	var refused bytes.Buffer
	if err := Prototype(context.Background(), add, &refused); err == nil || !strings.Contains(err.Error(), "commit the prototype first") {
		t.Fatalf("an uncommitted prototype = %v", err)
	}
	first := commitAll(t, repo, "add the sidebar prototype")
	runPrototype(t, add...)

	document, err := prototypes.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	source := document.Find("sidebar").CurrentRevision.Source
	if source.Kind != prototypes.SourceRepo || source.Entrypoint != "prototypes/sidebar/index.html" || source.Root != "prototypes/sidebar" || source.Commit != first || len(source.Object) != 40 {
		t.Fatalf("repo source = %#v", source)
	}
	if dir, _ := document.RevisionDir(*document.Find("sidebar"), "r1"); func() bool { _, err := os.Stat(filepath.Join(dir, "html")); return err == nil }() {
		t.Fatal("a repository prototype must not copy its files into the Saga")
	}
	if stale := prototypes.RepoStaleness(context.Background(), repo, source, "HEAD"); stale != "" {
		t.Fatalf("a fresh pin is stale: %s", stale)
	}
	if data, ok := prototypes.ReadRepoFile(context.Background(), repo, source, "app.css"); !ok || string(data) != `h1{color:red}` {
		t.Fatalf("pinned asset = %q %v", data, ok)
	}

	refresh := []string{"refresh", root, "--prototype", "urn:change-saga:batch:prototype:sidebar", "--revision", "r2", "--repo", repo}
	if err := Prototype(context.Background(), refresh, &refused); err == nil || !strings.Contains(err.Error(), "nothing changed") {
		t.Fatalf("refreshing an unchanged prototype = %v", err)
	}
	writeFile(t, filepath.Join(repo, "prototypes", "sidebar", "app.css"), `h1{color:blue}`)
	second := commitAll(t, repo, "restyle the sidebar prototype")
	if stale := prototypes.RepoStaleness(context.Background(), repo, source, "HEAD"); !strings.Contains(stale, "changed since") {
		t.Fatalf("a changed package must read as stale: %q", stale)
	}
	runPrototype(t, refresh...)
	document, err = prototypes.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	head := document.Find("sidebar").CurrentRevision
	if head.ID != "r2" || head.Source.Commit != second || head.Title != "Sidebar" || len(head.Parents) != 1 {
		t.Fatalf("refreshed revision = %#v", head)
	}
	// The first revision still shows what it pinned.
	if data, ok := prototypes.ReadRepoFile(context.Background(), repo, source, "app.css"); !ok || string(data) != `h1{color:red}` {
		t.Fatalf("r1 must still read its pinned asset, got %q", data)
	}
	assertValid(t, root)
}

// A prototype file at the repository root is served alone, so its pin never
// covers the whole repository.
func TestRepoPrototypeAtTheRootPinsTheFileAlone(t *testing.T) {
	t.Parallel()
	root, repo := coveredSaga(t)
	writeFile(t, filepath.Join(repo, "sidebar-prototype.html"), `<h1>Go</h1>`)
	commitAll(t, repo, "add a root prototype")
	runPrototype(t, "add-repo", root, "--feature", testFeature, "--id", "root-sidebar", "--revision", "r1", "--title", "Sidebar", "--path", "sidebar-prototype.html", "--repo", repo)
	document, err := prototypes.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	source := document.Find("root-sidebar").CurrentRevision.Source
	if source.Root != "" || source.RepoPackage() != "sidebar-prototype.html" || source.RepoEntry() != "sidebar-prototype.html" {
		t.Fatalf("root prototype source = %#v", source)
	}
	if _, ok := source.RepoFile("README.md"); ok {
		t.Fatal("a single-file prototype must serve nothing but its entrypoint")
	}
}
