package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// Epic content is always written to a named epic. Every command that authors
// epic content accepts --epic: a command that creates a new top-level record
// requires it, and a command that changes an existing record accepts it as an
// assertion, since the record's own location already decides its epic.
const epicFlagHelp = "epic id or URN; required when creating epic content, otherwise it must name the epic that holds the record"

func epicFlag(flags *flag.FlagSet) *string {
	return flags.String("epic", "", epicFlagHelp)
}

// requireEpic resolves the epic a creating command writes into.
func requireEpic(root, value string) (applayout.Epic, error) {
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return applayout.Epic{}, err
	}
	return applayout.Require(root, manifest.ID, value)
}

// assertEpic checks an optional --epic against the epic that holds an
// existing record.
func assertEpic(root, value, record, holding string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	epic, err := requireEpic(root, value)
	if err != nil {
		return err
	}
	if epic.ID != holding {
		return fmt.Errorf("%s is in epic %q, not %q; --epic must name the epic that holds it", record, holding, epic.ID)
	}
	return nil
}

// Epic dispatches the epic command family.
func Epic(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("epic", []string{"add"}, out)
	}
	var err error
	switch args[0] {
	case "add":
		err = epicAdd(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["epic"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "epic "+args[0], err)
	}
	return err
}

func epicAdd(_ context.Context, args []string, out io.Writer) error {
	name := "epic add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable epic id")
	title := flags.String("title", "", "epic title: the product domain it covers")
	description := flags.String("description", "", "what the domain covers")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *title); err != nil {
		return err
	}
	root := flags.Arg(0)
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return err
	}
	if *requestID != "" && !applayout.ValidID(*requestID) {
		return fmt.Errorf("--request-id must be a stable identifier")
	}
	urn := applayout.EpicURN(manifest.ID, *id)
	path := applayout.EpicRel(*id)
	var replayed bool
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		epics, err := applayout.Epics(root)
		if err != nil {
			return err
		}
		if existing, ok := applayout.Find(epics, *id); ok {
			if *requestID != "" && existing.RequestID == *requestID && existing.Title == strings.TrimSpace(*title) && existing.Description == strings.TrimSpace(*description) {
				replayed = true
				return nil
			}
			return fmt.Errorf("epic %q already exists", *id)
		}
		_, err = applayout.WriteEpic(root, applayout.EpicManifest{
			ID: *id, Title: strings.TrimSpace(*title), Description: strings.TrimSpace(*description),
			CreatedAt: time.Now().UTC(), RequestID: *requestID,
		})
		return err
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, urn, path, []string{urn}, nil, replayed, *jsonOutput)
}

// Persona dispatches the persona command family.
func Persona(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("persona", []string{"add", "revise", "set-state"}, out)
	}
	var err error
	switch args[0] {
	case "add":
		err = personaAdd(ctx, args[1:], out)
	case "revise":
		err = personaRevise(ctx, args[1:], out)
	case "set-state":
		err = personaSetState(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["persona"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "persona "+args[0], err)
	}
	return err
}

