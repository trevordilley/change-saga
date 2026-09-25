package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestInventoryPublic(t *testing.T) {
	ctx := context.Background()
	repo, commit := sourceRepo(t, map[string]string{"flags.go": "package flags\nvar enabled = true\nfunc Read() bool { return enabled }\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	var output bytes.Buffer
	source := filepath.Join(t.TempDir(), "definition.json")
	code := []map[string]any{{"commit": "HEAD", "path": "flags.go", "start": 2, "end": 2, "note": "Stores the evaluated flag state."}}
	define := func(name string) map[string]any {
		return map[string]any{"name": name, "explanation": "Reads a pinned flag value and returns an explicit Boolean to its caller.", "code": code}
	}
	run := func(kind, op, id string, def map[string]any, extra ...string) error {
		t.Helper()
		data, _ := json.Marshal(def)
		if err := os.WriteFile(source, data, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{op, "--id", id, "--from", source, "--repo", repo, "--json"}
		args = append(args, extra...)
		args = append(args, root)
		output.Reset()
		return Technical(ctx, kind, args, &output)
	}
	urn := "urn:change-saga:atomic:"
	for _, id := range []string{"store", "reader"} {
		if err := run("component", "add", id, define(id)); err != nil {
			t.Fatal(err)
		}
	}
	system := define("FeatureFlag")
	system["components"] = []saga.DocumentationLink{{Target: urn + "component:store", Revision: urn + "component:store:revision:r1"}, {Target: urn + "component:reader", Revision: urn + "component:reader:revision:r1"}}
	system["interactions"] = []map[string]any{{"id": "read", "from": urn + "component:reader", "to": urn + "component:store", "description": "Reads the current flag state.", "code": code}}
	if err := run("system", "add", "feature-flag", system); err != nil {
		t.Fatal(err)
	}
	if err := run("system", "add", "feature-flag", system); err != nil || !strings.Contains(output.String(), `"replayed": true`) {
		t.Fatalf("identical retry: %v %s", err, output.String())
	}
	changed := define("Different identity")
	if err := run("component", "add", "store", changed); err == nil {
		t.Fatal("duplicate identity overwritten")
	}
	bad := define("Bad graph")
	bad["components"] = []saga.DocumentationLink{{Target: urn + "component:store", Revision: urn + "component:store:revision:r1"}, {Target: urn + "component:store", Revision: urn + "component:store:revision:r1"}}
	bad["interactions"] = system["interactions"]
	if err := run("system", "add", "bad", bad); err == nil {
		t.Fatal("duplicate System component accepted")
	}
	bad["components"] = []saga.DocumentationLink{{Target: urn + "component:store", Revision: urn + "component:store:revision:r1"}, {Target: urn + "component:missing", Revision: urn + "component:missing:revision:r1"}}
	bad["interactions"] = []map[string]any{{"id": "read", "from": urn + "component:store", "to": urn + "component:missing", "description": "missing", "code": code}}
	if err := run("system", "add", "bad", bad); err == nil {
		t.Fatal("missing System component accepted")
	}
	query := func(args ...string) map[string]any {
		t.Helper()
		return queryData(t, append([]string{"inventory", "--saga", root, "--repo", repo}, args...)...)
	}
	records := query()["records"].([]any)
	if len(records) != 3 {
		t.Fatalf("inventory count: %d", len(records))
	}
	first := records[0].(map[string]any)["current_revision"].(map[string]any)["code"].([]any)[0].(map[string]any)
	if first["commit"] != commit || !strings.HasPrefix(first["digest"].(string), "sha256:") {
		t.Fatalf("evidence not pinned: %v", first)
	}
	if err := AddDeck(ctx, []string{"--feature", testFeature, "--objective", "Flag decision flow", root, "flags"}, &output); err != nil {
		t.Fatal(err)
	}
	target := urn + "system:feature-flag"
	for _, slide := range []string{"checkout", "search"} {
		if err := AddSlide(ctx, []string{"--deck", "flags", "--intent", "explain", "--layout", "diagram", root, slide}, &output); err != nil {
			t.Fatal(err)
		}
		if err := AddItem(ctx, []string{"--slide", slide, "--kind", "node", "--element-id", "slide-title", "--description", "Reuses the shared flag decision.", "--documentation", target, "--documentation-revision", target + ":revision:r1", root}, &output); err != nil {
			t.Fatal(err)
		}
		selected := queryData(t, "slide", "--saga", root, "--repo", repo, "--target", urn+"slide:"+slide)
		items := selected["items"].([]any)
		item := items[0].(map[string]any)
		if item["documentation"].(map[string]any)["target"] != target {
			t.Fatalf("query lost pin: %v", item)
		}
		doc, validation, err := saga.Load(root)
		if err != nil || !validation.Valid {
			t.Fatalf("load: %v %v", err, validation.Issues)
		}
		for _, deck := range doc.Decks {
			for _, s := range deck.Slides {
				for _, i := range s.Items {
					if len(i.Code) != 0 {
						t.Fatal("documentation silently inherited coverage")
					}
				}
			}
		}
	}
	if err := run("component", "revise", "store", define("FlagStore"), "--revision", "r2", "--parent", urn+"component:store:revision:r1"); err != nil {
		t.Fatal(err)
	}
	sys := query("--target", target, "--history")["records"].([]any)[0].(map[string]any)
	if sys["links"].([]any)[0].(map[string]any)["status"] != "stale" {
		t.Fatalf("stale member pin was hidden: %v", sys)
	}
	if err := run("system", "add", "stale-system", system); err == nil {
		t.Fatal("new System revision accepted stale Component")
	}
	system["components"].([]saga.DocumentationLink)[0].Revision = urn + "component:store:revision:r2"
	if err := run("system", "revise", "feature-flag", system, "--revision", "r2", "--parent", target+":revision:r1"); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := Validate(ctx, []string{"--json", root}, &output); err != nil {
		t.Fatalf("history invalid: %v %s", err, output.String())
	}
	if !strings.Contains(output.String(), "is stale") {
		t.Fatal("stale slide pins not reported")
	}
	// Code changes do not rewrite definitions or their old digests.
	writeFile(t, filepath.Join(repo, "flags.go"), "package flags\nvar enabled = false\nfunc Read() bool { return enabled }\n")
	git(t, repo, "commit", "-am", "change flag default")
	rec := query("--target", urn+"component:store")["records"].([]any)[0].(map[string]any)
	if rec["code_health"].([]any)[0].(map[string]any)["resolution"].(map[string]any)["state"] != "stale" {
		t.Fatal("changed evidence not stale")
	}
	output.Reset()
	if err := Technical(ctx, "component", []string{"set-state", "--id", "store", "--event", "retired", "--state", "retired", "--reason", "Replaced by runtime config", "--parent", urn + "component:store:event:active", "--json", root}, &output); err != nil {
		t.Fatal(err)
	}
	d, err := requirements.LoadInventory(root, "atomic")
	if err != nil {
		t.Fatal(err)
	}
	if d.LinkStatus(saga.DocumentationLink{Target: urn + "component:store", Revision: urn + "component:store:revision:r2"}) != "retired" {
		t.Fatal("retired pin hidden")
	}
}

func TestInventoryValidationPreservesMissingReviewDeckDiagnostic(t *testing.T) {
	fixture := newReviewFixture(t)
	document, validation, err := saga.Load(fixture.root)
	if err != nil || !validation.Valid {
		t.Fatalf("fixture: %v %+v", err, validation.Issues)
	}
	deck := document.FindReview("pr-7").Deck.Directory
	if err := os.Rename(deck, filepath.Join(t.TempDir(), "removed-deck")); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = Validate(context.Background(), []string{"--json", fixture.root}, &output)
	if err == nil || !strings.Contains(output.String(), "deck directory is missing") {
		t.Fatalf("missing deck must report invalid Saga, not panic: %v %s", err, &output)
	}
}

func TestInventoryQueryPaginationAndMissingTarget(t *testing.T) {
	repo, _ := sourceRepo(t, map[string]string{"a.go": "package a\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	var out bytes.Buffer
	if err := Query(context.Background(), []string{"inventory", "--saga", root, "--repo", repo, "--target", "urn:change-saga:atomic:system:missing"}, &out); err == nil || !strings.Contains(out.String(), "not_found") {
		t.Fatalf("missing target: %v %s", err, out.String())
	}
	out.Reset()
	if err := Query(context.Background(), []string{"schema", "inventory"}, &out); err != nil || !strings.Contains(out.String(), "data.records") {
		t.Fatalf("schema: %v %s", err, out.String())
	}
}

func TestInventoryTransaction(t *testing.T) {
	ctx := context.Background()
	root, repo, base, commit, id := newSlideTransactionFixture(t)
	request := slideTransactionRequest(t, repo, base, commit, id, "with-definition", "create", "absent", "node")
	ref := request.Items[0].Evidence[0].References[0]
	def := requirements.TechnicalDefinition{Name: "Worker", Explanation: "Runs the service safely.", Code: []requirements.Evidence{{Reference: ref}}}
	target := "urn:change-saga:" + id + ":component:worker"
	if _, err := requirements.WriteTechnical(root, id, "component", "worker", "r1", nil, def, true); err != nil {
		t.Fatal(err)
	}
	request.Items[0].Documentation = &saga.DocumentationLink{Target: target, Revision: target + ":revision:r1"}
	created, err := ApplySlideTransaction(ctx, root, base, repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	doc, _, err := saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	before, err := saga.CurrentDesignContentDigests(doc)
	if err != nil {
		t.Fatal(err)
	}
	def.Explanation = "Runs the service safely; failures propagate to the caller."
	if _, err := requirements.WriteTechnical(root, id, "component", "worker", "r2", []string{target + ":revision:r1"}, def, false); err != nil {
		t.Fatal(err)
	}
	doc, _, err = saga.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := saga.CurrentDesignContentDigests(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("definition revision silently changed slide/review currency")
	}
	retry, err := ApplySlideTransaction(ctx, root, base, repo, request, false)
	if err != nil || !retry.Replayed {
		t.Fatalf("stale identical retry: %v %#v", err, retry)
	}
	update := request
	update.Operation = "update"
	update.RequestID = "retained-pin"
	update.ExpectedSnapshot = created.Snapshot
	update.Slide.Title = "Clarified service flow"
	updated, err := ApplySlideTransaction(ctx, root, base, repo, update, false)
	if err != nil {
		t.Fatalf("an unchanged pin must be retained honestly: %v", err)
	}
	update.RequestID = "repinned"
	update.ExpectedSnapshot = updated.Snapshot
	update.Items[0].Documentation = &saga.DocumentationLink{Target: target, Revision: target + ":revision:r2"}
	repinned, err := ApplySlideTransaction(ctx, root, base, repo, update, false)
	if err != nil {
		t.Fatal(err)
	}
	if repinned.Snapshot == updated.Snapshot || len(repinned.Diff.UpdatedItems) != 1 {
		t.Fatalf("explicit repin not inspectable: %#v", repinned)
	}
}

func TestInventoryCursor(t *testing.T) {
	ctx := context.Background()
	repo, commit := sourceRepo(t, map[string]string{"flags.go": "package flags\n"})
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	root := newTermSaga(t, repo)
	ref := coderef.Reference{Commit: commit, Path: "flags.go", Start: 1, End: 1, Digest: coderef.DigestBytes([]byte("package flags\n")), Note: "Exact code"}
	def := requirements.TechnicalDefinition{Name: "Flag", Explanation: "A bounded definition.", Code: []requirements.Evidence{{Reference: ref}}}
	for _, id := range []string{"a", "b", "c"} {
		if _, err := requirements.WriteTechnical(root, "atomic", "component", id, "r1", nil, def, true); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	args := []string{"inventory", "--saga", root, "--repo", repo, "--limit", "1"}
	if err := Query(ctx, args, &out); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Snapshot string `json:"snapshot"`
		Page     struct {
			Total, Returned int
			HasMore         bool   `json:"has_more"`
			NextCursor      string `json:"next_cursor"`
		} `json:"page"`
	}
	if err := json.Unmarshal(out.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Page.Total != 3 || page.Page.Returned != 1 || !page.Page.HasMore {
		t.Fatalf("bad page: %s", out.String())
	}
	out.Reset()
	if err := Query(ctx, append(args, "--cursor", page.Page.NextCursor), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `component:b`) {
		t.Fatalf("cursor did not advance: %s", out.String())
	}
	if _, err := requirements.WriteTechnical(root, "atomic", "component", "d", "r1", nil, def, true); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Query(ctx, append(args, "--cursor", page.Page.NextCursor), &out); err == nil || !strings.Contains(out.String(), "stale_snapshot") {
		t.Fatalf("stale cursor accepted: %v %s", err, out.String())
	}
}
