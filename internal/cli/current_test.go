package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/currentrecord"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestCurrentSlideReplacesContentWithoutHistory(t *testing.T) {
	root, repo, base, commit, sagaID := newSlideTransactionFixture(t)
	create := slideTransactionRequest(t, repo, base, commit, sagaID, "", "create", "", "node-a")
	created, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := ApplySlideTransaction(context.Background(), root, base, repo, create, false); err != nil || !replay.Unchanged {
		t.Fatalf("value retry: %+v %v", replay, err)
	}
	update := slideTransactionRequest(t, repo, base, commit, sagaID, "", "update", "", "node-b")
	updated, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Snapshot == updated.Snapshot {
		t.Fatal("changed content did not change its digest")
	}
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("current slide: %v %+v", err, validation.Issues)
	}
	slide := document.Decks[0].Slides[0]
	if slide.ID != "flow" || slide.Items[0].ID != "worker" || slide.Items[0].Selector.ElementID != "node-b" {
		t.Fatal("stable slide/Item identity changed")
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(slide.Path)))
	if err != nil {
		t.Fatal(err)
	}
	for _, obsolete := range []string{`"revisions"`, `"current"`, `"parent_snapshots"`, `"created_at"`, `"request_id"`} {
		if bytes.Contains(data, []byte(obsolete)) {
			t.Errorf("slide retained %s", obsolete)
		}
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["version"] != float64(2) || stored["content"] == nil {
		t.Fatalf("current slide layout: %s", data)
	}
	update.Operation = "reconcile"
	if _, err := ApplySlideTransaction(context.Background(), root, base, repo, update, false); err == nil {
		t.Fatal("secondary reconciliation workflow is still available")
	}
}

func TestCurrentStoryCLIRequiresNoRevisionOrParent(t *testing.T) {
	root, _, _, _, sagaID := newSlideTransactionFixture(t)
	var output bytes.Buffer
	if err := Story(context.Background(), []string{"revise", "--story", "urn:change-saga:" + sagaID + ":story:run", "--title", "Revised outcome", root}, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), ":revision:") {
		t.Fatalf("author received an internal version identity: %s", output.String())
	}
}

func TestCurrentRequirementQueryExposesContentDigest(t *testing.T) {
	root, repo, _, _, _ := newSlideTransactionFixture(t)
	var output bytes.Buffer
	if err := Query(context.Background(), []string{"requirements", "--saga", root, "--repo", repo}, &output); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Data struct {
			Requirements []struct {
				ContentDigest string         `json:"content_digest"`
				Content       map[string]any `json:"content"`
			} `json:"requirements"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Requirements) != 1 || !strings.HasPrefix(result.Data.Requirements[0].ContentDigest, "sha256:") {
		t.Fatalf("authors cannot obtain an exact current pin: %s", output.String())
	}
	for _, key := range []string{"id", "parents", "created_at", "$schema"} {
		if _, exists := result.Data.Requirements[0].Content[key]; exists {
			t.Errorf("current content exposed compatibility metadata %s", key)
		}
	}
}

func TestNormalizeCommandValidatesLegacyContentBeforeDiscardingHistory(t *testing.T) {
	root, _, _, _, sagaID := newSlideTransactionFixture(t)
	dir := filepath.Join(root, "___features", "core.feature", "___requirements", "stories", "run.story")
	record, err := currentrecord.Read(dir)
	if err != nil || record == nil {
		t.Fatalf("fixture current record: %v", err)
	}
	var content, state map[string]any
	if err := json.Unmarshal(record.Content, &content); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(record.State, &state); err != nil {
		t.Fatal(err)
	}
	content["$schema"], content["version"], content["id"], content["parents"], content["created_at"] = requirements.RevisionSchemaURL, 3, "r1", []string{}, "2026-09-01T00:00:00Z"
	content["title"] = ""
	state["$schema"], state["version"], state["id"], state["parents"], state["created_at"] = requirements.LifecycleEventSchemaURL, 3, "proposed", []string{}, "2026-09-01T00:00:00Z"
	state["story"] = "urn:change-saga:" + sagaID + ":story:run"
	data, _ := json.Marshal(content)
	stateData, _ := json.Marshal(state)
	legacy := filepath.Join(dir, "revisions", "r1.json")
	writeFile(t, legacy, string(data))
	writeFile(t, filepath.Join(dir, "events", "proposed.json"), string(stateData))
	if err := os.Remove(filepath.Join(dir, currentrecord.Name)); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Normalize(context.Background(), []string{root}, &out); err == nil || !strings.Contains(err.Error(), "title is required") {
		t.Fatalf("invalid legacy content: %v %s", err, out.String())
	}
	after, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(after, data) {
		t.Fatal("normalization discarded invalid legacy content")
	}
	if _, err := os.Stat(filepath.Join(dir, currentrecord.Name)); !os.IsNotExist(err) {
		t.Fatal("invalid current content was published")
	}
}
