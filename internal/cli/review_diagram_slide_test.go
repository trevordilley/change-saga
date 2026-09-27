package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/twentyideas/changesaga/internal/diagram"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/store"
)

func reviewDiagram() diagram.Document {
	d := diagram.New()
	d.Elements = []diagram.Element{
		{ID: "title", Kind: "text", Label: "Queue to Postgres", X: 60, Y: 28, Width: 600, Height: 48, Style: "title", Decorative: true},
		{ID: "queue", Kind: "node", Shape: "service", Label: "Enqueue", Icon: "lucide:server", X: 80, Y: 120, Width: 260, Height: 100, Style: "primary"},
		{ID: "table", Kind: "node", Shape: "datastore", Label: "Jobs table", X: 520, Y: 120, Width: 240, Height: 110, Style: "normal"},
		{ID: "insert", Kind: "edge", From: "queue", To: "table", Points: []diagram.Point{{X: 340, Y: 170}, {X: 520, Y: 170}}, Head: "arrow", Label: "insert", LabelBox: &diagram.Box{X: 380, Y: 132, Width: 100, Height: 26}, Style: "secondary"},
	}
	return d
}

// reviewSlideRequest is a diagram-sourced slide of review pr-7 whose Items
// select the two changed components. Review Items carry no evidence: cover
// references their changed lines after the slide is published.
func reviewSlideRequest(requestID, operation, expected string, d diagram.Document) SlideTransactionRequest {
	return SlideTransactionRequest{
		Version: 1, Operation: operation, RequestID: requestID, Review: "pr-7", ExpectedSnapshot: expected,
		Slide:   SlideTransactionSlide{ID: "flow", Title: "Queue to Postgres", Intent: "explain", Layout: "diagram", Takeaway: "Jobs are inserted into a Postgres table instead of SQS.", ReadingOrder: []string{"queue", "table"}},
		Diagram: &d,
		Items: []SlideTransactionItemRequest{
			{ID: "queue", Rank: 10, Kind: "node", Label: "Enqueue", Description: "Enqueue now names Postgres.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "queue"}, Record: "urn:change-saga:app:feature:" + testFeature},
			{ID: "table", Rank: 20, Kind: "node", Label: "Jobs table", Description: "The table jobs are stored in.", Selector: saga.LandmarkSelector{Type: "element", ElementID: "table"}},
		},
	}
}

func loadReviewSlide(t *testing.T, root, id string) *saga.Slide {
	t.Helper()
	document, validation, err := saga.Load(root)
	if err != nil || !validation.Valid {
		t.Fatalf("load: err=%v issues=%#v", err, validation.Issues)
	}
	slide := document.FindReview("pr-7").Slide(id)
	if slide == nil {
		t.Fatalf("review pr-7 has no slide %s", id)
	}
	return slide
}

func TestApplySlidePublishesDiagramSourcedReviewSlide(t *testing.T) {
	t.Parallel()
	fixture, _ := newEmptyReviewFixture(t)
	root, repo := fixture.root, fixture.repo
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	ctx := context.Background()
	deckDir := filepath.Join(saga.ReviewDir(root, "pr-7"), saga.ReviewDeckDir)
	create := reviewSlideRequest("flow-create", "create", "absent", reviewDiagram())

	dry, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, create, true)
	if err != nil || !dry.DryRun || !dry.Diff.DiagramChanged {
		t.Fatalf("dry-run = %#v err=%v", dry, err)
	}
	if matches, _ := filepath.Glob(filepath.Join(deckDir, "2[45]-*")); len(matches) != 0 {
		t.Fatalf("dry-run published %v", matches)
	}
	created, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, create, false)
	if err != nil {
		t.Fatal(err)
	}
	slideURN := saga.ReviewSlideTarget("app", "pr-7", "flow")
	queueURN, tableURN := saga.ReviewItemTarget("app", "pr-7", "flow", "queue"), saga.ReviewItemTarget("app", "pr-7", "flow", "table")
	if created.Target != slideURN || strings.Join(created.ChangedIDs, ",") != strings.Join([]string{slideURN, queueURN, tableURN}, ",") {
		t.Fatalf("create = %#v", created)
	}
	if retry, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, create, false); err != nil || !retry.Replayed || retry.Snapshot != created.Snapshot {
		t.Fatalf("retry = %#v err=%v", retry, err)
	}
	slide := loadReviewSlide(t, root, "flow")
	if slide.Diagram == nil || slide.MediaType != "image/svg+xml" || slide.AuthoringSnapshot != created.Snapshot || slide.Items[0].Record == "" {
		t.Fatalf("review slide = %#v", slide)
	}
	asset, err := os.ReadFile(filepath.Join(deckDir, slide.Entrypoint))
	if err != nil || !strings.Contains(string(asset), `id="queue"`) {
		t.Fatalf("rendered SVG: %v", err)
	}

	// Items bound to diagram elements take their code from cover, against
	// the review's own range.
	run(t, Cover, "--target", queueURN, "--path", "queue.go", "--changed-lines", "--repo", repo, root)
	run(t, Cover, "--target", tableURN, "--path", "store.go", "--changed-lines", "--repo", repo, root)
	assertValid(t, root)
	report := reviewReport(t, fixture)
	if report.Coverage == nil || report.Coverage.Summary.Total == 0 || report.Coverage.Summary.Uncovered != 0 {
		t.Fatalf("review coverage = %#v", report.Coverage)
	}
	covered := map[string]int{}
	for _, item := range report.Coverage.Items {
		covered[item.Target] = item.Covered
	}
	if covered[queueURN] == 0 || covered[tableURN] == 0 {
		t.Fatalf("diagram Items cover nothing: %#v", report.Coverage.Items)
	}

	// A targeted diagram edit republishes the slide and keeps each Item's
	// record and coverage.
	edit, err := runDiagram(t, `[{"op":"move","id":"table","dx":40},{"op":"update","id":"insert","set":{"points":[{"x":340,"y":170},{"x":560,"y":170}]}}]`,
		"edit", "--review", "pr-7", "--slide", "flow", "--expected", created.Snapshot, "--request-id", "move-table", "--from", "OPS", "--json", root)
	if err != nil {
		t.Fatalf("diagram edit: %v\n%s", err, edit)
	}
	var edited DiagramEditResult
	if err := json.Unmarshal([]byte(edit), &edited); err != nil || edited.Target != slideURN || strings.Join(edited.ChangedElements, ",") != "insert,table" {
		t.Fatalf("edit = %s err=%v", edit, err)
	}
	slide = loadReviewSlide(t, root, "flow")
	if slide.AuthoringSnapshot != edited.Snapshot || len(slide.Items) != 2 || len(slide.Items[0].Code) != 1 || len(slide.Items[1].Code) != 1 || slide.Items[0].Record == "" {
		t.Fatalf("edit lost Items, coverage, or records: %#v", slide.Items)
	}

	// The slide reads compactly by its URN, or by id with --review.
	for _, args := range [][]string{{"describe", "--slide", slideURN, root}, {"describe", "--review", "pr-7", "--slide", "flow", root}} {
		text, err := runDiagram(t, "", args...)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`Slide: ` + slideURN + ` "Queue to Postgres"`, "Snapshot: " + edited.Snapshot, `1. queue [node] "Enqueue" element=queue code_files=1`, `table "Jobs table" shape=datastore`, "insert: queue -> table"} {
			if !strings.Contains(text, want) {
				t.Errorf("describe %v lacks %q:\n%s", args, want, text)
			}
		}
	}
	if got, err := runDiagram(t, "", "get", "--slide", slideURN, "--id", "table", root); err != nil || !strings.Contains(got, `"snapshot": "`+edited.Snapshot) {
		t.Fatalf("get = %s err=%v", got, err)
	}
	if check, err := runDiagram(t, "", "check", "--review", "pr-7", "--json", root); err != nil || !strings.Contains(check, `"current": true`) || !strings.Contains(check, slideURN) {
		t.Fatalf("check = %s err=%v", check, err)
	}
	// query slide reads a review slide like an implementation slide, with
	// the authoring snapshot an update names.
	queried := run(t, Query, "slide", "--saga", root, "--repo", repo, "--target", slideURN)
	var slideQuery struct {
		Data struct {
			Target            string `json:"target"`
			AuthoringSnapshot string `json:"authoring_snapshot"`
			Items             []struct {
				Target string `json:"target"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(queried), &slideQuery); err != nil || slideQuery.Data.Target != slideURN || slideQuery.Data.AuthoringSnapshot != edited.Snapshot ||
		len(slideQuery.Data.Items) != 2 || slideQuery.Data.Items[0].Target != queueURN {
		t.Fatalf("query slide = %s err=%v", queried, err)
	}
	asJSON, err := runDiagram(t, "", "describe", "--slide", slideURN, "--format", "json", root)
	var described SlideDescription
	if err != nil || json.Unmarshal([]byte(asJSON), &described) != nil || described.Snapshot != edited.Snapshot || described.Source != "diagram" || len(described.Items) != 2 || described.Items[1].CodeFiles != 1 {
		t.Fatalf("json describe = %s err=%v", asJSON, err)
	}
}

func TestApplySlideGuardsReviewSlides(t *testing.T) {
	t.Parallel()
	fixture, head := newEmptyReviewFixture(t)
	root, repo := fixture.root, fixture.repo
	git(t, repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	ctx := context.Background()
	created, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, reviewSlideRequest("flow-create", "create", "absent", reviewDiagram()), false)
	if err != nil {
		t.Fatal(err)
	}
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "flow", "table"), "--path", "store.go", "--changed-lines", "--repo", repo, root)

	refused := map[string]func(*SlideTransactionRequest){
		"covered after publishing with `change-saga cover": func(r *SlideTransactionRequest) {
			r.Items[1].Evidence = []saga.CodeFile{{Version: saga.CurrentVersion}}
		},
		"story \"urn:change-saga:app:story:missing\" does not exist": func(r *SlideTransactionRequest) {
			r.Items[1].Record = "urn:change-saga:app:story:missing"
		},
		"still has coverage record": func(r *SlideTransactionRequest) {
			r.Items, r.Slide.ReadingOrder = r.Items[:1], []string{"queue"}
		},
		"is not review pr-7's deck":                       func(r *SlideTransactionRequest) { r.Deck = "implementation" },
		"is a pull request review's deck; set \"review\"": func(r *SlideTransactionRequest) { r.Review, r.Deck = "", "pr-7" },
	}
	for want, mutate := range refused {
		request := reviewSlideRequest("flow-refused", "update", created.Snapshot, reviewDiagram())
		mutate(&request)
		if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, request, false); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, reviewSlideRequest("flow-again", "create", "absent", reviewDiagram()), false); err == nil || !strings.Contains(err.Error(), `slide id "flow" already exists`) {
		t.Errorf("duplicate create: %v", err)
	}
	visual := filepath.Join(t.TempDir(), "slide.svg")
	writeFile(t, visual, reviewSlideSVG)
	for _, args := range [][]string{
		{"--review", "pr-7", "--slide", "flow", "--kind", "node", "--element-id", "queue", "--id", "extra", "--description", "More", root},
	} {
		if err := AddItem(ctx, args, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "apply-slide --review pr-7") {
			t.Errorf("add-item on a transaction review slide: %v", err)
		}
	}
	if err := SetSlideContent(ctx, []string{"--review", "pr-7", "--target", "flow", "--source", visual, root}, &strings.Builder{}); err == nil || !strings.Contains(err.Error(), "complete-slide transaction") {
		t.Errorf("set-slide-content on a transaction review slide: %v", err)
	}
	assertValid(t, root)

	// A merged review is history: its slides stay readable but not writable.
	manifestPath := filepath.Join(saga.ReviewDir(root, "pr-7"), saga.ReviewManifestName)
	var manifest saga.ReviewManifest
	if err := readStrictJSONPath(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Merged = &saga.ReviewMerge{Base: head, Head: head, Landed: head, MergedAt: manifest.CreatedAt}
	if err := store.WriteJSON(manifestPath, manifest, false); err != nil {
		t.Fatal(err)
	}
	update := reviewSlideRequest("flow-late", "update", created.Snapshot, reviewDiagram())
	if _, err := ApplySlideTransaction(ctx, root, t.TempDir(), repo, update, false); err == nil || !strings.Contains(err.Error(), "is history") {
		t.Errorf("merged review update: %v", err)
	}
	if _, err := runDiagram(t, "", "describe", "--review", "pr-7", "--slide", "flow", root); err != nil {
		t.Errorf("a merged review's slide must stay readable: %v", err)
	}
}

