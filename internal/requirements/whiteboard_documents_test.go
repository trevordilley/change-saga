package requirements

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/store"
)

func TestWhiteboardNamedDocumentsHaveIndependentCurrentContent(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	doc := boardDocument(pin)
	ctx := context.Background()
	for _, id := range []string{"zeta", "Architecture.v2", "data-model"} {
		if _, err := PublishWhiteboard(ctx, root, "test", id, "r1", "", id, doc); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ListWhiteboardIDs(root)
	if err != nil || !reflect.DeepEqual(ids, []string{"Architecture.v2", "data-model", "zeta"}) {
		t.Fatalf("named boards = %v, %v", ids, err)
	}
	sibling := filepath.Join(root, WhiteboardDir, "zeta", currentrecord.Name)
	before, err := os.ReadFile(sibling)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishWhiteboard(ctx, root, "test", "data-model", "r2", "r1", "Updated model", doc); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		v, err := LoadWhiteboard(root, "test", id, "")
		want := id
		if id == "data-model" {
			want = "Updated model"
		}
		if err != nil || v == nil || v.Name != want {
			t.Fatalf("%s head = %+v, %v", id, v, err)
		}
		if err := ValidateWhiteboard(root, "test", id); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"missing", "erd"} {
		if v, err := LoadResolvedWhiteboard(root, "test", id, ""); err != nil || v != nil {
			t.Fatalf("missing board %s = %+v, %v", id, v, err)
		}
		if _, err := LoadWhiteboard(root, "test", id, "r1"); err == nil {
			t.Fatal("exact missing board revision fell back to a sibling")
		}
		if err := ValidateWhiteboard(root, "test", id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := PublishWhiteboard(ctx, root, "test", "../invalid", "", "", "Invalid", doc); err == nil {
		t.Fatal("invalid stable board identity accepted")
	}
	if _, err := PublishWhiteboard(ctx, root, "test", "zeta", "ignored", "unknown", "zeta", doc); err != nil {
		t.Fatalf("obsolete parent controlled current authoring: %v", err)
	}

	invalidDoc := boardDocument(pin)
	invalidDoc.Elements[1].Source.Revision = pin.Target + ":revision:missing"
	if _, err := PublishWhiteboard(ctx, root, "test", "bad-source", "r1", "", "Invalid source", invalidDoc); err == nil {
		t.Fatal("accepted unresolved source in new sibling")
	}
	if _, err := os.Stat(filepath.Join(root, WhiteboardDir, "bad-source")); !os.IsNotExist(err) {
		t.Fatalf("failed source validation left a board: %v", err)
	}
	after, err := os.ReadFile(sibling)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("sibling history changed: %v", err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
	// Only the selected document is validated; whole-root validation still
	// catches an invalid history in any sibling.
	if err := os.WriteFile(sibling, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboard(root, "test", "data-model"); err != nil {
		t.Fatalf("selected validation loaded sibling history: %v", err)
	}
	if err := ValidateWhiteboards(root, "test"); err == nil {
		t.Fatal("all-board validation skipped malformed sibling history")
	}
}

func TestWhiteboardMissingStorageAndInvalidIDs(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	if ids, err := ListWhiteboardIDs(root); err != nil || len(ids) != 0 {
		t.Fatalf("absent board root = %v, %v", ids, err)
	}
	for _, id := range []string{"", "..", "../escape", "a/b", "a\\b", "with space", strings.Repeat("a", 129)} {
		if _, err := LoadWhiteboard(root, "test", id, ""); err == nil {
			t.Errorf("load accepted invalid ID %q", id)
		}
		if err := ValidateWhiteboard(root, "test", id); err == nil {
			t.Errorf("validation accepted invalid ID %q", id)
		}
		if _, err := PublishWhiteboard(context.Background(), root, "test", id, "r1", "", "Invalid", boardDocument(pin)); err == nil {
			t.Errorf("publication accepted invalid ID %q", id)
		}
	}
	if _, err := os.Stat(filepath.Join(root, WhiteboardDir)); !os.IsNotExist(err) {
		t.Fatalf("invalid IDs wrote board storage: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, WhiteboardDir), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishWhiteboard(context.Background(), root, "test", "my-board", "r1", "", "First board", boardDocument(pin)); err != nil {
		t.Fatalf("cannot publish into empty existing root: %v", err)
	}
}

func TestWhiteboardNamedStorageRejectsMalformedDirectories(t *testing.T) {
	for _, name := range []string{"root-file", "root-symlink", "board-file", "board-symlink", "invalid-directory", "staging-file", "staging-symlink", "unrelated-hidden-directory", "missing-revisions", "revisions-file", "revisions-symlink", "empty-revisions", "extra-entry"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			boards := filepath.Join(root, WhiteboardDir)
			board := filepath.Join(boards, "example")
			if err := os.MkdirAll(boards, 0755); err != nil {
				t.Fatal(err)
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "root-file", "root-symlink":
				must(os.Remove(boards))
				if name == "root-file" {
					must(os.WriteFile(boards, nil, 0600))
				} else {
					must(os.Symlink(t.TempDir(), boards))
				}
			case "board-file":
				must(os.WriteFile(board, nil, 0600))
			case "board-symlink":
				must(os.Symlink(t.TempDir(), board))
			case "staging-file":
				must(os.WriteFile(filepath.Join(boards, ".change-saga-stage-bad"), nil, 0600))
			case "staging-symlink":
				must(os.Symlink(t.TempDir(), filepath.Join(boards, ".change-saga-stage-bad")))
			case "unrelated-hidden-directory":
				must(os.Mkdir(filepath.Join(boards, ".unexpected"), 0755))
			case "invalid-directory":
				must(os.Mkdir(filepath.Join(boards, "bad id"), 0755))
			default:
				must(os.Mkdir(board, 0755))
				revisions := filepath.Join(board, "revisions")
				switch name {
				case "revisions-file":
					must(os.WriteFile(revisions, nil, 0600))
				case "revisions-symlink":
					must(os.Symlink(t.TempDir(), revisions))
				case "empty-revisions":
					must(os.Mkdir(revisions, 0755))
				case "extra-entry":
					must(os.Mkdir(revisions, 0755))
					must(os.WriteFile(filepath.Join(board, "other"), nil, 0600))
				}
			}
			if err := ValidateWhiteboards(root, "test"); err == nil {
				t.Fatal("accepted malformed storage")
			}
			if strings.HasPrefix(name, "root-") || strings.HasPrefix(name, "board-") || strings.HasPrefix(name, "staging-") || name == "unrelated-hidden-directory" || name == "invalid-directory" {
				if _, err := ListWhiteboardIDs(root); err == nil {
					t.Fatal("listed malformed board directory")
				}
			}
			if name != "invalid-directory" && !strings.HasPrefix(name, "staging-") && name != "unrelated-hidden-directory" {
				if _, err := LoadWhiteboard(root, "test", "example", ""); err == nil {
					t.Fatal("load accepted malformed selected storage")
				}
			}
		})
	}
}

func TestWhiteboardValidationUsesSelectedContentAndDropsObsoleteSourceHistory(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	v, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "architecture", "r1", "", "Architecture", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "architecture", "r2", "r1", "Architecture", boardDocument(pin)); err != nil {
		t.Fatal(err)
	}
	v.Document.Elements[1].Source.Revision = pin.Target + ":revision:missing"
	if err := store.WriteJSON(filepath.Join(root, WhiteboardDir, "architecture", "revisions", "r1.json"), structLegacyWhiteboard(*v), false); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResolvedWhiteboard(root, "test", "architecture", ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboard(root, "test", "architecture"); err != nil {
		t.Fatalf("an obsolete source pin blocked current validation: %v", err)
	}
	commitCurrencyFixture(t, root)
	if err := currentrecord.Normalize(root); err != nil {
		t.Fatal(err)
	}
	selected, err := LoadResolvedWhiteboard(root, "test", "architecture", "")
	if err != nil || selected == nil || selected.Revision.Document.Elements[1].Source.Revision != pin.Revision {
		t.Fatalf("migration changed the selected exact source: %+v %v", selected, err)
	}
	if _, err := os.Stat(filepath.Join(root, WhiteboardDir, "architecture", "revisions")); !os.IsNotExist(err) {
		t.Fatal("obsolete source history was retained beside current content")
	}
}

func TestWhiteboardConcurrentFirstPublicationsPreserveBothDocuments(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{"first", "second"} {
		go func() {
			<-start
			_, err := PublishWhiteboard(context.Background(), root, "test", id, "r1", "", id, boardDocument(pin))
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	ids, err := ListWhiteboardIDs(root)
	if err != nil || !reflect.DeepEqual(ids, []string{"first", "second"}) {
		t.Fatalf("concurrent publication lost a document: %v, %v", ids, err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardDocumentCountIsBounded(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	boards := filepath.Join(root, WhiteboardDir)
	if err := os.Mkdir(boards, 0755); err != nil {
		t.Fatal(err)
	}
	for i := range MaxWhiteboards {
		if err := os.Mkdir(filepath.Join(boards, fmt.Sprintf("board-%05d", i)), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A pending or abandoned store stage must not consume a board slot.
	if _, err := os.MkdirTemp(boards, ".change-saga-stage-"); err != nil {
		t.Fatal(err)
	}
	if ids, err := ListWhiteboardIDs(root); err != nil || len(ids) != MaxWhiteboards {
		t.Fatalf("listing at limit = %d, %v", len(ids), err)
	}
	if _, err := PublishWhiteboard(context.Background(), root, "test", "overflow", "r1", "", "Overflow", boardDocument(pin)); err == nil || !strings.Contains(err.Error(), "document limit") {
		t.Fatalf("publication failed to enforce board limit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(boards, "overflow")); !os.IsNotExist(err) {
		t.Fatalf("overflow publication wrote a directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(boards, "overflow"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ListWhiteboardIDs(root); err == nil {
		t.Fatal("listing accepted too many boards")
	}
	if err := ValidateWhiteboards(root, "test"); err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("validation skipped board limit: %v", err)
	}
}

func TestWhiteboardReadersIgnoreIncompletePublicationStage(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	original, err := PublishWhiteboard(context.Background(), root, "test", "original", "r1", "", "Original", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	next := *original
	next.Name = "Next document"
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	resume := func() { releaseOnce.Do(func() { close(release) }) }
	defer resume()
	result := make(chan error, 1)
	go func() {
		result <- store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
			return store.CommitDir(root, filepath.Join(root, WhiteboardDir, "next"), func(stage string) error {
				close(entered)
				<-release // Readers see an actual, incomplete CommitDir stage.
				return currentrecord.Write(stage, next, nil)
			})
		})
	}()
	select {
	case <-entered:
	case err := <-result:
		t.Fatalf("publication did not enter populate: %v", err)
	}
	if ids, err := ListWhiteboardIDs(root); err != nil || !reflect.DeepEqual(ids, []string{"original"}) {
		t.Fatalf("incomplete stage leaked into listing: %v, %v", ids, err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatalf("stage broke validation: %v", err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "original", ""); err != nil || board == nil {
		t.Fatalf("stage broke original board reads: %+v, %v", board, err)
	}
	if board, err := LoadWhiteboard(root, "test", "next", ""); err != nil || board != nil {
		t.Fatalf("uncommitted board became visible: %+v, %v", board, err)
	}
	resume()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if ids, err := ListWhiteboardIDs(root); err != nil || !reflect.DeepEqual(ids, []string{"next", "original"}) {
		t.Fatalf("committed board missing: %v, %v", ids, err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "next", ""); err != nil || board == nil || board.Revision.Name != next.Name {
		t.Fatalf("committed board unreadable: %+v, %v", board, err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardReadersKeepAtomicCurrentPublication(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	original, err := PublishWhiteboard(context.Background(), root, "test", "board", "", "", "Original", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	next := *original
	next.Name = "Updated"
	content, err := currentrecord.Content(next)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(currentrecord.Record{Schema: currentrecord.Schema, Version: 1, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, WhiteboardDir, "board")
	temp, err := os.CreateTemp(dir, ".change-saga-write-*")
	if err != nil {
		t.Fatal(err)
	}
	defer temp.Close()
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data[:len(data)/2]); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatalf("partial write broke current validation: %v", err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "board", ""); err != nil || board == nil || board.Revision.Name != "Original" {
		t.Fatalf("partial write became visible: %+v %v", board, err)
	}
	nextDigest, _ := WhiteboardContentDigest(next)
	if _, err := LoadWhiteboard(root, "test", "board", nextDigest); err == nil {
		t.Fatal("unpublished content became visible")
	}
	if _, err := temp.Write(data[len(data)/2:]); err != nil {
		t.Fatal(err)
	}
	if err := temp.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp.Name(), filepath.Join(dir, currentrecord.Name)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "board", ""); err != nil || board == nil || board.Revision.Name != "Updated" {
		t.Fatalf("published content unreadable: %+v %v", board, err)
	}
}

func TestWhiteboardRevisionDiscoveryRejectsTemporaryImpostors(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "unrelated-file", "only-temporary"} {
		t.Run(kind, func(t *testing.T) {
			root, pin, _ := whiteboardFixture(t)
			if _, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "board", "r1", "", "Board", boardDocument(pin)); err != nil {
				t.Fatal(err)
			}
			revisions := filepath.Join(root, WhiteboardDir, "board", "revisions")
			temp := filepath.Join(revisions, ".change-saga-write-impostor")
			var err error
			switch kind {
			case "directory":
				err = os.Mkdir(temp, 0755)
			case "symlink":
				err = os.Symlink(filepath.Join(revisions, "r1.json"), temp)
			case "unrelated-file":
				err = os.WriteFile(filepath.Join(revisions, ".unexpected"), nil, 0600)
			case "only-temporary":
				if err := os.Remove(filepath.Join(revisions, "r1.json")); err != nil {
					t.Fatal(err)
				}
				err = os.WriteFile(temp, nil, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateWhiteboard(root, "test", "board"); err == nil {
				t.Fatal("accepted malformed revision storage")
			}
		})
	}
}

func TestWhiteboardRevisionDiscoveryLimitAllowsPendingWrite(t *testing.T) {
	dir := t.TempDir()
	for i := range MaxRevisionsPerStory {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("r%05d.json", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	temp, err := os.CreateTemp(dir, ".change-saga-write-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := temp.Close(); err != nil {
		t.Fatal(err)
	}
	if files, err := whiteboardRevisionFiles(dir); err != nil || len(files) != MaxRevisionsPerStory {
		t.Fatalf("pending write consumed a revision slot: %d, %v", len(files), err)
	}
	if err := os.WriteFile(filepath.Join(dir, "overflow.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := whiteboardRevisionFiles(dir); err == nil {
		t.Fatal("revision count limit was lost")
	}
}

func TestWhiteboardRevisionIDsFollowOneSelectedHistory(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	parent := ""
	for _, id := range []string{"r1", "r10", "r2"} {
		if _, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "planning", id, parent, "Planning", boardDocument(pin)); err != nil {
			t.Fatal(err)
		}
		parent = id
	}
	if _, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "other", "r99", "", "Other", boardDocument(pin)); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		board string
		want  []string
	}{{"planning", []string{"r2", "r10", "r1"}}, {"other", []string{"r99"}}, {"missing", nil}} {
		ids, err := ListWhiteboardRevisionIDs(root, "test", test.board)
		if err != nil || !reflect.DeepEqual(ids, test.want) {
			t.Fatalf("%s revision IDs = %v, %v; want %v", test.board, ids, err, test.want)
		}
	}
	if _, err := ListWhiteboardRevisionIDs(root, "test", "../planning"); err == nil {
		t.Fatal("accepted invalid board ID")
	}
	// Listing validates saved documents and parent links without resolving the
	// inventory pins or rendering compositions, just like an unresolved read.
	if err := os.Remove(filepath.Join(root, TechnicalPath(KindERD, "jobs"), currentrecord.Name)); err != nil {
		t.Fatal(err)
	}
	if ids, err := ListWhiteboardRevisionIDs(root, "test", "planning"); err != nil || len(ids) != 3 {
		t.Fatalf("history listing tried to resolve source pins: %v, %v", ids, err)
	}
}

func TestWhiteboardRevisionIDsRejectInvalidOrConflictingHistory(t *testing.T) {
	for _, problem := range []string{"missing-parent", "second-root", "conflicting-heads", "malformed-revision"} {
		t.Run(problem, func(t *testing.T) {
			root, pin, _ := whiteboardFixture(t)
			first, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "planning", "r1", "", "Planning", boardDocument(pin))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := publishLegacyWhiteboardFixture(context.Background(), root, "test", "planning", "r2", "r1", "Planning", boardDocument(pin)); err != nil {
				t.Fatal(err)
			}
			bad := *first
			bad.ID, bad.Parents = "bad", []string{"r1"}
			switch problem {
			case "missing-parent":
				bad.Parents = []string{"missing"}
			case "second-root":
				bad.Parents = []string{}
			case "malformed-revision":
				bad.Version = 99
			}
			if err := store.WriteJSON(filepath.Join(root, WhiteboardDir, "planning", "revisions", "bad.json"), structLegacyWhiteboard(bad), true); err != nil {
				t.Fatal(err)
			}
			if ids, err := ListWhiteboardRevisionIDs(root, "test", "planning"); err == nil || ids != nil {
				t.Fatalf("accepted %s: %v, %v", problem, ids, err)
			}
		})
	}
}
