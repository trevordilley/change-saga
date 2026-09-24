package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagaref"
	"github.com/twentyideas/changesaga/internal/store"
)

// SlideTransactionRequest is the public structured authoring contract for one
// complete implementation slide. Every list is a complete replacement, not a
// patch, so validation happens before one commit record makes it visible.
type SlideTransactionRequest struct {
	Version           int                           `json:"version"`
	Operation         string                        `json:"operation"`
	RequestID         string                        `json:"request_id"`
	Deck              string                        `json:"deck"`
	ExpectedSnapshot  string                        `json:"expected_snapshot,omitempty"`
	ExpectedSnapshots []string                      `json:"expected_snapshots,omitempty"`
	Slide             SlideTransactionSlide         `json:"slide"`
	Asset             SlideTransactionAsset         `json:"asset"`
	Items             []SlideTransactionItemRequest `json:"items"`
}

type SlideTransactionSlide struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Rank               int      `json:"rank"`
	Section            string   `json:"section,omitempty"`
	Intent             string   `json:"intent"`
	Layout             string   `json:"layout"`
	MediaType          string   `json:"media_type"`
	Takeaway           string   `json:"takeaway"`
	ReadingOrder       []string `json:"reading_order"`
	ExceptionRationale string   `json:"exception_rationale,omitempty"`
}

