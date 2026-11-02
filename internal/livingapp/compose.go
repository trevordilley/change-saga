package livingapp

import (
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func (s *session) requirementRows(filters Filters) []Requirement {
	rows := []Requirement{}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
		if filters.Feature != "" && story.Feature != filters.Feature {
			continue
		}
		urn, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		if !resourceMatches(filters.Requirement, story.Identity.ID, urn) {
			continue
		}
		state := "conflicted"
		if story.CurrentLifecycle != nil {
			state = string(story.CurrentLifecycle.State)
		}
		if filters.State != "" && filters.State != state {
			continue
		}
		if filters.Kind != "" && filters.Kind != "story" {
			continue
		}
		rows = append(rows, s.projectRequirement(story))
	}
	return rows
}

func (s *session) historyRows(filters Filters) ([]HistoryEvent, error) {
	if strings.TrimSpace(filters.Requirement) == "" {
		return nil, appError(CodeInvalidArgument, "requirement-history requires a requirement", false, nil, nil)
	}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
		urn, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		if !resourceMatches(filters.Requirement, story.Identity.ID, urn) {
			continue
		}
		rows := make([]HistoryEvent, 0, len(story.Revisions)+len(story.Events))
		revisionDepth := graphDepths(storyRevisionParents(story))
		for j := range story.Revisions {
			revision := &story.Revisions[j]
			resource, _ := livingid.Revision(s.requirements.SagaID, story.Identity.ID, revision.ID)
			rows = append(rows, HistoryEvent{Kind: "revision", Resource: resource, Depth: revisionDepth[resource], CreatedAt: revision.CreatedAt, Revision: revision})
		}
		eventDepth := graphDepths(storyEventParents(s.requirements.SagaID, story))
		for j := range story.Events {
			event := &story.Events[j]
			resource, _ := requirements.StoryEventURN(s.requirements.SagaID, story.Identity.ID, event.ID)
			rows = append(rows, HistoryEvent{Kind: "lifecycle", Resource: resource, Depth: eventDepth[resource], CreatedAt: event.CreatedAt, Lifecycle: event})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Depth != rows[j].Depth {
				return rows[i].Depth < rows[j].Depth
			}
			if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
				return rows[i].CreatedAt.Before(rows[j].CreatedAt)
			}
			return rows[i].Resource < rows[j].Resource
		})
		return rows, nil
	}
	return nil, appError(CodeNotFound, "requirement was not found", false, map[string]any{"kind": "requirement"}, nil)
}

func (s *session) citationRows(filters Filters) []Citation {
	linked := map[string]bool{}
	if filters.Requirement != "" {
		for _, story := range s.requirements.Stories {
			urn, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
			if resourceMatches(filters.Requirement, story.Identity.ID, urn) && story.CurrentRevision != nil {
				for _, citation := range story.CurrentRevision.Citations {
					linked[citation] = true
				}
			}
		}
	}
	rows := []Citation{}
	for _, citation := range s.requirements.Citations {
		urn, _ := livingid.Citation(s.requirements.SagaID, citation.ID)
		if !resourceMatches(filters.Citation, citation.ID, urn) || (filters.Requirement != "" && !linked[urn]) {
			continue
		}
		rows = append(rows, Citation{URN: urn, Citation: citation})
	}
	return rows
}

func (s *session) relationRows(filters Filters) []Relation {
	rows := []Relation{}
	for _, relation := range s.requirements.Relations {
		urn, _ := livingid.Relation(s.requirements.SagaID, relation.ID)
		if !resourceMatches(filters.Relation, relation.ID, urn) || (filters.Kind != "" && filters.Kind != string(relation.Type)) || (filters.From != "" && filters.From != relation.From) || (filters.To != "" && filters.To != relation.To) {
			continue
		}
		state := string(relation.State)
		if relation.Stale {
			state = "stale"
		}
		matchesState := filters.State == "" || filters.State == state || (filters.State == "current" && relation.State == requirements.RelationActive && !relation.Stale)
		if !matchesState {
			continue
		}
		rows = append(rows, Relation{URN: urn, Stale: relation.Stale, StaleReasons: copyStrings(relation.StaleReasons), Relation: relation})
	}
	return rows
}

