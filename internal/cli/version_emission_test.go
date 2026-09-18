package cli

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diffuri"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

// SPEC.md: "no existing command may silently upgrade a v2, v3, or v4 document
// or emit a v5 record." Only `change-saga upgrade --to 5` may write a v5 Saga
// manifest. The tests in this file drive every existing mutation family
// against v2 and v3 Sagas and prove the manifest and the whole tree stay
// untouched by v5.

const v5SchemaPrefix = "https://changesaga.dev/schema/v5/"

type emissionStep struct {
	name string
	run  func(t *testing.T) error
}

// assertNoV5Emission proves that a mutation left the manifest byte-identical,
// kept the declared format version, and wrote no v5 schema reference anywhere
// in the Saga tree.
func assertNoV5Emission(t *testing.T, root string, manifest []byte, version int, step string) {
	t.Helper()
	current, err := os.ReadFile(filepath.Join(root, "saga.json"))
	if err != nil {
		t.Fatalf("%s: read saga.json: %v", step, err)
	}
	if !bytes.Equal(current, manifest) {
		t.Fatalf("%s rewrote saga.json; no mutation command may touch the manifest.\nbefore:\n%s\nafter:\n%s", step, manifest, current)
	}
	read, err := saga.ReadManifest(root)
	if err != nil {
		t.Fatalf("%s: read manifest: %v", step, err)
	}
	if read.Version != version {
		t.Fatalf("%s silently changed the Saga version from %d to %d", step, version, read.Version)
	}
	assertTreeHasNoV5Schema(t, root, step)
}

func assertTreeHasNoV5Schema(t *testing.T, root, step string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(v5SchemaPrefix)) {
			relative, _ := filepath.Rel(root, path)
			t.Errorf("%s emitted a v5 record: %s references %s", step, relative, v5SchemaPrefix)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("%s: walk %s: %v", step, root, err)
	}
	if t.Failed() {
		t.FailNow()
	}
}

func runEmissionSteps(t *testing.T, root string, version int, output *bytes.Buffer, steps []emissionStep) {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join(root, "saga.json"))
	if err != nil {
		t.Fatal(err)
	}
	if read, err := saga.ReadManifest(root); err != nil || read.Version != version {
		t.Fatalf("fixture manifest version = %d, err = %v; want %d", read.Version, err, version)
	}
	assertTreeHasNoV5Schema(t, root, "fixture")
	for _, step := range steps {
		output.Reset()
		if err := step.run(t); err != nil {
			t.Fatalf("%s: %v\n%s", step.name, err, output.String())
		}
		assertNoV5Emission(t, root, manifest, version, step.name)
	}
	assertValid(t, root)
}

func emissionFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeFile(t, path, content)
	return path
}