// A hand-written review slide migrates to a diagram source on its first
// apply-slide update; the coverage cover wrote keeps attaching to its Items.
func TestApplySlideMigratesHandWrittenReviewSlide(t *testing.T) {
	t.Parallel()
	fixture := newReviewFixture(t)
	root, repo := fixture.root, fixture.repo
	before := reviewReport(t, fixture).Coverage
	legacy := loadReviewSlide(t, root, "queue")
	if transactionManagedSlide(legacy) || len(legacy.Items) != 1 || len(legacy.Items[0].Code) != 1 {
		t.Fatalf("fixture slide = %#v", legacy)
	}
	d := diagram.New()
	d.Elements = []diagram.Element{{ID: "node", Kind: "node", Shape: "service", Label: "Enqueue", X: 80, Y: 120, Width: 260, Height: 100, Style: "primary"}}
	request := SlideTransactionRequest{
		Version: 1, Operation: "update", RequestID: "queue-diagram", Deck: saga.ReviewDeckTarget("app", "pr-7", legacy.DeckID), ExpectedSnapshot: legacy.AuthoringSnapshot,
		Slide:   SlideTransactionSlide{ID: "queue", Title: legacy.Title, Rank: legacy.Rank, Intent: legacy.Intent, Layout: legacy.Layout, Takeaway: legacy.Takeaway, ReadingOrder: []string{"node"}},
		Diagram: &d,
		Items:   []SlideTransactionItemRequest{{ID: "node", Rank: legacy.Items[0].Rank, Kind: "node", Label: "Enqueue", Description: "The changed code", Selector: saga.LandmarkSelector{Type: "element", ElementID: "node"}}},
	}
	result, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), repo, request, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.PreviousSnapshot != legacy.AuthoringSnapshot || result.Target != legacy.Target {
		t.Fatalf("migration = %#v", result)
	}
	migrated := loadReviewSlide(t, root, "queue")
	if !transactionManagedSlide(migrated) || migrated.Diagram == nil || len(migrated.Items) != 1 || len(migrated.Items[0].Code) != 1 {
		t.Fatalf("migrated slide lost its diagram or coverage: %#v", migrated)
	}
	if other := loadReviewSlide(t, root, "table"); transactionManagedSlide(other) || len(other.Items[0].Code) != 1 {
		t.Fatalf("the other hand-written slide changed: %#v", other)
	}
	report := reviewReport(t, fixture)
	if before == nil || report.Coverage == nil || report.Coverage.Summary != before.Summary || len(report.Coverage.Items) != 2 {
		t.Fatalf("coverage after migration = %#v, before %#v", report.Coverage, before)
	}
}

