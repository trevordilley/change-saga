package quality

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/twentyideas/changesaga/internal/diffuri"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/qualityid"
)

type validationErrors struct{ values []error }

func (v *validationErrors) add(format string, args ...any) {
	v.values = append(v.values, fmt.Errorf(format, args...))
}

func (v *validationErrors) err() error {
	if len(v.values) == 0 {
		return nil
	}
	return errors.Join(v.values...)
}

func validateIdentity(value TestCaseIdentity, expectedID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, TestCaseSchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) || value.ID != expectedID {
		problems.add("test-case id must be stable and match its package name")
	}
	validateTime(&problems, value.CreatedAt, "created_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateRevision(value Revision, sagaID, testCaseID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, RevisionSchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) {
		problems.add("revision id is not a stable identifier")
	}
	wantTestCase, _ := qualityid.TestCase(sagaID, testCaseID)
	if value.TestCase != wantTestCase {
		problems.add("test_case must be %q", wantTestCase)
	}
	validateParents(&problems, value.Parents, qualityid.KindRevision, sagaID, testCaseID)
	if len(value.Parents) > 10_000 {
		problems.add("parents contains %d references; maximum is 10000", len(value.Parents))
	}
	if strings.TrimSpace(value.Title) == "" {
		problems.add("title is required")
	}
	if !validAutomation(value.Automation) {
		problems.add("automation must be manual, automated, or hybrid")
	}
	if len(value.CoverageKinds) == 0 {
		problems.add("coverage_kinds must contain at least one kind")
	}
	if len(value.CoverageKinds) > 3 {
		problems.add("coverage_kinds cannot contain more than three kinds")
	}
	seenKinds := map[CoverageKind]bool{}
	for _, kind := range value.CoverageKinds {
		if !validCoverageKind(kind) {
			problems.add("coverage kind %q is invalid", kind)
		} else if seenKinds[kind] {
			problems.add("coverage kind %q is duplicated", kind)
		}
		seenKinds[kind] = true
	}
	if len(value.Steps) > MaxStepsPerRevision {
		problems.add("steps contains %d entries; maximum is %d", len(value.Steps), MaxStepsPerRevision)
	}
	seenSteps := map[string]bool{}
	for index, step := range value.Steps {
		if !qualityid.ValidID(step.ID) {
			problems.add("step %d id is not a stable identifier", index+1)
		}
		if seenSteps[step.ID] {
			problems.add("step id %q is duplicated", step.ID)
		}
		seenSteps[step.ID] = true
		if strings.TrimSpace(step.Action) == "" || strings.TrimSpace(step.ExpectedResult) == "" {
			problems.add("step %q requires a nonblank action and expected_result", step.ID)
		}
	}
	for index, precondition := range value.Preconditions {
		if strings.TrimSpace(precondition) == "" {
			problems.add("precondition %d must be nonblank", index+1)
		}
	}
	if len(value.Preconditions) > MaxStepsPerRevision {
		problems.add("preconditions contains %d entries; maximum is %d", len(value.Preconditions), MaxStepsPerRevision)
	}
	if strings.TrimSpace(value.ExpectedResult) == "" {
		problems.add("expected_result is required")
	}
	validateTime(&problems, value.CreatedAt, "created_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateLifecycleEvent(value LifecycleEvent, sagaID, testCaseID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, LifecycleEventSchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) {
		problems.add("event id is not a stable identifier")
	}
	wantTestCase, _ := qualityid.TestCase(sagaID, testCaseID)
	if value.TestCase != wantTestCase {
		problems.add("test_case must be %q", wantTestCase)
	}
	validateParents(&problems, value.Parents, qualityid.KindEvent, sagaID, testCaseID)
	if len(value.Parents) > 10_000 {
		problems.add("parents contains %d references; maximum is 10000", len(value.Parents))
	}
	if !validLifecycleState(value.State) {
		problems.add("state must be proposed, active, deprecated, or retired")
	}
	validateTime(&problems, value.CreatedAt, "created_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateEvidence(value Evidence, sagaID, testCaseID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, EvidenceSchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) {
		problems.add("evidence id is not a stable identifier")
	}
	wantTestCase, _ := qualityid.TestCase(sagaID, testCaseID)
	if value.TestCase != wantTestCase {
		problems.add("test_case must be %q", wantTestCase)
	}
	if !isTestRevision(value.TestRevision, sagaID, testCaseID) {
		problems.add("test_revision must pin a revision of this test case")
	}
	if !validEvidenceRole(value.Role) {
		problems.add("role must be test_implementation, implementation_under_test, or execution_artifact")
	}
	if total := len(value.Diffs) + len(value.Verifications) + len(value.Citations); total == 0 {
		problems.add("evidence must contain at least one diff, verification, or citation")
	} else if total > MaxReferencesPerRecord {
		problems.add("evidence contains %d references; maximum is %d", total, MaxReferencesPerRecord)
	}
	for name, count := range map[string]int{
		"diffs": len(value.Diffs), "verifications": len(value.Verifications), "citations": len(value.Citations),
	} {
		if count > 20_000 {
			problems.add("%s contains %d references; maximum is 20000", name, count)
		}
	}
	if len(value.Supersedes) > 10_000 {
		problems.add("supersedes contains %d references; maximum is 10000", len(value.Supersedes))
	}
	if (value.Role == EvidenceTestImplementation || value.Role == EvidenceImplementationUnderTest) && len(value.Diffs) == 0 {
		problems.add("%s evidence requires at least one exact diff selector", value.Role)
	}
	if value.Role == EvidenceExecutionArtifact && len(value.Verifications)+len(value.Citations) == 0 {
		problems.add("execution_artifact evidence requires a verification or citation")
	}
	if value.Role == EvidenceExecutionArtifact && len(value.Diffs) != 0 {
		problems.add("execution_artifact evidence cannot contain diff selectors")
	}
	validateDiffs(&problems, value.Diffs)
	validateExternalURNs(&problems, value.Verifications, sagaID, "verification")
	validateExternalURNs(&problems, value.Citations, sagaID, "citation")
	validateQualityRefs(&problems, value.Supersedes, qualityid.KindEvidence, sagaID, testCaseID, value.ID, "supersedes")
	validateTime(&problems, value.CreatedAt, "created_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateRun(value Run, sagaID, testCaseID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, RunSchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) {
		problems.add("run id is not a stable identifier")
	}
	wantTestCase, _ := qualityid.TestCase(sagaID, testCaseID)
	if value.TestCase != wantTestCase {
		problems.add("test_case must be %q", wantTestCase)
	}
	if !isTestRevision(value.TestRevision, sagaID, testCaseID) {
		problems.add("test_revision must pin a revision of this test case")
	}
	validateParents(&problems, value.Parents, qualityid.KindRun, sagaID, testCaseID)
	if len(value.Parents) > 10_000 {
		problems.add("parents contains %d references; maximum is 10000", len(value.Parents))
	}
	validateSource(&problems, value.Source)
	if !validRunResult(value.Result) {
		problems.add("result must be passed, failed, blocked, or skipped")
	}
	if strings.TrimSpace(value.Summary) == "" {
		problems.add("summary is required")
	}
	validateQualityRefs(&problems, value.Evidence, qualityid.KindEvidence, sagaID, testCaseID, "", "evidence")
	if len(value.Evidence) == 0 {
		problems.add("evidence must contain at least one reference")
	} else if len(value.Evidence) > 20_000 {
		problems.add("evidence contains %d references; maximum is 20000", len(value.Evidence))
	}
	validateTime(&problems, value.ExecutedAt, "executed_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validatePolicy(value Policy, sagaID, expectedID string) error {
	var problems validationErrors
	validateRecordHeader(&problems, value.Schema, PolicySchemaURL, value.Version)
	if !qualityid.ValidID(value.ID) || value.ID != expectedID {
		problems.add("policy id must be stable and match its filename")
	}
	criterion, criterionOK := parseCriterion(value.Criterion, sagaID)
	storyID, revisionOK := parseStoryRevision(value.StoryRevision, sagaID)
	if !criterionOK {
		problems.add("criterion must be a canonical criterion URN in this saga")
	}
	if !revisionOK {
		problems.add("story_revision must be a canonical story revision URN in this saga")
	}
	if criterionOK && revisionOK && criterion.ParentID != storyID {
		problems.add("story_revision must pin the criterion's story")
	}
	if len(value.RequiredKinds) == 0 {
		problems.add("required_kinds must contain at least one kind")
	}
	seenKinds := map[CoverageKind]bool{}
	for _, kind := range value.RequiredKinds {
		if !validCoverageKind(kind) {
			problems.add("required kind %q is invalid", kind)
		} else if seenKinds[kind] {
			problems.add("required kind %q is duplicated", kind)
		}
		seenKinds[kind] = true
	}
	if len(value.RequiredKinds) > 3 {
		problems.add("required_kinds cannot contain more than three kinds")
	}
	if len(value.AllowedAutomation) == 0 {
		problems.add("allowed_automation must contain at least one value")
	}
	seenAutomation := map[Automation]bool{}
	for _, automation := range value.AllowedAutomation {
		if !validAutomation(automation) {
			problems.add("allowed automation %q is invalid", automation)
		} else if seenAutomation[automation] {
			problems.add("allowed automation %q is duplicated", automation)
		}
		seenAutomation[automation] = true
	}
	if len(value.AllowedAutomation) > 3 {
		problems.add("allowed_automation cannot contain more than three values")
	}
	if len(value.Supersedes) > 10_000 {
		problems.add("supersedes contains %d references; maximum is 10000", len(value.Supersedes))
	}
	validateQualityRefs(&problems, value.Supersedes, qualityid.KindQualityPolicy, sagaID, "", value.ID, "supersedes")
	if strings.TrimSpace(value.Rationale) == "" {
		problems.add("rationale is required")
	}
	validateTime(&problems, value.CreatedAt, "created_at")
	validateRequestID(&problems, value.RequestID)
	return problems.err()
}

