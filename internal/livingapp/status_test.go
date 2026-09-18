package livingapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/diffuri"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

const (
	fixtureSaga = "checkout"
	fixtureRepo = "https://example.com/repo.git"
	storyURN    = "urn:change-saga:checkout:story:refund"
	storyR1     = "urn:change-saga:checkout:story:refund:revision:r1"
	storyR2     = "urn:change-saga:checkout:story:refund:revision:r2"
)

var statusFixtureTime = time.Date(2026, 9, 17, 20, 0, 0, 0, time.UTC)

func criterionURN(id string) string { return storyURN + ":criterion:" + id }
func testURN(id string) string      { return "urn:change-saga:checkout:test-case:" + id }
func testRevisionURN(testCase, revision string) string {
	return testURN(testCase) + ":revision:" + revision
}
func relationURN(id string) string { return "urn:change-saga:checkout:relation:" + id }

func selector(t *testing.T, path string, start, end int) string {
	t.Helper()
	value, err := diffuri.Build(diffuri.Reference{Repository: fixtureRepo, Base: "base", Head: "head", Kind: "line", Path: path, Side: "new", Start: start, End: end})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// testCaseSpec describes one hand-authored v5 test-case package.
type testCaseSpec struct {
	id          string
	kinds       []quality.CoverageKind
	automation  quality.Automation
	revisions   []string
	state       quality.LifecycleState
	runRevision string
	result      quality.RunResult
}

func writeStatusJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newV5QualitySaga writes a v5 manifest and ___quality tree in a temp dir. The
// core Saga loader does not accept v5 yet, so only quality.Load reads it.
func newV5QualitySaga(t *testing.T, specs []testCaseSpec, policies []quality.Policy) string {
	t.Helper()
	root := t.TempDir()
	writeStatusJSON(t, filepath.Join(root, "saga.json"), map[string]any{
		"$schema": quality.ManifestSchemaURL, "version": quality.Version, "id": fixtureSaga, "title": "Checkout",
		"source": quality.SourceIdentity{Repository: fixtureRepo, Base: "base", Head: "head"},
	})
	for _, dir := range []string{"policies", "test-cases"} {
		if err := os.MkdirAll(filepath.Join(root, quality.RootDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, policy := range policies {
		writeStatusJSON(t, filepath.Join(root, quality.RootDir, "policies", policy.ID+".json"), policy)
	}
	for _, spec := range specs {
		dir := filepath.Join(root, quality.RootDir, "test-cases", spec.id+".test")
		for _, sub := range []string{"revisions", "events", "evidence", "runs"} {
			if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		writeStatusJSON(t, filepath.Join(dir, "test-case.json"), quality.TestCaseIdentity{Schema: quality.TestCaseSchemaURL, Version: quality.Version, ID: spec.id, CreatedAt: statusFixtureTime})
		parents := []string{}
		for index, revision := range spec.revisions {
			automation := spec.automation
			if automation == "" {
				automation = quality.AutomationAutomated
			}
			writeStatusJSON(t, filepath.Join(dir, "revisions", revision+".json"), quality.Revision{
				Schema: quality.RevisionSchemaURL, Version: quality.Version, ID: revision, TestCase: testURN(spec.id), Parents: parents,
				Title: spec.id + " " + revision, CoverageKinds: spec.kinds, Automation: automation, Preconditions: []string{},
				Steps:          []quality.Step{{ID: "act", Action: "Act on the refund.", ExpectedResult: "The documented outcome occurs."}},
				ExpectedResult: "The documented outcome occurs.", CreatedAt: statusFixtureTime.Add(time.Duration(index) * time.Minute),
			})
			parents = []string{testRevisionURN(spec.id, revision)}
		}
		writeStatusJSON(t, filepath.Join(dir, "events", "proposed.json"), quality.LifecycleEvent{
			Schema: quality.LifecycleEventSchemaURL, Version: quality.Version, ID: "proposed", TestCase: testURN(spec.id), Parents: []string{},
			State: quality.StateProposed, CreatedAt: statusFixtureTime,
		})
		if spec.state != quality.StateProposed {
			writeStatusJSON(t, filepath.Join(dir, "events", "active.json"), quality.LifecycleEvent{
				Schema: quality.LifecycleEventSchemaURL, Version: quality.Version, ID: "active", TestCase: testURN(spec.id),
				Parents: []string{testURN(spec.id) + ":event:proposed"}, State: quality.StateActive, CreatedAt: statusFixtureTime.Add(time.Minute),
			})
		}
		if spec.runRevision == "" {
			continue
		}
		evidence := testURN(spec.id) + ":evidence:code"
		writeStatusJSON(t, filepath.Join(dir, "evidence", "code.json"), quality.Evidence{
			Schema: quality.EvidenceSchemaURL, Version: quality.Version, ID: "code", TestCase: testURN(spec.id),
			TestRevision: testRevisionURN(spec.id, spec.runRevision), Role: quality.EvidenceTestImplementation,
			Diffs: []string{selector(t, "internal/refund_test.go", 1, 5)}, Verifications: []string{}, Citations: []string{}, Supersedes: []string{}, CreatedAt: statusFixtureTime,
		})
		writeStatusJSON(t, filepath.Join(dir, "runs", "ci-1.json"), quality.Run{
			Schema: quality.RunSchemaURL, Version: quality.Version, ID: "ci-1", TestCase: testURN(spec.id),
			TestRevision: testRevisionURN(spec.id, spec.runRevision), Parents: []string{},
			Source: quality.SourceIdentity{Repository: fixtureRepo, Base: "base", Head: "head"}, Result: spec.result,
			Summary: "CI completed.", Command: "go test ./...", Evidence: []string{evidence}, ExecutedAt: statusFixtureTime.Add(time.Hour),
		})
	}
	return root
}

// refundStory is an accepted story at r2 whose criteria exercise every quality
// state. It is built in memory because the requirements loader is v3-only.
func refundStory(criteria ...string) requirements.Story {
	revision := func(id string, parents []string) requirements.Revision {
		value := requirements.Revision{ID: id, Story: storyURN, Parents: parents, Title: "Refund window", Statement: "As a buyer I can request a refund.", Priority: "must"}
		for _, criterion := range criteria {
			value.AcceptanceCriteria = append(value.AcceptanceCriteria, requirements.Criterion{ID: criterion, Statement: "The " + criterion + " behavior holds."})
		}
		return value
	}
	r2 := revision("r2", []string{storyR1})
	accepted := requirements.LifecycleEvent{ID: "accepted", Story: storyURN, State: requirements.StateAccepted}
	return requirements.Story{
		Identity: requirements.StoryIdentity{ID: "refund"}, Revisions: []requirements.Revision{revision("r1", []string{}), r2},
		Events: []requirements.LifecycleEvent{accepted}, RevisionHeads: []string{storyR2}, LifecycleHeads: []string{storyURN + ":event:accepted"},
		CurrentRevision: &r2, CurrentLifecycle: &accepted,
	}
}

func verifies(id, testCase, testRevision, criterion, storyRevision string) requirements.Relation {
	return requirements.Relation{
		Version: requirements.V5RelationVersion, ID: id, Type: requirements.RelationVerifies, From: testURN(testCase), To: criterionURN(criterion),
		Scope: requirements.ScopeSelf, FromRevision: testRevisionURN(testCase, testRevision), ToRevision: storyRevision, State: requirements.RelationActive,
	}
}

// fixtureLinks judges relations exactly as the loader does: currency comes
// only from requirements.EvaluateRelations, with every test-case head supplied.
func fixtureLinks(stories []requirements.Story, relations []requirements.Relation, document quality.Document) []Link {
	requirementsDocument := requirements.Document{SagaID: fixtureSaga, SagaVersion: quality.Version, Stories: stories, Relations: relations}
	inputs := requirements.StaleInputs{}
	heads := map[string][]string{}
	for _, testCase := range document.TestCases {
		heads[testCase.Identity.ID] = testCase.RevisionHeads
	}
	inputs.SetTestCaseHeads(fixtureSaga, heads)
	return LinksFromCurrency(requirementsDocument, requirements.EvaluateRelations(requirementsDocument, inputs))
}

func testAtoms(t *testing.T) gitdiff.ChangeSet {
	t.Helper()
	changes := gitdiff.ChangeSet{Repository: fixtureRepo, Base: "base", Head: "head", BaseOID: "base", HeadOID: "head"}
	add := func(path string, line int) {
		uri := selector(t, path, line, line)
		changes.Atoms = append(changes.Atoms, gitdiff.Atom{Key: uri, URI: uri, Kind: "line", Path: path, Side: "new", Line: line, Content: "x"})
	}
	for line := 1; line <= 5; line++ {
		add("internal/refund_test.go", line)
	}
	add("internal/refund.go", 10)
	add("internal/other.go", 1)
	return changes
}

// qualityFixture assembles the v5 quality scenario: one criterion per state.
func qualityFixture(t *testing.T) StatusInputs {
	t.Helper()
	positive := []quality.CoverageKind{quality.CoveragePositive}
	root := newV5QualitySaga(t, []testCaseSpec{
		{id: "happy", kinds: positive, revisions: []string{"r1"}, runRevision: "r1", result: quality.RunPassed},
		{id: "boundary", kinds: []quality.CoverageKind{quality.CoverageNegative, quality.CoverageEdge}, revisions: []string{"r1"}, runRevision: "r1", result: quality.RunPassed},
		{id: "revised", kinds: positive, revisions: []string{"r1", "r2"}, runRevision: "r1", result: quality.RunPassed},
		{id: "broken", kinds: positive, revisions: []string{"r1"}, runRevision: "r1", result: quality.RunFailed},
		{id: "unrun", kinds: positive, revisions: []string{"r1"}},
		{id: "orphan", kinds: positive, revisions: []string{"r1"}, runRevision: "r1", result: quality.RunPassed},
	}, []quality.Policy{{
		Schema: quality.PolicySchemaURL, Version: quality.Version, ID: "cutoff-kinds", Criterion: criterionURN("cutoff"), StoryRevision: storyR2,
		RequiredKinds:     []quality.CoverageKind{quality.CoveragePositive, quality.CoverageNegative, quality.CoverageEdge},
		AllowedAutomation: []quality.Automation{quality.AutomationAutomated, quality.AutomationManual}, Supersedes: []string{}, Rationale: "The boundary has distinct risks.", CreatedAt: statusFixtureTime,
	}})
	document, err := quality.Load(root)
	if err != nil {
		t.Fatalf("hand-authored v5 fixture must load: %v", err)
	}
	item := &saga.Item{Target: "urn:change-saga:checkout:slide:guard:item:deadline", Diffs: []saga.DiffFile{{
		Path: "___slides/implementation.deck/40-e-guard.json", Diffs: []saga.DiffReference{{URI: selector(t, "internal/refund.go", 10, 10)}},
	}}}
	deck := &saga.Deck{DeckManifest: saga.DeckManifest{ID: "implementation", Role: "change"}, Target: "urn:change-saga:checkout:deck:implementation",
		Slides: []*saga.Slide{{Target: "urn:change-saga:checkout:slide:guard", Items: []*saga.Item{item}}}}
	changes := testAtoms(t)
	report := coverage.Report{SchemaValid: true, Uncovered: []gitdiff.Atom{}, Orphans: []coverage.Orphan{}}
	for _, atom := range changes.Atoms {
		if atom.Path != "internal/refund.go" {
			report.Uncovered = append(report.Uncovered, atom)
		}
	}
	stories := []requirements.Story{refundStory("positive-path", "cutoff", "stale-run", "failing", "not-run", "untested", "manual-only")}
	return StatusInputs{
		SagaID: fixtureSaga, SagaVersion: quality.Version, RequirementsAdopted: true,
		Stories:   stories,
		Citations: []requirements.Citation{{ID: "policy"}},
		Links: fixtureLinks(stories, []requirements.Relation{
			verifies("happy-positive", "happy", "r1", "positive-path", storyR2),
			verifies("happy-positive-old", "happy", "r1", "positive-path", storyR1),
			verifies("boundary-cutoff", "boundary", "r1", "cutoff", storyR2),
			verifies("revised-stale-run", "revised", "r2", "stale-run", storyR2),
			verifies("broken-failing", "broken", "r1", "failing", storyR2),
			verifies("unrun-not-run", "unrun", "r1", "not-run", storyR2),
			{
				Version: requirements.V5RelationVersion, ID: "guard-addresses-positive", Type: requirements.RelationAddresses, From: item.Target, To: criterionURN("positive-path"),
				Scope: requirements.ScopeSelf, FromContentDigest: "sha256:" + strings.Repeat("a", 64), ToRevision: storyR2, State: requirements.RelationActive,
			},
		}, document),
		Decks: []*saga.Deck{deck},
		Exceptions: []coverage.Exception{{
			URN: "urn:change-saga:checkout:coverage-exception:manual-only-quality", Axis: coverage.AxisQuality, Criterion: criterionURN("manual-only"),
			StoryRevision: storyR2, Rationale: "Verified by an operational playbook outside this change.",
			Citations: []string{"urn:change-saga:checkout:citation:policy"}, Supersedes: []string{}, CreatedAt: statusFixtureTime,
		}},
		ExceptionsAdopted: true,
		Quality:           document,
		Report:            report,
		Changes:           changes,
	}
}

func cell(t *testing.T, status Status, criterion string, axis coverage.Axis) coverage.AxisCoverage {
	t.Helper()
	row, ok := status.Axes.Criterion(criterionURN(criterion))
	if !ok {
		t.Fatalf("criterion %s missing from axes", criterion)
	}
	value, ok := row.Axis(axis)
	if !ok {
		t.Fatalf("criterion %s has no %s cell", criterion, axis)
	}
	return value
}

func kind(t *testing.T, status Status, criterion, name string) KindStatus {
	t.Helper()
	for _, row := range status.Quality.Criteria {
		if row.Criterion != criterionURN(criterion) {
			continue
		}
		for _, value := range row.Kinds {
			if value.Kind == name {
				return value
			}
		}
	}
	t.Fatalf("criterion %s has no %s kind row", criterion, name)
	return KindStatus{}
}

func TestQualityAxisReflectsTestCasesKindsRunsAndExceptions(t *testing.T) {
	status := Assemble(qualityFixture(t))

	if status.Policy.Name != readiness.PolicyFeature {
		t.Fatalf("a v5 Saga that adopted quality selects the feature policy, got %#v", status.Policy)
	}
	if got := cell(t, status, "positive-path", coverage.AxisQuality); got.State != coverage.StateCoveredDirect {
		t.Fatalf("a current passing positive test covers the criterion: %#v", got)
	}
	if got := kind(t, status, "positive-path", "positive"); got.State != kindCovered || !reflect.DeepEqual(got.TestCases, []string{testURN("happy")}) {
		t.Fatalf("positive kind = %#v", got)
	}

	cutoff := cell(t, status, "cutoff", coverage.AxisQuality)
	if cutoff.State != coverage.StateGap || !containsText(cutoff.Unsatisfied, "required positive test: missing_kind") {
		t.Fatalf("a policy-required kind no test declares keeps the cell a gap with its reason: %#v", cutoff)
	}
	if kind(t, status, "cutoff", "negative").State != kindCovered || kind(t, status, "cutoff", "edge").State != kindCovered {
		t.Fatal("declared negative and edge kinds with a current pass are covered")
	}

	staleRun := cell(t, status, "stale-run", coverage.AxisQuality)
	if staleRun.State != coverage.StateStale || !containsText(staleRun.StalePins, "test revision changed") {
		t.Fatalf("a run pinned to an older test revision is stale: %#v", staleRun)
	}
	if got := kind(t, status, "failing", "positive"); got.State != kindFailed {
		t.Fatalf("a failing current run is reported as failed: %#v", got)
	}
	if got := cell(t, status, "failing", coverage.AxisQuality); got.State != coverage.StateGap || !containsText(got.Unsatisfied, "failed") {
		t.Fatalf("a failing run never covers the criterion: %#v", got)
	}
	if got := kind(t, status, "not-run", "positive"); got.State != kindNotRun {
		t.Fatalf("a linked test without runs is not_run: %#v", got)
	}
	if got := kind(t, status, "untested", "positive"); got.State != kindMissing {
		t.Fatalf("a criterion without tests is missing its kind: %#v", got)
	}
	if got := cell(t, status, "manual-only", coverage.AxisQuality); got.State != coverage.StateExcluded {
		t.Fatalf("a current quality exception excludes the axis: %#v", got)
	}
	if got := kind(t, status, "manual-only", "positive"); got.State != kindExcluded {
		t.Fatalf("an excluded criterion reports its required kinds as excluded: %#v", got)
	}
	for _, fact := range status.Quality.Facts {
		if fact.Criterion == criterionURN("manual-only") && fact.Required {
			t.Fatalf("an excluded criterion must not contribute required quality facts: %#v", fact)
		}
	}

	orphaned := false
	for _, testCase := range status.Quality.TestCases {
		if testCase.TestCase == testURN("orphan") {
			orphaned = testCase.Orphaned
		}
	}
	if !orphaned {
		t.Fatal("a test case with no verifies relation is reported as orphaned")
	}

	gate, _ := status.Readiness.Gate(readiness.GateQualityReady)
	if gate.Status != readiness.StatusBlocked {
		t.Fatalf("quality_ready must block on failed, stale, unrun, and missing kinds: %#v", gate.Status)
	}
	review, _ := status.Readiness.Gate(readiness.GateReadyForReview)
	if !blockedBy(review, "no_failed_required_run") {
		t.Fatalf("ready_for_review names the failed required run: %#v", review.Blockers)
	}
}

func TestStaleSetIsPinDerivedAndNamesWhichHistoryMoved(t *testing.T) {
	status := Assemble(qualityFixture(t))
	stale := map[string]StaleRecord{}
	for _, record := range status.Stale {
		stale[record.Record] = record
	}
	relation, ok := stale[relationURN("happy-positive-old")]
	if !ok || relation.History != historyReview || relation.Type != "verifies" {
		t.Fatalf("a relation pinned to r1 after the story moved to r2 is stale review history: %#v", status.Stale)
	}
	if !hasPin(relation.Pins, "to_revision", storyR1, storyR2) {
		t.Fatalf("the stale record shows the pinned and current revision: %#v", relation.Pins)
	}
	if !reflect.DeepEqual(relation.Affects, []string{criterionURN("positive-path")}) {
		t.Fatalf("the stale record names the criterion it affects: %#v", relation.Affects)
	}
	run, ok := stale[testURN("revised")+":run:ci-1"]
	if !ok || run.Kind != "test_run" || run.History != historyReview || !containsText(run.Reasons, "test revision changed") {
		t.Fatalf("a run pinned to a superseded test revision is stale: %#v", run)
	}
	if !hasPin(run.Pins, "test_revision", testRevisionURN("revised", "r1"), testRevisionURN("revised", "r2")) {
		t.Fatalf("the stale run shows its revision pin: %#v", run.Pins)
	}
	if _, ok := stale[relationURN("happy-positive")]; ok {
		t.Fatal("a relation whose pins match the current heads is not stale")
	}
}

func TestChangedSourceAccountingIsSeparateAndCreditsOnlyTestEvidence(t *testing.T) {
	status := Assemble(qualityFixture(t))
	source := status.ChangedSource
	if len(source.TestOwned) != 5 || source.UncoveredAtoms != 1 || len(source.Uncovered) != 1 || source.Uncovered[0] != (UncoveredPath{Path: "internal/other.go", Atoms: 1}) {
		t.Fatalf("current test-code evidence owns only its own atoms: owned %#v uncovered %#v", source.TestOwned, source.Uncovered)
	}
	if source.Complete {
		t.Fatal("an unowned changed atom keeps changed-source accounting incomplete")
	}
	gate, _ := status.Readiness.Gate(readiness.GateImplementationTraceReady)
	if !blockedBy(gate, "changed_source_accounting_complete") {
		t.Fatalf("implementation_trace_ready keeps the omission invariant as its own fact: %#v", gate.Blockers)
	}
	if got := cell(t, status, "positive-path", coverage.AxisImplementation); got.State != coverage.StateCoveredDirect || len(got.Links[0].Link.Diffs) != 1 {
		t.Fatalf("criterion <- addresses - Item -> exact current diff covers implementation: %#v", got)
	}
	if got := cell(t, status, "cutoff", coverage.AxisImplementation); got.State != coverage.StateGap {
		t.Fatalf("no recorded path means an implementation gap: %#v", got)
	}
}

func TestImplementationPathWithOrphanedDiffIsStaleSourceHistory(t *testing.T) {
	inputs := qualityFixture(t)
	item := inputs.Decks[0].Slides[0].Items[0]
	inputs.Report.Orphans = []coverage.Orphan{{
		Assignment: coverage.Assignment{Target: item.Target, DiffFile: item.Diffs[0].Path, Diff: 1},
		Reference:  item.Diffs[0].Diffs[0], Reason: "diff URI does not match the current source comparison",
	}}
	status := Assemble(inputs)
	if got := cell(t, status, "positive-path", coverage.AxisImplementation); got.State != coverage.StateStale {
		t.Fatalf("an Item whose only diff no longer matches the comparison is stale, not covered: %#v", got)
	}
	found := false
	for _, record := range status.Stale {
		if record.Kind == "diff_selector" && record.History == historySource && reflect.DeepEqual(record.Affects, []string{criterionURN("positive-path")}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the orphaned selector is stale source history naming the criterion: %#v", status.Stale)
	}
	implicated := false
	for _, value := range status.ChangedSource.Implicated {
		implicated = implicated || value.Resource == criterionURN("positive-path")
	}
	if !implicated {
		t.Fatalf("the source change implicates the criterion reached through the Item: %#v", status.ChangedSource.Implicated)
	}
}

func TestV3QualityIsAVisibleNotAdoptedState(t *testing.T) {
	inputs := qualityFixture(t)
	inputs.SagaVersion = 3
	inputs.Quality = quality.Document{SagaID: fixtureSaga, Adoption: quality.NotAdopted}
	inputs.QualityReason = "quality records are v5; this is a v3 Saga"
	inputs.Exceptions = nil
	inputs.ExceptionsAdopted = false
	status := Assemble(inputs)

	if status.Policy.Name != readiness.PolicyCompatibility || status.Readiness.PeerReview == nil {
		t.Fatalf("a v3 Saga keeps peer-review readiness by default: %#v", status.Policy)
	}
	if status.Quality.Adoption != string(quality.NotAdopted) {
		t.Fatalf("quality adoption = %q", status.Quality.Adoption)
	}
	got := cell(t, status, "positive-path", coverage.AxisQuality)
	if got.State != coverage.StateGap || !containsText(got.Gap.Reasons, "not_adopted") {
		t.Fatalf("an unadopted quality axis is a visible gap with its reason, not a pass: %#v", got)
	}
	gate, _ := status.Readiness.Gate(readiness.GateQualityReady)
	if gate.Configured || gate.Status != readiness.StatusNotApplicable || !blockedBy(gate, "quality_adopted") {
		t.Fatalf("quality_ready is reported, unconfigured, and names adoption: %#v", gate)
	}
}

func TestUIAxisIsNeverSilentlyCovered(t *testing.T) {
	status := Assemble(qualityFixture(t))
	got := cell(t, status, "positive-path", coverage.AxisUI)
	if got.State != coverage.StateGap || !containsText(got.Unsatisfied, "no UI reference resource") {
		t.Fatalf("with no UI resource kind, the ui axis is a gap naming why: %#v", got)
	}
}

func TestStatusIsDeterministicAndHasNoReducingNumber(t *testing.T) {
	first, err := json.Marshal(Assemble(qualityFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(Assemble(qualityFixture(t)))
	if err != nil {
		t.Fatal(err)
	}
	var left, right any
	_ = json.Unmarshal(first, &left)
	_ = json.Unmarshal(second, &right)
	if !reflect.DeepEqual(left, right) {
		t.Fatal("the same records must produce the same status")
	}
	assertNoReducingKey(t, left, "")
}

var reducingKeys = map[string]bool{
	"percent": true, "percentage": true, "score": true, "ratio": true, "fraction": true,
	"progress": true, "readiness_score": true, "coverage_percent": true, "overall": true,
}

func assertNoReducingKey(t *testing.T, value any, path string) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if reducingKeys[key] {
				t.Errorf("%s/%s reduces status to one opaque number", path, key)
			}
			assertNoReducingKey(t, child, path+"/"+key)
		}
	case []any:
		for _, child := range value {
			assertNoReducingKey(t, child, path+"/[]")
		}
	}
}

func TestImpactGraphFollowsRecordedEdgesToCriteriaAndTestCases(t *testing.T) {
	graph := ImpactGraph(qualityFixture(t))
	item := "urn:change-saga:checkout:slide:guard:item:deadline"
	reaches := graph.Requirements[item]
	if len(reaches) != 1 || reaches[0].Requirement != criterionURN("positive-path") || reaches[0].Relation != relationURN("guard-addresses-positive") {
		t.Fatalf("an Item reaches the criterion its addresses relation names: %#v", reaches)
	}
	found := false
	for _, evidence := range graph.TestCases {
		if evidence.TestCase == testURN("happy") {
			found = reflect.DeepEqual(evidence.Criteria, []string{criterionURN("positive-path")}) && len(evidence.Diffs) == 1
		}
	}
	if !found {
		t.Fatalf("test-case evidence carries the criteria its verifies relations name: %#v", graph.TestCases)
	}
}

func containsText(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}

func hasPin(pins []Pin, field, pinned, current string) bool {
	for _, pin := range pins {
		if pin.Field == field && pin.Pinned == pinned && pin.Current == current {
			return true
		}
	}
	return false
}

func blockedBy(gate readiness.Gate, code string) bool {
	for _, blocker := range gate.Blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}
