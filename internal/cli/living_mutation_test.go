package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/workplan"
)

func decodeLivingOutput(t *testing.T, output *bytes.Buffer) livingMutationOutput {
	t.Helper()
	var result livingMutationOutput
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	if err := decoder.Decode(&result); err != nil {
		t.Fatalf("decode JSON output %q: %v", output.String(), err)
	}
	if decoder.Decode(&struct{}{}) == nil {
		t.Fatalf("mutation emitted more than one JSON value: %q", output.String())
	}
	return result
}

func newLivingSaga(t *testing.T) string {
	t.Helper()
	return newAuthoredSaga(t)
}

func TestLivingMutationFamilyHelpIsDeterministicAndHasNoPivot(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, []string, io.Writer) error
		want string
	}{
		{name: "story", run: Story, want: "add\n  revise\n  set-state\n  move"},
		{name: "epic", run: Epic, want: "  add"},
		{name: "persona", run: Persona, want: "add\n  revise\n  set-state"},
		{name: "flag", run: FeatureFlag, want: "add\n  revise\n  set-state"},
		{name: "criterion", run: Criterion, want: "add\n  revise\n  remove"},
		{name: "citation", run: Citation, want: "  add"},
		{name: "relation", run: Relation, want: "add\n  supersede"},
		{name: "plan", run: Plan, want: "record-merge"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var first, second bytes.Buffer
			if err := test.run(context.Background(), []string{"-h"}, &first); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("first help error = %v", err)
			}
			if err := test.run(context.Background(), []string{"--help"}, &second); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("second help error = %v", err)
			}
			if first.String() != second.String() {
				t.Fatalf("help is not deterministic:\n%s\n---\n%s", first.String(), second.String())
			}
			if !strings.Contains(first.String(), test.want) {
				t.Fatalf("help omitted %q:\n%s", test.want, first.String())
			}
			if strings.Contains(first.String(), "pivot") {
				t.Fatalf("help exposed a pivot resource:\n%s", first.String())
			}
		})
	}

	subcommands := []struct {
		name      string
		operation string
		run       func(context.Context, []string, io.Writer) error
	}{
		{"story add", "add", Story}, {"story revise", "revise", Story}, {"story set-state", "set-state", Story},
		{"criterion add", "add", Criterion}, {"criterion revise", "revise", Criterion}, {"criterion remove", "remove", Criterion},
		{"story move", "move", Story},
		{"citation add", "add", Citation}, {"relation add", "add", Relation}, {"relation supersede", "supersede", Relation},
		{"epic add", "add", Epic},
		{"persona add", "add", Persona}, {"persona revise", "revise", Persona}, {"persona set-state", "set-state", Persona},
		{"flag add", "add", FeatureFlag}, {"flag revise", "revise", FeatureFlag}, {"flag set-state", "set-state", FeatureFlag},
	}
	for _, operation := range planOperations {
		subcommands = append(subcommands, struct {
			name      string
			operation string
			run       func(context.Context, []string, io.Writer) error
		}{"plan " + operation, operation, Plan})
	}
	for _, test := range subcommands {
		t.Run(test.name+" help", func(t *testing.T) {
			var first, second bytes.Buffer
			if err := test.run(context.Background(), []string{test.operation, "-h"}, &first); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("first help error = %v\n%s", err, first.String())
			}
			if err := test.run(context.Background(), []string{test.operation, "--help"}, &second); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("second help error = %v\n%s", err, second.String())
			}
			if first.String() != second.String() || !strings.Contains(first.String(), commandUsage[test.name]) {
				t.Fatalf("subcommand help drifted:\n%s\n---\n%s", first.String(), second.String())
			}
		})
	}
}

