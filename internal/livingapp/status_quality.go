package livingapp

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// Quality kind states. They extend the design state vocabulary with the run
// outcomes the quality contract names, and are reported per required kind.
const (
	kindCovered     = "covered"
	kindMissing     = "missing_kind"
	kindNotRun      = "not_run"
	kindFailed      = "failed"
	kindBlocked     = "blocked"
	kindSkipped     = "skipped"
	kindStale       = "stale"
	kindInactive    = "inactive"
	kindInvalid     = "invalid"
	kindConflicted  = "conflicted"
	kindExcluded    = "excluded"
	kindUnsupported = "automation_not_allowed"
)

// factRank orders candidate states when several tests declare the same kind. A
// failing current run outranks a passing one on purpose: a required kind with a
// failing test is not quality-ready, and hiding it behind another pass would
// make the failure invisible to ready_for_review.
var factRank = map[string]int{
	kindFailed: 0, kindCovered: 1, kindConflicted: 2, kindBlocked: 3, kindSkipped: 4, kindStale: 5,
	kindNotRun: 6, kindUnsupported: 7, kindInactive: 8, kindInvalid: 9,
}

// testEval is one verifies relation from a test case to a criterion, with the
// test's current proving state.
type testEval struct {
	relation, testCase string
	kinds              []string
	state              string
	run, runResult     string
	runHeads           []string
	evidenceResolved   bool
	stale, conflicts   []string
	invalid            []string
	unsatisfied        []string
	diffs              []string
	paths              [][]string
	broad              bool
	pinned             string
}

type qualityEvaluation struct {
	links       map[string][]coverage.AxisLink
	unsatisfied map[string][]string
	criteria    map[string]*QualityCriterion
	facts       map[string][]QualityFactStatus
	verifies    map[string][]string
}

func (a *assembler) indexQuality() {
	for i := range a.in.Quality.TestCases {
		testCase := &a.in.Quality.TestCases[i]
		urn := testCaseURN(a.in.SagaID, testCase.Identity.ID)
		a.testCases[urn] = testCase
		if testCase.CurrentRevision != nil {
			a.currentRevs[urn], _ = qualityid.Revision(a.in.SagaID, testCase.Identity.ID, testCase.CurrentRevision.ID)
		}
		for j := range testCase.Evidence {
			evidence := &testCase.Evidence[j]
			evidenceURN, _ := qualityid.Evidence(a.in.SagaID, testCase.Identity.ID, evidence.ID)
			reasons := append([]string{}, evidence.StaleReasons...)
			if evidence.Current {
				for _, uri := range evidence.Diffs {
					if !a.matchesComparison(uri) {
						reasons = append(reasons, "diff "+uri+" does not match the current source comparison")
						a.addImplicated(urn, "test_case", evidenceURN+": diff no longer matches the source comparison")
					}
				}
			}
			if len(reasons) > 0 && evidence.Current {
				a.evidenceStale[evidenceURN] = uniqueSorted(reasons)
				a.markStale(evidenceURN, "quality_evidence", historySource, reasons, []Pin{{Field: "test_revision", Pinned: evidence.TestRevision, Current: a.currentRevs[urn]}}, nil)
			}
		}
	}
}

// matchesComparison reports whether one exact selector still selects atoms in
// the current source comparison. It uses the same selector semantics as
// changed-source accounting.
func (a *assembler) matchesComparison(uri string) bool {
	matched := coverage.SelectTarget([]saga.DiffFile{{Diffs: []saga.DiffReference{{URI: uri}}}}, a.in.Changes)
	return len(matched) > 0
}

// testOwnedAtoms returns the changed atoms owned by current test-code evidence.
func (a *assembler) testOwnedAtoms() map[string]TestOwned {
	result := map[string]TestOwned{}
	urns := make([]string, 0, len(a.testCases))
	for urn := range a.testCases {
		urns = append(urns, urn)
	}
	sort.Strings(urns)
	for _, urn := range urns {
		testCase := a.testCases[urn]
		for _, evidence := range testCase.Evidence {
			evidenceURN, _ := qualityid.Evidence(a.in.SagaID, testCase.Identity.ID, evidence.ID)
			if evidence.Role != quality.EvidenceTestImplementation || !evidence.Current || len(a.evidenceStale[evidenceURN]) > 0 {
				continue
			}
			files := []saga.DiffFile{{Path: evidenceURN}}
			for _, uri := range evidence.Diffs {
				files[0].Diffs = append(files[0].Diffs, saga.DiffReference{URI: uri})
			}
			for _, atom := range coverage.SelectTarget(files, a.in.Changes) {
				if _, exists := result[atom.Key]; !exists {
					result[atom.Key] = TestOwned{TestCase: urn, Evidence: evidenceURN}
				}
			}
		}
	}
	return result
}

