package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/sagaref"
)

const (
	// SlideTransactionVersion versions the atomic complete-slide authoring
	// record. It is deliberately separate from DeckRecordVersion: a transaction
	// revision contains existing v4 slide and Item values plus their evidence
	// and exact criterion pins.
	SlideTransactionVersion  = 1
	MaxSlideRevisions        = 256
	MaxSlideAssetBytes       = 8 << 20
	MaxSlideTransactionBytes = 8 << 20
)

var flatSlideTransactionName = regexp.MustCompile(`^25-t-([0-9a-f]{12})-([0-9a-f]{12})\.json$`)

// CriterionLink is an exact Item-level semantic link. Criterion identifies a
// single acceptance criterion and StoryRevision pins the complete story
// snapshot that contained it; broad Deck/Slide sources are not representable.
type CriterionLink struct {
	ID            string `json:"id"`
	Criterion     string `json:"criterion"`
	StoryRevision string `json:"story_revision"`
	Rationale     string `json:"rationale"`
}

// TransactionItem is one complete semantic Item revision and all evidence and
// criterion links published with it.
type TransactionItem struct {
	Item           ItemManifest    `json:"item"`
	Evidence       []CodeFile      `json:"evidence"`
	CriterionLinks []CriterionLink `json:"criterion_links"`
}

// SlideTransactionRevision is one immutable complete-slide snapshot. Asset is
// a content-addressed regular file committed before this record references it.
type SlideTransactionRevision struct {
	Snapshot    string            `json:"snapshot"`
	RequestID   string            `json:"request_id"`
	CreatedAt   time.Time         `json:"created_at"`
	Asset       string            `json:"asset"`
	AssetDigest string            `json:"asset_digest"`
	Slide       SlideManifest     `json:"slide"`
	Items       []TransactionItem `json:"items"`
}

// SlideTransactionRecord is the single logical publication point for a
// complete slide. Revisions retain lifecycle history; Current names exactly
// one revision by snapshot digest.
type SlideTransactionRecord struct {
	Version   int                        `json:"version"`
	DeckID    string                     `json:"deck"`
	SlideID   string                     `json:"slide"`
	Current   string                     `json:"current"`
	Revisions []SlideTransactionRevision `json:"revisions"`
}

// SlideTransactionFilename is stable across slide revisions, so replacing
// the record is the one atomic publication point.
func SlideTransactionFilename(deckTarget, slideTarget string) string {
	return fmt.Sprintf("25-t-%s-%s.json", FlatTargetKey(deckTarget), FlatTargetKey(slideTarget))
}

// SlideAssetFilename returns the content-addressed visual sidecar name. The
// 208-bit prefix fits the portable basename budget while retaining a
// collision-resistant identity; the full digest remains in the record.
func SlideAssetFilename(data []byte, extension string) (string, error) {
	extension = strings.ToLower(extension)
	if extension == "" || strings.ContainsAny(extension, `/\\`) || extension[0] != '.' {
		return "", fmt.Errorf("slide asset requires a simple file extension")
	}
	digest := sha256.Sum256(data)
	name := "24-a-" + hex.EncodeToString(digest[:26]) + extension
	if len(name) > FlatMaxBasename {
		return "", fmt.Errorf("slide asset filename exceeds %d characters", FlatMaxBasename)
	}
	return name, nil
}

