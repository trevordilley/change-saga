package integration

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/experiments/diagram-api/draft"
	"github.com/twentyideas/changesaga/internal/cli"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Everything authored here belongs to temporary fixture directories. No app.saga
// or review state is read or changed. Evidence is real within the fixture repo.
func TestGeneratedSVGPreservesSagaItemEvidenceAndCriterion(t *testing.T) {
	ctx := context.Background()
	tmp, err := os.MkdirTemp("", "diagram-saga-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	repo := filepath.Join(tmp, "repo")
	if err := os.Mkdir(repo, 0755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		c := exec.Command("git", args...)
		c.Dir = repo
		b, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("git: %s: %v", b, e)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "-b", "main")
	git("config", "user.name", "Fixture Author")
	git("config", "user.email", "fixture@example.test")
	git("remote", "add", "origin", "https://example.test/fixture.git")
	code := []byte("package fixture\n\nfunc Validate() bool { return true }\n")
	if err := os.WriteFile(filepath.Join(repo, "validate.go"), code, 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "fixture")
	commit := git("rev-parse", "HEAD")
	root := filepath.Join(tmp, "atomic.saga")
	var out bytes.Buffer
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatalf("%v\n%s", e, out.String())
		}
		out.Reset()
	}
	must(cli.Init(ctx, []string{"--repo", repo, "--repository", "https://example.test/fixture.git", root}, &out))
	// Direct read is confined to this test fixture, matching the existing CLI tests.
	manifest, err := saga.ReadManifest(root)
	must(err)
	prefix := "urn:change-saga:" + manifest.ID
	must(cli.Feature(ctx, []string{"add", "--id", "core", "--title", "Core", root}, &out))
	must(cli.Persona(ctx, []string{"add", "--id", "user", "--name", "User", "--description", "Fixture beneficiary", root}, &out))
	must(cli.Overview(ctx, []string{"set-description", "--text", "Temporary diagram integration fixture.", root}, &out))
	must(cli.Story(ctx, []string{"add", root, "--feature", "core", "--persona", prefix + ":persona:user", "--id", "valid", "--revision", "r1", "--event", "proposed", "--title", "Valid input", "--statement", "As a user I receive a valid result", "--criterion", "returns=Validation returns true", "--request-id", "fixture-story"}, &out))
	must(cli.AddDeck(ctx, []string{"--feature", "core", "--id", "implementation", "--objective", "Fixture selector preservation", root, "implementation"}, &out))
	digest, err := coderef.DigestRange(code, 3, 3)
	must(err)
	evidence := []saga.CodeFile{{Version: saga.CurrentVersion, References: []coderef.Reference{{Commit: commit, Path: "validate.go", Start: 3, End: 3, Digest: digest, Note: "Exact fixture validation function."}}}}
	links := []saga.CriterionLink{{ID: "validation-explains-returns", Criterion: prefix + ":story:valid:criterion:returns", StoryRevision: prefix + ":story:valid:revision:r1", Rationale: "The fixture Item identifies this exact function."}}
	d := draft.New("fixture", "Selector preservation")
	el := draft.Element{ID: "validator", Kind: "node", Shape: "service", Label: "Validate", X: 100, Y: 100, W: 300, H: 110, Style: "normal", Icon: "lucide:check"}
	for _, v := range evidence {
		b, _ := json.Marshal(v)
		el.Evidence = append(el.Evidence, b)
	}
	for _, v := range links {
		b, _ := json.Marshal(v)
		el.CriterionLinks = append(el.CriterionLinks, b)
	}
	d.Elements[el.ID] = el
	ds := draft.Store{Root: filepath.Join(tmp, "drawing")}
	drawResult, err := ds.Apply(draft.Request{Version: 1, RequestID: "create", Expected: "absent", Source: &d}, false)
	must(err)
	rec, err := ds.Load()
	must(err)
	svg, err := ds.Visual(rec)
	must(err)
	req := cli.SlideTransactionRequest{Version: 1, Operation: "create", RequestID: "slide-create", ExpectedSnapshot: "absent", Deck: "implementation", Slide: cli.SlideTransactionSlide{ID: "fixture", Title: "Fixture", Intent: "explain", Layout: "diagram", MediaType: "image/svg+xml", Takeaway: "Explicit geometry edits preserve exact links.", ReadingOrder: []string{"validator"}}, Asset: cli.SlideTransactionAsset{ContentBase64: base64.StdEncoding.EncodeToString(svg)}, Items: []cli.SlideTransactionItemRequest{{ID: "validator", Kind: "node", Label: "Validate", Description: "Fixture validation boundary.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "validator"}, Evidence: evidence, CriterionLinks: links}}}
	published, err := cli.ApplySlideTransaction(ctx, root, tmp, repo, req, false)
	must(err)
	_, err = ds.Apply(draft.Request{Version: 1, RequestID: "move", Expected: drawResult.Snapshot, Operations: []draft.Operation{{Op: "move", ID: "validator", DX: 40}, {Op: "update", ID: "validator", Set: json.RawMessage(`{"label":"Validate input","style":"primary"}`)}}}, false)
	must(err)
	rec, err = ds.Load()
	must(err)
	svg, err = ds.Visual(rec)
	must(err)
	req.Operation = "update"
	req.RequestID = "slide-update"
	req.ExpectedSnapshot = published.Snapshot
	req.Asset.ContentBase64 = base64.StdEncoding.EncodeToString(svg)
	req.Items[0].Label = "Validate input"
	updated, err := cli.ApplySlideTransaction(ctx, root, tmp, repo, req, false)
	must(err)

	if err := cli.Query(ctx, []string{"slide", "--saga", root, "--repo", repo, "--target", updated.Target}, &out); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"element_id":"validator"`, digest, links[0].Criterion, links[0].StoryRevision} {
		compact := bytes.NewBuffer(nil)
		if err := json.Compact(compact, out.Bytes()); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(compact.String(), value) {
			t.Fatalf("query lost %q", value)
		}
	}
	req.ExpectedSnapshot = updated.Snapshot
	req.RequestID = "broken"
	req.Asset.ContentBase64 = base64.StdEncoding.EncodeToString(bytes.ReplaceAll(svg, []byte(`"validator"`), []byte(`"broken"`)))
	if _, err := cli.ApplySlideTransaction(ctx, root, tmp, repo, req, true); err == nil {
		t.Fatal("Saga accepted broken selector")
	}
}
