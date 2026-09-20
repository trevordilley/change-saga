package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

type criterionMutationRequest struct {
	Story     string    `json:"story"`
	Criterion string    `json:"criterion,omitempty"`
	Parent    string    `json:"parent"`
	Revision  string    `json:"revision"`
	ID        string    `json:"id,omitempty"`
	Statement string    `json:"statement,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	RequestID string    `json:"request_id,omitempty"`
}

func Criterion(ctx context.Context, args []string, out io.Writer) error {
	return criterion(ctx, args, out, os.Stdin)
}

func criterion(ctx context.Context, args []string, out io.Writer, stdin io.Reader) error {
	operation := "criterion"
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		return livingFamilyHelp("criterion", []string{"add", "revise", "remove"}, out)
	}
	operation += " " + args[0]
	var err error
	switch args[0] {
	case "add", "revise", "remove":
		err = criterionMutate(ctx, args[0], args[1:], out, stdin)
	default:
		err = fmt.Errorf("usage: %s", commandUsage["criterion"])
	}
	if err != nil && jsonFlagRequested(args) {
		return reportLivingMutationFailure(out, operation, err)
	}
	return err
}

func criterionMutate(ctx context.Context, operation string, args []string, out io.Writer, stdin io.Reader) error {
	name := "criterion " + operation
	flags := commandFlags(name, commandUsage[name], out)
	story := flags.String("story", "", "canonical story URN")
	criterionURN := flags.String("criterion", "", "canonical criterion URN")
	parent := flags.String("parent", "", "explicit current story revision head URN")
	revision := flags.String("revision", "", "stable new story revision id")
	id := flags.String("id", "", "stable criterion id")
	statement := flags.String("statement", "", "acceptance criterion statement")
	reason := flags.String("reason", "", "reason for removing the criterion")
	from := flags.String("from", "", "read a structured mutation request from a JSON file, or - for stdin")
	requestID := flags.String("request-id", "", "idempotency key")
	jsonOutput := flags.Bool("json", false, "emit a machine-readable result")
	edit := flags.Bool("edit", false, "edit the complete proposed story revision with $EDITOR")
	feature := featureIDFlag(flags)
	if err := flags.Parse(normalizeLivingArgs(args)); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	if operation != "revise" && *edit {
		return fmt.Errorf("--edit is supported only by criterion revise")
	}
	if *from != "" && *edit {
		return fmt.Errorf("--from and --edit cannot be used together")
	}
	request := criterionMutationRequest{}
	if *from != "" {
		if err := readStrictAuthoringJSON(*from, stdin, &request); err != nil {
			return err
		}
	}
	overrideVisited(flags, map[string]func(){
		"story": func() { request.Story = *story }, "criterion": func() { request.Criterion = *criterionURN },
		"parent": func() { request.Parent = *parent }, "revision": func() { request.Revision = *revision },
		"id": func() { request.ID = *id }, "statement": func() { request.Statement = *statement },
		"reason": func() { request.Reason = *reason }, "request-id": func() { request.RequestID = *requestID },
	})
	if *from == "" {
		request = criterionMutationRequest{
			Story: *story, Criterion: *criterionURN, Parent: *parent, Revision: *revision,
			ID: *id, Statement: *statement, Reason: *reason, RequestID: *requestID,
		}
	}
	if strings.TrimSpace(request.Story) == "" || strings.TrimSpace(request.Parent) == "" || strings.TrimSpace(request.Revision) == "" {
		return fmt.Errorf("usage: %s", commandUsage[name])
	}
	root := flags.Arg(0)
	sagaID, err := requirementSagaID(root)
	if err != nil {
		return err
	}
	if err := assertRecordFeature(root, *feature, request.Story); err != nil {
		return err
	}
	if *edit {
		if strings.TrimSpace(request.Criterion) == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		request, err = editCriterionRevision(ctx, root, sagaID, request)
		if err != nil {
			return err
		}
	}
	var result requirements.MutationResult
	switch operation {
	case "add":
		if strings.TrimSpace(request.ID) == "" || strings.TrimSpace(request.Statement) == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		result, err = requirements.AddCriterion(root, sagaID, requirements.AddCriterionInput{
			Story: request.Story, Parent: request.Parent, RevisionID: request.Revision,
			Criterion: requirements.Criterion{ID: request.ID, Statement: request.Statement},
			CreatedAt: request.CreatedAt, RequestID: request.RequestID,
		})
	case "revise":
		if strings.TrimSpace(request.Criterion) == "" || strings.TrimSpace(request.Statement) == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		result, err = requirements.ReviseCriterion(root, sagaID, requirements.ReviseCriterionInput{
			Story: request.Story, Criterion: request.Criterion, Parent: request.Parent,
			RevisionID: request.Revision, Statement: request.Statement,
			CreatedAt: request.CreatedAt, RequestID: request.RequestID,
		})
	case "remove":
		if strings.TrimSpace(request.Criterion) == "" || strings.TrimSpace(request.Reason) == "" {
			return fmt.Errorf("usage: %s", commandUsage[name])
		}
		result, err = requirements.RemoveCriterion(root, sagaID, requirements.RemoveCriterionInput{
			Story: request.Story, Criterion: request.Criterion, Parent: request.Parent,
			RevisionID: request.Revision, Reason: request.Reason,
			CreatedAt: request.CreatedAt, RequestID: request.RequestID,
		})
	}
	if err != nil {
		return err
	}
	return writeRequirementsMutation(out, name, result, nil, *jsonOutput)
}

func editCriterionRevision(ctx context.Context, root, sagaID string, request criterionMutationRequest) (criterionMutationRequest, error) {
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return request, err
	}
	storyRef, err := livingid.Parse(request.Story)
	if err != nil || storyRef.Kind != livingid.KindStory || storyRef.SagaID != sagaID {
		return request, fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	criterionRef, err := livingid.Parse(request.Criterion)
	if err != nil || criterionRef.Kind != livingid.KindCriterion || criterionRef.SagaID != sagaID || criterionRef.ParentID != storyRef.ID {
		return request, fmt.Errorf("criterion must be a canonical criterion URN for story %q", storyRef.ID)
	}
	var current *requirements.Revision
	for index := range document.Stories {
		if document.Stories[index].Identity.ID != storyRef.ID {
			continue
		}
		if len(document.Stories[index].RevisionHeads) != 1 || document.Stories[index].RevisionHeads[0] != request.Parent {
			return request, fmt.Errorf("criterion parent is not the unique current story revision head")
		}
		current = document.Stories[index].CurrentRevision
		break
	}
	if current == nil {
		return request, fmt.Errorf("story %q does not have a unique current revision", storyRef.ID)
	}
	candidate := *current
	candidate.ID = request.Revision
	candidate.Parents = []string{request.Parent}
	candidate.AcceptanceCriteria = append([]requirements.Criterion{}, current.AcceptanceCriteria...)
	candidate.CreatedAt = request.CreatedAt
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now().UTC()
	}
	candidate.RequestID = request.RequestID
	found := false
	for index := range candidate.AcceptanceCriteria {
		if candidate.AcceptanceCriteria[index].ID == criterionRef.ID {
			found = true
			if strings.TrimSpace(request.Statement) != "" {
				candidate.AcceptanceCriteria[index].Statement = strings.TrimSpace(request.Statement)
			}
		}
	}
	if !found {
		return request, fmt.Errorf("criterion %q is not present in parent revision", criterionRef.ID)
	}
	edited, err := runRevisionEditor(ctx, candidate)
	if err != nil {
		return request, err
	}
	baseline := candidate
	baseline.AcceptanceCriteria = append([]requirements.Criterion{}, candidate.AcceptanceCriteria...)
	var editedStatement string
	for index := range edited.AcceptanceCriteria {
		if index >= len(current.AcceptanceCriteria) || edited.AcceptanceCriteria[index].ID != current.AcceptanceCriteria[index].ID {
			return request, fmt.Errorf("criterion revise --edit may not add, remove, or reorder criteria")
		}
		if edited.AcceptanceCriteria[index].ID == criterionRef.ID {
			editedStatement = edited.AcceptanceCriteria[index].Statement
			baseline.AcceptanceCriteria[index].Statement = edited.AcceptanceCriteria[index].Statement
		}
	}
	if len(edited.AcceptanceCriteria) != len(current.AcceptanceCriteria) || !reflect.DeepEqual(edited, baseline) {
		return request, fmt.Errorf("criterion revise --edit may change only the selected criterion statement")
	}
	request.Statement = editedStatement
	request.CreatedAt = edited.CreatedAt
	return request, nil
}

func runRevisionEditor(ctx context.Context, revision requirements.Revision) (requirements.Revision, error) {
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return requirements.Revision{}, fmt.Errorf("$EDITOR is not set")
	}
	editorArgs := strings.Fields(editor)
	if len(editorArgs) == 0 {
		return requirements.Revision{}, fmt.Errorf("$EDITOR is not set")
	}
	file, err := os.CreateTemp("", "change-saga-story-revision-*.json")
	if err != nil {
		return requirements.Revision{}, err
	}
	path := file.Name()
	defer os.Remove(path)
	data, err := json.MarshalIndent(revision, "", "  ")
	if err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return requirements.Revision{}, err
	}
	command := exec.CommandContext(ctx, editorArgs[0], append(editorArgs[1:], path)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := command.Run(); err != nil {
		return requirements.Revision{}, fmt.Errorf("editor failed: %w", err)
	}
	var edited requirements.Revision
	if err := readStrictAuthoringJSON(path, nil, &edited); err != nil {
		return requirements.Revision{}, fmt.Errorf("edited revision: %w", err)
	}
	return edited, nil
}

func readStrictAuthoringJSON(source string, stdin io.Reader, target any) error {
	var data []byte
	var err error
	if source == "-" {
		if stdin == nil {
			return fmt.Errorf("stdin is unavailable")
		}
		data, err = io.ReadAll(io.LimitReader(stdin, requirements.MaxRecordBytes+1))
	} else {
		info, statErr := os.Lstat(source)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("structured input must be a real regular file")
		}
		if info.Size() > requirements.MaxRecordBytes {
			return fmt.Errorf("structured input exceeds %d bytes", requirements.MaxRecordBytes)
		}
		data, err = os.ReadFile(filepath.Clean(source))
	}
	if err != nil {
		return err
	}
	if len(data) > requirements.MaxRecordBytes {
		return fmt.Errorf("structured input exceeds %d bytes", requirements.MaxRecordBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid structured input: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("invalid structured input: trailing content")
	}
	return nil
}

func overrideVisited(flags interface{ Visit(func(*flag.Flag)) }, values map[string]func()) {
	flags.Visit(func(value *flag.Flag) {
		if apply := values[value.Name]; apply != nil {
			apply()
		}
	})
}