func validateDocument(document *Document) error {
	var problems validationErrors
	testCases := map[string]bool{}
	for index := range document.TestCases {
		testCase := &document.TestCases[index]
		if testCases[testCase.Identity.ID] {
			problems.add("test-case id %q is duplicated", testCase.Identity.ID)
		}
		testCases[testCase.Identity.ID] = true
		if err := validateTestCaseGraphs(testCase, document.SagaID, document.Source); err != nil {
			problems.add("test case %q: %v", testCase.Identity.ID, err)
		}
	}
	if err := validatePolicyGraph(document); err != nil {
		problems.add("policies: %v", err)
	}
	return problems.err()
}

func validateTestCaseGraphs(testCase *TestCase, sagaID string, source SourceIdentity) error {
	var problems validationErrors
	testCaseID := testCase.Identity.ID

	revisions := map[string]Revision{}
	revisionParents := map[string][]string{}
	for _, revision := range testCase.Revisions {
		if _, exists := revisions[revision.ID]; exists {
			problems.add("revision id %q is duplicated", revision.ID)
		}
		revisions[revision.ID] = revision
	}
	for id, revision := range revisions {
		for _, parentURN := range revision.Parents {
			parentID, ok := nestedID(parentURN, qualityid.KindRevision, sagaID, testCaseID)
			if !ok {
				continue
			}
			if _, exists := revisions[parentID]; !exists {
				problems.add("revision %q names missing parent %q", id, parentURN)
			} else {
				revisionParents[id] = append(revisionParents[id], parentID)
			}
		}
	}
	revisionHeads, revisionRoots, revisionCycle := graphHeads(revisionParents, mapKeys(revisions))
	if len(revisionRoots) != 1 {
		problems.add("revision graph must have exactly one initial revision; found %d", len(revisionRoots))
	}
	if revisionCycle {
		problems.add("revision graph must be acyclic")
	}
	testCase.RevisionHeads = buildNestedURNs(sagaID, testCaseID, qualityid.KindRevision, revisionHeads)
	if len(revisionHeads) == 1 {
		value := revisions[revisionHeads[0]]
		testCase.CurrentRevision = &value
	}
	if !revisionCycle {
		validateStepHistory(&problems, revisions, revisionParents)
	}

	events := map[string]LifecycleEvent{}
	eventParents := map[string][]string{}
	for _, event := range testCase.Events {
		if _, exists := events[event.ID]; exists {
			problems.add("lifecycle event id %q is duplicated", event.ID)
		}
		events[event.ID] = event
	}
	for id, event := range events {
		for _, parentURN := range event.Parents {
			parentID, ok := nestedID(parentURN, qualityid.KindEvent, sagaID, testCaseID)
			if !ok {
				continue
			}
			parent, exists := events[parentID]
			if !exists {
				problems.add("lifecycle event %q names missing parent %q", id, parentURN)
				continue
			}
			eventParents[id] = append(eventParents[id], parentID)
			if !allowedLifecycleTransition(parent.State, event.State, len(event.Parents) > 1) {
				problems.add("lifecycle event %q cannot transition from %s to %s", id, parent.State, event.State)
			}
		}
	}
	eventHeads, eventRoots, eventCycle := graphHeads(eventParents, mapKeys(events))
	if len(eventRoots) != 1 {
		problems.add("lifecycle graph must have exactly one initial event; found %d", len(eventRoots))
	} else if events[eventRoots[0]].State != StateProposed {
		problems.add("initial lifecycle state must be proposed")
	}
	if eventCycle {
		problems.add("lifecycle graph must be acyclic")
	}
	testCase.LifecycleHeads = buildNestedURNs(sagaID, testCaseID, qualityid.KindEvent, eventHeads)
	if len(eventHeads) == 1 {
		value := events[eventHeads[0]]
		testCase.CurrentLifecycle = &value
		if value.State == StateActive {
			for _, head := range revisionHeads {
				if len(revisions[head].Steps) == 0 || len(revisions[head].CoverageKinds) == 0 {
					problems.add("active test case requires every current revision head to contain a step and coverage kind")
				}
			}
		}
	}

	evidence := map[string]Evidence{}
	evidenceParents := map[string][]string{}
	knownRevisions := map[string]bool{}
	for id := range revisions {
		urn, _ := qualityid.Revision(sagaID, testCaseID, id)
		knownRevisions[urn] = true
	}
	for _, value := range testCase.Evidence {
		if _, exists := evidence[value.ID]; exists {
			problems.add("evidence id %q is duplicated", value.ID)
		}
		evidence[value.ID] = value
		if !knownRevisions[value.TestRevision] {
			problems.add("evidence %q references missing test revision %q", value.ID, value.TestRevision)
		}
	}
	for id, value := range evidence {
		for _, parentURN := range value.Supersedes {
			parentID, ok := nestedID(parentURN, qualityid.KindEvidence, sagaID, testCaseID)
			if !ok {
				continue
			}
			_, exists := evidence[parentID]
			if !exists {
				problems.add("evidence %q supersedes missing evidence %q", id, parentURN)
			} else {
				evidenceParents[id] = append(evidenceParents[id], parentID)
			}
		}
	}
	evidenceHeads, _, evidenceCycle := graphHeads(evidenceParents, mapKeys(evidence))
	if evidenceCycle {
		problems.add("evidence supersession graph must be acyclic")
	}
	testCase.EvidenceHeads = buildNestedURNs(sagaID, testCaseID, qualityid.KindEvidence, evidenceHeads)
	evidenceHeadIDs := make(map[string]bool, len(evidenceHeads))
	for _, id := range evidenceHeads {
		evidenceHeadIDs[id] = true
	}
	currentRevisionURN := ""
	if testCase.CurrentRevision != nil {
		currentRevisionURN, _ = qualityid.Revision(sagaID, testCaseID, testCase.CurrentRevision.ID)
	}
	currentEvidence := map[string]bool{}
	for index := range testCase.Evidence {
		value := &testCase.Evidence[index]
		reasons := []string{}
		if !evidenceHeadIDs[value.ID] {
			reasons = append(reasons, "evidence was superseded")
		}
		if currentRevisionURN == "" {
			reasons = append(reasons, "test definition has multiple revision heads")
		} else if value.TestRevision != currentRevisionURN {
			reasons = append(reasons, "test revision changed")
		}
		for _, selector := range value.Diffs {
			ref, err := diffuri.Parse(selector)
			if err == nil && (ref.Repository != source.Repository || ref.Base != source.Base || ref.Head != source.Head) {
				reasons = append(reasons, "diff source comparison changed")
				break
			}
		}
		value.StaleReasons = reasons
		value.Current = len(reasons) == 0
		if value.Current {
			urn, _ := qualityid.Evidence(sagaID, testCaseID, value.ID)
			currentEvidence[urn] = true
		}
	}

	runs := map[string]Run{}
	runParents := map[string][]string{}
	for _, run := range testCase.Runs {
		if _, exists := runs[run.ID]; exists {
			problems.add("run id %q is duplicated", run.ID)
		}
		runs[run.ID] = run
		if !knownRevisions[run.TestRevision] {
			problems.add("run %q references missing test revision %q", run.ID, run.TestRevision)
		}
		for _, evidenceURN := range run.Evidence {
			evidenceID, ok := nestedID(evidenceURN, qualityid.KindEvidence, sagaID, testCaseID)
			if ok {
				if _, exists := evidence[evidenceID]; !exists {
					problems.add("run %q references missing evidence %q", run.ID, evidenceURN)
				}
			}
		}
	}
	for id, run := range runs {
		for _, parentURN := range run.Parents {
			parentID, ok := nestedID(parentURN, qualityid.KindRun, sagaID, testCaseID)
			if !ok {
				continue
			}
			if _, exists := runs[parentID]; !exists {
				problems.add("run %q names missing parent %q", id, parentURN)
			} else {
				runParents[id] = append(runParents[id], parentID)
			}
		}
	}
	runHeads, runRoots, runCycle := graphHeads(runParents, mapKeys(runs))
	if len(runs) > 0 && len(runRoots) != 1 {
		problems.add("run graph must have exactly one initial run; found %d", len(runRoots))
	}
	if runCycle {
		problems.add("run graph must be acyclic")
	}
	testCase.RunHeads = buildNestedURNs(sagaID, testCaseID, qualityid.KindRun, runHeads)
	runHeadIDs := make(map[string]bool, len(runHeads))
	for _, id := range runHeads {
		runHeadIDs[id] = true
	}
	for index := range testCase.Runs {
		value := &testCase.Runs[index]
		reasons := []string{}
		if len(runHeads) != 1 {
			reasons = append(reasons, "run graph has multiple heads")
		} else if !runHeadIDs[value.ID] {
			reasons = append(reasons, "run was superseded")
		}
		if currentRevisionURN == "" {
			reasons = append(reasons, "test definition has multiple revision heads")
		} else if value.TestRevision != currentRevisionURN {
			reasons = append(reasons, "test revision changed")
		}
		if value.Source != source {
			reasons = append(reasons, "source comparison changed")
		}
		for _, evidenceURN := range value.Evidence {
			if !currentEvidence[evidenceURN] {
				reasons = append(reasons, "referenced evidence is not current")
				break
			}
		}
		value.StaleReasons = reasons
		value.Current = len(reasons) == 0
		if value.Current {
			copy := *value
			testCase.CurrentRun = &copy
		}
		if len(runHeads) == 1 && value.ID == runHeads[0] {
			copy := *value
			testCase.HeadRun = &copy
		}
	}
	return problems.err()
}