func personaAdd(_ context.Context, args []string, out io.Writer) error {
	name := "persona add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable persona id")
	revision := flags.String("revision", "r1", "initial revision id")
	event := flags.String("event", "active", "initial active-event id")
	personaName := flags.String("name", "", "persona name")
	description := flags.String("description", "", "who the persona is and what they need")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *revision, *event, *personaName, *description); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.AddPersona(root, sagaID, requirements.AddPersonaInput{
		ID: *id, RevisionID: *revision, EventID: *event, Name: *personaName, Description: *description, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}

func personaRevise(_ context.Context, args []string, out io.Writer) error {
	name := "persona revise"
	flags := commandFlags(name, commandUsage[name], out)
	persona := flags.String("persona", "", "canonical persona URN")
	revision := flags.String("revision", "", "new revision id")
	personaName := flags.String("name", "", "complete revised name")
	description := flags.String("description", "", "complete revised description")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *persona, *revision, *personaName, *description); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.RevisePersona(root, sagaID, requirements.RevisePersonaInput{
		Persona: *persona, ID: *revision, Parents: parents, Name: *personaName, Description: *description, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func personaSetState(_ context.Context, args []string, out io.Writer) error {
	name := "persona set-state"
	flags := commandFlags(name, commandUsage[name], out)
	persona := flags.String("persona", "", "canonical persona URN")
	event := flags.String("event", "", "new lifecycle event id")
	state := flags.String("state", "", "active or retired")
	reason := flags.String("reason", "", "why the lifecycle changed")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents stringList
	flags.Var(&parents, "parent", "current lifecycle head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *persona, *event, *state); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.SetPersonaState(root, sagaID, requirements.SetPersonaStateInput{
		Persona: *persona, ID: *event, Parents: parents, State: requirements.PersonaState(*state), Reason: *reason, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}

// FeatureFlag dispatches the flag command family.
func FeatureFlag(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("flag", []string{"add", "revise", "set-state"}, out)
	}
	var err error
	switch args[0] {
	case "add":
		err = flagAdd(ctx, args[1:], out)
	case "revise":
		err = flagRevise(ctx, args[1:], out)
	case "set-state":
		err = flagSetState(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["flag"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "flag "+args[0], err)
	}
	return err
}

func flagAdd(_ context.Context, args []string, out io.Writer) error {
	name := "flag add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable flag id")
	revision := flags.String("revision", "r1", "initial revision id")
	event := flags.String("event", "", "initial lifecycle event id; defaults to the initial state")
	description := flags.String("description", "", "what the flag gates and why")
	state := flags.String("state", "off", "initial state: off or on")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var targets stringList
	flags.Var(&targets, "target", "story or epic URN the flag gates; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if *event == "" {
		*event = *state
	}
	if err := requireLivingArgs(flags, *id, *description); err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.AddFlag(root, sagaID, requirements.AddFlagInput{
		ID: *id, RevisionID: *revision, EventID: *event, Description: *description, Targets: targets,
		State: requirements.FlagState(*state), RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}

func flagRevise(_ context.Context, args []string, out io.Writer) error {
	name := "flag revise"
	flags := commandFlags(name, commandUsage[name], out)
	flagURN := flags.String("flag", "", "canonical flag URN")
	revision := flags.String("revision", "", "new revision id")
	description := flags.String("description", "", "complete revised description")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents, targets stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	flags.Var(&targets, "target", "complete set of story or epic URNs the flag gates; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *flagURN, *revision, *description); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.ReviseFlag(root, sagaID, requirements.ReviseFlagInput{
		Flag: *flagURN, ID: *revision, Parents: parents, Description: *description, Targets: targets, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func flagSetState(_ context.Context, args []string, out io.Writer) error {
	name := "flag set-state"
	flags := commandFlags(name, commandUsage[name], out)
	flagURN := flags.String("flag", "", "canonical flag URN")
	event := flags.String("event", "", "new lifecycle event id")
	state := flags.String("state", "", "off, on, or retired")
	reason := flags.String("reason", "", "why the flag changed")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents stringList
	flags.Var(&parents, "parent", "current lifecycle head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *flagURN, *event, *state); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.SetFlagState(root, sagaID, requirements.SetFlagStateInput{
		Flag: *flagURN, ID: *event, Parents: parents, State: requirements.FlagState(*state), Reason: *reason, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}

func storyMove(_ context.Context, args []string, out io.Writer) error {
	name := "story move"
	flags := commandFlags(name, commandUsage[name], out)
	story := flags.String("story", "", "canonical story URN")
	epic := flags.String("epic", "", "epic id or URN the story moves to")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *story, *epic); err != nil {
		return err
	}
	root := flags.Arg(0)
	target, err := requireEpic(root, *epic)
	if err != nil {
		return err
	}
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.MoveStory(root, sagaID, requirements.MoveStoryInput{Story: *story, Epic: target.ID})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

// recordEpic returns the epic that holds the record named by urn. Nested URNs
// (a criterion, revision, Item, or landmark) resolve through their parent.
func recordEpic(root, urn string) (string, error) {
	parts := strings.Split(urn, ":")
	if len(parts) < 5 || parts[0] != "urn" || parts[1] != "change-saga" {
		return "", fmt.Errorf("%q is not a canonical Saga URN", urn)
	}
	sagaID, kind, id := parts[2], parts[3], parts[4]
	missing := fmt.Errorf("%s does not exist", strings.Join(parts[:5], ":"))
	switch kind {
	case "story", "citation", "relation":
		document, err := requirements.Load(root, sagaID)
		if err != nil {
			return "", err
		}
		switch kind {
		case "story":
			if story := document.FindStory(id); story != nil {
				return story.Epic, nil
			}
		case "citation":
			for _, citation := range document.Citations {
				if citation.ID == id {
					return citation.Epic, nil
				}
			}
		case "relation":
			for _, relation := range document.Relations {
				if relation.ID == id {
					return relation.Epic, nil
				}
			}
		}
		return "", missing
	case "prototype":
		document, err := prototypes.Load(root, sagaID)
		if err != nil {
			return "", err
		}
		for _, prototype := range document.Prototypes {
			if prototype.Identity.ID == id {
				return prototype.Epic, nil
			}
		}
		return "", missing
	case "test-case", "quality-policy":
		document, err := quality.Load(root)
		if err != nil {
			return "", err
		}
		for _, testCase := range document.TestCases {
			if kind == "test-case" && testCase.Identity.ID == id {
				return testCase.Epic, nil
			}
		}
		for _, policy := range document.Policies {
			if kind == "quality-policy" && policy.ID == id {
				return policy.Epic, nil
			}
		}
		return "", missing
	case "wave", "work-item", "dependency", "contract":
		plan, _, err := workplan.Load(root)
		if err != nil {
			return "", err
		}
		switch {
		case kind == "wave" && plan.Waves[id] != nil:
			return plan.Waves[id].Epic, nil
		case kind == "work-item" && plan.WorkItems[id] != nil:
			return plan.WorkItems[id].Epic, nil
		case kind == "dependency" && plan.Dependencies[id] != nil:
			return plan.Dependencies[id].Epic, nil
		case kind == "contract" && plan.Contracts[id] != nil:
			return plan.Contracts[id].Epic, nil
		}
		return "", missing
	}
	document, _, err := saga.LoadOutline(root)
	if err != nil {
		return "", err
	}
	target := strings.Join(parts[:5], ":")
	found := ""
	var walk func(*saga.Section)
	walk = func(section *saga.Section) {
		if section.Target == target {
			found = section.Path
		}
		for _, fragment := range section.Fragments {
			if fragment.Target == target {
				found = fragment.Path
			}
		}
		for _, child := range section.Children {
			walk(child)
		}
	}
	walk(document.Section)
	if found == "" {
		return "", missing
	}
	if epic := saga.EpicOf(found); epic != "" {
		return epic, nil
	}
	return "", fmt.Errorf("%s belongs to the app, not an epic", target)
}

// assertRecordEpic checks an optional --epic against the epic holding urn.
func assertRecordEpic(root, value, urn string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	holding, err := recordEpic(root, urn)
	if err != nil {
		return err
	}
	return assertEpic(root, value, urn, holding)
}
