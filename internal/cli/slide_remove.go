package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
)

// The caller holds the Saga lock. Transaction removals append a tombstone to
// the same publication record; assets and previous revisions remain intact.
func removeDocumentationSlide(document *saga.Saga, deck *saga.Deck, slide *saga.Slide, edit *contentEdit) error {
	if !transactionManagedSlide(slide) {
		files := []string{filepath.Join(document.Root, filepath.FromSlash(slide.Path)), filepath.Join(slide.Directory, slide.Entrypoint)}
		for _, item := range slide.Items {
			files = append(files, itemFiles(document.Root, item)...)
		}
		return removeFiles(edit, files)
	}
	path := filepath.Join(document.Root, filepath.FromSlash(slide.Path))
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		return err
	}
	heads, err := record.Heads()
	if err != nil {
		return err
	}
	if len(heads) != 1 || heads[0] != record.Current {
		return fmt.Errorf("slide %s has divergent or non-current transaction heads; reconcile with apply-slide before removing it", slide.ID)
	}
	previous, err := record.CurrentRevision()
	if err != nil {
		return err
	}
	if previous.Deleted {
		return fmt.Errorf("slide %s is already removed", slide.ID)
	}
	revision := *previous
	revision.Deleted = true
	revision.ParentSnapshots = []string{previous.Snapshot}
	// Identity depends on the parent rather than the clock, making retries and
	// parallel branches that remove the same snapshot agree on the event.
	revision.RequestID = "remove-" + strings.TrimPrefix(previous.Snapshot, "sha256:")
	revision.CreatedAt = time.Now().UTC()
	revision.Snapshot, err = saga.SlideRevisionSnapshot(revision)
	if err != nil {
		return err
	}
	record.Current = revision.Snapshot
	record.Revisions = append(record.Revisions, revision)
	if len(record.Revisions) > saga.MaxSlideRevisions {
		return fmt.Errorf("slide revision limit of %d reached", saga.MaxSlideRevisions)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(encoded) > saga.MaxSlideTransactionBytes {
		return fmt.Errorf("slide transaction exceeds the %d-byte limit", saga.MaxSlideTransactionBytes)
	}
	validation := saga.ValidateSlideTransactionCandidate(document.Root, deck, record)
	if !validation.Valid {
		return fmt.Errorf("invalid slide removal: %v", validation.Issues)
	}
	return edit.writeJSON(path, record)
}
