package livingapp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/twentyideas/changesaga/internal/livingid"
	"github.com/twentyideas/changesaga/internal/requirements"
	"github.com/twentyideas/changesaga/internal/saga"
)

func (s *session) personaRows(filters Filters) ([]PersonaRecord, error) {
	selector := strings.TrimSpace(filters.Persona)
	var selectedID string
	if selector != "" {
		var err error
		selectedID, _, err = normalizeNamedRecord(s.requirements.SagaID, "persona", selector)
		if err != nil {
			return nil, err
		}
	}
	rows := []PersonaRecord{}
	for index := range s.requirements.Personas {
		persona := &s.requirements.Personas[index]
		if selectedID != "" && persona.Identity.ID != selectedID {
			continue
		}
		urn, _ := requirements.PersonaURN(s.requirements.SagaID, persona.Identity.ID)
		state := "conflicted"
		if persona.CurrentLifecycle != nil {
			state = string(persona.CurrentLifecycle.State)
		}
		rows = append(rows, PersonaRecord{
			Persona: urn, ID: persona.Identity.ID, CreatedAt: persona.Identity.CreatedAt, State: state,
			RevisionHeads: copyStrings(persona.RevisionHeads), LifecycleHeads: copyStrings(persona.LifecycleHeads),
			CurrentRevision: persona.CurrentRevision, CurrentLifecycle: persona.CurrentLifecycle,
			RevisionConflict: len(persona.RevisionHeads) != 1, LifecycleConflict: len(persona.LifecycleHeads) != 1,
		})
	}
	if selectedID != "" && len(rows) == 0 {
		return nil, appError(CodeNotFound, "persona was not found", false, map[string]any{"kind": "persona", "selector": selector}, nil)
	}
	return rows, nil
}

func (s *session) referenceResult(query Query) (Result, error) {
	kind := "persona"
	selector := query.Filters.Persona
	if query.Operation == "term-references" {
		kind, selector = "term", query.Filters.Term
	}
	id, subject, err := normalizeNamedRecord(s.requirements.SagaID, kind, strings.TrimSpace(selector))
	if err != nil {
		return Result{}, err
	}
	if (kind == "persona" && s.requirements.FindPersona(id) == nil) || (kind == "term" && s.requirements.FindTerm(id) == nil) {
		return Result{}, appError(CodeNotFound, kind+" was not found", false, map[string]any{"kind": kind, "selector": selector}, nil)
	}
	rows, completeness := s.recordReferences(kind, subject)
	counts := ReferenceCounts{Total: len(rows)}
	for _, row := range rows {
		if row.Direction == "incoming" {
			counts.Incoming++
		} else if row.Direction == "outgoing" {
			counts.Outgoing++
		}
	}
	start, end, page, err := s.page(query.Operation, normalizedQueryKey(query.Filters), query.Cursor, query.Limit, len(rows))
	if err != nil {
		return Result{}, err
	}
	conflictLimit := query.ConflictLimit
	if conflictLimit == 0 {
		conflictLimit = query.Limit
	}
	unresolvedStart, unresolvedEnd, unresolvedPage, err := s.page(query.Operation+"-conflicts", normalizedQueryKey(query.Filters), query.ConflictCursor, conflictLimit, len(completeness.UnresolvedOwners))
	if err != nil {
		return Result{}, err
	}
	completeness.UnresolvedOwners = completeness.UnresolvedOwners[unresolvedStart:unresolvedEnd]
	completeness.UnresolvedPage = unresolvedPage
	return Result{Data: ReferencePage{Subject: subject, References: rows[start:end], Counts: counts, Completeness: completeness}, Page: page}, nil
}

func normalizeNamedRecord(sagaID, kind, selector string) (string, string, error) {
	if selector == "" {
		return "", "", appError(CodeInvalidArgument, "a "+kind+" ID or canonical URN is required", false, map[string]any{"kind": kind}, nil)
	}
	if livingid.ValidID(selector) {
		var urn string
		if kind == "persona" {
			urn, _ = requirements.PersonaURN(sagaID, selector)
		} else {
			urn, _ = requirements.TermURN(sagaID, selector)
		}
		return selector, urn, nil
	}
	var id string
	var err error
	if kind == "persona" {
		id, err = requirements.ParsePersonaURN(sagaID, selector)
	} else {
		id, err = requirements.ParseTermURN(sagaID, selector)
	}
	if err != nil {
		return "", "", appError(CodeInvalidArgument, fmt.Sprintf("%s must be a stable %s ID or canonical URN in saga %q", selector, kind, sagaID), false,
			map[string]any{"kind": kind, "selector": selector, "expected": "ID or urn:change-saga:" + sagaID + ":" + kind + ":ID"}, err)
	}
	return id, selector, nil
}

