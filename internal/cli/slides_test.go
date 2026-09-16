package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/diffuri"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestSlideNativeAuthoringLoopAndCompatibilityRefusal(t *testing.T) {
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
	if err := Init(context.Background(), []string{"--mode", "slides", "--repo", repo, "--base", "main", "--head", "HEAD", "--title", "Visual", root}, &output); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := os.ReadFile(filepath.Join(root, "01-readme.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"surprise inventory", "reasonable reviewer expectation", "callout Items", "Do not manufacture novelty"} {
		if !strings.Contains(string(bootstrap), expected) {
			t.Errorf("slide-native bootstrap omitted %q", expected)
		}
	}
	if err := AddSlide(context.Background(), []string{"--deck", "overview", "--intent", "orient", "--layout", "hero", "--entrypoint", "assets/slide.svg", root, "nested-source"}, &output); err == nil || !strings.Contains(err.Error(), "simple filename") {
		t.Fatalf("nested v4 entrypoint was not refused clearly: %v", err)
	}
	if err := AddSlide(context.Background(), []string{"--deck", "overview", "--intent", "orient", "--layout", "hero", "--title", "What changed", "--takeaway", "Validation now happens first.", root, "change-overview"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "change-overview", "--kind", "callout", "--id", "premise", "--element-id", "slide-title", "--label", "Review premise", "--description", "The high-level behavioral change.", "--body", "Invalid requests never reach persistence.", "--placement", "right", "--leader", "arrow", root}, &output); err != nil {
		t.Fatal(err)
	}
	interactive := filepath.Join(t.TempDir(), "interactive.html")
	writeFile(t, interactive, `<main id="decision">Choose a path</main><script>document.querySelector('#decision').dataset.ready = 'true'</script>`)
	if err := AddSlide(context.Background(), []string{"--deck", "overview", "--intent", "compare", "--layout", "before-after", "--rank", "1", "--title", "Interactive comparison", "--takeaway", "The alternate path remains inspectable.", "--media-type", "text/html", "--entrypoint", "index.html", "--source", interactive, root, "interactive-comparison"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "interactive-comparison", "--kind", "statement", "--id", "decision", "--element-id", "decision", "--description", "The alternate path decision.", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("authored v4 invalid: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	if _, err := os.Stat(filepath.Join(root, document.Decks[0].Slides[1].Entrypoint)); err != nil {
		t.Fatalf("compact slide asset was not written: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || len(entry.Name()) > saga.FlatMaxBasename {
			t.Fatalf("v4 output is not compact and flat: %s", entry.Name())
		}
	}
	item := document.Decks[0].Slides[0].Items[0]
	uri, err := diffuri.Build(diffuri.Reference{Repository: document.Manifest.Source.Repository, Base: "main", Head: "HEAD", Kind: "line", Path: "README.md", Side: "new", Start: 1, End: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := Cover(context.Background(), []string{"--target", document.Decks[0].Slides[0].Target, "--uri", uri, root}, &output); err == nil || !strings.Contains(err.Error(), "must target an Item") {
		t.Fatalf("slide-level coverage was not refused: %v", err)
	}
	if err := Cover(context.Background(), []string{"--target", item.Target, "--uri", uri, root}, &output); err != nil {
		t.Fatalf("item coverage failed: %v", err)
	}
	if err := Review(context.Background(), []string{"--target", item.Path, "--state", "approved", "--reviewer-kind", "human", root}, &output); err == nil || !strings.Contains(err.Error(), "must target a slide") {
		t.Fatalf("Item approval was not explicitly refused: %v", err)
	}
	if err := Review(context.Background(), []string{"--target", document.Decks[0].Slides[0].Path, "--state", "approved", "--reviewer-kind", "human", "--body", "Visual argument checked.", root}, &output); err != nil {
		t.Fatalf("slide review by compact record path failed: %v", err)
	}
	if err := Thread(context.Background(), []string{"--target", item.Target, "--body", "Keep the validation boundary visible.", root}, &output); err != nil {
		t.Fatalf("flat thread failed: %v", err)
	}
	if err := AddClaim(context.Background(), []string{"--id", "validation-boundary", "--target", item.Target, "--statement", "Validation runs before persistence.", "--diff", uri, root}, &output); err != nil {
		t.Fatalf("flat claim failed: %v", err)
	}
	if err := VerifyClaim(context.Background(), []string{"--id", "validation-check", "--claim", "validation-boundary", "--status", "verified", "--method", "inspection", "--summary", "The mapped line establishes the boundary.", root}, &output); err != nil {
		t.Fatalf("flat verification failed: %v", err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || len(document.Decks[0].Slides[0].Reviews) != 1 || len(document.Decks[0].Slides[0].Items[0].Reviews) != 0 || len(document.Threads) != 1 || len(document.Threads[0].Messages) != 1 || len(document.Claims) != 1 || len(document.Verifications) != 1 {
		t.Fatalf("slide approval or Item comment was not preserved: valid=%v err=%v slide=%#v", validation.Valid, err, document.Decks[0].Slides[0])
	}
	var queryOutput bytes.Buffer
	if err := Query(context.Background(), []string{"slide", "--saga", root, "--repo", repo, "--target", document.Decks[0].Slides[0].Target}, &queryOutput); err != nil {
		t.Fatalf("slide query failed: %v\n%s", err, queryOutput.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if envelope["schema"] != slideQuerySchema || data["items"] == nil || data["landmarks"] != nil || data["takeaway"] != "Validation now happens first." {
		t.Fatalf("slide query leaked report vocabulary or metadata: %#v", envelope)
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"overview", "--saga", root, "--repo", repo}, &queryOutput); err != nil {
		t.Fatalf("v4 overview query failed: %v\n%s", err, queryOutput.String())
	}
	envelope = map[string]any{}
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ = envelope["data"].(map[string]any)
	if envelope["schema"] != slideQuerySchema || data["decks"] == nil || data["chapters"] != nil || data["overview_fragments"] != nil {
		t.Fatalf("v4 overview leaked report hierarchy: %#v", envelope)
	}
	if err := AddChapter(context.Background(), []string{root, "legacy"}, &output); err == nil || !strings.Contains(err.Error(), "use add-deck") {
		t.Fatalf("legacy authoring was not refused: %v", err)
	}
	if !strings.Contains(output.String(), "change-saga cover") {
		t.Fatalf("authoring guidance did not lead to Item evidence:\n%s", output.String())
	}
}

func TestReportSagaEmbedsSeveralIndependentSlideDecks(t *testing.T) {
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

	root := filepath.Join(t.TempDir(), "hybrid.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--base", "main", "--head", "HEAD", "--title", "Hybrid", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(context.Background(), []string{"--objective", "Explain the complex flow.", root, "flow"}, &output); err == nil || !strings.Contains(err.Error(), "v3 Report Saga") {
		t.Fatalf("v2 unexpectedly accepted embedded decks: %v", err)
	}
	if err := Upgrade(context.Background(), []string{"--to", "3", root}, &output); err != nil {
		t.Fatal(err)
	}
	for _, deck := range []string{"flow", "failure-path"} {
		if err := AddDeck(context.Background(), []string{"--objective", "Explain " + deck + " to reviewers.", root, deck}, &output); err != nil {
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
		t.Fatalf("hybrid load: valid=%v err=%v issues=%#v", validation.Valid, err, validation.Issues)
	}
	if document.Manifest.Version != saga.CurrentSagaVersion || len(document.Decks) != 2 || len(document.Section.Fragments) != 1 {
		t.Fatalf("report and deck surfaces did not coexist: manifest=%#v decks=%d overview=%d", document.Manifest, len(document.Decks), len(document.Section.Fragments))
	}
	for _, deck := range document.Decks {
		if !strings.HasPrefix(deck.Path, saga.EmbeddedSlidesDir+"/") || deck.Target != saga.DeckTarget(document.Manifest.ID, deck.ID) {
			t.Fatalf("embedded deck lost parent identity or independent path: %#v", deck)
		}
	}
	item := document.Decks[0].Slides[0].Items[0]
	changes, err := gitdiff.Read(context.Background(), repo, document.Manifest.Source.Repository, document.Manifest.Source.Base, document.Manifest.Source.Head)
	if err != nil {
		t.Fatal(err)
	}
	uri := ""
	for _, atom := range changes.Atoms {
		if atom.Path == "README.md" && atom.Side == "new" {
			uri = atom.URI
			break
		}
	}
	if uri == "" {
		t.Fatalf("fixture has no changed README line: %#v", changes.Atoms)
	}
	if err := Cover(context.Background(), []string{"--target", item.Target, "--uri", uri, root}, &output); err != nil {
		t.Fatalf("embedded Item coverage failed: %v", err)
	}
	var traceOutput bytes.Buffer
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--diff", uri}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), `"unlinked_code_evidence":[{"deck":`) {
		t.Fatalf("unlinked embedded evidence was not exposed: err=%v\n%s", err, traceOutput.String())
	}
	if err := Story(context.Background(), []string{"add", "--id", "checkout", "--revision", "r1", "--event", "proposed", "--title", "Checkout", "--statement", "As a buyer I can complete checkout", "--priority", "high", "--criterion", "safe=Checkout preserves the validated state", "--criterion", "fast=Checkout does not add a retry delay", root}, &output); err != nil {
		t.Fatalf("add linked story: %v", err)
	}
	storyURN, _ := livingid.Story("hybrid", "checkout")
	criterionURN, _ := livingid.Criterion("hybrid", "checkout", "safe")
	secondCriterionURN, _ := livingid.Criterion("hybrid", "checkout", "fast")
	revisionURN, _ := livingid.Revision("hybrid", "checkout", "r1")
	proposedURN, _ := requirements.StoryEventURN("hybrid", "checkout", "proposed")
	if err := Relation(context.Background(), []string{"add", "--id", "flow-explains-checkout", "--type", "explains", "--from", document.Decks[0].Slides[0].Target, "--to", storyURN, "--to-revision", revisionURN, "--rationale", "The visual implementation breakdown demonstrates this user story.", root}, &output); err != nil {
		t.Fatalf("link slide to story: %v", err)
	}
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--diff", uri}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), `"unlinked_code_evidence":[{"deck":`) {
		t.Fatalf("evidence linked only to an unaccepted story was incorrectly closed: err=%v\n%s", err, traceOutput.String())
	}
	if err := Story(context.Background(), []string{"set-state", "--story", storyURN, "--event", "accepted", "--parent", proposedURN, "--state", "accepted", root}, &output); err != nil {
		t.Fatalf("accept linked story: %v", err)
	}
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--diff", uri}, &traceOutput); err != nil {
		t.Fatalf("reverse requirement trace: %v\n%s", err, traceOutput.String())
	}
	for _, want := range []string{storyURN, criterionURN, secondCriterionURN, document.Decks[0].Slides[0].Target, item.Target, uri, `"unlinked_code_evidence":[]`} {
		if !strings.Contains(traceOutput.String(), want) {
			t.Fatalf("reverse requirement trace omitted %q:\n%s", want, traceOutput.String())
		}
	}
	headCommit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	traceOutput.Reset()
	if err := Query(context.Background(), []string{"traceability", "--saga", root, "--repo", repo, "--commit", headCommit}, &traceOutput); err != nil || !strings.Contains(traceOutput.String(), storyURN) || !strings.Contains(traceOutput.String(), item.Target) {
		t.Fatalf("comparison-head requirement trace failed for %q: err=%v\n%s", headCommit, err, traceOutput.String())
	}
	if err := Review(context.Background(), []string{"--target", document.Decks[0].Slides[0].Path, "--state", "approved", "--reviewer-kind", "human", root}, &output); err != nil {
		t.Fatalf("embedded slide review failed: %v", err)
	}
	if err := Thread(context.Background(), []string{"--target", item.Target, "--body", "Keep the surprise explicit.", root}, &output); err != nil {
		t.Fatalf("embedded Item thread failed: %v", err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || len(document.Decks[0].Slides[0].Items[0].Diffs) != 1 || len(document.Decks[0].Slides[0].Reviews) != 1 || len(document.Threads) != 1 {
		t.Fatalf("embedded Item evidence/review did not round-trip: valid=%v err=%v", validation.Valid, err)
	}
	if loadedURI := document.Decks[0].Slides[0].Items[0].Diffs[0].Diffs[0].URI; loadedURI != uri {
		t.Fatalf("embedded Item evidence changed identity: got %q want %q", loadedURI, uri)
	}
	if report := coverage.Evaluate(document, validation, changes); len(report.Orphans) != 0 {
		t.Fatalf("embedded Item evidence did not match the source comparison: %#v", report.Orphans)
	}

	var queryOutput bytes.Buffer
	if err := Query(context.Background(), []string{"slide-diffs", "--saga", root, "--repo", repo, "--target", item.Target}, &queryOutput); err != nil || !strings.Contains(queryOutput.String(), item.Target) || !strings.Contains(queryOutput.String(), "README.md") {
		t.Fatalf("embedded Item did not trace to its code: err=%v\n%s", err, queryOutput.String())
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"diff-owners", "--saga", root, "--repo", repo, "--diff", uri}, &queryOutput); err != nil || !strings.Contains(queryOutput.String(), item.Target) {
		t.Fatalf("code did not trace back to its embedded Item: err=%v\n%s", err, queryOutput.String())
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"overview", "--saga", root, "--repo", repo}, &queryOutput); err != nil {
		t.Fatalf("hybrid overview query: %v\n%s", err, queryOutput.String())
	}
	var envelope map[string]any
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ := envelope["data"].(map[string]any)
	if envelope["schema"] != querySchema || data["overview_fragments"] == nil || data["decks"] == nil {
		t.Fatalf("hybrid overview did not expose both native surfaces: %#v", envelope)
	}
	queryOutput.Reset()
	if err := Query(context.Background(), []string{"slide", "--saga", root, "--repo", repo, "--target", document.Decks[0].Slides[0].Target}, &queryOutput); err != nil {
		t.Fatalf("embedded slide query: %v\n%s", err, queryOutput.String())
	}
	envelope = map[string]any{}
	if err := json.Unmarshal(queryOutput.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data, _ = envelope["data"].(map[string]any)
	if envelope["schema"] != slideQuerySchema || data["items"] == nil || data["landmarks"] != nil {
		t.Fatalf("embedded slide query leaked report semantics: %#v", envelope)
	}
}
