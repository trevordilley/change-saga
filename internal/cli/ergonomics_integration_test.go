package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestQuerySlidePreservesAuthoringContract(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "create-query-slide", "create", "absent", "worker-node")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Query(context.Background(), []string{"slide", "--saga", root, "--repo", repo, "--target", saga.SlideTarget(sagaID, "flow")}, &output); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		OK   bool              `json:"ok"`
		Data slideQueryContent `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.OK || envelope.Data.AuthoringSnapshot != created.Snapshot || len(envelope.Data.AuthoringHeads) != 1 || envelope.Data.AuthoringConflict {
		t.Fatalf("CLI dropped authoring state: %s", output.String())
	}
	if len(envelope.Data.Items) != 1 || len(envelope.Data.Items[0].Evidence) != 1 || len(envelope.Data.Items[0].CriterionLinks) != 1 {
		t.Fatalf("CLI dropped complete Item state: %s", output.String())
	}
}

// Exercise the actual authoring format, not a hand-built transaction record:
// semantic consolidation must not retire intent still owned by a slide bundle.
func TestConsolidationRefusesRealSlideTransactionLinks(t *testing.T) {
	t.Parallel()
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, sagaID, "create-linked-slide", "create", "absent", "worker-node")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.AddStory(root, sagaID, requirements.AddStoryInput{
		Feature: testFeature, ID: "canonical", RevisionID: "r1", EventID: "proposed",
		Title: "Canonical run", Statement: "As a user I can run the service",
		Personas:           []string{personaURNFor(sagaID)},
		AcceptanceCriteria: []requirements.Criterion{{ID: "returns", Statement: "Run returns without error"}},
	}); err != nil {
		t.Fatal(err)
	}
	story := "urn:change-saga:" + sagaID + ":story:run"
	canonical := "urn:change-saga:" + sagaID + ":story:canonical"
	input := requirements.ConsolidateInput{
		Duplicate: story, Canonical: canonical, EventID: "consolidated", Parents: []string{story + ":event:proposed"}, Reason: "Duplicate intent",
		CriterionMap: map[string]string{story + ":criterion:returns": canonical + ":criterion:returns"},
	}
	path := filepath.Join(root, filepath.FromSlash(created.Path))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.ConsolidateProposal(root, sagaID, input); err == nil || !strings.Contains(err.Error(), "partial graph retirement") {
		t.Fatalf("consolidation must refuse transaction-owned links: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("refused consolidation changed slide: %v", err)
	}
	document, err := requirements.Load(root, sagaID)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range document.Stories {
		if candidate.Identity.ID == "run" && (candidate.CurrentLifecycle == nil || candidate.CurrentLifecycle.State != requirements.StateProposed) {
			t.Fatalf("refused consolidation changed story lifecycle: %#v", candidate.CurrentLifecycle)
		}
	}

	// Repointing the live transaction is the supported recovery. Its retained
	// historical link must not prevent consolidation after that update.
	request.Operation = "update"
	request.RequestID = "repoint-linked-slide"
	request.ExpectedSnapshot = created.Snapshot
	request.Items[0].CriterionLinks[0].Criterion = canonical + ":criterion:returns"
	request.Items[0].CriterionLinks[0].StoryRevision = canonical + ":revision:r1"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, request, false); err != nil {
		t.Fatal(err)
	}
	if _, err := requirements.ConsolidateProposal(root, sagaID, input); err != nil {
		t.Fatalf("consolidation after repointing current transaction: %v", err)
	}
}
