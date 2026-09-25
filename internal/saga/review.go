package saga

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/coderef"
)

// A review is a pull request's slide deck. It is the one part of a Saga that
// speaks in diffs: its Items reference the code the change touched, viewed
// against the review's base, and may reference the documentation records the
// change revised. Approvals and comments exist only here, per review slide.
//
//	___reviews/<id>.review/
//	  review.json          the pull request, its base, how its head is followed,
//	                       and, after merge, the frozen base and head commits
//	  deck/                the review deck: one flat deck bundle, role review
//	  approvals/<id>.json  append-only per-slide decisions
//	  comments/<id>.json   append-only comments on review slides and Items
const (
	ReviewSuffix       = ".review"
	ReviewManifestName = "review.json"
	ReviewDeckDir      = "deck"
	ReviewApprovalsDir = "approvals"
	ReviewCommentsDir  = "comments"

	ReviewVersion           = SagaVersion
	ReviewSchemaURL         = "https://changesaga.dev/schema/v5/review.schema.json"
	ReviewApprovalSchemaURL = "https://changesaga.dev/schema/v5/review-approval.schema.json"
	ReviewCommentSchemaURL  = "https://changesaga.dev/schema/v5/review-comment.schema.json"
)

// Per-slide decision states. None withdraws a reviewer's earlier decision.
const (
	ApprovalApproved         = "approved"
	ApprovalChangesRequested = "changes_requested"
	ApprovalNone             = "none"
)

// Comment thread states. A comment may resolve or reopen its thread.
const (
	CommentOpen     = "open"
	CommentResolved = "resolved"
)

