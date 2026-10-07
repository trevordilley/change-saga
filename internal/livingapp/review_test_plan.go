package livingapp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/coderef"
	"github.com/twentyideas/changesaga/internal/coderesolve"
	"github.com/twentyideas/changesaga/internal/coverage"
	"github.com/twentyideas/changesaga/internal/gitdiff"
	"github.com/twentyideas/changesaga/internal/inventoryview"
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/reviewstate"
	"github.com/twentyideas/changesaga/internal/saga"
)

// ReviewTestPlan selects tests, never executes them or records a passing run.
// Complete means the recorded mapping has no reported gaps, not that these
// tests prove the change correct or cover undeclared dependencies.
type ReviewTestPlan struct {
	Schema   string            `json:"schema"`
	Snapshot string            `json:"snapshot"`
	Review   string            `json:"review"`
	Range    reviewstate.Range `json:"range"`
	Strategy string            `json:"strategy"`
	Complete bool              `json:"complete"`
	Stories  []TestPlanStory   `json:"stories"`
	Tests    []PlannedTest     `json:"tests"`
	Commands []string          `json:"commands"`
	Gaps     []TestPlanGap     `json:"gaps"`
}

type TestPlanStory struct {
	Story string     `json:"story"`
	Title string     `json:"title"`
	Via   [][]string `json:"via"`
}

type PlannedTest struct {
	TestCase   string             `json:"test_case"`
	Revision   string             `json:"revision"`
	Title      string             `json:"title"`
	Automation quality.Automation `json:"automation"`
	Stories    []string           `json:"stories"`
	Criteria   []string           `json:"criteria"`
	Code       []coderef.Location `json:"code"`
	Commands   []TestPlanCommand  `json:"commands"`
	Reasons    []string           `json:"reasons"`
}

// Commands are recorded invocations from this test revision, not inferred
// from filenames. Run identifies the provenance; selection does not execute it.
type TestPlanCommand struct {
	Command string `json:"command"`
	Run     string `json:"run"`
}

type TestPlanGap struct {
	Code    string `json:"code"`
	Target  string `json:"target"`
	Message string `json:"message"`
}

func PlanReviewTests(ctx context.Context, root, checkout, reviewID string) (ReviewTestPlan, error) {
	opened, err := Open(ctx, OpenOptions{SagaRoot: root})
	if err != nil {
		return ReviewTestPlan{}, err
	}
	s := opened.(*session)
	review := s.saga.FindReview(reviewID)
	if review == nil {
		return ReviewTestPlan{}, fmt.Errorf("review %q was not found", reviewID)
	}
	if checkout == "" {
		checkout = s.saga.Root
	}
	rng, err := reviewstate.ResolveRange(ctx, checkout, review)
	if err != nil {
		return ReviewTestPlan{}, err
	}
	changes, err := gitdiff.ReadWithOptions(ctx, checkout, s.saga.Manifest.Source.Repository, rng.BaseOID, rng.HeadOID, gitdiff.ReadOptions{AllowRepositoryMismatch: true})
	if err != nil {
		return ReviewTestPlan{}, err
	}
	resolver, err := coderesolve.New(ctx, checkout)
	if err != nil {
		return ReviewTestPlan{}, err
	}
	defer resolver.Close()
	inv, err := requirements.LoadInventory(s.saga.Root, s.saga.Manifest.ID)
	if err != nil {
		return ReviewTestPlan{}, err
	}
	// A deletion may be represented only by a selection valid at the base.
	inherited, _ := inventoryview.InheritedReferences(ctx, s.saga, &inv, rng.HeadOID, resolver)
	baseSelections, _ := inventoryview.InheritedReferences(ctx, s.saga, &inv, rng.BaseOID, resolver)
	inherited = append(inherited, baseSelections...)
	result := s.reviewTestPlan(ctx, review, rng, changes, resolver, inherited)
	after, err := snapshotTree(s.saga.Root)
	if err != nil {
		return ReviewTestPlan{}, err
	}
	if after != s.snapshot {
		return ReviewTestPlan{}, fmt.Errorf("Saga changed while selecting tests; retry the command")
	}
	return result, ctx.Err()
}

