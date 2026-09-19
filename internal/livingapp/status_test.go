package livingapp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/quality"
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

// statusFixtureEpic holds every hand-authored status fixture record.
const statusFixtureEpic = "refunds"

func criterionURN(id string) string { return storyURN + ":criterion:" + id }
func testURN(id string) string      { return "urn:change-saga:checkout:test-case:" + id }
func testRevisionURN(testCase, revision string) string {
	return testURN(testCase) + ":revision:" + revision
}
func relationURN(id string) string { return "urn:change-saga:checkout:relation:" + id }

// The fixture comparison's commits. References are pinned at the head, and
// coderesolve.Pinned finds a reference current exactly at its own commit.
var (
	fixtureBase = strings.Repeat("b", 40)
	fixtureHead = strings.Repeat("a", 40)
)

func selector(t *testing.T, path string, start, end int) coderef.Reference {
	t.Helper()
	return coderef.Reference{Commit: fixtureHead, Path: path, Start: start, End: end, Digest: coderef.DigestBytes([]byte(path))}
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

// newV5QualitySaga writes a v5 manifest and an epic's ___quality tree in a temp dir. The
// core Saga loader does not accept v5 yet, so only quality.Load reads it.
func newV5QualitySaga(t *testing.T, specs []testCaseSpec, policies []quality.Policy) string {
	t.Helper()
	root := t.TempDir()
	writeStatusJSON(t, filepath.Join(root, "saga.json"), map[string]any{
		"$schema": quality.ManifestSchemaURL, "version": quality.Version, "id": fixtureSaga, "title": "Checkout",
		"source": map[string]string{"repository": fixtureRepo},
	})
	if _, err := applayout.WriteEpic(root, applayout.EpicManifest{ID: statusFixtureEpic, Title: "Refunds", CreatedAt: statusFixtureTime}); err != nil {
		t.Fatal(err)
	}
	qualityRoot := filepath.Join(applayout.EpicDir(root, statusFixtureEpic), quality.RootDir)
	for _, dir := range []string{"policies", "test-cases"} {
		if err := os.MkdirAll(filepath.Join(qualityRoot, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, policy := range policies {
		writeStatusJSON(t, filepath.Join(qualityRoot, "policies", policy.ID+".json"), policy)
	}
	for _, spec := range specs {
		dir := filepath.Join(qualityRoot, "test-cases", spec.id+".test")
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
			Code: []coderef.Reference{selector(t, "internal/refund_test.go", 1, 5)}, Verifications: []string{}, Citations: []string{}, Supersedes: []string{}, CreatedAt: statusFixtureTime,
		})
		writeStatusJSON(t, filepath.Join(dir, "runs", "ci-1.json"), quality.Run{
			Schema: quality.RunSchemaURL, Version: quality.Version, ID: "ci-1", TestCase: testURN(spec.id),
			TestRevision: testRevisionURN(spec.id, spec.runRevision), Parents: []string{},
			Source: quality.SourceIdentity{Repository: fixtureRepo, Commit: "0123456789abcdef0123456789abcdef01234567"}, Result: spec.result,
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
	requirementsDocument := requirements.Document{SagaID: fixtureSaga, Stories: stories, Relations: relations}
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
	changes := gitdiff.ChangeSet{Repository: fixtureRepo, Base: "base", Head: "head", BaseOID: fixtureBase, HeadOID: fixtureHead}
	add := func(path string, line int) {
		atom := gitdiff.Atom{Kind: "line", Path: path, Side: "new", Line: line, Content: "x"}
		atom.Key, atom.Ref = gitdiff.Key(atom), changes.Location(atom).String()
		changes.Atoms = append(changes.Atoms, atom)
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
	item := &saga.Item{Target: "urn:change-saga:checkout:slide:guard:item:deadline", Code: []saga.CodeFile{{
		Path: "___slides/implementation.deck/40-e-guard.json", References: []coderef.Reference{selector(t, "internal/refund.go", 10, 10)},
	}}}
	deck := &saga.Deck{DeckManifest: saga.DeckManifest{ID: "implementation", Role: "change"}, Target: "urn:change-saga:checkout:deck:implementation",
		Slides: []*saga.Slide{{Target: "urn:change-saga:checkout:slide:guard", Items: []*saga.Item{item}}}}
	changes := testAtoms(t)
	report := coverage.Report{SchemaValid: true, Uncovered: []gitdiff.Atom{}, StaleReferences: []coverage.StaleReference{}}
	for _, atom := range changes.Atoms {
		if atom.Path != "internal/refund.go" {
			report.Uncovered = append(report.Uncovered, atom)
		}
	}
	stories := []requirements.Story{refundStory("positive-path", "cutoff", "stale-run", "failing", "not-run", "untested", "manual-only")}
	return StatusInputs{
		SagaID: fixtureSaga, SagaVersion: quality.Version,
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
		Quality:     document,
		Report:      report,
		Changes:     changes,
		QualityCode: resolveQualityCode(document, changes),
	}
}

// resolveQualityCode does what LoadStatusInputs does with a repository.
func resolveQualityCode(document quality.Document, changes gitdiff.ChangeSet) map[string]coverage.ResolvedCode {
	result := map[string]coverage.ResolvedCode{}
	for _, testCase := range document.TestCases {
		for _, evidence := range testCase.Evidence {
			for _, reference := range evidence.Code {
				result[reference.Key()] = coverage.Resolve(context.Background(), reference, changes, coderesolve.Pinned{})
			}
		}
	}
	return result
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
	if got := cell(t, status, "positive-path", coverage.AxisImplementation); got.State != coverage.StateCoveredDirect || len(got.Links[0].Link.Code) != 1 {
		t.Fatalf("criterion <- addresses - Item -> exact current diff covers implementation: %#v", got)
	}
	if got := cell(t, status, "cutoff", coverage.AxisImplementation); got.State != coverage.StateGap {
		t.Fatalf("no recorded path means an implementation gap: %#v", got)
	}
}

func TestImplementationPathWithStaleReferenceIsStaleSourceHistory(t *testing.T) {
	inputs := qualityFixture(t)
	item := inputs.Decks[0].Slides[0].Items[0]
	inputs.Report.StaleReferences = []coverage.StaleReference{{
		Assignment: coverage.Assignment{Target: item.Target, EvidenceFile: item.Code[0].Path, Reference: 1},
		Reference:  item.Code[0].References[0], Reason: "lines 10-10 of internal/refund.go changed",
	}}
	status := Assemble(inputs)
	if got := cell(t, status, "positive-path", coverage.AxisImplementation); got.State != coverage.StateStale {
		t.Fatalf("an Item whose only reference is stale is stale, not covered: %#v", got)
	}
	found := false
	for _, record := range status.Stale {
		if record.Kind == "code_reference" && record.History == historySource && reflect.DeepEqual(record.Affects, []string{criterionURN("positive-path")}) {
			found = true
		}
	}
	if !found {
		t.Fatalf("the stale reference is stale source history naming the criterion: %#v", status.Stale)
	}
	implicated := false
	for _, value := range status.ChangedSource.Implicated {
		implicated = implicated || value.Resource == criterionURN("positive-path")
	}
	if !implicated {
		t.Fatalf("the source change implicates the criterion reached through the Item: %#v", status.ChangedSource.Implicated)
	}
}

// A Saga with no quality records has no exemption: every accepted criterion's
// quality axis is a visible gap naming the missing kind.
func TestNoQualityRecordsIsAVisibleGap(t *testing.T) {
	inputs := qualityFixture(t)
	inputs.Quality = quality.Document{SagaID: fixtureSaga}
	inputs.Exceptions = nil
	status := Assemble(inputs)

	got := cell(t, status, "untested", coverage.AxisQuality)
	if got.State != coverage.StateGap || !containsText(got.Gap.Reasons, "required positive test: missing_kind") {
		t.Fatalf("a Saga with no quality records is a visible quality gap with its reason: %#v", got)
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
			found = reflect.DeepEqual(evidence.Criteria, []string{criterionURN("positive-path")}) && len(evidence.Code) == 1
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

// Test code a test case's evidence owns reaches the story whose criterion the
// test case verifies, so a comparison's stories area counts the same lines
// its implementation area does, and growth never asks for a story for a test
// case that already verifies one.
func TestAVerifyingTestCaseReachesItsStory(t *testing.T) {
	status := Assemble(qualityFixture(t))
	if got := status.Chain.TargetStories[testURN("happy")]; len(got) != 1 || got[0] != storyURN {
		t.Fatalf("test case happy reaches %v, want the story it verifies", got)
	}
	if got := status.Chain.TargetStories[testURN("orphan")]; len(got) != 0 {
		t.Fatalf("a test case that verifies nothing reaches no story: %v", got)
	}
}
