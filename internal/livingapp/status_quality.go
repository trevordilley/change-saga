package livingapp

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/coderef"

	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// Quality describes verification definitions, never execution outcomes.
const (
	kindCovered        = "covered"
	kindMissing        = "missing_kind"
	kindStale          = "stale"
	kindInactive       = "inactive"
	kindInvalid        = "invalid"
	kindConflicted     = "conflicted"
	kindExcluded       = "excluded"
	kindUnsupported    = "automation_not_allowed"
	kindMissingMapping = "missing_mapping"
)

var factRank = map[string]int{kindCovered: 0, kindConflicted: 1, kindStale: 2, kindMissingMapping: 3, kindUnsupported: 4, kindInactive: 5, kindInvalid: 6}

// testEval is one verifies relation from a test case to a criterion, with the
// test's current definition and reference state.
type testEval struct {
	relation, testCase string
	kinds              []string
	state              string
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
	verifies    map[string][]VerifiesLink
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
				for _, reference := range evidence.Code {
					if resolved := a.in.QualityCode[reference.Key()]; len(resolved.Current) == 0 {
						reasons = append(reasons, "code "+reference.Location().String()+" is stale: "+resolved.Reason)
						a.addImplicated(urn, "test_case", evidenceURN+": code reference is stale")
					}
				}
			}
			if len(reasons) > 0 && contains(testCase.EvidenceHeads, evidenceURN) && evidence.Role != quality.EvidenceExecutionArtifact {
				a.evidenceStale[evidenceURN] = uniqueSorted(reasons)
				history := historySource
				if evidence.TestRevision != a.currentRevs[urn] {
					history = historyReview
				}
				a.markStale(evidenceURN, "quality_evidence", history, reasons, []Pin{{Field: "test_revision", Pinned: evidence.TestRevision, Current: a.currentRevs[urn]}}, nil)
			}
		}
	}
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
			var locations []coderef.Location
			for _, reference := range evidence.Code {
				locations = append(locations, a.in.QualityCode[reference.Key()].Current...)
			}
			for _, atom := range coverage.SelectLocations(a.in.Changes, locations) {
				if _, exists := result[atom.Key]; !exists {
					result[atom.Key] = TestOwned{TestCase: urn, Evidence: evidenceURN}
				}
			}
		}
	}
	return result
}