func validatePolicyGraph(document *Document) error {
	var problems validationErrors
	policies := map[string]Policy{}
	parents := map[string][]string{}
	for _, policy := range document.Policies {
		if _, exists := policies[policy.ID]; exists {
			problems.add("policy id %q is duplicated", policy.ID)
		}
		policies[policy.ID] = policy
	}
	for id, policy := range policies {
		for _, parentURN := range policy.Supersedes {
			ref, err := qualityid.Parse(parentURN)
			if err != nil || ref.Kind != qualityid.KindQualityPolicy || ref.SagaID != document.SagaID {
				continue
			}
			parent, exists := policies[ref.ID]
			if !exists {
				problems.add("policy %q supersedes missing policy %q", id, parentURN)
				continue
			}
			if parent.Criterion != policy.Criterion {
				problems.add("policy %q cannot supersede a policy for another criterion", id)
			}
			parents[id] = append(parents[id], ref.ID)
		}
	}
	_, _, cycle := graphHeads(parents, mapKeys(policies))
	if cycle {
		problems.add("policy supersession graph must be acyclic")
	}

	superseded := map[string]bool{}
	for _, values := range parents {
		for _, id := range values {
			superseded[id] = true
		}
	}
	sets := map[string]*PolicySet{}
	for _, policy := range document.Policies {
		key := policy.Criterion + "\x00" + policy.StoryRevision
		set := sets[key]
		if set == nil {
			set = &PolicySet{Criterion: policy.Criterion, StoryRevision: policy.StoryRevision, Policies: []Policy{}, Heads: []string{}}
			sets[key] = set
		}
		set.Policies = append(set.Policies, policy)
		if !superseded[policy.ID] {
			urn, _ := qualityid.QualityPolicy(document.SagaID, policy.ID)
			set.Heads = append(set.Heads, urn)
		}
	}
	document.PolicySets = document.PolicySets[:0]
	for _, set := range sets {
		sort.Slice(set.Policies, func(i, j int) bool { return set.Policies[i].ID < set.Policies[j].ID })
		sort.Strings(set.Heads)
		if len(set.Heads) == 1 {
			ref, _ := qualityid.Parse(set.Heads[0])
			value := policies[ref.ID]
			set.Current = &value
		}
		document.PolicySets = append(document.PolicySets, *set)
	}
	sort.Slice(document.PolicySets, func(i, j int) bool {
		if document.PolicySets[i].Criterion == document.PolicySets[j].Criterion {
			return document.PolicySets[i].StoryRevision < document.PolicySets[j].StoryRevision
		}
		return document.PolicySets[i].Criterion < document.PolicySets[j].Criterion
	})
	return problems.err()
}

