package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/reviewstore"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestReviewSnapshotIncludesSourceVisualAndItemEvidence(t *testing.T) {
	t.Parallel()
	f := newServerReviewFixture(t)
	document, _, err := saga.Load(f.root)
	if err != nil {
		t.Fatal(err)
	}
	review := document.FindReview("pr-7")
	slide := review.Slide("queue")
	rng, err := reviewstate.ResolveRange(context.Background(), f.repo, review)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reviewSlideSnapshot(slide, &rng)
	if snapshot == "" {
		t.Fatal("missing snapshot")
	}
	changed := rng
	changed.HeadOID = rng.BaseOID
	if snapshot == reviewSlideSnapshot(slide, &changed) {
		t.Fatal("changed head reused snapshot")
	}
	changed = rng
	changed.BaseOID = rng.HeadOID
	if snapshot == reviewSlideSnapshot(slide, &changed) {
		t.Fatal("changed base reused snapshot")
	}
	_, err = reviewstore.Comment(f.root, reviewstore.Remark{Review: "pr-7", Target: "queue/node", Body: "Feedback", Reviewer: saga.ReviewerIdentity{Kind: "human"}})
	if err != nil {
		t.Fatal(err)
	}
	document, _, _ = saga.Load(f.root)
	slide = document.FindReview("pr-7").Slide("queue")
	if snapshot != reviewSlideSnapshot(slide, &rng) {
		t.Fatal("feedback invalidated source snapshot")
	}
	path := filepath.Join(slide.Directory, slide.Entrypoint)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeServerFile(t, path, string(data)+"\n<!-- changed -->")
	if snapshot == reviewSlideSnapshot(slide, &rng) {
		t.Fatal("visual edit reused snapshot")
	}

	writeServerFile(t, path, string(data))
	a := &app{root: f.root, sourceDir: f.repo}
	original := a.reviewSnapshot(slide, &rng)
	other := &app{root: f.root, sourceDir: t.TempDir()}
	if original == other.reviewSnapshot(slide, &rng) {
		t.Fatal("another checkout reused snapshot")
	}
	other = &app{root: t.TempDir(), sourceDir: f.repo}
	if original == other.reviewSnapshot(slide, &rng) {
		t.Fatal("another Saga reused snapshot")
	}
	code := slide.Items[0].Code[0]
	code.References[0].Digest = "sha256:" + strings.Repeat("a", 64)
	writeServerJSON(t, filepath.Join(slide.Directory, saga.FlatEvidenceFilename(slide.Items[0].Target, "queue")), code)
	document, validation, err := saga.Load(f.root)
	if err != nil || !validation.Valid {
		t.Fatalf("changed evidence fixture: %v %+v", err, validation.Issues)
	}
	slide = document.FindReview("pr-7").Slide("queue")
	if original == a.reviewSnapshot(slide, &rng) {
		t.Fatal("changed exact evidence reused snapshot")
	}
	request := httptest.NewRequest("POST", "/reviews/pr-7/decision", nil)
	request.Header.Set("Accept", "application/json")
	request.PostForm = url.Values{"snapshot": {original}}
	if err := a.reviewSnapshotCheck(request, rng.HeadOID)(document.FindReview("pr-7"), slide.Target); err == nil {
		t.Fatal("changed exact evidence accepted old view")
	}
}

