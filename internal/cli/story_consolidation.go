package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func storyConsolidate(_ context.Context, args []string, out io.Writer) error {
	name := "story consolidate"
	flags := commandFlags(name, commandUsage[name], out)
	duplicate := flags.String("duplicate", "", "canonical duplicate story URN")
	canonical := flags.String("canonical", "", "canonical existing story URN that preserves the proposal")
	event := flags.String("event", "", "stable rejected or retired lifecycle event id")
	reason := flags.String("reason", "", "why these proposals are being consolidated")
	apply := flags.Bool("apply", false, "apply the validated preview; omitted is read-only")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable plan")
	feature := featureIDFlag(flags)
	var parents, mappings stringList
	flags.Var(&parents, "parent", "current duplicate lifecycle head URN; repeatable")
	flags.Var(&mappings, "map", "exact DUPLICATE_CRITERION_URN=CANONICAL_CRITERION_URN mapping; repeat for every current duplicate criterion")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *duplicate == "" || *canonical == "" || *event == "" || *reason == "" || len(parents) == 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	criterionMap := map[string]string{}
	for _, mapping := range mappings {
		from, to, ok := strings.Cut(mapping, "=")
		if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" || criterionMap[from] != "" {
			return fmt.Errorf("--map must be a unique DUPLICATE_CRITERION_URN=CANONICAL_CRITERION_URN pair")
		}
		criterionMap[from] = to
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	if err := assertRecordFeature(root, *feature, *duplicate); err != nil {
		return err
	}
	input := requirements.ConsolidateInput{Duplicate: *duplicate, Canonical: *canonical, EventID: *event, Parents: parents, Reason: *reason, CriterionMap: criterionMap}
	input.LoadCurrencyInputs = func() (requirements.StaleInputs, error) {
		heads, err := loadRelationHeads(root)
		if err != nil {
			return requirements.StaleInputs{}, fmt.Errorf("load current relation pins: %w", err)
		}
		return heads.inputs, nil
	}
	var plan requirements.ConsolidationPlan
	if *apply {
		plan, err = requirements.ConsolidateProposal(root, sagaID, input)
	} else {
		plan, err = requirements.PreviewConsolidation(root, sagaID, input)
	}
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, plan)
	}
	mode := "Preview"
	if plan.Applied {
		mode = "Applied"
	}
	fmt.Fprintf(out, "%s consolidation %s -> %s\nLifecycle: %s (%s)\nCriterion mappings: %d\nRelation remaps: %d\n", mode, plan.Duplicate, plan.Canonical, plan.LifecycleState, plan.LifecycleEvent, len(plan.CriterionMap), len(plan.RelationRemaps))
	for _, remap := range plan.RelationRemaps {
		fmt.Fprintf(out, "  %s -> %s: %s -> %s\n", remap.Existing, remap.Replacement, remap.From, remap.To)
	}
	if !plan.Applied {
		fmt.Fprintln(out, "No files changed. Re-run with --apply to revalidate current Saga state and apply the decision if it is still valid.")
	}
	return nil
}

func storyWithdraw(_ context.Context, args []string, out io.Writer) error {
	name := "story withdraw"
	flags := commandFlags(name, commandUsage[name], out)
	storyURN := flags.String("story", "", "canonical proposed or deferred story URN")
	event := flags.String("event", "", "stable rejected lifecycle event id")
	reason := flags.String("reason", "", "why the proposal is withdrawn")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	var parents stringList
	flags.Var(&parents, "parent", "current lifecycle head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || *storyURN == "" || *event == "" || strings.TrimSpace(*reason) == "" || len(parents) == 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	root := flags.Arg(0)
	document, err := requirements.Load(root, "")
	if err != nil {
		return err
	}
	ref, err := livingid.Parse(*storyURN)
	if err != nil || ref.Kind != livingid.KindStory || ref.SagaID != document.SagaID {
		return fmt.Errorf("story must be a canonical story URN in saga %q", document.SagaID)
	}
	story := document.FindStory(ref.ID)
	if story == nil || story.CurrentLifecycle == nil {
		return fmt.Errorf("withdrawal refuses missing or conflicted story lifecycle")
	}
	if story.CurrentLifecycle.State != requirements.StateProposed && story.CurrentLifecycle.State != requirements.StateDeferred {
		replay := *requestID != "" && story.CurrentLifecycle.State == requirements.StateRejected &&
			story.CurrentLifecycle.ID == *event && story.CurrentLifecycle.RequestID == *requestID
		if !replay {
			return fmt.Errorf("only a proposed or deferred story can be withdrawn; %q is %s", ref.ID, story.CurrentLifecycle.State)
		}
	}
	if err := assertRecordFeature(root, *feature, *storyURN); err != nil {
		return err
	}
	result, err := requirements.SetStoryState(root, document.SagaID, requirements.SetStoryStateInput{Story: *storyURN, ID: *event, Parents: parents, State: requirements.StateRejected, Reason: "withdrawn: " + strings.TrimSpace(*reason), RequestID: *requestID})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}
