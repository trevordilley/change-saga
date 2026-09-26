package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingapp"
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
			"add", root, "--feature", testFeature, "--persona", personaURNFor(manifest.ID), "--id", "run", "--revision", "r1", "--event", "proposed",
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
	if err != nil || !createRetry.Replayed || createRetry.Snapshot != created.Snapshot || createRetry.PreviousSnapshot != "" || len(createRetry.ChangedIDs) != 0 {
		t.Fatalf("create retry = %#v err=%v", createRetry, err)
	}
	changedCreate := create
	changedCreate.Slide.Title = "Different create payload"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, changedCreate, false); err == nil || !strings.Contains(err.Error(), "different complete-slide payload") {
		t.Fatalf("changed create retry error = %v", err)
	}
	differentCreateID := create
	differentCreateID.RequestID = "slide-create-again"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, differentCreateID, false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing slide create error = %v", err)
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
	if err != nil || !replayed.Replayed || replayed.Snapshot != updated.Snapshot || replayed.PreviousSnapshot != created.Snapshot || len(replayed.ChangedIDs) != 0 {
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
	reused = update
	reused.ExpectedSnapshot = "sha256:" + strings.Repeat("0", 64)
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, reused, false); err == nil || !strings.Contains(err.Error(), "different complete-slide payload") {
		t.Fatalf("request expectation reuse error = %v", err)
	}

	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(filepath.Join(root, filepath.FromSlash(updated.Path)), &record); err != nil {
		t.Fatal(err)
	}
	if len(record.Revisions) != 2 || record.Current != updated.Snapshot {
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
	if len(record.Revisions) != 2 || !strings.HasPrefix(record.Revisions[0].RequestID, "legacy-") {
		t.Fatalf("migration history = %#v", record)
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
		{"remove-slide", func(out *bytes.Buffer) error {
			return RemoveSlide(context.Background(), []string{"--slide", slide.Target, root}, out)
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
	parent, _ := requirements.StoryEventURN(sagaID, "run", "proposed")
	if err := Story(context.Background(), []string{"set-state", root, "--story", story, "--event", "accepted", "--parent", parent, "--state", "accepted", "--reason", "ready"}, &bytes.Buffer{}); err != nil {
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

	if err := Story(context.Background(), []string{"revise", root, "--story", story, "--revision", "r2", "--parent", story + ":revision:r1", "--criterion", "returns=Run now returns a detailed result", "--request-id", "revise-run"}, &bytes.Buffer{}); err != nil {
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

func TestDivergentTransactionHistoriesPreserveHeadsAndReconcile(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(root, filepath.FromSlash(created.Path))
	var baseRecord saga.SlideTransactionRecord
	if err := readStrictJSONPath(recordPath, &baseRecord); err != nil {
		t.Fatal(err)
	}
	leftRequest := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-left", "update", created.Snapshot, "node-left")
	left, err := ApplySlideTransaction(context.Background(), root, base, repo, leftRequest, false)
	if err != nil {
		t.Fatal(err)
	}
	var leftRecord saga.SlideTransactionRecord
	if err := readStrictJSONPath(recordPath, &leftRecord); err != nil {
		t.Fatal(err)
	}
	leftAssetPath := filepath.Join(filepath.Dir(recordPath), leftRecord.Revisions[1].Asset)
	leftAsset, err := os.ReadFile(leftAssetPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteJSON(recordPath, baseRecord, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(leftAssetPath); err != nil {
		t.Fatal(err)
	}
	rightRequest := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-right", "update", created.Snapshot, "node-right")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, rightRequest, false); err != nil {
		t.Fatal(err)
	}
	var merged saga.SlideTransactionRecord
	if err := readStrictJSONPath(recordPath, &merged); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteFile(leftAssetPath, leftAsset, 0o644, true); err != nil {
		t.Fatal(err)
	}
	merged.Revisions = append(merged.Revisions, leftRecord.Revisions[1])
	merged.Current = left.Snapshot
	if err := store.WriteJSON(recordPath, merged, false); err != nil {
		t.Fatal(err)
	}

	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("merged divergent load: %v issues=%#v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	if !slide.AuthoringConflict || len(slide.AuthoringHeads) != 2 {
		t.Fatalf("divergent authoring state = %#v", slide)
	}
	normal := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-normal", "update", slide.AuthoringSnapshot, "node-normal")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, normal, false); err == nil || !strings.Contains(err.Error(), "divergent heads") || !strings.Contains(err.Error(), "expected_snapshots") {
		t.Fatalf("normal update on divergence error = %v", err)
	}
	reconcile := slideTransactionRequest(t, repo, base, commit, sagaID, "slide-reconcile", "reconcile", "", "node-reconciled")
	reconcile.ExpectedSnapshots = append([]string{}, slide.AuthoringHeads...)
	reconciled, err := ApplySlideTransaction(context.Background(), root, base, repo, reconcile, false)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Operation != "reconcile" || reconciled.Snapshot == "" {
		t.Fatalf("reconcile result = %#v", reconciled)
	}
	var final saga.SlideTransactionRecord
	if err := readStrictJSONPath(recordPath, &final); err != nil {
		t.Fatal(err)
	}
	if len(final.Revisions) != 4 || !sameSnapshotSet(final.Revisions[3].ParentSnapshots, slide.AuthoringHeads) {
		t.Fatalf("reconciled history = %#v", final)
	}
	document, validation, err = saga.Load(root)
	if err != nil || !validation.Valid || document.Decks[0].Slides[0].AuthoringConflict || document.Decks[0].Slides[0].Items[0].Selector.ElementID != "node-reconciled" {
		t.Fatalf("reconciled load: err=%v issues=%#v slide=%#v", err, validation.Issues, document.Decks[0].Slides[0])
	}
}
