package requirements

// DefinitionMaturity describes the status of the term's meaning. It is
// independent of whether implementation evidence is available.
type DefinitionMaturity string

const (
	DefinitionMaturityUnknown  DefinitionMaturity = "unknown"
	DefinitionMaturityProposed DefinitionMaturity = "proposed"
	DefinitionMaturityAccepted DefinitionMaturity = "accepted"
)

// ImplementationEvidence describes how much evidence of implementation has
// been observed. Evidence availability is not proof that the concept is
// implemented, and Unknown is not an observed implementation gap.
type ImplementationEvidence string

const (
	ImplementationEvidenceUnknown ImplementationEvidence = "unknown"
	ImplementationEvidenceAbsent  ImplementationEvidence = "absent"
	ImplementationEvidencePartial ImplementationEvidence = "partial"
	ImplementationEvidencePresent ImplementationEvidence = "present"
)

// EffectiveDefinitionMaturity and EffectiveImplementationEvidence preserve
// old records: omitted metadata projects as unknown rather than being
// silently upgraded to accepted or implemented.
func (value TermRevision) EffectiveDefinitionMaturity() DefinitionMaturity {
	if value.DefinitionMaturity == nil {
		return DefinitionMaturityUnknown
	}
	return *value.DefinitionMaturity
}

func (value TermRevision) EffectiveImplementationEvidence() ImplementationEvidence {
	if value.ImplementationEvidence == nil {
		return ImplementationEvidenceUnknown
	}
	return *value.ImplementationEvidence
}

func validateTermSemantics(problems *validationErrors, value TermRevision) {
	if value.DefinitionMaturity != nil && *value.DefinitionMaturity != DefinitionMaturityUnknown && *value.DefinitionMaturity != DefinitionMaturityProposed && *value.DefinitionMaturity != DefinitionMaturityAccepted {
		problems.add("definition_maturity must be unknown, proposed, or accepted")
	}
	if value.ImplementationEvidence != nil && *value.ImplementationEvidence != ImplementationEvidenceUnknown && *value.ImplementationEvidence != ImplementationEvidenceAbsent && *value.ImplementationEvidence != ImplementationEvidencePartial && *value.ImplementationEvidence != ImplementationEvidencePresent {
		problems.add("implementation_evidence must be unknown, absent, partial, or present")
	}
}
