package livingapp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/planstore"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

var fixtureTime = time.Date(2026, 8, 31, 18, 0, 0, 0, time.UTC)

const (
	fixtureFeature   = "main"
	fixturePersonaID = "shopper"
)

func fixturePersona() string {
	urn, _ := requirements.PersonaURN("test", fixturePersonaID)
	return urn
}

func TestOrdinarySagaIsNotApplicableRatherThanInvalidOrBlocked(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ordinary.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"version": 5, "id": "ordinary", "title": "Ordinary", "source": map[string]string{"repository": "https://example.com/repo.git"}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "saga.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	session := openFixture(t, root)
	requirementsResult, err := session.Query(context.Background(), Query{Operation: "requirements"})
	if err != nil || requirementsResult.Page.Total != 0 || len(requirementsResult.Data.(RequirementPage).Requirements) != 0 {
		t.Fatalf("ordinary requirements = %+v, %v", requirementsResult, err)
	}
	readinessResult, err := session.Query(context.Background(), Query{Operation: "readiness"})
	if err != nil {
		t.Fatal(err)
	}
	readiness := readinessResult.Data.(ReadinessPage)
	if readiness.Summary.Status != "not_applicable" || readiness.Summary.PeerReviewReady || readinessResult.Page.Total != 0 {
		t.Fatalf("ordinary readiness = %+v page=%+v", readiness, readinessResult.Page)
	}
}

func TestLivingQueriesAreDeterministicSnapshotBoundAndProgressIsNotProof(t *testing.T) {
	root := livingFixture(t)
	addAcceptedStory(t, root, "checkout", "fast")
	addAcceptedStory(t, root, "refund", "clear")
	createDeliveryDesign(t, root, "checkout", "fast")
	addPlanTask(t, root, "checkout")
	finishPlanTask(t, root, "checkout")

	session := openFixture(t, root)
	first, err := session.Query(context.Background(), Query{Operation: "requirements", Limit: 1})
	if err != nil || first.Page.Total != 2 || first.Page.NextCursor == nil {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, err := session.Query(context.Background(), Query{Operation: "requirements", Limit: 1, Cursor: *first.Page.NextCursor})
	if err != nil || second.Page.Returned != 1 || second.Page.HasMore {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	tampered := *first.Page.NextCursor + "A"
	_, err = session.Query(context.Background(), Query{Operation: "requirements", Limit: 1, Cursor: tampered})
	if !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("tampered cursor error = %#v", err)
	}
	_, err = session.Query(context.Background(), Query{Operation: "relations", Limit: 1, Cursor: *first.Page.NextCursor})
	if !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("cross-operation cursor error = %#v", err)
	}
	_, err = session.Query(context.Background(), Query{Operation: "requirements", Filters: Filters{State: "accepted"}, Limit: 1, Cursor: *first.Page.NextCursor})
	if !errorHasCode(err, CodeInvalidArgument) {
		t.Fatalf("changed-filter cursor error = %#v", err)
	}
	traceResult, err := session.Query(context.Background(), Query{Operation: "traceability"})
	if err != nil {
		t.Fatal(err)
	}
	traces := traceResult.Data.(TraceabilityPage).Criteria
	var checkout Traceability
	for _, trace := range traces {
		if strings.HasSuffix(trace.Criterion, ":fast") {
			checkout = trace
		}
	}
	if checkout.Delivered || !hasBlocker(checkout.Blockers, "immutable_evidence_missing") {
		t.Fatalf("done/planned work was treated as proof: %+v", checkout)
	}

	verification := addVerifiedVerification(t, root, "checkout", "fast")
	reopened := openFixture(t, root)
	_, err = reopened.Query(context.Background(), Query{Operation: "requirements", Limit: 1, Cursor: *first.Page.NextCursor})
	var domainErr *Error
	if !errors.As(err, &domainErr) || domainErr.Code != CodeStaleSnapshot || !domainErr.Retryable {
		t.Fatalf("old cursor error = %#v", err)
	}
	traceResult, err = reopened.Query(context.Background(), Query{Operation: "traceability", Filters: Filters{Requirement: "checkout"}})
	if err != nil {
		t.Fatal(err)
	}
	checkout = traceResult.Data.(TraceabilityPage).Criteria[0]
	if !checkout.Delivered || len(checkout.Evidence) != 1 || checkout.Evidence[0] != verification {
		t.Fatalf("verified immutable evidence did not complete the path: %+v", checkout)
	}
}