// qualityAxis evaluates every accepted criterion's quality obligation from the
// recorded policy, verifies relations, test definitions and implementation evidence.
func (a *assembler) qualityAxis() qualityEvaluation {
	result := qualityEvaluation{
		links: map[string][]coverage.AxisLink{}, unsatisfied: map[string][]string{}, criteria: map[string]*QualityCriterion{},
		facts: map[string][]QualityFactStatus{}, verifies: a.verifiesByTestCase(),
	}
	evals := a.verifyEvaluations()
	for _, frame := range a.criteria {
		row := &QualityCriterion{
			Criterion: frame.urn, StoryRevision: frame.currentRevision, Policy: "default", PolicyState: "default",
			RequiredKinds: []string{string(quality.CoveragePositive), string(quality.CoverageNegative), string(quality.CoverageEdge)}, AllowedAutomation: []string{}, ObservedKinds: []string{},
			Tests: []string{}, Kinds: []KindStatus{}, Links: []TestLinkRow{},
		}
		result.criteria[frame.urn] = row
		a.applyPolicy(frame, row, &result)
		criterionEvals := evals[frame.urn]
		for _, eval := range criterionEvals {
			a.applyAutomation(row, eval)
			row.Tests = append(row.Tests, eval.testCase)
			row.ObservedKinds = append(row.ObservedKinds, eval.kinds...)
			row.Links = append(row.Links, TestLinkRow{
				Relation: eval.relation, TestCase: eval.testCase, State: eval.state, Kinds: copyStrings(eval.kinds),
				Reasons: uniqueSorted(append(append(append(append([]string{}, eval.stale...), eval.conflicts...), eval.invalid...), eval.unsatisfied...)),
			})
			result.links[frame.urn] = append(result.links[frame.urn], coverage.AxisLink{
				Axis: coverage.AxisQuality, Relation: eval.relation, Source: eval.testCase, Broad: eval.broad, PinnedRevision: eval.pinned,
				Paths: eval.paths, Code: eval.diffs, StaleReasons: eval.stale, InvalidReasons: eval.invalid,
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
// The relation's currency is taken as reported by requirements.EvaluateRelation;
// no pin is compared here.
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
		stale, conflicts, invalid := link.reasons()
		a.staleRelation(link, stale, criteria)
		for _, criterion := range criteria {
			eval := a.evaluateTest(link, criterion, broad, stale)
			eval.conflicts = append(eval.conflicts, conflicts...)
			eval.invalid = append(eval.invalid, invalid...)
			switch {
			case len(invalid) > 0:
				eval.state = kindInvalid
			case len(conflicts) > 0:
				eval.state = kindConflicted
			}
			result[criterion] = append(result[criterion], eval)
		}
	}
	return result
}

// verifiesByTestCase lists every active verifies relation from each test case,
// whatever its currency and whether or not its criterion is accepted. A test
// case with none is an orphan; a test case whose only link is stale is not.
func (a *assembler) verifiesByTestCase() map[string][]VerifiesLink {
	result := map[string][]VerifiesLink{}
	for _, link := range a.in.Links {
		if !link.Active || link.Type != requirements.RelationVerifies {
			continue
		}
		if ref, err := qualityid.Parse(link.From); err != nil || ref.Kind != qualityid.KindTestCase {
			continue
		}
		reasons := []string{}
		for _, reason := range link.Reasons {
			reasons = append(reasons, reason.Message)
		}
		result[link.From] = append(result[link.From], VerifiesLink{Relation: link.URN, Criterion: link.To, Currency: string(link.Currency), Reasons: uniqueSorted(reasons)})
	}
	for testCase := range result {
		sort.Slice(result[testCase], func(i, j int) bool { return result[testCase][i].Relation < result[testCase][j].Relation })
	}
	return result
}

func (a *assembler) evaluateTest(link Link, criterion string, broad bool, relationStale []string) *testEval {
	eval := &testEval{
		relation: link.URN, testCase: link.From, state: kindCovered, kinds: []string{},
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
	switch {
	case testCase.RevisionConflict():
		eval.conflicts = append(eval.conflicts, "test case has competing revision heads: "+joinReasons(testCase.RevisionHeads))
	case testCase.LifecycleConflict():
		eval.conflicts = append(eval.conflicts, "test case has competing lifecycle heads: "+joinReasons(testCase.LifecycleHeads))
	}
	if len(eval.conflicts) > 0 {
		eval.state = kindConflicted
	}
	if testCase.CurrentLifecycle != nil && (testCase.CurrentLifecycle.State == quality.StateDeprecated || testCase.CurrentLifecycle.State == quality.StateRetired) {
		eval.unsatisfied = append(eval.unsatisfied, "test case is "+string(testCase.CurrentLifecycle.State))
		if eval.state == kindCovered {
			eval.state = kindInactive
		}
	}
	path := hops(criterion, story, broad, link.From)
	if testCase.CurrentRevision == nil || len(testCase.CurrentRevision.Steps) == 0 {
		eval.unsatisfied = append(eval.unsatisfied, "test definition needs ordered steps")
		if eval.state == kindCovered {
			eval.state = kindMissingMapping
		}
	}
	mapped, explicit := false, false
	if testCase.CurrentRevision != nil {
		for _, step := range testCase.CurrentRevision.Steps {
			explicit = explicit || len(step.Criteria) > 0
			if contains(step.Criteria, criterion) {
				mapped = true
			}
		}
	}
	if explicit && !mapped {
		eval.unsatisfied = append(eval.unsatisfied, "no step maps this criterion")
		if eval.state == kindCovered {
			eval.state = kindMissingMapping
		}
	}
	for _, evidence := range testCase.Evidence {
		evidenceURN, _ := qualityid.Evidence(a.in.SagaID, testCase.Identity.ID, evidence.ID)
		if evidence.Role == quality.EvidenceExecutionArtifact || !contains(testCase.EvidenceHeads, evidenceURN) {
			continue
		}
		// Step evidence only contributes to the criteria those steps exercise.
		if len(evidence.Steps) > 0 && testCase.CurrentRevision != nil {
			relevant := false
			for _, step := range testCase.CurrentRevision.Steps {
				if contains(evidence.Steps, step.ID) && contains(step.Criteria, criterion) {
					relevant = true
				}
			}
			if !relevant {
				continue
			}
		}
		reasons := append(append([]string{}, evidence.StaleReasons...), a.evidenceStale[evidenceURN]...)
		if len(reasons) > 0 {
			eval.stale = append(eval.stale, "evidence "+evidenceURN+": "+joinReasons(reasons))
			eval.evidenceResolved = false
		}
		for _, reference := range evidence.Code {
			location := reference.Location().String()
			eval.diffs = append(eval.diffs, location)
			if len(evidence.Steps) == 0 {
				eval.paths = append(eval.paths, append(append([]string{}, path...), evidenceURN, location))
			} else if testCase.CurrentRevision != nil {
				for _, step := range testCase.CurrentRevision.Steps {
					if contains(evidence.Steps, step.ID) && contains(step.Criteria, criterion) {
						eval.paths = append(eval.paths, append(append([]string{}, path...), link.From+":step:"+step.ID, evidenceURN, location))
					}
				}
			}
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
	return eval
}

// kindState reduces the tests declaring one kind to that kind's state and the
// fact explaining definition coverage.
func kindState(criterion, kind string, required bool, evals []*testEval) (KindStatus, QualityFactStatus) {
	status := KindStatus{Kind: kind, Required: required, State: kindMissing, TestCases: []string{}, Reasons: []string{}}
	fact := QualityFactStatus{Criterion: criterion, Kind: kind, Required: required, State: kindMissing, EvidenceResolved: true, StaleReasons: []string{}}
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
	fact.EvidenceResolved = best.evidenceResolved
	fact.StaleReasons = copyStrings(best.stale)
	return status, fact
}

// finishQuality applies exclusions from the axis projection and produces the
// test-case rows. A criterion whose quality axis is explicitly excluded has no
// required kinds; its facts are kept only as observations.
func (a *assembler) finishQuality(evaluation qualityEvaluation, projection coverage.AxisProjection) QualityStatus {
	result := QualityStatus{Criteria: []QualityCriterion{}, TestCases: []TestCaseStatus{}, Facts: []QualityFactStatus{}}
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
			TestCase: urn, Feature: testCase.Feature, Lifecycle: "conflicted", RevisionHeads: copyStrings(testCase.RevisionHeads), Kinds: []string{},
			Definition: quality.DefinitionStage(*testCase), Verifies: append([]VerifiesLink{}, evaluation.verifies[urn]...),
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
		row.Steps = definitionSteps(*testCase, row.Verifies)
		for _, evidence := range testCase.Evidence {
			eid, _ := qualityid.Evidence(a.in.SagaID, testCase.Identity.ID, evidence.ID)
			if len(a.evidenceStale[eid]) > 0 && row.Definition != "conflicted" {
				row.Definition = "stale_evidence"
			}
		}
		row.Orphaned = len(row.Verifies) == 0
		result.TestCases = append(result.TestCases, row)
	}
	return result
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

func definitionSteps(test quality.TestCase, links []VerifiesLink) []TestStepStatus {
	rows := []TestStepStatus{}
	if test.CurrentRevision == nil {
		return rows
	}
	for _, step := range test.CurrentRevision.Steps {
		revision := test.CurrentRevision.TestCase + ":revision:" + test.CurrentRevision.ID
		row := TestStepStatus{Step: test.CurrentRevision.TestCase + ":step:" + step.ID, Revision: revision, ID: step.ID, Criteria: copyStrings(step.Criteria), Verifies: []VerifiesLink{}, Evidence: []string{}, Issues: []string{}}
		for _, criterion := range step.Criteria {
			found := false
			for _, link := range links {
				if link.Criterion != criterion {
					continue
				}
				found = true
				row.Verifies = append(row.Verifies, link)
				if link.Currency != "current" {
					row.Issues = append(row.Issues, "criterion mapping is "+link.Currency+": "+criterion)
				}
			}
			if !found {
				row.Issues = append(row.Issues, "missing case-level verifies relation: "+criterion)
			}
		}
		for _, evidence := range test.Evidence {
			if evidence.Role == quality.EvidenceExecutionArtifact || !contains(evidence.Steps, step.ID) {
				continue
			}
			eid := test.CurrentRevision.TestCase + ":evidence:" + evidence.ID
			if !contains(test.EvidenceHeads, eid) {
				continue
			}
			row.Evidence = append(row.Evidence, eid)
			row.Issues = append(row.Issues, evidence.StaleReasons...)
		}
		row.Issues = uniqueSorted(row.Issues)
		rows = append(rows, row)
	}
	return rows
}
