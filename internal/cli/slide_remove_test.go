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

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestRemoveTransactionContentPreservesHistory(t *testing.T) {
	t.Parallel()
	for _, wholeDeck := range []bool{false, true} {
		name := "slide"
		if wholeDeck {
			name = "deck"
		}
		t.Run(name, func(t *testing.T) {
			root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
			create := slideTransactionRequest(t, repo, base, commit, sagaID, "create", "create", "absent", "node-a")
			created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
			if err != nil {
				t.Fatal(err)
			}
			update := slideTransactionRequest(t, repo, base, commit, sagaID, "update", "update", created.Snapshot, "node-b")
			updated, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(updated.Path))
			var original saga.SlideTransactionRecord
			if err := readStrictJSONPath(path, &original); err != nil {
				t.Fatal(err)
			}
			// A second transaction slide and a legacy slide exercise mixed decks.
			second := create
			second.RequestID, second.Slide.ID, second.Slide.Rank = "second", "second", 10
			second.Items = append([]SlideTransactionItemRequest{}, create.Items...)
			second.Items[0].CriterionLinks = append([]saga.CriterionLink{}, create.Items[0].CriterionLinks...)
			second.Items[0].CriterionLinks[0].ID = "second-explains-returns"
			other, err := ApplySlideTransaction(context.Background(), root, base, repo, second, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "legacy"}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if err := AddDeck(context.Background(), []string{"--feature", testFeature, "--objective", "Keep this deck", root, "unrelated"}, io.Discard); err != nil {
				t.Fatal(err)
			}
			if err := Relation(context.Background(), []string{"add", "--feature", testFeature, "--id", "external", "--type", "explains", "--from", saga.ItemTarget(sagaID, "flow", "worker"), "--to", "urn:change-saga:" + sagaID + ":story:run", "--rationale", "External link", root}, io.Discard); err != nil {
				t.Fatal(err)
			}
			run := RemoveSlide
			args := []string{"--slide", "flow", "--expected-snapshot", updated.Snapshot}
			if wholeDeck {
				run, args = RemoveDeck, []string{"--deck", "implementation"}
			}
			before := snapshotFiles(t, root)
			dry, err := runContentJSON(t, run, append(append([]string{}, args...), "--dry-run", root)...)
			if err != nil || !dry.DryRun || !reflect.DeepEqual(before, snapshotFiles(t, root)) {
				t.Fatalf("dry run: %+v, %v", dry, err)
			}
			if !wholeDeck {
				if _, err := runContentJSON(t, run, "--slide", "flow", "--expected-snapshot", created.Snapshot, root); err == nil || !strings.Contains(err.Error(), "mismatch") {
					t.Fatalf("stale snapshot: %v", err)
				}
				if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
					t.Fatal("stale removal changed files")
				}
			}
			result, err := runContentJSON(t, run, append(args, root)...)
			if err != nil || !result.OK || !reflect.DeepEqual(result.DanglingRelations, []string{"urn:change-saga:" + sagaID + ":relation:external"}) {
				t.Fatalf("removal: %+v, %v", result, err)
			}
			var removed saga.SlideTransactionRecord
			if err := readStrictJSONPath(path, &removed); err != nil {
				t.Fatal(err)
			}
			if len(removed.Revisions) != 3 || !reflect.DeepEqual(original.Revisions, removed.Revisions[:2]) {
				t.Fatal("removal rewrote history")
			}
			validateStoredSlideSchema(t, removed)
			last, err := removed.CurrentRevision()
			if err != nil || !last.Deleted || !reflect.DeepEqual(last.ParentSnapshots, []string{updated.Snapshot}) {
				t.Fatalf("removal event: %+v, %v", last, err)
			}
			for _, revision := range original.Revisions {
				asset := filepath.Join(filepath.Dir(path), revision.Asset)
				data, err := os.ReadFile(asset)
				if err != nil || string(data) != before[asset] {
					t.Fatalf("historical asset changed: %s: %v", asset, err)
				}
			}
			document, validation, err := saga.Load(root)
			if err != nil || !validation.Valid || findSlide(document, "flow") != nil || findDeck(document, "unrelated") == nil {
				t.Fatalf("load after removal: %v, %+v", err, validation)
			}
			if wholeDeck {
				if findDeck(document, "implementation") != nil || findSlide(document, "second") != nil || findSlide(document, "legacy") != nil {
					t.Fatal("removed deck is still active")
				}
			} else {
				if findSlide(document, "second") == nil || findSlide(document, "legacy") == nil || before[filepath.Join(root, other.Path)] != snapshotFiles(t, root)[filepath.Join(root, other.Path)] {
					t.Fatal("slide removal changed a sibling")
				}
				if _, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false); err == nil || !strings.Contains(err.Error(), "removed") {
					t.Fatalf("create overwrote deleted history: %v", err)
				}
				if err := AddSlide(context.Background(), []string{"--deck", "implementation", "--intent", "explain", "--layout", "diagram", root, "flow"}, io.Discard); err == nil || !strings.Contains(err.Error(), "retained transaction history") {
					t.Fatalf("legacy create shadowed history: %v", err)
				}
				// Removing the deck later must retain already-deleted slide history.
				if _, err := runContentJSON(t, RemoveDeck, "--deck", "implementation", root); err != nil {
					t.Fatal(err)
				}
				var retained saga.SlideTransactionRecord
				if err := readStrictJSONPath(path, &retained); err != nil || !reflect.DeepEqual(removed, retained) {
					t.Fatalf("deck removal lost earlier tombstone: %v", err)
				}
			}
		})
	}
}