func validateStepHistory(problems *validationErrors, revisions map[string]Revision, parents map[string][]string) {
	memo := map[string]map[string]bool{}
	var ancestorSteps func(string) map[string]bool
	ancestorSteps = func(id string) map[string]bool {
		if known := memo[id]; known != nil {
			return known
		}
		result := map[string]bool{}
		for _, parentID := range parents[id] {
			for _, step := range revisions[parentID].Steps {
				result[step.ID] = true
			}
			for stepID := range ancestorSteps(parentID) {
				result[stepID] = true
			}
		}
		memo[id] = result
		return result
	}
	for id, revision := range revisions {
		if len(parents[id]) == 0 {
			continue
		}
		presentInParent := map[string]bool{}
		for _, parentID := range parents[id] {
			for _, step := range revisions[parentID].Steps {
				presentInParent[step.ID] = true
			}
		}
		ancestors := ancestorSteps(id)
		for _, step := range revision.Steps {
			if ancestors[step.ID] && !presentInParent[step.ID] {
				problems.add("revision %q reuses removed step id %q", id, step.ID)
			}
		}
	}
}

func validateRecordHeader(problems *validationErrors, actualSchema, expectedSchema string, version int) {
	if actualSchema != expectedSchema {
		problems.add("$schema must be %q", expectedSchema)
	}
	if version != Version {
		problems.add("version must be %d", Version)
	}
}