func TestReviewSnapshotCheckRefusesUnseenOrFrozenSlide(t *testing.T) {
	t.Parallel()
	f := newServerReviewFixture(t)
	document, _, _ := saga.Load(f.root)
	review := document.FindReview("pr-7")
	rng, err := reviewstate.ResolveRange(context.Background(), f.repo, review)
	if err != nil {
		t.Fatal(err)
	}
	a := &app{sourceDir: f.repo}
	r := httptest.NewRequest("POST", "/reviews/pr-7/decision", nil)
	r.Header.Set("Accept", "application/json")
	r.PostForm = url.Values{"snapshot": {a.reviewSnapshot(review.Slide("queue"), &rng)}}
	check := a.reviewSnapshotCheck(r, rng.HeadOID)
	if err := check(review, review.Slide("queue").Target); err != nil {
		t.Fatal(err)
	}
	if err := check(review, review.Slide("queue").Items[0].Target); err != nil {
		t.Fatal(err)
	}
	if err := check(review, "missing"); err == nil {
		t.Fatal("missing target accepted")
	}
	if err := a.reviewSnapshotCheck(r, rng.BaseOID)(review, "queue"); err == nil {
		t.Fatal("moved head accepted")
	}
	review.Merged = &saga.ReviewMerge{}
	if err := check(review, "queue"); err == nil {
		t.Fatal("frozen review accepted")
	}
	review.Merged = nil
	r.PostForm.Del("snapshot")
	if err := a.reviewSnapshotCheck(r, rng.HeadOID)(review, "queue"); err == nil {
		t.Fatal("async missing snapshot accepted")
	}
}

