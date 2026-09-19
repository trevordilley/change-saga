package livingapp

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/readiness"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
	"github.com/twentyideas/changesaga/internal/workplan"
)

func (s *session) requirementRows(filters Filters) []Requirement {
	rows := []Requirement{}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
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
		rows = append(rows, Requirement{
			Requirement: urn, ID: story.Identity.ID, CreatedAt: story.Identity.CreatedAt,
			RevisionHeads: copyStrings(story.RevisionHeads), LifecycleHeads: copyStrings(story.LifecycleHeads),
			CurrentRevision: story.CurrentRevision, CurrentLifecycle: story.CurrentLifecycle,
			RevisionConflict: story.RevisionConflict(), LifecycleConflict: story.LifecycleConflict(),
		})
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

func (s *session) waveRows(filters Filters) []Wave {
	rows := []Wave{}
	for _, id := range sortedKeys(s.plan.Waves) {
		wave := s.plan.Waves[id]
		urn, _ := livingid.Wave(s.plan.SagaID, id)
		if !resourceMatches(filters.Wave, id, urn) {
			continue
		}
		counts := map[string]int{"total": 0, "ready": 0, "blocked": 0, "planned": 0, "in_progress": 0, "done": 0, "cancelled": 0, "conflicted": 0}
		for _, itemID := range sortedKeys(s.plan.WorkItems) {
			item := s.plan.WorkItems[itemID]
			if item.CurrentRevision == nil || item.CurrentRevision.Wave != urn {
				continue
			}
			counts["total"]++
			row := s.projectWorkItem(itemID, item)
			if row.Conflicted {
				counts["conflicted"]++
			}
			if row.Ready {
				counts["ready"]++
			} else {
				counts["blocked"]++
			}
			if item.CurrentProgress != nil {
				counts[item.CurrentProgress.State]++
			}
		}
		rows = append(rows, Wave{Wave: urn, ID: id, Heads: copyStrings(wave.Heads), CurrentRevision: wave.CurrentRevision, Conflicted: len(wave.Heads) > 1, ItemCounts: counts})
	}
	sort.Slice(rows, func(i, j int) bool {
		li, lj := 0, 0
		if rows[i].CurrentRevision != nil {
			li = rows[i].CurrentRevision.Order
		}
		if rows[j].CurrentRevision != nil {
			lj = rows[j].CurrentRevision.Order
		}
		if li != lj {
			return li < lj
		}
		return rows[i].Wave < rows[j].Wave
	})
	return rows
}

func (s *session) workItemRows(filters Filters) []WorkItem {
	rows := []WorkItem{}
	for _, id := range sortedKeys(s.plan.WorkItems) {
		item := s.plan.WorkItems[id]
		urn, _ := livingid.WorkItem(s.plan.SagaID, id)
		if !resourceMatches(filters.Item, id, urn) {
			continue
		}
		if filters.Wave != "" && (item.CurrentRevision == nil || !resourceMatches(filters.Wave, urnID(item.CurrentRevision.Wave), item.CurrentRevision.Wave)) {
			continue
		}
		state := "conflicted"
		if item.CurrentProgress != nil {
			state = item.CurrentProgress.State
		}
		if filters.Status != "" && filters.Status != state {
			continue
		}
		rows = append(rows, s.projectWorkItem(id, item))
	}
	return rows
}

func (s *session) projectWorkItem(id string, item *workplan.WorkItem) WorkItem {
	urn, _ := livingid.WorkItem(s.plan.SagaID, id)
	blockers := s.directBlockers(urn)
	workspaces := activeWorkspaces(item)
	evidence := integratedEvidence(item)
	return WorkItem{WorkItem: urn, ID: id, Heads: copyStrings(item.Heads), CurrentRevision: item.CurrentRevision, CurrentProgress: item.CurrentProgress, Conflicted: len(item.Heads) > 1 || item.CurrentProgress == nil, Ready: len(blockers) == 0 && len(item.Heads) == 1 && item.CurrentProgress != nil, Blockers: blockers, ActiveWorkspaces: workspaces, MergeEvidence: evidence}
}

func (s *session) directBlockers(itemURN string) []DependencyBlocker {
	rows := []DependencyBlocker{}
	for _, id := range sortedKeys(s.plan.Dependencies) {
		dep := s.plan.Dependencies[id]
		if dep.Dependent != itemURN {
			continue
		}
		satisfied, reason := workplan.DependencySatisfied(s.plan, id)
		if satisfied {
			continue
		}
		urn, _ := livingid.Dependency(s.plan.SagaID, id)
		rows = append(rows, DependencyBlocker{Dependency: urn, Reason: reason, Path: []string{itemURN, dep.Prerequisite}})
	}
	return rows
}

func (s *session) workEventRows(filters Filters) []WorkEvent {
	rows := []WorkEvent{}
	for _, id := range sortedKeys(s.plan.WorkItems) {
		item := s.plan.WorkItems[id]
		itemURN, _ := livingid.WorkItem(s.plan.SagaID, id)
		if !resourceMatches(filters.Item, id, itemURN) {
			continue
		}
		for i := range item.Progress {
			e := &item.Progress[i]
			urn, _ := workplan.ProgressEventURN(s.plan.SagaID, id, e.ID)
			if filters.Kind == "" || filters.Kind == "progress" {
				rows = append(rows, WorkEvent{Kind: "progress", Resource: urn, Item: itemURN, CreatedAt: e.CreatedAt, Progress: e})
			}
		}
		for i := range item.Workspaces {
			e := &item.Workspaces[i]
			urn, _ := workplan.WorkspaceEventURN(s.plan.SagaID, id, e.ID)
			if filters.Kind == "" || filters.Kind == "workspace" {
				rows = append(rows, WorkEvent{Kind: "workspace", Resource: urn, Item: itemURN, CreatedAt: e.CreatedAt, Workspace: e})
			}
		}
		for i := range item.Merges {
			e := &item.Merges[i]
			urn, _ := workplan.MergeEventURN(s.plan.SagaID, id, e.ID)
			if filters.Kind == "" || filters.Kind == "merge" {
				rows = append(rows, WorkEvent{Kind: "merge", Resource: urn, Item: itemURN, CreatedAt: e.CreatedAt, Merge: e})
			}
		}
	}
	for _, id := range sortedKeys(s.plan.Contracts) {
		contract := s.plan.Contracts[id]
		for i := range contract.Events {
			e := &contract.Events[i]
			urn, _ := workplan.ContractEventURN(s.plan.SagaID, id, e.ID)
			if filters.Kind == "" || filters.Kind == "contract" {
				rows = append(rows, WorkEvent{Kind: "contract", Resource: urn, CreatedAt: e.CreatedAt, Contract: e})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		}
		return rows[i].Resource < rows[j].Resource
	})
	return rows
}

func (s *session) conflictRows(filters Filters) []Conflict {
	rows := []Conflict{}
	for _, conflict := range s.plan.Conflicts {
		if filters.Kind != "" && filters.Kind != conflict.Kind {
			continue
		}
		if filters.Item != "" && !resourceMatches(filters.Item, urnID(conflict.Resource), strings.Split(conflict.Resource, "#")[0]) {
			continue
		}
		if filters.Wave != "" && !s.conflictInWave(conflict, filters.Wave) {
			continue
		}
		heads := copyStrings(conflict.Heads)
		sort.Strings(heads)
		rows = append(rows, Conflict{ID: conflictID(conflict.Kind, conflict.Resource, heads), Kind: conflict.Kind, Severity: "blocking", Resource: conflict.Resource, Heads: heads})
	}
	return rows
}

func (s *session) conflictInWave(conflict workplan.Conflict, wave string) bool {
	id := urnID(strings.Split(conflict.Resource, "#")[0])
	item := s.plan.WorkItems[id]
	return item != nil && item.CurrentRevision != nil && resourceMatches(wave, urnID(item.CurrentRevision.Wave), item.CurrentRevision.Wave)
}

func conflictID(kind, resource string, heads []string) string {
	digest := sha256.Sum256([]byte(kind + "\x00" + resource + "\x00" + strings.Join(heads, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:])
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
		value.TransitiveBlockerPaths = r.TransitivePaths
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
	passingRuns := map[string]string{}
	for _, testCase := range s.quality.TestCases {
		if testCase.CurrentRun != nil && testCase.CurrentRun.Result == quality.RunPassed {
			testURN, _ := qualityid.TestCase(s.requirements.SagaID, testCase.Identity.ID)
			passingRuns[testURN], _ = qualityid.Run(s.requirements.SagaID, testCase.Identity.ID, testCase.CurrentRun.ID)
		}
	}
	inputs := []readiness.Criterion{}
	meta := []Traceability{}
	reviewEvidenceByTarget, _ := s.reviewEvidenceIndex()
	for _, story := range s.requirements.Stories {
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
			work := []string{}
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
				if r.Type != requirements.RelationImplements {
					continue
				}
				target := r.To
				if target == criterionURN || contains(design, target) {
					work = append(work, r.From)
					paths = append(paths, []string{criterionURN, target, r.From})
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
				// A verifying test case is evidence once its current run passed.
				if run, ok := passingRuns[r.From]; ok {
					evidence = append(evidence, r.From)
					paths = append(paths, []string{criterionURN, r.From, run})
				}
			}
			blockers := []readiness.Blocker{}
			transitive := []readiness.BlockerPath{}
			if len(design) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "design_missing", Resource: criterionURN})
			}
			if len(work) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "work_item_missing", Resource: criterionURN})
			}
			for _, itemURN := range uniqueSorted(work) {
				item := s.plan.WorkItems[urnID(itemURN)]
				if item == nil {
					blockers = append(blockers, readiness.Blocker{Code: "work_item_missing", Resource: itemURN})
					continue
				}
				merge := integratedEvidence(item)
				evidence = append(evidence, merge...)
				for _, ev := range merge {
					paths = append(paths, []string{criterionURN, itemURN, ev})
				}
				transitive = append(transitive, s.transitiveBlockers(itemURN)...)
				for _, conflict := range s.plan.Conflicts {
					if strings.HasPrefix(conflict.Resource, itemURN) {
						blockers = append(blockers, readiness.Blocker{Code: "work_conflict", Resource: conflict.Resource, Detail: conflict.Kind})
					}
				}
			}
			if len(evidence) == 0 {
				blockers = append(blockers, readiness.Blocker{Code: "immutable_evidence_missing", Resource: criterionURN, Detail: "progress is not delivery evidence"})
			}
			inputs = append(inputs, readiness.Criterion{URN: criterionURN, Designed: len(design) > 0, Planned: len(work) > 0, Evidence: evidence, DirectBlockers: blockers, UpstreamPaths: transitive})
			meta = append(meta, Traceability{Criterion: criterionURN, Story: storyURN, Revision: revisionURN, Design: uniqueSorted(design), BroadDesign: uniqueSorted(broadDesign), WorkItems: uniqueSorted(work), ReviewTargets: uniqueSorted(reviewTargets), CodeEvidence: uniqueSorted(codeEvidence), Paths: uniquePaths(paths)})
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

