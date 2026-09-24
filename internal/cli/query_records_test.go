package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/twentyideas/changesaga/internal/livingapp"
)

func TestPersonaQueriesTraverseEightStoriesWithExactIdentityAndPortablePaths(t *testing.T) {
	root, repo := coveredSaga(t)
	spacedRoot := filepath.Join(filepath.Dir(root), "batch records.saga")
	if err := os.Rename(root, spacedRoot); err != nil {
		t.Fatal(err)
	}
	root = spacedRoot
	persona := "urn:change-saga:batch:persona:" + testPersona
	for index := 1; index <= 8; index++ {
		addStory(t, root, testFeature, "persona-use-"+string(rune('a'+index-1)), persona)
	}
	mustLiving(t, "duplicate-name persona", personaCommand, "add", root, "--id", "second-user", "--name", "User", "--description", "A distinct identity")
	mustLiving(t, "retire persona", personaCommand, "set-state", root, "--persona", persona, "--event", "retired", "--parent", persona+":event:active", "--state", "retired", "--reason", "fixture")

	var read bytes.Buffer
	if err := Query(context.Background(), []string{"personas", "--saga", root, "--repo", repo, "--persona", testPersona, "--limit", "1"}, &read); err != nil {
		t.Fatalf("persona query: %v\n%s", err, read.String())
	}
	var personaEnvelope struct {
		Snapshot string                `json:"snapshot"`
		Data     livingapp.PersonaPage `json:"data"`
		Page     queryPageEnvelope     `json:"page"`
	}
	decodeOneJSONValue(t, read.Bytes(), &personaEnvelope)
	if personaEnvelope.Snapshot == "" || personaEnvelope.Page.Total != 1 || len(personaEnvelope.Data.Personas) != 1 {
		t.Fatalf("persona envelope = %#v", personaEnvelope)
	}
	selected := personaEnvelope.Data.Personas[0]
	if selected.Persona != persona || selected.ID != testPersona || selected.State != "retired" || selected.CurrentRevision == nil || selected.CurrentRevision.Name != "User" || selected.CurrentRevision.Description != "Someone who uses the app" {
		t.Fatalf("selected persona = %#v", selected)
	}
	other := queryData(t, "personas", "--saga", root, "--repo", repo, "--persona", "second-user")["personas"].([]any)[0].(map[string]any)
	if other["persona"] == selected.Persona || other["id"] != "second-user" || other["current_revision"].(map[string]any)["name"] != "User" {
		t.Fatalf("duplicate display names were not identity-selected: %#v", other)
	}

	var cursor string
	var references []livingapp.RecordReference
	pages := 0
	for {
		args := []string{"persona-references", "--saga", root, "--repo", repo, "--persona", testPersona, "--limit", "3"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		var output bytes.Buffer
		if err := Query(context.Background(), args, &output); err != nil {
			t.Fatalf("reference page %d: %v\n%s", pages+1, err, output.String())
		}
		var envelope struct {
			Data livingapp.ReferencePage `json:"data"`
			Page queryPageEnvelope       `json:"page"`
		}
		decodeOneJSONValue(t, output.Bytes(), &envelope)
		pages++
		if envelope.Data.Counts.Total != 8 || envelope.Data.Counts.Incoming != 8 || envelope.Data.Counts.Outgoing != 0 || envelope.Page.Total != 8 || envelope.Page.Returned != len(envelope.Data.References) {
			t.Fatalf("reference counts = %#v page=%#v", envelope.Data.Counts, envelope.Page)
		}
		references = append(references, envelope.Data.References...)
		if !envelope.Page.HasMore {
			break
		}
		cursor = *envelope.Page.NextCursor
	}
	if pages != 3 || len(references) != 8 {
		t.Fatalf("pages=%d references=%#v", pages, references)
	}
	for _, reference := range references {
		if reference.Kind != "story_persona" || reference.Direction != "incoming" || reference.Owner == "" || reference.Selector == "" || reference.Provenance.Class != "typed_field" {
			t.Fatalf("reference provenance = %#v", reference)
		}
	}

	bad, status, _ := runRealQuery(t, []string{"persona-references", "--saga", root, "--repo", repo, "--persona", testPersona, "--limit", "3", "--cursor", cursor + "A"})
	if status == 0 || bad.Error == nil || bad.Error.Code != "invalid_argument" {
		t.Fatalf("tampered cursor status=%d envelope=%#v", status, bad)
	}
	second := "urn:change-saga:batch:persona:second-user"
	mustLiving(t, "revise another persona", personaCommand, "revise", root, "--persona", second, "--revision", "r2", "--parent", second+":revision:r1", "--name", "User", "--description", "A changed distinct identity")
	stale, status, _ := runRealQuery(t, []string{"persona-references", "--saga", root, "--repo", repo, "--persona", testPersona, "--limit", "3", "--cursor", cursor})
	if status == 0 || stale.Error == nil || stale.Error.Code != "stale_snapshot" || !stale.Error.Retryable {
		t.Fatalf("stale cursor status=%d envelope=%#v", status, stale)
	}
}

func TestTermsBoundedModePreservesLegacyCompletenessAndReferenceDetail(t *testing.T) {
	repo, _ := sourceRepo(t, map[string]string{"terms.go": "package terms\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	persona := "urn:change-saga:atomic:persona:" + testPersona
	for _, args := range [][]string{
		{"add", "--id", "basket", "--name", "Basket", "--definition", "Selected goods", "--story", "sit-assessment", "--record", persona, root},
		{"add", "--id", "cart", "--name", "Basket", "--definition", "A distinct identity", "--record", "urn:change-saga:atomic:term:basket", root},
		{"add", "--id", "checkout", "--name", "Checkout", "--definition", "Payment flow", root},
	} {
		if err := Term(context.Background(), args, &bytes.Buffer{}); err != nil {
			t.Fatalf("term %v: %v", args, err)
		}
	}

	var legacy bytes.Buffer
	if err := Query(context.Background(), []string{"terms", "--saga", root, "--repo", repo}, &legacy); err != nil {
		t.Fatal(err)
	}
	var legacyEnvelope struct {
		Data termsQuery         `json:"data"`
		Page *queryPageEnvelope `json:"page,omitempty"`
	}
	decodeOneJSONValue(t, legacy.Bytes(), &legacyEnvelope)
	if legacyEnvelope.Page == nil || legacyEnvelope.Page.Total != 1 || legacyEnvelope.Page.Returned != 1 || legacyEnvelope.Page.HasMore || legacyEnvelope.Page.NextCursor != nil || len(legacyEnvelope.Data.Terms) != 3 {
		t.Fatalf("legacy terms response changed shape or completeness: %#v", legacyEnvelope)
	}

	var cursor string
	var ids []string
	for {
		args := []string{"terms", "--saga", root, "--repo", repo, "--limit", "1"}
		if cursor != "" {
			args = append(args, "--cursor", cursor)
		}
		var output bytes.Buffer
		if err := Query(context.Background(), args, &output); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data termsQuery        `json:"data"`
			Page queryPageEnvelope `json:"page"`
		}
		decodeOneJSONValue(t, output.Bytes(), &envelope)
		if envelope.Page.Total != 3 || envelope.Page.Returned != 1 || len(envelope.Data.Terms) != 1 {
			t.Fatalf("bounded terms page = %#v", envelope)
		}
		ids = append(ids, envelope.Data.Terms[0].ID)
		if !envelope.Page.HasMore {
			break
		}
		cursor = *envelope.Page.NextCursor
	}
	if got := ids; len(got) != 3 || got[0] != "basket" || got[1] != "cart" || got[2] != "checkout" {
		t.Fatalf("bounded term order = %v", got)
	}
	cross, status, _ := runRealQuery(t, []string{"term-references", "--saga", root, "--repo", repo, "--term", "basket", "--limit", "1", "--cursor", cursor})
	if status == 0 || cross.Error == nil || cross.Error.Code != "invalid_argument" {
		t.Fatalf("cross-operation cursor status=%d envelope=%#v", status, cross)
	}

	var output bytes.Buffer
	if err := Query(context.Background(), []string{"term-references", "--saga", root, "--repo", repo, "--term", "basket", "--limit", "10"}, &output); err != nil {
		t.Fatalf("term references: %v\n%s", err, output.String())
	}
	var references struct {
		Data livingapp.ReferencePage `json:"data"`
		Page queryPageEnvelope       `json:"page"`
	}
	decodeOneJSONValue(t, output.Bytes(), &references)
	if references.Data.Counts.Total != 3 || references.Data.Counts.Incoming != 1 || references.Data.Counts.Outgoing != 2 || references.Page.Total != 3 {
		t.Fatalf("term references = %#v", references)
	}
	exact := queryData(t, "terms", "--saga", root, "--repo", repo, "--term", "cart")["terms"].([]any)[0].(map[string]any)
	if exact["id"] != "cart" || exact["name"] != "Basket" || exact["definition"] != "A distinct identity" {
		t.Fatalf("exact duplicate-name term = %#v", exact)
	}

	for _, args := range [][]string{
		{"terms", "--saga", root, "--repo", repo, "--term", "urn:not-a-term"},
		{"terms", "--saga", root, "--repo", repo, "--term", "urn:change-saga:foreign:term:basket"},
		{"terms", "--saga", root, "--repo", repo, "--term", "missing"},
	} {
		envelope, status, _ := runRealQuery(t, args)
		if status == 0 || envelope.Error == nil {
			t.Fatalf("selector %v was accepted: %#v", args, envelope)
		}
	}
	if err := Term(context.Background(), []string{"add", "--id", "later", "--name", "Later", "--definition", "Changes the snapshot", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	stale, status, _ := runRealQuery(t, []string{"terms", "--saga", root, "--repo", repo, "--limit", "1", "--cursor", cursor})
	if status == 0 || stale.Error == nil || stale.Error.Code != "stale_snapshot" || !stale.Error.Retryable {
		t.Fatalf("stale terms cursor status=%d envelope=%#v", status, stale)
	}
}

func TestQuerySchemaAndSpecDiscoverRecordOperations(t *testing.T) {
	for _, operation := range []string{"personas", "persona-references", "terms", "term-references"} {
		var output bytes.Buffer
		if err := Query(context.Background(), []string{"schema", operation}, &output); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Data querySchemaDescription `json:"data"`
		}
		decodeOneJSONValue(t, output.Bytes(), &envelope)
		if envelope.Data.Operation != operation || envelope.Data.Usage == "" || envelope.Data.Pagination.Kind != "cursor" {
			t.Fatalf("schema %s = %#v", operation, envelope.Data)
		}
	}
	var output bytes.Buffer
	if err := Spec([]string{"--json"}, &output); err != nil {
		t.Fatal(err)
	}
	var spec map[string]json.RawMessage
	decodeOneJSONValue(t, output.Bytes(), &spec)
	var query struct {
		Schema     string                   `json:"schema"`
		Operations []querySchemaDescription `json:"operations"`
	}
	if err := json.Unmarshal(spec["query"], &query); err != nil || query.Schema != querySchema {
		t.Fatalf("spec query contract: %v %#v", err, query)
	}
	found := map[string]bool{}
	for _, operation := range query.Operations {
		found[operation.Operation] = true
	}
	if !found["personas"] || !found["persona-references"] || !found["term-references"] {
		t.Fatalf("spec query operations = %#v", found)
	}
}