func (s *session) recordReferences(kind, subject string) ([]RecordReference, ReferenceCompleteness) {
	completeness := ReferenceCompleteness{
		Complete: true, Scope: "direct incoming and outgoing explicit references at this snapshot",
		CoveredClasses: []string{
			"current story persona assignments", "current term applicability fields", "onboarding and review Item record fields",
			"canonical living relations, including complete-slide transaction relations", "current term code references",
		},
		ExcludedClasses: []ReferenceExclusion{
			{Class: "free_form_prose", Reason: "lexical text is not an explicit semantic reference"},
			{Class: "embedded_svg_text", Reason: "rendered labels are not explicit semantic references"},
			{Class: "historical_revisions", Reason: "the query reports current direct references; immutable history remains available through record history"},
			{Class: "transitive_references", Reason: "the named operation does not perform graph expansion"},
		},
		UnresolvedOwners: []ReferenceUnresolved{},
	}
	rows := []RecordReference{}
	if kind == "persona" {
		id, _ := requirements.ParsePersonaURN(s.requirements.SagaID, subject)
		if persona := s.requirements.FindPersona(id); persona != nil && persona.CurrentRevision == nil {
			completeness.UnresolvedOwners = append(completeness.UnresolvedOwners, ReferenceUnresolved{
				Owner: subject, Heads: copyStrings(persona.RevisionHeads), Reason: "the selected persona has competing current revisions",
			})
		}
	}
	add := func(row RecordReference) {
		if row.Source == subject {
			row.Direction = "outgoing"
		} else if row.Target == subject {
			row.Direction = "incoming"
		} else {
			return
		}
		rows = append(rows, row)
	}

	for index := range s.requirements.Stories {
		story := &s.requirements.Stories[index]
		storyURN, _ := livingid.Story(s.requirements.SagaID, story.Identity.ID)
		if story.CurrentRevision == nil {
			if kind == "persona" && conflictedStoryNamesPersona(story, subject) {
				completeness.UnresolvedOwners = append(completeness.UnresolvedOwners, ReferenceUnresolved{Owner: storyURN, Heads: copyStrings(story.RevisionHeads), Reason: "the story has competing current revisions"})
			}
			continue
		}
		owner, _ := livingid.Revision(s.requirements.SagaID, story.Identity.ID, story.CurrentRevision.ID)
		if kind == "persona" {
			for fieldIndex, persona := range story.CurrentRevision.Personas {
				if persona == subject {
					add(typedReference(storyURN, persona, "story_persona", owner, fmt.Sprintf("personas[%d]", fieldIndex)))
				}
			}
		}
	}

	for index := range s.requirements.Terms {
		term := &s.requirements.Terms[index]
		termURN, _ := requirements.TermURN(s.requirements.SagaID, term.Identity.ID)
		if term.CurrentRevision == nil {
			if conflictedTermTouches(term, subject) {
				completeness.UnresolvedOwners = append(completeness.UnresolvedOwners, ReferenceUnresolved{Owner: termURN, Heads: copyStrings(term.RevisionHeads), Reason: "the term has competing current revisions"})
			}
			continue
		}
		owner, _ := requirements.TermRevisionURN(s.requirements.SagaID, term.Identity.ID, term.CurrentRevision.ID)
		for fieldIndex, story := range term.CurrentRevision.Stories {
			if termURN == subject || story == subject {
				add(typedReference(termURN, story, "term_story", owner, fmt.Sprintf("stories[%d]", fieldIndex)))
			}
		}
		for fieldIndex, record := range term.CurrentRevision.Records {
			if termURN == subject || record == subject {
				add(typedReference(termURN, record, "term_record", owner, fmt.Sprintf("records[%d]", fieldIndex)))
			}
		}
		if termURN == subject {
			for fieldIndex := range term.CurrentRevision.Code {
				reference := term.CurrentRevision.Code[fieldIndex]
				row := typedReference(termURN, reference.Location().String(), "term_code", owner, fmt.Sprintf("code[%d]", fieldIndex))
				row.Code = &reference
				add(row)
			}
		}
	}

	for _, deck := range s.saga.Onboarding {
		appendDeckRecordReferences(deck, "onboarding_item", subject, add)
	}
	for _, review := range s.saga.Reviews {
		if review.Deck != nil {
			appendDeckRecordReferences(review.Deck, "review_item", subject, add)
		}
	}

	transactionOwners := s.transactionRelationOwners()
	for index, relation := range s.requirements.Relations {
		if relation.From != subject && relation.To != subject {
			continue
		}
		urn, _ := livingid.Relation(s.requirements.SagaID, relation.ID)
		class, resource := "relation_record", urn
		if owner := transactionOwners[relation.ID]; owner != "" {
			class, resource = "complete_slide_transaction", owner
		}
		row := RecordReference{Source: relation.From, Target: relation.To, Kind: string(relation.Type), Owner: urn,
			Selector: "from -> to", Provenance: ReferenceProvenance{Class: class, Resource: resource},
			Pins: &ReferencePins{FromRevision: relation.FromRevision, ToRevision: relation.ToRevision, FromContentDigest: relation.FromContentDigest, ToContentDigest: relation.ToContentDigest}}
		if index < len(s.currency) {
			currency := s.currency[index]
			row.Currency = &ReferenceCurrency{Status: currency.Status, Reasons: append([]requirements.CurrencyReason{}, currency.Reasons...), CarriedForward: append([]requirements.CarriedForward{}, currency.CarriedForward...)}
		}
		add(row)
	}

	completeness.Complete = len(completeness.UnresolvedOwners) == 0
	sort.Slice(completeness.UnresolvedOwners, func(i, j int) bool {
		return completeness.UnresolvedOwners[i].Owner < completeness.UnresolvedOwners[j].Owner
	})
	rows = deduplicateReferences(rows)
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		return referenceSortKey(left) < referenceSortKey(right)
	})
	return rows, completeness
}

