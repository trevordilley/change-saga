package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/deprecationview"
	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/store"
)

func deprecationFixture(t *testing.T) (string, string, string) {
	t.Helper()
	repo, commit := sourceRepo(t, map[string]string{"old.go": "package old\nfunc Legacy() {}\nfunc Current() {}\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	if err := Overview(context.Background(), []string{"set-description", "--text", "The old API is retained for compatibility.", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return root, repo, commit
}
func deprecationRun(t *testing.T, root, repo string, args ...string) {
	t.Helper()
	args = append(args, "--json")
	if args[0] != "set-state" {
		args = append(args, "--repo", repo)
	}
	args = append(args, root)
	var out bytes.Buffer
	if err := Deprecation(context.Background(), args, &out); err != nil {
		t.Fatalf("deprecation %v: %v %s", args, err, out.String())
	}
}
func deprecationQuery(t *testing.T, root, repo string, args ...string) []deprecationview.Record {
	t.Helper()
	var out bytes.Buffer
	args = append([]string{"deprecations", "--saga", root, "--repo", repo}, args...)
	if err := Query(context.Background(), args, &out); err != nil {
		t.Fatalf("query: %v %s", err, out.String())
	}
	var result struct {
		Data struct {
			Rows []deprecationview.Record `json:"deprecations"`
		}
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Data.Rows
}

const depPrefix = "urn:change-saga:atomic:"
const depDescription = depPrefix + "fragment:atomic-description"

func TestDeprecationPublicScopeHistoryAndStoryRelations(t *testing.T) {
	root, repo, commit := deprecationFixture(t)
	ctx := context.Background()
	for _, scope := range []string{"content", "code"} {
		deprecationRun(t, root, repo, "add", "--id", scope, "--target", depDescription, "--scope", scope, "--reason", "Use the supported API.", "--ref", "HEAD:old.go#L2", "--request-id", scope)
		deprecationRun(t, root, repo, "add", "--id", scope, "--target", depDescription, "--scope", scope, "--reason", "Use the supported API.", "--ref", "HEAD:old.go#L2", "--request-id", scope)
	}
	rows := deprecationQuery(t, root, repo, "--ref", "HEAD:old.go#L2")
	if len(rows) != 2 {
		t.Fatalf("rows %v", rows)
	}
	for _, row := range rows {
		a := row.Assertions[0]
		if a.Code[0].Reference.Commit != commit || a.Code[0].Match != "exact" || a.TargetHealth.Status != "current" {
			t.Fatalf("evidence %#v", a)
		}
		expected := "related_code"
		if row.CurrentRevision.Scope == "code" {
			expected = "deprecated_code"
		}
		if a.Code[0].Role != expected {
			t.Fatalf("scope was inferred: %v", a.Code)
		}
	}
	if got := deprecationQuery(t, root, repo, "--ref", "HEAD:old.go#L3"); len(got) != 0 {
		t.Fatalf("widened code scope: %v", got)
	}
	subject := depPrefix + "deprecation:code"
	var out bytes.Buffer
	if err := Relation(ctx, []string{"add", "--feature", testFeature, "--id", "deprecation-story", "--type", "explains", "--from", subject, "--to", depPrefix + "story:sit-assessment", "--rationale", "Migration context", root}, &out); err != nil {
		t.Fatalf("relation add: %v %s", err, out.String())
	}
	if got := deprecationQuery(t, root, repo, "--story", "sit-assessment"); len(got) != 1 || got[0].Stories[0].Currency.Status != "current" {
		t.Fatalf("story traversal: %v", got)
	}
	relationData := queryData(t, "relations", "--saga", root, "--repo", repo, "--from", subject)
	if len(relationData["relations"].([]any)) != 1 {
		t.Fatalf("normal relation discovery: %v", relationData)
	}
	if err := Story(ctx, []string{"set-state", "--story", depPrefix + "story:sit-assessment", "--event", "deferred", "--parent", depPrefix + "story:sit-assessment:event:proposed", "--state", "deferred", "--reason", "Historical migration", root}, &out); err != nil {
		t.Fatal(err)
	}
	if err := Story(ctx, []string{"set-state", "--story", depPrefix + "story:sit-assessment", "--event", "retired", "--parent", depPrefix + "story:sit-assessment:event:deferred", "--state", "retired", "--reason", "Historical migration", root}, &out); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo, "--story", "sit-assessment"); got[0].Stories[0].Lifecycle != "retired" {
		t.Fatalf("retired story hidden: %v", got)
	}
	story := depPrefix + "story:sit-assessment"
	if err := Story(ctx, []string{"revise", "--story", story, "--revision", "changed", "--parent", story + ":revision:r1", "--title", "Changed historical context", root}, &out); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo, "--story", "sit-assessment"); got[0].Stories[0].Currency.Status == "current" || got[0].Stories[0].Relation.ToRevision != story+":revision:r1" {
		t.Fatalf("changed story lost its historical pin: %v", got)
	}
	before, err := os.ReadFile(filepath.Join(root, requirements.DeprecationPackagePath("code"), "revisions/r1.json"))
	if err != nil {
		t.Fatal(err)
	}
	deprecationRun(t, root, repo, "set-state", "--deprecation", subject, "--event", "a-withdrawn", "--parent", subject+":event:active", "--state", "withdrawn", "--reason", "Mistaken assertion; the code remains supported.")
	deprecationRun(t, root, repo, "add", "--id", "code", "--target", depDescription, "--scope", "code", "--reason", "Use the supported API.", "--ref", "HEAD:old.go#L2", "--request-id", "code")
	history := deprecationQuery(t, root, repo, "--deprecation", "code", "--history")[0]
	if history.State != "withdrawn" || len(history.History.Events) != 2 || len(history.History.Revisions) != 1 || len(history.Stories) != 1 {
		t.Fatalf("history lost: %v", history)
	}
	after, _ := os.ReadFile(filepath.Join(root, requirements.DeprecationPackagePath("code"), "revisions/r1.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("withdrawal rewrote assertion")
	}
	assertValid(t, root)
}

func TestDeprecationStaleMissingConflictAndPagination(t *testing.T) {
	root, repo, commit := deprecationFixture(t)
	subject := depPrefix + "deprecation:legacy"
	deprecationRun(t, root, repo, "add", "--id", "legacy", "--target", depDescription, "--scope", "code", "--reason", "Use the replacement.", "--ref", "HEAD:old.go#L2")
	writeFile(t, filepath.Join(repo, "old.go"), "package old\nfunc Changed() {}\nfunc Current() {}\n")
	git(t, repo, "commit", "-am", "change deprecated line")
	rows := deprecationQuery(t, root, repo, "--ref", "HEAD:old.go#L2")
	if len(rows) != 1 || rows[0].Assertions[0].Code[0].Match != "uncertain_path" || rows[0].Assertions[0].Code[0].Resolution.Current() {
		t.Fatalf("stale evidence disappeared or asserted current: %v", rows)
	}
	var references bytes.Buffer
	if err := References(context.Background(), []string{"--repo", repo, "--against", commit, "--json", root}, &references); err != nil {
		t.Fatal(err)
	}
	var health referencesOutput
	if err := json.Unmarshal(references.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, reference := range health.References {
		if reference.Kind == "deprecation" {
			found = true
			if reference.State != "stale" || reference.Accept != nil {
				t.Fatalf("old-side reference concealed a stale deprecation: %#v", reference)
			}
		}
	}
	if !found {
		t.Fatal("normal references omitted active deprecation evidence")
	}
	if err := Overview(context.Background(), []string{"set-description", "--text", "Changed explanation.", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo); got[0].Assertions[0].TargetHealth.Status != "stale" {
		t.Fatalf("target edit not stale: %v", got)
	}
	// Simulate two valid branches merged together: never choose a winner.
	doc, err := requirements.Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	original := doc.FindDeprecation("legacy").Revisions[0]
	for _, id := range []string{"branch-a", "branch-b"} {
		r := original
		r.ID = id
		r.Parents = []string{subject + ":revision:r1"}
		r.Reason = id
		if err := store.WriteJSON(filepath.Join(root, requirements.DeprecationPackagePath("legacy"), "revisions", id+".json"), r, true); err != nil {
			t.Fatal(err)
		}
	}
	conflict := deprecationQuery(t, root, repo, "--target", depDescription)[0]
	if conflict.State != "conflicted" || conflict.CurrentRevision != nil || len(conflict.Assertions) != 2 {
		t.Fatalf("conflict fabricated a winner: %#v", conflict)
	}
	if err := Deprecation(context.Background(), []string{"revise", "--deprecation", subject, "--revision", "bad", "--parent", subject + ":revision:branch-a", "--scope", "content", "--target", depDescription, "--reason", "partial heads", root}, &bytes.Buffer{}); err == nil {
		t.Fatal("accepted partial heads")
	}
	deprecationRun(t, root, repo, "revise", "--deprecation", subject, "--revision", "reconciled", "--parent", subject+":revision:branch-a", "--parent", subject+":revision:branch-b", "--target", depDescription, "--scope", "content", "--reason", "Corrected: only the explanation is deprecated.")
	if got := deprecationQuery(t, root, repo); got[0].CurrentRevision.Scope != "content" || got[0].Assertions[0].TargetHealth.Status != "current" {
		t.Fatalf("reconcile: %v", got)
	}
	deprecationRun(t, root, repo, "add", "--id", "second", "--target", depDescription, "--scope", "content", "--reason", "Second annotation")
	var out bytes.Buffer
	args := []string{"deprecations", "--saga", root, "--repo", repo, "--limit", "1"}
	if err := Query(context.Background(), args, &out); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Snapshot string            `json:"snapshot"`
		Page     queryPageEnvelope `json:"page"`
	}
	json.Unmarshal(out.Bytes(), &page)
	if page.Page.NextCursor == nil {
		t.Fatal("missing cursor")
	}
	out.Reset()
	if err := Query(context.Background(), append(args, "--cursor", *page.Page.NextCursor), &out); err != nil {
		t.Fatalf("next page: %v %s", err, out.String())
	}
	deprecationRun(t, root, repo, "set-state", "--deprecation", subject, "--event", "withdraw", "--parent", subject+":event:active", "--state", "withdrawn", "--reason", "Correction")
	out.Reset()
	if err := Query(context.Background(), append(args, "--cursor", *page.Page.NextCursor), &out); err == nil {
		t.Fatal("cursor survived metadata change")
	}
	if err := RemoveFragment(context.Background(), []string{"--target", depDescription, root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo, "--target", depDescription); len(got) != 2 || got[0].Assertions[0].TargetHealth.Status != "missing" {
		t.Fatalf("missing target lost assertion: %v", got)
	}
	assertValid(t, root)
}

func TestDeprecationWhiteboardAppearanceAndValidation(t *testing.T) {
	root, repo, _ := deprecationFixture(t)
	ctx := context.Background()
	board := diagram.Document{Version: 1, Width: 600, Height: 400, Elements: []diagram.Element{{ID: "old", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 100, Style: "normal", Label: "Old service"}}}
	if _, err := requirements.PublishWhiteboard(ctx, root, "atomic", "systems", "b1", "", "Systems", board); err != nil {
		t.Fatal(err)
	}
	target := depPrefix + "whiteboard:systems"
	deprecationRun(t, root, repo, "add", "--id", "board", "--target", target, "--element", "old", "--scope", "content", "--reason", "This appearance is obsolete.")
	row := deprecationQuery(t, root, repo, "--target", target)[0]
	if row.CurrentRevision.Target.Revision != target+":revision:b1" || row.CurrentRevision.Target.Element != "old" {
		t.Fatalf("appearance not pinned: %v", row)
	}
	for _, args := range [][]string{
		{"--scope", "code", "--target", target},
		{"--scope", "content", "--target", target, "--element", "missing"},
		{"--scope", "content", "--target", depDescription, "--element", "old"},
		{"--scope", "content", "--target", target, "--replacement", target},
	} {
		if err := Deprecation(ctx, append(append([]string{"add", "--id", "bad", "--reason", "Bad"}, args...), root), &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	board.Elements[0].Label = "Updated service"
	if _, err := requirements.PublishWhiteboard(ctx, root, "atomic", "systems", "b2", "b1", "Systems", board); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo, "--target", target); got[0].Assertions[0].TargetHealth.Status != "stale" {
		t.Fatalf("board revision not stale: %v", got)
	}
	assertValid(t, root)
	// Unknown JSON fields and invalid lifecycle are rejected by normal validation.
	path := filepath.Join(root, requirements.DeprecationPackagePath("board"), "events/active.json")
	data, _ := os.ReadFile(path)
	data = bytes.Replace(data, []byte(`"state": "active"`), []byte(`"state": "erased"`), 1)
	os.WriteFile(path, data, 0600)
	var out bytes.Buffer
	if err := Validate(ctx, []string{"--json", root}, &out); err == nil || !strings.Contains(out.String(), "active or withdrawn") {
		t.Fatalf("normal validation ignored deprecations: %v %s", err, out.String())
	}
}

func TestDeprecationRelatedTargetEvidenceAndOptionalReplacement(t *testing.T) {
	root, repo, _ := deprecationFixture(t)
	ctx := context.Background()
	// Evidence directly attached to a deprecated explanation is discoverable but
	// cannot silently become an assertion that its implementation is deprecated.
	if err := Cover(ctx, []string{"--target", depDescription, "--ref", "HEAD:old.go#L3", "--repo", repo, "--note", "Explains the current implementation", root}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	board := diagram.Document{Version: 1, Width: 400, Height: 300, Elements: []diagram.Element{{ID: "new", Kind: "node", Shape: "rect", X: 20, Y: 20, Width: 200, Height: 100, Style: "normal", Label: "New"}}}
	if _, err := requirements.PublishWhiteboard(ctx, root, "atomic", "successor", "b1", "", "Successor", board); err != nil {
		t.Fatal(err)
	}
	replacement := depPrefix + "whiteboard:successor"
	deprecationRun(t, root, repo, "add", "--id", "legacy", "--target", depDescription, "--scope", "code", "--reason", "Replace the legacy function", "--ref", "HEAD:old.go#L2", "--replacement", replacement)
	rows := deprecationQuery(t, root, repo, "--ref", "HEAD:old.go#L3")
	if len(rows) != 1 {
		t.Fatalf("direct related evidence not found: %v", rows)
	}
	assertion := rows[0].Assertions[0]
	if assertion.ReplacementHealth == nil || assertion.ReplacementHealth.Status != "current" {
		t.Fatalf("replacement: %#v", assertion)
	}
	for _, evidence := range assertion.Code {
		if evidence.Match != "" && evidence.Role != "related_target_code" {
			t.Fatalf("related target code became deprecated: %#v", evidence)
		}
	}
	board.Elements[0].Label = "Changed"
	if _, err := requirements.PublishWhiteboard(ctx, root, "atomic", "successor", "b2", "b1", "Successor", board); err != nil {
		t.Fatal(err)
	}
	if got := deprecationQuery(t, root, repo); got[0].Assertions[0].ReplacementHealth.Status != "stale" {
		t.Fatalf("replacement change hidden: %v", got)
	}
	// Global records are still readable when code inspection is unavailable.
	noCheckout := t.TempDir()
	if got := deprecationQuery(t, root, noCheckout); len(got) != 1 || !got[0].Assertions[0].Code[0].Provisional {
		t.Fatalf("source absence was hidden: %v", got)
	}
	var out bytes.Buffer
	if err := Query(ctx, []string{"deprecations", "--saga", root, "--repo", noCheckout, "--ref", "HEAD:old.go#L2"}, &out); err == nil || !strings.Contains(out.String(), "source_unavailable") {
		t.Fatalf("code lookup hid missing checkout: %v %s", err, out.String())
	}
	assertValid(t, root)
}