func validateParents(problems *validationErrors, values []string, kind qualityid.Kind, sagaID, testCaseID string) {
	validateQualityRefs(problems, values, kind, sagaID, testCaseID, "", "parent")
}

func validateQualityRefs(problems *validationErrors, values []string, kind qualityid.Kind, sagaID, testCaseID, selfID, name string) {
	if len(values) > MaxReferencesPerRecord {
		problems.add("%s contains %d references; maximum is %d", name, len(values), MaxReferencesPerRecord)
	}
	seen := map[string]bool{}
	for _, value := range values {
		ref, err := qualityid.Parse(value)
		if err != nil || ref.Kind != kind || ref.SagaID != sagaID || ref.TestCaseID != testCaseID {
			problems.add("%s %q is not a canonical %s URN for this resource", name, value, kind)
		} else if selfID != "" && ref.ID == selfID {
			problems.add("%s cannot reference the record itself", name)
		}
		if seen[value] {
			problems.add("%s %q is duplicated", name, value)
		}
		seen[value] = true
	}
}

func validateDiffs(problems *validationErrors, values []string) {
	seen := map[string]bool{}
	var comparison *SourceIdentity
	for _, value := range values {
		ref, err := diffuri.Parse(value)
		if err != nil || ref.Kind == "file" {
			problems.add("diff %q must be a canonical exact line or event selector", value)
			continue
		}
		if seen[value] {
			problems.add("diff %q is duplicated", value)
		}
		seen[value] = true
		current := SourceIdentity{Repository: ref.Repository, Base: ref.Base, Head: ref.Head}
		if comparison == nil {
			comparison = &current
		} else if *comparison != current {
			problems.add("all evidence diffs must use one source comparison identity")
		}
	}
}