func TestTaskNamingAStoryPlansItsCriteria(t *testing.T) {
	root := livingFixture(t)
	addAcceptedStory(t, root, "checkout", "fast")
	addAcceptedStory(t, root, "refund", "clear")

	readinessOf := func() ReadinessPage {
		t.Helper()
		result, err := openFixture(t, root).Query(context.Background(), Query{Operation: "readiness"})
		if err != nil {
			t.Fatal(err)
		}
		return result.Data.(ReadinessPage)
	}
	if covered := readinessOf().Summary.PlanCoverage.Covered; covered != 0 {
		t.Fatalf("plan coverage before any Task = %d", covered)
	}

	task := addPlanTask(t, root, "checkout")
	if covered := readinessOf().Summary.PlanCoverage.Covered; covered != 1 {
		t.Fatalf("plan coverage after a Task named the story = %d, want 1", covered)
	}

	result, err := openFixture(t, root).Query(context.Background(), Query{Operation: "traceability"})
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range result.Data.(TraceabilityPage).Criteria {
		planned := strings.HasSuffix(trace.Criterion, ":fast")
		if planned == hasBlocker(trace.Blockers, "task_missing") {
			t.Fatalf("task_missing blocker on %s: %+v", trace.Criterion, trace.Blockers)
		}
		if !planned {
			if len(trace.Tasks) != 0 {
				t.Fatalf("a story no Task names was planned: %+v", trace)
			}
			continue
		}
		if len(trace.Tasks) != 1 || trace.Tasks[0] != task {
			t.Fatalf("tasks = %v, want %s", trace.Tasks, task)
		}
		found := false
		for _, path := range trace.Paths {
			found = found || (len(path) == 3 && path[2] == task)
		}
		if !found {
			t.Fatalf("paths %v do not reach the Task", trace.Paths)
		}
	}
}

func TestMissingCrossDomainEndpointsMakeRelationsAndPathsStale(t *testing.T) {
	root := livingFixture(t)
	addAcceptedStory(t, root, "checkout", "fast")
	createDeliveryDesign(t, root, "checkout", "fast")
	if err := os.RemoveAll(filepath.Join(applayout.FeatureDir(root, fixtureFeature), "___design", "checkout-design.fragment")); err != nil {
		t.Fatal(err)
	}
	session := openFixture(t, root)
	result, err := session.Query(context.Background(), Query{Operation: "relations", Filters: Filters{State: "stale"}})
	if err != nil {
		t.Fatal(err)
	}
	relations := result.Data.(RelationPage).Relations
	if len(relations) != 1 {
		t.Fatalf("stale relations = %+v", relations)
	}
	traceResult, err := session.Query(context.Background(), Query{Operation: "traceability"})
	if err != nil {
		t.Fatal(err)
	}
	trace := traceResult.Data.(TraceabilityPage).Criteria[0]
	if len(trace.Design) != 0 || !hasBlocker(trace.Blockers, "design_missing") {
		t.Fatalf("stale design still participated in traceability: %+v", trace)
	}
}

