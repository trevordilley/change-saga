package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewapp"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func newSlideTransactionFixture(t *testing.T) (root, repo, base, commit, sagaID string) {
	t.Helper()
	dir, values := slideTransactionTemplate.instantiate(t, func(t *testing.T, dir string) map[string]string {
		repo := mkdir(t, filepath.Join(dir, "repo"))
		git(t, repo, "init", "-b", "main")
		git(t, repo, "config", "user.name", "Test Author")
		git(t, repo, "config", "user.email", "test@example.test")
		git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
		writeFile(t, filepath.Join(repo, "service.go"), "package service\n\nfunc Run() error { return nil }\n")
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "base")
		commit := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))

		root := filepath.Join(dir, "saga", "transaction.saga")
		var output bytes.Buffer
		if err := Init(context.Background(), []string{"--repo", repo, "--repository", "https://example.test/acme/app.git", root}, &output); err != nil {
			t.Fatal(err)
		}
		addTestApp(t, root)
		manifest, err := saga.ReadManifest(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := Story(context.Background(), []string{
			"add", root, "--feature", testFeature, "--persona", personaURNFor(manifest.ID), "--id", "run",
			"--title", "Run safely", "--statement", "As a user I run the service", "--criterion", "returns=Run returns without error", "--request-id", "story-run",
		}, &output); err != nil {
			t.Fatal(err)
		}
		if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--id", "implementation", "--objective", "Explain the complete transaction.", root, "implementation"}, &output); err != nil {
			t.Fatal(err)
		}
		return map[string]string{"commit": commit, "saga": manifest.ID}
	})
	return filepath.Join(dir, "saga", "transaction.saga"), filepath.Join(dir, "repo"), t.TempDir(), values["commit"], values["saga"]
}

var slideTransactionTemplate fixtureTemplate

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
	sagaRoot := filepath.Join(filepath.Dir(repo), "saga", "transaction.saga")
	records, err := requirements.Load(sagaRoot, sagaID)
	if err != nil {
		t.Fatal(err)
	}
	story := records.FindStory("run")
	storyPin, _ := livingid.Revision(sagaID, "run", story.CurrentRevision.ID)
	return SlideTransactionRequest{
		Version: 1, Operation: operation, RequestID: requestID, Deck: "implementation", ExpectedSnapshot: expected,
		Slide: SlideTransactionSlide{ID: "flow", Title: "Atomic flow", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Takeaway: "The complete visual changes together.", ReadingOrder: []string{"worker"}},
		Asset: SlideTransactionAsset{Path: assetName},
		Items: []SlideTransactionItemRequest{{
			ID: "worker", Kind: "node", Label: "Worker", Description: "The worker performs the operation.", Selector: saga.LandmarkSelector{Type: "element", ElementID: element},
			Evidence:       []saga.CodeFile{{Version: saga.CurrentVersion, References: []coderef.Reference{{Commit: commit, Path: "service.go", Start: 3, End: 3, Digest: digest, Note: "Run is the exact implementation entrypoint."}}}},
			CriterionLinks: []saga.CriterionLink{{ID: "worker-explains-returns", Criterion: "urn:change-saga:" + sagaID + ":story:run:criterion:returns", StoryRevision: storyPin, Rationale: "This Item shows the code path that satisfies the criterion."}},
		}},
	}
}

