package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/twentyideas/changesaga/internal/requirements"
)

func Deprecation(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		return livingFamilyHelp("deprecation", []string{"add", "revise", "set-state"}, out)
	}
	op := args[0]
	if op != "add" && op != "revise" && op != "set-state" {
		return fmt.Errorf("usage: %s", commandUsage["deprecation"])
	}
	err := deprecationWrite(ctx, op, args[1:], out)
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "deprecation "+op, err)
	}
	return err
}

func deprecationWrite(ctx context.Context, op string, args []string, out io.Writer) error {
	name := "deprecation " + op
	flags := commandFlags(name, commandUsage[name], out)
	id, subject, revision, event := new(string), new(string), new(string), new(string)
	*revision, *event = "r1", "active"
	if op == "add" {
		id = flags.String("id", "", "new stable annotation ID")
	} else {
		subject = flags.String("deprecation", "", "canonical annotation URN")
	}
	if op != "set-state" {
		revision = flags.String("revision", "r1", "new immutable revision ID")
	}
	if op != "revise" {
		event = flags.String("event", "active", "new lifecycle event ID")
	}
	reason := flags.String("reason", "", "authored reason; lifecycle changes require their own reason")
	request := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "machine-readable result")
	var parents stringList
	if op != "add" {
		flags.Var(&parents, "parent", "observed revision heads (revise) or event heads (set-state); repeatable")
	}
	var target, element, replacement, replacementElement, scope, repo, state *string
	var refs stringList
	if op == "set-state" {
		state = flags.String("state", "", "active or withdrawn")
	} else {
		target = flags.String("target", "", "canonical content target; pin its current content")
		element = flags.String("element", "", "whiteboard appearance path; applies only to that appearance")
		replacement = flags.String("replacement", "", "optional successor content URN")
		replacementElement = flags.String("replacement-element", "", "optional successor whiteboard appearance")
		scope = flags.String("scope", "", "content or code; code requires explicit --ref")
		repo = flags.String("repo", "", "source repository checkout")
		flags.Var(&refs, "ref", "exact code reference COMMIT:PATH#LSTART-LEND; related evidence for content scope, deprecated code for code scope; repeatable")
	}
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *reason); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	var result requirements.MutationResult
	if op == "set-state" {
		if *subject == "" || *state == "" || !flagWasSet(flags, "event") {
			return fmt.Errorf("set-state requires --deprecation, --state and a new --event")
		}
		result, err = requirements.SetDeprecationState(root, sagaID, requirements.SetDeprecationStateInput{Deprecation: *subject, ID: *event, Parents: parents, State: requirements.DeprecationState(*state), Reason: *reason, RequestID: *request})
	} else {
		if *target == "" || *scope == "" {
			return fmt.Errorf("--target and explicit --scope content|code are required")
		}
		if op == "add" && (len(parents) > 0 || *subject != "") {
			return fmt.Errorf("add takes --id, not --deprecation or --parent")
		}
		if op == "revise" && (!flagWasSet(flags, "revision") || *subject == "" || *id != "") {
			return fmt.Errorf("revise requires --deprecation and a new --revision")
		}
		targets := requirements.DeprecationTargets{Root: root, SagaID: sagaID}
		pin, e := targets.Pin(*target, *element)
		if e != nil {
			return e
		}
		definition := requirements.DeprecationDefinition{Reason: *reason, Scope: *scope, Target: pin}
		if *replacement != "" {
			p, e := targets.Pin(*replacement, *replacementElement)
			if e != nil {
				return e
			}
			definition.Replacement = &p
		} else if *replacementElement != "" {
			return fmt.Errorf("--replacement-element requires --replacement")
		}
		definition.Code, err = termReferences(ctx, firstNonEmpty(*repo, root), refs)
		if err != nil {
			return err
		}
		if op == "add" {
			result, err = requirements.AddDeprecation(root, sagaID, requirements.AddDeprecationInput{ID: *id, RevisionID: *revision, EventID: *event, DeprecationDefinition: definition, RequestID: *request})
		} else {
			result, err = requirements.ReviseDeprecation(root, sagaID, requirements.ReviseDeprecationInput{Deprecation: *subject, ID: *revision, Parents: parents, DeprecationDefinition: definition, RequestID: *request})
		}
	}
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}
