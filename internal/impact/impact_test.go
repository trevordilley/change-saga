package impact

import (
	"testing"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/saga"
)

func TestAnalyzeProjectsReplacementAdditionsAndNewFilesWithoutReadingContent(t *testing.T) {
	flowTarget := saga.FragmentTarget("codebase", "flow")
	guardTarget := saga.LandmarkTarget("codebase", "flow", "guard")
	document := &saga.Saga{
		Manifest: saga.Manifest{ID: "codebase", Title: "Codebase", Source: saga.Source{Repository: "https://example.test/acme/app.git", Base: "root", Head: "documented"}},
		Section: &saga.Section{Target: saga.SagaTarget("codebase"), Fragments: []*saga.Fragment{{
			ID: "flow", Title: "Request flow", Path: "flow.fragment", Entrypoint: "content.md", Target: flowTarget,
			Landmarks: []saga.Landmark{{ID: "guard", Label: "Guard", Path: "flow.fragment/___landmarks/guard.landmark", Target: guardTarget}},
		}}},
	}
	baseline := gitdiff.ChangeSet{Repository: document.Manifest.Source.Repository, Base: "root", Head: "incoming-base", BaseOID: "root-oid", HeadOID: "baseline-patch", Atoms: []gitdiff.Atom{
		{Key: "base-1", URI: "base-1", Kind: "line", Path: "app.go", Side: "new", Line: 1, Content: "package app"},
		{Key: "base-2", URI: "base-2", Kind: "line", Path: "app.go", Side: "new", Line: 2, Content: "const Mode = \"old\""},
		{Key: "base-10", URI: "base-10", Kind: "line", Path: "app.go", Side: "new", Line: 10, Content: "func Guard() {}"},
	}}
	report := coverage.Report{
		Complete: true, Summary: coverage.Summary{Total: 3, Covered: 3},
		Ownership: map[string][]coverage.Assignment{
			"base-1":  {{Target: flowTarget, DiffFile: "flow.fragment/___diffs/package.json", Diff: 1}},
			"base-2":  {{Target: flowTarget, DiffFile: "flow.fragment/___diffs/mode.json", Diff: 1}},
			"base-10": {{Target: guardTarget, DiffFile: "flow.fragment/___landmarks/guard.landmark/___diffs/guard.json", Diff: 1}},
		},
	}
	incoming := gitdiff.ChangeSet{Repository: baseline.Repository, Base: "incoming-base", Head: "feature", BaseOID: "incoming-base-oid", HeadOID: "incoming-patch", Atoms: []gitdiff.Atom{
		{Key: "old-mode", URI: "old-mode", Kind: "line", Path: "app.go", Side: "old", Line: 2, Content: "const Mode = \"old\""},
		{Key: "new-mode", URI: "new-mode", Kind: "line", Path: "app.go", Side: "new", Line: 2, Content: "const Mode = \"new\""},
		{Key: "new-guard-line", URI: "new-guard-line", Kind: "line", Path: "app.go", Side: "new", Line: 11, Content: "// guarded"},
		{Key: "new-file-event", URI: "new-file-event", Kind: "event", Path: "new.go", Event: "add", Content: ""},
		{Key: "new-file-line", URI: "new-file-line", Kind: "line", Path: "new.go", Side: "new", Line: 1, Content: "package new"},
	}, DisplayLines: []gitdiff.DisplayLine{
		{Kind: "context", Path: "app.go", OldLine: 1, NewLine: 1, Content: "package app"},
		{Kind: "old", Path: "app.go", OldLine: 2, AtomKey: "old-mode"},
		{Kind: "new", Path: "app.go", NewLine: 2, AtomKey: "new-mode"},
		{Kind: "context", Path: "app.go", OldLine: 3, NewLine: 3},
		{Kind: "context", Path: "app.go", OldLine: 10, NewLine: 10, Content: "func Guard() {}"},
		{Kind: "new", Path: "app.go", NewLine: 11, AtomKey: "new-guard-line"},
		{Kind: "event", Path: "new.go", AtomKey: "new-file-event", Event: "add"},
		{Kind: "new", Path: "new.go", NewLine: 1, AtomKey: "new-file-line"},
	}}

	result := Analyze(document, baseline, report, incoming, "saga_to_diff", nil)
	if result.Schema != Schema || result.Summary.DirectIntersections != 1 || result.Summary.ContextualAdditions != 2 || result.Summary.NewContentRequired != 2 {
		t.Fatalf("unexpected impact summary: %#v", result.Summary)
	}
	if len(result.Targets) != 2 || result.Targets[0].Target != flowTarget || result.Targets[0].Action != "must_update" {
		t.Fatalf("direct target was not prioritized: %#v", result.Targets)
	}
	if result.Targets[0].ContentPath != "flow.fragment/content.md" || len(result.Targets[0].Changes) != 2 {
		t.Fatalf("target did not retain its update location and replacement pair: %#v", result.Targets[0])
	}
	if result.Targets[1].Target != guardTarget || result.Targets[1].Action != "consider_update" || result.Targets[1].Kind != "landmark" {
		t.Fatalf("adjacent addition did not reach its landmark: %#v", result.Targets[1])
	}
	if len(result.NewContent) != 2 || result.NewContent[0].Atom.Path != "new.go" || result.NewContent[1].Atom.Path != "new.go" {
		t.Fatalf("new file was not isolated for new Saga content: %#v", result.NewContent)
	}
}

