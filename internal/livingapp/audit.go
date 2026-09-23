package livingapp

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/applayout"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/sagaref"
)

const auditFindingsExitCode = 8

type auditCriterion struct {
	Story    string
	Revision string
}

func (s *session) audit(filters Filters) (AuditReport, error) {
	featureID, ok := applayout.FeatureFromURN(s.requirements.SagaID, strings.TrimSpace(filters.Feature))
	if !ok {
		return AuditReport{}, appError(CodeInvalidArgument, "audit requires a feature ID or canonical feature URN", false, nil, nil)
	}
	feature := s.saga.FindFeature(featureID)
	if feature == nil {
		return AuditReport{}, appError(CodeNotFound, "feature was not found", false, map[string]any{"feature": filters.Feature}, nil)
	}

	report := AuditReport{
		Feature: applayout.FeatureURN(s.requirements.SagaID, featureID), FeatureID: featureID,
		Complete: true, Ready: true, Findings: []AuditFinding{}, Exceptions: []AuditException{},
		Risks: []IntentionalRisk{}, Conflicts: []AuditConflict{}, ItemTargets: []string{},
	}
	owners := s.auditOwners()
	criteria, inputs := s.auditCriteria(featureID, &report, owners)
	storedExceptions := s.exceptions
	if storedExceptions == nil {
		return AuditReport{}, appError(CodeInvalidArgument, "audit data was not loaded; open the session with Audit enabled", false, nil, nil)
	}
	exceptions := s.auditExceptions(criteria, inputs, owners, storedExceptions, &report)
	s.auditDomainConflicts(featureID, owners, &report)
	currency := map[string]requirements.RelationCurrency{}
	for _, value := range s.currency {
		currency[value.Relation] = value
	}

	relevantRelations := 0
	intentByItem := map[string][]string{}
	exactExplanation := map[string]bool{}
	for _, relation := range s.requirements.Relations {
		relationURN, _ := livingid.Relation(s.requirements.SagaID, relation.ID)
		fromFeature, toFeature := owners[relation.From], owners[relation.To]
		if relation.Feature != featureID && fromFeature != featureID && toFeature != featureID {
			continue
		}
		relevantRelations++
		standing := currency[relationURN]
		current := relation.State == requirements.RelationActive && standing.Status == requirements.CurrencyCurrent

		if relation.State == requirements.RelationActive && standing.Status != requirements.CurrencyCurrent {
			reasons := []string{}
			for _, reason := range standing.Reasons {
				reasons = append(reasons, reason.Message)
			}
			if len(reasons) == 0 {
				reasons = append(reasons, "the relation is not current")
			}
			code := "stale_or_dangling_pin"
			if standing.Status == requirements.CurrencyConflicted {
				code = "conflicted_relation"
				report.Conflicts = append(report.Conflicts, AuditConflict{ID: relationURN, Kind: "relation", Heads: auditRelationHeads(standing), Reasons: uniqueSorted(reasons)})
			}
			report.Findings = append(report.Findings, AuditFinding{ID: relationURN, Severity: "error", Code: code, Reason: strings.Join(uniqueSorted(reasons), "; "), Related: []string{relation.From, relation.To}})
		}

		if relation.State == requirements.RelationActive && (relation.Type == requirements.RelationAddresses || relation.Type == requirements.RelationExplains) && auditBroadVisual(relation.From) && auditRequirementTarget(relation.To) {
			report.Findings = append(report.Findings, AuditFinding{
				ID: relationURN, Severity: "warning", Code: "broad_visual_intent",
				Reason:  "a Deck or Slide intent link is broader than exact Item-level ownership and must remain visible for handoff review",
				Related: []string{relation.From, relation.To},
			})
		}

		features := uniqueSorted(nonempty(relation.Feature, fromFeature, toFeature))
		if relation.State == requirements.RelationActive && len(features) > 1 {
			report.Findings = append(report.Findings, AuditFinding{
				ID: relationURN, Severity: "warning", Code: "cross_feature_unassigned_link",
				Reason:  "the relation crosses feature ownership and no separate persisted handoff assignment resolves that boundary",
				Related: []string{relation.From, relation.To}, Features: features,
			})
		}

		if !current || relation.Type != requirements.RelationExplains || !strings.Contains(relation.From, ":item:") {
			continue
		}
		if owners[relation.From] == featureID && owners[relation.To] == featureID && auditRequirementTarget(relation.To) {
			intentByItem[relation.From] = append(intentByItem[relation.From], relation.To)
			if _, exists := criteria[relation.To]; exists {
				exactExplanation[relation.To] = true
			}
		}
	}

	for _, deck := range feature.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				report.ItemTargets = append(report.ItemTargets, item.Target)
				report.Summary.Items++
				targets := uniqueSorted(intentByItem[item.Target])
				if len(targets) == 0 {
					report.Findings = append(report.Findings, AuditFinding{
						ID: item.Target, Severity: "error", Code: "item_intent_missing",
						Reason:  "the implementation Item has no current exact explains relation to a story or criterion in this feature",
						Related: []string{deck.Target, slide.Target},
					})
				}
				if auditItemHasEvidence(item) {
					continue
				}
				intentional := len(targets) > 0
				for _, target := range targets {
					criterionTargets := []string{target}
					if strings.Contains(target, ":story:") && !strings.Contains(target, ":criterion:") {
						criterionTargets = auditCriteriaForStory(criteria, target)
					}
					if len(criterionTargets) == 0 {
						intentional = false
					}
					for _, criterion := range criterionTargets {
						ids := exceptions[criterion]
						if len(ids) == 0 {
							intentional = false
							continue
						}
						report.Risks = append(report.Risks, IntentionalRisk{Item: item.Target, Criterion: criterion, Exceptions: ids, Reason: "implementation evidence is explicitly excluded by current cited coverage exception"})
					}
				}
				if !intentional {
					report.Findings = append(report.Findings, AuditFinding{
						ID: item.Target, Severity: "error", Code: "item_evidence_missing",
						Reason:  "the implementation Item has no exact code-reference evidence and no applicable current cited implementation exception",
						Related: []string{deck.Target, slide.Target},
					})
				}
			}
		}
	}

	criterionURNs := sortedKeys(criteria)
	for _, criterion := range criterionURNs {
		if exactExplanation[criterion] {
			continue
		}
		if ids := exceptions[criterion]; len(ids) > 0 {
			report.Risks = append(report.Risks, IntentionalRisk{Criterion: criterion, Exceptions: ids, Reason: "the missing implementation explanation is explicitly excluded by current cited coverage exception"})
			continue
		}
		report.Findings = append(report.Findings, AuditFinding{
			ID: criterion, Severity: "error", Code: "criterion_explanation_missing",
			Reason:  "the current criterion has no current exact Item-level explains relation and no applicable current cited implementation exception",
			Related: []string{criteria[criterion].Story, criteria[criterion].Revision},
		})
	}

	report.Summary.Relations = relevantRelations
	report.ItemTargets = uniqueSorted(report.ItemTargets)
	report.Risks = uniqueAuditRisks(report.Risks)
	sortAuditReport(&report)
	finalizeAudit(&report)
	return report, nil
}

