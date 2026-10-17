package quality

// DefinitionStage describes authored implementation links, not test execution.
// Planning remains on the owning feature; this is a derived observation only.
func DefinitionStage(test TestCase) string {
	if test.RevisionConflict() || test.LifecycleConflict() {
		return "conflicted"
	}
	stage := "planned"
	for _, evidence := range test.Evidence {
		if evidence.Role == EvidenceExecutionArtifact {
			continue
		}
		head := false
		for _, urn := range test.EvidenceHeads {
			if urn == evidence.TestCase+":evidence:"+evidence.ID {
				head = true
			}
		}
		if !head {
			continue
		}
		if !evidence.Current {
			return "stale_evidence"
		}
		if evidence.Role == EvidenceTestImplementation {
			stage = "implementation_linked"
		}
	}
	return stage
}