func TestCriterionCommandsSupportStructuredInputEditorAndCompleteResults(t *testing.T) {
	root := newLivingSaga(t)
	ctx := context.Background()
	var output bytes.Buffer
	if err := Story(ctx, []string{
		"add", root, "--epic", testEpic, "--persona", testPersonaURN, "--id", "checkout", "--revision", "r1", "--event", "proposed",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly", "--request-id", "story-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	storyURN := "urn:change-saga:atomic:story:checkout"
	r1 := storyURN + ":revision:r1"
	payload := `{"story":"` + storyURN + `","parent":"` + r1 + `","revision":"r2","id":"audited","statement":"The purchase is audited","request_id":"criterion-add"}`
	output.Reset()
	if err := criterion(ctx, []string{"add", root, "--from", "-", "--json"}, &output, strings.NewReader(payload)); err != nil {
		t.Fatalf("structured criterion add: %v\n%s", err, output.String())
	}
	added := decodeLivingOutput(t, &output)
	r2 := storyURN + ":revision:r2"
	if !added.OK || added.Resource != storyURN+":criterion:audited" || !reflect.DeepEqual(added.Created, []string{r2}) || !reflect.DeepEqual(added.CurrentHeads, []string{r2}) || len(added.Paths) != 1 {
		t.Fatalf("criterion add result = %#v", added)
	}

	t.Setenv("EDITOR", "true")
	output.Reset()
	if err := Criterion(ctx, []string{
		"revise", root, "--story", storyURN, "--criterion", added.Resource, "--parent", r2,
		"--revision", "r3", "--statement", "Every purchase is audited", "--request-id", "criterion-revise", "--edit", "--json",
	}, &output); err != nil {
		t.Fatalf("criterion editor revise: %v\n%s", err, output.String())
	}
	revised := decodeLivingOutput(t, &output)
	r3 := storyURN + ":revision:r3"
	if !reflect.DeepEqual(revised.CurrentHeads, []string{r3}) || revised.Replayed {
		t.Fatalf("criterion revise result = %#v", revised)
	}

	output.Reset()
	if err := Criterion(ctx, []string{
		"remove", root, "--story", storyURN, "--criterion", added.Resource, "--parent", r3,
		"--revision", "r4", "--reason", "Replaced by a materially different obligation", "--request-id", "criterion-remove", "--json",
	}, &output); err != nil {
		t.Fatalf("criterion remove: %v\n%s", err, output.String())
	}
	removed := decodeLivingOutput(t, &output)
	if removed.Reason == "" || removed.Resource != added.Resource {
		t.Fatalf("criterion remove result = %#v", removed)
	}
	document, err := requirements.Load(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	current := document.Stories[0].CurrentRevision
	if current == nil || current.ID != "r4" || len(current.AcceptanceCriteria) != 1 || current.AcceptanceCriteria[0].ID != "fast" {
		t.Fatalf("current full revision = %#v", current)
	}
}

func TestStoryStructuredInputAndEditorUseExistingRevisionWriter(t *testing.T) {
	root := newLivingSaga(t)
	payload := `{"epic":"core","personas":["` + testPersonaURN + `"],"id":"checkout","revision":"r1","event":"proposed","title":"Checkout","statement":"As a buyer I can check out","priority":"must","acceptance_criteria":[{"id":"fast","statement":"Checkout finishes promptly"}],"created_at":"2026-09-17T18:00:00Z","request_id":"story-from"}`
	var output bytes.Buffer
	if err := story(context.Background(), []string{"add", root, "--from", "-", "--json"}, &output, strings.NewReader(payload)); err != nil {
		t.Fatalf("structured story add: %v\n%s", err, output.String())
	}
	added := decodeLivingOutput(t, &output)
	if len(added.Created) != 3 || added.Created[0] != added.Resource || len(added.Paths) != 3 || len(added.CurrentHeads) != 2 {
		t.Fatalf("complete story add result = %#v", added)
	}
	t.Setenv("EDITOR", "true")
	output.Reset()
	if err := Story(context.Background(), []string{
		"revise", root, "--story", added.Resource, "--revision", "r2", "--parent", added.CurrentHeads[0],
		"--title", "Checkout clarified", "--edit", "--request-id", "story-edit", "--json",
	}, &output); err != nil {
		t.Fatalf("story editor revise: %v\n%s", err, output.String())
	}
	revised := decodeLivingOutput(t, &output)
	if revised.Resource != added.Resource+":revision:r2" || !reflect.DeepEqual(revised.CurrentHeads, []string{revised.Resource}) {
		t.Fatalf("story editor result = %#v", revised)
	}
}

func TestCriterionStructuredInputIsStrictAndFailureWritesNothing(t *testing.T) {
	root := newLivingSaga(t)
	var output bytes.Buffer
	if err := Story(context.Background(), []string{
		"add", root, "--epic", testEpic, "--persona", testPersonaURN, "--id", "checkout", "--revision", "r1", "--event", "proposed",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "must",
		"--criterion", "fast=Checkout finishes promptly",
	}, &output); err != nil {
		t.Fatal(err)
	}
	payload := `{"story":"urn:change-saga:atomic:story:checkout","parent":"urn:change-saga:atomic:story:checkout:revision:r1","revision":"r2","id":"audited","statement":"Audited","unexpected":true}`
	output.Reset()
	err := criterion(context.Background(), []string{"add", root, "--from", "-", "--json"}, &output, strings.NewReader(payload))
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("strict structured input error = %v\n%s", err, output.String())
	}
	result := decodeLivingOutput(t, &output)
	if result.OK || result.Error == nil || !strings.Contains(result.Error.Message, "unknown field") {
		t.Fatalf("structured failure = %#v", result)
	}
	path := filepath.Join(testEpicDir(root), "___requirements", "stories", "checkout.story", "revisions", "r2.json")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed structured mutation wrote %s: %v", path, statErr)
	}
}

// The installed skill teaches one Saga for a big change, driven by status next
// actions. It must never offer a lighter mode for small changes or treat
// requirements and design as optional.
func TestInstalledSkillDescribesOneSagaDrivenByStatus(t *testing.T) {
	var output bytes.Buffer
	if err := InstallSkill(nil, &output); err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(output.String()), " ")
	for _, want := range []string{
		"A repository has one app Saga",
		"Epics are durable product domains",
		"The chain is persona → story → design → code",
		"The Saga is documentation",
		"This is not a waterfall",
		"Parallel authoring is a core property",
		"The one thing asked of a change is that its implementation covers it",
		"Everything else is growth, not debt",
		"never invent a story",
		"What exists must stay healthy",
		"Drive the work with status",
		"next_actions",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("installed skill omitted %q:\n%s", want, text)
		}
	}
	for _, retired := range []string{
		"Choose the workflow", "choose a workflow", "small focused change", "small change may not need",
		"remain optional historical", "requirements are optional", "Requirements are optional",
	} {
		if strings.Contains(text, retired) {
			t.Fatalf("installed skill still offers a retired workflow: %q", retired)
		}
	}
}

