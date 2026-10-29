package applayout

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const FeaturePlanName = "plan.json"
const FeaturePlanSchemaURL = "https://changesaga.dev/schema/v5/feature-plan.schema.json"

// FeaturePlan is editable product planning metadata, independent of immutable
// feature identity and requirement acceptance. Git preserves its history.
type FeaturePlan struct {
	Schema    string   `json:"$schema,omitempty"`
	Version   int      `json:"version,omitempty"`
	Status    string   `json:"status"`
	Milestone string   `json:"milestone,omitempty"`
	Priority  int      `json:"priority,omitempty"`
	Labels    []string `json:"labels,omitempty"`
	// Archived hides the feature from the reviewer's feature lists and from
	// coverage gaps without touching its records. Status is kept, so restoring
	// returns the feature to the Roadmap or Features it left.
	Archived       bool   `json:"archived,omitempty"`
	ArchivedReason string `json:"archived_reason,omitempty"`
	// Augments names the existing feature a Roadmap feature extends: its
	// stories are planned work feature deliver moves into that feature.
	Augments string `json:"augments,omitempty"`
	// OverviewRevisionPending lists deliveries the feature's overview essay
	// has not yet been revised for. feature deliver records one; feature
	// set-overview clears them all.
	OverviewRevisionPending []OverviewRevision `json:"overview_revision_pending,omitempty"`
}

// OverviewRevision is one delivery the recipient's overview must absorb: the
// Roadmap feature delivered and the story URNs it brought.
type OverviewRevision struct {
	Feature string   `json:"feature"`
	Stories []string `json:"stories"`
}

var featureURNPattern = regexp.MustCompile(`^urn:change-saga:[^:]+:feature:[^:]+$`)
var storyURNPattern = regexp.MustCompile(`^urn:change-saga:[^:]+:story:[^:]+$`)

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
	if p.ArchivedReason != "" && !p.Archived {
		return fmt.Errorf("archived_reason requires archived")
	}
	if len(p.ArchivedReason) > 1000 || strings.ContainsAny(p.ArchivedReason, "\r\n") {
		return fmt.Errorf("archived_reason must be a single line of at most 1000 bytes")
	}
	if p.Augments != "" && !featureURNPattern.MatchString(p.Augments) {
		return fmt.Errorf("augments must be a feature URN")
	}
	if len(p.OverviewRevisionPending) > 1000 {
		return fmt.Errorf("a feature can have at most 1000 pending overview revisions")
	}
	for _, pending := range p.OverviewRevisionPending {
		if !featureURNPattern.MatchString(pending.Feature) {
			return fmt.Errorf("overview_revision_pending features must be feature URNs")
		}
		if len(pending.Stories) > 10000 {
			return fmt.Errorf("a pending overview revision can name at most 10000 stories")
		}
		for _, story := range pending.Stories {
			if !storyURNPattern.MatchString(story) {
				return fmt.Errorf("overview_revision_pending stories must be story URNs")
			}
		}
	}
	if len(p.Labels) > 100 {
		return fmt.Errorf("a feature can have at most 100 labels")
	}
	seen := map[string]bool{}
	for _, label := range p.Labels {
		if label == "" || label != strings.TrimSpace(label) || len(label) > 1000 || strings.ContainsAny(label, "\r\n\t") {
			return fmt.Errorf("labels must be nonempty canonical references of at most 1000 bytes")
		}
		if seen[label] {
			return fmt.Errorf("duplicate feature label %q", label)
		}
		seen[label] = true
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
