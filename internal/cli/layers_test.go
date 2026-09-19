package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/changeview"
)

type layerCommand func(context.Context, []string, io.Writer) error

func mustRun(t *testing.T, run layerCommand, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := run(context.Background(), args, &output); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, output.String())
	}
	return output.String()
}

// shopSaga builds an in-repo app Saga on main: a persona, a story serving
// it, and a queue design that addresses the story and references code.
func shopSaga(t *testing.T) (repo, root string) {
	t.Helper()
	repo = t.TempDir()
	git(t, repo, "init", "-b", "main")
	git(t, repo, "config", "user.name", "Test Author")
	git(t, repo, "config", "user.email", "test@example.test")
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/shop.git")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn sqs.Send(job)\n}\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Add checkout")
	root = filepath.Join(repo, "app.saga")
	mustRun(t, Init, "--repo", repo, "--id", "shop", root)
	mustRun(t, Epic, "add", "--id", "checkout", "--title", "Checkout", root)
	mustRun(t, Persona, "add", "--id", "shopper", "--name", "Shopper", "--description", "Buys things", root)
	mustRun(t, Story, "add", "--epic", "checkout", "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay", "--statement", "As a shopper I pay",
		"--priority", "must", "--criterion", "charged=The card is charged once", "--persona", "urn:change-saga:shop:persona:shopper", root)
	design := filepath.Join(t.TempDir(), "design.md")
	writeFile(t, design, "# Queue {#queue}\n\nJobs go to SQS.\n")
	mustRun(t, Design, "add-fragment", "--epic", "checkout", "--id", "queue-design", "--title", "Queue", "--type", "markdown", "--name", "queue-design", "--source", design, root)
	mustRun(t, Cover, "--target", "___epics/checkout.epic/___design/queue-design.fragment", "--commit", "HEAD", "--path", "src/queue.go", "--lines", "3-6", root)
	mustRun(t, Relation, "add", "--epic", "checkout", "--id", "design-pay", "--type", "addresses", "--from", "urn:change-saga:shop:fragment:queue-design",
		"--to", "urn:change-saga:shop:story:pay", "--rationale", "The queue is how payment reaches fulfilment", root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Document checkout")
	return repo, root
}

func statusLayers(t *testing.T, root string, args ...string) (statusDocument, map[string]json.RawMessage) {
	t.Helper()
	var output bytes.Buffer
	_ = Status(context.Background(), append(append([]string{"--json"}, args...), root), &output)
	var document statusDocument
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatalf("status --json: %v\n%s", err, output.String())
	}
	_ = json.Unmarshal(output.Bytes(), &raw)
	return document, raw
}

func affectedBy(layers *changeview.Layers, urn string) []changeview.Cause {
	for _, affected := range layers.Affected {
		if affected.URN == urn {
			return affected.Because
		}
	}
	return nil
}

// A change that edits only code still lights up the design that references
// it, the story the design addresses, and the persona the story serves.
func TestCodeOnlyChangeLightsUpTheChain(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(3, func() error { return sqs.Send(job) })\n}\n")
	git(t, repo, "commit", "-am", "Retry enqueue")

	status, _ := statusLayers(t, root, "--against", "main")
	layers := status.Comparison
	if layers == nil || status.Opening.Mode != "compare" {
		t.Fatalf("compare mode reported no layers: %#v", status.Opening)
	}
	if len(layers.Changed) != 0 {
		t.Fatalf("a code-only change edited no records, got %#v", layers.Changed)
	}
	design := affectedBy(layers, "urn:change-saga:shop:fragment:queue-design")
	if len(design) == 0 || design[0].Kind != changeview.CauseCode {
		t.Fatalf("the design whose code changed is not affected by code: %#v", layers.Affected)
	}
	story := affectedBy(layers, "urn:change-saga:shop:story:pay")
	if len(story) == 0 || story[0].Kind != changeview.CauseChain || story[0].Via != "urn:change-saga:shop:fragment:queue-design" {
		t.Fatalf("the story is not reached through its design: %#v", story)
	}
	if persona := affectedBy(layers, "urn:change-saga:shop:persona:shopper"); len(persona) == 0 || persona[0].Via != "urn:change-saga:shop:story:pay" {
		t.Fatalf("the persona is not reached through its story: %#v", persona)
	}
	if len(layers.Code.Groups) != 1 || layers.Code.Groups[0].URN != "urn:change-saga:shop:fragment:queue-design" || layers.Code.Groups[0].Layer != "affected" {
		t.Fatalf("the changed lines are not grouped under the design: %#v", layers.Code.Groups)
	}
}