func (s *session) transitiveBlockers(start string) []readiness.BlockerPath {
	result := []readiness.BlockerPath{}
	var visit func(string, []string, map[string]bool)
	visit = func(item string, path []string, seen map[string]bool) {
		if seen[item] {
			return
		}
		nextSeen := map[string]bool{}
		for k, v := range seen {
			nextSeen[k] = v
		}
		nextSeen[item] = true
		found := false
		for _, id := range sortedKeys(s.plan.Dependencies) {
			dep := s.plan.Dependencies[id]
			if dep.Dependent != item {
				continue
			}
			satisfied, reason := workplan.DependencySatisfied(s.plan, id)
			if satisfied {
				continue
			}
			found = true
			nextPath := append(append([]string{}, path...), dep.Prerequisite)
			result = append(result, readiness.BlockerPath{Path: nextPath, Blocker: readiness.Blocker{Code: reason, Resource: dep.Prerequisite}})
			visit(dep.Prerequisite, nextPath, nextSeen)
		}
		_ = found
	}
	visit(start, []string{start}, map[string]bool{})
	return result
}

func integratedEvidence(item *workplan.WorkItem) []string {
	// Only the current head of each merge-unit event graph is evidence. An
	// earlier integrated event followed by a revert must not remain delivered.
	parents := map[string]bool{}
	for _, event := range item.Merges {
		for _, parent := range event.Parents {
			parents[parent] = true
		}
	}
	values := []string{}
	for _, event := range item.Merges {
		urn := event.WorkItem + ":merge-event:" + event.ID
		if !parents[urn] && event.State == "integrated" && event.MergeOID != "" {
			values = append(values, "git-oid:"+strings.ToLower(event.MergeOID))
		}
	}
	return uniqueSorted(values)
}
func activeWorkspaces(item *workplan.WorkItem) []workplan.Workspace {
	parents := map[string]bool{}
	for _, event := range item.Workspaces {
		for _, parent := range event.Parents {
			parents[parent] = true
		}
	}
	values := []workplan.Workspace{}
	for _, event := range item.Workspaces {
		urn := event.WorkItem + ":workspace-event:" + event.ID
		if !parents[urn] && event.Action == "assigned" {
			values = append(values, event.Workspace)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values
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
