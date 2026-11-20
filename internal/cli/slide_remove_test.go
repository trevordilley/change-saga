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
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestRemoveCurrentSlidePreservesAssetsAndGitHistory(t *testing.T) {
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
			savedCommit := commitCurrentTestDocumentation(t, root, "current slides before removal")
			originalBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
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
			if len(removed.Revisions) != 1 {
				t.Fatal("secondary content history retained")
			}
			if got := committedCurrentTestFile(t, root, savedCommit, updated.Path); got != string(originalBytes) {
				t.Fatal("Git lost the prior slide")
			}
			validateStoredSlideSchema(t, removed)
			last, err := removed.CurrentRevision()
			if err != nil || !last.Deleted || len(last.ParentSnapshots) != 0 {
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
	for _, name := range []string{"v5/slide-current.schema.json", "v5/slide-transaction-request.schema.json", "v5/diagram.schema.json", "v4/slide.schema.json", "v4/item.schema.json", "v2/code.schema.json"} {
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
	schema, err := compiler.Compile("https://changesaga.dev/schema/v5/slide-current.schema.json")
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
	writeLegacySlideConflictTestFixture(t, path, record, false)
	before := snapshotFiles(t, root)
	var output bytes.Buffer
	if err := RemoveDeck(context.Background(), []string{"--deck", "implementation", root}, &output); err == nil || (!strings.Contains(err.Error(), "legacy") && !strings.Contains(err.Error(), "conflict")) {
		t.Fatalf("conflicting removal: %v", err)
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("deck removal did not roll back the first slide after the second conflicted")
	}
}

func TestLegacyRemovalCannotHideIndependentlyAuthoredLiveContent(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "create", "create", "absent", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, created.Path)
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		t.Fatal(err)
	}
	writeLegacySlideConflictTestFixture(t, path, record, true)
	before := snapshotFiles(t, root)
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, io.Discard); err == nil {
		t.Fatal("removal chose between competing legacy live/deleted values")
	}
	if !reflect.DeepEqual(before, snapshotFiles(t, root)) {
		t.Fatal("failed removal discarded legacy content")
	}
	// Preserve the competing file in Git, then explicitly select complete live content.
	commitCurrentTestDocumentation(t, root, "unresolved legacy slide branches")
	selected := record.Revisions[0]
	selected.Deleted = false
	selected.Snapshot, err = saga.SlideContentDigest(selected)
	if err != nil {
		t.Fatal(err)
	}
	record.Revisions = []saga.SlideTransactionRevision{selected}
	record.Current = selected.Snapshot
	if err := store.WriteJSON(path, record, false); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSlide(context.Background(), []string{"--slide", "flow", root}, io.Discard); err != nil {
		t.Fatalf("remove explicit selected content: %v", err)
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
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err = json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	wire["content"].(map[string]any)["parent_snapshots"] = []string{created.Snapshot}
	data, _ := json.Marshal(wire)
	if err = os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	_, validation, err := saga.Load(root)
	if err != nil || validation.Valid {
		t.Fatalf("invalid removal accepted: %v %+v", err, validation)
	}
}

// Actual old bytes exercise ambiguity handling without asking current writers
// to produce a graph or a second history store.
func writeLegacySlideConflictTestFixture(t *testing.T, path string, record saga.SlideTransactionRecord, deleted bool) {
	t.Helper()
	root := record.Revisions[0]
	root.Deleted = false
	root.ParentSnapshots = nil
	root.RequestID = "legacy-root"
	root.CreatedAt = time.Date(2026, 8, 31, 19, 0, 0, 0, time.UTC)
	root.Snapshot, _ = saga.SlideRevisionSnapshot(root)
	revisions := []saga.SlideTransactionRevision{root}
	chosen := ""
	for _, id := range []string{"left", "right"} {
		child := root
		child.RequestID = id
		child.ParentSnapshots = []string{root.Snapshot}
		if deleted && id == "left" {
			child.Deleted = true
		} else {
			child.Slide.Title = id
		}
		child.Snapshot, _ = saga.SlideRevisionSnapshot(child)
		revisions = append(revisions, child)
		if chosen == "" {
			chosen = child.Snapshot
		}
	}
	type legacy saga.SlideTransactionRecord
	record.Version = 1
	record.Revisions = revisions
	record.Current = chosen
	if err := store.WriteJSON(path, legacy(record), false); err != nil {
		t.Fatal(err)
	}
}