// ReviewManifest is review.json.
type ReviewManifest struct {
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	// PullRequest identifies the one pull request this review is.
	PullRequest *PullRequest `json:"pull_request,omitempty"`
	// Base is the revision the pull request merges into, such as main. The
	// review is viewed from the merge-base of Base and the head.
	Base string `json:"base"`
	// Head is the ref the review follows as commits are pushed, such as the
	// pull request's branch. Empty follows the checkout's HEAD.
	Head string `json:"head,omitempty"`
	// Merged freezes the exact commits once the change has landed, so the
	// review stays viewable after its branch is gone.
	Merged    *ReviewMerge `json:"merged,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type PullRequest struct {
	Number int    `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ReviewMerge is written by repin when the change lands.
type ReviewMerge struct {
	Base     string    `json:"base"`
	Head     string    `json:"head"`
	Landed   string    `json:"landed"`
	MergedAt time.Time `json:"merged_at"`
}

// ReviewApproval is one reviewer's decision on one review slide, given at one
// head commit. The latest decision per reviewer and slide is current.
type ReviewApproval struct {
	Path     string           `json:"-"`
	Schema   string           `json:"$schema"`
	Version  int              `json:"version"`
	ID       string           `json:"id"`
	Slide    string           `json:"slide"`
	State    string           `json:"state"`
	Reviewer ReviewerIdentity `json:"reviewer"`
	// Commit is the pull request head the decision was given at.
	Commit string `json:"commit"`
	// SlideDigest is the slide's content (manifest, visual, Items, and their
	// code references) when the decision was given.
	SlideDigest string    `json:"slide_digest"`
	Body        string    `json:"body,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ReviewComment is one comment on a review slide or Item, or a reply to
// another comment. A comment may resolve or reopen its thread.
type ReviewComment struct {
	Path    string `json:"-"`
	Schema  string `json:"$schema"`
	Version int    `json:"version"`
	ID      string `json:"id"`
	Target  string `json:"target"`
	ReplyTo string `json:"reply_to,omitempty"`
	Body    string `json:"body"`
	State   string `json:"state,omitempty"`
	// Anchor and AnnotationAction make visual markup part of the same
	// append-only discussion. The root creates the mark; later replies update
	// or delete it without rewriting the original review record.
	Anchor           *ReviewAnchor    `json:"anchor,omitempty"`
	AnnotationAction string           `json:"annotation_action,omitempty"`
	Reviewer         ReviewerIdentity `json:"reviewer"`
	Commit           string           `json:"commit,omitempty"`
	// CreatedAt orders comments; it is not an identity.
	CreatedAt time.Time `json:"created_at"`
}

type ReviewAnchor struct {
	Type       string        `json:"type"`
	Coordinate string        `json:"coordinate_space,omitempty"`
	Shapes     []ReviewShape `json:"shapes,omitempty"`
	Note       *ReviewNote   `json:"note,omitempty"`
}

type ReviewShape struct {
	Type        string        `json:"type"`
	X           float64       `json:"x,omitempty"`
	Y           float64       `json:"y,omitempty"`
	Width       float64       `json:"width,omitempty"`
	Height      float64       `json:"height,omitempty"`
	Points      []ReviewPoint `json:"points,omitempty"`
	Color       string        `json:"color,omitempty"`
	StrokeWidth float64       `json:"stroke_width,omitempty"`
}

type ReviewPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type ReviewNote struct {
	Text  string  `json:"text"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Color string  `json:"color,omitempty"`
}

// Review is one loaded pull request review.
type Review struct {
	ReviewManifest
	// Path is the app-relative review directory.
	Path      string           `json:"path"`
	Directory string           `json:"-"`
	Target    string           `json:"target"`
	Deck      *Deck            `json:"deck,omitempty"`
	Approvals []ReviewApproval `json:"approvals,omitempty"`
	Comments  []ReviewComment  `json:"comments,omitempty"`
}

// ReviewTarget is the URN of a review; its deck, slides, and Items are named
// beneath it so their IDs never collide with another review's.
func ReviewTarget(sagaID, reviewID string) string {
	return "urn:change-saga:" + sagaID + ":review:" + reviewID
}

func ReviewDeckTarget(sagaID, reviewID, deckID string) string {
	return ReviewTarget(sagaID, reviewID) + ":deck:" + deckID
}

func ReviewSlideTarget(sagaID, reviewID, slideID string) string {
	return ReviewTarget(sagaID, reviewID) + ":slide:" + slideID
}

func ReviewItemTarget(sagaID, reviewID, slideID, itemID string) string {
	return ReviewSlideTarget(sagaID, reviewID, slideID) + ":item:" + itemID
}

func reviewDeckTargets(sagaID, reviewID string) deckTargets {
	return deckTargets{
		deck:  func(id string) string { return ReviewDeckTarget(sagaID, reviewID, id) },
		slide: func(id string) string { return ReviewSlideTarget(sagaID, reviewID, id) },
		item:  func(slide, id string) string { return ReviewItemTarget(sagaID, reviewID, slide, id) },
	}
}

// ReviewDir returns the absolute directory of review id beneath root.
func ReviewDir(root, id string) string {
	return filepath.Join(root, ReviewsDir, id+ReviewSuffix)
}

// FindReview returns the loaded review with id.
func (document *Saga) FindReview(id string) *Review {
	for _, review := range document.Reviews {
		if review.ID == id {
			return review
		}
	}
	return nil
}

// Slide returns the review slide with id.
func (review *Review) Slide(id string) *Slide {
	if review.Deck == nil {
		return nil
	}
	for _, slide := range review.Deck.Slides {
		if slide.ID == id || slide.Target == id {
			return slide
		}
	}
	return nil
}

// ValidReviewApprovalState reports whether state is a per-slide decision.
func ValidReviewApprovalState(state string) bool {
	return state == ApprovalApproved || state == ApprovalChangesRequested || state == ApprovalNone
}

// ValidateReviewManifest checks review.json.
func ValidateReviewManifest(manifest ReviewManifest) error {
	switch {
	case manifest.Schema != ReviewSchemaURL:
		return fmt.Errorf("$schema must be %s", ReviewSchemaURL)
	case manifest.Version != ReviewVersion:
		return fmt.Errorf("version must be %d", ReviewVersion)
	case !ValidID(manifest.ID):
		return fmt.Errorf("id must be a stable identifier")
	case strings.TrimSpace(manifest.Title) == "":
		return fmt.Errorf("title is required")
	case strings.TrimSpace(manifest.Base) == "" || strings.ContainsAny(manifest.Base, " \t\n") || strings.HasPrefix(manifest.Base, "-"):
		return fmt.Errorf("base must name the revision the pull request merges into")
	case strings.ContainsAny(manifest.Head, " \t\n") || strings.HasPrefix(manifest.Head, "-"):
		return fmt.Errorf("head must be a ref name")
	case manifest.CreatedAt.IsZero():
		return fmt.Errorf("created_at is required")
	}
	if manifest.PullRequest != nil {
		if manifest.PullRequest.Number < 0 || (manifest.PullRequest.Number == 0 && manifest.PullRequest.URL == "") {
			return fmt.Errorf("pull_request needs a positive number or a URL")
		}
		if url := manifest.PullRequest.URL; url != "" && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			return fmt.Errorf("pull_request url must be an http(s) URL")
		}
	}
	if merged := manifest.Merged; merged != nil {
		if !coderef.ValidCommit(merged.Base) || !coderef.ValidCommit(merged.Head) || !coderef.ValidCommit(merged.Landed) || merged.MergedAt.IsZero() {
			return fmt.Errorf("merged requires full base, head, and landed commits and merged_at")
		}
	}
	return nil
}

