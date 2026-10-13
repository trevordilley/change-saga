package requirements

import "testing"

func TestOverviewChangeReassessesWholeStoryButCarriesUnchangedCriterion(t *testing.T) {
	urn := "urn:change-saga:test:story:checkout"
	r1 := Revision{ID: "r1", Story: urn, Title: "Check out", Statement: "As a buyer I can check out", Overview: "Deliver purchases reliably.", AcceptanceCriteria: []Criterion{{ID: "works", Statement: "An order is confirmed."}}}
	r2 := r1
	r2.ID = "r2"
	r2.Parents = []string{urn + ":revision:r1"}
	r2.Overview = "Deliver purchases reliably, including after interrupted payment."
	heads := documentHeads(Document{SagaID: "test", Stories: []Story{{Identity: StoryIdentity{ID: "checkout"}, Revisions: []Revision{r1, r2}, CurrentRevision: &r2, RevisionHeads: []string{urn + ":revision:r2"}}}})
	if got := heads.revisionChange(urn, urn+":revision:r1", urn+":revision:r2"); got != ReasonStoryObligationChanged {
		t.Fatalf("story rationale change: %s", got)
	}
	if got := heads.revisionChange(urn+":criterion:works", urn+":revision:r1", urn+":revision:r2"); got != CarriedCriterionUnchanged {
		t.Fatalf("independent criterion semantics changed: %s", got)
	}
}

func TestStoryOverviewParticipatesInIdempotency(t *testing.T) {
	root := newSaga(t)
	input := storyInput("overview", "r1", "proposed", []Criterion{{ID: "works", Statement: "It works"}})
	input.Overview = "## Purpose\n\nPreserve intent."
	if _, err := AddStory(root, "test", input); err != nil {
		t.Fatal(err)
	}
	if replay, err := AddStory(root, "test", input); err != nil || !replay.Replayed {
		t.Fatalf("identical request: %+v %v", replay, err)
	}
	input.Overview = "Different rationale."
	if _, err := AddStory(root, "test", input); err == nil {
		t.Fatal("same request ID with different overview silently replayed")
	}
}