// qualityAxis evaluates every accepted criterion's quality obligation from the
// recorded policy, verifies relations, test definitions, runs, and evidence.
func (a *assembler) qualityAxis() qualityEvaluation {
	result := qualityEvaluation{
		links: map[string][]coverage.AxisLink{}, unsatisfied: map[string][]string{}, criteria: map[string]*QualityCriterion{},
		facts: map[string][]QualityFactStatus{}, verifies: map[string][]string{},
	}
	adoption := a.in.Quality.Adoption
	if adoption == "" {
		adoption = quality.NotAdopted
	}
	evals := a.verifyEvaluations()
	for _, criterionEvals := range evals {
		for _, eval := range criterionEvals {
			result.verifies[eval.testCase] = append(result.verifies[eval.testCase], eval.relation)
		}
	}
	for _, frame := range a.criteria {
		row := &QualityCriterion{
			Criterion: frame.urn, StoryRevision: frame.currentRevision, Policy: "default", PolicyState: "default",
			RequiredKinds: []string{string(quality.CoveragePositive)}, AllowedAutomation: []string{}, ObservedKinds: []string{},
			Tests: []string{}, Kinds: []KindStatus{}, Links: []TestLinkRow{},
		}
		result.criteria[frame.urn] = row
		if adoption == quality.NotAdopted {
			reason := "quality capability is not_adopted"
			if a.in.QualityReason != "" {
				reason += ": " + a.in.QualityReason
			}
			result.unsatisfied[frame.urn] = []string{reason}
			row.PolicyState = "not_adopted"
			continue
		}
		a.applyPolicy(frame, row, &result)
		criterionEvals := evals[frame.urn]
		for _, eval := range criterionEvals {
			a.applyAutomation(row, eval)
			row.Tests = append(row.Tests, eval.testCase)
			row.ObservedKinds = append(row.ObservedKinds, eval.kinds...)
			row.Links = append(row.Links, TestLinkRow{
				Relation: eval.relation, TestCase: eval.testCase, State: eval.state, Kinds: copyStrings(eval.kinds),
				Run: eval.run, RunResult: eval.runResult, Reasons: uniqueSorted(append(append(append(append([]string{}, eval.stale...), eval.conflicts...), eval.invalid...), eval.unsatisfied...)),
			})
			result.links[frame.urn] = append(result.links[frame.urn], coverage.AxisLink{
				Axis: coverage.AxisQuality, Relation: eval.relation, Source: eval.testCase, Broad: eval.broad, PinnedRevision: eval.pinned,
				Paths: eval.paths, Diffs: eval.diffs, StaleReasons: eval.stale, InvalidReasons: eval.invalid,
				ConflictReasons: eval.conflicts, Unsatisfied: eval.unsatisfied,
			})
		}
		row.Tests = uniqueSorted(row.Tests)
		row.ObservedKinds = uniqueSorted(row.ObservedKinds)
		required := map[string]bool{}
		for _, kind := range row.RequiredKinds {
			required[kind] = true
		}
		for _, kind := range []string{string(quality.CoveragePositive), string(quality.CoverageNegative), string(quality.CoverageEdge)} {
			if !required[kind] && !contains(row.ObservedKinds, kind) {
				continue
			}
			status, fact := kindState(frame.urn, kind, required[kind], criterionEvals)
			row.Kinds = append(row.Kinds, status)
			result.facts[frame.urn] = append(result.facts[frame.urn], fact)
			if required[kind] && status.State != kindCovered {
				reason := "required " + kind + " test: " + status.State
				if len(status.Reasons) > 0 {
					reason += " (" + joinReasons(status.Reasons) + ")"
				}
				result.unsatisfied[frame.urn] = append(result.unsatisfied[frame.urn], reason)
			}
		}
	}
	return result
}

