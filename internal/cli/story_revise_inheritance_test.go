package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/store"
)

// currentStoryRevisionForTest returns the story's unique current revision.
func currentStoryRevisionForTest(t *testing.T, root string) *requirements.Revision {
	t.Helper()
	document, err := requirements.Load(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Stories) != 1 {
		t.Fatalf("fixture saga holds %d stories", len(document.Stories))
	}
	current := document.Stories[0].CurrentRevision
	if current == nil {
		t.Fatalf("story has no unique current revision; heads %v", document.Stories[0].RevisionHeads)
	}
	return current
}

// sourcedStory adds a citation and a story that cites it, carries two
// criteria, a persona, and a priority, so a revise has something to lose.
func sourcedStory(t *testing.T, root string) string {
	t.Helper()
	ctx := context.Background()
	var output bytes.Buffer
	if err := Citation(ctx, []string{"add", root, "--feature", testFeature, "--id", "interview", "--kind", "url",
		"--title", "Buyer interview", "--reference", "https://example.test/interview"}, &output); err != nil {
		t.Fatalf("citation add: %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Story(ctx, []string{"add", root, "--feature", testFeature, "--persona", testPersonaURN,
		"--id", "checkout", "--revision", "r1", "--event", "proposed",
		"--title", "Checkout", "--statement", "As a buyer, I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--criterion", "audited=Every purchase is audited",
		"--citation", "urn:change-saga:atomic:citation:interview"}, &output); err != nil {
		t.Fatalf("story add: %v\n%s", err, output.String())
	}
	return "urn:change-saga:atomic:story:checkout"
}

// A revision is a complete snapshot, so a flag-built revise that restates only
// what changed used to delete every criterion and citation it left out.
func TestStoryReviseInheritsWhatASingleParentAlreadySays(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	storyURN := sourcedStory(t, root)
	before := currentStoryRevisionForTest(t, root)
	if len(before.AcceptanceCriteria) != 2 || len(before.Citations) != 1 {
		t.Fatalf("fixture revision = %#v", before)
	}

	var output bytes.Buffer
	if err := Story(context.Background(), []string{"revise", root, "--story", storyURN, "--revision", "r2",
		"--parent", storyURN + ":revision:r1", "--title", "Checkout, clarified"}, &output); err != nil {
		t.Fatalf("title-only revise: %v\n%s", err, output.String())
	}
	after := currentStoryRevisionForTest(t, root)
	if after.ID != "r2" || after.Title != "Checkout, clarified" {
		t.Fatalf("revised title = %#v", after)
	}
	if len(after.AcceptanceCriteria) != len(before.AcceptanceCriteria) {
		t.Fatalf("criteria count went from %d to %d", len(before.AcceptanceCriteria), len(after.AcceptanceCriteria))
	}
	for index, criterion := range before.AcceptanceCriteria {
		if after.AcceptanceCriteria[index] != criterion {
			t.Fatalf("criterion %d = %#v, want %#v", index, after.AcceptanceCriteria[index], criterion)
		}
	}
	if strings.Join(after.Citations, ",") != strings.Join(before.Citations, ",") {
		t.Fatalf("citations = %v, want %v", after.Citations, before.Citations)
	}
	if strings.Join(after.Personas, ",") != strings.Join(before.Personas, ",") {
		t.Fatalf("personas = %v, want %v", after.Personas, before.Personas)
	}
	if after.Statement != before.Statement || after.Priority != before.Priority {
		t.Fatalf("statement %q priority %q, want %q and %q", after.Statement, after.Priority, before.Statement, before.Priority)
	}
}