func typedReference(source, target, kind, owner, selector string) RecordReference {
	return RecordReference{Source: source, Target: target, Kind: kind, Owner: owner, Selector: selector,
		Provenance: ReferenceProvenance{Class: "typed_field", Resource: owner}}
}

func appendDeckRecordReferences(deck *saga.Deck, class, subject string, add func(RecordReference)) {
	for _, slide := range deck.Slides {
		for _, item := range slide.Items {
			if item.Record != subject {
				continue
			}
			add(RecordReference{Source: item.Target, Target: item.Record, Kind: class, Owner: item.Target, Selector: "record",
				Provenance: ReferenceProvenance{Class: "typed_field", Resource: item.Target}})
		}
	}
}

func (s *session) transactionRelationOwners() map[string]string {
	owners := map[string]string{}
	for _, deck := range s.saga.Decks {
		for _, slide := range deck.Slides {
			for _, item := range slide.Items {
				for _, link := range item.CriterionLinks {
					owners[link.ID] = item.Target
				}
			}
		}
	}
	return owners
}

func conflictedStoryNamesPersona(story *requirements.Story, subject string) bool {
	heads := stringSet(story.RevisionHeads)
	for _, revision := range story.Revisions {
		urn, _ := livingid.Revision(revisionStorySaga(revision.Story), story.Identity.ID, revision.ID)
		if heads[urn] && contains(revision.Personas, subject) {
			return true
		}
	}
	return false
}

func revisionStorySaga(storyURN string) string {
	prefix := "urn:change-saga:"
	value := strings.TrimPrefix(storyURN, prefix)
	if value == storyURN {
		return ""
	}
	sagaID, _, _ := strings.Cut(value, ":story:")
	return sagaID
}

func conflictedTermTouches(term *requirements.Term, subject string) bool {
	heads := stringSet(term.RevisionHeads)
	for _, revision := range term.Revisions {
		urn := revision.Term + ":revision:" + revision.ID
		if !heads[urn] {
			continue
		}
		if revision.Term == subject || contains(revision.Stories, subject) || contains(revision.Records, subject) {
			return true
		}
	}
	return false
}

func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}

func deduplicateReferences(values []RecordReference) []RecordReference {
	seen := map[string]bool{}
	result := make([]RecordReference, 0, len(values))
	for _, value := range values {
		key := referenceSortKey(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func referenceSortKey(value RecordReference) string {
	return strings.Join([]string{value.Direction, value.Source, value.Target, value.Kind, value.Owner, value.Selector, value.Provenance.Class}, "\x00")
}