// applyPolicy resolves the unique policy head for the criterion's current story
// revision. A policy pinned to an older revision is stale: its kinds still
// apply, and the axis stays uncovered until the policy is re-pinned, so a story
// edit can never silently weaken the required kinds.
func (a *assembler) applyPolicy(frame criterionFrame, row *QualityCriterion, result *qualityEvaluation) {
	var stale []quality.PolicySet
	for _, set := range a.in.Quality.PolicySets {
		if set.Criterion != frame.urn {
			continue
		}
		if set.StoryRevision != frame.currentRevision || frame.currentRevision == "" {
			if len(set.Heads) > 0 {
				stale = append(stale, set)
			}
			continue
		}
		if set.Conflict() {
			row.PolicyState = "conflicted"
			row.Policy = joinReasons(set.Heads)
			result.links[frame.urn] = append(result.links[frame.urn], coverage.AxisLink{
				Axis: coverage.AxisQuality, Source: set.Heads[0], ConflictReasons: []string{"quality policy has competing heads: " + joinReasons(set.Heads)},
			})
			return
		}
		if set.Current != nil {
			row.Policy = set.Heads[0]
			row.PolicyState = "current"
			row.RequiredKinds = kindsOf(set.Current.RequiredKinds)
			row.AllowedAutomation = automationOf(set.Current.AllowedAutomation)
			return
		}
	}
	if len(stale) == 0 {
		return
	}
	kinds := append([]string{}, row.RequiredKinds...)
	for _, set := range stale {
		for _, head := range set.Heads {
			for _, policy := range set.Policies {
				urn, _ := qualityid.QualityPolicy(a.in.SagaID, policy.ID)
				if urn != head {
					continue
				}
				kinds = append(kinds, kindsOf(policy.RequiredKinds)...)
				reason := "quality policy pins story revision " + policy.StoryRevision + "; current is " + orUnknownRevision(frame.currentRevision)
				a.markStale(urn, "quality_policy", historyReview, []string{reason},
					[]Pin{{Field: "story_revision", Pinned: policy.StoryRevision, Current: frame.currentRevision}}, []string{frame.urn})
				result.unsatisfied[frame.urn] = append(result.unsatisfied[frame.urn], reason)
				row.Policy = urn
			}
		}
	}
	row.PolicyState = "stale"
	row.RequiredKinds = uniqueSorted(kinds)
}

func (a *assembler) applyAutomation(row *QualityCriterion, eval *testEval) {
	if len(row.AllowedAutomation) == 0 {
		return
	}
	testCase := a.testCases[eval.testCase]
	if testCase == nil || testCase.CurrentRevision == nil {
		return
	}
	if !contains(row.AllowedAutomation, string(testCase.CurrentRevision.Automation)) {
		eval.unsatisfied = append(eval.unsatisfied, "automation "+string(testCase.CurrentRevision.Automation)+" is not allowed by the quality policy")
		if eval.state == kindCovered {
			eval.state = kindUnsupported
		}
	}
}

// verifyEvaluations evaluates every active verifies relation from a test case.
// The relation's own pins are checked against current heads here: the test
// revision pin against the test's current revision, and the story revision pin
// against the story's current revision.
func (a *assembler) verifyEvaluations() map[string][]*testEval {
	result := map[string][]*testEval{}
	links := append([]Link(nil), a.in.Links...)
	sort.Slice(links, func(i, j int) bool { return links[i].URN < links[j].URN })
	for _, link := range links {
		if !link.Active || link.Type != requirements.RelationVerifies {
			continue
		}
		ref, err := qualityid.Parse(link.From)
		if err != nil || ref.Kind != qualityid.KindTestCase {
			continue
		}
		criteria, broad := a.reach(link.To)
		stale := append([]string{}, link.StaleReasons...)
		if current := a.currentRevs[link.From]; link.FromRevision != "" && current != "" && current != link.FromRevision {
			stale = append(stale, "from revision changed")
		}
		if current := a.currentRevision(link.To); link.ToRevision != "" && current != "" && current != link.ToRevision {
			stale = append(stale, "to revision changed")
		}
		stale = uniqueSorted(stale)
		if len(stale) > 0 {
			a.markStale(link.URN, "relation", historyReview, stale, a.relationPins(link), criteria)
			a.describeStale(link.URN, string(link.Type), link.From, link.To, "")
		}
		for _, criterion := range criteria {
			eval := a.evaluateTest(link, criterion, broad, stale)
			result[criterion] = append(result[criterion], eval)
		}
	}
	return result
}