func (s *session) auditDomainConflicts(featureID string, owners map[string]string, report *AuditReport) {
	for _, conflict := range s.plan.Conflicts {
		resource := strings.Split(conflict.Resource, "#")[0]
		if owners[resource] != featureID {
			continue
		}
		report.Conflicts = append(report.Conflicts, AuditConflict{ID: conflict.Resource, Kind: "work_" + conflict.Kind, Heads: uniqueSorted(conflict.Heads), Reasons: []string{"work plan has competing current heads"}})
	}
	for _, testCase := range s.quality.TestCases {
		if testCase.Feature != featureID {
			continue
		}
		urn, _ := qualityid.TestCase(s.quality.SagaID, testCase.Identity.ID)
		for _, value := range []struct {
			kind  string
			heads []string
		}{{"test_case_revision", testCase.RevisionHeads}, {"test_case_lifecycle", testCase.LifecycleHeads}, {"test_run", testCase.RunHeads}} {
			if len(value.heads) > 1 {
				report.Conflicts = append(report.Conflicts, AuditConflict{ID: urn, Kind: value.kind, Heads: uniqueSorted(value.heads), Reasons: []string{"quality record has competing current heads"}})
			}
		}
	}
	for _, set := range s.quality.PolicySets {
		if len(set.Heads) > 1 && owners[set.Criterion] == featureID {
			report.Conflicts = append(report.Conflicts, AuditConflict{ID: set.Criterion, Kind: "quality_policy", Heads: uniqueSorted(set.Heads), Reasons: []string{"quality policy has competing current heads"}})
		}
	}
}

