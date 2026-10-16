package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/saga"
)

func hierarchyStep(t *testing.T, run func(context.Context, []string, io.Writer) error, args ...string) {
	t.Helper()
	var out bytes.Buffer
	if err := run(context.Background(), args, &out); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
}

func hierarchyDocument(t *testing.T, root string) *saga.Saga {
	t.Helper()
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v, %+v", err, validation.Issues)
	}
	return document
}

func TestMoveSlidePreservesIdentityEvidenceAndRank(t *testing.T) {
	t.Parallel()
	root := contentSaga(t)
	hierarchyStep(t, Feature, "add", "--id", "destination", "--title", "Destination", root)
	hierarchyStep(t, AddDeck, "--feature", "destination", "--objective", "Second deck", root, "second")
	hierarchyStep(t, Story, "add", "--feature", testFeature, "--id", "handle", "--revision", "r1", "--event", "proposed", "--title", "Handle", "--statement", "As a user I want requests handled so that I am served", "--priority", "must", root)
	hierarchyStep(t, Relation, "add", "--feature", testFeature, "--id", "flow-explains-handle", "--type", "explains", "--from", "urn:change-saga:batch:slide:flow:item:handler", "--to", "urn:change-saga:batch:story:handle", "--rationale", "The handler shows it.", root)
	before := snapshotFiles(t, root)
	original := findSlide(hierarchyDocument(t, root), "flow")
	dry, err := runContentJSON(t, MoveSlide, "--slide", "flow", "--deck", "second", "--dry-run", root)
	if err != nil || !dry.DryRun || len(dry.Paths) == 0 {
		t.Fatalf("dry run: %+v %v", dry, err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("dry run changed files")
	}
	moved, err := runContentJSON(t, MoveSlide, "--slide", original.Target, "--deck", "second", root)
	if err != nil || moved.Resource != original.Target {
		t.Fatalf("move: %+v %v", moved, err)
	}
	document := hierarchyDocument(t, root)
	slide := findSlide(document, original.Target)
	if slide.DeckID != "second" || slide.Target != original.Target || len(slide.Items) != 2 || len(slide.Items[0].Code) != 1 {
		t.Fatalf("moved slide: %+v", slide)
	}
	if len(findDeck(document, "implementation").Slides) != 0 {
		t.Fatal("old deck retained slide")
	}
	if saga.FeatureOf(slide.Path) != "destination" {
		t.Fatalf("wrong destination feature: %s", slide.Path)
	}
	relationPath := filepath.Join(testFeatureDir(root), "___requirements", "relations", "flow-explains-handle.json")
	relation, err := os.ReadFile(relationPath)
	if err != nil || string(relation) != before[relationPath] {
		t.Fatalf("inbound relation changed: %v", err)
	}
	// The public children API reports the new deck membership across features.
	repo := filepath.Join(filepath.Dir(filepath.Dir(root)), "repo")
	for _, id := range []string{"implementation", "second"} {
		var out bytes.Buffer
		if err := Query(context.Background(), []string{"children", "--saga", root, "--repo", repo, "--parent", findDeck(document, id).Target}, &out); err != nil {
			t.Fatalf("children: %v %s", err, out.String())
		}
		var page struct {
			Data struct {
				Children []struct {
					Target string `json:"target"`
				} `json:"children"`
			} `json:"data"`
		}
		if err := json.Unmarshal(out.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, child := range page.Data.Children {
			if child.Target == original.Target {
				found = true
			}
		}
		if found != (id == "second") {
			t.Fatalf("wrong membership for %s: %s", id, out.String())
		}
	}

	if slide.Items[0].Target != original.Items[0].Target || !reflect.DeepEqual(slide.Items[0].Code[0].References, original.Items[0].Code[0].References) {
		t.Fatal("item identity or evidence changed")
	}
	if _, err := runContentJSON(t, ReviseSlide, "--slide", slide.Target, "--rank", "42", root); err != nil {
		t.Fatal(err)
	}
	ranked := findSlide(hierarchyDocument(t, root), original.Target)
	if ranked.Rank != 42 || ranked.Items[0].Target != original.Items[0].Target {
		t.Fatalf("rank changed identity: %+v", ranked)
	}
	replay, err := runContentJSON(t, MoveSlide, "--slide", slide.Target, "--deck", "second", root)
	if err != nil || !replay.Replayed || len(replay.Paths) != 0 {
		t.Fatalf("replay: %+v %v", replay, err)
	}
}

func TestMoveNarrativeContainmentAndIdentity(t *testing.T) {
	t.Parallel()
	root := contentSaga(t)
	hierarchyStep(t, AddChapter, "--feature", testFeature, root, "other")
	hierarchyStep(t, AddSection, "--feature", testFeature, "--id", "nested", root, "service/nested")
	hierarchyStep(t, AddSection, "--feature", testFeature, "--id", "child", root, "service.chapter/nested/child")
	document := hierarchyDocument(t, root)
	_, target, err := resolveNarrative(document, "fragment", testFeatureRel+"/service.chapter/notes.fragment")
	if err != nil {
		t.Fatal(err)
	}
	hierarchyStep(t, MoveFragment, "--target", target, "--parent", "nested", "--name", "renamed", root)
	dir, got, err := resolveNarrative(hierarchyDocument(t, root), "fragment", target)
	if err != nil || got != target || filepath.Base(dir) != "renamed.fragment" {
		t.Fatalf("fragment move %s %s %v", dir, got, err)
	}
	hierarchyStep(t, MoveSection, "--target", "nested", "--parent", "other", root)
	hierarchyStep(t, MoveChapter, "--target", "other", "--parent", "designsystem", "--name", "shared", root)
	dir, got, err = resolveNarrative(hierarchyDocument(t, root), "fragment", target)
	if err != nil || got != target || !strings.Contains(filepath.ToSlash(dir), "___designsystem/shared.chapter/nested/renamed.fragment") {
		t.Fatalf("subtree move %s %s %v", dir, got, err)
	}
	hierarchyStep(t, ReviseSection, "--target", "nested", "--order", "8", root)
	for _, test := range []struct {
		run  func(context.Context, []string, io.Writer) error
		args []string
		want string
	}{
		{MoveSection, []string{"--target", "nested", "--parent", "child"}, "inside itself"},
		{MoveChapter, []string{"--target", "other", "--parent", "nested"}, "direct children"},
		{MoveSection, []string{"--target", "nested", "--parent", testFeature}, "must belong"},
		{MoveFragment, []string{"--target", target, "--parent", target}, "not a valid"},
		{MoveFragment, []string{"--target", target, "--parent", "nested", "--name", "../escape"}, "basename"},
	} {
		before := snapshotFiles(t, root)
		for _, dry := range []bool{false, true} {
			args := append([]string{}, test.args...)
			if dry {
				args = append(args, "--dry-run")
			}
			args = append(args, root)
			if _, err := runContentJSON(t, test.run, args...); err == nil {
				t.Fatalf("accepted invalid containment: %v", args)
			}
			if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
				t.Fatalf("rejected move changed files: %v", args)
			}
		}
	}
}

func TestMoveSlideRollsBackEveryFileOnCollision(t *testing.T) {
	t.Parallel()
	root := contentSaga(t)
	hierarchyStep(t, AddDeck, "--feature", testFeature, "--objective", "Second deck", root, "second")
	document := hierarchyDocument(t, root)
	slide := findSlide(document, "flow")
	// Collision occurs after the slide manifest, asset, and first Item move.
	code := slide.Items[0].Code[0]
	writeFile(t, filepath.Join(findDeck(document, "second").Directory, filepath.Base(code.Path)), "collision")
	before := snapshotFiles(t, root)
	for _, dry := range []bool{false, true} {
		args := []string{"--slide", "flow", "--deck", "second"}
		if dry {
			args = append(args, "--dry-run")
		}
		args = append(args, root)
		if _, err := runContentJSON(t, MoveSlide, args...); err == nil || !strings.Contains(err.Error(), "destination already exists") {
			t.Fatalf("collision: %v", err)
		}
		if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
			t.Fatal("collision did not roll back every file")
		}
	}
}

