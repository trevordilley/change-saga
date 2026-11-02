package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/saga"
)

// A UX deck's slides are not asked for code, as SPEC says, so a slide drawn from
// a diagram publishes with none. Otherwise it follows the implementation Item
// rules: it still links a criterion, and any code it does cite must be exact.
func TestUXDeckPublishesADiagramSlideWithNoCode(t *testing.T) {
	t.Parallel()
	fixture := newPlanFixture(t)
	root := fixture.root
	run(t, AddDeck, "--feature", testFeature, "--role", "ux", "--id", "flows", "--objective", "Walk the flow.", root, "flows")
	run(t, AddDeck, "--feature", testFeature, "--id", "impl", "--objective", "Explain.", root, "impl")
	run(t, Story, "add", "--feature", testFeature, "--id", "walk", "--revision", "r1", "--event", "proposed",
		"--title", "Walk the flow", "--statement", "As a user I walk the flow", "--criterion", "steps=The flow has steps", "--persona", personaURNFor("app"), root)
	request := func(deck, id string) SlideTransactionRequest {
		slide := planSlideRequest("")
		slide.Plan, slide.Deck, slide.RequestID, slide.Slide.ID = "", deck, id, id
		slide.Items[0].CriterionLinks = []saga.CriterionLink{{ID: id + "-explains-steps", Criterion: "urn:change-saga:app:story:walk:criterion:steps", StoryRevision: "urn:change-saga:app:story:walk:revision:r1", Rationale: "It walks the flow."}}
		return slide
	}
	ctx := context.Background()

	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, request("flows", "flow"), false); err != nil {
		t.Fatalf("a UX slide with no code was refused: %v", err)
	}
	document, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if deck := findDeck(document, "flows"); deck == nil || len(deck.Slides) != 1 || deck.Slides[0].ID != "flow" || len(deck.Slides[0].Items) != 1 {
		t.Fatalf("deck = %#v", deck)
	}

	unlinked := request("flows", "unlinked")
	unlinked.Items[0].CriterionLinks = nil
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, unlinked, false); err == nil || !strings.Contains(err.Error(), "criterion link") {
		t.Fatalf("a UX Item with no criterion link was accepted: %v", err)
	}
	cited := request("flows", "cited")
	cited.Items[0].Evidence = []saga.CodeFile{{Version: 2}}
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, cited, false); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("a UX Item citing bad code was accepted: %v", err)
	}
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, request("impl", "code"), false); err == nil || !strings.Contains(err.Error(), "requires exact code evidence") {
		t.Fatalf("an implementation slide with no code was accepted: %v", err)
	}
	withRecord := request("flows", "recorded")
	withRecord.Items[0].Record = planStoryURN
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), fixture.repo, withRecord, false); err == nil || !strings.Contains(err.Error(), "record") {
		t.Fatalf("a UX Item naming a record was accepted: %v", err)
	}
}
