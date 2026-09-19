// Package reviewstore writes pull request reviews: the review record and its
// deck, per-slide decisions, comments, and the frozen range after merge. It
// records what reviewers did; it never decides whether a review is approved.
package reviewstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

// MaxBodyRunes bounds a decision note or comment.
const MaxBodyRunes = 20_000

// mutate runs operation on the freshly loaded, valid Saga under the writer
// lock, so a decision is checked against exactly the records it joins.
func mutate(root string, operation func(*saga.Saga) error) error {
	return store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		document, validation, err := saga.Load(root)
		if err != nil {
			return err
		}
		if !validation.Valid {
			return fmt.Errorf("cannot record into a structurally invalid Saga; run change-saga validate")
		}
		return operation(document)
	})
}

// Create writes a new review: review.json and its deck record.
func Create(root string, manifest saga.ReviewManifest, deck saga.DeckManifest) error {
	manifest.Schema, manifest.Version = saga.ReviewSchemaURL, saga.ReviewVersion
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	if err := saga.ValidateReviewManifest(manifest); err != nil {
		return err
	}
	deck.Version, deck.Role = saga.DeckRecordVersion, saga.DeckRoleReview
	if !saga.ValidID(deck.ID) || strings.TrimSpace(deck.Title) == "" || strings.TrimSpace(deck.Objective) == "" || utf8.RuneCountInString(deck.Objective) > 240 {
		return fmt.Errorf("the review deck needs a stable id, a title, and an objective of 1 to 240 characters")
	}
	return mutate(root, func(document *saga.Saga) error {
		if document.FindReview(manifest.ID) != nil {
			return fmt.Errorf("review %q already exists", manifest.ID)
		}
		if number := prNumber(manifest); number != 0 {
			for _, other := range document.Reviews {
				if prNumber(other.ReviewManifest) == number {
					return fmt.Errorf("pull request #%d already has review %q; a pull request has one review", number, other.ID)
				}
			}
		}
		deckTarget := saga.ReviewDeckTarget(document.Manifest.ID, manifest.ID, deck.ID)
		name, err := saga.FlatDeckFilename(deckTarget, deck.Rank)
		if err != nil {
			return err
		}
		reviews, err := store.EnsureDirWithin(document.Root, filepath.Join(document.Root, saga.ReviewsDir))
		if err != nil {
			return err
		}
		dir := filepath.Join(reviews, manifest.ID+saga.ReviewSuffix)
		if len(filepath.Join(dir, saga.ReviewDeckDir, strings.Repeat("x", saga.FlatMaxBasename))) > saga.FlatMaxPath {
			return fmt.Errorf("review path exceeds the portable %d-character budget; choose a shorter review id or Saga location", saga.FlatMaxPath)
		}
		err = store.CommitDir(document.Root, dir, func(stage string) error {
			if err := os.Chmod(stage, 0o755); err != nil {
				return err
			}
			if err := store.WriteJSON(filepath.Join(stage, saga.ReviewManifestName), manifest, true); err != nil {
				return err
			}
			if err := os.Mkdir(filepath.Join(stage, saga.ReviewDeckDir), 0o755); err != nil {
				return err
			}
			return store.WriteJSON(filepath.Join(stage, saga.ReviewDeckDir, name), deck, true)
		})
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("review %q already exists", manifest.ID)
		}
		return err
	})
}

func prNumber(manifest saga.ReviewManifest) int {
	if manifest.PullRequest == nil {
		return 0
	}
	return manifest.PullRequest.Number
}

// Decision is one reviewer's decision on one review slide.
type Decision struct {
	Review   string
	Slide    string
	State    string
	Reviewer saga.ReviewerIdentity
	// Commit is the pull request head the decision is given at.
	Commit string
	Body   string
}

// Decide appends a per-slide decision, recording the slide's content digest
// so a later edit to the slide puts the decision out of date.
func Decide(root string, decision Decision) (saga.ReviewApproval, error) {
	var written saga.ReviewApproval
	if !saga.ValidReviewApprovalState(decision.State) {
		return written, fmt.Errorf("a decision is approved, changes_requested, or none")
	}
	if err := saga.ValidateReviewerIdentity(&decision.Reviewer); err != nil {
		return written, err
	}
	if !coderef.ValidCommit(decision.Commit) {
		return written, fmt.Errorf("a decision records the full head commit it was given at")
	}
	if utf8.RuneCountInString(decision.Body) > MaxBodyRunes {
		return written, fmt.Errorf("the note exceeds %d characters", MaxBodyRunes)
	}
	err := mutate(root, func(document *saga.Saga) error {
		review, slide, err := findSlide(document, decision.Review, decision.Slide)
		if err != nil {
			return err
		}
		if review.Merged != nil {
			return fmt.Errorf("review %q is history: its change landed as %s", review.ID, review.Merged.Landed)
		}
		digest, err := saga.SlideDigest(slide)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		written = saga.ReviewApproval{
			Schema: saga.ReviewApprovalSchemaURL, Version: saga.ReviewVersion, ID: store.EventID(now),
			Slide: slide.ID, State: decision.State, Reviewer: decision.Reviewer, Commit: decision.Commit,
			SlideDigest: digest, Body: strings.TrimSpace(decision.Body), CreatedAt: now,
		}
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(review.Directory, saga.ReviewApprovalsDir))
		if err != nil {
			return err
		}
		written.Path = filepath.Join(dir, written.ID+".json")
		return store.WriteJSON(written.Path, written, true)
	})
	return written, err
}