// A review Item published by apply-slide is covered exactly like one added
// with add-item: cover writes the same flat evidence records for both, and
// the comparison's review area (status and check --covers review) reads
// them. A reader that skips a transaction Item's flat records would score an
// apply-slide review deck as explaining none of its change.
func TestReviewCoverageIsTheSameForApplySlideItems(t *testing.T) {
	t.Parallel()
	byAddItem := newReviewOnlyFixture(t, true)
	byApplySlide := newReviewOnlyFixture(t, false)
	run(t, Review, "create", "--id", "pr-7", "--base", "main", byApplySlide.root)
	d := diagram.New()
	d.Elements = []diagram.Element{{ID: "node", Kind: "node", Shape: "service", Label: "Queue", X: 80, Y: 120, Width: 260, Height: 100, Style: "primary"}}
	request := SlideTransactionRequest{
		Version: 1, Operation: "create", RequestID: "queue-create", Review: "pr-7", ExpectedSnapshot: "absent",
		Slide:   SlideTransactionSlide{ID: "queue", Title: "The queue moves to a table", Intent: "explain", Layout: "diagram", Takeaway: "Jobs are stored in Postgres.", ReadingOrder: []string{"node"}},
		Diagram: &d,
		Items:   []SlideTransactionItemRequest{{ID: "node", Rank: 10, Kind: "node", Label: "Queue", Description: "The queue moves to a table", Selector: saga.LandmarkSelector{Type: "element", ElementID: "node"}}},
	}
	if _, err := ApplySlideTransaction(context.Background(), byApplySlide.root, t.TempDir(), byApplySlide.repo, request, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"queue.go", "store.go"} {
		run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "queue", "node"), "--path", path, "--changed-lines", "--repo", byApplySlide.repo, byApplySlide.root)
	}
	assertValid(t, byApplySlide.root)

	want := statusJSON(t, byAddItem.root, "--repo", byAddItem.repo, "--against", "main").Coverage.Areas.Review
	got := statusJSON(t, byApplySlide.root, "--repo", byApplySlide.repo, "--against", "main").Coverage.Areas.Review
	if !want.Complete || want.Total != 6 || got.Total != want.Total || got.Covered != want.Covered || got.Complete != want.Complete {
		t.Fatalf("apply-slide review area = %+v, add-item review area = %+v", got, want)
	}
	if out := run(t, Check, "--covers", "review", "--repo", byApplySlide.repo, "--against", "main", byApplySlide.root); !strings.Contains(out, "fully covered") {
		t.Fatalf("check --covers review of an apply-slide review deck:\n%s", out)
	}
}

