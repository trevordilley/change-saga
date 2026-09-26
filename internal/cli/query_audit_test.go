package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingapp"
	"github.com/twentyideas/changesaga/internal/reviewapp"
)

func TestAuditAddsOnlyStaleSelectorsOwnedByTheFeatureItems(t *testing.T) {
	t.Parallel()
	item := "urn:change-saga:test:slide:flow:item:handler"
	ref := coderef.Reference{Commit: strings.Repeat("a", 40), Path: "internal/handler.go", Start: 10, End: 12, Digest: "sha256:" + strings.Repeat("b", 64)}
	review := &auditReviewSession{gaps: reviewapp.GapPage{Gaps: []reviewapp.Gap{
		{Kind: "stale", Stale: &reviewapp.StaleSelector{Target: item, Reference: ref, EvidenceFile: "handler.json", Reason: "lines changed"}},
		{Kind: "stale", Stale: &reviewapp.StaleSelector{Target: "urn:change-saga:test:slide:other:item:foreign", Reference: ref, Reason: "foreign"}},
	}, Page: reviewapp.Page{Total: 2, Returned: 2}}}
	living := auditLivingSession{report: livingapp.AuditReport{
		Feature: "urn:change-saga:test:feature:checkout", FeatureID: "checkout", Status: "pass", Complete: true, Ready: true,
		Findings: []livingapp.AuditFinding{}, Exceptions: []livingapp.AuditException{}, Risks: []livingapp.IntentionalRisk{}, Conflicts: []livingapp.AuditConflict{}, ItemTargets: []string{item},
	}}
	session := reviewAppQuerySession{session: review, livingSession: living, operation: "audit"}

	page, err := session.Living(context.Background(), livingQuery{Operation: "audit", Filters: livingapp.Filters{Feature: "checkout"}})
	if err != nil {
		t.Fatal(err)
	}
	report := page.Data.(livingapp.AuditReport)
	if report.Ready || report.Status != "findings" || report.ExitCode != 8 || len(report.Findings) != 1 {
		t.Fatalf("report = %#v", report)
	}
	finding := report.Findings[0]
	if finding.ID != item || finding.Code != "stale_or_dangling_selector" || len(finding.Related) != 2 || finding.Related[0] != ref.Location().String() || finding.Related[1] != "handler.json" {
		t.Fatalf("finding = %#v", finding)
	}
}

type auditLivingSession struct{ report livingapp.AuditReport }

func (s auditLivingSession) Snapshot() string { return "sha256:test" }
func (s auditLivingSession) Query(context.Context, livingapp.Query) (livingapp.Result, error) {
	return livingapp.Result{Data: s.report, Page: livingapp.Page{Total: 1, Returned: 1}}, nil
}

type auditReviewSession struct{ gaps reviewapp.GapPage }

func (s *auditReviewSession) Snapshot() string { return "sha256:test" }
func (s *auditReviewSession) Overview(context.Context, reviewapp.OverviewQuery) (reviewapp.Overview, error) {
	return reviewapp.Overview{}, nil
}
func (s *auditReviewSession) Children(context.Context, reviewapp.ChildrenQuery) (reviewapp.ChildrenPage, error) {
	return reviewapp.ChildrenPage{}, nil
}
func (s *auditReviewSession) ReadFragment(context.Context, reviewapp.FragmentQuery) (reviewapp.FragmentContent, error) {
	return reviewapp.FragmentContent{}, nil
}
func (s *auditReviewSession) FragmentDiffs(context.Context, reviewapp.FragmentDiffQuery) (reviewapp.FragmentDiffs, error) {
	return reviewapp.FragmentDiffs{}, nil
}
func (s *auditReviewSession) DiffOwners(context.Context, reviewapp.DiffOwnerQuery) (reviewapp.DiffOwnership, error) {
	return reviewapp.DiffOwnership{}, nil
}
func (s *auditReviewSession) Gaps(context.Context, reviewapp.GapQuery) (reviewapp.GapPage, error) {
	return s.gaps, nil
}
func (s *auditReviewSession) Mappings(context.Context, reviewapp.MappingQuery) (reviewapp.MappingPage, error) {
	return reviewapp.MappingPage{}, nil
}
func (s *auditReviewSession) Claims(context.Context, reviewapp.ClaimQuery) (reviewapp.ClaimPage, error) {
	return reviewapp.ClaimPage{}, nil
}
func (s *auditReviewSession) Verifications(context.Context, reviewapp.VerificationQuery) (reviewapp.VerificationPage, error) {
	return reviewapp.VerificationPage{}, nil
}