func (s *session) traceRows(filters Filters) ([]Traceability, map[string][]readiness.CriterionResult) {
	inputs, meta := s.criterionInputs(filters)
	projection := readiness.Evaluate(inputs)
	results := map[string]readiness.CriterionResult{}
	for _, item := range projection.Criteria {
		results[item.Criterion] = item
	}
	rows := make([]Traceability, 0, len(meta))
	grouped := map[string][]readiness.CriterionResult{}
	for _, value := range meta {
		r := results[value.Criterion]
		value.Delivered = r.Delivered
		value.Blockers = r.Blockers
		value.Evidence = r.Evidence
		rows = append(rows, value)
		grouped[value.Story] = append(grouped[value.Story], r)
	}
	return rows, grouped
}

func (s *session) readinessRows(filters Filters) (ReadinessPage, []RequirementReadiness) {
	if !s.adopted {
		return ReadinessPage{Summary: ReadinessSummary{Status: "not_applicable"}, Requirements: []RequirementReadiness{}}, []RequirementReadiness{}
	}
	inputs, _ := s.criterionInputs(filters)
	projection := readiness.Evaluate(inputs)
	_, grouped := s.traceRows(filters)
	rows := []RequirementReadiness{}
	for story, criteria := range grouped {
		status := "ready"
		for _, criterion := range criteria {
			if !criterion.Delivered {
				status = "blocked"
				break
			}
		}
		if filters.Status != "" && filters.Status != status {
			continue
		}
		rows = append(rows, RequirementReadiness{Requirement: story, Status: status, Criteria: criteria})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Requirement < rows[j].Requirement })
	status := "blocked"
	if projection.PeerReviewReady {
		status = "ready"
	}
	return ReadinessPage{Summary: ReadinessSummary{Status: status, PeerReviewReady: projection.PeerReviewReady, AcceptedCriteria: len(projection.Criteria), RequirementCoverage: projection.RequirementCoverage, PlanCoverage: projection.PlanCoverage, DeliveryCoverage: projection.DeliveryCoverage}}, rows
}