func postAsyncReview(t *testing.T, handler http.Handler, path string, values url.Values) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("POST", path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestReviewAsyncReceiptFeedbackAndFallback(t *testing.T) {
	t.Parallel()
	f := newServerReviewFixture(t)
	a, handler := reviewApp(t, f, gitdiff.Range{})
	document, _, _ := saga.Load(f.root)
	review := document.FindReview("pr-7")
	slide := review.Slide("queue")
	rng, _ := reviewstate.ResolveRange(context.Background(), f.repo, review)
	snapshot := a.reviewSnapshot(slide, &rng)
	values := url.Values{"token": {"review-token"}, "snapshot": {snapshot}, "slide": {"queue"}, "state": {"approved"}}
	for _, state := range []string{"approved", "changes_requested", "none"} {
		values.Set("state", state)
		response := postAsyncReview(t, handler, "/reviews/pr-7/decision", values)
		if response.Code != 200 || response.Header().Get("Location") != "" {
			t.Fatalf("decision %s: %d %s", state, response.Code, response.Body)
		}
		var receipt struct {
			Saved    bool           `json:"saved"`
			Event    string         `json:"event_id"`
			Feedback reviewFeedback `json:"feedback"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
			t.Fatal(err)
		}
		if !receipt.Saved || receipt.Event == "" || receipt.Feedback.Snapshot != snapshot {
			t.Fatalf("receipt: %+v", receipt)
		}
		if state != "none" && (len(receipt.Feedback.Report.Decisions) != 1 || receipt.Feedback.Report.Decisions[0].Currency != reviewstate.Current || receipt.Feedback.Report.Decisions[0].Reviewer.Kind != "human") {
			t.Fatalf("decision projection: %+v", receipt.Feedback.Report)
		}
		for _, forbidden := range []string{"<iframe", "<!DOCTYPE", "review-diff", "coverage-totals"} {
			if strings.Contains(response.Body.String(), forbidden) {
				t.Fatalf("receipt generated %s", forbidden)
			}
		}
	}
	values.Del("slide")
	values.Del("state")
	values.Set("target", slide.Items[0].Target)
	values.Set("body", "Question <script>unsafe</script>")
	first := postAsyncReview(t, handler, "/reviews/pr-7/comment", values)
	var receipt struct {
		Event    string         `json:"event_id"`
		Feedback reviewFeedback `json:"feedback"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("comment %d %s", first.Code, first.Body)
	}
	if !strings.Contains(receipt.Feedback.Threads[slide.Items[0].Target], "Question") {
		t.Fatal("missing Item discussion")
	}
	values.Del("target")
	values.Set("reply_to", receipt.Event)
	values.Set("body", "Reply")
	reply := postAsyncReview(t, handler, "/reviews/pr-7/comment", values)
	if reply.Code != 200 || !strings.Contains(reply.Body.String(), "Reply") {
		t.Fatalf("reply %d %s", reply.Code, reply.Body)
	}
	values.Del("snapshot")
	refused := postAsyncReview(t, handler, "/reviews/pr-7/comment", values)
	if refused.Code != 409 {
		t.Fatalf("missing viewed snapshot %d", refused.Code)
	}
	values.Del("reply_to")
	values.Set("target", "queue")
	values.Set("body", "Ordinary form")
	if fallback := postReview(t, handler, "/reviews/pr-7/comment", values); fallback.Code != 303 {
		t.Fatalf("fallback %d %s", fallback.Code, fallback.Body)
	}
	doc, validation, _ := saga.Load(f.root)
	if !validation.Valid || len(doc.FindReview("pr-7").Comments) != 3 || len(doc.FindReview("pr-7").Approvals) != 3 {
		t.Fatal("append-only count or validation changed")
	}
}

func TestReviewAsyncGuardRechecksUnderWriterLock(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"slide", "head", "frozen"} {
		t.Run(change, func(t *testing.T) {
			f := newServerReviewFixture(t)
			a, _ := reviewApp(t, f, gitdiff.Range{})
			document, _, _ := saga.Load(f.root)
			review := document.FindReview("pr-7")
			slide := review.Slide("queue")
			rng, _ := reviewstate.ResolveRange(context.Background(), f.repo, review)
			request := httptest.NewRequest("POST", "/reviews/pr-7/decision", nil)
			request.Header.Set("Accept", "application/json")
			request.PostForm = url.Values{"snapshot": {a.reviewSnapshot(slide, &rng)}}
			guard := a.reviewSnapshotCheck(request, rng.HeadOID)
			// This preflight succeeds before the competing writer changes the records.
			if err := guard(review, slide.Target); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "slide":
				path := filepath.Join(slide.Directory, slide.Entrypoint)
				data, _ := os.ReadFile(path)
				writeServerFile(t, path, string(data)+"\n<!-- unseen -->")
			case "head":
				writeServerFile(t, filepath.Join(f.repo, "queue.go"), "package queue\n// unseen\n")
				serverGit(t, f.repo, "commit", "-am", "unseen push")
			case "frozen":
				if err := reviewstore.Freeze(f.root, review.ID, saga.ReviewMerge{Base: rng.BaseOID, Head: rng.HeadOID, Landed: rng.HeadOID, MergedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}
			_, err := reviewstore.Decide(f.root, reviewstore.Decision{Review: review.ID, Slide: slide.ID, State: "approved", Commit: rng.HeadOID, Reviewer: saga.ReviewerIdentity{Kind: "human"}, CheckSnapshot: guard})
			if err == nil {
				t.Fatal("unseen content approved")
			}
			doc, validation, _ := saga.Load(f.root)
			if !validation.Valid || len(doc.FindReview(review.ID).Approvals) != 0 {
				t.Fatal("rejected snapshot wrote records")
			}
		})
	}
}

func TestReviewFeedbackAfterConcurrentSlideEditKeepsReceiptAndCurrency(t *testing.T) {
	t.Parallel()
	f := newServerReviewFixture(t)
	a, _ := reviewApp(t, f, gitdiff.Range{})
	document, _, _ := saga.Load(f.root)
	review := document.FindReview("pr-7")
	slide := review.Slide("queue")
	rng, _ := reviewstate.ResolveRange(context.Background(), f.repo, review)
	shown := a.reviewSnapshot(slide, &rng)
	decision, err := reviewstore.Decide(f.root, reviewstore.Decision{Review: review.ID, Slide: slide.ID, State: "approved", Commit: rng.HeadOID, Reviewer: saga.ReviewerIdentity{Kind: "human"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(slide.Directory, slide.Entrypoint)
	data, _ := os.ReadFile(path)
	writeServerFile(t, path, string(data)+"\n<!-- changed after append -->")
	request := httptest.NewRequest("POST", "/reviews/pr-7/decision", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	if !a.reviewSaved(response, request, review.ID, decision.ID, slide.Target) {
		t.Fatal("missing receipt")
	}
	var receipt struct {
		Saved    bool           `json:"saved"`
		Event    string         `json:"event_id"`
		Feedback reviewFeedback `json:"feedback"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Saved || receipt.Event != decision.ID || receipt.Feedback.Snapshot == shown || receipt.Feedback.Report.Decisions[0].Currency != reviewstate.OutOfDate {
		t.Fatalf("concurrent projection: %+v", receipt)
	}
}