func validDigest(value string) bool {
	hexPart, ok := strings.CutPrefix(value, "sha256:")
	if !ok || len(hexPart) != 64 {
		return false
	}
	_, err := hex.DecodeString(hexPart)
	return err == nil
}

// loadReviews loads every review beneath ___reviews.
func loadReviews(root string, manifest Manifest, options loadOptions, validation *Validation) ([]*Review, error) {
	dir := filepath.Join(root, ReviewsDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var reviews []*Review
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		rel := relativePath(root, path)
		id := strings.TrimSuffix(entry.Name(), ReviewSuffix)
		if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() || !strings.HasSuffix(entry.Name(), ReviewSuffix) || !ValidID(id) {
			addIssue(validation, "error", rel, "reviews must be real <id>.review directories")
			continue
		}
		review, err := loadReview(root, path, id, manifest, options, validation)
		if err != nil {
			return nil, err
		}
		if review != nil {
			reviews = append(reviews, review)
		}
	}
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].ID < reviews[j].ID })
	return reviews, nil
}

func loadReview(root, dir, id string, manifest Manifest, options loadOptions, validation *Validation) (*Review, error) {
	rel := relativePath(root, dir)
	var value ReviewManifest
	manifestPath := filepath.Join(dir, ReviewManifestName)
	if err := readJSON(manifestPath, &value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
		return nil, nil
	}
	if err := ValidateReviewManifest(value); err != nil {
		addIssue(validation, "error", relativePath(root, manifestPath), err.Error())
	}
	if value.ID != id {
		addIssue(validation, "error", relativePath(root, manifestPath), fmt.Sprintf("review id %q must match its directory %q", value.ID, id+ReviewSuffix))
	}
	review := &Review{ReviewManifest: value, Path: rel, Directory: dir, Target: ReviewTarget(manifest.ID, id)}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		switch entry.Name() {
		case ReviewManifestName:
			continue
		case ReviewDeckDir, ReviewApprovalsDir, ReviewCommentsDir:
			if entry.Type()&fs.ModeSymlink != 0 || !entry.IsDir() {
				addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "review record directories must be real directories")
			}
			continue
		}
		if strings.HasPrefix(entry.Name(), ".change-saga-") {
			continue
		}
		addIssue(validation, "error", relativePath(root, filepath.Join(dir, entry.Name())), "unknown entry in a review; a review holds review.json, deck, approvals, and comments")
	}

	deckDir := filepath.Join(dir, ReviewDeckDir)
	if realDirectoryExists(deckDir) {
		decks, err := loadDeckRecords(root, deckDir, reviewDeckTargets(manifest.ID, id), options, validation)
		if err != nil {
			return nil, err
		}
		if len(decks) != 1 {
			addIssue(validation, "error", relativePath(root, deckDir), "a review deck bundle must contain exactly one deck record")
		} else {
			review.Deck = decks[0]
			validateDeckRole(review.Deck, DeckRoleReview, manifest.ID, validation)
		}
	} else {
		addIssue(validation, "error", rel, "a review is always a slide deck; its deck directory is missing")
	}

	slides := map[string]bool{}
	targets := map[string]bool{}
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			slides[slide.ID] = true
			targets[slide.Target] = true
			for _, item := range slide.Items {
				targets[item.Target] = true
			}
		}
	}
	if err := loadReviewRecords(root, filepath.Join(dir, ReviewApprovalsDir), "approval", validation, func(path string) {
		var approval ReviewApproval
		if err := readJSON(path, &approval); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			return
		}
		approval.Path = path
		if problem := validateReviewApproval(approval, filepath.Base(path), slides); problem != "" {
			addIssue(validation, "error", relativePath(root, path), problem)
		}
		review.Approvals = append(review.Approvals, approval)
	}); err != nil {
		return nil, err
	}
	comments := map[string]ReviewComment{}
	if err := loadReviewRecords(root, filepath.Join(dir, ReviewCommentsDir), "comment", validation, func(path string) {
		var comment ReviewComment
		if err := readJSON(path, &comment); err != nil {
			addIssue(validation, "error", relativePath(root, path), err.Error())
			return
		}
		comment.Path = path
		comments[comment.ID] = comment
		review.Comments = append(review.Comments, comment)
	}); err != nil {
		return nil, err
	}
	for _, comment := range review.Comments {
		if problem := validateReviewComment(comment, filepath.Base(comment.Path), targets, comments); problem != "" {
			addIssue(validation, "error", relativePath(root, comment.Path), problem)
		}
	}
	sort.Slice(review.Approvals, func(i, j int) bool {
		return earlierRecord(review.Approvals[i].CreatedAt, review.Approvals[i].ID, review.Approvals[j].CreatedAt, review.Approvals[j].ID)
	})
	sort.Slice(review.Comments, func(i, j int) bool {
		return earlierRecord(review.Comments[i].CreatedAt, review.Comments[i].ID, review.Comments[j].CreatedAt, review.Comments[j].ID)
	})
	return review, nil
}