func (s *session) reviewTestPlan(ctx context.Context, review *saga.Review, rng reviewstate.Range, changes gitdiff.ChangeSet, resolver coverage.Resolver, inherited []coverage.InheritedReference) ReviewTestPlan {
	result := ReviewTestPlan{Schema: "change-saga.test-plan/v1", Snapshot: s.snapshot, Review: review.Target, Range: rng,
		Strategy: "all automated and hybrid tests linked to affected stories, plus tests whose own code changed; recorded dependencies only",
		Stories:  []TestPlanStory{}, Tests: []PlannedTest{}, Commands: []string{}, Gaps: []TestPlanGap{}}
	gaps := map[TestPlanGap]bool{}
	gap := func(code, target, message string) { gaps[TestPlanGap{code, target, message}] = true }
	links := LinksFromCurrency(s.requirements, s.currency)
	decks := append([]*saga.Deck{}, s.saga.Decks...)
	if review.Deck != nil {
		decks = append(decks, review.Deck)
	}
	graph := ImpactGraph(StatusInputs{SagaID: s.requirements.SagaID, Decks: decks, Stories: s.requirements.Stories, Links: links, Quality: s.quality})
	linkByURN := map[string]Link{}
	for _, link := range links {
		linkByURN[link.URN] = link
	}
	stories := map[string]*requirements.Story{}
	for i := range s.requirements.Stories {
		story := &s.requirements.Stories[i]
		urn, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		stories[urn] = story
	}
	storyOf := func(target string) string {
		ref, err := livingid.Parse(target)
		if err != nil || ref.SagaID != s.requirements.SagaID {
			return ""
		}
		if ref.Kind == livingid.KindStory {
			return target
		}
		if ref.Kind == livingid.KindCriterion {
			urn, _ := livingid.Story(ref.SagaID, ref.ParentID)
			return urn
		}
		return ""
	}
	affected := map[string]*TestPlanStory{}
	addStory := func(target string, path []string) bool {
		urn := storyOf(target)
		if urn == "" {
			return false
		}
		story := stories[urn]
		if story == nil {
			gap("missing_story", urn, "An affected code owner refers to a missing story.")
			return false
		}
		row := affected[urn]
		if row == nil {
			row = &TestPlanStory{Story: urn, Via: [][]string{}}
			if story.CurrentRevision != nil {
				row.Title = story.CurrentRevision.Title
			} else {
				gap("conflicted_story", urn, "Resolve the story revision heads before trusting the plan.")
			}
			if story.CurrentLifecycle == nil || story.CurrentLifecycle.State == requirements.StateRetired {
				gap("unresolved_story_lifecycle", urn, "The affected story is retired or has unresolved lifecycle heads.")
			}
			affected[urn] = row
		}
		if target != urn {
			path = append(append([]string{}, path...), urn)
		}
		key := strings.Join(path, "\x00")
		for _, old := range row.Via {
			if strings.Join(old, "\x00") == key {
				return true
			}
		}
		row.Via = append(row.Via, path)
		return true
	}
	tests := map[string]*quality.TestCase{}
	verifies := map[string][]Link{}
	for _, link := range links {
		if link.Active && link.Type == requirements.RelationVerifies {
			verifies[link.From] = append(verifies[link.From], link)
		}
	}
	for i := range s.quality.TestCases {
		test := &s.quality.TestCases[i]
		urn, _ := qualityid.TestCase(s.requirements.SagaID, test.Identity.ID)
		tests[urn] = test
	}
	// Resolve evidence on both sides of the comparison. Only this review is walked;
	// unrelated review decks cannot introduce stories into its plan.
	ownership := testPlanOwnership(ctx, func(visit func(string, []saga.CodeFile)) {
		coverage.WalkDocumentCode(s.saga, visit)
		if review.Deck != nil {
			for _, slide := range review.Deck.Slides {
				for _, item := range slide.Items {
					visit(item.Target, item.Code)
				}
			}
		}
		for _, ref := range inherited {
			visit(ref.Item, []saga.CodeFile{{References: []coderef.Reference{ref.Reference}}})
		}
		for urn, test := range tests {
			for _, evidence := range test.Evidence {
				eid, _ := qualityid.Evidence(s.requirements.SagaID, test.Identity.ID, evidence.ID)
				if contains(test.EvidenceHeads, eid) && evidence.Role != quality.EvidenceExecutionArtifact {
					visit(urn, []saga.CodeFile{{Path: eid, References: evidence.Code}})
				}
			}
		}
	}, changes, resolver)
	owners := map[string]bool{}
	for _, assignments := range ownership {
		for _, owner := range assignments {
			owners[owner.Target] = true
		}
	}
	ownerMapped := map[string]bool{}
	directTests := map[string]bool{}
	for _, owner := range sortedKeys(owners) {
		for _, reach := range graph.Requirements[owner] {
			if addStory(reach.Requirement, reach.Path) {
				ownerMapped[owner] = true
			}
			if link := linkByURN[reach.Relation]; link.Currency != requirements.CurrencyCurrent {
				gap("stale_story_link", reach.Relation, "The code-to-story link needs confirmation; its story is included conservatively.")
			}
		}
		if tests[owner] != nil {
			directTests[owner] = true
			for _, link := range verifies[owner] {
				if addStory(link.To, []string{owner, link.To}) {
					ownerMapped[owner] = true
				}
			}
		}
	}
	if review.Deck != nil {
		for _, slide := range review.Deck.Slides {
			for _, item := range slide.Items {
				if !owners[item.Target] {
					continue
				}
				if addStory(item.Record, []string{item.Target, item.Record}) {
					ownerMapped[item.Target] = true
				}
				for _, link := range item.CriterionLinks {
					if addStory(link.Criterion, []string{item.Target, link.Criterion}) {
						ownerMapped[item.Target] = true
					}
					if story := stories[storyOf(link.Criterion)]; story != nil && story.CurrentRevision != nil {
						current, _ := livingid.Revision(s.requirements.SagaID, story.Identity.ID, story.CurrentRevision.ID)
						if current != link.StoryRevision {
							gap("stale_story_link", item.Target, "The review Item pins an earlier story revision; its story is included conservatively.")
						}
					}
				}
			}
		}
	}
	// Group unmapped changes by path, keeping the plan compact on large reviews.
	for _, atom := range changes.Atoms {
		mapped := false
		for _, owner := range ownership[atom.Key] {
			mapped = mapped || ownerMapped[owner.Target]
		}
		if !mapped {
			gap("unmapped_change", atom.Path, "Some changed code has no recorded path to a user story; the plan may omit tests.")
		}
	}
	selectedByStory := map[string]bool{}
	selectedByCriterion := map[string]bool{}
	for _, urn := range sortedKeys(tests) {
		test := tests[urn]
		storySet, criteria := []string{}, []string{}
		for _, link := range verifies[urn] {
			if story := storyOf(link.To); affected[story] != nil {
				storySet = append(storySet, story)
				criteria = append(criteria, link.To)
			}
		}
		storySet, criteria = uniqueSorted(storySet), uniqueSorted(criteria)
		if len(storySet) == 0 && !directTests[urn] {
			continue
		}
		if test.CurrentRevision == nil || test.CurrentLifecycle == nil {
			gap("conflicted_test", urn, "Resolve the test definition and lifecycle heads before selecting it.")
			continue
		}
		if test.CurrentLifecycle.State == quality.StateRetired || test.CurrentLifecycle.State == quality.StateDeprecated {
			continue
		}
		if test.CurrentRevision.Automation == quality.AutomationManual {
			gap("manual_test", urn, "An affected story has a manual test; it is not in the automated list.")
			continue
		}
		revision, _ := qualityid.Revision(s.requirements.SagaID, test.Identity.ID, test.CurrentRevision.ID)
		row := PlannedTest{TestCase: urn, Revision: revision, Title: test.CurrentRevision.Title, Automation: test.CurrentRevision.Automation,
			Stories: storySet, Criteria: criteria, Code: []coderef.Location{}, Commands: []TestPlanCommand{}, Reasons: []string{}}
		if len(storySet) > 0 {
			row.Reasons = append(row.Reasons, "verifies an affected story")
		}
		if directTests[urn] {
			row.Reasons = append(row.Reasons, "test evidence intersects the review's changed code")
		}
		if len(verifies[urn]) == 0 {
			gap("unlinked_test", urn, "The changed automated test has no user-story link.")
		}
		if row.Automation == quality.AutomationHybrid {
			gap("hybrid_test", urn, "This case also has manual steps; the recorded command may cover only its automated portion.")
		}
		for _, link := range verifies[urn] {
			if affected[storyOf(link.To)] != nil && link.Currency != requirements.CurrencyCurrent {
				gap("stale_test_link", link.URN, "Confirm this test-to-story link; the test is included conservatively.")
			} else if link.Currency == requirements.CurrencyCurrent {
				selectedByCriterion[link.To] = true
			}
		}
		codeSeen := map[string]bool{}
		for _, evidence := range test.Evidence {
			eid, _ := qualityid.Evidence(s.requirements.SagaID, test.Identity.ID, evidence.ID)
			if !contains(test.EvidenceHeads, eid) || evidence.Role != quality.EvidenceTestImplementation {
				continue
			}
			if evidence.TestRevision != revision {
				gap("stale_test_evidence", urn, "Test implementation evidence pins a different test revision.")
				continue
			}
			for _, ref := range evidence.Code {
				resolved := resolver.Resolve(ctx, ref, rng.HeadOID)
				if !resolved.Current() {
					gap("stale_test_code", urn, "Test implementation code cannot be resolved at the review head: "+ref.Location().String())
					continue
				}
				key := resolved.Location.String()
				if !codeSeen[key] {
					row.Code = append(row.Code, resolved.Location)
					codeSeen[key] = true
				}
			}
		}
		if len(row.Code) == 0 {
			gap("missing_test_code", urn, "No current test_implementation code reference is available.")
		}
		// Prefer the newest recorded invocation for this exact test revision.
		var latest *quality.Run
		for i := range test.Runs {
			run := &test.Runs[i]
			if run.TestRevision != revision || strings.TrimSpace(run.Command) == "" {
				continue
			}
			if latest == nil || run.ExecutedAt.After(latest.ExecutedAt) || run.ExecutedAt.Equal(latest.ExecutedAt) && run.ID < latest.ID {
				latest = run
			}
		}
		if latest != nil {
			runURN, _ := qualityid.Run(s.requirements.SagaID, test.Identity.ID, latest.ID)
			row.Commands = append(row.Commands, TestPlanCommand{latest.Command, runURN})
			result.Commands = append(result.Commands, latest.Command)
		} else {
			gap("missing_command", urn, "No run command is recorded for this test revision; use its code locations to choose an invocation.")
		}
		sort.Slice(row.Code, func(i, j int) bool { return row.Code[i].String() < row.Code[j].String() })
		for _, story := range storySet {
			selectedByStory[story] = true
		}
		result.Tests = append(result.Tests, row)
	}
	for _, urn := range sortedKeys(affected) {
		row := affected[urn]
		sort.Slice(row.Via, func(i, j int) bool { return strings.Join(row.Via[i], "\x00") < strings.Join(row.Via[j], "\x00") })
		result.Stories = append(result.Stories, *row)
		if !selectedByStory[urn] {
			gap("no_automated_tests", urn, "No automated test is linked to this affected story.")
		}
		if story := stories[urn]; story.CurrentRevision != nil {
			for _, criterion := range story.CurrentRevision.AcceptanceCriteria {
				id, _ := livingid.Criterion(s.requirements.SagaID, story.Identity.ID, criterion.ID)
				if !selectedByCriterion[id] {
					gap("criterion_without_automated_test", id, "No selected automated test has a current link to this criterion of the affected story.")
				}
			}
		}
	}
	for value := range gaps {
		result.Gaps = append(result.Gaps, value)
	}
	sort.Slice(result.Gaps, func(i, j int) bool {
		a, b := result.Gaps[i], result.Gaps[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Message < b.Message
	})
	result.Complete = len(result.Gaps) == 0
	result.Commands = uniqueSorted(result.Commands)
	return result
}

// Test selection uses intersection rather than explanatory diff coverage: a
// whole-file dependency includes changed lines, and a file event affects every
// dependency inside that file. Narrow ranges still leave unrelated changed
// lines unmapped. A rename can intersect evidence on either side.
func testPlanOwnership(ctx context.Context, walk func(func(string, []saga.CodeFile)), changes gitdiff.ChangeSet, resolver coverage.Resolver) map[string][]coverage.Assignment {
	byFile := map[string][]gitdiff.Atom{}
	for _, atom := range changes.Atoms {
		location := changes.Location(atom)
		key := location.Commit + "\x00" + location.Path
		byFile[key] = append(byFile[key], atom)
		if atom.Event == "rename" && atom.OldPath != "" {
			key = changes.BaseOID + "\x00" + atom.OldPath
			byFile[key] = append(byFile[key], atom)
		}
	}
	ownership := map[string][]coverage.Assignment{}
	walk(func(target string, files []saga.CodeFile) {
		for _, file := range files {
			for i, ref := range file.References {
				locations, _ := coverage.Sides(ctx, ref, changes, resolver)
				for _, resolved := range locations {
					location := resolved.Location
					for _, atom := range byFile[location.Commit+"\x00"+location.Path] {
						if atom.Kind == "event" || location.WholeFile() || atom.Line >= location.Start && atom.Line <= location.End {
							ownership[atom.Key] = append(ownership[atom.Key], coverage.Assignment{Target: target, EvidenceFile: file.Path, Reference: i + 1})
						}
					}
				}
			}
		}
	})
	return ownership
}
