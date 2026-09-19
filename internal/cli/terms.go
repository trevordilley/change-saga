package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// Term dispatches the term command family: the project's own vocabulary.
func Term(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("term", []string{"add", "revise", "set-state"}, out)
	}
	var err error
	switch args[0] {
	case "add":
		err = termAdd(ctx, args[1:], out)
	case "revise":
		err = termRevise(ctx, args[1:], out)
	case "set-state":
		err = termSetState(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["term"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, "term "+args[0], err)
	}
	return err
}

// termDefinitionFlags registers the complete content of a term revision.
type termDefinitionFlags struct {
	name, definition, repo *string
	aliases, stories       stringList
	records, refs          stringList
}

func registerTermDefinition(flags *flag.FlagSet) *termDefinitionFlags {
	value := &termDefinitionFlags{
		name:       flags.String("name", "", "the term as the team says it"),
		definition: flags.String("definition", "", "what the term means in this project"),
		repo:       flags.String("repo", "", "code checkout when the Saga lives in a separate repository"),
	}
	flags.Var(&value.aliases, "alias", "another spelling the team uses; repeatable")
	flags.Var(&value.stories, "story", "story URN or ID the term belongs to; repeatable")
	flags.Var(&value.records, "record", "persona, epic, flag, or term URN the term names; repeatable")
	flags.Var(&value.refs, "ref", "code that defines the term: <commit>:<path>[#L<start>[-L<end>]], where the commit may be any revision such as HEAD; repeatable")
	return value
}

// resolve turns the flags into a complete term definition, reading each
// code reference's digest from the code repository.
func (value *termDefinitionFlags) resolve(ctx context.Context, root, sagaID string) (requirements.TermDefinition, error) {
	stories := make([]string, 0, len(value.stories))
	for _, story := range value.stories {
		if livingid.ValidID(story) {
			story, _ = livingid.Story(sagaID, story)
		}
		stories = append(stories, story)
	}
	code, err := termReferences(ctx, firstNonEmpty(*value.repo, root), value.refs)
	if err != nil {
		return requirements.TermDefinition{}, err
	}
	return requirements.TermDefinition{
		Name: *value.name, Definition: *value.definition, Aliases: value.aliases,
		Stories: stories, Records: value.records, Code: code,
	}, nil
}

// termReferences authors a code reference for each --ref. The commit may be
// spelled as any revision, which is resolved to its full object name so the
// reference is pinned; the digest is read from that commit.
func termReferences(ctx context.Context, checkout string, values []string) ([]coderef.Reference, error) {
	if len(values) == 0 {
		return nil, nil
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return nil, fmt.Errorf("open the code repository (use --repo for a separate saga repository): %w", err)
	}
	defer resolver.Close()
	references := make([]coderef.Reference, 0, len(values))
	for index, value := range values {
		location, err := coderef.ParseLocation(value)
		if err != nil {
			revision, rest, found := strings.Cut(value, ":")
			if !found || revision == "" {
				return nil, fmt.Errorf("--ref %d: %w", index+1, err)
			}
			commit, resolveErr := resolveCommit(ctx, checkout, revision)
			if resolveErr != nil {
				return nil, fmt.Errorf("--ref %d: %w", index+1, resolveErr)
			}
			if location, err = coderef.ParseLocation(commit + ":" + rest); err != nil {
				return nil, fmt.Errorf("--ref %d: %w", index+1, err)
			}
		}
		reference, err := resolver.Author(ctx, location, "")
		if err != nil {
			return nil, fmt.Errorf("--ref %d: %w", index+1, err)
		}
		references = append(references, reference)
	}
	return references, nil
}

func termAdd(ctx context.Context, args []string, out io.Writer) error {
	name := "term add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable term id")
	revision := flags.String("revision", "r1", "initial revision id")
	event := flags.String("event", "active", "initial active-event id")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	content := registerTermDefinition(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *revision, *event, *content.name, *content.definition); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	definition, err := content.resolve(ctx, root, sagaID)
	if err != nil {
		return err
	}
	result, err := requirements.AddTerm(root, sagaID, requirements.AddTermInput{
		ID: *id, RevisionID: *revision, EventID: *event, TermDefinition: definition, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}

func termRevise(ctx context.Context, args []string, out io.Writer) error {
	name := "term revise"
	flags := commandFlags(name, commandUsage[name], out)
	term := flags.String("term", "", "canonical term URN")
	revision := flags.String("revision", "", "new revision id")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	content := registerTermDefinition(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *term, *revision, *content.name, *content.definition); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	definition, err := content.resolve(ctx, root, sagaID)
	if err != nil {
		return err
	}
	result, err := requirements.ReviseTerm(root, sagaID, requirements.ReviseTermInput{
		Term: *term, ID: *revision, Parents: parents, TermDefinition: definition, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func termSetState(_ context.Context, args []string, out io.Writer) error {
	name := "term set-state"
	flags := commandFlags(name, commandUsage[name], out)
	term := flags.String("term", "", "canonical term URN")
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
	if err := requireLivingArgs(flags, *term, *event, *state); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.SetTermState(root, sagaID, requirements.SetTermStateInput{
		Term: *term, ID: *event, Parents: parents, State: requirements.TermState(*state), Reason: *reason, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{*event}, *jsonOutput)
}