func TestMoveRollsBackValidationFailureAndParents(t *testing.T) {
	t.Parallel()
	root := contentSaga(t)
	before := snapshotFiles(t, root)
	err := runContentEdit(contentEditRequest{root: root, operation: "move-chapter", apply: func(document *saga.Saga, edit *contentEdit, result *contentEditOutput) error {
		dir, _, err := resolveNarrative(document, "chapter", "service")
		if err != nil {
			return err
		}
		return edit.move(dir, filepath.Join(document.Root, "invalid", "service.chapter"))
	}}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("invalidating move: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("validation failure changed files")
	}
	if _, err := os.Stat(filepath.Join(root, "invalid")); !os.IsNotExist(err) {
		t.Fatalf("rollback retained new parent: %v", err)
	}
}

func TestMoveDeckPreservesTransactionHistoryAndRefusesDeletion(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "first", "create", "absent", "node")
	first, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	request = slideTransactionRequest(t, repo, base, commit, sagaID, "second", "update", first.Snapshot, "node")
	request.Slide.Title = "Updated title"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	hierarchyStep(t, Feature, "add", "--id", "destination", "--title", "Destination", root)
	hierarchyStep(t, AddDeck, "--feature", testFeature, "--objective", "Other deck", root, "other")
	before := snapshotFiles(t, root)
	for _, test := range []struct {
		run  func(context.Context, []string, io.Writer) error
		args []string
	}{
		{MoveSlide, []string{"--slide", "flow", "--deck", "other", root}},
		{RemoveDeck, []string{"--deck", "implementation", root}},
	} {
		if _, err := runContentJSON(t, test.run, test.args...); err == nil || !strings.Contains(err.Error(), "transaction history") {
			t.Fatalf("immutable edit error: %v", err)
		}
		if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
			t.Fatal("immutable history changed")
		}
	}
	document := hierarchyDocument(t, root)
	original := findDeck(document, "implementation")
	originals := snapshotFiles(t, original.Directory)
	moved, err := runContentJSON(t, MoveDeck, "--deck", original.Target, "--feature", "destination", root)
	if err != nil || moved.Resource != original.Target {
		t.Fatalf("move-deck: %+v %v", moved, err)
	}
	document = hierarchyDocument(t, root)
	deck := findDeck(document, original.Target)
	if saga.FeatureOf(deck.Path) != "destination" {
		t.Fatalf("wrong owner: %s", deck.Path)
	}
	for path, data := range originals {
		rel, _ := filepath.Rel(original.Directory, path)
		movedData, err := os.ReadFile(filepath.Join(deck.Directory, rel))
		if err != nil || string(movedData) != data {
			t.Fatalf("move changed historical bytes %s: %v", rel, err)
		}
	}
	if !strings.HasPrefix(deck.Directory, applayout.FeatureDir(root, "destination")) {
		t.Fatalf("deck path %s", deck.Directory)
	}
}