func loadReviewRecords(root, dir, kind string, validation *Validation, fn func(string)) error {
	if !realDirectoryExists(dir) {
		return nil
	}
	return loadFlatRecords(root, dir, kind, validation, func(path string) {
		if strings.HasPrefix(filepath.Base(path), ".change-saga-") {
			return
		}
		fn(path)
	})
}

func validateReviewApproval(approval ReviewApproval, name string, slides map[string]bool) string {
	reviewer := approval.Reviewer
	switch {
	case approval.Schema != ReviewApprovalSchemaURL || approval.Version != ReviewVersion:
		return fmt.Sprintf("approval requires $schema %s and version %d", ReviewApprovalSchemaURL, ReviewVersion)
	case !ValidID(approval.ID) || name != approval.ID+".json":
		return "approval id must be a stable identifier matching its filename"
	case !slides[approval.Slide]:
		return fmt.Sprintf("approval names slide %q, which is not in this review's deck", approval.Slide)
	case !ValidReviewApprovalState(approval.State):
		return "approval state must be approved, changes_requested, or none"
	case !coderef.ValidCommit(approval.Commit):
		return "approval commit must be the full head commit it was given at"
	case !validDigest(approval.SlideDigest):
		return "approval slide_digest must be sha256:<64 hex>"
	case approval.CreatedAt.IsZero():
		return "approval requires created_at"
	}
	if err := ValidateReviewerIdentity(&reviewer); err != nil {
		return err.Error()
	}
	return ""
}

func validateReviewComment(comment ReviewComment, name string, targets map[string]bool, comments map[string]ReviewComment) string {
	reviewer := comment.Reviewer
	reply, replyExists := comments[comment.ReplyTo]
	switch {
	case comment.Schema != ReviewCommentSchemaURL || comment.Version != ReviewVersion:
		return fmt.Sprintf("comment requires $schema %s and version %d", ReviewCommentSchemaURL, ReviewVersion)
	case !ValidID(comment.ID) || name != comment.ID+".json":
		return "comment id must be a stable identifier matching its filename"
	case !targets[comment.Target]:
		return "comment target must be a slide or Item of this review"
	case comment.ReplyTo != "" && (!replyExists || comment.ReplyTo == comment.ID):
		return fmt.Sprintf("comment replies to unknown comment %q", comment.ReplyTo)
	case strings.TrimSpace(comment.Body) == "":
		return "comment body is required"
	case comment.State != "" && comment.State != CommentOpen && comment.State != CommentResolved:
		return "comment state must be open or resolved"
	case comment.AnnotationAction != "" && comment.AnnotationAction != "create" && comment.AnnotationAction != "update" && comment.AnnotationAction != "delete":
		return "annotation_action must be create, update, or delete"
	case comment.AnnotationAction == "" && comment.Anchor != nil:
		return "an annotation anchor requires annotation_action"
	case comment.AnnotationAction == "create" && (comment.ReplyTo != "" || comment.Anchor == nil):
		return "an annotation create requires an anchor on a root comment"
	case (comment.AnnotationAction == "update" || comment.AnnotationAction == "delete") && comment.ReplyTo == "":
		return "an annotation update or delete must reply to its root comment"
	case (comment.AnnotationAction == "update" || comment.AnnotationAction == "delete") && (reply.AnnotationAction != "create" || reply.ReplyTo != ""):
		return "an annotation update or delete must reply to its annotation create root"
	case comment.AnnotationAction == "update" && comment.Anchor == nil:
		return "an annotation update requires an anchor"
	case comment.AnnotationAction == "delete" && comment.Anchor != nil:
		return "an annotation delete cannot carry an anchor"
	case comment.Commit != "" && !coderef.ValidCommit(comment.Commit):
		return "comment commit must be a full commit"
	case comment.CreatedAt.IsZero():
		return "comment requires created_at"
	}
	if err := ValidateReviewerIdentity(&reviewer); err != nil {
		return err.Error()
	}
	if comment.Anchor != nil {
		if err := ValidateReviewAnchor(*comment.Anchor); err != nil {
			return err.Error()
		}
	}
	return ""
}

