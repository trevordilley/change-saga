package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

type storyAddRequest struct {
	Epic               string                   `json:"epic"`
	ID                 string                   `json:"id"`
	Revision           string                   `json:"revision"`
	Event              string                   `json:"event"`
	Title              string                   `json:"title"`
	Statement          string                   `json:"statement"`
	Priority           string                   `json:"priority"`
	Personas           []string                 `json:"personas"`
	Citations          []string                 `json:"citations,omitempty"`
	AcceptanceCriteria []requirements.Criterion `json:"acceptance_criteria,omitempty"`
	CreatedAt          time.Time                `json:"created_at,omitempty"`
	RequestID          string                   `json:"request_id,omitempty"`
}

type storyReviseRequest struct {
	Story              string
	Revision           string
	Parents            []string
	Title              string
	Statement          string
	Priority           string
	Personas           []string
	Citations          []string
	AcceptanceCriteria []requirements.Criterion
	CreatedAt          time.Time
	RequestID          string
}

type storyStateRequest struct {
	Story     string
	Event     string
	Parents   []string
	State     requirements.LifecycleState
	Reason    string
	CreatedAt time.Time
	RequestID string
}

func editStoryRevision(ctx context.Context, root, sagaID string, request storyReviseRequest) (storyReviseRequest, error) {
	storyRef, err := livingid.Parse(request.Story)
	if err != nil || storyRef.Kind != livingid.KindStory || storyRef.SagaID != sagaID {
		return request, fmt.Errorf("story must be a canonical story URN in saga %q", sagaID)
	}
	if len(request.Parents) != 1 {
		return request, fmt.Errorf("story revise --edit requires exactly one explicit current parent; use structured --from to reconcile multiple heads")
	}
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		return request, err
	}
	var current *requirements.Revision
	for index := range document.Stories {
		story := &document.Stories[index]
		if story.Identity.ID != storyRef.ID {
			continue
		}
		if len(story.RevisionHeads) != 1 || story.RevisionHeads[0] != request.Parents[0] {
			return request, fmt.Errorf("story revise --edit parent is not the unique current revision head")
		}
		current = story.CurrentRevision
		break
	}
	if current == nil {
		return request, fmt.Errorf("story %q does not have a unique current revision", storyRef.ID)
	}
	candidate := *current
	candidate.ID = request.Revision
	candidate.Parents = append([]string{}, request.Parents...)
	candidate.Personas = append([]string{}, current.Personas...)
	candidate.Citations = append([]string{}, current.Citations...)
	candidate.AcceptanceCriteria = append([]requirements.Criterion{}, current.AcceptanceCriteria...)
	if request.Personas != nil {
		candidate.Personas = append([]string{}, request.Personas...)
	}
	if strings.TrimSpace(request.Title) != "" {
		candidate.Title = request.Title
	}
	if strings.TrimSpace(request.Statement) != "" {
		candidate.Statement = request.Statement
	}
	if strings.TrimSpace(request.Priority) != "" {
		candidate.Priority = request.Priority
	}
	if request.Citations != nil {
		candidate.Citations = append([]string{}, request.Citations...)
	}
	if request.AcceptanceCriteria != nil {
		candidate.AcceptanceCriteria = append([]requirements.Criterion{}, request.AcceptanceCriteria...)
	}
	candidate.CreatedAt = request.CreatedAt
	if candidate.CreatedAt.IsZero() {
		candidate.CreatedAt = time.Now().UTC()
	}
	candidate.RequestID = request.RequestID
	edited, err := runRevisionEditor(ctx, candidate)
	if err != nil {
		return request, err
	}
	if edited.Schema != requirements.RevisionSchemaURL || edited.Version != requirements.Version || edited.ID != request.Revision || edited.Story != request.Story || len(edited.Parents) != 1 || edited.Parents[0] != request.Parents[0] || !edited.CreatedAt.Equal(candidate.CreatedAt) || edited.RequestID != candidate.RequestID {
		return request, fmt.Errorf("story revise --edit may not change schema, version, story, revision id, parents, created_at, or request_id")
	}
	return storyReviseRequest{
		Story: edited.Story, Revision: edited.ID, Parents: edited.Parents, Title: edited.Title,
		Statement: edited.Statement, Priority: edited.Priority, Personas: edited.Personas, Citations: edited.Citations,
		AcceptanceCriteria: edited.AcceptanceCriteria, CreatedAt: edited.CreatedAt, RequestID: edited.RequestID,
	}, nil
}