func (s *session) auditCriteria(featureID string, report *AuditReport, owners map[string]string) (map[string]auditCriterion, []coverage.CriterionInput) {
	criteria := map[string]auditCriterion{}
	inputs := []coverage.CriterionInput{}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		for _, revision := range story.Revisions {
			for _, criterion := range revision.AcceptanceCriteria {
				criterionURN, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
				owners[criterionURN] = story.Feature
			}
		}
		if story.Feature != featureID {
			continue
		}
		report.Summary.Stories++
		if len(story.RevisionHeads) != 1 {
			report.Conflicts = append(report.Conflicts, AuditConflict{ID: storyURN, Kind: "story_revision", Heads: copyStrings(story.RevisionHeads), Reasons: []string{"story has no single current revision"}})
		}
		if len(story.LifecycleHeads) != 1 {
			report.Conflicts = append(report.Conflicts, AuditConflict{ID: storyURN, Kind: "story_lifecycle", Heads: copyStrings(story.LifecycleHeads), Reasons: []string{"story has no single current lifecycle state"}})
		}
		if story.CurrentLifecycle != nil && story.CurrentLifecycle.State != requirements.StateAccepted && story.CurrentLifecycle.State != requirements.StateProposed {
			continue
		}
		headSet := map[string]bool{}
		for _, head := range story.RevisionHeads {
			headSet[head] = true
		}
		for _, revision := range story.Revisions {
			revisionURN, _ := livingid.Revision(s.requirements.SagaID, story.Identity.ID, revision.ID)
			if !headSet[revisionURN] {
				continue
			}
			for _, criterion := range revision.AcceptanceCriteria {
				criterionURN, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
				criteria[criterionURN] = auditCriterion{Story: storyURN, Revision: revisionURN}
			}
		}
	}
	for _, urn := range sortedKeys(criteria) {
		value := criteria[urn]
		story := auditStoryByURN(s.requirements.Stories, value.Story)
		heads := []string{}
		if story != nil {
			heads = copyStrings(story.RevisionHeads)
		}
		inputs = append(inputs, coverage.CriterionInput{URN: urn, Story: value.Story, CurrentStoryRevision: value.Revision, RevisionHeads: heads, Links: []coverage.AxisLink{}, Unsatisfied: map[coverage.Axis][]string{}})
	}
	report.Summary.Criteria = len(criteria)
	return criteria, inputs
}