func (s *session) criterionInputs(filters Filters) ([]readiness.Criterion, []Traceability) {
	// A criterion is planned when a Task names its story. The Tasks that only
	// unblock those reach it through them.
	tasksByStory := map[string][]string{}
	for _, plan := range s.saga.Plans {
		for _, task := range plan.Tasks {
			if task.Story != "" {
				tasksByStory[task.Story] = append(tasksByStory[task.Story], task.Target)
			}
		}
	}
	active := []requirements.Relation{}
	for _, r := range s.requirements.Relations {
		if r.State == requirements.RelationActive && !r.Stale {
			active = append(active, r)
		}
	}
	claimByURN := map[string]saga.Claim{}
	for _, claim := range s.saga.Claims {
		claimByURN["urn:change-saga:"+s.requirements.SagaID+":claim:"+claim.ID] = claim
	}
	verificationByURN := map[string]saga.Verification{}
	for _, v := range s.saga.Verifications {
		verificationByURN["urn:change-saga:"+s.requirements.SagaID+":verification:"+v.ID] = v
	}
	inputs := []readiness.Criterion{}
	meta := []Traceability{}
	reviewEvidenceByTarget, _ := s.reviewEvidenceIndex()
	for _, story := range s.requirements.Stories {
		if filters.Feature != "" && story.Feature != filters.Feature {
			continue
		}
		if story.CurrentRevision == nil || story.CurrentLifecycle == nil || story.CurrentLifecycle.State != requirements.StateAccepted {
			continue
		}
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		if !resourceMatches(filters.Requirement, story.Identity.ID, storyURN) {
			continue
		}
		revisionURN, _ := livingid.Revision(s.requirements.SagaID, story.Identity.ID, story.CurrentRevision.ID)
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			criterionURN, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
			if filters.Kind != "" && !resourceMatches(filters.Kind, criterion.ID, criterionURN) {
				continue
			}
			design := []string{}
			evidence := []string{}
			reviewTargets := []string{}
			codeEvidence := []string{}
			paths := [][]string{}
			for _, r := range active {
				if r.Type != requirements.RelationExplains || r.To != criterionURN && r.To != storyURN {
					continue
				}
				matchedEvidence := false
				for _, reviewEvidence := range reviewEvidenceByTarget[r.From] {
					if !s.reviewEvidenceMatches(reviewEvidence, filters) {
						continue
					}
					matchedEvidence = true
					codeEvidence = append(codeEvidence, reviewEvidence.Ref)
					prefix := []string{criterionURN}
					if r.To == storyURN {
						prefix = append(prefix, storyURN)
					}
					paths = append(paths, append(prefix, reviewEvidence.Path...))
				}
				if filters.Ref == "" && filters.Commit == "" || matchedEvidence {
					reviewTargets = append(reviewTargets, r.From)
					if !matchedEvidence {
						path := []string{criterionURN}
						if r.To == storyURN {
							path = append(path, storyURN)
						}
						paths = append(paths, append(path, r.From))
					}
				}
			}
			if (filters.Ref != "" || filters.Commit != "") && len(codeEvidence) == 0 {
				continue
			}
			// Design that addresses the whole story reaches each of its
			// criteria too; it is listed again as broad because it was not
			// written for this criterion alone.
			broadDesign := []string{}
			for _, r := range active {
				if r.Type != requirements.RelationAddresses {
					continue
				}
				switch r.To {
				case criterionURN:
					design = append(design, r.From)
					paths = append(paths, []string{criterionURN, r.From})
				case storyURN:
					design = append(design, r.From)
					broadDesign = append(broadDesign, r.From)
					paths = append(paths, []string{criterionURN, storyURN, r.From})
				}
			}
			for _, r := range active {
				if r.Type != requirements.RelationVerifies || r.To != criterionURN {
					continue
				}
				if claim, ok := claimByURN[r.From]; ok && len(claim.Evidence) > 0 {
					for _, reference := range claim.Evidence {
						location := reference.Location().String()
						evidence = append(evidence, location)
						paths = append(paths, []string{criterionURN, r.From, location})
					}
				}
				if v, ok := verificationByURN[r.From]; ok && v.Status == "verified" {
					evidence = append(evidence, r.From)
					paths = append(paths, []string{criterionURN, r.From})
				}
			}
			blockers := []readiness.Blocker{}
			if len(design) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "design_missing", Resource: criterionURN})
			}
			tasks := tasksByStory[storyURN]
			if len(tasks) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "task_missing", Resource: criterionURN})
			}
			for _, taskURN := range tasks {
				paths = append(paths, []string{criterionURN, storyURN, taskURN})
			}
			if len(evidence) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "immutable_evidence_missing", Resource: criterionURN, Detail: "progress is not delivery evidence"})
			}
			inputs = append(inputs, readiness.Criterion{URN: criterionURN, Designed: len(design) > 0, Planned: len(tasks) > 0, Evidence: evidence, DirectBlockers: blockers})
			meta = append(meta, Traceability{Criterion: criterionURN, Story: storyURN, Revision: revisionURN, Design: uniqueSorted(design), BroadDesign: uniqueSorted(broadDesign), Tasks: uniqueSorted(tasks), ReviewTargets: uniqueSorted(reviewTargets), CodeEvidence: uniqueSorted(codeEvidence), Paths: uniquePaths(paths)})
		}
	}
	return inputs, meta
}

type reviewEvidence struct {
	Key          string
	Deck         string
	Slide        string
	Item         string
	Ref          string
	Reference    coderef.Reference
	Location     coderef.Location
	EvidenceFile string
	Path         []string
}

func (s *session) reviewEvidenceIndex() (map[string][]reviewEvidence, []reviewEvidence) {
	byTarget := map[string][]reviewEvidence{}
	all := []reviewEvidence{}
	for _, deck := range s.saga.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				for _, file := range item.Code {
					for _, reference := range file.References {
						location := reference.Location()
						ref := location.String()
						value := reviewEvidence{Deck: deck.Target, Slide: slide.Target, Item: item.Target, Ref: ref, Reference: reference, Location: location, EvidenceFile: file.Path}
						value.Key = value.Item + "\x00" + value.EvidenceFile + "\x00" + value.Ref
						all = append(all, value)
						for target, path := range map[string][]string{
							deck.Target:  {deck.Target, slide.Target, item.Target, ref},
							slide.Target: {slide.Target, item.Target, ref},
							item.Target:  {item.Target, ref},
						} {
							copy := value
							copy.Path = path
							byTarget[target] = append(byTarget[target], copy)
						}
					}
				}
			}
		}
	}
	return byTarget, all
}