// A story revision is Changed with its before and after, and the relation
// pinned to its old revision is Affected.
func TestStoryRevisionIsChangedWithBeforeAndAfter(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "once")
	mustRun(t, Story, "revise", "--story", "urn:change-saga:shop:story:pay", "--revision", "r2", "--parent", "urn:change-saga:shop:story:pay:revision:r1",
		"--title", "Pay once", "--statement", "As a shopper I pay exactly once", "--priority", "must", "--criterion", "charged=The card is charged exactly once", root)

	status, _ := statusLayers(t, root, "--against", "main")
	var story *changeview.Change
	for index := range status.Comparison.Changed {
		if status.Comparison.Changed[index].URN == "urn:change-saga:shop:story:pay" {
			story = &status.Comparison.Changed[index]
		}
	}
	if story == nil || story.Change != changeview.ChangeRevised || story.Before == nil || story.After == nil {
		t.Fatalf("the revised story is not Changed with before and after: %#v", status.Comparison.Changed)
	}
	if !strings.Contains(story.Before.Text, "charged once") || !strings.Contains(story.After.Text, "exactly once") {
		t.Fatalf("before/after text = %q / %q", story.Before.Text, story.After.Text)
	}
	if status.Comparison.Saga.Head.Source != changeview.SideWorking || status.Comparison.Saga.Base.Source != changeview.SideGit {
		t.Fatalf("uncommitted authoring must compare the working tree with the Saga at the merge-base: %#v", status.Comparison.Saga)
	}
	if causes := affectedBy(status.Comparison, "urn:change-saga:shop:relation:design-pay"); len(causes) == 0 || causes[0].Kind != changeview.CausePin {
		t.Fatalf("the relation pinned to the old revision is not affected: %#v", status.Comparison.Affected)
	}
}

// Observing has no change, so it reports no layers at all.
func TestObserveHasNoLayers(t *testing.T) {
	_, root := shopSaga(t)
	status, raw := statusLayers(t, root)
	if status.Opening.Mode != "observe" || status.Comparison != nil || raw["comparison"] != nil {
		t.Fatalf("observe mode reported a comparison: %#v", status.Opening)
	}
	if status.Report.Summary.Total != 0 {
		t.Fatalf("observe mode accounted for changed lines: %#v", status.Report.Summary)
	}
}

// A Saga created on the branch did not exist at the merge-base, so every
// record it holds is added.
func TestSagaAbsentAtTheBaseIsAllAdded(t *testing.T) {
	repo, root := shopSaga(t)
	git(t, repo, "branch", "-m", "main", "feature")
	git(t, repo, "checkout", "-b", "main", "HEAD~1")
	git(t, repo, "checkout", "feature")
	status, _ := statusLayers(t, root, "--against", "main")
	if status.Comparison == nil || status.Comparison.Saga.Base.Source != changeview.SideAbsent || len(status.Comparison.Changed) == 0 {
		t.Fatalf("a Saga absent at the base must be all added: %#v", status.Comparison)
	}
	for _, change := range status.Comparison.Changed {
		if change.Change != changeview.ChangeAdded {
			t.Fatalf("expected only added records, got %#v", change)
		}
	}
}
