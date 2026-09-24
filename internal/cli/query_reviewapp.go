package cli

import (
	"context"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/reviewapp"
)

type reviewAppQuerySession struct {
	session       reviewapp.Session
	livingSession livingapp.Session
	operation     string
}

type slideQueryContent struct {
	Target            string                       `json:"target"`
	ID                string                       `json:"id"`
	Title             string                       `json:"title"`
	Intent            string                       `json:"intent"`
	Layout            string                       `json:"layout"`
	Section           string                       `json:"section,omitempty"`
	Takeaway          string                       `json:"takeaway"`
	MediaType         string                       `json:"media_type"`
	Content           reviewapp.FragmentChunk      `json:"content"`
	Assets            []reviewapp.AssetSummary     `json:"assets"`
	Items             []reviewapp.SemanticLandmark `json:"items"`
	ReadingOrder      []string                     `json:"reading_order"`
	AuthoringSnapshot string                       `json:"authoring_snapshot,omitempty"`
	AuthoringHeads    []string                     `json:"authoring_heads,omitempty"`
	AuthoringConflict bool                         `json:"authoring_conflict,omitempty"`
}

func openReviewAppSession(ctx context.Context, options queryOpenOptions) (querySession, error) {
	if isLivingQueryOperation(options.Operation) {
		// The existing review application owns the public saga/source snapshot.
		// Reusing it keeps cursors and envelopes comparable across old and living
		// query operations while livingapp remains a transport-neutral composer.
		reviewSession, err := reviewapp.Open(ctx, reviewapp.OpenOptions{SagaRoot: options.SagaRoot, SourceDir: options.SourceDir, Range: options.Range, SummaryOnly: true})
		if err != nil {
			return nil, err
		}
		session, err := livingapp.Open(ctx, livingapp.OpenOptions{SagaRoot: options.SagaRoot, Snapshot: reviewSession.Snapshot(), Audit: options.Operation == "audit"})
		if err != nil {
			return nil, err
		}
		return &reviewAppQuerySession{session: reviewSession, livingSession: session, operation: options.Operation}, nil
	}
	session, err := reviewapp.Open(ctx, reviewapp.OpenOptions{SagaRoot: options.SagaRoot, SourceDir: options.SourceDir, Range: options.Range, SummaryOnly: options.SummaryOnly})
	if err != nil {
		return nil, err
	}
	return &reviewAppQuerySession{session: session, operation: options.Operation}, nil
}

func (s *reviewAppQuerySession) Snapshot() string {
	if s.livingSession != nil {
		return s.livingSession.Snapshot()
	}
	return s.session.Snapshot()
}

func (s *reviewAppQuerySession) Overview(ctx context.Context, _ overviewQuery) (any, error) {
	return s.session.Overview(ctx, reviewapp.OverviewQuery{})
}