func validateExternalURNs(problems *validationErrors, values []string, sagaID, kind string) {
	seen := map[string]bool{}
	for _, value := range values {
		valid := false
		if kind == "citation" {
			ref, err := livingid.Parse(value)
			valid = err == nil && ref.Kind == livingid.KindCitation && ref.SagaID == sagaID
		} else {
			parts := strings.Split(value, ":")
			valid = len(parts) == 5 && parts[0] == "urn" && parts[1] == "change-saga" && parts[2] == sagaID && parts[3] == kind && qualityid.ValidID(parts[4])
		}
		if !valid {
			problems.add("%s %q is not a canonical %s URN in this saga", kind, value, kind)
		} else if seen[value] {
			problems.add("%s %q is duplicated", kind, value)
		}
		seen[value] = true
	}
}

func validateSource(problems *validationErrors, source SourceIdentity) {
	canonical, err := diffuri.CanonicalRepository(source.Repository)
	if err != nil || canonical != source.Repository {
		problems.add("source.repository must be a canonical absolute repository URI")
	}
	if strings.TrimSpace(source.Base) == "" || strings.TrimSpace(source.Head) == "" {
		problems.add("source.base and source.head are required")
	}
}

func parseCriterion(value, sagaID string) (livingid.Reference, bool) {
	ref, err := livingid.Parse(value)
	return ref, err == nil && ref.Kind == livingid.KindCriterion && ref.SagaID == sagaID
}