// SlideRevisionSnapshot deterministically identifies the complete semantic
// revision. CreatedAt is lifecycle metadata and is intentionally excluded.
func SlideRevisionSnapshot(revision SlideTransactionRevision) (string, error) {
	value := revision
	value.Snapshot = ""
	value.CreatedAt = time.Time{}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (record SlideTransactionRecord) currentRevision() (*SlideTransactionRevision, error) {
	var current *SlideTransactionRevision
	for index := range record.Revisions {
		if record.Revisions[index].Snapshot == record.Current {
			if current != nil {
				return nil, fmt.Errorf("current snapshot appears more than once")
			}
			current = &record.Revisions[index]
		}
	}
	if current == nil {
		return nil, fmt.Errorf("current snapshot does not name a revision")
	}
	return current, nil
}

// CurrentRevision returns the revision selected by the record's atomic commit
// pointer.
func (record SlideTransactionRecord) CurrentRevision() (*SlideTransactionRevision, error) {
	return record.currentRevision()
}

func validateSlideTransactionRecord(root, recordRoot, name, sagaID string, deck *Deck, targets deckTargets, record SlideTransactionRecord, validation *Validation) *SlideTransactionRevision {
	problem := func(message string) { addIssue(validation, "error", name, "slide transaction: "+message) }
	if record.Version != SlideTransactionVersion {
		problem(fmt.Sprintf("version must be %d", SlideTransactionVersion))
	}
	if record.DeckID != deck.ID || !ValidID(record.SlideID) {
		problem("deck must name its containing deck and slide must be a stable id")
	}
	wantName := SlideTransactionFilename(deck.Target, targets.slide(record.SlideID))
	if name != wantName {
		problem("filename does not match its deck and stable slide target")
	}
	if len(record.Revisions) == 0 || len(record.Revisions) > MaxSlideRevisions {
		problem(fmt.Sprintf("must contain 1 to %d revisions", MaxSlideRevisions))
	}
	current, err := record.currentRevision()
	if err != nil {
		problem(err.Error())
		return nil
	}
	seenSnapshots, seenRequests := map[string]bool{}, map[string]bool{}
	for revisionIndex := range record.Revisions {
		revision := &record.Revisions[revisionIndex]
		if revision.Slide.ID != record.SlideID || revision.Slide.DeckID != record.DeckID {
			problem("every revision must preserve the record's deck and slide ids")
		}
		wantSnapshot, digestErr := SlideRevisionSnapshot(*revision)
		if digestErr != nil || revision.Snapshot != wantSnapshot || seenSnapshots[revision.Snapshot] {
			problem("every revision needs a unique snapshot matching its complete content")
		}
		seenSnapshots[revision.Snapshot] = true
		if !livingid.ValidID(revision.RequestID) || seenRequests[revision.RequestID] {
			problem("every revision needs a unique stable request_id")
		}
		seenRequests[revision.RequestID] = true
		if revision.CreatedAt.IsZero() {
			problem("every revision needs created_at")
		}
		legacyAsset := revisionIndex == 0 && strings.HasPrefix(revision.RequestID, "legacy-") && flatSlideName.MatchString(strings.TrimSuffix(revision.Asset, filepath.Ext(revision.Asset))+".json")
		if revision.Asset != revision.Slide.Entrypoint || (!strings.HasPrefix(revision.Asset, "24-a-") && !legacyAsset) || filepath.Base(revision.Asset) != revision.Asset {
			problem("asset must be the content-addressed flat entrypoint")
		}
		if !strings.HasPrefix(revision.AssetDigest, coderef.DigestPrefix) {
			problem("asset_digest must be a sha256 digest")
		}
		assetPath := filepath.Join(recordRoot, revision.Asset)
		assetInfo, statErr := os.Lstat(assetPath)
		if statErr != nil || assetInfo.Mode()&os.ModeSymlink != 0 || !assetInfo.Mode().IsRegular() {
			problem("asset must be an existing regular file, not a directory or symlink")
		} else if asset, readErr := os.ReadFile(assetPath); readErr != nil || coderef.DigestBytes(asset) != revision.AssetDigest {
			problem("asset_digest does not match the referenced asset bytes")
		}
		validateTransactionalSlide(root, recordRoot, name, sagaID, deck, targets, *revision, validation)
	}
	return current
}

// ValidateSlideTransactionCandidate runs the same strict checks used by the
// loader before a writer publishes record. Its content-addressed assets must
// already exist; until the record itself is committed they remain invisible
// to Saga readers.
func ValidateSlideTransactionCandidate(root string, deck *Deck, record SlideTransactionRecord) Validation {
	validation := Validation{Valid: true, Issues: []Issue{}}
	targets := appDeckTargets(manifestSagaID(deck.Target))
	name := SlideTransactionFilename(deck.Target, targets.slide(record.SlideID))
	validateSlideTransactionRecord(root, deck.Directory, name, manifestSagaID(deck.Target), deck, targets, record, &validation)
	validation.Valid = !hasErrors(validation.Issues)
	return validation
}

func validateTransactionalSlide(root, recordRoot, path, sagaID string, deck *Deck, targets deckTargets, revision SlideTransactionRevision, validation *Validation) {
	value := revision.Slide
	if value.Version != DeckRecordVersion || !stableID.MatchString(value.ID) || strings.TrimSpace(value.Title) == "" {
		addIssue(validation, "error", path, "transaction slide requires version 4, a stable id, and a title")
	}
	if value.DeckID != deck.ID || validFlatRank(value.Rank) != nil || !slideIntents[value.Intent] || !slideLayouts[value.Layout] || !slideMediaTypes[value.MediaType] {
		addIssue(validation, "error", path, "transaction slide requires its parent deck, a portable rank, and supported intent, layout, and media_type")
	}
	if value.Section != strings.TrimSpace(value.Section) || utf8Count(value.Section) > 80 || strings.TrimSpace(value.Takeaway) == "" || utf8Count(value.Takeaway) > 180 {
		addIssue(validation, "error", path, "transaction slide section/takeaway exceed their 80/180 character limits")
	}
	if value.Layout == "custom" && strings.TrimSpace(value.ExceptionRationale) == "" {
		addIssue(validation, "error", path, "custom transaction slide requires exception_rationale")
	}
	component := FragmentManifest{Version: CurrentVersion, ID: value.ID, Title: value.Title, MediaType: value.MediaType, Entrypoint: value.Entrypoint, Order: value.Rank}
	validateFragmentManifestMode(component, path, recordRoot, true, validation)

	slide := &Slide{Path: path, Directory: recordRoot, SlideManifest: value, Target: targets.slide(value.ID)}
	itemIDs, linkIDs := map[string]bool{}, map[string]bool{}
	for _, transactionItem := range revision.Items {
		manifest := transactionItem.Item
		item := &Item{Path: path, Directory: recordRoot, ItemManifest: manifest, Target: targets.item(value.ID, manifest.ID)}
		validateItem(item, slide, validation)
		if itemIDs[item.ID] {
			addIssue(validation, "error", path, fmt.Sprintf("transaction repeats item %q", item.ID))
		}
		itemIDs[item.ID] = true
		for _, evidence := range transactionItem.Evidence {
			evidence.Path = path
			validateCodeFile(evidence, validation)
			for _, reference := range evidence.References {
				if reference.WholeFile() || strings.TrimSpace(reference.Note) == "" {
					addIssue(validation, "error", path, "transaction evidence requires an exact line range and reviewer-facing note")
				}
			}
		}
		for _, link := range transactionItem.CriterionLinks {
			if linkIDs[link.ID] {
				addIssue(validation, "error", path, fmt.Sprintf("transaction repeats criterion link %q", link.ID))
			}
			linkIDs[link.ID] = true
			if err := validateCriterionLink(sagaID, link); err != nil {
				addIssue(validation, "error", path, err.Error())
			}
		}
		slide.Items = append(slide.Items, item)
	}
	validateSlideComposition(slide, validation)
}

func validateCriterionLink(sagaID string, link CriterionLink) error {
	if !livingid.ValidID(link.ID) || strings.TrimSpace(link.Rationale) == "" {
		return fmt.Errorf("criterion link requires a stable id and rationale")
	}
	criterion, err := sagaref.ParseTarget(link.Criterion)
	if err != nil || criterion.Kind != sagaref.TargetCriterion || criterion.SagaID != sagaID {
		return fmt.Errorf("criterion link %q must name an exact criterion in this Saga", link.ID)
	}
	revision, err := livingid.Parse(link.StoryRevision)
	if err != nil || revision.Kind != livingid.KindRevision || revision.SagaID != sagaID || revision.ParentID != criterion.ParentID {
		return fmt.Errorf("criterion link %q story_revision must pin its criterion's story", link.ID)
	}
	return nil
}

func utf8Count(value string) int { return len([]rune(value)) }
