package reviewstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"
)

const commit = "0a103972ac26d8dfd8a4a1f3be0b1b9b5a2c4e61"

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// reviewSaga is an app Saga with one review whose deck has one slide and Item.
func reviewSaga(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "app.saga")
	writeJSON(t, filepath.Join(root, saga.ManifestName), saga.Manifest{Schema: saga.SagaSchemaURL, Version: saga.SagaVersion, ID: "app", Title: "App", Source: saga.Source{Repository: "https://example.test/acme/app.git"}})
	if err := Create(root, saga.ReviewManifest{ID: "pr-7", Title: "PR 7", Base: "main", PullRequest: &saga.PullRequest{Number: 7}}, saga.DeckManifest{ID: "pr-7", Title: "PR 7", Objective: "Explain the change."}); err != nil {
		t.Fatal(err)
	}
	deckDir := filepath.Join(saga.ReviewDir(root, "pr-7"), saga.ReviewDeckDir)
	deck, slide := saga.ReviewDeckTarget("app", "pr-7", "pr-7"), saga.ReviewSlideTarget("app", "pr-7", "why")
	slideName, _ := saga.FlatSlideFilename(deck, slide, 0)
	asset, _ := saga.FlatSlideAssetFilename(slideName, ".svg")
	writeJSON(t, filepath.Join(deckDir, slideName), saga.SlideManifest{Version: saga.DeckRecordVersion, ID: "why", DeckID: "pr-7", Title: "Why", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Entrypoint: asset, Takeaway: "Why it moved.", ReadingOrder: []string{"node"}})
	if err := os.WriteFile(filepath.Join(deckDir, asset), []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect id="node" width="5" height="5"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	item := saga.ReviewItemTarget("app", "pr-7", "why", "node")
	itemName, _ := saga.FlatItemFilename(slide, item, 0)
	writeJSON(t, filepath.Join(deckDir, itemName), saga.ItemManifest{Version: saga.DeckRecordVersion, ID: "node", SlideID: "why", Kind: "node", Label: "Node", Description: "The node.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "node"}, Record: "urn:change-saga:app:story:enqueue"})
	return root
}

func loadIssues(t *testing.T, root string) string {
	t.Helper()
	_, validation, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	var issues []string
	for _, issue := range validation.Issues {
		if issue.Severity == "error" {
			issues = append(issues, issue.Message)
		}
	}
	return strings.Join(issues, "\n")
}

func TestReviewRecordsRoundTripAndValidate(t *testing.T) {
	root := reviewSaga(t)
	if issues := loadIssues(t, root); issues != "" {
		t.Fatalf("a fresh review is invalid:\n%s", issues)
	}
	human := saga.ReviewerIdentity{Kind: "human"}
	approval, err := Decide(root, Decision{Review: "pr-7", Slide: "why", State: saga.ApprovalApproved, Reviewer: human, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", Target: "why/node", Body: "Why here?", Reviewer: human}); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	review := document.FindReview("pr-7")
	if review.Target != "urn:change-saga:app:review:pr-7" || len(review.Approvals) != 1 || review.Approvals[0].SlideDigest != approval.SlideDigest || len(review.Comments) != 1 || review.Comments[0].Target != saga.ReviewItemTarget("app", "pr-7", "why", "node") {
		t.Fatalf("review = %#v", review)
	}
	if _, err := Decide(root, Decision{Review: "pr-7", Slide: "missing", State: saga.ApprovalApproved, Reviewer: human, Commit: commit}); err == nil {
		t.Fatal("a decision on a missing slide was accepted")
	}
	if _, err := Decide(root, Decision{Review: "pr-7", Slide: "why", State: saga.ApprovalApproved, Reviewer: saga.ReviewerIdentity{Kind: "ai"}, Commit: commit}); err == nil {
		t.Fatal("an AI decision without its seat was accepted")
	}
	if err := Freeze(root, "pr-7", saga.ReviewMerge{Base: commit, Head: commit, Landed: commit, MergedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if _, err := Decide(root, Decision{Review: "pr-7", Slide: "why", State: saga.ApprovalApproved, Reviewer: human, Commit: commit}); err == nil || !strings.Contains(err.Error(), "history") {
		t.Fatalf("a decision on a merged review = %v", err)
	}
}

func TestReviewAnnotationEditsAreAppendOnlyComments(t *testing.T) {
	root := reviewSaga(t)
	human := saga.ReviewerIdentity{Kind: "human"}
	anchor := &saga.ReviewAnchor{Type: "region", Coordinate: "normalized", Shapes: []saga.ReviewShape{{Type: "rect", X: .2, Y: .25, Width: .3, Height: .2, Color: "#d04832", StrokeWidth: .004}}}
	created, err := Comment(root, Remark{Review: "pr-7", Target: "why", Body: "Clarify this transition.", Reviewer: human, Anchor: anchor, AnnotationAction: "create"})
	if err != nil {
		t.Fatal(err)
	}
	moved := *anchor
	moved.Shapes = append([]saga.ReviewShape(nil), anchor.Shapes...)
	moved.Shapes[0].X = .3
	if _, err := Comment(root, Remark{Review: "pr-7", ReplyTo: created.ID, Body: "Annotation moved.", Reviewer: human, Anchor: &moved, AnnotationAction: "update"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", ReplyTo: created.ID, Body: "Annotation deleted.", Reviewer: human, AnnotationAction: "delete"}); err != nil {
		t.Fatal(err)
	}
	ordinary, err := Comment(root, Remark{Review: "pr-7", Target: "why", Body: "Ordinary discussion.", Reviewer: human})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", ReplyTo: ordinary.ID, Body: "Invalid annotation update.", Reviewer: human, Anchor: &moved, AnnotationAction: "update"}); err == nil || !strings.Contains(err.Error(), "annotation create root") {
		t.Fatalf("annotation update on an ordinary thread = %v", err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", Target: "why", Body: "Anchor without action.", Reviewer: human, Anchor: anchor}); err == nil || !strings.Contains(err.Error(), "annotation action") {
		t.Fatalf("anchor without annotation action = %v", err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: %v %#v", err, validation.Issues)
	}
	comments := document.FindReview("pr-7").Comments
	if len(comments) != 4 || comments[0].AnnotationAction != "create" || comments[1].AnnotationAction != "update" || comments[2].AnnotationAction != "delete" || comments[1].ReplyTo != created.ID || comments[3].ID != ordinary.ID {
		t.Fatalf("annotation history = %#v", comments)
	}
	if anchor.Shapes[0].X != .2 {
		t.Fatal("updating an annotation rewrote its original anchor")
	}
}

func TestLoadRejectsMalformedReviewRecords(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	approval := func(mutate func(*saga.ReviewApproval)) saga.ReviewApproval {
		value := saga.ReviewApproval{Schema: saga.ReviewApprovalSchemaURL, Version: saga.ReviewVersion, ID: "a1", Slide: "why", State: saga.ApprovalApproved, Reviewer: saga.ReviewerIdentity{Kind: "human"}, Commit: commit, SlideDigest: "sha256:" + strings.Repeat("ab", 32), CreatedAt: now}
		mutate(&value)
		return value
	}
	comment := func(mutate func(*saga.ReviewComment)) saga.ReviewComment {
		value := saga.ReviewComment{Schema: saga.ReviewCommentSchemaURL, Version: saga.ReviewVersion, ID: "c1", Target: saga.ReviewSlideTarget("app", "pr-7", "why"), Body: "hi", Reviewer: saga.ReviewerIdentity{Kind: "human"}, CreatedAt: now}
		mutate(&value)
		return value
	}
	codeLine := func(mutate func(*saga.ReviewCodeLine)) *saga.ReviewCodeLine {
		value := saga.ReviewCodeLine{Commit: commit, Path: "queue.go", Side: saga.CodeLineNew, Start: 3, End: 4, Digest: "sha256:" + strings.Repeat("cd", 32)}
		mutate(&value)
		return &value
	}
	cases := []struct {
		name  string
		write func(t *testing.T, dir string)
		want  string
	}{
		{"approval names a missing slide", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "approvals", "a1.json"), approval(func(a *saga.ReviewApproval) { a.Slide = "gone" }))
		}, "not in this review's deck"},
		{"approval has no head commit", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "approvals", "a1.json"), approval(func(a *saga.ReviewApproval) { a.Commit = "main" }))
		}, "full head commit"},
		{"approval state is a verdict word", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "approvals", "a1.json"), approval(func(a *saga.ReviewApproval) { a.State = "rejected" }))
		}, "approved, changes_requested, or none"},
		{"approval filename disagrees with its id", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "approvals", "other.json"), approval(func(*saga.ReviewApproval) {}))
		}, "matching its filename"},
		{"comment targets documentation", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) { c.Target = saga.FragmentTarget("app", "overview") }))
		}, "slide or Item of this review"},
		{"comment replies to nothing", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) { c.ReplyTo = "missing" }))
		}, "unknown comment"},
		{"annotation update replies to ordinary comment", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(*saga.ReviewComment) {}))
			writeJSON(t, filepath.Join(dir, "comments", "c2.json"), comment(func(c *saga.ReviewComment) {
				c.ID = "c2"
				c.ReplyTo = "c1"
				c.AnnotationAction = "update"
				c.Anchor = &saga.ReviewAnchor{Type: "region", Coordinate: "normalized", Shapes: []saga.ReviewShape{{Type: "rect", X: .2, Y: .2, Width: .2, Height: .2}}}
			}))
		}, "annotation create root"},
		{"code line on a reply", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(*saga.ReviewComment) {}))
			writeJSON(t, filepath.Join(dir, "comments", "c2.json"), comment(func(c *saga.ReviewComment) {
				c.ID, c.ReplyTo, c.CodeLine = "c2", "c1", codeLine(func(*saga.ReviewCodeLine) {})
			}))
		}, "anchors a root comment"},
		{"code line on an unknown side", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(l *saga.ReviewCodeLine) { l.Side = "left" })
			}))
		}, "side must be new or old"},
		{"code line without lines", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(l *saga.ReviewCodeLine) { l.Start, l.End = 0, 0 })
			}))
		}, "1 <= start <= end"},
		{"code line at a branch name", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(l *saga.ReviewCodeLine) { l.Commit = "main" })
			}))
		}, "full lowercase Git object name"},
		{"code line outside the repository", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(l *saga.ReviewCodeLine) { l.Path = "../secrets.go" })
			}))
		}, "repository-relative"},
		{"code line without a digest", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(l *saga.ReviewCodeLine) { l.Digest = "" })
			}))
		}, "digest"},
		{"code line with slide markup", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) {
				c.CodeLine = codeLine(func(*saga.ReviewCodeLine) {})
				c.AnnotationAction = "create"
				c.Anchor = &saga.ReviewAnchor{Type: "region", Coordinate: "normalized", Shapes: []saga.ReviewShape{{Type: "rect", X: .2, Y: .2, Width: .2, Height: .2}}}
			}))
		}, "annotation markup"},
		{"review-wide comment without code lines", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "comments", "c1.json"), comment(func(c *saga.ReviewComment) { c.Target = saga.ReviewTarget("app", "pr-7") }))
		}, "or the review itself for a code-line comment"},
		{"review holds an unknown entry", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "notes.json"), map[string]string{})
		}, "unknown entry in a review"},
		{"review.json names another id", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, saga.ReviewManifestName), saga.ReviewManifest{Schema: saga.ReviewSchemaURL, Version: saga.ReviewVersion, ID: "pr-8", Title: "x", Base: "main", CreatedAt: now})
		}, "must match its directory"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := reviewSaga(t)
			test.write(t, saga.ReviewDir(root, "pr-7"))
			if issues := loadIssues(t, root); !strings.Contains(issues, test.want) {
				t.Fatalf("issues do not mention %q:\n%s", test.want, issues)
			}
		})
	}
	root := reviewSaga(t)
	if err := os.MkdirAll(filepath.Join(root, saga.ReviewsDir, "loose"), 0o755); err != nil {
		t.Fatal(err)
	}
	if issues := loadIssues(t, root); !strings.Contains(issues, "<id>.review") {
		t.Fatalf("a non-review directory was accepted:\n%s", issues)
	}
}