func validateStoredSlideSchema(t *testing.T, record saga.SlideTransactionRecord) {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"v5/slide-transaction.schema.json", "v5/slide-transaction-request.schema.json", "v5/diagram.schema.json", "v4/slide.schema.json", "v4/item.schema.json", "v2/code.schema.json"} {
		data, err := os.ReadFile(filepath.Join("../../schema", name))
		if err != nil {
			t.Fatal(err)
		}
		var resource any
		if err := json.Unmarshal(data, &resource); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource("https://changesaga.dev/schema/"+name, resource); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("https://changesaga.dev/schema/v5/slide-transaction.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatalf("stored history does not match schema: %v", err)
	}
}

func TestRemoveTransactionDeckRollsBackOnConflictingSlide(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "create", "create", "absent", "node-a")
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false); err != nil {
		t.Fatal(err)
	}
	create.RequestID, create.Slide.ID, create.Slide.Rank = "second", "second", 10
	create.Items[0].CriterionLinks[0].ID = "second-explains-returns"
	second, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, second.Path)
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"left", "right"} {
		revision := record.Revisions[0]
		revision.RequestID, revision.Slide.Title = id, id
		revision.ParentSnapshots = []string{second.Snapshot}
		revision.Snapshot, _ = saga.SlideRevisionSnapshot(revision)
		record.Revisions = append(record.Revisions, revision)
		record.Current = revision.Snapshot
	}
	if err := store.WriteJSON(path, record, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root)
	var output bytes.Buffer
	if err := RemoveDeck(context.Background(), []string{"--deck", "implementation", root}, &output); err == nil || !strings.Contains(err.Error(), "reconcile") {
		t.Fatalf("conflicting removal: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("deck removal did not roll back the first slide after the second conflicted")
	}
}

func TestRemoveTransactionMergeExposesLiveHeadForReconcile(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, created.Path)
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		t.Fatal(err)
	}
	live := record.Revisions[0]
	live.ParentSnapshots = []string{created.Snapshot}
	live.RequestID, live.Slide.Title = "concurrent-edit", "Concurrent edit"
	live.Snapshot, _ = saga.SlideRevisionSnapshot(live)
	record.Revisions = append(record.Revisions, live)
	// Keep Current pointing to the deleted head, the dangerous merge case.
	if err := store.WriteJSON(path, record, false); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("merged load: %v %+v", err, validation.Issues)
	}
	slide := findSlide(document, "flow")
	if slide == nil || !slide.AuthoringConflict || slide.Title != live.Slide.Title {
		t.Fatalf("removal hid a concurrent edit: %+v", slide)
	}
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "resolve", "reconcile", "", "node-b")
	request.ExpectedSnapshots = slide.AuthoringHeads
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, io.Discard); err != nil {
		t.Fatalf("remove reconciled slide: %v", err)
	}
}

func TestRemoveTransactionValidationRejectsAlteredContent(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, created.Path)
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		t.Fatal(err)
	}
	last := &record.Revisions[len(record.Revisions)-1]
	last.Slide.Title = "Changed during deletion"
	last.Snapshot, _ = saga.SlideRevisionSnapshot(*last)
	record.Current = last.Snapshot
	if err := store.WriteJSON(path, record, false); err != nil {
		t.Fatal(err)
	}
	_, validation, err := saga.Load(root)
	if err != nil || validation.Valid {
		t.Fatalf("invalid removal accepted: %v %+v", err, validation)
	}
}