func (s *session) auditExceptions(criteria map[string]auditCriterion, inputs []coverage.CriterionInput, owners map[string]string, stored []coverage.Exception, report *AuditReport) map[string][]string {
	knownCitations := map[string]bool{}
	for _, citation := range s.requirements.Citations {
		urn, _ := livingid.Citation(s.requirements.SagaID, citation.ID)
		knownCitations[urn] = true
		owners[urn] = citation.Feature
	}
	exceptions := append([]coverage.Exception(nil), stored...)
	for index := range exceptions {
		for _, citation := range exceptions[index].Citations {
			if !knownCitations[citation] {
				exceptions[index].UnresolvedCitations = append(exceptions[index].UnresolvedCitations, citation)
			}
		}
	}
	projection := coverage.ProjectAxes(inputs, exceptions)
	current := map[string][]string{}
	for _, item := range projection.Exceptions {
		owner := owners[item.Exception.Criterion]
		if owner != report.FeatureID {
			continue
		}
		report.Exceptions = append(report.Exceptions, AuditException{
			ID: item.Exception.URN, Criterion: item.Exception.Criterion, Axis: string(item.Exception.Axis), State: string(item.State),
			Rationale: item.Exception.Rationale, Citations: copyStrings(item.Exception.Citations), Reasons: copyStrings(item.Reasons), CompetingHeads: copyStrings(item.CompetingHeads),
		})
		if item.Conflicted {
			report.Conflicts = append(report.Conflicts, AuditConflict{ID: item.Exception.Criterion, Kind: "coverage_exception", Heads: copyStrings(item.CompetingHeads), Reasons: []string{"multiple unsuperseded exception heads cover the same criterion and axis"}})
		}
		if item.State == coverage.ExceptionCurrent && !item.Conflicted && item.Exception.Axis == coverage.AxisImplementation {
			if _, exists := criteria[item.Exception.Criterion]; exists {
				current[item.Exception.Criterion] = append(current[item.Exception.Criterion], item.Exception.URN)
			}
		}
		if (item.State != coverage.ExceptionCurrent && item.State != coverage.ExceptionSuperseded) || item.Conflicted {
			reasons := copyStrings(item.Reasons)
			if item.Conflicted {
				reasons = append(reasons, "exception has competing heads")
			}
			report.Findings = append(report.Findings, AuditFinding{ID: item.Exception.URN, Severity: "error", Code: "exception_not_current", Reason: strings.Join(uniqueSorted(reasons), "; "), Related: []string{item.Exception.Criterion, item.Exception.StoryRevision}})
		}
	}
	for criterion := range current {
		current[criterion] = uniqueSorted(current[criterion])
	}
	return current
}

func (s *session) auditOwners() map[string]string {
	owners := map[string]string{}
	for _, feature := range s.saga.Features {
		owners[feature.Target] = feature.ID
		for _, deck := range feature.Decks {
			owners[deck.Target] = feature.ID
			for _, slide := range deck.Slides {
				owners[slide.Target] = feature.ID
				for _, item := range slide.Items {
					owners[item.Target] = feature.ID
				}
			}
		}
	}
	for _, story := range s.requirements.Stories {
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		owners[storyURN] = story.Feature
	}
	for id, item := range s.plan.WorkItems {
		urn, _ := livingid.WorkItem(s.plan.SagaID, id)
		owners[urn] = item.Feature
	}
	for id, wave := range s.plan.Waves {
		urn, _ := livingid.Wave(s.plan.SagaID, id)
		owners[urn] = wave.Feature
	}
	for id, contract := range s.plan.Contracts {
		urn, _ := livingid.Contract(s.plan.SagaID, id)
		owners[urn] = contract.Feature
	}
	for id, dependency := range s.plan.Dependencies {
		urn, _ := livingid.Dependency(s.plan.SagaID, id)
		owners[urn] = dependency.Feature
	}
	for _, testCase := range s.quality.TestCases {
		urn, _ := qualityid.TestCase(s.quality.SagaID, testCase.Identity.ID)
		owners[urn] = testCase.Feature
	}
	return owners
}

func auditItemHasEvidence(item *saga.Item) bool {
	for _, file := range item.Code {
		if len(file.References) > 0 {
			return true
		}
	}
	return false
}

func auditBroadVisual(target string) bool {
	parsed, err := sagaref.ParseTarget(target)
	return err == nil && (parsed.Kind == sagaref.TargetDeck || parsed.Kind == sagaref.TargetSlide)
}