// publishedReviewSlide is newEmptyReviewFixture with review pr-7's
// diagram-sourced flow slide published and its two Items covered.
func publishedReviewSlide(t *testing.T) (reviewFixture, SlideTransactionResult) {
	t.Helper()
	fixture, _ := newEmptyReviewFixture(t)
	git(t, fixture.repo, "remote", "add", "origin", "https://example.test/acme/app.git")
	created, err := ApplySlideTransaction(context.Background(), fixture.root, t.TempDir(), fixture.repo, reviewSlideRequest("flow-create", "create", "absent", reviewDiagram()), false)
	if err != nil {
		t.Fatal(err)
	}
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "flow", "queue"), "--path", "queue.go", "--changed-lines", "--repo", fixture.repo, fixture.root)
	run(t, Cover, "--target", saga.ReviewItemTarget("app", "pr-7", "flow", "table"), "--path", "store.go", "--changed-lines", "--repo", fixture.repo, fixture.root)
	return fixture, created
}

// apply-slide --print-current prints a review slide's request into its
// review, with each Item's record, so it applies back unchanged and an edit
// of one field publishes.
func TestApplySlidePrintsCurrentReviewSlide(t *testing.T) {
	t.Parallel()
	fixture, created := publishedReviewSlide(t)
	root := fixture.root
	var printed SlideTransactionRequest
	if err := json.Unmarshal([]byte(run(t, ApplySlide, "--print-current", "flow", "--review", "pr-7", root)), &printed); err != nil {
		t.Fatal(err)
	}
	if printed.Review != "pr-7" || printed.Deck != "" || printed.ExpectedSnapshot != created.Snapshot || printed.Items[0].Record == "" || len(printed.Items[0].Evidence) != 0 {
		t.Fatalf("printed request = %+v", printed)
	}
	byURN := run(t, ApplySlide, "--print-current", saga.ReviewSlideTarget("app", "pr-7", "flow"), root)
	if !strings.Contains(byURN, `"review": "pr-7"`) {
		t.Fatalf("print-current by URN:\n%s", byURN)
	}
	unchanged, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, printed, false)
	if err != nil || !unchanged.Unchanged || unchanged.Snapshot != created.Snapshot {
		t.Fatalf("applying the printed request = %+v err=%v", unchanged, err)
	}
	printed.Items[1].Label = "Postgres jobs table"
	printed.RequestID = "flow-relabel"
	edited, err := ApplySlideTransaction(context.Background(), root, t.TempDir(), fixture.repo, printed, false)
	if err != nil || edited.Unchanged || edited.Snapshot == created.Snapshot {
		t.Fatalf("an edited printed request = %+v err=%v", edited, err)
	}
	if slide := loadReviewSlide(t, root, "flow"); slide.Items[1].Label != "Postgres jobs table" || len(slide.Items[1].Code) != 1 {
		t.Fatalf("relabelled slide lost coverage: %+v", slide.Items[1])
	}
}