// ValidateReviewAnchor keeps slide markup in normalized coordinates so it
// survives fitting, zoom, filmstrip changes, and narrow layouts.
func ValidateReviewAnchor(anchor ReviewAnchor) error {
	valid := func(value float64) bool { return value >= 0 && value <= 1 }
	validColor := func(value string) bool {
		if len(value) != 7 || value[0] != '#' {
			return false
		}
		for _, r := range value[1:] {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return false
			}
		}
		return true
	}
	switch anchor.Type {
	case "region", "drawing", "highlight":
		if anchor.Coordinate != "normalized" || len(anchor.Shapes) == 0 || anchor.Note != nil {
			return fmt.Errorf("visual annotation requires normalized shapes")
		}
		for _, shape := range anchor.Shapes {
			if shape.Type != "rect" && shape.Type != "ellipse" && shape.Type != "path" && shape.Type != "highlight" {
				return fmt.Errorf("unsupported review annotation shape %q", shape.Type)
			}
			if !valid(shape.X) || !valid(shape.Y) || !valid(shape.Width) || !valid(shape.Height) || (shape.StrokeWidth < 0 || math.IsNaN(shape.StrokeWidth) || math.IsInf(shape.StrokeWidth, 0)) || (shape.Color != "" && !validColor(shape.Color)) {
				return fmt.Errorf("annotation shape coordinates and color must be normalized and valid")
			}
			for _, point := range shape.Points {
				if !valid(point.X) || !valid(point.Y) {
					return fmt.Errorf("annotation path points must be normalized")
				}
			}
		}
	case "note":
		if anchor.Coordinate != "normalized" || anchor.Note == nil || len(anchor.Shapes) != 0 || strings.TrimSpace(anchor.Note.Text) == "" || len([]rune(anchor.Note.Text)) > 2000 || !valid(anchor.Note.X) || !valid(anchor.Note.Y) || (anchor.Note.Color != "" && !validColor(anchor.Note.Color)) {
			return fmt.Errorf("sticky note requires normalized placement, text, and a valid color")
		}
	default:
		return fmt.Errorf("review annotation type must be region, drawing, highlight, or note")
	}
	return nil
}

// SlideDigest identifies a slide's reviewable content: its manifest, visual,
// Items, and each Item's code-reference records. An approval records it, so a
// slide edited after its approval is out of date.
func SlideDigest(slide *Slide) (string, error) {
	slideName := filepath.Base(filepath.FromSlash(slide.Path))
	names := []string{slideName, slide.Entrypoint}
	itemKeys := []string{}
	itemPrefix := "30-i-" + FlatTargetKey(slide.Target) + "-"
	entries, err := os.ReadDir(slide.Directory)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), itemPrefix) {
			names = append(names, entry.Name())
		}
	}
	for _, item := range slide.Items {
		itemKeys = append(itemKeys, "40-e-"+FlatTargetKey(item.Target)+"-")
	}
	for _, entry := range entries {
		for _, prefix := range itemKeys {
			if strings.HasPrefix(entry.Name(), prefix) {
				names = append(names, entry.Name())
			}
		}
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(slide.Directory, name))
		if err != nil {
			return "", err
		}
		data = normalizeCheckoutText(data)
		fmt.Fprintf(hash, "%s\x00%d\x00", name, len(data))
		hash.Write(data)
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