func TestCodeLineCommentsAreRootThreadsOnTheReviewOrItsItems(t *testing.T) {
	root := reviewSaga(t)
	human := saga.ReviewerIdentity{Kind: "human"}
	line := saga.ReviewCodeLine{Commit: commit, Path: "internal/queue.go", Side: saga.CodeLineOld, Start: 12, End: 12, Digest: "sha256:" + strings.Repeat("ef", 32)}
	onReview, err := Comment(root, Remark{Review: "pr-7", Body: "Deleted on purpose?", Reviewer: human, CodeLine: &line, Commit: commit})
	if err != nil {
		t.Fatal(err)
	}
	onItem, err := Comment(root, Remark{Review: "pr-7", Target: "why/node", Body: "Batch this.", Reviewer: human, CodeLine: &line})
	if err != nil {
		t.Fatal(err)
	}
	if onReview.Target != saga.ReviewTarget("app", "pr-7") || onItem.Target != saga.ReviewItemTarget("app", "pr-7", "why", "node") {
		t.Fatalf("targets = %s, %s", onReview.Target, onItem.Target)
	}
	reply, err := Comment(root, Remark{Review: "pr-7", ReplyTo: onReview.ID, Body: "Yes.", State: saga.CommentResolved, Reviewer: human})
	if err != nil || reply.Target != onReview.Target || reply.CodeLine != nil {
		t.Fatalf("reply = %#v, %v", reply, err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", ReplyTo: onReview.ID, Body: "Moved?", Reviewer: human, CodeLine: &line}); err == nil || !strings.Contains(err.Error(), "joins its thread") {
		t.Fatalf("a reply with its own lines = %v", err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", Body: "No lines.", Reviewer: human}); err == nil {
		t.Fatal("a comment with neither a target nor lines was accepted")
	}
	bad := line
	bad.Side = "both"
	if _, err := Comment(root, Remark{Review: "pr-7", Body: "Both?", Reviewer: human, CodeLine: &bad}); err == nil || !strings.Contains(err.Error(), "side") {
		t.Fatalf("an unknown side = %v", err)
	}
	if issues := loadIssues(t, root); issues != "" {
		t.Fatalf("code-line comments are invalid:\n%s", issues)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	comments := document.FindReview("pr-7").Comments
	if len(comments) != 3 || comments[0].CodeLine == nil || *comments[0].CodeLine != line {
		t.Fatalf("comments = %#v", comments)
	}
}