func TestAnalyzeMarksIncompleteBaselineInsteadOfClaimingCompleteImpact(t *testing.T) {
	document := &saga.Saga{Manifest: saga.Manifest{ID: "codebase", Title: "Codebase"}, Section: &saga.Section{Target: saga.SagaTarget("codebase")}}
	result := Analyze(document, gitdiff.ChangeSet{}, coverage.Report{Complete: false}, gitdiff.ChangeSet{}, "saga_to_diff", nil)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "baseline_incomplete" {
		t.Fatalf("missing baseline caveat: %#v", result.Diagnostics)
	}
}

// A source change that touches a test case's evidence implicates that test
// case and the criteria it verifies; a change under an Item implicates the
// criteria its recorded relations reach. Nothing is inferred from paths.
func TestAnalyzeGraphImplicatesTestCasesAndRequirements(t *testing.T) {
	item := "urn:change-saga:codebase:slide:guard:item:deadline"
	document := &saga.Saga{Manifest: saga.Manifest{ID: "codebase", Title: "Codebase"}, Section: &saga.Section{Target: saga.SagaTarget("codebase")}}
	testLine := "saga-diff://v1/line?base=root-oid&end=3&head=baseline-patch&path=app_test.go&repository=https%3A%2F%2Fexample.test%2Facme%2Fapp.git&side=new&start=1"
	baseline := gitdiff.ChangeSet{Repository: "https://example.test/acme/app.git", Base: "root", BaseOID: "root-oid", HeadOID: "baseline-patch", Atoms: []gitdiff.Atom{
		{Key: "code-10", URI: "code-10", Kind: "line", Path: "app.go", Side: "new", Line: 10, Content: "func Guard() {}"},
		{Key: "test-2", URI: "test-2", Kind: "line", Path: "app_test.go", Side: "new", Line: 2, Content: "func TestGuard() {}"},
	}}
	report := coverage.Report{Complete: true, Ownership: map[string][]coverage.Assignment{
		"code-10": {{Target: item, DiffFile: "___slides/review.deck/40-e-guard.json", Diff: 1}},
	}}
	incoming := gitdiff.ChangeSet{Repository: baseline.Repository, Atoms: []gitdiff.Atom{
		{Key: "old-code", URI: "old-code", Kind: "line", Path: "app.go", Side: "old", Line: 10, Content: "func Guard() {}"},
		{Key: "old-test", URI: "old-test", Kind: "line", Path: "app_test.go", Side: "old", Line: 2, Content: "func TestGuard() {}"},
	}}
	criterion := "urn:change-saga:codebase:story:refund:criterion:deadline"
	graph := Graph{
		Requirements: map[string][]Reach{item: {{Requirement: criterion, Relation: "urn:change-saga:codebase:relation:guard", Path: []string{item, criterion}}}},
		TestCases: []TestCaseEvidence{{
			TestCase: "urn:change-saga:codebase:test-case:guard", Evidence: "urn:change-saga:codebase:test-case:guard:evidence:code",
			Role: "test_implementation", Diffs: []string{testLine}, Criteria: []string{criterion},
		}},
	}
	result := AnalyzeGraph(document, baseline, report, incoming, "saga_to_diff", nil, graph)
	if len(result.TestCases) != 1 || result.TestCases[0].TestCase != "urn:change-saga:codebase:test-case:guard" || result.TestCases[0].Action != "must_update" {
		t.Fatalf("the test case whose evidence changed is implicated: %#v", result.TestCases)
	}
	if len(result.TestCases[0].Criteria) != 1 || result.TestCases[0].Criteria[0] != criterion {
		t.Fatalf("the implicated test case names the criteria it verifies: %#v", result.TestCases[0])
	}
	if len(result.Requirements) != 1 || result.Requirements[0].Requirement != criterion || len(result.Requirements[0].Via) != 2 {
		t.Fatalf("the criterion is implicated through both the Item and the test case: %#v", result.Requirements)
	}
	if result.Summary.TestCasesImpacted != 1 || result.Summary.RequirementsImpacted != 1 || len(result.NewContent) != 0 {
		t.Fatalf("summary = %#v new content = %#v", result.Summary, result.NewContent)
	}
	if len(report.Ownership) != 1 {
		t.Fatal("the caller's ownership map must not be mutated")
	}
	plain := Analyze(document, baseline, report, incoming, "saga_to_diff", nil)
	if len(plain.TestCases) != 0 || len(plain.Requirements) != 0 || len(plain.NewContent) != 1 {
		t.Fatalf("without a review graph the result is the version 1 projection: %#v", plain)
	}
}
