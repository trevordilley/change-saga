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
	"github.com/twentyideas/changesaga/internal/saga"
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
	dir, _ := shopSagaTemplate.instantiate(t, func(t *testing.T, dir string) map[string]string {
		repo := mkdir(t, filepath.Join(dir, "repo"))
		git(t, repo, "init", "-b", "main")
		git(t, repo, "config", "user.name", "Test Author")
		git(t, repo, "config", "user.email", "test@example.test")
		git(t, repo, "remote", "add", "origin", "https://example.test/acme/shop.git")
		writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn sqs.Send(job)\n}\n")
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "Add checkout")
		root := filepath.Join(repo, "app.saga")
		mustRun(t, Init, "--repo", repo, "--id", "shop", root)
		mustRun(t, Feature, "add", "--id", "checkout", "--title", "Checkout", root)
		mustRun(t, Persona, "add", "--id", "shopper", "--name", "Shopper", "--description", "Buys things", root)
		mustRun(t, Story, "add", "--feature", "checkout", "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay", "--statement", "As a shopper I pay",
			"--priority", "must", "--criterion", "charged=The card is charged once", "--persona", "urn:change-saga:shop:persona:shopper", root)
		design := filepath.Join(dir, "design.md")
		writeFile(t, design, "# Queue {#queue}\n\nJobs go to SQS.\n")
		mustRun(t, Design, "add-fragment", "--feature", "checkout", "--id", "queue-design", "--title", "Queue", "--type", "markdown", "--name", "queue-design", "--source", design, root)
		mustRun(t, Cover, "--target", "___features/checkout.feature/___design/queue-design.fragment", "--commit", "HEAD", "--path", "src/queue.go", "--lines", "3-6", root)
		mustRun(t, Relation, "add", "--feature", "checkout", "--id", "design-pay", "--type", "addresses", "--from", "urn:change-saga:shop:fragment:queue-design",
			"--to", "urn:change-saga:shop:story:pay", "--rationale", "The queue is how payment reaches fulfilment", root)
		git(t, repo, "add", ".")
		git(t, repo, "commit", "-m", "Document checkout")
		return nil
	})
	repo = filepath.Join(dir, "repo")
	return repo, filepath.Join(repo, "app.saga")
}