func (s *reviewAppQuerySession) Children(ctx context.Context, query childrenQuery) (queryPage, error) {
	value, err := s.session.Children(ctx, reviewapp.ChildrenQuery{Parent: query.Parent, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) ReadFragment(ctx context.Context, query fragmentQuery) (any, error) {
	value, err := s.session.ReadFragment(ctx, reviewapp.FragmentQuery{Target: query.Target, Offset: query.Offset, Limit: query.Limit})
	if err != nil {
		return value, err
	}
	if s.operation == "fragment" && value.Intent != "" {
		return nil, &queryError{Code: "invalid_argument", Message: "slide targets must be read with the slide operation"}
	}
	if s.operation != "slide" {
		return value, nil
	}
	if value.Intent == "" {
		return nil, &queryError{Code: "invalid_argument", Message: "slide operation target must identify a slide"}
	}
	return slideQueryContent{Target: value.Target, ID: value.ID, Title: value.Title, Intent: value.Intent, Layout: value.Layout, Section: value.Section, Takeaway: value.Takeaway, MediaType: value.MediaType, Content: value.Content, Assets: value.Assets, Items: value.Landmarks, ReadingOrder: value.ReadingOrder, AuthoringSnapshot: value.AuthoringSnapshot, AuthoringHeads: value.AuthoringHeads, AuthoringConflict: value.AuthoringConflict}, nil
}

func (s *reviewAppQuerySession) FragmentDiffs(ctx context.Context, query fragmentDiffQuery) (queryPage, error) {
	if s.operation == "slide-diffs" && !strings.Contains(query.Target, ":item:") {
		return queryPage{}, &queryError{Code: "invalid_argument", Message: "slide-diffs target must identify an Item"}
	}
	if s.operation == "fragment-diffs" && strings.Contains(query.Target, ":item:") {
		return queryPage{}, &queryError{Code: "invalid_argument", Message: "Item targets must be read with slide-diffs"}
	}
	value, err := s.session.FragmentDiffs(ctx, reviewapp.FragmentDiffQuery{Target: query.Target, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) DiffOwners(ctx context.Context, query diffOwnerQuery) (queryPage, error) {
	value, err := s.session.DiffOwners(ctx, reviewapp.DiffOwnerQuery{Ref: query.Ref, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) Gaps(ctx context.Context, query gapQuery) (queryPage, error) {
	value, err := s.session.Gaps(ctx, reviewapp.GapQuery{Kind: query.Kind, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) Mappings(ctx context.Context, query mappingQuery) (queryPage, error) {
	value, err := s.session.Mappings(ctx, reviewapp.MappingQuery{Target: query.Target, Sort: query.Sort, MinimumScore: query.MinimumScore, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) Claims(ctx context.Context, query claimQuery) (queryPage, error) {
	value, err := s.session.Claims(ctx, reviewapp.ClaimQuery{Target: query.Target, Status: query.Status, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) Verifications(ctx context.Context, query verificationQuery) (queryPage, error) {
	value, err := s.session.Verifications(ctx, reviewapp.VerificationQuery{Claim: query.Claim, Status: query.Status, Cursor: query.Cursor, Limit: query.Limit})
	return queryPage{Data: value, Page: queryPageFromApplication(value.Page)}, err
}

func (s *reviewAppQuerySession) Living(ctx context.Context, query livingQuery) (queryPage, error) {
	value, err := s.livingSession.Query(ctx, livingapp.Query{Operation: query.Operation, Filters: query.Filters, Cursor: query.Cursor, Limit: query.Limit})
	if err == nil && query.Operation == "audit" {
		report, ok := value.Data.(livingapp.AuditReport)
		if !ok {
			return queryPage{}, &queryError{Code: "internal", Message: "audit returned an unexpected report"}
		}
		targets := map[string]bool{}
		for _, target := range report.ItemTargets {
			targets[target] = true
		}
		cursor := ""
		for {
			gaps, gapErr := s.session.Gaps(ctx, reviewapp.GapQuery{Kind: "stale", Cursor: cursor, Limit: maxQueryPageSize})
			if gapErr != nil {
				return queryPage{}, gapErr
			}
			for _, gap := range gaps.Gaps {
				if gap.Stale == nil || !targets[gap.Stale.Target] {
					continue
				}
				report.AddStaleSelector(gap.Stale.Target, gap.Stale.Reference.Location().String(), gap.Stale.EvidenceFile, gap.Stale.Reason)
			}
			if !gaps.Page.HasMore || gaps.Page.NextCursor == nil {
				break
			}
			cursor = *gaps.Page.NextCursor
		}
		report.Finalize()
		value.Data = report
	}
	return queryPage{Data: value.Data, Page: queryPageFromLiving(value.Page)}, err
}

func queryPageFromApplication(page reviewapp.Page) queryPageEnvelope {
	return queryPageEnvelope{Total: page.Total, Returned: page.Returned, HasMore: page.HasMore, NextCursor: page.NextCursor}
}

func queryPageFromLiving(page livingapp.Page) queryPageEnvelope {
	return queryPageEnvelope{Total: page.Total, Returned: page.Returned, HasMore: page.HasMore, NextCursor: page.NextCursor}
}

func isLivingQueryOperation(operation string) bool {
	switch operation {
	case "context", "personas", "persona-references", "term-references", "requirements", "requirement-history", "citations", "relations", "waves", "work-items", "work-events", "work-conflicts", "traceability", "readiness", "audit":
		return true
	default:
		return false
	}
}