func (a *assembler) evaluateTest(link Link, criterion string, broad bool, relationStale []string) *testEval {
	eval := &testEval{
		relation: link.URN, testCase: link.From, state: kindCovered, kinds: []string{}, runHeads: []string{},
		evidenceResolved: true, stale: copyStrings(relationStale), conflicts: []string{}, invalid: []string{}, unsatisfied: []string{},
		diffs: []string{}, broad: broad, pinned: link.ToRevision,
	}
	story := a.byCriterion[criterion].story
	testCase := a.testCases[link.From]
	if testCase == nil {
		eval.invalid = append(eval.invalid, "verifies source test case is not recorded")
		eval.state = kindInvalid
		eval.paths = [][]string{hops(criterion, story, broad, link.From)}
		return eval
	}
	if testCase.CurrentRevision != nil {
		eval.kinds = kindsOf(testCase.CurrentRevision.CoverageKinds)
	}
	eval.runHeads = copyStrings(testCase.RunHeads)
	switch {
	case testCase.RevisionConflict():
		eval.conflicts = append(eval.conflicts, "test case has competing revision heads: "+joinReasons(testCase.RevisionHeads))
	case testCase.LifecycleConflict():
		eval.conflicts = append(eval.conflicts, "test case has competing lifecycle heads: "+joinReasons(testCase.LifecycleHeads))
	case testCase.RunConflict():
		eval.conflicts = append(eval.conflicts, "test case has competing run heads: "+joinReasons(testCase.RunHeads))
	}
	if len(eval.conflicts) > 0 {
		eval.state = kindConflicted
	}
	if testCase.CurrentLifecycle != nil && testCase.CurrentLifecycle.State != quality.StateActive {
		eval.unsatisfied = append(eval.unsatisfied, "test case is "+string(testCase.CurrentLifecycle.State))
		if eval.state == kindCovered {
			eval.state = kindInactive
		}
	}
	path := hops(criterion, story, broad, link.From)
	if head := testCase.HeadRun; head != nil {
		eval.run, _ = qualityid.Run(a.in.SagaID, testCase.Identity.ID, head.ID)
		eval.runResult = string(head.Result)
		path = append(path, eval.run)
		if !head.Current {
			reasons := []string{}
			for _, reason := range head.StaleReasons {
				reasons = append(reasons, "run "+eval.run+": "+reason)
			}
			eval.stale = append(eval.stale, reasons...)
			eval.evidenceResolved = false
			a.markStale(eval.run, "test_run", runHistory(head.StaleReasons), head.StaleReasons,
				[]Pin{{Field: "test_revision", Pinned: head.TestRevision, Current: a.currentRevs[link.From]}}, []string{criterion})
			a.addImplicated(link.From, "test_case", eval.run+": "+joinReasons(head.StaleReasons))
		}
		for _, evidenceURN := range head.Evidence {
			if reasons := a.evidenceStale[evidenceURN]; len(reasons) > 0 {
				eval.stale = append(eval.stale, "evidence "+evidenceURN+": "+joinReasons(reasons))
				eval.evidenceResolved = false
			}
		}
		for _, evidence := range testCase.Evidence {
			evidenceURN, _ := qualityid.Evidence(a.in.SagaID, testCase.Identity.ID, evidence.ID)
			if !contains(head.Evidence, evidenceURN) {
				continue
			}
			for _, uri := range evidence.Diffs {
				eval.diffs = append(eval.diffs, uri)
				eval.paths = append(eval.paths, append(append([]string{}, path...), evidenceURN, uri))
			}
		}
	} else if len(testCase.RunHeads) == 0 {
		eval.unsatisfied = append(eval.unsatisfied, "no run is recorded")
		if eval.state == kindCovered {
			eval.state = kindNotRun
		}
	}
	if len(eval.paths) == 0 {
		eval.paths = [][]string{path}
	}
	eval.diffs = uniqueSorted(eval.diffs)
	eval.stale = uniqueSorted(eval.stale)
	if len(eval.stale) > 0 && eval.state == kindCovered {
		eval.state = kindStale
	}
	if eval.state == kindCovered && testCase.HeadRun != nil && testCase.HeadRun.Result != quality.RunPassed {
		eval.state = string(testCase.HeadRun.Result)
		eval.unsatisfied = append(eval.unsatisfied, "current run "+eval.run+" "+eval.runResult)
	}
	return eval
}

