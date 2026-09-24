package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/twentyideas/changesaga/internal/saga"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

func Story(ctx context.Context, args []string, out io.Writer) error {
	return story(ctx, args, out, os.Stdin)
}

func story(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	operation := "story"
	var err error
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("story", []string{"add", "revise", "set-state", "move", "withdraw", "consolidate"}, out)
	}
	operation += " " + args[0]
	switch args[0] {
	case "add":
		err = storyAdd(ctx, args[1:], out, stdin)
	case "revise":
		err = storyRevise(ctx, args[1:], out, stdin)
	case "set-state":
		err = storySetState(ctx, args[1:], out, stdin)
	case "move":
		err = storyMove(ctx, args[1:], out)
	case "withdraw":
		err = storyWithdraw(ctx, args[1:], out)
	case "consolidate":
		err = storyConsolidate(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["story"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func Citation(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("citation", []string{"add"}, out)
	}
	operation := "citation " + args[0]
	var err error
	if args[0] == "add" {
		err = citationAdd(ctx, args[1:], out)
	} else {
		err = fmt.Errorf("usage: %s", commandUsage["citation"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func Relation(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("relation", []string{"add", "repin", "supersede", "status"}, out)
	}
	operation := "relation " + args[0]
	var err error
	switch args[0] {
	case "add":
		err = relationAdd(ctx, args[1:], out)
	case "repin":
		err = relationRepin(ctx, args[1:], out)
	case "supersede":
		err = relationSupersede(ctx, args[1:], out)
	case "status":
		err = relationStatus(ctx, args[1:], out)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["relation"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func requirementSagaID(root string) (string, error) {
	document, err := requirements.Load(root, "")
	if err != nil {
		return "", err
	}
	return document.SagaID, nil
}

func storyAdd(_ context.Context, args []string, out io.Writer, stdin io.Reader) error {
	name := "story add"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	id := flags.String("id", "", "stable story id")
	revision := flags.String("revision", "", "stable initial revision id")
	event := flags.String("event", "", "stable initial proposed-event id")
	title := flags.String("title", "", "story title")
	statement := flags.String("statement", "", "complete user-story statement")
	priority := flags.String("priority", "", "optional free-text priority the reviewer shows, such as must, should, or could; the tool reads no meaning into it")
	requestID := flags.String("request-id", "", "idempotency key")
	from := flags.String("from", "", "read a structured mutation request from a JSON file, or - for stdin")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	var citations, criteria, personas stringList
	flags.Var(&citations, "citation", "citation URN; repeatable")
	flags.Var(&criteria, "criterion", "acceptance criterion as ID=STATEMENT; repeatable")
	flags.Var(&personas, "persona", "persona URN the story serves: the \"As a ...\" of its statement; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", usage)
	}
	parsedCriteria, err := parseCriteria(criteria)
	if err != nil {
		return err
	}
	flagCriteria := make([]requirements.Criterion, 0, len(parsedCriteria))
	for _, value := range parsedCriteria {
		flagCriteria = append(flagCriteria, requirements.Criterion{ID: value.ID, Statement: value.Statement})
	}
	request := storyAddRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	overrideVisited(flags, map[string]func(){
		"id": func() { request.ID = *id }, "revision": func() { request.Revision = *revision },
		"event": func() { request.Event = *event }, "title": func() { request.Title = *title },
		"statement": func() { request.Statement = *statement }, "priority": func() { request.Priority = *priority },
		"request-id": func() { request.RequestID = *requestID },
		"citation":   func() { request.Citations = append([]string{}, citations...) },
		"criterion":  func() { request.AcceptanceCriteria = append([]requirements.Criterion{}, flagCriteria...) },
		"persona":    func() { request.Personas = append([]string{}, personas...) },
		"feature":    func() { request.Feature = *feature },
	})
	if *from == "" {
		request = storyAddRequest{Feature: *feature, ID: *id, Revision: *revision, Event: *event, Title: *title, Statement: *statement, Priority: *priority, Personas: personas, Citations: citations, RequestID: *requestID}
		request.AcceptanceCriteria = append([]requirements.Criterion{}, flagCriteria...)
	}
	if request.ID == "" || request.Revision == "" || request.Event == "" || request.Title == "" || request.Statement == "" {
		return fmt.Errorf("usage: %s", usage)
	}
	root := flags.Arg(0)
	target, err := requireFeature(root, request.Feature)
	if err != nil {
		return err
	}
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	result, err := requirements.AddStory(root, sagaID, requirements.AddStoryInput{
		Feature: target.ID, ID: request.ID, RevisionID: request.Revision, EventID: request.Event, Title: request.Title, Statement: request.Statement,
		Priority: request.Priority, Personas: request.Personas, Citations: request.Citations, AcceptanceCriteria: request.AcceptanceCriteria,
		CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{request.Event}, *jsonOutput)
}

func storyRevise(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	name := "story revise"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	story := flags.String("story", "", "canonical story URN")
	revision := flags.String("revision", "", "stable revision id")
	title := flags.String("title", "", "complete revised title; inherited from a single parent when omitted")
	statement := flags.String("statement", "", "complete revised user-story statement; inherited from a single parent when omitted")
	priority := flags.String("priority", "", "optional free-text priority; inherited from a single parent when omitted, so pass an empty value for none")
	requestID := flags.String("request-id", "", "idempotency key")
	from := flags.String("from", "", "read a structured complete revision from a JSON file, or - for stdin")
	edit := flags.Bool("edit", false, "edit the complete proposed revision with $EDITOR")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	var parents, citations, criteria, personas stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	flags.Var(&citations, "citation", "citation URN; repeatable; the parent's citations are kept when omitted")
	flags.Var(&criteria, "criterion", "acceptance criterion as ID=STATEMENT; repeatable; the parent's criteria are kept when omitted")
	flags.Var(&personas, "persona", "persona URN the revised story serves; repeatable; the parent's personas are kept when omitted")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 || (*from != "" && *edit) {
		return fmt.Errorf("usage: %s", usage)
	}
	parsedCriteria, err := parseCriteria(criteria)
	if err != nil {
		return err
	}
	flagCriteria := make([]requirements.Criterion, 0, len(parsedCriteria))
	for _, value := range parsedCriteria {
		flagCriteria = append(flagCriteria, requirements.Criterion{ID: value.ID, Statement: value.Statement})
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	request := storyReviseRequest{}
	if *from != "" {
		var value requirements.Revision
		if err := readStrictAuthoringJSON(*from, stdin, &value); err != nil {
			return err
		}
		if value.Schema != requirements.RevisionSchemaURL || value.Version != requirements.Version {
			return fmt.Errorf("structured story revision must use the v3 story-revision schema")
		}
		request = storyReviseRequest{Story: value.Story, Revision: value.ID, Parents: value.Parents, Title: value.Title, Statement: value.Statement, Priority: value.Priority, Personas: value.Personas, Citations: value.Citations, AcceptanceCriteria: value.AcceptanceCriteria, CreatedAt: value.CreatedAt, RequestID: value.RequestID}
	}
	overrideVisited(flags, map[string]func(){
		"story": func() { request.Story = *story }, "revision": func() { request.Revision = *revision },
		"title": func() { request.Title = *title }, "statement": func() { request.Statement = *statement },
		"priority": func() { request.Priority = *priority }, "request-id": func() { request.RequestID = *requestID },
		"parent":    func() { request.Parents = append([]string{}, parents...) },
		"citation":  func() { request.Citations = append([]string{}, citations...) },
		"criterion": func() { request.AcceptanceCriteria = append([]requirements.Criterion{}, flagCriteria...) },
		"persona":   func() { request.Personas = append([]string{}, personas...) },
	})
	if *from == "" {
		request = storyReviseRequest{Story: *story, Revision: *revision, Parents: parents, Title: *title, Statement: *statement, Priority: *priority, Personas: personas, Citations: citations, RequestID: *requestID}
		request.AcceptanceCriteria = append([]requirements.Criterion{}, flagCriteria...)
	}
	if *edit {
		if request.Story == "" || request.Revision == "" {
			return fmt.Errorf("usage: %s", usage)
		}
		request, err = editStoryRevision(ctx, root, sagaID, request)
		if err != nil {
			return err
		}
	}
	// A revision is a complete snapshot, so a flag-built revise that names one
	// parent inherits every field it was not given. Otherwise changing a title
	// would silently drop the criteria and citations nobody restated.
	if *from == "" && !*edit && request.Story != "" && len(request.Parents) == 1 {
		given := map[string]bool{}
		flags.Visit(func(value *flag.Flag) { given[value.Name] = true })
		request, err = inheritStoryRevision(root, sagaID, request, given)
		if err != nil {
			return err
		}
	}
	if request.Story == "" || request.Revision == "" || request.Title == "" || request.Statement == "" {
		return fmt.Errorf("usage: %s", usage)
	}
	if err := assertRecordFeature(root, *feature, request.Story); err != nil {
		return err
	}
	result, err := requirements.ReviseStory(root, sagaID, requirements.ReviseStoryInput{
		Story: request.Story, ID: request.Revision, Parents: request.Parents, Title: request.Title, Statement: request.Statement,
		Priority: request.Priority, Personas: request.Personas, Citations: request.Citations, AcceptanceCriteria: request.AcceptanceCriteria,
		CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func storySetState(_ context.Context, args []string, out io.Writer, stdin io.Reader) error {
	name := "story set-state"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	story := flags.String("story", "", "canonical story URN")
	event := flags.String("event", "", "stable lifecycle event id")
	state := flags.String("state", "", "proposed, accepted, deferred, rejected, or retired")
	reason := flags.String("reason", "", "reason for the lifecycle decision")
	requestID := flags.String("request-id", "", "idempotency key")
	from := flags.String("from", "", "read a structured lifecycle event from a JSON file, or - for stdin")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	var parents stringList
	flags.Var(&parents, "parent", "current lifecycle head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", usage)
	}
	request := storyStateRequest{}
	if *from != "" {
		var value requirements.LifecycleEvent
		if err := readStrictAuthoringJSON(*from, stdin, &value); err != nil {
			return err
		}
		if value.Schema != requirements.LifecycleEventSchemaURL || value.Version != requirements.Version {
			return fmt.Errorf("structured lifecycle event must use the v3 story-event schema")
		}
		request = storyStateRequest{Story: value.Story, Event: value.ID, Parents: value.Parents, State: value.State, Reason: value.Reason, CreatedAt: value.CreatedAt, RequestID: value.RequestID}
	}
	overrideVisited(flags, map[string]func(){
		"story": func() { request.Story = *story }, "event": func() { request.Event = *event },
		"state": func() { request.State = requirements.LifecycleState(*state) }, "reason": func() { request.Reason = *reason },
		"request-id": func() { request.RequestID = *requestID }, "parent": func() { request.Parents = append([]string{}, parents...) },
	})
	if *from == "" {
		request = storyStateRequest{Story: *story, Event: *event, Parents: parents, State: requirements.LifecycleState(*state), Reason: *reason, RequestID: *requestID}
	}
	if request.Story == "" || request.Event == "" || request.State == "" {
		return fmt.Errorf("usage: %s", usage)
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	if err := assertRecordFeature(root, *feature, request.Story); err != nil {
		return err
	}
	result, err := requirements.SetStoryState(root, sagaID, requirements.SetStoryStateInput{
		Story: request.Story, ID: request.Event, Parents: request.Parents, State: request.State,
		Reason: request.Reason, CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, []string{request.Event}, *jsonOutput)
}

func citationAdd(_ context.Context, args []string, out io.Writer) error {
	name := "citation add"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	id := flags.String("id", "", "stable citation id")
	kind := flags.String("kind", "", "url, repository_commit, issue, document, or decision")
	title := flags.String("title", "", "citation title")
	reference := flags.String("reference", "", "authoritative citation locator")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *kind, *title, *reference); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	result, err := requirements.AddCitation(root, sagaID, requirements.AddCitationInput{
		Feature: target.ID, ID: *id, Kind: requirements.CitationKind(*kind), Title: *title, Reference: *reference, RequestID: *requestID,
	})
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

func relationAdd(_ context.Context, args []string, out io.Writer) error {
	name := "relation add"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	id := flags.String("id", "", "stable relation id")
	typeName := flags.String("type", "", "refines, addresses, implements, explains, verifies, supersedes, or conflicts_with")
	from := flags.String("from", "", "source endpoint URN; visual story links require a slide Item")
	to := flags.String("to", "", "target endpoint URN")
	rationale := flags.String("rationale", "", "why the endpoints are related")
	fromRevision := flags.String("from-revision", "", "exact source definition revision URN (v5: defaults to the unique current head)")
	toRevision := flags.String("to-revision", "", "exact target definition revision URN (v5: defaults to the unique current head)")
	fromDigest := flags.String("from-content-digest", "", "exact source design content digest")
	toDigest := flags.String("to-content-digest", "", "exact target design content digest")
	scope := flags.String("scope", "", "self (default); new visual story links must belong to an Item, not descendants of a deck or slide")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *id, *typeName, *from, *to, *rationale); err != nil {
		return err
	}
	root := flags.Arg(0)
	target, err := requireFeature(root, *feature)
	if err != nil {
		return err
	}
	document, err := requirements.Load(root, "")
	if err != nil {
		return err
	}
	relationURN, _ := livingid.Relation(document.SagaID, *id)
	if err := guardEmbeddedCriterionRelationMutation(root, relationURN); err != nil {
		return err
	}
	input := requirements.AddRelationInput{
		Feature: target.ID, ID: *id, Type: requirements.RelationType(*typeName), From: *from, To: *to, Rationale: *rationale,
		FromRevision: *fromRevision, ToRevision: *toRevision, FromContentDigest: *fromDigest,
		ToContentDigest: *toDigest, Scope: requirements.RelationScope(*scope), RequestID: *requestID,
	}
	defaulted, err := defaultV5RelationPins(root, &input)
	if err != nil {
		return err
	}
	result, err := requirements.AddRelation(root, document.SagaID, input)
	if err != nil {
		return err
	}
	if len(defaulted) == 0 {
		return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
	}
	if *jsonOutput {
		return writeJSON(out, relationAddOutput{
			livingMutationOutput: livingMutationOutput{OK: true, Operation: name, Resource: result.URN, Path: result.Path, Created: []string{result.URN}, EventIDs: []string{}, Replayed: result.Replayed},
			DefaultedPins:        defaulted,
		})
	}
	if err := writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, false); err != nil {
		return err
	}
	for _, note := range defaulted {
		fmt.Fprintf(out, "Pinned %s\n", note)
	}
	return nil
}

// relationAddOutput extends the shared mutation output only when a pin was
// defaulted, so the reported bytes for explicit pins are unchanged.
type relationAddOutput struct {
	livingMutationOutput
	DefaultedPins []string `json:"defaulted_pins"`
}

// defaultV5RelationPins fills each omitted revision pin of a revision-bearing
// endpoint (story, criterion, work item, test case) with its unique current
// head, and reports each default so the author sees exactly what was pinned.
// supersedes never defaults: its pins are optional history, not currency.
func defaultV5RelationPins(root string, input *requirements.AddRelationInput) ([]string, error) {
	heads, err := loadRelationHeads(root)
	if err != nil {
		return nil, err
	}
	var defaulted []string
	if input.Type != requirements.RelationSupersedes {
		for _, side := range []struct {
			name     string
			endpoint string
			pin      *string
		}{{"from_revision", input.From, &input.FromRevision}, {"to_revision", input.To, &input.ToRevision}} {
			if *side.pin != "" {
				continue
			}
			head, revisionBearing, err := heads.currentRevisionHead(side.endpoint)
			if !revisionBearing {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("cannot default %s: %w", side.name, err)
			}
			*side.pin = head
			defaulted = append(defaulted, fmt.Sprintf("%s to current head %s", side.name, head))
		}
	}
	if input.Type == requirements.RelationAddresses || input.Type == requirements.RelationImplements {
		// A design endpoint is pinned by its content digest. Defaulting it to
		// the current digest matches the revision rule above and is reported.
		document, _, err := saga.Load(root)
		if err == nil {
			digests, digestErr := saga.CurrentDesignContentDigests(document)
			if digestErr != nil {
				return nil, digestErr
			}
			for _, side := range []struct {
				name     string
				endpoint string
				pin      *string
			}{{"from_content_digest", input.From, &input.FromContentDigest}, {"to_content_digest", input.To, &input.ToContentDigest}} {
				if digest, ok := digests[side.endpoint]; ok && *side.pin == "" {
					*side.pin = digest
					defaulted = append(defaulted, fmt.Sprintf("%s to current digest %s", side.name, digest))
				}
			}
		}
	}
	if err := heads.checkTestCasePin(input.From, input.FromRevision); err != nil {
		return nil, err
	}
	if err := heads.checkTestCasePin(input.To, input.ToRevision); err != nil {
		return nil, err
	}
	return defaulted, nil
}

func relationSupersede(_ context.Context, args []string, out io.Writer) error {
	name := "relation supersede"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	relation := flags.String("relation", "", "canonical relation URN")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *relation); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	if err := guardEmbeddedCriterionRelationMutation(root, *relation); err != nil {
		return err
	}
	if err := assertRecordFeature(root, *feature, *relation); err != nil {
		return err
	}
	result, err := requirements.SupersedeRelation(root, sagaID, *relation, time.Time{}, *requestID)
	if err != nil {
		return err
	}
	return writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, *jsonOutput)
}

// relationRepin appends one confirmation that a relation still holds. It is
// the answer to a stale pin that is still true: the relation keeps its id, its
// rationale, and the record of every revision anyone read it against, instead
// of being retired and renamed.
func relationRepin(_ context.Context, args []string, out io.Writer) error {
	name := "relation repin"
	usage := commandUsage[name]
	flags := commandFlags(name, usage, out)
	relation := flags.String("relation", "", "canonical relation URN")
	id := flags.String("id", "", "stable repin id; defaults to the next rN")
	rationale := flags.String("rationale", "", "why the relation still holds against the new pins")
	fromRevision := flags.String("from-revision", "", "advance the source revision pin")
	toRevision := flags.String("to-revision", "", "advance the target revision pin")
	fromDigest := flags.String("from-content-digest", "", "advance the source content digest")
	toDigest := flags.String("to-content-digest", "", "advance the target content digest")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if err := requireLivingArgs(flags, *relation, *rationale); err != nil {
		return err
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	if err := guardEmbeddedCriterionRelationMutation(root, *relation); err != nil {
		return err
	}
	if err := assertRecordFeature(root, *feature, *relation); err != nil {
		return err
	}
	input := requirements.RepinRelationInput{
		Relation: *relation, ID: *id, Rationale: *rationale, RequestID: *requestID,
		FromRevision: *fromRevision, ToRevision: *toRevision,
		FromContentDigest: *fromDigest, ToContentDigest: *toDigest,
	}
	defaulted, err := defaultRepinPins(root, &input)
	if err != nil {
		return err
	}
	result, err := requirements.RepinRelation(root, sagaID, input)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return writeJSON(out, relationAddOutput{
			livingMutationOutput: livingMutationOutput{OK: true, Operation: name, Resource: result.URN, Path: result.Path,
				Created: []string{result.URN}, Replayed: result.Replayed},
			DefaultedPins: append([]string{}, defaulted...),
		})
	}
	if err := writeLivingMutation(out, name, result.URN, result.Path, []string{result.URN}, nil, result.Replayed, false); err != nil {
		return err
	}
	for _, note := range defaulted {
		fmt.Fprintf(out, "Re-pinned %s\n", note)
	}
	return nil
}

func guardEmbeddedCriterionRelationMutation(root, relationURN string) error {
	ref, err := livingid.Parse(relationURN)
	if err != nil || ref.Kind != livingid.KindRelation {
		return nil
	}
	document, validation, err := saga.Load(root)
	if err != nil {
		return err
	}
	if !validation.Valid {
		return fmt.Errorf("the Saga is invalid; run change-saga validate for details")
	}
	for _, deck := range document.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				for _, link := range item.CriterionLinks {
					if link.ID == ref.ID {
						return completeSlideMutationError("relation mutation", slide.Target)
					}
				}
			}
		}
	}
	return nil
}

// defaultRepinPins fills every omitted pin the relation is behind on with the
// endpoint's current head or digest, and reports each one. It never decides
// whether the repin is worth writing: requirements.RepinRelation refuses one
// that records the pins already confirmed, so naming a pin explicitly is not a
// way around that check.
func defaultRepinPins(root string, input *requirements.RepinRelationInput) ([]string, error) {
	heads, err := loadRelationHeads(root)
	if err != nil {
		return nil, err
	}
	ref, err := livingid.Parse(input.Relation)
	if err != nil || ref.Kind != livingid.KindRelation {
		return nil, fmt.Errorf("relation must be a canonical relation URN")
	}
	var record *requirements.Relation
	for index := range heads.document.Relations {
		if heads.document.Relations[index].ID == ref.ID {
			record = &heads.document.Relations[index]
		}
	}
	if record == nil {
		return nil, fmt.Errorf("relation %q does not exist", input.Relation)
	}
	// A replayed request re-states the repin it already wrote, so an agent that
	// retries gets the same answer instead of "nothing moved".
	if input.RequestID != "" {
		for _, known := range record.Repins {
			if known.RequestID != input.RequestID {
				continue
			}
			input.ID = known.ID
			input.FromRevision, input.ToRevision = known.FromRevision, known.ToRevision
			input.FromContentDigest, input.ToContentDigest = known.FromContentDigest, known.ToContentDigest
			return nil, nil
		}
	}
	confirmed := record.Confirmed()
	var defaulted []string
	for _, side := range []struct {
		name, endpoint, was string
		pin                 *string
	}{
		{"from_revision", confirmed.From, confirmed.FromRevision, &input.FromRevision},
		{"to_revision", confirmed.To, confirmed.ToRevision, &input.ToRevision},
	} {
		if *side.pin != "" || side.was == "" {
			continue
		}
		head, revisionBearing, err := heads.currentRevisionHead(side.endpoint)
		if !revisionBearing {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("cannot default %s: %w", side.name, err)
		}
		if head == side.was {
			continue
		}
		*side.pin = head
		defaulted = append(defaulted, fmt.Sprintf("%s from %s to the current head %s", side.name, side.was, head))
	}
	for _, side := range []struct {
		name, endpoint, was string
		pin                 *string
	}{
		{"from_content_digest", confirmed.From, confirmed.FromContentDigest, &input.FromContentDigest},
		{"to_content_digest", confirmed.To, confirmed.ToContentDigest, &input.ToContentDigest},
	} {
		if *side.pin != "" || side.was == "" {
			continue
		}
		if digest, ok := heads.inputs.CurrentContentDigests[side.endpoint]; ok && digest != side.was {
			*side.pin = digest
			defaulted = append(defaulted, fmt.Sprintf("%s from %s to the current digest %s", side.name, side.was, digest))
		}
	}
	return defaulted, nil
}