func TestApplySlideTransactionCreateUpdateRetryAndGuards(t *testing.T) {
	t.Parallel()
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
	createRetry, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil || !createRetry.Replayed || createRetry.Snapshot != created.Snapshot || createRetry.PreviousSnapshot != created.Snapshot || len(createRetry.ChangedIDs) != 0 {
		t.Fatalf("create retry = %#v err=%v", createRetry, err)
	}
	changedCreate := create
	changedCreate.Slide.Title = "Different create payload"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, changedCreate, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("changed create retry error = %v", err)
	}
	differentCreateID := create
	differentCreateID.RequestID = "slide-create-again"
	if result, err := ApplySlideTransaction(context.Background(), root, base, repo, differentCreateID, false); err != nil || !result.Unchanged {
		t.Fatalf("identical content with another request id = %#v %v", result, err)
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
	if err != nil || !replayed.Replayed || replayed.Snapshot != updated.Snapshot || replayed.PreviousSnapshot != updated.Snapshot || len(replayed.ChangedIDs) != 0 {
		t.Fatalf("retry = %#v err=%v", replayed, err)
	}
	stale := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-stale", "update", created.Snapshot, "node-c")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, stale, false); err == nil || !strings.Contains(err.Error(), "expected_snapshot mismatch") {
		t.Fatalf("stale update error = %v", err)
	}

	reused := update
	reused.Slide.Title = "Different payload"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, reused, false); err == nil || !strings.Contains(err.Error(), "expected_snapshot mismatch") {
		t.Fatalf("request reuse error = %v", err)
	}
	reused = update
	reused.ExpectedSnapshot = "sha256:" + strings.Repeat("0", 64)
	if result, err := ApplySlideTransaction(context.Background(), root, base, repo, reused, false); err != nil || !result.Unchanged {
		t.Fatalf("a value retry should need no saved request ledger: %#v %v", result, err)
	}

	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(root, filepath.FromSlash(updated.Path)), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Revisions) != 1 || record.Current != updated.Snapshot {
		t.Fatalf("history = %#v", record)
	}
}

