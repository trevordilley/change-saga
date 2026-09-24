package reviewstore

import (
	"github.com/twentyideas/changesaga/internal/saga"
	"math"
	"testing"
)

func TestAnnotationRefusalsLeaveValidAppendOnlyHistory(t *testing.T) {
	root := reviewSaga(t)
	human := saga.ReviewerIdentity{Kind: "human"}
	anchor := &saga.ReviewAnchor{Type: "region", Coordinate: "normalized", Shapes: []saga.ReviewShape{{Type: "rect", X: .1, Y: .1, Width: .2, Height: .2, StrokeWidth: .004}}}
	created, err := Comment(root, Remark{Review: "pr-7", Target: "why", Body: "mark", Anchor: anchor, AnnotationAction: "create", Reviewer: human})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Comment(root, Remark{Review: "pr-7", ReplyTo: created.ID, Anchor: anchor, AnnotationAction: "delete", Reviewer: human}); err == nil {
		t.Fatal("delete carrying anchor accepted")
	}
	for _, width := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		anchor.Shapes[0].StrokeWidth = width
		if _, err := Comment(root, Remark{Review: "pr-7", Target: "why", Anchor: anchor, AnnotationAction: "create", Reviewer: human}); err == nil {
			t.Fatalf("stroke %v accepted", width)
		}
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid || len(document.FindReview("pr-7").Comments) != 1 {
		t.Fatalf("refusal poisoned history: %v %+v", err, validation.Issues)
	}
}
