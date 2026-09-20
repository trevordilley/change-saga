package requirements

import (
	"strings"
	"testing"
	"time"
)

// A repin records that somebody read a revision and confirmed the relation
// against it. Recording the pin it already confirms records nothing, and these
// records are immutable, so the noise would be permanent. The refusal has to
// live on the write: passing the pin explicitly is exactly what the stale
// next action prints, so a guard on the defaulting path alone is no guard.
func TestRepinRefusesThePinsItAlreadyConfirms(t *testing.T) {
	root := newV5Saga(t)
	if _, err := AddRelation(root, "test", verifiesInput()); err != nil {
		t.Fatal(err)
	}
	storyR2 := "urn:change-saga:test:story:checkout:revision:r2"
	if _, err := ReviseStory(root, "test", ReviseStoryInput{Personas: testPersonas,
		Story: v5Story, ID: "r2", Parents: []string{v5StoryR1}, Title: "Checkout", Statement: "As a buyer, I can check out",
		AcceptanceCriteria: []Criterion{{ID: "fast", Statement: "Checkout finishes in two seconds"}},
		CreatedAt:          testTime.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	advance := func(id, rationale string) error {
		_, err := RepinRelation(root, "test", RepinRelationInput{
			Relation: v5RelationURN, ID: id, ToRevision: storyR2, Rationale: rationale, CreatedAt: testTime.Add(2 * time.Minute)})
		return err
	}
	// The first advances the confirmed pin from r1 to r2: a real confirmation.
	if err := advance("r2", "Two seconds is the promptness the case asserts."); err != nil {
		t.Fatalf("first repin: %v", err)
	}
	// The second names the pin already confirmed, however it was arrived at.
	if err := advance("r3", "Saying it again."); err == nil || !strings.Contains(err.Error(), "nothing to re-pin") {
		t.Fatalf("second repin of the same pin = %v", err)
	}
	// Naming no pin at all is the same no-op by another route.
	if _, err := RepinRelation(root, "test", RepinRelationInput{
		Relation: v5RelationURN, Rationale: "Nothing moved.", CreatedAt: testTime.Add(3 * time.Minute),
	}); err == nil || !strings.Contains(err.Error(), "nothing to re-pin") {
		t.Fatalf("repin naming no pin = %v", err)
	}
	document, err := Load(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	if repins := document.Relations[0].Repins; len(repins) != 1 || repins[0].ID != "r2" {
		t.Fatalf("only the confirmation that moved a pin was written: %#v", repins)
	}
}