type SlideTransactionAsset struct {
	Path          string `json:"path,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
}

type SlideTransactionItemRequest struct {
	ID             string                `json:"id"`
	Rank           int                   `json:"rank"`
	Kind           string                `json:"kind"`
	Label          string                `json:"label"`
	Description    string                `json:"description"`
	Selector       saga.LandmarkSelector `json:"selector"`
	Hotspot        *saga.LandmarkRegion  `json:"hotspot,omitempty"`
	About          string                `json:"about,omitempty"`
	Body           string                `json:"body,omitempty"`
	Placement      string                `json:"placement,omitempty"`
	Leader         string                `json:"leader,omitempty"`
	Evidence       []saga.CodeFile       `json:"evidence"`
	CriterionLinks []saga.CriterionLink  `json:"criterion_links"`
}

type SlideSemanticDiff struct {
	AssetChanged    bool     `json:"asset_changed"`
	CreatedItems    []string `json:"created_items"`
	UpdatedItems    []string `json:"updated_items"`
	RemovedItems    []string `json:"removed_items"`
	SelectorChanges []string `json:"selector_changes"`
}

type SlideTransactionResult struct {
	OK               bool              `json:"ok"`
	Operation        string            `json:"operation"`
	DryRun           bool              `json:"dry_run"`
	Replayed         bool              `json:"replayed"`
	Target           string            `json:"target"`
	PreviousSnapshot string            `json:"previous_snapshot,omitempty"`
	Snapshot         string            `json:"snapshot"`
	ChangedIDs       []string          `json:"changed_ids"`
	Diff             SlideSemanticDiff `json:"semantic_diff"`
	Path             string            `json:"path"`
}

var slideTransactionFault func(string) error
var writeSlideTransactionRecord = store.WriteJSON

func transactionManagedSlide(slide *saga.Slide) bool {
	return slide != nil && strings.HasPrefix(filepath.Base(filepath.FromSlash(slide.Path)), "25-t-")
}

func completeSlideMutationError(operation, target string) error {
	slideTarget, _, _ := strings.Cut(target, ":item:")
	return fmt.Errorf("%s cannot safely mutate %s because it is managed by complete-slide transaction history; read authoring_snapshot, authoring_heads, and items (including evidence and criterion_links) with `change-saga query slide --saga PATH --target %s`, then submit the complete replacement with `change-saga apply-slide --from REQUEST.json PATH`", operation, target, slideTarget)
}

func guardCompleteSlideMutation(operation string, slide *saga.Slide) error {
	if transactionManagedSlide(slide) {
		return completeSlideMutationError(operation, slide.Target)
	}
	return nil
}

// ApplySlideTransaction validates and applies request. Atomicity is one slide
// transaction record: an asset is committed first under its content digest,
// then the record is atomically created/replaced. The operation does not claim
// atomicity with unrelated Saga records or arbitrary filesystem writes.
func ApplySlideTransaction(ctx context.Context, root, requestBase, repo string, request SlideTransactionRequest, dryRun bool) (SlideTransactionResult, error) {
	asset, extension, err := readSlideTransactionAsset(requestBase, request.Asset)
	if err != nil {
		return SlideTransactionResult{}, err
	}
	if len(asset) > saga.MaxSlideAssetBytes {
		return SlideTransactionResult{}, fmt.Errorf("slide asset exceeds %d bytes", saga.MaxSlideAssetBytes)
	}
	if request.Version != saga.SlideTransactionVersion || (request.Operation != "create" && request.Operation != "update" && request.Operation != "reconcile") {
		return SlideTransactionResult{}, fmt.Errorf("request requires version %d and operation create, update, or reconcile", saga.SlideTransactionVersion)
	}
	if !livingid.ValidID(request.RequestID) {
		return SlideTransactionResult{}, fmt.Errorf("request_id must be a stable identifier")
	}
	if request.Operation == "create" && (request.ExpectedSnapshot != "absent" || len(request.ExpectedSnapshots) != 0) {
		return SlideTransactionResult{}, fmt.Errorf("create requires expected_snapshot %q", "absent")
	}
	if request.Operation == "update" && (!validSnapshotSet([]string{request.ExpectedSnapshot}) || len(request.ExpectedSnapshots) != 0) {
		return SlideTransactionResult{}, fmt.Errorf("update requires the exact sha256 expected_snapshot returned by the previous transaction")
	}
	if request.Operation == "reconcile" {
		if request.ExpectedSnapshot != "" || len(request.ExpectedSnapshots) < 2 || !validSnapshotSet(request.ExpectedSnapshots) {
			return SlideTransactionResult{}, fmt.Errorf("reconcile requires expected_snapshot to be empty and expected_snapshots to contain every distinct divergent sha256 head")
		}
	}
	assetName, err := saga.SlideAssetFilename(asset, extension)
	if err != nil {
		return SlideTransactionResult{}, err
	}
	revision := buildSlideTransactionRevision(request, assetName, coderef.DigestBytes(asset))
	if err := verifyTransactionEvidence(ctx, firstNonEmpty(repo, root), revision.Items); err != nil {
		return SlideTransactionResult{}, err
	}

	var result SlideTransactionResult
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, _, loadErr := saga.Load(root)
		if loadErr != nil {
			return loadErr
		}
		deck := findDeck(document, request.Deck)
		if deck == nil {
			return fmt.Errorf("deck %q does not exist", request.Deck)
		}
		if deck.Role != saga.DeckRoleChange {
			return fmt.Errorf("complete-slide transactions are scoped to feature implementation decks")
		}
		revision.Slide.DeckID = deck.ID
		if reason := transactionMediaExtension(request.Slide.MediaType, extension); reason != "" {
			return fmt.Errorf("asset: %s", reason)
		}
		target := saga.SlideTarget(document.Manifest.ID, request.Slide.ID)
		existing := findSlide(document, target)
		recordPath := filepath.Join(deck.Directory, saga.SlideTransactionFilename(deck.Target, target))
		record := saga.SlideTransactionRecord{Version: saga.SlideTransactionVersion, DeckID: deck.ID, SlideID: request.Slide.ID, Revisions: []saga.SlideTransactionRevision{}}
		var previous *saga.SlideTransactionRevision
		var heads []string
		recordExists := false
		if existing != nil && strings.HasPrefix(filepath.Base(existing.Path), "25-t-") {
			recordExists = true
			if err := readStrictJSONPath(recordPath, &record); err != nil {
				return err
			}
			previous, err = record.CurrentRevision()
			if err != nil {
				return err
			}
			heads, err = record.Heads()
			if err != nil {
				return err
			}
		} else if existing != nil {
			legacy, legacyErr := legacySlideRevision(existing)
			if legacyErr != nil {
				return legacyErr
			}
			previous = &legacy
			record.Revisions = append(record.Revisions, legacy)
			heads = []string{legacy.Snapshot}
		}

		// Idempotency precedes create/update existence checks. The same request
		// may be retried after a response was lost; a reused ID with any changed
		// complete payload or operation is always rejected.
		for storedIndex, stored := range record.Revisions {
			if stored.RequestID != request.RequestID {
				continue
			}
			candidate := revision
			candidate.ParentSnapshots = append([]string{}, stored.ParentSnapshots...)
			candidate.Snapshot, err = saga.SlideRevisionSnapshot(candidate)
			if err != nil {
				return err
			}
			storedOperation := transactionOperationForParents(stored.ParentSnapshots)
			if len(stored.ParentSnapshots) == 0 && storedIndex > 0 {
				storedOperation = "update"
			}
			if stored.Snapshot != candidate.Snapshot || storedOperation != request.Operation || !storedExpectationMatches(request, record, storedIndex) {
				return fmt.Errorf("request_id %q was already used with a different complete-slide payload or operation", request.RequestID)
			}
			result = transactionResult(document.Manifest.ID, target, recordPath, root, request.Operation, dryRun, true, &stored, &stored)
			result.PreviousSnapshot = storedPreviousSnapshot(record, storedIndex)
			return nil
		}
		if err := validateTransactionCriteria(document, revision.Items); err != nil {
			return err
		}

		if request.Operation == "create" {
			if existing != nil || targetIDExists(document, request.Slide.ID) {
				return fmt.Errorf("slide id %q already exists", request.Slide.ID)
			}
			revision.ParentSnapshots = []string{}
		} else if request.Operation == "update" {
			if existing == nil {
				return fmt.Errorf("slide %q does not exist", request.Slide.ID)
			}
			if existing.DeckID != deck.ID {
				return fmt.Errorf("slide %q belongs to deck %q, not %q", request.Slide.ID, existing.DeckID, deck.ID)
			}
			if len(heads) > 1 {
				return fmt.Errorf("slide transaction has divergent heads %s; preserve every revision and submit operation reconcile with expected_snapshots containing every head", strings.Join(heads, ", "))
			}
			if len(heads) != 1 || request.ExpectedSnapshot != heads[0] {
				actual := "absent"
				if len(heads) == 1 {
					actual = heads[0]
				}
				return fmt.Errorf("expected_snapshot mismatch: got %q, current is %q", request.ExpectedSnapshot, actual)
			}
			revision.ParentSnapshots = []string{heads[0]}
		} else {
			if !recordExists || existing == nil {
				return fmt.Errorf("reconcile requires an existing complete-slide transaction with divergent heads")
			}
			if len(heads) < 2 {
				return fmt.Errorf("slide transaction has no divergent heads to reconcile; use operation update with expected_snapshot %q", firstSnapshot(heads))
			}
			if !sameSnapshotSet(request.ExpectedSnapshots, heads) {
				return fmt.Errorf("expected_snapshots mismatch: got %s, divergent heads are %s", strings.Join(sortedSnapshotCopy(request.ExpectedSnapshots), ", "), strings.Join(heads, ", "))
			}
			revision.ParentSnapshots = append([]string{}, heads...)
		}

		revision.Snapshot, err = saga.SlideRevisionSnapshot(revision)
		if err != nil {
			return err
		}
		revision.CreatedAt = time.Now().UTC()
		record.Current = revision.Snapshot
		record.Revisions = append(record.Revisions, revision)
		if len(record.Revisions) > saga.MaxSlideRevisions {
			return fmt.Errorf("slide revision limit of %d reached", saga.MaxSlideRevisions)
		}
		encodedRecord, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			return marshalErr
		}
		if len(encodedRecord) > saga.MaxSlideTransactionBytes {
			return fmt.Errorf("slide transaction exceeds the %d-byte limit", saga.MaxSlideTransactionBytes)
		}
		result = transactionResult(document.Manifest.ID, target, recordPath, root, request.Operation, dryRun, false, previous, &revision)
		if dryRun {
			return validateTransactionCandidateWithoutPublish(document.Root, deck, record, assetName, asset)
		}

		assetPath := filepath.Join(deck.Directory, assetName)
		assetCreated, writeErr := commitTransactionAsset(assetPath, asset)
		if writeErr != nil {
			return writeErr
		}
		rollbackAsset := func() {
			if assetCreated {
				_ = os.Remove(assetPath)
				_ = store.SyncDir(deck.Directory)
			}
		}
		validation := saga.ValidateSlideTransactionCandidate(document.Root, deck, record)
		if !validation.Valid {
			rollbackAsset()
			return transactionValidationError(validation)
		}
		if slideTransactionFault != nil {
			if faultErr := slideTransactionFault("before-record-commit"); faultErr != nil {
				rollbackAsset()
				return faultErr
			}
		}
		if err := writeSlideTransactionRecord(recordPath, record, !recordExists); err != nil {
			published, _, known := store.PublicationStatus(err)
			if !published {
				published = transactionSnapshotPublished(recordPath, revision.Snapshot)
			}
			if published {
				return fmt.Errorf("slide transaction snapshot %s was published, but directory durability could not be confirmed; retry the same request_id to verify the published result: %w", revision.Snapshot, err)
			}
			rollbackAsset()
			if known {
				return fmt.Errorf("slide transaction was not published: %w", err)
			}
			return err
		}
		return nil
	})
	return result, err
}

func transactionSnapshotPublished(path, snapshot string) bool {
	var record saga.SlideTransactionRecord
	if err := readStrictJSONPath(path, &record); err != nil {
		return false
	}
	return record.Current == snapshot
}

func buildSlideTransactionRevision(request SlideTransactionRequest, assetName, assetDigest string) saga.SlideTransactionRevision {
	manifest := saga.SlideManifest{
		Version: saga.DeckRecordVersion, ID: request.Slide.ID, Title: request.Slide.Title, Rank: request.Slide.Rank,
		Section: strings.TrimSpace(request.Slide.Section), Intent: request.Slide.Intent, Layout: request.Slide.Layout,
		MediaType: request.Slide.MediaType, Entrypoint: assetName, Takeaway: strings.TrimSpace(request.Slide.Takeaway),
		ReadingOrder: append([]string{}, request.Slide.ReadingOrder...), ExceptionRationale: strings.TrimSpace(request.Slide.ExceptionRationale),
	}
	revision := saga.SlideTransactionRevision{RequestID: request.RequestID, Asset: assetName, AssetDigest: assetDigest, Slide: manifest, Items: []saga.TransactionItem{}}
	for _, input := range request.Items {
		item := saga.ItemManifest{
			Version: saga.DeckRecordVersion, ID: input.ID, SlideID: request.Slide.ID, Rank: input.Rank, Kind: input.Kind,
			Label: input.Label, Description: strings.TrimSpace(input.Description), Selector: input.Selector, Hotspot: input.Hotspot,
			About: input.About, Body: input.Body, Placement: input.Placement, Leader: input.Leader,
		}
		revision.Items = append(revision.Items, saga.TransactionItem{Item: item, Evidence: append([]saga.CodeFile{}, input.Evidence...), CriterionLinks: append([]saga.CriterionLink{}, input.CriterionLinks...)})
	}
	return revision
}

func verifyTransactionEvidence(ctx context.Context, repo string, items []saga.TransactionItem) error {
	resolver, err := coderesolve.New(ctx, repo)
	if err != nil {
		return err
	}
	defer resolver.Close()
	for _, item := range items {
		if len(item.Evidence) == 0 {
			return fmt.Errorf("item %q requires exact code evidence", item.Item.ID)
		}
		for evidenceIndex, file := range item.Evidence {
			if file.Version != saga.CurrentVersion || len(file.References) == 0 {
				return fmt.Errorf("item %q evidence %d requires version %d and at least one reference", item.Item.ID, evidenceIndex, saga.CurrentVersion)
			}
			for referenceIndex, reference := range file.References {
				if err := coderef.Validate(reference); err != nil {
					return fmt.Errorf("item %q evidence %d reference %d: %w", item.Item.ID, evidenceIndex, referenceIndex, err)
				}
				if reference.WholeFile() || strings.TrimSpace(reference.Note) == "" {
					return fmt.Errorf("item %q evidence must use exact line ranges with a reviewer-facing note", item.Item.ID)
				}
				authored, authorErr := resolver.Author(ctx, reference.Location(), reference.Note)
				if authorErr != nil {
					return fmt.Errorf("item %q evidence cannot be verified: %w", item.Item.ID, authorErr)
				}
				if authored.Digest != reference.Digest {
					return fmt.Errorf("item %q evidence digest does not match %s", item.Item.ID, reference.Location())
				}
			}
		}
	}
	return nil
}

func validateTransactionCriteria(document *saga.Saga, items []saga.TransactionItem) error {
	requirementsDocument, err := requirements.Load(document.Root, document.Manifest.ID)
	if err != nil {
		return err
	}
	linkOwners := map[string]string{}
	for _, relation := range requirementsDocument.Relations {
		linkOwners[relation.ID] = "persisted requirements relation"
	}
	candidateSlide := ""
	if len(items) > 0 {
		candidateSlide = items[0].Item.SlideID
	}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			if slide.ID == candidateSlide {
				continue
			}
			for _, item := range slide.Items {
				for _, link := range item.CriterionLinks {
					linkOwners[link.ID] = "complete-slide Item " + item.Target
				}
			}
		}
	}
	for _, item := range items {
		if len(item.CriterionLinks) == 0 {
			return fmt.Errorf("item %q requires at least one exact criterion link", item.Item.ID)
		}
		for _, link := range item.CriterionLinks {
			if owner := linkOwners[link.ID]; owner != "" {
				return fmt.Errorf("item %q link id %q is already used by %s", item.Item.ID, link.ID, owner)
			}
			linkOwners[link.ID] = "candidate Item " + item.Item.ID
			target, parseErr := sagaref.ParseTarget(link.Criterion)
			if parseErr != nil || target.Kind != sagaref.TargetCriterion || target.SagaID != document.Manifest.ID {
				return fmt.Errorf("item %q link %q must name an exact criterion in this Saga", item.Item.ID, link.ID)
			}
			story := requirementsDocument.FindStory(target.ParentID)
			if story == nil || story.CurrentRevision == nil {
				return fmt.Errorf("item %q link %q criterion story has no single current revision", item.Item.ID, link.ID)
			}
			currentRevision, _ := livingid.Build(livingid.Reference{SagaID: document.Manifest.ID, Kind: livingid.KindRevision, ParentID: target.ParentID, ID: story.CurrentRevision.ID})
			if link.StoryRevision != currentRevision {
				return fmt.Errorf("item %q link %q story_revision is stale (got %q, current is %q)", item.Item.ID, link.ID, link.StoryRevision, currentRevision)
			}
			found := false
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				found = found || criterion.ID == target.ID
			}
			if !found {
				return fmt.Errorf("item %q link %q criterion does not exist in its pinned story revision", item.Item.ID, link.ID)
			}
		}
	}
	return nil
}

func legacySlideRevision(slide *saga.Slide) (saga.SlideTransactionRevision, error) {
	return saga.LegacySlideTransactionRevision(slide)
}

func transactionResult(sagaID, target, recordPath, root, operation string, dryRun, replayed bool, before, after *saga.SlideTransactionRevision) SlideTransactionResult {
	result := SlideTransactionResult{OK: true, Operation: operation, DryRun: dryRun, Replayed: replayed, Target: target, Snapshot: after.Snapshot, ChangedIDs: []string{}, Path: relativePathForOutput(root, recordPath)}
	if before != nil {
		result.PreviousSnapshot = before.Snapshot
	}
	result.Diff, result.ChangedIDs = slideTransactionDiff(sagaID, before, after)
	return result
}

func slideTransactionDiff(sagaID string, before, after *saga.SlideTransactionRevision) (SlideSemanticDiff, []string) {
	diff := SlideSemanticDiff{CreatedItems: []string{}, UpdatedItems: []string{}, RemovedItems: []string{}, SelectorChanges: []string{}}
	if before == nil {
		changed := []string{saga.SlideTarget(sagaID, after.Slide.ID)}
		diff.AssetChanged = true
		for _, item := range after.Items {
			diff.CreatedItems = append(diff.CreatedItems, item.Item.ID)
			changed = append(changed, saga.ItemTarget(sagaID, after.Slide.ID, item.Item.ID))
		}
		return diff, changed
	}
	if before.AssetDigest == after.AssetDigest && reflect.DeepEqual(before.Slide, after.Slide) && reflect.DeepEqual(before.Items, after.Items) {
		return diff, []string{}
	}
	changed := []string{saga.SlideTarget(sagaID, after.Slide.ID)}
	diff.AssetChanged = before.AssetDigest != after.AssetDigest || before.Slide.MediaType != after.Slide.MediaType
	oldItems, newItems := map[string]saga.TransactionItem{}, map[string]saga.TransactionItem{}
	for _, item := range before.Items {
		oldItems[item.Item.ID] = item
	}
	for _, item := range after.Items {
		newItems[item.Item.ID] = item
	}
	for id, item := range newItems {
		old, found := oldItems[id]
		itemTarget := saga.ItemTarget(sagaID, after.Slide.ID, id)
		switch {
		case !found:
			diff.CreatedItems = append(diff.CreatedItems, id)
			changed = append(changed, itemTarget)
		case !reflect.DeepEqual(old, item):
			diff.UpdatedItems = append(diff.UpdatedItems, id)
			changed = append(changed, itemTarget)
			if !reflect.DeepEqual(old.Item.Selector, item.Item.Selector) {
				diff.SelectorChanges = append(diff.SelectorChanges, id)
			}
		}
	}
	for id := range oldItems {
		if _, found := newItems[id]; !found {
			diff.RemovedItems = append(diff.RemovedItems, id)
			changed = append(changed, saga.ItemTarget(sagaID, after.Slide.ID, id))
		}
	}
	sort.Strings(diff.CreatedItems)
	sort.Strings(diff.UpdatedItems)
	sort.Strings(diff.RemovedItems)
	sort.Strings(diff.SelectorChanges)
	changed = sortedUniqueTransactionIDs(changed)
	return diff, changed
}

func sortedUniqueTransactionIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func transactionOperationForParents(parents []string) string {
	switch len(parents) {
	case 0:
		return "create"
	case 1:
		return "update"
	default:
		return "reconcile"
	}
}

func storedPreviousSnapshot(record saga.SlideTransactionRecord, index int) string {
	if index < 0 || index >= len(record.Revisions) {
		return ""
	}
	if parents := record.Revisions[index].ParentSnapshots; len(parents) > 0 {
		return sortedSnapshotCopy(parents)[0]
	}
	if index > 0 { // Parent edges omitted by records written before the DAG field.
		return record.Revisions[index-1].Snapshot
	}
	return ""
}

func storedExpectationMatches(request SlideTransactionRequest, record saga.SlideTransactionRecord, index int) bool {
	switch request.Operation {
	case "create":
		return request.ExpectedSnapshot == "absent"
	case "update":
		return request.ExpectedSnapshot == storedPreviousSnapshot(record, index)
	case "reconcile":
		return sameSnapshotSet(request.ExpectedSnapshots, record.Revisions[index].ParentSnapshots)
	default:
		return false
	}
}

func validSnapshotSet(values []string) bool {
	seen := map[string]bool{}
	for _, value := range values {
		if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") || seen[value] {
			return false
		}
		if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
			return false
		}
		seen[value] = true
	}
	return true
}

func sortedSnapshotCopy(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func sameSnapshotSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	l, r := sortedSnapshotCopy(left), sortedSnapshotCopy(right)
	return reflect.DeepEqual(l, r)
}

func firstSnapshot(values []string) string {
	if len(values) == 0 {
		return "absent"
	}
	return values[0]
}

func readSlideTransactionAsset(base string, asset SlideTransactionAsset) ([]byte, string, error) {
	if (asset.Path == "") == (asset.ContentBase64 == "") {
		return nil, "", fmt.Errorf("asset requires exactly one of path or content_base64")
	}
	if asset.ContentBase64 != "" {
		data, err := base64.StdEncoding.Strict().DecodeString(asset.ContentBase64)
		if err != nil {
			return nil, "", fmt.Errorf("decode asset content_base64: %w", err)
		}
		return data, extensionFromContent(data), nil
	}
	if filepath.IsAbs(asset.Path) || filepath.Clean(asset.Path) != asset.Path || asset.Path == "." || asset.Path == ".." || strings.HasPrefix(asset.Path, ".."+string(filepath.Separator)) || strings.ContainsRune(asset.Path, '\\') {
		return nil, "", fmt.Errorf("asset path must be a normalized relative path beneath the request file")
	}
	path := filepath.Join(base, asset.Path)
	current := base
	for _, part := range strings.Split(asset.Path, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, "", fmt.Errorf("read asset: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, "", fmt.Errorf("asset path must not contain symlinks")
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, "", fmt.Errorf("asset path must name a regular file")
	}
	data, err := os.ReadFile(path)
	return data, strings.ToLower(filepath.Ext(path)), err
}

func extensionFromContent(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("<svg")) || bytes.Contains(trimmed, []byte("<svg ")) {
		return ".svg"
	}
	if bytes.HasPrefix(trimmed, []byte("<!DOCTYPE html")) || bytes.HasPrefix(trimmed, []byte("<html")) {
		return ".html"
	}
	if len(data) >= 8 && bytes.Equal(data[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return ".png"
	}
	if len(data) >= 3 && bytes.Equal(data[:3], []byte{0xff, 0xd8, 0xff}) {
		return ".jpg"
	}
	if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return ".webp"
	}
	return ""
}

func transactionMediaExtension(mediaType, extension string) string {
	want := map[string][]string{"image/svg+xml": {".svg"}, "image/png": {".png"}, "image/jpeg": {".jpg", ".jpeg"}, "image/webp": {".webp"}, "text/html": {".html", ".htm"}}
	for _, allowed := range want[mediaType] {
		if extension == allowed {
			return ""
		}
	}
	return fmt.Sprintf("extension %q does not match media_type %q", extension, mediaType)
}

func commitTransactionAsset(path string, data []byte) (bool, error) {
	if existing, err := os.ReadFile(path); err == nil {
		if bytes.Equal(existing, data) {
			return false, nil
		}
		return false, fmt.Errorf("content-addressed asset name collision at %s", filepath.Base(path))
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := store.WriteFile(path, data, 0o644, true); err != nil {
		return false, err
	}
	return true, nil
}

func validateTransactionCandidateWithoutPublish(root string, deck *saga.Deck, record saga.SlideTransactionRecord, assetName string, asset []byte) error {
	stage, err := os.MkdirTemp("", "change-saga-slide-dry-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	assets := map[string][]byte{assetName: asset}
	for _, revision := range record.Revisions {
		if _, present := assets[revision.Asset]; present {
			continue
		}
		value, readErr := os.ReadFile(filepath.Join(deck.Directory, revision.Asset))
		if readErr != nil {
			return readErr
		}
		assets[revision.Asset] = value
	}
	for name, value := range assets {
		if err := os.WriteFile(filepath.Join(stage, name), value, 0o600); err != nil {
			return err
		}
	}
	stagedDeck := *deck
	stagedDeck.Directory = stage
	validation := saga.ValidateSlideTransactionCandidate(root, &stagedDeck, record)
	if !validation.Valid {
		return transactionValidationError(validation)
	}
	return nil
}

func transactionValidationError(validation saga.Validation) error {
	var problems []string
	for _, issue := range validation.Issues {
		if issue.Severity == "error" {
			problems = append(problems, issue.Path+": "+issue.Message)
		}
	}
	return fmt.Errorf("complete slide is invalid: %s", strings.Join(problems, "; "))
}

func readStrictJSONPath(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%s contains more than one JSON value", path)
	}
	return nil
}

func decodeSlideTransactionRequest(reader io.Reader) (SlideTransactionRequest, error) {
	var request SlideTransactionRequest
	decoder := json.NewDecoder(io.LimitReader(reader, 17<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return request, fmt.Errorf("request contains more than one JSON value")
	}
	return request, nil
}

func ApplySlide(ctx context.Context, args []string, out io.Writer) error {
	flags := commandFlags("apply-slide", commandUsage["apply-slide"], out)
	from := flags.String("from", "", "complete slide transaction JSON file, or - for standard input")
	repo := flags.String("repo", "", "code repository used to verify every exact evidence digest")
	dryRun := flags.Bool("dry-run", false, "validate and return the semantic diff without publishing")
	jsonOutput := flags.Bool("json", false, "emit one machine-readable JSON result")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *from == "" {
		return fmt.Errorf("usage: %s", commandUsage["apply-slide"])
	}
	var reader io.Reader
	base, err := os.Getwd()
	if err != nil {
		return err
	}
	if *from == "-" {
		reader = os.Stdin
	} else {
		absolute, err := filepath.Abs(*from)
		if err != nil {
			return err
		}
		file, err := os.Open(absolute)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("--from must name a regular JSON file")
		}
		reader, base = file, filepath.Dir(absolute)
	}
	request, err := decodeSlideTransactionRequest(reader)
	if err != nil {
		return fmt.Errorf("read slide transaction: %w", err)
	}
	result, err := ApplySlideTransaction(ctx, flags.Arg(0), base, *repo, request, *dryRun)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, result)
	}
	verb := "Applied"
	if result.DryRun {
		verb = "Would apply"
	} else if result.Replayed {
		verb = "Replayed"
	}
	fmt.Fprintf(out, "%s complete slide %s\nSnapshot: %s\nChanged IDs: %s\n", verb, result.Target, result.Snapshot, strings.Join(result.ChangedIDs, ", "))
	return nil
}