func TestApplySlideCommandPublishesStructuredRequest(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-cli", "create", "absent", "node-cli")
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	requestPath := filepath.Join(base, "request.json")
	writeFile(t, requestPath, string(data))
	var out bytes.Buffer
	if err := ApplySlide(context.Background(), []string{"--from", requestPath, "--repo", repo, "--json", root}, &out); err != nil {
		t.Fatalf("apply-slide: %v\n%s", err, out.String())
	}
	var result SlideTransactionResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || result.Operation != "create" || result.Snapshot == "" {
		t.Fatalf("apply-slide result=%#v decode=%v body=%s", result, err, out.String())
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
	staleCriterion.Items[0].CriterionLinks[0].StoryRevision = "sha256:" + strings.Repeat("f", 64)
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

func TestApplySlideTransactionPreservesReferencedAssetAfterPublishedWriteFailure(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	update := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-published", "update", created.Snapshot, "node-published")
	originalWrite := writeSlideTransactionRecord
	writeSlideTransactionRecord = func(path string, value any, exclusive bool) error {
		if err := originalWrite(path, value, exclusive); err != nil {
			return err
		}
		return &store.PublicationError{Path: path, Published: true, Durable: false, Err: errors.New("injected post-commit fsync failure")}
	}
	t.Cleanup(func() { writeSlideTransactionRecord = originalWrite })
	result, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err == nil || !strings.Contains(err.Error(), "was published") || result.Snapshot == "" {
		t.Fatalf("published failure result=%#v err=%v", result, err)
	}
	writeSlideTransactionRecord = originalWrite
	document, validation, loadErr := saga.Load(root)
	if loadErr != nil || !validation.Valid || document.Decks[0].Slides[0].Items[0].Selector.ElementID != "node-published" {
		t.Fatalf("published record did not remain readable: err=%v issues=%#v", loadErr, validation.Issues)
	}
	asset, _ := os.ReadFile(filepath.Join(base, "slide-published.svg"))
	assetName, _ := saga.SlideAssetFilename(asset, ".svg")
	if _, statErr := os.Stat(filepath.Join(document.Decks[0].Directory, assetName)); statErr != nil {
		t.Fatalf("published record's asset was rolled back: %v", statErr)
	}
	replayed, retryErr := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if retryErr != nil || !replayed.Replayed || replayed.Snapshot != result.Snapshot {
		t.Fatalf("retry after published failure = %#v err=%v", replayed, retryErr)
	}
}

func TestSlideTransactionAssetPathRejectsTraversalAndSymlinks(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	var output bytes.Buffer
	if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--id", "flow", "--intent", "explain", "--layout", "diagram", "--title", "Legacy flow", "--takeaway", "The original slide remains history.", root, "flow"}, &output); err != nil {
		t.Fatal(err)
	}
	if err := AddItem(context.Background(), []string{"--slide", "flow", "--id", "worker", "--kind", "node", "--element-id", "slide-title", "--label", "Worker", "--description", "The original worker.", root}, &output); err != nil {
		t.Fatal(err)
	}
	if err := Cover(context.Background(), []string{"--target", saga.ItemTarget(sagaID, "flow", "worker"), "--repo", repo, "--ref", commit + ":service.go#L3", "--note", "Legacy implementation evidence", root}, &output); err != nil {
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
	if document.Decks[0].Slides[0].AuthoringSnapshot != legacy.Snapshot || len(document.Decks[0].Slides[0].AuthoringHeads) != 1 {
		t.Fatalf("legacy query snapshot = %#v, want %s", document.Decks[0].Slides[0], legacy.Snapshot)
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
	if len(record.Revisions) != 1 || record.Revisions[0].RequestID != "" || len(record.Revisions[0].ParentSnapshots) != 0 {
		t.Fatalf("migration history = %#v", record)
	}
	beforeRemoval := snapshotFiles(t, root)
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, &output); err != nil {
		t.Fatal(err)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || len(document.Decks[0].Slides) != 0 {
		t.Fatalf("legacy content reappeared after removal: %v %#v", err, validation.Issues)
	}
	for path, before := range beforeRemoval {
		if path == filepath.Join(root, result.Path) {
			continue
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != before {
			t.Fatalf("removal lost migration history at %s: %v", path, err)
		}
	}
}

func TestLegacySlideAndCoverageCommandsRefuseTransactionManagedTargets(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, filepath.FromSlash(created.Path))
	before, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	slide, item := document.Decks[0].Slides[0], document.Decks[0].Slides[0].Items[0]
	evidencePath := item.Code[0].Path
	relationURN := "urn:change-saga:" + sagaID + ":relation:worker-explains-returns"
	criterionURN := "urn:change-saga:" + sagaID + ":story:run:criterion:returns"
	assetSource := filepath.Join(base, "legacy-set.svg")
	writeFile(t, assetSource, `<svg xmlns="http://www.w3.org/2000/svg"><rect id="node-a"/></svg>`)
	ref := commit + ":service.go#L3"

	tests := []struct {
		name string
		run  func(*bytes.Buffer) error
	}{
		{"add-item", func(out *bytes.Buffer) error {
			return AddItem(context.Background(), []string{"--slide", slide.Target, "--id", "extra", "--kind", "node", "--element-id", "node-a", "--description", "extra", root}, out)
		}},
		{"set-slide-content", func(out *bytes.Buffer) error {
			return SetSlideContent(context.Background(), []string{"--target", slide.Target, "--source", assetSource, root}, out)
		}},
		{"revise-slide", func(out *bytes.Buffer) error {
			return ReviseSlide(context.Background(), []string{"--slide", slide.Target, "--title", "Changed", root}, out)
		}},
		{"revise-item", func(out *bytes.Buffer) error {
			return ReviseItem(context.Background(), []string{"--item", item.Target, "--label", "Changed", root}, out)
		}},
		{"remove-item", func(out *bytes.Buffer) error {
			return RemoveItem(context.Background(), []string{"--item", item.Target, root}, out)
		}},
		{"cover", func(out *bytes.Buffer) error {
			return Cover(context.Background(), []string{"--target", item.Target, "--repo", repo, "--ref", ref, "--note", "legacy mutation", root}, out)
		}},
		{"remove-coverage", func(out *bytes.Buffer) error {
			return RemoveCoverage(context.Background(), []string{"--record", evidencePath, root}, out)
		}},
		{"replace-coverage", func(out *bytes.Buffer) error {
			return ReplaceCoverage(context.Background(), []string{"--record", evidencePath, "--target", item.Target, "--repo", repo, "--ref", ref, "--note", "legacy mutation", root}, out)
		}},
		{"relation-add-collision", func(out *bytes.Buffer) error {
			return Relation(context.Background(), []string{"add", "--feature", testFeature, "--id", "worker-explains-returns", "--type", "explains", "--from", item.Target, "--to", criterionURN, "--rationale", "duplicate", root}, out)
		}},
		{"relation-supersede", func(out *bytes.Buffer) error {
			return Relation(context.Background(), []string{"supersede", "--relation", relationURN, root}, out)
		}},
		{"relation-repin", func(out *bytes.Buffer) error {
			return Relation(context.Background(), []string{"repin", "--relation", relationURN, "--rationale", "still true", root}, out)
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var out bytes.Buffer
			err := testCase.run(&out)
			if err == nil || !strings.Contains(err.Error(), "complete-slide transaction history") || !strings.Contains(err.Error(), "query slide") || !strings.Contains(err.Error(), "apply-slide") {
				t.Fatalf("error = %v output=%s", err, out.String())
			}
			after, readErr := os.ReadFile(recordPath)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatalf("transaction record changed: err=%v", readErr)
			}
		})
	}
}

func TestTransactionLinksUseCanonicalQueryTraceabilityAuditAndCurrencyGraph(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	story := "urn:change-saga:" + sagaID + ":story:run"
	if err := Story(context.Background(), []string{"set-state", root, "--story", story, "--state", "accepted", "--reason", "ready"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	review, err := reviewapp.Open(context.Background(), reviewapp.OpenOptions{SagaRoot: root, SourceDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	slideTarget := saga.SlideTarget(sagaID, "flow")
	slide, err := review.ReadFragment(context.Background(), reviewapp.FragmentQuery{Target: slideTarget})
	if err != nil {
		t.Fatal(err)
	}
	if slide.AuthoringSnapshot != created.Snapshot || len(slide.AuthoringHeads) != 1 || slide.AuthoringConflict || len(slide.Landmarks) != 1 || len(slide.Landmarks[0].Evidence) != 1 || len(slide.Landmarks[0].CriterionLinks) != 1 {
		t.Fatalf("query slide authoring contract = %#v", slide)
	}

	openLiving := func() livingapp.Session {
		session, openErr := livingapp.Open(context.Background(), livingapp.OpenOptions{SagaRoot: root, Audit: true})
		if openErr != nil {
			t.Fatal(openErr)
		}
		return session
	}
	living := openLiving()
	relations, err := living.Query(context.Background(), livingapp.Query{Operation: "relations", Filters: livingapp.Filters{Relation: "worker-explains-returns"}})
	if err != nil {
		t.Fatal(err)
	}
	relationRows := relations.Data.(livingapp.RelationPage).Relations
	if len(relationRows) != 1 || relationRows[0].From != saga.ItemTarget(sagaID, "flow", "worker") || relationRows[0].To != create.Items[0].CriterionLinks[0].Criterion || relationRows[0].Stale {
		t.Fatalf("projected relation = %#v", relationRows)
	}
	contextResult, err := living.Query(context.Background(), livingapp.Query{Operation: "context", Filters: livingapp.Filters{Feature: testFeature}})
	if err != nil {
		t.Fatal(err)
	}
	contextPage := contextResult.Data.(livingapp.FeatureContextPage)
	if len(contextPage.Visuals) != 1 || len(contextPage.Visuals[0].Items) != 1 || len(contextPage.Visuals[0].Items[0].CriterionLinks) != 1 {
		t.Fatalf("context visuals = %#v", contextPage.Visuals)
	}
	traceResult, err := living.Query(context.Background(), livingapp.Query{Operation: "traceability", Filters: livingapp.Filters{Requirement: story}})
	if err != nil {
		t.Fatal(err)
	}
	traces := traceResult.Data.(livingapp.TraceabilityPage).Criteria
	if len(traces) != 1 || len(traces[0].ReviewTargets) != 1 || traces[0].ReviewTargets[0] != saga.ItemTarget(sagaID, "flow", "worker") {
		t.Fatalf("traceability = %#v", traces)
	}
	auditResult, err := living.Query(context.Background(), livingapp.Query{Operation: "audit", Filters: livingapp.Filters{Feature: testFeature}})
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range auditResult.Data.(livingapp.AuditReport).Findings {
		if finding.Code == "item_intent_missing" || finding.Code == "criterion_explanation_missing" {
			t.Fatalf("audit ignored projected relation: %#v", finding)
		}
	}

	if err := Story(context.Background(), []string{"revise", root, "--story", story, "--criterion", "returns=Run now returns a detailed result", "--request-id", "revise-run"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	living = openLiving()
	relations, err = living.Query(context.Background(), livingapp.Query{Operation: "relations", Filters: livingapp.Filters{Relation: "worker-explains-returns"}})
	if err != nil {
		t.Fatal(err)
	}
	relationRows = relations.Data.(livingapp.RelationPage).Relations
	if len(relationRows) != 1 || !relationRows[0].Stale || len(relationRows[0].StaleReasons) == 0 {
		t.Fatalf("projected relation currency = %#v", relationRows)
	}
}

func TestLegacyDivergentTransactionsRequireExplicitContentResolution(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "base", "create", "absent", "node-base")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, filepath.FromSlash(created.Path))
	baseBytes, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	var revisions []saga.SlideTransactionRevision
	readRevision := func() saga.SlideTransactionRevision {
		var record saga.SlideTransactionRecord
		if err := readStrictJSONPath(recordPath, &record); err != nil {
			t.Fatal(err)
		}
		return record.Revisions[0]
	}
	rootRevision := readRevision()
	rootRevision.RequestID = "base"
	rootRevision.CreatedAt = time.Now().UTC()
	rootRevision.Snapshot, _ = saga.SlideRevisionSnapshot(rootRevision)
	revisions = append(revisions, rootRevision)
	for _, side := range []string{"left", "right"} {
		if err := os.WriteFile(recordPath, baseBytes, 0644); err != nil {
			t.Fatal(err)
		}
		request := slideTransactionRequest(t, repo, base, commit, sagaID, side, "update", created.Snapshot, "node-"+side)
		if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
			t.Fatal(err)
		}
		revision := readRevision()
		revision.RequestID, revision.CreatedAt = side, time.Now().UTC()
		revision.ParentSnapshots = []string{rootRevision.Snapshot}
		revision.Snapshot, _ = saga.SlideRevisionSnapshot(revision)
		revisions = append(revisions, revision)
	}
	// Encode an actual legacy graph, bypassing the current-only marshaler.
	type legacyRecord saga.SlideTransactionRecord
	legacy := legacyRecord{Version: 1, DeckID: "implementation", SlideID: "flow", Current: revisions[1].Snapshot, Revisions: revisions}
	if err := store.WriteJSON(recordPath, legacy, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "edit", "update", "", "node-new")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err == nil || !strings.Contains(err.Error(), "competing") && !strings.Contains(err.Error(), "divergent") {
		t.Fatalf("ambiguous legacy update = %v", err)
	}
	after := snapshotFiles(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("ambiguity refusal changed authored content or assets")
	}
	// The author explicitly chooses left content. The on-disk current value is
	// the resolution; no Saga reconcile operation or parent/head ledger is added.
	selected := revisions[1]
	selected.ParentSnapshots = nil
	selected.Snapshot, err = saga.SlideContentDigest(selected)
	if err != nil {
		t.Fatal(err)
	}
	current := saga.SlideTransactionRecord{Version: 1, DeckID: "implementation", SlideID: "flow", Current: selected.Snapshot, Revisions: []saga.SlideTransactionRevision{selected}}
	if err := store.WriteJSON(recordPath, current, false); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || document.Decks[0].Slides[0].Items[0].Selector.ElementID != "node-left" {
		t.Fatalf("explicit current resolution: %v %#v", err, validation.Issues)
	}
	for _, revision := range revisions {
		if _, err := os.Stat(filepath.Join(filepath.Dir(recordPath), revision.Asset)); err != nil {
			t.Fatal("resolution removed an authored asset:", err)
		}
	}
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	var final saga.SlideTransactionRecord
	if err := readStrictJSONPath(recordPath, &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Revisions) != 1 || len(final.Revisions[0].ParentSnapshots) != 0 {
		t.Fatal("current edit recreated internal history")
	}
}
