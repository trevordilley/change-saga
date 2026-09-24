// Package semanticgraph composes exact semantic links owned by other record
// formats into the canonical requirements relation graph.
package semanticgraph

import (
	"fmt"
	"sort"

	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

// ProjectSlideCriterionLinks appends the current complete-slide Item links to
// records.Relations. It is the sole projection of embedded transaction links;
// every GUI, query, traceability, audit, and currency consumer reads the same
// requirements.Relation values after this boundary.
func ProjectSlideCriterionLinks(document *saga.Saga, records *requirements.Document) error {
	if document == nil || records == nil {
		return fmt.Errorf("project slide criterion links requires saga and requirements documents")
	}
	seen := map[string]string{}
	for _, relation := range records.Relations {
		seen[relation.ID] = "persisted requirements relation"
	}
	for _, feature := range document.Features {
		for _, deck := range feature.Decks {
			for _, slide := range deck.Slides {
				for _, item := range slide.Items {
					for _, link := range item.CriterionLinks {
						if previous := seen[link.ID]; previous != "" {
							return fmt.Errorf("relation id %q is shared by %s and complete-slide Item %s", link.ID, previous, item.Target)
						}
						seen[link.ID] = "complete-slide Item " + item.Target
						records.Relations = append(records.Relations, requirements.Relation{
							Schema: requirements.V5RelationSchemaURL, Version: requirements.V5RelationVersion,
							ID: link.ID, Type: requirements.RelationExplains, From: item.Target, To: link.Criterion,
							Scope: requirements.ScopeSelf, Rationale: link.Rationale, ToRevision: link.StoryRevision,
							State: requirements.RelationActive, CreatedAt: slide.AuthoringCreatedAt, Feature: feature.ID,
						})
					}
				}
			}
		}
	}
	if len(records.Relations) > requirements.MaxRelations {
		return fmt.Errorf("the app has more than %d relations after complete-slide links are projected", requirements.MaxRelations)
	}
	sort.SliceStable(records.Relations, func(i, j int) bool { return records.Relations[i].ID < records.Relations[j].ID })
	currency := requirements.EvaluateRelations(*records, requirements.StaleInputs{})
	for index := range records.Relations {
		reasons := []string{}
		for _, reason := range currency[index].Reasons {
			reasons = append(reasons, reason.Message)
		}
		records.Relations[index].StaleReasons = reasons
		records.Relations[index].Stale = len(reasons) > 0
	}
	return nil
}