// Naming a field still replaces it, and a statement-only revise keeps the
// title it did not restate.
func TestStoryReviseReplacesOnlyTheFieldsItIsGiven(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	storyURN := sourcedStory(t, root)
	ctx := context.Background()
	var output bytes.Buffer
	if err := Story(ctx, []string{"revise", root, "--story", storyURN, "--revision", "r2",
		"--parent", storyURN + ":revision:r1", "--statement", "As a buyer, I can check out in one step"}, &output); err != nil {
		t.Fatalf("statement-only revise: %v\n%s", err, output.String())
	}
	statementOnly := currentStoryRevisionForTest(t, root)
	if statementOnly.Title != "Checkout" || statementOnly.Statement != "As a buyer, I can check out in one step" || len(statementOnly.AcceptanceCriteria) != 2 {
		t.Fatalf("statement-only revision = %#v", statementOnly)
	}

	output.Reset()
	if err := Story(ctx, []string{"revise", root, "--story", storyURN, "--revision", "r3",
		"--parent", storyURN + ":revision:r2", "--criterion", "fast=Checkout finishes in under a second",
		"--persona", testPersonaURN, "--citation", ""}, &output); err == nil {
		t.Fatalf("empty citation accepted:\n%s", output.String())
	}
	output.Reset()
	if err := Story(ctx, []string{"revise", root, "--story", storyURN, "--revision", "r3",
		"--parent", storyURN + ":revision:r2", "--criterion", "fast=Checkout finishes in under a second",
		"--priority", ""}, &output); err != nil {
		t.Fatalf("criterion revise: %v\n%s", err, output.String())
	}
	replaced := currentStoryRevisionForTest(t, root)
	if len(replaced.AcceptanceCriteria) != 1 || replaced.AcceptanceCriteria[0].ID != "fast" {
		t.Fatalf("named criteria replace the parent's: %#v", replaced.AcceptanceCriteria)
	}
	if replaced.Priority != "" {
		t.Fatalf("an empty --priority clears it, got %q", replaced.Priority)
	}
	if len(replaced.Citations) != 1 {
		t.Fatalf("citations = %v", replaced.Citations)
	}
}

// Reconciling competing heads is a real conflict, so it still takes the
// complete definition; nothing is inherited from a parent the author must
// choose between.
func TestStoryReviseWithCompetingHeadsStillNeedsTheWholeDefinition(t *testing.T) {
	t.Parallel()
	root := newLivingSaga(t)
	storyURN := sourcedStory(t, root)
	ctx := context.Background()
	var output bytes.Buffer
	if err := Story(ctx, []string{"revise", root, "--story", storyURN, "--revision", "r2",
		"--parent", storyURN + ":revision:r1", "--title", "Checkout, clarified"}, &output); err != nil {
		t.Fatalf("first revise: %v\n%s", err, output.String())
	}
	// A concurrent Git merge leaves two revisions parented on r1.
	competing := requirements.Revision{
		Schema: requirements.RevisionSchemaURL, Version: requirements.Version, ID: "r2-other", Story: storyURN,
		Parents: []string{storyURN + ":revision:r1"}, Title: "Checkout elsewhere", Statement: "As a buyer, I can check out",
		Personas: []string{testPersonaURN}, Citations: []string{},
		AcceptanceCriteria: []requirements.Criterion{{ID: "fast", Statement: "Checkout finishes promptly"}},
		CreatedAt:          time.Now().UTC(),
	}
	path := filepath.Join(testFeatureDir(root), "___requirements", "stories", "checkout.story", "revisions", "r2-other.json")
	if err := store.WriteJSON(path, competing, true); err != nil {
		t.Fatal(err)
	}

	parents := []string{"--parent", storyURN + ":revision:r2", "--parent", storyURN + ":revision:r2-other"}
	output.Reset()
	args := append([]string{"revise", root, "--story", storyURN, "--revision", "r3"}, parents...)
	if err := Story(ctx, append(args, "--title", "Reconciled checkout"), &output); err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("multi-parent revise without a statement = %v\n%s", err, output.String())
	}
	output.Reset()
	if err := Story(ctx, append(args, "--title", "Reconciled checkout", "--statement", "As a buyer, I can check out",
		"--persona", testPersonaURN, "--criterion", "fast=Checkout finishes promptly"), &output); err != nil {
		t.Fatalf("multi-parent reconciliation: %v\n%s", err, output.String())
	}
	reconciled := currentStoryRevisionForTest(t, root)
	if reconciled.ID != "r3" || len(reconciled.AcceptanceCriteria) != 1 || len(reconciled.Citations) != 0 {
		t.Fatalf("reconciled revision = %#v", reconciled)
	}
}