func TestDesignContentChangeStalesPinnedRelations(t *testing.T) {
	root := livingFixture(t)
	addAcceptedStory(t, root, "checkout", "fast")
	createDeliveryDesign(t, root, "checkout", "fast")
	path := filepath.Join(applayout.FeatureDir(root, fixtureFeature), "___design", "checkout-design.fragment", "content.txt")
	if err := os.WriteFile(path, []byte("revised design\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := openFixture(t, root).Query(context.Background(), Query{Operation: "relations", Filters: Filters{State: "stale"}})
	if err != nil {
		t.Fatal(err)
	}
	relations := result.Data.(RelationPage).Relations
	if len(relations) != 1 {
		t.Fatalf("stale relations = %+v", relations)
	}
	for _, relation := range relations {
		if !strings.Contains(strings.Join(relation.StaleReasons, ","), "content digest changed") {
			t.Fatalf("relation did not report its stale design pin: %+v", relation)
		}
	}
}

func livingFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "test.saga")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"$schema": "https://changesaga.dev/schema/v5/saga.schema.json", "version": 5, "id": "test", "title": "Test", "source": map[string]string{"repository": "https://example.com/repo.git"}}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "saga.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applayout.WriteFeature(root, applayout.FeatureManifest{ID: fixtureFeature, Title: "Main", CreatedAt: fixtureTime}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"___requirements", "___design"} {
		if err := os.Mkdir(filepath.Join(applayout.FeatureDir(root, fixtureFeature), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := requirements.AddPersona(root, "test", requirements.AddPersonaInput{ID: fixturePersonaID, RevisionID: "r1", EventID: "active", Name: "Shopper", Description: "Buys things", CreatedAt: fixtureTime, RequestID: "persona-shopper"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func addAcceptedStory(t *testing.T, root, id, criterion string) {
	t.Helper()
	if _, err := requirements.AddStory(root, "test", requirements.AddStoryInput{Feature: fixtureFeature, Personas: []string{fixturePersona()}, ID: id, RevisionID: "r1", EventID: "proposed", Title: id, Statement: "Deliver " + id, Priority: "high", Citations: []string{}, AcceptanceCriteria: []requirements.Criterion{{ID: criterion, Statement: "Criterion " + criterion}}, CreatedAt: fixtureTime, RequestID: "story-" + id}); err != nil {
		t.Fatal(err)
	}
	story, _ := livingid.Story("test", id)
	parent, _ := requirements.StoryEventURN("test", id, "proposed")
	if _, err := requirements.SetStoryState(root, "test", requirements.SetStoryStateInput{Story: story, ID: "accepted", Parents: []string{parent}, State: requirements.StateAccepted, CreatedAt: fixtureTime.Add(time.Minute), RequestID: "accept-" + id}); err != nil {
		t.Fatal(err)
	}
}

func createDeliveryDesign(t *testing.T, root, storyID, criterionID string) {
	t.Helper()
	criterion, _ := livingid.Criterion("test", storyID, criterionID)
	revision, _ := livingid.Revision("test", storyID, "r1")
	design, _ := livingid.Design("test", livingid.DesignFragment, storyID+"-design")
	designDir := filepath.Join(applayout.FeatureDir(root, fixtureFeature), "___design", storyID+"-design.fragment")
	if err := os.MkdirAll(designDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"version": 2, "id": storyID + "-design", "media_type": "text/plain", "entrypoint": "content.txt"}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(designDir, "fragment.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(designDir, "content.txt"), []byte("current design\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load design: valid=%v err=%v issues=%+v", validation.Valid, err, validation.Issues)
	}
	digest, exists, err := saga.CurrentDesignContentDigest(document, design)
	if err != nil || !exists {
		t.Fatalf("design digest: exists=%v err=%v", exists, err)
	}
	if _, err := requirements.AddRelation(root, "test", requirements.AddRelationInput{Feature: fixtureFeature, ID: "addresses-" + storyID, Type: requirements.RelationAddresses, From: design, To: criterion, Rationale: "current design", FromContentDigest: digest, ToRevision: revision, CreatedAt: fixtureTime, RequestID: "addresses-" + storyID}); err != nil {
		t.Fatal(err)
	}
}

// addPlanTask adds a Plan with one Task naming the story, and returns the Task's URN.
func addPlanTask(t *testing.T, root, storyID string) string {
	t.Helper()
	deck := saga.DeckManifest{ID: storyID, Title: "Deliver " + storyID, Objective: "Deliver " + storyID}
	if err := planstore.Create(root, saga.PlanManifest{ID: storyID, Title: "Deliver " + storyID, CreatedAt: fixtureTime}, deck); err != nil {
		t.Fatal(err)
	}
	story, _ := livingid.Story("test", storyID)
	task, err := planstore.AddTask(root, storyID, saga.TaskManifest{ID: storyID, Title: "Deliver " + storyID, Story: story, CreatedAt: fixtureTime}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func finishPlanTask(t *testing.T, root, storyID string) {
	t.Helper()
	err := planstore.UpdateTask(root, storyID, storyID, func(task *saga.TaskManifest) error {
		task.State = saga.TaskDone
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// addVerifiedVerification records a claim, its verified result, and a verifies
// relation to the criterion, and returns the verification's URN.
func addVerifiedVerification(t *testing.T, root, storyID, criterionID string) string {
	t.Helper()
	id := storyID + "-verified"
	writeRecord := func(dir string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, dir, id+".json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	design, _ := livingid.Design("test", livingid.DesignFragment, storyID+"-design")
	writeRecord("___claims", saga.Claim{Version: saga.CurrentVersion, ID: id, Target: design, Kind: "behavior", Statement: "It works", CreatedAt: fixtureTime,
		Evidence: []coderef.Reference{{Commit: strings.Repeat("a", 40), Path: "checkout.go", Start: 1, End: 2, Digest: "sha256:" + strings.Repeat("b", 64)}}})
	writeRecord("___verifications", saga.Verification{Version: saga.CurrentVersion, ID: id, Claim: id, Status: "verified", Method: "test", Summary: "It passed", CreatedAt: fixtureTime})
	criterion, _ := livingid.Criterion("test", storyID, criterionID)
	revision, _ := livingid.Revision("test", storyID, "r1")
	verification := "urn:change-saga:test:verification:" + id
	if _, err := requirements.AddRelation(root, "test", requirements.AddRelationInput{Feature: fixtureFeature, ID: "verifies-" + storyID, Type: requirements.RelationVerifies, From: verification, To: criterion, Rationale: "verified", ToRevision: revision, CreatedAt: fixtureTime, RequestID: "verifies-" + storyID}); err != nil {
		t.Fatal(err)
	}
	return verification
}

func openFixture(t *testing.T, root string) Session {
	t.Helper()
	session, err := Open(context.Background(), OpenOptions{SagaRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	return session
}
func hasBlocker(values []readiness.Blocker, code string) bool {
	for _, value := range values {
		if value.Code == code {
			return true
		}
	}
	return false
}

func errorHasCode(err error, code ErrorCode) bool {
	var domainErr *Error
	return errors.As(err, &domainErr) && domainErr.Code == code
}