func auditRequirementTarget(target string) bool {
	parsed, err := sagaref.ParseTarget(target)
	return err == nil && (parsed.Kind == sagaref.TargetStory || parsed.Kind == sagaref.TargetCriterion)
}

func auditCriteriaForStory(criteria map[string]auditCriterion, story string) []string {
	result := []string{}
	for urn, criterion := range criteria {
		if criterion.Story == story {
			result = append(result, urn)
		}
	}
	sort.Strings(result)
	return result
}

func auditStoryByURN(stories []requirements.Story, urn string) *requirements.Story {
	for index := range stories {
		if strings.HasSuffix(urn, ":story:"+stories[index].Identity.ID) {
			return &stories[index]
		}
	}
	return nil
}

func auditRelationHeads(value requirements.RelationCurrency) []string {
	heads := []string{}
	for _, reason := range value.Reasons {
		heads = append(heads, reason.Current...)
	}
	return uniqueSorted(heads)
}

func nonempty(values ...string) []string {
	result := []string{}
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func uniqueAuditRisks(values []IntentionalRisk) []IntentionalRisk {
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i].Item+"\x00"+values[i].Criterion, values[j].Item+"\x00"+values[j].Criterion
		return left < right
	})
	result := []IntentionalRisk{}
	for _, value := range values {
		value.Exceptions = uniqueSorted(value.Exceptions)
		if len(result) > 0 && result[len(result)-1].Item == value.Item && result[len(result)-1].Criterion == value.Criterion {
			result[len(result)-1].Exceptions = uniqueSorted(append(result[len(result)-1].Exceptions, value.Exceptions...))
			continue
		}
		result = append(result, value)
	}
	return result
}

func sortAuditReport(report *AuditReport) {
	severity := map[string]int{"error": 0, "warning": 1, "info": 2}
	sort.Slice(report.Findings, func(i, j int) bool {
		if severity[report.Findings[i].Severity] != severity[report.Findings[j].Severity] {
			return severity[report.Findings[i].Severity] < severity[report.Findings[j].Severity]
		}
		if report.Findings[i].ID != report.Findings[j].ID {
			return report.Findings[i].ID < report.Findings[j].ID
		}
		return report.Findings[i].Code < report.Findings[j].Code
	})
	sort.Slice(report.Exceptions, func(i, j int) bool { return report.Exceptions[i].ID < report.Exceptions[j].ID })
	sort.Slice(report.Conflicts, func(i, j int) bool {
		if report.Conflicts[i].ID != report.Conflicts[j].ID {
			return report.Conflicts[i].ID < report.Conflicts[j].ID
		}
		return report.Conflicts[i].Kind < report.Conflicts[j].Kind
	})
}

func finalizeAudit(report *AuditReport) {
	report.Summary.Errors = 0
	report.Summary.Warnings = 0
	for _, finding := range report.Findings {
		switch finding.Severity {
		case "error":
			report.Summary.Errors++
		case "warning":
			report.Summary.Warnings++
		}
	}
	report.Complete = len(report.Conflicts) == 0
	report.Ready = report.Complete && len(report.Findings) == 0
	switch {
	case !report.Complete:
		report.Status = "incomplete"
	case !report.Ready:
		report.Status = "findings"
	default:
		report.Status = "pass"
	}
	report.ExitCode = 0
	if !report.Ready {
		report.ExitCode = auditFindingsExitCode
	}
}

func (report *AuditReport) AddStaleSelector(id, reference, evidenceFile, reason string) {
	related := []string{reference}
	if evidenceFile != "" {
		related = append(related, evidenceFile)
	}
	report.Findings = append(report.Findings, AuditFinding{ID: id, Severity: "error", Code: "stale_or_dangling_selector", Reason: reason, Related: related})
}

func (report *AuditReport) Finalize() {
	sortAuditReport(report)
	finalizeAudit(report)
}