func TestMoveRejectsSymlinkedDestination(t *testing.T) {
	t.Parallel()
	root := contentSaga(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, applayout.DesignSystemDir)); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, testFeatureDir(root))
	if _, err := runContentJSON(t, MoveChapter, "--target", "service", "--parent", "designsystem", root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink move: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, testFeatureDir(root))) {
		t.Fatal("symlink move changed source")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("symlink move escaped Saga")
	}
}

func TestRemoveFragmentRejectsLandmarkAndAccountsDescendants(t *testing.T) {
	t.Parallel()
	root, _ := landmarkSaga(t)
	fragment := "urn:change-saga:batch:fragment:batch-service-overview"
	landmark := fragment + ":landmark:submit-action"
	before := snapshotFiles(t, root)
	if _, err := runContentJSON(t, RemoveFragment, "--target", landmark, root); err == nil || !strings.Contains(err.Error(), "not a fragment") {
		t.Fatalf("landmark removal: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("wrong-kind removal changed files")
	}
	removed, err := runContentJSON(t, RemoveFragment, "--target", fragment, "--dry-run", root)
	if err != nil || !contains(removed.Removed, landmark) || !contains(removed.Removed, fragment) {
		t.Fatalf("descendant accounting: %+v %v", removed, err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("removal dry run changed files")
	}
}

func TestMoveSlideRejectsReviewOwnershipChanges(t *testing.T) {
	t.Parallel()
	fixture := newReviewFixture(t)
	hierarchyStep(t, AddDeck, "--feature", testFeature, "--objective", "App implementation", fixture.root, "implementation")
	hierarchyStep(t, AddSlide, "--deck", "implementation", "--intent", "explain", "--layout", "diagram", fixture.root, "app-flow")
	hierarchyStep(t, Review, "approve", "--review", "pr-7", "--slide", "queue", "--reviewer-kind", "human", fixture.root)
	before := snapshotFiles(t, fixture.root)
	document := hierarchyDocument(t, fixture.root)
	review := document.FindReview("pr-7")
	for _, args := range [][]string{
		{"--slide", review.Deck.Slides[0].Target, "--deck", "implementation", fixture.root},
		{"--slide", "app-flow", "--deck", review.Deck.Target, "--dry-run", fixture.root},
	} {
		if _, err := runContentJSON(t, MoveSlide, args...); err == nil || !strings.Contains(err.Error(), "ownership cannot change") {
			t.Fatalf("review move: %v", err)
		}
		if !reflect.DeepEqual(before, snapshotFiles(t, fixture.root)) {
			t.Fatal("review history or evidence changed")
		}
	}
}

func TestMoveAndRemoveRefuseDanglingOverviewReferencesInDryRun(t *testing.T) {
	t.Parallel()
	for _, overview := range []string{
		`{"body":"[Callout](annotation:why)","annotations":[{"id":"why","label":"Why","slide":"flow","item":"why"}]}`,
		`{"body":"![Flow](slide:flow)"}`,
	} {
		root := contentSaga(t)
		hierarchyStep(t, AddDeck, "--feature", testFeature, "--objective", "Other deck", root, "other")
		file := filepath.Join(t.TempDir(), "overview.json")
		writeFile(t, file, overview)
		hierarchyStep(t, Deck, "overview", "--deck", "implementation", "--file", file, root)
		before := snapshotFiles(t, root)
		cases := []struct {
			run  func(context.Context, []string, io.Writer) error
			args []string
		}{
			{MoveSlide, []string{"--slide", "flow", "--deck", "other"}},
			{RemoveSlide, []string{"--slide", "flow"}},
		}
		if strings.Contains(overview, "annotations") {
			cases = append(cases, struct {
				run  func(context.Context, []string, io.Writer) error
				args []string
			}{RemoveItem, []string{"--slide", "flow", "--item", "why"}})
		}
		for _, test := range cases {
			for _, dry := range []bool{true, false} {
				args := append([]string{}, test.args...)
				if dry {
					args = append(args, "--dry-run")
				}
				args = append(args, root)
				if _, err := runContentJSON(t, test.run, args...); err == nil || !strings.Contains(err.Error(), "deck overview references") {
					t.Fatalf("overview reference: %v", err)
				}
				if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
					t.Fatal("reference rejection changed files")
				}
			}
		}
	}
}

func TestNarrativeParentRejectsReviewTargetsWithHierarchyNames(t *testing.T) {
	t.Parallel()
	for _, ids := range []struct{ saga, review string }{
		{"example", "chapter"}, {"example", "section"},
		{"chapter", "pr-7"}, {"section", "pr-7"},
	} {
		t.Run(ids.saga+"/"+ids.review, func(t *testing.T) {
			root := t.TempDir()
			deckDir := filepath.Join(root, "___reviews", ids.review+".review", "deck")
			slideTarget := saga.ReviewSlideTarget(ids.saga, ids.review, "queue")
			itemTarget := slideTarget + ":item:node"
			document := &saga.Saga{
				Root: root, Manifest: saga.Manifest{ID: ids.saga}, Section: &saga.Section{},
				Reviews: []*saga.Review{{
					Target: saga.ReviewTarget(ids.saga, ids.review),
					Deck: &saga.Deck{Slides: []*saga.Slide{{
						Target: slideTarget, Directory: deckDir,
						Items: []*saga.Item{{Target: itemTarget, Directory: deckDir}},
					}}},
				}},
			}
			for _, target := range []string{slideTarget, itemTarget} {
				dir, isRoot, err := narrativeParent(document, target)
				if err == nil || !strings.Contains(err.Error(), "parent must be") {
					t.Fatalf("review target accepted as narrative parent: target=%s dir=%s root=%v err=%v", target, dir, isRoot, err)
				}
			}
		})
	}
}