// kindState reduces the tests declaring one kind to that kind's state and the
// readiness fact the quality gate evaluates.
func kindState(criterion, kind string, required bool, evals []*testEval) (KindStatus, QualityFactStatus) {
	status := KindStatus{Kind: kind, Required: required, State: kindMissing, TestCases: []string{}, Reasons: []string{}}
	fact := QualityFactStatus{Criterion: criterion, Kind: kind, Required: required, State: kindMissing, RunHeads: []string{}, EvidenceResolved: true, StaleReasons: []string{}}
	var best *testEval
	for _, eval := range evals {
		if !contains(eval.kinds, kind) {
			continue
		}
		status.TestCases = append(status.TestCases, eval.testCase)
		if best == nil || factRank[eval.state] < factRank[best.state] || factRank[eval.state] == factRank[best.state] && eval.testCase < best.testCase {
			best = eval
		}
	}
	status.TestCases = uniqueSorted(status.TestCases)
	if best == nil {
		status.Reasons = append(status.Reasons, "no linked test case declares "+kind)
		return status, fact
	}
	status.State = best.state
	status.Reasons = uniqueSorted(append(append(append(append([]string{}, best.stale...), best.conflicts...), best.invalid...), best.unsatisfied...))
	fact.State = best.state
	fact.TestCase = best.testCase
	fact.RunResult = best.runResult
	if best.state != kindCovered && best.runResult == string(quality.RunPassed) {
		// A pass that is stale, conflicted, or otherwise unsatisfying is not a
		// current pass; the fact must not read as one.
		fact.RunResult = best.state
	}
	fact.RunHeads = copyStrings(best.runHeads)
	fact.EvidenceResolved = best.evidenceResolved
	fact.StaleReasons = copyStrings(best.stale)
	return status, fact
}

// finishQuality applies exclusions from the axis projection and produces the
// test-case rows. A criterion whose quality axis is explicitly excluded has no
// required kinds; its facts are kept only as observations.
func (a *assembler) finishQuality(evaluation qualityEvaluation, projection coverage.AxisProjection) QualityStatus {
	adoption := string(a.in.Quality.Adoption)
	if adoption == "" {
		adoption = string(quality.NotAdopted)
	}
	result := QualityStatus{Adoption: adoption, Reason: a.in.QualityReason, Criteria: []QualityCriterion{}, TestCases: []TestCaseStatus{}, Facts: []QualityFactStatus{}}
	for _, frame := range a.criteria {
		row := evaluation.criteria[frame.urn]
		excluded := false
		if criterion, ok := projection.Criterion(frame.urn); ok {
			if cell, ok := criterion.Axis(coverage.AxisQuality); ok && cell.State == coverage.StateExcluded {
				excluded = true
			}
		}
		for i := range row.Kinds {
			if excluded && row.Kinds[i].Required {
				row.Kinds[i].State = kindExcluded
			}
		}
		for _, fact := range evaluation.facts[frame.urn] {
			if excluded {
				fact.Required = false
				fact.State = kindExcluded
			}
			result.Facts = append(result.Facts, fact)
		}
		result.Criteria = append(result.Criteria, *row)
	}
	urns := make([]string, 0, len(a.testCases))
	for urn := range a.testCases {
		urns = append(urns, urn)
	}
	sort.Strings(urns)
	for _, urn := range urns {
		testCase := a.testCases[urn]
		row := TestCaseStatus{
			TestCase: urn, Lifecycle: "conflicted", RevisionHeads: copyStrings(testCase.RevisionHeads), Kinds: []string{},
			RunHeads: copyStrings(testCase.RunHeads), Verifies: uniqueSorted(evaluation.verifies[urn]),
		}
		if testCase.CurrentLifecycle != nil {
			row.Lifecycle = string(testCase.CurrentLifecycle.State)
		}
		if testCase.CurrentRevision != nil {
			row.Title = testCase.CurrentRevision.Title
			row.CurrentRevision = a.currentRevs[urn]
			row.Kinds = kindsOf(testCase.CurrentRevision.CoverageKinds)
			row.Automation = string(testCase.CurrentRevision.Automation)
		}
		if testCase.CurrentRun != nil {
			row.CurrentRun, _ = qualityid.Run(a.in.SagaID, testCase.Identity.ID, testCase.CurrentRun.ID)
			row.RunResult = string(testCase.CurrentRun.Result)
		}
		row.Orphaned = len(row.Verifies) == 0
		result.TestCases = append(result.TestCases, row)
	}
	return result
}

// runHistory says which history invalidated a run: a changed test definition
// is a review-repo change; a changed source identity is a source-repo change.
func runHistory(reasons []string) string {
	for _, reason := range reasons {
		if reason == "source comparison changed" {
			return historySource
		}
	}
	return historyReview
}

func kindsOf(values []quality.CoverageKind) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return uniqueSorted(result)
}

func automationOf(values []quality.Automation) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return uniqueSorted(result)
}

func orUnknownRevision(value string) string {
	if value == "" {
		return "unavailable (multiple story revision heads)"
	}
	return value
}
