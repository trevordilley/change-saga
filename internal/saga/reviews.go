package saga

import (
	"fmt"
	"strings"
)

// ValidateReviewerIdentity validates explicit provenance on a newly recorded
// review. A nil identity is accepted for compatibility with legacy records.
func ValidateReviewerIdentity(reviewer *ReviewerIdentity) error {
	if reviewer == nil {
		return nil
	}
	switch reviewer.Kind {
	case "human":
		if strings.TrimSpace(reviewer.Name) != "" || strings.TrimSpace(reviewer.Agent) != "" || strings.TrimSpace(reviewer.Model) != "" {
			return fmt.Errorf("human reviewer identity cannot include an AI reviewer name, agent, or model")
		}
	case "ai":
		if strings.TrimSpace(reviewer.Name) == "" || strings.TrimSpace(reviewer.Agent) == "" || strings.TrimSpace(reviewer.Model) == "" {
			return fmt.Errorf("AI reviewer identity requires reviewer name, agent, and model")
		}
	default:
		return fmt.Errorf("reviewer kind must be human or ai")
	}
	return nil
}
