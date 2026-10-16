package applayout

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const FeaturePlanName = "plan.json"
const FeaturePlanSchemaURL = "https://changesaga.dev/schema/v5/feature-plan.schema.json"

// FeaturePlan is editable product planning metadata, independent of immutable
// feature identity and requirement acceptance. Git preserves its history.
type FeaturePlan struct {
	Schema    string `json:"$schema,omitempty"`
	Version   int    `json:"version,omitempty"`
	Status    string `json:"status"`
	Milestone string `json:"milestone,omitempty"`
	Priority  int    `json:"priority,omitempty"`
}

func (p FeaturePlan) Planned() bool { return p.Status == "planned" }
func (p FeaturePlan) State() string {
	if p.Status == "" {
		return "built"
	}
	return p.Status
}
func (p FeaturePlan) Validate() error {
	if p.Schema != FeaturePlanSchemaURL || p.Version != 5 {
		return fmt.Errorf("feature plan requires its v5 schema and version")
	}
	if p.Status != "planned" && p.Status != "built" {
		return fmt.Errorf("feature status must be planned or built")
	}
	if p.Priority < 0 || p.Priority > 1000000 {
		return fmt.Errorf("priority must be between 0 (unranked) and 1000000; lower positive numbers come first")
	}
	if len(p.Milestone) > 1000 || strings.ContainsAny(p.Milestone, "\r\n") {
		return fmt.Errorf("milestone must be a single label of at most 1000 bytes")
	}
	return nil
}

func readFeaturePlan(dir string) (FeaturePlan, error) {
	p := FeaturePlan{Status: "built"}
	path := filepath.Join(dir, FeaturePlanName)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return p, nil
	} else if err != nil {
		return p, err
	}
	if err := ReadStrictJSON(path, &p); err != nil {
		return p, err
	}
	return p, p.Validate()
}

// PriorityBefore keeps unranked work after numbered priorities and ties stable.
func PriorityBefore(a, b int) bool {
	if a == b {
		return false
	}
	if a == 0 {
		return false
	}
	if b == 0 {
		return true
	}
	return a < b
}
