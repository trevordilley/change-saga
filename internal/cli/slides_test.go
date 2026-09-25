package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestQualifiedDiagramIDs(t *testing.T) {
	t.Parallel()
	root := newAuthoredSaga(t)
	var output bytes.Buffer
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--feature-qualified-id", "--objective", "Explain the flow.", root, "arch"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "core--arch", "--feature-qualified-id", "--intent", "explain", "--layout", "diagram", root, "flow"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "core--flow", "--kind", "node", "--element-id", "slide-title", "--description", "The request entry point.", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--id", "legacy-deck", "--objective", "Keep caller identity.", root, "legacy"}, &output); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	generated := findDeck(document, "core--arch")
	if generated == nil || generated.ID != "core--arch" || len(generated.Slides) != 1 || generated.Slides[0].ID != "core--flow" || generated.Slides[0].Items[0].ID != "slide-title" {
		t.Fatalf("generated diagram identities = %#v", generated)
	}
	if explicit := findDeck(document, "legacy-deck"); explicit == nil || explicit.ID != "legacy-deck" {
		t.Fatalf("explicit id was changed: %#v", explicit)
	}
}

func TestImplementationDeckAuthoringLoop(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "base")

	root := filepath.Join(t.TempDir(), "visual.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--title", "Visual", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--role", "overview", "--objective", "Explain the change.", root, "implementation"}, &output); err == nil || !strings.Contains(err.Error(), "--role must be change") {
		t.Fatalf("overview deck role was not refused: %v", err)
	}
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Explain the change.", root, "implementation"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "orient", "--layout", "hero", "--entrypoint", "assets/slide.svg", root, "nested-source"}, &output); err == nil || !strings.Contains(err.Error(), "simple filename") {
		t.Fatalf("nested entrypoint was not refused clearly: %v", err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--section", "Overview", "--intent", "orient", "--layout", "hero", "--title", "What changed", "--takeaway", "Validation now happens first.", root, "change-overview"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "change-overview", "--kind", "callout", "--id", "premise", "--element-id", "slide-title", "--label", "Review premise", "--description", "The high-level behavioral change.", "--body", "Invalid requests never reach persistence.", "--placement", "right", "--leader", "arrow", root}, &output); err != nil {
		t.Fatal(err)
	}
	interactive := filepath.Join(t.TempDir(), "interactive.html")
	writeFile(t, interactive, `<main id="decision">Choose a path</main><script>document.querySelector('#decision').dataset.ready = 'true'</script>`)
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "compare", "--layout", "before-after", "--rank", "1", "--title", "Interactive comparison", "--takeaway", "The alternate path remains inspectable.", "--media-type", "text/html", "--entrypoint", "index.html", "--source", interactive, root, "interactive-comparison"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "interactive-comparison", "--kind", "statement", "--id", "decision", "--element-id", "decision", "--description", "The alternate path decision.", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("authored deck invalid: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	if _, err := os.Stat(filepath.Join(document.Decks[0].Slides[1].Directory, document.Decks[0].Slides[1].Entrypoint)); err != nil {
		t.Fatalf("compact slide asset was not written: %v", err)
	}
	if document.Decks[0].Slides[0].Section != "Overview" {
		t.Fatalf("slide section = %q, want Overview", document.Decks[0].Slides[0].Section)
	}
	entries, err := os.ReadDir(document.Decks[0].Directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) > saga.FlatMaxBasename {
			t.Fatalf("deck bundle is not compact and flat: %s", entry.Name())
		}
	}
	item := document.Decks[0].Slides[0].Items[0]
	uri := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD")) + ":README.md#L1"
	if err := Cover(context.Background(), []string{"--against", "main", "--repo", repo, "--target", document.Decks[0].Slides[0].Target, "--ref", uri, root}, &output); err == nil || !strings.Contains(err.Error(), "must target an Item") {
		t.Fatalf("slide-level coverage was not refused: %v", err)
	}
	if err := Cover(context.Background(), []string{"--against", "main", "--repo", repo, "--target", item.Target, "--ref", uri, root}, &output); err != nil {
		t.Fatalf("item coverage failed: %v", err)
	}
	if err := AddClaim(context.Background(), []string{"--id", "validation-boundary", "--target", item.Target, "--statement", "Validation runs before persistence.", "--repo", repo, "--ref", uri, root}, &output); err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if err := VerifyClaim(context.Background(), []string{"--id", "validation-check", "--claim", "validation-boundary", "--status", "verified", "--method", "inspection", "--summary", "The mapped line establishes the boundary.", root}, &output); err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || len(document.Claims) != 1 || len(document.Verifications) != 1 {
		t.Fatalf("claim or verification was not preserved: valid=%v err=%v slide=%#v", validation.Valid, err, document.Decks[0].Slides[0])
	}
	var queryOutput bytes.Buffer
	if err := Query(context.Background(), []string{"slide", "--saga", root, "--repo", repo, "--against", "main", "--target", document.Decks[0].Slides[0].Target}, &queryOutput); err != nil {
		t.Fatalf("slide query failed: %v\n%s", err, queryOutput.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if envelope["schema"] != querySchema || data["items"] == nil || data["landmarks"] != nil || data["section"] != "Overview" || data["takeaway"] != "Validation now happens first." {
		t.Fatalf("slide query leaked report vocabulary or metadata: %#v", envelope)
	}
	if !strings.Contains(output.String(), "change-saga cover") {
		t.Fatalf("authoring guidance did not lead to Item evidence:\n%s", output.String())
	}
}

func TestSagaEmbedsSeveralIndependentSlideDecks(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	writeFile(t, filepath.Join(repo, "README.md"), "base\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "base")
	git(t, repo, "checkout", "-b", "feature")
	writeFile(t, filepath.Join(repo, "README.md"), "implemented\n")
	git(t, repo, "add", "README.md")
	git(t, repo, "commit", "-m", "implement change")

	root := filepath.Join(t.TempDir(), "decks.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--title", "Decks", root}, &output); err != nil {
		t.Fatal(err)
	}
	personaURN := addTestApp(t, root)
	for _, deck := range []string{"flow", "failure-path"} {
		if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Explain " + deck + " to reviewers.", root, deck}, &output); err != nil {
			t.Fatalf("add embedded deck %s: %v", deck, err)
		}
		if err := AddSlide(context.Background(), []string{"--deck", deck, "--intent", "explain", "--layout", "diagram", "--title", deck, "--takeaway", "The complex behavior is explicit.", root, deck + "-change"}, &output); err != nil {
			t.Fatalf("add embedded slide %s: %v", deck, err)
		}
		if err := AddItem(context.Background(), []string{"--slide", deck + "-change", "--kind", "callout", "--id", "surprise", "--element-id", "slide-title", "--description", "The reviewer surprise for this change.", "--body", "The implementation takes the non-obvious path.", root}, &output); err != nil {
			t.Fatalf("add embedded item %s: %v", deck, err)
		}
	}

	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("deck load: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	if document.Manifest.Version != saga.SagaVersion || len(document.Decks) != 2 || len(document.Section.Fragments) != 1 {
		t.Fatalf("report and deck surfaces did not coexist: manifest=%#v decks=%d overview=%d", document.Manifest, len(document.Decks), len(document.Section.Fragments))
	}
	for _, deck := range document.Decks {
		if !strings.HasPrefix(deck.Path, testFeatureRel+"/"+saga.EmbeddedSlidesDir+"/") || deck.Target != saga.DeckTarget(document.Manifest.ID, deck.ID) {
			t.Fatalf("embedded deck lost parent identity or independent path: %#v", deck)
		}
	}
	item := document.Decks[0].Slides[0].Items[0]
	changes, err := gitdiff.Read(context.Background(), repo, document.Manifest.Source.Repository, "main", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	uri := ""
	for _, atom := range changes.Atoms {
		if atom.Path == "README.md" && atom.Side == "new" {
			uri = atom.Ref
			break
		}
	}
	if uri == "" {
		t.Fatalf("fixture has no changed README line: %#v", changes.Atoms)
	}
	if err := Cover(context.Background(), []string{"--against", "main", "--repo", repo, "--target", item.Target, "--ref", uri, root}, &output); err != nil {
		t.Fatalf("embedded Item coverage failed: %v", err)
	}
	var traceOutput bytes.Buffer
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--against", "main", "--ref", uri}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), `"unlinked_code_evidence":[{"deck":`) {
		t.Fatalf("unlinked embedded evidence was not exposed: err=%v\n%s", err, traceOutput.String())
	}
	if err := Story(context.Background(), []string{"add", "--feature", testFeature, "--persona", personaURN, "--id", "checkout", "--revision", "r1", "--event", "proposed", "--title", "Checkout", "--statement", "As a buyer I can complete checkout", "--priority", "high", "--criterion", "safe=Checkout preserves the validated state", "--criterion", "fast=Checkout does not add a retry delay", root}, &output); err != nil {
		t.Fatalf("add linked story: %v", err)
	}
	storyURN, _ := livingid.Story("decks", "checkout")
	criterionURN, _ := livingid.Criterion("decks", "checkout", "safe")
	secondCriterionURN, _ := livingid.Criterion("decks", "checkout", "fast")
	revisionURN, _ := livingid.Revision("decks", "checkout", "r1")
	proposedURN, _ := requirements.StoryEventURN("decks", "checkout", "proposed")
	if err := Relation(context.Background(), []string{"add", "--feature", testFeature, "--id", "flow-explains-checkout", "--type", "explains", "--from", item.Target, "--to", storyURN, "--to-revision", revisionURN, "--rationale", "The visual implementation element demonstrates this user story.", root}, &output); err != nil {
		t.Fatalf("link Item to story: %v", err)
	}
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--against", "main", "--ref", uri}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), `"unlinked_code_evidence":[{"deck":`) {
		t.Fatalf("evidence linked only to an unaccepted story was incorrectly closed: err=%v\n%s", err, traceOutput.String())
	}
	if err := Story(context.Background(), []string{"set-state", "--story", storyURN, "--event", "accepted", "--parent", proposedURN, "--state", "accepted", root}, &output); err != nil {
		t.Fatalf("accept linked story: %v", err)
	}
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--against", "main", "--ref", uri}, &traceOutput); err != nil {
		t.Fatalf("reverse requirement trace: %v\n%s", err, traceOutput.String())
	}
	for _, want := range []string{storyURN, criterionURN, secondCriterionURN, document.Decks[0].Slides[0].Target, item.Target, uri, `"unlinked_code_evidence":[]`} {
		if !strings.Contains(traceOutput.String(), want) {
			t.Fatalf("reverse requirement trace omitted %q:\n%s", want, traceOutput.String())
		}
	}
	headCommit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--against", "main", "--commit", headCommit}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), storyURN) || !strings.Contains(traceOutput.String(), item.Target) {
		t.Fatalf("comparison-head requirement trace failed for %q: err=%v\n%s", headCommit, err, traceOutput.String())
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || len(document.Decks[0].Slides[0].Items[0].Code) != 1 {
		t.Fatalf("embedded Item evidence did not round-trip: valid=%v err=%v", validation.Valid, err)
	}
	if loaded := document.Decks[0].Slides[0].Items[0].Code[0].References[0].Location().String(); loaded != uri {
		t.Fatalf("embedded Item evidence changed identity: got %q want %q", loaded, uri)
	}
	resolver, err := coderesolve.New(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	if report := coverage.Evaluate(context.Background(), document, validation, changes, resolver); len(report.StaleReferences) != 0 || len(report.Ownership) == 0 {
		t.Fatalf("embedded Item evidence did not match the source comparison: stale=%#v ownership=%#v", report.StaleReferences, report.Ownership)
	}

	var queryOutput bytes.Buffer
	if err := Query(context.Background(), []string{"slide-diffs", "--saga", root, "--repo", repo, "--against", "main", "--target", item.Target}, &queryOutput); err != nil || !strings.Contains(queryOutput.String(), item.Target) || !strings.Contains(queryOutput.String(), "README.md") {
		t.Fatalf("embedded Item did not trace to its code: err=%v\n%s", err, queryOutput.String())
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"diff-owners", "--saga", root, "--repo", repo, "--against", "main", "--ref", uri}, &queryOutput); err != nil || !strings.Contains(queryOutput.String(), item.Target) {
		t.Fatalf("code did not trace back to its embedded Item: err=%v\n%s", err, queryOutput.String())
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"overview", "--saga", root, "--repo", repo}, &queryOutput); err != nil {
		t.Fatalf("overview query: %v\n%s", err, queryOutput.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if envelope["schema"] != querySchema || data["overview_fragments"] == nil || data["decks"] == nil {
		t.Fatalf("overview did not expose both the report and its decks: %#v", envelope)
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"slide", "--saga", root, "--repo", repo, "--against", "main", "--target", document.Decks[0].Slides[0].Target}, &queryOutput); err != nil {
		t.Fatalf("embedded slide query: %v\n%s", err, queryOutput.String())
	}
	envelope = map[string]any{}
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ = envelope["data"].(map[string]any)
	if envelope["schema"] != querySchema || data["items"] == nil || data["landmarks"] != nil {
		t.Fatalf("embedded slide query leaked report semantics: %#v", envelope)
	}
}
