package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/gitexec"
	"github.com/twentyideas/changesaga/internal/prototypes"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// Feature content is always written to a feature. Every command that authors feature
// content accepts --feature: a command that creates a new top-level record
// writes into it, and a command that changes an existing record accepts it as
// an assertion, since the record's own location already decides its feature.
// A creating command without --feature uses the app's only feature, or creates the
// first one from the branch name, and says which it chose.
const featureIDFlagHelp = "feature id or URN; when creating feature content it defaults to the app's only feature (or a first feature named after the branch), otherwise it must name the feature that holds the record"

func featureIDFlag(flags *flag.FlagSet) *string {
	return flags.String("feature", "", featureIDFlagHelp)
}

// featureNotices receives what an omitted --feature resolved to. It is stderr, so
// a --json result on stdout stays one JSON value.
var featureNotices io.Writer = os.Stderr

// requireFeature resolves the feature a creating command writes into.
func requireFeature(root, value string) (applayout.Feature, error) {
	manifest, err := saga.ReadManifest(root)
	if err != nil {
		return applayout.Feature{}, err
	}
	if strings.TrimSpace(value) != "" {
		return applayout.Require(root, manifest.ID, value)
	}
	return defaultFeature(root, manifest)
}

// defaultFeature is the feature a creating command uses when --feature is omitted:
// the app's only feature, or, when the app has none, a first feature named after
// the branch (the closest thing to the pull request's title that is always
// at hand). With several features the author must choose. Nothing is locked in:
// no story, deck, or slide URN names its feature, so stories can move later.
func defaultFeature(root string, manifest saga.Manifest) (applayout.Feature, error) {
	features, err := applayout.Features(root)
	if err != nil {
		return applayout.Feature{}, err
	}
	switch len(features) {
	case 1:
		fmt.Fprintf(featureNotices, "Using feature %q, the app's only feature (pass --feature to choose another)\n", features[0].ID)
		return features[0], nil
	case 0:
		id, title, source := firstFeatureName(root, manifest)
		feature, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: id, Title: title, CreatedAt: time.Now().UTC()})
		if err != nil {
			// A concurrent command may have created the same first feature.
			if again, findErr := applayout.Features(root); findErr == nil {
				if found, ok := applayout.Find(again, id); ok {
					return found, nil
				}
			}
			return applayout.Feature{}, err
		}
		fmt.Fprintf(featureNotices, "Created feature %q (%q), named after %s, for this content; stories can move to other features later without breaking a link\n", id, title, source)
		return feature, nil
	}
	return applayout.Require(root, manifest.ID, "")
}

// firstFeatureName derives the first feature's id and title from the current
// branch, ignoring prefixes such as feature/; on a default branch, or outside
// Git, it falls back to the app's own name.
func firstFeatureName(root string, manifest saga.Manifest) (id, title, source string) {
	output, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "-q", "HEAD").Output()
	branch := strings.TrimSpace(string(output))
	if err == nil && branch != "" {
		switch branch {
		case "HEAD", "main", "master", "trunk", "develop", "development":
		default:
			name := branch[strings.LastIndex(branch, "/")+1:]
			if id := store.Slug(name); applayout.ValidID(id) {
				return id, humanTitle(id), "branch " + branch
			}
		}
	}
	id = store.Slug(firstNonEmpty(manifest.Title, manifest.ID))
	if !applayout.ValidID(id) {
		id = "app"
	}
	return id, firstNonEmpty(manifest.Title, humanTitle(id)), "the app"
}

// humanTitle turns a slug into a title: "checkout-flow" is "Checkout flow".
func humanTitle(id string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(id))
	if len(words) == 0 {
		return id
	}
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	return strings.Join(words, " ")
}

// assertFeature checks an optional --feature against the feature that holds an
// existing record.
func assertFeature(root, value, record, holding string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	feature, err := requireFeature(root, value)
	if err != nil {
		return err
	}
	if feature.ID != holding {
		return fmt.Errorf("%s is in feature %q, not %q; --feature must name the feature that holds it", record, holding, feature.ID)
	}
	return nil
}