func TestLivingMutationSubcommandHelpExplainsSemantics(t *testing.T) {
	checks := []struct {
		family    func(context.Context, []string, io.Writer) error
		args      []string
		fragments []string
	}{
		{Citation, []string{"add", "-h"}, []string{"requirement or decision came from", "not delivery evidence"}},
		{Relation, []string{"add", "-h"}, []string{"typed rationale", "content digest", "go stale"}},
		{Relation, []string{"supersede", "-h"}, []string{"without erasing", "pivot"}},
		{Plan, []string{"add-contract", "-h"}, []string{"parallel provider and consumer", "stable seam"}},
		{Plan, []string{"progress", "-h"}, []string{"helps coordination", "never proves"}},
	}
	for _, check := range checks {
		var output bytes.Buffer
		if err := check.family(context.Background(), check.args, &output); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("help error = %v\n%s", err, output.String())
		}
		for _, fragment := range check.fragments {
			if !strings.Contains(output.String(), fragment) {
				t.Fatalf("help omitted %q:\n%s", fragment, output.String())
			}
		}
	}
}

func TestRequirementsMutationCommandsReturnJSONAndReplay(t *testing.T) {
	root := newLivingSaga(t)
	ctx := context.Background()

	citationArgs := []string{
		"add", root, "--epic", testEpic, "--id", "source", "--kind", "url", "--title", "Source",
		"--reference", "https://example.test/source", "--request-id", "citation-request", "--json",
	}
	var output bytes.Buffer
	if err := Citation(ctx, citationArgs, &output); err != nil {
		t.Fatalf("citation add: %v\n%s", err, output.String())
	}
	citation := decodeLivingOutput(t, &output)
	if !citation.OK || citation.Replayed || citation.Resource != "urn:change-saga:atomic:citation:source" {
		t.Fatalf("citation result = %#v", citation)
	}
	output.Reset()
	if err := Citation(ctx, citationArgs, &output); err != nil {
		t.Fatal(err)
	}
	if replay := decodeLivingOutput(t, &output); !replay.OK || !replay.Replayed || replay.Resource != citation.Resource {
		t.Fatalf("citation replay = %#v", replay)
	}

	storyArgs := []string{
		"add", "--epic", testEpic, "--persona", testPersonaURN, "--id", "checkout", "--revision", "checkout-r1", "--event", "checkout-proposed",
		"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "high",
		"--citation", citation.Resource, "--criterion", "fast=Completes promptly", "--request-id", "story-request",
		"--json", root,
	}
	output.Reset()
	if err := Story(ctx, storyArgs, &output); err != nil {
		t.Fatal(err)
	}
	story := decodeLivingOutput(t, &output)
	if !story.OK || story.Resource != "urn:change-saga:atomic:story:checkout" {
		t.Fatalf("story result = %#v", story)
	}

	parent, err := requirements.StoryEventURN("atomic", "checkout", "checkout-proposed")
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Story(ctx, []string{
		"set-state", root, "--story", story.Resource, "--event", "checkout-accepted", "--parent", parent,
		"--state", "accepted", "--reason", "Ready", "--request-id", "accept-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if state := decodeLivingOutput(t, &output); !state.OK || len(state.EventIDs) != 1 || state.EventIDs[0] != "checkout-accepted" {
		t.Fatalf("state result = %#v", state)
	}

	criterion, err := livingid.Criterion("atomic", "checkout", "fast")
	if err != nil {
		t.Fatal(err)
	}
	relationArgs := []string{
		"add", root, "--epic", testEpic, "--id", "checkout-refines-fast", "--type", "refines", "--from", story.Resource,
		"--to", criterion, "--rationale", "The criterion sharpens the story", "--request-id", "relation-request", "--json",
	}
	output.Reset()
	if err := Relation(ctx, relationArgs, &output); err != nil {
		t.Fatal(err)
	}
	relation := decodeLivingOutput(t, &output)
	if !relation.OK || relation.Resource == "" {
		t.Fatalf("relation result = %#v", relation)
	}
	output.Reset()
	if err := Relation(ctx, []string{"supersede", root, "--relation", relation.Resource, "--request-id", "supersede-request", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	if superseded := decodeLivingOutput(t, &output); !superseded.OK || superseded.Replayed {
		t.Fatalf("supersede result = %#v", superseded)
	}
	output.Reset()
	if err := Relation(ctx, []string{"supersede", root, "--relation", relation.Resource, "--request-id", "supersede-request", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	if replay := decodeLivingOutput(t, &output); !replay.OK || !replay.Replayed {
		t.Fatalf("supersede replay = %#v", replay)
	}
}

func TestMutationFamiliesAdoptOnlyTheirOwnedOptionalRoot(t *testing.T) {
	root := newLivingSaga(t)
	epicDir := testEpicDir(root)
	for _, name := range []string{"___requirements", "___design", "___workplan"} {
		if err := os.RemoveAll(filepath.Join(epicDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	assertAbsent := func(name string) {
		t.Helper()
		if _, err := os.Stat(filepath.Join(epicDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s should remain absent, stat error = %v", name, err)
		}
	}

	var output bytes.Buffer
	if err := Citation(context.Background(), []string{
		"add", root, "--epic", testEpic, "--id", "decision", "--kind", "decision", "--title", "Decision",
		"--reference", "decision-1", "--request-id", "optional-requirements", "--json",
	}, &output); err != nil {
		t.Fatalf("adopt requirements root: %v\n%s", err, output.String())
	}
	if info, err := os.Stat(filepath.Join(epicDir, "___requirements")); err != nil || !info.IsDir() {
		t.Fatalf("requirements root was not adopted: %v", err)
	}
	assertAbsent("___design")
	assertAbsent("___workplan")

	output.Reset()
	if err := Plan(context.Background(), []string{
		"add-wave", root, "--epic", testEpic, "--id", "delivery", "--revision", "delivery-r1", "--title", "Delivery",
		"--objective", "Coordinate delivery", "--request-id", "optional-workplan", "--json",
	}, &output); err != nil {
		t.Fatalf("adopt work-plan root: %v\n%s", err, output.String())
	}
	if info, err := os.Stat(filepath.Join(epicDir, "___workplan")); err != nil || !info.IsDir() {
		t.Fatalf("work-plan root was not adopted: %v", err)
	}
	assertAbsent("___design")
}

func TestPlanCommandsReturnJSONAndDelegateValidationAndReplay(t *testing.T) {
	root := newLivingSaga(t)
	ctx := context.Background()
	var output bytes.Buffer

	waveArgs := []string{
		"add-wave", root, "--epic", testEpic, "--id", "delivery", "--revision", "delivery-r1", "--title", "Delivery",
		"--objective", "Ship independently", "--request-id", "wave-request", "--json",
	}
	if err := Plan(ctx, waveArgs, &output); err != nil {
		t.Fatalf("wave add: %v\n%s", err, output.String())
	}
	wave := decodeLivingOutput(t, &output)
	if !wave.OK || wave.Replayed || wave.Resource != "urn:change-saga:atomic:wave:delivery" {
		t.Fatalf("wave result = %#v", wave)
	}
	output.Reset()
	if err := Plan(ctx, waveArgs, &output); err != nil {
		t.Fatal(err)
	}
	if replay := decodeLivingOutput(t, &output); !replay.OK || !replay.Replayed {
		t.Fatalf("wave replay = %#v", replay)
	}
	waveParent, err := livingid.DefinitionRevision("atomic", livingid.KindWave, "delivery", "delivery-r1")
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Plan(ctx, []string{
		"revise-wave", root, "--wave", wave.Resource, "--revision", "delivery-r2", "--parent", waveParent,
		"--title", "Delivery", "--objective", "Ship mergeable work", "--request-id", "wave-revise-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if revised := decodeLivingOutput(t, &output); !revised.OK || !strings.HasSuffix(revised.Resource, ":revision:delivery-r2") {
		t.Fatalf("wave revision = %#v", revised)
	}

	mergeUnit := `{"id":"primary","repository":"https://example.test/acme/app.git","source_branch":"feature/item","target_branch":"main","required":true}`
	itemArgs := []string{
		"add-item", root, "--epic", testEpic, "--id", "cli", "--revision", "cli-r1", "--title", "CLI", "--objective", "Expose writers",
		"--deliverable", "Mutation commands", "--wave", wave.Resource, "--merge-unit", mergeUnit,
		"--request-id", "item-request", "--json",
	}
	output.Reset()
	if err := Plan(ctx, itemArgs, &output); err != nil {
		t.Fatal(err)
	}
	item := decodeLivingOutput(t, &output)
	if !item.OK || item.Resource != "urn:change-saga:atomic:work-item:cli" || len(item.EventIDs) != 1 {
		t.Fatalf("item result = %#v", item)
	}
	output.Reset()
	if err := Plan(ctx, []string{
		"add-item", root, "--epic", testEpic, "--id", "docs", "--revision", "docs-r1", "--title", "Docs", "--objective", "Document the CLI",
		"--deliverable", "CLI examples", "--wave", wave.Resource, "--request-id", "docs-item-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	docsItem := decodeLivingOutput(t, &output)
	itemParent, err := livingid.DefinitionRevision("atomic", livingid.KindWorkItem, "cli", "cli-r1")
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Plan(ctx, []string{
		"revise-item", root, "--item", item.Resource, "--revision", "cli-r2", "--parent", itemParent,
		"--title", "CLI", "--objective", "Expose supported writers", "--deliverable", "Mutation commands",
		"--wave", wave.Resource, "--merge-unit", mergeUnit, "--request-id", "item-revise-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if revised := decodeLivingOutput(t, &output); !revised.OK || !strings.HasSuffix(revised.Resource, ":revision:cli-r2") {
		t.Fatalf("item revision = %#v", revised)
	}

	output.Reset()
	if err := Plan(ctx, []string{
		"add-dependency", root, "--epic", testEpic, "--id", "cli-before-docs", "--prerequisite", item.Resource, "--dependent", docsItem.Resource,
		"--condition", "progress_done", "--reason", "Examples follow the command contract", "--request-id", "dependency-valid-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if dependency := decodeLivingOutput(t, &output); !dependency.OK {
		t.Fatalf("dependency result = %#v", dependency)
	}

	output.Reset()
	if err := Plan(ctx, []string{
		"add-contract", root, "--epic", testEpic, "--id", "cli-docs", "--revision", "cli-docs-r1", "--kind", "handoff",
		"--provider", item.Resource, "--consumer", docsItem.Resource, "--statement", "Publish stable help",
		"--acceptance", "Every command has deterministic help", "--request-id", "contract-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if contract := decodeLivingOutput(t, &output); !contract.OK || len(contract.EventIDs) != 1 {
		t.Fatalf("contract result = %#v", contract)
	}

	output.Reset()
	if err := Plan(ctx, []string{
		"assign", root, "--item", item.Resource, "--workspace", "dacab14f-c2d6-4dde-99e4-4132090cd897",
		"--repository-id", "repo-1", "--branch", "feature/cli", "--request-id", "assignment-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if assignment := decodeLivingOutput(t, &output); !assignment.OK || len(assignment.EventIDs) != 1 {
		t.Fatalf("assignment result = %#v", assignment)
	}

	progressParent, err := workplan.ProgressEventURN("atomic", "cli", item.EventIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	progressArgs := []string{
		"progress", root, "--item", item.Resource, "--from", progressParent, "--to", "ready",
		"--request-id", "progress-request", "--json",
	}
	output.Reset()
	if err := Plan(ctx, progressArgs, &output); err != nil {
		t.Fatal(err)
	}
	progress := decodeLivingOutput(t, &output)
	if !progress.OK || len(progress.EventIDs) != 1 {
		t.Fatalf("progress result = %#v", progress)
	}
	output.Reset()
	if err := Plan(ctx, progressArgs, &output); err != nil {
		t.Fatal(err)
	}
	if replay := decodeLivingOutput(t, &output); !replay.OK || !replay.Replayed || replay.EventIDs[0] != progress.EventIDs[0] {
		t.Fatalf("progress replay = %#v", replay)
	}

	output.Reset()
	if err := Plan(ctx, []string{
		"record-merge", root, "--item", item.Resource, "--unit", "primary", "--state", "planned",
		"--request-id", "merge-planned-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	planned := decodeLivingOutput(t, &output)
	plannedURN, err := workplan.MergeEventURN("atomic", "cli", planned.EventIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	headOID := strings.Repeat("1", 40)
	output.Reset()
	if err := Plan(ctx, []string{
		"record-merge", root, "--item", item.Resource, "--unit", "primary", "--state", "ready", "--from", plannedURN,
		"--head-oid", headOID, "--request-id", "merge-ready-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	ready := decodeLivingOutput(t, &output)
	readyURN, err := workplan.MergeEventURN("atomic", "cli", ready.EventIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Plan(ctx, []string{
		"record-merge", root, "--item", item.Resource, "--unit", "primary", "--state", "integrated", "--from", readyURN,
		"--head-oid", headOID, "--commit", strings.Repeat("2", 40), "--request-id", "merge-integrated-request", "--json",
	}, &output); err != nil {
		t.Fatal(err)
	}
	if integrated := decodeLivingOutput(t, &output); !integrated.OK || len(integrated.EventIDs) != 1 {
		t.Fatalf("merge result = %#v", integrated)
	}

	output.Reset()
	err = Plan(ctx, []string{
		"add-dependency", root, "--epic", testEpic, "--id", "self", "--prerequisite", item.Resource, "--dependent", item.Resource,
		"--condition", "progress_done", "--reason", "invalid self edge", "--request-id", "dependency-request", "--json",
	}, &output)
	var status *StatusError
	if !errors.As(err, &status) || status.Code != 1 {
		t.Fatalf("JSON validation error = %T %v", err, err)
	}
	failure := decodeLivingOutput(t, &output)
	if failure.OK || failure.Error == nil || !strings.Contains(failure.Error.Message, "self-edge") {
		t.Fatalf("dependency failure = %#v", failure)
	}
}
