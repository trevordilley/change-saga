package livingapp

import (
	"sort"

	"github.com/twentyideas/changesaga/internal/impact"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/sagaref"
)

// ImpactGraph projects the review repository onto source evidence owners so an
// incoming source change can be carried back to the stories, criteria, and test
// cases it implicates. Only persisted edges are followed: active addresses and
// explains relations from an owner (or from a deck or slide whose scope reaches
// its Items) and active verifies relations from a test case. Superseded test
// evidence is not followed; a stale-but-unsuperseded record still is, because
// the code it names is still what that record claims.
func ImpactGraph(in StatusInputs) impact.Graph {
	a := &assembler{in: in, storyRevision: map[string]string{}, byCriterion: map[string]*criterionFrame{}, visual: map[string]visualTarget{}}
	a.indexVisual()
	graph := impact.Graph{Requirements: map[string][]impact.Reach{}, TestCases: []impact.TestCaseEvidence{}}
	links := append([]Link(nil), in.Links...)
	sort.Slice(links, func(i, j int) bool { return links[i].URN < links[j].URN })
	verifies := map[string][]string{}
	for _, link := range links {
		if !link.Active {
			continue
		}
		switch link.Type {
		case requirements.RelationVerifies:
			verifies[link.From] = append(verifies[link.From], link.To)
		case requirements.RelationAddresses, requirements.RelationExplains:
			graph.Requirements[link.From] = append(graph.Requirements[link.From], impact.Reach{Requirement: link.To, Relation: link.URN, Path: []string{link.From, link.To}})
			visual, ok := a.visual[link.From]
			if !ok || visual.kind == sagaref.TargetItem || link.Scope != scopeDescendants {
				continue
			}
			for _, item := range visual.items {
				graph.Requirements[item.Target] = append(graph.Requirements[item.Target], impact.Reach{
					Requirement: link.To, Relation: link.URN, Path: append(a.itemPath(link.From, item), link.To),
				})
			}
		}
	}
	for _, testCase := range in.Quality.TestCases {
		urn := testCaseURN(in.SagaID, testCase.Identity.ID)
		heads := map[string]bool{}
		for _, head := range testCase.EvidenceHeads {
			heads[head] = true
		}
		for _, evidence := range testCase.Evidence {
			evidenceURN, _ := qualityid.Evidence(in.SagaID, testCase.Identity.ID, evidence.ID)
			if !heads[evidenceURN] || len(evidence.Diffs) == 0 || evidence.Role == quality.EvidenceExecutionArtifact {
				continue
			}
			graph.TestCases = append(graph.TestCases, impact.TestCaseEvidence{
				TestCase: urn, Evidence: evidenceURN, Role: string(evidence.Role), Diffs: copyStrings(evidence.Diffs), Criteria: uniqueSorted(verifies[urn]),
			})
		}
	}
	return graph
}