var shopSagaTemplate fixtureTemplate

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// addQueueSlide adds a slide explaining the charged criterion, with one Item
// referencing the queue code.
func addQueueSlide(t *testing.T, root, id, title string) {
	t.Helper()
	visual := filepath.Join(t.TempDir(), id+".svg")
	writeFile(t, visual, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><text>`+title+`</text></svg>`+"\n")
	mustRun(t, AddSlide, "--deck", "urn:change-saga:shop:deck:impl", "--id", id, "--title", title, "--intent", "explain", "--layout", "diagram", "--source", visual, "--takeaway", title, root, id)
	mustRun(t, AddItem, "--slide", "urn:change-saga:shop:slide:"+id, "--id", id+"-item", "--kind", "node", "--label", title, "--description", title, "--region", "0,0,1,1", root)
	mustRun(t, Relation, "add", "--feature", "checkout", "--id", id+"-charged", "--type", "explains", "--from", "urn:change-saga:shop:slide:"+id+":item:"+id+"-item",
		"--to", "urn:change-saga:shop:story:pay:criterion:charged", "--scope", "self", "--rationale", "How the charge reaches fulfilment", root)
}

// removeSlides deletes every slide and Item of the implementation deck.
func removeSlides(t *testing.T, repo, root string) {
	t.Helper()
	deckDir := filepath.Join(root, "___features", "checkout.feature", "___slides", "impl.deck")
	for _, pattern := range []string{"20-s-*", "30-i-*"} {
		matches, _ := filepath.Glob(filepath.Join(deckDir, pattern))
		for _, match := range matches {
			git(t, repo, "rm", "-q", match)
		}
	}
}

// A slide that drops out and one that explains the same criterion pair up,
// the commit that replaced them sits beside both, and the new slide's
// history names what it replaced and the comparison that opens it.
func TestReplacedSlidePairsWithItsReplacementAndItsReason(t *testing.T) {
	t.Parallel()
	repo, root := shopSaga(t)
	mustRun(t, AddDeck, "--feature", "checkout", "--id", "impl", "--title", "Implementation", "--objective", "How payment ships", root, "impl")
	addQueueSlide(t, root, "sqs", "Jobs go through SQS")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Explain the queue")
	git(t, repo, "checkout", "-b", "postgres")
	mustRun(t, Relation, "supersede", "--relation", "urn:change-saga:shop:relation:sqs-charged", root)
	removeSlides(t, repo, root)
	addQueueSlide(t, root, "postgres", "Jobs go through a Postgres table")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "Move jobs to a Postgres table", "-m", "SQS could not order jobs.")

	status, _ := statusLayers(t, root, "--against", "main")
	changes := map[string]changeview.Change{}
	for _, change := range status.Comparison.Changed {
		changes[change.URN] = change
	}
	added, dropped := changes["urn:change-saga:shop:slide:postgres"], changes["urn:change-saga:shop:slide:sqs"]
	if added.Pair == nil || added.Pair.Role != changeview.PairReplaces || added.Pair.With != "urn:change-saga:shop:slide:sqs" || added.Pair.Basis != changeview.PairInferred {
		t.Fatalf("the new slide is not paired with the one it replaced: %#v", added.Pair)
	}
	if dropped.Change != changeview.ChangeRetired || !dropped.Removed || dropped.Pair == nil || dropped.Pair.With != "urn:change-saga:shop:slide:postgres" {
		t.Fatalf("the dropped slide is not paired: %#v", dropped)
	}
	if len(added.Pair.Shared) != 1 || added.Pair.Shared[0] != "urn:change-saga:shop:story:pay:criterion:charged" {
		t.Fatalf("pairing basis = %#v", added.Pair.Shared)
	}
	for _, change := range []changeview.Change{added, dropped} {
		if len(change.Reasons) != 1 || change.Reasons[0].Subject != "Move jobs to a Postgres table" || change.Reasons[0].Body != "SQS could not order jobs." {
			t.Fatalf("%s reasons = %#v", change.URN, change.Reasons)
		}
	}

	var output bytes.Buffer
	if err := Query(context.Background(), []string{"history", "--saga", root, "--node", "urn:change-saga:shop:slide:postgres"}, &output); err != nil {
		t.Fatalf("history: %v\n%s", err, output.String())
	}
	var envelope struct {
		Data changeview.History `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	history := envelope.Data
	if history.Introduced == nil || history.Introduced.Subject != "Move jobs to a Postgres table" || len(history.Replaced) != 1 || history.Replaced[0] != "urn:change-saga:shop:slide:sqs" {
		t.Fatalf("history = %#v", history)
	}
	if open := strings.Join(history.Introduced.Open, " "); !strings.Contains(open, "--against "+history.Introduced.Against) || !strings.Contains(open, "--head "+history.Introduced.Commit) || history.Introduced.Against == "" {
		t.Fatalf("history does not open the comparison where the slide was introduced: %q", open)
	}
}

// Renaming the Saga, as app.saga became change.saga, moves every record file
// without changing one. History still reaches back to where each record was
// introduced and what it replaced, and a comparison with a commit before the
// move reads the Saga where it was then instead of treating it as new.
func TestHistoryAndComparisonFollowAMovedSaga(t *testing.T) {
	t.Parallel()
	repo, root := shopSaga(t)
	mustRun(t, AddDeck, "--feature", "checkout", "--id", "impl", "--title", "Implementation", "--objective", "How payment ships", root, "impl")
	addQueueSlide(t, root, "sqs", "Jobs go through SQS")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Explain the queue")
	mustRun(t, Relation, "supersede", "--relation", "urn:change-saga:shop:relation:sqs-charged", root)
	removeSlides(t, repo, root)
	addQueueSlide(t, root, "postgres", "Jobs go through a Postgres table")
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "Move jobs to a Postgres table")
	git(t, repo, "checkout", "-b", "rename")
	git(t, repo, "mv", "app.saga", "change.saga")
	git(t, repo, "commit", "-m", "Rename the Saga to change.saga")
	moved := filepath.Join(repo, "change.saga")

	var output bytes.Buffer
	if err := Query(context.Background(), []string{"history", "--saga", moved, "--node", "urn:change-saga:shop:slide:postgres"}, &output); err != nil {
		t.Fatalf("history: %v\n%s", err, output.String())
	}
	var envelope struct {
		Data changeview.History `json:"data"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	history := envelope.Data
	if history.Introduced == nil || history.Introduced.Subject != "Move jobs to a Postgres table" || len(history.Replaced) != 1 || history.Replaced[0] != "urn:change-saga:shop:slide:sqs" {
		t.Fatalf("history after the move = %#v", history)
	}
	for _, event := range history.Events {
		if event.Subject == "Rename the Saga to change.saga" {
			t.Fatalf("the move is listed as a change to the slide: %#v", history.Events)
		}
	}

	status, _ := statusLayers(t, moved, "--against", "main")
	if status.Comparison.Saga.Base.Source != changeview.SideGit || len(status.Comparison.Changed) != 0 {
		t.Fatalf("comparing across the move: base %#v, changed %#v", status.Comparison.Saga.Base, status.Comparison.Changed)
	}
}

// When two new records could each replace the dropped one, the pair is
// ambiguous until an explicit supersedes relation decides it.
func TestAmbiguousReplacementNeedsAnExplicitLink(t *testing.T) {
	t.Parallel()
	repo, root := shopSaga(t)
	mustRun(t, AddDeck, "--feature", "checkout", "--id", "impl", "--title", "Implementation", "--objective", "How payment ships", root, "impl")
	addQueueSlide(t, root, "sqs", "Jobs go through SQS")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Explain the queue")
	git(t, repo, "checkout", "-b", "split")
	mustRun(t, Relation, "supersede", "--relation", "urn:change-saga:shop:relation:sqs-charged", root)
	removeSlides(t, repo, root)
	addQueueSlide(t, root, "table", "Jobs table")
	addQueueSlide(t, root, "worker", "Job worker")

	pairOf := func(urn string) *changeview.Pair {
		status, _ := statusLayers(t, root, "--against", "main")
		for _, change := range status.Comparison.Changed {
			if change.URN == urn {
				return change.Pair
			}
		}
		return nil
	}
	if pairing := pairOf("urn:change-saga:shop:slide:table"); pairing == nil || pairing.Basis != changeview.PairAmbiguous || pairing.Link == "" {
		t.Fatalf("an ambiguous replacement must ask for an explicit link: %#v", pairing)
	}
	mustRun(t, Relation, "add", "--feature", "checkout", "--id", "table-replaces-sqs", "--type", "supersedes", "--from", "urn:change-saga:shop:slide:table",
		"--to", "urn:change-saga:shop:slide:sqs", "--rationale", "The table holds the jobs the queue held", root)
	if pairing := pairOf("urn:change-saga:shop:slide:table"); pairing == nil || pairing.Basis != changeview.PairExplicit || pairing.With != "urn:change-saga:shop:slide:sqs" {
		t.Fatalf("an explicit supersedes relation must decide the pair: %#v", pairing)
	}
	_ = repo
}

// A squash merge collapses its branch's commits into one message; the
// ___merges record repin writes keeps the branch messages, and they sit
// beside the records the squash commit touched.
func TestSquashMergeKeepsItsBranchReasons(t *testing.T) {
	t.Parallel()
	repo, root := shopSaga(t)
	git(t, repo, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(repo, "src", "queue.go"), "package shop\n\n// Enqueue sends a job to SQS.\nfunc Enqueue(job string) error {\n\treturn retry(3, func() error { return sqs.Send(job) })\n}\n")
	git(t, repo, "commit", "-am", "Retry enqueue", "-m", "SQS throttles under load.")
	git(t, repo, "checkout", "main")
	base := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	git(t, repo, "merge", "--squash", "retry")
	git(t, repo, "commit", "-m", "Retry enqueue (#12)")
	landed := strings.TrimSpace(git(t, repo, "rev-parse", "HEAD"))
	mustRun(t, Repin, "--onto", landed, "--branch", "retry", root)
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-m", "Re-pin after landing")
	git(t, repo, "branch", "-D", "retry")

	status, _ := statusLayers(t, root, "--against", base, "--head", landed)
	var reasons []changeview.Reason
	for _, affected := range status.Comparison.Affected {
		if affected.URN == "urn:change-saga:shop:fragment:queue-design" {
			reasons = affected.Reasons
		}
	}
	if len(reasons) != 1 || reasons[0].Subject != "Retry enqueue (#12)" || len(reasons[0].Collapsed) != 1 || reasons[0].Collapsed[0].Body != "SQS throttles under load." {
		t.Fatalf("the squash commit's branch reasons are lost: %#v", reasons)
	}
}

// A Saga in its own repository documents a client's code: the code delta
// comes from the code repository and the Saga delta from the Saga commit
// whose sync cursor named the merge-base.
func TestCompanionSagaComparesThroughItsSyncCursor(t *testing.T) {
	t.Parallel()
	code := t.TempDir()
	git(t, code, "init", "-b", "main")
	git(t, code, "config", "user.name", "Code Author")
	git(t, code, "config", "user.email", "code@example.test")
	git(t, code, "remote", "add", "origin", "https://example.test/client/shop.git")
	writeFile(t, filepath.Join(code, "src", "queue.go"), "package shop\n\n// Enqueue sends a job.\nfunc Enqueue(job string) error {\n\treturn sqs.Send(job)\n}\n")
	git(t, code, "add", ".")
	git(t, code, "commit", "-m", "Add checkout")
	documented := strings.TrimSpace(git(t, code, "rev-parse", "HEAD"))

	docs := t.TempDir()
	git(t, docs, "init", "-b", "main")
	git(t, docs, "config", "user.name", "Docs Author")
	git(t, docs, "config", "user.email", "docs@example.test")
	root := filepath.Join(docs, "app.saga")
	mustRun(t, Init, "--repo", code, "--id", "shop", root)
	mustRun(t, Feature, "add", "--id", "checkout", "--title", "Checkout", root)
	mustRun(t, Story, "add", "--feature", "checkout", "--id", "pay", "--revision", "r1", "--event", "proposed", "--title", "Pay", "--statement", "As a shopper I pay",
		"--priority", "must", "--criterion", "charged=The card is charged once", root)
	design := filepath.Join(t.TempDir(), "design.md")
	writeFile(t, design, "# Queue {#queue}\n\nJobs go to SQS.\n")
	mustRun(t, Design, "add-fragment", "--feature", "checkout", "--id", "queue-design", "--title", "Queue", "--type", "markdown", "--name", "queue-design", "--source", design, root)
	mustRun(t, Cover, "--repo", code, "--target", "___features/checkout.feature/___design/queue-design.fragment", "--commit", "HEAD", "--path", "src/queue.go", "--lines", "3-6", root)
	mustRun(t, Relation, "add", "--feature", "checkout", "--id", "design-pay", "--type", "addresses", "--from", "urn:change-saga:shop:fragment:queue-design",
		"--to", "urn:change-saga:shop:story:pay", "--rationale", "How payment reaches fulfilment", root)
	mustRun(t, Sync, "--repo", code, root)
	git(t, docs, "add", ".")
	git(t, docs, "commit", "-m", "Document the checkout")
	if cursor, ok, err := saga.ReadCursor(root); err != nil || !ok || cursor.Commit != documented {
		t.Fatalf("sync did not record the documented commit: %#v %v %v", cursor, ok, err)
	}

	git(t, code, "checkout", "-b", "retry")
	writeFile(t, filepath.Join(code, "src", "queue.go"), "package shop\n\n// Enqueue sends a job.\nfunc Enqueue(job string) error {\n\treturn retry(3, func() error { return sqs.Send(job) })\n}\n")
	git(t, code, "commit", "-am", "Retry enqueue")
	mustRun(t, Story, "revise", "--story", "urn:change-saga:shop:story:pay", "--revision", "r2", "--parent", "urn:change-saga:shop:story:pay:revision:r1",
		"--title", "Pay reliably", "--statement", "As a shopper my payment survives throttling", "--priority", "must", "--criterion", "charged=The card is charged once", root)
	mustRun(t, Sync, "--repo", code, root)
	git(t, docs, "add", "-A")
	git(t, docs, "commit", "-m", "Say payment survives throttling")

	status, _ := statusLayers(t, root, "--repo", code, "--against", "main")
	layers := status.Comparison
	if !status.Opening.Companion || status.Opening.Cursor != status.Opening.HeadOID {
		t.Fatalf("status does not report the companion cursor: %#v", status.Opening)
	}
	if layers.Saga.Base.Source != changeview.SideGit || layers.Saga.Base.Commit == "" || layers.Saga.Head.Source != changeview.SideWorking {
		t.Fatalf("the Saga delta must come from the Saga commit whose cursor matched the base: %#v", layers.Saga)
	}
	if len(layers.Changed) != 1 || layers.Changed[0].URN != "urn:change-saga:shop:story:pay" || len(layers.Changed[0].Reasons) != 1 || layers.Changed[0].Reasons[0].Subject != "Say payment survives throttling" {
		t.Fatalf("the Saga delta = %#v", layers.Changed)
	}
	designReasons, found := []changeview.Reason{}, false
	for _, affected := range layers.Affected {
		if affected.URN == "urn:change-saga:shop:fragment:queue-design" {
			designReasons, found = affected.Reasons, affected.Because[0].Kind == changeview.CauseCode
		}
	}
	if !found || len(designReasons) != 1 || designReasons[0].Subject != "Retry enqueue" {
		t.Fatalf("the code delta must come from the code repository: %#v", layers.Affected)
	}

	// In the code repository the cursor is implicit, so sync refuses.
	_, inRepo := shopSaga(t)
	var output bytes.Buffer
	if err := Sync(context.Background(), []string{"--repo", filepath.Dir(inRepo), inRepo}, &output); err == nil || !strings.Contains(err.Error(), "no sync cursor") {
		t.Fatalf("sync in the code repository = %v", err)
	}
}
