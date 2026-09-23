package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

func newSlideTransactionFixture(t *testing.T) (root, repo, base, commit, sagaID string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	writeFile(t, filepath.Join(repo, "service.go"), "package service\n\nfunc Run() error { return nil }\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "base")
	commit = strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

	root = filepath.Join(shortTempDir(t), "transaction.saga")
	var output bytes.Buffer
	if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
		t.Fatal(err)
	}
	addTestApp(t, root)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	sagaID = manifest.ID
	if err := Story(context.Background(), []string{
		"add", root, "--feature", testFeature, "--persona", personaURNFor(sagaID), "--id", "run", "--revision", "r1", "--event", "proposed",
		"--title", "Run safely", "--statement", "As a user I run the service", "--criterion", "returns=Run returns without error", "--request-id", "story-run",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--id", "implementation", "--objective", "Explain the complete transaction.", root, "implementation"}, &output); err != nil {
		t.Fatal(err)
	}
	base = t.TempDir()
	return root, repo, base, commit, sagaID
}

func slideTransactionRequest(t *testing.T, repo, base, commit, sagaID, requestID, operation, expected, element string) SlideTransactionRequest {
	t.Helper()
	assetName := requestID + ".svg"
	writeFile(t, filepath.Join(base, assetName), `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect id="`+element+`" x="1" y="1" width="50" height="20"/></svg>`)
	content, err := os.ReadFile(filepath.Join(repo, "service.go"))
	if err != nil {
		t.Fatal(err)
	}
	digest, err := coderef.DigestRange(content, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	return SlideTransactionRequest{
		Version: 1, Operation: operation, RequestID: requestID, Deck: "implementation", ExpectedSnapshot: expected,
		Slide: SlideTransactionSlide{ID: "flow", Title: "Atomic flow", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Takeaway: "The complete visual changes together.", ReadingOrder: []string{"worker"}},
		Asset: SlideTransactionAsset{Path: assetName},
		Items: []SlideTransactionItemRequest{{
			ID: "worker", Kind: "node", Label: "Worker", Description: "The worker performs the operation.", Selector: saga.LandmarkSelector{Type: "element", ElementID: element},
			Evidence:       []saga.CodeFile{{Version: saga.CurrentVersion, References: []coderef.Reference{{Commit: commit, Path: "service.go", Start: 3, End: 3, Digest: digest, Note: "Run is the exact implementation entrypoint."}}}},
			CriterionLinks: []saga.CriterionLink{{ID: "worker-explains-returns", Criterion: "urn:change-saga:" + sagaID + ":story:run:criterion:returns", StoryRevision: "urn:change-saga:" + sagaID + ":story:run:revision:r1", Rationale: "This Item shows the code path that satisfies the criterion."}},
		}},
	}
}

func TestApplySlideTransactionCreateUpdateRetryAndGuards(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")

	dry, err := ApplySlideTransaction(context.Background(), root, base, repo, create, true)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || !dry.Diff.AssetChanged || strings.Join(dry.Diff.CreatedItems, ",") != "worker" {
		t.Fatalf("dry-run = %#v", dry)
	}
	deckDir := filepath.Join(testFeatureDir(root), saga.EmbeddedSlidesDir, "implementation"+saga.EmbeddedDeckSuffix)
	if matches, _ := filepath.Glob(filepath.Join(deckDir, "25-t-*.json")); len(matches) != 0 {
		t.Fatalf("dry-run published %v", matches)
	}
	if matches, _ := filepath.Glob(filepath.Join(deckDir, "24-a-*")); len(matches) != 0 {
		t.Fatalf("dry-run left asset sidecars %v", matches)
	}

	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Replayed || created.Snapshot == "" || len(created.ChangedIDs) != 2 {
		t.Fatalf("create = %#v", created)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load create: err=%v issues=%#v", err, validation.Issues)
	}
	item := document.Decks[0].Slides[0].Items[0]
	if item.Selector.ElementID != "node-a" || len(item.Code) != 1 || len(item.CriterionLinks) != 1 {
		t.Fatalf("created item = %#v", item)
	}
	index := saga.MutationIndexFromDocument(document)
	evidence, evidenceValidation, err := saga.LoadTargetCode(index, item.Target)
	if err != nil || !evidenceValidation.Valid || len(evidence) != 1 {
		t.Fatalf("transaction evidence = %#v valid=%v err=%v", evidence, evidenceValidation.Valid, err)
	}

	update := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-update", "update", created.Snapshot, "node-b")
	updated, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PreviousSnapshot != created.Snapshot || !updated.Diff.AssetChanged || strings.Join(updated.Diff.SelectorChanges, ",") != "worker" {
		t.Fatalf("update = %#v", updated)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || document.Decks[0].Slides[0].Items[0].Selector.ElementID != "node-b" {
		t.Fatalf("updated load: err=%v issues=%#v", err, validation.Issues)
	}

	replayed, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err != nil || !replayed.Replayed || replayed.Snapshot != updated.Snapshot || len(replayed.ChangedIDs) != 0 {
		t.Fatalf("retry = %#v err=%v", replayed, err)
	}
	stale := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-stale", "update", created.Snapshot, "node-c")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, stale, false); err == nil || !strings.Contains(err.Error(), "expected_snapshot mismatch") {
		t.Fatalf("stale update error = %v", err)
	}

	reused := update
	reused.Slide.Title = "Different payload"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, reused, false); err == nil || !strings.Contains(err.Error(), "different complete-slide payload") {
		t.Fatalf("request reuse error = %v", err)
	}

	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(root, filepath.FromSlash(updated.Path)), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Revisions) != 2 || record.Current != updated.Snapshot {
		t.Fatalf("history = %#v", record)
	}
}

