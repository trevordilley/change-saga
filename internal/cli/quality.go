package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
)

var qualityOperations = []string{
	"test-case add", "test-case revise", "test-case set-state",
	"policy set", "evidence add", "run record",
}

// Quality exposes the v5 quality writers: test-case definitions and lifecycle,
// per-criterion kind policies, exact evidence, and immutable runs. Every
// record is append-only; the command never computes a score or percentage.
func Quality(ctx context.Context, args []string, out io.Writer) error {
	return qualityCommand(ctx, args, out, os.Stdin)
}

func qualityCommand(_ context.Context, args []string, out io.Writer, stdin io.Reader) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("quality", qualityOperations, out)
	}
	resource := args[0]
	if resource == "test" {
		resource = "test-case"
	}
	if len(args) < 2 || args[1] == "-h" || args[1] == "--help" {
		family := "quality " + resource
		if _, known := commandUsage[family]; !known {
			return fmt.Errorf("usage: %s", commandUsage["quality"])
		}
		return livingFamilyHelp(family, qualityFamilyOperations(resource), out)
	}
	operation := "quality " + resource + " " + args[1]
	var err error
	switch operation {
	case "quality test-case add", "quality test-case revise":
		err = qualityTestCaseDefine(operation, args[2:], out, stdin)
	case "quality test-case set-state":
		err = qualityTestCaseSetState(args[2:], out, stdin)
	case "quality policy set":
		err = qualityPolicySet(args[2:], out, stdin)
	case "quality evidence add":
		err = qualityEvidenceAdd(args[2:], out, stdin)
	case "quality run record":
		err = qualityRunRecord(args[2:], out, stdin)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["quality"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func qualityFamilyOperations(resource string) []string {
	operations := []string{}
	for _, operation := range qualityOperations {
		if strings.HasPrefix(operation, resource+" ") {
			operations = append(operations, strings.TrimPrefix(operation, resource+" "))
		}
	}
	return operations
}

// qualityTestCaseRequest is the structured --from shape for test-case add and
// revise. Definition fields mirror the frozen revision record.
type qualityTestCaseRequest struct {
	ID             string                 `json:"id,omitempty"`
	TestCase       string                 `json:"test_case,omitempty"`
	Revision       string                 `json:"revision,omitempty"`
	Event          string                 `json:"event,omitempty"`
	Parents        []string               `json:"parents,omitempty"`
	Title          string                 `json:"title,omitempty"`
	CoverageKinds  []quality.CoverageKind `json:"coverage_kinds,omitempty"`
	Automation     quality.Automation     `json:"automation,omitempty"`
	Preconditions  []string               `json:"preconditions,omitempty"`
	Steps          []quality.Step         `json:"steps,omitempty"`
	ExpectedResult string                 `json:"expected_result,omitempty"`
	CreatedAt      time.Time              `json:"created_at,omitempty"`
	RequestID      string                 `json:"request_id,omitempty"`
}

func qualityTestCaseDefine(name string, args []string, out io.Writer, stdin io.Reader) error {
	adding := name == "quality test-case add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable test-case id")
	testCase := registerTestCaseFlag(flags)
	revision := flags.String("revision", "", "stable revision id")
	event := flags.String("event", "", "stable id of the initial proposed lifecycle event")
	title := flags.String("title", "", "test-case title")
	automation := flags.String("automation", "", "manual, automated, or hybrid")
	expected := flags.String("expected-result", "", "overall expected result")
	from := flags.String("from", "", "read a structured request from a JSON file, or - for stdin")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents, kinds, preconditions, steps stringList
	flags.Var(&parents, "parent", "current revision head URN; repeatable")
	flags.Var(&kinds, "kind", "coverage kind: positive, negative, or edge; repeatable")
	flags.Var(&preconditions, "precondition", "ordered precondition; repeatable")
	flags.Var(&steps, "step", `ordered step JSON {"id":ID,"action":TEXT,"expected_result":TEXT}; repeatable`)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	request := qualityTestCaseRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	var parsedSteps []quality.Step
	for index, raw := range steps {
		var step quality.Step
		if err := decodeInlineJSON(raw, fmt.Sprintf("--step %d", index+1), &step); err != nil {
			return err
		}
		parsedSteps = append(parsedSteps, step)
	}
	overrideVisited(flags, map[string]func(){
		"id": func() { request.ID = *id }, "test": func() { request.TestCase = *testCase },
		"test-case": func() { request.TestCase = *testCase }, "revision": func() { request.Revision = *revision },
		"event": func() { request.Event = *event }, "parent": func() { request.Parents = parents },
		"title": func() { request.Title = *title }, "automation": func() { request.Automation = quality.Automation(*automation) },
		"expected-result": func() { request.ExpectedResult = *expected }, "request-id": func() { request.RequestID = *requestID },
		"kind": func() { request.CoverageKinds = coverageKinds(kinds) }, "precondition": func() { request.Preconditions = preconditions },
		"step": func() { request.Steps = parsedSteps },
	})
	root := flags.Arg(0)
	if adding {
		if request.Revision == "" {
			request.Revision = "r1"
		}
		if strings.TrimSpace(request.ID) == "" || request.TestCase != "" || len(request.Parents) != 0 {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		result, err := quality.AddTestCase(root, quality.AddTestCaseInput{
			ID: request.ID, RevisionID: request.Revision, EventID: request.Event,
			Definition: requestDefinition(quality.Definition{}, request), CreatedAt: request.CreatedAt, RequestID: request.RequestID,
		})
		if err != nil {
			return err
		}
		return writeQualityMutation(out, name, []quality.MutationResult{result}, *jsonOutput)
	}
	if strings.TrimSpace(request.TestCase) == "" || strings.TrimSpace(request.Revision) == "" || len(request.Parents) == 0 || request.ID != "" || request.Event != "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	base, err := singleParentDefinition(root, request.TestCase, request.Parents)
	if err != nil {
		return err
	}
	result, err := quality.ReviseTestCase(root, quality.ReviseTestCaseInput{
		TestCase: request.TestCase, RevisionID: request.Revision, Parents: request.Parents,
		Definition: requestDefinition(base, request), CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeQualityMutation(out, name, []quality.MutationResult{result}, *jsonOutput)
}

// singleParentDefinition lets a linear revision state only what changed: the
// parent's immutable definition is the base. A reconciliation of several heads
// has no single base, so it must state the complete definition.
func singleParentDefinition(root, testCaseURN string, parents []string) (quality.Definition, error) {
	if len(parents) != 1 {
		return quality.Definition{}, nil
	}
	document, err := quality.Load(root)
	if err != nil {
		return quality.Definition{}, err
	}
	for _, testCase := range document.TestCases {
		for _, revision := range testCase.Revisions {
			urn, _ := qualityid.Revision(document.SagaID, testCase.Identity.ID, revision.ID)
			if revision.TestCase == testCaseURN && urn == parents[0] {
				return quality.DefinitionOf(revision), nil
			}
		}
	}
	return quality.Definition{}, nil
}

func requestDefinition(base quality.Definition, request qualityTestCaseRequest) quality.Definition {
	if request.Title != "" {
		base.Title = request.Title
	}
	if request.CoverageKinds != nil {
		base.CoverageKinds = request.CoverageKinds
	}
	if request.Automation != "" {
		base.Automation = request.Automation
	}
	if request.Preconditions != nil {
		base.Preconditions = request.Preconditions
	}
	if request.Steps != nil {
		base.Steps = request.Steps
	}
	if request.ExpectedResult != "" {
		base.ExpectedResult = request.ExpectedResult
	}
	return base
}

type qualityStateRequest struct {
	TestCase  string    `json:"test_case,omitempty"`
	Event     string    `json:"event,omitempty"`
	Parents   []string  `json:"parents,omitempty"`
	State     string    `json:"state,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
}

func qualityTestCaseSetState(args []string, out io.Writer, stdin io.Reader) error {
	name := "quality test-case set-state"
	flags := commandFlags(name, commandUsage[name], out)
	testCase := registerTestCaseFlag(flags)
	event := flags.String("event", "", "stable lifecycle event id")
	state := flags.String("state", "", "proposed, active, deprecated, or retired")
	reason := flags.String("reason", "", "why the lifecycle changed")
	from := flags.String("from", "", "read a structured request from a JSON file, or - for stdin")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents stringList
	flags.Var(&parents, "parent", "current lifecycle head URN; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	request := qualityStateRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	overrideVisited(flags, map[string]func(){
		"test": func() { request.TestCase = *testCase }, "test-case": func() { request.TestCase = *testCase },
		"event": func() { request.Event = *event }, "state": func() { request.State = *state },
		"reason": func() { request.Reason = *reason }, "parent": func() { request.Parents = parents },
		"request-id": func() { request.RequestID = *requestID },
	})
	if request.Event == "" {
		request.Event = request.State
	}
	if strings.TrimSpace(request.TestCase) == "" || strings.TrimSpace(request.State) == "" || len(request.Parents) == 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	result, err := quality.SetTestCaseState(flags.Arg(0), quality.SetTestCaseStateInput{
		TestCase: request.TestCase, EventID: request.Event, Parents: request.Parents,
		State: quality.LifecycleState(request.State), Reason: request.Reason,
		CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeQualityMutation(out, name, []quality.MutationResult{result}, *jsonOutput)
}

type qualityPolicyRequest struct {
	ID                string                 `json:"id,omitempty"`
	Criterion         string                 `json:"criterion,omitempty"`
	StoryRevision     string                 `json:"story_revision,omitempty"`
	RequiredKinds     []quality.CoverageKind `json:"required_kinds,omitempty"`
	AllowedAutomation []quality.Automation   `json:"allowed_automation,omitempty"`
	Supersedes        []string               `json:"supersedes,omitempty"`
	Rationale         string                 `json:"rationale,omitempty"`
	CreatedAt         time.Time              `json:"created_at,omitempty"`
	RequestID         string                 `json:"request_id,omitempty"`
}

func qualityPolicySet(args []string, out io.Writer, stdin io.Reader) error {
	name := "quality policy set"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable policy id; defaults to <story>.<criterion>.<story-revision>")
	criterion := flags.String("criterion", "", "canonical criterion URN")
	storyRevision := flags.String("story-revision", "", "exact story revision URN the policy applies to")
	rationale := flags.String("rationale", "", "why these kinds are required")
	from := flags.String("from", "", "read a structured request from a JSON file, or - for stdin")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var required, allowed, supersedes stringList
	flags.Var(&required, "require", "required coverage kind: positive, negative, or edge; repeatable")
	flags.Var(&allowed, "allow", "allowed automation: automated, hybrid, or manual; repeatable (default all)")
	flags.Var(&supersedes, "supersedes", "current policy head URN this replaces; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	request := qualityPolicyRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	overrideVisited(flags, map[string]func(){
		"id": func() { request.ID = *id }, "criterion": func() { request.Criterion = *criterion },
		"story-revision": func() { request.StoryRevision = *storyRevision }, "rationale": func() { request.Rationale = *rationale },
		"require": func() { request.RequiredKinds = coverageKinds(required) }, "supersedes": func() { request.Supersedes = supersedes },
		"allow": func() { request.AllowedAutomation = automations(allowed) }, "request-id": func() { request.RequestID = *requestID },
	})
	if request.AllowedAutomation == nil {
		request.AllowedAutomation = []quality.Automation{quality.AutomationAutomated, quality.AutomationHybrid, quality.AutomationManual}
	}
	if strings.TrimSpace(request.Criterion) == "" || strings.TrimSpace(request.StoryRevision) == "" || len(request.RequiredKinds) == 0 || strings.TrimSpace(request.Rationale) == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	result, err := quality.SetPolicy(flags.Arg(0), quality.SetPolicyInput{
		ID: request.ID, Criterion: request.Criterion, StoryRevision: request.StoryRevision,
		RequiredKinds: request.RequiredKinds, AllowedAutomation: request.AllowedAutomation,
		Supersedes: request.Supersedes, Rationale: request.Rationale,
		CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeQualityMutation(out, name, []quality.MutationResult{result}, *jsonOutput)
}

type qualityEvidenceRequest struct {
	ID            string    `json:"id,omitempty"`
	TestCase      string    `json:"test_case,omitempty"`
	TestRevision  string    `json:"test_revision,omitempty"`
	Role          string    `json:"role,omitempty"`
	Code          []string  `json:"code,omitempty"`
	Verifications []string  `json:"verifications,omitempty"`
	Citations     []string  `json:"citations,omitempty"`
	Supersedes    []string  `json:"supersedes,omitempty"`
	CreatedAt     time.Time `json:"created_at,omitempty"`
	RequestID     string    `json:"request_id,omitempty"`
}

// input reads each code location's digest from the source repository, so a
// request names code the same way cover does and never supplies a digest.
func (request qualityEvidenceRequest) input(repository string) (quality.AddEvidenceInput, error) {
	code, err := authorLocations(context.Background(), repository, request.Code, "code")
	if err != nil {
		return quality.AddEvidenceInput{}, err
	}
	return quality.AddEvidenceInput{
		ID: request.ID, TestCase: request.TestCase, TestRevision: request.TestRevision,
		Role: quality.EvidenceRole(request.Role), Code: code, Verifications: request.Verifications,
		Citations: request.Citations, Supersedes: request.Supersedes,
		CreatedAt: request.CreatedAt, RequestID: request.RequestID,
	}, nil
}

func qualityEvidenceAdd(args []string, out io.Writer, stdin io.Reader) error {
	name := "quality evidence add"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable evidence id; defaults to the role name")
	testCase := registerTestCaseFlag(flags)
	testRevision := flags.String("test-revision", "", "test revision URN; defaults to the unique current head")
	role := flags.String("role", "", "test_implementation, implementation_under_test, or execution_artifact")
	from := flags.String("from", "", "read one structured request from a JSON file, or - for stdin")
	batch := flags.String("batch", "", "read a JSON array of requests from a file, or - for stdin; all or none are written")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	repoDir := flags.String("repo", "", "source repository checkout; required when separate")
	var code, verifications, citations, supersedes stringList
	flags.Var(&code, "code", "code location <commit>:<path>[#L<start>[-L<end>]]; repeatable")
	flags.Var(&verifications, "verification", "verification URN; repeatable")
	flags.Var(&citations, "citation", "citation URN; repeatable")
	flags.Var(&supersedes, "supersedes", "current evidence head URN this replaces; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	root := flags.Arg(0)
	if *batch != "" {
		combined := false
		flags.Visit(func(value *flag.Flag) {
			combined = combined || (value.Name != "batch" && value.Name != "json" && value.Name != "repo")
		})
		if combined {
			return fmt.Errorf("--batch cannot be combined with other request flags")
		}
		var requests []qualityEvidenceRequest
		if err := readStrictAuthoringJSON(*batch, stdin, &requests); err != nil {
			return err
		}
		inputs := make([]quality.AddEvidenceInput, 0, len(requests))
		for _, request := range requests {
			input, err := request.input(firstNonEmpty(*repoDir, root))
			if err != nil {
				return err
			}
			inputs = append(inputs, input)
		}
		results, err := quality.AddEvidenceBatch(root, inputs)
		if err != nil {
			return err
		}
		return writeQualityMutation(out, name, results, *jsonOutput)
	}
	request := qualityEvidenceRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	overrideVisited(flags, map[string]func(){
		"id": func() { request.ID = *id }, "test": func() { request.TestCase = *testCase },
		"test-case": func() { request.TestCase = *testCase }, "test-revision": func() { request.TestRevision = *testRevision },
		"role": func() { request.Role = *role }, "code": func() { request.Code = code },
		"verification": func() { request.Verifications = verifications }, "citation": func() { request.Citations = citations },
		"supersedes": func() { request.Supersedes = supersedes }, "request-id": func() { request.RequestID = *requestID },
	})
	if strings.TrimSpace(request.TestCase) == "" || strings.TrimSpace(request.Role) == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	input, err := request.input(firstNonEmpty(*repoDir, root))
	if err != nil {
		return err
	}
	result, err := quality.AddEvidence(root, input)
	if err != nil {
		return err
	}
	return writeQualityMutation(out, name, []quality.MutationResult{result}, *jsonOutput)
}

type qualityRunRequest struct {
	ID           string                  `json:"id,omitempty"`
	TestCase     string                  `json:"test_case,omitempty"`
	TestRevision string                  `json:"test_revision,omitempty"`
	Parents      []string                `json:"parents,omitempty"`
	Source       *quality.SourceIdentity `json:"source,omitempty"`
	Result       string                  `json:"result,omitempty"`
	Summary      string                  `json:"summary,omitempty"`
	Command      string                  `json:"command,omitempty"`
	Evidence     []string                `json:"evidence,omitempty"`
	ExecutedAt   time.Time               `json:"executed_at,omitempty"`
	RequestID    string                  `json:"request_id,omitempty"`
}

func qualityRunRecord(args []string, out io.Writer, stdin io.Reader) error {
	name := "quality run record"
	flags := commandFlags(name, commandUsage[name], out)
	id := flags.String("id", "", "stable run id; defaults to a time-plus-random id")
	testCase := registerTestCaseFlag(flags)
	testRevision := flags.String("test-revision", "", "test revision URN that ran; defaults to the unique current head")
	result := flags.String("result", "", "passed, failed, blocked, or skipped")
	summary := flags.String("summary", "", "what ran and what was observed")
	command := flags.String("command", "", "command that ran, if any; recorded, never executed")
	executedAt := flags.String("executed-at", "", "RFC 3339 execution time; defaults to now")
	repository := flags.String("repository", "", "source repository that ran; defaults to the Saga source")
	base := flags.String("base", "", "source base that ran; defaults to the Saga source")
	head := flags.String("head", "", "source head that ran; defaults to the Saga source")
	from := flags.String("from", "", "read a structured request from a JSON file, or - for stdin")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	var parents, evidence stringList
	flags.Var(&parents, "parent", "current run head URN; repeatable, omitted for the first run")
	flags.Var(&evidence, "evidence", "evidence URN the run relied on; repeatable")
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	request := qualityRunRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	var parseErr error
	overrideVisited(flags, map[string]func(){
		"id": func() { request.ID = *id }, "test": func() { request.TestCase = *testCase },
		"test-case": func() { request.TestCase = *testCase }, "test-revision": func() { request.TestRevision = *testRevision },
		"parent": func() { request.Parents = parents }, "result": func() { request.Result = *result },
		"summary": func() { request.Summary = *summary }, "command": func() { request.Command = *command },
		"evidence": func() { request.Evidence = evidence }, "request-id": func() { request.RequestID = *requestID },
		"executed-at": func() { request.ExecutedAt, parseErr = time.Parse(time.RFC3339Nano, *executedAt) },
	})
	if parseErr != nil {
		return fmt.Errorf("--executed-at must be RFC 3339: %w", parseErr)
	}
	root := flags.Arg(0)
	if *repository != "" || *base != "" || *head != "" {
		if *repository == "" || *base == "" || *head == "" {
			return fmt.Errorf("--repository, --base, and --head must be provided together")
		}
		request.Source = &quality.SourceIdentity{Repository: *repository, Base: *base, Head: *head}
	}
	if strings.TrimSpace(request.TestCase) == "" || strings.TrimSpace(request.Result) == "" || strings.TrimSpace(request.Summary) == "" || len(request.Evidence) == 0 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	recorded, err := quality.RecordRun(root, quality.RecordRunInput{
		ID: request.ID, TestCase: request.TestCase, TestRevision: request.TestRevision,
		Parents: request.Parents, Source: request.Source, Result: quality.RunResult(request.Result),
		Summary: request.Summary, Command: request.Command, Evidence: request.Evidence,
		ExecutedAt: request.ExecutedAt, RequestID: request.RequestID,
	})
	if err != nil {
		return err
	}
	return writeQualityMutation(out, name, []quality.MutationResult{recorded}, *jsonOutput)
}

// registerTestCaseFlag accepts the contract's --test spelling and the
// resource-named --test-case alias for the same canonical URN.
func registerTestCaseFlag(flags *flag.FlagSet) *string {
	value := flags.String("test", "", "canonical test-case URN")
	flags.StringVar(value, "test-case", "", "alias for --test")
	return value
}

func coverageKinds(values []string) []quality.CoverageKind {
	result := make([]quality.CoverageKind, 0, len(values))
	for _, value := range values {
		result = append(result, quality.CoverageKind(value))
	}
	return result
}

func automations(values []string) []quality.Automation {
	result := make([]quality.Automation, 0, len(values))
	for _, value := range values {
		result = append(result, quality.Automation(value))
	}
	return result
}

func writeQualityMutation(out io.Writer, operation string, results []quality.MutationResult, jsonOutput bool) error {
	output := livingMutationOutput{
		OK: true, Operation: operation, Paths: []string{}, Created: []string{},
		EventIDs: []string{}, CurrentHeads: []string{}, Replayed: len(results) > 0,
	}
	for _, result := range results {
		output.Created = append(output.Created, result.Created...)
		output.Paths = append(output.Paths, result.Paths...)
		output.CurrentHeads = append(output.CurrentHeads, result.CurrentHeads...)
		output.Replayed = output.Replayed && result.Replayed
	}
	if len(results) == 1 {
		output.Resource, output.Path = results[0].URN, results[0].Path
	}
	if jsonOutput {
		return writeJSON(out, output)
	}
	for _, result := range results {
		verb := "Created"
		if result.Replayed {
			verb = "Replayed"
		}
		fmt.Fprintf(out, "%s %s\n", verb, result.URN)
		fmt.Fprintf(out, "Path: %s\n", result.Path)
	}
	if len(output.CurrentHeads) > 0 {
		fmt.Fprintf(out, "Current heads: %s\n", strings.Join(output.CurrentHeads, " "))
	}
	return nil
}
