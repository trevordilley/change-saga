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

	"github.com/twentyideas/changesaga/internal/store"
)

func TestWhiteboardNamedDocumentsHaveIndependentHistories(t *testing.T) {
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
	sibling := filepath.Join(root, WhiteboardDir, "zeta", "revisions", "r1.json")
	before, err := os.ReadFile(sibling)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishWhiteboard(ctx, root, "test", "data-model", "r2", "r1", "Updated model", doc); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		v, err := LoadWhiteboard(root, "test", id, "")
		want := "r1"
		if id == "data-model" {
			want = "r2"
		}
		if err != nil || v == nil || v.ID != want {
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
	for _, update := range []struct{ board, revision, parent string }{
		{"missing", "r1", "r1"}, // A sibling revision is not this board's parent.
		{"zeta", "r3", "r2"},
		{"data-model", "r3", "r1"},
	} {
		if _, err := PublishWhiteboard(ctx, root, "test", update.board, update.revision, update.parent, "Invalid", doc); err == nil {
			t.Fatalf("accepted invalid parent: %+v", update)
		}
	}
	if _, err := os.Stat(filepath.Join(root, WhiteboardDir, "missing")); !os.IsNotExist(err) {
		t.Fatalf("failed first publication left a board: %v", err)
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

func TestWhiteboardValidationChecksHistoricalPinsForSelectedDocument(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	v, err := PublishWhiteboard(context.Background(), root, "test", "architecture", "r1", "", "Architecture", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishWhiteboard(context.Background(), root, "test", "architecture", "r2", "r1", "Architecture", boardDocument(pin)); err != nil {
		t.Fatal(err)
	}
	v.Document.Elements[1].Source.Revision = pin.Target + ":revision:missing"
	if err := store.WriteJSON(filepath.Join(root, WhiteboardDir, "architecture", "revisions", "r1.json"), v, false); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadResolvedWhiteboard(root, "test", "architecture", "r2"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboard(root, "test", "architecture"); err == nil {
		t.Fatal("selected validation ignored historical source pin")
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
				revisions := filepath.Join(stage, "revisions")
				if err := os.Mkdir(revisions, 0755); err != nil {
					return err
				}
				close(entered)
				<-release // Readers see an actual, incomplete CommitDir stage.
				return store.WriteJSON(filepath.Join(revisions, "r1.json"), next, true)
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

func TestWhiteboardReadersIgnoreIncompleteRevisionWrite(t *testing.T) {
	root, pin, _ := whiteboardFixture(t)
	original, err := PublishWhiteboard(context.Background(), root, "test", "board", "r1", "", "Original", boardDocument(pin))
	if err != nil {
		t.Fatal(err)
	}
	next := *original
	next.ID, next.Parents, next.Name = "r2", []string{"r1"}, "Updated"
	revisions := filepath.Join(root, WhiteboardDir, "board", "revisions")
	// Pause the same temporary-file/hard-link protocol used by store.WriteFile
	// after writing only a JSON prefix. A reader must never parse this file.
	temp, err := os.CreateTemp(revisions, ".change-saga-write-*")
	if err != nil {
		t.Fatal(err)
	}
	defer temp.Close()
	defer os.Remove(temp.Name())
	data, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temp.Write(data[:len(data)/2]); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatalf("partial write broke validation: %v", err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "board", ""); err != nil || board == nil || board.Revision.ID != "r1" {
		t.Fatalf("partial write changed visible head: %+v, %v", board, err)
	}
	if _, err := LoadWhiteboard(root, "test", "board", "r2"); err == nil {
		t.Fatal("uncommitted revision became visible")
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
	if err := os.Link(temp.Name(), filepath.Join(revisions, "r2.json")); err != nil {
		t.Fatal(err)
	}
	// The temp can briefly coexist with its complete published hard link.
	if err := ValidateWhiteboards(root, "test"); err != nil {
		t.Fatal(err)
	}
	if board, err := LoadResolvedWhiteboard(root, "test", "board", ""); err != nil || board == nil || board.Revision.ID != "r2" {
		t.Fatalf("committed revision unreadable: %+v, %v", board, err)
	}
}

func TestWhiteboardRevisionDiscoveryRejectsTemporaryImpostors(t *testing.T) {
	for _, kind := range []string{"directory", "symlink", "unrelated-file", "only-temporary"} {
		t.Run(kind, func(t *testing.T) {
			root, pin, _ := whiteboardFixture(t)
			if _, err := PublishWhiteboard(context.Background(), root, "test", "board", "r1", "", "Board", boardDocument(pin)); err != nil {
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
