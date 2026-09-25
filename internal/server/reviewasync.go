package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

type reviewFeedback struct {
	Target   string                  `json:"target"`
	Snapshot string                  `json:"snapshot"`
	Frozen   bool                    `json:"frozen"`
	Menu     string                  `json:"menu"`
	Report   reviewstate.SlideReport `json:"report"`
	Threads  map[string]string       `json:"threads"`
}

// reviewFeedbackFor rebuilds only the selected slide's controls and discussion.
// It never constructs file patches, coverage markup, a shell or visual iframe.
func (a *app) reviewFeedbackFor(r *http.Request, id, target string) (*reviewFeedback, error) {
	document, validation, err := saga.Load(a.root)
	if err != nil || !validation.Valid {
		return nil, fmt.Errorf("The saved feedback could not be loaded. Check saved feedback again.")
	}
	review := document.FindReview(id)
	if review == nil {
		return nil, fmt.Errorf("The review is unavailable.")
	}
	slide := reviewTargetSlide(review, target)
	if slide == nil {
		return nil, fmt.Errorf("The slide is unavailable.")
	}
	resolver, err := coderesolve.New(r.Context(), a.sourceDir)
	if err == nil {
		defer resolver.Close()
	} else {
		resolver = nil
	}
	report := reviewstate.Build(r.Context(), review, reviewstate.Options{Checkout: a.sourceDir, SagaRoot: document.Root, Resolver: resolver, Repository: document.Manifest.Source.Repository, SkipCoverage: true})
	view := reviewPageView{Saga: document, Review: review, Report: report, Frozen: review.Merged != nil, MutationToken: a.mutationToken}
	sv := &reviewSlideView{Slide: slide}
	for _, state := range report.Slides {
		if state.Target == slide.Target {
			sv.Report = state
		}
	}
	threads := reviewstate.Threads(review.Comments)
	sv.Threads = threadViewsFor(threads, slide.Target, id, a.mutationToken, view.Frozen)
	var menu bytes.Buffer
	if err := reviewTemplates.ExecuteTemplate(&menu, "review-slide-menu", reviewSlideMenuView{Page: view, Slide: sv}); err != nil {
		return nil, err
	}
	feedback := &reviewFeedback{Target: slide.Target, Snapshot: a.reviewSnapshot(slide, report.Range), Frozen: view.Frozen, Menu: menu.String(), Report: sv.Report, Threads: map[string]string{}}
	targets := []string{slide.Target}
	for _, item := range slide.Items {
		targets = append(targets, item.Target)
	}
	for _, target := range targets {
		var body bytes.Buffer
		if err := reviewTemplates.ExecuteTemplate(&body, "review-threads", threadViewsFor(threads, target, id, a.mutationToken, view.Frozen)); err != nil {
			return nil, err
		}
		feedback.Threads[target] = body.String()
	}
	return feedback, nil
}

func (a *app) reviewFeedbackSurface(w http.ResponseWriter, r *http.Request) {
	feedback, err := a.reviewFeedbackFor(r, r.PathValue("id"), r.URL.Query().Get("target"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeIncrementalHeaders(w, "application/json")
	_ = json.NewEncoder(w).Encode(feedback)
}

func (a *app) reviewSaved(w http.ResponseWriter, r *http.Request, id, event, target string) bool {
	if !asyncReviewRequest(r) {
		return false
	}
	feedback, err := a.reviewFeedbackFor(r, id, target)
	response := struct {
		Saved    bool            `json:"saved"`
		EventID  string          `json:"event_id"`
		Target   string          `json:"target"`
		Feedback *reviewFeedback `json:"feedback,omitempty"`
		Warning  string          `json:"warning,omitempty"`
	}{Saved: true, EventID: event, Target: target, Feedback: feedback}
	if err != nil {
		response.Warning = err.Error()
	}
	writeIncrementalHeaders(w, "application/json")
	_ = json.NewEncoder(w).Encode(response)
	return true
}

func reviewWriteError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errReviewViewChanged) {
		status = http.StatusConflict
	}
	http.Error(w, err.Error(), status)
}

var errReviewViewChanged = errors.New("The review changed since this slide was shown. Reload and inspect the current slide before saving.")

// reviewSlideSnapshot binds a shown slide, its visual, Items and exact evidence
// to the resolved source comparison. Feedback events deliberately do not enter
// this identity: another comment does not change what a reviewer has seen.
func reviewSlideSnapshot(slide *saga.Slide, rng *reviewstate.Range) string {
	if slide == nil || rng == nil {
		return ""
	}
	digest, err := saga.SlideDigest(slide)
	if err != nil {
		return ""
	}
	data, _ := json.Marshal([]any{slide.Target, digest, rng})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// A snapshot from another served checkout or Saga must not authorize this view.
func (a *app) reviewSnapshot(slide *saga.Slide, rng *reviewstate.Range) string {
	base := reviewSlideSnapshot(slide, rng)
	if base == "" {
		return ""
	}
	canonical := func(path string) string {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		absolute, _ := filepath.Abs(path)
		return absolute
	}
	data, _ := json.Marshal([]string{base, canonical(a.sourceDir), canonical(a.root)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func reviewTargetSlide(review *saga.Review, target string) *saga.Slide {
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			if target == slide.Target || target == slide.ID {
				return slide
			}
			for _, item := range slide.Items {
				if item.Target == target {
					return slide
				}
			}
		}
	}
	return nil
}

func asyncReviewRequest(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

// reviewSnapshotCheck is called by the store with its freshly loaded review
// under the Saga writer lock. A push after this check cannot be approved by
// accident: the event still records the explicitly checked old head commit.
func (a *app) reviewSnapshotCheck(r *http.Request, head string) func(*saga.Review, string) error {
	expected := r.PostForm.Get("snapshot")
	if expected == "" && !asyncReviewRequest(r) {
		return nil // Older ordinary forms and CLI callers retain their contract.
	}
	return func(review *saga.Review, target string) error {
		if review.Merged != nil || expected == "" {
			return errReviewViewChanged
		}
		rng, err := reviewstate.ResolveRange(r.Context(), a.sourceDir, review)
		if err != nil || rng.HeadOID != head || a.reviewSnapshot(reviewTargetSlide(review, target), &rng) != expected {
			return errReviewViewChanged
		}
		return nil
	}
}
