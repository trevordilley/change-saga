package livingapp

import (
	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/quality"
	"github.com/twentyideas/changesaga/internal/qualityid"
	"github.com/twentyideas/changesaga/internal/requirements"
)

// QualityRecord exposes definitions and canonical trace references, never run outcomes.
// Definition is derived from recorded evidence pins; source-code currency is in status.
type QualityRecord struct {
	TestCase       string                  `json:"test_case"`
	Feature        string                  `json:"feature"`
	Definition     string                  `json:"definition"`
	RevisionHeads  []string                `json:"revision_heads"`
	LifecycleHeads []string                `json:"lifecycle_heads"`
	Revision       *quality.Revision       `json:"revision,omitempty"`
	Lifecycle      *quality.LifecycleEvent `json:"lifecycle,omitempty"`
	Verifies       []VerifiesLink          `json:"verifies"`
	Steps          []TestStepStatus        `json:"steps"`
	Evidence       []QualityEvidenceRecord `json:"evidence"`
}

// qualityResult keeps archived records behind an explicit history query.
func (s *session) qualityResult(query Query) (Result, error) {
	rows := []any{}
	for _, test := range s.quality.TestCases {
		urn, _ := qualityid.TestCase(s.quality.SagaID, test.Identity.ID)
		if query.Filters.Feature != "" && query.Filters.Feature != test.Feature {
			continue
		}
		if !resourceMatches(query.Filters.TestCase, test.Identity.ID, urn) {
			continue
		}
		if query.Operation == "quality-history" {
			rows = append(rows, map[string]any{"test_case": urn, "feature": test.Feature, "revisions": test.Revisions, "events": test.Events, "evidence": test.Evidence, "legacy_runs": test.Runs})
			continue
		}
		row := QualityRecord{TestCase: urn, Feature: test.Feature, Definition: quality.DefinitionStage(test), RevisionHeads: copyStrings(test.RevisionHeads), LifecycleHeads: copyStrings(test.LifecycleHeads), Revision: test.CurrentRevision, Lifecycle: test.CurrentLifecycle, Verifies: []VerifiesLink{}, Evidence: []QualityEvidenceRecord{}}
		for index, relation := range s.requirements.Relations {
			if relation.From != urn || relation.Type != requirements.RelationVerifies || relation.State != requirements.RelationActive {
				continue
			}
			currency := "current"
			reasons := relation.StaleReasons
			rid, _ := livingid.Relation(s.requirements.SagaID, relation.ID)
			if index < len(s.currency) {
				currency = string(s.currency[index].Status)
				reasons = nil
				for _, reason := range s.currency[index].Reasons {
					reasons = append(reasons, reason.Message)
				}
			} else if relation.Stale {
				currency = "stale"
			}
			row.Verifies = append(row.Verifies, VerifiesLink{Relation: rid, Criterion: relation.To, Currency: currency, Reasons: copyStrings(reasons)})
		}
		row.Steps = definitionSteps(test, row.Verifies)
		for _, evidence := range test.Evidence {
			eid, _ := qualityid.Evidence(s.quality.SagaID, test.Identity.ID, evidence.ID)
			if evidence.Role != quality.EvidenceExecutionArtifact && contains(test.EvidenceHeads, eid) {
				row.Evidence = append(row.Evidence, QualityEvidenceRecord{URN: eid, Evidence: evidence, Current: evidence.Current, StaleReasons: copyStrings(evidence.StaleReasons)})
			}
		}
		rows = append(rows, row)
	}
	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, len(rows))
	if err != nil {
		return Result{}, err
	}
	return Result{Data: map[string]any{"test_cases": rows[start:end]}, Page: page}, nil
}

// QualityEvidenceRecord adds derived revision currency without rewriting evidence.
type QualityEvidenceRecord struct {
	URN string `json:"evidence"`
	quality.Evidence
	Current      bool     `json:"current"`
	StaleReasons []string `json:"stale_reasons"`
}