func TestApplySlideTransactionRejectsBrokenSelectorAndRollsBackInjectedFailure(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	invalidReference := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-invalid-ref", "update", created.Snapshot, "node-ref")
	invalidReference.Items[0].Evidence[0].References[0].Path = "../secret.go"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, invalidReference, false); err == nil || !strings.Contains(err.Error(), "repository-relative") {
		t.Fatalf("invalid code reference error = %v", err)
	}
	staleCriterion := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-stale-criterion", "update", created.Snapshot, "node-criterion")
	staleCriterion.Items[0].CriterionLinks[0].StoryRevision = "urn:change-saga:" + sagaID + ":story:run:revision:r9"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, staleCriterion, false); err == nil || !strings.Contains(err.Error(), "story_revision is stale") {
		t.Fatalf("stale criterion error = %v", err)
	}

	broken := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-broken", "update", created.Snapshot, "present")
	broken.Items[0].Selector.ElementID = "missing"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, broken, false); err == nil || !strings.Contains(err.Error(), "does not appear") {
		t.Fatalf("broken selector error = %v", err)
	}

	valid := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-fault", "update", created.Snapshot, "node-fault")
	slideTransactionFault = func(step string) error {
		if step == "before-record-commit" {
			return errors.New("injected failure")
		}
		return nil
	}
	t.Cleanup(func() { slideTransactionFault = nil })
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, valid, false); err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("fault error = %v", err)
	}
	slideTransactionFault = nil
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || document.Decks[0].Slides[0].Items[0].Selector.ElementID != "node-a" {
		t.Fatalf("failed update leaked: err=%v issues=%#v", err, validation.Issues)
	}
	asset, _ := os.ReadFile(filepath.Join(base, "slide-fault.svg"))
	assetName, _ := saga.SlideAssetFilename(asset, ".svg")
	if _, err := os.Stat(filepath.Join(document.Decks[0].Directory, assetName)); !os.IsNotExist(err) {
		t.Fatalf("orphan asset survived rollback: %v", err)
	}
}

func TestSlideTransactionAssetPathRejectsTraversalAndSymlinks(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.svg")
	writeFile(t, outside, `<svg xmlns="http://www.w3.org/2000/svg"/>`)
	if _, _, err := readSlideTransactionAsset(base, SlideTransactionAsset{Path: "../outside.svg"}); err == nil || !strings.Contains(err.Error(), "normalized relative") {
		t.Fatalf("traversal error = %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "linked.svg")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readSlideTransactionAsset(base, SlideTransactionAsset{Path: "linked.svg"}); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestApplySlideTransactionMigratesLegacySlideWithoutChangingStableIDs(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	var output bytes.Buffer
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--id", "flow", "--intent", "explain", "--layout", "diagram", "--title", "Legacy flow", "--takeaway", "The original slide remains history.", root, "flow"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "flow", "--id", "worker", "--kind", "node", "--element-id", "slide-title", "--label", "Worker", "--description", "The original worker.", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("legacy load: %v %#v", err, validation.Issues)
	}
	legacyTarget := document.Decks[0].Slides[0].Items[0].Target
	legacy, err := legacySlideRevision(document.Decks[0].Slides[0])
	if err != nil {
		t.Fatal(err)
	}
	update := slideTransactionRequest(t, repo, base, commit, sagaID, "legacy-migration", "update", legacy.Snapshot, "node-new")
	result, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("migrated load: %v %#v", err, validation.Issues)
	}
	if got := document.Decks[0].Slides[0].Items[0]; got.Target != legacyTarget || got.Selector.ElementID != "node-new" {
		t.Fatalf("migrated Item = %#v", got)
	}
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(root, filepath.FromSlash(result.Path)), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Revisions) != 2 || !strings.HasPrefix(record.Revisions[0].RequestID, "legacy-") {
		t.Fatalf("migration history = %#v", record)
	}
}