func parseStoryRevision(value, sagaID string) (string, bool) {
	ref, err := livingid.Parse(value)
	if err != nil || ref.Kind != livingid.KindRevision || ref.SagaID != sagaID || (ref.ParentKind != "" && ref.ParentKind != livingid.KindStory) {
		return "", false
	}
	return ref.ParentID, true
}

func isTestRevision(value, sagaID, testCaseID string) bool {
	_, ok := nestedID(value, qualityid.KindRevision, sagaID, testCaseID)
	return ok
}

func nestedID(value string, kind qualityid.Kind, sagaID, testCaseID string) (string, bool) {
	ref, err := qualityid.Parse(value)
	return ref.ID, err == nil && ref.Kind == kind && ref.SagaID == sagaID && ref.TestCaseID == testCaseID
}

func graphHeads(parents map[string][]string, ids []string) (heads, roots []string, cycle bool) {
	isParent := map[string]bool{}
	for _, id := range ids {
		if len(parents[id]) == 0 {
			roots = append(roots, id)
		}
		for _, parent := range parents[id] {
			isParent[parent] = true
		}
	}
	for _, id := range ids {
		if !isParent[id] {
			heads = append(heads, id)
		}
	}
	colors := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if colors[id] == 1 {
			return true
		}
		if colors[id] == 2 {
			return false
		}
		colors[id] = 1
		for _, parent := range parents[id] {
			if visit(parent) {
				return true
			}
		}
		colors[id] = 2
		return false
	}
	for _, id := range ids {
		cycle = cycle || visit(id)
	}
	sort.Strings(heads)
	sort.Strings(roots)
	return heads, roots, cycle
}

func buildNestedURNs(sagaID, testCaseID string, kind qualityid.Kind, ids []string) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		value, _ := qualityid.Build(qualityid.Reference{SagaID: sagaID, Kind: kind, TestCaseID: testCaseID, ID: id})
		values = append(values, value)
	}
	return values
}

func mapKeys[T any](values map[string]T) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

func validateTime(problems *validationErrors, value time.Time, name string) {
	if value.IsZero() || value.Location() != time.UTC {
		problems.add("%s must be a non-zero UTC RFC 3339 value", name)
	}
}

func validateRequestID(problems *validationErrors, value string) {
	if value != "" && !qualityid.ValidID(value) {
		problems.add("request_id must be a stable identifier")
	}
}

func validCoverageKind(value CoverageKind) bool {
	return value == CoveragePositive || value == CoverageNegative || value == CoverageEdge
}

func validAutomation(value Automation) bool {
	return value == AutomationManual || value == AutomationAutomated || value == AutomationHybrid
}

func validLifecycleState(value LifecycleState) bool {
	return value == StateProposed || value == StateActive || value == StateDeprecated || value == StateRetired
}

func allowedLifecycleTransition(from, to LifecycleState, reconciliation bool) bool {
	if reconciliation {
		return true
	}
	allowed := map[LifecycleState]map[LifecycleState]bool{
		StateProposed:   {StateActive: true, StateDeprecated: true, StateRetired: true},
		StateActive:     {StateDeprecated: true, StateRetired: true},
		StateDeprecated: {StateActive: true, StateRetired: true},
	}
	return allowed[from][to]
}

func validEvidenceRole(value EvidenceRole) bool {
	return value == EvidenceTestImplementation || value == EvidenceImplementationUnderTest || value == EvidenceExecutionArtifact
}

func validRunResult(value RunResult) bool {
	return value == RunPassed || value == RunFailed || value == RunBlocked || value == RunSkipped
}