// Feature dispatches the feature command family.
func Feature(ctx context.Context, args []string, out io.Writer) error {
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("feature", []string{"add"}, out)
	}
	var err error
	switch args[0] {
	case "add":
		err = featureAdd(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["feature"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "feature "+args[0], err)
	}
	return err
}

func featureAdd(_ context.Context, args []string, out io.Writer) error {
	name := "feature add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable feature id")
	title := flags.String("title", "", "feature title: the product domain it covers")
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
	urn := applayout.FeatureURN(manifest.ID, *id)
	path := applayout.FeatureRel(*id)
	var replayed bool
	err = store.WithSagaLock(root, store.DefaultLockTimeout, func() error {
		features, err := applayout.Features(root)
		if err != nil {
			return err
		}
		if existing, ok := applayout.Find(features, *id); ok {
			if *requestID != "" && existing.RequestID == *requestID && existing.Title == strings.TrimSpace(*title) && existing.Description == strings.TrimSpace(*description) {
				replayed = true
				return nil
			}
			return fmt.Errorf("feature %q already exists", *id)
		}
		_, err = applayout.WriteFeature(root, applayout.FeatureManifest{
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
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
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
	description := flags.String("description", "", "who this person is and what value they get from the app")
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
	description := flags.String("description", "", "complete revised description; keep it a person who gets value from the app")
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
	ctx, endGit := gitexec.Begin(ctx)
	defer endGit()
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
	flags.Var(&targets, "target", "story or feature URN the flag gates; repeatable")
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
	flags.Var(&targets, "target", "complete set of story or feature URNs the flag gates; repeatable")
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
	feature := flags.String("feature", "", "feature id or URN the story moves to")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *story, *feature); err != nil {
		return err
	}
	root := flags.Arg(0)
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.MoveStory(root, sagaID, requirements.MoveStoryInput{Story: *story, Feature: target.ID})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeRequirementsMutation(out, name, result, nil, true)
	}
	verb := "Moved"
	if result.Replayed {
		verb = "Already in feature " + target.ID + ":"
	}
	fmt.Fprintf(out, "%s %s\nPath: %s\n", verb, result.URN, result.Path)
	return nil
}

// recordFeature returns the feature that holds the record named by urn. Nested URNs
// (a criterion, revision, Item, or landmark) resolve through their parent.
func recordFeature(root, urn string) (string, error) {
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
				return story.Feature, nil
			}
		case "citation":
			for _, citation := range document.Citations {
				if citation.ID == id {
					return citation.Feature, nil
				}
			}
		case "relation":
			for _, relation := range document.Relations {
				if relation.ID == id {
					return relation.Feature, nil
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
				return prototype.Feature, nil
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
				return testCase.Feature, nil
			}
		}
		for _, policy := range document.Policies {
			if kind == "quality-policy" && policy.ID == id {
				return policy.Feature, nil
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
			return plan.Waves[id].Feature, nil
		case kind == "work-item" && plan.WorkItems[id] != nil:
			return plan.WorkItems[id].Feature, nil
		case kind == "dependency" && plan.Dependencies[id] != nil:
			return plan.Dependencies[id].Feature, nil
		case kind == "contract" && plan.Contracts[id] != nil:
			return plan.Contracts[id].Feature, nil
		}
		return "", missing
	case "claim":
		// A claim is an app-level overlay record; it belongs to the feature
		// holding the report content it makes a claim about.
		document, _, err := saga.Load(root)
		if err != nil {
			return "", err
		}
		for _, claim := range document.Claims {
			if claim.ID == id {
				return recordFeature(root, claim.Target)
			}
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
	if feature := saga.FeatureOf(found); feature != "" {
		return feature, nil
	}
	return "", fmt.Errorf("%s belongs to the app, not a feature", target)
}

// assertRecordFeature checks an optional --feature against the feature holding urn.
func assertRecordFeature(root, value, urn string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	holding, err := recordFeature(root, urn)
	if err != nil {
		return err
	}
	return assertFeature(root, value, urn, holding)
}