func (s *session) reviewEvidenceMatches(value reviewEvidence, filters Filters) bool {
	if filters.Ref != "" {
		wanted, err := coderef.ParseLocation(filters.Ref)
		if err != nil {
			return false
		}
		location := value.Location
		if filters.Locate != nil {
			at, current := filters.Locate(value.Reference)
			if !current {
				return false
			}
			location = at
		}
		if !overlaps(wanted, location) {
			return false
		}
	}
	if filters.Commit != "" && !strings.EqualFold(value.Location.Commit, filters.Commit) {
		return false
	}
	return true
}

// overlaps reports whether two locations share a commit, a path, and at least
// one line. A whole file overlaps every location in it.
func overlaps(left, right coderef.Location) bool {
	if left.Commit != right.Commit || left.Path != right.Path {
		return false
	}
	if left.WholeFile() || right.WholeFile() {
		return true
	}
	return left.Start <= right.End && right.Start <= left.End
}

func (s *session) unlinkedReviewEvidence(filters Filters) []UnlinkedCodeEvidence {
	byTarget, all := s.reviewEvidenceIndex()
	linked := map[string]bool{}
	acceptedTargets := map[string]bool{}
	for _, story := range s.requirements.Stories {
		if story.CurrentRevision == nil || story.CurrentLifecycle == nil || story.CurrentLifecycle.State != requirements.StateAccepted || len(story.CurrentRevision.AcceptanceCriteria) == 0 {
			continue
		}
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		acceptedTargets[storyURN] = true
		for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
			criterionURN, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
			acceptedTargets[criterionURN] = true
		}
	}
	for _, relation := range s.requirements.Relations {
		if relation.State != requirements.RelationActive || relation.Stale || relation.Type != requirements.RelationExplains || !acceptedTargets[relation.To] {
			continue
		}
		for _, value := range byTarget[relation.From] {
			linked[value.Key] = true
		}
	}
	result := []UnlinkedCodeEvidence{}
	for _, value := range all {
		if linked[value.Key] || !s.reviewEvidenceMatches(value, filters) {
			continue
		}
		result = append(result, UnlinkedCodeEvidence{Deck: value.Deck, Slide: value.Slide, Item: value.Item, Reference: value.Ref, EvidenceFile: value.EvidenceFile})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Item == result[j].Item {
			if result[i].Reference == result[j].Reference {
				return result[i].EvidenceFile < result[j].EvidenceFile
			}
			return result[i].Reference < result[j].Reference
		}
		return result[i].Item < result[j].Item
	})
	return result
}

func storyRevisionParents(story *requirements.Story) map[string][]string {
	m := map[string][]string{}
	for _, r := range story.Revisions {
		m[r.Story+":revision:"+r.ID] = r.Parents
	}
	return m
}
func storyEventParents(sagaID string, story *requirements.Story) map[string][]string {
	m := map[string][]string{}
	for _, e := range story.Events {
		urn, _ := requirements.StoryEventURN(sagaID, story.Identity.ID, e.ID)
		m[urn] = e.Parents
	}
	return m
}
func graphDepths(parents map[string][]string) map[string]int {
	memo := map[string]int{}
	var depth func(string, map[string]bool) int
	depth = func(node string, seen map[string]bool) int {
		if v, ok := memo[node]; ok {
			return v
		}
		if seen[node] {
			return 0
		}
		seen[node] = true
		d := 0
		for _, p := range parents[node] {
			candidate := depth(p, seen) + 1
			if candidate > d {
				d = candidate
			}
		}
		delete(seen, node)
		memo[node] = d
		return d
	}
	for node := range parents {
		depth(node, map[string]bool{})
	}
	return memo
}
func resourceMatches(filter, id, urn string) bool {
	return filter == "" || filter == id || filter == urn
}
func urnID(value string) string {
	if i := strings.LastIndex(value, ":"); i >= 0 {
		return value[i+1:]
	}
	return value
}
func copyStrings(values []string) []string { return append([]string{}, values...) }
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
func uniquePaths(values [][]string) [][]string {
	seen := map[string]bool{}
	result := [][]string{}
	for _, value := range values {
		key := strings.Join(value, "\x00")
		if !seen[key] {
			seen[key] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return strings.Join(result[i], "\x00") < strings.Join(result[j], "\x00") })
	return result
}