func emissionDiffURI(t *testing.T, start, end int) string {
	t.Helper()
	uri, err := diffuri.Build(diffuri.Reference{
		Repository: "https://example.test/acme/app.git", Base: "aaa", Head: "bbb",
		Kind: "line", Path: "worker.go", Side: "new", Start: start, End: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	return uri
}

// narrativeEmissionSteps are the v2-capable families: hierarchy authoring,
// fragment content, landmarks, coverage, claims, and review discussion.
func narrativeEmissionSteps(t *testing.T, root string, output *bytes.Buffer) []emissionStep {
	ctx := context.Background()
	svg := emissionFile(t, "map.svg", `<svg xmlns="http://www.w3.org/2000/svg"><g id="worker-pool"><text>Workers</text></g></svg>`)
	content := emissionFile(t, "content.md", "# Worker flow {#worker-flow}\n\nWorkers pull one job at a time.\n")
	first := emissionDiffURI(t, 3, 7)
	second := emissionDiffURI(t, 8, 9)
	return []emissionStep{
		{"add-chapter", func(*testing.T) error {
			return AddChapter(ctx, []string{"--id", "backend", "--title", "Backend", root, "backend"}, output)
		}},
		{"add-section", func(*testing.T) error {
			return AddSection(ctx, []string{"--id", "request-flow", root, "backend/request-flow"}, output)
		}},
		{"add-fragment", func(*testing.T) error {
			return AddFragment(ctx, []string{"--section", "request-flow", "--id", "worker-flow", "--name", "worker-flow", "--title", "Worker flow", root}, output)
		}},
		{"set-fragment-content", func(*testing.T) error {
			return SetFragmentContent(ctx, []string{"--target", "worker-flow", "--source", content, root}, output)
		}},
		{"add-fragment svg", func(*testing.T) error {
			return AddFragment(ctx, []string{"--type", "svg", "--title", "System map", "--source", svg, root}, output)
		}},
		{"add-landmark", func(*testing.T) error {
			return AddLandmark(ctx, []string{"--target", "system-map.fragment", "--element-id", "worker-pool", "--label", "Worker pool", "--description", "Workers pull one job at a time from the shared queue.", root}, output)
		}},
		{"cover", func(*testing.T) error {
			return Cover(ctx, []string{"--target", "system-map.fragment/___landmarks/worker-pool.landmark", "--uri", first, "--name", "workers", "--note", "Implements the worker pool.", root}, output)
		}},
		{"replace-coverage", func(*testing.T) error {
			return ReplaceCoverage(ctx, []string{"--record", "system-map.fragment/___landmarks/worker-pool.landmark/___diffs/workers.json", "--target", "overview.fragment", "--uri", second, "--name", "overview-workers", root}, output)
		}},
		{"remove-coverage", func(*testing.T) error {
			return RemoveCoverage(ctx, []string{"--record", "overview.fragment/___diffs/overview-workers.json", root}, output)
		}},
		{"add-claim", func(*testing.T) error {
			return AddClaim(ctx, []string{"--id", "single-flight", "--target", "overview.fragment", "--kind", "invariant", "--statement", "Only one sampler can run at a time.", "--diff", first, root}, output)
		}},
		{"verify-claim", func(*testing.T) error {
			return VerifyClaim(ctx, []string{"--id", "single-flight-check", "--claim", "single-flight", "--status", "verified", "--method", "test", "--summary", "The concurrency test passed.", "--command", "go test ./...", root}, output)
		}},
		{"thread", func(*testing.T) error {
			return Thread(ctx, []string{"--target", "overview.fragment", "--body", "Please clarify this.", root}, output)
		}},
		{"reply", func(t *testing.T) error {
			document, _, err := saga.Load(root)
			if err != nil {
				return err
			}
			if len(document.Threads) != 1 {
				t.Fatalf("threads = %d, want 1", len(document.Threads))
			}
			return Reply(ctx, []string{"--thread", document.Threads[0].ID, "--body", "Clarified.", "--state", "resolved", root}, output)
		}},
		{"review", func(*testing.T) error {
			return Review(ctx, []string{"--target", "overview.fragment", "--state", "approved", "--reviewer-kind", "human", root}, output)
		}},
	}
}

func TestNoMutationCommandEmitsV5OnV3Saga(t *testing.T) {
	root := newLivingSaga(t)
	ctx := context.Background()
	var output bytes.Buffer
	requests := 0
	json := func(args ...string) []string {
		requests++
		return append(args, "--request-id", fmt.Sprintf("emission-request-%d", requests), "--json")
	}

	const (
		storyURN     = "urn:change-saga:atomic:story:checkout"
		prototypeURN = "urn:change-saga:atomic:prototype:checkout"
		waveURN      = "urn:change-saga:atomic:wave:delivery"
		itemURN      = "urn:change-saga:atomic:work-item:cli"
		docsItemURN  = "urn:change-saga:atomic:work-item:docs"
		mergeUnit    = `{"id":"primary","repository":"https://example.test/acme/app.git","source_branch":"feature/item","target_branch":"main","required":true}`
	)
	var citationURN, relationURN string
	var itemEventID string
	var plannedURN, readyURN string
	headOID := strings.Repeat("1", 40)
	prototypeV1 := newPrototypeSource(t, `<!doctype html><button id="buy">Buy</button>`)
	prototypeV2 := newPrototypeSource(t, `<!doctype html><button id="buy">Buy now</button>`)
	designContent := emissionFile(t, "design.md", "# Request sequence {#request-sequence}\n\nThe design is independently editable.\n")
	slideContent := emissionFile(t, "slide.svg", `<svg xmlns="http://www.w3.org/2000/svg"><text id="slide-title">Flow</text></svg>`)

	living := []emissionStep{
		{"citation add", func(t *testing.T) error {
			output.Reset()
			if err := Citation(ctx, json("add", root, "--id", "source", "--kind", "url", "--title", "Source", "--reference", "https://example.test/source"), &output); err != nil {
				return err
			}
			citationURN = decodeLivingOutput(t, &output).Resource
			return nil
		}},
		{"story add", func(*testing.T) error {
			return Story(ctx, json("add", root, "--id", "checkout", "--revision", "r1", "--event", "proposed",
				"--title", "Checkout", "--statement", "As a buyer I can check out", "--priority", "high",
				"--citation", citationURN, "--criterion", "fast=Completes promptly"), &output)
		}},
		{"story revise", func(*testing.T) error {
			return Story(ctx, json("revise", root, "--story", storyURN, "--revision", "r2", "--parent", storyURN+":revision:r1",
				"--title", "Checkout clarified", "--statement", "As a buyer I can check out quickly", "--priority", "high",
				"--citation", citationURN, "--criterion", "fast=Completes promptly"), &output)
		}},
		{"story set-state", func(*testing.T) error {
			parent, err := requirements.StoryEventURN("atomic", "checkout", "proposed")
			if err != nil {
				return err
			}
			return Story(ctx, json("set-state", root, "--story", storyURN, "--event", "accepted", "--parent", parent,
				"--state", "accepted", "--reason", "Ready"), &output)
		}},
		{"criterion add", func(*testing.T) error {
			return Criterion(ctx, json("add", root, "--story", storyURN, "--parent", storyURN+":revision:r2", "--revision", "r3",
				"--id", "audited", "--statement", "The purchase is audited"), &output)
		}},
		{"criterion revise", func(*testing.T) error {
			return Criterion(ctx, json("revise", root, "--story", storyURN, "--criterion", storyURN+":criterion:audited",
				"--parent", storyURN+":revision:r3", "--revision", "r4", "--statement", "Every purchase is audited"), &output)
		}},
		{"criterion remove", func(*testing.T) error {
			return Criterion(ctx, json("remove", root, "--story", storyURN, "--criterion", storyURN+":criterion:audited",
				"--parent", storyURN+":revision:r4", "--revision", "r5", "--reason", "Replaced by a different obligation"), &output)
		}},
		{"relation add", func(t *testing.T) error {
			output.Reset()
			if err := Relation(ctx, json("add", root, "--id", "checkout-refines-fast", "--type", "refines", "--from", storyURN,
				"--to", storyURN+":criterion:fast", "--rationale", "The criterion sharpens the story"), &output); err != nil {
				return err
			}
			relationURN = decodeLivingOutput(t, &output).Resource
			return nil
		}},
		{"relation supersede", func(*testing.T) error {
			return Relation(ctx, json("supersede", root, "--relation", relationURN), &output)
		}},
		{"prototype add-html", func(*testing.T) error {
			return Prototype(ctx, json("add-html", root, "--id", "checkout", "--revision", "r1", "--title", "Checkout",
				"--source", prototypeV1, "--state", "ready"), &output)
		}},
		{"prototype add-external", func(*testing.T) error {
			return Prototype(ctx, json("add-external", root, "--id", "figma-link", "--revision", "r1",
				"--title", "Figma", "--url", "https://www.figma.com/file/abc"), &output)
		}},
		{"prototype revise", func(*testing.T) error {
			return Prototype(ctx, json("revise", root, "--prototype", prototypeURN, "--revision", "r2",
				"--parent", prototypeURN+":revision:r1", "--title", "Checkout", "--state", "ready", "--source", prototypeV2), &output)
		}},
		{"prototype annotate", func(*testing.T) error {
			return Prototype(ctx, json("annotate", root, "--prototype", prototypeURN, "--id", "buy-button",
				"--target", storyURN, "--rationale", "The buy button is the primary action.",
				"--prototype-revision", prototypeURN+":revision:r2", "--story-revision", storyURN+":revision:r5",
				"--element-id", "buy"), &output)
		}},
		{"plan add-wave", func(*testing.T) error {
			return Plan(ctx, json("add-wave", root, "--id", "delivery", "--revision", "delivery-r1", "--title", "Delivery",
				"--objective", "Ship independently"), &output)
		}},
		{"plan revise-wave", func(*testing.T) error {
			return Plan(ctx, json("revise-wave", root, "--wave", waveURN, "--revision", "delivery-r2",
				"--parent", waveURN+":revision:delivery-r1", "--title", "Delivery", "--objective", "Ship mergeable work"), &output)
		}},
		{"plan add-item", func(t *testing.T) error {
			output.Reset()
			if err := Plan(ctx, json("add-item", root, "--id", "cli", "--revision", "cli-r1", "--title", "CLI",
				"--objective", "Expose writers", "--deliverable", "Mutation commands", "--wave", waveURN,
				"--merge-unit", mergeUnit), &output); err != nil {
				return err
			}
			result := decodeLivingOutput(t, &output)
			if len(result.EventIDs) != 1 {
				t.Fatalf("add-item result = %#v", result)
			}
			itemEventID = result.EventIDs[0]
			output.Reset()
			return Plan(ctx, json("add-item", root, "--id", "docs", "--revision", "docs-r1", "--title", "Docs",
				"--objective", "Document the CLI", "--deliverable", "CLI examples", "--wave", waveURN), &output)
		}},
		{"plan revise-item", func(*testing.T) error {
			return Plan(ctx, json("revise-item", root, "--item", itemURN, "--revision", "cli-r2",
				"--parent", itemURN+":revision:cli-r1", "--title", "CLI", "--objective", "Expose supported writers",
				"--deliverable", "Mutation commands", "--wave", waveURN, "--merge-unit", mergeUnit), &output)
		}},
		{"plan add-dependency", func(*testing.T) error {
			return Plan(ctx, json("add-dependency", root, "--id", "cli-before-docs", "--prerequisite", itemURN,
				"--dependent", docsItemURN, "--condition", "progress_done", "--reason", "Examples follow the command contract"), &output)
		}},
		{"plan add-contract", func(*testing.T) error {
			return Plan(ctx, json("add-contract", root, "--id", "cli-docs", "--revision", "cli-docs-r1", "--kind", "handoff",
				"--provider", itemURN, "--consumer", docsItemURN, "--statement", "Publish stable help",
				"--acceptance", "Every command has deterministic help"), &output)
		}},
		{"plan assign", func(*testing.T) error {
			return Plan(ctx, json("assign", root, "--item", itemURN, "--workspace", "dacab14f-c2d6-4dde-99e4-4132090cd897",
				"--repository-id", "repo-1", "--branch", "feature/cli"), &output)
		}},
		{"plan progress", func(*testing.T) error {
			parent, err := workplan.ProgressEventURN("atomic", "cli", itemEventID)
			if err != nil {
				return err
			}
			return Plan(ctx, json("progress", root, "--item", itemURN, "--from", parent, "--to", "ready"), &output)
		}},
		{"plan record-merge planned", func(t *testing.T) error {
			output.Reset()
			if err := Plan(ctx, json("record-merge", root, "--item", itemURN, "--unit", "primary", "--state", "planned"), &output); err != nil {
				return err
			}
			var err error
			plannedURN, err = workplan.MergeEventURN("atomic", "cli", decodeLivingOutput(t, &output).EventIDs[0])
			return err
		}},
		{"plan record-merge ready", func(t *testing.T) error {
			output.Reset()
			if err := Plan(ctx, json("record-merge", root, "--item", itemURN, "--unit", "primary", "--state", "ready",
				"--from", plannedURN, "--head-oid", headOID), &output); err != nil {
				return err
			}
			var err error
			readyURN, err = workplan.MergeEventURN("atomic", "cli", decodeLivingOutput(t, &output).EventIDs[0])
			return err
		}},
		{"plan record-merge integrated", func(*testing.T) error {
			return Plan(ctx, json("record-merge", root, "--item", itemURN, "--unit", "primary", "--state", "integrated",
				"--from", readyURN, "--head-oid", headOID, "--commit", strings.Repeat("2", 40)), &output)
		}},
		{"design add-fragment root", func(*testing.T) error {
			return Design(ctx, []string{"add-fragment", "--id", "system-overview", "--name", "system-overview", "--title", "System overview", root}, &output)
		}},
		{"design add-chapter", func(*testing.T) error {
			return Design(ctx, []string{"add-chapter", "--id", "architecture", "--title", "Architecture", root, "architecture"}, &output)
		}},
		{"design add-section", func(*testing.T) error {
			return Design(ctx, []string{"add-section", "--id", "design-flow", root, "architecture/design-flow"}, &output)
		}},
		{"design add-fragment", func(*testing.T) error {
			return Design(ctx, []string{"add-fragment", "--section", "architecture.chapter/design-flow", "--id", "sequence", "--name", "sequence", "--title", "Sequence", root}, &output)
		}},
		{"design set-fragment-content", func(*testing.T) error {
			return Design(ctx, []string{"set-fragment-content", "--target", "sequence", "--source", designContent, root}, &output)
		}},
		{"add-deck", func(*testing.T) error {
			return AddDeck(ctx, []string{"--objective", "Explain the flow to reviewers.", root, "flow"}, &output)
		}},
		{"add-slide", func(*testing.T) error {
			return AddSlide(ctx, []string{"--deck", "flow", "--intent", "explain", "--layout", "diagram", "--title", "Flow", "--takeaway", "The complex behavior is explicit.", root, "flow-change"}, &output)
		}},
		{"add-item", func(*testing.T) error {
			return AddItem(ctx, []string{"--slide", "flow-change", "--kind", "callout", "--id", "surprise", "--element-id", "slide-title", "--description", "The reviewer surprise for this change.", "--body", "The implementation takes the non-obvious path.", root}, &output)
		}},
		{"set-slide-content", func(*testing.T) error {
			return SetSlideContent(ctx, []string{"--target", "flow-change", "--source", slideContent, root}, &output)
		}},
	}
	runEmissionSteps(t, root, saga.CurrentSagaVersion, &output, append(living, narrativeEmissionSteps(t, root, &output)...))
}

// rebase-evidence needs a real product diff whose base moved; it reuses the
// dedicated rebase fixture (a v3 Saga with v2-era evidence and claims).
func TestNoMutationCommandEmitsV5WhenRebasingEvidence(t *testing.T) {
	fixture := newEvidenceRebaseFixture(t)
	var output bytes.Buffer
	runEmissionSteps(t, fixture.root, saga.CurrentSagaVersion, &output, []emissionStep{
		{"rebase-evidence", func(*testing.T) error {
			return RebaseEvidence(context.Background(), []string{"--repo", fixture.repo, "--from-base", fixture.old.BaseOID, "--carry-verifications", "--json", fixture.root}, &output)
		}},
	})
}

func TestNoMutationCommandEmitsV5OnV2Saga(t *testing.T) {
	root := newAuthoredSaga(t)
	var output bytes.Buffer
	runEmissionSteps(t, root, saga.LegacySagaVersion, &output, narrativeEmissionSteps(t, root, &output))
}

// TestOnlyUpgradeWritesV5Manifest is a static guard for the SPEC.md rule that
// no existing command may silently upgrade a v2, v3, or v4 document or emit a
// v5 record: writing a well-formed v5 manifest requires the v5 schema URL, so
// only the core saga package (which defines and validates it), the quality
// package (which reads and validates v5 quality records), and
// `change-saga upgrade --to 5` may reference it.
func TestOnlyUpgradeWritesV5Manifest(t *testing.T) {
	allowedDirs := []string{
		filepath.Join("..", "..", "internal", "saga") + string(filepath.Separator),
	}
	literalAllowedDirs := append([]string{
		filepath.Join("..", "..", "internal", "quality") + string(filepath.Separator),
	}, allowedDirs...)
	upgrade := filepath.Join("..", "..", "internal", "cli", "upgrade.go")
	allowed := func(path string, dirs []string) bool {
		if path == upgrade {
			return true
		}
		for _, dir := range dirs {
			if strings.HasPrefix(path, dir) {
				return true
			}
		}
		return false
	}
	scanned := 0
	for _, tree := range []string{filepath.Join("..", "..", "internal"), filepath.Join("..", "..", "cmd")} {
		err := filepath.WalkDir(tree, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			source := string(data)
			if strings.Contains(source, "V5SchemaURL") && !allowed(path, allowedDirs) {
				t.Errorf("%s references V5SchemaURL; SPEC.md: no existing command may silently upgrade a v2, v3, or v4 document or emit a v5 record, so only `change-saga upgrade --to 5` (internal/cli/upgrade.go) may write a v5 Saga manifest", path)
			}
			if strings.Contains(source, "schema/v5/saga.schema.json") && !allowed(path, literalAllowedDirs) {
				t.Errorf("%s contains the v5 Saga schema URL literal; SPEC.md: no existing command may silently upgrade a v2, v3, or v4 document or emit a v5 record, so only `change-saga upgrade --to 5` (internal/cli/upgrade.go) may write a v5 Saga manifest", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", tree, err)
		}
	}
	if scanned == 0 {
		t.Fatal("source guard scanned no Go files; the relative roots are wrong")
	}
}