// Remark is one comment to append.
type Remark struct {
	Review string
	// Target is a review slide or Item: its ID, "slide/item", or its URN.
	// It is ignored for a reply, which joins the thread it replies to.
	Target   string
	ReplyTo  string
	Body     string
	State    string
	Reviewer saga.ReviewerIdentity
	Commit   string
}

// Comment appends a comment on a review slide or Item, or a reply.
func Comment(root string, remark Remark) (saga.ReviewComment, error) {
	var written saga.ReviewComment
	if strings.TrimSpace(remark.Body) == "" {
		return written, fmt.Errorf("a comment needs a body")
	}
	if utf8.RuneCountInString(remark.Body) > MaxBodyRunes {
		return written, fmt.Errorf("the comment exceeds %d characters", MaxBodyRunes)
	}
	if remark.State != "" && remark.State != saga.CommentOpen && remark.State != saga.CommentResolved {
		return written, fmt.Errorf("a comment may set its thread open or resolved")
	}
	if err := saga.ValidateReviewerIdentity(&remark.Reviewer); err != nil {
		return written, err
	}
	if remark.Commit != "" && !coderef.ValidCommit(remark.Commit) {
		return written, fmt.Errorf("a comment's commit must be a full commit")
	}
	err := mutate(root, func(document *saga.Saga) error {
		review := document.FindReview(remark.Review)
		if review == nil {
			return fmt.Errorf("review %q does not exist", remark.Review)
		}
		target := ""
		if remark.ReplyTo != "" {
			for _, comment := range review.Comments {
				if comment.ID == remark.ReplyTo {
					target = comment.Target
				}
			}
			if target == "" {
				return fmt.Errorf("comment %q does not exist in review %q", remark.ReplyTo, review.ID)
			}
		} else {
			var err error
			if target, err = resolveTarget(review, remark.Target); err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		written = saga.ReviewComment{
			Schema: saga.ReviewCommentSchemaURL, Version: saga.ReviewVersion, ID: store.EventID(now),
			Target: target, ReplyTo: remark.ReplyTo, Body: strings.TrimSpace(remark.Body), State: remark.State,
			Reviewer: remark.Reviewer, Commit: remark.Commit, CreatedAt: now,
		}
		dir, err := store.EnsureDirWithin(document.Root, filepath.Join(review.Directory, saga.ReviewCommentsDir))
		if err != nil {
			return err
		}
		written.Path = filepath.Join(dir, written.ID+".json")
		return store.WriteJSON(written.Path, written, true)
	})
	return written, err
}

// Freeze records the exact commits of a landed review in review.json.
func Freeze(root, reviewID string, merged saga.ReviewMerge) error {
	return mutate(root, func(document *saga.Saga) error {
		review := document.FindReview(reviewID)
		if review == nil {
			return fmt.Errorf("review %q does not exist", reviewID)
		}
		manifest := review.ReviewManifest
		manifest.Merged = &merged
		if err := saga.ValidateReviewManifest(manifest); err != nil {
			return err
		}
		return store.WriteJSON(filepath.Join(review.Directory, saga.ReviewManifestName), manifest, false)
	})
}

func findSlide(document *saga.Saga, reviewID, slideID string) (*saga.Review, *saga.Slide, error) {
	review := document.FindReview(reviewID)
	if review == nil {
		return nil, nil, fmt.Errorf("review %q does not exist", reviewID)
	}
	slide := review.Slide(slideID)
	if slide == nil {
		return nil, nil, fmt.Errorf("review %q has no slide %q", reviewID, slideID)
	}
	return review, slide, nil
}

// resolveTarget accepts a review slide or Item as its URN, its slide ID, or
// "<slide>/<item>".
func resolveTarget(review *saga.Review, value string) (string, error) {
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			if value == slide.ID || value == slide.Target {
				return slide.Target, nil
			}
			for _, item := range slide.Items {
				if value == item.Target || value == slide.ID+"/"+item.ID {
					return item.Target, nil
				}
			}
		}
	}
	return "", fmt.Errorf("review %q has no slide or Item %q; name a slide id, <slide>/<item>, or its URN", review.ID, value)
}
